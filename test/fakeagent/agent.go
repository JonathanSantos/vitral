package fakeagent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/JonathanSantos/vitral/test/tokens"
)

// Caller é como o agente fala com o Vitral (no e2e, um cliente MCP real por stdio).
type Caller interface {
	Call(ctx context.Context, tool string, args map[string]any) (text string, isError bool, err error)
}

// Field é um campo a preencher, na ordem em que aparece no objetivo.
type Field struct {
	Label string
	Value string
}

// Fields preserva a ordem do mapa YAML (mapas do Go não têm ordem e o teste precisa ser determinístico).
type Fields []Field

// UnmarshalYAML lê `fill: {Nome: Maria, E-mail: x}` mantendo a ordem.
func (f *Fields) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return errors.New("fill must be a mapping of label: value")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		*f = append(*f, Field{Label: n.Content[i].Value, Value: n.Content[i+1].Value})
	}
	return nil
}

// Goal é o objetivo que o agente persegue (docs/TESTING.md, seção 7.3).
type Goal struct {
	Open  string `yaml:"open"`
	Fill  Fields `yaml:"fill"`
	Click string `yaml:"click"`
	Until struct {
		Text string `yaml:"text"`
	} `yaml:"until"`
}

// ParseGoal lê o objetivo em YAML (`goal: {...}` ou direto os campos).
func ParseGoal(data []byte) (Goal, error) {
	var wrapper struct {
		Goal *Goal `yaml:"goal"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err == nil && wrapper.Goal != nil {
		return *wrapper.Goal, nil
	}
	var g Goal
	err := yaml.Unmarshal(data, &g)
	return g, err
}

// Outcome é como o objetivo terminou.
type Outcome string

const (
	Done         Outcome = "done"           // chegou ao `until`
	HandedToUser Outcome = "handed_to_user" // o Vitral pausou e pediu ação da pessoa
)

// Exchange é uma chamada e sua resposta, para o transcript.
type Exchange struct {
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Text    string         `json:"text"`
	IsError bool           `json:"isError"`
}

// Result resume a execução.
type Result struct {
	Outcome    Outcome
	Steps      int
	Tokens     int
	Transcript []Exchange
	LastText   string
}

// Agent executa um objetivo.
type Agent struct {
	caller   Caller
	maxSteps int
	view     *View
	result   Result
	// AfterCall roda depois de cada chamada (o e2e usa para mudar a página "por fora" do agente).
	AfterCall func(step int, tool string)
}

// New cria um agente com limite de passos.
func New(c Caller, maxSteps int) *Agent {
	return &Agent{caller: c, maxSteps: maxSteps, view: NewView()}
}

// errHandedToUser encerra o objetivo quando o Vitral entrega a vez para a pessoa.
var errHandedToUser = errors.New("handed to user")

// Run persegue o objetivo. Erro = o agente não conseguiu (com o motivo).
func (a *Agent) Run(ctx context.Context, g Goal) (Result, error) {
	err := a.run(ctx, g)
	if errors.Is(err, errHandedToUser) {
		a.result.Outcome = HandedToUser
		return a.result, nil
	}
	if err != nil {
		return a.result, err
	}
	a.result.Outcome = Done
	return a.result, nil
}

func (a *Agent) run(ctx context.Context, g Goal) error {
	if g.Open != "" {
		if err := a.act(ctx, "open", func() (map[string]any, error) { return map[string]any{"url": g.Open}, nil }); err != nil {
			return err
		}
	}
	for _, f := range g.Fill {
		if err := a.fill(ctx, f); err != nil {
			return err
		}
	}
	if g.Click != "" {
		err := a.act(ctx, "click", func() (map[string]any, error) {
			el, err := a.locate(ctx, g.Click, "button", "link", "checkbox", "radio", "tab", "menuitem", "clickable")
			if err != nil {
				return nil, err
			}
			return map[string]any{"ref": el.Ref}, nil
		})
		if err != nil {
			return err
		}
	}
	if g.Until.Text != "" {
		return a.act(ctx, "wait_for", func() (map[string]any, error) { return map[string]any{"text": g.Until.Text}, nil })
	}
	return nil
}

var fieldRoles = []string{"textbox", "searchbox", "combobox", "listbox", "spinbutton", "checkbox", "radio", "switch", "slider"}

func (a *Agent) fill(ctx context.Context, f Field) error {
	return a.act(ctx, "", func() (map[string]any, error) {
		el, err := a.locate(ctx, f.Label, fieldRoles...)
		if err != nil {
			return nil, err
		}
		switch el.Role {
		case "combobox", "listbox":
			return map[string]any{"ref": el.Ref, "option": f.Value, "__tool": "select"}, nil
		case "checkbox", "radio", "switch":
			return map[string]any{"ref": el.Ref, "__tool": "click"}, nil
		}
		return map[string]any{"ref": el.Ref, "text": f.Value, "__tool": "type"}, nil
	})
}

// locate acha o elemento na visão; se não estiver lá, pede um snapshot (e depois o completo).
func (a *Agent) locate(ctx context.Context, name string, roles ...string) (*Element, error) {
	if el := a.view.Find(name, roles...); el != nil {
		return el, nil
	}
	for _, args := range []map[string]any{{}, {"full": true}} {
		if _, _, err := a.call(ctx, "snapshot", args); err != nil {
			return nil, err
		}
		if el := a.view.Find(name, roles...); el != nil {
			return el, nil
		}
	}
	return nil, fmt.Errorf("element %q (%s) not found on the page", name, strings.Join(roles, "/"))
}

var coveredRe = regexp.MustCompile(`covered by (\S+) (e\d+)`)

// act chama a ferramenta e, em erro, faz só o que a mensagem manda. Mensagem sem instrução reprova.
func (a *Agent) act(ctx context.Context, tool string, args func() (map[string]any, error)) error {
	for attempt := 0; attempt < 3; attempt++ {
		params, err := args()
		if err != nil {
			return err
		}
		name := tool
		if t, ok := params["__tool"].(string); ok {
			name = t
			delete(params, "__tool")
		}
		text, isErr, err := a.call(ctx, name, params)
		if err != nil {
			return err
		}
		if strings.Contains(text, "Paused by the user") || strings.Contains(text, "Waiting for the user") {
			return errHandedToUser
		}
		if !isErr {
			return nil
		}
		switch {
		case strings.Contains(text, "Call snapshot"):
			if _, _, err := a.call(ctx, "snapshot", map[string]any{}); err != nil {
				return err
			}
		case strings.Contains(text, "Close it first"):
			if err := a.closeCovering(ctx, text); err != nil {
				return err
			}
		default:
			return fmt.Errorf("error is not actionable: %s", text)
		}
	}
	return fmt.Errorf("%s still failing after following the error instructions", tool)
}

var closeNames = regexp.MustCompile(`(?i)^(fechar|close|cancelar|cancel|ok|entendi|x|×)$`)

// closeCovering fecha o dialog que a mensagem de erro apontou, pelo botão de fechar dele.
func (a *Agent) closeCovering(ctx context.Context, text string) error {
	m := coveredRe.FindStringSubmatch(text)
	if m == nil {
		return fmt.Errorf("error is not actionable: %s", text)
	}
	buttons := []*Element{}
	for _, el := range a.view.Children(m[2]) {
		if el.Role == "button" {
			buttons = append(buttons, el)
		}
	}
	if len(buttons) == 0 {
		return fmt.Errorf("no button to close %s %s", m[1], m[2])
	}
	target := buttons[0]
	for _, b := range buttons {
		if closeNames.MatchString(strings.TrimSpace(b.Name)) {
			target = b
			break
		}
	}
	_, _, err := a.call(ctx, "click", map[string]any{"ref": target.Ref})
	return err
}

func (a *Agent) call(ctx context.Context, tool string, args map[string]any) (string, bool, error) {
	if a.result.Steps >= a.maxSteps {
		return "", false, fmt.Errorf("step limit of %d reached", a.maxSteps)
	}
	a.result.Steps++
	text, isErr, err := a.caller.Call(ctx, tool, args)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", tool, err)
	}
	a.result.Tokens += tokens.Estimate(text)
	a.result.Transcript = append(a.result.Transcript, Exchange{Tool: tool, Args: args, Text: text, IsError: isErr})
	a.result.LastText = text
	a.view.Absorb(text)
	if a.AfterCall != nil {
		a.AfterCall(a.result.Steps, tool)
	}
	return text, isErr, nil
}
