//go:build integration && axperf

package app_test

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Medição das opções do CDP para o snapshot da área visível (pedido "Snapshot v2", 2b). Rode com:
// go test -tags 'integration axperf' -run TestAXPerf -v ./internal/app/
func TestAXPerf(t *testing.T) {
	hh, cleanup := newHarness(nil)
	defer cleanup()
	med := func(n int, f func() int) (time.Duration, int) {
		var ds []time.Duration
		count := 0
		f()
		for i := 0; i < n; i++ {
			start := time.Now()
			count = f()
			ds = append(ds, time.Since(start))
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		return ds[len(ds)/2].Round(100 * time.Microsecond), count
	}
	for _, page := range []string{"/pages/large.html", "/pages/big-table.html", "/pages/form.html"} {
		hh.ok(t, "open", map[string]any{"url": hh.url(page)})
		p := hh.page(t)
		vp := [2]float64{1280, 713}

		dsnap, _ := med(9, func() int {
			s, err := proto.DOMSnapshotCaptureSnapshot{ComputedStyles: []string{"display", "visibility", "opacity", "cursor"}}.Call(p)
			if err != nil {
				t.Fatal(err)
			}
			return len(s.Documents[0].Nodes.BackendNodeID)
		})
		full, nFull := med(9, func() int {
			r, err := proto.AccessibilityGetFullAXTree{}.Call(p)
			if err != nil {
				t.Fatal(err)
			}
			return len(r.Nodes)
		})
		if err := (proto.AccessibilityEnable{}).Call(p); err != nil {
			t.Fatal(err)
		}
		fullOn, _ := med(9, func() int {
			r, err := proto.AccessibilityGetFullAXTree{}.Call(p)
			if err != nil {
				t.Fatal(err)
			}
			return len(r.Nodes)
		})
		t.Logf("%s: getFullAXTree com Accessibility.enable %v", page, fullOn)
		// Poda pelos retângulos: getChildAXNodes só onde a subárvore do DOM chega perto da viewport.
		union := subtreeBoxes(t, p)
		near := func(b [4]float64) bool {
			mx, my := vp[0]*0.25, vp[1]*0.25
			return b[2] > 0 && b[0]+b[2] >= -mx && b[0] <= vp[0]+mx && b[1]+b[3] >= -my && b[1] <= vp[1]+my
		}
		prune, nPrune := med(9, func() int {
			return pruned(t, p, func(b int) bool { u, ok := union.union[b]; return !ok || near(u) })
		})
		// Parcial por candidato: getPartialAXTree (com parentes) de cada elemento com layout perto da viewport.
		var cands, internal []int
		for _, b := range union.order {
			if !near(union.union[b]) {
				continue
			}
			cands = append(cands, b)
			if !union.text[b] && len(union.children[b]) > 0 {
				internal = append(internal, b)
			}
		}
		// Cobertura: a chamada num filho traz os irmãos (os filhos do pai) e os próprios filhos.
		var cover []int
		covered := map[int]bool{}
		isInternal := map[int]bool{}
		for _, b := range internal {
			isInternal[b] = true
		}
		for _, b := range internal { // ordem do documento: pais antes dos filhos
			if covered[b] {
				continue
			}
			pick := b
			for _, c := range union.children[b] {
				if isInternal[c] && near(union.union[c]) {
					pick = c
					break
				}
			}
			cover = append(cover, pick)
			covered[b], covered[pick] = true, true
		}
		partial, nPartial := med(5, func() int { return partialTree(t, p, cands) })
		partialInt, nInt := med(5, func() int { return partialTree(t, p, internal) })
		partialCover, nCover := med(5, func() int { return partialTree(t, p, cover) })
		t.Logf("%s: captureSnapshot %v | getFullAXTree %v (%d nós) | poda getChildAXNodes %v (%d nós)", page, dsnap, full, nFull, prune, nPrune)
		t.Logf("%s: getPartialAXTree: todos %v (%d chamadas, %d nós) | internos %v (%d, %d nós) | cobertura %v (%d, %d nós)",
			page, partial, len(cands), nPartial, partialInt, len(internal), nInt, partialCover, len(cover), nCover)
		snap, _ := med(9, func() int { hh.ok(t, "snapshot", nil); return 0 })
		snapFull, _ := med(9, func() int { hh.ok(t, "snapshot", map[string]any{"full": true}); return 0 })
		t.Logf("%s: ferramenta snapshot %v | snapshot full %v (com enable)", page, snap, snapFull)
		hh.ok(t, "snapshot", nil)
		t.Logf("%s: fases do snapshot visível %+v", page, hh.app.Session.AXStats())
		_ = proto.AccessibilityDisable{}.Call(p)
	}
}

// subtreeBoxes devolve, por backend, a união das caixas da subárvore (coordenadas da viewport).
type perfDOM struct {
	union    map[int][4]float64
	children map[int][]int
	text     map[int]bool
	order    []int
}

func subtreeBoxes(t *testing.T, p *rod.Page) perfDOM {
	s, err := proto.DOMSnapshotCaptureSnapshot{ComputedStyles: []string{}}.Call(p)
	if err != nil {
		t.Fatal(err)
	}
	d := s.Documents[0]
	n := len(d.Nodes.BackendNodeID)
	box := make([][4]float64, n)
	has := make([]bool, n)
	sy := 0.0
	if d.ScrollOffsetY != nil {
		sy = *d.ScrollOffsetY
	}
	for k, idx := range d.Layout.NodeIndex {
		b := d.Layout.Bounds[k]
		if len(b) == 4 && b[2] > 0 && b[3] > 0 {
			box[idx], has[idx] = [4]float64{b[0], b[1] - sy, b[2], b[3]}, true
		}
	}
	for i := n - 1; i > 0; i-- { // filhos vêm depois dos pais
		pi := d.Nodes.ParentIndex[i]
		if pi < 0 || !has[i] {
			continue
		}
		if !has[pi] {
			box[pi], has[pi] = box[i], true
			continue
		}
		a, b := box[pi], box[i]
		x0, y0 := min(a[0], b[0]), min(a[1], b[1])
		x1, y1 := max(a[0]+a[2], b[0]+b[2]), max(a[1]+a[3], b[1]+b[3])
		box[pi] = [4]float64{x0, y0, x1 - x0, y1 - y0}
	}
	out := perfDOM{union: map[int][4]float64{}, children: map[int][]int{}, text: map[int]bool{}}
	for i := 0; i < n; i++ {
		b := int(d.Nodes.BackendNodeID[i])
		out.order = append(out.order, b)
		if has[i] {
			out.union[b] = box[i]
		} else {
			out.union[b] = [4]float64{}
		}
		out.text[b] = s.Strings[d.Nodes.NodeName[i]] == "#text"
		if pi := d.Nodes.ParentIndex[i]; pi >= 0 {
			pb := int(d.Nodes.BackendNodeID[pi])
			out.children[pb] = append(out.children[pb], b)
		}
	}
	return out
}

func pruned(t *testing.T, p *rod.Page, expand func(backend int) bool) int {
	root, err := proto.AccessibilityGetRootAXNode{}.Call(p)
	if err != nil {
		t.Fatal(err)
	}
	total := 1
	level := []*proto.AccessibilityAXNode{root.Node}
	for len(level) > 0 {
		var next []*proto.AccessibilityAXNode
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, 16)
		for _, n := range level {
			if len(n.ChildIDs) == 0 || (n.BackendDOMNodeID != 0 && !expand(int(n.BackendDOMNodeID))) {
				continue
			}
			if r := n.Role; r != nil && fmt.Sprint(r.Value.Val()) == "StaticText" {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				res, err := proto.AccessibilityGetChildAXNodes{ID: n.NodeID}.Call(p)
				if err != nil {
					return
				}
				mu.Lock()
				next = append(next, res.Nodes...)
				mu.Unlock()
			}()
		}
		wg.Wait()
		total += len(next)
		level = next
	}
	return total
}

func partialTree(t *testing.T, p *rod.Page, cands []int) int {
	seen := map[proto.AccessibilityAXNodeID]bool{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for _, b := range cands {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := proto.AccessibilityGetPartialAXTree{BackendNodeID: proto.DOMBackendNodeID(b), FetchRelatives: true}.Call(p)
			if err != nil {
				return
			}
			mu.Lock()
			for _, n := range res.Nodes {
				seen[n.NodeID] = true
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return len(seen)
}
