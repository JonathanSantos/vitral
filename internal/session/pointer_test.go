package session

import (
	"testing"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/cursor"
)

func TestNewPointerFollowsConfig(t *testing.T) {
	cfg := config.Defaults("/home/u")
	cfg.Headless, cfg.Cursor, cfg.CursorSpeed, cfg.FlowCursorSpeed, cfg.CursorSeed = true, "force", "slow", "instant", 9
	p := newPointer(cfg)
	if !p.on || !p.pacing || p.prof.Speed != cursor.Slow || p.flow.Speed != cursor.Instant {
		t.Fatalf("pointer %+v", p)
	}
	// Semente fixa: a mesma sequência de sorteios.
	if a, b := newPointer(cfg).rng.Uint64(), newPointer(cfg).rng.Uint64(); a != b {
		t.Fatal("fixed seed must repeat")
	}
	cfg.CursorSeed = 0
	if a, b := newPointer(cfg).rng.Uint64(), newPointer(cfg).rng.Uint64(); a == b {
		t.Fatal("seed 0 is random per session")
	}
	cfg.Headless, cfg.Cursor = true, "on"
	if q := newPointer(cfg); q.on || q.pacing {
		t.Fatal("headless without force: no cursor, no pacing")
	}

	s := &Session{cur: p}
	if s.profile().Speed != cursor.Slow {
		t.Fatal("actions use the configured speed")
	}
	s.replaying.Store(true)
	if s.profile().Speed != cursor.Instant {
		t.Fatal("flow_run uses the flow speed")
	}
}

func TestPointerHelpers(t *testing.T) {
	if kindOf("pointer") != cursor.Pointer || kindOf("text") != cursor.Text || kindOf("default") != cursor.Arrow {
		t.Fatal("kindOf")
	}
	if rectOf([4]int{1, 2, 3, 4}) != (cursor.Rect{X: 1, Y: 2, W: 3, H: 4}) {
		t.Fatal("rectOf")
	}
	if keyLabel("Escape") != "Esc" || keyLabel("Space") != "Espaço" || keyLabel(" ") != "Espaço" || keyLabel("Control+A") != "Control+A" {
		t.Fatal("keyLabel")
	}
	base := [4]int{100, 100, 80, 40}
	for _, tt := range []struct {
		now   [4]int
		moved bool
	}{
		{[4]int{100, 100, 80, 40}, false},
		{[4]int{103, 102, 80, 40}, false},
		{[4]int{105, 100, 80, 40}, true},
		{[4]int{100, 100, 90, 40}, true},
		{[4]int{100, 100, 80, 46}, true},
	} {
		if movedBox(base, tt.now) != tt.moved {
			t.Errorf("movedBox(%v) = %v", tt.now, !tt.moved)
		}
	}
	if ringColor("e2") != "#e5484d" {
		t.Fatalf("ref e2 rings in Ruby, got %s", ringColor("e2"))
	}
}
