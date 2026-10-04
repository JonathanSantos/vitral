package tools

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/mock"
	"github.com/JonathanSantos/vitral/internal/session"
)

type flowSaveInput struct {
	Name        string            `json:"name" jsonschema:"file name: lowercase letters, digits, - and _"`
	Description string            `json:"description,omitempty" jsonschema:"what the flow does, in one sentence"`
	Params      map[string]string `json:"params,omitempty" jsonschema:"turn typed values into parameters: {name: value used in this session}, e.g. {numero: 1023}"`
	FromStep    int               `json:"fromStep,omitempty" jsonschema:"first step number to save (default: everything since the last flow_save)"`
	ToStep      int               `json:"toStep,omitempty" jsonschema:"last step number to save (default: the last step)"`
	Exclude     []int             `json:"excludeSteps,omitempty" jsonschema:"step numbers (#N) inside the range to leave out, e.g. [12, 14]"`
	Preview     bool              `json:"preview,omitempty" jsonschema:"true: only list the steps that would be saved (number, action, target); nothing is saved"`
	Ignore      bool              `json:"ignoreWarnings,omitempty" jsonschema:"save even with warnings, only when the steps are right as they are"`
	Expect      string            `json:"expect,omitempty" jsonschema:"text that must be visible at the end: adds a final wait_for so flow_run checks the result (a param value inside it becomes the placeholder)"`
}

type flowRunInput struct {
	Name          string            `json:"name" jsonschema:"flow name from flow_list"`
	Params        map[string]string `json:"params,omitempty" jsonschema:"values for the flow parameters, e.g. {numero: 2048}"`
	ResumeFrom    int               `json:"resumeFrom,omitempty" jsonschema:"continue a stopped run from this step number"`
	AcceptHealing bool              `json:"acceptHealing,omitempty" jsonschema:"with resumeFrom: the user approved the healing proposed for that step"`
}

type flowUpdateInput struct {
	Name         string `json:"name" jsonschema:"flow name"`
	ApplyHealing bool   `json:"applyHealing" jsonschema:"true: rewrite the steps healed in the last flow_run with the elements found"`
}

func registerFlows(s *mcp.Server, d Deps) {
	addTool(s, d, &mcp.Tool{
		Name:        "flow_save",
		Description: "Save the actions done so far (each action ends with 'flow step #N') as a reusable flow. Call with preview=true first: it lists the steps it would save (#N, action, target) without saving; pick the range with fromStep/toStep and leave out steps with excludeSteps. Use params to turn typed values into placeholders. Use expect=<text> so flow_run checks the result.",
	}, flowSaveTool(d))

	addTool(s, d, &mcp.Tool{
		Name:        "flow_list",
		Description: "List saved flows with their description and parameters.",
	}, flowListTool(d))

	addTool(s, d, &mcp.Tool{
		Name:        "flow_run",
		Description: "Run a saved flow step by step without AI. If the page changed, it heals steps by finding the same element (asks first on sensitive steps). On failure it stops and shows the step, the reason and the page.",
	}, flowRunTool(d))

	addTool(s, d, &mcp.Tool{
		Name:        "flow_update",
		Description: "After the user agrees, save the steps healed in the last flow_run of this flow into its file (the previous version goes to .history).",
	}, flowUpdateTool(d))
}

func flowSaveTool(d Deps) mcp.ToolHandlerFor[flowSaveInput, any] {
	return textHandler(func(ctx context.Context, in flowSaveInput) (string, error) {
		if err := flow.ValidateName(in.Name); err != nil {
			return "", err
		}
		rec := d.Session.Recorder()
		asPreviewed := false
		// O save logo depois da prévia reaproveita a chamada da prévia no que não for repetido (um
		// modelo confere a prévia e salva só com o nome).
		if sel, ok := rec.LastPreview(in.Name); ok && !in.Preview {
			if in.FromStep == 0 && in.ToStep == 0 && len(in.Exclude) == 0 {
				in.FromStep, in.ToStep, in.Exclude, asPreviewed = sel.From, sel.To, sel.Exclude, true
			}
			if in.Params == nil && sel.Params != nil {
				in.Params, asPreviewed = sel.Params, true
			}
			if in.Expect == "" && sel.Expect != "" {
				in.Expect, asPreviewed = sel.Expect, true
			}
			if in.Description == "" && sel.Description != "" {
				in.Description, asPreviewed = sel.Description, true
			}
		}
		all, from, to, err := rec.Range(in.FromStep, in.ToStep)
		if err != nil {
			return "", err
		}
		picked, err := flow.Pick(all, from, in.Exclude)
		if err != nil {
			return "", err
		}
		steps := make([]flow.Step, len(picked))
		for i, p := range picked {
			steps[i] = p.Step
		}
		// O resultado esperado vira um wait_for no fim; o Parameterize troca o valor do parâmetro por
		// {{nome}} também nele, então o fluxo confere o termo que rodou, não o da gravação.
		if expect := strings.TrimSpace(in.Expect); expect != "" {
			steps = append(steps, flow.Step{WaitFor: &flow.WaitStep{Text: expect}})
		}
		raw := steps
		steps, subs, err := flow.Parameterize(steps, in.Params)
		if err != nil {
			return "", err
		}
		// Avisos (alvo que depende do valor, busca sem Enter no fim, expect que não está na página): na
		// prévia aparecem; no save, impedem salvar um fluxo que o flow_run não conseguiria repetir.
		warn := flow.Warnings(raw, in.Params)
		if expect := strings.TrimSpace(in.Expect); expect != "" {
			if w := flow.ExpectWarnings(expect, in.Params); len(w) > 0 {
				warn = append(warn, w...)
			} else if w := expectWarning(ctx, d, expect, rec.URLAt(picked[len(picked)-1].N)); w != "" {
				warn = append(warn, w)
			}
		}
		if !in.Preview && len(warn) > 0 && !in.Ignore {
			msg := []string{"not saved: the flow would not repeat the task with other values."}
			for _, w := range warn {
				msg = append(msg, "  "+w)
			}
			msg = append(msg, "Fix the steps (call flow_save with preview=true to check them), or, only if the flow is right as it is, save again with ignoreWarnings=true.")
			return "", errors.New(strings.Join(msg, "\n"))
		}
		params := make([]string, 0, len(in.Params))
		for name := range in.Params {
			params = append(params, name)
		}
		slices.Sort(params)

		// Cada linha leva o número da sessão (#N), o mesmo do "flow step #N" das respostas: é por ele que
		// a IA confere a prévia e escolhe o que tirar com excludeSteps.
		describe := func() []string {
			out := make([]string, len(steps))
			for i, s := range steps {
				origin := "(expect)"
				if i < len(picked) {
					origin = fmt.Sprintf("(#%d)", picked[i].N)
				}
				out[i] = fmt.Sprintf("%d. %s %s", i+1, s.Describe(), origin)
			}
			return out
		}
		var lines []string
		if in.Preview {
			rec.RememberPreview(flow.Selection{
				Name: in.Name, From: from, To: to, Exclude: in.Exclude,
				Params: in.Params, Expect: in.Expect, Description: in.Description,
			})
			lines = []string{fmt.Sprintf("preview of flow %s: %d steps from session steps #%d to #%d (nothing saved yet)", in.Name, len(steps), from, to)}
			lines = append(lines, describe()...)
		} else {
			f := flow.Flow{Name: in.Name, Description: in.Description, Params: params, Steps: steps}
			path, replaced, err := d.Flows.Save(f)
			if err != nil {
				return "", err
			}
			rec.MarkSaved(to)
			lines = []string{fmt.Sprintf("saved %s (%d steps, from session steps #%d to #%d)", path, len(steps), from, to)}
			if asPreviewed {
				lines[0] += ", as in the last preview"
			}
			if replaced {
				lines[0] += ", replacing the previous file"
			}
			lines = append(lines, describe()...)
		}
		if last := steps[len(steps)-1]; last.WaitFor == nil {
			if in.Preview {
				lines = append(lines, "tip: this flow would not check its result. Add expect=<text visible at the end> so flow_run confirms it worked.")
			} else {
				lines = append(lines, "tip: this flow does not check its result. Save it again with the same steps and expect=<text visible at the end> so flow_run confirms it worked.")
			}
		}
		if len(subs) > 0 {
			lines = append(lines, "substitutions:")
			for _, s := range subs {
				lines = append(lines, "  "+s.String())
			}
		}
		if len(warn) > 0 {
			lines = append(lines, "warning:")
			for _, w := range warn {
				lines = append(lines, "  "+w)
			}
		}
		if in.Preview {
			lines = append(lines, "Check these steps against the task (leave out extra ones with excludeSteps=[N] or narrow fromStep/toStep), then call flow_save again without preview to save. With only the name it saves exactly this preview (steps, params, expect and description).")
		}
		return strings.Join(lines, "\n"), nil
	})
}

// expectWarning confere o expect na página onde o fluxo termina: o flow_run espera esse texto visível
// (wait_for), e um modelo pequeno às vezes escolhe o título da aba ou o nome de um campo. Só confere
// quando a página atual é a do último passo salvo (endURL); em outra página, não dá para saber.
func expectWarning(ctx context.Context, d Deps, expect, endURL string) string {
	visible, inTitle, current, err := d.Session.TextOnPage(ctx, expect)
	if err != nil || visible || endURL != "" && withoutFragment(current) != withoutFragment(endURL) {
		return ""
	}
	where := "is not in the visible text of the current page"
	if inTitle {
		where = "is only in the page title (the browser tab), not in the visible text of the current page"
	}
	return fmt.Sprintf("expect %q %s. flow_run checks the final page with wait_for, which reads the visible text: if this page is where the flow ends, use text from the page itself (e.g. the main heading).", expect, where)
}

func flowListTool(d Deps) mcp.ToolHandlerFor[noInput, any] {
	return textHandler(func(_ context.Context, _ noInput) (string, error) {
		flows, err := d.Flows.List()
		if err != nil {
			return "", err
		}
		if len(flows) == 0 {
			return fmt.Sprintf("no flows saved in %s yet. Do the actions, then call flow_save.", d.Flows.Dir), nil
		}
		lines := make([]string, 0, len(flows))
		for _, f := range flows {
			line := f.Name
			if f.Description != "" {
				line += " — " + f.Description
			}
			details := []string{fmt.Sprintf("%d steps", len(f.Steps))}
			if len(f.Params) > 0 {
				details = append([]string{"params: " + strings.Join(f.Params, ", ")}, details...)
			}
			lines = append(lines, line+" ("+strings.Join(details, "; ")+")")
		}
		return strings.Join(lines, "\n"), nil
	})
}

func flowRunTool(d Deps) mcp.ToolHandlerFor[flowRunInput, any] {
	return textHandler(func(ctx context.Context, in flowRunInput) (string, error) {
		f, err := d.Flows.Load(in.Name)
		if err != nil {
			return "", err
		}
		if in.AcceptHealing && in.ResumeFrom == 0 {
			return "", errors.New("acceptHealing needs resumeFrom: the step number where flow_run asked for confirmation")
		}
		out, err := d.Session.RunFlow(ctx, f, in.Params, mockAdder(d.Mocks), session.RunOptions{ResumeFrom: in.ResumeFrom, AcceptHealing: in.AcceptHealing})
		var flowErr *session.FlowError
		if errors.As(err, &flowErr) {
			return "", flowErr
		}
		return out, err
	})
}

func mockAdder(reg *mock.Registry) session.MockAdder {
	if reg == nil {
		return nil
	}
	return func(m flow.MockStep) error {
		_, err := reg.Add(mockRule(m))
		return err
	}
}

func mockRule(m flow.MockStep) mock.Rule {
	return mock.Rule{
		URLPattern: m.URLPattern, Method: m.Method, Status: m.Status, Body: m.Body,
		Headers: m.Headers, Fail: m.Fail, DelayMs: m.DelayMs, Times: m.Times,
	}
}

func flowUpdateTool(d Deps) mcp.ToolHandlerFor[flowUpdateInput, any] {
	return textHandler(func(_ context.Context, in flowUpdateInput) (string, error) {
		if !in.ApplyHealing {
			return "", errors.New("nothing to update: call flow_update with applyHealing=true to save the steps healed in the last flow_run")
		}
		f, err := d.Flows.Load(in.Name)
		if err != nil {
			return "", err
		}
		healed := d.Session.LastHealing(in.Name)
		if len(healed) == 0 {
			return "", fmt.Errorf("no healed steps for flow %s: run it with flow_run first", in.Name)
		}
		changes := map[int]flow.Selector{}
		lines := []string{}
		for _, h := range healed {
			changes[h.Step] = h.Healing.Now
			lines = append(lines, fmt.Sprintf("  step %d: %s → %s", h.Step, h.Healing.Was.Describe(), h.Healing.Now.Describe()))
		}
		updated, err := flow.ApplyHealing(f, changes)
		if err != nil {
			return "", err
		}
		path, history, err := d.Flows.Update(updated)
		if err != nil {
			return "", err
		}
		d.Session.ForgetHealing(in.Name)
		head := fmt.Sprintf("updated %s (%d steps changed; previous version in %s)", path, len(healed), history)
		return strings.Join(append([]string{head}, lines...), "\n"), nil
	})
}

func withoutFragment(u string) string {
	before, _, _ := strings.Cut(u, "#")
	return before
}
