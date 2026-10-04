package cursor

import (
	"fmt"
	"math"
)

// round é a tolerância do arredondamento para 0,1 px que vai para o overlay.
const round = 0.1

// checkPlan confere os invariantes da seção 9 que valem para qualquer entrada. Devolve o primeiro
// problema encontrado (string vazia se o plano está certo).
func checkPlan(in Input, p Profile, plan Plan) string {
	f := plan.Frames
	if len(f) < 3 || len(f)%3 != 0 {
		return fmt.Sprintf("bad frame count %d", len(f))
	}
	for _, v := range f {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "NaN or Inf in frames"
		}
	}
	for _, v := range []float64{plan.Move, plan.Press, plan.Release, plan.ShapeAt, plan.Fade, plan.ID, plan.Overshoot} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "NaN or Inf in timings"
		}
	}
	n := len(f) / 3
	if f[0] != 0 {
		return "first keyframe is not at t=0"
	}
	for i := 1; i < n; i++ {
		dt := f[3*i] - f[3*(i-1)]
		if i < n-1 && dt != p.Frame {
			return fmt.Sprintf("keyframe %d spacing %v, want %v", i, dt, p.Frame)
		}
		if i == n-1 && (dt <= 0 || dt > p.Frame) {
			return fmt.Sprintf("last keyframe spacing %v", dt)
		}
	}
	last := Point{f[3*n-2], f[3*n-1]}
	if last != plan.End || plan.End != roundPoint(sanePoint(in.To)) {
		return fmt.Sprintf("last keyframe %v, end %v, want %v", last, plan.End, roundPoint(sanePoint(in.To)))
	}
	// Sem trajeto (fade), o único keyframe é o destino em t=0 e o movimento dura o fade.
	if lastT := f[3*n-3]; (plan.Fade == 0 && lastT != plan.Move) || (plan.Fade > 0 && (n != 1 || plan.Move != plan.Fade)) {
		return fmt.Sprintf("last keyframe at %v, move ends at %v (fade %v)", lastT, plan.Move, plan.Fade)
	}
	if plan.ShapeAt > plan.Move || plan.Press < plan.Move || plan.Release < plan.Press {
		return fmt.Sprintf("timeline out of order: %+v", plan)
	}

	from := roundPoint(sanePoint(in.From))
	d := dist(from, plan.End)
	animated := p.Speed != Instant && in.Motion == Animated && d >= p.Still
	if !animated {
		return ""
	}
	k := p.Speed.Factor()
	if first := (Point{f[1], f[2]}); first != from {
		return fmt.Sprintf("first keyframe %v, want from %v", first, from)
	}
	if aim := plan.Press - plan.Move; aim < p.AimMin*k-1 || aim > p.AimMax*k+1 {
		return fmt.Sprintf("aim %v out of range", aim)
	}
	if press := plan.Release - plan.Press; press < p.PressMin*k-1 || press > p.PressMax*k+1 {
		return fmt.Sprintf("press %v out of range", press)
	}
	if plan.Peak < p.PeakMin || plan.Peak > p.PeakMax {
		return fmt.Sprintf("peak %v outside [%v, %v]", plan.Peak, p.PeakMin, p.PeakMax)
	}

	// Sem saltos: nenhum passo entre keyframes passa de ~4× a velocidade média do movimento principal
	// (o pico do jerk mínimo deformado fica perto de 2×).
	maxStep := 4.5*(1.2*d+2*plan.Overshoot)*p.Frame/math.Max(plan.MainEnd, 1) + 1
	for i := 1; i < n; i++ {
		if step := dist(Point{f[3*i-2], f[3*i-1]}, Point{f[3*i+1], f[3*i+2]}); step > maxStep {
			return fmt.Sprintf("jump of %.1f px between keyframes %d and %d (max %.1f)", step, i-1, i, maxStep)
		}
	}
	// Curvatura: desvio da reta from→destino.
	u, nrm := unit(from, plan.End), perp(unit(from, plan.End))
	limit := math.Min(p.ArcLimit*d, p.ArcLimitPx)
	// Sem laços: a projeção na direção principal só recua o que o overshoot permite.
	var best float64
	for i := 0; i < n; i++ {
		pt := Point{f[3*i+1], f[3*i+2]}.sub(from)
		if dev := math.Abs(pt.dot(nrm)); dev > limit+round {
			return fmt.Sprintf("deviation %.2f > limit %.2f at keyframe %d", dev, limit, i)
		}
		proj := pt.dot(u)
		best = math.Max(best, proj)
		if back := best - proj; back > plan.Overshoot+round {
			return fmt.Sprintf("goes back %.2f px (overshoot %.2f) at keyframe %d", back, plan.Overshoot, i)
		}
	}
	if in.Correct {
		return ""
	}

	// Tempo: Fitts com a variação, dentro do limite da velocidade.
	if plan.Move < p.MinMove*k-1 || plan.Move > p.MaxMove*k+1 {
		return fmt.Sprintf("move %v outside [%v, %v]", plan.Move, p.MinMove*k, p.MaxMove*k)
	}
	fitts := (p.FittsA + p.FittsB*plan.ID) * k
	lo, hi := math.Max(fitts*(1-p.Jitter), p.MinMove*k), math.Min(fitts*(1+p.Jitter), p.MaxMove*k)
	if plan.Move < math.Min(lo, hi)-1 || plan.Move > math.Max(lo, hi)+1 {
		return fmt.Sprintf("move %v does not follow Fitts (%v ±%v%%)", plan.Move, fitts, 100*p.Jitter)
	}
	// Submovimentos só com ID alto; overshoot limitado.
	if plan.Submove != (plan.ID >= p.SubmoveID) {
		return fmt.Sprintf("submove=%v with ID %.2f", plan.Submove, plan.ID)
	}
	if plan.Overshoot > math.Min(p.OverMax*d, p.OverPx)+1e-9 {
		return fmt.Sprintf("overshoot %.2f too large for D=%.1f", plan.Overshoot, d)
	}
	if p.Speed == Normal && plan.Release > 1300 {
		return fmt.Sprintf("click takes %v ms, budget is 1300", plan.Release)
	}
	return ""
}
