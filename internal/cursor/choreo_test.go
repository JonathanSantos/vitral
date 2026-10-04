package cursor

import (
	"math"
	"testing"

	"pgregory.net/rapid"
)

func TestBadgePointAndMinPress(t *testing.T) {
	if BadgePoint(1280, 800) != (Point{1244, 762}) || BadgePoint(10, 10) != (Point{0, 0}) {
		t.Fatal("badge point is 36×38 px from the bottom-right corner, never negative")
	}
	plan := Plan{Move: 300, Press: 400, Release: 480}
	if got := plan.WithMinPress(350); got.Press != plan.Press || got.Release != plan.Release {
		t.Fatalf("press already after the minimum: %+v", got)
	}
	if got := plan.WithMinPress(500); got.Press != 500 || got.Release != 580 || got.Move != 300 {
		t.Fatalf("aim extends to the minimum, keeping the press duration: %+v", got)
	}
}

func TestPropertyAway(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := DefaultProfile()
		p.Speed = genSpeed(t)
		view := Rect{0, 0, rapid.Float64Range(200, 2000).Draw(t, "vw"), rapid.Float64Range(200, 1200).Draw(t, "vh")}
		from := Point{rapid.Float64Range(0, view.W).Draw(t, "x"), rapid.Float64Range(0, view.H).Draw(t, "y")}
		plan := Away(from, view, p, newRand(rapid.Uint64().Draw(t, "seed")))
		f := plan.Frames
		end := Point{f[len(f)-2], f[len(f)-1]}
		if end != plan.End || !view.Contains(end) {
			t.Fatalf("ends at %v (end %v), view %v", end, plan.End, view)
		}
		if first := (Point{f[1], f[2]}); first != roundPoint(from) {
			t.Fatalf("starts at %v, want %v", first, roundPoint(from))
		}
		k := p.Speed.Factor()
		if plan.Move < p.AwayMin*k-1 || plan.Move > p.AwayMax*k+1 || plan.Press != plan.Move || plan.Release != plan.Move {
			t.Fatalf("away timing %+v", plan)
		}
		if d := dist(roundPoint(from), end); d > p.AwayDistMax+round {
			t.Fatalf("moved %.1f px", d)
		}
		// Para baixo e à direita (ou parado na borda).
		if end.X < roundPoint(from).X-round-1e-9 || end.Y < roundPoint(from).Y-round-1e-9 {
			t.Fatalf("away goes down and right: %v → %v", from, end)
		}
	})
}

func TestAwayFarFromEdgesCoversTheRange(t *testing.T) {
	p := DefaultProfile()
	d, dur := newSpread(), newSpread()
	rng := newRand(60)
	for i := 0; i < 500; i++ {
		plan := Away(Point{300, 300}, Rect{0, 0, 1000, 800}, p, rng)
		d.add(dist(Point{300, 300}, plan.End))
		dur.add(plan.Move)
	}
	d.covers(t, "away distance", 40, 80, 2)
	dur.covers(t, "away duration", 150, 250, 3)
	instant := p
	instant.Speed = Instant
	if plan := Away(Point{300, 300}, Rect{0, 0, 1000, 800}, instant, rng); len(plan.Frames) != 3 || plan.Move != 0 {
		t.Fatalf("instant away jumps: %+v", plan)
	}
}

func TestPropertyTyping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := DefaultProfile()
		n := rapid.IntRange(0, 500).Draw(t, "chars")
		delays := Typing(n, p, newRand(rapid.Uint64().Draw(t, "seed")))
		var total float64
		for _, d := range delays {
			if d < p.KeyMin || d > p.KeyMax {
				t.Fatalf("key delay %v", d)
			}
			total += d
		}
		if total > p.TypeCap || len(delays) > n {
			t.Fatalf("%d delays, %v ms for %d chars", len(delays), total, n)
		}
		// Até ~33 caracteres (1500/45) sempre cabem com ritmo.
		if n <= int(p.TypeCap/p.KeyMax) && len(delays) != n {
			t.Fatalf("short text should be fully paced: %d of %d", len(delays), n)
		}
	})
}

func TestPropertyWheel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := DefaultProfile()
		amount := rapid.Float64Range(-20000, 20000).Draw(t, "amount")
		w := WheelSteps(amount, p, newRand(rapid.Uint64().Draw(t, "seed")))
		sum, elapsed := w.Rest, 0.0
		for i, s := range w.Steps {
			if math.Abs(s) > p.WheelStep || math.Signbit(s) != math.Signbit(amount) || s == 0 {
				t.Fatalf("step %v for amount %v", s, amount)
			}
			if i == 0 && w.Delays[i] != 0 || i > 0 && (w.Delays[i] < p.WheelMin || w.Delays[i] > p.WheelMax) {
				t.Fatalf("delay %v at step %d", w.Delays[i], i)
			}
			sum += s
			elapsed += w.Delays[i]
		}
		if math.Abs(sum-amount) > 1e-6 || elapsed > p.WheelCap {
			t.Fatalf("wheel moves %v of %v in %v ms", sum, amount, elapsed)
		}
		if w.Rest != 0 && math.Abs(w.Rest) <= 0 {
			t.Fatal("rest sign")
		}
	})
}

func TestWheelExamples(t *testing.T) {
	p := DefaultProfile()
	w := WheelSteps(250, p, newRand(61))
	if len(w.Steps) != 3 || w.Steps[2] != 50 || w.Rest != 0 {
		t.Fatalf("250 px = 100 + 100 + 50: %+v", w)
	}
	w = WheelSteps(-5000, p, newRand(62))
	if w.Rest >= 0 || len(w.Steps) < 9 || len(w.Steps) > 15 {
		t.Fatalf("long scroll is capped at %v ms and the rest goes at once: %+v", p.WheelCap, w)
	}
	if w := WheelSteps(0, p, newRand(63)); len(w.Steps) != 0 || w.Rest != 0 {
		t.Fatal("no scroll, no steps")
	}
}
