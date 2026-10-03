// Package tools registra as ferramentas MCP. Cada handler é montado por closure com suas dependências.
package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jors/browser-mcp/internal/config"
	"github.com/jors/browser-mcp/internal/console"
	"github.com/jors/browser-mcp/internal/network"
	"github.com/jors/browser-mcp/internal/session"
)

// Deps são as dependências compartilhadas pelos handlers.
type Deps struct {
	Config  config.Config
	Session *session.Session
	Network *network.Tracker
	Console *console.Buffer
}

// Register adiciona todas as ferramentas ao servidor.
func Register(s *mcp.Server, d Deps) {
	registerNavigation(s, d)
	registerActions(s, d)
	registerVision(s, d)
	registerObserve(s, d)
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// textHandler adapta uma função que devolve texto para o formato do SDK.
func textHandler[In any](fn func(ctx context.Context, in In) (string, error)) mcp.ToolHandlerFor[In, any] {
	return guard(func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := fn(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return text(out), nil, nil
	})
}

// guard é a rede de segurança: um panic numa dependência vira erro de ferramenta, não derruba o servidor.
func guard[In any](h mcp.ToolHandlerFor[In, any]) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (res *mcp.CallToolResult, out any, err error) {
		defer func() {
			if r := recover(); r != nil {
				res, out, err = nil, nil, fmt.Errorf("internal error: %v. Call snapshot and try again.", r)
			}
		}()
		return h(ctx, req, in)
	}
}
