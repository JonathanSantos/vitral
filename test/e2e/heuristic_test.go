//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/test/fakeagent"
)

// Cenários do agente heurístico (docs/TESTING.md, seção 7.3). O agente só enxerga o texto das
// ferramentas e só se recupera de erro seguindo a instrução da própria mensagem. Roda sem skill.
func TestHeuristicAgent(t *testing.T) {
	tests := []struct {
		name     string
		goal     string
		maxSteps int
		outcome  fakeagent.Outcome
		// afterCall muda a página "por fora" do agente (ex.: re-render), sem ele ver na resposta.
		afterCall func(s *Session, step int, tool string)
		check     func(t *testing.T, res fakeagent.Result)
	}{
		{
			name: "form",
			goal: `goal:
  open: "{{base}}/pages/form.html"
  fill: { "Nome": "Maria", "E-mail": "maria@exemplo.test", "País": "Brasil", "Aceito os termos": "sim" }
  click: "Enviar"
  until: { text: "Cadastro enviado" }`,
			maxSteps: 12,
			outcome:  fakeagent.Done,
		},
		{
			name: "dialog covering the button",
			goal: `goal:
  open: "{{base}}/pages/dialog.html"
  fill: { "Nome": "Rui" }
  click: "Enviar"
  until: { text: "Mensagem enviada por Rui" }`,
			maxSteps: 12,
			outcome:  fakeagent.Done,
			check: func(t *testing.T, res fakeagent.Result) {
				if !transcriptHas(res, "Close it first") {
					t.Fatal("the agent should have hit the covering dialog and closed it")
				}
			},
		},
		{
			name: "stale ref after re-render",
			goal: `goal:
  open: "{{base}}/pages/rerender.html"
  fill: { "Nome": "Bia" }
  click: "Salvar"
  until: { text: "Salvo: Bia" }`,
			maxSteps: 12,
			outcome:  fakeagent.Done,
			afterCall: func(s *Session, _ int, tool string) {
				if tool == "type" {
					s.mainWorld(`() => window.rerender()`)
				}
			},
			check: func(t *testing.T, res fakeagent.Result) {
				if !transcriptHas(res, "Call snapshot") {
					t.Fatal("the agent should have used a stale ref and recovered via snapshot")
				}
			},
		},
		{
			name: "password field hands over to the user",
			goal: `goal:
  open: "{{base}}/pages/form.html"
  fill: { "Senha": "segredo" }`,
			maxSteps: 6,
			outcome:  fakeagent.HandedToUser,
			check: func(t *testing.T, res fakeagent.Result) {
				if !strings.Contains(res.LastText, `click "Retomar"`) || strings.Contains(res.LastText, "segredo") {
					t.Fatalf("the reply must ask the user to act, without typing:\n%s", res.LastText)
				}
			},
		},
		{
			name: "navigation",
			goal: `goal:
  open: "{{base}}/pages/form.html"
  click: "Link do rodapé"
  until: { text: "Cadastro de cliente" }`,
			maxSteps: 8,
			outcome:  fakeagent.Done,
			check: func(t *testing.T, res fakeagent.Result) {
				if !transcriptHas(res, "bottom=1") {
					t.Fatal("the click should navigate to the footer link")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := start(t)
			g, err := fakeagent.ParseGoal([]byte(strings.ReplaceAll(tt.goal, "{{base}}", site.URL)))
			if err != nil {
				t.Fatal(err)
			}
			agent := fakeagent.New(s, tt.maxSteps)
			if tt.afterCall != nil {
				agent.AfterCall = func(step int, tool string) { tt.afterCall(s, step, tool) }
			}
			res, err := agent.Run(context.Background(), g)
			t.Logf("steps=%d tokens=%d outcome=%s", res.Steps, res.Tokens, res.Outcome)
			if err != nil {
				t.Fatalf("agent failed: %v", err)
			}
			if res.Outcome != tt.outcome {
				t.Fatalf("outcome = %s, want %s", res.Outcome, tt.outcome)
			}
			if tt.check != nil {
				tt.check(t, res)
			}
		})
	}
}

func transcriptHas(res fakeagent.Result, text string) bool {
	for _, e := range res.Transcript {
		if strings.Contains(e.Text, text) {
			return true
		}
	}
	return false
}
