package flow

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jors/browser-mcp/internal/snapshot"
)

// contextRoles são ancestrais que ajudam a desambiguar um elemento repetido.
var contextRoles = map[string]bool{
	"dialog": true, "table": true, "grid": true, "row": true, "form": true, "list": true, "listitem": true,
	"region": true, "navigation": true, "group": true, "tabpanel": true, "menu": true, "listbox": true,
	"banner": true, "contentinfo": true, "complementary": true, "main": true,
}

type located struct {
	node      *snapshot.Node
	ancestors []*snapshot.Node
}

func locate(nodes []*snapshot.Node) []located {
	var out []located
	var visit func(list []*snapshot.Node, path []*snapshot.Node)
	visit = func(list []*snapshot.Node, path []*snapshot.Node) {
		for _, n := range list {
			out = append(out, located{node: n, ancestors: path})
			visit(n.Children, append(append([]*snapshot.Node{}, path...), n))
		}
	}
	visit(nodes, nil)
	return out
}

func sameText(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

func hasAncestor(l located, c *Context) bool {
	if c == nil {
		return true
	}
	for _, a := range l.ancestors {
		if a.Role == c.Role && sameText(a.Name, c.Name) {
			return true
		}
	}
	return false
}

func matching(all []located, role, name string, within *Context) []located {
	var out []located
	for _, l := range all {
		if l.node.Role == role && sameText(l.node.Name, name) && hasAncestor(l, within) {
			out = append(out, l)
		}
	}
	return out
}

// Describe gera o seletor semântico mínimo que acha a ref na página (role+name, depois contexto, depois nth).
func Describe(page *snapshot.Page, ref string) (Selector, error) {
	all := locate(page.Nodes)
	var target *located
	for i := range all {
		if all[i].node.Ref == ref {
			target = &all[i]
			break
		}
	}
	if target == nil {
		return Selector{}, fmt.Errorf("element %s has no role/name in the snapshot", ref)
	}
	sel := Selector{Role: target.node.Role, Name: target.node.Name}
	matches := matching(all, sel.Role, sel.Name, nil)
	if len(matches) == 1 {
		return sel, nil
	}
	for i := len(target.ancestors) - 1; i >= 0; i-- {
		a := target.ancestors[i]
		if a.Name == "" || !contextRoles[a.Role] {
			continue
		}
		within := &Context{Role: a.Role, Name: a.Name}
		narrowed := matching(all, sel.Role, sel.Name, within)
		if len(narrowed) < len(matches) {
			sel.Within, matches = within, narrowed
			break
		}
	}
	for i, m := range matches {
		if m.node.Ref == ref && i > 0 {
			sel.Nth = i + 1
		}
	}
	return sel, nil
}

// Resolve acha a ref atual de um seletor. O erro explica o que existe na página para ajudar a corrigir.
func Resolve(page *snapshot.Page, sel Selector) (string, error) {
	all := locate(page.Nodes)
	matches := matching(all, sel.Role, sel.Name, sel.Within)
	nth := max(sel.Nth, 1)
	if len(matches) >= nth {
		return matches[nth-1].node.Ref, nil
	}
	if len(matches) > 0 {
		return "", fmt.Errorf("only %d %s found, step wants the %s", len(matches), sel.Describe(), ordinal(nth))
	}
	return "", fmt.Errorf("no %s found. %s", sel.Describe(), similar(all, sel.Role))
}

// similar lista nomes de elementos do mesmo papel, para a IA perceber um botão renomeado.
func similar(all []located, role string) string {
	var names []string
	seen := map[string]bool{}
	for _, l := range all {
		if l.node.Role != role || seen[l.node.Name] {
			continue
		}
		seen[l.node.Name] = true
		names = append(names, strconv.Quote(snapshot.Truncate(l.node.Name, 60)))
		if len(names) == 10 {
			break
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("There is no %s on the page.", role)
	}
	return fmt.Sprintf("%s elements on the page: %s.", role, strings.Join(names, ", "))
}
