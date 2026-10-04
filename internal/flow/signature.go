package flow

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/JonathanSantos/vitral/internal/heal"
	"github.com/JonathanSantos/vitral/internal/snapshot"
)

// landmarkRoles são as regiões da página que entram no contexto da assinatura.
var landmarkRoles = map[string]bool{
	"navigation": true, "main": true, "banner": true, "contentinfo": true, "complementary": true,
	"form": true, "region": true, "search": true,
}

// signatures calcula a assinatura de cada elemento do snapshot completo, na ordem do documento: o
// contexto (região, dialog, título mais próximo acima), a posição entre iguais e a posição na página.
func signatures(page *snapshot.Page, all []located) []heal.Signature {
	out := make([]heal.Signature, len(all))
	same := map[string]int{}
	for _, l := range all {
		same[l.node.Role+"\x00"+heal.Normalize(l.node.Name)]++
	}
	ordinal := map[string]int{}
	heading := ""
	vw, vh := float64(max(page.Viewport[0], 1)), float64(max(page.Viewport[1], 1))
	top, height := float64(page.Scroll[0]), float64(page.Scroll[1])+vh
	for i, l := range all {
		n := l.node
		s := heal.Signature{Role: n.Role, Name: n.Name, Heading: heading}
		for _, a := range l.ancestors {
			switch {
			case a.Role == "dialog":
				s.Dialog = a.Name
			case landmarkRoles[a.Role]:
				s.Landmark = strings.TrimSpace(a.Role + " " + a.Name)
			}
		}
		key := n.Role + "\x00" + heal.Normalize(n.Name)
		s.Same = same[key]
		ctxKey := key + "\x00" + s.Landmark + "\x00" + s.Dialog
		ordinal[ctxKey]++
		s.Ordinal = ordinal[ctxKey]
		b := n.Box
		if b[2] > 0 && b[3] > 0 {
			s.Pos = [2]float64{round2((float64(b[0]) + float64(b[2])/2) / vw), round2((float64(b[1]) + top + float64(b[3])/2) / height)}
		}
		if a := n.Attrs; a != nil {
			s.Tag, s.TestID, s.NameAttr, s.Href = a.Tag, a.TestID, a.NameAttr, a.Href
			s.Label, s.Placeholder, s.InputType, s.Column, s.RowHeader = a.Label, a.Placeholder, a.InputType, a.Column, a.RowHeader
			if heal.StableID(a.ID) {
				s.ID = a.ID
			}
		}
		out[i] = s
		if n.Role == "heading" {
			heading = n.Name
		}
	}
	return out
}

func round2(v float64) float64 { return math.Round(math.Max(0, math.Min(1, v))*100) / 100 }

// Healing é uma autocura: o seletor gravado não achou o elemento (ou achou mais de um) e a assinatura
// escolheu outro com confiança suficiente.
type Healing struct {
	Ref        string
	Was        Selector
	Now        Selector // seletor e assinatura novos (para o flow_update)
	Confidence float64
}

// Describe da autocura: was button "Confirmar", now [e14] button "Confirmar pedido" (confidence 0.82).
func (h Healing) Describe() string {
	return fmt.Sprintf("was %s, now [%s] %s (confidence %.2f)", h.Was.Describe(), h.Ref, h.Now.Describe(), h.Confidence)
}

// heal pontua os elementos da página contra a assinatura gravada e decide.
func healSelector(page *snapshot.Page, all []located, sel Selector) (*Healing, error) {
	sigs := signatures(page, all)
	d := heal.Decide(heal.Rank(*sel.Sig, sigs))
	if !d.Accept {
		return nil, fmt.Errorf("no %s found and healing did not pick a replacement: %s. %s", sel.Describe(), d.Reason, closest(all, d))
	}
	cand := all[d.Best.Index]
	now, err := Describe(page, cand.node.Ref)
	if err != nil {
		return nil, err
	}
	return &Healing{Ref: cand.node.Ref, Was: sel, Now: now, Confidence: d.Best.Score}, nil
}

// closest lista os 3 melhores candidatos com a nota, para a IA ou a pessoa decidirem.
func closest(all []located, d heal.Decision) string {
	if len(d.Top) == 0 {
		return "There is no element of a compatible role on the page."
	}
	parts := make([]string, len(d.Top))
	for i, r := range d.Top {
		n := all[r.Index].node
		parts[i] = fmt.Sprintf("[%s] %s %s (%.2f)", n.Ref, n.Role, strconv.Quote(snapshot.Truncate(n.Name, 60)), r.Score)
	}
	return fmt.Sprintf("Closest matches: %s. Healing needs confidence ≥ %.2f and a margin of %.2f over the next one.", strings.Join(parts, ", "), heal.MinScore, heal.MinMargin)
}
