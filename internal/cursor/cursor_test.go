package cursor

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

func newRand(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, 7)) }

func TestParseSpeedAndFactor(t *testing.T) {
	for _, tt := range []struct {
		in     string
		factor float64
	}{{"slow", 1.6}, {"normal", 1}, {"fast", 0.6}, {"instant", 0}} {
		s, err := ParseSpeed(tt.in)
		if err != nil || s.Factor() != tt.factor {
			t.Fatalf("%s: speed %q factor %v err %v", tt.in, s, s.Factor(), err)
		}
	}
	if _, err := ParseSpeed("turbo"); err == nil || !strings.Contains(err.Error(), "slow, normal, fast or instant") {
		t.Fatalf("invalid speed should list the options: %v", err)
	}
	if Speed("").Factor() != 1 {
		t.Fatal("empty speed should behave as normal")
	}
}

func TestKindString(t *testing.T) {
	if Arrow.String() != "arrow" || Pointer.String() != "pointer" || Text.String() != "text" || Kind(9).String() != "arrow" {
		t.Fatal("kind names")
	}
}

func TestRectHelpers(t *testing.T) {
	r := Rect{10, 20, 100, 50}
	if r.Center() != (Point{60, 45}) {
		t.Fatalf("center %v", r.Center())
	}
	if in := r.Inset(0.7); math.Abs(in.X-25) > 1e-9 || math.Abs(in.W-70) > 1e-9 || math.Abs(in.Y-27.5) > 1e-9 || math.Abs(in.H-35) > 1e-9 {
		t.Fatalf("inset %v", in)
	}
	if !r.Contains(Point{10, 20}) || !r.Contains(Point{110, 70}) || r.Contains(Point{110.1, 70}) || r.Contains(Point{50, 19.9}) {
		t.Fatal("contains should include the edges and nothing beyond")
	}
	if unit(Point{1, 1}, Point{1, 1}) != (Point{1, 0}) {
		t.Fatal("unit of a zero vector falls back to the x axis")
	}
	if finite(math.NaN()) != 0 || finite(math.Inf(1)) != 0 || finite(5e9) != maxCoord || finite(-5e9) != -maxCoord {
		t.Fatal("finite should neutralize NaN/Inf and clamp huge coordinates")
	}
	if r := saneRect(Rect{1, 2, -5, math.NaN()}); r.W != 0 || r.H != 0 {
		t.Fatalf("negative or NaN sizes become zero: %v", r)
	}
}

func TestCandidatesTextGoesLeft(t *testing.T) {
	p := DefaultProfile()
	field := Rect{100, 100, 300, 40}
	cands := Candidates(field, Text, p, newRand(1))
	inner := field.Inset(p.Inner)
	for _, c := range cands {
		if c.X > inner.X+inner.W/3+1e-9 {
			t.Fatalf("text candidate %v is not in the left third", c)
		}
	}
	if last := cands[len(cands)-1]; last != roundPoint(Point{inner.X + inner.W/6, inner.Y + inner.H/2}) {
		t.Fatalf("last candidate for text is the middle of the left third, got %v", last)
	}
	if last := Candidates(field, Pointer, p, newRand(1)); last[len(last)-1] != field.Center() {
		t.Fatalf("last candidate is the center, got %v", last[len(last)-1])
	}
}

func TestCandidatesSpreadAroundCenter(t *testing.T) {
	p := DefaultProfile()
	target := Rect{0, 0, 200, 100}
	var sx, sy, n float64
	rng := newRand(2)
	distinct := map[Point]bool{}
	for i := 0; i < 400; i++ {
		for _, c := range Candidates(target, Pointer, p, rng)[:p.Tries] {
			sx += (c.X - 100) * (c.X - 100)
			sy += (c.Y - 50) * (c.Y - 50)
			n++
			distinct[c] = true
		}
	}
	// σ ≈ 15% do tamanho (a limitação aos 70% internos corta só as caudas).
	if gx, gy := math.Sqrt(sx/n), math.Sqrt(sy/n); math.Abs(gx-30) > 3 || math.Abs(gy-15) > 1.5 {
		t.Fatalf("spread σx=%.1f σy=%.1f, want about 30 and 15", gx, gy)
	}
	if len(distinct) < 500 {
		t.Fatalf("candidates should vary, got %d distinct points", len(distinct))
	}
}

func TestRoundInside(t *testing.T) {
	area := Rect{0.15, 1.15, 0.7, 0.7}
	if got := roundInside(Point{0.88, 1.12}, area); got != (Point{0.8, 1.2}) {
		t.Fatalf("rounding must stay inside the area, got %v", got)
	}
	if got := roundInside(Point{5, 5}, Rect{1.01, 1.01, 0.05, 0.05}); got != (Point{1, 1}) {
		t.Fatalf("an area smaller than 0.1 px uses the rounded center, got %v", got)
	}
}

func TestFreeArea(t *testing.T) {
	tests := []struct {
		name      string
		target    Rect
		obstacles []Rect
		want      Rect
		ok        bool
	}{
		{"no obstacles", Rect{10, 10, 100, 40}, nil, Rect{10, 10, 100, 40}, true},
		// Card clicável com lixeira no canto: a maior área livre é a faixa à esquerda da lixeira.
		{"card with trash icon", Rect{0, 0, 300, 100}, []Rect{{270, 6, 24, 24}}, Rect{0, 0, 270, 100}, true},
		// Botão dividido: a seta à direita é outro botão; sobra a parte principal.
		{"split button", Rect{0, 0, 120, 36}, []Rect{{90, 0, 30, 36}}, Rect{0, 0, 90, 36}, true},
		{"obstacle outside is ignored", Rect{0, 0, 50, 50}, []Rect{{100, 100, 10, 10}, {0, 0, 0, 10}}, Rect{0, 0, 50, 50}, true},
		{"fully covered", Rect{0, 0, 50, 50}, []Rect{{-5, -5, 60, 60}}, Rect{}, false},
		{"empty target", Rect{0, 0, 0, 10}, nil, Rect{}, false},
		// Ícone no meio: as quatro faixas competem; vence a maior (embaixo, largura inteira).
		{"icon in the middle", Rect{0, 0, 100, 100}, []Rect{{40, 20, 20, 20}}, Rect{0, 40, 100, 60}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FreeArea(tt.target, tt.obstacles)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("FreeArea = %v %v, want %v %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSideKeepsTheHand(t *testing.T) {
	p := DefaultProfile()
	rng := newRand(3)
	same, sides := 0, map[int]int{}
	for i := 0; i < 4000; i++ {
		if side(1, p, rng) == 1 {
			same++
		}
		sides[side(0, p, rng)]++
	}
	if frac := float64(same) / 4000; math.Abs(frac-p.SameSide) > 0.03 {
		t.Fatalf("repeats the side %.2f of the time, want %.2f", frac, p.SameSide)
	}
	if sides[1] < 1800 || sides[-1] < 1800 || len(sides) != 2 {
		t.Fatalf("first movement picks a side at random: %v", sides)
	}
	if side(-1, Profile{SameSide: 1}, rng) != -1 || side(-1, Profile{SameSide: 0}, rng) != 1 {
		t.Fatal("side must follow SameSide")
	}
}

func TestComputeNoTrajectoryModes(t *testing.T) {
	target := Rect{500, 400, 80, 30}
	to := target.Center()
	tests := []struct {
		name   string
		speed  Speed
		motion Motion
		fade   float64
		aim    float64 // mira exata (0 = sorteada)
	}{
		{"instant", Instant, Animated, 80, 70},
		{"reduced motion", Normal, Reduced, 120, 0},
		{"instant wins over reduced", Instant, Reduced, 80, 70},
		{"hidden tab applies at once", Normal, Hidden, 0, 70},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultProfile()
			p.Speed = tt.speed
			plan := Compute(Input{From: Point{10, 10}, To: to, Target: target, Motion: tt.motion}, p, newRand(4))
			if len(plan.Frames) != 3 || plan.Frames[1] != to.X || plan.Frames[2] != to.Y {
				t.Fatalf("no intermediate keyframes expected, got %v", plan.Frames)
			}
			if plan.Fade != tt.fade || plan.Move != tt.fade {
				t.Fatalf("fade %v move %v, want %v", plan.Fade, plan.Move, tt.fade)
			}
			aim := plan.Press - plan.Move
			if tt.aim > 0 && (aim != tt.aim || plan.Release-plan.Press != 60) {
				t.Fatalf("aim %v press %v, want the minimums", aim, plan.Release-plan.Press)
			}
			if tt.aim == 0 && (aim < 70 || aim > 140) {
				t.Fatalf("reduced motion keeps a normal aim, got %v", aim)
			}
			if msg := checkPlan(Input{From: Point{10, 10}, To: to, Target: target, Motion: tt.motion}, p, plan); msg != "" {
				t.Fatal(msg)
			}
		})
	}
}

func TestComputeTinyDistanceOnlyClicks(t *testing.T) {
	p := DefaultProfile()
	in := Input{From: Point{100, 100}, To: Point{101, 101}, Target: Rect{90, 90, 20, 20}}
	plan := Compute(in, p, newRand(5))
	if len(plan.Frames) != 3 || plan.Move != 0 || plan.ShapeAt != 0 || plan.Press < 70 {
		t.Fatalf("under 3 px there is no movement, only the click: %+v", plan)
	}
}

func TestComputeSubmoveKinds(t *testing.T) {
	p := DefaultProfile()
	target := Rect{1500, 900, 10, 10}
	var under, over int
	for seed := uint64(0); seed < 300; seed++ {
		in := Input{From: Point{20, 20}, To: target.Center(), Target: target}
		plan := Compute(in, p, newRand(seed))
		if !plan.Submove {
			t.Fatalf("ID %.2f should have a submovement", plan.ID)
		}
		if plan.Overshoot > 0 {
			over++
			if plan.Overshoot > p.OverPx {
				t.Fatalf("overshoot %.1f above %v px", plan.Overshoot, p.OverPx)
			}
		} else {
			under++
		}
		if plan.MainEnd >= plan.Move || plan.MainEnd < plan.Move/2 {
			t.Fatalf("main movement %v of %v", plan.MainEnd, plan.Move)
		}
		if msg := checkPlan(in, p, plan); msg != "" {
			t.Fatal(msg)
		}
	}
	if frac := float64(under) / 300; math.Abs(frac-p.Undershoot) > 0.08 {
		t.Fatalf("undershoot in %.2f of the moves, want about %.2f (over=%d)", frac, p.Undershoot, over)
	}
}

func TestComputeShortMoveSingleSegment(t *testing.T) {
	p := DefaultProfile()
	target := Rect{200, 100, 200, 60}
	plan := Compute(Input{From: Point{280, 120}, To: target.Center(), Target: target, Kind: Pointer}, p, newRand(6))
	if plan.Submove || plan.MainEnd != plan.Move || plan.Overshoot != 0 {
		t.Fatalf("easy target: one movement, got %+v", plan)
	}
	if plan.Move != p.MinMove {
		t.Fatalf("very easy movement is clamped to the minimum %v, got %v", p.MinMove, plan.Move)
	}
	if plan.ShapeAt != plan.Move-p.ShapeLead || plan.Shape != Pointer {
		t.Fatalf("shape changes %v ms before arriving: %+v", p.ShapeLead, plan)
	}
}

func TestComputeCorrection(t *testing.T) {
	p := DefaultProfile()
	in := Input{From: Point{300, 300}, To: Point{340, 312}, Target: Rect{320, 300, 40, 24}, Correct: true}
	plan := Compute(in, p, newRand(7))
	if plan.Move < p.CorrMin || plan.Move > p.CorrMax || plan.Submove {
		t.Fatalf("correction lasts %v ms", plan.Move)
	}
	if msg := checkPlan(in, p, plan); msg != "" {
		t.Fatal(msg)
	}
}

func TestComputeHugeArcIsLimited(t *testing.T) {
	p := DefaultProfile()
	p.ArcMin, p.ArcMax = 0.5, 0.5 // pede um arco enorme: o limite de 120 px manda
	in := Input{From: Point{0, 0}, To: Point{2000, 0}, Target: Rect{1950, -50, 100, 100}}
	if msg := checkPlan(in, p, Compute(in, p, newRand(8))); msg != "" {
		t.Fatal(msg)
	}
}

func TestPlanJSON(t *testing.T) {
	plan := Plan{Frames: []float64{0, 1.5, 2, 16, 3, 4}, Shape: Text, ShapeAt: 0, Move: 16, Press: 100, Release: 170, End: Point{3, 4}}
	b, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"f":[0,1.5,2,16,3,4],"shape":"text","shapeAt":0,"move":16,"press":100,"release":170,"fade":0}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
}

func TestWarpAndProfile(t *testing.T) {
	if math.Abs(peakOf(1)-0.5) > 1e-12 {
		t.Fatal("without warp the minimum-jerk peak is at the middle")
	}
	if minJerk(0) != 0 || minJerk(1) != 1 || math.Abs(minJerk(0.5)-0.5) > 1e-12 {
		t.Fatal("minimum jerk endpoints")
	}
	for _, peak := range []float64{0.35, 0.4, 0.45} {
		if k := warpFor(peak); math.Abs(peakOf(k)-peak) > 1e-9 || k >= 1 {
			t.Fatalf("warpFor(%v) = %v (peak %v)", peak, k, peakOf(k))
		}
	}
	if envelope(0, 0.1) != 0 || envelope(0.1, 0.1) != 0 || envelope(0.9, 0.1) != 0 || envelope(1, 0.1) != 0 || envelope(0.5, 0.1) != 1 {
		t.Fatal("envelope is zero at both ends and one in the middle")
	}
	// Faixa de pico estreita demais: usa a faixa como está.
	p := DefaultProfile()
	p.PeakMin, p.PeakMax = 0.4, 0.41
	seg := newSegment(Point{}, Point{100, 0}, 300, 0, 1, 0, p, newRand(9))
	if seg.peak < 0.4 || seg.peak > 0.41 {
		t.Fatalf("peak %v outside the narrow range", seg.peak)
	}
	if seg.at(-1) != (Point{}) || seg.at(2) != (Point{100, 0}) || seg.param(-1) != 0 || seg.param(1e9) != 1 {
		t.Fatal("segment endpoints")
	}
	if zero := newSegment(Point{5, 5}, Point{5, 5}, 300, 0, 1, 0, p, newRand(9)); zero.at(0.5) != (Point{5, 5}) {
		t.Fatal("zero-length segment stays put")
	}
}
