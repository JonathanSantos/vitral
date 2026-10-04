package overlay

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/ux"
)

// cursorFn entrega a ordem ao overlay e diz se ele existia (no meio de uma navegação pode não
// existir: aí ninguém vai responder e não vale a pena esperar).
const cursorFn = `(c) => { const o = window.__vitralOverlay; if (!o || !o.cursor) return false; o.cursor(c); return true; }`

// errNoOverlay indica que a página ainda não tem o overlay (documento novo carregando).
var errNoOverlay = errors.New("overlay not ready")

// recentPlans é quantos planos o Manager lembra (RecentPlans).
const recentPlans = 8

// RecentPlans devolve os últimos planos tocados, do mais antigo ao mais novo.
func (m *Manager) RecentPlans() []cursor.Plan {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]cursor.Plan(nil), m.plans...)
}

// arrivalSlack é a folga sobre a duração do plano antes de desistir de esperar o overlay (2.1).
const arrivalSlack = 300 * time.Millisecond

// scrollFn entrega a rolagem ao overlay (mesmo protocolo do cursorFn).
const scrollFn = `(c) => { const o = window.__vitralOverlay; if (!o || !o.scroll) return false; o.scroll(c); return true; }`

func (m *Manager) cursorCmd(ctx context.Context, c map[string]any) error {
	return m.cursorCmdFn(ctx, cursorFn, c)
}

func (m *Manager) cursorCmdFn(ctx context.Context, fn string, c map[string]any) error {
	if m.opts.Minimal {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var ok bool
	if err := m.world.Unmarshal(ctx, &ok, fn, c); err != nil {
		m.log.Debug("overlay cursor", "op", c["op"], "err", err)
		return err
	}
	if !ok {
		return errNoOverlay
	}
	return nil
}

// Arrival é quando o evento real deve sair: no press visual, ou no fim do movimento se não há
// clique (ring vazio).
func Arrival(plan cursor.Plan, click bool) time.Duration {
	ms := plan.Move
	if click {
		ms = plan.Press
	}
	return time.Duration(ms * float64(time.Millisecond))
}

// PlayCursor toca o plano e espera o overlay avisar a chegada (cursor_done). ring é a cor do anel
// do clique ("" = sem clique); appear faz o cursor surgir com fade (primeira aparição). Nunca bloqueia
// a ação: devolve false se o overlay não respondeu dentro da duração + 300 ms, ou se não existe.
func (m *Manager) PlayCursor(ctx context.Context, plan cursor.Plan, ring string, appear bool) bool {
	if m.opts.Minimal {
		return false
	}
	m.mu.Lock()
	m.cursorSeq++
	id := strconv.FormatUint(m.cursorSeq, 10)
	ch := make(chan string, 1)
	if m.waiters == nil {
		m.waiters = map[string]chan string{}
	}
	m.waiters[id] = ch
	defer func() {
		m.mu.Lock()
		delete(m.waiters, id)
		m.mu.Unlock()
	}()

	m.cursorAt, m.cursorShown = plan.End, true
	m.plans = append(m.plans, plan)
	if len(m.plans) > recentPlans {
		m.plans = m.plans[len(m.plans)-recentPlans:]
	}
	m.mu.Unlock()
	cmd := map[string]any{"op": "play", "id": id, "plan": plan, "appear": appear, "label": ux.TextCursorLabel}
	if ring != "" {
		cmd["ring"] = ring
	}
	if err := m.cursorCmd(ctx, cmd); err != nil {
		return false
	}
	timeout := Arrival(plan, ring != "") + arrivalSlack
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		m.log.Debug("cursor_done timeout: acting anyway", "id", id, "timeout", timeout)
	case <-ctx.Done():
	}
	return false
}

// cursorDone acorda quem espera o plano id (cursor_done ou scroll_done da página, já com token
// válido), com o resultado que a página informou.
func (m *Manager) cursorDone(id, result string) {
	m.mu.Lock()
	ch := m.waiters[id]
	m.mu.Unlock()
	if ch != nil {
		select {
		case ch <- result:
		default:
		}
	}
}

// waiter registra quem espera o id; release tira o registro.
func (m *Manager) waiter() (string, chan string, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cursorSeq++
	id := strconv.FormatUint(m.cursorSeq, 10)
	ch := make(chan string, 1)
	if m.waiters == nil {
		m.waiters = map[string]chan string{}
	}
	m.waiters[id] = ch
	return id, ch, func() {
		m.mu.Lock()
		delete(m.waiters, id)
		m.mu.Unlock()
	}
}

// Resultados de PlayScroll além dos que a página informa ("ok", "hidden", "unresponsive",
// "interrupted", "missing").
const (
	ScrollTimeout   = "timeout"
	ScrollNoOverlay = "no-overlay"
)

// PlayScroll toca a rolagem no container ("page" ou a ref do container) e espera o scroll_done, com
// timeout = duração + 300 ms: a ação nunca fica presa na animação.
func (m *Manager) PlayScroll(ctx context.Context, container string, plan cursor.ScrollPlan) string {
	if m.opts.Minimal {
		return ScrollNoOverlay
	}
	id, ch, release := m.waiter()
	defer release()
	m.mu.Lock()
	m.scrollCount++
	m.scrolls = append(m.scrolls, plan)
	if len(m.scrolls) > recentPlans {
		m.scrolls = m.scrolls[len(m.scrolls)-recentPlans:]
	}
	m.mu.Unlock()
	if err := m.cursorCmdFn(ctx, scrollFn, map[string]any{"id": id, "container": container, "plan": plan}); err != nil {
		return ScrollNoOverlay
	}
	timeout := time.Duration(plan.Duration*float64(time.Millisecond)) + arrivalSlack
	result := ScrollTimeout
	select {
	case result = <-ch:
	case <-time.After(timeout):
		m.log.Debug("scroll_done timeout: acting anyway", "id", id, "timeout", timeout)
	case <-ctx.Done():
	}
	m.mu.Lock()
	m.lastScroll = result
	m.mu.Unlock()
	return result
}

// ScrollsStarted conta as rolagens enviadas ao overlay (diagnóstico e testes).
func (m *Manager) ScrollsStarted() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scrollCount
}

// LastScrollResult é o resultado da última rolagem tocada (diagnóstico e testes).
func (m *Manager) LastScrollResult() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastScroll
}

// RecentScrollPlans devolve as últimas rolagens tocadas, da mais antiga à mais nova.
func (m *Manager) RecentScrollPlans() []cursor.ScrollPlan {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]cursor.ScrollPlan(nil), m.scrolls...)
}

// PlaceCursor põe o cursor num ponto sem animar.
func (m *Manager) PlaceCursor(ctx context.Context, p cursor.Point, visible bool) error {
	if m.opts.Minimal {
		return nil
	}
	m.mu.Lock()
	m.cursorAt, m.cursorShown = p, true
	m.mu.Unlock()
	return m.cursorCmd(ctx, map[string]any{"op": "place", "x": p.X, "y": p.Y, "visible": visible, "label": ux.TextCursorLabel})
}

// CursorPosition é onde o cursor está na página atual; ok=false antes da primeira aparição.
func (m *Manager) CursorPosition() (cursor.Point, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cursorAt, m.cursorShown
}

// rehydrateCursor recoloca o cursor na última posição quando um documento novo monta o overlay
// (navegação ou reload; a página limita à viewport).
func (m *Manager) rehydrateCursor() {
	at, shown := m.CursorPosition()
	if !shown || m.opts.Minimal {
		return
	}
	_ = m.cursorCmd(context.Background(), map[string]any{"op": "place", "x": at.X, "y": at.Y, "visible": true, "label": ux.TextCursorLabel})
}

// NavStart mostra "Abrindo {host}" ou "Voltando" no selo e deixa o cursor a 40% (5.6).
func (m *Manager) NavStart(ctx context.Context, label string) {
	m.Apply(ux.Event{Kind: ux.NavStarted, Reason: label})
	_ = m.CursorNav(ctx, true)
}

// NavEnd devolve o selo e o cursor ao normal depois que a página carregou.
func (m *Manager) NavEnd(ctx context.Context) {
	m.Apply(ux.Event{Kind: ux.NavEnded})
	_ = m.CursorNav(ctx, false)
}

// ShowHighlight mostra o destaque por d sem esperar: com o cursor, o destaque começa junto com o
// movimento e a espera mínima vai para a mira (5.1).
func (m *Manager) ShowHighlight(ctx context.Context, ref string, box [4]int, d time.Duration) {
	if m.opts.Minimal || d <= 0 || box[2] <= 0 || box[3] <= 0 {
		return
	}
	data, _ := json.Marshal(newMark(ref, ux.Box(box), int(d.Milliseconds())))
	m.eval(ctx, "highlight", string(data))
}

// HighlightDuration é o tempo mínimo de destaque configurado (--highlight-ms).
func (m *Manager) HighlightDuration() time.Duration { return m.opts.Highlight }

// CursorNav deixa o cursor a 40% enquanto a página carrega (open/back).
func (m *Manager) CursorNav(ctx context.Context, on bool) error {
	return m.cursorCmd(ctx, map[string]any{"op": "nav", "on": on, "label": ux.TextCursorLabel})
}

// HideCursor esconde o cursor ("Sua vez": a pessoa assume).
func (m *Manager) HideCursor(ctx context.Context) error {
	return m.cursorCmd(ctx, map[string]any{"op": "hide", "label": ux.TextCursorLabel})
}

// CursorNote mostra uma etiqueta curta junto ao cursor (ex.: "Selecionado: Brasil").
func (m *Manager) CursorNote(ctx context.Context, text string, d time.Duration) error {
	return m.cursorCmd(ctx, map[string]any{"op": "note", "text": text, "ms": d.Milliseconds(), "label": ux.TextCursorLabel})
}

// CursorFail faz o "não" com a cabeça e explica para a pessoa por que nada aconteceu (6.4).
func (m *Manager) CursorFail(ctx context.Context, text string) error {
	return m.cursorCmd(ctx, map[string]any{"op": "fail", "text": text, "label": ux.TextCursorLabel})
}

// Keycap mostra a tecla pressionada acima do selo (5.5).
func (m *Manager) Keycap(ctx context.Context, key string) error {
	return m.cursorCmd(ctx, map[string]any{"op": "keycap", "text": key, "label": ux.TextCursorLabel})
}
