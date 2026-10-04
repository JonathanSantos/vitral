package storagestate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	st := Store{Dir: dir}
	if list, err := st.List(); list != nil || err != nil {
		t.Fatalf("no dir: %v %v", list, err)
	}
	s := State{
		Name: "portal", Profile: "hml", Created: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
		Cookies: []Cookie{{Name: "s", Value: "v", Domain: ".p.test"}}, Origins: []Origin{{Origin: "https://p.test", LocalStorage: map[string]string{"k": "v"}}},
	}
	path, err := st.Save(s)
	if err != nil || path != filepath.Join(dir, "portal.json") {
		t.Fatal(path, err)
	}
	for p, want := range map[string]os.FileMode{path: 0o600, dir: 0o700} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v %v", p, info.Mode().Perm(), err)
		}
	}
	got, err := st.Load("portal")
	if err != nil || got.Version != 1 || got.Cookies[0].Value != "v" || got.Origins[0].LocalStorage["k"] != "v" {
		t.Fatalf("%+v %v", got, err)
	}
	sum := got.Summary(path)
	if sum.Name != "portal" || sum.Profile != "hml" || sum.Cookies != 1 || len(sum.Origins) != 1 || sum.Origins[0] != "https://p.test" || sum.Path != path {
		t.Fatalf("%+v", sum)
	}
	if _, err := st.Load("nada"); err == nil || !strings.Contains(err.Error(), "session not found: nada") {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ruim.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load("ruim"); err == nil || !strings.Contains(err.Error(), "invalid session file") {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nota.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "pasta.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	list, err := st.List()
	if err != nil || len(list) != 2 || list[0].Name != "portal" || list[1].Name != "ruim" || !list[1].Created.IsZero() {
		t.Fatalf("%+v %v", list, err)
	}
	if err := st.Delete("portal"); err != nil {
		t.Fatal(err)
	}
	if err := st.Delete("portal"); err == nil {
		t.Fatal("second delete must fail")
	}
	// Sem permissão de escrita no pai: erro claro.
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: filepath.Join(ro, "s")}).Save(s); err == nil {
		t.Fatal("read-only parent must fail")
	}
	if _, err := (Store{Dir: ro}).Save(s); err == nil {
		t.Fatal("read-only dir must fail")
	}
}

func TestOrigins(t *testing.T) {
	for raw, want := range map[string]string{
		"https://p.test/a?b": "https://p.test", "http://127.0.0.1:8080/x": "http://127.0.0.1:8080",
		"about:blank": "", "chrome://newtab": "", "file:///x": "", "%%": "", "https://": "",
	} {
		if got := OriginOf(raw); got != want {
			t.Errorf("OriginOf(%q) = %q, want %q", raw, got, want)
		}
	}
	got := CookieOrigins([]Cookie{{Domain: ".p.test"}, {Domain: "localhost"}, {Domain: "127.0.0.1", Secure: true}, {Domain: ""}})
	if strings.Join(got, " ") != "https://p.test http://localhost https://127.0.0.1" {
		t.Fatalf("%v", got)
	}
	if m := Merge([]string{"a", "b"}, []string{"b", "", "c"}); strings.Join(m, "") != "abc" {
		t.Fatalf("%v", m)
	}
}

func TestVisited(t *testing.T) {
	v := Visited{Dir: filepath.Join(t.TempDir(), "perfil")}
	if v.List() != nil {
		t.Fatal("empty")
	}
	if changed, err := v.Add(""); changed || err != nil {
		t.Fatal("empty origin")
	}
	for i := 0; i < maxVisited+5; i++ {
		if changed, err := v.Add("https://s" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + ".test/" + string(rune('a'+i/26))); !changed || err != nil {
			t.Fatal(i, changed, err)
		}
	}
	list := v.List()
	if len(list) != maxVisited {
		t.Fatalf("cap: %d", len(list))
	}
	if changed, _ := v.Add(list[0]); changed {
		t.Fatal("no repeats")
	}
	if err := os.WriteFile(filepath.Join(v.Dir, visitedFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v.List() != nil {
		t.Fatal("broken file reads as empty")
	}
	if info, err := os.Stat(filepath.Join(v.Dir, visitedFile)); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal(err)
	}
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := (Visited{Dir: filepath.Join(ro, "p")}).Add("https://x.test"); err == nil {
		t.Fatal("read-only must fail")
	}
}
