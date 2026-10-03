package snapshot

import (
	"strings"
	"testing"
)

// Fronteiras exatas (apontadas pela mutação): limites iguais ao total não podem mudar a saída.
func TestFormatBoundaries(t *testing.T) {
	rows := func(n int) []*Node {
		var out []*Node
		for i := 0; i < n; i++ {
			out = append(out, &Node{Ref: "r" + string(rune('a'+i)), Role: "row", Name: "x"})
		}
		return out
	}
	two := 2
	tests := []struct {
		name   string
		page   *Page
		opts   FormatOptions
		has    []string
		hasNot []string
	}{
		{
			name:   "items equal to the list limit: no '(showing)'",
			page:   page("d", &Node{Ref: "e1", Role: "table", Count: &two, Children: rows(2)}),
			opts:   FormatOptions{MaxLines: 10, TextMax: 80, ListMax: 2},
			has:    []string{"rows=2", "[rb] row"},
			hasNot: []string{"showing"},
		},
		{
			name:   "lines equal to maxLines: no '… more'",
			page:   page("d", &Node{Ref: "e1", Role: "button"}, &Node{Ref: "e2", Role: "button"}),
			opts:   FormatOptions{MaxLines: 2, TextMax: 80, ListMax: 5},
			hasNot: []string{"more elements"},
		},
		{
			name: "scope hint points at the first top-level ancestor",
			page: page("d", &Node{Ref: "e1", Role: "main", Children: []*Node{{Ref: "e2", Role: "button"}, {Ref: "e3", Role: "button"}}}),
			opts: FormatOptions{MaxLines: 2, TextMax: 80, ListMax: 5},
			has:  []string{"… 1 more elements (use snapshot with scope=e1)"},
		},
		{
			name: "name exactly at the limit is not truncated",
			page: page("d", &Node{Ref: "e1", Role: "button", Name: "abcd"}),
			opts: FormatOptions{MaxLines: 5, TextMax: 4, ListMax: 5},
			has:  []string{`"abcd"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := Format(tt.page, tt.opts)
			for _, h := range tt.has {
				if !strings.Contains(out, h) {
					t.Errorf("missing %q in:\n%s", h, out)
				}
			}
			for _, h := range tt.hasNot {
				if strings.Contains(out, h) {
					t.Errorf("unexpected %q in:\n%s", h, out)
				}
			}
		})
	}
}

func TestDiffBoundaries(t *testing.T) {
	list := func() *Node {
		three := 3
		return &Node{Ref: "e1", Role: "list", Count: &three, Children: []*Node{
			{Ref: "e2", Role: "listitem", Name: "a"}, {Ref: "e3", Role: "listitem", Name: "b"}, {Ref: "e4", Role: "listitem", Name: "c"},
		}}
	}
	// Lista sem mudança, maior que o limite de itens: nada de "~" falso nos itens escondidos.
	if got := Summarize(page("d", list()), page("d", list()), FormatOptions{MaxLines: 300, TextMax: 80, ListMax: 1}); got != "no visible changes" {
		t.Fatalf("unchanged list: %q", got)
	}
	// Folha removida não ganha "(+0 inside)".
	got := Summarize(page("d", &Node{Ref: "e1", Role: "button", Name: "a"}, &Node{Ref: "e2", Role: "button", Name: "b"}), page("d", &Node{Ref: "e1", Role: "button", Name: "a"}), DefaultFormat())
	if got != `- [e2] button "b"` {
		t.Fatalf("leaf removal: %q", got)
	}
}

func TestParseValidPage(t *testing.T) {
	p, err := Parse([]byte(`{"docId":"d","url":"http://x","nodes":[{"ref":"e1","role":"button"}]}`))
	if err != nil || p.DocID != "d" || len(p.Nodes) != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := Parse([]byte(`{"error":"boom"}`)); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("generic error: %v", err)
	}
}

func TestFormatInspectionOptionalLines(t *testing.T) {
	base := func() *Inspection {
		return &Inspection{Ref: "e1", Role: "button", Name: "Ok", Tag: "button", Styles: map[string]string{
			"background-color": "rgb(0, 0, 0)", "background-image": "none", "box-shadow": "none",
			"text-decoration-line": "none", "text-transform": "none", "border-top-width": "0px",
		}, Text: "Ok"}
	}
	plain := FormatInspection(base())
	for _, absent := range []string{"background-image", "box-shadow", "text-decoration", "text-transform", "states:", "aria:", "text:"} {
		if strings.Contains(plain, absent) {
			t.Errorf("%q should be omitted:\n%s", absent, plain)
		}
	}
	rich := base()
	rich.Styles["background-image"] = "url(x.png)"
	rich.Styles["box-shadow"] = "0 1px 2px red"
	rich.Styles["text-decoration-line"] = "underline"
	rich.Styles["text-transform"] = "uppercase"
	rich.States = []string{"disabled"}
	rich.Text = "Ok agora"
	out := FormatInspection(rich)
	for _, want := range []string{"background-image: url(x.png)", "box-shadow: 0 1px 2px red", "text-decoration: underline", "text-transform: uppercase", "states: disabled", `text: "Ok agora"`, "border: none"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestZeroLimitsHaveMeaning(t *testing.T) {
	three := 3
	p := page("d", &Node{Ref: "e1", Role: "list", Count: &three, Children: []*Node{
		{Ref: "e2", Role: "listitem"}, {Ref: "e3", Role: "listitem"}, {Ref: "e4", Role: "listitem"},
	}})
	// ListMax 0: nenhum item, só o total.
	if out := Format(p, FormatOptions{MaxLines: 10, TextMax: 80, ListMax: 0}); strings.Contains(out, "listitem") || !strings.Contains(out, "items=3 (showing 0)") {
		t.Fatalf("ListMax 0:\n%s", out)
	}
	// MaxLines 0: sem limite de linhas.
	if out := Format(p, FormatOptions{MaxLines: 0, TextMax: 80, ListMax: -1}); !strings.Contains(out, "[e4]") || strings.Contains(out, "more elements") {
		t.Fatalf("MaxLines 0:\n%s", out)
	}
}
