package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/heal"
	"github.com/JonathanSantos/vitral/internal/snapshot"
	"github.com/JonathanSantos/vitral/internal/ux"
)

// MockAdder registra um mock (nil quando o mock está desabilitado).
type MockAdder func(flow.MockStep) error

// FlowError é a falha de um flow_run: relatório até o passo, motivo e snapshot atual.
type FlowError struct{ Report string }

func (e *FlowError) Error() string { return e.Report }

// RunOptions continuam um flow_run parado: do passo ResumeFrom (1 = início), aceitando a autocura
// desse passo quando AcceptHealing (a pessoa confirmou).
type RunOptions struct {
	ResumeFrom    int
	AcceptHealing bool
}

// HealedStep é um passo curado na última execução de um fluxo (para o flow_update).
type HealedStep struct {
	Step    int // número do passo, a partir de 1
	Healing flow.Healing
}

// flowRun é o estado da autocura durante um flow_run.
type flowRun struct {
	mode      string
	step      int // passo em execução (a partir de 1)
	accept    int // passo cuja autocura a pessoa aceitou (resumeFrom + acceptHealing)
	lastClick int
	healed    []HealedStep
}

// needsConfirm para o flow_run num passo que precisa de autocura e de confirmação.
type needsConfirm struct{ healing *flow.Healing }

func (e needsConfirm) Error() string { return "healing needs confirmation" }

// allow aplica a política de autocura do fluxo: off falha; passo sensível (pelo léxico ou o último
// clique) ou heal: ask param e pedem confirmação, a não ser que a pessoa já tenha aceitado.
func (r *flowRun) allow(h *flow.Healing, click bool) error {
	if r.mode == flow.HealOff {
		return fmt.Errorf("%s needs healing, but the flow has heal: off (%s)", h.Was.Describe(), h.Describe())
	}
	sensitive := heal.Sensitive(h.Was.Name, h.Now.Name) || click && r.step == r.lastClick
	if (sensitive || r.mode == flow.HealAsk) && r.accept != r.step {
		return needsConfirm{healing: h}
	}
	r.healed = append(r.healed, HealedStep{Step: r.step, Healing: *h})
	return nil
}

// LastHealing devolve os passos curados na última execução do fluxo.
func (s *Session) LastHealing(name string) []HealedStep {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healed[name]
}

// ForgetHealing esquece a autocura de um fluxo (depois do flow_update).
func (s *Session) ForgetHealing(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.healed, name)
}

// RunFlow executa os passos sem IA, resolvendo cada seletor no snapshot atual (com autocura).
func (s *Session) RunFlow(ctx context.Context, f flow.Flow, values map[string]string, addMock MockAdder, opts RunOptions) (string, error) {
	steps, err := flow.Bind(f, values)
	if err != nil {
		return "", err
	}
	start := max(opts.ResumeFrom, 1)
	if start > len(steps) {
		return "", fmt.Errorf("resumeFrom must be between 1 and %d", len(steps))
	}
	// Passos de um replay não entram na gravação: o fluxo já existe.
	s.replaying.Store(true)
	defer s.replaying.Store(false)

	run := &flowRun{mode: f.Heal}
	if opts.AcceptHealing {
		run.accept = start
	}
	for i, st := range steps {
		if st.Click != nil {
			run.lastClick = i + 1
		}
	}
	defer s.keepHealing(f.Name, start, run)

	header := fmt.Sprintf("flow %s%s: %d steps", f.Name, describeParams(f.Params, values), len(steps))
	if start > 1 {
		header += fmt.Sprintf(", resuming at step %d", start)
	}
	lines := []string{header}
	defer s.ui.FlowEnded()
	for i, step := range steps {
		if i+1 < start {
			continue
		}
		run.step = i + 1
		if err := ctx.Err(); err != nil {
			return "", err
		}
		// A pausa vale entre passos: o passo em andamento termina, o próximo não começa.
		if s.ui.Blocked() {
			lines = append(lines, fmt.Sprintf(ux.FlowPausedMessage, f.Name, i+1, len(steps)))
			return strings.Join(lines, "\n"), nil
		}
		s.ui.FlowProgress(f.Name, i+1, len(steps))
		healedBefore := len(run.healed)
		if err := s.runStep(ctx, step, addMock, run); err != nil {
			var confirm needsConfirm
			if errors.As(err, &confirm) {
				h := confirm.healing
				lines = append(lines, fmt.Sprintf("%d. stopped: %s", i+1, step.Describe()))
				lines = append(lines, healedReport(f.Name, run.healed)...)
				return s.withSnapshot(ctx, lines, fmt.Sprintf("Healing needed at step %d: was %s, best match [%s] %s (confidence %.2f). Ask the user before continuing. To continue, call flow_run with resumeFrom=%d and acceptHealing=true.",
					i+1, h.Was.Describe(), h.Ref, h.Now.Describe(), h.Confidence, i+1)), nil
			}
			lines = append(lines, fmt.Sprintf("%d. FAILED %s", i+1, step.Describe()), "reason: "+err.Error())
			lines = append(lines, healedReport(f.Name, run.healed)...)
			return "", &FlowError{Report: s.withSnapshot(ctx, lines, fmt.Sprintf("stopped at step %d of %d.", i+1, len(steps)))}
		}
		// Um passo pode entregar o controle à pessoa (ex.: campo de senha): ele não foi feito.
		if s.ui.State().Mode == ux.Waiting {
			lines = append(lines, fmt.Sprintf("%d. waiting for the user: %s", i+1, step.Describe()),
				fmt.Sprintf(ux.FlowPausedMessage, f.Name, i+1, len(steps)))
			return strings.Join(lines, "\n"), nil
		}
		status := "ok"
		if len(run.healed) > healedBefore {
			status = "ok (healed)"
		}
		lines = append(lines, fmt.Sprintf("%d. %s %s", i+1, status, step.Describe()))
	}
	lines = append(lines, healedReport(f.Name, run.healed)...)
	return s.withSnapshot(ctx, lines, fmt.Sprintf("done: %d/%d steps passed.", len(steps)-start+1, len(steps)-start+1)), nil
}

// keepHealing guarda os passos curados para o flow_update. Uma execução retomada continua a anterior:
// os passos de antes do ponto de retomada ficam.
func (s *Session) keepHealing(name string, start int, run *flowRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.healed == nil {
		s.healed = map[string][]HealedStep{}
	}
	var kept []HealedStep
	if start > 1 {
		for _, h := range s.healed[name] {
			if h.Step < start {
				kept = append(kept, h)
			}
		}
	}
	kept = append(kept, run.healed...)
	if len(kept) > 0 {
		s.healed[name] = kept
	} else {
		delete(s.healed, name)
	}
}

// healedReport lista os passos curados e orienta a perguntar antes de atualizar o fluxo.
func healedReport(name string, healed []HealedStep) []string {
	if len(healed) == 0 {
		return nil
	}
	out := []string{"healed steps (the page changed; the flow file was not changed):"}
	for _, h := range healed {
		out = append(out, fmt.Sprintf("  step %d: %s", h.Step, h.Healing.Describe()))
	}
	return append(out, fmt.Sprintf("Ask the user whether to update the flow with these changes; if yes, call flow_update with name=%s and applyHealing=true.", name))
}

func describeParams(names []string, values map[string]string) string {
	if len(names) == 0 {
		return ""
	}
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s=%q", n, values[n]))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func (s *Session) withSnapshot(ctx context.Context, lines []string, summary string) string {
	lines = append(lines, summary)
	snap, err := s.Snapshot(ctx, SnapshotOptions{})
	if err != nil {
		snap = "(snapshot failed: " + err.Error() + ")"
	}
	return strings.Join(lines, "\n") + "\n\ncurrent page:\n" + snap
}

func (s *Session) runStep(ctx context.Context, step flow.Step, addMock MockAdder, run *flowRun) error {
	var err error
	switch step.Kind() {
	case "open":
		_, err = s.Open(ctx, step.Open)
	case "click":
		err = s.withRef(ctx, run, true, *step.Click, func(ref string) error {
			_, err := s.Click(ctx, ref)
			return err
		})
	case "type":
		t := step.Type
		err = s.withRef(ctx, run, false, t.Selector, func(ref string) error {
			_, err := s.Type(ctx, ref, t.Text, t.Clear == nil || *t.Clear, t.Submit)
			return err
		})
	case "select":
		err = s.withRef(ctx, run, false, step.Select.Selector, func(ref string) error {
			_, err := s.Select(ctx, ref, step.Select.Option)
			return err
		})
	case "press":
		_, err = s.Press(ctx, step.Press)
	case "scroll":
		err = s.runScroll(ctx, *step.Scroll, run)
	case "back":
		_, err = s.Back(ctx)
	case "wait_for":
		err = s.runWait(ctx, *step.WaitFor, run)
	case "mock_add":
		if addMock == nil {
			return errors.New("mocks are disabled (--disable-mock)")
		}
		err = addMock(*step.Mock)
	case "emulate":
		_, err = s.Emulate(ctx, step.Emulate.Device, step.Emulate.Network)
	default:
		err = errors.New("invalid step")
	}
	return err
}

func (s *Session) runScroll(ctx context.Context, sc flow.ScrollStep, run *flowRun) error {
	if sc.Selector == nil {
		_, err := s.Scroll(ctx, ScrollOptions{Direction: sc.Direction, Amount: sc.Amount})
		return err
	}
	return s.withRef(ctx, run, false, *sc.Selector, func(ref string) error {
		_, err := s.Scroll(ctx, ScrollOptions{Ref: ref})
		return err
	})
}

func (s *Session) runWait(ctx context.Context, w flow.WaitStep, run *flowRun) error {
	timeout := time.Duration(w.TimeoutMs) * time.Millisecond
	if w.Element == nil {
		_, err := s.WaitFor(ctx, WaitOptions{Text: w.Text, Gone: w.Gone, Timeout: timeout})
		return err
	}
	// Esperar um elemento aparecer é o próprio resolve com espera.
	ref, h, err := s.resolve(ctx, *w.Element, timeout)
	if err != nil {
		return err
	}
	if h != nil {
		if err := run.allow(h, false); err != nil {
			return err
		}
	}
	if !w.Gone {
		return nil
	}
	_, err = s.WaitFor(ctx, WaitOptions{Ref: ref, Gone: true, Timeout: timeout})
	return err
}

// withRef resolve o seletor (esperando o elemento aparecer), aplica a política de autocura e executa
// a ação com a ref atual.
func (s *Session) withRef(ctx context.Context, run *flowRun, click bool, sel flow.Selector, fn func(ref string) error) error {
	ref, h, err := s.resolve(ctx, sel, 0)
	if err != nil {
		return err
	}
	if h != nil {
		if err := run.allow(h, click); err != nil {
			return err
		}
		sel = h.Now
	}
	// O passo de um fluxo alimenta o mapa com o alvo do seletor (o replay não recalcula seletores).
	if s.smap != nil && sel.Sig != nil {
		s.smap.hint.Store(sel.Sig)
		defer s.smap.hint.Store(nil)
	}
	return fn(ref)
}

// healAfter é quanto o seletor gravado precisa falhar antes da autocura entrar: a página pode ainda
// estar renderizando o elemento certo, e um parecido não pode ganhar dele.
const healAfter = time.Second

// resolve tenta achar o seletor até o timeout, porque a página pode ainda estar renderizando.
func (s *Session) resolve(ctx context.Context, sel flow.Selector, timeout time.Duration) (string, *flow.Healing, error) {
	if timeout <= 0 {
		timeout = s.cfg.StabilityTimeout
	}
	start := time.Now()
	deadline := start.Add(timeout)
	for {
		allowHeal := time.Since(start) >= min(healAfter, timeout) || time.Now().After(deadline)
		ref, h, err := s.resolveOnce(ctx, sel, allowHeal)
		if err == nil || time.Now().After(deadline) {
			return ref, h, err
		}
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (s *Session) resolveOnce(ctx context.Context, sel flow.Selector, allowHeal bool) (string, *flow.Healing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.page(); err != nil {
		return "", nil, err
	}
	page, err := s.capture(ctx, snapshot.Options{Full: true})
	if err != nil {
		return "", nil, err
	}
	return flow.Resolve(page, sel, allowHeal)
}
