package cursor

import (
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"pgregory.net/rapid"
)

func genRect(t *rapid.T) Rect {
	return Rect{
		X: rapid.Float64Range(-300, 3000).Draw(t, "x"), Y: rapid.Float64Range(-300, 3000).Draw(t, "y"),
		W: rapid.Float64Range(1, 2000).Draw(t, "w"), H: rapid.Float64Range(1, 2000).Draw(t, "h"),
	}
}

func genSpeed(t *rapid.T) Speed {
	return rapid.SampledFrom([]Speed{Slow, Normal, Fast}).Draw(t, "speed")
}

// Um movimento qualquer: alvo sorteado, ponto entre os candidatos, de qualquer lugar.
func genMove(t *rapid.T) (Input, Profile, *rand.Rand) {
	p := DefaultProfile()
	p.Speed = genSpeed(t)
	seed := rapid.Uint64().Draw(t, "seed")
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b9))
	target := genRect(t)
	kind := Kind(rapid.IntRange(0, 2).Draw(t, "kind"))
	cands := Candidates(target, kind, p, rng)
	in := Input{
		From:   Point{rapid.Float64Range(-300, 3000).Draw(t, "fx"), rapid.Float64Range(-300, 3000).Draw(t, "fy")},
		To:     cands[rapid.IntRange(0, len(cands)-1).Draw(t, "pick")],
		Target: target, Kind: kind,
		Side:    rapid.IntRange(-1, 1).Draw(t, "side"),
		Correct: rapid.Float64Range(0, 1).Draw(t, "correct") < 0.15,
	}
	return in, p, rng
}

func TestPropertyPlanInvariants(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in, p, rng := genMove(t)
		if msg := checkPlan(in, p, Compute(in, p, rng)); msg != "" {
			t.Fatal(msg)
		}
	})
}

// Movimentos longos e para alvos pequenos: é onde moram submovimentos e overshoot.
func TestPropertyHardTargets(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := DefaultProfile()
		seed := rapid.Uint64().Draw(t, "seed")
		rng := rand.New(rand.NewPCG(seed, 1))
		size := rapid.Float64Range(1, 24).Draw(t, "size")
		target := Rect{rapid.Float64Range(0, 2500).Draw(t, "x"), rapid.Float64Range(0, 1400).Draw(t, "y"), size, size}
		in := Input{From: Point{rapid.Float64Range(0, 2500).Draw(t, "fx"), 0}, To: Candidates(target, Pointer, p, rng)[0], Target: target}
		plan := Compute(in, p, rng)
		if msg := checkPlan(in, p, plan); msg != "" {
			t.Fatal(msg)
		}
	})
}

func TestPropertyCandidatesInsideInner(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := DefaultProfile()
		target := genRect(t)
		kind := Kind(rapid.IntRange(0, 2).Draw(t, "kind"))
		seed := rapid.Uint64().Draw(t, "seed")
		cands := Candidates(target, kind, p, rand.New(rand.NewPCG(seed, 2)))
		if len(cands) != p.Tries+1 {
			t.Fatalf("got %d candidates", len(cands))
		}
		inner := target.Inset(p.Inner)
		if kind == Text {
			inner.W /= 3
		}
		// Estrito; só uma área menor que 0,1 px (sem ponto da grade de 0,1 dentro) aceita o centro arredondado.
		tol := 1e-9
		if inner.W < 0.1 || inner.H < 0.1 {
			tol = 0.05 + 1e-9
		}
		grown := Rect{inner.X - tol, inner.Y - tol, inner.W + 2*tol, inner.H + 2*tol}
		for i, c := range cands {
			if !grown.Contains(c) {
				t.Fatalf("candidate %d %v outside the inner area %v", i, c, inner)
			}
		}
	})
}

// Pico de velocidade do perfil (contínuo) e medido nos keyframes do movimento principal.
func TestPropertyVelocityPeak(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := DefaultProfile()
		peak := rapid.Float64Range(p.PeakMin+0.01, p.PeakMax-0.01).Draw(t, "peak")
		k := warpFor(peak)
		if got := numericPeak(func(tau float64) float64 { return progress(tau, k) }); math.Abs(got-peak) > 0.002 {
			t.Fatalf("peak at %.4f, want %.4f (k=%.4f)", got, peak, k)
		}

		seed := rapid.Uint64().Draw(t, "seed")
		rng := rand.New(rand.NewPCG(seed, 3))
		target := Rect{rapid.Float64Range(600, 1800).Draw(t, "x"), rapid.Float64Range(300, 900).Draw(t, "y"), 120, 40}
		in := Input{From: Point{10, 10}, To: target.Center(), Target: target}
		plan := Compute(in, p, rng)
		if plan.MainEnd < 400 {
			return // poucos keyframes para medir
		}
		var bestV, bestT float64
		f := plan.Frames
		for i := 3; i < len(f) && f[i] <= plan.MainEnd; i += 3 {
			v := dist(Point{f[i-2], f[i-1]}, Point{f[i+1], f[i+2]}) / (f[i] - f[i-3])
			if v > bestV {
				bestV, bestT = v, (f[i]+f[i-3])/2
			}
		}
		if frac := bestT / plan.MainEnd; frac < p.PeakMin-0.03 || frac > p.PeakMax+0.03 {
			t.Fatalf("measured peak at %.2f of the main movement (planned %.2f)", frac, plan.Peak)
		}
	})
}

func numericPeak(s func(float64) float64) float64 {
	const steps = 20000
	var best, at float64
	for i := 0; i < steps; i++ {
		a, b := float64(i)/steps, float64(i+1)/steps
		if v := s(b) - s(a); v > best {
			best, at = v, (a+b)/2
		}
	}
	return at
}

// O tremor é zero nas duas pontas: os primeiros e os últimos 10% são idênticos à curva sem tremor.
func TestPropertyTremorEdges(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := DefaultProfile()
		seed := rapid.Uint64().Draw(t, "seed")
		a := Point{rapid.Float64Range(0, 1000).Draw(t, "ax"), rapid.Float64Range(0, 1000).Draw(t, "ay")}
		b := Point{rapid.Float64Range(0, 1000).Draw(t, "bx"), rapid.Float64Range(0, 1000).Draw(t, "by")}
		dur := rapid.Float64Range(100, 1500).Draw(t, "dur")
		with := newSegment(a, b, dur, 30, 1, p.TremorAmp, p, rand.New(rand.NewPCG(seed, 4)))
		without := newSegment(a, b, dur, 30, 1, 0, p, rand.New(rand.NewPCG(seed, 4)))
		tau := rapid.Float64Range(0, 1).Draw(t, "tau")
		gap := dist(with.at(tau), without.at(tau))
		if tau <= p.TremorEdge || tau >= 1-p.TremorEdge {
			if gap != 0 {
				t.Fatalf("tremor %.4f px at tau=%.3f (must be zero at both ends)", gap, tau)
			}
		} else if gap > p.TremorAmp+1e-9 {
			t.Fatalf("tremor %.4f px exceeds the amplitude", gap)
		}
	})
}

func TestPropertyDeterminism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		seed := rapid.Uint64().Draw(t, "seed")
		p := DefaultProfile()
		target := Rect{rapid.Float64Range(200, 1500).Draw(t, "x"), 400, 60, 30}
		run := func(seed uint64) []Plan {
			target := target
			rng := rand.New(rand.NewPCG(seed, 5))
			var plans []Plan
			from, side := Point{20, 20}, 0
			for i := 0; i < 3; i++ {
				to := Candidates(target, Pointer, p, rng)[0]
				plan := Compute(Input{From: from, To: to, Target: target, Side: side}, p, rng)
				plans = append(plans, plan)
				from, side = plan.End, plan.Side
				target.Y += 100
			}
			return plans
		}
		if !reflect.DeepEqual(run(seed), run(seed)) {
			t.Fatal("same seed produced different plans")
		}
		if reflect.DeepEqual(run(seed), run(seed+1)) {
			t.Fatal("different seeds produced the same plans")
		}
	})
}

// FreeArea nunca devolve área sobre um obstáculo nem fora do alvo.
func TestPropertyFreeArea(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		target := Rect{0, 0, rapid.Float64Range(1, 500).Draw(t, "w"), rapid.Float64Range(1, 300).Draw(t, "h")}
		obs := rapid.SliceOfN(rapid.Custom(func(t *rapid.T) Rect {
			return Rect{
				rapid.Float64Range(-50, 500).Draw(t, "ox"), rapid.Float64Range(-50, 300).Draw(t, "oy"),
				rapid.Float64Range(0, 200).Draw(t, "ow"), rapid.Float64Range(0, 200).Draw(t, "oh"),
			}
		}), 0, 8).Draw(t, "obstacles")
		free, ok := FreeArea(target, obs)
		if !ok {
			return
		}
		if free.X < target.X || free.Y < target.Y || free.X+free.W > target.X+target.W+1e-9 || free.Y+free.H > target.Y+target.H+1e-9 {
			t.Fatalf("free area %v outside target %v", free, target)
		}
		for _, o := range obs {
			ix := math.Min(free.X+free.W, o.X+o.W) - math.Max(free.X, o.X)
			iy := math.Min(free.Y+free.H, o.Y+o.H) - math.Max(free.Y, o.Y)
			if ix > 1e-9 && iy > 1e-9 {
				t.Fatalf("free area %v overlaps obstacle %v", free, o)
			}
		}
	})
}
