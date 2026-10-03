package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/JonathanSantos/vitral/internal/security"
)

// Filter seleciona entradas em network_list.
type Filter struct {
	URL    string
	Method string
	Status string
	Type   string
	Limit  int
}

// Apply filtra e devolve as últimas Limit entradas, em ordem cronológica.
func Apply(entries []Entry, f Filter) []Entry {
	var out []Entry
	for _, e := range entries {
		if f.URL != "" && !strings.Contains(strings.ToLower(e.URL), strings.ToLower(f.URL)) {
			continue
		}
		if f.Method != "" && !strings.EqualFold(e.Method, f.Method) {
			continue
		}
		if f.Type != "" && !strings.EqualFold(e.Type, f.Type) {
			continue
		}
		if f.Status != "" && !MatchStatus(f.Status, e) {
			continue
		}
		out = append(out, e)
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[len(out)-f.Limit:]
	}
	return out
}

// MatchStatus aceita "2xx".."5xx", "failed", "error" (>= 400 ou falha), "pending" ou um código exato.
func MatchStatus(filter string, e Entry) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	switch filter {
	case "failed":
		return e.Failed != ""
	case "error", "errors":
		return e.Failed != "" || e.Status >= 400
	case "pending":
		return !e.Done
	}
	if len(filter) == 3 && strings.HasSuffix(filter, "xx") {
		return e.Status/100 == int(filter[0]-'0')
	}
	code, err := strconv.Atoi(filter)
	return err == nil && e.Status == code
}

// IsProblem indica requests que merecem aparecer no resumo de uma ação.
// O favicon é pedido pelo próprio browser e um 404 nele não interessa a ninguém.
func IsProblem(e Entry) bool {
	if IsNoise(e.URL) {
		return false
	}
	return e.Status >= 400 || (e.Failed != "" && e.Failed != "canceled")
}

// IsNoise identifica URLs pedidas automaticamente pelo browser.
func IsNoise(rawURL string) bool {
	path, _, _ := strings.Cut(rawURL, "?")
	return strings.HasSuffix(path, "/favicon.ico")
}

// Line é a linha compacta: id METHOD STATUS URL (ms, tamanho).
func Line(e Entry) string {
	status := strconv.Itoa(e.Status)
	switch {
	case e.Failed != "":
		status = "failed"
	case !e.Done && e.Status == 0:
		status = "pending"
	}
	var extra []string
	if e.Done {
		extra = append(extra, fmt.Sprintf("%dms", e.DurationMS()), HumanSize(e.Size))
	}
	if e.Failed != "" {
		extra = append(extra, e.Failed)
	}
	if e.Mocked {
		extra = append(extra, "mocked")
	}
	line := fmt.Sprintf("%d %s %s %s", e.ID, e.Method, status, shorten(e.URL, 150))
	if len(extra) > 0 {
		line += " (" + strings.Join(extra, ", ") + ")"
	}
	return line
}

// HumanSize formata bytes de forma curta.
func HumanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

func shorten(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max-1]) + "…"
}

// BodyLimit é o tamanho máximo de corpo exibido.
const BodyLimit = 8 * 1024

// FormatBody formata JSON de forma legível e trunca o resultado.
func FormatBody(raw []byte, mime string) string {
	if len(raw) == 0 {
		return "(empty)"
	}
	if !utf8.Valid(raw) {
		return fmt.Sprintf("(binary %s, %s)", mime, HumanSize(int64(len(raw))))
	}
	text := raw
	var pretty bytes.Buffer
	if json.Valid(raw) && json.Indent(&pretty, raw, "", "  ") == nil {
		text = pretty.Bytes()
	}
	if len(text) <= BodyLimit {
		return string(text)
	}
	cut := BodyLimit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return fmt.Sprintf("%s\n… (truncated, %s total)", text[:cut], HumanSize(int64(len(text))))
}

// Detail monta a saída de network_get. body pode ser vazio quando não foi pedido.
func Detail(e Entry, body string, maskExtra []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", e.Method, e.URL)
	fmt.Fprintf(&b, "status: %s\n", strings.TrimPrefix(Line(e), fmt.Sprintf("%d %s ", e.ID, e.Method)))
	if e.Type != "" {
		fmt.Fprintf(&b, "type: %s\n", strings.ToLower(e.Type))
	}
	writeHeaders(&b, "request headers", security.MaskHeaders(e.ReqHeaders, maskExtra))
	if e.PostData != "" {
		b.WriteString("request body:\n" + FormatBody([]byte(e.PostData), "") + "\n")
	}
	if e.PostData == "" && e.HasPostData {
		b.WriteString("request body: (not captured)\n")
	}
	writeHeaders(&b, "response headers", security.MaskHeaders(e.RespHeaders, maskExtra))
	if body != "" {
		fmt.Fprintf(&b, "response body (%s):\n%s\n", e.MIMEType, body)
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeHeaders(b *strings.Builder, title string, headers map[string]string) {
	if len(headers) == 0 {
		return
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	b.WriteString(title + ":\n")
	for _, k := range keys {
		fmt.Fprintf(b, "  %s: %s\n", k, shorten(headers[k], 200))
	}
}
