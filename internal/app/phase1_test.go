//go:build integration

package app_test

import (
	"regexp"
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

// Textos longos vêm cortados no snapshot normal e inteiros com scope (o jeito de ler o que foi cortado).
func TestScopeShowsFullText(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/cursor.html")})
	full := "Nunca te ensinaram Claude Code assim (GUIA COMPLETO DO ZERO) 219 mil visualizações há 2 meses · 3 horas e 20 minutos"
	if strings.Contains(snap, full) || !strings.Contains(snap, `link "Nunca te ensinaram Claude Code assim`) || !strings.Contains(snap, "…\"") {
		t.Fatalf("default snapshot should cut the long link at 80 chars:\n%s", snap)
	}
	m := regexp.MustCompile(`\[(e\d+)\] link "Nunca te ensinaram`).FindStringSubmatch(snap)
	if m == nil {
		t.Fatalf("no long link in:\n%s", snap)
	}
	link := m[1]
	mustContain(t, h.ok(t, "snapshot", map[string]any{"scope": link}), `"`+full+`"`)
}

// Um texto maior que o limite do script (300) chega cortado com "…" mesmo no snapshot com scope:
// sem a marca, pareceria completo.
func TestScopeMarksTextCutAtTheLimit(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	h.mainWorld(t, `() => { const b = document.createElement('button'); b.id = 'enorme';
		b.textContent = 'capítulo '.repeat(60); document.body.prepend(b); return {}; }`)
	snap := h.ok(t, "snapshot", nil)
	m := regexp.MustCompile(`\[(e\d+)\] button "capítulo`).FindStringSubmatch(snap)
	if m == nil {
		t.Fatalf("no long button in:\n%s", snap)
	}
	scoped := h.ok(t, "snapshot", map[string]any{"scope": m[1]})
	_, line, found := strings.Cut(scoped, "["+m[1]+"]")
	if !found {
		t.Fatalf("scope output without %s:\n%s", m[1], scoped)
	}
	line, _, _ = strings.Cut(line, "\n")
	if !strings.HasSuffix(line, `…"`) || len([]rune(line)) > 340 {
		t.Fatalf("text over 300 chars must end with …:\n%s", line)
	}
}

// text=true lê parágrafos (que o snapshot não lista), do conteúdo principal ou de um scope, sem o
// texto do selo do Vitral e com aviso quando corta.
func TestSnapshotText(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/dialog.html")})
	h.mainWorld(t, `() => { const p = document.createElement('p'); p.id = 'paragrafo';
		p.textContent = 'Um parágrafo que o snapshot não lista, mas que a IA precisa ler.'; document.body.append(p); return {}; }`)
	if strings.Contains(h.ok(t, "snapshot", nil), "Um parágrafo") {
		t.Fatal("plain paragraphs are not in the element list")
	}
	text := h.ok(t, "snapshot", map[string]any{"text": true})
	mustContain(t, text, "text of main content:", "Um parágrafo que o snapshot não lista, mas que a IA precisa ler.")
	for _, own := range []string{"Vitral está usando", "Pausar IA"} {
		if strings.Contains(text, own) {
			t.Fatalf("badge text leaked into the page text:\n%s", text)
		}
	}
	_ = snap
	h.mainWorld(t, `() => { const d = document.createElement('section'); d.setAttribute('aria-label', 'Longo');
		d.textContent = 'palavra '.repeat(900); document.body.append(d); return {}; }`)
	long := h.ok(t, "snapshot", nil)
	m := regexp.MustCompile(`\[(e\d+)\] region "Longo"`).FindStringSubmatch(long)
	if m == nil {
		t.Fatalf("no region in:\n%s", long)
	}
	scoped := h.ok(t, "snapshot", map[string]any{"text": true, "scope": m[1]})
	mustContain(t, scoped, "text of ["+m[1]+"] region:", "… showing chars 0-4000 of 7199. Looking for something specific? Use find=", "To keep reading: textFrom=4000")
	rest := h.ok(t, "snapshot", map[string]any{"text": true, "scope": m[1], "textFrom": 4000})
	mustContain(t, rest, "text of ["+m[1]+"] region, from char 4000:")
	if strings.Contains(rest, "showing chars") {
		t.Fatalf("the second page reaches the end:\n%s", rest[len(rest)-200:])
	}
	mustContain(t, h.ok(t, "snapshot", map[string]any{"text": true, "scope": m[1], "textFrom": 9000}), "nothing after char 9000 (it has 7199 chars)")
	h.fail(t, "snapshot", map[string]any{"text": true, "scope": "e9999"}, "e9999")
}

// text=true num título lê a seção (até o próximo título do mesmo nível), inclusive com o título num
// invólucro como o da Wikipédia, e deixa de fora as linhas que são só menu ou botão.
func TestSnapshotTextSection(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/dialog.html")})
	h.mainWorld(t, `() => { const a = document.createElement('article');
		a.innerHTML = '<nav><a href="#x">Ler</a> <a href="#y">Editar</a></nav>' +
			'<p>Introdução do artigo.</p>' +
			'<div class="mw-heading"><h2>História</h2><span><a href="#e">editar</a></span></div>' +
			'<p>Parágrafo um da história.</p><h3>Idade Média</h3><p>Parágrafo da subseção.</p>' +
			'<div class="mw-heading"><h2>Técnica</h2></div><p>Parágrafo da técnica.</p>';
		document.body.append(a); return {}; }`)
	full := h.ok(t, "snapshot", map[string]any{"full": true})
	m := regexp.MustCompile(`\[(e\d+)\] heading "História"`).FindStringSubmatch(full)
	if m == nil {
		t.Fatalf("no heading in:\n%s", full)
	}
	sec := h.ok(t, "snapshot", map[string]any{"text": true, "scope": m[1]})
	mustContain(t, sec, "text of section ["+m[1]+"] heading:", "Parágrafo um da história.", "Parágrafo da subseção.")
	for _, out := range []string{"Introdução", "Parágrafo da técnica", "Técnica"} {
		if strings.Contains(sec, out) {
			t.Fatalf("section should stop at the next h2 (%q leaked):\n%s", out, sec)
		}
	}
	page := h.ok(t, "snapshot", map[string]any{"text": true})
	if regexp.MustCompile(`(?m)^(Ler|Editar)$`).MatchString(page) {
		t.Fatalf("menu-only lines should be dropped:\n%s", page)
	}
	mustContain(t, page, "Introdução do artigo.", "Parágrafo da técnica.")
}

// Os links do texto lido vêm com a ref ("texto[eN]") e a ref serve para clicar.
func TestSnapshotTextLinksHaveRefs(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/styles.html")})
	h.mainWorld(t, `() => { const p = document.createElement('p');
		p.innerHTML = 'Vitrais famosos estão na <a href="#chartres" onclick="this.textContent=\'visitada\'; return false;">Catedral de Chartres</a> e em <a href="#paris">Notre-Dame</a>.';
		(document.querySelector('main') || document.body).prepend(p); return {}; }`)
	text := h.ok(t, "snapshot", map[string]any{"text": true})
	m := regexp.MustCompile(`Vitrais famosos estão na Catedral de Chartres\[(e\d+)\] e em Notre-Dame\[e\d+\]\.`).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("links should be marked with their refs:\n%s", text)
	}
	h.ok(t, "click", map[string]any{"ref": m[1]})
	mustContain(t, h.ok(t, "snapshot", map[string]any{"text": true}), "visitada[")
}

// Títulos vêm marcados no texto lido, e um link de índice (#âncora) como scope lê a seção de destino.
func TestSnapshotTextHeadingsAndTOC(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/styles.html")})
	h.mainWorld(t, `() => { const a = document.createElement('article');
		a.innerHTML = '<nav><a href="#Historia">História</a></nav><p>Intro.</p>' +
			'<div class="mw-heading"><h2 id="Historia">História</h2></div><p>Texto da história.</p>' +
			'<h2 id="Tecnica">Técnica</h2><p>Texto da técnica.</p>';
		(document.querySelector('main') || document.body).prepend(a); return {}; }`)
	text := h.ok(t, "snapshot", map[string]any{"text": true})
	m := regexp.MustCompile(`(?m)^## Técnica\[(e\d+)\]$`).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("headings should be marked with level and ref:\n%s", text)
	}
	mustContain(t, h.ok(t, "snapshot", map[string]any{"text": true, "scope": m[1]}), "Texto da técnica.")
	mustContain(t, text, "Intro.", "Texto da história.")
	if !regexp.MustCompile(`(?m)^## História\[e\d+\]$`).MatchString(text) {
		t.Fatalf("a heading with the same text as a menu item must stay:\n%s", text)
	}
	// O índice fica no nav (fora do texto lido); a IA pega a ref dele no snapshot normal.
	toc := regexp.MustCompile(`\[(e\d+)\] link "História"`).FindStringSubmatch(h.ok(t, "snapshot", nil))
	if toc == nil {
		t.Fatal("table-of-contents link not in the snapshot")
	}
	sec := h.ok(t, "snapshot", map[string]any{"text": true, "scope": toc[1]})
	mustContain(t, sec, "Texto da história.")
	if strings.Contains(sec, "Texto da técnica") {
		t.Fatalf("toc link should read only its section:\n%s", sec)
	}
}

// find traz só os trechos com o termo (sem acento, sem maiúsculas), com links marcados e a posição
// para ler em volta; legendas vêm marcadas como (caption).
func TestSnapshotTextFindAndCaptions(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/styles.html")})
	h.mainWorld(t, `() => { const a = document.createElement('article');
		a.innerHTML = '<figure><img alt="" src="data:image/gif;base64,R0lGODlhAQABAAAAACw="><figcaption>Vitral na Catédral de Friburgo</figcaption></figure>' +
			'<p>' + 'texto de enchimento '.repeat(300) + '</p>' +
			'<p>O exemplo mais famoso fica na <a href="#c">Catedral de Chartres</a>, na França.</p>' +
			'<p>' + 'mais texto '.repeat(200) + '</p>';
		(document.querySelector('main') || document.body).prepend(a); return {}; }`)
	text := h.ok(t, "snapshot", map[string]any{"text": true})
	mustContain(t, text, "(caption) Vitral na Catédral de Friburgo")

	found := h.ok(t, "snapshot", map[string]any{"find": "CATEDRAL"})
	mustContain(t, found, `"CATEDRAL" in the text of main content`, "2 match(es)", "Catedral de Chartres[e", "textFrom=<@number>")
	m := regexp.MustCompile(`@(\d+): [^\n]*Catedral de Chartres`).FindStringSubmatch(found)
	if m == nil || len(found) > 1500 {
		t.Fatalf("passages should be short and positioned:\n%s", found)
	}
	around := h.ok(t, "snapshot", map[string]any{"text": true, "textFrom": atoi(t, m[1])})
	mustContain(t, around, "Catedral de Chartres[e")
	mustContain(t, h.ok(t, "snapshot", map[string]any{"find": "inexistente"}), `"inexistente" not found`)
	// Frase que não existe inteira: cai para qualquer uma das palavras, e a resposta diz isso.
	mustContain(t, h.ok(t, "snapshot", map[string]any{"find": "França catedral gótica"}),
		`the exact phrase "França catedral gótica" is not in the text`, "passages with any of its words", "Catedral de Chartres[e")
}

// Notas de rodapé ([2]) saem do texto lido; links de verdade continuam marcados.
func TestSnapshotTextDropsFootnotes(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/styles.html")})
	h.mainWorld(t, `() => { const p = document.createElement('p');
		p.innerHTML = 'Vidro colorido<sup><a href="#n1">[1]</a></sup><sup><a href="#n2">[2]</a></sup> em <a href="#ig">igrejas</a>.';
		(document.querySelector('main') || document.body).prepend(p); return {}; }`)
	text := h.ok(t, "snapshot", map[string]any{"text": true})
	if !regexp.MustCompile(`Vidro colorido em igrejas\[e\d+\]\.`).MatchString(text) {
		t.Fatalf("footnotes should be dropped and real links kept:\n%s", text)
	}
}
