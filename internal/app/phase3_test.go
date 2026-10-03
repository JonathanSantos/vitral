//go:build integration

package app_test

import (
	"regexp"
	"strings"
	"testing"
)

// Critério de aceite da Fase 3: a lista mostra a chamada e o detalhe traz o corpo JSON.
func TestNetworkListAndDetail(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	out := h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Carregar pedidos")})
	mustContain(t, out, `2 pedidos carregados`)

	list := h.ok(t, "network_list", map[string]any{"filter": "/api/orders"})
	m := regexp.MustCompile(`(\d+) GET 200 http://\S+/api/orders \(\d+ms, [\d.]+K?B\)`).FindStringSubmatch(list)
	if m == nil {
		t.Fatalf("api call missing in:\n%s", list)
	}

	detail := h.ok(t, "network_get", map[string]any{"id": atoi(t, m[1])})
	mustContain(t, detail,
		"GET http://", "status: 200",
		"Authorization: ***",
		"response body (application/json):",
		`"status": "Aprovado"`,
	)
	if strings.Contains(detail, "segredo-123") || strings.Contains(detail, "session=secret") {
		t.Fatalf("sensitive header leaked:\n%s", detail)
	}

	noBody := h.ok(t, "network_get", map[string]any{"id": atoi(t, m[1]), "body": false})
	if strings.Contains(noBody, "response body") {
		t.Fatalf("body=false should skip the body:\n%s", noBody)
	}

	h.fail(t, "network_get", map[string]any{"id": 99999}, "request 99999 not found")
}

func TestNetworkRequestPayloadAndFilters(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Salvar pedido")})

	list := h.ok(t, "network_list", map[string]any{"method": "POST"})
	m := regexp.MustCompile(`(\d+) POST 200 `).FindStringSubmatch(list)
	if m == nil {
		t.Fatalf("POST missing in:\n%s", list)
	}
	detail := h.ok(t, "network_get", map[string]any{"id": atoi(t, m[1])})
	mustContain(t, detail, "request body:", `"id": 1025`)

	docs := h.ok(t, "network_list", map[string]any{"type": "document"})
	mustContain(t, docs, "/pages/api.html")
	empty := h.ok(t, "network_list", map[string]any{"status": "5xx"})
	mustContain(t, empty, "no matching requests")
}

func TestConsoleList(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Carregar pedidos")})
	out := h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Gerar erro")})
	mustContain(t, out, "console: 1 new error(s)", "Cannot read properties of null")

	all := h.ok(t, "console_list", nil)
	mustContain(t, all, "[log] pedidos carregados 2", "[error] Uncaught TypeError", "/pages/api.html:")

	errs := h.ok(t, "console_list", map[string]any{"level": "error"})
	if strings.Contains(errs, "[log]") {
		t.Fatalf("level=error should hide logs:\n%s", errs)
	}
}
