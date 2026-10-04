package cursor

import (
	"math"
	"math/rand/v2"
)

// ScrollProfile tem os parâmetros da rolagem humana (pedido "próximos passos", seção 1.2). Os padrões
// ficam em DefaultScrollProfile; a velocidade (Speed.Factor) escala tempos e teto.
type ScrollProfile struct {
	RiseMin, RiseMax     float64 // ms até o pico de velocidade de um flick
	TauMin, TauMax       float64 // ms: constante do atrito (decaimento exponencial)
	DecayTaus            float64 // duração do decaimento em múltiplos de τ (a velocidade chega a zero no fim)
	SmallFlick           float64 // em viewports: flicks menores encurtam o atrito proporcionalmente
	MinTauScale          float64 // menor fração de τ num flick pequeno
	SingleFlick          float64 // em viewports: até aqui, um flick só
	FirstMin, FirstMax   float64 // fração da distância coberta por cada flick antes do último
	MaxFlicks            int
	PauseMin, PauseMax   float64 // ms "olhando a tela" entre flicks
	JumpBeyond           float64 // em viewports: acima disso, salto até perto do alvo e um flick final
	JumpLeave            float64 // em viewports: quanto falta depois do salto
	Cap                  float64 // ms: teto total em normal
	MinCompress          float64 // menor fator de compressão do tempo antes de preferir o salto
	TargetMin, TargetMax float64 // faixa da viewport onde o topo do alvo termina
	Frame                float64
	Still                float64 // px: abaixo disso, vai direto (não vale um gesto)
}

// DefaultScrollProfile é o ponto de partida da velocidade normal.
func DefaultScrollProfile() ScrollProfile {
	return ScrollProfile{
		RiseMin: 60, RiseMax: 90, TauMin: 220, TauMax: 320, DecayTaus: 2.2,
		SmallFlick: 1.2, MinTauScale: 0.5, SingleFlick: 1.2,
		FirstMin: 0.75, FirstMax: 0.90, MaxFlicks: 3, PauseMin: 120, PauseMax: 250,
		JumpBeyond: 6, JumpLeave: 1, Cap: 1200, MinCompress: 0.6,
		TargetMin: 0.30, TargetMax: 0.40, Frame: 16, Still: 4,
	}
}

// ScrollPlan é a rolagem de um container: deslocamentos acumulados a partir da posição inicial.
type ScrollPlan struct {
	Frames   []float64 // t, deslocamento, t, deslocamento… a cada Frame ms; o último é exatamente Distance
	Distance float64
	Duration float64
	Jump     float64 // salto instantâneo no começo (0 se não houve)
	Flicks   int
	Peaks    []float64 // instante do pico de velocidade de cada flick (ms, para testes e cursorlab)
	Starts   []float64 // início de cada flick (ms)
	Ends     []float64 // fim de cada flick (ms)
	Compress float64   // fator aplicado ao tempo para caber no teto (1 = nenhum)
}

// flick é um gesto: sobe rápido até o pico e decai por atrito até parar.
type flick struct {
	start, rise, decay, dist float64
	norm                     float64 // integral da forma de velocidade, para escalar até dist
}

// velocity é a forma da velocidade (sem escala) em t ms desde o início do flick.
func (f flick) velocity(t float64) float64 {
	switch {
	case t <= 0 || t >= f.rise+f.decay:
		return 0
	case t < f.rise:
		x := t / f.rise
		return x * x * (3 - 2*x) // smoothstep até o pico
	}
	// Decaimento exponencial deslocado para chegar a zero exatamente no fim (parada sem tranco).
	tau := f.decay / 2.2
	end := math.Exp(-f.decay / tau)
	return (math.Exp(-(t-f.rise)/tau) - end) / (1 - end)
}

// progress é a fração da distância percorrida em t, pela integral numérica da velocidade.
func (f flick) progress(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= f.rise+f.decay {
		return 1
	}
	return integrate(f.velocity, t) / f.norm
}

func integrate(v func(float64) float64, upTo float64) float64 {
	const step = 1.0
	var sum float64
	for t := 0.0; t < upTo; t += step {
		h := math.Min(step, upTo-t)
		sum += (v(t) + v(t+h)) / 2 * h
	}
	return sum
}

// newFlick monta um flick de dist px; viewport dá a escala do atrito (flicks pequenos são curtos).
func newFlick(dist, viewport float64, p ScrollProfile, k float64, rng *rand.Rand) flick {
	rise := uniform(rng, p.RiseMin, p.RiseMax) * k
	tau := uniform(rng, p.TauMin, p.TauMax) * k
	size := math.Abs(dist) / math.Max(viewport, 1)
	tau *= clamp(size/p.SmallFlick, p.MinTauScale, 1)
	f := flick{rise: rise, decay: p.DecayTaus * tau, dist: dist}
	f.norm = integrate(f.velocity, f.rise+f.decay)
	return f
}

// Scroll planeja a rolagem de distance px (negativo = para cima) num container de altura viewport.
// Com speed instant (ou motion diferente de Animated), vai direto: um só keyframe no destino.
func Scroll(distance, viewport float64, speed Speed, motion Motion, p ScrollProfile, rng *rand.Rand) ScrollPlan {
	distance, viewport = finite(distance), math.Max(finite(viewport), 1)
	plan := ScrollPlan{Distance: distance, Compress: 1}
	k := speed.Factor()
	if math.Abs(distance) < p.Still || speed == Instant || motion != Animated {
		plan.Frames = []float64{0, distance}
		return plan
	}
	sign := math.Copysign(1, distance)
	abs := math.Abs(distance)
	views := abs / viewport

	var flicks []flick
	var pauses []float64
	jump := 0.0
	build := func(remaining float64) {
		for len(flicks) < p.MaxFlicks && remaining > 0.5 {
			chunk := remaining
			if len(flicks) < p.MaxFlicks-1 && remaining/viewport > p.SingleFlick {
				chunk = remaining * uniform(rng, p.FirstMin, p.FirstMax)
			}
			flicks = append(flicks, newFlick(chunk, viewport, p, k, rng))
			remaining -= chunk
			if remaining > 0.5 {
				pauses = append(pauses, uniform(rng, p.PauseMin, p.PauseMax)*k)
			}
		}
		if remaining > 0.5 { // sobrou com o máximo de flicks: o último leva o resto
			flicks[len(flicks)-1].dist += remaining
		}
	}
	if views > p.JumpBeyond {
		jump = abs - p.JumpLeave*viewport
		build(abs - jump)
	} else {
		build(abs)
	}
	total := func() float64 {
		d := 0.0
		for _, f := range flicks {
			d += f.rise + f.decay
		}
		for _, s := range pauses {
			d += s
		}
		return d
	}
	// Teto de tempo: comprime até MinCompress; se ainda não cabe, salto + flick final.
	if cap := p.Cap * k; total() > cap {
		if c := cap / total(); c >= p.MinCompress && jump == 0 {
			plan.Compress = c
		} else {
			flicks, pauses = nil, nil
			jump = math.Max(abs-p.JumpLeave*viewport, 0)
			build(abs - jump)
			if t := total(); t > cap {
				plan.Compress = cap / t
			}
		}
	}
	for i := range flicks {
		flicks[i].rise *= plan.Compress
		flicks[i].decay *= plan.Compress
		flicks[i].norm = integrate(flicks[i].velocity, flicks[i].rise+flicks[i].decay)
	}
	for i := range pauses {
		pauses[i] *= plan.Compress
	}

	// Linha do tempo: flick, pausa, flick… Com salto, ele acontece no segundo frame: o plano sempre
	// começa na posição atual.
	t := 0.0
	if jump > 0 {
		t = p.Frame
	}
	for i := range flicks {
		flicks[i].start = t
		plan.Starts = append(plan.Starts, t)
		plan.Peaks = append(plan.Peaks, t+flicks[i].rise)
		t += flicks[i].rise + flicks[i].decay
		plan.Ends = append(plan.Ends, t)
		if i < len(pauses) {
			t += pauses[i]
		}
	}
	plan.Duration = math.Round(t)
	plan.Jump = sign * jump
	plan.Flicks = len(flicks)
	at := func(t float64) float64 {
		pos := jump
		for _, f := range flicks {
			pos += f.dist * f.progress(t-f.start)
		}
		return sign * pos
	}
	for ft := 0.0; ft < plan.Duration; ft += p.Frame {
		pos := math.Round(at(ft)*10) / 10
		if ft == 0 {
			pos = 0
		}
		// O arredondamento nunca passa do destino (o último frame é o destino exato).
		pos = sign * math.Min(math.Abs(pos), abs)
		plan.Frames = append(plan.Frames, ft, pos)
	}
	plan.Frames = append(plan.Frames, plan.Duration, distance)
	return plan
}

// TargetOffset é quanto rolar para o topo do alvo terminar na faixa sorteada da viewport, abaixo
// dos headers fixos (covered). top é o topo do alvo relativo ao topo da viewport agora.
func TargetOffset(top, viewport, covered, maxDown, maxUp float64, p ScrollProfile, rng *rand.Rand) float64 {
	top, viewport, covered = finite(top), math.Max(finite(viewport), 1), math.Max(finite(covered), 0)
	usable := math.Max(viewport-covered, 1)
	want := covered + usable*uniform(rng, p.TargetMin, p.TargetMax)
	return clamp(top-want, -math.Max(maxUp, 0), math.Max(maxDown, 0))
}

// InBand diz se o topo do alvo está na faixa aceitável depois de rolar (25%–45% da área útil).
func InBand(top, viewport, covered float64) bool {
	usable := math.Max(viewport-covered, 1)
	rel := (top - covered) / usable
	return rel >= 0.25 && rel <= 0.45
}
