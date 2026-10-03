// Comando vitral: servidor MCP (stdio) que controla um Chromium.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/app"
	"github.com/JonathanSantos/vitral/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", config.Name, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(config.Name, config.Version)
		return nil
	}
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

	a := app.Build(cfg, logger)
	defer a.Close()

	logger.Info("server ready", "version", config.Version)
	err = a.Server.Run(ctx, &mcp.IOTransport{Reader: os.Stdin, Writer: protocolOut})
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
