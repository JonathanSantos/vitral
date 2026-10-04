package app_test

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/JonathanSantos/vitral/internal/app"
	"github.com/JonathanSantos/vitral/internal/config"
)

var update = flag.Bool("update", false, "rewrite golden files")

type toolContract struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// listContract sobe o servidor sem browser (ele só abre na primeira ação) e lê o tools/list.
func listContract(t *testing.T, tweak func(*config.Config)) []toolContract {
	t.Helper()
	cfg := config.Defaults(t.TempDir())
	if tweak != nil {
		tweak(&cfg)
	}
	a := app.Build(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer a.Close()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := a.Server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "contract"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cs.Close() }()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]toolContract, 0, len(res.Tools))
	for _, tool := range res.Tools {
		out = append(out, toolContract{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Contrato das ferramentas (docs/TESTING.md, seção 6): qualquer mudança aparece no diff do golden.
// Atualize com: go test ./internal/app -run TestToolContract -update
func TestToolContract(t *testing.T) {
	tests := []struct {
		golden string
		tweak  func(*config.Config)
	}{
		{"tools.json", nil},
		{"tools-allow-eval.json", func(c *config.Config) { c.AllowEval = true }},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			got, err := json.MarshalIndent(listContract(t, tt.tweak), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("..", "..", "testdata", "golden", tt.golden)
			if *update {
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run with -update): %v", err)
			}
			if string(got) != string(want) {
				t.Fatalf("tool contract changed (%s). If intended, run: go test ./internal/app -run TestToolContract -update\ngot:\n%s", tt.golden, got)
			}
		})
	}
}

// eval_js só existe com --allow-eval; mock_* somem com --disable-mock.
func TestToolListByFlags(t *testing.T) {
	names := func(list []toolContract) map[string]bool {
		m := map[string]bool{}
		for _, c := range list {
			m[c.Name] = true
		}
		return m
	}
	if names(listContract(t, nil))["eval_js"] {
		t.Fatal("eval_js must not be listed without --allow-eval")
	}
	if !names(listContract(t, func(c *config.Config) { c.AllowEval = true }))["eval_js"] {
		t.Fatal("eval_js must be listed with --allow-eval")
	}
	if all := names(listContract(t, nil)); !all["goto"] || !all["map_find"] {
		t.Fatal("goto and map_find are listed by default")
	}
	if off := names(listContract(t, func(c *config.Config) { c.NoMap = true })); off["goto"] || off["map_find"] {
		t.Fatal("goto and map_find must not be listed with --no-map")
	}
	for name := range names(listContract(t, func(c *config.Config) { c.DisableMock = true })) {
		if len(name) > 5 && name[:5] == "mock_" {
			t.Fatalf("%s must not be listed with --disable-mock", name)
		}
	}
}
