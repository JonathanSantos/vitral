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
	"github.com/JonathanSantos/vitral/internal/overlay"
	"github.com/JonathanSantos/vitral/internal/session"
	"github.com/JonathanSantos/vitral/internal/tools"
	"github.com/JonathanSantos/vitral/internal/world"
)

const instructions = `Controls a real browser window. Start with "open" (or "snapshot" if a page is already open).
Snapshots list elements as [eN] role "name"; pass the ref (eN) to click/type/select/scroll.
Every action returns only what changed. If a ref is not found, call snapshot again.
Never type passwords: ask the user to log in in the browser window.
The user can pause you from the browser at any time; when a tool says so, stop and tell the user.`

// App é o servidor montado, com acesso ao browser para quem precisa (testes e encerramento).
type App struct {
	Server  *mcp.Server
	Browser *browser.Manager
	Overlay *overlay.Manager
	World   *world.Tracker
}

// Close fecha o browser lançado pelo servidor.
func (a *App) Close() { a.Browser.Close() }

// Build cria o servidor com todas as dependências.
func Build(cfg config.Config, logger *slog.Logger) *App {
	mgr := browser.New(cfg, logger)
	net := network.NewTracker(cfg.NetworkBuffer, cfg.PreserveNetwork)
	con := console.NewBuffer(cfg.ConsoleBuffer)
	// O rastreador do mundo isolado vem primeiro: os scripts dos outros preparos dependem dele.
	w := world.New()
	mgr.OnNewPage(w.Attach)
	ui := overlay.New(overlay.Options{Minimal: cfg.NoOverlay, Highlight: cfg.Highlight()}, logger, w)
	sess := session.New(cfg, logger, mgr, net, con, ui, w)
	mgr.OnNewPage(ui.Attach)

	var mocks *mock.Registry
	if !cfg.DisableMock {
		mocks = mock.NewRegistry(cfg.AllowedHosts)
	}
	// O interceptador também barra navegação fora do --allowed-hosts, mesmo sem mock.
	if mocks != nil || len(cfg.AllowedHosts) > 0 {
		mgr.OnNewPage(mock.NewInterceptor(mocks, cfg.AllowedHosts, logger, net.MarkMocked).Attach)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: config.Name, Version: config.Version}, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       logger,
	})
	tools.Register(server, tools.Deps{
		Config: cfg, Session: sess, Network: net, Console: con, Mocks: mocks,
		Flows: flow.Store{Dir: cfg.FlowsDir}, UI: ui,
	})
	return &App{Server: server, Browser: mgr, Overlay: ui, World: w}
}
