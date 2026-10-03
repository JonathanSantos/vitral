package flow

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// MinParamLength evita trocar valores curtos demais (ex.: "1" apareceria em todo lugar).
const MinParamLength = 3

// Substitution registra uma troca feita pelo flow_save, para conferência.
type Substitution struct {
	Step   int
	Field  string
	Before string
	After  string
}

func (s Substitution) String() string {
	return fmt.Sprintf("step %d %s: %q → %q", s.Step, s.Field, s.Before, s.After)
}

// valueField é um campo de valor onde parâmetros podem entrar. Seletores nunca entram.
type valueField struct {
	name string
	ptr  *string
}

func valueFields(s *Step) []valueField {
	var out []valueField
	if s.Open != "" {
		out = append(out, valueField{"open.url", &s.Open})
	}
	if s.Type != nil {
		out = append(out, valueField{"type.text", &s.Type.Text})
	}
	if s.Select != nil {
		out = append(out, valueField{"select.option", &s.Select.Option})
	}
	if s.WaitFor != nil && s.WaitFor.Text != "" {
		out = append(out, valueField{"wait_for.text", &s.WaitFor.Text})
	}
	return out
}

func cloneSteps(steps []Step) []Step {
	out := make([]Step, len(steps))
	for i, s := range steps {
		c := s
		if s.Type != nil {
			t := *s.Type
			c.Type = &t
		}
		if s.Select != nil {
			v := *s.Select
			c.Select = &v
		}
		if s.WaitFor != nil {
			w := *s.WaitFor
			c.WaitFor = &w
		}
		out[i] = c
	}
	return out
}

// Parameterize troca valores gravados por {{nome}} só nos campos de valor e lista cada troca.
func Parameterize(steps []Step, params map[string]string) ([]Step, []Substitution, error) {
	names := make([]string, 0, len(params))
	for name, value := range params {
		if !paramName.MatchString(name) {
			return nil, nil, fmt.Errorf("invalid param name %q: use letters, digits and _", name)
		}
		if utf8.RuneCountInString(value) < MinParamLength {
			return nil, nil, fmt.Errorf("param %q value %q is too short: use at least %d characters so it is not replaced by accident", name, value, MinParamLength)
		}
		names = append(names, name)
	}
	// Valores mais longos primeiro: "10234" não pode ser estragado por um parâmetro "1023".
	slices.SortFunc(names, func(a, b string) int {
		if d := len(params[b]) - len(params[a]); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})

	out := cloneSteps(steps)
	var subs []Substitution
	used := map[string]bool{}
	for i := range out {
		for _, f := range valueFields(&out[i]) {
			before := *f.ptr
			after := before
			for _, name := range names {
				if strings.Contains(after, params[name]) {
					after = strings.ReplaceAll(after, params[name], "{{"+name+"}}")
					used[name] = true
				}
			}
			if after != before {
				*f.ptr = after
				subs = append(subs, Substitution{Step: i + 1, Field: f.name, Before: before, After: after})
			}
		}
	}
	for _, name := range names {
		if !used[name] {
			return nil, nil, fmt.Errorf("param %q (value %q) was not found in any value field (open url, type text, select option, wait_for text)", name, params[name])
		}
	}
	return out, subs, nil
}

var (
	paramName   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
)

// Bind confere os parâmetros de execução e devolve os passos com os valores aplicados.
func Bind(f Flow, values map[string]string) ([]Step, error) {
	var missing []string
	for _, p := range f.Params {
		if _, ok := values[p]; !ok {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing params: %s. This flow needs: %s", strings.Join(missing, ", "), strings.Join(f.Params, ", "))
	}
	for name := range values {
		if !slices.Contains(f.Params, name) {
			return nil, fmt.Errorf("unknown param %q. This flow accepts: %s", name, strings.Join(f.Params, ", "))
		}
	}
	out := cloneSteps(f.Steps)
	for i := range out {
		for _, field := range valueFields(&out[i]) {
			var unknown string
			*field.ptr = placeholder.ReplaceAllStringFunc(*field.ptr, func(m string) string {
				name := placeholder.FindStringSubmatch(m)[1]
				v, ok := values[name]
				if !ok {
					unknown = name
					return m
				}
				return v
			})
			if unknown != "" {
				return nil, fmt.Errorf("step %d uses {{%s}}, which is not declared in params", i+1, unknown)
			}
		}
	}
	return out, nil
}
