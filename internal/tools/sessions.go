package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sessionSaveInput struct {
	Name string `json:"name" jsonschema:"session name: lowercase letters, digits, - and _ (e.g. portal-hml)"`
}

func registerSessions(s *mcp.Server, d Deps) {
	addTool(s, d, &mcp.Tool{
		Name:        "session_save",
		Description: "After the user logs in by hand, save the login state (cookies and localStorage) under a name, so flows can run without the window with --session <name>. The content is never shown.",
	}, sessionSaveTool(d))
}

func sessionSaveTool(d Deps) mcp.ToolHandlerFor[sessionSaveInput, any] {
	return textHandler(func(_ context.Context, in sessionSaveInput) (string, error) {
		sum, err := d.Session.SaveState(in.Name)
		if err != nil {
			return "", err
		}
		origins := "no localStorage"
		if len(sum.Origins) > 0 {
			origins = fmt.Sprintf("localStorage of %d origin(s): %s", len(sum.Origins), strings.Join(sum.Origins, ", "))
		}
		return strings.Join([]string{
			fmt.Sprintf("saved session %q: %d cookies, %s. File: %s (readable only by this user).", sum.Name, sum.Cookies, origins, sum.Path),
			"Tell the user: the file gives access to these accounts; never commit or share it. IndexedDB and sessionStorage are not saved, so some sites may still ask to log in.",
			fmt.Sprintf("To use it: start Vitral with --session %s (e.g. to run flows without the window).", sum.Name),
		}, "\n"), nil
	})
}
