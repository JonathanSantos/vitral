//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/JonathanSantos/vitral/test/tokens"
)

// Scenario é um roteiro fixo de chamadas com asserts (docs/TESTING.md, seção 7.2).
type Scenario struct {
	Name  string   `yaml:"name"`
	Args  []string `yaml:"args"`
	Steps []Step   `yaml:"steps"`
}

// Step é uma chamada de ferramenta ou uma ação do harness.
type Step struct {
	Call        string            `yaml:"call"`
	Args        map[string]any    `yaml:"args"`
	Expect      *Expect           `yaml:"expect"`
	Save        map[string]string `yaml:"save"`
	Async       string            `yaml:"async"`
	Await       string            `yaml:"await"`
	Pause       bool              `yaml:"pause"`
	Resume      bool              `yaml:"resume"`
	WaitOverlay string            `yaml:"waitOverlay"`
	Fixture     *FixtureChange    `yaml:"fixture"`
}

// Expect são os asserts sobre a resposta.
type Expect struct {
	Contains    []string `yaml:"contains"`
	NotContains []string `yaml:"notContains"`
	Regex       string   `yaml:"regex"`
	IsError     *bool    `yaml:"isError"`
	MaxTokens   int      `yaml:"maxTokens"`
}

// FixtureChange muda o site de fixtures durante o cenário.
type FixtureChange struct {
	RenameSubmit *bool `yaml:"renameSubmit"`
	OrdersStatus *int  `yaml:"ordersStatus"`
}

type response struct {
	text    string
	isError bool
	err     error
}

func TestScriptedScenarios(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(repoRoot(), "test", "e2e", "scenarios", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenarios: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var sc Scenario
		if err := yaml.Unmarshal(data, &sc); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		t.Run(sc.Name, func(t *testing.T) { runScenario(t, sc) })
	}
}

func runScenario(t *testing.T, sc Scenario) {
	s := start(t, sc.Args...)
	t.Cleanup(func() {
		site.RenameSubmit.Store(false)
		site.OrdersStatus.Store(0)
	})
	vars := map[string]string{}
	pending := map[string]chan response{}
	for i, step := range sc.Steps {
		label := fmt.Sprintf("step %d", i+1)
		switch {
		case step.Pause, step.Resume:
			s.clickOverlay("toggle_pause")
		case step.WaitOverlay != "":
			eventually(t, 20*time.Second, func() error {
				if got := s.overlayText(); !strings.Contains(got, step.WaitOverlay) {
					return fmt.Errorf("overlay shows %q", got)
				}
				return nil
			})
		case step.Fixture != nil:
			if step.Fixture.RenameSubmit != nil {
				site.RenameSubmit.Store(*step.Fixture.RenameSubmit)
			}
			if step.Fixture.OrdersStatus != nil {
				site.OrdersStatus.Store(int32(*step.Fixture.OrdersStatus))
			}
		case step.Await != "":
			ch, ok := pending[step.Await]
			if !ok {
				t.Fatalf("%s: await %q without async", label, step.Await)
			}
			r := <-ch
			check(t, label+" (await "+step.Await+")", r, step.Expect, vars, step.Save)
		case step.Call != "":
			args, err := render(step.Args, s.history(), vars)
			if err != nil {
				t.Fatalf("%s: %v", label, err)
			}
			if step.Async != "" {
				ch := make(chan response, 1)
				pending[step.Async] = ch
				go func() {
					text, isErr, err := s.Call(context.Background(), step.Call, args)
					ch <- response{text, isErr, err}
				}()
				continue
			}
			text, isErr, err := s.Call(context.Background(), step.Call, args)
			recordTokens(fmt.Sprintf("action/%s/%02d-%s", sc.Name, i+1, step.Call), text)
			check(t, label+" "+step.Call, response{text, isErr, err}, step.Expect, vars, step.Save)
		default:
			t.Fatalf("%s: empty step", label)
		}
	}
}

func check(t *testing.T, label string, r response, e *Expect, vars map[string]string, save map[string]string) {
	t.Helper()
	if r.err != nil {
		t.Fatalf("%s: protocol error: %v", label, r.err)
	}
	for name, pattern := range save {
		m := regexp.MustCompile(pattern).FindStringSubmatch(r.text)
		if len(m) < 2 {
			t.Fatalf("%s: save %s: %q not found in:\n%s", label, name, pattern, r.text)
		}
		vars[name] = m[1]
	}
	if e == nil {
		if r.isError {
			t.Fatalf("%s: unexpected error:\n%s", label, r.text)
		}
		return
	}
	wantErr := e.IsError != nil && *e.IsError
	if r.isError != wantErr {
		t.Fatalf("%s: isError = %v, want %v:\n%s", label, r.isError, wantErr, r.text)
	}
	for _, c := range e.Contains {
		if !strings.Contains(r.text, c) {
			t.Fatalf("%s: missing %q in:\n%s", label, c, r.text)
		}
	}
	for _, c := range e.NotContains {
		if strings.Contains(r.text, c) {
			t.Fatalf("%s: unexpected %q in:\n%s", label, c, r.text)
		}
	}
	if e.Regex != "" && !regexp.MustCompile(e.Regex).MatchString(r.text) {
		t.Fatalf("%s: regex %q does not match:\n%s", label, e.Regex, r.text)
	}
	if e.MaxTokens > 0 {
		if got := tokens.Estimate(r.text); got > e.MaxTokens {
			t.Fatalf("%s: %d tokens > max %d", label, got, e.MaxTokens)
		}
	}
}

// quotes troca 'x' por "x" dentro das ações {{ }} (o formato da spec usa aspas simples).
var (
	actionRe     = regexp.MustCompile(`\{\{.*?\}\}`)
	singleQuoted = regexp.MustCompile(`'([^']*)'`)
)

// render aplica os templates nos valores de texto dos argumentos.
func render(args map[string]any, last []string, vars map[string]string) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range args {
		r, err := renderValue(v, last, vars)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = r
	}
	return out, nil
}

func renderValue(v any, last []string, vars map[string]string) (any, error) {
	switch x := v.(type) {
	case string:
		return renderString(x, last, vars)
	case map[string]any:
		return render(x, last, vars)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			r, err := renderValue(item, last, vars)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	return v, nil
}

func renderString(s string, last []string, vars map[string]string) (any, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	src := actionRe.ReplaceAllStringFunc(s, func(a string) string { return singleQuoted.ReplaceAllString(a, `"$1"`) })
	funcs := template.FuncMap{
		"base": func() string { return site.URL },
		"ref":  func(role, name string) (string, error) { return refIn(last, role, name) },
		"var": func(name string) (string, error) {
			v, ok := vars[name]
			if !ok {
				return "", fmt.Errorf("var %q not saved", name)
			}
			return v, nil
		},
	}
	tpl, err := template.New("arg").Funcs(funcs).Option("missingkey=error").Parse(src)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	if err := tpl.Execute(&b, nil); err != nil {
		return nil, err
	}
	// Um valor que é só um template e vira número (ex.: fromStep) vai como número.
	if strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") && strings.Count(s, "{{") == 1 {
		if n, err := strconv.Atoi(b.String()); err == nil {
			return n, nil
		}
	}
	return b.String(), nil
}

// refIn resolve a ref pelo papel e nome a partir das respostas textuais, da mais recente para a
// mais antiga (nunca do DOM): é o que uma IA faria lendo a conversa.
func refIn(history []string, role, name string) (string, error) {
	re := regexp.MustCompile(`\[(e\d+)\] ` + regexp.QuoteMeta(role) + ` "` + regexp.QuoteMeta(name) + `"`)
	for i := len(history) - 1; i >= 0; i-- {
		if m := re.FindStringSubmatch(history[i]); m != nil {
			return m[1], nil
		}
	}
	last := ""
	if len(history) > 0 {
		last = history[len(history)-1]
	}
	return "", fmt.Errorf("no %s %q in the responses so far; last:\n%s", role, name, last)
}
