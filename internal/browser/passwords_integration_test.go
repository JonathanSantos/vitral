//go:build integration

package browser

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/config"
)

func integrationBrowser(t *testing.T) string {
	t.Helper()
	if p := os.Getenv(config.EnvBrowserPath); p != "" {
		return p
	}
	cache, _ := os.UserCacheDir()
	dir := filepath.Join(cache, "vitral-e2e", "browsers")
	cfg := config.Defaults(dir)
	cfg.BrowsersDir = dir
	inst, _ := NewInstaller(cfg, nil)
	if _, err := os.Stat(inst.Executable()); err == nil {
		return inst.Executable()
	}
	p, _ := launcher.LookPath()
	return p
}

// A migração de um perfil antigo do macOS: os cookies gravados no disco atravessam os dois lançamentos e a
// marca só aparece no fim. Aqui a "chave do sistema" é a mesma chave fixa (systemStore trocado), para
// o teste nunca tocar o Keychain; o que se testa é o caminho dos cookies e a marca.
func TestPasswordStoreMigration(t *testing.T) {
	defer func(origOS string, origStore func(*launcher.Launcher) *launcher.Launcher) {
		goos, systemStore = origOS, origStore
	}(goos, systemStore)
	goos = "darwin"
	var systemLaunches int
	systemStore = func(l *launcher.Launcher) *launcher.Launcher { systemLaunches++; return l }
	bin := integrationBrowser(t)
	profile := t.TempDir()

	// Perfil "antigo": um cookie que fica no disco e um de sessão, gravados com a chave fixa.
	expires := proto.TimeSinceEpoch(time.Now().Add(24 * time.Hour).Unix())
	err := withHeadless(bin, profile, func(l *launcher.Launcher) *launcher.Launcher { return l }, func(b *rod.Browser) error {
		return proto.StorageSetCookies{Cookies: []*proto.NetworkCookieParam{
			{Name: "login", Value: "ok", Domain: "portal.example", Path: "/", Expires: expires},
			{Name: "temp", Value: "x", Domain: "portal.example", Path: "/"},
		}}.Call(b)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasCookies(profile) {
		t.Fatal("the old profile should have a cookies file")
	}

	if UseMockKeychain(config.Config{PasswordStore: "system"}, bin, profile, func(msg string, args ...any) { t.Logf(msg, args...) }) {
		t.Fatal("migration should succeed")
	}
	if systemLaunches != 1 {
		t.Fatalf("one launch with the system key, got %d", systemLaunches)
	}
	if _, err := os.Stat(filepath.Join(profile, storeMarker)); err != nil {
		t.Fatalf("marker after migration: %v", err)
	}
	var names []string
	err = withHeadless(bin, profile, systemStore, func(b *rod.Browser) error {
		res, err := proto.StorageGetCookies{}.Call(b)
		for _, c := range res.Cookies {
			names = append(names, c.Name+"="+c.Value)
		}
		return err
	})
	// Com a mesma chave nos dois lados, os cookies originais continuam legíveis; o que importa é o de
	// login estar lá depois da regravação (numa migração de verdade, só os regravados abrem).
	if err != nil || !slices.Contains(names, "login=ok") {
		t.Fatalf("cookies after migration: %v %v", names, err)
	}
}
