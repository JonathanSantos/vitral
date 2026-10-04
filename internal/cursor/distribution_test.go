package cursor

import (
	"encoding/json"
	"flag"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Os sorteios cobrem as faixas da seção 4: cada valor fica dentro da faixa e o conjunto chega perto
// das duas pontas (a faixa não encolheu nem deslocou).
type spread struct{ min, max, sum, sq, n float64 }

func newSpread() *spread { return &spread{min: math.Inf(1), max: math.Inf(-1)} }

func (s *spread) add(v float64) {
	s.min, s.max = math.Min(s.min, v), math.Max(s.max, v)
	s.sum += v
	s.sq += v * v
	s.n++
}

func (s *spread) mean() float64 { return s.sum / s.n }
func (s *spread) std() float64  { return math.Sqrt(s.sq/s.n - s.mean()*s.mean()) }

// covers exige min e max dentro de [lo, hi] e a menos de slack das pontas.
func (s *spread) covers(t *testing.T, name string, lo, hi, slack float64) {
	t.Helper()
	if s.min < lo-1e-9 || s.max > hi+1e-9 || s.min > lo+slack || s.max < hi-slack {
		t.Fatalf("%s spans [%.4f, %.4f], want [%.4f, %.4f] (slack %.4f, n=%v)", name, s.min, s.max, lo, hi, slack, s.n)
	}
}

func TestDistributionTimings(t *testing.T) {
	p := DefaultProfile()
	aim, press, ratio, peak := newSpread(), newSpread(), newSpread(), newSpread()
	rng := newRand(20)
	for i := 0; i < 2000; i++ {
		target := Rect{float64(300 + i%900), float64(200 + i%500), 80, 40}
		in := Input{From: Point{10, 10}, To: target.Center(), Target: target}
		plan := Compute(in, p, rng)
		aim.add(plan.Press - plan.Move)
		press.add(plan.Release - plan.Press)
		peak.add(plan.Peak)
		if fitts := p.FittsA + p.FittsB*plan.ID; fitts > p.MinMove/(1-p.Jitter) && fitts < p.MaxMove/(1+p.Jitter) {
			ratio.add(plan.Move / fitts)
		}
	}
	aim.covers(t, "aim", 70, 140, 3)
	press.covers(t, "press", 60, 110, 3)
	peak.covers(t, "peak", 0.36, 0.44, 0.005)
	ratio.covers(t, "duration/Fitts", 0.918, 1.082, 0.012) // ±1 ms do arredondamento
	// Variação normal com σ = metade do limite: ~4%, não uniforme (seria ~4,6%) nem estreita.
	if sd := ratio.std(); sd < 0.032 || sd > 0.044 {
		t.Fatalf("duration jitter σ = %.4f, want about 0.04", sd)
	}
}

func TestDistributionSubmoves(t *testing.T) {
	p := DefaultProfile()
	frac, lateral, over, gap, corr := newSpread(), newSpread(), newSpread(), newSpread(), newSpread()
	var capped float64
	rng := newRand(21)
	for i := 0; i < 3000; i++ {
		target := Rect{float64(700 + i%600), float64(400 + i%300), 12, 12}
		from := Point{10, 10}
		in := Input{From: from, To: target.Center(), Target: target}
		plan := Compute(in, p, rng)
		if !plan.Submove {
			t.Fatalf("ID %.2f should have a submovement", plan.ID)
		}
		d := dist(roundPoint(from), plan.End)
		u, n := unit(from, plan.End), perp(unit(from, plan.End))
		mainEnd := frameAt(plan, plan.MainEnd).sub(roundPoint(from))
		if plan.Overshoot > 0 {
			// Longe, o teto de 24 px manda antes dos 6%.
			capped = math.Max(capped, plan.Overshoot)
		} else {
			frac.add(mainEnd.dot(u) / d)
			lateral.add(mainEnd.dot(n) / 12)
		}
		gap.add(plan.Gap)
		corr.add(plan.Move - plan.MainEnd - plan.Gap)
		// Parado durante a pausa entre os dois movimentos.
		for j := 0; j < len(plan.Frames); j += 3 {
			if ft := plan.Frames[j]; ft >= plan.MainEnd && ft <= plan.MainEnd+plan.Gap {
				if pt := (Point{plan.Frames[j+1], plan.Frames[j+2]}); pt != frameAt(plan, plan.MainEnd) {
					t.Fatalf("cursor moves during the pause at %v ms: %v", ft, pt)
				}
			}
		}
	}
	// Perto (D ≤ ~390 px, alvo de 2 px): o overshoot cobre 2–6% de D sem bater no teto.
	for i := 0; i < 3000; i++ {
		target := Rect{float64(150 + i%240), 300, 2, 2}
		from := Point{10, 300}
		plan := Compute(Input{From: from, To: target.Center(), Target: target}, p, rng)
		if plan.Overshoot > 0 {
			over.add(plan.Overshoot / dist(roundPoint(from), plan.End))
		}
	}
	if math.Abs(capped-p.OverPx) > 1e-9 {
		t.Fatalf("far movements should hit the %v px overshoot cap, max %v", p.OverPx, capped)
	}
	frac.covers(t, "undershoot fraction", 0.92, 0.97, 0.003)
	lateral.covers(t, "lateral error / W", -0.31, 0.31, 0.03) // 0,1 px de arredondamento em W = 12
	over.covers(t, "overshoot / D", 0.02, 0.06, 0.003)
	gap.covers(t, "pause", 20, 60, 2)
	corr.covers(t, "corrective", 80, 160, 3)
}

// frameAt é a posição do keyframe no instante t (t precisa ser um instante de keyframe ou o fim).
func frameAt(plan Plan, t float64) Point {
	f := plan.Frames
	for i := 0; i < len(f); i += 3 {
		if f[i] >= t {
			return Point{f[i+1], f[i+2]}
		}
	}
	return plan.End
}

func TestDistributionArc(t *testing.T) {
	p := DefaultProfile()
	dev, sides := newSpread(), map[int]int{}
	rng := newRand(22)
	for i := 0; i < 2000; i++ {
		target := Rect{400, 300, 200, 80} // ID baixo: um movimento só, sem erro lateral
		from := Point{float64(800 + i%300), float64(700 + i%200)}
		plan := Compute(Input{From: from, To: target.Center(), Target: target}, p, rng)
		d := dist(from, plan.End)
		n := perp(unit(from, plan.End))
		var worst float64
		for j := 0; j < len(plan.Frames); j += 3 {
			off := Point{plan.Frames[j+1], plan.Frames[j+2]}.sub(from).dot(n)
			if math.Abs(off) > math.Abs(worst) {
				worst = off
			}
		}
		dev.add(math.Abs(worst) / d)
		// O lado do arco é o lado do plano.
		if math.Signbit(worst) != (plan.Side < 0) {
			t.Fatalf("arc bends to %v but plan.Side = %d", worst, plan.Side)
		}
		sides[plan.Side]++
	}
	// Desvio máximo da Bézier com o 2º controle na metade do 1º: ~0,577 × deslocamento (4% a 12% de D).
	// Folga de 0,002 para o tremor e o arredondamento de 0,1 px.
	dev.covers(t, "arc deviation / D", 0.577*0.04-0.002, 0.577*0.12+0.002, 0.006)
	if sides[1] < 600 || sides[-1] < 600 {
		t.Fatalf("both sides should appear: %v", sides)
	}
}

func TestTextCandidatesSpread(t *testing.T) {
	p := DefaultProfile()
	field := Rect{0, 0, 360, 40}
	inner := field.Inset(p.Inner)
	xs, ys := newSpread(), newSpread()
	rng := newRand(23)
	for i := 0; i < 1000; i++ {
		for _, c := range Candidates(field, Text, p, rng)[:p.Tries] {
			xs.add(c.X)
			ys.add(c.Y)
		}
	}
	cx, cy := inner.X+inner.W/6, inner.Y+inner.H/2
	if math.Abs(xs.mean()-cx) > 1 || math.Abs(xs.std()-inner.W/18) > 1.2 || math.Abs(ys.mean()-cy) > 0.3 || math.Abs(ys.std()-inner.H/20) > 0.2 {
		t.Fatalf("text spread x %.1f±%.1f y %.1f±%.2f, want %.1f±%.1f and %.1f±%.2f",
			xs.mean(), xs.std(), ys.mean(), ys.std(), cx, inner.W/18, cy, inner.H/20)
	}
}

func TestPointerCandidatesCentered(t *testing.T) {
	p := DefaultProfile()
	xs := newSpread()
	rng := newRand(24)
	for i := 0; i < 1000; i++ {
		for _, c := range Candidates(Rect{0, 0, 200, 100}, Pointer, p, rng)[:p.Tries] {
			xs.add(c.X)
		}
	}
	xs.covers(t, "pointer x", 30, 170, 25)
	if math.Abs(xs.mean()-100) > 2 {
		t.Fatalf("pointer candidates should center on the target, mean x %.1f", xs.mean())
	}
}

func TestMoreNamedCases(t *testing.T) {
	p := DefaultProfile()
	t.Run("3 px exactly is a movement", func(t *testing.T) {
		plan := Compute(Input{From: Point{0, 0}, To: Point{3, 0}, Target: Rect{0, -10, 20, 20}}, p, newRand(30))
		if plan.Move == 0 || len(plan.Frames) == 3 {
			t.Fatalf("3 px should animate: %+v", plan)
		}
	})
	t.Run("speed scales reduced motion and corrections", func(t *testing.T) {
		slow := p
		slow.Speed = Slow
		red := Compute(Input{From: Point{0, 0}, To: Point{300, 0}, Target: Rect{290, -10, 20, 20}, Motion: Reduced}, slow, newRand(31))
		if aim := red.Press - red.Move; aim < 70*1.6 || aim > 140*1.6 {
			t.Fatalf("slow reduced-motion aim %v", aim)
		}
		cor := Compute(Input{From: Point{0, 0}, To: Point{40, 0}, Target: Rect{30, -10, 20, 20}, Correct: true}, slow, newRand(32))
		if cor.Move < 80*1.6 || cor.Move > 160*1.6 {
			t.Fatalf("slow correction lasts %v", cor.Move)
		}
	})
	t.Run("long correction keeps half for the main movement", func(t *testing.T) {
		q := p
		q.CorrMin, q.CorrMax, q.GapMin, q.GapMax = 400, 400, 100, 100
		plan := Compute(Input{From: Point{0, 0}, To: Point{1500, 0}, Target: Rect{1495, -5, 10, 10}}, q, newRand(33))
		if math.Abs(plan.MainEnd-plan.Move/2) > 1 {
			t.Fatalf("main %v of %v, want half", plan.MainEnd, plan.Move)
		}
		if math.Abs(plan.Gap/(plan.Move-plan.MainEnd-plan.Gap)-0.25) > 0.02 {
			t.Fatalf("pause and correction keep their proportion: gap %v corr %v", plan.Gap, plan.Move-plan.MainEnd-plan.Gap)
		}
	})
	t.Run("huge lateral error is limited", func(t *testing.T) {
		q := p
		q.UnderLateral, q.Undershoot = 50, 1
		for seed := uint64(0); seed < 50; seed++ {
			in := Input{From: Point{0, 0}, To: Point{400, 0}, Target: Rect{395, -10, 10, 20}}
			if msg := checkPlan(in, q, Compute(in, q, newRand(seed))); msg != "" {
				t.Fatal(msg)
			}
		}
	})
	t.Run("correction curvature is at most 2%", func(t *testing.T) {
		var worst float64
		for seed := uint64(0); seed < 200; seed++ {
			plan := Compute(Input{From: Point{0, 0}, To: Point{200, 0}, Target: Rect{190, -10, 20, 20}, Correct: true}, p, newRand(seed))
			for i := 0; i < len(plan.Frames); i += 3 {
				worst = math.Max(worst, math.Abs(plan.Frames[i+2]))
			}
		}
		if worst > 0.02*200*0.6+round || worst < 0.01*200*0.577 {
			t.Fatalf("correction bends %.2f px over 200 px", worst)
		}
	})
	t.Run("tremor amplitude", func(t *testing.T) {
		if tremorAmp(p, 500) != 0.4 || math.Abs(tremorAmp(p, 10)-0.2) > 1e-12 {
			t.Fatal("tremor is 0.4 px, at most 2% of the distance")
		}
		with := newSegment(Point{}, Point{500, 0}, 600, 20, 1, p.TremorAmp, p, newRand(34))
		without := newSegment(Point{}, Point{500, 0}, 600, 20, 1, 0, p, newRand(34))
		var peak float64
		for tau := 0.3; tau <= 0.7; tau += 0.001 {
			peak = math.Max(peak, dist(with.at(tau), without.at(tau)))
		}
		if peak < 0.1 { // a soma de senoides raramente chega ao pico teórico; sem tremor seria 0
			t.Fatalf("tremor should show in the middle of the movement, peak %.3f px", peak)
		}
	})
	t.Run("envelope shape", func(t *testing.T) {
		if v := envelope(0.25, 0.1); math.Abs(v-math.Pow(math.Sin(math.Pi*0.1875), 2)) > 1e-12 {
			t.Fatalf("envelope(0.25) = %v", v)
		}
	})
	t.Run("free area ties keep the first (top) rectangle", func(t *testing.T) {
		got, _ := FreeArea(Rect{0, 0, 100, 100}, []Rect{{0, 45, 100, 10}})
		if got != (Rect{0, 0, 100, 45}) {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("obstacles are clipped to the target", func(t *testing.T) {
		got, _ := FreeArea(Rect{0, 0, 100, 100}, []Rect{{70, 80, 100, 100}})
		if got != (Rect{0, 0, 100, 80}) {
			t.Fatalf("got %v", got)
		}
		got, _ = FreeArea(Rect{10, 10, 100, 100}, []Rect{{-50, 0, 80, 200}})
		if got != (Rect{30, 10, 80, 100}) {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("round inside a sub-0.1 px area", func(t *testing.T) {
		if got := roundInside(Point{}, Rect{1.06, 1.06, 0.03, 0.03}); got != (Point{1.1, 1.1}) {
			t.Fatalf("got %v", got)
		}
		if got := roundInside(Point{}, Rect{1.02, 1.02, 0.04, 0.04}); got != (Point{1, 1}) {
			t.Fatalf("got %v", got)
		}
	})
}

var update = flag.Bool("update", false, "rewrite the golden plans")

// Planos exatos para entradas fixas: a física é determinística, então qualquer mudança nos números
// aparece aqui (revise no cursorlab e rode com -update para aceitar).
func TestGoldenPlans(t *testing.T) {
	cases := map[string]Input{
		"short-pointer":  {From: Point{100, 100}, To: Point{240, 130}, Target: Rect{200, 110, 80, 40}, Kind: Pointer},
		"long-small":     {From: Point{20, 20}, To: Point{1180, 700}, Target: Rect{1172, 692, 16, 16}, Kind: Pointer, Side: 1},
		"text-field":     {From: Point{600, 500}, To: Point{130, 220}, Target: Rect{100, 200, 300, 40}, Kind: Text, Side: -1},
		"vertical":       {From: Point{400, 50}, To: Point{400, 650}, Target: Rect{340, 630, 120, 40}},
		"correction":     {From: Point{300, 300}, To: Point{320, 290}, Target: Rect{300, 280, 40, 20}, Correct: true},
		"reduced-motion": {From: Point{0, 0}, To: Point{500, 400}, Target: Rect{480, 390, 40, 20}, Motion: Reduced},
	}
	got := map[string]Plan{}
	for name, in := range cases {
		got[name] = Compute(in, DefaultProfile(), newRand(40))
	}
	data, err := json.MarshalIndent(got, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "plans.golden.json")
	if *update {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if string(want) != string(data) {
		t.Fatalf("plans differ from %s (review in tools/cursorlab, then run with -update)", path)
	}
}

func TestLimitsUnderPressure(t *testing.T) {
	p := DefaultProfile()
	t.Run("slow speed scales press and the pause", func(t *testing.T) {
		slow := p
		slow.Speed = Slow
		red := Compute(Input{From: Point{0, 0}, To: Point{300, 0}, Target: Rect{290, -10, 20, 20}, Motion: Reduced}, slow, newRand(50))
		if pr := red.Release - red.Press; pr < 60*1.6 || pr > 110*1.6 {
			t.Fatalf("slow reduced-motion press %v", pr)
		}
		gap := newSpread()
		for seed := uint64(0); seed < 200; seed++ {
			plan := Compute(Input{From: Point{0, 0}, To: Point{900, 0}, Target: Rect{895, -5, 10, 10}}, slow, newRand(seed))
			gap.add(plan.Gap)
		}
		gap.covers(t, "slow pause", 20*1.6, 60*1.6, 4)
	})
	t.Run("arc limit holds with forced lateral errors", func(t *testing.T) {
		q := p
		q.ArcMin, q.ArcMax, q.Undershoot = 0.5, 0.5, 1
		for seed := uint64(0); seed < 100; seed++ {
			in := Input{From: Point{0, 0}, To: Point{2000, 0}, Target: Rect{1990, -10, 20, 20}}
			if msg := checkPlan(in, q, Compute(in, q, newRand(seed))); msg != "" {
				t.Fatalf("seed %d: %s", seed, msg)
			}
		}
	})
	t.Run("corrective curvature respects the limit", func(t *testing.T) {
		q := p
		q.CorrCurve, q.Undershoot = 5, 1 // pede curvatura absurda no corretivo
		for seed := uint64(0); seed < 100; seed++ {
			in := Input{From: Point{0, 0}, To: Point{3000, 0}, Target: Rect{2990, -10, 20, 20}}
			if msg := checkPlan(in, q, Compute(in, q, newRand(seed))); msg != "" {
				t.Fatalf("seed %d: %s", seed, msg)
			}
		}
	})
	t.Run("narrow peak ranges are used as they are", func(t *testing.T) {
		q := p
		q.PeakMin, q.PeakMax = 0.4, 0.4
		if seg := newSegment(Point{}, Point{100, 0}, 300, 0, 1, 0, q, newRand(51)); seg.peak != 0.4 {
			t.Fatalf("fixed peak 0.4, got %v", seg.peak)
		}
		q.PeakMin, q.PeakMax = 0.40, 0.42
		seen := map[float64]bool{}
		for seed := uint64(0); seed < 20; seed++ {
			seg := newSegment(Point{}, Point{100, 0}, 300, 0, 1, 0, q, newRand(seed))
			if seg.peak < 0.40 || seg.peak > 0.42 {
				t.Fatalf("peak %v outside [0.40, 0.42]", seg.peak)
			}
			seen[seg.peak] = true
		}
		if len(seen) < 10 {
			t.Fatal("a 0.02 range is used whole, not shrunk to a point")
		}
	})
}
