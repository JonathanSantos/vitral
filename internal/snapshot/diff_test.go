package snapshot

import (
	"strings"
	"testing"
)

func page(doc string, nodes ...*Node) *Page {
	return &Page{DocID: doc, URL: "http://x/", Title: "T", Nodes: nodes}
}

func TestSummarize(t *testing.T) {
	form := func() []*Node {
		return []*Node{
			{Ref: "e1", Role: "textbox", Name: "Email", Value: str("")},
			{Ref: "e2", Role: "button", Name: "Enviar"},
		}
	}
	dialog := &Node{Ref: "e6", Role: "dialog", Name: "Confirmar", States: []string{"open"}, Children: []*Node{
		{Ref: "e7", Role: "button", Name: "Cancelar"},
		{Ref: "e8", Role: "button", Name: "Excluir"},
	}}

	tests := []struct {
		name string
		prev *Page
		curr *Page
		want string
	}{
		{
			name: "no changes",
			prev: page("d", form()...),
			curr: page("d", form()...),
			want: "no visible changes",
		},
		{
			name: "value changed",
			prev: page("d", form()...),
			curr: func() *Page {
				n := form()
				n[0].Value = str("a@b.com")
				return page("d", n...)
			}(),
			want: `~ [e1] textbox "Email" value="a@b.com"`,
		},
		{
			name: "dialog added with children",
			prev: page("d", form()...),
			curr: page("d", append(form(), dialog)...),
			want: strings.Join([]string{
				`+ [e6] dialog "Confirmar" open`,
				`+   [e7] button "Cancelar"`,
				`+   [e8] button "Excluir"`,
			}, "\n"),
		},
		{
			name: "dialog removed collapses children",
			prev: page("d", append(form(), dialog)...),
			curr: page("d", form()...),
			want: `- [e6] dialog "Confirmar" open (+2 inside)`,
		},
		{
			name: "added inside existing parent",
			prev: page("d", &Node{Ref: "e1", Role: "form", Children: []*Node{{Ref: "e2", Role: "button", Name: "Ok"}}}),
			curr: page("d", &Node{Ref: "e1", Role: "form", Children: []*Node{
				{Ref: "e2", Role: "button", Name: "Ok"},
				{Ref: "e3", Role: "alert", Name: "Saved"},
			}}),
			want: `+ [e3] alert "Saved" (in form e1)`,
		},
		{
			name: "scroll summarizes elements that left",
			prev: page("d", form()...),
			curr: func() *Page {
				p := page("d", &Node{Ref: "e9", Role: "link", Name: "Rodapé"})
				p.Offscreen = []string{"e1", "e2"}
				return p
			}(),
			want: "+ [e9] link \"Rodapé\"\n- 2 elements left the viewport",
		},
		{
			name: "url and title",
			prev: page("d", form()...),
			curr: func() *Page {
				p := page("d", form()...)
				p.URL, p.Title = "http://x/#tab2", "Novo"
				return p
			}(),
			want: "url: http://x/#tab2\ntitle: \"Novo\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize(tt.prev, tt.curr, DefaultFormat())
			if got != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestSummarizeFallsBackToSnapshot(t *testing.T) {
	prev := page("d1", &Node{Ref: "e1", Role: "button", Name: "A"})
	curr := page("d2", &Node{Ref: "e1", Role: "button", Name: "B"})
	if got := Summarize(prev, curr, DefaultFormat()); !strings.HasPrefix(got, "page: ") {
		t.Fatalf("navigation should return full snapshot, got %q", got)
	}
	if got := Summarize(nil, curr, DefaultFormat()); !strings.HasPrefix(got, "page: ") {
		t.Fatalf("no previous snapshot should return full snapshot, got %q", got)
	}

	// Trocar tudo gera um diff maior que o próprio snapshot.
	var many, other []*Node
	for i := 0; i < 20; i++ {
		many = append(many, &Node{Ref: "a" + string(rune('a'+i)), Role: "button", Name: "x"})
		other = append(other, &Node{Ref: "b" + string(rune('a'+i)), Role: "button", Name: "y"})
	}
	if got := Summarize(page("d", many...), page("d", other...), DefaultFormat()); !strings.HasPrefix(got, "page: ") {
		t.Fatalf("big diff should return full snapshot, got %q", got)
	}
}
