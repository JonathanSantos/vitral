package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jors/browser-mcp/internal/flow"
	"github.com/jors/browser-mcp/internal/snapshot"
)

// MockAdder registra um mock (nil quando o mock está desabilitado).
type MockAdder func(flow.MockStep) error

// FlowError é a falha de um flow_run: relatório até o passo, motivo e snapshot atual.
type FlowError struct{ Report string }

func (e *FlowError) Error() string { return e.Report }

// RunFlow executa os passos sem IA, resolvendo cada seletor no snapshot atual.
func (s *Session) RunFlow(ctx context.Context, f flow.Flow, values map[string]string, addMock MockAdder) (string, error) {
	steps, err := flow.Bind(f, values)
	if err != nil {
		return "", err
	}
	// Passos de um replay não entram na gravação: o fluxo já existe.
	s.replaying.Store(true)
	defer s.replaying.Store(false)

	header := fmt.Sprintf("flow %s%s: %d steps", f.Name, describeParams(f.Params, values), len(steps))
	lines := []string{header}
	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := s.runStep(ctx, step, addMock); err != nil {
			lines = append(lines, fmt.Sprintf("%d. FAILED %s", i+1, step.Describe()), "reason: "+err.Error())
			return "", &FlowError{Report: s.withSnapshot(ctx, lines, fmt.Sprintf("stopped at step %d of %d.", i+1, len(steps)))}
		}
		lines = append(lines, fmt.Sprintf("%d. ok %s", i+1, step.Describe()))
	}
	return s.withSnapshot(ctx, lines, fmt.Sprintf("done: %d/%d steps passed.", len(steps), len(steps))), nil
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

func (s *Session) runStep(ctx context.Context, step flow.Step, addMock MockAdder) error {
	var err error
	switch step.Kind() {
	case "open":
		_, err = s.Open(ctx, step.Open)
	case "click":
		err = s.withRef(ctx, *step.Click, func(ref string) error {
			_, err := s.Click(ctx, ref)
			return err
		})
	case "type":
		t := step.Type
		err = s.withRef(ctx, t.Selector, func(ref string) error {
			_, err := s.Type(ctx, ref, t.Text, t.Clear == nil || *t.Clear, t.Submit)
			return err
		})
	case "select":
		err = s.withRef(ctx, step.Select.Selector, func(ref string) error {
			_, err := s.Select(ctx, ref, step.Select.Option)
			return err
		})
	case "press":
		_, err = s.Press(ctx, step.Press)
	case "scroll":
		err = s.runScroll(ctx, *step.Scroll)
	case "back":
		_, err = s.Back(ctx)
	case "wait_for":
		err = s.runWait(ctx, *step.WaitFor)
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

func (s *Session) runScroll(ctx context.Context, sc flow.ScrollStep) error {
	if sc.Selector == nil {
		_, err := s.Scroll(ctx, ScrollOptions{Direction: sc.Direction, Amount: sc.Amount})
		return err
	}
	return s.withRef(ctx, *sc.Selector, func(ref string) error {
		_, err := s.Scroll(ctx, ScrollOptions{Ref: ref})
		return err
	})
}

func (s *Session) runWait(ctx context.Context, w flow.WaitStep) error {
	timeout := time.Duration(w.TimeoutMs) * time.Millisecond
	if w.Element == nil {
		_, err := s.WaitFor(ctx, WaitOptions{Text: w.Text, Gone: w.Gone, Timeout: timeout})
		return err
	}
	// Esperar um elemento aparecer é o próprio resolve com espera.
	ref, err := s.resolve(ctx, *w.Element, timeout)
	if err != nil {
		return err
	}
	if !w.Gone {
		return nil
	}
	_, err = s.WaitFor(ctx, WaitOptions{Ref: ref, Gone: true, Timeout: timeout})
	return err
}

// withRef resolve o seletor (esperando o elemento aparecer) e executa a ação com a ref atual.
func (s *Session) withRef(ctx context.Context, sel flow.Selector, fn func(ref string) error) error {
	ref, err := s.resolve(ctx, sel, 0)
	if err != nil {
		return err
	}
	return fn(ref)
}

// resolve tenta achar o seletor até o timeout, porque a página pode ainda estar renderizando.
func (s *Session) resolve(ctx context.Context, sel flow.Selector, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = s.cfg.StabilityTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		ref, err := s.resolveOnce(ctx, sel)
		if err == nil || time.Now().After(deadline) {
			return ref, err
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (s *Session) resolveOnce(ctx context.Context, sel flow.Selector) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.page()
	if err != nil {
		return "", err
	}
	page, err := capture(ctx, p, snapshot.Options{Full: true})
	if err != nil {
		return "", err
	}
	return flow.Resolve(page, sel)
}
