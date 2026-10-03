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
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// textHandler adapta uma função que devolve texto para o formato do SDK.
func textHandler[In any](fn func(ctx context.Context, in In) (string, error)) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (res *mcp.CallToolResult, _ any, err error) {
		// Rede de segurança: um panic numa dependência não pode derrubar o servidor.
		defer func() {
			if r := recover(); r != nil {
				res, err = nil, fmt.Errorf("internal error: %v. Call snapshot and try again.", r)
			}
		}()
		out, err := fn(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return text(out), nil, nil
	}
}
