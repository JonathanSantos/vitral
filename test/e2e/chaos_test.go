//go:build e2e

package e2e

import (
	"context"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// chaosOp é uma chamada aleatória (válida ou não).
type chaosOp struct {
	tool string
	args map[string]any
}

func chaosOps(r *rand.Rand) []chaosOp {
	ref := func() string { return "e" + strconv.Itoa(r.IntN(250)+1) }
	return []chaosOp{
		{"open", map[string]any{"url": site.URL + "/pages/form.html"}},
		{"open", map[string]any{"url": site.URL + "/pages/api.html"}},
		{"open", map[string]any{"url": "http://127.0.0.1:1/nada"}},
		{"open", map[string]any{"url": ""}},
		{"snapshot", map[string]any{}},
		{"snapshot", map[string]any{"scope": ref()}},
		{"snapshot", map[string]any{"full": true, "maxLines": -5}},
		{"click", map[string]any{"ref": ref()}},
		{"click", map[string]any{"ref": "e99999"}},
		{"click", map[string]any{}},
		{"click", map[string]any{"ref": 12}},
		{"type", map[string]any{"ref": ref(), "text": "caos"}},
		{"type", map[string]any{"ref": ref(), "text": strings.Repeat("x", 2000), "submit": true}},
		{"select", map[string]any{"ref": ref(), "option": "Nenhuma"}},
		{"press", map[string]any{"key": "Banana"}},
		{"press", map[string]any{"key": "Tab"}},
		{"scroll", map[string]any{"direction": "sideways"}},
		{"scroll", map[string]any{"direction": "down", "amount": 300}},
		{"scroll", map[string]any{"ref": ref()}},
		{"wait_for", map[string]any{"text": "nunca vai aparecer", "timeoutMs": 300}},
		{"wait_for", map[string]any{}},
		{"back", map[string]any{}},
		{"screenshot", map[string]any{"ref": ref()}},
		{"inspect", map[string]any{"ref": ref(), "props": []any{"color"}}},
		{"network_list", map[string]any{"status": "5xx", "limit": -1}},
		{"network_get", map[string]any{"id": -1}},
		{"console_list", map[string]any{"level": "nope"}},
		{"mock_add", map[string]any{"urlPattern": "/api/orders", "fail": "explode"}},
		{"mock_add", map[string]any{"urlPattern": "/api/orders", "status": 503}},
		{"mock_clear", map[string]any{"id": "m404"}},
		{"mock_clear", map[string]any{}},
		{"emulate", map[string]any{"device": "watch"}},
		{"emulate", map[string]any{"network": "none"}},
		{"flow_save", map[string]any{"name": "../fora"}},
		{"flow_run", map[string]any{"name": "nao-existe"}},
		{"flow_list", map[string]any{}},
	}
}

// Modo caótico (docs/TESTING.md, seção 7.5): o servidor nunca cai, nunca dá panic, nunca escreve
// fora do protocolo, e todo erro volta como isError com mensagem.
func TestChaos(t *testing.T) {
	seed := uint64(20261003)
	if v := os.Getenv("VITRAL_CHAOS_SEED"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		seed = parsed
	}
	t.Logf("chaos seed: %d (repeat with VITRAL_CHAOS_SEED=%d)", seed, seed)
	r := rand.New(rand.NewPCG(seed, seed))
	s := start(t)
	ctx := context.Background()
	if _, _, err := s.Call(ctx, "open", map[string]any{"url": site.URL + "/pages/form.html"}); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	errorsSeen := map[string]int{}
	check := func(op chaosOp, text string, isErr bool, err error) {
		if err != nil {
			t.Errorf("%s %v: protocol error: %v", op.tool, op.args, err)
			return
		}
		if !isErr {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		first, _, _ := strings.Cut(text, "\n")
		errorsSeen[first]++
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s %v: empty error", op.tool, op.args)
		}
		for _, bad := range []string{"panic", "goroutine ", "internal error", "runtime error", "nil pointer"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s %v: error leaks a crash (%q): %s", op.tool, op.args, bad, text)
			}
		}
	}

	paused := false
	for round := 0; round < 60; round++ {
		ops := chaosOps(r)
		switch r.IntN(6) {
		case 0: // várias chamadas em paralelo na mesma página, incluindo open no meio de outra ação
			var wg sync.WaitGroup
			for i := 0; i < 2+r.IntN(4); i++ {
				op := ops[r.IntN(len(ops))]
				wg.Add(1)
				go func() {
					defer wg.Done()
					text, isErr, err := s.Call(ctx, op.tool, op.args)
					check(op, text, isErr, err)
				}()
			}
			wg.Wait()
		case 1: // pausa ou retoma pelo selo, como a pessoa
			if _, err := os.Stat(s.debugInfo); err == nil {
				s.clickOverlay("toggle_pause")
				paused = !paused
			}
		default:
			op := ops[r.IntN(len(ops))]
			text, isErr, err := s.Call(ctx, op.tool, op.args)
			check(op, text, isErr, err)
		}
	}
	if paused {
		s.clickOverlay("toggle_pause")
	}

	// O servidor continua de pé e respondendo.
	if _, isErr, err := s.Call(ctx, "open", map[string]any{"url": site.URL + "/pages/form.html"}); err != nil || isErr {
		t.Fatalf("server not usable after chaos: %v", err)
	}
	keys := make([]string, 0, len(errorsSeen))
	for k := range errorsSeen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	t.Logf("%d distinct errors:\n  %s", len(keys), strings.Join(keys, "\n  "))
}
