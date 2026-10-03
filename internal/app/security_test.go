//go:build integration

package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/ux"
)

// hostStyle lê o atributo style do <vitral-overlay> pelo DOM do CDP.
func (h *harness) hostStyle(t *testing.T) string {
	t.Helper()
	depth := -1
	doc, err := proto.DOMGetDocument{Depth: &depth, Pierce: true}.Call(h.page(t))
	if err != nil {
		t.Fatal(err)
	}
	style := ""
	collect(doc.Root, func(n *proto.DOMNode) {
		if n.NodeName != "VITRAL-OVERLAY" {
			return
		}
		for i := 0; i+1 < len(n.Attributes); i += 2 {
			if n.Attributes[i] == "style" {
				style = n.Attributes[i+1]
			}
		}
	})
	return style
}

// mainWorld avalia no JavaScript da própria página (onde o atacante está).
func (h *harness) mainWorld(t *testing.T, js string) map[string]any {
	t.Helper()
	res, err := h.page(t).Eval(js)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := res.Value.Unmarshal(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Etapa 0: a página não alcança nada do Vitral nem consegue despausar ou esconder o selo.
func TestMaliciousPageCannotTakeOver(t *testing.T) {
	t.Cleanup(func() { h.resume(t) })
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/malicious/takeover.html")})

	// Protótipos envenenados no mundo da página não afetam o snapshot (mundo isolado).
	mustContain(t, snap, `button "Salvar"`, `status "pronto"`)

	probe := h.mainWorld(t, `() => window.probe`)
	for _, key := range []string{"vitral", "snapshotFn", "overlay", "binding"} {
		if probe[key] != "undefined" {
			t.Errorf("page can see window.%s: %v", key, probe[key])
		}
	}
	if probe["shadow"] != "closed" {
		t.Errorf("page should not open the overlay shadow root: %v", probe["shadow"])
	}
	if probe["forged"] != float64(0) {
		t.Errorf("page reached a binding: %v", probe["forged"])
	}

	// A página tenta esconder o selo a cada 50 ms; o overlay desfaz.
	h.waitOverlayText(t, ux.TextIdle)
	waitFor(t, func() bool { return strings.Contains(h.hostStyle(t), "display: block !important") })

	// Pausa pelo botão de verdade: a página não consegue retomar (nem por binding, nem por clique sintético).
	h.clickOverlay(t, "toggle_pause")
	waitFor(t, func() bool { return h.app.Overlay.Blocked() })
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !h.app.Overlay.Blocked() {
			t.Fatal("malicious page resumed the AI")
		}
		time.Sleep(50 * time.Millisecond)
	}
	mustContain(t, h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Salvar")}), "Paused by the user")

	h.resume(t)
	mustContain(t, h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Salvar")}), "")

	// Remover o host não some com o selo: ele volta.
	h.mainWorld(t, `() => { window.removeOverlay(); return {}; }`)
	waitFor(t, func() bool { return h.mainWorld(t, `() => window.probe`)["removed"] == float64(1) })
	h.waitOverlayText(t, ux.TextIdle)
}
