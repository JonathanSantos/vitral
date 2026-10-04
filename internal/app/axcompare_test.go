//go:build integration && axcompare

package app_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/JonathanSantos/vitral/internal/config"
	"github.com/JonathanSantos/vitral/test/tokens"
)

// Comparação dos motores do snapshot (pedido "próximos passos", 2.2). Grava as saídas em
// .quality/out/axcompare e imprime a tabela. Rode com:
// go test -tags 'integration axcompare' -run TestAXCompare -v ./internal/app/
func TestAXCompare(t *testing.T) {
	out := filepath.Join("..", "..", ".quality", "out", "axcompare")
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatal(err)
	}
	engines := map[string]*harness{}
	for _, e := range []string{"dom", "ax"} {
		hh, cleanup := newHarness(func(c *config.Config) { c.SnapshotEngine = e })
		defer cleanup()
		engines[e] = hh
	}
	pages, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "pages", "*.html"))
	extra, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "pages", "malicious", "*.html"))
	pages = append(pages, extra...)
	refRe := regexp.MustCompile(`\[e\d+\]`)
	norm := func(s string) []string {
		var lines []string
		for _, l := range strings.Split(refRe.ReplaceAllString(s, "[e]"), "\n") {
			if !strings.HasPrefix(l, "flow step") && !strings.HasPrefix(l, "page: ") {
				lines = append(lines, l)
			}
		}
		return lines
	}
	diff := func(a, b []string) int {
		count := map[string]int{}
		for _, l := range a {
			count[l]++
		}
		d := 0
		for _, l := range b {
			if count[l] > 0 {
				count[l]--
			} else {
				d++
			}
		}
		for _, c := range count {
			d += c
		}
		return d
	}

	var rows []string
	rows = append(rows, "| página | tokens dom | tokens ax | linhas diferentes | tokens dom (full) | tokens ax (full) | linhas diferentes (full) |", "|---|---|---|---|---|---|---|")
	totals := [4]int{}
	for _, p := range pages {
		rel, _ := filepath.Rel(filepath.Join("..", "..", "testdata", "pages"), p)
		url := "/pages/" + filepath.ToSlash(rel)
		res := map[string][2]string{}
		for e, hh := range engines {
			hh.ok(t, "open", map[string]any{"url": hh.url(url)})
			time.Sleep(300 * time.Millisecond) // páginas com conteúdo carregado por script
			snap, _ := hh.call(t, "snapshot", nil)
			full, _ := hh.call(t, "snapshot", map[string]any{"full": true})
			res[e] = [2]string{snap, full}
			name := strings.ReplaceAll(strings.TrimSuffix(rel, ".html"), "/", "_")
			_ = os.WriteFile(filepath.Join(out, name+"."+e+".txt"), []byte(snap+"\n\n--- full ---\n"+full), 0o600)
		}
		td, ta := tokens.Estimate(res["dom"][0]), tokens.Estimate(res["ax"][0])
		fd, fa := tokens.Estimate(res["dom"][1]), tokens.Estimate(res["ax"][1])
		totals[0], totals[1], totals[2], totals[3] = totals[0]+td, totals[1]+ta, totals[2]+fd, totals[3]+fa
		rows = append(rows, fmt.Sprintf("| %s | %d | %d | %d | %d | %d | %d |", rel, td, ta,
			diff(norm(res["dom"][0]), norm(res["ax"][0])), fd, fa, diff(norm(res["dom"][1]), norm(res["ax"][1]))))
	}
	rows = append(rows, fmt.Sprintf("| **total** | %d | %d | | %d | %d | |", totals[0], totals[1], totals[2], totals[3]))

	// Tempo do snapshot (mediana de 15 chamadas, depois do primeiro).
	rows = append(rows, "", "| página | mediana dom | mediana ax | razão |", "|---|---|---|---|")
	for _, page := range []string{"/pages/large.html", "/pages/big-table.html", "/pages/form.html"} {
		med := map[string]time.Duration{}
		for e, hh := range engines {
			hh.ok(t, "open", map[string]any{"url": hh.url(page)})
			hh.ok(t, "snapshot", map[string]any{"full": true})
			var ds []time.Duration
			for i := 0; i < 15; i++ {
				start := time.Now()
				hh.ok(t, "snapshot", map[string]any{"full": true})
				ds = append(ds, time.Since(start))
			}
			sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
			med[e] = ds[len(ds)/2]
		}
		rows = append(rows, fmt.Sprintf("| %s (full) | %v | %v | %.2f× |", page, med["dom"].Round(100*time.Microsecond), med["ax"].Round(100*time.Microsecond), float64(med["ax"])/float64(med["dom"])))
	}

	// Shadow root fechado: o snapshot enxerga? A ação alcança?
	rows = append(rows, "", "| shadow fechado | dom | ax |", "|---|---|---|")
	shadow := map[string][3]string{}
	for e, hh := range engines {
		snap := hh.ok(t, "open", map[string]any{"url": hh.url("/pages/closed-shadow.html")})
		seen := "não"
		click := "-"
		n := strings.Count(snap, "\n")
		if m := regexp.MustCompile(`\[(e\d+)\] button "Pagar agora"`).FindStringSubmatch(snap); m != nil {
			seen = "sim"
			outText, isErr := hh.call(t, "click", map[string]any{"ref": m[1]})
			click = "ok"
			if isErr {
				first, _, _ := strings.Cut(outText, "\n")
				click = "falha: " + first
			}
		}
		shadow[e] = [3]string{seen, click, fmt.Sprint(n)}
	}
	rows = append(rows, fmt.Sprintf("| enxerga o botão do shadow fechado | %s | %s |", shadow["dom"][0], shadow["ax"][0]),
		fmt.Sprintf("| clique nele | %s | %s |", shadow["dom"][1], shadow["ax"][1]),
		fmt.Sprintf("| linhas do snapshot | %s | %s |", shadow["dom"][2], shadow["ax"][2]),
		fmt.Sprintf("| nós do ax dentro de shadow fechado | - | %d |", engines["ax"].app.Session.AXStats().ClosedShadowNodes))

	// Divs clicáveis só por listener: aparecem e funcionam?
	rows = append(rows, "", "| divs com listener | dom | ax |", "|---|---|---|")
	divs := map[string]string{}
	for e, hh := range engines {
		snap := hh.ok(t, "open", map[string]any{"url": hh.url("/pages/divs-onclick.html")})
		m := regexp.MustCompile(`\[(e\d+)\] clickable "Plano Pro`).FindStringSubmatch(snap)
		if m == nil {
			divs[e] = "não aparecem"
			continue
		}
		hh.ok(t, "click", map[string]any{"ref": m[1]})
		got := hh.mainWorld(t, `() => ({s: document.getElementById('status').textContent})`)["s"]
		divs[e] = fmt.Sprintf("aparecem; clique: %v", got)
	}
	rows = append(rows, fmt.Sprintf("| Plano Pro | %s | %s |", divs["dom"], divs["ax"]))

	report := strings.Join(rows, "\n")
	_ = os.WriteFile(filepath.Join(out, "report.md"), []byte(report+"\n"), 0o600)
	t.Log("\n" + report)
}
