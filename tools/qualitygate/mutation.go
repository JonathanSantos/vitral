package main

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// MutationConfig é a seção mutation do thresholds.yaml (docs/TESTING.md, seção 4).
type MutationConfig struct {
	Min                float64  `yaml:"min"`
	Packages           []string `yaml:"packages"`
	TimeoutCoefficient int      `yaml:"timeoutCoefficient"`
}

// Exclusion é um mutante equivalente ou inviável, com o motivo (lista versionada).
type Exclusion struct {
	File    string `yaml:"file"`
	Line    int    `yaml:"line"`
	Mutator string `yaml:"mutator"`
	Reason  string `yaml:"reason"`
}

// ParseExclusions lê .quality/mutation-exclusions.yaml; toda exclusão precisa de motivo.
func ParseExclusions(data []byte) ([]Exclusion, error) {
	var out []Exclusion
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("invalid mutation exclusions: %w", err)
	}
	for _, e := range out {
		if e.File == "" || e.Line == 0 || e.Mutator == "" || strings.TrimSpace(e.Reason) == "" {
			return nil, fmt.Errorf("mutation exclusion %s:%d %s needs file, line, mutator and reason", e.File, e.Line, e.Mutator)
		}
	}
	return out, nil
}

type gremlinsReport struct {
	Files []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Line   int    `json:"line"`
		} `json:"mutations"`
	} `json:"files"`
}

// MutationResult é a nota de um pacote.
type MutationResult struct {
	Package  string
	Killed   int
	Lived    int
	Excluded int
	Ignored  int // timeout, não coberto, inviável: fora da conta (docs/TESTING.md, seção 4)
	LivedAt  []string
}

// Score é killed / (killed + lived), em porcentagem; sem mutantes contáveis vale 100.
func (m MutationResult) Score() float64 {
	if m.Killed+m.Lived == 0 {
		return 100
	}
	return 100 * float64(m.Killed) / float64(m.Killed+m.Lived)
}

// ParseMutation lê o JSON do Gremlins de um pacote e aplica as exclusões.
func ParseMutation(pkg string, data []byte, exclusions []Exclusion) (MutationResult, error) {
	var rep gremlinsReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return MutationResult{}, fmt.Errorf("%s: invalid gremlins report: %w", pkg, err)
	}
	r := MutationResult{Package: pkg}
	for _, f := range rep.Files {
		file := path.Join(pkg, path.Base(f.FileName))
		for _, m := range f.Mutations {
			switch m.Status {
			case "KILLED":
				r.Killed++
			case "LIVED":
				if excluded(exclusions, file, m.Line, m.Type) {
					r.Excluded++
					continue
				}
				r.Lived++
				r.LivedAt = append(r.LivedAt, fmt.Sprintf("%s:%d %s", file, m.Line, m.Type))
			default:
				r.Ignored++
			}
		}
	}
	slices.Sort(r.LivedAt)
	return r, nil
}

func excluded(list []Exclusion, file string, line int, mutator string) bool {
	for _, e := range list {
		if e.File == file && e.Line == line && strings.EqualFold(e.Mutator, mutator) {
			return true
		}
	}
	return false
}

// ChangedPackages cruza os arquivos alterados com o escopo da mutação.
func ChangedPackages(files, scope []string) []string {
	var out []string
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") {
			continue
		}
		dir := path.Dir(f)
		if slices.Contains(scope, dir) && !slices.Contains(out, dir) {
			out = append(out, dir)
		}
	}
	slices.Sort(out)
	return out
}

// EvaluateMutation compara as notas com o mínimo.
func EvaluateMutation(results []MutationResult, minScore float64) []string {
	var failures []string
	for _, r := range results {
		if r.Score() < minScore {
			failures = append(failures, fmt.Sprintf("mutação %s: %.1f%% < mínimo %.0f%% (vivos: %s)", r.Package, r.Score(), minScore, strings.Join(r.LivedAt, ", ")))
		}
	}
	return failures
}

// FormatMutation mostra a nota de cada pacote.
func FormatMutation(results []MutationResult, minScore float64) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-26s %6s %6s %8s %9s %6s  %s\n", "mutação", "mortos", "vivos", "excluídos", "ignorados", "nota", "status")
	for _, r := range results {
		status := "ok"
		if r.Score() < minScore {
			status = "FALHA"
		}
		fmt.Fprintf(&b, "%-26s %6d %6d %8d %9d %5.1f%%  %s\n", r.Package, r.Killed, r.Lived, r.Excluded, r.Ignored, r.Score(), status)
	}
	return b.String()
}
