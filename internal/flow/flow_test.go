package flow

import (
	"strconv"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/snapshot"
)

func pageWith(nodes ...*snapshot.Node) *snapshot.Page { return &snapshot.Page{Nodes: nodes} }

func samplePage() *snapshot.Page {
	return pageWith(
		&snapshot.Node{Ref: "e1", Role: "textbox", Name: "Buscar pedido"},
		&snapshot.Node{Ref: "e2", Role: "button", Name: "Filtrar"},
		&snapshot.Node{Ref: "e3", Role: "table", Name: "Pedidos", Children: []*snapshot.Node{
			{Ref: "e4", Role: "row", Name: "#1023 | Aprovado", Children: []*snapshot.Node{{Ref: "e5", Role: "link", Name: "Ver detalhes"}}},
			{Ref: "e6", Role: "row", Name: "#1024 | Pendente", Children: []*snapshot.Node{{Ref: "e7", Role: "link", Name: "Ver detalhes"}}},
		}},
		&snapshot.Node{Ref: "e8", Role: "button", Name: "Excluir"},
		&snapshot.Node{Ref: "e9", Role: "dialog", Name: "Confirmar", Children: []*snapshot.Node{
			{Ref: "e10", Role: "button", Name: "Excluir"},
		}},
		&snapshot.Node{Ref: "e11", Role: "button", Name: ""},
		&snapshot.Node{Ref: "e12", Role: "button", Name: ""},
	)
}

func TestDescribeAndResolveRoundTrip(t *testing.T) {
	page := samplePage()
	tests := []struct {
		ref  string
		want Selector
	}{
		{"e2", Selector{Role: "button", Name: "Filtrar"}},
		{"e7", Selector{Role: "link", Name: "Ver detalhes", Within: &Context{Role: "row", Name: "#1024 | Pendente"}}},
		{"e10", Selector{Role: "button", Name: "Excluir", Within: &Context{Role: "dialog", Name: "Confirmar"}}},
		{"e8", Selector{Role: "button", Name: "Excluir"}},
		{"e12", Selector{Role: "button", Name: "", Nth: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got, err := Describe(page, tt.ref)
			if err != nil {
				t.Fatal(err)
			}
			if got.Describe() != tt.want.Describe() {
				t.Fatalf("Describe = %s, want %s", got.Describe(), tt.want.Describe())
			}
			ref, err := Resolve(page, got)
			if err != nil || ref != tt.ref {
				t.Fatalf("Resolve = %s, %v; want %s", ref, err, tt.ref)
			}
		})
	}
	if _, err := Describe(page, "e99"); err == nil {
		t.Fatal("unknown ref should fail")
	}
}

func TestResolveErrors(t *testing.T) {
	page := samplePage()
	_, err := Resolve(page, Selector{Role: "button", Name: "Buscar"})
	if err == nil || !strings.Contains(err.Error(), `no button "Buscar" found`) || !strings.Contains(err.Error(), `"Filtrar"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = Resolve(page, Selector{Role: "link", Name: "Ver detalhes", Nth: 3})
	if err == nil || !strings.Contains(err.Error(), "only 2") {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = Resolve(page, Selector{Role: "checkbox", Name: "x"})
	if err == nil || !strings.Contains(err.Error(), "There is no checkbox") {
		t.Fatalf("unexpected error: %v", err)
	}
	// Espaços e caixa diferentes ainda casam.
	if ref, err := Resolve(page, Selector{Role: "textbox", Name: "buscar  PEDIDO"}); err != nil || ref != "e1" {
		t.Fatalf("normalized match failed: %s %v", ref, err)
	}
}

func recordedSteps() []Step {
	return []Step{
		{Open: "https://portal/pedidos?busca=1023"},
		{Type: &TypeStep{Selector: Selector{Role: "textbox", Name: "Buscar pedido 1023"}, Text: "1023"}},
		{Click: &Selector{Role: "button", Name: "Filtrar"}},
		{Select: &SelectStep{Selector: Selector{Role: "combobox", Name: "Status"}, Option: "Aprovado"}},
		{WaitFor: &WaitStep{Text: "#1023 | Aprovado"}},
	}
}

func TestParameterize(t *testing.T) {
	steps := recordedSteps()
	out, subs, err := Parameterize(steps, map[string]string{"numero": "1023", "status": "Aprovado"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`step 1 open.url: "https://portal/pedidos?busca=1023" → "https://portal/pedidos?busca={{numero}}"`,
		`step 2 type.text: "1023" → "{{numero}}"`,
		`step 4 select.option: "Aprovado" → "{{status}}"`,
		`step 5 wait_for.text: "#1023 | Aprovado" → "#{{numero}} | {{status}}"`,
	}
	if len(subs) != len(want) {
		t.Fatalf("subs = %v", subs)
	}
	for i := range want {
		if subs[i].String() != want[i] {
			t.Errorf("sub %d = %s\nwant %s", i, subs[i], want[i])
		}
	}
	if out[1].Type.Name != "Buscar pedido 1023" {
		t.Fatal("selectors must never be parameterized")
	}
	if steps[1].Type.Text != "1023" {
		t.Fatal("input steps must not be modified")
	}
}

func TestParameterizeErrors(t *testing.T) {
	tests := []struct {
		params map[string]string
		want   string
	}{
		{map[string]string{"n": "12"}, "too short"},
		{map[string]string{"cliente": "Maria"}, `param "cliente" (value "Maria") was not found`},
		{map[string]string{"bad-name": "1023"}, "invalid param name"},
		{map[string]string{"botao": "Filtrar"}, "was not found"}, // só em seletor: não conta
	}
	for _, tt := range tests {
		_, _, err := Parameterize(recordedSteps(), tt.params)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Parameterize(%v) error = %v, want %q", tt.params, err, tt.want)
		}
	}
}

func TestBind(t *testing.T) {
	steps, _, _ := Parameterize(recordedSteps(), map[string]string{"numero": "1023"})
	f := Flow{Name: "busca", Params: []string{"numero"}, Steps: steps}

	bound, err := Bind(f, map[string]string{"numero": "2048"})
	if err != nil {
		t.Fatal(err)
	}
	if bound[1].Type.Text != "2048" || bound[4].WaitFor.Text != "#2048 | Aprovado" || bound[0].Open != "https://portal/pedidos?busca=2048" {
		t.Fatalf("bind failed: %+v %+v", bound[1].Type, bound[4].WaitFor)
	}
	if f.Steps[1].Type.Text != "{{numero}}" {
		t.Fatal("flow must not be modified")
	}
	if _, err := Bind(f, nil); err == nil || !strings.Contains(err.Error(), "missing params: numero") {
		t.Fatalf("missing param error: %v", err)
	}
	if _, err := Bind(f, map[string]string{"numero": "1", "x": "y"}); err == nil || !strings.Contains(err.Error(), `unknown param "x"`) {
		t.Fatalf("unknown param error: %v", err)
	}
	undeclared := Flow{Name: "x", Steps: []Step{{Type: &TypeStep{Text: "{{nome}}"}}}}
	if _, err := Bind(undeclared, nil); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("undeclared placeholder error: %v", err)
	}
}

func TestYAMLMatchesSpec(t *testing.T) {
	f := Flow{
		Name: "buscar-pedido", Description: "Abre o portal e busca um pedido pelo número", Params: []string{"numero"},
		Steps: []Step{
			{Open: "https://portal/pedidos"},
			{Type: &TypeStep{Selector: Selector{Role: "textbox", Name: "Buscar pedido"}, Text: "{{numero}}"}},
			{Click: &Selector{Role: "button", Name: "Filtrar"}},
			{WaitFor: &WaitStep{Text: "#{{numero}}"}},
		},
	}
	data, err := Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	want := `name: buscar-pedido
description: Abre o portal e busca um pedido pelo número
params:
    - numero
steps:
    - open: https://portal/pedidos
    - type:
        role: textbox
        name: Buscar pedido
        text: '{{numero}}'
    - click:
        role: button
        name: Filtrar
    - wait_for:
        text: '#{{numero}}'
`
	if string(data) != want {
		t.Fatalf("yaml:\n%s\nwant:\n%s", data, want)
	}
	back, err := Unmarshal(data)
	if err != nil || len(back.Steps) != 4 || back.Steps[1].Type.Name != "Buscar pedido" {
		t.Fatalf("roundtrip: %+v %v", back, err)
	}

	// O formato compacto do exemplo da spec também precisa ser aceito.
	spec := `name: buscar-pedido
params: [numero]
steps:
  - open: https://portal/pedidos
  - type: { role: textbox, name: "Buscar pedido", text: "{{numero}}" }
  - click: { role: button, name: "Filtrar" }
  - wait_for: { text: "#{{numero}}" }
`
	if _, err := Unmarshal([]byte(spec)); err != nil {
		t.Fatal(err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		f    Flow
		want string
	}{
		{Flow{Name: "Bad Name", Steps: []Step{{Back: true}}}, "invalid flow name"},
		{Flow{Name: "ok"}, "no steps"},
		{Flow{Name: "ok", Steps: []Step{{}}}, "step 1 is empty"},
		{Flow{Name: "ok", Steps: []Step{{Back: true, Press: "Enter"}}}, "more than one action"},
	}
	for _, tt := range tests {
		if err := Validate(tt.f); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Validate(%+v) = %v, want %q", tt.f, err, tt.want)
		}
	}
}

func TestStepDescribe(t *testing.T) {
	tests := map[string]Step{
		`click button "Ok" (2nd, in dialog "X")`: {Click: &Selector{Role: "button", Name: "Ok", Nth: 2, Within: &Context{Role: "dialog", Name: "X"}}},
		`type textbox "Nome" = "Ana"`:            {Type: &TypeStep{Selector: Selector{Role: "textbox", Name: "Nome"}, Text: "Ana"}},
		`scroll down`:                            {Scroll: &ScrollStep{Direction: "down"}},
		`wait_for text "Pronto" gone`:            {WaitFor: &WaitStep{Text: "Pronto", Gone: true}},
		`emulate mobile`:                         {Emulate: &EmulateStep{Device: "mobile"}},
		`press Enter`:                            {Press: "Enter"},
	}
	for want, step := range tests {
		if got := step.Describe(); got != want {
			t.Errorf("Describe = %q want %q", got, want)
		}
	}
}

func TestRecorderRange(t *testing.T) {
	var r Recorder
	if _, _, _, err := r.Range(0, 0); err == nil || !strings.Contains(err.Error(), "no steps recorded") {
		t.Fatalf("empty recorder: %v", err)
	}
	for i := 0; i < 4; i++ {
		r.Add(Step{Press: "Tab"})
	}
	steps, from, to, err := r.Range(0, 0)
	if err != nil || len(steps) != 4 || from != 1 || to != 4 {
		t.Fatalf("range all: %d %d %d %v", len(steps), from, to, err)
	}
	r.MarkSaved(4)
	if _, _, _, err := r.Range(0, 0); err == nil || !strings.Contains(err.Error(), "no new steps") {
		t.Fatalf("after save: %v", err)
	}
	if n := r.Add(Step{Back: true}); n != 5 {
		t.Fatalf("step number = %d", n)
	}
	if steps, from, _, _ := r.Range(0, 0); len(steps) != 1 || from != 5 {
		t.Fatalf("since save: %d from %d", len(steps), from)
	}
	if steps, _, _, _ := r.Range(2, 0); len(steps) != 4 {
		t.Fatalf("fromStep 2: %d", len(steps))
	}
	if _, _, _, err := r.Range(9, 0); err == nil {
		t.Fatal("out of range should fail")
	}
}

func TestStore(t *testing.T) {
	s := Store{Dir: t.TempDir() + "/flows"}
	if list, err := s.List(); err != nil || len(list) != 0 {
		t.Fatalf("missing dir should list nothing: %v %v", list, err)
	}
	f := Flow{Name: "login", Description: "x", Steps: []Step{{Open: "http://a"}}}
	path, replaced, err := s.Save(f)
	if err != nil || replaced || !strings.HasSuffix(path, "flows/login.yaml") {
		t.Fatalf("save: %s %v %v", path, replaced, err)
	}
	if _, replaced, _ := s.Save(f); !replaced {
		t.Fatal("second save should report replace")
	}
	got, err := s.Load("login")
	if err != nil || got.Steps[0].Open != "http://a" {
		t.Fatalf("load: %+v %v", got, err)
	}
	if _, err := s.Load("nope"); err == nil || !strings.Contains(err.Error(), "Call flow_list") {
		t.Fatalf("missing flow error: %v", err)
	}
	if list, _ := s.List(); len(list) != 1 || list[0].Name != "login" {
		t.Fatalf("list: %+v", list)
	}
}

func TestRecorderRangeTo(t *testing.T) {
	r := &Recorder{}
	for i := 0; i < 5; i++ {
		r.Add(Step{Press: "K" + strconv.Itoa(i)})
	}
	steps, from, to, err := r.Range(1, 2)
	if err != nil || len(steps) != 2 || from != 1 || to != 2 || steps[1].Press != "K1" {
		t.Fatalf("range 1-2: %v %d %d %v", steps, from, to, err)
	}
	r.MarkSaved(to)
	// Os passos depois do toStep continuam "novos" para o próximo flow_save.
	if steps, from, to, _ := r.Range(0, 0); len(steps) != 3 || from != 3 || to != 5 {
		t.Fatalf("after saving 1-2: %d steps from %d to %d", len(steps), from, to)
	}
	for _, bad := range [][2]int{{3, 2}, {1, 6}} {
		if _, _, _, err := r.Range(bad[0], bad[1]); err == nil || !strings.Contains(err.Error(), "toStep must be between") {
			t.Fatalf("range %v: %v", bad, err)
		}
	}
}

func TestDescribeTypeSubmit(t *testing.T) {
	sel := Selector{Role: "searchbox", Name: "Pesquisar"}
	if got := (Step{Type: &TypeStep{Selector: sel, Text: "x", Submit: true}}).Describe(); got != `type searchbox "Pesquisar" = "x" + Enter` {
		t.Fatalf("got %s", got)
	}
	if got := (Step{Type: &TypeStep{Selector: sel, Text: "x"}}).Describe(); strings.Contains(got, "Enter") {
		t.Fatalf("no submit, no Enter: %s", got)
	}
}

func TestPick(t *testing.T) {
	steps := []Step{{Open: "a"}, {Press: "Enter"}, {Back: true}, {Press: "Tab"}}
	got, err := Pick(steps, 5, []int{6, 8})
	if err != nil || len(got) != 2 || got[0].N != 5 || got[0].Step.Open != "a" || got[1].N != 7 || !got[1].Step.Back {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err := Pick(steps, 5, nil); err != nil || len(got) != 4 || got[3].N != 8 {
		t.Fatalf("no exclusions: %+v %v", got, err)
	}
	for _, tt := range []struct {
		exclude []int
		want    string
	}{
		{[]int{4}, "step #4 is not in the steps being saved (#5-#8)"},
		{[]int{9}, "step #9 is not in the steps being saved (#5-#8)"},
		{[]int{6, 6}, "step #6 appears twice"},
		{[]int{5, 6, 7, 8}, "excludeSteps removes every step (#5-#8): nothing to save"},
	} {
		if _, err := Pick(steps, 5, tt.exclude); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("exclude %v: %v, want %q", tt.exclude, err, tt.want)
		}
	}
	if got, err := Pick(steps, 5, []int{5, 8}); err != nil || len(got) != 2 || got[0].N != 6 || got[1].N != 7 {
		t.Fatalf("edges: %+v %v", got, err)
	}
}

func TestRecorderRemembersPreview(t *testing.T) {
	r := &Recorder{}
	if _, ok := r.LastPreview("a"); ok {
		t.Fatal("no preview yet")
	}
	ex := []int{3}
	r.RememberPreview(Selection{Name: "a", From: 1, To: 4, Exclude: ex})
	ex[0] = 9 // a prévia guardada não muda com quem chamou
	if sel, ok := r.LastPreview("a"); !ok || sel.From != 1 || sel.To != 4 || sel.Exclude[0] != 3 {
		t.Fatalf("%+v %v", sel, ok)
	}
	if _, ok := r.LastPreview("b"); ok {
		t.Fatal("another name has no preview")
	}
	r.MarkSaved(4)
	if _, ok := r.LastPreview("a"); ok {
		t.Fatal("saving forgets the preview")
	}
}

func TestFragileTargets(t *testing.T) {
	steps := []Step{
		{Open: "https://pt.wikipedia.org"},
		{Type: &TypeStep{Selector: Selector{Role: "searchbox", Name: "Pesquisar"}, Text: "Vitral"}},
		{Click: &Selector{Role: "link", Name: "Vitral vidraça decorativa"}},
		{Click: &Selector{Role: "button", Name: "Abrir", Within: &Context{Role: "row", Name: "Pedido VITRAL-9"}}},
		{Select: &SelectStep{Selector: Selector{Role: "combobox", Name: "Ordem"}, Option: "Vitral"}},
		{Scroll: &ScrollStep{Selector: &Selector{Role: "heading", Name: "Sobre vitral"}}},
		{WaitFor: &WaitStep{Element: &Selector{Role: "heading", Name: "Vitral"}}},
		{WaitFor: &WaitStep{Text: "Vitral"}},
		{Type: &TypeStep{Selector: Selector{Role: "textbox", Name: "Nome"}, Text: "x"}},
	}
	got := FragileTargets(steps, map[string]string{"termo": "Vitral", "vazio": ""})
	if len(got) != 4 {
		t.Fatalf("%d warnings:\n%s", len(got), strings.Join(got, "\n"))
	}
	for i, want := range []string{`step 3 (click link "Vitral vidraça decorativa")`, "step 4 (click", "step 6 (scroll to", "step 7 (wait_for"} {
		if !strings.HasPrefix(got[i], want) || !strings.Contains(got[i], `the value of param termo ("Vitral")`) {
			t.Errorf("warning %d: %s", i, got[i])
		}
	}
	// Sem params: o texto digitado antes também conta (a sugestão de busca depende dele).
	got = FragileTargets(steps, nil)
	if len(got) != 4 || !strings.Contains(got[0], `the text typed in step 2 ("Vitral")`) || !strings.Contains(got[0], "submit=true") {
		t.Fatalf("typed text:\n%s", strings.Join(got, "\n"))
	}
	// O texto digitado só vale para os passos depois dele, e textos curtos não contam.
	if got := FragileTargets([]Step{steps[2], steps[1]}, nil); got != nil {
		t.Errorf("target before the typing: %v", got)
	}
	short := []Step{{Type: &TypeStep{Selector: Selector{Role: "textbox", Name: "Q"}, Text: "ab"}}, {Click: &Selector{Role: "link", Name: "abacate"}}}
	if got := FragileTargets(short, nil); got != nil {
		t.Errorf("short typed text: %v", got)
	}
}

func TestParameterizeSwappedHint(t *testing.T) {
	steps := []Step{{Type: &TypeStep{Selector: Selector{Role: "searchbox", Name: "Busca"}, Text: "Vitral"}}}
	_, _, err := Parameterize(steps, map[string]string{"Vitral": "termo"})
	if err == nil || !strings.Contains(err.Error(), `name and value look swapped. Use params={termo: "Vitral"}`) {
		t.Fatalf("%v", err)
	}
	_, _, err = Parameterize(steps, map[string]string{"cidade": "Recife"})
	if err == nil || strings.Contains(err.Error(), "swapped") {
		t.Fatalf("%v", err)
	}
}

func TestWarnings(t *testing.T) {
	typed := Step{Type: &TypeStep{Selector: Selector{Role: "searchbox", Name: "Busca"}, Text: "Vitral"}}
	sent := Step{Type: &TypeStep{Selector: Selector{Role: "searchbox", Name: "Busca"}, Text: "Vitral", Submit: true}}
	wait := Step{WaitFor: &WaitStep{Text: "Vitral"}}
	open := Step{Open: "https://x"}
	got := Warnings([]Step{open, typed, wait}, nil)
	if len(got) != 1 || !strings.HasPrefix(got[0], `step 2 (type searchbox "Busca" = "Vitral") is the last action and does not press Enter`) {
		t.Fatalf("%v", got)
	}
	for _, steps := range [][]Step{{open, sent, wait}, {open, typed, {Press: "Enter"}, wait}, {wait}, {typed, {Click: &Selector{Role: "button", Name: "Ir"}}}} {
		if got := Warnings(steps, nil); len(got) != 0 {
			t.Errorf("%v: %v", steps, got)
		}
	}
	// Junta com os alvos frágeis.
	click := Step{Click: &Selector{Role: "option", Name: "Vitral vidraça"}}
	if got := Warnings([]Step{typed, click, typed}, nil); len(got) != 2 {
		t.Fatalf("%v", got)
	}
}

func TestExpectWarnings(t *testing.T) {
	params := map[string]string{"termo": "Vitral"}
	if got := ExpectWarnings("{{termo}}", params); len(got) != 1 || !strings.Contains(got[0], "placeholder written by hand") {
		t.Fatalf("%v", got)
	}
	if got := ExpectWarnings("O vitral é", params); len(got) != 1 || !strings.Contains(got[0], "different capitalization, so it does not become {{termo}}") {
		t.Fatalf("%v", got)
	}
	for _, ok := range []string{"Vitral – Wikipédia", "O Vitral é", "Pedido enviado"} {
		if got := ExpectWarnings(ok, params); len(got) != 0 {
			t.Errorf("%q: %v", ok, got)
		}
	}
	if got := ExpectWarnings("o vitral", map[string]string{"vazio": ""}); len(got) != 0 {
		t.Errorf("empty value: %v", got)
	}
}
