package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/storagestate"
)

func TestSessionCommand(t *testing.T) {
	f := newFakeEnv(t, "linux")
	dir := filepath.Join(f.Home, ".vitral", "sessions")
	load := func(args []string) (config.Config, error) {
		cfg := config.Defaults(f.Home)
		if len(args) > 0 {
			return cfg, errorf("bad flag %s", args[0])
		}
		cfg.SessionsDir = dir
		return cfg, nil
	}
	for _, args := range [][]string{nil, {"save"}, {"save", "--profile"}, {"delete"}, {"rename", "x"}} {
		if err := SessionCommand(f.Env, args, load); err == nil || !strings.Contains(err.Error(), "use: vitral session") {
			t.Errorf("%v: %v", args, err)
		}
	}
	if err := SessionCommand(f.Env, []string{"save", "Portal"}, load); err == nil || !strings.Contains(err.Error(), `inválido "Portal"`) {
		t.Fatalf("%v", err)
	}
	if err := SessionCommand(f.Env, []string{"list", "--x"}, load); err == nil || !strings.Contains(err.Error(), "bad flag --x") {
		t.Fatalf("%v", err)
	}
	if err := SessionCommand(f.Env, []string{"list"}, load); err != nil || !strings.Contains(f.out.String(), "Nenhuma sessão salva em "+dir) {
		t.Fatalf("%v\n%s", err, f.out.String())
	}
	st := storagestate.Store{Dir: dir}
	if _, err := st.Save(storagestate.State{
		Name: "portal", Profile: "hml", Created: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
		Cookies: []storagestate.Cookie{{Name: "s", Value: "segredo"}}, Origins: []storagestate.Origin{{Origin: "https://p.test", LocalStorage: map[string]string{"t": "segredo"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Save(storagestate.State{Name: "vazia", Profile: "default", Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "quebrada.json"), "{")
	f.out.Reset()
	if err := SessionCommand(f.Env, []string{"list"}, load); err != nil {
		t.Fatal(err)
	}
	out := f.out.String()
	for _, want := range []string{"  portal: perfil hml, salva em ", "1 cookies, localStorage de: https://p.test", "  quebrada: arquivo ilegível", "localStorage de: nenhuma origem"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "segredo") {
		t.Fatal("list must never show the content")
	}
	if err := SessionCommand(f.Env, []string{"delete", "portal"}, load); err != nil || !strings.Contains(f.out.String(), "sessão portal apagada") {
		t.Fatalf("%v", err)
	}
	if err := SessionCommand(f.Env, []string{"delete", "portal"}, load); err == nil || !strings.Contains(err.Error(), `não há sessão "portal"`) {
		t.Fatalf("%v", err)
	}
	// O status mostra o perfil e quantas sessões há.
	f.out.Reset()
	cfg, _ := load(nil)
	Status(f.Env, cfg)
	if !strings.Contains(f.out.String(), "Perfil: default (") || !strings.Contains(f.out.String(), "Sessões salvas: 2 em "+dir) {
		t.Fatalf("%s", f.out.String())
	}
}

func errorf(format string, args ...any) error { return fmt.Errorf(format, args...) }
