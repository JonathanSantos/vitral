package sitemap

import (
	"fmt"
	"strings"
)

// Text descreve o mapa para pessoas: cada tela com o padrão de URL, a impressão, o catálogo e as
// saídas observadas.
func (m *Map) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d telas, %d transições\n", m.Host, len(m.Screens), len(m.Edges))
	for _, s := range m.Screens {
		fmt.Fprintf(&b, "\n[%s] %q  %s  (visitas: %d)\n", s.ID, s.Name, s.URL, s.Visits)
		items := make([]string, len(s.Print))
		for i, it := range s.Print {
			items[i] = strings.TrimSpace(fmt.Sprintf("%s %q", it.Role, it.Name))
		}
		fmt.Fprintf(&b, "  impressão: %s\n", strings.Join(items, ", "))
		if len(s.Dialogs) > 0 {
			fmt.Fprintf(&b, "  dialogs: %s\n", strings.Join(s.Dialogs, ", "))
		}
		for _, el := range s.Elements {
			fmt.Fprintf(&b, "  elemento: %s %q (usos: %d)\n", el.Sig.Role, el.Sig.Name, el.Uses)
		}
		for _, e := range m.Edges {
			if e.From != s.ID {
				continue
			}
			to := m.Screen(e.To)
			suspect := ""
			if e.Suspect() {
				suspect = " [suspeita]"
			}
			fmt.Fprintf(&b, "  → %s ⇒ %q (%d ok, %d falhas)%s\n", e.Action.Label(), to.Name, e.Successes, e.Failures, suspect)
		}
	}
	return b.String()
}

// Mermaid desenha o grafo (telas e transições entre telas diferentes) em Mermaid.
func (m *Map) Mermaid() string {
	esc := func(s string) string { return strings.NewReplacer(`"`, "'", "<", "&lt;", ">", "&gt;").Replace(s) }
	var b strings.Builder
	b.WriteString("graph LR\n")
	for _, s := range m.Screens {
		fmt.Fprintf(&b, "  %s[\"%s<br/><small>%s</small>\"]\n", s.ID, esc(s.Name), esc(s.URL))
	}
	for _, e := range m.Edges {
		if e.From == e.To {
			continue
		}
		style := "-->"
		if e.Suspect() {
			style = "-.->"
		}
		label := e.Action.Kind
		if t := e.Action.Target; t != nil {
			label += " " + t.Role + " '" + t.Name + "'"
		} else if e.Action.Key != "" {
			label += " " + e.Action.Key
		}
		fmt.Fprintf(&b, "  %s %s|\"%s\"| %s\n", e.From, style, esc(label), e.To)
	}
	return b.String()
}
