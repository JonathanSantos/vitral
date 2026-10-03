package main

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// TokenRule é um teto de tokens; o primeiro padrão (path.Match) que casar com a chave vale.
type TokenRule struct {
	Match string `yaml:"match"`
	Max   int    `yaml:"max"`
}

// TokenRow é uma medição comparada com o teto e com a base versionada.
type TokenRow struct {
	Key     string
	Value   int
	Base    int
	HasBase bool
	Max     int
	OK      bool
	Problem string
}

// EvaluateTokens compara as medições com os tetos e com a base (docs/TESTING.md, seção 6).
func EvaluateTokens(rules []TokenRule, current, base map[string]int) ([]TokenRow, []string) {
	keys := make([]string, 0, len(current))
	for k := range current {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var rows []TokenRow
	var failures []string
	for _, k := range keys {
		row := TokenRow{Key: k, Value: current[k], OK: true}
		row.Base, row.HasBase = base[k]
		rule, ok := matchRule(rules, k)
		switch {
		case !ok:
			row.OK, row.Problem = false, "sem teto no thresholds.yaml"
		case row.Value > rule.Max:
			row.Max = rule.Max
			row.OK, row.Problem = false, fmt.Sprintf("%d > teto %d", row.Value, rule.Max)
		default:
			row.Max = rule.Max
		}
		if !row.OK {
			failures = append(failures, "tokens "+k+": "+row.Problem)
		}
		rows = append(rows, row)
	}
	return rows, failures
}

func matchRule(rules []TokenRule, key string) (TokenRule, bool) {
	for _, r := range rules {
		if ok, _ := path.Match(r.Match, key); ok {
			return r, true
		}
	}
	return TokenRule{}, false
}

// FormatTokens mostra valor, base, diferença e teto de cada medição.
func FormatTokens(rows []TokenRow) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-58s %6s %6s %6s %6s  %s\n", "tokens", "atual", "base", "dif", "teto", "status")
	for _, r := range rows {
		base, diff := "-", "-"
		if r.HasBase {
			base, diff = fmt.Sprint(r.Base), fmt.Sprintf("%+d", r.Value-r.Base)
		}
		limit := "-"
		if r.Max > 0 {
			limit = fmt.Sprint(r.Max)
		}
		status := "ok"
		if !r.OK {
			status = "FALHA: " + r.Problem
		}
		fmt.Fprintf(&b, "%-58s %6d %6s %6s %6s  %s\n", r.Key, r.Value, base, diff, limit, status)
	}
	return b.String()
}
