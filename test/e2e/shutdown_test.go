//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/browser/install"
	"github.com/JonathanSantos/vitral/test/procs"
)

// Fechar o stdin no meio de uma ação: o servidor sai e o navegador que ele lançou morre.
func TestShutdownMidAction(t *testing.T) {
	s := start(t)
	ctx := context.Background()
	if _, _, err := s.Call(ctx, "open", map[string]any{"url": site.URL + "/pages/form.html"}); err != nil {
		t.Fatal(err)
	}
	info := s.readDebugInfo()
	if info.PID == 0 {
		t.Fatal("debug info should have the launched browser pid")
	}
	go func() { _, _, _ = s.Call(ctx, "wait_for", map[string]any{"text": "nunca", "timeoutMs": 30000}) }()
	eventually(t, 10*time.Second, func() error {
		if got := s.overlayText(); !strings.Contains(got, "trabalhando") {
			return fmt.Errorf("overlay shows %q", got)
		}
		return nil
	})
	start := time.Now()
	s.close() // fecha o stdin com o wait_for ainda rodando; close() já exige saída e navegador morto
	if procs.Alive(info.PID) {
		t.Fatal("launched browser still alive")
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Fatalf("shutdown took %s", took)
	}
}

// No attach o navegador é da pessoa: o Vitral nunca o fecha.
func TestAttachDoesNotCloseBrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses process signals")
	}
	platform, ok := install.Platform(runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("no Chrome for Testing for this platform")
	}
	exe := filepath.Join(browsersDir, install.PinnedVersion, filepath.FromSlash(install.ExecutableRel(platform)))
	l := launcher.New().Bin(exe).Headless(true).UserDataDir(t.TempDir())
	u, err := l.Launch()
	if err != nil {
		t.Skipf("own browser not available: %v", err)
	}
	t.Cleanup(l.Kill)

	s := start(t, "--attach", u)
	if _, isErr, err := s.Call(context.Background(), "open", map[string]any{"url": site.URL + "/pages/form.html"}); err != nil || isErr {
		t.Fatalf("open via attach failed: %v", err)
	}
	s.close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := rod.New().Context(ctx).ControlURL(u)
	if err := b.Connect(); err != nil {
		t.Fatalf("attached browser was closed by Vitral: %v", err)
	}
	if _, err := (proto.BrowserGetVersion{}).Call(b); err != nil {
		t.Fatalf("attached browser not responding: %v", err)
	}
}
