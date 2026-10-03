package ux

import (
	"testing"

	"pgregory.net/rapid"
)

// Nenhuma sequência de eventos de ferramenta/fluxo tira o selo da pausa: só "Retomar" faz isso.
func TestPropertyOnlyResumeLeavesPause(t *testing.T) {
	nonResume := []EventKind{ToolStarted, ToolFinished, FlowStep, FlowEnded, UserPause, WaitUser, Collapse, Expand}
	rapid.Check(t, func(t *rapid.T) {
		start := rapid.SampledFrom([]Event{{Kind: UserPause}, {Kind: WaitUser, Reason: "senha"}}).Draw(t, "start")
		s := Next(State{Mode: Idle}, start)
		events := rapid.SliceOf(rapid.SampledFrom(nonResume)).Draw(t, "events")
		for i, k := range events {
			s = Next(s, Event{Kind: k, FlowName: "f", Step: 1, Total: 2, Reason: "r"})
			if !s.Blocked() {
				t.Fatalf("event %d (%s) left the pause: %+v", i, k, s)
			}
		}
		resume := rapid.SampledFrom([]EventKind{UserResume, UserToggle}).Draw(t, "resume")
		if s = Next(s, Event{Kind: resume}); s.Blocked() {
			t.Fatalf("%s must leave the pause: %+v", resume, s)
		}
	})
}
