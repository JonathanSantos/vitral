package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/session"
)

type refInput struct {
	Ref string `json:"ref" jsonschema:"element ref from the snapshot, e.g. e5"`
}

type typeInput struct {
	Ref    string `json:"ref" jsonschema:"ref of the text field"`
	Text   string `json:"text" jsonschema:"text to type"`
	Clear  *bool  `json:"clear,omitempty" jsonschema:"clear the field before typing (default true)"`
	Submit bool   `json:"submit,omitempty" jsonschema:"press Enter after typing"`
}

type selectInput struct {
	Ref    string `json:"ref" jsonschema:"ref of the select (combobox)"`
	Option string `json:"option" jsonschema:"label or value of the option"`
}

type pressInput struct {
	Key string `json:"key" jsonschema:"key name like Enter, Escape, Tab, ArrowDown, or a combo like Control+A"`
}

type scrollInput struct {
	Ref       string `json:"ref,omitempty" jsonschema:"scroll until this element is visible"`
	Direction string `json:"direction,omitempty" jsonschema:"up or down (when no ref)"`
	Amount    int    `json:"amount,omitempty" jsonschema:"pixels to scroll (default: most of the screen)"`
}

func registerActions(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "click",
		Description: "Click an element by ref (button, link, checkbox…). Returns what changed on the page.",
	}, clickTool(d.Session))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "type",
		Description: "Type text into a field by ref. Clears it first unless clear=false; submit=true presses Enter. Never use for passwords.",
	}, typeTool(d.Session))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "select",
		Description: "Choose an option in a dropdown (combobox) by its label or value.",
	}, selectTool(d.Session))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "press",
		Description: "Press a keyboard key on the focused element, e.g. Enter, Escape, Tab.",
	}, pressTool(d.Session))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "scroll",
		Description: "Scroll to an element by ref, or the page up/down. Returns the elements that came into view.",
	}, scrollTool(d.Session))
}

func clickTool(sess *session.Session) mcp.ToolHandlerFor[refInput, any] {
	return textHandler(func(ctx context.Context, in refInput) (string, error) {
		return sess.Click(ctx, in.Ref)
	})
}

func typeTool(sess *session.Session) mcp.ToolHandlerFor[typeInput, any] {
	return textHandler(func(ctx context.Context, in typeInput) (string, error) {
		clearFirst := in.Clear == nil || *in.Clear
		return sess.Type(ctx, in.Ref, in.Text, clearFirst, in.Submit)
	})
}

func selectTool(sess *session.Session) mcp.ToolHandlerFor[selectInput, any] {
	return textHandler(func(ctx context.Context, in selectInput) (string, error) {
		return sess.Select(ctx, in.Ref, in.Option)
	})
}

func pressTool(sess *session.Session) mcp.ToolHandlerFor[pressInput, any] {
	return textHandler(func(ctx context.Context, in pressInput) (string, error) {
		return sess.Press(ctx, in.Key)
	})
}

func scrollTool(sess *session.Session) mcp.ToolHandlerFor[scrollInput, any] {
	return textHandler(func(ctx context.Context, in scrollInput) (string, error) {
		return sess.Scroll(ctx, session.ScrollOptions{Ref: in.Ref, Direction: in.Direction, Amount: in.Amount})
	})
}
