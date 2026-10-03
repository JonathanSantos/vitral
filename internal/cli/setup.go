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

// Setup instala o binário em ~/.vitral/bin e configura o VS Code (pasta atual ou usuário).
func Setup(e Env, o SetupOptions) error {
	res, err := installBinary(e)
	if err != nil {
		return fmt.Errorf(msgInstallFailed, res.Path, err)
	}
	printInstall(e, res)
	entry := ServerEntry{Type: "stdio", Command: res.Path, Args: []string{}}

	if !o.Global {
		mcpPath := filepath.Join(e.Cwd, ".vscode", "mcp.json")
		if err := writeMCP(e, mcpPath, entry); err != nil {
			return err
		}
		if vscodeInGit(e) {
			fmt.Fprintf(e.Stdout, msgGitWarning, mcpPath)
		}
		if err := copyPrompts(e, o, filepath.Join(e.Cwd, ".github", "prompts")); err != nil {
			return err
		}
		fmt.Fprint(e.Stdout, msgNextStepsLocal)
		return nil
	}

	codes := installedVSCodes(e)
	if len(codes) == 0 {
		return errors.New(msgGlobalNoVSCode)
	}
	if !o.Yes && !e.Interactive {
		for _, c := range codes {
			fmt.Fprintf(e.Stdout, msgGlobalCandidate, c.Name, c.MCPPath(), c.PromptsDir())
		}
		return errors.New(msgGlobalNeedsTTY)
	}
	for _, c := range codes {
		// Perfis e locais variam entre versões do VS Code: mostra o caminho e pede confirmação.
		fmt.Fprintf(e.Stdout, msgGlobalCandidate, c.Name, c.MCPPath(), c.PromptsDir())
		if c.HasProfiles() {
			fmt.Fprint(e.Stdout, msgGlobalProfiles)
		}
		if !o.Yes && !e.confirm(msgGlobalConfirm) {
			fmt.Fprintf(e.Stdout, msgGlobalSkipped, c.Name)
			continue
		}
		if err := writeMCP(e, c.MCPPath(), entry); err != nil {
			return err
		}
		if err := copyPrompts(e, o, c.PromptsDir()); err != nil {
			return err
		}
	}
	fmt.Fprint(e.Stdout, msgNextStepsGlobal)
	return nil
}

// writeMCP faz o merge preservando o resto do arquivo e guarda um .bak antes de gravar.
func writeMCP(e Env, file string, entry ServerEntry) error {
	old, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf(msgMCPFailed, file, err)
	}
	merged, err := MergeServer(old, ServerName, entry)
	if err != nil {
		return fmt.Errorf(msgMCPFailed, file, err)
	}
	if bytes.Equal(merged, old) {
		fmt.Fprintf(e.Stdout, msgMCPWritten, file)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf(msgMCPFailed, file, err)
	}
	if len(old) > 0 {
		backup := file + ".bak"
		if err := os.WriteFile(backup, old, 0o644); err != nil {
			return fmt.Errorf(msgMCPFailed, file, err)
		}
		defer fmt.Fprintf(e.Stdout, msgMCPBackup, backup)
	}
	if err := os.WriteFile(file, merged, 0o644); err != nil {
		return fmt.Errorf(msgMCPFailed, file, err)
	}
	fmt.Fprintf(e.Stdout, msgMCPWritten, file)
	return nil
}

// vscodeInGit diz se há arquivos de .vscode versionados (o caminho absoluto é desta máquina).
func vscodeInGit(e Env) bool {
	out, err := e.Run(e.Cwd, "git", "ls-files", "--", ".vscode")
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
