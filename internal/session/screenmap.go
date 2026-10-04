package session

import (
	"context"
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
