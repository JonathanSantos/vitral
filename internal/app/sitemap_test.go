//go:build integration

package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/mapstore"
)

// Mapa de telas (pedido 5.6): navegar pelo mini-sistema e conferir o grafo aprendido, a privacidade
// (o arquivo não tem nenhum dado pessoal da tela nem o que foi digitado), o caminho e a busca.
func TestScreenMapMiniSystem(t *testing.T) {
	var mapsDir string
	hh, cleanup := newHarness(func(c *config.Config) { mapsDir = c.MapsDir })
	defer cleanup()
	click := func(snap, role, name string) string {
		t.Helper()
		return hh.ok(t, "click", map[string]any{"ref": ref(t, snap, role, name)})
	}
	snap := hh.ok(t, "open", map[string]any{"url": hh.url("/pages/mini/index.html")})
	click(snap, "link", "Pedidos")
	click(hh.ok(t, "snapshot", nil), "button", "Novo pedido")
	snap = hh.ok(t, "snapshot", nil)
	hh.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Cliente"), "text": "Ana Prado"})
	hh.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Valor"), "text": "350"})
	click(snap, "button", "Enviar pedido")
	click(hh.ok(t, "snapshot", nil), "link", "Ver detalhes")
	snap = hh.ok(t, "snapshot", nil)
	click(snap, "button", "Exportar CSV")
	click(snap, "link", "Voltar para pedidos")
	click(hh.ok(t, "snapshot", nil), "link", "Clientes")
	snap = hh.ok(t, "snapshot", nil)
	click(snap, "button", "Novo cliente")
	hh.ok(t, "press", map[string]any{"key": "Escape"})
	click(hh.ok(t, "snapshot", nil), "link", "Início")

	hh.app.Session.CloseMap()
	host := strings.TrimPrefix(hh.site.URL, "http://")
	file := mapstore.Store{Dir: mapsDir}.Path(host)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Fatalf("map file mode %v", info.Mode().Perm())
	}
	// Privacidade: nada da tela que seja dado pessoal, nem o que foi digitado, nem valores de query.
	for _, secret := range []string{
		"Maria Silva", "João Souza", "123.456.789-09", "987.654.321-00", "maria.silva@exemplo.com",
		"joao@exemplo.com", "98765-4321", "3456-7890", "suporte@miniERP", "10234", "10235", "Ana Prado", "350", "Bem-vinda",
	} {
		if strings.Contains(string(data), secret) {
			t.Errorf("the map file has %q", secret)
		}
	}
	m := hh.app.Session.Map(host)
	names := map[string]string{}
	for _, s := range m.Screens {
		names[s.Name] = s.ID
	}
	for _, want := range []string{"Painel", "Lista de pedidos", "Cadastro de pedido", "Pedido <num>", "Clientes"} {
		if names[want] == "" {
			t.Errorf("missing screen %q in:\n%s", want, m.Text())
		}
	}
	if len(m.Screens) != 5 {
		t.Errorf("want 5 screens:\n%s", m.Text())
	}
	if !regexp.MustCompile(`"url": "/pages/mini/pedido.html\?id"`).Match(data) {
		t.Error("query keys only")
	}
	// Caminho de Clientes até o detalhe de um pedido, e a busca de "Exportar CSV".
	path, ok := m.Path(names["Clientes"], names["Pedido <num>"], time.Now())
	if !ok || len(path) == 0 {
		t.Fatalf("no path:\n%s", m.Text())
	}
	var steps []string
	for _, e := range path {
		steps = append(steps, e.Action.Label())
	}
	found := m.Find("exportar csv")
	if len(found) == 0 || found[0].Screen.Name != "Pedido <num>" || found[0].Element == nil {
		t.Fatalf("find: %+v", found)
	}
	// A linha "screen:" (a tela atual é o Painel), o map_find e o goto.
	mustContain(t, hh.ok(t, "snapshot", nil), `screen: "Painel" (known) · exits: "Pedidos" → "Lista de pedidos"`)
	mustContain(t, hh.ok(t, "map_find", map[string]any{"text": "exportar csv"}),
		`button "Exportar CSV" on screen "Pedido <num>": 2 step(s) from here: click link "Pedidos" → click link "Ver detalhes"`, "call goto")
	mustContain(t, hh.ok(t, "map_find", map[string]any{"text": "relatório financeiro"}), "is not in the map of")
	hh.fail(t, "goto", map[string]any{"screen": "Financeiro"}, `no screen named "Financeiro"`)
	mustContain(t, hh.ok(t, "goto", map[string]any{"screen": "Painel"}), `already on screen "Painel"`)
	hh.fail(t, "goto", map[string]any{"screen": "pedido"}, `"pedido" matches several screens: `)
	goOut := hh.ok(t, "goto", map[string]any{"screen": "Pedido <num>"})
	mustContain(t, goOut, `goto "Pedido <num>" from "Painel": 2 steps`, "done: 2/2 steps passed.", `heading "Pedido 10234"`)
	if strings.Contains(goOut, "Warning") {
		t.Fatalf("goto should arrive:\n%s", goOut)
	}
	// O link "Início" do menu, usado na tela de Clientes, vale no pedido (mesmo menu): 1 passo.
	mustContain(t, hh.ok(t, "goto", map[string]any{"screen": "Painel"}), `goto "Painel" from "Pedido <num>": 1 steps`, "done: 1/1")
	// Passo sensível: o caminho do cadastro para a lista passa por "Enviar pedido".
	snap = hh.ok(t, "open", map[string]any{"url": hh.url("/pages/mini/novo-pedido.html")})
	hh.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Cliente"), "text": "Bia"})
	mustContain(t, hh.ok(t, "goto", map[string]any{"screen": "Lista de pedidos"}),
		`goes through a sensitive action (step 1: click button "Enviar pedido")`, "confirm=true")
	if got := hh.mainWorld(t, `() => ({u: location.pathname})`)["u"]; got != "/pages/mini/novo-pedido.html" {
		t.Fatalf("goto must not run before the user agrees: %v", got)
	}
	mustContain(t, hh.ok(t, "goto", map[string]any{"screen": "Lista de pedidos", "confirm": true}), "done: 1/1", `status "Pedido criado"`)

	// Link do menu usado em outra tela: do cadastro (mesmo menu) até Clientes em 1 passo, sem desvio.
	hh.ok(t, "open", map[string]any{"url": hh.url("/pages/mini/novo-pedido.html")})
	mustContain(t, hh.ok(t, "goto", map[string]any{"screen": "Clientes"}), `goto "Clientes" from "Cadastro de pedido": 1 steps`, "done: 1/1", `heading "Clientes"`)

	// Tokens para "ir até o detalhe de um pedido" saindo de Clientes: à mão (ler a tela e clicar) e com o mapa.
	estimate := func(s string) int { return (len(s) + 3) / 4 }
	snap = hh.ok(t, "open", map[string]any{"url": hh.url("/pages/mini/clientes.html")})
	manual, manualCalls := estimate(snap), 1
	snap = hh.ok(t, "click", map[string]any{"ref": ref(t, snap, "link", "Pedidos")})
	// Clique que leva a uma tela conhecida traz a mesma linha "screen:" do open.
	mustContain(t, snap, `screen: "Lista de pedidos" (known)`)
	manual += estimate(snap)
	snap = hh.ok(t, "snapshot", nil)
	manual += estimate(snap)
	snap = hh.ok(t, "click", map[string]any{"ref": ref(t, snap, "link", "Ver detalhes")})
	manual += estimate(snap)
	manualCalls += 3
	hh.ok(t, "open", map[string]any{"url": hh.url("/pages/mini/clientes.html")})
	withMap := hh.ok(t, "goto", map[string]any{"screen": "Pedido <num>"})
	mustContain(t, withMap, `heading "Pedido 10234"`)
	tokenReport := fmt.Sprintf("Ir de Clientes até o detalhe de um pedido: à mão %d chamadas e ~%d tokens; com goto 1 chamada e ~%d tokens.", manualCalls, manual, estimate(withMap))
	t.Log(tokenReport)

	out := filepath.Join("..", "..", ".quality", "out", "sitemap")
	_ = os.MkdirAll(out, 0o750)
	report := "# Mapa do mini-sistema\n\n```mermaid\n" + m.Mermaid() + "```\n\n```\n" + m.Text() + "```\n\nCaminho Clientes → Pedido: " +
		strings.Join(steps, " → ") + "\n\n" + tokenReport + "\n"
	_ = os.WriteFile(filepath.Join(out, "mini.md"), []byte(report), 0o600)
	_ = os.WriteFile(filepath.Join(out, "mini.json"), data, 0o600)
	t.Log("\n" + report)
}
