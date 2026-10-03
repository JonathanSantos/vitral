package flow

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzParameterizeBind: trocar um valor por {{param}} e depois aplicar o mesmo valor devolve o texto.
func FuzzParameterizeBind(f *testing.F) {
	f.Add("https://portal/pedidos?busca=1023", "Buscar 1023", "1023")
	f.Add("http://x", "Maria Silva", "Maria Silva")
	f.Add("http://x/{{a}}", "{{nome}} texto", "texto")
	f.Fuzz(func(t *testing.T, url, text, value string) {
		if utf8.RuneCountInString(value) < MinParamLength || strings.Contains(value, "{{") || strings.Contains(value, "}}") ||
			strings.Contains(url, "{{") || strings.Contains(text, "{{") || !strings.Contains(text, value) {
			t.Skip()
		}
		steps := []Step{{Open: "http://x/" + url}, {Type: &TypeStep{Selector: Selector{Role: "textbox", Name: value}, Text: text}}}
		out, subs, err := Parameterize(steps, map[string]string{"p": value})
		if err != nil {
			t.Fatalf("parameterize: %v", err)
		}
		if len(subs) == 0 || out[1].Type.Name != value {
			t.Fatalf("selector must not change: %+v", out[1].Type)
		}
		bound, err := Bind(Flow{Name: "f", Params: []string{"p"}, Steps: out}, map[string]string{"p": value})
		if err != nil {
			t.Fatalf("bind: %v", err)
		}
		if bound[1].Type.Text != text || bound[0].Open != steps[0].Open {
			t.Fatalf("roundtrip: %q -> %q", text, bound[1].Type.Text)
		}
	})
}

// FuzzFlowYAML: o que o parser aceita, o render reproduz e o parser aceita de novo, igual.
func FuzzFlowYAML(f *testing.F) {
	f.Add("name: a\nsteps:\n  - open: http://x\n")
	f.Add("name: buscar-pedido\nparams: [numero]\nsteps:\n  - type: { role: textbox, name: \"Buscar\", text: \"{{numero}}\" }\n  - click: { role: button, name: Filtrar, nth: 2, within: { role: dialog, name: X } }\n  - wait_for: { text: \"#{{numero}}\", gone: true }\n")
	f.Add("name: x\nsteps: [ { back: true }, { press: Enter }, { mock_add: { urlPattern: '/api', status: 500 } } ]")
	f.Add(":")
	f.Fuzz(func(t *testing.T, data string) {
		parsed, err := Unmarshal([]byte(data))
		if err != nil {
			return
		}
		out, err := Marshal(parsed)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		again, err := Unmarshal(out)
		if err != nil {
			t.Fatalf("re-parse of rendered flow failed: %v\n%s", err, out)
		}
		out2, _ := Marshal(again)
		if string(out) != string(out2) {
			t.Fatalf("render is not stable:\n%s\n---\n%s", out, out2)
		}
	})
}
