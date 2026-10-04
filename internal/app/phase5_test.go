//go:build integration

package app_test

import (
	"fmt"
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
		fmt.Sprintf("1. open http://%s/pages/form.html (#%d)", strings.TrimPrefix(h.site.URL, "http://"), first),
		fmt.Sprintf(`2. type textbox "Nome" = "{{nome}}" (#%d)`, first+1),
		fmt.Sprintf(`6. click button "Enviar" (#%d)`, first+5),
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
	h.site.RenameSubmit.Store(true)
	defer h.site.RenameSubmit.Store(false)
	h.fail(t, "flow_run", map[string]any{"name": "cadastro", "params": map[string]any{"nome": "Ana Lima"}},
		`6. FAILED click button "Enviar"`)
	out, _ = h.call(t, "flow_run", map[string]any{"name": "cadastro", "params": map[string]any{"nome": "Ana Lima"}})
	mustContain(t, out, `5. ok click checkbox "Aceito os termos"`,
		`reason: no button "Enviar" found and healing did not pick a replacement: best match is below the confidence threshold.`,
		`Closest matches: [`, `button "Confirmar" (0.`,
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

// A prévia mostra o que seria salvo (número da sessão, ação, alvo) sem salvar; excludeSteps tira passos
// a mais (achado da avaliação com o Haiku: salvou passos que não eram da tarefa).
func TestFlowSavePreviewAndExclude(t *testing.T) {
	first := recordForm(t, "Maria Silva")
	h.ok(t, "snapshot", nil) // não grava passo
	extra := h.ok(t, "scroll", map[string]any{"direction": "down"})
	mustContain(t, extra, fmt.Sprintf("flow step #%d", first+7))

	preview := h.ok(t, "flow_save", map[string]any{
		"name": "cadastro-previa", "fromStep": first, "params": map[string]any{"nome": "Maria Silva"}, "preview": true,
	})
	mustContain(t, preview,
		fmt.Sprintf("preview of flow cadastro-previa: 8 steps from session steps #%d to #%d (nothing saved yet)", first, first+7),
		fmt.Sprintf(`2. type textbox "Nome" = "{{nome}}" (#%d)`, first+1),
		fmt.Sprintf("8. scroll down (#%d)", first+7),
		"substitutions:", "call flow_save again without preview",
		"tip: this flow would not check its result")
	// O expect é conferido na página atual: o título da aba e o nome de um campo não são texto visível.
	title := h.ok(t, "flow_save", map[string]any{"name": "cadastro-previa", "fromStep": first, "preview": true, "expect": "Cadastro - Fixture"})
	mustContain(t, title, `expect "Cadastro - Fixture" is only in the page title`)
	field := h.ok(t, "flow_save", map[string]any{"name": "cadastro-previa", "fromStep": first, "preview": true, "expect": "Nome completo do titular"})
	mustContain(t, field, `expect "Nome completo do titular" is not in the visible text of the current page`)
	if list := h.ok(t, "flow_list", nil); strings.Contains(list, "cadastro-previa") {
		t.Fatalf("preview must not save:\n%s", list)
	}

	h.fail(t, "flow_save", map[string]any{"name": "cadastro-previa", "fromStep": first, "excludeSteps": []int{first - 1}},
		fmt.Sprintf("excludeSteps: step #%d is not in the steps being saved (#%d-#%d)", first-1, first, first+7))
	// A prévia com a chamada completa; o save logo depois, só com o nome, reaproveita tudo (passos,
	// parâmetros, resultado esperado e descrição).
	h.ok(t, "flow_save", map[string]any{
		"name": "cadastro-previa", "fromStep": first, "excludeSteps": []int{first + 7}, "preview": true,
		"params": map[string]any{"nome": "Maria Silva"}, "expect": "Cadastro enviado: Maria Silva", "description": "Cadastro de teste",
	})
	out := h.ok(t, "flow_save", map[string]any{"name": "cadastro-previa"})
	mustContain(t, h.ok(t, "flow_list", nil), "cadastro-previa — Cadastro de teste (params: nome; 8 steps)")
	mustContain(t, out, "cadastro-previa.yaml (8 steps", "as in the last preview", fmt.Sprintf(`7. wait_for text "Cadastro enviado: {{nome}}" (#%d)`, first+6),
		`8. wait_for text "Cadastro enviado: {{nome}}" (expect)`)
	if strings.Contains(out, "scroll down") || strings.Contains(out, "warning:") {
		t.Fatalf("excluded step was saved, or a visible expect warned:\n%s", out)
	}
	run := h.ok(t, "flow_run", map[string]any{"name": "cadastro-previa", "params": map[string]any{"nome": "Rui Costa"}})
	mustContain(t, run, "done: 8/8 steps passed.")
}

// O expect só é conferido na página onde o fluxo termina: salvando com outra página aberta, não há aviso.
func TestFlowSaveExpectOnAnotherPage(t *testing.T) {
	first := recordForm(t, "Maria Silva")
	h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	out := h.ok(t, "flow_save", map[string]any{"name": "cadastro-outra-pagina", "fromStep": first, "toStep": first + 6, "expect": "Cadastro enviado: Maria Silva", "preview": true})
	if strings.Contains(out, "is not in the visible text") || strings.Contains(out, "warning:") {
		t.Fatalf("the expect cannot be checked on another page:\n%s", out)
	}
	// Na própria página, o texto que não está lá ainda avisa.
	recordForm(t, "Maria Silva")
	out = h.ok(t, "flow_save", map[string]any{"name": "cadastro-outra-pagina", "fromStep": first + 8, "expect": "Texto que não existe", "preview": true})
	mustContain(t, out, `expect "Texto que não existe" is not in the visible text`)
}

// expect adiciona o wait_for no fim, com o valor do parâmetro trocado pelo placeholder: o fluxo
// confere o resultado do termo que rodou, mesmo sem um wait_for gravado logo depois dos passos.
func TestFlowSaveExpect(t *testing.T) {
	first := recordForm(t, "Maria Silva")
	out := h.ok(t, "flow_save", map[string]any{
		"name": "cadastro-expect", "fromStep": first, "toStep": first + 5,
		"params": map[string]any{"nome": "Maria Silva"}, "expect": "Cadastro enviado: Maria Silva",
	})
	mustContain(t, out, "(7 steps", `6. click button "Enviar"`, `7. wait_for text "Cadastro enviado: {{nome}}"`,
		`step 7 wait_for.text: "Cadastro enviado: Maria Silva" → "Cadastro enviado: {{nome}}"`)
	if strings.Contains(out, "tip: this flow does not check") {
		t.Fatalf("a flow with expect needs no tip:\n%s", out)
	}
	run := h.ok(t, "flow_run", map[string]any{"name": "cadastro-expect", "params": map[string]any{"nome": "Ana Prado"}})
	mustContain(t, run, "7. ok wait_for text", "done: 7/7 steps passed.")
	// Sem verificação no fim, a resposta sugere o expect.
	noCheck := h.ok(t, "flow_save", map[string]any{"name": "sem-expect", "fromStep": first, "toStep": first + 5})
	mustContain(t, noCheck, "tip: this flow does not check its result", "expect=<text visible at the end>")
	// Um fluxo que termina digitando sem enviar não é salvo (o flow_run digitaria e pararia), a não
	// ser que a IA confirme com ignoreWarnings.
	h.fail(t, "flow_save", map[string]any{"name": "so-digita", "fromStep": first, "toStep": first + 1},
		"not saved: the flow would not repeat the task with other values.")
	out, _ = h.call(t, "flow_save", map[string]any{"name": "so-digita", "fromStep": first, "toStep": first + 1})
	mustContain(t, out, `step 2 (type textbox "Nome" = "Maria Silva") is the last action and does not press Enter`, "ignoreWarnings=true")
	mustContain(t, h.ok(t, "flow_save", map[string]any{"name": "so-digita", "fromStep": first, "toStep": first + 1, "ignoreWarnings": true}),
		"so-digita.yaml (2 steps", "warning:")
}
