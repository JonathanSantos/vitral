//go:build integration

package app_test

import (
	"math"
	"regexp"
	"testing"

	"github.com/JonathanSantos/vitral/internal/config"
)

type mouseEvent struct {
	Type   string  `json:"type"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Target string  `json:"target"`
}

// takeEvents devolve e limpa os eventos de mouse que a fixture registrou.
func (h *harness) takeEvents(t *testing.T) []mouseEvent {
	t.Helper()
	res, err := h.page(t).Eval(`() => window.__events.splice(0)`)
	if err != nil {
		t.Fatal(err)
	}
	var out []mouseEvent
	if err := res.Value.Unmarshal(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (h *harness) status(t *testing.T) string {
	t.Helper()
	return h.mainWorld(t, `() => ({s: document.getElementById('status').textContent})`)["s"].(string)
}

func only(events []mouseEvent, kind string) []mouseEvent {
	var out []mouseEvent
	for _, e := range events {
		if e.Type == kind {
			out = append(out, e)
		}
	}
	return out
}

// Ações com o cursor ligado (--cursor=force em headless): o clique real sai no ponto final que a
// pessoa viu, sem eventos de passagem, e nunca num filho interativo (docs/CURSOR.md, 2.3 e 12.2).
func TestCursorActions(t *testing.T) {
	ch, cleanup := newHarness(func(c *config.Config) {
		c.Cursor, c.CursorSpeed, c.CursorSeed, c.HighlightMs = "force", "fast", 7, 0
	})
	defer cleanup()
	snap := ch.ok(t, "open", map[string]any{"url": ch.url("/pages/cursor.html")})

	t.Run("click lands where the cursor stopped, with a single move", func(t *testing.T) {
		ch.takeEvents(t)
		ch.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Pedido 42")})
		ev := ch.takeEvents(t)
		moves, downs := only(ev, "mousemove"), only(ev, "mousedown")
		if len(moves) != 1 || len(downs) != 1 {
			t.Fatalf("want one direct move and one press, got %d moves and %d presses: %+v", len(moves), len(downs), ev)
		}
		at, shown := ch.app.Overlay.CursorPosition()
		if !shown || math.Abs(downs[0].X-at.X) > 1 || math.Abs(downs[0].Y-at.Y) > 1 || moves[0].X != downs[0].X || moves[0].Y != downs[0].Y {
			t.Fatalf("press at (%v, %v), move at (%v, %v), cursor at %v", downs[0].X, downs[0].Y, moves[0].X, moves[0].Y, at)
		}
	})

	t.Run("card with a trash icon never clicks the trash", func(t *testing.T) {
		card := ref(t, snap, "button", "Pedido 42")
		for i := 0; i < 12; i++ {
			ch.ok(t, "click", map[string]any{"ref": card})
			if got := ch.status(t); got != "card" {
				t.Fatalf("click %d hit %q", i, got)
			}
			for _, e := range only(ch.takeEvents(t), "mousedown") {
				if e.Target == "trash" {
					t.Fatalf("press on the trash: %+v", e)
				}
			}
		}
	})

	t.Run("split button never clicks the arrow", func(t *testing.T) {
		split := ref(t, snap, "button", "Salvar")
		for i := 0; i < 12; i++ {
			ch.ok(t, "click", map[string]any{"ref": split})
			if got := ch.status(t); got != "salvar" {
				t.Fatalf("click %d hit %q (the arrow covers the center: the free area must win)", i, got)
			}
		}
	})

	t.Run("type clicks the field and types", func(t *testing.T) {
		ch.takeEvents(t)
		ch.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Nome"), "text": "Ana Lúcia"})
		if v := ch.mainWorld(t, `() => ({v: document.getElementById('nome').value})`)["v"]; v != "Ana Lúcia" {
			t.Fatalf("value %q", v)
		}
		if downs := only(ch.takeEvents(t), "mousedown"); len(downs) != 1 || downs[0].Target != "nome" {
			t.Fatalf("type should click the field once: %+v", downs)
		}
	})

	t.Run("select clicks only visually", func(t *testing.T) {
		ch.takeEvents(t)
		ch.ok(t, "select", map[string]any{"ref": ref(t, snap, "combobox", "País"), "option": "Brasil"})
		if v := ch.mainWorld(t, `() => ({v: document.getElementById('pais').value})`)["v"]; v != "Brasil" {
			t.Fatalf("value %q", v)
		}
		if downs := only(ch.takeEvents(t), "mousedown"); len(downs) != 0 {
			t.Fatalf("a real click would open the native list: %+v", downs)
		}
	})

	t.Run("covered target: same error for the AI", func(t *testing.T) {
		ch.mainWorld(t, `() => { window.cover(); return {}; }`)
		ch.fail(t, "click", map[string]any{"ref": ref(t, snap, "button", "Coberto")}, "is covered by")
		if got := ch.status(t); got == "coberto" {
			t.Fatal("covered button was clicked")
		}
	})

	t.Run("far target scrolls and clicks", func(t *testing.T) {
		full := ch.ok(t, "snapshot", map[string]any{"full": true})
		ch.ok(t, "click", map[string]any{"ref": ref(t, full, "button", "Lá embaixo")})
		if got := ch.status(t); got != "longe" {
			t.Fatalf("status %q", got)
		}
	})

	t.Run("scroll and press", func(t *testing.T) {
		ch.ok(t, "scroll", map[string]any{"direction": "up", "amount": 900})
		ch.ok(t, "press", map[string]any{"key": "Tab"})
	})

	t.Run("open again keeps the cursor", func(t *testing.T) {
		before, _ := ch.app.Overlay.CursorPosition()
		out := ch.ok(t, "open", map[string]any{"url": ch.url("/pages/cursor.html")})
		if regexp.MustCompile(`\bIA\b`).MatchString(out) {
			t.Fatalf("cursor label leaked into the snapshot:\n%s", out)
		}
		if after, shown := ch.app.Overlay.CursorPosition(); !shown || after != before {
			t.Fatalf("cursor should stay at %v after navigating, got %v", before, after)
		}
	})
}
