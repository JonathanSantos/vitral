package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/JonathanSantos/vitral"
	"github.com/JonathanSantos/vitral/internal/config"
)

// SkillName é a pasta da skill: o "name" do SKILL.md precisa ser igual a ela.
const SkillName = "vitral"

// O marcador guarda a versão e o hash do conteúdo instalado: com ele o setup sabe se a pessoa
// editou a skill (pergunta antes de trocar) ou se ela está como foi instalada (atualiza sozinho).
var skillMarker = regexp.MustCompile(`<!-- vitral-skill-version: (\S+?)(?: sha256:([0-9a-f]+))? -->`)

// skillHash é o hash do SKILL.md sem o marcador (12 caracteres bastam para detectar edição).
func skillHash(data []byte) string {
	sum := sha256.Sum256(skillMarker.ReplaceAll(data, []byte("<!-- vitral-skill -->")))
	return hex.EncodeToString(sum[:6])
}

// SkillContent é o SKILL.md com o marcador de versão e hash preenchido.
func SkillContent(version string) []byte {
	src := []byte(vitral.Skill)
	return skillMarker.ReplaceAll(src, []byte("<!-- vitral-skill-version: "+version+" sha256:"+skillHash(src)+" -->"))
}

// SkillVersion lê a versão do marcador de um SKILL.md instalado ("" se não houver).
func SkillVersion(data []byte) string {
	if m := skillMarker.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}

// skillPristine diz se o SKILL.md está exatamente como o Vitral instalou (o hash do marcador bate).
func skillPristine(data []byte) bool {
	m := skillMarker.FindSubmatch(data)
	return m != nil && len(m[2]) > 0 && string(m[2]) == skillHash(data)
}

// WorkspaceSkillDir é onde o VS Code procura skills do projeto.
func WorkspaceSkillDir(cwd string) string {
	return filepath.Join(cwd, ".github", "skills", SkillName)
}

// GlobalSkillDir é onde o Copilot procura skills do usuário (vale para todos os projetos).
func GlobalSkillDir(home string) string {
	return filepath.Join(home, ".copilot", "skills", SkillName)
}

// installSkill grava o SKILL.md em dir. Nunca sobrescreve um arquivo diferente sem confirmação.
func installSkill(e Env, o SetupOptions, dir string) error {
	target := filepath.Join(dir, "SKILL.md")
	data := SkillContent(config.Version)
	current, err := os.ReadFile(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	case bytes.Equal(current, data):
		fmt.Fprintf(e.Stdout, msgSkillSame, target, config.Version)
		return nil
	case skillPristine(current):
		// Instalada por uma versão anterior e sem edição: atualiza sem perguntar.
	case o.Yes || e.confirm(fmt.Sprintf(msgSkillOverwrite, target, SkillVersion(current), config.Version)):
		// A pessoa confirmou trocar a versão editada.
	default:
		fmt.Fprintf(e.Stdout, msgSkillKept, target)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, msgSkillWritten, target, config.Version)
	return nil
}

// checkSkill confere se a skill está instalada (no projeto ou no usuário) e na versão do binário.
func checkSkill(e Env) Check {
	var found []string
	for _, dir := range []string{WorkspaceSkillDir(e.Cwd), GlobalSkillDir(e.Home)} {
		target := filepath.Join(dir, "SKILL.md")
		data, err := os.ReadFile(target)
		if err != nil {
			continue
		}
		if v := SkillVersion(data); v != config.Version {
			return Check{Text: fmt.Sprintf(msgCheckSkillOld, target, v, config.Version), Fix: msgFixInstall}
		}
		found = append(found, target)
	}
	if len(found) == 0 {
		return Check{Text: msgCheckSkillMiss, Fix: msgFixInstall}
	}
	return Check{OK: true, Text: fmt.Sprintf(msgCheckSkillOK, found[0], config.Version)}
}
