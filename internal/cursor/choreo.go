package cursor

import (
	"math"
	"math/rand/v2"
)

// BadgePoint é de onde o cursor surge na primeira ação: o selo, no canto inferior direito (3).
func BadgePoint(vw, vh float64) Point { return Point{math.Max(vw-36, 0), math.Max(vh-38, 0)} }

// WithMinPress estende a mira para o press não sair antes de ms (o --highlight-ms vira o tempo
// mínimo de destaque visível antes do clique, 5.1).
func (p Plan) WithMinPress(ms float64) Plan {
	if d := ms - p.Press; d > 0 {
		p.Press += d
		p.Release += d
	}
	return p
}

// Away é o movimento curto depois de clicar num campo: a mão sai de cima do texto para baixo e à
// direita, sem clique (5.2). view limita o destino à viewport.
func Away(from Point, view Rect, p Profile, rng *rand.Rand) Plan {
	from = roundPoint(sanePoint(from))
	k := p.Speed.Factor()
	dist := uniform(rng, p.AwayDistMin, p.AwayDistMax)
	angle := uniform(rng, math.Pi/6, math.Pi/3) // entre 30° e 60° abaixo da horizontal
	to := Point{from.X + dist*math.Cos(angle), from.Y + dist*math.Sin(angle)}
	view = saneRect(view)
	to = roundInside(to, view)
	dur := math.Round(uniform(rng, p.AwayMin, p.AwayMax) * k)
	plan := Plan{Shape: Arrow, End: to, Side: 1}
	if p.Speed == Instant || dur <= 0 || dist == 0 {
		plan.Frames = []float64{0, to.X, to.Y}
		return timeline(plan, 0, 0, 0, 0)
	}
	seg := newSegment(from, to, dur, 0.04*dist, 1, 0, p, rng)
	plan.Peak, plan.MainEnd = seg.peak, dur
	plan.Frames = frames(p, dur, func(t float64) Point { return seg.at(t / dur) })
	return timeline(plan, 0, dur, 0, 0)
}

// Typing é o ritmo de digitação visível (5.2): o intervalo antes de cada tecla, até o teto. Os
// caracteres que não couberem (a partir de len(delays)) entram de uma vez.
func Typing(chars int, p Profile, rng *rand.Rand) []float64 {
	var delays []float64
	var total float64
	for i := 0; i < chars; i++ {
		d := math.Round(uniform(rng, p.KeyMin, p.KeyMax))
		if total+d > p.TypeCap {
			break
		}
		total += d
		delays = append(delays, d)
	}
	return delays
}

// Wheel divide uma rolagem em passos de roda de ~WheelStep px, com intervalo entre eles, até o
// teto de tempo; o que sobrar vai num último passo sem espera (5.4).
type Wheel struct {
	Steps  []float64 // deslocamento de cada passo (sinal = direção)
	Delays []float64 // espera antes de cada passo (o primeiro é 0)
	Rest   float64   // o que sobrou depois do teto, aplicado de uma vez no fim
}

// WheelSteps planeja a rolagem de amount px (negativo = para cima).
func WheelSteps(amount float64, p Profile, rng *rand.Rand) Wheel {
	var w Wheel
	amount = finite(amount)
	left, sign := math.Abs(amount), math.Copysign(1, amount)
	var elapsed float64
	for left > 0 {
		delay := 0.0
		if len(w.Steps) > 0 {
			delay = math.Round(uniform(rng, p.WheelMin, p.WheelMax))
			if elapsed+delay > p.WheelCap {
				break
			}
		}
		step := math.Min(p.WheelStep, left)
		elapsed += delay
		left -= step
		w.Steps = append(w.Steps, sign*step)
		w.Delays = append(w.Delays, delay)
	}
	if left > 0 {
		w.Rest = sign * left
	}
	return w
}
