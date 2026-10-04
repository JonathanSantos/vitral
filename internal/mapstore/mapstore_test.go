package mapstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/sitemap"
)

var t0 = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func sample(host string, seen time.Time) *sitemap.Map {
	m := sitemap.New(host)
	a := m.Visit(sitemap.Observe("http://"+host+"/a", "", "A", "", []sitemap.Item{{Role: "heading", Name: "A"}}), seen)
	b := m.Visit(sitemap.Observe("http://"+host+"/b", "", "B", "", []sitemap.Item{{Role: "heading", Name: "B"}}), seen)
	m.Transition(a, b, sitemap.Action{Kind: "back"}, seen)
	return m
}

func TestStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "maps")
	st := Store{Dir: dir, Now: func() time.Time { return t0 }}
	if got := st.Path("Portal.Test:8080"); got != filepath.Join(dir, "portal.test_8080.json") {
		t.Fatal(got)
	}
	empty, err := st.Load("p.test")
	if err != nil || empty.Host != "p.test" || len(empty.Screens) != 0 {
		t.Fatalf("%+v %v", empty, err)
	}
	if err := st.Save(sample("p.test", t0)); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{st.Path("p.test"): 0o600, dir: 0o700} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v %v", p, info, err)
		}
	}
	got, err := st.Load("p.test")
	if err != nil || len(got.Screens) != 2 || len(got.Edges) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	// O carregamento tira o que envelheceu.
	if err := st.Save(sample("velho.test", t0.Add(-100*24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	if old, _ := st.Load("velho.test"); len(old.Screens) != 0 || len(old.Edges) != 0 {
		t.Fatalf("aging: %+v", old)
	}
	// Export e import (para compartilhar com a equipe).
	to := filepath.Join(t.TempDir(), "equipe.json")
	if err := st.Export("p.test", to); err != nil {
		t.Fatal(err)
	}
	if err := st.Export("nada.test", to); err == nil || !strings.Contains(err.Error(), "there is no map for nada.test") {
		t.Fatalf("%v", err)
	}
	other := Store{Dir: filepath.Join(t.TempDir(), "b")}
	imported, err := other.Import(to)
	if err != nil || imported.Host != "p.test" || len(imported.Screens) != 2 {
		t.Fatalf("%+v %v", imported, err)
	}
	if _, err := os.Stat(other.Path("p.test")); err != nil {
		t.Fatal("import saves the map")
	}
	if _, err := other.Import(filepath.Join(dir, "nada.json")); err == nil {
		t.Fatal("missing file")
	}
	// Clear apaga; de novo não é erro.
	if err := st.Clear("p.test"); err != nil {
		t.Fatal(err)
	}
	if err := st.Clear("p.test"); err != nil {
		t.Fatal(err)
	}
	if m, _ := st.Load("p.test"); len(m.Screens) != 0 {
		t.Fatal("cleared")
	}
	if (Store{}).now().IsZero() {
		t.Fatal("default clock")
	}
}

func TestDecodeErrors(t *testing.T) {
	for in, want := range map[string]string{
		"{":                           "invalid map file",
		`{"version": 9, "host": "x"}`: "unsupported map version 9",
		`{"version": 1}`:              "no host",
	} {
		if _, err := Decode([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", in, err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: dir}).Load("x"); err == nil || !strings.Contains(err.Error(), "x.json") {
		t.Fatalf("%v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "d.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Dir: dir}).Load("d"); err == nil {
		t.Fatal("a directory is not a map")
	}
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := (Store{Dir: filepath.Join(ro, "m")}).Save(sample("p.test", t0)); err == nil {
		t.Fatal("read-only parent")
	}
	if err := (Store{Dir: ro}).Save(sample("p.test", t0)); err == nil {
		t.Fatal("read-only dir")
	}
	if err := (Store{Dir: ro}).Clear("p.test"); err != nil {
		t.Fatal("clearing a missing map is fine")
	}
	if err := os.WriteFile(filepath.Join(dir, "cheio.json"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()
	if err := (Store{Dir: dir}).Clear("cheio"); err == nil {
		t.Fatal("cannot remove in a read-only dir")
	}
}

func TestSaver(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	var errs []error
	s := &Saver{Store: Store{Dir: dir}, Delay: 20 * time.Millisecond, OnError: func(err error) { mu.Lock(); errs = append(errs, err); mu.Unlock() }}
	m := sample("p.test", t0)
	s.Schedule(m)
	s.Schedule(m) // debounce: uma gravação só
	if _, err := os.Stat(s.Store.Path("p.test")); err == nil {
		t.Fatal("must wait the delay")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(s.Store.Path("p.test")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("debounced save never happened")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Mudanças depois do Schedule não entram na gravação agendada (cópia na hora).
	s.Delay = time.Hour
	s.Schedule(m)
	m.Next = 99
	s.Flush()
	got, _ := s.Store.Load("p.test")
	if got.Next == 99 {
		t.Fatal("Schedule must snapshot the map")
	}
	s.Flush() // nada pendente
	// Erro de gravação vai para OnError.
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	s.Store.Dir = ro
	s.Schedule(m)
	s.Flush()
	mu.Lock()
	defer mu.Unlock()
	if len(errs) != 1 || errors.Is(errs[0], os.ErrNotExist) {
		t.Fatalf("%v", errs)
	}
}
