//go:build integration

package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Snapshot pela árvore de acessibilidade (docs/SNAPSHOT-V2.md): mesma saída do snapshot antigo (motor
// dom) nas fixtures comuns e mais alcance (listeners sem atributos, slots, shadow fechado), com as
// ações pelo backendNodeId.
func TestAXEngine(t *testing.T) {
	ax, cleanup := newHarness(nil)
	defer cleanup()
	clickable := func(t *testing.T, snap, label string) string {
		t.Helper()
		m := regexp.MustCompile(`\[(e\d+)\] (?:clickable|button) "` + regexp.QuoteMeta(label)).FindStringSubmatch(snap)
		if m == nil {
			t.Fatalf("no %q in:\n%s", label, snap)
		}
		return m[1]
	}

	t.Run("same output as the dom engine", func(t *testing.T) {
		snap := ax.ok(t, "open", map[string]any{"url": ax.url("/pages/form.html")})
		got := strings.ReplaceAll(snap, ax.site.URL, "http://fixture")
		got = regexp.MustCompile(`step #\d+`).ReplaceAllString(got, "step #N")
		want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "form.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Fatalf("ax differs from the dom golden:\n%s", got)
		}
	})

	t.Run("divs with click listeners only", func(t *testing.T) {
		snap := ax.ok(t, "open", map[string]any{"url": ax.url("/pages/divs-onclick.html")})
		ax.ok(t, "click", map[string]any{"ref": clickable(t, snap, "Plano Pro")})
		if got := ax.mainWorld(t, `() => ({s: document.getElementById('status').textContent})`)["s"]; got != "escolheu pro" {
			t.Fatalf("status %v", got)
		}
	})

	t.Run("slotted content and closed shadow are visible", func(t *testing.T) {
		snap := ax.ok(t, "open", map[string]any{"url": ax.url("/pages/web-components.html")})
		mustContain(t, snap, `region "Conta"`, `button "Trocar senha"`)
		snap = ax.ok(t, "open", map[string]any{"url": ax.url("/pages/closed-shadow.html")})
		mustContain(t, snap, `form "Cartão"`, `button "Pagar agora"`)
		if n := ax.app.Session.AXStats().ClosedShadowNodes; n == 0 {
			t.Fatal("closed shadow nodes should be counted")
		}
	})

	t.Run("actions reach the closed shadow by backendNodeId", func(t *testing.T) {
		snap := ax.ok(t, "open", map[string]any{"url": ax.url("/pages/closed-shadow.html")})
		ref := func(re string) string {
			m := regexp.MustCompile(`\[(e\d+)\] ` + re).FindStringSubmatch(snap)
			if m == nil {
				t.Fatalf("no %s in:\n%s", re, snap)
			}
			return m[1]
		}
		ax.ok(t, "type", map[string]any{"ref": ref(`textbox "Nome no cartão"`), "text": "Ana Lima"})
		ax.ok(t, "select", map[string]any{"ref": ref(`combobox "Parcelas"`), "option": "2x"})
		snap = ax.ok(t, "click", map[string]any{"ref": ref(`button "Pagar agora"`)})
		if got := ax.mainWorld(t, `() => ({s: document.getElementById('status').textContent})`)["s"]; got != "pago" {
			t.Fatalf("status %v", got)
		}
		mustContain(t, ax.ok(t, "snapshot", nil), `textbox "Nome no cartão" value="Ana Lima"`, `combobox "Parcelas" value="2x"`)
	})

	t.Run("refs live only in Go and the isolated world", func(t *testing.T) {
		snap := ax.ok(t, "open", map[string]any{"url": ax.url("/pages/reading.html")})
		if got := ax.mainWorld(t, `() => ({n: document.querySelectorAll('[data-vitral-ref]').length})`)["n"]; got != float64(0) {
			t.Fatalf("the page sees %v ref attributes", got)
		}
		first := regexp.MustCompile(`\[(e\d+)\] link "Primeiro link"`).FindStringSubmatch(snap)
		if first == nil || strings.Contains(snap, "Link distante") {
			t.Fatalf("unexpected snapshot:\n%s", snap)
		}
		// O link lido no texto tem a mesma ref do snapshot.
		text := ax.ok(t, "snapshot", map[string]any{"text": true})
		mustContain(t, text, "Primeiro link["+first[1]+"]")
		// A ref criada pela leitura vale para ações e continua a mesma no próximo snapshot.
		far := regexp.MustCompile(`Link distante\[(e\d+)\]`).FindStringSubmatch(text)
		if far == nil {
			t.Fatalf("no ref for the far link:\n%s", text)
		}
		ax.ok(t, "click", map[string]any{"ref": far[1]})
		if got := ax.mainWorld(t, `() => ({s: document.getElementById('status').textContent})`)["s"]; got != "distante" {
			t.Fatalf("status %v", got)
		}
		mustContain(t, ax.ok(t, "snapshot", map[string]any{"full": true}), "["+far[1]+`] link "Link distante"`)
	})

	// Regressões dos três problemas que a comparação com o motor dom achou (docs/SNAPSHOT-V2.md).
	t.Run("regressions", func(t *testing.T) {
		body := func(snap string) string {
			lines := strings.Split(snap, "\n")
			return strings.Join(lines[2:len(lines)-1], "\n") // sem page:, viewport: e flow step
		}
		tests := []struct {
			page, want, click, status string
		}{
			{"layout-table.html", `[e1] heading "Preços" level=1
[e2] table rows=3
  [e3] row "Produto | Preço"
  [e4] row "Caneta | R$ 3"
  [e5] row "Caderno | R$ 12"`, "", ""},
			{"delegated-document.html", `[e1] heading "Entregas" level=1
[e2] clickable "Entrega normal"
[e3] clickable "Entrega expressa"
[e4] status "nenhuma"`, "e3", "entrega expressa"},
			{"body-listener.html", `[e1] heading "Cupons" level=1
[e2] clickable "Cupom 10%"
[e3] clickable "Frete grátis"
[e4] status "nenhum"`, "e3", "cupom frete"},
		}
		for _, tt := range tests {
			t.Run(tt.page, func(t *testing.T) {
				if got := body(ax.ok(t, "open", map[string]any{"url": ax.url("/pages/" + tt.page)})); got != tt.want {
					t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
				}
				if tt.click == "" {
					return
				}
				ax.ok(t, "click", map[string]any{"ref": tt.click})
				if got := ax.mainWorld(t, `() => ({s: document.getElementById('status').textContent})`)["s"]; got != tt.status {
					t.Fatalf("status %v", got)
				}
			})
		}
	})

	t.Run("partial ax tree matches the full tree", func(t *testing.T) {
		pages, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "pages", "*.html"))
		extra, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "pages", "malicious", "*.html"))
		both := func(t *testing.T, args map[string]any) {
			t.Helper()
			ax.app.Session.AXFullTree = true
			want := ax.ok(t, "snapshot", args)
			ax.app.Session.AXFullTree = false
			if got := ax.ok(t, "snapshot", args); got != want {
				t.Fatalf("partial differs from full:\n--- partial\n%s\n--- full\n%s", got, want)
			}
		}
		for _, p := range append(pages, extra...) {
			rel, _ := filepath.Rel(filepath.Join("..", "..", "testdata", "pages"), p)
			t.Run(rel, func(t *testing.T) {
				ax.ok(t, "open", map[string]any{"url": ax.url("/pages/" + filepath.ToSlash(rel))})
				both(t, nil)
				// No meio da página (refs que já existem e as novas) e numa subárvore.
				ax.mainWorld(t, `() => { scrollTo({top: (document.scrollingElement.scrollHeight - innerHeight) / 2, behavior: 'instant'}); return {}; }`)
				both(t, nil)
				snap := ax.ok(t, "snapshot", nil)
				if m := regexp.MustCompile(`\[(e\d+)\] (?:list|table|form|main|navigation|dialog)`).FindStringSubmatch(snap); m != nil {
					both(t, map[string]any{"scope": m[1]})
				}
			})
		}
	})

	t.Run("scope, full and offscreen refs in the diff", func(t *testing.T) {
		snap := ax.ok(t, "open", map[string]any{"url": ax.url("/pages/form.html")})
		list := regexp.MustCompile(`\[(e\d+)\] list`).FindStringSubmatch(snap)[1]
		mustContain(t, ax.ok(t, "snapshot", map[string]any{"scope": list}), `listitem "Eva"`, `listitem "Gabi"`)
		ax.fail(t, "snapshot", map[string]any{"scope": "e9999"}, "e9999")
		mustContain(t, ax.ok(t, "snapshot", map[string]any{"full": true}), `link "Link do rodapé"`)
		// O diff depois de rolar usa as refs conhecidas que saíram da viewport (offscreen).
		mustContain(t, ax.ok(t, "scroll", map[string]any{"direction": "down", "amount": 900}), "flow step #")
	})
}
