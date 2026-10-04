package session

import (
	"context"
	"math"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"

	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/overlay"
)

// Rolagem humana (pedido "próximos passos", seção 1): o Go planeja (internal/cursor/scroll.go), o
// overlay toca com scrollTo(instant) por frame e o Go mede de novo depois de cada scroll_done.

type scrollLink struct {
	Key       string  `json:"key"`
	Top       float64 `json:"top"`
	Height    float64 `json:"height"`
	ScrollTop float64 `json:"scrollTop"`
	Max       float64 `json:"max"`
	Covered   float64 `json:"covered"`
	Rect      [4]int  `json:"rect"`
}

type scrollChain struct {
	Error string       `json:"error"`
	Chain []scrollLink `json:"chain"`
	Clip  *struct {
		Key    string  `json:"key"`
		Rect   [4]int  `json:"rect"`
		Top    float64 `json:"top"`
		Height float64 `json:"height"`
	} `json:"clip"`
	Hidden   bool       `json:"hidden"`
	Reduced  bool       `json:"reduced"`
	Viewport [2]float64 `json:"viewport"`
}

func (c scrollChain) motion() cursor.Motion {
	switch {
	case c.Hidden:
		return cursor.Hidden
	case c.Reduced:
		return cursor.Reduced
	}
	return cursor.Animated
}

func center(r [4]int) cursor.Point {
	return cursor.Point{X: float64(r[0]) + float64(r[2])/2, Y: float64(r[1]) + float64(r[3])/2}
}

// scrollIntoBand rola, com gesto humano, cada contêiner da cadeia até o alvo ficar entre 30% e 40%
// da área visível (descontando headers fixos). No máximo uma correção por contêiner depois de medir
// de novo (lazy load, imagens carregando). Se a pessoa rolar por conta própria, não briga: segue.
func (s *Session) scrollIntoBand(ctx context.Context, p *rod.Page, ref string) error {
	prof, speed := s.cur.scroll, s.profile().Speed
	attempts := map[string]int{}
	var info scrollChain
	for iter := 0; iter < 8; iter++ {
		info = scrollChain{}
		if err := s.callRef(ctx, &info, ref, `(el) => window.__vitral.scrollChain(el)`); err != nil {
			return err
		}
		if err := refError(ref, info.Error); err != nil {
			return err
		}
		moved := false
		for _, c := range info.Chain {
			if cursor.InBand(c.Top, c.Height, c.Covered) || attempts[c.Key] >= 2 {
				continue
			}
			off := cursor.TargetOffset(c.Top, c.Height, c.Covered, c.Max-c.ScrollTop, c.ScrollTop, prof, s.cur.rng)
			if math.Abs(off) < prof.Still {
				continue
			}
			attempts[c.Key]++
			plan := cursor.Scroll(off, c.Height, info.Viewport[1], speed, info.motion(), prof, s.cur.rng)
			switch s.ui.PlayScroll(ctx, c.Key, plan) {
			case "interrupted":
				return nil // a pessoa rolou: a posição dela vale
			case "unresponsive", overlay.ScrollNoOverlay:
				if err := s.wheelPlan(ctx, p, center(c.Rect), plan); err != nil {
					return err
				}
			}
			moved = true
			break
		}
		if !moved {
			break
		}
	}
	// Alvo cortado num contêiner que só responde à roda: rola com a roda até a faixa.
	if info.Clip != nil {
		off := info.Clip.Top - info.Clip.Height*cursor.DefaultScrollProfile().TargetMin
		plan := cursor.Scroll(off, info.Clip.Height, info.Viewport[1], speed, info.motion(), prof, s.cur.rng)
		return s.wheelPlan(ctx, p, center(info.Clip.Rect), plan)
	}
	return nil
}

// scrollBy é a rolagem por direção: um gesto de amount px na página, com o cursor parado no centro.
func (s *Session) scrollBy(ctx context.Context, p *rod.Page, at cursor.Point, amount float64) error {
	var page struct {
		Top, Max, Height float64
		Hidden, Reduced  bool
	}
	js := `() => { const e = document.scrollingElement; const v = window.__vitral.env();
		return {top: e.scrollTop, max: e.scrollHeight - e.clientHeight, height: innerHeight, hidden: v.hidden, reduced: v.reduced}; }`
	if err := s.evalInto(ctx, &page, js); err != nil {
		return err
	}
	// O fim do conteúdo limita o gesto: ele desacelera até a borda em vez de bater nela.
	amount = math.Max(-page.Top, math.Min(amount, page.Max-page.Top))
	motion := cursor.Animated
	if page.Hidden {
		motion = cursor.Hidden
	} else if page.Reduced {
		motion = cursor.Reduced
	}
	if amount == 0 {
		return nil // já no fim (ou no começo): não há para onde ir
	}
	plan := cursor.Scroll(amount, page.Height, page.Height, s.profile().Speed, motion, s.cur.scroll, s.cur.rng)
	switch s.ui.PlayScroll(ctx, "page", plan) {
	case "unresponsive", overlay.ScrollNoOverlay:
		return s.wheelPlan(ctx, p, at, plan)
	}
	return nil
}

// wheelPlan rola com a roda, para contêineres que não aceitam rolagem programática. Medido (pedido
// "próximos passos", 1.3): mouseWheel por keyframe engasga (47% dos frames parados, porque cada
// evento espera o CDP), e Input.synthesizeScrollGesture é fluido (0%), com velocidade constante.
// Usa o gesto com a velocidade média do plano (e o salto, se houver, num evento só); se o gesto
// experimental falhar, cai para a roda por keyframe.
func (s *Session) wheelPlan(ctx context.Context, p *rod.Page, at cursor.Point, plan cursor.ScrollPlan) error {
	pc := p.Context(ctx)
	if err := pc.Mouse.MoveTo(proto.Point{X: at.X, Y: at.Y}); err != nil {
		return err
	}
	wheel := func(delta float64) error {
		return proto.InputDispatchMouseEvent{Type: proto.InputDispatchMouseEventTypeMouseWheel, X: at.X, Y: at.Y, DeltaY: delta}.Call(pc)
	}
	rest := plan.Distance
	if plan.Jump != 0 {
		if err := wheel(plan.Jump); err != nil {
			return err
		}
		rest -= plan.Jump
	}
	if math.Abs(rest) < 1 {
		return nil
	}
	dur := math.Max(plan.Duration, 100)
	speed := int(math.Max(math.Abs(rest)/(dur/1000), 200))
	gesture := proto.InputSynthesizeScrollGesture{
		X: at.X, Y: at.Y, YDistance: gson.Num(-rest), Speed: gson.Int(speed),
		GestureSourceType: proto.InputGestureSourceTypeMouse, PreventFling: true,
	}
	if err := gesture.Call(pc); err == nil {
		return nil
	}
	return s.wheelFrames(ctx, pc, wheel, plan, rest)
}

// wheelFrames é o plano B do wheelPlan: um evento de roda por keyframe com o delta do plano.
func (s *Session) wheelFrames(ctx context.Context, p *rod.Page, wheel func(float64) error, plan cursor.ScrollPlan, rest float64) error {
	f := plan.Frames
	start := time.Now()
	last := plan.Distance - rest
	for i := 0; i < len(f); i += 2 {
		if f[i+1] == 0 || (plan.Jump != 0 && i == 2) {
			continue
		}
		if wait := time.Until(start.Add(time.Duration(f[i] * float64(time.Millisecond)))); wait > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		delta := f[i+1] - last
		last = f[i+1]
		if delta != 0 {
			if err := wheel(delta); err != nil {
				return err
			}
		}
	}
	return nil
}
