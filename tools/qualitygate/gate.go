package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Row é uma linha da tabela do gate.
type Row struct {
	Package   string
	Unit      *float64
	Combined  *float64
	Threshold PackageThreshold
	OK        bool
	Problem   string
}

// Report é o resultado do gate.
type Report struct {
	Rows        []Row
	Tokens      []TokenRow
	Quarantines []Quarantine
	Failures    []string
}

// OK indica gate verde.
func (r Report) OK() bool { return len(r.Failures) == 0 }

// Input reúne tudo o que o gate avalia.
type Input struct {
	Thresholds Thresholds
	Packages   []string            // pacotes do módulo (relativos), ex.: internal/flow
	Unit       map[string]Coverage // nil quando não medido
	Combined   map[string]Coverage // nil quando não medido (ex.: check-fast)
	Quarantine []Quarantine
	Now        time.Time
	TokenNow   map[string]int // nil quando não medido
	TokenBase  map[string]int
}

func pct(m map[string]Coverage, pkg string) *float64 {
	if m == nil {
		return nil
	}
	c, ok := m[pkg]
	if !ok {
		zero := 0.0
		return &zero
	}
	v := c.Percent()
	return &v
}

// Evaluate compara a cobertura com os limiares. Função pura: tudo vem do Input.
func Evaluate(in Input) Report {
	var r Report
	pkgs := slices.Clone(in.Packages)
	slices.Sort(pkgs)
	for _, pkg := range pkgs {
		th, ok := in.Thresholds.Coverage[pkg]
		row := Row{Package: pkg, Unit: pct(in.Unit, pkg), Combined: pct(in.Combined, pkg), Threshold: th, OK: true}
		switch {
		case !ok && strings.HasPrefix(pkg, "internal/"):
			row.OK, row.Problem = false, "sem entrada no thresholds.yaml"
		case !ok || th.Criterion == CriterionNone:
		default:
			value := row.Unit
			if th.Criterion == CriterionCombined {
				value = row.Combined
			}
			if value == nil {
				row.Problem = "não medido nesta execução"
				break
			}
			if *value < th.Min {
				row.OK, row.Problem = false, fmt.Sprintf("%.1f%% < mínimo %.1f%%", *value, th.Min)
			}
		}
		if !row.OK {
			r.Failures = append(r.Failures, pkg+": "+row.Problem)
		}
		r.Rows = append(r.Rows, row)
	}
	for pkg := range in.Thresholds.Coverage {
		if !slices.Contains(pkgs, pkg) {
			r.Failures = append(r.Failures, pkg+": está no thresholds.yaml mas o pacote não existe")
		}
	}
	if in.TokenNow != nil {
		var failures []string
		r.Tokens, failures = EvaluateTokens(in.Thresholds.Tokens, in.TokenNow, in.TokenBase)
		r.Failures = append(r.Failures, failures...)
	}
	slices.Sort(r.Failures)
	for _, q := range in.Quarantine {
		r.Quarantines = append(r.Quarantines, q)
		if in.Now.After(q.Until.Add(24 * time.Hour)) {
			r.Failures = append(r.Failures, fmt.Sprintf("quarentena vencida em %s:%d (%s, prazo %s)", q.File, q.Line, q.Reason, q.Until.Format("2006-01-02")))
		}
	}
	return r
}

// Format gera a tabela legível do relatório.
func Format(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-26s %9s %10s  %-9s %6s %6s  %s\n", "pacote", "unitária", "combinada", "critério", "mínimo", "meta", "status")
	for _, row := range r.Rows {
		crit, minimum, target := row.Threshold.Criterion, "-", "-"
		if crit == "" {
			crit = "-"
		}
		if row.Threshold.Min > 0 {
			minimum = fmt.Sprintf("%.0f", row.Threshold.Min)
		}
		if row.Threshold.Target > 0 {
			target = fmt.Sprintf("%.0f", row.Threshold.Target)
		}
		status := "ok"
		if !row.OK {
			status = "FALHA: " + row.Problem
		} else if row.Problem != "" {
			status = "ok (" + row.Problem + ")"
		}
		fmt.Fprintf(&b, "%-26s %9s %10s  %-9s %6s %6s  %s\n", row.Package, show(row.Unit), show(row.Combined), crit, minimum, target, status)
	}
	b.WriteString(FormatTokens(r.Tokens))
	if len(r.Quarantines) > 0 {
		b.WriteString("\nquarentena:\n")
		for _, q := range r.Quarantines {
			fmt.Fprintf(&b, "  %s:%d  %s (até %s)\n", q.File, q.Line, q.Reason, q.Until.Format("2006-01-02"))
		}
	}
	if r.OK() {
		b.WriteString("\ngate: OK\n")
		return b.String()
	}
	b.WriteString("\ngate: FALHOU\n")
	for _, f := range r.Failures {
		b.WriteString("  - " + f + "\n")
	}
	return b.String()
}

func show(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", *v)
}
