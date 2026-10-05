//go:build integration

package app_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/retouch"
)

// killOpened fecha o navegador do vitral open e espera o perfil ficar livre.
func killOpened(t *testing.T, pid int, profile string) {
	t.Helper()
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(filepath.Join(profile, "SingletonLock")); os.IsNotExist(err) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// vitral open (docs/RETOQUES.md, 2.3 e 14.1): sem automação nem depuração (os sites veem um Chrome
// comum), com os retoques; e o perfil ocupado nos dois sentidos.
func TestVitralOpen(t *testing.T) {
	bin := testBrowserPath()
	requireCfT(t, bin)
	reports := make(chan string, 4)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/report" {
			reports <- r.URL.RawQuery
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<body><script>addEventListener('load', () => fetch('/report?wd=' + navigator.webdriver +
			'&bg=' + encodeURIComponent(getComputedStyle(document.body).backgroundColor)))</script></body>`))
	}))
	defer site.Close()

	home := t.TempDir()
	cfg := config.Defaults(home)
	cfg.Headless = true // a pessoa sempre tem janela; aqui, só para o teste não abrir janelas
	cfg.ProfileDir = filepath.Join(home, "profile")
	cfg.PasswordStore = "mock" // testes nunca tocam o Keychain
	store := retouch.Store{Dir: cfg.RetouchesDir}
	if _, err := store.Save(retouch.Retouch{Name: "fundo", Matches: []string{"*://127.0.0.1/*"}, CSS: "body { background: rgb(1, 2, 3) }"}); err != nil {
		t.Fatal(err)
	}
	dirs, err := retouch.Builder{Store: store}.Launch()
	if err != nil || len(dirs) != 1 {
		t.Fatalf("build: %v %v", dirs, err)
	}

	res, err := browser.OpenForPerson(cfg, bin, site.URL+"/", dirs)
	if err != nil || res.PID == 0 {
		t.Fatalf("vitral open: %+v %v", res, err)
	}
	defer killOpened(t, res.PID, cfg.ProfileDir)
	select {
	case got := <-reports:
		if got != "wd=false&bg=rgb(1%2C%202%2C%203)" {
			t.Fatalf("vitral open should look like a plain Chrome with the retouch: %s", got)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the page opened by vitral open did not report")
	}

	// A IA começa com o vitral open aberto no mesmo perfil: erro que pede para fechar a janela.
	busy, cleanup := newHarness(func(c *config.Config) { c.ProfileDir = cfg.ProfileDir; c.RetouchesDir = cfg.RetouchesDir })
	busy.fail(t, "open", map[string]any{"url": busy.url("/pages/form.html")}, "the person is using the profile")
	cleanup()
	killOpened(t, res.PID, cfg.ProfileDir)

	// vitral open com a IA usando o perfil sem janela: avisa, não abre nada.
	ai, cleanupAI := newHarness(func(c *config.Config) { c.ProfileDir = cfg.ProfileDir; c.RetouchesDir = cfg.RetouchesDir })
	defer cleanupAI()
	ai.ok(t, "open", map[string]any{"url": ai.url("/pages/form.html")})
	again, err := browser.OpenForPerson(cfg, bin, "", dirs)
	if err != nil || !again.Headless || again.PID != 0 {
		t.Fatalf("with the AI's headless browser on the profile, vitral open must only report it: %+v %v", again, err)
	}
}
