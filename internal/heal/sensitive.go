package heal

import (
	_ "embed"
	"strings"
)

//go:embed sensitive.txt
var lexiconFile string

// lexicon são as palavras de ações sensíveis (sensitive.txt, versionado).
var lexicon = func() map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(lexiconFile, "\n") {
		if w := Normalize(line); w != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			m[w] = true
		}
	}
	return m
}()

// Sensitive diz se algum dos nomes tem uma palavra do léxico de ações sensíveis.
func Sensitive(names ...string) bool {
	for _, n := range names {
		for _, t := range strings.Fields(Normalize(n)) {
			if lexicon[t] {
				return true
			}
		}
	}
	return false
}
