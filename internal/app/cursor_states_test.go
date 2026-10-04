//go:build integration

package app_test

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/cursor"
)

// setHidden simula a aba oculta no mundo isolado do Vitral (onde o overlay e o script da sessão
// leem document.visibilityState) e avisa os ouvintes, como o navegador faria.
func (h *harness) setHidden(t *testing.T, hidden bool) {
	t.Helper()
	js := `(delete document.visibilityState, document.dispatchEvent(new Event('visibilitychange')), true)`
	if hidden {
		js = `(Object.defineProperty(document, 'visibilityState', { get: () => 'hidden', configurable: true }),
			document.dispatchEvent(new Event('visibilitychange')), true)`
	}
	if err := h.app.World.Eval(context.Background(), js); err != nil {
		t.Fatal(err)
	}
}

// Aba oculta no meio da animação: o overlay pula para o fim e avisa na hora (docs/CURSOR.md, 12.5).
func TestCursorHiddenDuringAnimation(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	prof := cursor.DefaultProfile()
	prof.Speed = cursor.Slow
	rng := rand.New(rand.NewPCG(3, 4))
	target := cursor.Rect{X: 1100, Y: 600, W: 12, H: 12}
	plan := cursor.Compute(cursor.Input{From: cursor.Point{X: 20, Y: 20}, To: target.Center(), Target: target}, prof, rng)
	if plan.Press < 800 {
		t.Fatalf("test needs a long plan, got %v ms", plan.Press)
	}
	t.Cleanup(func() { h.setHidden(t, false) })
	go func() {
		time.Sleep(150 * time.Millisecond)
		h.setHidden(t, true)
	}()
	start := time.Now()
	if !h.app.Overlay.PlayCursor(context.Background(), plan, "#3e63dd", false) {
		t.Fatal("hidden tab must still report the arrival")
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Fatalf("waited %v for a plan of %v ms: hiding the tab should finish it at once", took, plan.Press)
	}
}

func TestCursorStates(t *testing.T) {
	ch, cleanup := newHarness(func(c *config.Config) {
		c.Cursor, c.CursorSpeed, c.CursorSeed, c.HighlightMs = "force", "normal", 11, 0
	})
	defer cleanup()
	snap := ch.ok(t, "open", map[string]any{"url": ch.url("/pages/cursor.html")})
	card := ref(t, snap, "button", "Pedido 42")
	split := ref(t, snap, "button", "Salvar")
	travelOf := func(t *testing.T) cursor.Plan {
		t.Helper()
		plans := ch.app.Overlay.RecentPlans()
		if len(plans) < 2 {
			t.Fatalf("want travel and press plans, got %d", len(plans))
		}
		return plans[len(plans)-2]
	}

	t.Run("click budget in normal speed", func(t *testing.T) {
		for i, r := range []string{card, split, card, split, card, split} {
			start := time.Now()
			ch.ok(t, "click", map[string]any{"ref": r})
			wall := time.Since(start)
			plans := ch.app.Overlay.RecentPlans()
			travel, press := plans[len(plans)-2], plans[len(plans)-1]
			if visual := travel.Move + press.Release; visual > 1300 {
				t.Fatalf("click %d: %v ms of animation, budget is 1300", i, visual)
			}
			// O resto é a espera de estabilidade da página (rede e DOM quietos).
			if wall > 1300*time.Millisecond+1500*time.Millisecond {
				t.Fatalf("click %d took %v", i, wall)
			}
		}
	})

	t.Run("hidden tab: no trajectory, action completes", func(t *testing.T) {
		ch.setHidden(t, true)
		defer ch.setHidden(t, false)
		start := time.Now()
		ch.ok(t, "click", map[string]any{"ref": card})
		if travel := travelOf(t); len(travel.Frames) != 3 || travel.Fade != 0 {
			t.Fatalf("hidden tab plans with no keyframes and no fade: %+v", travel)
		}
		if got := ch.status(t); got != "card" {
			t.Fatalf("status %q", got)
		}
		if took := time.Since(start); took > 2*time.Second {
			t.Fatalf("hidden tab click took %v", took)
		}
	})

	t.Run("prefers-reduced-motion: fade, no intermediate keyframes", func(t *testing.T) {
		media := proto.EmulationSetEmulatedMedia{Features: []*proto.EmulationMediaFeature{{Name: "prefers-reduced-motion", Value: "reduce"}}}
		if err := media.Call(ch.page(t)); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = proto.EmulationSetEmulatedMedia{}.Call(ch.page(t)) }()
		ch.ok(t, "click", map[string]any{"ref": split})
		if travel := travelOf(t); len(travel.Frames) != 3 || travel.Fade != 120 {
			t.Fatalf("reduced motion plans a 120 ms fade with no trajectory: %+v", travel)
		}
	})

	t.Run("covered: the cursor shakes its head and explains", func(t *testing.T) {
		ch.mainWorld(t, `() => { window.cover(); return {}; }`)
		ch.fail(t, "click", map[string]any{"ref": ref(t, snap, "button", "Coberto")}, "is covered by dialog")
		at, _ := ch.app.Overlay.CursorPosition()
		img := pageShotOf(t, ch)
		// A etiqueta "Coberto por um diálogo" fica abaixo e à direita do cursor por 1,2 s.
		if n := inkNear(img, cursor.Point{X: at.X + 60, Y: at.Y + 50}, 12); n < 40 {
			t.Fatalf("failure label not drawn next to the cursor (%d ink pixels)", n)
		}
	})
}
