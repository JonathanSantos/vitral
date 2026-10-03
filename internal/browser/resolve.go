package browser

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-rod/rod/lib/launcher"

	"github.com/JonathanSantos/vitral/internal/browser/install"
	"github.com/JonathanSantos/vitral/internal/config"
)

// downloadTimeout limita o download do Chrome for Testing (~150 MB) em redes lentas.
const downloadTimeout = 15 * time.Minute

// NewInstaller cria o instalador do Chrome for Testing para esta máquina.
// ok=false quando não há build para a plataforma.
func NewInstaller(cfg config.Config, progress install.Logger) (*install.Installer, bool) {
	platform, ok := install.Platform(runtime.GOOS, runtime.GOARCH)
	return install.NewInstaller(cfg.BrowsersDir, platform, cfg.BrowserMirror, progress), ok
}

// Discover monta o ambiente real da resolução. dry=true não baixa nada (usado pelo doctor).
func Discover(cfg config.Config, progress install.Logger, dry bool) install.Env {
	home, _ := os.UserHomeDir()
	inst, supported := NewInstaller(cfg, progress)
	env := install.Env{
		ExplicitPath: cfg.BrowserPath,
		AllowSystem:  cfg.AllowSystemBrowser,
		Supported:    supported,
		ManagedPath:  inst.Executable(),
		CachePaths:   install.CacheCandidates(runtime.GOOS, home, os.Getenv, filepath.Glob),
		Exists: func(p string) bool {
			info, err := os.Stat(p)
			return err == nil && !info.IsDir()
		},
		Version: install.RunVersion,
	}
	// O navegador do sistema só é procurado com opt-in: nunca usamos o do usuário sem permissão.
	if cfg.AllowSystemBrowser {
		env.SystemPaths = install.SystemCandidates(runtime.GOOS, os.Getenv, exec.LookPath, launcher.LookPath)
	}
	if !dry && supported {
		env.Install = func() (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
			defer cancel()
			return inst.Install(ctx)
		}
	}
	return env
}

// Resolve escolhe o navegador (ordem em install.Resolve).
func Resolve(cfg config.Config, progress install.Logger, dry bool) (install.Resolution, error) {
	return install.Resolve(Discover(cfg, progress, dry))
}

// stderrProgress é o progresso padrão quando ninguém traduz (stdout é do protocolo MCP).
var stderrProgress install.Logger = log.New(os.Stderr, "[browser] ", log.LstdFlags)
