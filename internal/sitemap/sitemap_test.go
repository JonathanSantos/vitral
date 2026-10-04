package sitemap

import (
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/JonathanSantos/vitral/internal/heal"
)

var t0 = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		"Pedido 10234":                          "Pedido <num>",
		"Pedido 123":                            "Pedido 123",
		"Contato: ana.silva@exemplo.com.br":     "Contato: <email>",
		"CPF 123.456.789-09 e 12345678909":      "CPF <cpf> e <cpf>",
		"CNPJ 12.345.678/0001-95":               "CNPJ <cnpj>",
		"Ligue (11) 98765-4321 ou 11 3456-7890": "Ligue <telefone> ou <telefone>",
		"  Salvar   alterações ":                "Salvar alterações",
	} {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	for in, want := range map[string][2]string{
		"https://Portal.Test/pedidos/10234?aba=itens&q=maria#topo":                   {"portal.test", "/pedidos/:id?aba&q"},
		"http://127.0.0.1:8080/clientes/550e8400-e29b-41d4-a716-446655440000/editar": {"127.0.0.1:8080", "/clientes/:id/editar"},
		"https://p.test/arquivos/9f86d081884c7d65/x":                                 {"p.test", "/arquivos/:id/x"},
		"https://p.test/t/aB3dE5fG7hJ9kL1mN2/":                                       {"p.test", "/t/:id/"},
		"https://p.test/u/ana@exemplo.com":                                           {"p.test", "/u/:id"},
		"https://p.test/v2/pedidos":                                                  {"p.test", "/v2/pedidos"},
		"https://p.test":                                                             {"p.test", "/"},
		"https://p.test/app#/pedidos/123?status=x":                                   {"p.test", "/app#/pedidos/:id?status"},
		"https://p.test/app#!/clientes":                                              {"p.test", "/app#!/clientes"},
		"https://p.test/a?x=1&x=2&&y":                                                {"p.test", "/a?x&y"},
		"https://p.test/a?%zz":                                                       {"p.test", "/a"},
		"https://p.test/a%2Fb/x":                                                     {"p.test", "/a%2Fb/x"},
		"https://p.test/a/%zz":                                                       {"", ""},
		"about:blank":                                                                {"", ""},
		"::":                                                                         {"", ""},
	} {
		host, pattern := NormalizeURL(in)
		if host != want[0] || pattern != want[1] {
			t.Errorf("NormalizeURL(%q) = %q %q, want %q %q", in, host, pattern, want[0], want[1])
		}
	}
}

func TestScreenNameAndObserve(t *testing.T) {
	for _, tt := range []struct{ title, h1, want string }{
		{"Pedidos - Portal", "", "Pedidos"},
		{"Pedidos | Portal", "Lista de pedidos", "Lista de pedidos"},
		{"Portal", "", "Portal"},
		{" - Portal", "", "- Portal"},
		{"", "Pedido 10234", "Pedido <num>"},
	} {
		if got := ScreenName(tt.title, tt.h1); got != tt.want {
			t.Errorf("ScreenName(%q, %q) = %q", tt.title, tt.h1, got)
		}
	}
	v := Observe("https://p.test/pedidos/10234", "Pedido - Portal", "", "Excluir 10234?", []Item{
		{"heading", "Pedido 10234"}, {"button", "Salvar"}, {"button", "salvar"}, {"paragraph", "texto"}, {"textbox", "E-mail"},
	})
	if v.Host != "p.test" || v.URL != "/pedidos/:id" || v.Name != "Pedido" || v.Dialog != "Excluir <num>?" {
		t.Fatalf("%+v", v)
	}
	if len(v.Print) != 3 || v.Print[0] != (Item{"button", "Salvar"}) || v.Print[1] != (Item{"heading", "Pedido <num>"}) {
		t.Fatalf("print: %+v", v.Print)
	}
	many := make([]Item, 300)
	for i := range many {
		many[i] = Item{"button", strings.Repeat("b", i%50+1) + string(rune('a'+i%26))}
	}
	if got := Observe("https://p.test/", "", "", "", many); len(got.Print) != maxPrint {
		t.Fatalf("cap: %d", len(got.Print))
	}
}

func TestJaccard(t *testing.T) {
	a := []Item{{"button", "Salvar"}, {"heading", "Pedidos"}, {"textbox", "Busca"}}
	b := []Item{{"button", "salvar"}, {"heading", "Pedidos"}, {"textbox", "Busca"}, {"button", "Novo"}}
	if got := Jaccard(a, b); got != 0.75 {
		t.Fatalf("%v", got)
	}
	if Jaccard(nil, nil) != 1 || Jaccard(a, nil) != 0 {
		t.Fatal("edges")
	}
}

func visit(url, name string, items ...Item) Visit {
	return Observe("https://p.test"+url, "", name, "", items)
}

var (
	listPrint = []Item{{"heading", "Pedidos"}, {"button", "Novo pedido"}, {"textbox", "Busca"}, {"navigation", "Menu"}}
	formPrint = []Item{{"heading", "Cadastro de pedido"}, {"textbox", "Cliente"}, {"textbox", "Valor"}, {"button", "Salvar"}}
)

func sig(role, name string) *heal.Signature { return &heal.Signature{Role: role, Name: name} }

func TestVisitTransitionAndQueries(t *testing.T) {
	m := New("p.test")
	list := m.Visit(visit("/pedidos", "Pedidos", listPrint...), t0)
	// A query não separa telas ("?criado=1" depois de salvar); outro caminho separa.
	if again := m.Visit(visit("/pedidos?criado=1", "Pedidos", listPrint...), t0); again != list || list.Visits != 2 {
		t.Fatal("a query alone is the same screen")
	}
	if other := m.Visit(visit("/pedidos/arquivados", "Pedidos", listPrint...), t0); other == list {
		t.Fatal("another path is another screen")
	}
	list.Visits = 1
	// Um botão a mais (Jaccard 4/5 = 0,8): a mesma tela.
	same := m.Visit(visit("/pedidos", "Pedidos", append(listPrint[:4:4], Item{"button", "Exportar"})...), t0.Add(time.Minute))
	if same != list || list.Visits != 2 || !list.LastSeen.Equal(t0.Add(time.Minute)) {
		t.Fatalf("Jaccard 0.8 is the same screen: same=%v visits=%d", same == list, list.Visits)
	}
	// Trocar um item por outro (3/5 = 0,6): outra tela.
	if other := m.Visit(visit("/pedidos", "Pedidos", append(listPrint[:3:3], Item{"button", "Importar"})...), t0); other == list {
		t.Fatal("Jaccard 0.6 is another screen")
	}
	// SPA: mesma URL, impressão bem diferente: outra tela.
	form := m.Visit(visit("/pedidos", "Cadastro de pedido", formPrint...), t0)
	if form == list {
		t.Fatal("same URL with a different print is another screen")
	}
	dlg := Observe("https://p.test/pedidos", "", "Pedidos", "Excluir pedido?", listPrint)
	if s := m.Visit(dlg, t0); s != list || len(s.Dialogs) != 1 || s.Dialogs[0] != "Excluir pedido?" {
		t.Fatalf("dialog is a substate: %+v", s)
	}
	m.Visit(dlg, t0)
	if len(list.Dialogs) != 1 {
		t.Fatal("dialog listed once")
	}

	e := m.Transition(list, form, Action{Kind: "click", Target: sig("button", "Novo pedido 10234")}, t0)
	m.Transition(list, form, Action{Kind: "click", Target: sig("button", "Novo pedido 99999")}, t0.Add(time.Hour))
	if len(m.Edges) != 1 || e.Count != 2 || e.Successes != 2 || e.Action.Target.Name != "Novo pedido <num>" {
		t.Fatalf("one edge, sanitized target: %+v", e)
	}
	if len(list.Elements) != 1 || list.Elements[0].Uses != 2 {
		t.Fatalf("catalog: %+v", list.Elements)
	}
	back := m.Transition(form, list, Action{Kind: "back"}, t0)
	m.Transition(form, list, Action{Kind: "press", Key: "Escape"}, t0)
	if back.Action.Label() != "back" || m.Edges[2].Action.Label() != "press Escape" || e.Action.Label() != `click button "Novo pedido <num>"` {
		t.Fatal("labels")
	}
	if got := m.Exits(list.ID, 5); len(got) != 1 || got[0] != e {
		t.Fatalf("exits: %+v", got)
	}

	// Falhas seguidas deixam a aresta suspeita; um sucesso a reabilita.
	for i := 0; i < SuspectAfter; i++ {
		m.Failure(list, Action{Kind: "click", Target: sig("button", "Novo pedido 55555")}, t0)
	}
	if !e.Suspect() || e.Failures != 3 || len(m.Exits(list.ID, 5)) != 0 {
		t.Fatalf("suspect: %+v", e)
	}
	if _, ok := m.Path(list.ID, form.ID, t0); ok {
		t.Fatal("suspect edges are not suggested")
	}
	m.Transition(list, form, Action{Kind: "click", Target: sig("button", "Novo pedido 77777")}, t0)
	if e.Suspect() {
		t.Fatal("a success clears the streak")
	}
	m.Failure(form, Action{Kind: "click", Target: sig("button", "Inexistente")}, t0)

	// Find: telas e elementos pelo nome.
	found := m.Find("novo pedido")
	if len(found) == 0 || found[0].Element == nil || found[0].Screen != list {
		t.Fatalf("find element: %+v", found)
	}
	if f := m.Find("cadastro de pedido"); len(f) == 0 || f[0].Screen != form || f[0].Element != nil {
		t.Fatalf("find screen: %+v", f)
	}
	if len(m.Find("relatório anual")) != 0 {
		t.Fatal("nothing")
	}
	if m.Screen("s99") != nil || m.Screen(list.ID) != list {
		t.Fatal("Screen")
	}
}

func TestPath(t *testing.T) {
	m := New("p.test")
	a := m.Visit(visit("/a", "A", Item{"heading", "A"}), t0)
	b := m.Visit(visit("/b", "B", Item{"heading", "B"}), t0)
	c := m.Visit(visit("/c", "C", Item{"heading", "C"}), t0)
	d := m.Visit(visit("/d", "D", Item{"heading", "D"}), t0)
	click := func(name string) Action { return Action{Kind: "click", Target: sig("link", name)} }
	direct := m.Transition(a, d, click("D direto"), t0)
	m.Transition(a, b, click("B"), t0)
	m.Transition(b, d, click("D"), t0)
	m.Transition(a, c, click("C"), t0)
	m.Transition(c, a, click("A"), t0)
	if p, ok := m.Path(a.ID, d.ID, t0); !ok || len(p) != 1 || p[0] != direct {
		t.Fatalf("short and reliable: %+v", p)
	}
	// O atalho falha muito: o caminho de dois passos confiável ganha.
	direct.Failures, direct.Successes = 5, 1
	if p, ok := m.Path(a.ID, d.ID, t0); !ok || len(p) != 2 || p[0].To != b.ID {
		t.Fatalf("reliable path: %+v", p)
	}
	// Recência: uma aresta velha pesa mais que uma nova igual.
	direct.Failures, direct.Successes = 0, 1
	m.Edges[1].LastUsed = t0.Add(-80 * 24 * time.Hour)
	if weight(direct, t0) >= weight(m.Edges[1], t0) {
		t.Fatal("older edges cost more")
	}
	if p, ok := m.Path(a.ID, a.ID, t0); !ok || p != nil {
		t.Fatal("same screen")
	}
	if _, ok := m.Path("s99", "s99", t0); ok {
		t.Fatal("unknown screen")
	}
	if _, ok := m.Path(d.ID, a.ID, t0); ok {
		t.Fatal("no way back from D")
	}
	m.Transition(d, d, click("Recarregar"), t0)
	if _, ok := m.Path(d.ID, a.ID, t0); ok {
		t.Fatal("self loops lead nowhere")
	}
}

func TestPrune(t *testing.T) {
	m := New("p.test")
	old := m.Visit(visit("/velha", "Velha", Item{"heading", "V"}), t0.Add(-100*24*time.Hour))
	cur := m.Visit(visit("/nova", "Nova", Item{"heading", "N"}), t0)
	other := m.Visit(visit("/outra", "Outra", Item{"heading", "O"}), t0)
	m.Transition(cur, old, Action{Kind: "click", Target: sig("link", "Velha")}, t0)
	stale := m.Transition(cur, other, Action{Kind: "click", Target: sig("link", "Outra")}, t0.Add(-91*24*time.Hour))
	fresh := m.Transition(other, cur, Action{Kind: "click", Target: sig("link", "Nova")}, t0.Add(-89*24*time.Hour))
	m.Prune(t0)
	if len(m.Screens) != 2 || m.Screen(old.ID) != nil || len(m.Edges) != 1 || m.Edges[0] != fresh || stale == fresh {
		t.Fatalf("%+v %+v", m.Screens, m.Edges)
	}
}

func TestSanitizeSig(t *testing.T) {
	s := SanitizeSig(heal.Signature{
		Role: "link", Name: "Pedido 10234", Heading: "Cliente ana@x.com", Href: "/pedidos/10234/itens", Label: "CPF 123.456.789-09",
		Column: "Tel", RowHeader: "(11) 98765-4321", Placeholder: "0000", Landmark: "region 2026", Dialog: "Pedido 5555", TestID: "row-12345", NameAttr: "n1234", ID: "x-9999",
	})
	joined := strings.Join([]string{s.Name, s.Heading, s.Href, s.Label, s.RowHeader, s.Placeholder, s.Landmark, s.Dialog, s.TestID, s.NameAttr, s.ID}, "|")
	if regexp.MustCompile(`\d{4}|@`).MatchString(joined) || s.Href != "/pedidos/:id/itens" {
		t.Fatalf("%s", joined)
	}
	if SanitizeSig(heal.Signature{Href: "#secao"}).Href != "#secao" {
		t.Fatal("anchor hrefs stay")
	}
}

// Propriedades: Sanitize não deixa 4 dígitos seguidos nem e-mail e é idempotente; Jaccard é
// simétrica e fica entre 0 e 1; todo caminho é contínuo, sem arestas suspeitas, de from até to.
func TestProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.StringMatching(`[a-zA-Z0-9 @.()/+-]{0,40}`).Draw(t, "s")
		out := Sanitize(s)
		if regexp.MustCompile(`\d{4}`).MatchString(out) || Sanitize(out) != out {
			t.Fatalf("Sanitize(%q) = %q", s, out)
		}
	})
	rapid.Check(t, func(t *rapid.T) {
		gen := func(l string) []Item {
			n := rapid.IntRange(0, 6).Draw(t, l)
			var out []Item
			for i := 0; i < n; i++ {
				out = append(out, Item{rapid.SampledFrom([]string{"button", "heading"}).Draw(t, l+"r"), rapid.SampledFrom([]string{"A", "B", "C", "D"}).Draw(t, l+"n")})
			}
			return out
		}
		a, b := gen("a"), gen("b")
		j := Jaccard(a, b)
		if j != Jaccard(b, a) || j < 0 || j > 1 || math.IsNaN(j) {
			t.Fatalf("%v", j)
		}
	})
	rapid.Check(t, func(t *rapid.T) {
		m := New("p.test")
		n := rapid.IntRange(1, 6).Draw(t, "screens")
		var screens []*Screen
		for i := 0; i < n; i++ {
			name := string(rune('A' + i))
			screens = append(screens, m.Visit(visit("/"+name, name, Item{"heading", name}), t0))
		}
		edges := rapid.IntRange(0, 15).Draw(t, "edges")
		for i := 0; i < edges; i++ {
			from := screens[rapid.IntRange(0, n-1).Draw(t, "f")]
			to := screens[rapid.IntRange(0, n-1).Draw(t, "t")]
			e := m.Transition(from, to, Action{Kind: "click", Target: sig("link", to.Name+from.Name)}, t0)
			e.Failures = rapid.IntRange(0, 4).Draw(t, "fail")
			e.Streak = rapid.IntRange(0, 4).Draw(t, "streak")
		}
		from, to := screens[0], screens[n-1]
		path, ok := m.Path(from.ID, to.ID, t0)
		if !ok {
			return
		}
		at := from.ID
		for _, e := range path {
			if e.From != at || e.Suspect() {
				t.Fatalf("broken path at %s: %+v", at, e)
			}
			at = e.To
		}
		if at != to.ID {
			t.Fatalf("path ends at %s, want %s", at, to.ID)
		}
	})
}

func TestRender(t *testing.T) {
	m := New("p.test")
	list := m.Visit(Observe("https://p.test/pedidos", "", "Lista de pedidos", "Excluir?", listPrint), t0)
	form := m.Visit(visit("/novo", "Cadastro de pedido", formPrint...), t0)
	m.Transition(list, form, Action{Kind: "click", Target: sig("button", "Novo pedido")}, t0)
	bad := m.Transition(form, list, Action{Kind: "click", Target: sig("link", `Voltar "já"`)}, t0)
	bad.Streak = SuspectAfter
	m.Transition(form, form, Action{Kind: "type", Target: sig("textbox", "Cliente")}, t0)
	m.Transition(list, form, Action{Kind: "press", Key: "Enter"}, t0)
	text := m.Text()
	for _, want := range []string{
		"p.test: 2 telas, 4 transições", `[s1] "Lista de pedidos"  /pedidos  (visitas: 1)`, `impressão: button "Novo pedido"`,
		"dialogs: Excluir?", `elemento: button "Novo pedido" (usos: 1)`, `→ click button "Novo pedido" ⇒ "Cadastro de pedido" (1 ok, 0 falhas)`, "[suspeita]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	mm := m.Mermaid()
	for _, want := range []string{"graph LR", `s1["Lista de pedidos<br/><small>/pedidos</small>"]`, `s1 -->|"click button 'Novo pedido'"| s2`, `s2 -.->|"click link 'Voltar 'já''"| s1`, `s1 -->|"press Enter"| s2`} {
		if !strings.Contains(mm, want) {
			t.Errorf("missing %q in:\n%s", want, mm)
		}
	}
	if strings.Contains(mm, "s2 -->|\"type") {
		t.Error("self loops are not drawn")
	}
}

func TestRouteOf(t *testing.T) {
	for in, want := range map[string]string{"/a?x": "/a", "/a": "/a", "/app#/p/:id?s": "/app#/p/:id", "/app?x#/q": "/app#/q"} {
		if got := routeOf(in); got != want {
			t.Errorf("routeOf(%q) = %q", in, got)
		}
	}
}

func TestLineFindScreenAndNavigable(t *testing.T) {
	m := New("p.test")
	list := m.Visit(visit("/pedidos", "Lista de pedidos", listPrint...), t0)
	form := m.Visit(visit("/novo", "Cadastro de pedido", formPrint...), t0)
	detail := m.Visit(visit("/pedido/1", "Detalhe do pedido", Item{"heading", "Detalhe"}), t0)
	if got := m.Line(list, 5); got != `screen: "Lista de pedidos" (known)` {
		t.Fatal(got)
	}
	m.Transition(list, form, Action{Kind: "click", Target: sig("button", "Novo pedido")}, t0)
	m.Transition(list, detail, Action{Kind: "press", Key: "Enter"}, t0)
	m.Transition(list, detail, Action{Kind: "type", Target: sig("searchbox", "Busca"), Key: "Enter"}, t0)
	m.Transition(form, list, Action{Kind: "back"}, t0)
	got := m.Line(list, 2)
	if !strings.HasPrefix(got, `screen: "Lista de pedidos" (known) · exits: `) || strings.Count(got, "→") != 2 || !strings.Contains(got, `"Novo pedido" → "Cadastro de pedido"`) {
		t.Fatal(got)
	}
	if !strings.Contains(m.Line(form, 5), `"back" → "Lista de pedidos"`) || !strings.Contains(m.Line(list, 5), `"Enter" → "Detalhe do pedido"`) {
		t.Fatal(m.Line(form, 5), m.Line(list, 5))
	}
	if (Action{Kind: "click", Target: sig("button", "")}).ExitLabel() != "click" {
		t.Fatal("unnamed target")
	}
	// Só navegação: o caminho por "type" (que pede um valor) não serve ao goto.
	for _, e := range m.Edges {
		if e.Action.Kind == "press" {
			e.Streak = SuspectAfter
		}
	}
	if _, ok := m.PathWith(list.ID, detail.ID, t0, Navigable); ok {
		t.Fatal("only a type edge is left")
	}
	if p, ok := m.Path(list.ID, detail.ID, t0); !ok || p[0].Action.Kind != "type" {
		t.Fatal("without the filter the type edge is used")
	}
	if !Navigable(&Edge{Action: Action{Kind: "back"}}) || Navigable(&Edge{Action: Action{Kind: "select"}}) {
		t.Fatal("Navigable")
	}
	// FindScreen: por nome; no empate, a mais visitada.
	other := m.Visit(visit("/pedidos/arquivo", "Lista de pedidos", Item{"heading", "Arquivo"}), t0)
	m.Visit(visit("/pedidos/arquivo", "Lista de pedidos", Item{"heading", "Arquivo"}), t0)
	found := m.FindScreen("lista de pedidos")
	if len(found) != 2 || found[0] != other || found[1] != list {
		t.Fatalf("%+v", found)
	}
	if got := m.FindScreen("cadastro"); len(got) != 1 || got[0] != form {
		t.Fatalf("%+v", got)
	}
	if len(m.FindScreen("financeiro")) != 0 {
		t.Fatal("none")
	}
}

func TestGlobalMenuEdges(t *testing.T) {
	m := New("p.test")
	menu := Item{"navigation", "Menu"}
	list := m.Visit(visit("/pedidos", "Pedidos", menu, Item{"heading", "Pedidos"}), t0)
	detail := m.Visit(visit("/pedido", "Pedido", menu, Item{"heading", "Pedido"}), t0)
	clients := m.Visit(visit("/clientes", "Clientes", menu, Item{"heading", "Clientes"}), t0)
	lonely := m.Visit(visit("/login", "Entrar", Item{"heading", "Entrar"}), t0)
	navClients := &heal.Signature{Role: "link", Name: "Clientes", Landmark: "navigation Menu"}
	m.Transition(list, detail, Action{Kind: "click", Target: sig("link", "Ver detalhes")}, t0)
	m.Transition(detail, clients, Action{Kind: "click", Target: navClients}, t0)
	m.Transition(lonely, list, Action{Kind: "click", Target: sig("button", "Entrar")}, t0)

	// O link "Clientes" do menu, usado na tela do pedido, vale na lista (mesmo menu): 1 passo.
	p, ok := m.Path(list.ID, clients.ID, t0)
	if !ok || len(p) != 1 || !p[0].Global || p[0].From != list.ID || p[0].Action.Target.Name != "Clientes" {
		t.Fatalf("%+v", p)
	}
	if len(m.Edges) != 3 {
		t.Fatal("virtual edges are not stored")
	}
	// A tela sem o menu não ganha o atalho: primeiro "Entrar" (visto ali), depois o menu da lista.
	if p, ok := m.Path(lonely.ID, clients.ID, t0); !ok || len(p) != 2 || p[0].Global || !p[1].Global {
		t.Fatalf("lonely: %+v", p)
	}
	// Um caminho visto na própria tela, com o mesmo custo, ganha do atalho do menu.
	direct := m.Transition(list, clients, Action{Kind: "click", Target: sig("button", "Ver clientes")}, t0)
	if p, _ := m.Path(list.ID, clients.ID, t0); len(p) != 1 || p[0] != direct {
		t.Fatalf("observed wins: %+v", p)
	}
	// O mesmo link já visto na própria tela não vira atalho repetido; e a saída para a própria tela não conta.
	m.Transition(list, clients, Action{Kind: "click", Target: navClients}, t0)
	for _, e := range m.outgoing(list.ID) {
		if e.Global && e.To == clients.ID {
			t.Fatalf("duplicate global edge: %+v", e)
		}
	}
	for _, e := range m.outgoing(clients.ID) {
		if e.Global && e.To == clients.ID {
			t.Fatal("a menu link to the same screen is not an exit")
		}
	}
	if m.outgoing("s99") != nil {
		t.Fatal("unknown screen")
	}
	// Só regiões globais: um link de uma região comum não vira atalho.
	region := &heal.Signature{Role: "link", Name: "Pedido", Landmark: "region Resumo"}
	m.Transition(clients, detail, Action{Kind: "click", Target: region}, t0)
	if !globalLandmark("banner") || globalLandmark("region Resumo") || globalLandmark("") {
		t.Fatal("globalLandmark")
	}
}
