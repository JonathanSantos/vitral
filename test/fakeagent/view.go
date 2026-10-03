// Package fakeagent é uma "IA burra" e determinística para os testes e2e: só enxerga o texto que
// as ferramentas devolvem e só reage a um erro fazendo o que a própria mensagem manda.
package fakeagent

import (
	"regexp"
	"strconv"
	"strings"
)

// Element é o que o agente sabe sobre um elemento: só o que apareceu nas respostas.
type Element struct {
	Ref    string
	Role   string
	Name   string
	Line   string
	Parent string
}

// View é a tela como o agente a entende, montada a partir de snapshots e diffs.
type View struct {
	elems map[string]*Element
	order []string
}

// NewView cria uma visão vazia.
func NewView() *View { return &View{elems: map[string]*Element{}} }

var (
	nodeRe  = regexp.MustCompile(`^(\s*)\[(e\d+)\] (\S+)(?: ("(?:[^"\\]|\\.)*"))?(.*)$`)
	inRe    = regexp.MustCompile(`\(in \S+ (e\d+)\)\s*$`)
	removed = regexp.MustCompile(`^- \[(e\d+)\]`)
)

func parseNode(line string) (indent int, el *Element, ok bool) {
	m := nodeRe.FindStringSubmatch(line)
	if m == nil {
		return 0, nil, false
	}
	name := ""
	if m[4] != "" {
		if s, err := strconv.Unquote(m[4]); err == nil {
			name = s
		}
	}
	return len(m[1]) / 2, &Element{Ref: m[2], Role: m[3], Name: name, Line: strings.TrimSpace(line)}, true
}

// Absorb incorpora uma resposta de ferramenta: snapshot completo (troca a visão) ou diff.
func (v *View) Absorb(text string) {
	lines := strings.Split(text, "\n")
	if isSnapshot(lines) {
		v.elems, v.order = map[string]*Element{}, nil
	}
	var stack []string // refs por profundidade, para achar o pai pela indentação
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+ "):
			v.absorbAdded(strings.TrimPrefix(line, "+ "), &stack)
		case strings.HasPrefix(line, "~ "):
			if _, el, ok := parseNode(strings.TrimPrefix(line, "~ ")); ok {
				if cur := v.elems[el.Ref]; cur != nil {
					cur.Name, cur.Line = el.Name, el.Line
				}
			}
		case removed.MatchString(line):
			v.remove(removed.FindStringSubmatch(line)[1])
		default:
			indent, el, ok := parseNode(line)
			if !ok {
				continue
			}
			stack = append(stack[:min(indent, len(stack))], el.Ref)
			if indent > 0 && indent <= len(stack)-1 {
				el.Parent = stack[indent-1]
			}
			v.put(el)
		}
	}
}

func isSnapshot(lines []string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "page: ") {
			return true
		}
	}
	return false
}

func (v *View) absorbAdded(line string, stack *[]string) {
	parent := ""
	if m := inRe.FindStringSubmatch(line); m != nil {
		parent = m[1]
		line = strings.TrimSpace(inRe.ReplaceAllString(line, ""))
	}
	indent, el, ok := parseNode(line)
	if !ok {
		return
	}
	*stack = append((*stack)[:min(indent, len(*stack))], el.Ref)
	switch {
	case parent != "":
		el.Parent = parent
	case indent > 0:
		el.Parent = (*stack)[indent-1]
	}
	v.put(el)
}

func (v *View) put(el *Element) {
	if _, ok := v.elems[el.Ref]; !ok {
		v.order = append(v.order, el.Ref)
	}
	v.elems[el.Ref] = el
}

func (v *View) remove(ref string) {
	delete(v.elems, ref)
	for _, el := range v.elems {
		if el.Parent == ref {
			v.remove(el.Ref)
		}
	}
}

// Find devolve o primeiro elemento (na ordem da tela) com um dos papéis e o nome dado.
func (v *View) Find(name string, roles ...string) *Element {
	want := normalize(name)
	for _, ref := range v.order {
		el := v.elems[ref]
		if el == nil || normalize(el.Name) != want {
			continue
		}
		for _, r := range roles {
			if el.Role == r {
				return el
			}
		}
	}
	return nil
}

// Children devolve os elementos diretamente dentro de ref.
func (v *View) Children(ref string) []*Element {
	var out []*Element
	for _, r := range v.order {
		if el := v.elems[r]; el != nil && el.Parent == ref {
			out = append(out, el)
		}
	}
	return out
}

func normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
