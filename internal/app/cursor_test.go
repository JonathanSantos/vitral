//go:build integration

package app_test

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/brand"
	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/ux"
)

// inkNear conta pixels Chumbo (o corpo do cursor) num quadrado em volta do ponto.
func inkNear(img image.Image, p cursor.Point, r int) int {
	n := 0
	for y := int(p.Y) - r; y <= int(p.Y)+r; y++ {
		for x := int(p.X) - r; x <= int(p.X)+r; x++ {
			if near(rgba(img.At(x, y)), brand.Ink) {
				n++
			}
		}
	}
	return n
}

// O overlay toca o plano, avisa a chegada no press e o cursor fica visível para a pessoa, mas fora
// do screenshot da IA (docs/CURSOR.md, seção 1).
func TestCursorPlaysAndStaysOutOfScreenshot(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	p, err := h.app.Browser.Page()
	if err != nil {
		t.Fatal(err)
	}
	vw, vh := viewport(t, p)
	prof := cursor.DefaultProfile()
	rng := rand.New(rand.NewPCG(1, 2))
	// Uma área vazia da página, longe dos campos.
	target := cursor.Rect{X: float64(vw) - 300, Y: float64(vh) - 300, W: 60, H: 40}
	to := cursor.Candidates(target, cursor.Pointer, prof, rng)[0]
	plan := cursor.Compute(cursor.Input{From: cursor.Point{X: float64(vw) - 36, Y: float64(vh) - 38}, To: to, Target: target, Kind: cursor.Pointer}, prof, rng)

	start := time.Now()
	if !h.app.Overlay.PlayCursor(context.Background(), plan, "#3e63dd", true) {
		t.Fatal("overlay did not report the arrival")
	}
	took := time.Since(start)
	press := time.Duration(plan.Press) * time.Millisecond
	if took < press-50*time.Millisecond || took > press+300*time.Millisecond {
		t.Fatalf("arrival after %v, plan presses at %v", took, press)
	}
	time.Sleep(time.Duration(plan.Release-plan.Press)*time.Millisecond + 200*time.Millisecond)

	data, err := p.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if err != nil {
		t.Fatal(err)
	}
	page, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if n := inkNear(page, plan.End, 14); n < 20 {
		t.Fatalf("cursor not drawn at the end point (%d ink pixels)", n)
	}
	shot, _ := h.shot(t, map[string]any{"marks": false})
	if n := inkNear(shot, plan.End, 14); n > 0 {
		t.Fatalf("cursor leaked into the AI screenshot (%d ink pixels)", n)
	}
	if snap := h.ok(t, "snapshot", map[string]any{"full": true}); bytes.Contains([]byte(snap), []byte("IA")) {
		t.Fatalf("cursor label leaked into the snapshot:\n%s", snap)
	}
}

func pageShot(t *testing.T) image.Image {
	t.Helper()
	p, err := h.app.Browser.Page()
	if err != nil {
		t.Fatal(err)
	}
	data, err := p.Screenshot(false, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// Pausa: o cursor some (a pessoa está no controle) e volta no mesmo lugar ao retomar (6.2).
func TestCursorHidesWhilePaused(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	at := cursor.Point{X: 700, Y: 420}
	if err := h.app.Overlay.PlaceCursor(context.Background(), at, true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if n := inkNear(pageShot(t), at, 14); n < 20 {
		t.Fatalf("placed cursor not visible (%d)", n)
	}
	h.app.Overlay.Apply(ux.Event{Kind: ux.UserPause})
	defer h.app.Overlay.Apply(ux.Event{Kind: ux.UserResume})
	time.Sleep(400 * time.Millisecond)
	if n := inkNear(pageShot(t), at, 14); n > 0 {
		t.Fatalf("cursor still visible while paused (%d)", n)
	}
	h.app.Overlay.Apply(ux.Event{Kind: ux.UserResume})
	time.Sleep(400 * time.Millisecond)
	if n := inkNear(pageShot(t), at, 14); n < 20 {
		t.Fatalf("cursor did not come back after resume (%d)", n)
	}
}

func TestKeycapShowsAboveTheBadge(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	p, _ := h.app.Browser.Page()
	vw, vh := viewport(t, p)
	if err := h.app.Overlay.Keycap(context.Background(), "Enter"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	center := cursor.Point{X: float64(vw) / 2, Y: float64(vh) - 100}
	if n := inkNear(pageShot(t), center, 20); n < 100 {
		t.Fatalf("keycap not drawn (%d)", n)
	}
	time.Sleep(900 * time.Millisecond)
	if n := inkNear(pageShot(t), center, 20); n > 0 {
		t.Fatalf("keycap should leave after 600 ms (%d)", n)
	}
}
