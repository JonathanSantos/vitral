package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/retouch"
)

func TestRetouchCommand(t *testing.T) {
	f := newFakeEnv(t, "linux")
	dir := filepath.Join(f.Home, ".vitral", "retoques")
	load := func(args []string) (config.Config, error) {
		cfg := config.Defaults(f.Home)
		if len(args) > 0 {
			return cfg, errors.New("bad flag " + args[0])
		}
		return cfg, nil
	}
	for _, args := range [][]string{nil, {"rename"}, {"delete"}, {"delete", "--x"}} {
		if err := RetouchCommand(f.Env, args, load); err == nil || !strings.Contains(err.Error(), "uso: vitral retoque") {
			t.Errorf("%v: %v", args, err)
		}
	}
	if err := RetouchCommand(f.Env, []string{"list", "--x"}, load); err == nil || !strings.Contains(err.Error(), "bad flag --x") {
		t.Fatalf("%v", err)
	}
	if err := RetouchCommand(f.Env, []string{"list"}, load); err != nil || !strings.Contains(f.out.String(), "Nenhum retoque salvo em "+dir) {
		t.Fatalf("%v %q", err, f.out.String())
	}
	store := retouch.Store{Dir: dir}
	for _, r := range []retouch.Retouch{
		{Name: "tema", Description: "Tema escuro", Matches: []string{"*://a.com/*"}, CSS: "a{}"},
		{Name: "fonte", Matches: []string{"<all_urls>"}, CSS: "b{}"},
	} {
		if _, err := store.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "quebrado"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "quebrado", "retoque.yaml"), []byte("name: [x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := RetouchCommand(f.Env, []string{"list"}, load); err != nil {
		t.Fatal(err)
	}
	want := "- fonte: <all_urls>\n- tema: *://a.com/* · Tema escuro\n- quebrado: com problema, não é carregado"
	if !strings.HasPrefix(f.out.String(), want) {
		t.Fatalf("list:\n%s\nwant prefix:\n%s", f.out.String(), want)
	}
	f.out.Reset()
	if err := RetouchCommand(f.Env, []string{"delete", "tema"}, load); err != nil || !strings.Contains(f.out.String(), "✓ retoque tema apagado") {
		t.Fatalf("%v %q", err, f.out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "tema")); !os.IsNotExist(err) {
		t.Fatalf("tema should be gone: %v", err)
	}
	if err := RetouchCommand(f.Env, []string{"delete", "tema"}, load); err == nil || !strings.Contains(err.Error(), `não há retoque chamado "tema"`) {
		t.Fatalf("%v", err)
	}
	// Pasta dos retoques que não dá para ler: o erro sobe.
	file := filepath.Join(f.Home, "arquivo")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	loadFile := func([]string) (config.Config, error) {
		cfg := config.Defaults(f.Home)
		cfg.RetouchesDir = file
		return cfg, nil
	}
	if err := RetouchCommand(f.Env, []string{"list"}, loadFile); err == nil {
		t.Fatal("a file as the retouches dir should fail")
	}
}
