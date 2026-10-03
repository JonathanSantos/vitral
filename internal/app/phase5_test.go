//go:build integration

package app_test

import (
	"regexp"
	"strings"
	"testing"
)

// recordForm executa o fluxo da Fase 1 e devolve o número do primeiro passo gravado.
func recordForm(t *testing.T, nome string) int {
	t.Helper()
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	m := regexp.MustCompile(`step #(\d+)`).FindStringSubmatch(snap)
	if m == nil {
		t.Fatalf("open should be recorded:\n%s", snap)
	}
	first := atoi(t, m[1])

	mustContain(t, h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Nome"), "text": nome}), "step #")
	h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "E-mail"), "text": "contato@exemplo.com"})
	h.ok(t, "select", map[string]any{"ref": ref(t, snap, "combobox", "País"), "option": "Brasil"})
	h.ok(t, "click", map[string]any{"ref": ref(t, snap, "checkbox", "Aceito os termos")})
	// Senha é com a pessoa: não entra no fluxo.
	mustContain(t, h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Senha"), "text": "x"}), "password")
	h.resume(t)
	h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Enviar")})
	h.ok(t, "wait_for", map[string]any{"text": "Cadastro enviado: " + nome})
	return first
}

// Critério de aceite da Fase 5: gravar, salvar com parâmetro, reexecutar e falhar no passo certo.
func TestFlowRecordSaveRun(t *testing.T) {
	first := recordForm(t, "Maria Silva")

	h.fail(t, "flow_save", map[string]any{"name": "cadastro", "fromStep": first, "params": map[string]any{"nome": "Ma"}}, "too short")
	h.fail(t, "flow_save", map[string]any{"name": "cadastro", "fromStep": first, "params": map[string]any{"cidade": "Recife"}}, `param "cidade"`)

	out := h.ok(t, "flow_save", map[string]any{
		"name": "cadastro", "description": "Preenche e envia o cadastro", "fromStep": first,
		"params": map[string]any{"nome": "Maria Silva"},
	})
	mustContain(t, out,
		"saved ", "cadastro.yaml (7 steps",
		`1. open http://`,
		`2. type textbox "Nome" = "{{nome}}"`,
		`6. click button "Enviar"`,
		`step 2 type.text: "Maria Silva" → "{{nome}}"`,
		`step 7 wait_for.text: "Cadastro enviado: Maria Silva" → "Cadastro enviado: {{nome}}"`,
	)

	list := h.ok(t, "flow_list", nil)
	mustContain(t, list, "cadastro — Preenche e envia o cadastro (params: nome; 7 steps)")

	out = h.ok(t, "flow_run", map[string]any{"name": "cadastro", "params": map[string]any{"nome": "João Souza"}})
	mustContain(t, out, `flow cadastro (nome="João Souza"): 7 steps`, "7. ok wait_for", "done: 7/7 steps passed.",
		`alert "Cadastro enviado: João Souza / contato@exemplo.com / Brasil"`)
	if strings.Contains(out, "step #") {
		t.Fatalf("replayed steps must not be recorded:\n%s", out)
	}

	h.fail(t, "flow_run", map[string]any{"name": "cadastro"}, "missing params: nome")
	h.fail(t, "flow_run", map[string]any{"name": "nao-existe"}, `flow "nao-existe" not found`)

	// A tela mudou: o replay para no passo 6 e explica o que existe agora.
	renameSubmit.Store(true)
	defer renameSubmit.Store(false)
	h.fail(t, "flow_run", map[string]any{"name": "cadastro", "params": map[string]any{"nome": "Ana Lima"}},
		`6. FAILED click button "Enviar"`)
	out, _ = h.call(t, "flow_run", map[string]any{"name": "cadastro", "params": map[string]any{"nome": "Ana Lima"}})
	mustContain(t, out, `5. ok click checkbox "Aceito os termos"`,
		`reason: no button "Enviar" found. button elements on the page: `, `"Confirmar"`,
		"stopped at step 6 of 7.", "current page:", `button "Confirmar"`)
}

func TestFlowWithMockStep(t *testing.T) {
	t.Cleanup(func() { h.ok(t, "mock_clear", nil) })
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	first := atoi(t, regexp.MustCompile(`step #(\d+)`).FindStringSubmatch(snap)[1])

	out := h.ok(t, "mock_add", map[string]any{"urlPattern": "*/api/orders*", "status": 503})
	mustContain(t, out, "step #")
	h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Carregar pedidos")})
	h.ok(t, "wait_for", map[string]any{"text": "HTTP 503"})

	out = h.ok(t, "flow_save", map[string]any{"name": "api-fora", "fromStep": first})
	mustContain(t, out, "2. mock_add */api/orders*", "3. click button \"Carregar pedidos\"")

	h.ok(t, "mock_clear", nil)
	out = h.ok(t, "flow_run", map[string]any{"name": "api-fora"})
	mustContain(t, out, "done: 4/4 steps passed.", `alert "Erro ao carregar pedidos (HTTP 503)"`)

	// Sem passos novos desde o último save.
	h.fail(t, "flow_save", map[string]any{"name": "vazio"}, "no new steps since the last flow_save")
}
