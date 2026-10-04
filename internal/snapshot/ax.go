package snapshot

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Motor "ax" do snapshot (protótipo, pedido "próximos passos", seção 2): monta os mesmos nós do
// motor "dom" a partir da árvore de acessibilidade do Chrome (papel, nome, valor e estados calculados
// por ele) unida ao DOMSnapshot (caixa, estilos, isClickable) por backendNodeId. Puro: a coleta pelo
// CDP fica na sessão.

// AXNode é um nó de Accessibility.getFullAXTree, já com os valores em texto.
type AXNode struct {
	ID       string
	Children []string
	Ignored  bool
	Role     string
	Name     string
	Value    *string
	Props    map[string]string // ex.: checked=true, expanded=false, level=2
	Backend  int
}

// DOMNode é um nó do DOMSnapshot.captureSnapshot.
type DOMNode struct {
	Backend        int
	Parent         int // backend do pai (0 = raiz)
	Children       []int
	Tag            string // nodeName (DIV, INPUT, #text)
	Attrs          map[string]string
	Text           string // nodeValue de #text
	Box            [4]float64
	HasLayout      bool
	Display        string
	Visibility     string
	Opacity        float64
	Cursor         string
	Clickable      bool
	OptionSelected bool
}

// AXInput é tudo que o construtor precisa.
type AXInput struct {
	AX       map[string]*AXNode
	Root     string
	DOM      map[int]*DOMNode
	Viewport [2]float64
	Skip     map[int]bool             // backends fora do snapshot (o overlay do Vitral e a subárvore dele)
	Ref      func(backend int) string // devolve (ou cria) a ref do elemento
}

var axRoleMap = map[string]string{
	"alertdialog": "dialog", "LabelText": "label", "MenuListOption": "option",
	// O Chrome chama de "tabela de layout" uma <table> sem cabeçalhos; o motor dom trata toda <table>
	// como tabela.
	"LayoutTable": "table", "LayoutTableRow": "row", "LayoutTableCell": "cell",
}

var axInteractive = map[string]bool{
	"button": true, "link": true, "checkbox": true, "radio": true, "switch": true, "tab": true, "menuitem": true,
	"menuitemcheckbox": true, "menuitemradio": true, "option": true, "combobox": true, "textbox": true,
	"searchbox": true, "slider": true, "spinbutton": true, "treeitem": true,
}

var axStructure = map[string]bool{
	"dialog": true, "alert": true, "status": true, "heading": true, "table": true, "grid": true, "row": true,
	"list": true, "listbox": true, "listitem": true, "menu": true, "menubar": true, "tablist": true, "tabpanel": true,
	"tree": true, "navigation": true, "main": true, "banner": true, "contentinfo": true, "complementary": true,
	"form": true, "region": true, "search": true, "group": true, "label": true, "error": true,
}

// Papéis de contêiner: o nome só vale se vier de aria-label/aria-labelledby (como no motor dom).
var axUnnamedContainers = map[string]bool{
	"list": true, "navigation": true, "main": true, "banner": true, "contentinfo": true, "complementary": true,
	"form": true, "region": true, "search": true, "menu": true, "menubar": true, "tablist": true, "tree": true,
	"listbox": true, "tabpanel": true,
}

var axCounted = map[string]string{"table": "row", "grid": "row", "list": "listitem", "listbox": "option", "menu": "menuitem", "tree": "treeitem", "combobox": "option"}

var axErrorClass = regexp.MustCompile(`(?i)(^|[\s_-])(error|errors|invalid|danger)([\s_-]|$)`)

var axSpaces = regexp.MustCompile(`\s+`)

func axNorm(s string) string { return strings.TrimSpace(axSpaces.ReplaceAllString(s, " ")) }

// axCut é o mesmo corte do motor dom: 300 caracteres, com "…" quando corta.
func axCut(s string) string {
	r := []rune(s)
	if len(r) <= 300 {
		return s
	}
	return string(r[:299]) + "…"
}

var htmlInputTypes = map[string]bool{
	"text": true, "search": true, "email": true, "url": true, "tel": true, "password": true, "number": true,
	"date": true, "datetime-local": true, "month": true, "week": true, "time": true, "color": true, "range": true,
	"file": true, "checkbox": true, "radio": true, "submit": true, "reset": true, "button": true, "image": true, "hidden": true,
}

type axBuilder struct {
	in   AXInput
	full bool
}

// BuildAX monta os nós a partir da árvore de acessibilidade. scope (backend) restringe à subárvore
// desse elemento, inteira (como o scope do motor dom).
func BuildAX(in AXInput, full bool, scope int) []*Node {
	b := &axBuilder{in: in, full: full}
	root := in.Root
	if scope != 0 {
		root = ""
		for id, n := range in.AX {
			if n.Backend == scope {
				root = id
				break
			}
		}
		if root == "" {
			return nil
		}
		b.full = true
	}
	var out []*Node
	b.visit(root, &out)
	return out
}

func (b *axBuilder) dom(n *AXNode) *DOMNode {
	if n == nil || n.Backend == 0 {
		return nil
	}
	return b.in.DOM[n.Backend]
}

// visible repete o checkVisibility do motor dom: com layout, display/visibility e opacidade da cadeia.
func (b *axBuilder) visible(d *DOMNode) bool {
	if d == nil || !d.HasLayout || d.Display == "none" || d.Visibility == "hidden" || d.Visibility == "collapse" {
		return false
	}
	for n := d; n != nil; n = b.in.DOM[n.Parent] {
		if n.Opacity == 0 && n.HasLayout {
			return false
		}
		if n.Parent == 0 {
			break
		}
	}
	return true
}

func (b *axBuilder) near(box [4]float64) bool {
	vw, vh := b.in.Viewport[0], b.in.Viewport[1]
	mx, my := vw*0.25, vh*0.25
	return box[0]+box[2] >= -mx && box[0] <= vw+mx && box[1]+box[3] >= -my && box[1] <= vh+my
}

func (b *axBuilder) visit(id string, out *[]*Node) {
	n := b.in.AX[id]
	if n == nil {
		return
	}
	d := b.dom(n)
	if d != nil && b.in.Skip[d.Backend] {
		return
	}
	if d != nil && d.Display == "none" {
		return
	}
	target := out
	if role := b.role(n, d); role != "" && b.visible(d) && b.include(n, d, role) {
		box := d.Box
		if box[2] > 0 && box[3] > 0 && (b.full || b.near(box)) {
			node := b.node(n, d, role)
			*out = append(*out, node)
			if d.Tag == "SELECT" {
				node.Children = b.options(d)
				return
			}
			children := []*Node{}
			target = &children
			defer func() {
				if len(children) > 0 {
					node.Children = children
				}
			}()
		}
	}
	for _, c := range n.Children {
		b.visit(c, target)
	}
}

func (b *axBuilder) role(n *AXNode, d *DOMNode) string {
	if n.Ignored || d == nil {
		return ""
	}
	// O documento, <html> e <body> recebem os listeners delegados da página: nunca são "clicáveis".
	if n.Role == "RootWebArea" || n.Role == "WebArea" || isPageRoot(d.Tag) {
		return ""
	}
	role := n.Role
	if m, ok := axRoleMap[role]; ok {
		role = m
	}
	if axInteractive[role] || axStructure[role] {
		return role
	}
	// Papéis que o Chrome não dá a um genérico, mas o motor dom dava.
	if cls := d.Attrs["class"]; cls != "" && axErrorClass.MatchString(cls) && !b.hasControl(d) {
		if t := b.text(n); t != "" && len([]rune(t)) < 300 {
			return "error"
		}
	}
	if live := d.Attrs["aria-live"]; (live == "polite" || live == "assertive") && b.text(n) != "" {
		return "status"
	}
	if _, ok := d.Attrs["onclick"]; ok {
		return "clickable"
	}
	if ti, ok := d.Attrs["tabindex"]; ok {
		if v, err := strconv.Atoi(strings.TrimSpace(ti)); err == nil && v >= 0 {
			return "clickable"
		}
	}
	// Genérico clicável: isClickable (listener de clique) ou cursor pointer, com o pai que não é, e com
	// texto ou imagem dentro.
	if d.Clickable || d.Cursor == "pointer" {
		// O pai já é clicável: o filho não se repete. <body>, <html> e o documento não contam (listeners
		// delegados ali marcariam a página inteira como clicável).
		if p := b.in.DOM[d.Parent]; p != nil && (p.Cursor == "pointer" || p.Clickable) && !isPageRoot(p.Tag) {
			return ""
		}
		if b.text(n) != "" || b.hasTag(d, "IMG", "svg", "SVG") {
			return "clickable"
		}
	}
	return ""
}

func (b *axBuilder) include(n *AXNode, d *DOMNode, role string) bool {
	switch role {
	case "label":
		return false // o rótulo já dá nome ao controle
	case "group":
		_, l1 := d.Attrs["aria-label"]
		_, l2 := d.Attrs["aria-labelledby"]
		return d.Tag == "FIELDSET" || l1 || l2
	}
	return true
}

func (b *axBuilder) hasControl(d *DOMNode) bool {
	return b.hasTag(d, "INPUT", "SELECT", "TEXTAREA", "BUTTON") || b.hasLink(d)
}

func (b *axBuilder) hasLink(d *DOMNode) bool {
	for _, c := range d.Children {
		cd := b.in.DOM[c]
		if cd == nil {
			continue
		}
		if _, ok := cd.Attrs["href"]; ok && cd.Tag == "A" {
			return true
		}
		if b.hasLink(cd) {
			return true
		}
	}
	return false
}

func (b *axBuilder) hasTag(d *DOMNode, tags ...string) bool {
	for _, c := range d.Children {
		cd := b.in.DOM[c]
		if cd == nil {
			continue
		}
		for _, t := range tags {
			if cd.Tag == t {
				return true
			}
		}
		if b.hasTag(cd, tags...) {
			return true
		}
	}
	return false
}

// text junta os textos estáticos da subárvore (o nome de um genérico, que o Chrome não calcula).
func (b *axBuilder) text(n *AXNode) string {
	var parts []string
	var walk func(id string)
	walk = func(id string) {
		c := b.in.AX[id]
		if c == nil {
			return
		}
		if c.Role == "StaticText" && !c.Ignored {
			parts = append(parts, c.Name)
			return
		}
		for _, k := range c.Children {
			walk(k)
		}
	}
	for _, k := range n.Children {
		walk(k)
	}
	return axNorm(strings.Join(parts, " "))
}

func (b *axBuilder) name(n *AXNode, d *DOMNode, role string) string {
	_, l1 := d.Attrs["aria-label"]
	_, l2 := d.Attrs["aria-labelledby"]
	switch {
	case role == "row":
		var cells []string
		for _, c := range n.Children {
			if cn := b.in.AX[c]; cn != nil && (cn.Role == "cell" || cn.Role == "gridcell" || cn.Role == "columnheader" || cn.Role == "rowheader" || cn.Role == "LayoutTableCell") {
				cells = append(cells, axNorm(cn.Name))
			}
		}
		if len(cells) > 0 && !l1 && !l2 {
			return axCut(strings.Join(cells, " | "))
		}
	case role == "dialog" && axNorm(n.Name) == "":
		return axCut(b.firstHeading(n))
	case axUnnamedContainers[role] && !l1 && !l2:
		return ""
	}
	// O Chrome não calcula nome pelo conteúdo para alguns papéis (listitem, genéricos clicáveis): o
	// motor dom usava o texto.
	if name := axNorm(n.Name); name != "" {
		return axCut(name)
	}
	return axCut(b.text(n))
}

func (b *axBuilder) firstHeading(n *AXNode) string {
	for _, c := range n.Children {
		cn := b.in.AX[c]
		if cn == nil {
			continue
		}
		if cn.Role == "heading" {
			return axNorm(cn.Name)
		}
		if h := b.firstHeading(cn); h != "" {
			return h
		}
	}
	return ""
}

func (b *axBuilder) node(n *AXNode, d *DOMNode, role string) *Node {
	node := &Node{Ref: b.in.Ref(d.Backend), Role: role, Name: b.name(n, d, role)}
	if v, ok := b.value(n, d, role); ok {
		v = axCut(v)
		node.Value = &v
	}
	node.States = b.states(n, d, role)
	if role == "heading" {
		node.Level, _ = strconv.Atoi(n.Props["level"])
		if node.Level == 0 {
			node.Level = 2
		}
	}
	if d.Tag == "INPUT" {
		t := strings.ToLower(strings.TrimSpace(d.Attrs["type"]))
		if !htmlInputTypes[t] {
			t = "text"
		}
		switch t {
		case "text", "checkbox", "radio", "button", "submit", "reset":
		default:
			node.Type = t
		}
	}
	if c, ok := b.count(n, d, role); ok {
		node.Count = &c
	}
	node.Box = [4]int{int(math.Round(d.Box[0])), int(math.Round(d.Box[1])), int(math.Round(d.Box[2])), int(math.Round(d.Box[3]))}
	return node
}

func (b *axBuilder) value(n *AXNode, d *DOMNode, role string) (string, bool) {
	switch role {
	case "textbox", "searchbox", "spinbutton", "slider", "combobox":
	default:
		return "", false
	}
	if d.Tag == "SELECT" {
		var sel []string
		for _, o := range b.optionNodes(d) {
			if o.OptionSelected {
				sel = append(sel, b.optionLabel(o))
			}
		}
		return strings.Join(sel, ", "), true
	}
	if d.Tag == "INPUT" && strings.EqualFold(d.Attrs["type"], "password") {
		if n.Value != nil && *n.Value != "" {
			return "••••", true
		}
		return "", true
	}
	if n.Value != nil {
		return *n.Value, true
	}
	if d.Tag == "INPUT" || d.Tag == "TEXTAREA" {
		return "", true
	}
	return "", false
}

func (b *axBuilder) states(n *AXNode, d *DOMNode, role string) []string {
	var s []string
	p := n.Props
	if role == "dialog" {
		s = append(s, "open")
		if p["modal"] == "true" || d.Attrs["aria-modal"] == "true" {
			s = append(s, "modal")
		}
	}
	if p["disabled"] == "true" {
		s = append(s, "disabled")
	}
	switch role {
	case "checkbox", "radio", "switch", "menuitemcheckbox", "menuitemradio":
		switch p["checked"] {
		case "mixed":
			s = append(s, "checked=mixed")
		case "true":
			s = append(s, "checked")
		}
	}
	// Expandido/recolhido só pelo atributo (ou <summary>), como no motor dom: o Chrome marca todo
	// select fechado como "não expandido", o que seria ruído.
	switch {
	case d.Tag == "SUMMARY":
		if _, open := b.in.DOM[d.Parent].Attrs["open"]; open {
			s = append(s, "expanded")
		} else {
			s = append(s, "collapsed")
		}
	case d.Attrs["aria-expanded"] == "true":
		s = append(s, "expanded")
	case d.Attrs["aria-expanded"] == "false":
		s = append(s, "collapsed")
	}
	if p["selected"] == "true" {
		s = append(s, "selected")
	}
	if p["pressed"] == "true" {
		s = append(s, "pressed")
	}
	if c := d.Attrs["aria-current"]; c != "" && c != "false" {
		s = append(s, "current")
	}
	if p["required"] == "true" {
		s = append(s, "required")
	}
	if p["readonly"] == "true" && (role == "textbox" || role == "searchbox" || role == "spinbutton") {
		s = append(s, "readonly")
	}
	if inv := p["invalid"]; inv != "" && inv != "false" {
		s = append(s, "invalid")
	}
	if p["focused"] == "true" {
		s = append(s, "focused")
	}
	return s
}

func (b *axBuilder) count(n *AXNode, d *DOMNode, role string) (int, bool) {
	child, ok := axCounted[role]
	if !ok {
		return 0, false
	}
	switch d.Tag {
	case "SELECT":
		return len(b.optionNodes(d)), true
	case "TABLE":
		rows, body := 0, 0
		for _, c := range d.Children {
			cd := b.in.DOM[c]
			if cd == nil {
				continue
			}
			if cd.Tag == "TBODY" {
				body += b.countTag(cd, "TR")
			}
			rows += b.countTag(cd, "TR")
		}
		if body > 0 {
			return body, true
		}
		return rows, true
	case "UL", "OL":
		k := 0
		for _, c := range d.Children {
			if cd := b.in.DOM[c]; cd != nil && cd.Tag == "LI" {
				k++
			}
		}
		return k, true
	}
	attr := "aria-setsize"
	if role == "table" || role == "grid" {
		attr = "aria-rowcount"
	}
	if v, err := strconv.Atoi(d.Attrs[attr]); err == nil && v > 0 {
		return v, true
	}
	return b.countRole(n, child), true
}

func (b *axBuilder) countTag(d *DOMNode, tag string) int {
	k := 0
	for _, c := range d.Children {
		if cd := b.in.DOM[c]; cd != nil && cd.Tag == tag {
			k++
		}
	}
	return k
}

func (b *axBuilder) countRole(n *AXNode, role string) int {
	k := 0
	for _, c := range n.Children {
		cn := b.in.AX[c]
		if cn == nil {
			continue
		}
		if cn.Role == role || axRoleMap[cn.Role] == role {
			k++
		}
		k += b.countRole(cn, role)
	}
	return k
}

// optionNodes são os <option> do <select> (inclusive dentro de <optgroup>), na ordem do documento.
func (b *axBuilder) optionNodes(d *DOMNode) []*DOMNode {
	var out []*DOMNode
	for _, c := range d.Children {
		cd := b.in.DOM[c]
		if cd == nil {
			continue
		}
		switch cd.Tag {
		case "OPTION":
			out = append(out, cd)
		case "OPTGROUP":
			out = append(out, b.optionNodes(cd)...)
		}
	}
	return out
}

func (b *axBuilder) optionLabel(o *DOMNode) string {
	if l, ok := o.Attrs["label"]; ok {
		return axNorm(l)
	}
	var parts []string
	for _, c := range o.Children {
		if cd := b.in.DOM[c]; cd != nil && cd.Tag == "#text" {
			parts = append(parts, cd.Text)
		}
	}
	return axNorm(strings.Join(parts, " "))
}

// options repete o optionNode do motor dom: opções de select fechado entram sem caixa.
func (b *axBuilder) options(d *DOMNode) []*Node {
	var out []*Node
	for _, o := range b.optionNodes(d) {
		node := &Node{Ref: b.in.Ref(o.Backend), Role: "option", Name: axCut(b.optionLabel(o))}
		if o.OptionSelected {
			node.States = append(node.States, "selected")
		}
		if _, dis := o.Attrs["disabled"]; dis {
			node.States = append(node.States, "disabled")
		}
		out = append(out, node)
	}
	return out
}

// stateOrder é a ordem dos estados no motor dom (a saída precisa ser igual).
var stateOrder = []string{"open", "modal", "disabled", "checked", "checked=mixed", "expanded", "collapsed", "selected", "pressed", "current", "required", "readonly", "invalid", "focused"}

// InsertState põe o estado na posição da ordem do motor dom.
func InsertState(states []string, state string) []string {
	rank := func(s string) int {
		for i, o := range stateOrder {
			if o == s {
				return i
			}
		}
		return len(stateOrder)
	}
	out := append([]string(nil), states...)
	i := 0
	for i < len(out) && rank(out[i]) <= rank(state) {
		i++
	}
	return append(out[:i], append([]string{state}, out[i:]...)...)
}

func isPageRoot(tag string) bool { return tag == "#document" || tag == "HTML" || tag == "BODY" }
