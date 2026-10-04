// Package flow define fluxos gravados: passos com seletores semânticos, parâmetros e arquivos YAML.
package flow

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Selector acha um elemento pelo papel e nome, nunca pela ref (que é volátil).
type Selector struct {
	Role   string   `yaml:"role"`
	Name   string   `yaml:"name"`
	Nth    int      `yaml:"nth,omitempty"`
	Within *Context `yaml:"within,omitempty"`
}

// Context restringe a busca a um ancestral (dialog, tabela, linha…).
type Context struct {
	Role string `yaml:"role"`
	Name string `yaml:"name"`
}

// TypeStep digita num campo.
type TypeStep struct {
	Selector `yaml:",inline"`
	Text     string `yaml:"text"`
	Clear    *bool  `yaml:"clear,omitempty"`
	Submit   bool   `yaml:"submit,omitempty"`
}

// SelectStep escolhe uma opção.
type SelectStep struct {
	Selector `yaml:",inline"`
	Option   string `yaml:"option"`
}

// ScrollStep rola até um elemento ou numa direção.
type ScrollStep struct {
	Selector  *Selector `yaml:"to,omitempty"`
	Direction string    `yaml:"direction,omitempty"`
	Amount    int       `yaml:"amount,omitempty"`
}

// WaitStep espera um texto ou elemento.
type WaitStep struct {
	Text      string    `yaml:"text,omitempty"`
	Element   *Selector `yaml:"element,omitempty"`
	Gone      bool      `yaml:"gone,omitempty"`
	TimeoutMs int       `yaml:"timeoutMs,omitempty"`
}

// MockStep reproduz um mock_add.
type MockStep struct {
	URLPattern string            `yaml:"urlPattern"`
	Method     string            `yaml:"method,omitempty"`
	Status     int               `yaml:"status,omitempty"`
	Body       string            `yaml:"body,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty"`
	Fail       string            `yaml:"fail,omitempty"`
	DelayMs    int               `yaml:"delayMs,omitempty"`
	Times      int               `yaml:"times,omitempty"`
}

// EmulateStep reproduz um emulate.
type EmulateStep struct {
	Device  string `yaml:"device,omitempty"`
	Network string `yaml:"network,omitempty"`
}

// Step tem exatamente um campo preenchido; a chave do YAML é o nome da ferramenta.
type Step struct {
	Open    string       `yaml:"open,omitempty"`
	Click   *Selector    `yaml:"click,omitempty"`
	Type    *TypeStep    `yaml:"type,omitempty"`
	Select  *SelectStep  `yaml:"select,omitempty"`
	Press   string       `yaml:"press,omitempty"`
	Scroll  *ScrollStep  `yaml:"scroll,omitempty"`
	Back    bool         `yaml:"back,omitempty"`
	WaitFor *WaitStep    `yaml:"wait_for,omitempty"`
	Mock    *MockStep    `yaml:"mock_add,omitempty"`
	Emulate *EmulateStep `yaml:"emulate,omitempty"`
}

// Flow é o conteúdo de um arquivo .yaml de fluxo.
type Flow struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Params      []string `yaml:"params,omitempty"`
	Steps       []Step   `yaml:"steps"`
}

// Kind devolve o nome da ação do passo (vazio se inválido).
func (s Step) Kind() string {
	kinds := s.kinds()
	if len(kinds) != 1 {
		return ""
	}
	return kinds[0]
}

func (s Step) kinds() []string {
	var k []string
	add := func(set bool, name string) {
		if set {
			k = append(k, name)
		}
	}
	add(s.Open != "", "open")
	add(s.Click != nil, "click")
	add(s.Type != nil, "type")
	add(s.Select != nil, "select")
	add(s.Press != "", "press")
	add(s.Scroll != nil, "scroll")
	add(s.Back, "back")
	add(s.WaitFor != nil, "wait_for")
	add(s.Mock != nil, "mock_add")
	add(s.Emulate != nil, "emulate")
	return k
}

// Validate confere o fluxo carregado de arquivo.
func Validate(f Flow) error {
	if err := ValidateName(f.Name); err != nil {
		return err
	}
	if len(f.Steps) == 0 {
		return errors.New("flow has no steps")
	}
	for i, s := range f.Steps {
		switch kinds := s.kinds(); len(kinds) {
		case 0:
			return fmt.Errorf("step %d is empty", i+1)
		case 1:
		default:
			return fmt.Errorf("step %d has more than one action: %s", i+1, strings.Join(kinds, ", "))
		}
	}
	return nil
}

// ValidateName aceita nomes que viram nomes de arquivo seguros.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("flow name is required")
	}
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("invalid flow name %q: use lowercase letters, digits, - and _", name)
		}
	}
	return nil
}

// Describe é a descrição curta do seletor: button "Filtrar" (2nd, in dialog "Confirmar").
func (s Selector) Describe() string {
	text := s.Role
	if s.Name != "" {
		text += " " + strconv.Quote(s.Name)
	}
	var extra []string
	if s.Nth > 1 {
		extra = append(extra, ordinal(s.Nth))
	}
	if s.Within != nil {
		extra = append(extra, fmt.Sprintf("in %s %q", s.Within.Role, s.Within.Name))
	}
	if len(extra) > 0 {
		text += " (" + strings.Join(extra, ", ") + ")"
	}
	return text
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return strconv.Itoa(n) + suffix
}

// Describe é a descrição curta do passo, usada nos relatórios.
func (s Step) Describe() string {
	switch s.Kind() {
	case "open":
		return "open " + s.Open
	case "click":
		return "click " + s.Click.Describe()
	case "type":
		line := fmt.Sprintf("type %s = %q", s.Type.Describe(), s.Type.Text)
		if s.Type.Submit {
			line += " + Enter"
		}
		return line
	case "select":
		return fmt.Sprintf("select %s = %q", s.Select.Describe(), s.Select.Option)
	case "press":
		return "press " + s.Press
	case "scroll":
		if s.Scroll.Selector != nil {
			return "scroll to " + s.Scroll.Selector.Describe()
		}
		return "scroll " + s.Scroll.Direction
	case "back":
		return "back"
	case "wait_for":
		return describeWait(*s.WaitFor)
	case "mock_add":
		return fmt.Sprintf("mock_add %s", s.Mock.URLPattern)
	case "emulate":
		return strings.TrimSpace(fmt.Sprintf("emulate %s %s", s.Emulate.Device, s.Emulate.Network))
	}
	return "invalid step"
}

func describeWait(w WaitStep) string {
	target := fmt.Sprintf("text %q", w.Text)
	if w.Element != nil {
		target = w.Element.Describe()
	}
	if w.Gone {
		return "wait_for " + target + " gone"
	}
	return "wait_for " + target
}
