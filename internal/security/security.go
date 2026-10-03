// Package security reúne as regras de proteção: allowlist de hosts, glob e mascaramento de headers.
package security

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// MatchGlob casa s com um padrão onde * é qualquer sequência e ? é um caractere.
func MatchGlob(pattern, s string) bool {
	return globRegexp(pattern).MatchString(s)
}

func globRegexp(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// HostAllowed diz se o host da URL está na allowlist. Lista vazia libera tudo.
// URLs internas do browser (about:, data:, blob:) são sempre liberadas.
func HostAllowed(allowed []string, rawURL string) bool {
	if len(allowed) == 0 {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme == "about" || u.Scheme == "data" || u.Scheme == "blob" {
		return true
	}
	host := strings.ToLower(u.Hostname())
	for _, pattern := range allowed {
		if MatchGlob(strings.ToLower(pattern), host) {
			return true
		}
	}
	return false
}

// CheckNavigation devolve um erro acionável quando a URL não é permitida.
func CheckNavigation(allowed []string, rawURL string) error {
	if HostAllowed(allowed, rawURL) {
		return nil
	}
	return fmt.Errorf("host not allowed: %s. Allowed hosts: %s", rawURL, strings.Join(allowed, ", "))
}

// DefaultSensitiveHeaders são sempre mascarados.
var DefaultSensitiveHeaders = []string{
	"authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-auth-token", "x-csrf-token",
}

// Masked é o valor exibido no lugar de um header sensível.
const Masked = "***"

// MaskHeaders devolve uma cópia com os valores sensíveis trocados por ***.
func MaskHeaders(headers map[string]string, extra []string) map[string]string {
	sensitive := map[string]bool{}
	for _, h := range append(DefaultSensitiveHeaders, extra...) {
		sensitive[strings.ToLower(h)] = true
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if sensitive[strings.ToLower(k)] {
			out[k] = Masked
			continue
		}
		out[k] = v
	}
	return out
}

// NormalizeURL completa URLs digitadas sem esquema ("localhost:3000", "portal/pedidos").
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "://") || strings.HasPrefix(raw, "about:") || strings.HasPrefix(raw, "data:") {
		return raw
	}
	host := strings.ToLower(raw)
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "[::1]") {
		return "http://" + raw
	}
	return "https://" + raw
}
