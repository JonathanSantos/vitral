//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/JonathanSantos/vitral/test/tokens"
)

// budget guarda os tokens medidos (docs/TESTING.md, seção 6); o qualitygate compara com os tetos.
var budget = struct {
	mu     sync.Mutex
	values map[string]int
}{values: map[string]int{}}

func recordTokens(key, text string) {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	budget.values[key] = tokens.Estimate(text)
}

// writeBudget grava o relatório no fim da suíte.
func writeBudget() error {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if len(budget.values) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(budget.values, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "..", "tokens.json"), append(data, '\n'), 0o600)
}

// Snapshot de cada página de fixture: o padrão (viewport) e o completo.
func TestSnapshotBudget(t *testing.T) {
	pages := []string{"form.html", "api.html", "styles.html", "dialog.html", "rerender.html", "large.html"}
	s := start(t)
	ctx := context.Background()
	for _, page := range pages {
		text, isErr, err := s.Call(ctx, "open", map[string]any{"url": site.URL + "/pages/" + page})
		if err != nil || isErr {
			t.Fatalf("open %s: %v %s", page, err, text)
		}
		recordTokens("snapshot/"+page, stripStep(text))
		full, isErr, err := s.Call(ctx, "snapshot", map[string]any{"full": true})
		if err != nil || isErr {
			t.Fatalf("full snapshot %s: %v %s", page, err, full)
		}
		recordTokens("snapshot-full/"+page, full)
	}
}

// stripStep tira o "flow step #N" do fim (depende da ordem dos testes, não da página).
func stripStep(text string) string {
	if i := lastLineIndex(text, "flow step #"); i >= 0 {
		return text[:i]
	}
	return text
}

func lastLineIndex(text, prefix string) int {
	for i := len(text) - 1; i > 0; i-- {
		if text[i-1] == '\n' && len(text[i:]) >= len(prefix) && text[i:i+len(prefix)] == prefix {
			return i
		}
	}
	return -1
}
