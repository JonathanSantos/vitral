// Package app monta o servidor MCP com todas as dependências.
package app

import (
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/console"
	"github.com/JonathanSantos/vitral/internal/flow"
	"github.com/JonathanSantos/vitral/internal/mock"
	"github.com/JonathanSantos/vitral/internal/network"
	"github.com/JonathanSantos/vitral/internal/session"
	"github.com/JonathanSantos/vitral/internal/tools"
)

const instructions = `Controls a real browser window. Start with "open" (or "snapshot" if a page is already open).
Snapshots list elements as [eN] role "name"; pass the ref (eN) to click/type/select/scroll.
Every action returns only what changed. If a ref is not found, call snapshot again.
Never type passwords: ask the user to log in in the browser window.`

// Build cria o servidor e devolve a função que fecha o browser.
func Build(cfg config.Config, logger *slog.Logger) (*mcp.Server, func()) {
	mgr := browser.New(cfg, logger)
	net := network.NewTracker(cfg.NetworkBuffer, cfg.PreserveNetwork)
	con := console.NewBuffer(cfg.ConsoleBuffer)
	sess := session.New(cfg, logger, mgr, net, con)

	var mocks *mock.Registry
	if !cfg.DisableMock {
		mocks = mock.NewRegistry(cfg.AllowedHosts)
		mgr.OnNewPage(mock.NewInterceptor(mocks, logger, net.MarkMocked).Attach)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: config.Name, Version: config.Version}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       logger,
	})
	tools.Register(server, tools.Deps{Config: cfg, Session: sess, Network: net, Console: con, Mocks: mocks,
		Flows: flow.Store{Dir: cfg.FlowsDir}})
	return server, mgr.Close
}
