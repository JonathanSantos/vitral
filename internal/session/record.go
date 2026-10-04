package session

import (
	"context"
	"fmt"

	"github.com/go-rod/rod"

	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/snapshot"
)

// Recorder expõe os passos gravados (usado por flow_save).
func (s *Session) Recorder() *flow.Recorder { return s.rec }

// selectorFor calcula o seletor semântico do alvo com um snapshot completo da página.
func (s *Session) selectorFor(ctx context.Context, p *rod.Page, rec *recording) (*flow.Selector, string) {
	if rec == nil || rec.ref == "" || s.replaying.Load() {
		return nil, ""
	}
	page, err := s.capture(ctx, snapshot.Options{Full: true})
	if err != nil {
		return nil, err.Error()
	}
	sel, err := flow.Describe(page, rec.ref)
	if err != nil {
		return nil, err.Error()
	}
	return &sel, ""
}

// record grava o passo e devolve a linha "flow step #N" (ou o motivo de não ter gravado).
func (s *Session) record(rec *recording, sel *flow.Selector, note string) string {
	if rec == nil || s.replaying.Load() {
		return ""
	}
	if rec.ref != "" && sel == nil {
		return fmt.Sprintf("(not recorded for flows: %s)", note)
	}
	return fmt.Sprintf("flow step #%d", s.rec.Add(rec.step(sel)))
}

// Record grava um passo vindo de fora da sessão (ex.: mock_add). Devolve vazio durante flow_run.
func (s *Session) Record(step flow.Step) string {
	return s.record(&recording{step: func(*flow.Selector) flow.Step { return step }}, nil, "")
}
