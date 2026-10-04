package snapshot

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// FormatOptions controla a compactação do texto.
type FormatOptions struct {
	MaxLines int
	TextMax  int
	ListMax  int
	// ScopeRoot desliga o limite de itens nos contêineres de topo (usado com scope).
	ScopeRoot bool
}

// DefaultFormat são os limites padrão da spec.
// FullTextMax é o tamanho dos textos no snapshot com scope (o mesmo limite do script da página).
const FullTextMax = 300

func DefaultFormat() FormatOptions {
	return FormatOptions{MaxLines: 300, TextMax: 80, ListMax: 5}
}

// itemRole diz qual papel de filho é contado/limitado em cada contêiner.
var itemRole = map[string]string{
	"table": "row", "grid": "row", "list": "listitem", "listbox": "option",
	"combobox": "option", "menu": "menuitem", "tree": "treeitem",
}

var countLabel = map[string]string{
	"table": "rows", "grid": "rows", "list": "items", "listbox": "options",
	"combobox": "options", "menu": "items", "tree": "items",
}

// Line é um nó já posicionado na saída.
type Line struct {
	Node   *Node
	Parent *Node
	Depth  int
	Shown  int // itens visíveis em contêineres limitados
}

// Flatten aplica o limite de itens por lista e devolve as linhas na ordem de exibição.
func Flatten(nodes []*Node, opts FormatOptions) []Line {
	var out []Line
	var visit func(list []*Node, parent *Node, depth int)
	visit = func(list []*Node, parent *Node, depth int) {
		for _, n := range list {
			idx := len(out)
			out = append(out, Line{Node: n, Parent: parent, Depth: depth})
			children := n.Children
			if item, ok := itemRole[n.Role]; ok {
				limit := opts.ListMax
				if opts.ScopeRoot && depth == 0 {
					limit = -1
				}
				children, out[idx].Shown = limitItems(children, item, limit)
			}
			visit(children, n, depth+1)
		}
	}
	visit(nodes, nil, 0)
	return out
}

func limitItems(children []*Node, item string, limit int) ([]*Node, int) {
	kept := make([]*Node, 0, len(children))
	shown := 0
	for _, c := range children {
		if c.Role != item {
			kept = append(kept, c)
			continue
		}
		if limit >= 0 && shown >= limit {
			continue
		}
		shown++
		kept = append(kept, c)
	}
	return kept, shown
}

// Truncate corta textos longos em runas, terminando com reticências.
func Truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}

// NodeText é a linha de um nó, sem indentação e sem prefixo de diff.
func NodeText(l Line, opts FormatOptions) string {
	n := l.Node
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s", n.Ref, n.Role)
	if n.Name != "" {
		b.WriteString(" " + strconv.Quote(Truncate(n.Name, opts.TextMax)))
	}
	if n.Level > 0 {
		fmt.Fprintf(&b, " level=%d", n.Level)
	}
	if n.Type != "" {
		b.WriteString(" type=" + n.Type)
	}
	if n.Value != nil {
		b.WriteString(" value=" + strconv.Quote(Truncate(*n.Value, opts.TextMax)))
	}
	for _, s := range n.States {
		if s == "invalid" && n.InvalidMessage != "" {
			s += "=" + strconv.Quote(Truncate(n.InvalidMessage, opts.TextMax))
		}
		b.WriteString(" " + s)
	}
	if n.Count != nil {
		fmt.Fprintf(&b, " %s=%d", countLabel[n.Role], *n.Count)
		if l.Shown < *n.Count {
			fmt.Fprintf(&b, " (showing %d)", l.Shown)
		}
	}
	return b.String()
}

// Header descreve página, viewport e rolagem.
func Header(p *Page) string {
	return fmt.Sprintf("page: %s (%s)\nviewport: %dx%d, scroll: %d/%d",
		strconv.Quote(p.Title), p.URL, p.Viewport[0], p.Viewport[1], p.Scroll[0], p.Scroll[1])
}

// Format gera o snapshot completo em texto.
func Format(p *Page, opts FormatOptions) string {
	var b strings.Builder
	b.WriteString(Header(p))
	lines := Flatten(p.Nodes, opts)
	if len(lines) == 0 {
		b.WriteString("\n(no visible elements)")
		return b.String()
	}
	limit := len(lines)
	if opts.MaxLines > 0 && limit > opts.MaxLines {
		limit = opts.MaxLines
	}
	for _, l := range lines[:limit] {
		b.WriteString("\n" + strings.Repeat("  ", l.Depth) + NodeText(l, opts))
	}
	if limit < len(lines) {
		fmt.Fprintf(&b, "\n… %d more elements (use snapshot with scope=%s)", len(lines)-limit, scopeHint(lines, limit))
	}
	return b.String()
}

// scopeHint escolhe o ancestral de topo do primeiro elemento omitido.
func scopeHint(lines []Line, firstOmitted int) string {
	for i := firstOmitted; i >= 0; i-- {
		if lines[i].Depth == 0 {
			return lines[i].Node.Ref
		}
	}
	return lines[firstOmitted].Node.Ref
}
