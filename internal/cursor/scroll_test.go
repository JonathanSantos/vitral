package cursor

import (
	"math"
	"reflect"
	"testing"

	"pgregory.net/rapid"
)

// checkScroll confere os invariantes da seção 1.4; devolve o primeiro problema ("" se ok).
func checkScroll(plan ScrollPlan, distance, viewport float64, speed Speed, p ScrollProfile) string {
	f := plan.Frames
	if len(f) < 2 || len(f)%2 != 0 {
		return "bad frame count"
	}
	for _, v := range f {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "NaN or Inf in frames"
		}
	}
	if f[0] != 0 || f[1] != 0 && len(f) > 2 {
		return "plan must start at t=0, offset 0"
	}
	if last := f[len(f)-1]; last != plan.Distance {
		return "plan must end exactly at the distance"
	}
	if math.Abs(plan.Distance-finite(distance)) > 1 {
		return "distance changed"
	}
	sign := math.Copysign(1, plan.Distance)
	for i := 2; i < len(f); i += 2 {
		if (f[i+1]-f[i-1])*sign < -1e-9 {
			return "movement goes back"
		}
		if dt := f[i] - f[i-2]; dt <= 0 || dt > p.Frame+1e-9 {
			return "keyframe spacing"
		}
	}
	if k := speed.Factor(); speed != Instant && plan.Duration > p.Cap*k+p.Frame+1 {
		return "over the time cap"
	}
	// Teto de velocidade: o passo planejado (PeakStep) bate com os keyframes, e só passa de MaxStep
	// quando nem o flick final depois do salto cabe nos dois tetos (aí o de tempo manda).
	measured := 0.0
	for i := 2; i < len(f); i += 2 {
		if i == 2 && plan.Jump != 0 {
			continue
		}
		measured = math.Max(measured, math.Abs(f[i+1]-f[i-1]))
	}
	if measured > plan.PeakStep*1.1+1 {
		return "PeakStep underestimates the real step"
	}
	if p.MaxStep > 0 && plan.PeakStep > p.MaxStep*viewport*1.001 && !(plan.Jump != 0 && plan.Duration >= p.Cap*speed.Factor()-p.Frame) {
		return "over the speed cap without falling back to the jump"
	}
	// Pico de velocidade nos primeiros 30% de cada flick (com a folga de um frame da amostragem).
	for i := range plan.Starts {
		start, end := plan.Starts[i], plan.Ends[i]
		if plan.Peaks[i]-start > 0.3*(end-start)+1e-9 {
			return "planned peak after 30% of the flick"
		}
		var best, at float64
		for j := 2; j < len(f); j += 2 {
			mid := (f[j] + f[j-2]) / 2
			if mid < start || mid > end {
				continue
			}
			if v := math.Abs(f[j+1]-f[j-1]) / (f[j] - f[j-2]); v > best {
				best, at = v, mid
			}
		}
		// Medir nos keyframes só faz sentido com pixels suficientes (o arredondamento é de 0,1 px).
		if math.Abs(plan.Distance) >= 100 && best > 0 && at-start > 0.3*(end-start)+p.Frame {
			return "measured peak after 30% of the flick"
		}
	}
	return ""
}

func TestPropertyScroll(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := DefaultScrollProfile()
		viewport := rapid.Float64Range(200, 1600).Draw(t, "viewport")
		distance := rapid.Float64Range(-20, 20).Draw(t, "views") * viewport
		speed := rapid.SampledFrom([]Speed{Slow, Normal, Fast}).Draw(t, "speed")
		seed := rapid.Uint64().Draw(t, "seed")
		plan := Scroll(distance, viewport, speed, Animated, p, newRand(seed))
		if msg := checkScroll(plan, distance, viewport, speed, p); msg != "" {
			t.Fatalf("%s: %+v", msg, plan)
		}
		if !reflect.DeepEqual(plan, Scroll(distance, viewport, speed, Animated, p, newRand(seed))) {
			t.Fatal("same seed, different plan")
		}
	})
}

func TestScrollShapes(t *testing.T) {
	p := DefaultScrollProfile()
	p.MaxStep = 0 // a forma dos flicks sem o teto de velocidade (ele tem teste próprio)
	vh := 700.0
	tests := []struct {
		views        float64
		flicks       int
		jump, capped bool
	}{
		{0.5, 1, false, false},
		{1.2, 1, false, false},
		{3, 2, false, false},
		{8, 1, true, false},
		{-3, 2, false, false},
	}
	for _, tt := range tests {
		plan := Scroll(tt.views*vh, vh, Normal, Animated, p, newRand(1))
		if plan.Flicks != tt.flicks || (plan.Jump != 0) != tt.jump {
			t.Errorf("%.1f viewports: %d flicks, jump %.0f", tt.views, plan.Flicks, plan.Jump)
		}
		if tt.jump && math.Abs(math.Abs(plan.Jump)-(8-p.JumpLeave)*vh) > 1 {
			t.Errorf("jump should leave %.0f viewport before the target: %.0f", p.JumpLeave, plan.Jump)
		}
		if msg := checkScroll(plan, tt.views*vh, vh, Normal, p); msg != "" {
			t.Errorf("%.1f viewports: %s", tt.views, msg)
		}
	}
	if plan := Scroll(5.5*vh, vh, Normal, Animated, p, newRand(2)); plan.Duration > p.Cap || plan.Flicks > p.MaxFlicks {
		t.Errorf("5.5 viewports must fit the cap: %+v", plan)
	}
}

// O teto de velocidade troca a compressão pelo salto: com 12% da viewport por frame e 1,2 s, a partir
// de duas telas o plano salta até uma tela antes e faz um flick só.
func TestScrollSpeedCap(t *testing.T) {
	p := DefaultScrollProfile()
	vh := 700.0
	for _, views := range []float64{2, 3, 5} {
		plan := Scroll(views*vh, vh, Normal, Animated, p, newRand(7))
		if plan.Jump == 0 || plan.Flicks != 1 || plan.PeakStep > p.MaxStep*vh {
			t.Errorf("%.0f viewports with the cap: jump %.0f, %d flicks, peak %.0f px/frame", views, plan.Jump, plan.Flicks, plan.PeakStep)
		}
	}
	slow := p
	slow.MaxStep = 0.3 // teto alto: 3 telas cabem em dois flicks
	if plan := Scroll(3*vh, vh, Normal, Animated, slow, newRand(7)); plan.Jump != 0 || plan.Flicks != 2 {
		t.Errorf("a higher cap keeps two flicks: %+v", plan)
	}
	// Um flick rápido demais é esticado (se ainda couber no teto de tempo).
	tight := p
	tight.MaxStep = 0.05
	if plan := Scroll(0.6*vh, vh, Normal, Animated, tight, newRand(8)); plan.TimeScale <= 1 || plan.PeakStep > 0.05*vh*1.001 {
		t.Errorf("fast flick should be stretched: scale %.2f, peak %.1f", plan.TimeScale, plan.PeakStep)
	}
}

// Fim do flick: a velocidade e a aceleração chegam a zero juntas (sem tranco); no modo natural, o
// flick termina em 1% do pico.
func TestFlickEnds(t *testing.T) {
	p := DefaultScrollProfile()
	f := newFlick(700, 700, p, 1, newRand(9))
	T := f.rise + f.decay
	slope := func(t float64) float64 { return (f.velocity(t+0.01) - f.velocity(t-0.01)) / 0.02 }
	maxSlope := 0.0
	for t := f.rise + 1; t < T-1; t++ {
		maxSlope = math.Max(maxSlope, math.Abs(slope(t)))
	}
	if v := f.velocity(T - 0.5); v > 1e-4 {
		t.Fatalf("velocity just before the end: %v", v)
	}
	if s := math.Abs(slope(T - 0.5)); s > maxSlope*0.01 {
		t.Fatalf("acceleration jumps at the end: %.5f vs max %.5f", s, maxSlope)
	}
	if math.Abs(f.velocity(f.rise)-1) > 1e-9 || math.Abs(f.velocity(f.rise-1e-9)-1) > 1e-6 {
		t.Fatal("velocity is continuous at the peak")
	}
	n := p
	n.Natural, n.TauMin, n.TauMax = true, 140, 200
	nf := newFlick(700, 700, n, 1, newRand(9))
	if v := nf.velocity(nf.rise + nf.decay - 0.01); math.Abs(v-0.01) > 1e-3 {
		t.Fatalf("natural decay ends at 1%% of the peak, got %v", v)
	}
	if nf.tau < 140*0.5 || nf.tau > 200 {
		t.Fatalf("natural tau %v", nf.tau)
	}
}

func TestScrollWithoutAnimation(t *testing.T) {
	p := DefaultScrollProfile()
	for _, tt := range []struct {
		speed  Speed
		motion Motion
	}{{Instant, Animated}, {Normal, Reduced}, {Normal, Hidden}} {
		plan := Scroll(1500, 700, tt.speed, tt.motion, p, newRand(3))
		if len(plan.Frames) != 2 || plan.Frames[1] != 1500 || plan.Duration != 0 {
			t.Fatalf("%v/%v must go straight to the end: %+v", tt.speed, tt.motion, plan)
		}
	}
	if plan := Scroll(3, 700, Normal, Animated, p, newRand(3)); len(plan.Frames) != 2 || plan.Frames[1] != 3 {
		t.Fatalf("a 3 px scroll is not a gesture: %+v", plan)
	}
	for _, d := range []float64{0, math.NaN(), math.Inf(1)} {
		plan := Scroll(d, 0, Normal, Animated, p, newRand(4))
		if msg := checkScroll(plan, d, 1, Normal, p); msg != "" || plan.Distance != 0 {
			t.Fatalf("distance %v: %s %+v", d, msg, plan)
		}
	}
	if plan := Scroll(1e9, 700, Normal, Animated, p, newRand(5)); checkScroll(plan, 1e9, 700, Normal, p) != "" || plan.Jump == 0 {
		t.Fatalf("huge distance must jump: %+v", plan.Jump)
	}
}

func TestTargetOffsetAndBand(t *testing.T) {
	p := DefaultScrollProfile()
	vh, header := 700.0, 64.0
	off := newSpread()
	rng := newRand(6)
	for i := 0; i < 500; i++ {
		d := TargetOffset(3000, vh, header, 1e6, 1e6, p, rng)
		top := 3000 - d
		off.add((top - header) / (vh - header))
		if !InBand(top, vh, header) {
			t.Fatalf("target top %.0f outside the band", top)
		}
	}
	off.covers(t, "target band", 0.30, 0.40, 0.01)
	if d := TargetOffset(3000, vh, header, 500, 0, p, rng); d != 500 {
		t.Fatalf("offset limited by the content end, got %v", d)
	}
	if d := TargetOffset(-400, vh, header, 0, 100, p, rng); d != -100 {
		t.Fatalf("offset up limited, got %v", d)
	}
	if InBand(header, vh, header) || InBand(vh, vh, header) {
		t.Fatal("top edge or bottom edge are outside the band")
	}
}
