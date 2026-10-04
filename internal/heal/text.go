package heal

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// stopwords não contam na comparação de nomes: "Salvar as alterações" e "Salvar alterações" são o
// mesmo botão.
var stopwords = map[string]bool{
	"a": true, "o": true, "as": true, "os": true, "de": true, "da": true, "do": true, "das": true, "dos": true,
	"e": true, "em": true, "no": true, "na": true, "nos": true, "nas": true, "para": true, "por": true,
	"um": true, "uma": true, "the": true, "an": true, "of": true, "to": true, "and": true, "for": true,
	"in": true, "on": true,
}

// Normalize deixa o texto comparável: minúsculas, sem acento, só letras e dígitos, espaços colapsados.
func Normalize(s string) string {
	var b strings.Builder
	space := false
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(unicode.ToLower(r))
		default:
			space = true
		}
	}
	return b.String()
}

// tokens são as palavras do texto normalizado, sem stopwords (ou todas, se só houver stopwords).
func tokens(s string) []string {
	all := strings.Fields(Normalize(s))
	var out []string
	for _, t := range all {
		if !stopwords[t] {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// Similarity compara dois nomes por palavras (coeficiente de Dice, sem repetição): 1 é igual, 0 não
// tem palavra em comum. Dois textos vazios são iguais.
func Similarity(a, b string) float64 {
	ta, tb := set(tokens(a)), set(tokens(b))
	if len(ta) == 0 && len(tb) == 0 {
		return 1
	}
	common := 0
	for t := range ta {
		if tb[t] {
			common++
		}
	}
	return 2 * float64(common) / float64(len(ta)+len(tb))
}

func set(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, t := range list {
		m[t] = true
	}
	return m
}
