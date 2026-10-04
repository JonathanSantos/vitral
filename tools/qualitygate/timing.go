package main

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// TimingRule é um teto de tempo em milissegundos; o primeiro padrão (path.Match) que casar vale.
type TimingRule struct {
	Match string  `yaml:"match"`
	MaxMs float64 `yaml:"maxMs"`
}

// TimingRow é uma medição de tempo (mediana, em ms) comparada com o teto.
type TimingRow struct {
	Key     string
	Value   float64
	Max     float64
	OK      bool
	Problem string
}

// EvaluateTiming compara os tempos medidos no e2e com os tetos do thresholds.yaml.
func EvaluateTiming(rules []TimingRule, current map[string]float64) ([]TimingRow, []string) {
	keys := make([]string, 0, len(current))
	for k := range current {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var rows []TimingRow
	var failures []string
	for _, k := range keys {
		row := TimingRow{Key: k, Value: current[k], OK: true}
		rule, ok := matchTiming(rules, k)
		switch {
		case !ok:
			row.OK, row.Problem = false, "sem teto no thresholds.yaml"
		case row.Value > rule.MaxMs:
			row.Max = rule.MaxMs
			row.OK, row.Problem = false, fmt.Sprintf("%.1f ms > teto %.0f ms", row.Value, rule.MaxMs)
		default:
			row.Max = rule.MaxMs
		}
		if !row.OK {
			failures = append(failures, "tempo "+k+": "+row.Problem)
		}
		rows = append(rows, row)
	}
	return rows, failures
}

func matchTiming(rules []TimingRule, key string) (TimingRule, bool) {
	for _, r := range rules {
		if ok, _ := path.Match(r.Match, key); ok {
			return r, true
		}
	}
	return TimingRule{}, false
}

// FormatTiming mostra cada tempo com o teto.
func FormatTiming(rows []TimingRow) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-58s %9s %9s  %s\n", "tempo (mediana)", "atual", "teto", "status")
	for _, r := range rows {
		limit := "-"
		if r.Max > 0 {
			limit = fmt.Sprintf("%.0f ms", r.Max)
		}
		status := "ok"
		if !r.OK {
			status = "FALHA: " + r.Problem
		}
		fmt.Fprintf(&b, "%-58s %6.1f ms %9s  %s\n", r.Key, r.Value, limit, status)
	}
	return b.String()
}
