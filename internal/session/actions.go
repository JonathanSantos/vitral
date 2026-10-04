package session

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/cursor"
	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/overlay"
	"github.com/JonathanSantos/vitral/internal/security"
	"github.com/JonathanSantos/vitral/internal/sitemap"
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
	if s.cur.on {
		// "Abrindo {host}" no selo e o cursor a 40% até a página carregar (5.6).
		host := target
		if u, err := url.Parse(target); err == nil && u.Host != "" {
			host = u.Host
		}
		s.ui.NavStart(ctx, fmt.Sprintf(ux.TextOpening, host))
		defer s.ui.NavEnd(ctx)
	}
	if err := s.navigate(ctx, p, target); err != nil {
		return "", err
	}
	warning := s.waitStable(ctx, p, start)
	page, err := s.capture(ctx, snapshot.Options{})
	if err != nil {
		return "", fmt.Errorf("page opened, but snapshot failed: %w. Call snapshot.", err)
	}
	s.last = page
	rec := &recording{step: func(*flow.Selector) flow.Step { return flow.Step{Open: target} }}
	recorded := s.record(rec, nil, "")
	s.learn(ctx, sitemap.Visit{}, false, nil, false)
	verify, err := s.verificationGate(ctx)
	if err != nil {
		return "", err
	}
	return joinNonEmpty(snapshot.Format(page, s.format), s.events(netMark, conMark), warning, recorded, verify), nil
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
	if s.cur.on {
		s.ui.NavStart(ctx, ux.TextGoingBack)
		defer s.ui.NavEnd(ctx)
	}
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
	Kind      string   `json:"kind"`
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
		if s.cur.on {
			return s.clickWithCursor(ctx, p, ref)
		}
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

// clickWithCursor é o clique com o cursor visual (5.1): rolagem suave, ponto validado, movimento,
// mira e o evento real no mesmo ponto final que a pessoa viu.
func (s *Session) clickWithCursor(ctx context.Context, p *rod.Page, ref string) error {
	if err := s.scrollIntoBand(ctx, p, ref); err != nil {
		return err
	}
	var hit hitInfo
	if err := s.callRef(ctx, &hit, ref, `(el) => window.__vitral.hit(el, true)`); err != nil {
		return err
	}
	if hit.Error == "offscreen" { // a pessoa rolou para longe no meio: traz de volta sem animar
		if err := s.callRef(ctx, &hit, ref, `(el) => window.__vitral.hit(el)`); err != nil {
			return err
		}
	}
	if err := refError(ref, hit.Error); err != nil {
		return err
	}
	kind := kindOf(hit.Kind)
	if hit.Disabled {
		s.failAt(ctx, hit.Box, kind, ux.TextDisabled)
		return refError(ref, "disabled")
	}
	plan, ok, err := s.approach(ctx, ref, hit.Box, kind)
	if err != nil {
		return err
	}
	if ok {
		return pressAt(ctx, p, plan)
	}
	if hit.coveredByOverlay() {
		// Só o selo do Vitral cobre o alvo: esconde o selo durante o clique, como antes.
		return s.ui.Hidden(ctx, p, func() error {
			hit, err := s.hit(ctx, p, ref)
			if err != nil {
				return err
			}
			return clickAt(ctx, p, ref, hit)
		})
	}
	s.failAt(ctx, hit.Box, kind, ux.TextCovered)
	if hit.CoveredBy != nil {
		return fmt.Errorf("element %s is covered by %s. Close it first.", ref, hit.CoveredBy)
	}
	return fmt.Errorf("element %s has no free spot to click: every point hits another interactive element inside it. Click that element instead, or call snapshot.", ref)
}

// hit rola até o elemento e confere se ele pode receber o clique.
func (s *Session) hit(ctx context.Context, p *rod.Page, ref string) (hitInfo, error) {
	var hit hitInfo
	if err := s.callRef(ctx, &hit, ref, `(el) => window.__vitral.hit(el)`); err != nil {
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
	Code       bool   `json:"code"`
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
		if s.cur.on {
			if err := s.scrollIntoBand(ctx, p, ref); err != nil {
				return err
			}
		}
		var info typeInfo
		if err := s.callRef(ctx, &info, ref, `(el, c, n, k) => window.__vitral.prepareType(el, c, n, k)`, clearFirst, s.cur.on, verification.Code); err != nil {
			return err
		}
		switch {
		case info.Error != "":
			return refError(ref, info.Error)
		case info.Password:
			// "Sua vez" (UX-3): pausa com o campo destacado até a pessoa digitar e clicar em Retomar.
			if s.cur.on {
				s.besideAndHide(ctx, info.Box)
			}
			s.ui.WaitForUser(ux.ReasonPassword, ref, info.Box)
			return pausedResult(fmt.Sprintf(ux.PasswordMessage, ref))
		case info.Code:
			// Código de uso único (2FA): a IA nunca digita; sem janela, ninguém digita (4.3).
			if _, err := handOff(s.cfg.Headless, "a one-time code (2FA)"); err != nil {
				return err
			}
			if s.cur.on {
				s.besideAndHide(ctx, info.Box)
			}
			s.ui.WaitForUser(ux.ReasonCode, ref, info.Box)
			return pausedResult(fmt.Sprintf(ux.CodeMessage, ref))
		case info.Disabled:
			if s.cur.on {
				s.failAt(ctx, info.Box, cursor.Text, ux.TextDisabled)
			}
			return refError(ref, "disabled")
		case info.Readonly:
			return fmt.Errorf("element %s is read-only.", ref)
		case !info.Editable:
			return fmt.Errorf("element %s (%s) is not a text field. Use click or select instead.", ref, info.Role)
		}
		if s.cur.on {
			// Vai até o campo e clica como uma pessoa (5.2). O clique move o cursor de texto: prepara de
			// novo (foco e seleção para limpar) e tira a mão de cima do texto.
			plan, ok, err := s.approach(ctx, ref, info.Box, cursor.Text)
			if err != nil {
				return err
			}
			if ok {
				if err := pressAt(ctx, p, plan); err != nil {
					return err
				}
				if err := s.callRef(ctx, &info, ref, `(el, c, k) => window.__vitral.prepareType(el, c, true, k)`, clearFirst, verification.Code); err != nil {
					return err
				}
				s.awayFrom(ctx, plan.End)
			}
		} else {
			s.ui.Highlight(ctx, p, ref, info.Box)
		}
		pc := p.Context(ctx)
		if clearFirst && info.HasContent {
			if err := pc.Keyboard.Type(input.Backspace); err != nil {
				return fmt.Errorf("clear field: %w", err)
			}
		}
		if text != "" {
			if err := s.typeText(ctx, p, text); err != nil {
				return fmt.Errorf("type text: %w", err)
			}
		}
		if submit {
			if s.cur.on {
				_ = s.ui.Keycap(ctx, "Enter")
			}
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
		if s.cur.on {
			// Vai até o select e clica só no visual: um clique real abriria a lista nativa (5.3).
			box, kind, err := s.revealWithCursor(ctx, p, ref)
			if err != nil {
				return err
			}
			if _, _, err := s.approach(ctx, ref, box, kind); err != nil {
				return err
			}
		} else if err := s.revealAndHighlight(ctx, p, ref); err != nil {
			return err
		}
		var info selectInfo
		if err := s.callRef(ctx, &info, ref, `(el, o) => window.__vitral.selectOption(el, o)`, option); err != nil {
			return err
		}
		switch info.Error {
		case "":
			if s.cur.on {
				_ = s.ui.CursorNote(ctx, fmt.Sprintf(ux.TextSelected, info.Label), 800*time.Millisecond)
			}
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

// revealWithCursor rola suavemente até o alvo (cursor parado) e devolve a caixa e o tipo de cursor.
func (s *Session) revealWithCursor(ctx context.Context, p *rod.Page, ref string) ([4]int, cursor.Kind, error) {
	if err := s.scrollIntoBand(ctx, p, ref); err != nil {
		return [4]int{}, cursor.Arrow, err
	}
	var res struct {
		Error string `json:"error"`
		Box   [4]int `json:"box"`
		Kind  string `json:"kind"`
	}
	if err := s.callRef(ctx, &res, ref, `(el) => window.__vitral.reveal(el)`); err != nil {
		return res.Box, cursor.Arrow, err
	}
	return res.Box, kindOf(res.Kind), refError(ref, res.Error)
}

// revealAndHighlight rola até o alvo (se preciso) e mostra o destaque antes da ação.
func (s *Session) revealAndHighlight(ctx context.Context, p *rod.Page, ref string) error {
	var res struct {
		Error string `json:"error"`
		Box   [4]int `json:"box"`
	}
	if err := s.callRef(ctx, &res, ref, `(el) => window.__vitral.reveal(el)`); err != nil {
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
		if s.cur.on {
			_ = s.ui.Keycap(ctx, keyLabel(combo))
		}
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
			if !s.cur.on {
				return s.revealAndHighlight(ctx, p, opts.Ref)
			}
			// Rola até o alvo com o cursor parado e depois leva o cursor até ele (5.4).
			box, kind, err := s.revealWithCursor(ctx, p, opts.Ref)
			if err != nil {
				return err
			}
			t, ok, err := s.aim(ctx, opts.Ref, box, kind)
			if err != nil {
				return err
			}
			if !ok {
				t.to = rectOf(box).Center()
			}
			s.travel(ctx, t)
			return nil
		}
		var vp [2]float64
		if err := s.evalInto(ctx, &vp, `() => [window.innerWidth, window.innerHeight]`); err != nil {
			return err
		}
		amount := float64(opts.Amount)
		if amount <= 0 {
			amount = vp[1] * 0.8
		}
		if opts.Direction == "up" {
			amount = -amount
		}
		if s.cur.on {
			// O cursor vai ao centro da viewport e rola em passos de roda visíveis.
			center := cursor.Point{X: math.Round(vp[0] / 2), Y: math.Round(vp[1] / 2)}
			s.travelTo(ctx, center, [4]int{int(center.X) - 20, int(center.Y) - 20, 40, 40}, cursor.Arrow)
			return s.scrollBy(ctx, p, center, amount)
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
		err := s.evalInto(ctx, &m, `(t) => window.__vitral.findText(t)`, opts.Text)
		return m, err
	}
	js := `(el, r) => { if (!el || !el.isConnected) return {found: false};
		const r2 = el.getBoundingClientRect();
		const vis = el.checkVisibility ? el.checkVisibility({checkOpacity: true, checkVisibilityCSS: true}) : true;
		return {found: vis && r2.width > 0 && r2.height > 0, ref: r}; }`
	err := s.callRef(ctx, &m, opts.Ref, js, opts.Ref)
	return m, err
}

func (s *Session) waitResult(ctx context.Context, p *rod.Page, opts WaitOptions, m textMatch) (string, error) {
	page, err := s.capture(ctx, snapshot.Options{})
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
