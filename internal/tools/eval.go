package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/session"
)

type evalInput struct {
	Expression string `json:"expression" jsonschema:"JavaScript expression evaluated in the page; promises are awaited"`
}

// registerEval só é chamada com --allow-eval: sem a flag a ferramenta nem aparece na lista.
func registerEval(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "eval_js",
		Description: "Run a JavaScript expression in the page and return the result. Use only when other tools cannot answer.",
	}, evalTool(d.Session))
}

func evalTool(sess *session.Session) mcp.ToolHandlerFor[evalInput, any] {
	return textHandler(func(ctx context.Context, in evalInput) (string, error) {
		return sess.Eval(ctx, in.Expression)
	})
}
