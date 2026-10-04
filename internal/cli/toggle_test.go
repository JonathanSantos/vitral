package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisableEnable(t *testing.T) {
	f := newFakeEnv(t, "darwin")
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Setup(f.Env, SetupOptions{Global: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	local, global := filepath.Join(f.Cwd, ".mcp.json"), filepath.Join(f.Home, ".copilot", "mcp-config.json")
	state := func(file string) string {
		for _, p := range Inspect([]Location{{Path: file, Key: KeyPortable}}) {
			return p.State
		}
		return "missing"
	}

	// Só na pasta.
	f.out.Reset()
	if err := Disable(f.Env, ScopeOptions{Workspace: true}); err != nil {
		t.Fatal(err)
	}
	if state(local) != StateDisabled || state(global) != StateActive || !strings.Contains(f.out.String(), "Vitral desligado em "+local) {
		t.Fatalf("%s %s\n%s", state(local), state(global), f.out.String())
	}
	// Nos dois (padrão), idempotente.
	opts, err := parseScope("disable", nil)
	if err != nil || !opts.Workspace || !opts.Global {
		t.Fatalf("%+v %v", opts, err)
	}
	if err := Disable(f.Env, opts); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := Disable(f.Env, opts); err != nil || !strings.Contains(f.out.String(), "nada mudou") {
		t.Fatalf("second disable: %v\n%s", err, f.out.String())
	}
	if state(global) != StateDisabled {
		t.Fatal("global must be disabled")
	}
	f.out.Reset()
	if err := Enable(f.Env, ScopeOptions{Global: true}); err != nil {
		t.Fatal(err)
	}
	if state(global) != StateActive || state(local) != StateDisabled || !strings.Contains(f.out.String(), "Vitral ligado em "+global) {
		t.Fatalf("%s %s\n%s", state(local), state(global), f.out.String())
	}
	if err := Enable(f.Env, opts); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	if err := Enable(f.Env, opts); err != nil || !strings.Contains(f.out.String(), "nada mudou") {
		t.Fatalf("second enable: %v\n%s", err, f.out.String())
	}
	if state(local) != StateActive {
		t.Fatal("local must be active again")
	}
	if _, err := parseScope("disable", []string{"--todos"}); err == nil || !strings.Contains(err.Error(), `"--todos" para disable`) {
		t.Fatalf("%v", err)
	}
	if o, _ := parseScope("enable", []string{"-global", "-workspace"}); !o.Global || !o.Workspace {
		t.Fatal("short flags")
	}
	// Arquivo quebrado: erro claro, nada gravado.
	writeFile(t, local, `{"mcpServers": {"vitral": {"command": "/v"}`)
	if err := Disable(f.Env, ScopeOptions{Workspace: true}); err != nil {
		t.Fatalf("an unreadable file is skipped by Inspect: %v", err)
	}
	if err := os.WriteFile(local, []byte(`{"mcpServers": {"vitral": {}}, "_disabled": 3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Disable(f.Env, ScopeOptions{Workspace: true}); err == nil {
		t.Fatal("a file that cannot take the change must fail")
	}
}

func TestStatus(t *testing.T) {
	f := newFakeEnv(t, "darwin")
	cfg := doctorConfig(t, f, "")
	Status(f.Env, cfg)
	out := f.out.String()
	for _, want := range []string{"não instalado em", "nenhum arquivo com o Vitral", "Skill:", "não instalada", "Navegador: ", "Perfil: "} {
		if !strings.Contains(out, want) {
			t.Errorf("empty: missing %q in:\n%s", want, out)
		}
	}
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Setup(f.Env, SetupOptions{Global: true, Yes: true}); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(f.Cwd, ".vscode", "mcp.json")
	writeFile(t, legacy, `{"servers": {"vitral": {"command": "/old"}}}`)
	writeFile(t, filepath.Join(f.Cwd, ".github", "mcp.json"), `{ quebrado`)
	if err := Disable(f.Env, ScopeOptions{Global: true}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(f.Cwd, ".github", "skills", "vitral", "SKILL.md"), string(SkillContent("0.0.1")))
	f.out.Reset()
	Status(f.Env, cfg)
	out = f.out.String()
	for _, want := range []string{
		"instalado em " + InstalledPath(f.Env),
		"✓ ligado     " + filepath.Join(f.Cwd, ".mcp.json") + " (pasta)",
		"✓ ligado     " + legacy + " (pasta, .vscode, formato antigo do VS Code: rode vitral setup para migrar)",
		"– desligado  " + filepath.Join(f.Home, ".copilot", "mcp-config.json") + " (usuário)",
		"✗ ilegível   " + filepath.Join(f.Cwd, ".github", "mcp.json"),
		"ligado em 2 lugares",
		"está na versão 0.0.1",
		"✓ " + filepath.Join(f.Home, ".copilot", "skills", "vitral", "SKILL.md"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
