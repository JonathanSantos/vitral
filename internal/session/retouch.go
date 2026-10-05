package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/go-rod/rod"

	"github.com/JonathanSantos/vitral/internal/retouch"
	"github.com/JonathanSantos/vitral/internal/screenshot"
)

// Retoques (docs/RETOQUES.md): a prévia é um <style> posto pelo mundo isolado do Vitral só na aba
// atual; o save grava o retoque, recompila a extensão gerada e a recarrega no navegador aberto.

// retouchState é a última prévia (vale até ser salva ou descartada) e a última prévia de save.
type retouchState struct {
	css     string
	pageURL string // página onde a prévia foi feita: o escopo sai dela
	save    *RetouchSaveInput
}

// RetouchSaveInput são os parâmetros do retouch_save.
type RetouchSaveInput struct {
	Name           string
	Description    string
	Scope          string
	Preview        bool
	IgnoreWarnings bool
}

// RetouchResult é a resposta de uma prévia: o texto e, quando pedidos, os prints de antes e depois.
type RetouchResult struct {
	Text   string
	Before []byte
	After  []byte
}

// EnableRetouches liga os Retoques na sessão (sem isso as ferramentas não existem).
func (s *Session) EnableRetouches(b retouch.Builder) { s.retouches = &b }

var errNoRetouches = errors.New("retouches are off (--no-retouches)")

// previewJS aplica a prévia (idempotente) e conta, para cada regra, quantos elementos ela pega agora.
const previewJS = `(css) => {
  const root = document.documentElement;
  if (!root) return {error: 'this page has no document element'};
  const P = window.__vitralPreview || (window.__vitralPreview = (() => {
    const st = {el: null};
    const apply = () => { if (st.el) st.el.media = root.hasAttribute('data-vitral-original') ? 'not all' : 'all'; };
    new MutationObserver(apply).observe(root, {attributes: true, attributeFilter: ['data-vitral-original']});
    return {
      set(text) {
        if (!st.el) { st.el = document.createElement('style'); st.el.dataset.vitralPreview = ''; }
        st.el.textContent = text;
        if (!st.el.isConnected) (document.head || root).append(st.el);
        apply();
      },
      clear() { if (st.el) st.el.remove(); st.el = null; },
      active() { return !!(st.el && st.el.isConnected); },
    };
  })());
  if (css === null) { P.clear(); return {rules: []}; }
  const sheet = new CSSStyleSheet();
  try { sheet.replaceSync(css); } catch (e) { return {error: String(e.message || e)}; }
  const rules = [];
  const pseudo = /::?(before|after|placeholder|selection|marker|first-line|first-letter|backdrop|file-selector-button|hover|focus|focus-visible|focus-within|active|visited)\b/g;
  const walk = (list) => { for (const r of list) {
    if (r.selectorText !== undefined) {
      let n = -1;
      try { n = document.querySelectorAll(r.selectorText.replace(pseudo, '') || '*').length; } catch (_) {}
      rules.push([r.selectorText, n]);
    } else if (r.cssRules) walk(r.cssRules);
  } };
  walk(sheet.cssRules);
  if (!rules.length) return {error: 'no CSS rule could be read'};
  P.set(css);
  return {rules};
}`

type previewResult struct {
	Rules [][2]any `json:"rules"`
	Error string   `json:"error"`
}

// RetouchPreview aplica o CSS só na aba atual e diz o que cada regra pegou.
func (s *Session) RetouchPreview(ctx context.Context, css string, shots bool) (RetouchResult, error) {
	if s.retouches == nil {
		return RetouchResult{}, errNoRetouches
	}
	if strings.TrimSpace(css) == "" {
		return RetouchResult{}, errors.New("css is empty: pass the CSS rules to apply")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return RetouchResult{}, err
	}
	pageURL := s.pageURL()
	if _, err := retouch.MatchesFor(retouch.ScopeSite, pageURL); err != nil {
		return RetouchResult{}, err
	}
	var res previewResult
	if err := s.evalInto(ctx, &res, previewJS, css); err != nil {
		return RetouchResult{}, err
	}
	if res.Error != "" {
		return RetouchResult{}, fmt.Errorf("CSS not applied: %s. Fix the CSS and call retouch_preview again", res.Error)
	}
	s.retouch = &retouchState{css: css, pageURL: pageURL}
	var b strings.Builder
	b.WriteString("retouch preview applied on this tab only (the site is not changed; a reload removes it):\n")
	b.WriteString(describeRules(res.Rules))
	if w := retouch.CheckCSS(css, siteMatches(pageURL)); len(w) > 0 {
		b.WriteString("warnings:\n- " + strings.Join(w, "\n- ") + "\n")
	}
	b.WriteString("Check the result (snapshot or screenshot), show the person and ask whether to keep it; to keep it, call retouch_save with preview=true.")
	out := RetouchResult{Text: b.String()}
	if shots {
		if out.Before, out.After, err = s.beforeAfter(ctx, p); err != nil {
			return RetouchResult{}, err
		}
	}
	return out, nil
}

func siteMatches(pageURL string) []string {
	m, _ := retouch.MatchesFor(retouch.ScopeSite, pageURL)
	return m
}

func describeRules(rules [][2]any) string {
	var b strings.Builder
	for _, r := range rules {
		sel, _ := r[0].(string)
		n, _ := r[1].(float64)
		switch {
		case n < 0:
			fmt.Fprintf(&b, "- %q: could not count (selector not supported by querySelectorAll)\n", sel)
		case n == 0:
			fmt.Fprintf(&b, "- %q: 0 elements (matches nothing on this page)\n", sel)
		case n == 1:
			fmt.Fprintf(&b, "- %q: 1 element\n", sel)
		default:
			fmt.Fprintf(&b, "- %q: %d elements\n", sel, int(n))
		}
	}
	return b.String()
}

// beforeAfter tira o print sem os retoques ("Ver original") e com eles, sem selo nem cursor.
func (s *Session) beforeAfter(ctx context.Context, p *rod.Page) (before, after []byte, err error) {
	shot := func(original bool) ([]byte, error) {
		js := `(on) => { document.documentElement.toggleAttribute('data-vitral-original', on); return true; }`
		var ok bool
		if err := s.evalInto(ctx, &ok, js, original); err != nil {
			return nil, err
		}
		time.Sleep(80 * time.Millisecond) // um quadro para o estilo novo pintar
		var img image.Image
		err := s.ui.Hidden(ctx, p, func() error {
			var err error
			_, img, _, err = s.captureImage(ctx, p, false)
			return err
		})
		if err != nil {
			return nil, err
		}
		return screenshot.Encode(img)
	}
	if before, err = shot(true); err != nil {
		return nil, nil, err
	}
	after, err = shot(false)
	return before, after, err
}

// RetouchDiscard tira a prévia da aba.
func (s *Session) RetouchDiscard(ctx context.Context) (string, error) {
	if s.retouches == nil {
		return "", errNoRetouches
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.page(); err != nil {
		return "", err
	}
	var res previewResult
	if err := s.evalInto(ctx, &res, previewJS, nil); err != nil {
		return "", err
	}
	had := s.retouch != nil
	s.retouch = nil
	if !had {
		return "no retouch preview to discard", nil
	}
	return "retouch preview removed from this tab", nil
}

// RetouchSave grava a última prévia como retoque. Com preview=true só mostra o que seria salvo (com os
// prints de antes e depois); depois, o save só com o nome salva exatamente aquela prévia.
func (s *Session) RetouchSave(ctx context.Context, in RetouchSaveInput) (RetouchResult, error) {
	if s.retouches == nil {
		return RetouchResult{}, errNoRetouches
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.retouch
	if st == nil {
		return RetouchResult{}, errors.New("no retouch preview: call retouch_preview with the CSS first, show the person, then save")
	}
	if !in.Preview && st.save != nil && st.save.Name == in.Name && in.Description == "" && in.Scope == "" {
		keep := *st.save
		keep.Preview, keep.IgnoreWarnings = false, in.IgnoreWarnings
		in = keep
	}
	if in.Scope == "" {
		in.Scope = string(retouch.ScopeSite)
	}
	matches, err := retouch.MatchesFor(retouch.Scope(in.Scope), st.pageURL)
	if err != nil {
		return RetouchResult{}, err
	}
	r := retouch.Retouch{Name: in.Name, Description: in.Description, Scope: retouch.Scope(in.Scope), Matches: matches, CSS: st.css}
	if err := r.Validate(); err != nil {
		return RetouchResult{}, err
	}
	warnings := retouch.CheckCSS(st.css, matches)
	if in.Preview {
		p, err := s.page()
		if err != nil {
			return RetouchResult{}, err
		}
		keep := in
		st.save = &keep
		var b strings.Builder
		fmt.Fprintf(&b, "retouch %q (preview, not saved yet):\n", r.Name)
		fmt.Fprintf(&b, "- where: %s (%s)\n", scopeWords(r.Scope, st.pageURL), strings.Join(matches, ", "))
		if r.Description != "" {
			fmt.Fprintf(&b, "- description: %s\n", r.Description)
		}
		fmt.Fprintf(&b, "- CSS: %d characters\n", len(r.CSS))
		if len(warnings) > 0 {
			b.WriteString("warnings (a save is refused until fixed, or ignoreWarnings=true if the person confirms):\n- " + strings.Join(warnings, "\n- ") + "\n")
		}
		fmt.Fprintf(&b, "Show the person the before and after pictures and where it applies. Only after a clear yes, call retouch_save with only name=%q to save exactly this.", r.Name)
		out := RetouchResult{Text: b.String()}
		if out.Before, out.After, err = s.beforeAfter(ctx, p); err != nil {
			return RetouchResult{}, err
		}
		return out, nil
	}
	if len(warnings) > 0 && !in.IgnoreWarnings {
		return RetouchResult{}, fmt.Errorf("retouch not saved:\n- %s", strings.Join(warnings, "\n- "))
	}
	replaced, err := s.retouches.Store.Save(r)
	if err != nil {
		return RetouchResult{}, err
	}
	if _, err := s.retouches.Build(); err != nil {
		return RetouchResult{}, fmt.Errorf("retouch saved, but the browser extension could not be built: %w", err)
	}
	loadErr := s.browser.LoadExtension(s.retouches.Dir())
	st.save = nil
	var b strings.Builder
	verb := "saved"
	if replaced {
		verb = "updated (the previous version is in .history)"
	}
	fmt.Fprintf(&b, "retouch %q %s: it applies on %s every time it opens in this browser, also with vitral open.\n", r.Name, verb, scopeWords(r.Scope, st.pageURL))
	b.WriteString("The preview stays on this tab until it reloads; from the next page load on, the saved retouch applies by itself.\n")
	if loadErr != nil {
		fmt.Fprintf(&b, "Note: the browser did not load the retouch now (%v); it applies from the next start.\n", loadErr)
	}
	b.WriteString("Tell the person: it changes only this browser; to turn it off, use the Vitral button in the browser toolbar.")
	return RetouchResult{Text: b.String()}, nil
}

func scopeWords(scope retouch.Scope, pageURL string) string {
	m := siteMatches(pageURL)
	host := ""
	if len(m) > 0 {
		host = strings.TrimSuffix(strings.TrimPrefix(m[0], "*://"), "/*")
	}
	switch scope {
	case retouch.ScopePage:
		return "this page of " + host
	case retouch.ScopeAll:
		return "every site"
	}
	return "every page of " + host
}

// retouchesHere são os retoques aplicados na página atual pela extensão: nome → ligado.
const retouchesHereJS = `() => {
  const out = {};
  document.querySelectorAll('style[data-vitral-retoque]').forEach((s) => { out[s.dataset.vitralRetoque] = !s.hasAttribute('data-vitral-off'); });
  return {here: out, original: document.documentElement.hasAttribute('data-vitral-original'), preview: !!(window.__vitralPreview && window.__vitralPreview.active())};
}`

type hereState struct {
	Here     map[string]bool `json:"here"`
	Original bool            `json:"original"`
	Preview  bool            `json:"preview"`
}

// RetouchList lista os retoques salvos e o estado de cada um na página atual.
func (s *Session) RetouchList(ctx context.Context) (string, error) {
	if s.retouches == nil {
		return "", errNoRetouches
	}
	list, bad, err := s.retouches.Store.List()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pageURL, here := "", hereState{}
	if s.current != nil {
		pageURL = s.pageURL()
		_ = s.evalInto(ctx, &here, retouchesHereJS)
	}
	var b strings.Builder
	if len(list) == 0 {
		b.WriteString("no saved retouches\n")
	} else {
		fmt.Fprintf(&b, "saved retouches (%d):\n", len(list))
	}
	for _, r := range list {
		fmt.Fprintf(&b, "- %s", r.Name)
		if r.Description != "" {
			fmt.Fprintf(&b, ": %q", r.Description)
		}
		fmt.Fprintf(&b, " · %s", strings.Join(r.Matches, ", "))
		on, loaded := here.Here[r.Name]
		switch {
		case loaded && on && here.Original:
			b.WriteString(" · on here, but the page is showing the original")
		case loaded && on:
			b.WriteString(" · on here")
		case loaded:
			b.WriteString(" · off here (turned off in the Vitral button)")
		case pageURL != "" && r.Applies(pageURL):
			b.WriteString(" · applies here from the next page load")
		}
		b.WriteString("\n")
	}
	for name, err := range bad {
		fmt.Fprintf(&b, "- %s: broken, not loaded (%v)\n", name, err)
	}
	if here.Preview {
		b.WriteString("this tab also has an unsaved retouch preview\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// RetouchToggle liga ou desliga um retoque salvo (o mesmo interruptor do botão do Vitral na barra).
// O pedido vai para a extensão por um atributo no <html>: o único ponto em comum entre o mundo isolado
// do Vitral e o da extensão.
func (s *Session) RetouchToggle(ctx context.Context, name string, on bool) (string, error) {
	if s.retouches == nil {
		return "", errNoRetouches
	}
	if _, err := s.retouches.Store.Load(name); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.page(); err != nil {
		return "", err
	}
	var here hereState
	if err := s.evalInto(ctx, &here, retouchesHereJS); err != nil {
		return "", err
	}
	if _, ok := here.Here[name]; !ok {
		return "", fmt.Errorf("retouch %q is not active on this page: open a page where it applies (retouch_list shows where), then call again", name)
	}
	cmd, _ := json.Marshal(map[string]any{"name": name, "on": on})
	js := `(cmd) => { document.documentElement.setAttribute('data-vitral-retouch-cmd', cmd); return true; }`
	var ok bool
	if err := s.evalInto(ctx, &ok, js, string(cmd)); err != nil {
		return "", err
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := s.evalInto(ctx, &here, retouchesHereJS); err == nil && here.Here[name] == on {
			state := "off"
			if on {
				state = "on"
			}
			return fmt.Sprintf("retouch %q is %s on every page where it applies (the person can switch it back with the Vitral button in the browser toolbar)", name, state), nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", fmt.Errorf("the retouch extension did not answer; reload the page and call again")
}
