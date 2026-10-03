package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/session"
)

type screenshotInput struct {
	Marks    *bool  `json:"marks,omitempty" jsonschema:"draw element refs on the image (default true)"`
	Ref      string `json:"ref,omitempty" jsonschema:"crop the image around this element"`
	FullPage bool   `json:"fullPage,omitempty" jsonschema:"capture the whole page, not only the visible area"`
}

type inspectInput struct {
	Ref   string   `json:"ref" jsonschema:"element ref from the snapshot"`
	Props []string `json:"props,omitempty" jsonschema:"extra CSS properties to read, e.g. outline-color"`
}

func registerVision(s *mcp.Server, d Deps) {
	addTool(s, d, &mcp.Tool{
		Name:        "screenshot",
		Description: "Take a picture of the page with element refs drawn on it. Use when layout, colors or images matter.",
	}, screenshotTool(d.Session))

	addTool(s, d, &mcp.Tool{
		Name:        "inspect",
		Description: "Show the computed styles of an element (color, background, font, border, size), its states and aria attributes.",
	}, inspectTool(d.Session))
}

func screenshotTool(sess *session.Session) mcp.ToolHandlerFor[screenshotInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in screenshotInput) (*mcp.CallToolResult, any, error) {
		shot, err := sess.Screenshot(ctx, session.ShotOptions{Marks: in.Marks == nil || *in.Marks, Ref: in.Ref, FullPage: in.FullPage})
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.ImageContent{Data: shot.PNG, MIMEType: "image/png"},
			&mcp.TextContent{Text: shot.Caption},
		}}, nil, nil
	}
}

func inspectTool(sess *session.Session) mcp.ToolHandlerFor[inspectInput, any] {
	return textHandler(func(ctx context.Context, in inspectInput) (string, error) {
		return sess.Inspect(ctx, in.Ref, in.Props)
	})
}
