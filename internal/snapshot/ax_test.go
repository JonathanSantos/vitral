package snapshot

import (
	"strconv"
	"strings"
	"testing"
)

// axFixture monta entradas sintéticas: cada elemento tem um nó AX e um nó DOM com o mesmo backend.
type axFixture struct {
	in   AXInput
	next int
}

func newAXFixture() *axFixture {
	f := &axFixture{in: AXInput{AX: map[string]*AXNode{}, DOM: map[int]*DOMNode{}, Viewport: [2]float64{1000, 800}, Skip: map[int]bool{}}}
	refs := map[int]string{}
	f.in.Ref = func(b int) string {
		if r, ok := refs[b]; ok {
			return r
		}
		r := "e" + strconv.Itoa(len(refs)+1)
		refs[b] = r
		return r
	}
	root := f.add(0, "RootWebArea", "Página", "#document", nil)
	f.in.Root = root.ID
	return f
}

// add cria um nó visível com caixa padrão; parent é o backend do pai (0 = sem pai).
func (f *axFixture) add(parent int, role, name, tag string, attrs map[string]string) *AXNode {
	f.next++
	b := f.next
	id := "ax" + strconv.Itoa(b)
	if attrs == nil {
		attrs = map[string]string{}
	}
	n := &AXNode{ID: id, Role: role, Name: name, Backend: b, Props: map[string]string{}}
	d := &DOMNode{
		Backend: b, Parent: parent, Tag: tag, Attrs: attrs, HasLayout: true, Display: "block", Visibility: "visible",
		Opacity: 1, Box: [4]float64{10, float64(10 * b), 100, 20},
	}
	f.in.AX[id] = n
	f.in.DOM[b] = d
	if p := f.in.DOM[parent]; p != nil {
		p.Children = append(p.Children, b)
		f.in.AX["ax"+strconv.Itoa(parent)].Children = append(f.in.AX["ax"+strconv.Itoa(parent)].Children, id)
	}
	return n
}

func (f *axFixture) format(full bool) string {
	return Format(&Page{URL: "u", Nodes: BuildAX(f.in, full, 0), Viewport: [2]int{1000, 800}}, DefaultFormat())
}

func TestBuildAXMatchesDOMEngineRules(t *testing.T) {
	f := newAXFixture()
	main := f.add(1, "main", "", "MAIN", nil)
	form := f.add(main.Backend, "form", "Cadastro", "FORM", map[string]string{"aria-label": "Cadastro"})
	name := f.add(form.Backend, "textbox", "Nome", "INPUT", nil)
	v := "Ana"
	name.Value = &v
	name.Props["required"] = "true"
	pw := f.add(form.Backend, "textbox", "Senha", "INPUT", map[string]string{"type": "password"})
	secret := "x"
	pw.Value = &secret
	sel := f.add(form.Backend, "combobox", "País", "SELECT", nil)
	o1 := f.add(sel.Backend, "MenuListOption", "Brasil", "OPTION", nil)
	f.in.DOM[o1.Backend].OptionSelected = true
	f.add(o1.Backend, "StaticText", "Brasil", "#text", nil).Ignored = true
	f.in.DOM[o1.Backend+1].Text = "Brasil"
	o2 := f.add(sel.Backend, "MenuListOption", "Angola", "OPTION", map[string]string{"disabled": ""})
	f.add(o2.Backend, "StaticText", "Angola", "#text", nil)
	f.in.DOM[o2.Backend+1].Text = "Angola"
	cb := f.add(form.Backend, "checkbox", "Aceito", "INPUT", map[string]string{"type": "checkbox"})
	cb.Props["checked"] = "true"
	btn := f.add(form.Backend, "button", "Enviar", "BUTTON", nil)
	btn.Props["disabled"] = "true"
	list := f.add(main.Backend, "list", "", "UL", nil)
	li := f.add(list.Backend, "listitem", "", "LI", nil)
	f.add(li.Backend, "StaticText", "Primeiro item", "#text", nil)
	h := f.add(main.Backend, "heading", "Título", "H2", nil)
	h.Props["level"] = "2"
	link := f.add(main.Backend, "link", "Ajuda", "A", map[string]string{"href": "#", "aria-current": "page"})
	_ = link
	date := f.add(main.Backend, "textbox", "Data", "INPUT", map[string]string{"type": "date"})
	_ = date

	out := f.format(false)
	for _, want := range []string{
		`[e1] main`,
		`[e2] form "Cadastro"`,
		`[e3] textbox "Nome" value="Ana" required`,
		`[e4] textbox "Senha" type=password value="••••"`,
		`[e5] combobox "País" value="Brasil" options=2`,
		`[e6] option "Brasil" selected`,
		`[e7] option "Angola" disabled`,
		`[e8] checkbox "Aceito" checked`,
		`[e9] button "Enviar" disabled`,
		`[e10] list items=1`,
		`[e11] listitem "Primeiro item"`,
		`[e12] heading "Título" level=2`,
		`[e13] link "Ajuda" current`,
		`[e14] textbox "Data" type=date value=""`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestBuildAXGenericsAndVisibility(t *testing.T) {
	f := newAXFixture()
	body := f.add(1, "generic", "", "BODY", nil)
	f.in.DOM[body.Backend].Clickable = true // listener delegado no body: nunca é "clicável"
	card := f.add(body.Backend, "generic", "", "DIV", nil)
	f.in.DOM[card.Backend].Clickable = true
	f.add(card.Backend, "StaticText", "Plano Pro", "#text", nil)
	inner := f.add(card.Backend, "generic", "", "SPAN", nil)
	f.in.DOM[inner.Backend].Cursor = "pointer" // o pai já é clicável: não duplica
	f.add(inner.Backend, "StaticText", "detalhe", "#text", nil)
	errBox := f.add(body.Backend, "generic", "", "DIV", map[string]string{"class": "field-error"})
	f.add(errBox.Backend, "StaticText", "Campo obrigatório", "#text", nil)
	live := f.add(body.Backend, "generic", "", "DIV", map[string]string{"aria-live": "polite"})
	f.add(live.Backend, "StaticText", "Salvo", "#text", nil)
	onclick := f.add(body.Backend, "generic", "", "DIV", map[string]string{"onclick": "x()"})
	f.add(onclick.Backend, "StaticText", "Abrir", "#text", nil)
	tab := f.add(body.Backend, "generic", "", "DIV", map[string]string{"tabindex": "0"})
	f.add(tab.Backend, "StaticText", "Foco", "#text", nil)
	hidden := f.add(body.Backend, "button", "Escondido", "BUTTON", nil)
	f.in.DOM[hidden.Backend].Visibility = "hidden"
	faded := f.add(body.Backend, "generic", "", "DIV", nil)
	f.in.DOM[faded.Backend].Opacity = 0
	f.add(faded.Backend, "button", "Transparente", "BUTTON", nil)
	far := f.add(body.Backend, "button", "Longe", "BUTTON", nil)
	f.in.DOM[far.Backend].Box = [4]float64{10, 5000, 100, 20}
	gone := f.add(body.Backend, "button", "Sumido", "BUTTON", nil)
	f.in.DOM[gone.Backend].Display = "none"
	ov := f.add(body.Backend, "button", "Pausar IA", "BUTTON", nil)
	f.in.Skip[ov.Backend] = true
	ign := f.add(body.Backend, "button", "Ignorado", "BUTTON", nil)
	ign.Ignored = true
	label := f.add(body.Backend, "LabelText", "Rótulo", "LABEL", nil)
	_ = label

	out := f.format(false)
	for _, want := range []string{`clickable "Plano Pro detalhe"`, `error "Campo obrigatório"`, `status "Salvo"`, `clickable "Abrir"`, `clickable "Foco"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, not := range []string{"Escondido", "Transparente", "Longe", "Sumido", "Pausar IA", "Ignorado", "Rótulo", `clickable "detalhe"`, `"Página"`} {
		if strings.Contains(out, not) {
			t.Errorf("%q should not be in:\n%s", not, out)
		}
	}
	if !strings.Contains(f.format(true), `button "Longe"`) {
		t.Error("full snapshot includes elements far from the viewport")
	}
}

func TestBuildAXNamesStatesAndCounts(t *testing.T) {
	f := newAXFixture()
	dlg := f.add(1, "alertdialog", "", "DIV", map[string]string{"aria-modal": "true"})
	f.add(dlg.Backend, "heading", "Confirmar", "H2", nil)
	nav := f.add(1, "navigation", "Menu", "NAV", nil) // nome sem aria-label: o snapshot antigo não dava
	_ = nav
	named := f.add(1, "navigation", "Principal", "NAV", map[string]string{"aria-label": "Principal"})
	_ = named
	table := f.add(1, "LayoutTable", "", "TABLE", nil)
	tbody := f.add(table.Backend, "generic", "", "TBODY", nil)
	for i := 0; i < 2; i++ {
		row := f.add(tbody.Backend, "LayoutTableRow", "", "TR", nil)
		f.add(row.Backend, "LayoutTableCell", "#"+strconv.Itoa(i), "TD", nil)
		f.add(row.Backend, "LayoutTableCell", "Ana", "TD", nil)
	}
	// Tabela de layout: o Chrome a nomeia com o texto das células; só title, caption ou aria dão nome.
	f.add(1, "LayoutTable", "Produto Preço Caneta", "TABLE", nil)
	f.add(1, "LayoutTable", "Preços", "TABLE", map[string]string{"title": "Preços"})
	capt := f.add(1, "LayoutTable", "Estoque", "TABLE", nil)
	f.add(capt.Backend, "caption", "Estoque", "CAPTION", nil)
	grid := f.add(1, "grid", "", "DIV", map[string]string{"aria-rowcount": "40"})
	_ = grid
	tree := f.add(1, "tree", "", "DIV", nil)
	f.add(tree.Backend, "treeitem", "A", "DIV", nil)
	f.add(tree.Backend, "treeitem", "B", "DIV", nil)
	exp := f.add(1, "button", "Mais", "BUTTON", map[string]string{"aria-expanded": "false"})
	exp.Props["expanded"] = "false"
	exp.Props["pressed"] = "true"
	exp.Props["focused"] = "true"
	details := f.add(1, "group", "", "DETAILS", map[string]string{"open": ""})
	sum := f.add(details.Backend, "button", "Resumo", "SUMMARY", nil)
	_ = sum
	sw := f.add(1, "switch", "Modo", "BUTTON", nil)
	sw.Props["checked"] = "mixed"
	ro := f.add(1, "textbox", "Código", "INPUT", map[string]string{"type": "weird"})
	ro.Props["readonly"] = "true"
	ro.Props["invalid"] = "spelling"
	opt := f.add(1, "option", "Item", "DIV", nil)
	opt.Props["selected"] = "true"
	h := f.add(1, "heading", "Sem nível", "DIV", nil)
	_ = h

	out := f.format(true)
	for _, want := range []string{
		`dialog "Confirmar" open modal`, `[e3] navigation`, `navigation "Principal"`,
		`table rows=2`, `row "#0 | Ana"`, `grid rows=40`, `tree items=2`,
		`button "Mais" collapsed pressed focused`, `button "Resumo" expanded`, `switch "Modo" checked=mixed`,
		`textbox "Código" value="" readonly invalid`, `option "Item" selected`, `heading "Sem nível" level=2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `navigation "Menu"`) {
		t.Error("container names only come from aria-label/labelledby")
	}
	if strings.Contains(out, `table "Produto`) || !strings.Contains(out, `table "Preços"`) || !strings.Contains(out, `table "Estoque"`) {
		t.Errorf("layout table names:\n%s", out)
	}
}

func TestBuildAXScopeAndCut(t *testing.T) {
	f := newAXFixture()
	sec := f.add(1, "region", "Seção", "SECTION", map[string]string{"aria-label": "Seção"})
	long := f.add(sec.Backend, "button", strings.Repeat("a", 400), "BUTTON", nil)
	f.in.DOM[long.Backend].Box = [4]float64{0, 9000, 10, 10} // longe: o scope traz tudo
	f.add(1, "button", "Fora", "BUTTON", nil)
	nodes := BuildAX(f.in, false, sec.Backend)
	if len(nodes) != 1 || nodes[0].Role != "region" || len(nodes[0].Children) != 1 {
		t.Fatalf("scope should return the subtree, full: %+v", nodes)
	}
	if name := nodes[0].Children[0].Name; len([]rune(name)) != 300 || !strings.HasSuffix(name, "…") {
		t.Fatalf("names are cut at 300 with …: %d", len([]rune(name)))
	}
	if BuildAX(f.in, false, 999) != nil {
		t.Fatal("unknown scope returns nil")
	}
}

func TestInsertState(t *testing.T) {
	got := InsertState([]string{"disabled", "focused"}, "invalid")
	if strings.Join(got, ",") != "disabled,invalid,focused" {
		t.Fatalf("got %v", got)
	}
	if strings.Join(InsertState(nil, "custom"), ",") != "custom" {
		t.Fatal("unknown state goes last")
	}
}
