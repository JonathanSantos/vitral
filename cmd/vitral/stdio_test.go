//go:build integration

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Sobe o binário de verdade via stdio (como o Copilot faz) e confere que o protocolo não é poluído.
func TestStdioServer(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "vitral")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	profile, err := os.MkdirTemp("", "vitral-stdio-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(profile)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Navegador explícito: o CI padrão não baixa o Chrome for Testing.
	browserPath := os.Getenv("VITRAL_BROWSER_PATH")
	if browserPath == "" {
		browserPath, _ = launcher.LookPath()
	}
	cmd := exec.Command(bin, "--headless", "--profile-dir", profile, "--browser-path", browserPath)
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"open", "snapshot", "click", "type", "select", "press", "scroll", "wait_for", "back"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("tool %s missing in %s", want, joined)
		}
	}
	if strings.Contains(joined, "eval_js") {
		t.Fatal("eval_js must not be registered without --allow-eval")
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "open", Arguments: map[string]any{"url": "data:text/html,<button>Oi</button>"}})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if res.IsError || !strings.Contains(text, `button "Oi"`) {
		t.Fatalf("unexpected open result: %s", text)
	}
}

// Sem navegador nenhum: a IA recebe um erro acionável e a pessoa vê as saídas em PT-BR no stderr.
func TestStdioNoBrowserFound(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "vitral")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	empty := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.Command(bin, "--headless", "--profile-dir", filepath.Join(empty, "p"), "--browsers-dir", filepath.Join(empty, "b"))
	cmd.Env = append(os.Environ(),
		"VITRAL_BROWSER_PATH=",
		"VITRAL_BROWSER_MIRROR=http://127.0.0.1:1/cft",
		"PUPPETEER_CACHE_DIR="+filepath.Join(empty, "pp"),
		"PLAYWRIGHT_BROWSERS_PATH="+filepath.Join(empty, "pw"),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test"}, nil)
	cs, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "open", Arguments: map[string]any{"url": "about:blank"}})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	if !res.IsError || !strings.Contains(text, "no browser available") || !strings.Contains(text, "vitral doctor") {
		t.Fatalf("tool result: %v %s", res.IsError, text)
	}
	cs.Close()
	if !strings.Contains(stderr.String(), "O Vitral não encontrou um navegador") || !strings.Contains(stderr.String(), "install-browser --from") {
		t.Fatalf("stderr should explain the options in PT-BR:\n%s", stderr.String())
	}
}
