//go:build network

package install

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// Download real do Chrome for Testing (≈150 MB). Fora do CI padrão: go test -tags network ./internal/browser/install/
func TestRealDownload(t *testing.T) {
	platform, ok := Platform(runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("no build for this platform")
	}
	root := t.TempDir()
	inst := NewInstaller(root, platform, os.Getenv("VITRAL_BROWSER_MIRROR"), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if _, err := inst.Probe(ctx); err != nil {
		t.Fatalf("probe: %v", err)
	}
	exe, err := inst.Install(ctx)
	if err != nil {
		t.Fatal(err)
	}
	v, err := RunVersion(exe)
	if err != nil || v != PinnedVersion {
		t.Fatalf("installed version %q, err %v", v, err)
	}
	// O navegador baixado precisa abrir de verdade (no macOS, o .app depende dos symlinks do zip).
	l := launcher.New().Bin(exe).Headless(true).UserDataDir(t.TempDir()).Leakless(false)
	u, err := l.Launch()
	if err != nil {
		t.Fatalf("launch downloaded browser: %v", err)
	}
	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		t.Fatal(err)
	}
	ver, err := proto.BrowserGetVersion{}.Call(b)
	if err != nil || !strings.Contains(ver.Product, PinnedVersion) {
		t.Fatalf("browser product %+v err %v", ver, err)
	}
	_ = b.Close()
	l.Kill()
}
