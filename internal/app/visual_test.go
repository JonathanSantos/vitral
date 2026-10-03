//go:build integration && visual

package app_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/browser/install"
	"github.com/JonathanSantos/vitral/internal/ux"
	"github.com/JonathanSantos/vitral/test/visual"
)

// Regressão visual (docs/TESTING.md, seção 10): marcações num print real e o selo em cada estado.
// Os goldens ficam por sistema e versão do Chrome for Testing, porque a fonte do sistema e o
// rasterizador mudam entre eles. Rode com make visual; make visual-update aceita as mudanças.
func TestVisual(t *testing.T) {
	p, err := h.app.Browser.Page()
	if err != nil {
		t.Fatal(err)
	}
	dir := goldenDir(t, p)
	out := filepath.Join("..", "..", ".quality", "out", "visual")
	check := func(name string, img image.Image) {
		t.Helper()
		if err := visual.Golden(filepath.Join(dir, name+".png"), img, *update, out); err != nil {
			t.Error(err)
		}
	}

	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/visual.html")})
	img, _ := h.shot(t, map[string]any{})
	check("marks-page", img)

	password := ref(t, snap, "textbox", "Senha")
	box := boxOf(t, h.ok(t, "inspect", map[string]any{"ref": password}))
	vw, vh := viewport(t, p)
	badge := image.Rect(vw-620, vh-110, vw, vh)

	steps := []struct {
		name   string
		events []ux.Event
		clip   image.Rectangle
		reset  []ux.Event
	}{
		{name: "badge-idle", clip: badge},
		{name: "badge-working", events: []ux.Event{{Kind: ux.ToolStarted}}, clip: badge},
		{
			name: "badge-flow", events: []ux.Event{{Kind: ux.FlowStep, FlowName: "pedido-rapido", Step: 2, Total: 5}},
			clip: badge, reset: []ux.Event{{Kind: ux.ToolFinished}},
		},
		{name: "badge-paused", events: []ux.Event{{Kind: ux.UserPause}}, clip: badge, reset: []ux.Event{{Kind: ux.UserResume}}},
		{
			// "Sua vez": o selo e o destaque que fica no campo até a pessoa retomar.
			name: "badge-waiting",
			events: []ux.Event{{Kind: ux.WaitUser, Reason: ux.ReasonPassword, Hold: &ux.Highlight{
				Ref: password, Box: ux.Box{box.Min.X, box.Min.Y, box.Dx(), box.Dy()},
			}}},
			clip: image.Rect(0, 0, vw, vh), reset: []ux.Event{{Kind: ux.UserResume}},
		},
		{name: "badge-collapsed", events: []ux.Event{{Kind: ux.Collapse}}, clip: badge, reset: []ux.Event{{Kind: ux.Expand}}},
	}
	for _, s := range steps {
		for _, e := range s.events {
			h.app.Overlay.Apply(e)
		}
		check(s.name, stable(t, p, s.clip))
		for _, e := range s.reset {
			h.app.Overlay.Apply(e)
		}
	}
	if st := h.app.Overlay.State(); st.Mode != ux.Idle || st.Collapsed {
		t.Fatalf("overlay should be back to idle, got %+v", st)
	}
}

// goldenDir é testdata/visual/<sistema>-chrome-<versão>. Os goldens são atrelados ao Chrome for
// Testing fixado: outro navegador reprova, em vez de comparar com a referência errada.
func goldenDir(t *testing.T, p *rod.Page) string {
	t.Helper()
	v, err := proto.BrowserGetVersion{}.Call(p)
	if err != nil {
		t.Fatal(err)
	}
	_, version, _ := strings.Cut(v.Product, "/")
	if version != install.PinnedVersion {
		t.Fatalf("visual goldens are tied to Chrome for Testing %s, but the tests run %s; run `make e2e` or `go run ./cmd/vitral install-browser` once, and unset VITRAL_BROWSER_PATH",
			install.PinnedVersion, v.Product)
	}
	return filepath.Join("..", "..", "testdata", "visual", runtime.GOOS+"-chrome-"+version)
}

func viewport(t *testing.T, p *rod.Page) (int, int) {
	t.Helper()
	res, err := p.Eval(`() => [window.innerWidth, window.innerHeight]`)
	if err != nil {
		t.Fatal(err)
	}
	arr := res.Value.Arr()
	return arr[0].Int(), arr[1].Int()
}

// stable captura a região até dois quadros seguidos saírem iguais: o selo redesenha de forma
// assíncrona depois do evento.
func stable(t *testing.T, p *rod.Page, clip image.Rectangle) image.Image {
	t.Helper()
	var last []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, err := p.Context(ctx).Screenshot(false, &proto.PageCaptureScreenshot{
			Format: proto.PageCaptureScreenshotFormatPng,
			Clip: &proto.PageViewport{
				X: float64(clip.Min.X), Y: float64(clip.Min.Y),
				Width: float64(clip.Dx()), Height: float64(clip.Dy()), Scale: 1,
			},
		})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if last != nil && bytes.Equal(data, last) {
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			return img
		}
		last = data
	}
	t.Fatal("the overlay did not settle in 5s")
	return nil
}
