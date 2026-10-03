package tools

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/session"
)

type openInput struct {
	URL string `json:"url" jsonschema:"address to open, e.g. https://example.com or localhost:3000"`
}

type snapshotInput struct {
	Scope    string `json:"scope,omitempty" jsonschema:"ref of an element to show only its subtree (shows all its items)"`
	Full     bool   `json:"full,omitempty" jsonschema:"include elements outside the visible area"`
	MaxLines int    `json:"maxLines,omitempty" jsonschema:"max lines in the output (default 300)"`
}

type waitForInput struct {
	Text      string `json:"text,omitempty" jsonschema:"visible text to wait for"`
	Ref       string `json:"ref,omitempty" jsonschema:"element ref to wait for"`
	Gone      bool   `json:"gone,omitempty" jsonschema:"wait until the text or element disappears"`
	TimeoutMs int    `json:"timeoutMs,omitempty" jsonschema:"max wait in milliseconds (default 10000, max 60000)"`
}

type noInput struct{}

func registerNavigation(s *mcp.Server, d Deps) {
	addTool(s, d, &mcp.Tool{
		Name:        "open",
		Description: "Open a URL in the browser. Returns a snapshot of the page with element refs (e1, e2…) to use in other tools.",
	}, openTool(d.Session))

	addTool(s, d, &mcp.Tool{
		Name:        "snapshot",
		Description: "Show the current page as text with element refs. Call it when you need to see the page again or after an error.",
	}, snapshotTool(d.Session))

	addTool(s, d, &mcp.Tool{
		Name:        "wait_for",
		Description: "Wait until a visible text (or element ref) appears, or disappears with gone=true. Use after actions that load data slowly.",
	}, waitForTool(d.Session))

	addTool(s, d, &mcp.Tool{
		Name:        "back",
		Description: "Go back to the previous page in history. Returns what changed.",
	}, backTool(d.Session))
}

func openTool(sess *session.Session) mcp.ToolHandlerFor[openInput, any] {
	return textHandler(func(ctx context.Context, in openInput) (string, error) {
		return sess.Open(ctx, in.URL)
	})
}

func snapshotTool(sess *session.Session) mcp.ToolHandlerFor[snapshotInput, any] {
	return textHandler(func(ctx context.Context, in snapshotInput) (string, error) {
		return sess.Snapshot(ctx, session.SnapshotOptions{Scope: in.Scope, Full: in.Full, MaxLines: in.MaxLines})
	})
}

func waitForTool(sess *session.Session) mcp.ToolHandlerFor[waitForInput, any] {
	return textHandler(func(ctx context.Context, in waitForInput) (string, error) {
		timeout := time.Duration(min(in.TimeoutMs, 60000)) * time.Millisecond
		return sess.WaitFor(ctx, session.WaitOptions{Text: in.Text, Ref: in.Ref, Gone: in.Gone, Timeout: timeout})
	})
}

func backTool(sess *session.Session) mcp.ToolHandlerFor[noInput, any] {
	return textHandler(func(ctx context.Context, _ noInput) (string, error) {
		return sess.Back(ctx)
	})
}
