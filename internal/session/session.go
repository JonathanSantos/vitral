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
	"github.com/go-rod/rod/lib/proto"

	"github.com/jors/browser-mcp/internal/browser"
	"github.com/jors/browser-mcp/internal/config"
	"github.com/jors/browser-mcp/internal/console"
	"github.com/jors/browser-mcp/internal/flow"
	"github.com/jors/browser-mcp/internal/network"
	"github.com/jors/browser-mcp/internal/snapshot"
)

// Tempos da espera por estabilidade (seção 8.2 da spec).
const (
	networkQuiet = 500 * time.Millisecond
	domQuiet     = 300 * time.Millisecond
	pollEvery    = 100 * time.Millisecond
	evalTimeout  = 5 * time.Second
)

// Session serializa as ações na página ativa e guarda o último snapshot para o diff.
type Session struct {
	cfg     config.Config
	log     *slog.Logger
	browser *browser.Manager
	net     *network.Tracker
	console *console.Buffer
	format  snapshot.FormatOptions

	rec       *flow.Recorder
	replaying atomic.Bool

	mu        sync.Mutex
	last      *snapshot.Page
	current   *rod.Page
	emulation Emulation
}

// New cria a sessão e registra o preparo de páginas novas no gerenciador.
func New(cfg config.Config, logger *slog.Logger, b *browser.Manager, net *network.Tracker, con *console.Buffer) *Session {
	s := &Session{cfg: cfg, log: logger, browser: b, net: net, console: con, format: snapshot.DefaultFormat(), rec: &flow.Recorder{}}
	b.OnNewPage(s.preparePage)
	return s
}

func (s *Session) preparePage(p *rod.Page) error {
	if _, err := p.EvalOnNewDocument(snapshot.Script); err != nil {
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

// evalJSON chama uma função JS e devolve o resultado em JSON.
func evalJSON(ctx context.Context, p *rod.Page, js string, args ...any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	if err := ensureScript(ctx, p); err != nil {
		return nil, err
	}
	res, err := p.Context(ctx).Evaluate(rod.Eval(js, args...).ByPromise())
	if err != nil {
		return nil, fmt.Errorf("evaluate in page: %w", err)
	}
	return []byte(res.Value.JSON("", "")), nil
}

// ensureScript injeta o script em documentos abertos antes do EvalOnNewDocument (ex.: attach).
func ensureScript(ctx context.Context, p *rod.Page) error {
	res, err := p.Context(ctx).Evaluate(rod.Eval(`() => !!window.__mcp`))
	if err != nil {
		return fmt.Errorf("evaluate in page: %w", err)
	}
	if res.Value.Bool() {
		return nil
	}
	_, err = proto.RuntimeEvaluate{Expression: snapshot.Script}.Call(p.Context(ctx))
	return err
}

func evalInto(ctx context.Context, p *rod.Page, out any, js string, args ...any) error {
	data, err := evalJSON(ctx, p, js, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// capture tira um snapshot estruturado.
func capture(ctx context.Context, p *rod.Page, opts snapshot.Options) (*snapshot.Page, error) {
	data, err := evalJSON(ctx, p, `(o) => window.__mcp.snapshot(o)`, opts)
	if err != nil {
		return nil, err
	}
	return snapshot.Parse(data)
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
			return fmt.Sprintf("warning: page did not settle within %s (%d requests pending); result may be incomplete.",
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
	if err := evalInto(ctx, p, &ms, `() => window.__mcp.quietMs()`); err != nil {
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
	netMark, conMark := s.net.Mark(), s.console.Mark()
	start := time.Now()
	if err := do(p); err != nil {
		return "", err
	}
	warning := s.waitStable(ctx, p, start)
	curr, err := capture(ctx, p, snapshot.Options{Known: knownRefs(s.last)})
	if err != nil {
		return "", fmt.Errorf("action done, but snapshot failed: %w. Call snapshot.", err)
	}
	text := snapshot.Summarize(s.last, curr, s.format)
	s.last = curr
	return joinNonEmpty(text, s.events(netMark, conMark), warning, s.record(rec, sel, note)), nil
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
		lines = append(lines, fmt.Sprintf("console: %d new error(s), last: %s", len(errs), console.Line(errs[len(errs)-1])))
	}
	var bad []string
	for _, e := range s.net.Entries() {
		if e.ID > netMark && network.IsProblem(e) {
			bad = append(bad, network.Line(e))
		}
	}
	if len(bad) > 0 {
		shown := bad
		if len(shown) > 3 {
			shown = shown[len(shown)-3:]
		}
		lines = append(lines, fmt.Sprintf("network: %d failed request(s): %s", len(bad), strings.Join(shown, "; ")))
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
	p, err := s.page()
	if err != nil {
		return "", err
	}
	page, err := capture(ctx, p, snapshot.Options{Full: opts.Full, Scope: opts.Scope})
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
	return snapshot.Format(page, format), nil
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
