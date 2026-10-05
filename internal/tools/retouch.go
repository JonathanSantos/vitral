package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/session"
)

// Retoques (docs/RETOQUES.md): mudar como um site se parece, só neste navegador.

type retouchPreviewInput struct {
	CSS   string `json:"css" jsonschema:"CSS rules to apply on the current page (for example body { background: #111; color: #eee })"`
	Shots bool   `json:"shots,omitempty" jsonschema:"also return before and after pictures (about 1300 tokens each)"`
}

type retouchDiscardInput struct{}

type retouchSaveInput struct {
	Name           string `json:"name" jsonschema:"short name: lowercase letters, digits, - and _ (for example tema-escuro)"`
	Description    string `json:"description,omitempty" jsonschema:"what it changes, in the person's words (shown in the Vitral button)"`
	Scope          string `json:"scope,omitempty" jsonschema:"where it applies: page, site (default) or all"`
	Preview        bool   `json:"preview,omitempty" jsonschema:"only show what would be saved, with before and after pictures; nothing is saved"`
	IgnoreWarnings bool   `json:"ignoreWarnings,omitempty" jsonschema:"save despite warnings, only when the person confirmed"`
}

type retouchListInput struct{}

type retouchToggleInput struct {
	Name string `json:"name" jsonschema:"retouch name (see retouch_list)"`
	On   bool   `json:"on" jsonschema:"true to turn it on, false to turn it off"`
}

func registerRetouches(s *mcp.Server, d Deps) {
	addTool(s, d, &mcp.Tool{
		Name: "retouch_preview",
		Description: "Change how the current page looks with CSS, only on this tab and only in this browser (the site itself is not changed; a reload removes it). " +
			"Returns how many elements each rule matched. Use it when the person asks to change a site's look (theme, font size, hide or highlight something); " +
			"iterate until it looks right, then show the person and ask whether to keep it (retouch_save).",
	}, retouchPreviewTool(d.Session))
	addTool(s, d, &mcp.Tool{
		Name:        "retouch_discard",
		Description: "Remove the retouch preview from the current tab.",
	}, textHandler(func(ctx context.Context, _ retouchDiscardInput) (string, error) {
		return d.Session.RetouchDiscard(ctx)
	}))
	addTool(s, d, &mcp.Tool{
		Name: "retouch_save",
		Description: "Keep the last retouch_preview: it then applies every time the site opens in this browser, also when the person uses it with vitral open. " +
			"First call with preview=true: it returns where it applies, warnings and before and after pictures to show the person. " +
			"Save only after a clear yes, calling again with only the name. Retouches change looks; they never send, save or delete anything (that is a flow).",
	}, retouchSaveTool(d.Session))
	addTool(s, d, &mcp.Tool{
		Name:        "retouch_list",
		Description: "List the saved retouches, where each applies and whether it is on in the current page.",
	}, textHandler(func(ctx context.Context, _ retouchListInput) (string, error) {
		return d.Session.RetouchList(ctx)
	}))
	addTool(s, d, &mcp.Tool{
		Name:        "retouch_toggle",
		Description: "Turn a saved retouch on or off (the same switch the person has in the Vitral button of the browser toolbar). Works from a page where it applies.",
	}, textHandler(func(ctx context.Context, in retouchToggleInput) (string, error) {
		return d.Session.RetouchToggle(ctx, in.Name, in.On)
	}))
}

// retouchResult põe o texto e, quando há, os prints de antes e depois.
func retouchResult(r session.RetouchResult) *mcp.CallToolResult {
	out := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: r.Text}}}
	if r.Before != nil && r.After != nil {
		out.Content = append(out.Content,
			&mcp.TextContent{Text: "before (original):"}, &mcp.ImageContent{Data: r.Before, MIMEType: "image/png"},
			&mcp.TextContent{Text: "after (with the retouch):"}, &mcp.ImageContent{Data: r.After, MIMEType: "image/png"})
	}
	return out
}

func retouchPreviewTool(sess *session.Session) mcp.ToolHandlerFor[retouchPreviewInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in retouchPreviewInput) (*mcp.CallToolResult, any, error) {
		r, err := sess.RetouchPreview(ctx, in.CSS, in.Shots)
		if err != nil {
			return nil, nil, err
		}
		return retouchResult(r), nil, nil
	}
}

func retouchSaveTool(sess *session.Session) mcp.ToolHandlerFor[retouchSaveInput, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in retouchSaveInput) (*mcp.CallToolResult, any, error) {
		r, err := sess.RetouchSave(ctx, session.RetouchSaveInput{
			Name: in.Name, Description: in.Description, Scope: in.Scope, Preview: in.Preview, IgnoreWarnings: in.IgnoreWarnings,
		})
		if err != nil {
			return nil, nil, err
		}
		return retouchResult(r), nil, nil
	}
}
