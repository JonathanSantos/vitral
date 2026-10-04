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
			// Erro comum: nome e valor trocados ({Vitral: termo}). Se o nome é que aparece gravado, diz.
			if inValues(steps, name) {
				return nil, nil, fmt.Errorf("param %q (value %q) was not found in any value field, but %q was: name and value look swapped. Use params={%s: %q} (the param name first, the value typed in this session second)", name, params[name], name, params[name], name)
			}
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

func inValues(steps []Step, text string) bool {
	c := cloneSteps(steps)
	for i := range c {
		for _, f := range valueFields(&c[i]) {
			if strings.Contains(*f.ptr, text) {
				return true
			}
		}
	}
	return false
}

// FragileTargets avisa dos passos cujo alvo (nome do elemento) contém um valor que muda: o de um
// parâmetro ou um texto digitado num passo anterior (uma sugestão de busca, "Vitral vidraça
// decorativa", depende do que foi digitado). O seletor não é parametrizado, então com outro valor o
// fluxo procura um elemento que não existe. Recebe os passos antes da troca por {{nome}}.
func FragileTargets(steps []Step, params map[string]string) []string {
	type source struct{ value, label string }
	var sources []source
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if params[name] != "" {
			sources = append(sources, source{params[name], "the value of param " + name})
		}
	}
	var out []string
	for i, s := range steps {
		for _, sel := range s.selectors() {
			for _, src := range sources {
				v := strings.ToLower(src.value)
				if !strings.Contains(strings.ToLower(sel.Name), v) && (sel.Within == nil || !strings.Contains(strings.ToLower(sel.Within.Name), v)) {
					continue
				}
				out = append(out, fmt.Sprintf("step %d (%s) targets an element named with %s (%q), like a search suggestion: with another value it will not exist. Leave it out with excludeSteps; to finish a search, type the term again with submit=true (it presses Enter) and save that step instead.", i+1, s.Describe(), src.label, src.value))
				break
			}
		}
		if s.Type != nil && utf8.RuneCountInString(strings.TrimSpace(s.Type.Text)) >= MinParamLength {
			sources = append(sources, source{strings.TrimSpace(s.Type.Text), fmt.Sprintf("the text typed in step %d", i+1)})
		}
	}
	return out
}

// Warnings junta os avisos de um fluxo que o flow_run não conseguiria repetir com outros valores:
// alvos que dependem do valor (FragileTargets) e um fluxo que termina digitando sem enviar (a busca
// fica na caixa e nada acontece). Recebe os passos antes da troca por {{nome}}.
func Warnings(steps []Step, params map[string]string) []string {
	out := FragileTargets(steps, params)
	last := -1
	for i, s := range steps {
		if s.WaitFor == nil {
			last = i
		}
	}
	if last >= 0 && steps[last].Type != nil && !steps[last].Type.Submit {
		out = append(out, fmt.Sprintf("step %d (%s) is the last action and does not press Enter: the flow would type and stop, without searching or sending. Type it again with submit=true and save that step instead (or keep the click or key press that sent it).", last+1, steps[last].Describe()))
	}
	return out
}

// ExpectWarnings confere o expect do flow_save contra os parâmetros: um placeholder escrito à mão
// ({{termo}}) nunca aparece na página, e o valor com outra caixa ("O vitral" com termo=Vitral) não vira
// parâmetro, então o fluxo procuraria o texto da gravação com qualquer valor.
func ExpectWarnings(expect string, params map[string]string) []string {
	if strings.Contains(expect, "{{") {
		return []string{fmt.Sprintf("expect %q has a placeholder written by hand: write the text as it appears on the page now (with the value used in this session); params turn that value into the placeholder.", expect)}
	}
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}
	slices.Sort(names)
	var out []string
	for _, name := range names {
		v := params[name]
		if v != "" && !strings.Contains(expect, v) && strings.Contains(strings.ToLower(expect), strings.ToLower(v)) {
			out = append(out, fmt.Sprintf("expect %q has the value of param %s (%q) with different capitalization, so it does not become {{%s}} and flow_run would look for this same text with any value. Use text with the value spelled exactly %q.", expect, name, v, name, v))
		}
	}
	return out
}

// selectors devolve os alvos do passo.
func (s Step) selectors() []Selector {
	var out []Selector
	switch {
	case s.Click != nil:
		out = append(out, *s.Click)
	case s.Type != nil:
		out = append(out, s.Type.Selector)
	case s.Select != nil:
		out = append(out, s.Select.Selector)
	case s.Scroll != nil && s.Scroll.Selector != nil:
		out = append(out, *s.Scroll.Selector)
	case s.WaitFor != nil && s.WaitFor.Element != nil:
		out = append(out, *s.WaitFor.Element)
	}
	return out
}
