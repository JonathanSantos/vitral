package session

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/brand"
	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/cursor"
)

// pointer coordena o cursor visual da IA com as ações (docs/CURSOR.md, seção 5). O cursor é só
// visual: os eventos reais são um mouseMoved direto ao ponto final, press e release.
type pointer struct {
	on     bool
	pacing bool
	prof   cursor.Profile // velocidade configurada
	flow   cursor.Profile // velocidade durante flow_run
	rng    *rand.Rand
	side   int         // lado do arco do último movimento (a "mão")
	view   cursor.Rect // viewport vista no último movimento
}

func newPointer(cfg config.Config) *pointer {
	seed := uint64(cfg.CursorSeed) //nolint:gosec // qualquer padrão de bits serve de semente
	if seed == 0 {
		var b [8]byte
		_, _ = crand.Read(b[:])
		seed = binary.LittleEndian.Uint64(b[:])
	}
	prof, flowProf := cursor.DefaultProfile(), cursor.DefaultProfile()
	prof.Speed, _ = cursor.ParseSpeed(cfg.CursorSpeed)
	flowProf.Speed, _ = cursor.ParseSpeed(cfg.FlowCursorSpeed)
	return &pointer{
		on: cfg.CursorActive(), pacing: cfg.TypePacingActive(),
		// Sorteio visual reproduzível pela semente (--cursor-seed), não segurança.
		prof: prof, flow: flowProf, rng: rand.New(rand.NewPCG(seed, seed^0x5eed)), //nolint:gosec // G404: ver acima
	}
}

func (s *Session) profile() cursor.Profile {
	if s.replaying.Load() {
		return s.cur.flow
	}
	return s.cur.prof
}

type aimInfo struct {
	Error     string     `json:"error"`
	Box       [4]int     `json:"box"`
	Index     int        `json:"index"`
	Obstacles [][4]int   `json:"obstacles"`
	Hidden    bool       `json:"hidden"`
	Reduced   bool       `json:"reduced"`
	Viewport  [2]float64 `json:"viewport"`
}

func rectOf(b [4]int) cursor.Rect {
	return cursor.Rect{X: float64(b[0]), Y: float64(b[1]), W: float64(b[2]), H: float64(b[3])}
}

func kindOf(name string) cursor.Kind {
	switch name {
	case "pointer":
		return cursor.Pointer
	case "text":
		return cursor.Text
	}
	return cursor.Arrow
}

// target é o alvo de um movimento já validado na página.
type target struct {
	ref    string
	box    [4]int
	kind   cursor.Kind
	to     cursor.Point
	motion cursor.Motion
	view   cursor.Rect
}

// aim escolhe o ponto do alvo (12.2): candidatos validados numa ida só; se nenhum vale, o centro da
// maior área livre de filhos interativos. ok=false se nada vale ("covered by").
func (s *Session) aim(ctx context.Context, ref string, box [4]int, kind cursor.Kind) (target, bool, error) {
	prof := s.profile()
	rect := rectOf(box)
	cands := cursor.Candidates(rect, kind, prof, s.cur.rng)
	pts := make([][2]float64, len(cands))
	for i, c := range cands {
		pts[i] = [2]float64{c.X, c.Y}
	}
	var info aimInfo
	if err := s.evalInto(ctx, &info, `(r, c) => window.__vitral.aim(r, c)`, ref, pts); err != nil {
		return target{}, false, err
	}
	if err := refError(ref, info.Error); err != nil {
		return target{}, false, err
	}
	t := target{ref: ref, box: box, kind: kind}
	t.view, t.motion = s.envOf(info)
	if info.Index >= 0 && info.Index < len(cands) {
		t.to = cands[info.Index]
		return t, true, nil
	}
	obstacles := make([]cursor.Rect, len(info.Obstacles))
	for i, o := range info.Obstacles {
		obstacles[i] = rectOf(o)
	}
	free, ok := cursor.FreeArea(rect, obstacles)
	if !ok {
		return t, false, nil
	}
	c := free.Center()
	c = cursor.Point{X: math.Round(c.X*10) / 10, Y: math.Round(c.Y*10) / 10}
	var hits bool
	if err := s.evalInto(ctx, &hits, `(r, x, y) => window.__vitral.aimAt(r, x, y)`, ref, c.X, c.Y); err != nil {
		return t, false, err
	}
	t.to = c
	return t, hits, nil
}

// envOf lê o ambiente devolvido pela página (aim ou env) e lembra a viewport.
func (s *Session) envOf(info aimInfo) (cursor.Rect, cursor.Motion) {
	view := cursor.Rect{W: info.Viewport[0], H: info.Viewport[1]}
	s.cur.view = view
	switch {
	case info.Hidden:
		return view, cursor.Hidden
	case info.Reduced:
		return view, cursor.Reduced
	}
	return view, cursor.Animated
}

// env consulta aba oculta, movimento reduzido e viewport (para movimentos sem alvo validado).
func (s *Session) env(ctx context.Context) (cursor.Rect, cursor.Motion, error) {
	var info aimInfo
	if err := s.evalInto(ctx, &info, `() => window.__vitral.env()`); err != nil {
		return cursor.Rect{}, cursor.Animated, err
	}
	view, motion := s.envOf(info)
	return view, motion, nil
}

// travelTo leva o cursor a um ponto qualquer (sem alvo validado), sem clicar.
func (s *Session) travelTo(ctx context.Context, to cursor.Point, box [4]int, kind cursor.Kind) {
	view, motion, err := s.env(ctx)
	if err != nil {
		return
	}
	s.travel(ctx, target{box: box, kind: kind, to: to, view: view, motion: motion})
}

// from é onde o movimento começa: a última posição ou, na primeira ação, o selo (com fade).
func (s *Session) from(view cursor.Rect) (cursor.Point, bool) {
	if at, shown := s.ui.CursorPosition(); shown {
		return at, false
	}
	return cursor.BadgePoint(view.W, view.H), true
}

// travel leva o cursor até t.to sem clicar e espera chegar (ou o timeout).
func (s *Session) travel(ctx context.Context, t target) cursor.Plan {
	prof := s.profile()
	start, appear := s.from(t.view)
	plan := cursor.Compute(cursor.Input{
		From: start, To: t.to, Target: rectOf(t.box), Kind: t.kind, Side: s.cur.side, Motion: t.motion,
	}, prof, s.cur.rng)
	s.cur.side = plan.Side
	s.ui.PlayCursor(ctx, plan, "", appear)
	return plan
}

// pressPlan é a mira e o press no ponto atual (depois do movimento ou da correção).
func (s *Session) pressPlan(ctx context.Context, t target, moved float64) cursor.Plan {
	prof := s.profile()
	start, _ := s.ui.CursorPosition()
	plan := cursor.Compute(cursor.Input{
		From: start, To: t.to, Target: rectOf(t.box), Kind: t.kind, Side: s.cur.side, Motion: t.motion,
		Correct: true,
	}, prof, s.cur.rng)
	s.cur.side = plan.Side
	// --highlight-ms é o tempo mínimo de destaque visível antes do clique (5.1).
	if minimum := float64(s.ui.HighlightDuration().Milliseconds()) - moved; minimum > 0 && t.motion == cursor.Animated {
		plan = plan.WithMinPress(minimum)
	}
	return plan
}

// moved diz se o alvo andou mais de 4 px desde o planejamento (4.5).
func movedBox(a, b [4]int) bool {
	d := math.Hypot(float64(a[0]+a[2]/2-b[0]-b[2]/2), float64(a[1]+a[3]/2-b[1]-b[3]/2))
	return d > 4 || math.Abs(float64(a[2]-b[2])) > 4 || math.Abs(float64(a[3]-b[3])) > 4
}

// ringColor é a cor da marcação da ref, a mesma das marcações do screenshot (3).
func ringColor(ref string) string {
	fill, _ := brand.MarkColor(brand.RefNumber(ref))
	return brand.Hex(fill)
}

// approach leva o cursor até o alvo e faz a mira e o press visuais. Devolve o plano do press (o
// ponto final é onde o evento real deve acontecer) ou ok=false se nenhum ponto vale.
func (s *Session) approach(ctx context.Context, ref string, box [4]int, kind cursor.Kind) (cursor.Plan, bool, error) {
	t, ok, err := s.aim(ctx, ref, box, kind)
	if err != nil || !ok {
		return cursor.Plan{}, ok, err
	}
	// O destaque começa junto com o movimento e fica até o clique (5.1).
	prof := s.profile()
	k := prof.Speed.Factor()
	s.ui.ShowHighlight(ctx, ref, box, s.ui.HighlightDuration()+time.Duration((prof.MaxMove+prof.AimMax+prof.PressMax)*k)*time.Millisecond)
	move := s.travel(ctx, t)

	// Alvo que se mexeu no caminho: recalcula o ponto com a mesma regra e corrige.
	var now [4]int
	if err := s.evalInto(ctx, &now, `(r) => window.__vitral.box(r) || [0, 0, 0, 0]`, ref); err == nil && now[2] > 0 && movedBox(box, now) {
		nt, ok, err := s.aim(ctx, ref, now, kind)
		if err != nil || !ok {
			return cursor.Plan{}, ok, err
		}
		t = nt
	}
	press := s.pressPlan(ctx, t, move.Move)
	s.ui.PlayCursor(ctx, press, ringColor(ref), false)
	return press, true, nil
}

// pressAt é o clique real: um mouseMoved direto ao ponto final, press e release no tempo do plano.
func pressAt(ctx context.Context, p *rod.Page, plan cursor.Plan) error {
	mouse := p.Context(ctx).Mouse
	if err := mouse.MoveTo(proto.Point{X: plan.End.X, Y: plan.End.Y}); err != nil {
		return fmt.Errorf("move mouse: %w", err)
	}
	if err := mouse.Down(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	hold := time.Duration((plan.Release - plan.Press) * float64(time.Millisecond))
	select {
	case <-ctx.Done():
	case <-time.After(hold):
	}
	return mouse.Up(proto.InputMouseButtonLeft, 1)
}

// failAt leva o cursor até o alvo e faz o "não" com a cabeça (6.4). O erro para a IA não muda.
func (s *Session) failAt(ctx context.Context, box [4]int, kind cursor.Kind, text string) {
	s.travelTo(ctx, rectOf(box).Center(), box, kind)
	_ = s.ui.CursorFail(ctx, text)
}

// besideAndHide para o cursor ao lado do campo e some: no "Sua vez" quem age é a pessoa (6.2).
func (s *Session) besideAndHide(ctx context.Context, box [4]int) {
	r := rectOf(box)
	to := cursor.Point{X: r.X + r.W + 16, Y: r.Y + r.H/2}
	if s.cur.view.W > 0 {
		to.X = math.Min(to.X, s.cur.view.W-24)
	}
	s.travelTo(ctx, to, box, cursor.Arrow)
	_ = s.ui.HideCursor(ctx)
}

// awayFrom tira a mão de cima do campo depois do clique, para a pessoa ver o texto (5.2).
func (s *Session) awayFrom(ctx context.Context, from cursor.Point) {
	plan := cursor.Away(from, s.cur.view, s.profile(), s.cur.rng)
	s.ui.PlayCursor(ctx, plan, "", false)
}

// smoothReveal rola suavemente até o alvo, com o cursor parado, até ScrollCap; depois disso a
// rolagem instantânea do hit termina o caminho (5.1).
func (s *Session) smoothReveal(ctx context.Context, ref string) {
	var res struct {
		Scrolling bool `json:"scrolling"`
	}
	if err := s.evalInto(ctx, &res, `(r) => window.__vitral.revealSmooth(r)`, ref); err != nil || !res.Scrolling {
		return
	}
	deadline := time.Now().Add(time.Duration(s.profile().ScrollCap) * time.Millisecond)
	var last [4]int
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
		var now [4]int
		if err := s.evalInto(ctx, &now, `(r) => window.__vitral.box(r) || [0, 0, 0, 0]`, ref); err != nil {
			return
		}
		if now == last {
			return
		}
		last = now
	}
}

// typeText digita com ritmo visível (5.2): teclas reais de 25 a 45 ms até 1,5 s; o resto de uma vez.
func (s *Session) typeText(ctx context.Context, p *rod.Page, text string) error {
	pc := p.Context(ctx)
	if !s.cur.pacing {
		return pc.InsertText(text)
	}
	runes := []rune(text)
	delays := cursor.Typing(len(runes), s.profile(), s.cur.rng)
	for i, d := range delays {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(d) * time.Millisecond):
		}
		if err := typeRune(pc, runes[i]); err != nil {
			return err
		}
	}
	if rest := runes[len(delays):]; len(rest) > 0 {
		return pc.InsertText(string(rest))
	}
	return nil
}

func typeRune(p *rod.Page, r rune) error {
	if r == '\n' {
		return p.Keyboard.Type(input.Enter)
	}
	ch := string(r)
	if err := (proto.InputDispatchKeyEvent{Type: proto.InputDispatchKeyEventTypeKeyDown, Key: ch, Text: ch, UnmodifiedText: ch}).Call(p); err != nil {
		return err
	}
	return proto.InputDispatchKeyEvent{Type: proto.InputDispatchKeyEventTypeKeyUp, Key: ch}.Call(p)
}

// wheel rola em passos de roda visíveis no ponto do cursor (5.4).
func (s *Session) wheel(ctx context.Context, p *rod.Page, at cursor.Point, amount float64) error {
	mouse := p.Context(ctx).Mouse
	if err := mouse.MoveTo(proto.Point{X: at.X, Y: at.Y}); err != nil {
		return err
	}
	w := cursor.WheelSteps(amount, s.profile(), s.cur.rng)
	for i, step := range w.Steps {
		if d := w.Delays[i]; d > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(d) * time.Millisecond):
			}
		}
		if err := mouse.Scroll(0, step, 1); err != nil {
			return err
		}
	}
	if w.Rest != 0 {
		return mouse.Scroll(0, w.Rest, 1)
	}
	return nil
}

// keyLabel é o nome curto da tecla no keycap (5.5).
func keyLabel(combo string) string {
	switch combo {
	case "Escape":
		return "Esc"
	case " ", "Space":
		return "Espaço"
	}
	return combo
}
