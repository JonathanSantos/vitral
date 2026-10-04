package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral"
	"github.com/JonathanSantos/vitral/internal/config"
)

// O VS Code ignora em silêncio uma skill com frontmatter inválido: o formato é conferido aqui.
func TestSkillFormat(t *testing.T) {
	src := vitral.Skill
	if !strings.HasPrefix(src, "---\n") {
		t.Fatal("SKILL.md must start with the YAML frontmatter")
	}
	front, _, ok := strings.Cut(strings.TrimPrefix(src, "---\n"), "\n---\n")
	if !ok {
		t.Fatal("frontmatter not closed")
	}
	name := regexp.MustCompile(`(?m)^name: (.+)$`).FindStringSubmatch(front)
	desc := regexp.MustCompile(`(?m)^description: (.+)$`).FindStringSubmatch(front)
	if name == nil || name[1] != SkillName || !regexp.MustCompile(`^[a-z0-9-]{1,64}$`).MatchString(name[1]) {
		t.Fatalf("name must be %q (lowercase, digits, hyphens; same as the folder): %v", SkillName, name)
	}
	if desc == nil || len(desc[1]) > 1024 || !strings.HasPrefix(desc[1], "Use when") {
		t.Fatalf("description must say when to use it, up to 1024 chars: %v", desc)
	}
	if SkillVersion([]byte(src)) != "dev" {
		t.Fatal("the source keeps the version marker with the placeholder")
	}
	// As regras combinadas vivem só na skill.
	for _, rule := range []string{
		"Say in one short sentence", `"sim"`, "Never type passwords", "Retomar",
		"Prefer `snapshot` over `screenshot`", "flow_save", "expect=", "mock_clear", "user's language",
	} {
		if !strings.Contains(src, rule) {
			t.Errorf("skill missing rule %q", rule)
		}
	}
}

func TestSkillContentAndVersion(t *testing.T) {
	data := SkillContent("1.2.3")
	if SkillVersion(data) != "1.2.3" || strings.Count(string(data), "vitral-skill-version") != 1 || !skillPristine(data) {
		t.Fatalf("marker not filled:\n%s", data[:300])
	}
	if skillPristine([]byte(strings.Replace(string(data), "Act like", "Behave like", 1))) || skillPristine([]byte(vitral.Skill)) {
		t.Fatal("an edited skill (or one without hash) is not pristine")
	}
	if SkillVersion([]byte("no marker")) != "" {
		t.Fatal("no marker, no version")
	}
	if WorkspaceSkillDir("/p") != filepath.Join("/p", ".github", "skills", "vitral") || GlobalSkillDir("/h") != filepath.Join("/h", ".copilot", "skills", "vitral") {
		t.Fatal("skill dirs")
	}
}

// Os prompts ficaram curtos e apontam para a skill.
func TestPromptsPointToSkill(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", vitral.PromptsDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join("..", "..", vitral.PromptsDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		_, body, _ := strings.Cut(strings.TrimPrefix(string(data), "---\n"), "\n---\n")
		if !strings.Contains(body, "Siga a skill `vitral`") || len(body) > 600 {
			t.Errorf("%s: body should be short and point to the skill (%d chars)", e.Name(), len(body))
		}
	}
}

func TestSetupInstallsSkill(t *testing.T) {
	f := newFakeEnv(t, "linux")
	target := filepath.Join(WorkspaceSkillDir(f.Cwd), "SKILL.md")
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	if SkillVersion([]byte(readFile(t, target))) != config.Version || !strings.Contains(f.out.String(), "✓ skill do Vitral em "+target) {
		t.Fatalf("skill not installed:\n%s", f.out.String())
	}

	f.out.Reset()
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	mustOut(t, f, "skill do Vitral já está em "+target)

	// Uma skill editada pela pessoa (ou de outra versão) nunca é sobrescrita sem confirmação.
	writeFile(t, target, "---\nname: vitral\n---\n<!-- vitral-skill-version: 0.9.0 -->\nminha versão")
	f.out.Reset()
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	mustOut(t, f, "skill mantida sem alterar: "+target)
	if !strings.Contains(readFile(t, target), "minha versão") {
		t.Fatal("skill overwritten without confirmation")
	}
	f.Interactive, f.Stdin = true, strings.NewReader("s\n")
	f.out.Reset()
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	mustOut(t, f, "(versão 0.9.0) foi editada")
	if SkillVersion([]byte(readFile(t, target))) != config.Version {
		t.Fatal("confirmed overwrite did not update the skill")
	}
}

// Instalada por uma versão anterior e sem edição: o setup atualiza sem perguntar (atualizar o
// Vitral não pode deixar a skill velha para trás).
func TestSetupUpdatesPristineSkill(t *testing.T) {
	f := newFakeEnv(t, "linux")
	target := filepath.Join(WorkspaceSkillDir(f.Cwd), "SKILL.md")
	old := strings.Replace(string(SkillContent("0.9.0")), "Act like", "Work like", 1)
	// Simula o texto de outra versão: refaz o hash para ele parecer instalado e intacto.
	old = skillMarker.ReplaceAllString(old, "<!-- vitral-skill-version: 0.9.0 sha256:"+skillHash([]byte(old))+" -->")
	writeFile(t, target, old)
	if !skillPristine([]byte(old)) {
		t.Fatal("test setup: old skill should be pristine")
	}
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	if SkillVersion([]byte(readFile(t, target))) != config.Version || strings.Contains(f.out.String(), "foi editada") {
		t.Fatalf("pristine skill should update silently:\n%s", f.out.String())
	}
}

func TestDoctorSkillCheck(t *testing.T) {
	f := newFakeEnv(t, "linux")
	if c := checkSkill(f.Env); c.OK || !strings.Contains(c.Text, "não está instalada") || c.Fix == "" {
		t.Fatalf("missing skill: %+v", c)
	}
	global := filepath.Join(GlobalSkillDir(f.Home), "SKILL.md")
	writeFile(t, global, string(SkillContent(config.Version)))
	if c := checkSkill(f.Env); !c.OK || !strings.Contains(c.Text, global) {
		t.Fatalf("global skill should count: %+v", c)
	}
	writeFile(t, filepath.Join(WorkspaceSkillDir(f.Cwd), "SKILL.md"), string(SkillContent("0.1.0")))
	if c := checkSkill(f.Env); c.OK || !strings.Contains(c.Text, "versão 0.1.0, e o Vitral na "+config.Version) {
		t.Fatalf("old workspace skill should fail: %+v", c)
	}
}

func mustOut(t *testing.T, f *fakeEnv, want string) {
	t.Helper()
	if !strings.Contains(f.out.String(), want) {
		t.Fatalf("missing %q in:\n%s", want, f.out.String())
	}
}
