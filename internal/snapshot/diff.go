package snapshot

import (
	"fmt"
	"strconv"
	"strings"
)

// Diff resume o que mudou entre dois snapshots do mesmo documento.
type Diff struct {
	URL          string // nova URL, se mudou
	Title        string // novo título, se mudou
	TitleChanged bool
	Added        []string
	Removed      []string
	Changed      []string
	LeftViewport int
}

// Empty indica que nada mudou.
func (d Diff) Empty() bool {
	return d.URL == "" && !d.TitleChanged && len(d.Added) == 0 && len(d.Removed) == 0 &&
		len(d.Changed) == 0 && d.LeftViewport == 0
}

// Compute compara prev e curr. Elementos que saíram da viewport mas ainda existem só são contados.
func Compute(prev, curr *Page, opts FormatOptions) Diff {
	var d Diff
	if prev.URL != curr.URL {
		d.URL = curr.URL
	}
	if prev.Title != curr.Title {
		d.Title, d.TitleChanged = curr.Title, true
	}

	prevNodes, _ := Index(prev)
	currNodes, _ := Index(curr)
	offscreen := toSet(curr.Offscreen)

	prevLines := map[string]string{}
	for _, l := range Flatten(prev.Nodes, FormatOptions{ListMax: -1, TextMax: opts.TextMax}) {
		prevLines[l.Node.Ref] = NodeText(Line{Node: l.Node, Shown: l.Shown}, opts)
	}

	d.Added, d.Changed = addedAndChanged(curr, prevNodes, prevLines, opts)

	removed := map[string]bool{}
	for ref := range prevNodes {
		if _, still := currNodes[ref]; !still && !offscreen[ref] {
			removed[ref] = true
		}
	}
	Walk(prev.Nodes, func(n, parent *Node) {
		if _, still := currNodes[n.Ref]; still {
			return
		}
		if offscreen[n.Ref] {
			d.LeftViewport++
			return
		}
		if parent != nil && removed[parent.Ref] {
			return
		}
		line := prevLines[n.Ref]
		if inner := countDescendants(n); inner > 0 {
			line += fmt.Sprintf(" (+%d inside)", inner)
		}
		d.Removed = append(d.Removed, line)
	})
	return d
}

func addedAndChanged(curr *Page, prevNodes map[string]*Node, prevLines map[string]string, opts FormatOptions) ([]string, []string) {
	var added, changed []string
	relDepth := map[string]int{}
	isAdded := map[string]bool{}
	// Contêineres limitados ao comparar mudanças usam a contagem sem limite, como em prevLines.
	unlimited := map[string]int{}
	for _, l := range Flatten(curr.Nodes, FormatOptions{ListMax: -1}) {
		unlimited[l.Node.Ref] = l.Shown
	}
	for _, l := range Flatten(curr.Nodes, opts) {
		ref := l.Node.Ref
		if _, existed := prevNodes[ref]; existed {
			text := NodeText(Line{Node: l.Node, Shown: unlimited[ref]}, opts)
			if text != prevLines[ref] {
				changed = append(changed, NodeText(l, opts))
			}
			continue
		}
		isAdded[ref] = true
		depth := 0
		suffix := ""
		if l.Parent != nil && isAdded[l.Parent.Ref] {
			depth = relDepth[l.Parent.Ref] + 1
		}
		if l.Parent != nil && !isAdded[l.Parent.Ref] {
			suffix = fmt.Sprintf(" (in %s %s)", l.Parent.Role, l.Parent.Ref)
		}
		relDepth[ref] = depth
		added = append(added, strings.Repeat("  ", depth)+NodeText(l, opts)+suffix)
	}
	return added, changed
}

func countDescendants(n *Node) int {
	total := 0
	Walk(n.Children, func(*Node, *Node) { total++ })
	return total
}

func toSet(list []string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, s := range list {
		set[s] = true
	}
	return set
}

// FormatDiff gera o texto do diff. Devolve vazio quando não há mudanças.
func FormatDiff(d Diff) string {
	var lines []string
	if d.URL != "" {
		lines = append(lines, "url: "+d.URL)
	}
	if d.TitleChanged {
		lines = append(lines, "title: "+strconv.Quote(d.Title))
	}
	for _, l := range d.Added {
		lines = append(lines, "+ "+l)
	}
	for _, l := range d.Removed {
		lines = append(lines, "- "+l)
	}
	for _, l := range d.Changed {
		lines = append(lines, "~ "+l)
	}
	if d.LeftViewport > 0 {
		lines = append(lines, fmt.Sprintf("- %d elements left the viewport", d.LeftViewport))
	}
	return strings.Join(lines, "\n")
}

// Summarize escolhe entre diff e snapshot completo: navegação ou diff grande devolvem o snapshot.
func Summarize(prev, curr *Page, opts FormatOptions) string {
	full := Format(curr, opts)
	if prev == nil || prev.DocID != curr.DocID {
		return full
	}
	d := Compute(prev, curr, opts)
	if d.Empty() {
		return "no visible changes"
	}
	text := FormatDiff(d)
	if len(text) > len(full) {
		return full
	}
	return text
}
