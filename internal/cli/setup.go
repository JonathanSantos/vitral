package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/JonathanSantos/vitral"
)

// SetupOptions são as opções do "vitral setup".
type SetupOptions struct {
	Global bool
	Yes    bool // responde "sim" para as confirmações
}

func parseSetup(args []string) (SetupOptions, error) {
	var o SetupOptions
	for _, a := range args {
		switch a {
		case "--global", "-global":
			o.Global = true
		case "--yes", "-yes", "-y":
			o.Yes = true
		default:
			return o, fmt.Errorf(msgUnknownFlag, a, "setup")
		}
	}
	return o, nil
}

// Setup instala o binário em ~/.vitral/bin e configura o Vitral no formato portável: .mcp.json na
// pasta atual, ou ~/.copilot/mcp-config.json para todos os projetos (--global). Onde o Vitral estiver
// no formato antigo do VS Code (deprecado), oferece tirar de lá, para não subir duas vezes.
func Setup(e Env, o SetupOptions) error {
	res, err := installBinary(e)
	if err != nil {
		return fmt.Errorf(msgInstallFailed, res.Path, err)
	}
	printInstall(e, res)
	entry := ServerEntry{Type: "stdio", Command: res.Path, Args: []string{}, Tools: []string{"*"}}

	if !o.Global {
		loc := PortableWorkspace(e)
		if err := writeMCP(e, loc, entry); err != nil {
			return err
		}
		if inGit(e, ".mcp.json") {
			fmt.Fprintf(e.Stdout, msgGitWarning, loc.Path)
		}
		if err := migrate(e, o, WorkspaceLocations(e)); err != nil {
			return err
		}
		if err := copyPrompts(e, o, filepath.Join(e.Cwd, ".github", "prompts")); err != nil {
			return err
		}
		if err := installSkill(e, o, WorkspaceSkillDir(e.Cwd)); err != nil {
			return err
		}
		fmt.Fprint(e.Stdout, msgNextStepsLocal)
		return nil
	}

	loc := PortableGlobal(e)
	codes := installedVSCodes(e)
	fmt.Fprintf(e.Stdout, msgGlobalPlan, loc.Path)
	for _, c := range codes {
		fmt.Fprintf(e.Stdout, msgGlobalPrompts, c.Name, c.PromptsDir())
	}
	if !o.Yes && !e.Interactive {
		return errors.New(msgGlobalNeedsTTY)
	}
	if !o.Yes && !e.confirm(msgGlobalConfirm) {
		fmt.Fprint(e.Stdout, msgGlobalSkipped)
		return nil
	}
	if err := writeMCP(e, loc, entry); err != nil {
		return err
	}
	if err := migrate(e, o, GlobalLocations(e)); err != nil {
		return err
	}
	for _, c := range codes {
		if err := copyPrompts(e, o, c.PromptsDir()); err != nil {
			return err
		}
	}
	if len(codes) == 0 {
		fmt.Fprint(e.Stdout, msgGlobalNoVSCode)
	}
	// A skill do usuário fica em ~/.copilot/skills: vale para todos os VS Code de uma vez.
	if err := installSkill(e, o, GlobalSkillDir(e.Home)); err != nil {
		return err
	}
	fmt.Fprint(e.Stdout, msgNextStepsGlobal)
	return nil
}

// writeMCP faz o merge preservando o resto do arquivo e guarda um .bak antes de gravar.
func writeMCP(e Env, loc Location, entry ServerEntry) error {
	old, err := os.ReadFile(loc.Path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf(msgMCPFailed, loc.Path, err)
	}
	merged, err := MergeServer(old, loc.Key, ServerName, entry)
	if err != nil {
		return fmt.Errorf(msgMCPFailed, loc.Path, err)
	}
	if err := writeConfig(e, loc.Path, old, merged); err != nil {
		return err
	}
	fmt.Fprintf(e.Stdout, msgMCPWritten, loc.Path)
	return nil
}

// migrate tira o Vitral dos arquivos no formato antigo (com confirmação): com ele em dois lugares, o
// VS Code sobe dois servidores e dois navegadores no mesmo perfil.
func migrate(e Env, o SetupOptions, locs []Location) error {
	for _, p := range Inspect(locs) {
		if !p.Deprecated || p.State == StateAbsent {
			continue
		}
		if !o.Yes && !e.confirm(fmt.Sprintf(msgMigrateConfirm, p.Path)) {
			fmt.Fprintf(e.Stdout, msgMigrateKept, p.Path)
			continue
		}
		from := p.Key
		if p.State == StateDisabled {
			from = KeyDisabled
		}
		if _, err := moveIn(e, p.Path, from, ""); err != nil {
			return err
		}
		fmt.Fprintf(e.Stdout, msgMigrated, p.Path)
	}
	return nil
}

// inGit diz se o arquivo da pasta está versionado (o caminho do Vitral nele é desta máquina).
func inGit(e Env, file string) bool {
	out, err := e.Run(e.Cwd, "git", "ls-files", "--", file)
	return err == nil && out != ""
}

// copyPrompts copia os prompts embutidos sem sobrescrever arquivos diferentes sem confirmação.
func copyPrompts(e Env, o SetupOptions, dir string) error {
	entries, err := fs.ReadDir(vitral.Prompts, vitral.PromptsDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var added, updated, same, kept int
	for _, entry := range entries {
		data, err := fs.ReadFile(vitral.Prompts, path.Join(vitral.PromptsDir, entry.Name()))
		if err != nil {
			return err
		}
		target := filepath.Join(dir, entry.Name())
		current, err := os.ReadFile(target)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			added++
		case err != nil:
			return err
		case bytes.Equal(current, data):
			same++
			continue
		case o.Yes || e.confirm(fmt.Sprintf(msgPromptOverwrite, target)):
			updated++
		default:
			kept++
			fmt.Fprintf(e.Stdout, msgPromptKept, target)
			continue
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(e.Stdout, msgPromptsCopied, dir, added, updated, same, kept)
	return nil
}
