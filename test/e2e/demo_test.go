//go:build e2e && demo

package e2e

import (
	"context"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// Roteiro do make demo-cursor (docs/demo.md): janela visível, cursor em slow, na cursor-playground.
// Serve para avaliar se o movimento "parece humano" e para gravar o GIF do README.
func TestDemoCursor(t *testing.T) {
	s := start(t, "--cursor-speed", "slow", "--cursor", "force", "--highlight-ms", "350",
		"--cursor-seed", envOr("VITRAL_DEMO_SEED", "1"))
	ctx := context.Background()
	step := func(tool string, args map[string]any) string {
		t.Helper()
		text, isErr, err := s.Call(ctx, tool, args)
		if err != nil || isErr {
			t.Fatalf("%s %v: %v %s", tool, args, err, text)
		}
		time.Sleep(700 * time.Millisecond) // respiro entre ações, para quem assiste
		return text
	}
	refOf := func(role, name string) string {
		t.Helper()
		r, err := refIn(s.history(), role, name)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	step("open", map[string]any{"url": site.URL + "/pages/cursor-playground.html"})
	step("click", map[string]any{"ref": refOf("button", "Continuar")})
	step("click", map[string]any{"ref": refOf("checkbox", "Aceito os termos")})
	step("click", map[string]any{"ref": refOf("link", "política de privacidade")})
	step("type", map[string]any{"ref": refOf("textbox", "Mensagem"), "text": "Olá! Aqui é o Vitral.", "submit": false})
	step("select", map[string]any{"ref": refOf("combobox", "Cidade"), "option": "Porto Alegre"})
	step("click", map[string]any{"ref": refOf("button", "Pedido 1042")})
	step("click", map[string]any{"ref": refOf("button", "Salvar")})
	step("press", map[string]any{"key": "Tab"})
	step("snapshot", map[string]any{"full": true})
	step("click", map[string]any{"ref": refOf("button", "Voltar ao topo")})
	step("scroll", map[string]any{"direction": "up", "amount": 1200})
}

// Roteiro de rolagem do make demo-cursor: 0,5, 1, 3, 5 e 10 telas na scroll-playground, com uma
// pausa entre elas, mais os casos especiais (alvo longe, lista aninhada, scroller que só aceita roda).
func TestDemoScroll(t *testing.T) {
	s := start(t, "--cursor-speed", "normal", "--cursor", "force", "--highlight-ms", "350",
		"--cursor-seed", envOr("VITRAL_DEMO_SEED", "1"))
	ctx := context.Background()
	step := func(tool string, args map[string]any) string {
		t.Helper()
		text, isErr, err := s.Call(ctx, tool, args)
		if err != nil || isErr {
			t.Fatalf("%s %v: %v %s", tool, args, err, text)
		}
		time.Sleep(900 * time.Millisecond)
		return text
	}
	page := site.URL + "/pages/scroll-playground.html"
	snap := step("open", map[string]any{"url": page})
	m := regexp.MustCompile(`viewport: \d+x(\d+)`).FindStringSubmatch(snap)
	if m == nil {
		t.Fatalf("no viewport in:\n%s", snap)
	}
	vh, _ := strconv.Atoi(m[1])
	for _, screens := range []float64{0.5, 1, 3, 5, 10} {
		step("open", map[string]any{"url": page}) // volta ao topo para cada distância
		step("scroll", map[string]any{"direction": "down", "amount": int(screens * float64(vh))})
	}
	step("open", map[string]any{"url": page})
	full := step("snapshot", map[string]any{"full": true})
	refOf := func(label string) string {
		m := regexp.MustCompile(`\[(e\d+)\] button "` + label + `"`).FindStringSubmatch(full)
		if m == nil {
			t.Fatalf("no %q", label)
		}
		return m[1]
	}
	step("click", map[string]any{"ref": refOf("Alvo bem longe")})
	step("open", map[string]any{"url": page})
	full = step("snapshot", map[string]any{"full": true})
	step("click", map[string]any{"ref": refOf("Alvo no fim da lista")})
	step("click", map[string]any{"ref": refOf("Alvo da roda")})
}
