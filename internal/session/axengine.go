package session

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/snapshot"
	"github.com/JonathanSantos/vitral/internal/world"
)

// Motor "ax" do snapshot (docs/SNAPSHOT-V2.md): Accessibility + DOMSnapshot.captureSnapshot unidos
// por backendNodeId. As refs ficam num mapa em Go (ref ↔ backendNodeId) e as ações acham o elemento
// pelo DOM.resolveNode no mundo isolado: nada é gravado no DOM da página. O script do mundo isolado
// recebe as refs novas (bind) para as buscas reversas (refs no texto, o que cobre um alvo), e as refs
// que ele cria voltam para cá (fresh). Só o frame principal.

// axRefs é o mapa de refs do documento atual.
type axRefs struct {
	docID     string
	byBackend map[int]string
	byRef     map[string]int
	next      int
	bound     map[int]bool // backends cuja ref o script já conhece
}

// AXStats mede o que o motor ax enxergou na última captura (para a tabela comparativa).
type AXStats struct {
	ClosedShadowNodes int // nós incluídos que estão dentro de shadow root fechado
	Bound             int // refs novas registradas no script nesta captura
	Candidates        int // chamadas ao getPartialAXTree (0 = árvore inteira)
	// Tempo de cada fase: script (meta, junto com o DOMSnapshot), o resto do DOMSnapshot, árvore de acessibilidade, montagem, refs e validação.
	Meta, DOM, Tree, Build, Bind, Invalid time.Duration
}

// elem é o elemento de uma ref para as chamadas ao script (world.Elem).
func (s *Session) elem(ref string) world.Elem {
	if s.ax != nil {
		if b, ok := s.ax.byRef[ref]; ok {
			return world.Elem{Backend: b, Ref: ref}
		}
	}
	return world.Elem{Ref: ref}
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

// capture tira um snapshot estruturado.
func (s *Session) capture(ctx context.Context, opts snapshot.Options) (*snapshot.Page, error) {
	p := s.current
	if p == nil {
		return nil, fmt.Errorf("no page")
	}
	pc := p.Context(ctx)
	st := AXStats{}
	mark := time.Now()
	lap := func(d *time.Duration) { now := time.Now(); *d = now.Sub(mark); mark = now }
	var meta struct {
		DocID    string   `json:"docId"`
		URL      string   `json:"url"`
		Title    string   `json:"title"`
		Viewport [2]int   `json:"viewport"`
		Scroll   [2]int   `json:"scroll"`
		DPR      float64  `json:"dpr"`
		Counter  int      `json:"counter"`
		Fresh    []string `json:"fresh"`
	}
	js := `() => { const se = document.scrollingElement || document.documentElement;
		return { docId: window.__vitral.docId(), url: location.href, title: document.title,
			viewport: [innerWidth, innerHeight], scroll: [Math.round(se.scrollTop), Math.max(0, se.scrollHeight - innerHeight)],
			dpr: devicePixelRatio || 1, counter: window.__vitral.counter(), fresh: window.__vitral.fresh() }; }`
	// O DOMSnapshot não depende do script: vai junto com a avaliação dos metadados.
	type domResult struct {
		snap *proto.DOMSnapshotCaptureSnapshotResult
		err  error
	}
	domDone := make(chan domResult, 1)
	go func() {
		snap, err := proto.DOMSnapshotCaptureSnapshot{ComputedStyles: axStyles}.Call(pc)
		domDone <- domResult{snap, err}
	}()
	if err := s.evalInto(ctx, &meta, js); err != nil {
		<-domDone
		return nil, err
	}
	if s.ax == nil || s.ax.docID != meta.DocID {
		s.ax = &axRefs{docID: meta.DocID, byBackend: map[int]string{}, byRef: map[string]int{}, bound: map[int]bool{}}
	}
	s.ax.next = max(s.ax.next, meta.Counter)
	if len(meta.Fresh) > 0 {
		if err := s.axPullFresh(ctx, meta.Fresh); err != nil {
			return nil, err
		}
	}

	lap(&st.Meta)
	dr := <-domDone
	snap, err := dr.snap, dr.err
	if err != nil {
		return nil, fmt.Errorf("dom snapshot: %w", err)
	}
	if len(snap.Documents) == 0 {
		return nil, fmt.Errorf("dom snapshot: no document")
	}
	dom, skip, closed := axDOM(snap.Documents[0], snap.Strings)

	lap(&st.DOM)
	vp := [2]float64{float64(meta.Viewport[0]), float64(meta.Viewport[1])}
	scope := 0
	if opts.Scope != "" {
		b, ok := s.ax.byRef[opts.Scope]
		if !ok || dom[b] == nil {
			return nil, snapshot.ErrRefNotFound{Ref: opts.Scope}
		}
		scope = b
	}
	in := snapshot.AXInput{AX: map[string]*snapshot.AXNode{}, DOM: dom, Skip: skip, Viewport: vp}
	if opts.Full && scope == 0 || s.AXFullTree {
		tree, err := proto.AccessibilityGetFullAXTree{}.Call(pc)
		if err != nil {
			return nil, fmt.Errorf("ax tree: %w", err)
		}
		for _, n := range tree.Nodes {
			axAdd(&in, n)
		}
	} else {
		plan := snapshot.PlanAXFetch(dom, vp, scope, skip)
		st.Candidates = len(plan.Subtrees) + len(plan.Partial)
		if err := axPartial(pc, &in, plan); err != nil {
			return nil, err
		}
	}
	lap(&st.Tree)
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
	nodes := snapshot.BuildAX(in, opts.Full, scope)

	s.axStats = st
	snapshot.Walk(nodes, func(n, _ *snapshot.Node) {
		if closed[s.ax.byRef[n.Ref]] {
			s.axStats.ClosedShadowNodes++
		}
	})
	lap(&s.axStats.Build)
	if err := s.axBind(ctx, fresh); err != nil {
		return nil, err
	}
	lap(&s.axStats.Bind)
	// Validação nativa (:user-invalid e validationMessage) não está na árvore de acessibilidade nem no
	// DOMSnapshot: uma avaliação só para os campos inválidos.
	var invalid map[string]string
	_ = s.evalInto(ctx, &invalid, `() => window.__vitral.invalid()`)
	lap(&s.axStats.Invalid)
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

// axAdd converte um nó da árvore de acessibilidade; a raiz é o documento (RootWebArea sem pai).
func axAdd(in *snapshot.AXInput, n *proto.AccessibilityAXNode) {
	if _, ok := in.AX[string(n.NodeID)]; ok {
		return
	}
	an := &snapshot.AXNode{
		ID: string(n.NodeID), Ignored: n.Ignored, Role: axValue(n.Role), Name: axValue(n.Name),
		Backend: int(n.BackendDOMNodeID), Props: map[string]string{},
	}
	if n.Value != nil {
		v := axValue(n.Value)
		an.Value = &v
	}
	// O getPartialAXTree repete ids nos filhos dos ancestrais ([16 16 38]): um filho conta uma vez.
	seen := map[proto.AccessibilityAXNodeID]bool{}
	for _, c := range n.ChildIDs {
		if !seen[c] {
			seen[c] = true
			an.Children = append(an.Children, string(c))
		}
	}
	for _, pr := range n.Properties {
		an.Props[string(pr.Name)] = axValue(pr.Value)
	}
	in.AX[an.ID] = an
	if n.ParentID == "" && an.Role == "RootWebArea" {
		in.Root = an.ID
	}
}

// axPartial busca só a parte da árvore de acessibilidade que o snapshot usa (snapshot.PlanAXFetch),
// em paralelo. Medido em docs/SNAPSHOT-V2.md.
func axPartial(p *rod.Page, in *snapshot.AXInput, plan snapshot.AXFetch) error {
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	sem := make(chan struct{}, 16)
	run := func(call func() ([]*proto.AccessibilityAXNode, error)) {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			nodes, err := call()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil && p.GetContext().Err() != nil {
					firstErr = err
				}
				return // nó que sumiu entre o DOMSnapshot e a chamada: fica de fora
			}
			for _, n := range nodes {
				axAdd(in, n)
			}
		}()
	}
	for _, b := range plan.Subtrees {
		run(func() ([]*proto.AccessibilityAXNode, error) {
			res, err := proto.AccessibilityQueryAXTree{BackendNodeID: proto.DOMBackendNodeID(b)}.Call(p)
			if err != nil {
				return nil, err
			}
			return res.Nodes, nil
		})
	}
	for _, b := range plan.Partial {
		run(func() ([]*proto.AccessibilityAXNode, error) {
			res, err := proto.AccessibilityGetPartialAXTree{BackendNodeID: proto.DOMBackendNodeID(b), FetchRelatives: true}.Call(p)
			if err != nil {
				return nil, err
			}
			return res.Nodes, nil
		})
	}
	wg.Wait()
	return firstErr
}

// axBind registra no script as refs novas, numa chamada só (os elementos chegam pelo backendNodeId).
func (s *Session) axBind(ctx context.Context, backends []int) error {
	refs := []string{}
	var els []any
	for _, b := range backends {
		if !s.ax.bound[b] {
			s.ax.bound[b] = true
			refs = append(refs, s.ax.byBackend[b])
			els = append(els, world.Elem{Backend: b})
		}
	}
	s.axStats.Bound = len(refs)
	if len(refs) == 0 {
		return nil
	}
	_, err := s.evalJSON(ctx, `(refs, ...els) => window.__vitral.bind(refs, els)`, append([]any{refs}, els...)...)
	return err
}

// axPullFresh traz para o mapa as refs que o script criou (links do texto, findText): o mesmo
// elemento continua com a mesma ref no próximo snapshot.
func (s *Session) axPullFresh(ctx context.Context, refs []string) error {
	ctx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()
	backends, err := s.world.Nodes(ctx, `() => window.__vitral.takeFresh()`)
	if err != nil {
		return err
	}
	for i, b := range backends {
		if i >= len(refs) || b == 0 {
			continue
		}
		if _, ok := s.ax.byBackend[b]; ok {
			continue // já tinha ref pelo Go: a do script vale só para o que já foi lido
		}
		s.ax.byBackend[b], s.ax.byRef[refs[i]] = refs[i], b
		s.ax.bound[b] = true
	}
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
