package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Critérios de cobertura (docs/TESTING.md, seção 3).
const (
	CriterionUnit     = "unit"
	CriterionCombined = "combined"
	CriterionNone     = "none"
)

// PackageThreshold é a regra de cobertura de um pacote.
type PackageThreshold struct {
	Criterion string  `yaml:"criterion"`
	Min       float64 `yaml:"min"`
	Target    float64 `yaml:"target"`
	Note      string  `yaml:"note"`
}

// Thresholds é o conteúdo de .quality/thresholds.yaml.
type Thresholds struct {
	Module   string                      `yaml:"module"`
	Coverage map[string]PackageThreshold `yaml:"coverage"`
	Tokens   []TokenRule                 `yaml:"tokens"`
	Timing   []TimingRule                `yaml:"timing"`
	Mutation MutationConfig              `yaml:"mutation"`
}

// ParseThresholds lê e valida o arquivo de limiares.
func ParseThresholds(data []byte) (Thresholds, error) {
	var t Thresholds
	if err := yaml.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("invalid thresholds: %w", err)
	}
	if t.Module == "" {
		return t, fmt.Errorf("thresholds: module is required")
	}
	for pkg, th := range t.Coverage {
		switch th.Criterion {
		case CriterionUnit, CriterionCombined, CriterionNone:
		default:
			return t, fmt.Errorf("thresholds: %s has invalid criterion %q (use unit, combined or none)", pkg, th.Criterion)
		}
		if th.Criterion != CriterionNone && (th.Min <= 0 || th.Min > 100) {
			return t, fmt.Errorf("thresholds: %s needs min between 0 and 100", pkg)
		}
		if th.Target != 0 && th.Target < th.Min {
			return t, fmt.Errorf("thresholds: %s target %.1f is below min %.1f", pkg, th.Target, th.Min)
		}
	}
	for _, r := range t.Tokens {
		if r.Match == "" || r.Max <= 0 {
			return t, fmt.Errorf("thresholds: token rule %q needs match and max > 0", r.Match)
		}
	}
	for _, r := range t.Timing {
		if r.Match == "" || r.MaxMs <= 0 {
			return t, fmt.Errorf("thresholds: timing rule %q needs match and maxMs > 0", r.Match)
		}
	}
	return t, nil
}
