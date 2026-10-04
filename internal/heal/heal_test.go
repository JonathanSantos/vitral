package heal

import (
	"math"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestNormalizeAndSimilarity(t *testing.T) {
	if got := Normalize("  Salvar   AS alterações!! "); got != "salvar as alteracoes" {
		t.Fatalf("normalize: %q", got)
	}
	if got := Normalize("Ação—rápida_2"); got != "acao rapida 2" {
		t.Fatalf("normalize punctuation: %q", got)
	}
	tests := []struct {
		a, b string
		want float64
	}{
		{"Salvar alterações", "Salvar as alterações", 1},
		{"Salvar alterações", "salvar ALTERACOES", 1},
		{"Confirmar", "Confirmar pedido", 2.0 / 3},
		{"Salvar perfil", "Gravar dados", 0},
		{"", "", 1},
		{"Ok", "", 0},
		{"de", "de", 1},           // só stopwords: comparam como estão
		{"a b", "a b c", 2.0 / 3}, // "a" é stopword: sobram {b} e {b, c}
		{"x x y", "x y", 1},       // repetição não conta
	}
	for _, tt := range tests {
		if got := Similarity(tt.a, tt.b); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Similarity(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRoleCompatible(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"button", "button", true},
		{"button", "link", true},
		{"link", "button", true},
		{"textbox", "spinbutton", false},
		{"checkbox", "switch", false},
		{"button", "clickable", false},
		{"link", "link", true},
	} {
		if got := RoleCompatible(tt.a, tt.b); got != tt.want {
			t.Errorf("RoleCompatible(%s, %s) = %v", tt.a, tt.b, got)
		}
	}
}

func TestStableID(t *testing.T) {
	for id, want := range map[string]bool{
		"salvar-perfil": true, "cep": true, "btn_enviar": true, "user-2": true,
		"": false, "   ": false, "pedido-10234": false, "a1b2c3d4e5f6": false, ":r1a:": false, "ember312": false,
		"react-select-3-input": false, "mui-17": false, "headlessui-menu-button-:r2:": false, "radix-:R1:": false,
		"__next": false, "css-1x2y3z": false, "id-7": false, strings.Repeat("a", 65): false,
	} {
		if got := StableID(id); got != want {
			t.Errorf("StableID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestSensitive(t *testing.T) {
	for name, want := range map[string]bool{
		"Excluir pedido": true, "Confirmar": true, "CONFIRMAÇÃO da compra": true, "Pay now": true, "Submit": true,
		"Salvar": false, "Detalhes": false, "Confirmado": false, "": false, "Envio de arquivo": true,
	} {
		if got := Sensitive(name); got != want {
			t.Errorf("Sensitive(%q) = %v, want %v", name, got, want)
		}
	}
	if !Sensitive("Voltar", "Apagar tudo") || Sensitive() {
		t.Error("any of the names counts; none is not sensitive")
	}
	if lexicon["#"] || lexicon["portugues"] {
		t.Error("comments are not words of the lexicon")
	}
}

// base é um botão gravado no conteúdo principal, abaixo do título "Perfil".
func base() Signature {
	return Signature{Role: "button", Name: "Salvar alterações", Tag: "BUTTON", Landmark: "main", Heading: "Perfil", Ordinal: 1, Same: 1, Pos: [2]float64{0.3, 0.4}}
}

func TestScore(t *testing.T) {
	rec := base()
	if s, ok := Score(rec, rec); !ok || s != 1 {
		t.Fatalf("identical: %v %v", s, ok)
	}
	// Incompatíveis nunca servem.
	cand := rec
	cand.Role = "textbox"
	if _, ok := Score(rec, cand); ok {
		t.Fatal("incompatible role")
	}
	cand = rec
	cand.InputType = "email"
	if _, ok := Score(rec, cand); ok {
		t.Fatal("input type never changes")
	}
	// Botão ↔ link vale, com nota menor.
	cand = rec
	cand.Role = "link"
	if s, ok := Score(rec, cand); !ok || s >= 1 || s < MinScore {
		t.Fatalf("button to link: %v %v", s, ok)
	}
	// Um botão diferente no mesmo lugar não passa do limiar.
	cand = rec
	cand.Name = "Cancelar"
	if s, _ := Score(rec, cand); s >= MinScore {
		t.Fatalf("different button scored %v", s)
	}
	cand.Name = "Salvar rascunho"
	if s, _ := Score(rec, cand); s >= MinScore {
		t.Fatalf("half the name scored %v", s)
	}
	// O id só soma quando bate; um id diferente não derruba a nota.
	rec.ID = "salvar"
	cand = rec
	cand.ID = "save-btn"
	withOther, _ := Score(rec, cand)
	cand.ID = "salvar"
	withSame, _ := Score(rec, cand)
	if withOther != 1 || withSame != 1 {
		t.Fatalf("id is a bonus: other %v, same %v", withOther, withSame)
	}
	cand.Name = "Gravar dados"
	renamedSameID, _ := Score(rec, cand)
	cand.ID = ""
	renamed, _ := Score(rec, cand)
	if renamedSameID <= renamed {
		t.Fatalf("a matching id must help: %v vs %v", renamedSameID, renamed)
	}
	// Atributos de identidade e contexto contam.
	rec = base()
	rec.TestID, rec.NameAttr, rec.Href, rec.Label, rec.Placeholder = "save", "salvar", "/perfil", "Salvar", "Digite"
	rec.Column, rec.RowHeader = "Ações", "Ana"
	if s, _ := Score(rec, rec); s != 1 {
		t.Fatalf("full signature with itself: %v", s)
	}
	for _, change := range []func(*Signature){
		func(c *Signature) { c.TestID = "other" }, func(c *Signature) { c.NameAttr = "x" }, func(c *Signature) { c.Href = "/x" },
		func(c *Signature) { c.Label = "Outro" }, func(c *Signature) { c.Placeholder = "Outro" }, func(c *Signature) { c.Tag = "A" },
		func(c *Signature) { c.Landmark = "navigation" }, func(c *Signature) { c.Dialog = "Confirmar" }, func(c *Signature) { c.Heading = "Outro" },
		func(c *Signature) { c.Column = "Outro" }, func(c *Signature) { c.RowHeader = "Bia" }, func(c *Signature) { c.Ordinal = 2 },
		func(c *Signature) { c.Pos = [2]float64{0.9, 0.9} },
	} {
		c := rec
		change(&c)
		if s, _ := Score(rec, c); s >= 1 {
			t.Errorf("a changed attribute must lower the score: %+v", c)
		}
	}
	// Cabeçalho de coluna só conta quando existe em algum dos lados.
	a, b := base(), base()
	b.Column = "Ações"
	if s, _ := Score(a, b); s >= 1 {
		t.Error("a column only on the candidate counts")
	}
	b.Column, b.RowHeader = "", "Ana"
	if s, _ := Score(a, b); s >= 1 {
		t.Error("a row header only on the candidate counts")
	}
}

// Os seis casos do pedido (3.5), como assinaturas: a página v1 gravou rec; a v2 tem os candidatos.
func TestCases(t *testing.T) {
	decide := func(rec Signature, cands ...Signature) Decision { return Decide(Rank(rec, cands)) }

	// id e texto mudaram juntos ("Aplicar" com id aplicar → "Aplicar filtros" com outro id).
	rec := Signature{Role: "button", Name: "Aplicar", ID: "aplicar", Tag: "BUTTON", Landmark: "main", Heading: "Filtros", Ordinal: 1, Same: 1, Pos: [2]float64{0.2, 0.3}}
	v2 := rec
	v2.Name, v2.ID = "Aplicar filtros", "apply-filters"
	other := rec
	other.Name, other.ID, other.Pos = "Limpar", "limpar", [2]float64{0.3, 0.3}
	if d := decide(rec, other, v2); !d.Accept || d.Best.Index != 1 {
		t.Errorf("id changed: %+v", d)
	}

	// Texto mudou levemente.
	rec = base()
	v2 = rec
	v2.Name = "Salvar as alterações"
	other = rec
	other.Name, other.Ordinal = "Salvar rascunho", 1
	if d := decide(rec, other, v2); !d.Accept || d.Best.Index != 1 {
		t.Errorf("slight text: %+v", d)
	}

	// Mudou de lugar: o Exportar do relatório saiu da região e foi para a barra do topo; o do gráfico ficou.
	rec = Signature{Role: "button", Name: "Exportar", TestID: "exportar-relatorio", Tag: "BUTTON", Landmark: "region Relatório", Heading: "Relatório", Ordinal: 1, Same: 2, Pos: [2]float64{0.5, 0.4}}
	moved := rec
	moved.Landmark, moved.Heading, moved.Pos = "", "Painel", [2]float64{0.9, 0.05}
	graph := rec
	graph.TestID, graph.Landmark, graph.Heading, graph.Pos = "exportar-grafico", "region Gráfico", "Gráfico", [2]float64{0.5, 0.7}
	if d := decide(rec, graph, moved); !d.Accept || d.Best.Index != 1 {
		t.Errorf("moved: %+v", d)
	}

	// Dois botões parecidos: "Detalhes" virou dois "Ver detalhes" no mesmo lugar.
	rec = Signature{Role: "button", Name: "Detalhes", Tag: "BUTTON", Landmark: "main", Heading: "Pedidos", Ordinal: 1, Same: 1, Pos: [2]float64{0.5, 0.5}}
	a, b := rec, rec
	a.Name, b.Name, b.Ordinal = "Ver detalhes", "Ver detalhes", 2
	if d := decide(rec, a, b); d.Accept || !strings.Contains(d.Reason, "ambiguous") || len(d.Top) != 2 {
		t.Errorf("two similar: %+v", d)
	}

	// Botão sensível renomeado: a nota passa (quem pede confirmação é o flow_run).
	rec = Signature{Role: "button", Name: "Confirmar", Tag: "BUTTON", Landmark: "main", Heading: "Revisão", Ordinal: 1, Same: 1, Pos: [2]float64{0.5, 0.8}}
	v2 = rec
	v2.Name = "Confirmar pedido"
	back := rec
	back.Name, back.Pos = "Voltar", [2]float64{0.3, 0.8}
	if d := decide(rec, back, v2); !d.Accept || d.Best.Index != 1 || !Sensitive(rec.Name, v2.Name) {
		t.Errorf("sensitive: %+v", d)
	}

	// Input trocou de tipo: texto virou número (spinbutton) e o e-mail virou senha.
	rec = Signature{Role: "textbox", Name: "Quantidade", Tag: "INPUT", Landmark: "main", Ordinal: 1, Same: 1}
	num := rec
	num.Role, num.InputType = "spinbutton", "number"
	if d := decide(rec, num); d.Accept || d.Reason != "no compatible element on the page" {
		t.Errorf("input type: %+v", d)
	}
	rec.InputType = "email"
	pw := rec
	pw.InputType = "password"
	if d := decide(rec, pw); d.Accept {
		t.Errorf("email to password: %+v", d)
	}
}

func TestDecide(t *testing.T) {
	if d := Decide(nil); d.Accept || d.Reason != "no compatible element on the page" || len(d.Top) != 0 {
		t.Fatalf("%+v", d)
	}
	if d := Decide([]Ranked{{0, 0.80}}); !d.Accept || d.Margin != 0.8 {
		t.Fatalf("single: %+v", d)
	}
	if d := Decide([]Ranked{{0, 0.74}}); d.Accept || !strings.Contains(d.Reason, "below the confidence threshold") {
		t.Fatalf("low: %+v", d)
	}
	if d := Decide([]Ranked{{0, 0.75}, {1, 0.60}}); !d.Accept || d.Margin != 0.15 {
		t.Fatalf("at the limits: %+v", d)
	}
	if d := Decide([]Ranked{{0, 0.90}, {1, 0.76}, {2, 0.5}, {3, 0.1}}); d.Accept || len(d.Top) != 3 || d.Margin != 0.14 {
		t.Fatalf("narrow margin: %+v", d)
	}
}

func TestRankOrder(t *testing.T) {
	rec := base()
	far, near, bad := rec, rec, rec
	far.Name = "Cancelar"
	near.Name = "Salvar as alterações"
	bad.Role = "checkbox"
	got := Rank(rec, []Signature{far, bad, near, near})
	if len(got) != 3 || got[0].Index != 2 || got[1].Index != 3 || got[2].Index != 0 {
		t.Fatalf("%+v", got)
	}
}

func genSig(t *rapid.T, label string) Signature {
	pick := func(name string, from ...string) string { return rapid.SampledFrom(from).Draw(t, label+name) }
	return Signature{
		Role:      pick("role", "button", "link", "textbox", "checkbox"),
		Name:      pick("name", "", "Salvar", "Salvar alterações", "Cancelar", "Excluir pedido", "Ver detalhes"),
		TestID:    pick("testid", "", "save", "cancel"),
		ID:        pick("id", "", "salvar", "x"),
		Tag:       pick("tag", "BUTTON", "A", "INPUT"),
		InputType: pick("type", "", "", "email"),
		Landmark:  pick("landmark", "", "main", "navigation"),
		Heading:   pick("heading", "", "Perfil", "Pedidos"),
		Column:    pick("column", "", "Ações"),
		Ordinal:   rapid.IntRange(0, 3).Draw(t, label+"ordinal"),
		Pos:       [2]float64{rapid.Float64Range(0, 1).Draw(t, label+"x"), rapid.Float64Range(0, 1).Draw(t, label+"y")},
	}
}

// Propriedades (pedido 3.5): a autocura nunca aceita com margem abaixo do limiar nem nota abaixo do
// mínimo; notas ficam entre 0 e 1; a ordem é decrescente; um elemento idêntico tem nota 1.
func TestHealingProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rec := genSig(t, "rec")
		n := rapid.IntRange(0, 6).Draw(t, "n")
		cands := make([]Signature, n)
		for i := range cands {
			cands[i] = genSig(t, "c")
		}
		ranked := Rank(rec, cands)
		for i, r := range ranked {
			if r.Score < 0 || r.Score > 1 {
				t.Fatalf("score out of range: %v", r.Score)
			}
			if i > 0 && r.Score > ranked[i-1].Score {
				t.Fatalf("not sorted: %+v", ranked)
			}
		}
		d := Decide(ranked)
		if d.Accept {
			second := 0.0
			if len(ranked) > 1 {
				second = ranked[1].Score
			}
			if d.Best.Score < MinScore || d.Best.Score-second < MinMargin-1e-9 {
				t.Fatalf("accepted below the thresholds: %+v (second %v)", d, second)
			}
		}
		if s, ok := Score(rec, rec); !ok || s != 1 {
			t.Fatalf("identical signature: %v %v", s, ok)
		}
	})
}

func FuzzSimilarity(f *testing.F) {
	f.Add("Salvar alterações", "salvar as ALTERAÇÕES")
	f.Add("", "x")
	f.Add("ç̧̧", "́")
	f.Fuzz(func(t *testing.T, a, b string) {
		s1, s2 := Similarity(a, b), Similarity(b, a)
		if s1 != s2 || s1 < 0 || s1 > 1 {
			t.Fatalf("Similarity(%q, %q) = %v / %v", a, b, s1, s2)
		}
		if Similarity(a, a) != 1 {
			t.Fatalf("Similarity(%q, itself) != 1", a)
		}
	})
}
