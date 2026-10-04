// Package sitemap é o mapa de telas (pedido "próximos passos", seção 5): o Vitral aprende a estrutura
// do sistema com as ações que de fato aconteceram. Nó = tela (padrão de URL + impressão estrutural),
// aresta = transição observada, catálogo = elementos conhecidos de cada tela. Puro e determinístico:
// atualização e consultas são funções; a persistência fica fora (internal/mapstore). Nunca substitui
// a leitura da página: só acrescenta nome da tela e caminhos conhecidos.
package sitemap

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/JonathanSantos/vitral/internal/heal"
)

// Version é a versão do formato do arquivo.
const Version = 1

// Limiares (pedido 5.2 e 5.4).
const (
	SameScreen   = 0.7                 // Jaccard mínima entre impressões da mesma tela
	SuspectAfter = 3                   // falhas seguidas que tiram a aresta das sugestões
	MaxAge       = 90 * 24 * time.Hour // tela ou aresta não vista há mais tempo sai no carregamento
)

// Item é um elemento da impressão estrutural: papel e nome (já sem dados).
type Item struct {
	Role string `json:"role"`
	Name string `json:"name,omitempty"`
}

func (i Item) key() string { return i.Role + "\x00" + heal.Normalize(i.Name) }

// Visit é uma observação de tela: o que o snapshot viu, já reduzido a estrutura.
type Visit struct {
	Host   string
	URL    string // padrão (NormalizeURL)
	Name   string // título ou h1, sem dados
	Dialog string // nome do dialog aberto (subestado), se houver
	Print  []Item
}

// Element é um elemento conhecido de uma tela (catálogo), com a assinatura da autocura.
type Element struct {
	Sig  heal.Signature `json:"signature"`
	Uses int            `json:"uses"`
}

// Screen é uma tela.
type Screen struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	URL      string     `json:"url"`
	Print    []Item     `json:"print"`
	Dialogs  []string   `json:"dialogs,omitempty"`
	Elements []*Element `json:"elements,omitempty"`
	Visits   int        `json:"visits"`
	LastSeen time.Time  `json:"lastSeen"`
}

// Action é a ação de uma transição: o tipo (click, type, select, press, back) e o alvo.
type Action struct {
	Kind   string          `json:"kind"`
	Target *heal.Signature `json:"target,omitempty"`
	Key    string          `json:"key,omitempty"` // press: a tecla; type: se enviou (Enter)
}

// Label é a descrição curta da ação: click button "Novo pedido".
func (a Action) Label() string {
	if a.Target == nil {
		if a.Key != "" {
			return a.Kind + " " + a.Key
		}
		return a.Kind
	}
	return fmt.Sprintf("%s %s %q", a.Kind, a.Target.Role, a.Target.Name)
}

func (a Action) same(b Action) bool {
	if a.Kind != b.Kind || a.Key != b.Key || (a.Target == nil) != (b.Target == nil) {
		return false
	}
	return a.Target == nil || a.Target.Role == b.Target.Role && heal.Normalize(a.Target.Name) == heal.Normalize(b.Target.Name)
}

// Edge é uma transição observada entre telas.
type Edge struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Action    Action    `json:"action"`
	Count     int       `json:"count"`
	Successes int       `json:"successes"`
	Failures  int       `json:"failures"`
	Streak    int       `json:"failStreak,omitempty"` // falhas seguidas
	LastUsed  time.Time `json:"lastUsed"`
	// Global marca, nos caminhos, um link de menu global aprendido em outra tela (não é gravado).
	Global bool `json:"-"`
}

// Suspect diz se a aresta saiu das sugestões (3 falhas seguidas, até um sucesso).
func (e *Edge) Suspect() bool { return e.Streak >= SuspectAfter }

// Map é o mapa de um host.
type Map struct {
	Version int       `json:"version"`
	Host    string    `json:"host"`
	Screens []*Screen `json:"screens"`
	Edges   []*Edge   `json:"edges"`
	Next    int       `json:"next"`
}

// New cria o mapa vazio de um host.
func New(host string) *Map { return &Map{Version: Version, Host: host} }

// Jaccard é a similaridade entre duas impressões (conjuntos de papel + nome).
func Jaccard(a, b []Item) float64 {
	sa, sb := map[string]bool{}, map[string]bool{}
	for _, i := range a {
		sa[i.key()] = true
	}
	for _, i := range b {
		sb[i.key()] = true
	}
	if len(sa) == 0 && len(sb) == 0 {
		return 1
	}
	inter := 0
	for k := range sa {
		if sb[k] {
			inter++
		}
	}
	return float64(inter) / float64(len(sa)+len(sb)-inter)
}

// Screen devolve a tela pelo id (nil se não existe).
func (m *Map) Screen(id string) *Screen {
	for _, s := range m.Screens {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// Locate acha a tela da visita: o mesmo caminho no padrão de URL e Jaccard ≥ 0,7 (a mais parecida).
// A query não separa telas (só as chaves ficam no padrão, e um "?criado=1" depois de salvar não é
// outra tela); a impressão é que separa. Numa SPA, a mesma URL com impressões diferentes são telas
// diferentes.
func (m *Map) Locate(v Visit) *Screen {
	var best *Screen
	bestScore := -1.0
	for _, s := range m.Screens {
		if routeOf(s.URL) != routeOf(v.URL) {
			continue
		}
		if j := Jaccard(s.Print, v.Print); j >= SameScreen && j > bestScore {
			best, bestScore = s, j
		}
	}
	return best
}

// Visit registra a visita e devolve a tela (criada se nova). A impressão e o nome seguem a última
// visita; o dialog aberto entra como subestado.
func (m *Map) Visit(v Visit, now time.Time) *Screen {
	s := m.Locate(v)
	if s == nil {
		m.Next++
		s = &Screen{ID: fmt.Sprintf("s%d", m.Next), URL: v.URL}
		m.Screens = append(m.Screens, s)
	}
	s.Name, s.Print = v.Name, slices.Clone(v.Print)
	s.Visits++
	s.LastSeen = now
	if v.Dialog != "" && !slices.Contains(s.Dialogs, v.Dialog) {
		s.Dialogs = append(s.Dialogs, v.Dialog)
	}
	return s
}

// Transition registra uma ação bem-sucedida de from para to (telas já visitadas) e põe o alvo no
// catálogo de from.
func (m *Map) Transition(from, to *Screen, a Action, now time.Time) *Edge {
	if a.Target != nil {
		sig := SanitizeSig(*a.Target)
		a.Target = &sig
		from.learn(*a.Target)
	}
	e := m.edge(from.ID, to.ID, a)
	if e == nil {
		e = &Edge{From: from.ID, To: to.ID, Action: a}
		m.Edges = append(m.Edges, e)
	}
	e.Count++
	e.Successes++
	e.Streak = 0
	e.LastUsed = now
	return e
}

// Failure marca uma falha da ação a partir de from (a aresta conhecida com essa ação, se houver).
func (m *Map) Failure(from *Screen, a Action, now time.Time) {
	if a.Target != nil {
		sig := SanitizeSig(*a.Target)
		a.Target = &sig
	}
	for _, e := range m.Edges {
		if e.From == from.ID && e.Action.same(a) {
			e.Count++
			e.Failures++
			e.Streak++
			e.LastUsed = now
		}
	}
}

func (m *Map) edge(from, to string, a Action) *Edge {
	for _, e := range m.Edges {
		if e.From == from && e.To == to && e.Action.same(a) {
			return e
		}
	}
	return nil
}

func (s *Screen) learn(sig heal.Signature) {
	for _, el := range s.Elements {
		if el.Sig.Role == sig.Role && heal.Normalize(el.Sig.Name) == heal.Normalize(sig.Name) {
			el.Sig = sig
			el.Uses++
			return
		}
	}
	s.Elements = append(s.Elements, &Element{Sig: sig, Uses: 1})
}

// Exits são as saídas mais usadas da tela (sem as suspeitas), no máximo n.
func (m *Map) Exits(id string, n int) []*Edge {
	var out []*Edge
	for _, e := range m.Edges {
		if e.From == id && e.To != id && !e.Suspect() {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Successes != out[j].Successes {
			return out[i].Successes > out[j].Successes
		}
		return out[i].LastUsed.After(out[j].LastUsed)
	})
	return out[:min(n, len(out))]
}

// Prune tira o que não é visto há mais de 90 dias (telas, arestas e arestas para telas removidas).
func (m *Map) Prune(now time.Time) {
	keep := map[string]bool{}
	screens := m.Screens[:0]
	for _, s := range m.Screens {
		if now.Sub(s.LastSeen) <= MaxAge {
			screens = append(screens, s)
			keep[s.ID] = true
		}
	}
	m.Screens = screens
	edges := m.Edges[:0]
	for _, e := range m.Edges {
		if keep[e.From] && keep[e.To] && now.Sub(e.LastUsed) <= MaxAge {
			edges = append(edges, e)
		}
	}
	m.Edges = edges
}

// Match é um resultado do Find: a tela e, se for um elemento dela, o elemento.
type Match struct {
	Screen  *Screen
	Element *Element
	Score   float64
}

// Find procura telas e elementos conhecidos cujo nome bata com o texto (palavras, sem acento),
// do mais parecido para o menos.
func (m *Map) Find(text string) []Match {
	var out []Match
	for _, s := range m.Screens {
		if sc := heal.Similarity(text, s.Name); sc >= 0.5 {
			out = append(out, Match{Screen: s, Score: sc})
		}
		for _, el := range s.Elements {
			if sc := heal.Similarity(text, el.Sig.Name); sc >= 0.5 {
				out = append(out, Match{Screen: s, Element: el, Score: sc})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// ScreenName é o nome da tela: o h1, ou o título sem o sufixo do site ("Pedidos - Portal"), sem dados.
func ScreenName(title, h1 string) string {
	name := strings.TrimSpace(h1)
	if name == "" {
		name = strings.TrimSpace(title)
		for _, sep := range []string{" | ", " - ", " – ", " — ", " · "} {
			if before, _, ok := strings.Cut(name, sep); ok && before != "" {
				name = before
				break
			}
		}
	}
	return Sanitize(name)
}

// Landmarks e controles que entram na impressão (pedido 5.2): regiões, títulos, campos e botões.
var printRoles = map[string]bool{
	"banner": true, "navigation": true, "main": true, "complementary": true, "contentinfo": true, "form": true,
	"region": true, "search": true, "heading": true, "textbox": true, "searchbox": true, "combobox": true,
	"listbox": true, "checkbox": true, "radio": true, "switch": true, "spinbutton": true, "slider": true,
	"button": true, "tab": true,
}

// maxPrint limita a impressão (páginas enormes não viram arquivos enormes).
const maxPrint = 120

// Observe reduz o que a página mostra a uma visita: padrão de URL, nome da tela, dialog e a impressão
// (papel + nome, sem dados, sem repetir, ordenada). Itens de papéis fora da impressão são ignorados.
func Observe(rawURL, title, h1, dialog string, items []Item) Visit {
	host, pattern := NormalizeURL(rawURL)
	v := Visit{Host: host, URL: pattern, Name: ScreenName(title, h1), Dialog: Sanitize(dialog)}
	seen := map[string]bool{}
	for _, it := range items {
		if !printRoles[it.Role] {
			continue
		}
		it.Name = Sanitize(it.Name)
		if k := it.key(); !seen[k] {
			seen[k] = true
			v.Print = append(v.Print, it)
		}
	}
	sort.Slice(v.Print, func(i, j int) bool { return v.Print[i].key() < v.Print[j].key() })
	if len(v.Print) > maxPrint {
		v.Print = v.Print[:maxPrint]
	}
	return v
}

// routeOf é o padrão sem a query (o fragmento de SPA fica: "#/pedidos/:id" é rota).
func routeOf(pattern string) string {
	path, frag, hasFrag := strings.Cut(pattern, "#")
	path, _, _ = strings.Cut(path, "?")
	if hasFrag {
		frag, _, _ = strings.Cut(frag, "?")
		return path + "#" + frag
	}
	return path
}

// FindScreen acha as telas pelo nome (palavras, sem acento), da mais parecida para a menos; no
// empate, a mais visitada.
func (m *Map) FindScreen(name string) []*Screen {
	type scored struct {
		s     *Screen
		score float64
	}
	var list []scored
	for _, s := range m.Screens {
		if sc := heal.Similarity(name, s.Name); sc >= 0.5 {
			list = append(list, scored{s, sc})
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].s.Visits > list[j].s.Visits
	})
	out := make([]*Screen, len(list))
	for i, l := range list {
		out[i] = l.s
	}
	return out
}

// ExitLabel é o nome curto de uma saída: o nome do alvo, a tecla ou a ação.
func (a Action) ExitLabel() string {
	switch {
	case a.Target != nil && a.Target.Name != "":
		return a.Target.Name
	case a.Key != "":
		return a.Key
	}
	return a.Kind
}

// Line é a linha "screen:" do snapshot (pedido 5.3): o nome da tela conhecida e até n saídas mais
// usadas: screen: "Lista de pedidos" (known) · exits: "Novo pedido" → "Cadastro de pedido".
func (m *Map) Line(s *Screen, n int) string {
	line := fmt.Sprintf("screen: %q (known)", s.Name)
	var exits []string
	for _, e := range m.Exits(s.ID, n) {
		if to := m.Screen(e.To); to != nil {
			exits = append(exits, fmt.Sprintf("%q → %q", e.Action.ExitLabel(), to.Name))
		}
	}
	if len(exits) > 0 {
		line += " · exits: " + strings.Join(exits, ", ")
	}
	return line
}
