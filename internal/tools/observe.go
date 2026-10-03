package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jors/browser-mcp/internal/console"
	"github.com/jors/browser-mcp/internal/network"
)

type networkListInput struct {
	Filter string `json:"filter,omitempty" jsonschema:"text that the URL must contain"`
	Method string `json:"method,omitempty" jsonschema:"HTTP method, e.g. GET or POST"`
	Status string `json:"status,omitempty" jsonschema:"2xx, 3xx, 4xx, 5xx, failed, error (4xx/5xx/failed), pending or an exact code"`
	Type   string `json:"type,omitempty" jsonschema:"resource type: xhr, fetch, document, script, stylesheet, image…"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max requests to show, newest last (default 20)"`
}

type networkGetInput struct {
	ID   int   `json:"id" jsonschema:"request id from network_list"`
	Body *bool `json:"body,omitempty" jsonschema:"include the response body (default true)"`
}

type consoleListInput struct {
	Level string `json:"level,omitempty" jsonschema:"error, warn (warnings and errors) or all (default)"`
	Limit int    `json:"limit,omitempty" jsonschema:"max messages, newest last (default 20)"`
}

func registerObserve(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "network_list",
		Description: "List the page's network requests (id METHOD STATUS URL). Use to check which API calls ran and which failed.",
	}, networkListTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "network_get",
		Description: "Show one request from network_list: headers, request payload and response body (JSON formatted).",
	}, networkGetTool(d))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "console_list",
		Description: "List browser console messages and uncaught errors with their source file:line.",
	}, consoleListTool(d))
}

func networkListTool(d Deps) mcp.ToolHandlerFor[networkListInput, any] {
	return textHandler(func(_ context.Context, in networkListInput) (string, error) {
		all := d.Network.Entries()
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		list := network.Apply(all, network.Filter{URL: in.Filter, Method: in.Method, Status: in.Status, Type: in.Type, Limit: limit})
		if len(list) == 0 {
			return fmt.Sprintf("no matching requests (%d recorded; history resets on each page load)", len(all)), nil
		}
		lines := make([]string, 0, len(list)+1)
		lines = append(lines, fmt.Sprintf("%d requests (showing %d, newest last):", len(all), len(list)))
		for _, e := range list {
			lines = append(lines, network.Line(e))
		}
		return strings.Join(lines, "\n"), nil
	})
}

func networkGetTool(d Deps) mcp.ToolHandlerFor[networkGetInput, any] {
	return textHandler(func(ctx context.Context, in networkGetInput) (string, error) {
		e, ok := d.Network.Get(in.ID)
		if !ok {
			return "", fmt.Errorf("request %d not found. Call network_list to see current ids.", in.ID)
		}
		if in.Body != nil && !*in.Body {
			return network.Detail(e, "", d.Config.MaskHeaders), nil
		}
		return network.Detail(e, responseBody(ctx, d, e), d.Config.MaskHeaders), nil
	})
}

// responseBody explica no próprio texto quando o corpo não pode ser lido, em vez de falhar a ferramenta.
func responseBody(ctx context.Context, d Deps, e network.Entry) string {
	switch {
	case e.Failed != "":
		return "(no body: request failed)"
	case !e.Done:
		return "(no body yet: request still pending)"
	case e.Status >= 300 && e.Status < 400:
		return "(no body: redirect)"
	}
	raw, err := d.Session.ResponseBody(ctx, e.RequestID)
	if err != nil {
		return "(" + err.Error() + ")"
	}
	return network.FormatBody(raw, e.MIMEType)
}

func consoleListTool(d Deps) mcp.ToolHandlerFor[consoleListInput, any] {
	return textHandler(func(_ context.Context, in consoleListInput) (string, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 20
		}
		all := d.Console.Messages()
		msgs := console.Filter(all, in.Level, limit)
		if len(msgs) == 0 {
			return fmt.Sprintf("no console messages for level %q (%d recorded)", defaultLevel(in.Level), len(all)), nil
		}
		lines := make([]string, 0, len(msgs))
		for _, m := range msgs {
			lines = append(lines, console.Line(m))
		}
		return strings.Join(lines, "\n"), nil
	})
}

func defaultLevel(level string) string {
	if level == "" {
		return "all"
	}
	return level
}
