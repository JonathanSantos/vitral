package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/go-rod/rod/lib/launcher"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/config"
)

// Check é um item do doctor: ✓ quando OK, ✗ com a ação sugerida quando não.
type Check struct {
	OK   bool
	Text string
	Fix  string
}

// Doctor confere o ambiente e devolve false se algo precisa de atenção.
func Doctor(e Env, cfg config.Config) bool {
	fmt.Fprintf(e.Stdout, msgDoctorTitle, config.Version)
	checks := []Check{checkBrowser(), checkProfile(cfg.ProfileDir), checkLaunch(cfg)}
	checks = append(checks, checkInstalled(e))
	checks = append(checks, checkMCP(e)...)
	allOK := true
	for _, c := range checks {
		if c.OK {
			fmt.Fprintf(e.Stdout, msgDoctorOK, c.Text)
			continue
		}
		allOK = false
		fmt.Fprintf(e.Stdout, msgDoctorFail, c.Text, c.Fix)
	}
	if allOK {
		fmt.Fprint(e.Stdout, msgDoctorAllGood)
	} else {
		fmt.Fprint(e.Stdout, msgDoctorSomeFailed)
	}
	return allOK
}

func checkBrowser() Check {
	if bin, ok := launcher.LookPath(); ok {
		return Check{OK: true, Text: fmt.Sprintf(msgCheckBrowserOK, bin)}
	}
	downloaded := launcher.NewBrowser()
	downloaded.Logger = quietLogger{}
	if _, err := os.Stat(downloaded.BinPath()); err == nil {
		return Check{OK: true, Text: fmt.Sprintf(msgCheckBrowserOK, downloaded.BinPath())}
	}
	return Check{Text: msgCheckBrowserMissing, Fix: msgFixBrowser}
}

func checkProfile(dir string) Check {
	fail := Check{Text: fmt.Sprintf(msgCheckProfileFail, dir), Fix: msgFixProfile}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail
	}
	f, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return fail
	}
	f.Close()
	_ = os.Remove(f.Name())
	return Check{OK: true, Text: fmt.Sprintf(msgCheckProfileOK, dir)}
}

// checkLaunch abre e fecha um navegador headless com perfil temporário, para não disputar
// o perfil com um Vitral que o VS Code já esteja usando.
func checkLaunch(cfg config.Config) Check {
	tmp, err := os.MkdirTemp("", config.Name+"-doctor-*")
	if err != nil {
		return Check{Text: fmt.Sprintf(msgCheckLaunchFail, err), Fix: msgFixLaunch}
	}
	defer os.RemoveAll(tmp)
	cfg.Headless, cfg.ProfileDir, cfg.Attach, cfg.AttachPort = true, tmp, "", 0
	m := browser.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer m.Close()
	if _, err := m.Page(); err != nil {
		return Check{Text: fmt.Sprintf(msgCheckLaunchFail, err), Fix: msgFixLaunch}
	}
	return Check{OK: true, Text: msgCheckLaunchOK}
}

func checkInstalled(e Env) Check {
	path := InstalledPath(e)
	if _, err := os.Stat(path); err != nil {
		return Check{Text: fmt.Sprintf(msgCheckInstalledMiss, path), Fix: msgFixInstall}
	}
	v := binaryVersion(e, path)
	if v != config.Version {
		return Check{Text: fmt.Sprintf(msgCheckInstalledOld, path, v, config.Version), Fix: msgFixInstall}
	}
	return Check{OK: true, Text: fmt.Sprintf(msgCheckInstalledOK, path, v)}
}

// checkMCP olha o mcp.json da pasta atual e os do usuário: o caminho existe e é a versão instalada?
func checkMCP(e Env) []Check {
	files := []string{filepath.Join(e.Cwd, ".vscode", "mcp.json")}
	for _, c := range installedVSCodes(e) {
		files = append(files, c.MCPPath())
	}
	installed := binaryVersion(e, InstalledPath(e))
	var checks []Check
	for _, file := range files {
		data, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		entry, ok, err := ReadServer(data, ServerName)
		if err != nil || !ok {
			continue
		}
		checks = append(checks, checkEntry(e, file, entry, installed))
	}
	if len(checks) == 0 {
		return []Check{{Text: msgCheckMCPNone, Fix: msgFixMCP}}
	}
	return checks
}

func checkEntry(e Env, file string, entry ServerEntry, installed string) Check {
	if _, err := os.Stat(entry.Command); err != nil {
		return Check{Text: fmt.Sprintf(msgCheckMCPBadPath, file, entry.Command), Fix: msgFixMCP}
	}
	v := binaryVersion(e, entry.Command)
	if v != installed {
		return Check{Text: fmt.Sprintf(msgCheckMCPOldVersion, file, entry.Command, v, installed), Fix: msgFixMCP}
	}
	return Check{OK: true, Text: fmt.Sprintf(msgCheckMCPOK, file, entry.Command)}
}

type quietLogger struct{}

func (quietLogger) Println(...any) {}
