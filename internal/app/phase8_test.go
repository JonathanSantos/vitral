//go:build integration

package app_test

import (
	"context"
	"image"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/internal/ux"
)

// UX-2: o selo aparece após open, sobrevive a navegação e reload e não aparece no snapshot.
func TestBadgeSurvivesNavigation(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	h.waitOverlayText(t, ux.TextIdle)
	h.waitOverlayText(t, ux.TextPause)
	if strings.Contains(snap, "Vitral") || strings.Contains(snap, "Pausar") {
		t.Fatalf("snapshot must not see the overlay:\n%s", snap)
	}

	out := h.ok(t, "scroll", map[string]any{"direction": "down", "amount": 2000})
	h.ok(t, "click", map[string]any{"ref": ref(t, out, "link", "Link do rodapé")})
	h.waitOverlayText(t, ux.TextIdle)

	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	h.waitOverlayText(t, ux.TextIdle)

	// Não interfere: nada no histórico de rede nem no console vem do overlay.
	if out := h.ok(t, "network_list", nil); strings.Contains(out, "data:") || strings.Contains(out, "font") {
		t.Fatalf("overlay generated requests:\n%s", out)
	}
	if out := h.ok(t, "console_list", nil); strings.Contains(strings.ToLower(out), "vitral") {
		t.Fatalf("overlay wrote to the console:\n%s", out)
	}
}

// --no-overlay: sem texto de status nem destaque, mas o botão de pausa continua.
func TestNoOverlayKeepsPauseButton(t *testing.T) {
	min, cleanup := newHarness(func(c *config.Config) { c.NoOverlay = true; c.HighlightMs = 400 })
	defer cleanup()
	snap := min.ok(t, "open", map[string]any{"url": min.url("/pages/form.html")})
	min.waitOverlayText(t, ux.TextPause)
	if strings.Contains(min.overlayText(t), ux.TextIdle) {
		t.Fatalf("minimal badge should not show the status: %q", min.overlayText(t))
	}
	min.ok(t, "click", map[string]any{"ref": ref(t, snap, "checkbox", "Aceito os termos")})
	if hasRing(t, min) {
		t.Fatal("no highlight with --no-overlay")
	}
	min.clickOverlay(t, "toggle_pause")
	waitFor(t, func() bool { return min.app.Overlay.Blocked() })
	mustContain(t, min.ok(t, "press", map[string]any{"key": "Tab"}), "Paused by the user")
	min.resume(t)
}

func TestBadgeShowsWorkingDuringSlowTool(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.call(t, "wait_for", map[string]any{"text": "texto que nunca aparece", "timeoutMs": 2500})
	}()
	h.waitOverlayText(t, ux.TextWorking)
	wg.Wait()
	h.waitOverlayText(t, ux.TextIdle)
}

// UX-3: pausa pelo botão bloqueia as ações (sem erro) e libera as leituras.
func TestPauseBlocksActionsAllowsReads(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	h.clickOverlay(t, "toggle_pause")
	h.waitOverlayText(t, ux.TextPaused)
	h.waitOverlayText(t, ux.TextResume)
	t.Cleanup(func() { h.resume(t) })

	for _, call := range []struct {
		tool string
		args map[string]any
	}{
		{"click", map[string]any{"ref": ref(t, snap, "checkbox", "Aceito os termos")}},
		{"type", map[string]any{"ref": ref(t, snap, "textbox", "Nome"), "text": "x"}},
		{"open", map[string]any{"url": h.url("/pages/api.html")}},
		{"press", map[string]any{"key": "Tab"}},
		{"flow_run", map[string]any{"name": "qualquer"}},
		{"mock_add", map[string]any{"urlPattern": "/x"}},
	} {
		out := h.ok(t, call.tool, call.args)
		mustContain(t, out, "Paused by the user")
	}
	mustContain(t, h.ok(t, "snapshot", nil), `checkbox "Aceito os termos"`)
	h.shot(t, map[string]any{})
	h.ok(t, "network_list", nil)
	h.ok(t, "console_list", nil)
	h.ok(t, "mock_list", nil)
	h.ok(t, "flow_list", nil)

	// Nada foi digitado durante a pausa.
	if s := h.ok(t, "snapshot", nil); !strings.Contains(s, `textbox "Nome" value=""`) {
		t.Fatalf("type must not run while paused:\n%s", s)
	}

	h.resume(t)
	h.waitOverlayText(t, ux.TextIdle)
	out := h.ok(t, "click", map[string]any{"ref": ref(t, snap, "checkbox", "Aceito os termos")})
	mustContain(t, out, "checked")
}

func TestScreenshotHidesOverlay(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	h.waitOverlayText(t, ux.TextIdle)
	img, _ := h.shot(t, map[string]any{"marks": false})
	b := img.Bounds()
	// O selo fica no canto inferior direito; no print essa área é só o fundo branco da página.
	area := image.Rect(b.Max.X-200, b.Max.Y-70, b.Max.X-16, b.Max.Y-16)
	for x := area.Min.X; x < area.Max.X; x += 4 {
		for y := area.Min.Y; y < area.Max.Y; y += 4 {
			if c := rgba(img.At(x, y)); c.R < 250 || c.G < 250 || c.B < 250 {
				t.Fatalf("overlay leaked into the screenshot at (%d,%d): %v", x, y, c)
			}
		}
	}
	// E volta depois da captura.
	h.waitOverlayText(t, ux.TextIdle)
}

func TestCollapseIsKeptAcrossNavigation(t *testing.T) {
	h.ok(t, "open", map[string]any{"url": h.url("/pages/form.html")})
	h.clickOverlay(t, "collapse")
	waitFor(t, func() bool { return h.app.Overlay.State().Collapsed })
	h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	waitFor(t, func() bool { return !strings.Contains(h.overlayText(t), ux.TextIdle) })
	h.clickOverlay(t, "expand")
	h.waitOverlayText(t, ux.TextIdle)
}

// flow_run mostra o progresso no selo e respeita a pausa entre passos.
func TestFlowProgressAndPauseBetweenSteps(t *testing.T) {
	t.Cleanup(func() { h.resume(t); h.ok(t, "mock_clear", nil) })
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/api.html")})
	first := stepNumber(t, snap)
	h.ok(t, "mock_add", map[string]any{
		"urlPattern": "/api/orders", "delayMs": 2500,
		"body": `{"orders":[{"id":9,"status":"Lento","total":1}]}`,
	})
	h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Carregar pedidos")})
	h.ok(t, "wait_for", map[string]any{"text": "1 pedidos carregados", "timeoutMs": 8000})
	h.ok(t, "wait_for", map[string]any{"text": "Lento"})
	h.ok(t, "flow_save", map[string]any{"name": "lento", "fromStep": first})
	h.ok(t, "mock_clear", nil)

	result := make(chan string, 1)
	go func() {
		out, _ := h.call(t, "flow_run", map[string]any{"name": "lento"})
		result <- out
	}()
	h.waitOverlayText(t, "Executando fluxo: lento · passo 4 de 5")
	h.clickOverlay(t, "toggle_pause")
	out := <-result
	mustContain(t, out, "4. ok wait_for", "Flow lento stopped before step 5 of 5: paused by the user")
}

func stepNumber(t *testing.T, out string) int {
	t.Helper()
	i := strings.LastIndex(out, "step #")
	if i < 0 {
		t.Fatalf("no step number in:\n%s", out)
	}
	return atoi(t, strings.Fields(out[i+len("step #"):])[0])
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in 5s")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// UX-1: com destaque ligado, o alvo pisca antes da ação, o clique acerta e o snapshot não vê o overlay.
func TestHighlightBeforeAction(t *testing.T) {
	hl, cleanup := newHarness(func(c *config.Config) { c.HighlightMs = 400 })
	defer cleanup()
	snap := hl.ok(t, "open", map[string]any{"url": hl.url("/pages/form.html")})
	checkbox := ref(t, snap, "checkbox", "Aceito os termos")

	// Com o anel na tela, o snapshot (o mesmo caminho das ações) não enxerga nada do overlay.
	hl.app.Overlay.ShowHighlight(context.Background(), checkbox, [4]int{10, 10, 120, 30}, 3*time.Second)
	waitFor(t, func() bool { return hasRing(t, hl) })
	full := hl.ok(t, "snapshot", map[string]any{"full": true})
	if _, rest, _ := strings.Cut(full, "\n"); strings.Contains(strings.ToLower(rest), "vitral") || strings.Contains(rest, "Pausar") {
		t.Fatalf("snapshot during highlight sees the overlay: %s", full)
	}

	done := make(chan string, 1)
	start := time.Now()
	go func() { done <- hl.ok(t, "click", map[string]any{"ref": checkbox}) }()
	out := <-done
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Fatalf("click should wait for the highlight, took %v", elapsed)
	}
	mustContain(t, out, `checkbox "Aceito os termos" checked`)
}

// hasRing diz se o overlay está desenhando um destaque (anel) agora.
func hasRing(t *testing.T, hl *harness) bool {
	found := false
	collect(hl.overlayRoot(t), func(n *proto.DOMNode) {
		for i := 0; i+1 < len(n.Attributes); i += 2 {
			if n.Attributes[i] == "class" && strings.HasPrefix(n.Attributes[i+1], "ring") {
				found = true
			}
		}
	})
	return found
}

// O selo cobre o canto inferior direito; um botão ali ainda pode ser clicado pela IA.
func TestClickUnderBadge(t *testing.T) {
	snap := h.ok(t, "open", map[string]any{"url": h.url("/pages/corner.html")})
	h.waitOverlayText(t, ux.TextIdle)
	out := h.ok(t, "click", map[string]any{"ref": ref(t, snap, "button", "Canto")})
	mustContain(t, out, `status "clicado"`)
	h.waitOverlayText(t, ux.TextIdle)
}
