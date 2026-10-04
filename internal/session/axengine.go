package session

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/snapshot"
)

// Motor "ax" do snapshot (protótipo, --snapshot-engine=ax): Accessibility.getFullAXTree +
// DOMSnapshot.captureSnapshot unidos por backendNodeId. As refs ficam num mapa em Go; para as ações
// atuais funcionarem sem migração, o protótipo grava o atributo de ref no DOM pelo CDP e o script da
// página as registra (adopt). Só o frame principal.

// axRefs é o mapa de refs do documento atual.
type axRefs struct {
	docID     string
	byBackend map[int]string
	byRef     map[string]int
	next      int
	written   map[int]bool // backends que já têm o atributo no DOM
}

// AXStats mede o que o motor ax enxergou na última captura (para a tabela comparativa).
type AXStats struct {
	ClosedShadowNodes int // nós incluídos que estão dentro de shadow root fechado (ações não alcançam)
	Adopted           int
}

var axStyles = []string{"display", "visibility", "opacity", "cursor"}

func axValue(v *proto.AccessibilityAXValue) string {
	if v == nil || v.Value.Nil() {
		return ""
	}
	switch val := v.Value.Val().(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	}
	return v.Value.String()
}

// AXStats devolve as medidas da última captura do motor ax.
func (s *Session) AXStats() AXStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.axStats
}

// captureAX é o capture do motor ax.
func (s *Session) captureAX(ctx context.Context, opts snapshot.Options) (*snapshot.Page, error) {
	p := s.current
	if p == nil {
		return nil, fmt.Errorf("no page")
	}
	pc := p.Context(ctx)
	var meta struct {
		DocID    string  `json:"docId"`
		URL      string  `json:"url"`
		Title    string  `json:"title"`
		Viewport [2]int  `json:"viewport"`
		Scroll   [2]int  `json:"scroll"`
		DPR      float64 `json:"dpr"`
		Counter  int     `json:"counter"`
	}
	js := `() => { const se = document.scrollingElement || document.documentElement;
		return { docId: window.__vitral.docId(), url: location.href, title: document.title,
			viewport: [innerWidth, innerHeight], scroll: [Math.round(se.scrollTop), Math.max(0, se.scrollHeight - innerHeight)],
			dpr: devicePixelRatio || 1, counter: window.__vitral.counter() }; }`
	if err := s.evalInto(ctx, &meta, js); err != nil {
		return nil, err
	}
	if s.ax == nil || s.ax.docID != meta.DocID {
		s.ax = &axRefs{docID: meta.DocID, byBackend: map[int]string{}, byRef: map[string]int{}, written: map[int]bool{}}
	}
	s.ax.next = max(s.ax.next, meta.Counter)

	snap, err := proto.DOMSnapshotCaptureSnapshot{ComputedStyles: axStyles}.Call(pc)
	if err != nil {
		return nil, fmt.Errorf("dom snapshot: %w", err)
	}
	if len(snap.Documents) == 0 {
		return nil, fmt.Errorf("dom snapshot: no document")
	}
	dom, skip, closed := axDOM(snap.Documents[0], snap.Strings)

	tree, err := proto.AccessibilityGetFullAXTree{}.Call(pc)
	if err != nil {
		return nil, fmt.Errorf("ax tree: %w", err)
	}
	in := snapshot.AXInput{
		AX: map[string]*snapshot.AXNode{}, DOM: dom, Skip: skip,
		Viewport: [2]float64{float64(meta.Viewport[0]), float64(meta.Viewport[1])},
	}
	for i, n := range tree.Nodes {
		an := &snapshot.AXNode{
			ID: string(n.NodeID), Ignored: n.Ignored, Role: axValue(n.Role), Name: axValue(n.Name),
			Backend: int(n.BackendDOMNodeID), Props: map[string]string{},
		}
		if n.Value != nil {
			v := axValue(n.Value)
			an.Value = &v
		}
		for _, c := range n.ChildIDs {
			an.Children = append(an.Children, string(c))
		}
		for _, pr := range n.Properties {
			an.Props[string(pr.Name)] = axValue(pr.Value)
		}
		in.AX[an.ID] = an
		if i == 0 {
			in.Root = an.ID
		}
	}
	var fresh []int
	in.Ref = func(backend int) string {
		if r, ok := s.ax.byBackend[backend]; ok {
			return r
		}
		s.ax.next++
		r := "e" + strconv.Itoa(s.ax.next)
		s.ax.byBackend[backend], s.ax.byRef[r] = r, backend
		fresh = append(fresh, backend)
		return r
	}
	scope := 0
	if opts.Scope != "" {
		b, ok := s.ax.byRef[opts.Scope]
		if !ok {
			return nil, snapshot.ErrRefNotFound{Ref: opts.Scope}
		}
		scope = b
	}
	nodes := snapshot.BuildAX(in, opts.Full, scope)

	s.axStats = AXStats{}
	snapshot.Walk(nodes, func(n, _ *snapshot.Node) {
		if closed[s.ax.byRef[n.Ref]] {
			s.axStats.ClosedShadowNodes++
		}
	})
	if err := s.axWriteRefs(ctx, fresh); err != nil {
		return nil, err
	}
	// Validação nativa (:user-invalid e validationMessage) não está na árvore de acessibilidade nem no
	// DOMSnapshot: uma avaliação só para os campos inválidos.
	var invalid map[string]string
	_ = s.evalInto(ctx, &invalid, `() => { const out = {};
		for (const el of document.querySelectorAll('[data-vitral-ref]')) {
			let bad = false; try { bad = el.matches(':user-invalid'); } catch (_) {}
			if (bad) out[el.getAttribute('data-vitral-ref')] = (el.validationMessage || '').replace(/\s+/g, ' ').trim();
		} return out; }`)
	if len(invalid) > 0 {
		snapshot.Walk(nodes, func(n, _ *snapshot.Node) {
			msg, ok := invalid[n.Ref]
			if !ok {
				return
			}
			if !slices.Contains(n.States, "invalid") {
				n.States = snapshot.InsertState(n.States, "invalid")
			}
			n.InvalidMessage = msg
		})
	}

	page := &snapshot.Page{
		DocID: meta.DocID, URL: meta.URL, Title: meta.Title, Viewport: meta.Viewport,
		Scroll: meta.Scroll, DPR: meta.DPR, Nodes: nodes, Offscreen: []string{},
	}
	if page.Nodes == nil {
		page.Nodes = []*snapshot.Node{}
	}
	for _, ref := range opts.Known {
		d := dom[s.ax.byRef[ref]]
		for d != nil && d.Tag == "OPTION" {
			d = dom[d.Parent]
		}
		if d != nil && d.HasLayout && d.Display != "none" && d.Visibility != "hidden" && !axNear(d.Box, in.Viewport) {
			page.Offscreen = append(page.Offscreen, ref)
		}
	}
	return page, nil
}

func axNear(box [4]float64, vp [2]float64) bool {
	mx, my := vp[0]*0.25, vp[1]*0.25
	return box[0]+box[2] >= -mx && box[0] <= vp[0]+mx && box[1]+box[3] >= -my && box[1] <= vp[1]+my
}

// axWriteRefs grava o atributo de ref nos elementos novos (protótipo) e pede ao script que os registre.
func (s *Session) axWriteRefs(ctx context.Context, backends []int) error {
	pc := s.current.Context(ctx)
	var todo []proto.DOMBackendNodeID
	for _, b := range backends {
		if !s.ax.written[b] {
			todo = append(todo, proto.DOMBackendNodeID(b))
		}
	}
	if len(todo) > 0 {
		if _, err := (proto.DOMGetDocument{Depth: new(0)}).Call(pc); err != nil {
			return fmt.Errorf("dom document: %w", err)
		}
		res, err := proto.DOMPushNodesByBackendIDsToFrontend{BackendNodeIDs: todo}.Call(pc)
		if err != nil {
			return fmt.Errorf("push nodes: %w", err)
		}
		// Uma chamada por nó: em paralelo, para o primeiro snapshot de uma página grande não demorar.
		var wg sync.WaitGroup
		sem := make(chan struct{}, 16)
		for i, id := range res.NodeIDs {
			if id == 0 {
				continue
			}
			ref := s.ax.byBackend[int(todo[i])]
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				_ = proto.DOMSetAttributeValue{NodeID: id, Name: "data-vitral-ref", Value: ref}.Call(pc)
			}()
			s.ax.written[int(todo[i])] = true
		}
		wg.Wait()
	}
	var adopted struct {
		Adopted int `json:"adopted"`
		Counter int `json:"counter"`
	}
	if err := s.evalInto(ctx, &adopted, `() => window.__vitral.adopt()`); err != nil {
		return err
	}
	s.axStats.Adopted = adopted.Adopted
	s.ax.next = max(s.ax.next, adopted.Counter)
	return nil
}

// axDOM converte o documento do DOMSnapshot: nós por backend, a subárvore do overlay (para pular) e os
// nós dentro de shadow root fechado.
func axDOM(doc *proto.DOMSnapshotDocumentSnapshot, strs []string) (map[int]*snapshot.DOMNode, map[int]bool, map[int]bool) {
	str := func(i proto.DOMSnapshotStringIndex) string {
		if int(i) >= 0 && int(i) < len(strs) {
			return strs[i]
		}
		return ""
	}
	nodes := doc.Nodes
	n := len(nodes.BackendNodeID)
	out := make(map[int]*snapshot.DOMNode, n)
	byIndex := make([]*snapshot.DOMNode, n)
	for i := 0; i < n; i++ {
		d := &snapshot.DOMNode{Backend: int(nodes.BackendNodeID[i]), Attrs: map[string]string{}, Opacity: 1}
		if i < len(nodes.NodeName) {
			d.Tag = str(nodes.NodeName[i])
		}
		if i < len(nodes.NodeValue) {
			d.Text = str(nodes.NodeValue[i])
		}
		if i < len(nodes.Attributes) {
			a := nodes.Attributes[i]
			for k := 0; k+1 < len(a); k += 2 {
				d.Attrs[strings.ToLower(str(proto.DOMSnapshotStringIndex(a[k])))] = str(proto.DOMSnapshotStringIndex(a[k+1]))
			}
		}
		out[d.Backend] = d
		byIndex[i] = d
	}
	for i := 0; i < n; i++ {
		if i < len(nodes.ParentIndex) && nodes.ParentIndex[i] >= 0 {
			parent := byIndex[nodes.ParentIndex[i]]
			byIndex[i].Parent = parent.Backend
			parent.Children = append(parent.Children, byIndex[i].Backend)
		}
	}
	if nodes.IsClickable != nil {
		for _, i := range nodes.IsClickable.Index {
			byIndex[i].Clickable = true
		}
	}
	if nodes.OptionSelected != nil {
		for _, i := range nodes.OptionSelected.Index {
			byIndex[i].OptionSelected = true
		}
	}
	sx, sy := 0.0, 0.0
	if doc.ScrollOffsetX != nil {
		sx = *doc.ScrollOffsetX
	}
	if doc.ScrollOffsetY != nil {
		sy = *doc.ScrollOffsetY
	}
	lay := doc.Layout
	for k, idx := range lay.NodeIndex {
		d := byIndex[idx]
		d.HasLayout = true
		if k < len(lay.Bounds) && len(lay.Bounds[k]) == 4 {
			b := lay.Bounds[k]
			d.Box = [4]float64{b[0] - sx, b[1] - sy, b[2], b[3]}
		}
		if k < len(lay.Styles) {
			st := lay.Styles[k]
			get := func(j int) string {
				if j < len(st) {
					return str(proto.DOMSnapshotStringIndex(st[j]))
				}
				return ""
			}
			d.Display, d.Visibility, d.Cursor = get(0), get(1), get(3)
			if o, err := strconv.ParseFloat(get(2), 64); err == nil {
				d.Opacity = o
			}
		}
	}
	// Nós sem layout: display vem do pai mais próximo com layout? Não; sem layout = não renderizado.
	for _, d := range out {
		if !d.HasLayout && d.Display == "" {
			d.Display = "none"
		}
	}
	skip, closed := map[int]bool{}, map[int]bool{}
	var mark func(b int, set map[int]bool)
	mark = func(b int, set map[int]bool) {
		set[b] = true
		for _, c := range out[b].Children {
			mark(c, set)
		}
	}
	for i, d := range byIndex {
		if d.Tag == "VITRAL-OVERLAY" {
			mark(d.Backend, skip)
		}
		if nodes.ShadowRootType != nil {
			for k, si := range nodes.ShadowRootType.Index {
				if si == i && str(nodes.ShadowRootType.Value[k]) == "closed" {
					mark(d.Backend, closed)
				}
			}
		}
	}
	return out, skip, closed
}
