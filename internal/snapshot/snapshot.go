// Package snapshot monta a página a partir da árvore de acessibilidade (ax.go) e a transforma em
// texto compacto para a IA.
package snapshot

import (
	_ "embed"
	"fmt"
)

// Script é injetado em todo documento novo (mundo isolado) e expõe window.__vitral.
//
//go:embed snapshot.js
var Script string

// Node é um elemento relevante da página.
type Node struct {
	Ref    string   `json:"ref"`
	Role   string   `json:"role"`
	Name   string   `json:"name"`
	Value  *string  `json:"value,omitempty"`
	States []string `json:"states,omitempty"`
	// InvalidMessage é a mensagem de validação nativa do browser (ex.: "Preencha este campo.").
	InvalidMessage string  `json:"invalidMessage,omitempty"`
	Level          int     `json:"level,omitempty"`
	Type           string  `json:"type,omitempty"`
	Count          *int    `json:"count,omitempty"`
	Box            [4]int  `json:"box"`
	Children       []*Node `json:"children,omitempty"`
}

// Page é um snapshot da página.
type Page struct {
	DocID     string   `json:"docId"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	Viewport  [2]int   `json:"viewport"`
	Scroll    [2]int   `json:"scroll"`
	DPR       float64  `json:"dpr"`
	Nodes     []*Node  `json:"nodes"`
	Offscreen []string `json:"offscreen"`
}

// Options controla o que o snapshot inclui (Known: refs do snapshot anterior, para as que saíram da
// área visível).
type Options struct {
	Full  bool     `json:"full,omitempty"`
	Scope string   `json:"scope,omitempty"`
	Known []string `json:"known,omitempty"`
}

// ErrRefNotFound é devolvido quando o escopo pedido não existe mais.
type ErrRefNotFound struct{ Ref string }

func (e ErrRefNotFound) Error() string {
	return fmt.Sprintf("ref %s not found: page changed since last snapshot. Call snapshot.", e.Ref)
}

// Walk visita todos os nós em pré-ordem, com o pai de cada um.
func Walk(nodes []*Node, fn func(n, parent *Node)) {
	var visit func(list []*Node, parent *Node)
	visit = func(list []*Node, parent *Node) {
		for _, n := range list {
			fn(n, parent)
			visit(n.Children, n)
		}
	}
	visit(nodes, nil)
}

// Index mapeia ref -> nó.
func Index(p *Page) map[string]*Node {
	nodes := map[string]*Node{}
	if p == nil {
		return nodes
	}
	Walk(p.Nodes, func(n, _ *Node) { nodes[n.Ref] = n })
	return nodes
}
