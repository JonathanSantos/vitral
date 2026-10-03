package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/overlay"
	"github.com/JonathanSantos/vitral/internal/security"
	"github.com/JonathanSantos/vitral/internal/snapshot"
	"github.com/JonathanSantos/vitral/internal/ux"
)

// Open navega para a URL e devolve o snapshot completo.
func (s *Session) Open(ctx context.Context, rawURL string) (string, error) {
	target := security.NormalizeURL(rawURL)
	if target == "" {
		return "", errors.New("url is required")
	}
	if err := security.CheckNavigation(s.cfg.AllowedHosts, target); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return "", err
	}
	conMark, netMark := s.console.Mark(), s.net.Mark()
	start := time.Now()
	if err := s.navigate(ctx, p, target); err != nil {
		return "", err
	}
	warning := s.waitStable(ctx, p, start)
	page, err := capture(ctx, p, snapshot.Options{})
	if err != nil {
		return "", fmt.Errorf("page opened, but snapshot failed: %w. Call snapshot.", err)
	}
	s.last = page
	rec := &recording{step: func(*flow.Selector) flow.Step { return flow.Step{Open: target} }}
	return joinNonEmpty(snapshot.Format(page, s.format), s.events(netMark, conMark), warning, s.record(rec, nil, "")), nil
}

func (s *Session) navigate(ctx context.Context, p *rod.Page, target string) error {
	timeout := s.cfg.NavTimeout
	navCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := p.Context(navCtx).Navigate(target)
	if err == nil {
		err = waitDOMReady(navCtx, p)
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || navCtx.Err() != nil {
		return fmt.Errorf("navigation timeout after %s. Page may still be loading; call snapshot.", timeout)
	}
	var navErr *rod.NavigationError
	if errors.As(err, &navErr) {
		return fmt.Errorf("navigation to %s failed: %s. Check the URL or whether the server is running.", target, navErr.Reason)
	}
	return fmt.Errorf("navigate to %s: %w", target, err)
}

func waitDOMReady(ctx context.Context, p *rod.Page) error {
	for {
		res, err := p.Context(ctx).Evaluate(rod.Eval(`() => document.readyState`))
		if err == nil && res.Value.Str() != "loading" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollEvery):
		}
	}
}

// Back volta uma página no histórico.
func (s *Session) Back(ctx context.Context) (string, error) {
	rec := &recording{step: func(*flow.Selector) flow.Step { return flow.Step{Back: true} }}
	return s.action(ctx, rec, func(p *rod.Page) error {
		hist, err := proto.PageGetNavigationHistory{}.Call(p)
		if err == nil && hist.CurrentIndex == 0 {
			return errors.New("no previous page in history.")
		}
		return p.NavigateBack()
	})
}

type hitInfo struct {
	Error     string   `json:"error"`
	Role      string   `json:"role"`
	Tag       string   `json:"tag"`
	Password  bool     `json:"password"`
	Disabled  bool     `json:"disabled"`
	X         float64  `json:"x"`
	Y         float64  `json:"y"`
	Box       [4]int   `json:"box"`
	CoveredBy *covered `json:"coveredBy"`
}

type covered struct {
	Ref  string `json:"ref"`
	Role string `json:"role"`
}

func (c covered) String() string {
	if c.Ref == "" {
		return c.Role
	}
	return c.Role + " " + c.Ref
}

// Click faz um clique real (CDP) no centro do elemento, depois de checar se está livre.
func (s *Session) Click(ctx context.Context, ref string) (string, error) {
	rec := &recording{ref: ref, step: func(sel *flow.Selector) flow.Step { return flow.Step{Click: sel} }}
	return s.action(ctx, rec, func(p *rod.Page) error {
		hit, err := s.hit(ctx, p, ref)
		if err != nil {
			return err
		}
		s.ui.Highlight(ctx, p, ref, hit.Box)
		if !hit.coveredByOverlay() {
			return clickAt(ctx, p, ref, hit)
		}
		// O alvo está sob o selo do Vitral: esconde o selo só durante o clique.
		return s.ui.Hidden(ctx, p, func() error {
			hit, err := s.hit(ctx, p, ref)
			if err != nil {
				return err
			}
			return clickAt(ctx, p, ref, hit)
		})
	})
}

// hit rola até o elemento e confere se ele pode receber o clique.
func (s *Session) hit(ctx context.Context, p *rod.Page, ref string) (hitInfo, error) {
	var hit hitInfo
	if err := evalInto(ctx, p, &hit, `(r) => window.__vitral.hit(r)`, ref); err != nil {
		return hit, err
	}
	if err := refError(ref, hit.Error); err != nil {
		return hit, err
	}
	if hit.Disabled {
		return hit, refError(ref, "disabled")
	}
	return hit, nil
}

func (h hitInfo) coveredByOverlay() bool {
	return h.CoveredBy != nil && h.CoveredBy.Role == overlay.Tag
}

func clickAt(ctx context.Context, p *rod.Page, ref string, hit hitInfo) error {
	if hit.CoveredBy != nil {
		return fmt.Errorf("element %s is covered by %s. Close it first.", ref, hit.CoveredBy)
	}
	mouse := p.Context(ctx).Mouse
	if err := mouse.MoveTo(proto.Point{X: hit.X, Y: hit.Y}); err != nil {
		return fmt.Errorf("move mouse: %w", err)
	}
	return mouse.Click(proto.InputMouseButtonLeft, 1)
}

type typeInfo struct {
	Error      string `json:"error"`
	Role       string `json:"role"`
	Password   bool   `json:"password"`
	Disabled   bool   `json:"disabled"`
	Readonly   bool   `json:"readonly"`
	Editable   bool   `json:"editable"`
	HasContent bool   `json:"hasContent"`
	Box        [4]int `json:"box"`
}

// Type foca o campo, limpa (por padrão) e digita o texto.
func (s *Session) Type(ctx context.Context, ref, text string, clearFirst, submit bool) (string, error) {
	rec := &recording{ref: ref, step: func(sel *flow.Selector) flow.Step {
		step := &flow.TypeStep{Selector: *sel, Text: text, Submit: submit}
		if !clearFirst {
			step.Clear = &clearFirst
		}
		return flow.Step{Type: step}
	}}
	return s.action(ctx, rec, func(p *rod.Page) error {
		var info typeInfo
		if err := evalInto(ctx, p, &info, `(r, c) => window.__vitral.prepareType(r, c)`, ref, clearFirst); err != nil {
			return err
		}
		switch {
		case info.Error != "":
			return refError(ref, info.Error)
		case info.Password:
			// "Sua vez" (UX-3): pausa com o campo destacado até a pessoa digitar e clicar em Retomar.
			s.ui.WaitForUser(ux.ReasonPassword, ref, info.Box)
			return pausedResult(fmt.Sprintf(ux.PasswordMessage, ref))
		case info.Disabled:
			return refError(ref, "disabled")
		case info.Readonly:
			return fmt.Errorf("element %s is read-only.", ref)
		case !info.Editable:
			return fmt.Errorf("element %s (%s) is not a text field. Use click or select instead.", ref, info.Role)
		}
		s.ui.Highlight(ctx, p, ref, info.Box)
		pc := p.Context(ctx)
		if clearFirst && info.HasContent {
			if err := pc.Keyboard.Type(input.Backspace); err != nil {
				return fmt.Errorf("clear field: %w", err)
			}
		}
		if text != "" {
			if err := pc.InsertText(text); err != nil {
				return fmt.Errorf("type text: %w", err)
			}
		}
		if submit {
			return pc.Keyboard.Type(input.Enter)
		}
		return nil
	})
}

type selectInfo struct {
	Error     string   `json:"error"`
	Role      string   `json:"role"`
	Label     string   `json:"label"`
	Available []string `json:"available"`
}

// Select escolhe uma opção de <select> pelo label ou value.
func (s *Session) Select(ctx context.Context, ref, option string) (string, error) {
	rec := &recording{ref: ref, step: func(sel *flow.Selector) flow.Step {
		return flow.Step{Select: &flow.SelectStep{Selector: *sel, Option: option}}
	}}
	return s.action(ctx, rec, func(p *rod.Page) error {
		if err := s.revealAndHighlight(ctx, p, ref); err != nil {
			return err
		}
		var info selectInfo
		if err := evalInto(ctx, p, &info, `(r, o) => window.__vitral.selectOption(r, o)`, ref, option); err != nil {
			return err
		}
		switch info.Error {
		case "":
			return nil
		case "not_select":
			return fmt.Errorf("element %s (%s) is not a <select>. Click it and pick the option instead.", ref, info.Role)
		case "no_option":
			return fmt.Errorf("option %q not found in %s. Available: %s", option, ref, strings.Join(quoteAll(info.Available), ", "))
		case "option_disabled":
			return fmt.Errorf("option %q in %s is disabled.", info.Label, ref)
		}
		return refError(ref, info.Error)
	})
}

// revealAndHighlight rola até o alvo (se preciso) e mostra o destaque antes da ação.
func (s *Session) revealAndHighlight(ctx context.Context, p *rod.Page, ref string) error {
	var res struct {
		Error string `json:"error"`
		Box   [4]int `json:"box"`
	}
	if err := evalInto(ctx, p, &res, `(r) => window.__vitral.reveal(r)`, ref); err != nil {
		return err
	}
	if err := refError(ref, res.Error); err != nil {
		return err
	}
	s.ui.Highlight(ctx, p, ref, res.Box)
	return nil
}

func quoteAll(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// Press pressiona uma tecla ou combinação (ex.: Enter, Escape, Control+A).
func (s *Session) Press(ctx context.Context, combo string) (string, error) {
	mods, key, err := ParseKeys(combo)
	if err != nil {
		return "", err
	}
	rec := &recording{step: func(*flow.Selector) flow.Step { return flow.Step{Press: combo} }}
	return s.action(ctx, rec, func(p *rod.Page) error {
		return p.Context(ctx).KeyActions().Press(mods...).Type(key).Do()
	})
}

// ScrollOptions descreve a rolagem: até um elemento ou numa direção.
type ScrollOptions struct {
	Ref       string
	Direction string
	Amount    int
}

// Scroll rola até um elemento ou pela roda do mouse no centro da viewport.
func (s *Session) Scroll(ctx context.Context, opts ScrollOptions) (string, error) {
	if opts.Ref == "" && opts.Direction != "up" && opts.Direction != "down" {
		return "", errors.New("provide ref, or direction \"up\" or \"down\"")
	}
	rec := &recording{ref: opts.Ref, step: func(sel *flow.Selector) flow.Step {
		return flow.Step{Scroll: &flow.ScrollStep{Selector: sel, Direction: opts.Direction, Amount: opts.Amount}}
	}}
	return s.action(ctx, rec, func(p *rod.Page) error {
		if opts.Ref != "" {
			return s.revealAndHighlight(ctx, p, opts.Ref)
		}
		var vp [2]float64
		if err := evalInto(ctx, p, &vp, `() => [window.innerWidth, window.innerHeight]`); err != nil {
			return err
		}
		amount := float64(opts.Amount)
		if amount <= 0 {
			amount = vp[1] * 0.8
		}
		if opts.Direction == "up" {
			amount = -amount
		}
		mouse := p.Context(ctx).Mouse
		if err := mouse.MoveTo(proto.Point{X: vp[0] / 2, Y: vp[1] / 2}); err != nil {
			return err
		}
		return mouse.Scroll(0, amount, 1)
	})
}

// WaitOptions descreve a espera por texto ou elemento.
type WaitOptions struct {
	Text    string
	Ref     string
	Gone    bool
	Timeout time.Duration
}

type textMatch struct {
	Found bool   `json:"found"`
	Ref   string `json:"ref"`
	Role  string `json:"role"`
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// WaitFor espera um texto visível (ou ref) aparecer ou sumir e devolve o snapshot.
func (s *Session) WaitFor(ctx context.Context, opts WaitOptions) (string, error) {
	if opts.Text == "" && opts.Ref == "" {
		return "", errors.New("provide text or ref to wait for")
	}
	recordedTimeout := int(opts.Timeout.Milliseconds())
	if opts.Timeout <= 0 {
		opts.Timeout = s.cfg.StabilityTimeout
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return "", err
	}
	rec := &recording{ref: opts.Ref, step: func(sel *flow.Selector) flow.Step {
		return flow.Step{WaitFor: &flow.WaitStep{Text: opts.Text, Element: sel, Gone: opts.Gone, TimeoutMs: recordedTimeout}}
	}}
	sel, note := s.selectorFor(ctx, p, rec)
	deadline := time.Now().Add(opts.Timeout)
	for {
		match, err := s.probe(ctx, p, opts)
		if err == nil && match.Found != opts.Gone {
			out, err := s.waitResult(ctx, p, opts, match)
			if err != nil {
				return "", err
			}
			return joinNonEmpty(out, s.record(rec, sel, note)), nil
		}
		if time.Now().After(deadline) {
			return "", waitTimeoutError(opts)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (s *Session) probe(ctx context.Context, p *rod.Page, opts WaitOptions) (textMatch, error) {
	var m textMatch
	if opts.Text != "" {
		err := evalInto(ctx, p, &m, `(t) => window.__vitral.findText(t)`, opts.Text)
		return m, err
	}
	js := `(r) => { const el = window.__vitral.find(r); if (!el) return {found: false};
		const r2 = el.getBoundingClientRect();
		const vis = el.checkVisibility ? el.checkVisibility({checkOpacity: true, checkVisibilityCSS: true}) : true;
		return {found: vis && r2.width > 0 && r2.height > 0, ref: r}; }`
	err := evalInto(ctx, p, &m, js, opts.Ref)
	return m, err
}

func (s *Session) waitResult(ctx context.Context, p *rod.Page, opts WaitOptions, m textMatch) (string, error) {
	page, err := capture(ctx, p, snapshot.Options{})
	if err != nil {
		return "", err
	}
	s.last = page
	head := describeWait(opts, m)
	return head + "\n" + snapshot.Format(page, s.format), nil
}

func describeWait(opts WaitOptions, m textMatch) string {
	target := fmt.Sprintf("%q", opts.Text)
	if opts.Text == "" {
		target = opts.Ref
	}
	if opts.Gone {
		return fmt.Sprintf("%s is gone.", target)
	}
	if opts.Text == "" {
		return fmt.Sprintf("%s is visible.", target)
	}
	line := fmt.Sprintf("found %s in [%s] %s %q", target, m.Ref, m.Role, snapshot.Truncate(m.Text, 80))
	if m.Count > 1 {
		line += fmt.Sprintf(" (%d matches, showing the first)", m.Count)
	}
	return line
}

func waitTimeoutError(opts WaitOptions) error {
	target := fmt.Sprintf("text %q", opts.Text)
	if opts.Text == "" {
		target = "element " + opts.Ref
	}
	state := "to appear"
	if opts.Gone {
		state = "to disappear"
	}
	return fmt.Errorf("timeout after %s waiting for %s %s. Call snapshot to see the page.", opts.Timeout, target, state)
}
