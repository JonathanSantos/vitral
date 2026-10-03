package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JonathanSantos/vitral/internal/browser"
	"github.com/JonathanSantos/vitral/internal/browser/install"
	"github.com/JonathanSantos/vitral/internal/config"
)

// SourceLabel descreve em PT-BR de onde veio o navegador.
func SourceLabel(s install.Source) string {
	switch s {
	case install.SourceExplicit:
		return msgSourceExplicit
	case install.SourceCache:
		return msgSourceCache
	case install.SourceSystem:
		return msgSourceSystem
	}
	return msgSourceManaged
}

// FormatNotFound transforma o erro da resolução na explicação amigável com as saídas.
func FormatNotFound(err error, platform string) string {
	var nf *install.NotFoundError
	if !errors.As(err, &nf) {
		return err.Error()
	}
	var b strings.Builder
	b.WriteString(msgNotFoundTitle)
	if nf.ExplicitPath != "" {
		fmt.Fprintf(&b, msgNotFoundExplicit, nf.ExplicitPath, nf.ExplicitErr)
		b.WriteString("Corrija o caminho ou remova --browser-path para o Vitral usar o próprio navegador.\n")
		return b.String()
	}
	switch {
	case nf.Unsupported:
		b.WriteString(msgNotFoundUnsupported)
	case nf.InstallErr != nil:
		fmt.Fprintf(&b, msgNotFoundDownload, nf.InstallErr)
	case nf.NotInstalled:
		b.WriteString(msgNotFoundPending)
	}
	b.WriteString(msgNotFoundCaches)
	if nf.SystemAllowed {
		b.WriteString(msgNotFoundSystemNone)
	} else {
		b.WriteString(msgNotFoundNoSystem)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, msgNotFoundWays, install.ZipURL(install.DefaultBaseURL, install.PinnedVersion, platformOr(platform)))
	return b.String()
}

func platformOr(p string) string {
	if p == "" {
		return "<plataforma>"
	}
	return p
}

// NotFoundReporter imprime a explicação amigável em stderr quando o servidor não acha navegador.
func NotFoundReporter(w io.Writer, cfg config.Config) func(error) {
	inst, _ := browser.NewInstaller(cfg, nil)
	return func(err error) { fmt.Fprint(w, FormatNotFound(err, inst.Platform)) }
}

// InstallBrowser baixa o Chrome for Testing agora ou instala a partir de um zip (--from).
func InstallBrowser(e Env, cfg config.Config, zipPath string) error {
	inst, ok := browser.NewInstaller(cfg, DownloadLogger{W: e.Stderr})
	if !ok {
		return errors.New(msgInstallBrowserUnsupported)
	}
	if zipPath == "" {
		if v, err := inst.VersionOf(inst.Executable()); err == nil && v == inst.Version {
			fmt.Fprintf(e.Stdout, msgInstallBrowserAlready, inst.Executable(), v)
			return nil
		}
	}
	var exe string
	var err error
	if zipPath != "" {
		exe, err = inst.InstallZip(zipPath)
	} else {
		exe, err = inst.Install(context.Background())
	}
	if err != nil {
		return fmt.Errorf(msgInstallBrowserFailed, err)
	}
	fmt.Fprintf(e.Stdout, msgInstallBrowserOK, exe, inst.Version)
	return nil
}

// parseInstallBrowser separa --from das flags de configuração (mirror, browsers-dir…).
func parseInstallBrowser(args []string) (zipPath string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--from" || a == "-from":
			if i+1 >= len(args) {
				return "", nil, errors.New(msgInstallBrowserNeedsFile)
			}
			zipPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--from="):
			zipPath = strings.TrimPrefix(a, "--from=")
		default:
			rest = append(rest, a)
		}
	}
	if zipPath != "" {
		if _, err := os.Stat(zipPath); err != nil {
			return "", nil, fmt.Errorf(msgInstallBrowserFailed, err)
		}
	}
	return zipPath, rest, nil
}
