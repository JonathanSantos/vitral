// Package overlay desenha o selo, a pausa e o destaque na página controlada (docs/UX-TASKS.md).
// O estado vive em Go (pacote ux); a página só renderiza o que recebe.
package overlay

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/brand"
	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/ux"
	"github.com/JonathanSantos/vitral/internal/world"
)

//go:embed overlay.js
var scriptTemplate string

// CursorScript é o tocador do cursor visual (cursor.js): o overlay e o tools/cursorlab usam o mesmo.
//
//go:embed cursor.js
var CursorScript string

// ScrollScript é o tocador da rolagem humana (scroll.js): o overlay e o tools/cursorlab usam o mesmo.
//
//go:embed scroll.js
var ScrollScript string

// Tag é o nome do elemento host. O snapshot ignora tudo dentro dele.
const Tag = "vitral-overlay"

// bindingName é a ponte página → Go (Runtime.addBinding).
const bindingName = "__vitral_event"

// DefaultHighlight é o tempo padrão do destaque antes de uma ação (UX-1).
const DefaultHighlight = 350 * time.Millisecond

// Options configura o overlay.
type Options struct {
	// Minimal deixa só o selo com o botão de pausa (--no-overlay).
	Minimal bool
	// Highlight é quanto o destaque fica na tela antes da ação; 0 desliga.
	Highlight time.Duration
}

// Manager guarda o estado do selo e conversa com a página ativa.
type Manager struct {
	log    *slog.Logger
	opts   Options
	token  string
	script string

	world *world.Tracker

	mu    sync.Mutex
	state ux.State
	page  *rod.Page

	renderMu sync.Mutex

	// Cursor visual: quem espera o cursor_done de cada plano tocado, e onde o cursor está na página
	// atual (para reaparecer no mesmo lugar depois de navegar).
	cursorSeq   uint64
	waiters     map[string]chan struct{}
	cursorAt    cursor.Point
	cursorShown bool
	plans       []cursor.Plan // últimos planos tocados (diagnóstico e testes)
}

// New cria o gerenciador. O token impede que scripts da página acionem a pausa pela binding.
func New(opts Options, logger *slog.Logger, w *world.Tracker) *Manager {
	token := newToken()
	return &Manager{
		world:  w,
		log:    logger,
		opts:   opts,
		token:  token,
		script: Script(token, opts.Minimal),
		state:  ux.State{Mode: ux.Idle},
	}
}

func newToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// Script monta o JS do overlay com o token e as cores da marca.
func Script(token string, minimal bool) string {
	q := func(s string) string {
		b, _ := json.Marshal(s)
		return string(b)
	}
	r := strings.NewReplacer(
		"__TOKEN__", q(token),
		"__CSS_VARS__", q(brand.CSSVars()),
		"__MINIMAL__", strconv.FormatBool(minimal),
		"__CURSOR__", CursorScript,
	)
	return r.Replace(scriptTemplate)
}

// Attach prepara uma página nova: binding e script só no mundo isolado do Vitral, onde o
// JavaScript da página não alcança (nem para chamar a binding, nem para mexer no overlay).
func (m *Manager) Attach(p *rod.Page) error {
	m.mu.Lock()
	m.page = p
	m.cursorShown = false // aba nova: o cursor surge do selo de novo
	m.mu.Unlock()
	if err := (proto.RuntimeAddBinding{Name: bindingName, ExecutionContextName: world.Name}).Call(p); err != nil {
		return fmt.Errorf("add overlay binding: %w", err)
	}
	go p.EachEvent(func(e *proto.RuntimeBindingCalled) {
		if e.Name == bindingName {
			go m.onPageEvent(e.Payload)
		}
	})()
	if err := world.AddScript(p, m.script); err != nil {
		return fmt.Errorf("inject overlay: %w", err)
	}
	return nil
}

type pageEvent struct {
	Token string `json:"token"`
	Type  string `json:"type"`
	ID    string `json:"id"`
}

// onPageEvent valida o que veio da página. Só o botão do selo pausa, retoma ou recolhe.
func (m *Manager) onPageEvent(payload string) {
	var e pageEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil || e.Token != m.token {
		m.log.Debug("ignored overlay event", "payload_len", len(payload))
		return
	}
	switch ux.EventKind(e.Type) {
	case ux.UserToggle, ux.Collapse, ux.Expand:
		m.Apply(ux.Event{Kind: ux.EventKind(e.Type)})
	case "ready":
		m.render()
		m.rehydrateCursor()
	case "cursor_done":
		m.cursorDone(e.ID)
	}
}

// Apply aplica um evento ao estado e redesenha o selo.
func (m *Manager) Apply(e ux.Event) {
	m.mu.Lock()
	m.state = ux.Next(m.state, e)
	m.mu.Unlock()
	go m.render()
}

// State devolve o estado atual.
func (m *Manager) State() ux.State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Blocked indica pausa (pelo usuário ou aguardando o usuário).
func (m *Manager) Blocked() bool { return m.State().Blocked() }

// ToolStarted e ToolFinished marcam o selo como "trabalhando".
func (m *Manager) ToolStarted()  { m.Apply(ux.Event{Kind: ux.ToolStarted}) }
func (m *Manager) ToolFinished() { m.Apply(ux.Event{Kind: ux.ToolFinished}) }

// FlowProgress mostra "Executando fluxo: nome · passo i de n".
func (m *Manager) FlowProgress(name string, step, total int) {
	m.Apply(ux.Event{Kind: ux.FlowStep, FlowName: name, Step: step, Total: total})
}

// FlowEnded volta o selo para "trabalhando" até a ferramenta terminar.
func (m *Manager) FlowEnded() { m.Apply(ux.Event{Kind: ux.FlowEnded}) }

// WaitForUser pausa e destaca o elemento até a pessoa clicar em Retomar (UX-3, "Sua vez").
func (m *Manager) WaitForUser(reason, ref string, box [4]int) {
	m.Apply(ux.Event{Kind: ux.WaitUser, Reason: reason, Hold: &ux.Highlight{Ref: ref, Box: ux.Box(box)}})
}

type markView struct {
	Ref  string `json:"ref"`
	Box  ux.Box `json:"box"`
	Mark int    `json:"mark"`
	Text string `json:"text"`
	Ms   int    `json:"ms,omitempty"`
}

func newMark(ref string, box ux.Box, ms int) markView {
	n := brand.RefNumber(ref)
	_, text := brand.MarkColor(n)
	textVar := "ink"
	if text == brand.White {
		textVar = "white"
	}
	return markView{Ref: ref, Box: box, Mark: brand.MarkIndex(n), Text: textVar, Ms: ms}
}

// View é o que a página recebe para desenhar: textos já prontos, nada decidido no JS.
type View struct {
	Mode          ux.Mode   `json:"mode"`
	Blocked       bool      `json:"blocked"`
	Label         string    `json:"label"`
	Button        string    `json:"button"`
	Collapsed     bool      `json:"collapsed"`
	CollapseLabel string    `json:"collapseLabel"`
	ExpandLabel   string    `json:"expandLabel"`
	Hold          *markView `json:"hold,omitempty"`
}

// ViewOf converte o estado no que o JS desenha.
func ViewOf(s ux.State) View {
	v := View{
		Mode: s.Mode, Blocked: s.Blocked(), Label: ux.Label(s), Button: ux.ButtonLabel(s),
		Collapsed: s.Collapsed, CollapseLabel: ux.TextCollapse, ExpandLabel: ux.TextExpand,
	}
	if s.Hold != nil {
		mark := newMark(s.Hold.Ref, s.Hold.Box, 0)
		v.Hold = &mark
	}
	return v
}

func (m *Manager) render() {
	m.renderMu.Lock()
	defer m.renderMu.Unlock()
	m.mu.Lock()
	p, s := m.page, m.state
	m.mu.Unlock()
	if p == nil {
		return
	}
	data, _ := json.Marshal(ViewOf(s))
	m.eval(context.Background(), "render", string(data))
}

func (m *Manager) eval(ctx context.Context, fn, arg string) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	expr := fmt.Sprintf("window.__vitralOverlay && window.__vitralOverlay.%s(%s)", fn, arg)
	if err := m.world.Eval(ctx, expr); err != nil {
		m.log.Debug("overlay eval", "fn", fn, "err", err)
	}
}

// Highlight mostra o alvo da próxima ação e espera o tempo configurado (0 = desligado).
func (m *Manager) Highlight(ctx context.Context, p *rod.Page, ref string, box [4]int) {
	d := m.opts.Highlight
	if d <= 0 || m.opts.Minimal || box[2] <= 0 || box[3] <= 0 {
		return
	}
	data, _ := json.Marshal(newMark(ref, ux.Box(box), int(d.Milliseconds())))
	m.eval(ctx, "highlight", string(data))
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// Hidden esconde o overlay enquanto fn roda (screenshot e clique sob o selo) e sempre o devolve.
func (m *Manager) Hidden(ctx context.Context, p *rod.Page, fn func() error) error {
	m.eval(ctx, "hidden", "true")
	// context.Background: o selo precisa voltar mesmo se a ferramenta foi cancelada.
	defer m.eval(context.Background(), "hidden", "false")
	return fn()
}
