package fakeagent

import (
	"context"
	"strings"
	"testing"
)

const formSnapshot = `page: "Cadastro" (http://x/form.html)
viewport: 1280x713, scroll: 0/0
[e3] main
  [e4] form "Cadastro"
    [e5] textbox "Nome" value="" required
    [e8] combobox "País" value="Selecione" options=3
      [e9] option "Brasil"
    [e13] checkbox "Aceito os termos"
    [e15] button "Enviar"`

func TestViewAbsorb(t *testing.T) {
	v := NewView()
	v.Absorb(formSnapshot)
	if el := v.Find("nome", "textbox"); el == nil || el.Ref != "e5" || el.Parent != "e4" {
		t.Fatalf("textbox: %+v", el)
	}
	v.Absorb(`+ [e20] dialog "Aviso" open modal
+   [e21] button "Fechar"
~ [e5] textbox "Nome" value="Ana"
+ [e22] alert "Salvo" (in form e4)`)
	if kids := v.Children("e20"); len(kids) != 1 || kids[0].Name != "Fechar" {
		t.Fatalf("dialog children: %+v", kids)
	}
	if el := v.Find("Salvo", "alert"); el == nil || el.Parent != "e4" {
		t.Fatalf("alert parent: %+v", el)
	}
	if !strings.Contains(v.Find("Nome", "textbox").Line, `value="Ana"`) {
		t.Fatal("~ should update the line")
	}
	v.Absorb(`- [e20] dialog "Aviso" open modal (+1 inside)
- 3 elements left the viewport`)
	if v.Find("Fechar", "button") != nil || v.Find("Aviso", "dialog") != nil {
		t.Fatal("removed dialog and its children must disappear")
	}
	v.Absorb("found \"x\" in [e40] status \"x\"\npage: \"Nova\" (http://x/)\nviewport: 1x1, scroll: 0/0\n[e1] button \"Só este\"")
	if v.Find("Enviar", "button") != nil || v.Find("Só este", "button") == nil {
		t.Fatal("a full snapshot replaces the view")
	}
}

// script responde cada chamada com uma função; registra as chamadas feitas.
type script struct {
	calls []string
	reply func(n int, tool string, args map[string]any) (string, bool)
}

func (s *script) Call(_ context.Context, tool string, args map[string]any) (string, bool, error) {
	s.calls = append(s.calls, tool)
	text, isErr := s.reply(len(s.calls), tool, args)
	return text, isErr, nil
}

func goal(t *testing.T, y string) Goal {
	t.Helper()
	g, err := ParseGoal([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

const formGoal = `goal:
  open: http://x/form.html
  fill: { "Nome": "Maria", "País": "Brasil", "Aceito os termos": "sim" }
  click: "Enviar"
  until: { text: "Cadastro enviado" }`

func TestRunHappyPathKeepsFieldOrder(t *testing.T) {
	s := &script{reply: func(_ int, tool string, _ map[string]any) (string, bool) {
		if tool == "open" {
			return formSnapshot, false
		}
		return "no visible changes", false
	}}
	res, err := New(s, 20).Run(context.Background(), goal(t, formGoal))
	if err != nil {
		t.Fatal(err)
	}
	want := "open,type,select,click,click,wait_for"
	if strings.Join(s.calls, ",") != want || res.Outcome != Done || res.Steps != 6 || res.Tokens == 0 {
		t.Fatalf("calls %v outcome %s steps %d", s.calls, res.Outcome, res.Steps)
	}
}

func TestRecoversOnlyByFollowingErrorInstructions(t *testing.T) {
	tests := []struct {
		name    string
		failing string // resposta de erro na primeira tentativa do click
		after   string // o que a ferramenta devolve depois da instrução
		calls   string
		wantErr string
	}{
		{
			name:    "stale ref: Call snapshot",
			failing: "ref e15 not found: page changed since last snapshot. Call snapshot.",
			after:   strings.Replace(formSnapshot, "[e15] button", "[e30] button", 1),
			calls:   "open,click,snapshot,click",
		},
		{
			name:    "covered: Close it first",
			failing: "element e15 is covered by dialog e20. Close it first.",
			calls:   "open,click,click,click",
		},
		{
			name:    "message without instruction",
			failing: "something went wrong",
			wantErr: "error is not actionable: something went wrong",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clicks := 0
			s := &script{reply: func(_ int, tool string, args map[string]any) (string, bool) {
				switch tool {
				case "open":
					return formSnapshot + "\n[e20] dialog \"Aviso\" open\n  [e21] button \"Fechar\"", false
				case "snapshot":
					return tt.after, false
				case "click":
					clicks++
					if clicks == 1 {
						return tt.failing, true
					}
				}
				return "no visible changes", false
			}}
			_, err := New(s, 20).Run(context.Background(), goal(t, "open: http://x\nclick: Enviar"))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(s.calls, ","); got != tt.calls {
				t.Fatalf("calls = %s, want %s", got, tt.calls)
			}
		})
	}
}

func TestPauseHandsToUserAndStepLimit(t *testing.T) {
	s := &script{reply: func(_ int, tool string, _ map[string]any) (string, bool) {
		if tool == "open" {
			return formSnapshot, false
		}
		return "Waiting for the user: element e5 is a password field...", false
	}}
	res, err := New(s, 10).Run(context.Background(), goal(t, formGoal))
	if err != nil || res.Outcome != HandedToUser {
		t.Fatalf("outcome %s err %v", res.Outcome, err)
	}

	loop := &script{reply: func(int, string, map[string]any) (string, bool) { return "page: \"x\" (y)\n", false }}
	if _, err := New(loop, 3).Run(context.Background(), goal(t, formGoal)); err == nil || !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "step limit") {
		t.Fatalf("expected failure, got %v", err)
	}
	limit := &script{reply: func(int, string, map[string]any) (string, bool) { return formSnapshot, false }}
	if _, err := New(limit, 2).Run(context.Background(), goal(t, formGoal)); err == nil || !strings.Contains(err.Error(), "step limit of 2") {
		t.Fatalf("expected step limit, got %v", err)
	}
}
