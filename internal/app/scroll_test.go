//go:build integration

package app_test

import (
	"context"
	"math"
	"regexp"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/cursor"
)

// recorder grava, por requestAnimationFrame, o scrollTop do contêiner (page ou seletor CSS).
func (h *harness) startScrollRecorder(t *testing.T, selector string) {
	t.Helper()
	js := `(() => { const el = ` + selector + `; const d = { f: [], on: true }; window.__scrollRec = d;
		const loop = (ts) => { if (!d.on) return; d.f.push([ts, el.scrollTop]); requestAnimationFrame(loop); };
		requestAnimationFrame(loop); return true; })()`
	if err := h.app.World.Eval(context.Background(), js); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) stopScrollRecorder(t *testing.T) [][2]float64 {
	t.Helper()
	var f [][2]float64
	if err := h.app.World.Unmarshal(context.Background(), &f, `() => { window.__scrollRec.on = false; return window.__scrollRec.f; }`); err != nil {
		t.Fatal(err)
	}
	return f
}

// maxGapFrames é o maior intervalo entre mudanças de posição, em frames, durante o miolo do gesto
// (até 95% do deslocamento: no fim do atrito o passo fica abaixo de 1 px e o scrollTop inteiro para,
// o que é desaceleração, não travamento). Ignora o salto e até `pauses` pausas planejadas.
func maxGapFrames(f [][2]float64, pauses int) (float64, float64) {
	if len(f) < 3 {
		return 0, 0
	}
	frame := 1000.0 / 60
	start, end := f[0][1], f[len(f)-1][1]
	total := math.Abs(end - start)
	var gaps []float64
	lastT := -1.0
	for i := 1; i < len(f); i++ {
		if f[i][1] == f[i-1][1] {
			continue
		}
		if lastT >= 0 && math.Abs(f[i-1][1]-start) < 0.95*total {
			gaps = append(gaps, (f[i][0]-lastT)/frame)
		}
		lastT = f[i][0]
	}
	// As maiores `pauses` lacunas são as pausas "olhando a tela" entre flicks.
	for k := 0; k < pauses && len(gaps) > 0; k++ {
		maxI := 0
		for i, g := range gaps {
			if g > gaps[maxI] {
				maxI = i
			}
		}
		gaps = append(gaps[:maxI], gaps[maxI+1:]...)
	}
	worst := 0.0
	for _, g := range gaps {
		worst = math.Max(worst, g)
	}
	return worst, total
}

func TestScrollEngine(t *testing.T) {
	sh, cleanup := newHarness(func(c *config.Config) {
		c.Cursor, c.CursorSpeed, c.CursorSeed, c.HighlightMs = "force", "normal", 21, 0
	})
	defer cleanup()
	refOf := func(t *testing.T, label string) string {
		t.Helper()
		full := sh.ok(t, "snapshot", map[string]any{"full": true})
		m := regexp.MustCompile(`\[(e\d+)\] button "` + label + `"`).FindStringSubmatch(full)
		if m == nil {
			t.Fatalf("no %q", label)
		}
		return m[1]
	}
	open := func(t *testing.T) {
		t.Helper()
		sh.ok(t, "open", map[string]any{"url": sh.url("/pages/scroll-playground.html")})
	}
	// inBand confere o topo do alvo dentro da faixa, abaixo do header fixo de 64 px.
	inBand := func(t *testing.T, ref string) {
		t.Helper()
		box := boxOf(t, sh.ok(t, "inspect", map[string]any{"ref": ref}))
		_, vh := viewport(t, sh.page(t))
		if !cursor.InBand(float64(box.Min.Y), float64(vh), 64) {
			t.Fatalf("target top at %d px of %d (header 64): outside 25%%-45%% of the visible area", box.Min.Y, vh)
		}
	}

	for _, tt := range []struct{ label, status string }{
		{"Alvo a uma tela", "uma tela"},
		{"Alvo a três telas", "três telas"},
		{"Alvo bem longe", "longe"},
	} {
		t.Run("click "+tt.label, func(t *testing.T) {
			open(t)
			ref := refOf(t, tt.label)
			sh.startScrollRecorder(t, "document.scrollingElement")
			sh.ok(t, "click", map[string]any{"ref": ref})
			f := sh.stopScrollRecorder(t)
			if got := sh.status(t); got != "Clicou: "+tt.status {
				t.Fatalf("status %q", got)
			}
			inBand(t, ref)
			plans := sh.app.Overlay.RecentScrollPlans()
			pauses := plans[len(plans)-1].Flicks - 1
			if gap, total := maxGapFrames(f, pauses); gap > 2.6 || total < 100 {
				t.Fatalf("scroll not fluid: largest gap %.1f frames (moved %.0f px)", gap, total)
			}
		})
	}

	t.Run("nested container", func(t *testing.T) {
		open(t)
		ref := refOf(t, "Alvo no fim da lista")
		before := sh.app.Overlay.ScrollsStarted()
		sh.ok(t, "click", map[string]any{"ref": ref})
		if got := sh.status(t); got != "Clicou: aninhado" {
			t.Fatalf("status %q", got)
		}
		if n := sh.app.Overlay.ScrollsStarted() - before; n < 2 {
			t.Fatalf("page and inner list should both scroll, got %d plans", n)
		}
	})

	t.Run("wheel-only scroller falls back to the wheel", func(t *testing.T) {
		open(t)
		sh.ok(t, "click", map[string]any{"ref": refOf(t, "Alvo da roda")})
		if got := sh.status(t); got != "Clicou: roda" {
			t.Fatalf("status %q", got)
		}
	})

	t.Run("scroll by direction is one fluid gesture", func(t *testing.T) {
		open(t)
		sh.startScrollRecorder(t, "document.scrollingElement")
		start := time.Now()
		sh.ok(t, "scroll", map[string]any{"direction": "down", "amount": 600})
		f := sh.stopScrollRecorder(t)
		if gap, total := maxGapFrames(f, 0); gap > 2.6 || math.Abs(total-600) > 2 {
			t.Fatalf("direction scroll: gap %.1f frames, moved %.0f px", gap, total)
		}
		if took := time.Since(start); took > 3*time.Second {
			t.Fatalf("scroll took %v", took)
		}
	})

	t.Run("the person scrolling stops the animation", func(t *testing.T) {
		open(t)
		ref := refOf(t, "Alvo a três telas")
		before := sh.app.Overlay.ScrollsStarted()
		done := make(chan struct{})
		go func() {
			defer close(done)
			sh.ok(t, "click", map[string]any{"ref": ref})
		}()
		// Espera a animação começar (o plano é registrado ao ser enviado ao overlay).
		for deadline := time.Now().Add(5 * time.Second); sh.app.Overlay.ScrollsStarted() == before; {
			if time.Now().After(deadline) {
				t.Fatal("scroll never started")
			}
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(120 * time.Millisecond)
		// Roda de verdade (o CDP gera eventos isTrusted, como o mouse da pessoa).
		_ = sh.page(t).Mouse.Scroll(0, 40, 1)
		<-done
		if r := sh.app.Overlay.LastScrollResult(); r != "interrupted" {
			t.Fatalf("scroll result %q, want interrupted", r)
		}
		if got := sh.status(t); got != "Clicou: três telas" {
			t.Fatalf("the action should go on after the interruption, status %q", got)
		}
	})
}
