//go:build integration && visual

package app_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"math/rand/v2"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/browser/install"
	"github.com/JonathanSantos/vitral/internal/cursor"
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
	// O cursor de testes anteriores reaparece na página nova: estes goldens são só do selo.
	_ = h.app.Overlay.HideCursor(context.Background())
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

// Goldens do cursor visual (docs/CURSOR.md, seção 9): parado com etiqueta, mão sobre botão, anel no
// meio do efeito (animações congeladas pelo CDP para o quadro ser sempre o mesmo) e keycap.
func TestVisualCursor(t *testing.T) {
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
	ctx := context.Background()
	prof := cursor.DefaultProfile()
	rng := rand.New(rand.NewPCG(5, 6))
	// still é um plano sem movimento no ponto: só a forma, a etiqueta e (com anel) o clique.
	still := func(at cursor.Point, kind cursor.Kind) cursor.Plan {
		return cursor.Compute(cursor.Input{From: at, To: at, Target: cursor.Rect{X: at.X - 10, Y: at.Y - 10, W: 20, H: 20}, Kind: kind}, prof, rng)
	}
	around := func(at cursor.Point) image.Rectangle {
		return image.Rect(int(at.X)-24, int(at.Y)-24, int(at.X)+140, int(at.Y)+76)
	}

	blank := cursor.Point{X: 700, Y: 300}
	if err := h.app.Overlay.PlaceCursor(ctx, blank, true); err != nil {
		t.Fatal(err)
	}
	h.app.Overlay.PlayCursor(ctx, still(blank, cursor.Arrow), "", false)
	check("cursor-idle-label", stable(t, p, around(blank)))

	button := boxOf(t, h.ok(t, "inspect", map[string]any{"ref": ref(t, snap, "button", "Sobreposto")}))
	onButton := cursor.Point{X: float64(button.Min.X + button.Dx()/2), Y: float64(button.Min.Y + button.Dy()/2)}
	h.app.Overlay.PlayCursor(ctx, still(onButton, cursor.Pointer), "", false)
	check("cursor-pointer-on-button", stable(t, p, around(onButton)))

	// Anel no meio: as animações criadas no press (anel e encolhimento) são pausadas e levadas a
	// 150 ms; o release termina normalmente antes da captura. Congelar tudo pararia também o relógio
	// do requestAnimationFrame, e o cursor nunca chegaria ao press.
	if err := (proto.AnimationEnable{}).Call(p); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proto.AnimationDisable{}.Call(p) }()
	created := make(chan string, 16)
	wait := p.Context(ctx).EachEvent(func(e *proto.AnimationAnimationCreated) bool {
		created <- e.ID
		return false
	})
	go wait()
	ringAt := cursor.Point{X: 500, Y: 400}
	ringPlan := still(ringAt, cursor.Arrow)
	if !h.app.Overlay.PlayCursor(ctx, ringPlan, "#e5484d", false) {
		t.Fatal("no arrival for the ring plan")
	}
	var ids []string
collect:
	for {
		select {
		case id := <-created:
			ids = append(ids, id)
		case <-time.After(60 * time.Millisecond):
			break collect
		}
	}
	if len(ids) == 0 {
		t.Fatal("the click ring animation was not created")
	}
	if err := (proto.AnimationSetPaused{Animations: ids, Paused: true}).Call(p); err != nil {
		t.Fatal(err)
	}
	if err := (proto.AnimationSeekAnimations{Animations: ids, CurrentTime: 150}).Call(p); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Duration(ringPlan.Release-ringPlan.Press)*time.Millisecond + 250*time.Millisecond)
	check("cursor-ring-mid", stable(t, p, around(ringAt).Inset(-20)))

	vw, vh := viewport(t, p)
	if err := h.app.Overlay.Keycap(ctx, "Enter"); err != nil {
		t.Fatal(err)
	}
	check("keycap-enter", stable(t, p, image.Rect(vw/2-120, vh-150, vw/2+120, vh-60)))
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
