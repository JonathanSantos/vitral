package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jors/browser-mcp/internal/mock"
)

type mockAddInput struct {
	URLPattern string            `json:"urlPattern" jsonschema:"URL glob like */api/orders* (text without * matches anywhere in the URL)"`
	Method     string            `json:"method,omitempty" jsonschema:"only this HTTP method (default: any)"`
	Status     int               `json:"status,omitempty" jsonschema:"HTTP status to return (default 200)"`
	Body       string            `json:"body,omitempty" jsonschema:"response body, e.g. a JSON string"`
	Headers    map[string]string `json:"headers,omitempty" jsonschema:"response headers"`
	Fail       string            `json:"fail,omitempty" jsonschema:"simulate a network failure instead: timeout, connection_refused, offline, dns, aborted…"`
	DelayMs    int               `json:"delayMs,omitempty" jsonschema:"wait this long before answering (to see loading states)"`
	Times      int               `json:"times,omitempty" jsonschema:"apply only N times, then expire (default: unlimited)"`
}

type mockClearInput struct {
	ID string `json:"id,omitempty" jsonschema:"mock id like m1; without id removes all mocks"`
}

func registerMock(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "mock_add",
		Description: "Fake an API response or failure for matching requests (e.g. return 500 or slow down). The newest mock wins. Reload or repeat the action to see the effect.",
	}, mockAddTool(d.Mocks))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "mock_list",
		Description: "List active mocks and how many times each was used.",
	}, mockListTool(d.Mocks))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "mock_clear",
		Description: "Remove one mock by id, or all mocks when no id is given.",
	}, mockClearTool(d.Mocks))
}

func mockAddTool(reg *mock.Registry) mcp.ToolHandlerFor[mockAddInput, any] {
	return textHandler(func(_ context.Context, in mockAddInput) (string, error) {
		rule, err := reg.Add(mock.Rule{
			URLPattern: in.URLPattern, Method: in.Method, Status: in.Status, Body: in.Body,
			Headers: in.Headers, Fail: in.Fail, DelayMs: in.DelayMs, Times: in.Times,
		})
		if err != nil {
			return "", err
		}
		return "mock added: " + mock.Summary(rule), nil
	})
}

func mockListTool(reg *mock.Registry) mcp.ToolHandlerFor[noInput, any] {
	return textHandler(func(_ context.Context, _ noInput) (string, error) {
		rules := reg.List()
		if len(rules) == 0 {
			return "no active mocks", nil
		}
		lines := []string{fmt.Sprintf("%d active mocks (checked in this order):", len(rules))}
		for _, r := range rules {
			lines = append(lines, mock.Summary(r))
		}
		return strings.Join(lines, "\n"), nil
	})
}

func mockClearTool(reg *mock.Registry) mcp.ToolHandlerFor[mockClearInput, any] {
	return textHandler(func(_ context.Context, in mockClearInput) (string, error) {
		n, err := reg.Clear(in.ID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("ok: %d mock(s) removed", n), nil
	})
}
