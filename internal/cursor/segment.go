package cursor

import (
	"math"
	"math/rand/v2"
	"sort"
)

// segment é um trecho de movimento: Bézier cúbica percorrida por comprimento de arco com perfil de
// jerk mínimo deformado no tempo, mais tremor perpendicular à corda.
type segment struct {
	a, c1, c2, b Point
	n            Point     // perpendicular à corda (direção do tremor)
	lens         []float64 // comprimento acumulado nas amostras do parâmetro
	length       float64
	k            float64 // deformação do tempo (pico de velocidade antes da metade)
	peak         float64
	side         int
	dur          float64 // ms
	amp          float64 // px
	edge         float64
	waves        [3]wave
}

type wave struct{ weight, hz, phase float64 }

const arcSamples = 256

// newSegment monta a curva de a até b. Os pontos de controle ficam a 30% e 70% da corda, deslocados
// na perpendicular: offset no primeiro e metade no segundo, do mesmo lado (arco mais curvo na saída).
func newSegment(a, b Point, dur, offset float64, side int, amp float64, p Profile, rng *rand.Rand) segment {
	d := dist(a, b)
	u := unit(a, b)
	s := segment{a: a, b: b, n: perp(u), side: side, dur: dur, amp: amp, edge: p.TremorEdge}
	off := s.n.scale(offset * float64(side))
	s.c1 = a.add(u.scale(0.3 * d)).add(off)
	s.c2 = a.add(u.scale(0.7 * d)).add(off.scale(0.5))

	lo, hi := p.PeakMin, p.PeakMax
	if hi-lo > 0.02 { // folga para a amostragem em keyframes não cair fora da faixa
		lo, hi = lo+0.01, hi-0.01
	}
	s.peak = uniform(rng, lo, hi)
	s.k = warpFor(s.peak)

	var sum float64
	for i := range s.waves {
		s.waves[i] = wave{weight: uniform(rng, 0.5, 1), hz: uniform(rng, p.TremorHzMin, p.TremorHzMax), phase: uniform(rng, 0, 2*math.Pi)}
		sum += s.waves[i].weight
	}
	for i := range s.waves {
		s.waves[i].weight /= sum // |ruído| ≤ 1
	}

	s.lens = make([]float64, arcSamples+1)
	prev := a
	for i := 1; i <= arcSamples; i++ {
		pt := s.bezier(float64(i) / arcSamples)
		s.lens[i] = s.lens[i-1] + dist(prev, pt)
		prev = pt
	}
	s.length = s.lens[arcSamples]
	return s
}

func (s segment) bezier(t float64) Point {
	mt := 1 - t
	return s.a.scale(mt * mt * mt).
		add(s.c1.scale(3 * mt * mt * t)).
		add(s.c2.scale(3 * mt * t * t)).
		add(s.b.scale(t * t * t))
}

// at é a posição na fração tau da duração do trecho.
func (s segment) at(tau float64) Point {
	switch {
	case tau <= 0 || s.length == 0:
		return s.a
	case tau >= 1:
		return s.b
	}
	pt := s.bezier(s.param(progress(tau, s.k) * s.length))
	if e := envelope(tau, s.edge); e > 0 && s.amp > 0 {
		pt = pt.add(s.n.scale(s.amp * e * s.noise(tau*s.dur)))
	}
	return pt
}

// param inverte o comprimento de arco: a velocidade visual é a do perfil, não a da Bézier.
func (s segment) param(length float64) float64 {
	i := sort.SearchFloat64s(s.lens, length)
	if i <= 0 {
		return 0
	}
	if i > arcSamples {
		return 1
	}
	span := s.lens[i] - s.lens[i-1]
	f := 0.0
	if span > 0 {
		f = (length - s.lens[i-1]) / span
	}
	return (float64(i-1) + f) / arcSamples
}

// noise é ruído suavizado (soma de senoides de 8 a 12 Hz), entre -1 e 1.
func (s segment) noise(ms float64) float64 {
	var v float64
	for _, w := range s.waves {
		v += w.weight * math.Sin(2*math.Pi*w.hz*ms/1000+w.phase)
	}
	return v
}

// minJerk é o perfil de jerk mínimo: fração percorrida em função do tempo normalizado.
func minJerk(t float64) float64 { return t * t * t * (10 + t*(-15+6*t)) }

// progress aplica a deformação de tempo t^k: com k < 1 o pico sai da metade para antes (4.4).
func progress(tau, k float64) float64 { return minJerk(math.Pow(tau, k)) }

// peakOf é onde fica o pico de velocidade de progress com a deformação k. Derivando
// v(τ) ∝ τ^(3k-1)·(1-τ^k)², o máximo é em τ^k = (3k-1)/(5k-1).
func peakOf(k float64) float64 { return math.Pow((3*k-1)/(5*k-1), 1/k) }

// warpFor acha k para o pico cair em peak (peakOf cresce com k; k = 1 dá 0,5).
func warpFor(peak float64) float64 {
	lo, hi := 1.0/3+1e-6, 1.0
	for i := 0; i < 60; i++ {
		mid := (lo + hi) / 2
		if peakOf(mid) < peak {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// envelope zera o tremor no começo e no fim do movimento (fração edge de cada lado).
func envelope(tau, edge float64) float64 {
	if tau <= edge || tau >= 1-edge {
		return 0
	}
	x := math.Sin(math.Pi * (tau - edge) / (1 - 2*edge))
	return x * x
}
