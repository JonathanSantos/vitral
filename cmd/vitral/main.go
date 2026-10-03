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
	"github.com/JonathanSantos/vitral/internal/cli"
	"github.com/JonathanSantos/vitral/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", config.Name, err)
		os.Exit(1)
	}
}

func run() error {
	env, err := cli.DefaultEnv()
	if err != nil {
		return err
	}
	// Comandos para pessoas (setup, doctor, version, ajuda). O resto é o servidor MCP.
	if handled, code := cli.Main(os.Args[1:], env, loadConfig(env.Home)); handled {
		os.Exit(code)
	}

	// stdout pertence ao protocolo MCP: qualquer escrita perdida de dependências vai para stderr.
	protocolOut := os.Stdout
	os.Stdout = os.Stderr

	cfg, err := loadConfig(env.Home)(os.Args[1:])
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := app.Build(cfg, logger)
	a.Browser.SetDownloadLogger(cli.DownloadLogger{W: os.Stderr})
	a.Browser.SetNotFoundReporter(cli.NotFoundReporter(os.Stderr, cfg))
	defer a.Close()

	logger.Info("server ready", "version", config.Version)
	err = a.Server.Run(ctx, &mcp.IOTransport{Reader: os.Stdin, Writer: protocolOut})
	if err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func loadConfig(home string) func([]string) (config.Config, error) {
	return func(args []string) (config.Config, error) {
		return config.Load(args, home, config.ReadFile, os.Getenv)
	}
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
