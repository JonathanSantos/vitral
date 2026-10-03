package tools

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jors/browser-mcp/internal/flow"
	"github.com/jors/browser-mcp/internal/mock"
	"github.com/jors/browser-mcp/internal/session"
)

type flowSaveInput struct {
	Name        string            `json:"name" jsonschema:"file name: lowercase letters, digits, - and _"`
	Description string            `json:"description,omitempty" jsonschema:"what the flow does, in one sentence"`
	Params      map[string]string `json:"params,omitempty" jsonschema:"turn typed values into parameters: {name: value used in this session}, e.g. {numero: 1023}"`
	FromStep    int               `json:"fromStep,omitempty" jsonschema:"first step number to save (default: everything since the last flow_save)"`
}

type flowRunInput struct {
	Name   string            `json:"name" jsonschema:"flow name from flow_list"`
	Params map[string]string `json:"params,omitempty" jsonschema:"values for the flow parameters, e.g. {numero: 2048}"`
}

func registerFlows(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_save",
		Description: "Save the actions done so far (each action shows 'step #N') as a reusable flow. Use params to turn typed values into placeholders.",
	}, flowSaveTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_list",
		Description: "List saved flows with their description and parameters.",
	}, flowListTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "flow_run",
		Description: "Run a saved flow step by step without AI. On failure it stops and shows the step, the reason and the page.",
	}, flowRunTool(d))
}

func flowSaveTool(d Deps) mcp.ToolHandlerFor[flowSaveInput, any] {
	return textHandler(func(_ context.Context, in flowSaveInput) (string, error) {
		if err := flow.ValidateName(in.Name); err != nil {
			return "", err
		}
		rec := d.Session.Recorder()
		steps, from, to, err := rec.Range(in.FromStep)
		if err != nil {
			return "", err
		}
		steps, subs, err := flow.Parameterize(steps, in.Params)
		if err != nil {
			return "", err
		}
		params := make([]string, 0, len(in.Params))
		for name := range in.Params {
			params = append(params, name)
		}
		slices.Sort(params)

		f := flow.Flow{Name: in.Name, Description: in.Description, Params: params, Steps: steps}
		path, replaced, err := d.Flows.Save(f)
		if err != nil {
			return "", err
		}
		rec.MarkSaved(to)

		lines := []string{fmt.Sprintf("saved %s (%d steps, from session steps #%d to #%d)", path, len(steps), from, to)}
		if replaced {
			lines[0] += ", replacing the previous file"
		}
		for i, s := range steps {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, s.Describe()))
		}
		if len(subs) > 0 {
			lines = append(lines, "substitutions:")
			for _, s := range subs {
				lines = append(lines, "  "+s.String())
			}
		}
		return strings.Join(lines, "\n"), nil
	})
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
		out, err := d.Session.RunFlow(ctx, f, in.Params, mockAdder(d.Mocks))
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
