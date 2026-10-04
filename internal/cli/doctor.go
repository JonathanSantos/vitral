package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/browser/install"
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
	res, resErr := browser.Resolve(cfg, nil, true)
	checks := []Check{checkBrowser(cfg, res, resErr), checkDownload(cfg), checkProfile(cfg.ProfileDir), checkLaunch(cfg, res, resErr)}
	checks = append(checks, checkInstalled(e))
	checks = append(checks, checkMCP(e)...)
	checks = append(checks, checkSkill(e))
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

// checkBrowser mostra qual navegador será usado, sem baixar nada. Se o Chrome for Testing
// ainda não foi baixado, é ele que o Vitral vai baixar primeiro; o resultado da consulta é o plano B.
func checkBrowser(cfg config.Config, res install.Resolution, err error) Check {
	inst, supported := browser.NewInstaller(cfg, nil)
	_, statErr := os.Stat(inst.Executable())
	pending := cfg.BrowserPath == "" && supported && statErr != nil
	switch {
	case pending && err == nil:
		text := fmt.Sprintf(msgCheckBrowserPending, install.PinnedVersion) +
			fmt.Sprintf(msgCheckBrowserFallback, SourceLabel(res.Source), res.Version)
		return Check{OK: true, Text: text}
	case pending:
		// Sem plano B: tudo depende do download (o checkDownload diz se a rede deixa).
		return Check{OK: true, Text: fmt.Sprintf(msgCheckBrowserPending, install.PinnedVersion)}
	case err == nil:
		return Check{OK: true, Text: fmt.Sprintf(msgCheckBrowserOK, SourceLabel(res.Source), res.Version, res.Path)}
	}
	return Check{Text: msgCheckBrowserMissing, Fix: FormatNotFound(err, inst.Platform)}
}

// checkDownload faz só um HEAD na URL do zip para saber se a rede deixa baixar.
func checkDownload(cfg config.Config) Check {
	inst, ok := browser.NewInstaller(cfg, nil)
	if !ok {
		return Check{OK: true, Text: msgCheckDownloadSkip}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	url, err := inst.Probe(ctx)
	if err != nil {
		return Check{Text: fmt.Sprintf(msgCheckDownloadBlocked, err), Fix: msgFixDownload}
	}
	return Check{OK: true, Text: fmt.Sprintf(msgCheckDownloadOK, url)}
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

// checkLaunch abre e fecha o navegador escolhido em headless, com perfil temporário para não
// disputar o perfil com um Vitral que o VS Code já esteja usando. Não baixa nada.
func checkLaunch(cfg config.Config, res install.Resolution, resErr error) Check {
	if resErr != nil {
		return Check{Text: msgCheckLaunchSkipped, Fix: msgFixLaunchMissing}
	}
	tmp, err := os.MkdirTemp("", config.Name+"-doctor-*")
	if err != nil {
		return Check{Text: fmt.Sprintf(msgCheckLaunchFail, err), Fix: msgFixLaunch}
	}
	defer os.RemoveAll(tmp)
	cfg.Headless, cfg.ProfileDir, cfg.Attach, cfg.AttachPort = true, tmp, "", 0
	cfg.BrowserPath = res.Path
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
