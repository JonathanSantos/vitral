package cursor

import (
	"math"
	"math/rand/v2"
)

// Motion diz como a página pode animar (consultado antes de planejar).
type Motion int

const (
	Animated Motion = iota
	Reduced         // prefers-reduced-motion: sem trajeto, fade no destino
	Hidden          // aba oculta ou janela minimizada: aplica na hora
)

// Input é o pedido de um movimento.
type Input struct {
	From   Point
	To     Point // ponto aceito pela página (um dos Candidates ou o centro de FreeArea)
	Target Rect
	Kind   Kind
	Side   int // lado do arco no movimento anterior: -1, 1 ou 0 (nenhum)
	Motion Motion
	// Correct planeja só um submovimento corretivo (alvo que se mexeu no caminho, 4.5).
	Correct bool
}

// Plan é o que o overlay toca. Tempos em ms desde o início; coordenadas em CSS px da viewport.
type Plan struct {
	Frames  []float64 // t, x, y, t, x, y, ... a cada Frame ms; o último é exatamente End
	Shape   Kind      // forma ao chegar
	ShapeAt float64   // quando a forma muda
	Move    float64   // fim do movimento
	Press   float64   // instante do press (o evento real sai aqui)
	Release float64
	Fade    float64 // > 0: sem trajeto, o cursor aparece no destino com fade deste tamanho
	End     Point   // ponto final: o clique real usa exatamente este ponto

	// Para os testes e o cursorlab.
	Side      int     // lado do arco usado (passe como Input.Side no próximo movimento)
	ID        float64 // índice de dificuldade de Fitts
	Submove   bool    // o movimento principal errou e houve correção
	MainEnd   float64 // fim do movimento principal (ms)
	Gap       float64 // pausa entre o principal e o corretivo (ms)
	Overshoot float64 // quanto passou do ponto, em px (0 se parou antes ou acertou)
	Peak      float64 // fração da duração principal onde fica o pico de velocidade
}

// Compute planeja o movimento. Mesma entrada e mesma sequência do rng geram o mesmo plano.
func Compute(in Input, p Profile, rng *rand.Rand) Plan {
	from, to := roundPoint(sanePoint(in.From)), roundPoint(sanePoint(in.To))
	target := saneRect(in.Target)
	k := p.Speed.Factor()
	plan := Plan{Shape: in.Kind, End: to, Side: in.Side}

	if p.Speed == Instant || in.Motion != Animated {
		switch {
		case in.Motion == Hidden:
			plan.Fade = 0
		case in.Motion == Reduced && p.Speed != Instant:
			plan.Fade = p.FadeReduced
		default:
			plan.Fade = p.FadeInstant
		}
		aim, press := p.AimMin, p.PressMin
		if in.Motion == Reduced && p.Speed != Instant {
			aim, press = uniform(rng, p.AimMin, p.AimMax)*k, uniform(rng, p.PressMin, p.PressMax)*k
		}
		plan.Frames = []float64{0, to.X, to.Y}
		return timeline(plan, p.ShapeLead, plan.Fade, aim, press)
	}

	d := dist(from, to)
	aim, press := uniform(rng, p.AimMin, p.AimMax)*k, uniform(rng, p.PressMin, p.PressMax)*k
	if d < p.Still {
		plan.Frames = []float64{0, to.X, to.Y}
		return timeline(plan, p.ShapeLead, 0, aim, press)
	}
	w := math.Max(math.Min(target.W, target.H), p.MinWidth)
	plan.ID = math.Log2(d/w + 1)

	if in.Correct {
		dur := math.Round(uniform(rng, p.CorrMin, p.CorrMax) * k)
		seg := newSegment(from, to, dur, math.Min(uniform(rng, 0, p.CorrCurve)*d, arcLimit(p, d)), side(in.Side, p, rng), 0, p, rng)
		plan.Side = seg.side
		plan.Frames = frames(p, dur, func(t float64) Point { return seg.at(t / dur) })
		plan.MainEnd, plan.Peak = dur, seg.peak
		return timeline(plan, p.ShapeLead, dur, aim, press)
	}

	total := (p.FittsA + p.FittsB*plan.ID) * (1 + truncNormal(rng, p.Jitter/2, p.Jitter)) * k
	total = math.Round(clamp(total, p.MinMove*k, p.MaxMove*k))
	u, n := unit(from, to), perp(unit(from, to))
	limit := arcLimit(p, d)

	mainDur, gap, corr := total, 0.0, 0.0
	mainEnd, lateral := to, 0.0
	// Os sorteios do submovimento acontecem sempre, para o rng andar igual nos dois casos.
	under := rng.Float64() < p.Undershoot
	frac, lat := uniform(rng, p.UnderMin, p.UnderMax), uniform(rng, -1, 1)*p.UnderLateral*w
	over := math.Min(uniform(rng, p.OverMin, p.OverMax)*d, p.OverPx)
	corrDur, gapDur := uniform(rng, p.CorrMin, p.CorrMax)*k, uniform(rng, p.GapMin, p.GapMax)*k
	corrCurve := uniform(rng, 0, p.CorrCurve)
	if plan.ID >= p.SubmoveID {
		plan.Submove = true
		corr, gap = math.Round(corrDur), math.Round(gapDur)
		if corr+gap > total/2 { // nunca deixa o principal com menos da metade do tempo
			s := (total / 2) / (corr + gap)
			corr, gap = math.Round(corr*s), math.Round(gap*s)
		}
		mainDur = total - corr - gap
		if under {
			lateral = math.Copysign(math.Min(math.Abs(lat), limit/2), lat)
			mainEnd = from.add(u.scale(d * frac)).add(n.scale(lateral))
		} else {
			plan.Overshoot = over
			mainEnd = to.add(u.scale(over))
		}
	}

	// O arco usa o que sobra do limite depois do erro lateral e do tremor (4.3).
	amp := tremorAmp(p, d)
	arc := math.Max(math.Min(uniform(rng, p.ArcMin, p.ArcMax)*dist(from, mainEnd), limit-math.Abs(lateral)-amp), 0)
	main := newSegment(from, mainEnd, mainDur, arc, side(in.Side, p, rng), amp, p, rng)
	plan.Side, plan.Peak = main.side, main.peak
	// O corretivo vai quase reto até o destino, com o mesmo perfil de velocidade. Ele também respeita
	// o desvio máximo da reta principal (começa deslocado pelo erro lateral).
	corrOffset := math.Min(corrCurve*dist(mainEnd, to), math.Max(limit-math.Abs(lateral), 0))
	fix := newSegment(mainEnd, to, corr, corrOffset, main.side, 0, p, rng)

	plan.MainEnd, plan.Gap = mainDur, gap
	plan.Frames = frames(p, total, func(t float64) Point {
		switch {
		case t <= mainDur:
			return main.at(t / mainDur)
		case t <= mainDur+gap || corr == 0:
			return mainEnd
		}
		return fix.at((t - mainDur - gap) / corr)
	})
	return timeline(plan, p.ShapeLead, total, aim, press)
}

// tremorAmp é a amplitude do tremor: 0,4 px, mas nunca mais que 2% da distância (num movimento de
// poucos pixels o tremor seria maior que o próprio arco).
func tremorAmp(p Profile, d float64) float64 { return math.Min(p.TremorAmp, 0.02*d) }

// arcLimit é o desvio máximo da reta (4.3).
func arcLimit(p Profile, d float64) float64 { return math.Min(p.ArcLimit*d, p.ArcLimitPx) }

// side decide o lado do arco: a mão tende a repetir o mesmo "jeito".
func side(last int, p Profile, rng *rand.Rand) int {
	r := rng.Float64()
	if last == 0 {
		if r < 0.5 {
			return -1
		}
		return 1
	}
	if r < p.SameSide {
		return last
	}
	return -last
}

// timeline fecha os tempos depois do movimento: mira, press e release (4.6).
func timeline(plan Plan, lead, move, aim, press float64) Plan {
	plan.Move = move
	plan.ShapeAt = math.Max(0, move-lead)
	plan.Press = math.Round(move + aim)
	plan.Release = math.Round(plan.Press + press)
	return plan
}

// frames amostra a posição a cada Frame ms; o último keyframe é exatamente em dur.
func frames(p Profile, dur float64, at func(t float64) Point) []float64 {
	out := make([]float64, 0, 3*(int(dur/p.Frame)+2))
	for t := 0.0; t < dur; t += p.Frame {
		pt := roundPoint(at(t))
		out = append(out, t, pt.X, pt.Y)
	}
	end := roundPoint(at(dur))
	return append(out, dur, end.X, end.Y)
}
