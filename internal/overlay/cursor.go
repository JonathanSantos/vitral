package overlay

import (
	"context"
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

// arrivalSlack é a folga sobre a duração do plano antes de desistir de esperar o overlay (2.1).
const arrivalSlack = 300 * time.Millisecond

func (m *Manager) cursorCmd(ctx context.Context, c map[string]any) error {
	if m.opts.Minimal {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var ok bool
	if err := m.world.Unmarshal(ctx, &ok, cursorFn, c); err != nil {
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
	ch := make(chan struct{}, 1)
	if m.waiters == nil {
		m.waiters = map[string]chan struct{}{}
	}
	m.waiters[id] = ch
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.waiters, id)
		m.mu.Unlock()
	}()

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

// cursorDone acorda quem espera o plano id (evento cursor_done da página, já com token válido).
func (m *Manager) cursorDone(id string) {
	m.mu.Lock()
	ch := m.waiters[id]
	m.mu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// PlaceCursor põe o cursor num ponto sem animar (reidratação depois de navegar).
func (m *Manager) PlaceCursor(ctx context.Context, p cursor.Point, visible bool) error {
	return m.cursorCmd(ctx, map[string]any{"op": "place", "x": p.X, "y": p.Y, "visible": visible, "label": ux.TextCursorLabel})
}

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
