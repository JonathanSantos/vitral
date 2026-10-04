package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/mapstore"
	"github.com/JonathanSantos/vitral/internal/sitemap"
)

func TestMapCommand(t *testing.T) {
	f := newFakeEnv(t, "linux")
	dir := filepath.Join(f.Home, ".vitral", "maps")
	load := func(args []string) (config.Config, error) {
		cfg := config.Defaults(f.Home)
		if len(args) > 0 {
			return cfg, errorf("bad flag %s", args[0])
		}
		return cfg, nil
	}
	for _, args := range [][]string{nil, {"show"}, {"clear", "--x"}, {"export", "p.test"}, {"export", "p.test", "-o", "x"}, {"apagar"}} {
		if err := MapCommand(f.Env, args, load); err == nil || !strings.Contains(err.Error(), "use: vitral map") {
			t.Errorf("%v: %v", args, err)
		}
	}
	if err := MapCommand(f.Env, []string{"list", "--x"}, load); err == nil || !strings.Contains(err.Error(), "bad flag --x") {
		t.Fatal(err)
	}
	if err := MapCommand(f.Env, []string{"list"}, load); err != nil || !strings.Contains(f.out.String(), "Nenhum mapa de telas em "+dir) {
		t.Fatalf("%v %s", err, f.out.String())
	}
	m := sitemap.New("p.test")
	a := m.Visit(sitemap.Observe("https://p.test/a", "", "Início", "", []sitemap.Item{{Role: "heading", Name: "Início"}}), time.Now())
	b := m.Visit(sitemap.Observe("https://p.test/b", "", "Pedidos", "", []sitemap.Item{{Role: "heading", Name: "Pedidos"}}), time.Now())
	m.Transition(a, b, sitemap.Action{Kind: "back"}, time.Now())
	st := mapstore.Store{Dir: dir}
	if err := st.Save(m); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "ruim.json"), "{")
	f.out.Reset()
	if err := MapCommand(f.Env, []string{"list"}, load); err != nil {
		t.Fatal(err)
	}
	if out := f.out.String(); !strings.Contains(out, "  p.test: 2 telas, 1 transições") || !strings.Contains(out, "  ruim: arquivo ilegível") {
		t.Fatal(out)
	}
	f.out.Reset()
	if err := MapCommand(f.Env, []string{"show", "p.test"}, load); err != nil || !strings.Contains(f.out.String(), `[s1] "Início"`) || !strings.Contains(f.out.String(), "graph LR") {
		t.Fatalf("%v %s", err, f.out.String())
	}
	if err := MapCommand(f.Env, []string{"show", "nada.test"}, load); err == nil || !strings.Contains(err.Error(), "não há mapa de nada.test") {
		t.Fatal(err)
	}
	if err := MapCommand(f.Env, []string{"show", "ruim"}, load); err == nil {
		t.Fatal("broken map")
	}
	to := filepath.Join(f.Home, "equipe.json")
	if err := MapCommand(f.Env, []string{"export", "p.test", "--to", to}, load); err != nil || !strings.Contains(f.out.String(), "exportado para "+to) {
		t.Fatal(err)
	}
	if err := MapCommand(f.Env, []string{"export", "nada.test", "--to", to}, load); err == nil {
		t.Fatal("no map to export")
	}
	if err := MapCommand(f.Env, []string{"clear", "p.test"}, load); err != nil || !strings.Contains(f.out.String(), "mapa de p.test apagado") {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.Path("p.test")); err == nil {
		t.Fatal("cleared")
	}
	if err := MapCommand(f.Env, []string{"import", to}, load); err != nil || !strings.Contains(f.out.String(), "mapa de p.test importado: 2 telas") {
		t.Fatal(err)
	}
	if err := MapCommand(f.Env, []string{"import", filepath.Join(f.Home, "nada.json")}, load); err == nil {
		t.Fatal("missing file")
	}
	ro := filepath.Join(f.Home, "ro")
	if err := os.MkdirAll(filepath.Join(ro, "x.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ro, "x.json", "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MapCommand(f.Env, []string{"clear", "x"}, func([]string) (config.Config, error) {
		cfg := config.Defaults(f.Home)
		cfg.MapsDir = ro
		return cfg, nil
	}); err == nil {
		t.Fatal("a non-empty directory cannot be removed")
	}
}
