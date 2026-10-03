package cli

import (
	"os"
	"path/filepath"
)

// VSCode é uma instalação do VS Code com pasta de configuração do usuário.
type VSCode struct {
	Name    string
	UserDir string
}

// MCPPath e PromptsDir são os locais de configuração do usuário (perfil padrão).
func (v VSCode) MCPPath() string    { return filepath.Join(v.UserDir, "mcp.json") }
func (v VSCode) PromptsDir() string { return filepath.Join(v.UserDir, "prompts") }

// HasProfiles indica perfis personalizados, que podem ter configurações próprias.
func (v VSCode) HasProfiles() bool {
	entries, err := os.ReadDir(filepath.Join(v.UserDir, "profiles"))
	return err == nil && len(entries) > 0
}

// VSCodeCandidates lista onde o VS Code e o Insiders guardam a configuração do usuário neste SO.
// Pura: só calcula caminhos; quem chama confere se existem.
func VSCodeCandidates(goos, home string, getenv func(string) string) []VSCode {
	base := ""
	switch goos {
	case "darwin":
		base = filepath.Join(home, "Library", "Application Support")
	case "windows":
		base = getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
	default:
		base = getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
	}
	return []VSCode{
		{Name: "VS Code", UserDir: filepath.Join(base, "Code", "User")},
		{Name: "VS Code Insiders", UserDir: filepath.Join(base, "Code - Insiders", "User")},
	}
}

// installedVSCodes devolve só os candidatos cuja pasta existe.
func installedVSCodes(e Env) []VSCode {
	var out []VSCode
	for _, c := range VSCodeCandidates(e.GOOS, e.Home, e.Getenv) {
		if info, err := os.Stat(c.UserDir); err == nil && info.IsDir() {
			out = append(out, c)
		}
	}
	return out
}
