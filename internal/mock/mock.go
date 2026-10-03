// Package mock guarda regras de resposta falsa e as aplica via domínio Fetch do CDP.
package mock

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/go-rod/rod/lib/proto"

	"github.com/JonathanSantos/vitral/internal/security"
)

// Rule é uma regra de mock. Times 0 significa sem limite.
type Rule struct {
	ID         string
	URLPattern string
	Method     string
	Status     int
	Body       string
	Headers    map[string]string
	Fail       string
	DelayMs    int
	Times      int
	Used       int
}

// failReasons traduz nomes amigáveis para os motivos de falha do CDP.
var failReasons = map[string]proto.NetworkErrorReason{
	"timeout":               proto.NetworkErrorReasonTimedOut,
	"connection_refused":    proto.NetworkErrorReasonConnectionRefused,
	"connection_reset":      proto.NetworkErrorReasonConnectionReset,
	"connection_closed":     proto.NetworkErrorReasonConnectionClosed,
	"aborted":               proto.NetworkErrorReasonAborted,
	"dns":                   proto.NetworkErrorReasonNameNotResolved,
	"name_not_resolved":     proto.NetworkErrorReasonNameNotResolved,
	"offline":               proto.NetworkErrorReasonInternetDisconnected,
	"internet_disconnected": proto.NetworkErrorReasonInternetDisconnected,
	"access_denied":         proto.NetworkErrorReasonAccessDenied,
	"blocked":               proto.NetworkErrorReasonBlockedByClient,
	"failed":                proto.NetworkErrorReasonFailed,
}

// FailReason converte o nome amigável; devolve erro listando as opções válidas.
func FailReason(name string) (proto.NetworkErrorReason, error) {
	reason, ok := failReasons[strings.ToLower(strings.TrimSpace(name))]
	if ok {
		return reason, nil
	}
	names := make([]string, 0, len(failReasons))
	for k := range failReasons {
		names = append(names, k)
	}
	sort.Strings(names)
	return "", fmt.Errorf("unknown fail %q. Use one of: %s", name, strings.Join(names, ", "))
}

// Validate normaliza e confere uma regra nova.
func Validate(r Rule) (Rule, error) {
	r.URLPattern = strings.TrimSpace(r.URLPattern)
	r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
	if r.URLPattern == "" {
		return r, errors.New("urlPattern is required, e.g. */api/orders*")
	}
	if r.Fail != "" {
		if _, err := FailReason(r.Fail); err != nil {
			return r, err
		}
		r.Fail = strings.ToLower(strings.TrimSpace(r.Fail))
	}
	if r.Status == 0 {
		r.Status = 200
	}
	if r.Status < 100 || r.Status > 599 {
		return r, fmt.Errorf("invalid status %d", r.Status)
	}
	if r.DelayMs < 0 || r.DelayMs > 120000 {
		return r, errors.New("delayMs must be between 0 and 120000")
	}
	if r.Times < 0 {
		return r, errors.New("times must be 0 (unlimited) or positive")
	}
	return r, nil
}

// Matches diz se a regra vale para a request. Padrão sem curingas casa como substring.
func Matches(r Rule, url, method string) bool {
	if r.Method != "" && !strings.EqualFold(r.Method, method) {
		return false
	}
	if !strings.ContainsAny(r.URLPattern, "*?") {
		return strings.Contains(url, r.URLPattern)
	}
	return security.MatchGlob(r.URLPattern, url)
}

// FetchPattern converte a regra no padrão do Fetch.enable (que também usa * e ?).
func FetchPattern(r Rule) string {
	if strings.ContainsAny(r.URLPattern, "*?") {
		return r.URLPattern
	}
	return "*" + r.URLPattern + "*"
}

// ResponseHeaders monta os headers da resposta, inferindo o Content-Type quando ausente.
func ResponseHeaders(r Rule) []*proto.FetchHeaderEntry {
	headers := make([]*proto.FetchHeaderEntry, 0, len(r.Headers)+1)
	hasType := false
	keys := make([]string, 0, len(r.Headers))
	for k := range r.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		hasType = hasType || strings.EqualFold(k, "content-type")
		headers = append(headers, &proto.FetchHeaderEntry{Name: k, Value: r.Headers[k]})
	}
	if !hasType && r.Body != "" {
		contentType := "text/plain; charset=utf-8"
		if json.Valid([]byte(r.Body)) {
			contentType = "application/json"
		}
		headers = append(headers, &proto.FetchHeaderEntry{Name: "Content-Type", Value: contentType})
	}
	return headers
}

// Summary é a descrição curta da regra para mock_add e mock_list.
func Summary(r Rule) string {
	method := r.Method
	if method == "" {
		method = "ANY"
	}
	outcome := fmt.Sprintf("%d", r.Status)
	if r.Fail != "" {
		outcome = "fail " + r.Fail
	}
	used := fmt.Sprintf("used %d", r.Used)
	if r.Times > 0 {
		used = fmt.Sprintf("used %d/%d", r.Used, r.Times)
	}
	line := fmt.Sprintf("%s %s %s → %s (%s", r.ID, method, r.URLPattern, outcome, used)
	if r.DelayMs > 0 {
		line += fmt.Sprintf(", delay %dms", r.DelayMs)
	}
	return line + ")"
}

// Registry guarda as regras ativas. A mais recente tem prioridade.
type Registry struct {
	mu       sync.Mutex
	rules    []*Rule
	nextID   int
	allowed  []string
	onChange func()
}

// NewRegistry cria o registro; allowed restringe os hosts em que o mock se aplica.
func NewRegistry(allowed []string) *Registry {
	return &Registry{allowed: allowed}
}

// Add valida e registra uma regra.
func (r *Registry) Add(rule Rule) (Rule, error) {
	rule, err := Validate(rule)
	if err != nil {
		return rule, err
	}
	r.mu.Lock()
	r.nextID++
	rule.ID = fmt.Sprintf("m%d", r.nextID)
	rule.Used = 0
	r.rules = append(r.rules, &rule)
	r.mu.Unlock()
	r.changed()
	return rule, nil
}

// List devolve cópias das regras ativas, da mais nova para a mais antiga (ordem de avaliação).
func (r *Registry) List() []Rule {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Rule, 0, len(r.rules))
	for i := len(r.rules) - 1; i >= 0; i-- {
		out = append(out, *r.rules[i])
	}
	return out
}

// Clear remove uma regra (id) ou todas (id vazio). Devolve quantas removeu.
func (r *Registry) Clear(id string) (int, error) {
	r.mu.Lock()
	removed := 0
	if id == "" {
		removed = len(r.rules)
		r.rules = nil
	} else {
		before := len(r.rules)
		r.rules = slices.DeleteFunc(r.rules, func(x *Rule) bool { return x.ID == id })
		removed = before - len(r.rules)
	}
	r.mu.Unlock()
	if id != "" && removed == 0 {
		return 0, fmt.Errorf("mock %s not found. Call mock_list.", id)
	}
	r.changed()
	return removed, nil
}

// Take escolhe a regra para a request, conta o uso e remove regras que atingiram Times.
func (r *Registry) Take(url, method string) (Rule, bool) {
	if !security.HostAllowed(r.allowed, url) {
		return Rule{}, false
	}
	r.mu.Lock()
	var picked *Rule
	for i := len(r.rules) - 1; i >= 0; i-- {
		if Matches(*r.rules[i], url, method) {
			picked = r.rules[i]
			break
		}
	}
	if picked == nil {
		r.mu.Unlock()
		return Rule{}, false
	}
	picked.Used++
	if Expired(*picked) {
		r.rules = slices.DeleteFunc(r.rules, func(x *Rule) bool { return x == picked })
	}
	rule := *picked
	r.mu.Unlock()
	// Sem r.changed() aqui: desligar o Fetch agora liberaria a própria request pausada.
	// Quem chamou Take avisa depois de responder (ver Interceptor.handle).
	return rule, true
}

// Expired indica que a regra atingiu o limite de usos.
func Expired(r Rule) bool { return r.Times > 0 && r.Used >= r.Times }

// Notify avisa os interessados que as regras mudaram.
func (r *Registry) Notify() { r.changed() }

// Patterns devolve os padrões do Fetch.enable para as regras ativas.
func (r *Registry) Patterns() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, rule := range r.rules {
		p := FetchPattern(*rule)
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// OnChange registra quem precisa reagir a mudanças (ligar/desligar o Fetch na página).
func (r *Registry) OnChange(fn func()) { r.onChange = fn }

func (r *Registry) changed() {
	if r.onChange != nil {
		r.onChange()
	}
}
