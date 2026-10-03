//go:build integration

package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/config"
)

func clearMocks(t *testing.T) {
	t.Cleanup(func() { h.ok(t, "mock_clear", nil) })
}

// Critério de aceite da Fase 4: mock de 500 mostra o estado de erro.
func TestMockServerError(t *testing.T) {
	clearMocks(t)
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	out := h.ok(t, "mock_add", map[string]any{"urlPattern": "*/api/orders*", "method": "GET", "status": 500, "body": `{"error":"boom"}`})
	mustContain(t, out, "mock added: m", "GET */api/orders* → 500")

	out = h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Carregar pedidos")})
	mustContain(t, out, `alert "Erro ao carregar pedidos (HTTP 500)"`, "network: 1 failed request(s)", "mocked")

	list := h.ok(t, "network_list", map[string]any{"filter": "/api/orders"})
	mustContain(t, list, "GET 500", "mocked")
	mustContain(t, h.ok(t, "mock_list", nil), "(used 1)")
}

// Critério de aceite da Fase 4: mock com delay mostra o loading.
func TestMockDelayShowsLoading(t *testing.T) {
	clearMocks(t)
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	h.ok(t, "mock_add", map[string]any{"urlPattern": "/api/orders", "delayMs": 2500,
		"body": `{"orders":[{"id":7,"status":"Lento","total":1}]}`})

	out := h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Carregar pedidos")})
	mustContain(t, out, `status "Carregando pedidos…"`)
	if strings.Contains(out, "warning: page did not settle") {
		t.Fatalf("delayed mock should not block the action:\n%s", out)
	}
	out = h.ok(t, "wait_for", map[string]any{"text": "1 pedidos carregados"})
	mustContain(t, out, `row "#7 | Lento | R$ 1"`)
}

func TestMockFailureAndTimes(t *testing.T) {
	clearMocks(t)
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	carregar := ref(t, snap, "button", "Carregar pedidos")

	h.ok(t, "mock_add", map[string]any{"urlPattern": "/api/orders", "fail": "connection_refused", "times": 1})
	out := h.ok(t, "click", map[string]any{"ref": carregar})
	mustContain(t, out, `alert "Sem conexão com o servidor"`)
	mustContain(t, h.ok(t, "mock_list", nil), "no active mocks")

	// A regra expirou: a segunda chamada vai para a API real.
	out = h.ok(t, "click", map[string]any{"ref": carregar})
	mustContain(t, out, `2 pedidos carregados`)

	h.fail(t, "mock_add", map[string]any{"urlPattern": "/x", "fail": "explode"}, "unknown fail")
	h.fail(t, "mock_clear", map[string]any{"id": "m999"}, "mock m999 not found")
}

func TestEmulate(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	t.Cleanup(func() { h.ok(t, "emulate", map[string]any{"device": "desktop", "network": "none"}) })

	out := h.ok(t, "emulate", map[string]any{"device": "mobile"})
	mustContain(t, out, "device mobile (390x844, touch)")
	mustContain(t, h.ok(t, "snapshot", nil), "viewport: 390x844")

	h.ok(t, "emulate", map[string]any{"device": "desktop"})
	if snap := h.ok(t, "snapshot", nil); strings.Contains(snap, "viewport: 390x844") {
		t.Fatalf("desktop should restore the viewport:\n%s", snap)
	}

	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	h.ok(t, "emulate", map[string]any{"network": "offline"})
	out = h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Carregar pedidos")})
	mustContain(t, out, `alert "Sem conexão com o servidor"`)

	h.fail(t, "emulate", map[string]any{"device": "watch"}, "unknown device")
	h.fail(t, "emulate", nil, "provide device")
}

// Flags de segurança: mock desabilitado, eval desligado e allowlist de hosts.
func TestSecurityFlags(t *testing.T) {
	strict, cleanup := newHarness(func(c *config.Config) {
		c.DisableMock = true
		c.AllowedHosts = []string{"localhost"}
	})
	defer cleanup()

	tools, err := strict.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if strings.HasPrefix(tool.Name, "mock_") || tool.Name == "eval_js" {
			t.Fatalf("tool %s should not be registered", tool.Name)
		}
	}
	strict.fail(t, "open", map[string]any{"url": strict.url("/pages/api.html")}, "host not allowed")
}

func TestEvalJSWhenAllowed(t *testing.T) {
	open, cleanup := newHarness(func(c *config.Config) { c.AllowEval = true })
	defer cleanup()
	open.ok(t, "open", map[string]any{"url": open.url("/pages/api.html")})

	mustContain(t, open.ok(t, "eval_js", map[string]any{"expression": "document.title"}), `"Pedidos - Fixture"`)
	mustContain(t, open.ok(t, "eval_js", map[string]any{"expression": "({a: 1, b: [2]})"}), "\"a\": 1")
	mustContain(t, open.ok(t, "eval_js", map[string]any{"expression": "Promise.resolve(41 + 1)"}), "42")
	mustContain(t, open.ok(t, "eval_js", map[string]any{"expression": "undefined"}), "undefined")
	open.fail(t, "eval_js", map[string]any{"expression": "nope.x"}, "javascript error: ReferenceError")
}
