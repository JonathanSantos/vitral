package overlay

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/world"
)

func TestCursorDoneNeedsTokenAndID(t *testing.T) {
	m := newTestManager()
	ch := make(chan string, 1)
	m.waiters = map[string]chan string{"7": ch}
	for _, payload := range []string{
		`{"token":"chute","type":"cursor_done","id":"7"}`,
		`{"token":"` + m.token + `","type":"cursor_done","id":"8"}`,
		`{"token":"` + m.token + `","type":"cursor_done"}`,
	} {
		m.onPageEvent(payload)
		select {
		case <-ch:
			t.Fatalf("%s should not wake the waiter", payload)
		default:
		}
	}
	m.onPageEvent(`{"token":"` + m.token + `","type":"cursor_done","id":"7"}`)
	m.onPageEvent(`{"token":"` + m.token + `","type":"cursor_done","id":"7"}`) // repetido não trava
	select {
	case <-ch:
	default:
		t.Fatal("valid cursor_done should wake the waiter")
	}
}

func TestCursorMinimalIsNoop(t *testing.T) {
	m := New(Options{Minimal: true}, slog.New(slog.NewTextHandler(io.Discard, nil)), world.New())
	plan := cursor.Plan{Frames: []float64{0, 1, 1}, Move: 5000, Press: 5000}
	start := time.Now()
	if m.PlayCursor(context.Background(), plan, "#3e63dd", true) {
		t.Fatal("minimal overlay has no cursor")
	}
	for _, err := range []error{
		m.PlaceCursor(context.Background(), cursor.Point{X: 1, Y: 2}, true), m.CursorNav(context.Background(), true),
		m.HideCursor(context.Background()), m.CursorNote(context.Background(), "x", time.Second),
		m.CursorFail(context.Background(), "x"), m.Keycap(context.Background(), "Enter"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("minimal overlay must not wait")
	}
}

// Sem página (ou sem overlay), a ação não espera a duração do plano.
func TestPlayCursorWithoutPageDoesNotWait(t *testing.T) {
	m := newTestManager()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if m.PlayCursor(ctx, cursor.Plan{Frames: []float64{0, 1, 1}, Move: 3000, Press: 3000}, "", false) {
		t.Fatal("no page, no arrival")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("waited %v", time.Since(start))
	}
	if len(m.waiters) != 0 {
		t.Fatal("waiter not cleaned up")
	}
}

func TestArrival(t *testing.T) {
	plan := cursor.Plan{Move: 400, Press: 510}
	if Arrival(plan, true) != 510*time.Millisecond || Arrival(plan, false) != 400*time.Millisecond {
		t.Fatal("arrival is the press with a click and the end of the move without one")
	}
}

func TestRecentPlansKeepsTheLastOnes(t *testing.T) {
	m := newTestManager()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	for i := 0; i < recentPlans+3; i++ {
		m.PlayCursor(ctx, cursor.Plan{Frames: []float64{0, float64(i), 0}, End: cursor.Point{X: float64(i)}}, "", false)
	}
	plans := m.RecentPlans()
	if len(plans) != recentPlans || plans[len(plans)-1].End.X != float64(recentPlans+2) || plans[0].End.X != 3 {
		t.Fatalf("got %d plans, last %v", len(plans), plans[len(plans)-1].End)
	}
	if at, shown := m.CursorPosition(); !shown || at.X != float64(recentPlans+2) {
		t.Fatalf("position %v %v", at, shown)
	}
}
