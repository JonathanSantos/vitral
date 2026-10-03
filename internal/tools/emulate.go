package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/session"
)

type emulateInput struct {
	Device  string `json:"device,omitempty" jsonschema:"mobile, tablet or desktop"`
	Network string `json:"network,omitempty" jsonschema:"offline, slow3g, fast3g or none"`
}

func registerEmulate(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "emulate",
		Description: "Simulate a phone/tablet screen or a slow/offline network. Use desktop and none to go back to normal.",
	}, emulateTool(d.Session))
}

func emulateTool(sess *session.Session) mcp.ToolHandlerFor[emulateInput, any] {
	return textHandler(func(ctx context.Context, in emulateInput) (string, error) {
		return sess.Emulate(ctx, in.Device, in.Network)
	})
}
