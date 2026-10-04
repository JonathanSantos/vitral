package session

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/heal"
	"github.com/JonathanSantos/vitral/internal/mapstore"
	"github.com/JonathanSantos/vitral/internal/sitemap"
)

// Mapa de telas (pedido 5): aprende só com ações que aconteceram, observando a tela antes e depois de
// cada uma (a impressão estrutural, pelo script do mundo isolado: papéis e nomes, nunca valores).
// Nunca muda o resultado de uma ação: erros do mapa só vão para o log.

type screenMap struct {
	saver *mapstore.Saver
	maps  map[string]*sitemap.Map // por host (carregados sob demanda)
	hint  atomic.Pointer[heal.Signature]
}

func newScreenMap(dir string, onError func(error)) *screenMap {
	return &screenMap{
		saver: &mapstore.Saver{Store: mapstore.Store{Dir: dir}, Delay: 2 * time.Second, OnError: onError},
		maps:  map[string]*sitemap.Map{},
	}
}

func (m *screenMap) load(host string) *sitemap.Map {
	if mp, ok := m.maps[host]; ok {
		return mp
	}
	mp, err := m.saver.Store.Load(host)
	if err != nil {
		if m.saver.OnError != nil {
			m.saver.OnError(err)
		}
		mp = sitemap.New(host)
	}
	m.maps[host] = mp
	return mp
}

// Map devolve o mapa do host como está agora (nil sem mapa; para testes e diagnóstico).
func (s *Session) Map(host string) *sitemap.Map {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.smap == nil {
		return nil
	}
	return s.smap.load(host)
}

// CloseMap grava o que estiver pendente.
func (s *Session) CloseMap() {
	if s.smap != nil {
		s.smap.saver.Flush()
	}
}

type printResult struct {
	URL    string         `json:"url"`
	Title  string         `json:"title"`
	H1     string         `json:"h1"`
	Dialog string         `json:"dialog"`
	Items  []sitemap.Item `json:"items"`
}

// observe lê a tela atual (com s.mu travado). ok=false sem mapa ou fora de http(s).
func (s *Session) observe(ctx context.Context) (sitemap.Visit, bool) {
	if s.smap == nil {
		return sitemap.Visit{}, false
	}
	var r printResult
	if err := s.evalInto(ctx, &r, `() => window.__vitral.screenPrint()`); err != nil {
		return sitemap.Visit{}, false
	}
	v := sitemap.Observe(r.URL, r.Title, r.H1, r.Dialog, r.Items)
	return v, v.Host != ""
}

// mapAction é a ação da gravação para o mapa: o tipo, o alvo (a assinatura do seletor, ou a do passo
// do fluxo em execução) e a tecla.
func (s *Session) mapAction(rec *recording, sel *flow.Selector) *sitemap.Action {
	if rec == nil || s.smap == nil {
		return nil
	}
	st := rec.step(&flow.Selector{})
	a := &sitemap.Action{Kind: st.Kind()}
	switch {
	case sel != nil && sel.Sig != nil:
		a.Target = sel.Sig
	case rec.ref != "":
		a.Target = s.smap.hint.Load()
	}
	switch {
	case st.Press != "":
		a.Key = st.Press
	case st.Type != nil && st.Type.Submit:
		a.Key = "Enter"
	}
	return a
}

// learn registra o que aconteceu: a tela de destino (sempre) e a transição a partir da de origem
// (quando as duas são do mesmo host), ou uma falha da ação.
func (s *Session) learn(ctx context.Context, from sitemap.Visit, fromOK bool, a *sitemap.Action, failed bool) {
	if s.smap == nil {
		return
	}
	now := time.Now()
	if failed {
		if !fromOK || a == nil {
			return
		}
		mp := s.smap.load(from.Host)
		if src := mp.Locate(from); src != nil {
			mp.Failure(src, *a, now)
			s.smap.saver.Schedule(mp)
		}
		return
	}
	to, ok := s.observe(ctx)
	if !ok {
		return
	}
	mp := s.smap.load(to.Host)
	if fromOK && from.Host == to.Host && a != nil {
		src := mp.Locate(from)
		if src == nil {
			src = mp.Visit(from, now)
		}
		dst := mp.Visit(to, now)
		mp.Transition(src, dst, *a, now)
	} else {
		mp.Visit(to, now)
	}
	s.smap.saver.Schedule(mp)
}

// screenLine é a linha "screen:" de uma tela já conhecida (visitada antes ou com saídas): o nome e as
// até 5 saídas mais usadas. Só lê o mapa. Com s.mu travado.
func (s *Session) screenLine(ctx context.Context) string {
	if s.smap == nil {
		return ""
	}
	v, ok := s.observe(ctx)
	if !ok {
		return ""
	}
	mp := s.smap.load(v.Host)
	sc := mp.Locate(v)
	if sc == nil || sc.Visits < 2 && len(mp.Exits(sc.ID, 1)) == 0 {
		return ""
	}
	return mp.Line(sc, 5)
}

// withScreenLine põe a linha "screen:" logo depois da linha "page:" do snapshot.
func withScreenLine(snap, line string) string {
	if line == "" {
		return snap
	}
	first, rest, ok := strings.Cut(snap, "\n")
	if !ok {
		return snap + "\n" + line
	}
	return first + "\n" + line + "\n" + rest
}

// MapFind procura telas e elementos conhecidos no mapa do site atual (map_find) e mostra o caminho a
// partir da tela atual.
func (s *Session) MapFind(ctx context.Context, text string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.smap == nil {
		return "", errMapOff
	}
	if _, err := s.page(); err != nil {
		return "", err
	}
	cur, ok := s.observe(ctx)
	if !ok {
		return "", errNoSite
	}
	mp := s.smap.load(cur.Host)
	src := mp.Locate(cur)
	matches := mp.Find(text)
	if len(matches) == 0 {
		return fmt.Sprintf("%q is not in the map of %s yet (%d known screens). The map only knows what was done before; look on the page with snapshot.", text, cur.Host, len(mp.Screens)), nil
	}
	lines := []string{fmt.Sprintf("%q in the map of %s:", text, cur.Host)}
	for i, m := range matches {
		if i == 5 {
			break
		}
		what := fmt.Sprintf("screen %q", m.Screen.Name)
		if m.Element != nil {
			what = fmt.Sprintf("%s %q on screen %q", m.Element.Sig.Role, m.Element.Sig.Name, m.Screen.Name)
		}
		lines = append(lines, "- "+what+": "+describePath(mp, src, m.Screen))
	}
	return strings.Join(append(lines, "To go there, call goto with the screen name."), "\n"), nil
}

func describePath(mp *sitemap.Map, src, dst *sitemap.Screen) string {
	switch {
	case src == nil:
		return "no known way from the current screen (it is not in the map yet)"
	case src.ID == dst.ID:
		return "you are on it"
	}
	path, ok := mp.PathWith(src.ID, dst.ID, time.Now(), sitemap.Navigable)
	if !ok {
		return "no known way from here"
	}
	steps := make([]string, len(path))
	for i, e := range path {
		steps[i] = e.Action.Label()
	}
	return fmt.Sprintf("%d step(s) from here: %s", len(path), strings.Join(steps, " → "))
}

var (
	errMapOff = errors.New("the screen map is off (--no-map)")
	errNoSite = errors.New("open a page of the site first: the map is per site")
)

// Goto vai até a tela pelo caminho mais confiável do mapa, executado como um fluxo (com autocura).
// Nunca atravessa um passo sensível sem a pessoa concordar (confirm). Em falha, para no passo,
// marca a aresta e devolve o relatório do fluxo.
func (s *Session) Goto(ctx context.Context, screen string, confirm bool, addMock MockAdder) (string, error) {
	s.mu.Lock()
	if s.smap == nil {
		s.mu.Unlock()
		return "", errMapOff
	}
	if _, err := s.page(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	cur, ok := s.observe(ctx)
	if !ok {
		s.mu.Unlock()
		return "", errNoSite
	}
	mp := s.smap.load(cur.Host)
	src := mp.Locate(cur)
	dests := mp.FindScreen(screen)
	var path []*sitemap.Edge
	var dst *sitemap.Screen
	if len(dests) > 0 {
		dst = dests[0]
	}
	if src != nil && dst != nil {
		path, ok = mp.PathWith(src.ID, dst.ID, time.Now(), sitemap.Navigable)
	}
	known := screenNames(mp)
	ambiguous := ""
	if len(dests) > 1 && heal.Normalize(dests[0].Name) != heal.Normalize(screen) &&
		heal.Similarity(screen, dests[0].Name) == heal.Similarity(screen, dests[1].Name) {
		var names []string
		for _, d := range dests {
			names = append(names, strconv.Quote(d.Name))
		}
		ambiguous = strings.Join(names, ", ")
	}
	s.mu.Unlock()
	if ambiguous != "" {
		return "", fmt.Errorf("%q matches several screens: %s. Call goto with the full name", screen, ambiguous)
	}

	switch {
	case dst == nil:
		return "", fmt.Errorf("no screen named %q in the map of %s. Known screens: %s", screen, cur.Host, known)
	case src == nil:
		return "", errors.New("the current screen is not in the map yet: navigate by hand once (the map learns from what is done), or open a known screen first")
	case src.ID == dst.ID:
		return fmt.Sprintf("already on screen %q.", dst.Name), nil
	case !ok:
		return "", fmt.Errorf("no known way from %q to %q yet: navigate there by hand once and goto will learn it", src.Name, dst.Name)
	}
	var sensitive []string
	steps := make([]flow.Step, len(path))
	for i, e := range path {
		steps[i] = gotoStep(e.Action)
		if t := e.Action.Target; t != nil && heal.Sensitive(t.Name) {
			sensitive = append(sensitive, fmt.Sprintf("step %d: %s", i+1, e.Action.Label()))
		}
	}
	if len(sensitive) > 0 && !confirm {
		return fmt.Sprintf("The way from %q to %q goes through a sensitive action (%s). Ask the user before going; if they agree, call goto again with confirm=true.",
			src.Name, dst.Name, strings.Join(sensitive, "; ")), nil
	}
	out, err := s.RunFlow(ctx, flow.Flow{Name: "goto", Steps: steps}, nil, addMock, RunOptions{})
	head := fmt.Sprintf("goto %q from %q: %d steps (the most reliable known way)", dst.Name, src.Name, len(path))
	var ferr *FlowError
	if errors.As(err, &ferr) {
		s.mu.Lock()
		if ferr.Step >= 1 && ferr.Step <= len(path) {
			e := path[ferr.Step-1]
			if from := mp.Screen(e.From); from != nil {
				mp.Failure(from, e.Action, time.Now())
				s.smap.saver.Schedule(mp)
			}
		}
		s.mu.Unlock()
		return "", &FlowError{Report: replaceFirstLine(ferr.Report, head), Step: ferr.Step}
	}
	if err != nil {
		return "", err
	}
	// Chegou? Se a tela final não é a pedida, avisa e marca a última aresta.
	s.mu.Lock()
	end, endOK := s.observe(ctx)
	arrived := endOK && mp.Locate(end) == dst
	if !arrived {
		last := path[len(path)-1]
		if from := mp.Screen(last.From); from != nil {
			mp.Failure(from, last.Action, time.Now())
			s.smap.saver.Schedule(mp)
		}
	}
	s.mu.Unlock()
	if !arrived {
		head += fmt.Sprintf(". Warning: the steps ran, but the page does not look like %q now; check the page below", dst.Name)
	}
	return replaceFirstLine(out, head), nil
}

func gotoStep(a sitemap.Action) flow.Step {
	switch a.Kind {
	case "press":
		return flow.Step{Press: a.Key}
	case "back":
		return flow.Step{Back: true}
	}
	sig := *a.Target
	return flow.Step{Click: &flow.Selector{Role: sig.Role, Name: sig.Name, Sig: &sig}}
}

func replaceFirstLine(text, line string) string {
	if _, rest, ok := strings.Cut(text, "\n"); ok {
		return line + "\n" + rest
	}
	return line
}

func screenNames(mp *sitemap.Map) string {
	names := make([]string, 0, len(mp.Screens))
	for _, sc := range mp.Screens {
		names = append(names, strconv.Quote(sc.Name))
	}
	if len(names) == 0 {
		return "none yet"
	}
	return strings.Join(names, ", ")
}
