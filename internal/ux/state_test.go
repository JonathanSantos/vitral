package ux

import "testing"

func run(events ...Event) State {
	var s State
	s.Mode = Idle
	for _, e := range events {
		s = Next(s, e)
	}
	return s
}

func TestNext(t *testing.T) {
	hold := &Highlight{Ref: "e7", Box: Box{1, 2, 3, 4}}
	tests := []struct {
		name   string
		events []Event
		mode   Mode
		label  string
	}{
		{"starts idle", nil, Idle, TextIdle},
		{"tool runs", []Event{{Kind: ToolStarted}}, Working, TextWorking},
		{"tool ends", []Event{{Kind: ToolStarted}, {Kind: ToolFinished}}, Idle, TextIdle},
		{"parallel tools keep working", []Event{{Kind: ToolStarted}, {Kind: ToolStarted}, {Kind: ToolFinished}}, Working, TextWorking},
		{"flow progress", []Event{{Kind: ToolStarted}, {Kind: FlowStep, FlowName: "busca", Step: 2, Total: 4}}, Flow, "Executando fluxo: busca · passo 2 de 4"},
		{"flow end returns to working", []Event{{Kind: ToolStarted}, {Kind: FlowStep, FlowName: "x", Step: 1, Total: 1}, {Kind: FlowEnded}}, Working, TextWorking},
		{"pause while idle", []Event{{Kind: UserPause}}, Paused, TextPaused},
		{"pause survives tool end", []Event{{Kind: ToolStarted}, {Kind: UserPause}, {Kind: ToolFinished}}, Paused, TextPaused},
		{"pause ignores flow steps", []Event{{Kind: UserPause}, {Kind: FlowStep, FlowName: "x", Step: 2, Total: 3}}, Paused, TextPaused},
		{"toggle pauses", []Event{{Kind: UserToggle}}, Paused, TextPaused},
		{"toggle resumes", []Event{{Kind: UserToggle}, {Kind: UserToggle}}, Idle, TextIdle},
		{"resume during a tool goes back to working", []Event{{Kind: ToolStarted}, {Kind: UserPause}, {Kind: UserResume}}, Working, TextWorking},
		{"resume when not paused is a no-op", []Event{{Kind: UserResume}}, Idle, TextIdle},
		{"wait user", []Event{{Kind: ToolStarted}, {Kind: WaitUser, Reason: ReasonPassword, Hold: hold}}, Waiting, "Sua vez: " + ReasonPassword},
		{"wait user then tool end stays waiting", []Event{{Kind: ToolStarted}, {Kind: WaitUser, Reason: "x"}, {Kind: ToolFinished}}, Waiting, "Sua vez: x"},
		{"resume from waiting", []Event{{Kind: WaitUser, Reason: "x", Hold: hold}, {Kind: UserToggle}}, Idle, TextIdle},
		{"explicit pause wins over wait", []Event{{Kind: UserPause}, {Kind: WaitUser, Reason: "x"}}, Paused, TextPaused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := run(tt.events...)
			if s.Mode != tt.mode || Label(s) != tt.label {
				t.Fatalf("mode=%s label=%q, want %s %q", s.Mode, Label(s), tt.mode, tt.label)
			}
			if s.Blocked() != (tt.mode == Paused || tt.mode == Waiting) {
				t.Fatal("Blocked mismatch")
			}
		})
	}
}

func TestResumeClearsHoldAndCollapseIsIndependent(t *testing.T) {
	s := run(Event{Kind: Collapse}, Event{Kind: WaitUser, Reason: "x", Hold: &Highlight{Ref: "e1"}})
	if s.Hold == nil || !s.Collapsed {
		t.Fatalf("unexpected %+v", s)
	}
	s = Next(s, Event{Kind: UserResume})
	if s.Hold != nil || s.Reason != "" || !s.Collapsed {
		t.Fatalf("resume should clear the hold and keep collapsed: %+v", s)
	}
	if s = Next(s, Event{Kind: Expand}); s.Collapsed {
		t.Fatal("expand failed")
	}
	if ButtonLabel(run(Event{Kind: UserPause})) != TextResume || ButtonLabel(run()) != TextPause {
		t.Fatal("button label")
	}
}

func TestNavNote(t *testing.T) {
	s := Next(State{Mode: Idle}, Event{Kind: ToolStarted})
	s = Next(s, Event{Kind: NavStarted, Reason: "Abrindo exemplo.com"})
	if Label(s) != "Abrindo exemplo.com" {
		t.Fatalf("label %q", Label(s))
	}
	if s2 := Next(s, Event{Kind: NavEnded}); Label(s2) != TextWorking || s2.Note != "" {
		t.Fatalf("nav ended: %q", Label(s2))
	}
	if s3 := Next(s, Event{Kind: ToolFinished}); s3.Note != "" || s3.Mode != Idle {
		t.Fatalf("tool finished clears the note: %+v", s3)
	}
	// Com outra ferramenta ainda rodando, a nota fica.
	two := Next(Next(s, Event{Kind: ToolStarted}), Event{Kind: ToolFinished})
	if two.Note == "" {
		t.Fatal("note stays while a tool is still running")
	}
	paused := Next(State{Mode: Paused}, Event{Kind: NavStarted, Reason: "Voltando"})
	if paused.Note != "" || Label(paused) != TextPaused {
		t.Fatalf("paused ignores navigation notes: %+v", paused)
	}
	// A nota só aparece no modo trabalhando.
	if Label(State{Mode: Idle, Note: "x"}) != TextIdle {
		t.Fatal("idle shows the idle text")
	}
}

func TestTag(t *testing.T) {
	for _, tt := range []struct{ profile, session, want string }{
		{"default", "", ""}, {"", "", ""}, {"hml", "", "perfil hml"}, {"hml", "portal", "sessão portal"}, {"default", "portal", "sessão portal"},
	} {
		if got := Tag(tt.profile, tt.session); got != tt.want {
			t.Errorf("Tag(%q, %q) = %q", tt.profile, tt.session, got)
		}
	}
	if Tagged("Vitral está usando esta janela", "perfil hml") != "Vitral está usando esta janela · perfil hml" || Tagged("x", "") != "x" {
		t.Error("Tagged")
	}
}
