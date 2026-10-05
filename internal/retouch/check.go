package retouch

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// CheckCSS devolve avisos (para a IA repassar à pessoa) quando o CSS busca algo em outro host: um
// seletor de atributo com url() manda para fora o que a pessoa digita (docs/RETOQUES.md, 3.3.1).
// Referências relativas, ao próprio site (os hosts dos padrões) e data: não avisam.
func CheckCSS(css string, matches []string) []string {
	own := ownHosts(matches)
	var warnings []string
	seen := map[string]bool{}
	for _, ref := range cssRefs(unescapeCSS(stripComments(css))) {
		host, external := externalHost(ref)
		if !external || own[host] || seen[ref] {
			continue
		}
		seen[ref] = true
		warnings = append(warnings, fmt.Sprintf("CSS loads %q from another host (%s): a retouch must not fetch from other sites, because CSS can send what the person types. Remove it, or save with ignoreWarnings=true only if the person confirms", ref, host))
	}
	return warnings
}

// ownHosts são os hosts dos padrões do retoque (sem "*.").
func ownHosts(matches []string) map[string]bool {
	out := map[string]bool{}
	for _, m := range matches {
		_, rest, ok := strings.Cut(m, "://")
		if !ok {
			continue
		}
		host, _, _ := strings.Cut(rest, "/")
		host = strings.TrimPrefix(host, "*.")
		if host != "*" {
			out[strings.Trim(host, "[]")] = true
		}
	}
	return out
}

// externalHost diz se a referência é absoluta (com esquema ou "//") e devolve o host.
func externalHost(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	low := strings.ToLower(ref)
	if strings.HasPrefix(low, "data:") || strings.HasPrefix(low, "#") {
		return "", false
	}
	if strings.HasPrefix(ref, "//") {
		ref = "https:" + ref
	} else if !regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`).MatchString(ref) {
		return "", false // relativa: o próprio site
	}
	u, err := url.Parse(ref)
	if err != nil || u.Hostname() == "" {
		return ref, true // esquema estranho (javascript:, blob:…): avisa com a referência inteira
	}
	return strings.ToLower(u.Hostname()), true
}

var (
	urlFn    = regexp.MustCompile(`(?i)url\(\s*`)
	setFn    = regexp.MustCompile(`(?i)(-webkit-)?image-set\(`)
	importAt = regexp.MustCompile(`(?i)@import\s*`)
)

// cssRefs acha os endereços em url(), @import "..." e nas strings de image-set().
func cssRefs(css string) []string {
	var refs []string
	for _, loc := range urlFn.FindAllStringIndex(css, -1) {
		refs = append(refs, readArg(css[loc[1]:]))
	}
	for _, loc := range importAt.FindAllStringIndex(css, -1) {
		rest := css[loc[1]:]
		if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
			refs = append(refs, readArg(rest))
		}
	}
	for _, loc := range setFn.FindAllStringIndex(css, -1) {
		body := css[loc[1]:]
		if end := strings.IndexByte(body, ')'); end >= 0 {
			body = body[:end]
		}
		for _, m := range regexp.MustCompile(`"([^"]*)"|'([^']*)'`).FindAllStringSubmatch(body, -1) {
			refs = append(refs, m[1]+m[2])
		}
	}
	return refs
}

// readArg lê o argumento de url( ou de um @import: entre aspas ou até o ")".
func readArg(s string) string {
	s = strings.TrimLeft(s, " \t\n\r\f")
	if s == "" {
		return ""
	}
	if q := s[0]; q == '"' || q == '\'' {
		if end := strings.IndexByte(s[1:], q); end >= 0 {
			return s[1 : 1+end]
		}
		return s[1:]
	}
	if end := strings.IndexByte(s, ')'); end >= 0 {
		return strings.TrimSpace(s[:end])
	}
	return strings.TrimSpace(s)
}

func stripComments(css string) string {
	return regexp.MustCompile(`(?s)/\*.*?(\*/|$)`).ReplaceAllString(css, " ")
}

// unescapeCSS desfaz os escapes do CSS (\75 rl( é url(), para o aviso não ser driblado.
func unescapeCSS(css string) string {
	var b strings.Builder
	for i := 0; i < len(css); i++ {
		c := css[i]
		if c != '\\' || i+1 >= len(css) {
			b.WriteByte(c)
			continue
		}
		j := i + 1
		for j < len(css) && j-i <= 6 && isHex(css[j]) {
			j++
		}
		if j == i+1 { // \x vira x
			b.WriteByte(css[j])
			i = j
			continue
		}
		var n rune // até 6 dígitos hexadecimais: cabe num rune
		for _, h := range css[i+1 : j] {
			n = n*16 + hexValue(h)
		}
		b.WriteRune(n)
		if j < len(css) && (css[j] == ' ' || css[j] == '\t' || css[j] == '\n') {
			j++ // um espaço depois do escape faz parte dele
		}
		i = j - 1
	}
	return b.String()
}

func hexValue(h rune) rune {
	switch {
	case h >= 'a':
		return h - 'a' + 10
	case h >= 'A':
		return h - 'A' + 10
	}
	return h - '0'
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
