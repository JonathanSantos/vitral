package snapshot

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

var roles = []string{"button", "link", "textbox", "heading", "table", "row", "list", "listitem", "dialog", "alert", "form", "checkbox"}

// genNodes gera uma árvore com refs únicas (o contador vem de fora para não repetir ref).
func genNodes(t *rapid.T, next *int, depth int, label string) []*Node {
	n := rapid.IntRange(0, 4).Draw(t, label+"-n")
	out := make([]*Node, 0, n)
	for i := 0; i < n; i++ {
		*next++
		node := &Node{
			Ref:  fmt.Sprintf("e%d", *next),
			Role: rapid.SampledFrom(roles).Draw(t, label+"-role"),
			Name: rapid.StringMatching(`[A-Za-zÀ-ú0-9 ]{0,12}`).Draw(t, label+"-name"),
		}
		if node.Role == "table" || node.Role == "list" {
			c := rapid.IntRange(0, 9).Draw(t, label+"-count")
			node.Count = &c
		}
		if depth < 3 {
			node.Children = genNodes(t, next, depth+1, label+"c")
		}
		out = append(out, node)
	}
	return out
}

func genPage(t *rapid.T, next *int) *Page {
	return &Page{DocID: "d", URL: "http://x/", Title: "T", Nodes: genNodes(t, next, 0, "root")}
}

func refsOf(nodes []*Node) map[string]bool {
	out := map[string]bool{}
	Walk(nodes, func(n, _ *Node) { out[n.Ref] = true })
	return out
}

// mutate copia a árvore removendo subárvores, mudando nomes e acrescentando nós com refs novas.
func mutate(t *rapid.T, nodes []*Node, next *int, label string) []*Node {
	var out []*Node
	for i, n := range nodes {
		if rapid.IntRange(0, 4).Draw(t, fmt.Sprintf("%s-drop%d", label, i)) == 0 {
			continue
		}
		c := *n
		if rapid.Bool().Draw(t, fmt.Sprintf("%s-rename%d", label, i)) {
			c.Name += "!"
		}
		c.Children = mutate(t, n.Children, next, label+"c")
		out = append(out, &c)
	}
	if rapid.IntRange(0, 2).Draw(t, label+"-add") == 0 {
		*next++
		out = append(out, &Node{Ref: fmt.Sprintf("e%d", *next), Role: "button", Name: "novo"})
	}
	return out
}

func TestPropertyDiffOfSelfIsEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		next := 0
		p := genPage(t, &next)
		if d := Compute(p, p, DefaultFormat()); !d.Empty() {
			t.Fatalf("diff(a, a) not empty: %+v", d)
		}
	})
}

var lineRef = regexp.MustCompile(`^\s*\[(e\d+)\]`)

// Aplicar os + e - de diff(a, b) sobre a dá exatamente os elementos de b.
func TestPropertyDiffRebuildsTarget(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		next := 0
		a := genPage(t, &next)
		b := &Page{DocID: "d", URL: a.URL, Title: a.Title, Nodes: mutate(t, a.Nodes, &next, "m")}
		d := Compute(a, b, FormatOptions{TextMax: 80, ListMax: -1})

		aNodes := Index(a)
		got := refsOf(a.Nodes)
		for _, line := range d.Removed {
			m := lineRef.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("removed line without ref: %q", line)
			}
			delete(got, m[1])
			Walk(aNodes[m[1]].Children, func(n, _ *Node) { delete(got, n.Ref) })
		}
		for _, line := range d.Added {
			m := lineRef.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("added line without ref: %q", line)
			}
			got[m[1]] = true
		}
		want := refsOf(b.Nodes)
		if fmt.Sprint(sortedKeys(toStrings(got))) != fmt.Sprint(sortedKeys(toStrings(want))) {
			t.Fatalf("rebuilt %v, want %v\nadded %v\nremoved %v", got, want, d.Added, d.Removed)
		}
	})
}

func toStrings(m map[string]bool) map[string]string {
	out := map[string]string{}
	for k := range m {
		out[k] = k
	}
	return out
}

// Compactação: nunca passa de maxLines e só mostra refs que existem na entrada.
func TestPropertyCompaction(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		next := 0
		p := genPage(t, &next)
		opts := FormatOptions{
			MaxLines: rapid.IntRange(1, 25).Draw(t, "maxLines"),
			TextMax:  rapid.IntRange(2, 80).Draw(t, "textMax"),
			ListMax:  rapid.IntRange(0, 6).Draw(t, "listMax"),
		}
		out := Format(p, opts)
		known := refsOf(p.Nodes)
		nodeLines := 0
		for _, line := range strings.Split(out, "\n")[2:] {
			m := lineRef.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			nodeLines++
			if !known[m[1]] {
				t.Fatalf("output shows unknown ref %s", m[1])
			}
		}
		if nodeLines > opts.MaxLines {
			t.Fatalf("%d element lines > maxLines %d:\n%s", nodeLines, opts.MaxLines, out)
		}
	})
}
