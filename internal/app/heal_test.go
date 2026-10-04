//go:build integration

package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Autocura (pedido "próximos passos", 3.5): cada caso grava o fluxo na página v1 e roda na v2.
func TestFlowHealing(t *testing.T) {
	hh, cleanup := newHarness(nil)
	defer cleanup()
	defer hh.site.HealV2.Store(false)

	// record abre a v1, faz as ações e salva o fluxo com o status final como expect; devolve o arquivo.
	record := func(t *testing.T, name string, act func(snap string)) string {
		t.Helper()
		hh.site.HealV2.Store(false)
		snap := hh.ok(t, "open", map[string]any{"url": hh.url("/pages/heal/" + name + ".html")})
		first := atoi(t, regexp.MustCompile(`step #(\d+)`).FindStringSubmatch(snap)[1])
		act(snap)
		snap = hh.ok(t, "click", map[string]any{"ref": ref(t, hh.ok(t, "snapshot", nil), "button", "Continuar")})
		status := regexp.MustCompile(`status "([^"]+)"`).FindStringSubmatch(snap)[1]
		out := hh.ok(t, "flow_save", map[string]any{"name": name, "fromStep": first, "expect": status})
		hh.site.HealV2.Store(true)
		return regexp.MustCompile(`saved (\S+\.yaml)`).FindStringSubmatch(out)[1]
	}
	status := func(t *testing.T) string {
		t.Helper()
		return hh.mainWorld(t, `() => ({s: document.getElementById('status').textContent})`)["s"].(string)
	}
	click := func(t *testing.T, label string) func(string) {
		return func(snap string) { hh.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", label)}) }
	}

	t.Run("id changed: heals", func(t *testing.T) {
		record(t, "id-mudou", click(t, "Aplicar"))
		out := hh.ok(t, "flow_run", map[string]any{"name": "id-mudou"})
		mustContain(t, out, `2. ok (healed) click button "Aplicar"`, "healed steps", `step 2: was button "Aplicar", now [`,
			`button "Aplicar filtros" (confidence`, "done: 4/4 steps passed.", "flow_update with name=id-mudou")
		if got := status(t); got != "aplicar, continuar" {
			t.Fatalf("status %q", got)
		}
	})

	t.Run("text changed slightly: heals, and flow_update saves it", func(t *testing.T) {
		file := record(t, "texto-mudou", click(t, "Salvar alterações"))
		before, _ := os.ReadFile(file)
		out := hh.ok(t, "flow_run", map[string]any{"name": "texto-mudou"})
		mustContain(t, out, `step 2: was button "Salvar alterações", now [`, `button "Salvar as alterações"`, "done: 4/4")
		if got := status(t); got != "alteracoes, continuar" {
			t.Fatalf("status %q", got)
		}

		hh.fail(t, "flow_update", map[string]any{"name": "texto-mudou", "applyHealing": false}, "applyHealing=true")
		up := hh.ok(t, "flow_update", map[string]any{"name": "texto-mudou", "applyHealing": true})
		mustContain(t, up, "updated "+file, "1 steps changed", ".history", `step 2: button "Salvar alterações" → button "Salvar as alterações"`)
		hist, _ := filepath.Glob(filepath.Join(filepath.Dir(file), ".history", "texto-mudou.*.yaml"))
		if len(hist) != 1 {
			t.Fatalf("history files: %v", hist)
		}
		if old, _ := os.ReadFile(hist[0]); string(old) != string(before) {
			t.Fatal("history must keep the previous version")
		}
		// Atualizado: roda sem autocura; e não há mais o que atualizar.
		out = hh.ok(t, "flow_run", map[string]any{"name": "texto-mudou"})
		if strings.Contains(out, "healed") {
			t.Fatalf("updated flow still heals:\n%s", out)
		}
		hh.fail(t, "flow_update", map[string]any{"name": "texto-mudou", "applyHealing": true}, "no healed steps for flow texto-mudou")

		// heal: off desliga; heal: ask pergunta mesmo em passo comum.
		hh.site.HealV2.Store(false)
		file = record(t, "texto-mudou", click(t, "Salvar alterações"))
		data, _ := os.ReadFile(file)
		writeFlow(t, file, string(data)+"heal: off\n")
		hh.fail(t, "flow_run", map[string]any{"name": "texto-mudou"}, `needs healing, but the flow has heal: off`)
		writeFlow(t, file, string(data)+"heal: ask\n")
		mustContain(t, hh.ok(t, "flow_run", map[string]any{"name": "texto-mudou"}), "Healing needed at step 2")
		writeFlow(t, file, string(data)+"heal: sometimes\n")
		hh.fail(t, "flow_run", map[string]any{"name": "texto-mudou"}, `heal must be auto, ask or off`)
	})

	t.Run("element moved: heals", func(t *testing.T) {
		record(t, "mudou-de-lugar", func(snap string) {
			// O Exportar da região Relatório (o primeiro).
			hh.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Exportar")})
		})
		out := hh.ok(t, "flow_run", map[string]any{"name": "mudou-de-lugar"})
		mustContain(t, out, `step 2: was button "Exportar" (in region "Relatório"), now [`, "done: 4/4")
		if got := status(t); got != "relatorio, continuar" {
			t.Fatalf("status %q", got)
		}
	})

	t.Run("two similar buttons: does not heal", func(t *testing.T) {
		record(t, "parecidos", click(t, "Detalhes"))
		out, isErr := hh.call(t, "flow_run", map[string]any{"name": "parecidos"})
		if !isErr {
			t.Fatalf("ambiguous healing must fail:\n%s", out)
		}
		mustContain(t, out, `2. FAILED click button "Detalhes"`, "two elements match almost equally (ambiguous)",
			`Closest matches: [`, `button "Ver detalhes" (0.`, "confidence ≥ 0.75 and a margin of 0.15")
		if got := status(t); got != "nada" {
			t.Fatalf("nothing should be clicked: %q", got)
		}
	})

	t.Run("sensitive button renamed: asks first", func(t *testing.T) {
		record(t, "sensivel", click(t, "Confirmar"))
		out := hh.ok(t, "flow_run", map[string]any{"name": "sensivel"})
		mustContain(t, out, `2. stopped: click button "Confirmar"`,
			`Healing needed at step 2: was button "Confirmar", best match [`, `button "Confirmar pedido" (confidence 0.`,
			"Ask the user before continuing. To continue, call flow_run with resumeFrom=2 and acceptHealing=true.")
		if got := status(t); got != "nada" {
			t.Fatalf("the sensitive step must not run: %q", got)
		}
		hh.fail(t, "flow_run", map[string]any{"name": "sensivel", "acceptHealing": true}, "acceptHealing needs resumeFrom")
		// Sem acceptHealing, pergunta de novo; com ele, continua do passo 2.
		mustContain(t, hh.ok(t, "flow_run", map[string]any{"name": "sensivel", "resumeFrom": 2}), "Healing needed at step 2")
		// Outra página aberta no meio (a pessoa conferiu outra coisa): retomar volta para onde parou.
		hh.ok(t, "open", map[string]any{"url": hh.url("/pages/form.html")})
		out = hh.ok(t, "flow_run", map[string]any{"name": "sensivel", "resumeFrom": 2, "acceptHealing": true})
		mustContain(t, out, "resuming at step 2", "reopened "+hh.url("/pages/heal/sensivel.html")+" (where the flow stopped)",
			`2. ok (healed) click button "Confirmar"`, "done: 3/3 steps passed.")
		if got := status(t); got != "confirmar, continuar" {
			t.Fatalf("status %q", got)
		}
		hh.fail(t, "flow_run", map[string]any{"name": "sensivel", "resumeFrom": 9}, "resumeFrom must be between 1 and 4")
	})

	t.Run("input changed type: does not heal", func(t *testing.T) {
		record(t, "tipo-input", func(snap string) {
			hh.ok(t, "type", map[string]any{"ref": ref(t, snap, "textbox", "Quantidade"), "text": "3"})
		})
		out, isErr := hh.call(t, "flow_run", map[string]any{"name": "tipo-input"})
		if !isErr {
			t.Fatalf("a field that changed type must not heal:\n%s", out)
		}
		mustContain(t, out, `2. FAILED type textbox "Quantidade"`, "no compatible element on the page")
	})
}

func writeFlow(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
