package snapshot

import (
	"strings"
	"testing"
)

func str(s string) *string { return &s }
func num(n int) *int       { return &n }

func samplePage() *Page {
	rows := []*Node{}
	for i := 1; i <= 7; i++ {
		rows = append(rows, &Node{Ref: "r" + string(rune('0'+i)), Role: "row", Name: "#10" + string(rune('0'+i))})
	}
	rows[0].Children = []*Node{{Ref: "e5", Role: "link", Name: "Ver detalhes"}}
	return &Page{
		DocID: "d1", URL: "https://portal/pedidos", Title: "Pedidos",
		Viewport: [2]int{1280, 800}, Scroll: [2]int{0, 2400},
		Nodes: []*Node{
			{Ref: "e1", Role: "textbox", Name: "Buscar pedido", Value: str(""), States: []string{"invalid"}, InvalidMessage: "Preencha"},
			{Ref: "e2", Role: "button", Name: "Filtrar", States: []string{"disabled"}},
			{Ref: "e3", Role: "table", Name: "Pedidos", Count: num(24), Children: rows},
			{Ref: "e9", Role: "heading", Name: "Resumo", Level: 2},
		},
	}
}

func TestFormat(t *testing.T) {
	got := Format(samplePage(), DefaultFormat())
	want := strings.Join([]string{
		`page: "Pedidos" (https://portal/pedidos)`,
		`viewport: 1280x800, scroll: 0/2400`,
		`[e1] textbox "Buscar pedido" value="" invalid="Preencha"`,
		`[e2] button "Filtrar" disabled`,
		`[e3] table "Pedidos" rows=24 (showing 5)`,
		`  [r1] row "#101"`,
		`    [e5] link "Ver detalhes"`,
		`  [r2] row "#102"`,
		`  [r3] row "#103"`,
		`  [r4] row "#104"`,
		`  [r5] row "#105"`,
		`[e9] heading "Resumo" level=2`,
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatLimits(t *testing.T) {
	tests := []struct {
		name     string
		opts     FormatOptions
		contains []string
		absent   []string
	}{
		{
			name:     "max lines",
			opts:     FormatOptions{MaxLines: 3, TextMax: 80, ListMax: 5},
			contains: []string{"… 7 more elements (use snapshot with scope=e3)"},
			absent:   []string{"[r1]"},
		},
		{
			name:     "scope root shows all items",
			opts:     FormatOptions{MaxLines: 300, TextMax: 80, ListMax: 5, ScopeRoot: true},
			contains: []string{"[r7]", "rows=24 (showing 7)"},
		},
		{
			name:     "truncate",
			opts:     FormatOptions{MaxLines: 300, TextMax: 6, ListMax: 5},
			contains: []string{`"Busca…"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Format(samplePage(), tt.opts)
			for _, c := range tt.contains {
				if !strings.Contains(got, c) {
					t.Errorf("missing %q in:\n%s", c, got)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(got, a) {
					t.Errorf("unexpected %q in:\n%s", a, got)
				}
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		max  int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdef", 4, "abc…"},
		{"ação longa", 4, "açã…"},
		{"x", 0, "x"},
	}
	for _, tt := range tests {
		if got := Truncate(tt.in, tt.max); got != tt.want {
			t.Errorf("Truncate(%q,%d)=%q want %q", tt.in, tt.max, got, tt.want)
		}
	}
}

func TestRefNotFoundMessage(t *testing.T) {
	if got := (ErrRefNotFound{Ref: "e9"}).Error(); got != "ref e9 not found: page changed since last snapshot. Call snapshot." {
		t.Fatalf("unexpected error: %q", got)
	}
}
