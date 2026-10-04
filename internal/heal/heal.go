// Package heal é a autocura dos fluxos: a assinatura de um elemento gravado, a pontuação de cada
// candidato na página atual e a decisão de aceitar ou não. Determinístico e sem LLM: a mesma página
// dá sempre a mesma resposta. Pesos e limiares ficam aqui, num lugar só.
package heal

import (
	"math"
	"regexp"
	"slices"
	"strings"
)

// Signature descreve um elemento gravado além do seletor primário (papel + nome). Só textos de
// controles e cabeçalhos: valores digitados e dados da página nunca entram.
type Signature struct {
	Role        string     `yaml:"role,omitempty"`
	Name        string     `yaml:"name,omitempty"`
	Label       string     `yaml:"label,omitempty"`
	Placeholder string     `yaml:"placeholder,omitempty"`
	Tag         string     `yaml:"tag,omitempty"`
	TestID      string     `yaml:"testId,omitempty"`
	ID          string     `yaml:"id,omitempty"` // só se não parecer gerado (StableID)
	NameAttr    string     `yaml:"nameAttr,omitempty"`
	Href        string     `yaml:"href,omitempty"` // caminho do link, sem query
	InputType   string     `yaml:"inputType,omitempty"`
	Landmark    string     `yaml:"landmark,omitempty"` // "navigation Menu", "main"
	Dialog      string     `yaml:"dialog,omitempty"`
	Heading     string     `yaml:"heading,omitempty"` // título mais próximo acima
	Column      string     `yaml:"column,omitempty"`  // cabeçalho da coluna (tabelas)
	RowHeader   string     `yaml:"rowHeader,omitempty"`
	Ordinal     int        `yaml:"ordinal,omitempty"`  // posição entre iguais (papel + nome) no mesmo contexto
	Same        int        `yaml:"same,omitempty"`     // quantos iguais havia na página ao gravar
	Pos         [2]float64 `yaml:"pos,flow,omitempty"` // centro relativo na página (0..1)
}

// Limiares de aceitação (pedido "próximos passos", 3.2).
const (
	MinScore  = 0.75
	MinMargin = 0.15
)

// weights: quanto cada atributo pesa. A nota é a média ponderada dos atributos que a gravação tem, então
// o nome domina: um botão diferente no mesmo lugar não passa do limiar.
var weights = struct {
	Name, Role, TestID, ID, NameAttr, Href, Label, Placeholder, Tag float64
	Landmark, Dialog, Heading, Column, RowHeader, Ordinal, Pos      float64
}{
	Name: 0.40, Role: 0.10, TestID: 0.30, ID: 0.20, NameAttr: 0.15, Href: 0.15, Label: 0.06, Placeholder: 0.05,
	Tag: 0.02, Landmark: 0.04, Dialog: 0.04, Heading: 0.05, Column: 0.05, RowHeader: 0.03, Ordinal: 0.03, Pos: 0.02,
}

// RoleCompatible diz se um elemento pode virar outro: o mesmo papel, ou botão ↔ link.
func RoleCompatible(a, b string) bool {
	if a == b {
		return true
	}
	pair := map[string]bool{a: true, b: true}
	return pair["button"] && pair["link"]
}

// Score dá a nota (0..1) de um candidato para a assinatura gravada; ok é falso quando o candidato
// nunca serve (papel incompatível ou tipo de input diferente).
func Score(rec, cand Signature) (score float64, ok bool) {
	if !RoleCompatible(rec.Role, cand.Role) || rec.InputType != cand.InputType {
		return 0, false
	}
	var sum, total float64
	add := func(w, s float64) {
		sum += w * s
		total += w
	}
	same := func(a, b string) float64 {
		if Normalize(a) == Normalize(b) {
			return 1
		}
		return 0
	}
	if rec.Name != "" {
		add(weights.Name, Similarity(rec.Name, cand.Name))
	}
	role := 1.0
	if rec.Role != cand.Role {
		role = 0.5
	}
	add(weights.Role, role)
	// Atributos de identidade: contam quando a gravação tem. O id é só bônus (ids mudam numa
	// refatoração sem o elemento mudar): conta quando bate.
	if rec.TestID != "" {
		add(weights.TestID, same(rec.TestID, cand.TestID))
	}
	if rec.ID != "" && rec.ID == cand.ID {
		add(weights.ID, 1)
	}
	if rec.NameAttr != "" {
		add(weights.NameAttr, same(rec.NameAttr, cand.NameAttr))
	}
	if rec.Href != "" {
		add(weights.Href, same(rec.Href, cand.Href))
	}
	if rec.Label != "" {
		add(weights.Label, Similarity(rec.Label, cand.Label))
	}
	if rec.Placeholder != "" {
		add(weights.Placeholder, Similarity(rec.Placeholder, cand.Placeholder))
	}
	add(weights.Tag, same(rec.Tag, cand.Tag))
	// Contexto: sempre conta (estar fora de um dialog também é contexto).
	add(weights.Landmark, same(rec.Landmark, cand.Landmark))
	add(weights.Dialog, same(rec.Dialog, cand.Dialog))
	add(weights.Heading, Similarity(rec.Heading, cand.Heading))
	if rec.Column != "" || cand.Column != "" {
		add(weights.Column, Similarity(rec.Column, cand.Column))
	}
	if rec.RowHeader != "" || cand.RowHeader != "" {
		add(weights.RowHeader, Similarity(rec.RowHeader, cand.RowHeader))
	}
	ordinal := 0.0
	if rec.Ordinal == cand.Ordinal {
		ordinal = 1
	}
	add(weights.Ordinal, ordinal)
	dist := math.Hypot(rec.Pos[0]-cand.Pos[0], rec.Pos[1]-cand.Pos[1])
	add(weights.Pos, math.Max(0, 1-2*dist))
	return math.Round(sum/total*1000) / 1000, true
}

// Ranked é um candidato pontuado (Index é a posição na lista de candidatos).
type Ranked struct {
	Index int
	Score float64
}

// Rank pontua os candidatos compatíveis, do melhor para o pior (empate: ordem do documento).
func Rank(rec Signature, cands []Signature) []Ranked {
	var out []Ranked
	for i, c := range cands {
		if s, ok := Score(rec, c); ok {
			out = append(out, Ranked{Index: i, Score: s})
		}
	}
	slices.SortStableFunc(out, func(a, b Ranked) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		}
		return 0
	})
	return out
}

// Decision é o resultado da autocura para um passo.
type Decision struct {
	Accept bool
	Best   Ranked
	Margin float64
	Top    []Ranked // os 3 melhores, para o relatório de falha
	Reason string   // por que não aceitou (em inglês, para a IA)
}

// Decide aceita o melhor candidato só com nota ≥ MinScore e margem ≥ MinMargin sobre o segundo.
func Decide(ranked []Ranked) Decision {
	d := Decision{Top: ranked[:min(3, len(ranked))]}
	if len(ranked) == 0 {
		d.Reason = "no compatible element on the page"
		return d
	}
	d.Best = ranked[0]
	second := 0.0
	if len(ranked) > 1 {
		second = ranked[1].Score
	}
	d.Margin = math.Round((d.Best.Score-second)*1000) / 1000
	switch {
	case d.Best.Score < MinScore:
		d.Reason = "best match is below the confidence threshold"
	case d.Margin < MinMargin:
		d.Reason = "two elements match almost equally (ambiguous)"
	default:
		d.Accept = true
	}
	return d
}

// generatedID reconhece ids gerados por frameworks ou com cara de sequência: não servem de identidade.
var generatedID = regexp.MustCompile(`(?i)(\d{4,}|[0-9a-f]{8,}|^:r[0-9a-z]+:$|^(ember|react-|mui-|headlessui-|radix-|ng-|yui_|ext-gen|j_id|__|css-|sc-|svelte-|v-|uid-|id-))`)

// StableID diz se o id parece escrito por uma pessoa (e pode identificar o elemento).
func StableID(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && len(id) <= 64 && !generatedID.MatchString(id)
}
