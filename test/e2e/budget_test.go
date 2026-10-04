//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

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

// timing guarda as medianas de tempo do snapshot (docs/SNAPSHOT-V2.md); o qualitygate compara com os tetos.
var timing = struct {
	mu     sync.Mutex
	values map[string]float64
}{values: map[string]float64{}}

// writeBudget grava os relatórios de tokens e de tempo no fim da suíte.
func writeBudget() error {
	budget.mu.Lock()
	defer budget.mu.Unlock()
	timing.mu.Lock()
	defer timing.mu.Unlock()
	for name, values := range map[string]any{"tokens.json": budget.values, "timing.json": timing.values} {
		if v, ok := values.(map[string]int); ok && len(v) == 0 {
			continue
		}
		if v, ok := values.(map[string]float64); ok && len(v) == 0 {
			continue
		}
		data, err := json.MarshalIndent(values, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "..", name), append(data, '\n'), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Tempo do snapshot pela ferramenta, num binário compilado como o de release (o das outras suítes tem
// -race, que deixa a decodificação das respostas do CDP 5 a 7 vezes mais lenta): mediana de 15
// chamadas depois de uma de aquecimento. A área visível é o snapshot do diff depois de cada ação.
func TestSnapshotTiming(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "vitral")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vitral")
	build.Dir = repoRoot()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	s := startBin(t, bin)
	ctx := context.Background()
	median := func(args map[string]any) float64 {
		var ds []time.Duration
		for i := 0; i < 16; i++ {
			start := time.Now()
			text, isErr, err := s.Call(ctx, "snapshot", args)
			if err != nil || isErr {
				t.Fatalf("snapshot %v: %v %s", args, err, text)
			}
			if i > 0 {
				ds = append(ds, time.Since(start))
			}
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		return float64(ds[len(ds)/2].Microseconds()) / 1000
	}
	record := func(key string, ms float64) {
		timing.mu.Lock()
		defer timing.mu.Unlock()
		timing.values[key] = ms
	}
	for _, page := range []string{"large.html", "big-table.html", "form.html"} {
		text, isErr, err := s.Call(ctx, "open", map[string]any{"url": site.URL + "/pages/" + page})
		if err != nil || isErr {
			t.Fatalf("open %s: %v %s", page, err, text)
		}
		record("snapshot/"+page, median(nil))
		if page == "large.html" {
			record("snapshot-full/"+page, median(map[string]any{"full": true}))
		}
	}
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
