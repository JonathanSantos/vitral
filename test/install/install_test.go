//go:build install

// Package install testa instalação e atualização com binários reais, como a pessoa faz
// (docs/TESTING.md, seção 10): setup numa pasta com mcp.json existente, setup de novo, atualização
// para uma versão nova e o doctor percebendo a diferença. Roda igual em Linux, macOS e Windows.
package install

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/cli"
)

const existing = `{
  // configuração da equipe: o setup não pode apagar este comentário
  "servers": {
    "outro": { "type": "stdio", "command": "outro-mcp" }
  }
}
`

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

// build compila o Vitral com a versão dada, como o make build faz.
func build(t *testing.T, dir, version string) string {
	t.Helper()
	out := filepath.Join(dir, version, cli.BinaryName(runtime.GOOS))
	cmd := exec.Command("go", "build", "-ldflags", "-X github.com/JonathanSantos/vitral/internal/config.Version="+version, "-o", out, "./cmd/vitral")
	cmd.Dir = repoRoot(t)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", version, err, b)
	}
	return out
}

type machine struct {
	t         *testing.T
	home, cwd string
}

// run executa o binário sem terminal (stdin vazio), com HOME e pasta isolados.
func (m machine) run(bin string, args ...string) (string, error) {
	m.t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = m.cwd
	cmd.Stdin = bytes.NewReader(nil)
	cmd.Env = append(os.Environ(), "HOME="+m.home, "USERPROFILE="+m.home, "APPDATA="+filepath.Join(m.home, "AppData"),
		"XDG_CONFIG_HOME="+filepath.Join(m.home, ".config"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (m machine) must(bin string, args ...string) string {
	m.t.Helper()
	out, err := m.run(bin, args...)
	if err != nil {
		m.t.Fatalf("%s %v: %v\n%s", filepath.Base(bin), args, err, out)
	}
	return out
}

func contains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in:\n%s", w, out)
		}
	}
}

func TestInstallAndUpdate(t *testing.T) {
	bins := t.TempDir()
	v1, v2 := build(t, bins, "1.0.0"), build(t, bins, "2.0.0")
	// Caminhos sem symlink (no macOS o TMPDIR passa por /private), como o processo enxerga.
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := machine{t: t, home: home, cwd: cwd}
	mcpPath := filepath.Join(m.cwd, ".vscode", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(cli.InstallDir(m.home), cli.BinaryName(runtime.GOOS))

	// 1. Primeira instalação: copia para ~/.vitral/bin, faz merge no mcp.json e guarda o .bak.
	out := m.must(v1, "setup")
	contains(t, out, "Vitral instalado em "+installed+" (versão 1.0.0)", "mcp.json configurado")
	contains(t, m.must(installed, "version"), "vitral 1.0.0")
	mcp := readFile(t, mcpPath)
	contains(t, mcp, "o setup não pode apagar este comentário", `"outro"`)
	entry, ok, err := cli.ReadServer([]byte(mcp), cli.ServerName)
	if err != nil || !ok || entry.Command != installed || entry.Type != "stdio" {
		t.Fatalf("vitral entry = %+v ok=%v err=%v, want command %s", entry, ok, err, installed)
	}
	if bak := readFile(t, mcpPath+".bak"); bak != existing {
		t.Fatalf("backup should keep the original file, got:\n%s", bak)
	}
	prompts, err := os.ReadDir(filepath.Join(m.cwd, ".github", "prompts"))
	if err != nil || len(prompts) == 0 {
		t.Fatalf("prompts not copied: %v", err)
	}
	skill := filepath.Join(cli.WorkspaceSkillDir(m.cwd), "SKILL.md")
	if v := cli.SkillVersion([]byte(readFile(t, skill))); v != "1.0.0" {
		t.Fatalf("skill version %q, want 1.0.0", v)
	}

	// 2. Setup de novo: nada muda.
	out = m.must(v1, "setup")
	contains(t, out, "já está instalado em "+installed+" (versão 1.0.0)")
	if again := readFile(t, mcpPath); again != mcp {
		t.Fatalf("second setup changed mcp.json:\n%s", again)
	}

	// 3. Um binário novo sem setup: o doctor aponta a versão instalada antiga e a skill antiga.
	out, _ = m.run(v2, "doctor", "--browsers-dir", filepath.Join(m.home, "browsers"))
	contains(t, out, "a versão instalada em "+installed+" (1.0.0) é diferente desta (2.0.0)",
		"a skill em "+skill+" está na versão 1.0.0, e o Vitral na 2.0.0")

	// 4. Atualização: substitui o binário instalado e mostra as duas versões; o mcp.json continua igual.
	out = m.must(v2, "setup")
	contains(t, out, "Vitral atualizado em "+installed+": versão 2.0.0 (antes: 1.0.0)")
	contains(t, m.must(installed, "version"), "vitral 2.0.0")
	// A skill intacta é atualizada sem perguntar (não há terminal aqui).
	if v := cli.SkillVersion([]byte(readFile(t, skill))); v != "2.0.0" {
		t.Fatalf("skill should follow the update, got %q", v)
	}
	if after := readFile(t, mcpPath); after != mcp {
		t.Fatalf("update should not touch mcp.json (same path):\n%s", after)
	}
	for _, leftover := range []string{installed + ".new", installed + ".old"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Fatalf("leftover file after update: %s", leftover)
		}
	}

	// 5. O doctor do binário instalado confirma instalação e mcp.json (o navegador não importa aqui).
	out, _ = m.run(installed, "doctor", "--browsers-dir", filepath.Join(m.home, "browsers"))
	contains(t, out, "✓ Vitral instalado em "+installed+" (versão 2.0.0)", "✓ "+mcpPath+" aponta para "+installed,
		"✓ skill do Vitral em "+skill+" (versão 2.0.0)")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
