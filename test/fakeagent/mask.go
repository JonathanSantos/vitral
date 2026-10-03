package fakeagent

import (
	"regexp"
	"strings"

	"github.com/JonathanSantos/vitral/internal/security"
)

var sensitiveLine = regexp.MustCompile(`(?im)^(\s*)(` + strings.Join(security.DefaultSensitiveHeaders, "|") + `)(\s*:\s*)(.+)$`)

// MaskTranscript esconde valores de headers sensíveis antes de salvar o transcript como artefato:
// nos argumentos (ex.: headers de um mock_add) e nas respostas (segunda barreira, além do servidor).
func MaskTranscript(in []Exchange) []Exchange {
	out := make([]Exchange, len(in))
	for i, e := range in {
		e.Args = maskMap(e.Args)
		e.Text = sensitiveLine.ReplaceAllString(e.Text, "${1}${2}${3}"+security.Masked)
		out[i] = e
	}
	return out
}

func maskMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			out[k] = maskMap(x)
		case string:
			if isSensitive(k) {
				out[k] = security.Masked
				continue
			}
			out[k] = x
		default:
			out[k] = v
		}
	}
	return out
}

func isSensitive(key string) bool {
	for _, h := range security.DefaultSensitiveHeaders {
		if strings.EqualFold(h, key) {
			return true
		}
	}
	return false
}
