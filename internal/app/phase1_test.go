//go:build integration

package app_test

import (
	"strings"
	"testing"
)

func TestFormSnapshotGolden(t *testing.T) {
	out := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	golden(t, "form.txt", out)
}

// Critério de aceite da Fase 1: preencher e enviar o formulário só com as ferramentas.
func TestFillAndSubmitForm(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})

	out := h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Nome"), "text": "Maria"})
	mustContain(t, out, `textbox "Nome" value="Maria"`)

	h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "E-mail"), "text": "maria@exemplo.com"})

	out = h.ok(t, "select", map[string]any{"ref": ref(t, snap, "combobox", "País"), "option": "Brasil"})
	mustContain(t, out, `value="Brasil"`)

	out = h.ok(t, "click", map[string]any{"ref": ref(t, snap, "checkbox", "Aceito os termos")})
	mustContain(t, out, "checked")

	h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Observações"), "text": "linha 1"})

	out = h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Enviar")})
	mustContain(t, out, `+ [`, `alert "Cadastro enviado: Maria / maria@exemplo.com / Brasil"`)

	out = h.ok(t, "wait_for", map[string]any{"text": "Cadastro enviado"})
	mustContain(t, out, "found \"Cadastro enviado\"")
}

func TestValidationErrorAppears(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})

	// Campo obrigatório vazio: o browser bloqueia o envio e a mensagem nativa aparece no diff.
	out := h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Enviar")})
	mustContain(t, out, `~ [`, `textbox "Nome"`, `invalid="`)

	h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Nome"), "text": "Ana"})
	out = h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Enviar")})
	mustContain(t, out, `error "Aceite os termos para continuar"`)
}

func TestActionableErrors(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})

	// Senha: não é erro, o Vitral entrega a vez para a pessoa (UX-3) e espera ela retomar.
	out := h.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Senha"), "text": "x"})
	mustContain(t, out, "password fields must be filled by the user in the browser window", `click "Retomar"`)
	h.resume(t)
	h.fail(t, "click", map[string]any{"ref": ref(t, snap, "button", "Rascunho")}, "is disabled.")
	h.fail(t, "click", map[string]any{"ref": "e999"}, "ref e999 not found: page changed since last snapshot. Call snapshot.")
	h.fail(t, "select", map[string]any{"ref": ref(t, snap, "combobox", "País"), "option": "Chile"}, `"Portugal"`)
	h.fail(t, "type", map[string]any{"ref": ref(t, snap, "button", "Enviar"), "text": "x"}, "is not a text field")
	h.fail(t, "press", map[string]any{"key": "Banana"}, "unknown key")

	// O overlay cobre a página: clicar em algo por baixo deve apontar o dialog.
	out = h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Abrir aviso")})
	mustContain(t, out, `dialog "Aviso importante" open modal`)
	h.fail(t, "click", map[string]any{"ref": ref(t, snap, "button", "Enviar")}, "is covered by dialog")

	out = h.ok(t, "click", map[string]any{"ref": ref(t, out, "button", "Fechar")})
	mustContain(t, out, `- [`, `dialog "Aviso importante"`)

	// Por último: navegação que falha leva o browser para a página de erro.
	h.fail(t, "open", map[string]any{"url": "http://127.0.0.1:1/"}, "navigation to http://127.0.0.1:1/ failed")
}

func TestListCompaction(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	mustContain(t, snap, `list "Últimos cadastros" items=7 (showing 5)`)
	if strings.Contains(snap, `"Fábio"`) {
		t.Fatalf("list should be limited:\n%s", snap)
	}
	list := ref(t, snap, "list", "Últimos cadastros")
	out := h.ok(t, "snapshot", map[string]any{"scope": list})
	mustContain(t, out, `listitem "Gabi"`)
}

func TestScrollPressBack(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})

	out := h.ok(t, "scroll", map[string]any{"direction": "down", "amount": 2000})
	mustContain(t, out, `link "Link do rodapé"`, "elements left the viewport")

	out = h.ok(t, "scroll", map[string]any{"ref": ref(t, snap, "textbox", "Nome")})
	mustContain(t, out, `textbox "Nome"`)

	h.ok(t, "click", map[string]any{"ref": ref(t, snap, "textbox", "Nome")})
	out = h.ok(t, "press", map[string]any{"key": "Tab"})
	mustContain(t, out, `textbox "E-mail"`, "focused")

	out = h.ok(t, "scroll", map[string]any{"direction": "down", "amount": 2000})
	footer := ref(t, out, "link", "Link do rodapé")
	out = h.ok(t, "click", map[string]any{"ref": footer})
	mustContain(t, out, "page: ", "bottom=1")

	out = h.ok(t, "back", nil)
	mustContain(t, out, "/pages/form.html")
}

func TestWaitForTimeoutAndGone(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	h.fail(t, "wait_for", map[string]any{"text": "não existe", "timeoutMs": 500}, "timeout after 500ms")
	out := h.ok(t, "wait_for", map[string]any{"text": "não existe", "gone": true})
	mustContain(t, out, "is gone")
}
