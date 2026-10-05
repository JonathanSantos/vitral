// Package retouch guarda e compila os Retoques (docs/RETOQUES.md): CSS que muda como um site se
// parece, só neste navegador. Cada retoque é uma pasta em ~/.vitral/retoques/<nome> (retoque.yaml e
// style.css); todos juntos viram uma extensão MV3 gerada (Compile), carregada em todo lançamento.
// Puro: sem navegador.
package retouch

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Scope é onde um retoque vale.
type Scope string

const (
	ScopePage Scope = "page" // a página (o caminho, com qualquer query)
	ScopeSite Scope = "site" // o site inteiro (o host, em http e https)
	ScopeAll  Scope = "all"  // todos os sites
)

// Retouch é um retoque salvo. CSS fica em style.css, fora do YAML.
type Retouch struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Scope       Scope    `yaml:"scope"`
	Matches     []string `yaml:"matches"` // padrões de URL do Chrome (match patterns)
	CSS         string   `yaml:"-"`
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// ValidName aceita nomes que viram pasta e nome de arquivo seguros (o mesmo padrão dos perfis).
func ValidName(name string) bool { return validName.MatchString(name) }

// Validate confere o que é gravado e compilado.
func (r Retouch) Validate() error {
	if !ValidName(r.Name) {
		return fmt.Errorf("invalid retouch name %q: use lowercase letters, digits, - and _ (up to 40), starting with a letter or digit", r.Name)
	}
	if strings.TrimSpace(r.CSS) == "" {
		return fmt.Errorf("retouch %q has no CSS", r.Name)
	}
	if len(r.Matches) == 0 {
		return fmt.Errorf("retouch %q has no URL patterns", r.Name)
	}
	for _, m := range r.Matches {
		if _, err := compilePattern(m); err != nil {
			return fmt.Errorf("retouch %q: %w", r.Name, err)
		}
	}
	return nil
}

// MatchesFor converte o escopo na lista de padrões a partir da página atual. Portas não entram: no
// Chrome, um padrão sem porta vale para todas as portas do host.
func MatchesFor(scope Scope, pageURL string) ([]string, error) {
	if scope == ScopeAll {
		return []string{"<all_urls>"}, nil
	}
	u, err := url.Parse(pageURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, fmt.Errorf("retouches work on http and https pages; the current page is %q. Open the site first", pageURL)
	}
	host := u.Hostname()
	if strings.Contains(host, ":") { // IPv6: o padrão usa colchetes
		host = "[" + host + "]"
	}
	switch scope {
	case ScopeSite, "":
		return []string{"*://" + host + "/*"}, nil
	case ScopePage:
		path := u.EscapedPath()
		if path == "" {
			path = "/"
		}
		return []string{"*://" + host + path, "*://" + host + path + "?*"}, nil
	}
	return nil, fmt.Errorf("invalid scope %q: use page, site or all", scope)
}

// Applies diz se o retoque vale numa URL (para listar os retoques da página atual).
func (r Retouch) Applies(pageURL string) bool {
	for _, m := range r.Matches {
		if re, err := compilePattern(m); err == nil && re.MatchString(stripPort(pageURL)) {
			return true
		}
	}
	return false
}

// compilePattern traduz um match pattern do Chrome (<all_urls>, scheme://host/path com *) numa
// expressão regular sobre a URL sem porta.
func compilePattern(p string) (*regexp.Regexp, error) {
	if p == "<all_urls>" {
		return regexp.MustCompile(`^(https?|file|ftp)://`), nil
	}
	scheme, rest, ok := strings.Cut(p, "://")
	host, path, okPath := strings.Cut(rest, "/")
	if !ok || !okPath || host == "" || (scheme != "*" && scheme != "http" && scheme != "https") {
		return nil, fmt.Errorf("invalid URL pattern %q: use <all_urls> or scheme://host/path", p)
	}
	if (strings.Contains(host, ":") && !strings.HasPrefix(host, "[")) || (strings.Contains(host, "*") && host != "*" && !strings.HasPrefix(host, "*.")) ||
		strings.Count(host, "*") > 1 {
		return nil, fmt.Errorf("invalid host in URL pattern %q: use host, *.host or *", p)
	}
	var b strings.Builder
	b.WriteString("^")
	if scheme == "*" {
		b.WriteString("https?")
	} else {
		b.WriteString(regexp.QuoteMeta(scheme))
	}
	b.WriteString("://")
	switch {
	case host == "*":
		b.WriteString(`[^/]+`)
	case strings.HasPrefix(host, "*."):
		b.WriteString(`([^/]+\.)?` + regexp.QuoteMeta(host[2:]))
	default:
		b.WriteString(regexp.QuoteMeta(host))
	}
	b.WriteString("/")
	for i, part := range strings.Split(path, "*") {
		if i > 0 {
			b.WriteString(".*")
		}
		b.WriteString(regexp.QuoteMeta(part))
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// stripPort tira a porta da URL, como o Chrome faz ao comparar com um padrão sem porta.
func stripPort(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Port() == "" {
		return raw
	}
	u.Host = u.Hostname()
	if strings.Contains(u.Host, ":") {
		u.Host = "[" + u.Host + "]"
	}
	return u.String()
}
