package tools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/session"
)

type gotoInput struct {
	Screen  string `json:"screen" jsonschema:"name of a known screen (from the screen: line or map_find)"`
	Confirm bool   `json:"confirm,omitempty" jsonschema:"true only after the user agreed to the sensitive steps goto listed"`
}

type mapFindInput struct {
	Text string `json:"text" jsonschema:"what to look for, e.g. Exportar CSV"`
}

func registerMaps(s *mcp.Server, d Deps) {
	addTool(s, d, &mcp.Tool{
		Name:        "goto",
		Description: "Go to a known screen of this site by the most reliable path learned from earlier actions (runs it like a flow). Asks first if the path has a sensitive step.",
	}, gotoTool(d))
	addTool(s, d, &mcp.Tool{
		Name:        "map_find",
		Description: "Search the screens and elements learned for this site (e.g. a button seen before) and show the path from the current screen.",
	}, mapFindTool(d))
}

func gotoTool(d Deps) mcp.ToolHandlerFor[gotoInput, any] {
	return textHandler(func(ctx context.Context, in gotoInput) (string, error) {
		out, err := d.Session.Goto(ctx, in.Screen, in.Confirm, mockAdder(d.Mocks))
		var flowErr *session.FlowError
		if errors.As(err, &flowErr) {
			return "", flowErr
		}
		return out, err
	})
}

func mapFindTool(d Deps) mcp.ToolHandlerFor[mapFindInput, any] {
	return textHandler(func(ctx context.Context, in mapFindInput) (string, error) {
		return d.Session.MapFind(ctx, in.Text)
	})
}
