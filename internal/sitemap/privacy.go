package sitemap

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Privacidade (pedido 5.5): o mapa guarda estrutura, não dados. Nomes de controles, headings,
// landmarks e cabeçalhos passam por Sanitize; valores de campos, células, parágrafos e valores de
// query nunca entram.

var (
	reEmail = regexp.MustCompile(`[\p{L}\p{N}._%+-]+@[\p{L}\p{N}.-]+\.[\p{L}]{2,}`)
	reCNPJ  = regexp.MustCompile(`\b\d{2}\.?\d{3}\.?\d{3}/?\d{4}-?\d{2}\b`)
	reCPF   = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)
	rePhone = regexp.MustCompile(`(\+?\d{1,3}[\s-]?)?\(?\d{2}\)?[\s-]?9?\d{4}[\s-]?\d{4}\b`)
	reDigit = regexp.MustCompile(`\d{4,}`)
	reSpace = regexp.MustCompile(`\s+`)
)

// Marcadores no lugar dos dados.
const (
	MarkEmail = "<email>"
	MarkCNPJ  = "<cnpj>"
	MarkCPF   = "<cpf>"
	MarkPhone = "<telefone>"
	MarkNum   = "<num>"
)

// Sanitize troca e-mails, CNPJ, CPF, telefones e sequências de 4 ou mais dígitos por marcadores
// (nesta ordem: o mais específico primeiro) e colapsa espaços.
func Sanitize(s string) string {
	s = reEmail.ReplaceAllString(s, MarkEmail)
	s = reCNPJ.ReplaceAllString(s, MarkCNPJ)
	s = reCPF.ReplaceAllString(s, MarkCPF)
	s = rePhone.ReplaceAllString(s, MarkPhone)
	s = reDigit.ReplaceAllString(s, MarkNum)
	return strings.TrimSpace(reSpace.ReplaceAllString(s, " "))
}

var (
	reNumericSeg = regexp.MustCompile(`^\d+$`)
	reUUID       = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reHexSeg     = regexp.MustCompile(`(?i)^[0-9a-f]{8,}$`)
	reTokenSeg   = regexp.MustCompile(`^[A-Za-z0-9_-]{16,}$`)
	reHasDigit   = regexp.MustCompile(`\d`)
)

// idLike reconhece segmentos de caminho que identificam um registro: números, UUID, hashes e tokens
// longos com dígitos (e segmentos com dados pessoais, como um e-mail).
func idLike(seg string) bool {
	switch {
	case reNumericSeg.MatchString(seg), reUUID.MatchString(seg), reHexSeg.MatchString(seg):
		return true
	case reTokenSeg.MatchString(seg) && reHasDigit.MatchString(seg):
		return true
	}
	return Sanitize(seg) != seg
}

// NormalizeURL devolve o host (com porta) e o padrão da URL: segmentos que parecem ID viram :id, a
// query fica só com as chaves (ordenadas) e o fragmento só vale como rota de SPA ("#/pedidos/:id").
func NormalizeURL(raw string) (host, pattern string) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ""
	}
	pattern = normalizePath(u.EscapedPath())
	if keys := queryKeys(u.RawQuery); len(keys) > 0 {
		pattern += "?" + strings.Join(keys, "&")
	}
	if f := u.Fragment; strings.HasPrefix(f, "/") || strings.HasPrefix(f, "!/") {
		route, query, _ := strings.Cut(f, "?")
		pattern += "#" + normalizePath(route)
		if keys := queryKeys(query); len(keys) > 0 {
			pattern += "?" + strings.Join(keys, "&")
		}
	}
	return strings.ToLower(u.Host), pattern
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		dec, err := url.PathUnescape(s)
		if err != nil {
			dec = s
		}
		if dec != "" && idLike(dec) {
			segs[i] = ":id"
		}
	}
	return strings.Join(segs, "/")
}

// queryKeys são as chaves da query, sem valores, ordenadas e sem repetir.
func queryKeys(q string) []string {
	vals, err := url.ParseQuery(q)
	if err != nil {
		return nil
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		if k != "" {
			keys = append(keys, Sanitize(k))
		}
	}
	sort.Strings(keys)
	return keys
}
