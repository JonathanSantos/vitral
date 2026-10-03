//go:build integration

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Sobe o binário de verdade via stdio (como o Copilot faz) e confere que o protocolo não é poluído.
func TestStdioServer(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "browser-mcp")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	profile, err := os.MkdirTemp("", "browser-mcp-stdio-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(profile)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.Command(bin, "--headless", "--profile-dir", profile)
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
