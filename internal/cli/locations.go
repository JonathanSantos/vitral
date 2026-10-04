package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Location é um arquivo onde o Vitral pode estar configurado. O formato portável (.mcp.json na
// pasta e ~/.copilot/mcp-config.json no usuário, chave mcpServers) vale para o VS Code, o Copilot
// CLI e o Agent Host; o formato do VS Code (.vscode/mcp.json e o mcp.json do perfil, chave
// servers) está deprecado na documentação do VS Code.
type Location struct {
	Path       string
	Key        string
	Global     bool
	Deprecated bool
	Label      string
}

// copilotHome é $COPILOT_HOME ou ~/.copilot.
func copilotHome(e Env) string {
	if h := e.Getenv("COPILOT_HOME"); h != "" {
		return h
	}
	return filepath.Join(e.Home, ".copilot")
}

// PortableWorkspace e PortableGlobal são onde o setup grava.
func PortableWorkspace(e Env) Location {
	return Location{Path: filepath.Join(e.Cwd, ".mcp.json"), Key: KeyPortable, Label: msgLocWorkspace}
}

func PortableGlobal(e Env) Location {
	return Location{Path: filepath.Join(copilotHome(e), "mcp-config.json"), Key: KeyPortable, Global: true, Label: msgLocGlobal}
}

// WorkspaceLocations são os arquivos da pasta atual; GlobalLocations, os do usuário (inclusive o
// mcp.json de cada VS Code instalado e de cada perfil, deprecados).
func WorkspaceLocations(e Env) []Location {
	return []Location{
		PortableWorkspace(e),
		{Path: filepath.Join(e.Cwd, ".github", "mcp.json"), Key: KeyPortable, Label: msgLocWorkspaceGitHub},
		{Path: filepath.Join(e.Cwd, ".vscode", "mcp.json"), Key: KeyVSCode, Deprecated: true, Label: msgLocWorkspaceVSCode},
	}
}

func GlobalLocations(e Env) []Location {
	out := []Location{PortableGlobal(e)}
	for _, c := range installedVSCodes(e) {
		out = append(out, Location{Path: c.MCPPath(), Key: KeyVSCode, Global: true, Deprecated: true, Label: fmt.Sprintf(msgLocVSCodeUser, c.Name)})
		profiles, _ := filepath.Glob(filepath.Join(c.UserDir, "profiles", "*", "mcp.json"))
		sort.Strings(profiles)
		for _, p := range profiles {
			out = append(out, Location{Path: p, Key: KeyVSCode, Global: true, Deprecated: true, Label: fmt.Sprintf(msgLocVSCodeProfile, c.Name, filepath.Base(filepath.Dir(p)))})
		}
	}
	return out
}

// Estados do Vitral num arquivo.
const (
	StateAbsent   = "absent"
	StateActive   = "active"
	StateDisabled = "disabled"
)

// Placement é o Vitral num arquivo: ativo, desligado (vitral disable) ou ausente.
type Placement struct {
	Location
	State string
	Entry ServerEntry
	Err   error
}

// Inspect lê cada arquivo e diz o estado do Vitral nele (arquivos que não existem ficam de fora).
func Inspect(locs []Location) []Placement {
	var out []Placement
	for _, l := range locs {
		data, err := os.ReadFile(l.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		p := Placement{Location: l, State: StateAbsent}
		if err != nil {
			p.Err = err
			out = append(out, p)
			continue
		}
		if entry, ok, err := ReadServer(data, l.Key, ServerName); err != nil {
			p.Err = err
		} else if ok {
			p.State, p.Entry = StateActive, entry
		} else if entry, ok, _ := ReadServer(data, KeyDisabled, ServerName); ok {
			p.State, p.Entry = StateDisabled, entry
		}
		out = append(out, p)
	}
	return out
}

// writeConfig grava o arquivo com cópia .bak do anterior; mesmo conteúdo não é regravado.
func writeConfig(e Env, file string, old, data []byte) error {
	if bytes.Equal(old, data) {
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
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return fmt.Errorf(msgMCPFailed, file, err)
	}
	return nil
}

// moveIn move a entrada do Vitral entre chaves num arquivo (disable, enable, migração).
func moveIn(e Env, file, from, to string) (bool, error) {
	old, err := os.ReadFile(file)
	if err != nil {
		return false, fmt.Errorf(msgMCPFailed, file, err)
	}
	data, moved, err := MoveServer(old, from, to, ServerName)
	if err != nil {
		return false, fmt.Errorf(msgMCPFailed, file, err)
	}
	if !moved {
		return false, nil
	}
	return true, writeConfig(e, file, old, data)
}
