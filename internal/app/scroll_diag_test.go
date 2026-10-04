//go:build integration && diag

package app_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"

	"github.com/JonathanSantos/vitral/internal/config"
)

// Diagnóstico do scroll atual (pedido "próximos passos", seção 1.1): grava por requestAnimationFrame,
// no mundo isolado, o horário e o scrollTop durante cada forma de rolar que o Vitral usa hoje, e mede
// frames parados no meio do gesto, intervalos entre mudanças e a distribuição dos deltas.
// Rode com: go test -tags 'integration diag' -run TestScrollDiagnostic -v ./internal/app/
// (VITRAL_DIAG_VISIBLE=1 abre a janela, que é o caso que a pessoa vê).
func TestScrollDiagnostic(t *testing.T) {
	visible := os.Getenv("VITRAL_DIAG_VISIBLE") != ""
	cursorOn, cleanupOn := newHarness(func(c *config.Config) {
		c.Cursor, c.CursorSpeed, c.CursorSeed, c.HighlightMs, c.Headless = "force", "normal", 3, 0, !visible
	})
	defer cleanupOn()
	cursorOff, cleanupOff := newHarness(func(c *config.Config) { c.Cursor, c.Headless = "off", !visible })
	defer cleanupOff()

	type scenario struct {
		name string
		hh   *harness
		run  func(t *testing.T, hh *harness, snap string)
	}
	far := func(t *testing.T, hh *harness, label string) string {
		full := hh.ok(t, "snapshot", map[string]any{"full": true})
		m := regexp.MustCompile(`\[(e\d+)\] button "` + label + `"`).FindStringSubmatch(full)
		if m == nil {
			t.Fatalf("no %q in:\n%s", label, full)
		}
		return m[1]
	}
	scenarios := []scenario{
		{"referência: scrollTo smooth nativo do Chrome", cursorOff, func(t *testing.T, hh *harness, _ string) {
			hh.mainWorld(t, `() => { document.scrollingElement.scrollTo({top: 1500, behavior: 'smooth'}); return {}; }`)
			time.Sleep(1500 * time.Millisecond)
		}},
		{"scroll down 1500 sem cursor (1 evento de roda)", cursorOff, func(t *testing.T, hh *harness, _ string) {
			hh.ok(t, "scroll", map[string]any{"direction": "down", "amount": 1500})
		}},
		{"scroll down 1500 com cursor (passos de roda)", cursorOn, func(t *testing.T, hh *harness, _ string) {
			hh.ok(t, "scroll", map[string]any{"direction": "down", "amount": 1500})
		}},
		{"scroll até ref a 3 telas, com cursor (smooth + instant)", cursorOn, func(t *testing.T, hh *harness, _ string) {
			hh.ok(t, "scroll", map[string]any{"ref": far(t, hh, "Alvo a três telas")})
		}},
		{"click em alvo bem longe, com cursor", cursorOn, func(t *testing.T, hh *harness, _ string) {
			hh.ok(t, "click", map[string]any{"ref": far(t, hh, "Alvo bem longe")})
		}},
		{"click em alvo bem longe, sem cursor (scrollIntoView instant)", cursorOff, func(t *testing.T, hh *harness, _ string) {
			hh.ok(t, "click", map[string]any{"ref": far(t, hh, "Alvo bem longe")})
		}},
	}

	var report []string
	report = append(report, fmt.Sprintf("modo: %s", map[bool]string{true: "janela visível", false: "headless"}[visible]))
	for _, sc := range scenarios {
		snap := sc.hh.ok(t, "open", map[string]any{"url": sc.hh.url("/pages/scroll-playground.html")})
		startRecorder(t, sc.hh)
		start := time.Now()
		sc.run(t, sc.hh, snap)
		took := time.Since(start)
		time.Sleep(400 * time.Millisecond)
		frames := stopRecorder(t, sc.hh)
		var final float64
		_ = sc.hh.app.World.Unmarshal(context.Background(), &final, `() => document.scrollingElement.scrollTop`)
		report = append(report, sc.name+fmt.Sprintf(" (ferramenta: %v, posição final %.0f, %d frames gravados)", took.Round(time.Millisecond), final, len(frames)), analyzeScroll(frames))
		if strings.Contains(sc.name, "passos de roda") {
			report = append(report, "  intervalos entre passos (ms): "+stepGaps(frames))
		}
	}
	t.Log("\n" + strings.Join(report, "\n"))
}

func startRecorder(t *testing.T, hh *harness) {
	t.Helper()
	js := `(() => { const d = { f: [], on: true }; window.__scrollDiag = d;
		const loop = (ts) => { if (!d.on) return; d.f.push([ts, document.scrollingElement.scrollTop]); requestAnimationFrame(loop); };
		requestAnimationFrame(loop); return true; })()`
	if err := hh.app.World.Eval(context.Background(), js); err != nil {
		t.Fatal(err)
	}
}

func stopRecorder(t *testing.T, hh *harness) [][2]float64 {
	t.Helper()
	var frames [][2]float64
	if err := hh.app.World.Unmarshal(context.Background(), &frames, `() => { window.__scrollDiag.on = false; return window.__scrollDiag.f; }`); err != nil {
		t.Fatal(err)
	}
	return frames
}

// analyzeScroll resume o gesto: do primeiro ao último frame com mudança de posição.
func analyzeScroll(f [][2]float64) string {
	if len(f) < 3 {
		return "  sem frames"
	}
	var intervals []float64
	for i := 1; i < len(f); i++ {
		intervals = append(intervals, f[i][0]-f[i-1][0])
	}
	frameMs := percentile(intervals, 0.5)
	first, last := -1, -1
	for i := 1; i < len(f); i++ {
		if f[i][1] != f[i-1][1] {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return fmt.Sprintf("  nenhum movimento (frame ≈ %.1f ms)", frameMs)
	}
	var deltas, gaps []float64
	stalls, changes := 0, 0
	lastChange := f[first-1][0]
	for i := first; i <= last; i++ {
		d := math.Abs(f[i][1] - f[i-1][1])
		if d == 0 {
			stalls++
			continue
		}
		changes++
		deltas = append(deltas, d)
		gaps = append(gaps, f[i][0]-lastChange)
		lastChange = f[i][0]
	}
	maxGap := 0.0
	for _, g := range gaps {
		maxGap = math.Max(maxGap, g)
	}
	jumps := 0
	for _, g := range gaps {
		if g > 1.5*frameMs {
			jumps++
		}
	}
	dur := f[last][0] - f[first-1][0]
	return fmt.Sprintf("  gesto %.0f ms, %.0f px em %d mudanças | frames parados no meio: %d (%.0f%%) | intervalos >1 frame: %d, maior %.0f ms (%.1f frames) | delta por mudança p50 %.0f px, p90 %.0f px, máx %.0f px | frame ≈ %.1f ms",
		dur, math.Abs(f[last][1]-f[first-1][1]), changes, stalls, 100*float64(stalls)/float64(last-first+1), jumps, maxGap, maxGap/frameMs,
		percentile(deltas, 0.5), percentile(deltas, 0.9), percentile(deltas, 1), frameMs)
}

// stepGaps lista o tempo entre mudanças de posição (cada passo de roda).
func stepGaps(f [][2]float64) string {
	var out []string
	last := -1.0
	for i := 1; i < len(f); i++ {
		if f[i][1] != f[i-1][1] {
			if last >= 0 {
				out = append(out, fmt.Sprintf("%.0f", f[i][0]-last))
			}
			last = f[i][0]
		}
	}
	return strings.Join(out, " ")
}

func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	i := int(math.Round(p * float64(len(s)-1)))
	return s[i]
}

// Fallback do scroller que só responde à roda: mouseWheel por keyframe (wheelPlan) contra
// Input.synthesizeScrollGesture, medindo o translateY real do conteúdo por frame.
func TestWheelFallbackDiagnostic(t *testing.T) {
	visible := os.Getenv("VITRAL_DIAG_VISIBLE") != ""
	sh, cleanup := newHarness(func(c *config.Config) {
		c.Cursor, c.CursorSpeed, c.CursorSeed, c.HighlightMs, c.Headless = "force", "normal", 4, 0, !visible
	})
	defer cleanup()
	record := `(() => { const tr = document.getElementById('track'); const d = { f: [], on: true }; window.__wheelDiag = d;
		const y = () => { const m = /translateY\((-?[\d.]+)px\)/.exec(tr.style.transform); return m ? -Number(m[1]) : 0; };
		const loop = (ts) => { if (!d.on) return; d.f.push([ts, y()]); requestAnimationFrame(loop); };
		requestAnimationFrame(loop); return true; })()`
	stop := func() [][2]float64 {
		var f [][2]float64
		_ = sh.app.World.Unmarshal(context.Background(), &f, `() => { window.__wheelDiag.on = false; return window.__wheelDiag.f; }`)
		return f
	}
	prepare := func() (float64, float64) {
		sh.ok(t, "open", map[string]any{"url": sh.url("/pages/scroll-playground.html")})
		var r [4]float64
		_ = sh.app.World.Unmarshal(context.Background(), &r, `() => { const w = document.getElementById('wheel');
			w.scrollIntoView({block: 'center', behavior: 'instant'}); const b = w.getBoundingClientRect(); return [b.left, b.top, b.width, b.height]; }`)
		return r[0] + r[2]/2, r[1] + r[3]/2
	}
	var report []string
	report = append(report, "modo: "+map[bool]string{true: "janela visível", false: "headless"}[visible])

	x, y := prepare()
	if err := sh.app.World.Eval(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	full := sh.ok(t, "snapshot", map[string]any{"full": true})
	ref := regexp.MustCompile(`\[(e\d+)\] button "Alvo da roda"`).FindStringSubmatch(full)[1]
	start := time.Now()
	sh.ok(t, "click", map[string]any{"ref": ref})
	report = append(report, fmt.Sprintf("fallback do Vitral: salto + synthesizeScrollGesture (ação inteira %v)", time.Since(start).Round(time.Millisecond)), analyzeScroll(stop()))

	x, y = prepare()
	if err := sh.app.World.Eval(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	p := sh.page(t)
	start = time.Now()
	gesture := proto.InputSynthesizeScrollGesture{X: x, Y: y, YDistance: gson.Num(-1150), Speed: gson.Int(2000), GestureSourceType: proto.InputGestureSourceTypeMouse}
	if err := gesture.Call(p); err != nil {
		report = append(report, "synthesizeScrollGesture: erro: "+err.Error())
	} else {
		time.Sleep(300 * time.Millisecond)
		report = append(report, fmt.Sprintf("synthesizeScrollGesture 2000 px/s (%v)", time.Since(start).Round(time.Millisecond)), analyzeScroll(stop()))
	}
	t.Log("\n" + strings.Join(report, "\n"))
}
