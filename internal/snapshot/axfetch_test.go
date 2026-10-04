package snapshot

import (
	"slices"
	"testing"
)

// domTree monta um DOM sintético para o plano de busca: viewport 1000x800 (margem de 250x200).
type domTree struct{ dom map[int]*DOMNode }

func newDOMTree() *domTree {
	t := &domTree{dom: map[int]*DOMNode{}}
	t.dom[1] = &DOMNode{Backend: 1, Tag: "#document"}
	return t
}

// add cria o nó b sob parent; box zero = sem layout.
func (t *domTree) add(b, parent int, tag string, box [4]float64) {
	t.dom[b] = &DOMNode{Backend: b, Parent: parent, Tag: tag, Box: box, HasLayout: box[2] > 0}
	t.dom[parent].Children = append(t.dom[parent].Children, b)
}

func TestPlanAXFetch(t *testing.T) {
	vp := [2]float64{1000, 800}
	tr := newDOMTree()
	tr.add(2, 1, "HTML", [4]float64{0, 0, 1000, 5000})
	tr.add(3, 2, "BODY", [4]float64{0, 0, 1000, 5000})
	// Linhas irmãs inteiras dentro da margem: subárvores, uma chamada de ancestrais para as duas.
	tr.add(10, 3, "DIV", [4]float64{0, 0, 500, 40})
	tr.add(11, 10, "#text", [4]float64{0, 0, 100, 20})
	tr.add(12, 3, "DIV", [4]float64{0, 50, 500, 40})
	tr.add(13, 12, "#text", [4]float64{0, 50, 100, 20})
	// Lista alta que atravessa a borda: desce; o item perto vem como subárvore, o longe fica de fora.
	tr.add(20, 3, "UL", [4]float64{0, 900, 500, 3000})
	tr.add(21, 20, "LI", [4]float64{0, 900, 500, 40})
	tr.add(22, 21, "#text", [4]float64{0, 900, 100, 20})
	tr.add(23, 20, "LI", [4]float64{0, 3000, 500, 40})
	tr.add(24, 23, "#text", [4]float64{0, 3000, 100, 20})
	// Folha solta que atravessa a borda e um <select> (opções sem layout) que também atravessa.
	tr.add(30, 3, "IMG", [4]float64{0, 950, 100, 200})
	tr.add(31, 3, "SELECT", [4]float64{0, 980, 100, 30})
	tr.add(32, 31, "OPTION", [4]float64{})
	// Pai fora da tela com filho absoluto perto: a união da subárvore conta.
	tr.add(40, 3, "DIV", [4]float64{0, 4000, 100, 20})
	tr.add(41, 40, "BUTTON", [4]float64{0, 960, 50, 300})
	// Comentário e nó sem layout nenhum ficam de fora.
	tr.add(50, 3, "#comment", [4]float64{0, 10, 10, 10})
	tr.add(51, 3, "SCRIPT", [4]float64{})
	tr.add(52, 51, "#text", [4]float64{})
	// O overlay do Vitral é pulado, e quem o contém não vem como subárvore inteira.
	tr.add(60, 2, "VITRAL-OVERLAY", [4]float64{0, 0, 10, 10})
	skip := map[int]bool{60: true}

	plan := PlanAXFetch(tr.dom, vp, 0, skip)
	sort := func(s []int) []int { s = slices.Clone(s); slices.Sort(s); return s }
	if got, want := sort(plan.Subtrees), []int{10, 12, 21}; !slices.Equal(got, want) {
		t.Errorf("subtrees %v, want %v", got, want)
	}
	if got, want := sort(plan.Partial), []int{10, 21, 30, 31, 41}; !slices.Equal(got, want) {
		t.Errorf("partial %v, want %v", got, want)
	}

	// Sem o overlay, a página inteira cabe? Não (a página tem 5000 px): o plano é o mesmo.
	if p := PlanAXFetch(tr.dom, vp, 0, nil); !slices.Equal(sort(p.Subtrees), []int{10, 12, 21}) {
		t.Errorf("without skip: %v", p.Subtrees)
	}

	// Com scope: a subárvore do scope numa chamada só.
	if p := PlanAXFetch(tr.dom, vp, 20, skip); !slices.Equal(p.Subtrees, []int{20}) || len(p.Partial) != 0 {
		t.Errorf("scope plan %+v", p)
	}
}

func TestPlanAXFetchSmallPage(t *testing.T) {
	vp := [2]float64{1000, 800}
	tr := newDOMTree()
	tr.add(2, 1, "HTML", [4]float64{0, 0, 1000, 800})
	tr.add(3, 2, "BODY", [4]float64{8, 8, 984, 400})
	tr.add(4, 3, "H1", [4]float64{8, 8, 984, 40})
	tr.add(5, 4, "#text", [4]float64{8, 8, 200, 40})
	// Página inteira dentro da margem: o <html> vem numa chamada só.
	if p := PlanAXFetch(tr.dom, vp, 0, nil); !slices.Equal(p.Subtrees, []int{2}) || !slices.Equal(p.Partial, []int{2}) {
		t.Errorf("small page %+v", p)
	}
	// Com o overlay dentro, desce: o <body> inteiro vem numa chamada, o overlay fica de fora.
	tr.add(9, 2, "VITRAL-OVERLAY", [4]float64{0, 0, 10, 10})
	if p := PlanAXFetch(tr.dom, vp, 0, map[int]bool{9: true}); !slices.Equal(p.Subtrees, []int{3}) || !slices.Equal(p.Partial, []int{3}) {
		t.Errorf("page with overlay %+v", p)
	}
	// Overlay mais fundo: hasSkipped olha a subárvore toda.
	tr2 := newDOMTree()
	tr2.add(2, 1, "HTML", [4]float64{0, 0, 1000, 800})
	tr2.add(3, 2, "DIV", [4]float64{0, 0, 100, 100})
	tr2.add(4, 3, "DIV", [4]float64{0, 0, 100, 100})
	tr2.add(5, 4, "VITRAL-OVERLAY", [4]float64{0, 0, 10, 10})
	tr2.add(6, 3, "P", [4]float64{0, 50, 100, 20})
	tr2.add(7, 6, "#text", [4]float64{0, 50, 50, 20})
	if p := PlanAXFetch(tr2.dom, vp, 0, map[int]bool{5: true}); !slices.Equal(p.Subtrees, []int{6}) || !slices.Equal(sort2(p.Partial), []int{4, 6}) {
		t.Errorf("deep overlay %+v", p)
	}
}

func sort2(s []int) []int { s = slices.Clone(s); slices.Sort(s); return s }

func TestUnionBox(t *testing.T) {
	a, b := [4]float64{0, 0, 10, 10}, [4]float64{20, 30, 5, 5}
	if got := unionBox(a, b); got != [4]float64{0, 0, 25, 35} {
		t.Errorf("union %v", got)
	}
	if got := unionBox([4]float64{}, b); got != b {
		t.Errorf("empty left %v", got)
	}
	if got := unionBox(a, [4]float64{5, 5, 0, 3}); got != a {
		t.Errorf("empty right %v", got)
	}
	if got := unionBox([4]float64{-5, 2, 3, 0}, b); got != b {
		t.Errorf("zero height left %v", got)
	}
}
