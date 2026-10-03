// Comando browser-mcp: servidor MCP (stdio) que controla um Chromium.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jors/browser-mcp/internal/app"
	"github.com/jors/browser-mcp/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", config.Name, err)
		os.Exit(1)
	}
}

func run() error {
	// stdout pertence ao protocolo MCP: qualquer escrita perdida de dependências vai para stderr.
	protocolOut := os.Stdout
	os.Stdout = os.Stderr

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home dir: %w", err)
	}
	cfg, err := config.Load(os.Args[1:], home, config.ReadFile)
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server, closeBrowser := app.Build(cfg, logger)
	defer closeBrowser()

	logger.Info("server ready", "version", config.Version)
	err = server.Run(ctx, &mcp.IOTransport{Reader: os.Stdin, Writer: protocolOut})
	if err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
