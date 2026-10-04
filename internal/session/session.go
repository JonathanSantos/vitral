// Package session executa ações na página ativa e devolve o que mudou.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/console"
	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/network"
	"github.com/JonathanSantos/vitral/internal/snapshot"
	"github.com/JonathanSantos/vitral/internal/storagestate"
	"github.com/JonathanSantos/vitral/internal/ux"
	"github.com/JonathanSantos/vitral/internal/world"
)

// Tempos da espera por estabilidade (seção 8.2 da spec).
const (
	networkQuiet = 500 * time.Millisecond
	domQuiet     = 300 * time.Millisecond
	pollEvery    = 100 * time.Millisecond
	evalTimeout  = 5 * time.Second
)

// Overlay é o que a sessão usa do selo/destaque (implementado por overlay.Manager).
type Overlay interface {
	Highlight(ctx context.Context, p *rod.Page, ref string, box [4]int)
	Hidden(ctx context.Context, p *rod.Page, fn func() error) error
	WaitForUser(reason, ref string, box [4]int)
	FlowProgress(name string, step, total int)
	FlowEnded()
	Blocked() bool
	State() ux.State

	// Cursor visual (docs/CURSOR.md).
	PlayCursor(ctx context.Context, plan cursor.Plan, ring string, appear bool) bool
	PlayScroll(ctx context.Context, container string, plan cursor.ScrollPlan) string
	CursorPosition() (cursor.Point, bool)
	HideCursor(ctx context.Context) error
	CursorNote(ctx context.Context, text string, d time.Duration) error
	CursorFail(ctx context.Context, text string) error
	Keycap(ctx context.Context, key string) error
	NavStart(ctx context.Context, label string)
	NavEnd(ctx context.Context)
	ShowHighlight(ctx context.Context, ref string, box [4]int, d time.Duration)
	HighlightDuration() time.Duration
}

// pausedResult interrompe a ação sem erro: a resposta é a mensagem para a IA (UX-3).
type pausedResult string

func (p pausedResult) Error() string { return string(p) }

// Session serializa as ações na página ativa e guarda o último snapshot para o diff.
type Session struct {
	cfg     config.Config
	log     *slog.Logger
	browser *browser.Manager
	net     *network.Tracker
	console *console.Buffer
	ui      Overlay
	world   *world.Tracker
	format  snapshot.FormatOptions

	rec       *flow.Recorder
	replaying atomic.Bool
	cur       *pointer
	ax        *axRefs                 // motor ax: refs do documento atual
	healed    map[string][]HealedStep // autocura da última execução de cada fluxo (flow_update)
	smap      *screenMap              // mapa de telas (nil com --no-map)
	axStats   AXStats

	// AXFullTree faz o snapshot da área visível usar a árvore de acessibilidade inteira: só para os
	// testes compararem com a busca parcial (a referência).
	AXFullTree bool

	mu        sync.Mutex
	last      *snapshot.Page
	current   *rod.Page
	emulation Emulation
}

// New cria a sessão e registra o preparo de páginas novas no gerenciador.
func New(cfg config.Config, logger *slog.Logger, b *browser.Manager, net *network.Tracker, con *console.Buffer, ui Overlay, w *world.Tracker) *Session {
	s := &Session{cfg: cfg, log: logger, browser: b, net: net, console: con, ui: ui, world: w, format: snapshot.DefaultFormat(), rec: &flow.Recorder{}, cur: newPointer(cfg)}
	if !cfg.NoMap {
		s.smap = newScreenMap(cfg.MapsDir, func(err error) { logger.Warn("screen map", "err", err) })
	}
	b.OnNewPage(s.preparePage)
	return s
}

func (s *Session) preparePage(p *rod.Page) error {
	if err := world.AddScript(p, snapshot.Script); err != nil {
		return fmt.Errorf("inject snapshot script: %w", err)
	}
	if err := s.net.Attach(p); err != nil {
		return fmt.Errorf("enable network: %w", err)
	}
	if err := s.console.Attach(p); err != nil {
		return fmt.Errorf("enable console: %w", err)
	}
	// Roda dentro de s.page() (mutex já travado), então pode ler s.emulation.
	return s.reapplyEmulation(p)
}

// page devolve a página ativa (com s.mu travado). Se a aba mudou, o último snapshot não vale mais.
func (s *Session) page() (*rod.Page, error) {
	p, err := s.browser.Page()
	if err != nil {
		return nil, err
	}
	if p != s.current {
		s.current, s.last = p, nil
	}
	return p, nil
}

// WithPage roda fn com a página ativa e o mutex travado (para ferramentas que não são ações).
func (s *Session) WithPage(fn func(p *rod.Page) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return err
	}
	return fn(p)
}

// evalJSON chama uma função JS no mundo isolado do Vitral e devolve o resultado em JSON.
func (s *Session) evalJSON(ctx context.Context, js string, args ...any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	if err := s.ensureScript(ctx); err != nil {
		return nil, err
	}
	return s.world.Call(ctx, js, args...)
}

// ensureScript reinjeta o script se o mundo isolado existir sem ele (ex.: documento criado
// por um caminho em que o Chrome não rodou os scripts de documento novo).
func (s *Session) ensureScript(ctx context.Context) error {
	var ready bool
	if err := s.world.Unmarshal(ctx, &ready, `() => !!window.__vitral`); err != nil {
		return err
	}
	if ready {
		return nil
	}
	return s.world.Eval(ctx, snapshot.Script)
}

// callRefJSON chama fn(el, ...args) com o elemento da ref: pelo backendNodeId que o Go guarda
// (DOM.resolveNode no mundo isolado, alcança shadow root fechado) ou, para refs criadas pelo próprio
// script, pelo registro dele. A ref fica registrada no script para as buscas reversas (texto, cobertura).
// Elemento que não existe mais chega como null. Ref vazia: el é null.
func (s *Session) callRefJSON(ctx context.Context, ref, fn string, args ...any) ([]byte, error) {
	var el any
	if ref != "" {
		el = s.elem(ref)
	}
	js := `function (el, ref, ...a) { if (el) window.__vitral.bind([ref], [el]); return (` + fn + `)(el, ...a); }`
	return s.evalJSON(ctx, js, append([]any{el, ref}, args...)...)
}

func (s *Session) callRef(ctx context.Context, out any, ref, fn string, args ...any) error {
	data, err := s.callRefJSON(ctx, ref, fn, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (s *Session) evalInto(ctx context.Context, out any, js string, args ...any) error {
	data, err := s.evalJSON(ctx, js, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func knownRefs(p *snapshot.Page) []string {
	var refs []string
	if p == nil {
		return refs
	}
	snapshot.Walk(p.Nodes, func(n, _ *snapshot.Node) { refs = append(refs, n.Ref) })
	return refs
}

// waitStable espera rede ociosa e DOM quieto; devolve um aviso se estourar o timeout.
func (s *Session) waitStable(ctx context.Context, p *rod.Page, start time.Time) string {
	deadline := time.Now().Add(s.cfg.StabilityTimeout)
	pending := 0
	for {
		var idle time.Duration
		pending, idle = s.net.IdleSince(start)
		if pending == 0 && idle >= networkQuiet && s.domQuietFor(ctx, p, start) >= domQuiet {
			return ""
		}
		if time.Now().After(deadline) {
			return fmt.Sprintf("warning: page did not settle within %s (%d requests pending); result may be incomplete. If something you expect is missing, call wait_for with its text.",
				s.cfg.StabilityTimeout, pending)
		}
		select {
		case <-ctx.Done():
			return "warning: wait canceled; result may be incomplete."
		case <-time.After(pollEvery):
		}
	}
}

func (s *Session) domQuietFor(ctx context.Context, p *rod.Page, start time.Time) time.Duration {
	var ms float64
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	// Durante uma navegação a avaliação falha: tratamos como "ainda mexendo".
	if err := s.evalInto(ctx, &ms, `() => window.__vitral.quietMs()`); err != nil {
		return 0
	}
	quiet := time.Duration(ms) * time.Millisecond
	if since := time.Since(start); since < quiet {
		return since
	}
	return quiet
}

// recording diz como gravar uma ação no fluxo: a ref do alvo (se houver) e o passo resultante.
type recording struct {
	ref  string
	step func(sel *flow.Selector) flow.Step
}

// action roda do() e devolve o diff da página, com eventos de rede/console disparados.
func (s *Session) action(ctx context.Context, rec *recording, do func(p *rod.Page) error) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return "", err
	}
	// O seletor é calculado antes: depois do clique o elemento pode nem existir mais.
	sel, note := s.selectorFor(ctx, p, rec)
	from, fromOK := s.observe(ctx)
	mapAct := s.mapAction(rec, sel)
	netMark, conMark := s.net.Mark(), s.console.Mark()
	start := time.Now()
	if err := do(p); err != nil {
		var paused pausedResult
		if errors.As(err, &paused) {
			return string(paused), nil
		}
		s.learn(ctx, from, fromOK, mapAct, true)
		return "", err
	}
	warning := s.waitStable(ctx, p, start)
	curr, err := s.capture(ctx, snapshot.Options{Known: knownRefs(s.last)})
	if err != nil {
		return "", fmt.Errorf("action done, but snapshot failed: %w. Call snapshot.", err)
	}
	text := snapshot.Summarize(s.last, curr, s.format)
	s.last = curr
	recorded := s.record(rec, sel, note)
	s.learn(ctx, from, fromOK, mapAct, false)
	verify, err := s.verificationGate(ctx)
	if err != nil {
		return "", err
	}
	return joinNonEmpty(text, s.events(netMark, conMark), warning, recorded, verify), nil
}

// events resume erros de console e requests com problema desde as marcas.
func (s *Session) events(netMark, conMark int) string {
	var lines []string
	var errs []console.Message
	for _, m := range s.console.Messages() {
		if m.ID > conMark && m.Level == "error" && !network.IsNoise(m.Source) {
			errs = append(errs, m)
		}
	}
	if len(errs) > 0 {
		lines = append(lines, fmt.Sprintf("console: %d new error(s), last: %s (details: console_list)", len(errs), console.Brief(errs[len(errs)-1])))
	}
	var problems []network.Entry
	for _, e := range s.net.Entries() {
		if e.ID > netMark && network.IsProblem(e) {
			problems = append(problems, e)
		}
	}
	bad := network.BriefGroups(problems)
	for _, e := range s.net.Entries() {
		if e.ID > netMark && strings.EqualFold(e.Type, "document") && strings.Contains(e.Failed, "ERR_BLOCKED_BY_CLIENT") && len(s.cfg.AllowedHosts) > 0 {
			lines = append(lines, fmt.Sprintf("blocked: navigation to %s is outside --allowed-hosts (%s). Tell the user or stay on the allowed hosts.", e.URL, strings.Join(s.cfg.AllowedHosts, ", ")))
		}
	}
	if len(bad) > 0 {
		shown := bad
		if len(shown) > 3 {
			shown = shown[len(shown)-3:]
		}
		lines = append(lines, fmt.Sprintf("network: %d failed request(s): %s (details: network_list)", len(problems), strings.Join(shown, "; ")))
	}
	return strings.Join(lines, "\n")
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n")
}

// SnapshotOptions são as opções da ferramenta snapshot.
type SnapshotOptions struct {
	Scope    string
	Full     bool
	MaxLines int
}

// Snapshot devolve o snapshot em texto. Só o snapshot padrão vira base do próximo diff.
func (s *Session) Snapshot(ctx context.Context, opts SnapshotOptions) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.page(); err != nil {
		return "", err
	}
	page, err := s.capture(ctx, snapshot.Options{Full: opts.Full, Scope: opts.Scope})
	if err != nil {
		return "", err
	}
	if !opts.Full && opts.Scope == "" {
		s.last = page
	}
	format := s.format
	if opts.MaxLines > 0 {
		format.MaxLines = opts.MaxLines
	}
	format.ScopeRoot = opts.Scope != ""
	if format.ScopeRoot {
		// Com scope, os textos vêm inteiros: é o jeito de ler o que o snapshot cortou com "…".
		format.TextMax = snapshot.FullTextMax
		return snapshot.Format(page, format), nil
	}
	return withScreenLine(snapshot.Format(page, format), s.screenLine(ctx)), nil
}

// TextOnPage diz se o texto está visível na página atual (como o wait_for procura) e se está no título.
func (s *Session) TextOnPage(ctx context.Context, text string) (visible, inTitle bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.page(); err != nil {
		return false, false, err
	}
	var res struct {
		Found bool `json:"found"`
		Title bool `json:"title"`
	}
	js := `(t) => ({found: window.__vitral.findText(t).found,
		title: document.title.toLowerCase().includes(t.replace(/\s+/g, ' ').trim().toLowerCase())})`
	err = s.evalInto(ctx, &res, js, text)
	return res.Found, res.Title, err
}

// refError traduz o resultado das funções JS em mensagens acionáveis.
func refError(ref, code string) error {
	switch code {
	case "":
		return nil
	case "not_found":
		return snapshot.ErrRefNotFound{Ref: ref}
	case "hidden":
		return fmt.Errorf("element %s is not visible. Call snapshot.", ref)
	case "offscreen":
		return fmt.Errorf("element %s could not be scrolled into view. Try scroll, then snapshot.", ref)
	case "disabled":
		return fmt.Errorf("element %s is disabled.", ref)
	}
	return errors.New(code)
}

// ReadTextMax é quanto texto o snapshot com text=true devolve de uma vez (≈1000 tokens).
const ReadTextMax = 4000

// ReadText devolve o texto visível de um elemento (ou do conteúdo principal da página): é como a IA
// lê parágrafos, que o snapshot não lista. Com find, devolve só os trechos que contêm o termo.
func (s *Session) ReadText(ctx context.Context, scope string, from int, find string) (string, error) {
	from = max(from, 0)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.page(); err != nil {
		return "", err
	}
	var res struct {
		Error   string `json:"error"`
		Role    string `json:"role"`
		Ref     string `json:"ref"`
		Section bool   `json:"section"`
		Text    string `json:"text"`
		Total   int    `json:"total"`
		Count   int    `json:"count"`
		AnyWord bool   `json:"anyWord"`
		Hits    []struct {
			At   int    `json:"at"`
			Text string `json:"text"`
		} `json:"hits"`
	}
	js := `(el, r, m, f, q) => window.__vitral.readText(el, r !== '', m, f, q)`
	if err := s.callRef(ctx, &res, scope, js, scope, ReadTextMax, from, strings.TrimSpace(find)); err != nil {
		return "", err
	}
	if err := refError(scope, res.Error); err != nil {
		return "", err
	}
	var head struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	}
	_ = s.evalInto(ctx, &head, `() => ({title: document.title, url: location.href})`)
	what := "main content"
	if res.Ref != "" {
		what = fmt.Sprintf("[%s] %s", res.Ref, res.Role)
	}
	if res.Section {
		what = "section " + what
	}
	lines := []string{fmt.Sprintf("page: %q (%s)", head.Title, head.URL)}

	if find = strings.TrimSpace(find); find != "" {
		if res.Count == 0 {
			return strings.Join(append(lines, fmt.Sprintf("%q not found in the text of %s (%d chars).", find, what, res.Total)), "\n"), nil
		}
		summary := fmt.Sprintf("%q in the text of %s (%d chars): %d match(es)", find, what, res.Total, res.Count)
		if res.AnyWord {
			summary = fmt.Sprintf("the exact phrase %q is not in the text of %s (%d chars); passages with any of its words: %d match(es)", find, what, res.Total, res.Count)
		}
		if len(res.Hits) > 0 && res.Count > len(res.Hits) {
			summary += fmt.Sprintf(", passages around the first %d shown", len(res.Hits))
		}
		lines = append(lines, summary+":")
		for _, h := range res.Hits {
			lines = append(lines, fmt.Sprintf("@%d: %s", h.At, h.Text))
		}
		return strings.Join(append(lines, "To read around a passage, use text=true with textFrom=<@number>."), "\n"), nil
	}

	if res.Text == "" {
		if from > 0 && res.Total > 0 {
			return strings.Join(append(lines, fmt.Sprintf("text of %s: nothing after char %d (it has %d chars).", what, from, res.Total)), "\n"), nil
		}
		return strings.Join(append(lines, fmt.Sprintf("text of %s: (no visible text)", what)), "\n"), nil
	}
	header := fmt.Sprintf("text of %s:", what)
	if from > 0 {
		header = fmt.Sprintf("text of %s, from char %d:", what, from)
	}
	lines = append(lines, header, res.Text)
	if end := from + len([]rune(res.Text)); end < res.Total {
		lines = append(lines, fmt.Sprintf("… showing chars %d-%d of %d. Looking for something specific? Use find=<words> to get only the passages with them. To keep reading: textFrom=%d, or one section with scope=<heading ref> (headings are marked like ## Title[eN]).", from, end, res.Total, end))
	}
	return strings.Join(lines, "\n"), nil
}

// SaveState salva o estado de login do navegador (session_save); devolve só o resumo.
func (s *Session) SaveState(name string) (storagestate.Summary, error) {
	return s.browser.SaveState(name)
}
