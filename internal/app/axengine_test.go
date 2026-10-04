//go:build integration

package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/config"
)

// Protótipo do motor ax (--snapshot-engine=ax): mesma saída do motor dom nas fixtures comuns e mais
// alcance onde o dom não chega (listeners sem atributos, slots, shadow fechado).
func TestAXEngine(t *testing.T) {
	ax, cleanup := newHarness(func(c *config.Config) { c.SnapshotEngine = "ax" })
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
