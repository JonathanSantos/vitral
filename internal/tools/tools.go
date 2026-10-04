// Package tools registra as ferramentas MCP. Cada handler é montado por closure com suas dependências.
package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/console"
	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/mock"
	"github.com/JonathanSantos/vitral/internal/network"
	"github.com/JonathanSantos/vitral/internal/session"
	"github.com/JonathanSantos/vitral/internal/ux"
)

// Deps são as dependências compartilhadas pelos handlers.
type Deps struct {
	Config  config.Config
	Session *session.Session
	Network *network.Tracker
	Console *console.Buffer
	Mocks   *mock.Registry // nil quando o mock está desabilitado
	Flows   flow.Store
	UI      UI
}

// Register adiciona todas as ferramentas ao servidor.
func Register(s *mcp.Server, d Deps) {
	registerNavigation(s, d)
	registerActions(s, d)
	registerVision(s, d)
	registerObserve(s, d)
	registerEmulate(s, d)
	if d.Mocks != nil {
		registerMock(s, d)
	}
	registerFlows(s, d)
	registerSessions(s, d)
	if d.Config.AllowEval {
		registerEval(s, d)
	}
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// textHandler adapta uma função que devolve texto para o formato do SDK.
func textHandler[In any](fn func(ctx context.Context, in In) (string, error)) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := fn(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		return text(out), nil, nil
	}
}

// UI é o que as ferramentas usam do selo: estado "trabalhando" e pausa (UX-2, UX-3).
type UI interface {
	Blocked() bool
	ToolStarted()
	ToolFinished()
}

// acting são as ferramentas que mexem no browser e ficam bloqueadas durante a pausa.
// As demais (leitura, mock_list, wait_for, flow_save) continuam funcionando.
var acting = map[string]bool{
	"open": true, "click": true, "type": true, "select": true, "press": true, "scroll": true, "back": true,
	"emulate": true, "mock_add": true, "mock_clear": true, "flow_run": true, "eval_js": true,
}

// IsActing indica se a ferramenta é bloqueada pela pausa.
func IsActing(name string) bool { return acting[name] }

// addTool registra a ferramenta com o guard: pausa, selo "trabalhando" e proteção contra panic.
func addTool[In any](s *mcp.Server, d Deps, t *mcp.Tool, h mcp.ToolHandlerFor[In, any]) {
	mcp.AddTool(s, t, guard(d.UI, t.Name, h))
}

func guard[In any](ui UI, name string, h mcp.ToolHandlerFor[In, any]) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (res *mcp.CallToolResult, out any, err error) {
		// Rede de segurança: um panic numa dependência vira erro de ferramenta, não derruba o servidor.
		defer func() {
			if r := recover(); r != nil {
				res, out, err = nil, nil, fmt.Errorf("internal error: %v. Call snapshot and try again.", r)
			}
		}()
		// Pausa não é erro: a IA deve avisar a pessoa e esperar (UX-3).
		if acting[name] && ui.Blocked() {
			return text(ux.PausedMessage), nil, nil
		}
		ui.ToolStarted()
		defer ui.ToolFinished()
		return h(ctx, req, in)
	}
}
