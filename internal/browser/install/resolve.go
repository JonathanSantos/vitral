package install

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Source diz de onde veio o navegador escolhido.
type Source string

const (
	SourceExplicit Source = "explicit" // --browser-path / VITRAL_BROWSER_PATH
	SourceManaged  Source = "managed"  // Chrome for Testing em ~/.vitral/browsers
	SourceCache    Source = "cache"    // Chromium já baixado pelo Puppeteer ou Playwright
	SourceSystem   Source = "system"   // Chrome/Edge do sistema (só com --allow-system-browser)
)

// Resolution é o navegador escolhido e o motivo (para logs e para o doctor).
type Resolution struct {
	Source  Source
	Path    string
	Version string
	Reason  string
}

// Env é tudo o que a resolução consulta. Os caminhos já vêm descobertos; a função é pura.
type Env struct {
	ExplicitPath string
	AllowSystem  bool
	Supported    bool   // há build do Chrome for Testing para esta plataforma
	ManagedPath  string // executável da versão fixada (pode não existir)
	// Install baixa o Chrome for Testing. nil = não baixar (modo de consulta do doctor).
	Install     func() (string, error)
	CachePaths  []string
	SystemPaths []string
	Exists      func(path string) bool
	// Version roda o executável com --version; erro = não executa.
	Version func(path string) (string, error)
}

// NotFoundError explica tudo o que foi tentado, para a mensagem amigável.
type NotFoundError struct {
	ExplicitPath  string
	ExplicitErr   error
	Unsupported   bool
	InstallErr    error
	NotInstalled  bool // modo consulta: o gerenciado ainda não foi baixado
	CachesChecked int
	SystemAllowed bool
}

func (e *NotFoundError) Error() string {
	if e.ExplicitPath != "" {
		return fmt.Sprintf("browser at %s does not run: %v", e.ExplicitPath, e.ExplicitErr)
	}
	var tried []string
	switch {
	case e.Unsupported:
		tried = append(tried, "no Chrome for Testing build for this platform")
	case e.InstallErr != nil:
		tried = append(tried, "Chrome for Testing download failed: "+e.InstallErr.Error())
	case e.NotInstalled:
		tried = append(tried, "Chrome for Testing not downloaded yet")
	}
	tried = append(tried, fmt.Sprintf("no usable Chromium in Puppeteer/Playwright caches (%d checked)", e.CachesChecked))
	if !e.SystemAllowed {
		tried = append(tried, "system browser not allowed (--allow-system-browser)")
	} else {
		tried = append(tried, "no Chrome or Edge found on the system")
	}
	return "no browser available: " + strings.Join(tried, "; ") +
		". Tell the user to run `vitral doctor`: it explains how to use a mirror (VITRAL_BROWSER_MIRROR), install from a zip (vitral install-browser --from) or allow the system browser (--allow-system-browser)."
}

// Resolve escolhe o navegador na ordem: explícito, gerenciado (baixa se faltar), caches do
// Puppeteer/Playwright (o mais novo que roda) e, só com permissão, o do sistema.
func Resolve(e Env) (Resolution, error) {
	if e.ExplicitPath != "" {
		v, err := e.Version(e.ExplicitPath)
		if err != nil || !e.Exists(e.ExplicitPath) {
			if err == nil {
				err = errors.New("file not found")
			}
			return Resolution{}, &NotFoundError{ExplicitPath: e.ExplicitPath, ExplicitErr: err}
		}
		return Resolution{Source: SourceExplicit, Path: e.ExplicitPath, Version: v, Reason: "set by --browser-path or VITRAL_BROWSER_PATH"}, nil
	}

	nf := &NotFoundError{SystemAllowed: e.AllowSystem, CachesChecked: len(e.CachePaths), Unsupported: !e.Supported}
	if e.Supported {
		if r, ok := managed(e, nf); ok {
			return r, nil
		}
	}

	reason := "Chrome for Testing unavailable"
	switch {
	case nf.Unsupported:
		reason = "no Chrome for Testing build for this platform"
	case nf.InstallErr != nil:
		reason = "Chrome for Testing download failed"
	}
	if r, ok := newest(e, e.CachePaths); ok {
		r.Source, r.Reason = SourceCache, reason+"; using a Chromium from the Puppeteer/Playwright cache"
		return r, nil
	}
	if e.AllowSystem {
		for _, p := range e.SystemPaths {
			if !e.Exists(p) {
				continue
			}
			if v, err := e.Version(p); err == nil {
				return Resolution{Source: SourceSystem, Path: p, Version: v, Reason: reason + "; system browser allowed by --allow-system-browser"}, nil
			}
		}
	}
	return Resolution{}, nf
}

func managed(e Env, nf *NotFoundError) (Resolution, bool) {
	if e.Exists(e.ManagedPath) {
		if v, err := e.Version(e.ManagedPath); err == nil {
			return Resolution{Source: SourceManaged, Path: e.ManagedPath, Version: v, Reason: "managed Chrome for Testing"}, true
		}
	}
	if e.Install == nil {
		nf.NotInstalled = true
		return Resolution{}, false
	}
	path, err := e.Install()
	if err != nil {
		nf.InstallErr = err
		return Resolution{}, false
	}
	v, err := e.Version(path)
	if err != nil {
		nf.InstallErr = err
		return Resolution{}, false
	}
	return Resolution{Source: SourceManaged, Path: path, Version: v, Reason: "managed Chrome for Testing (just downloaded)"}, true
}

// newest devolve o executável de versão mais alta entre os que existem e rodam.
func newest(e Env, paths []string) (Resolution, bool) {
	var best Resolution
	for _, p := range paths {
		if !e.Exists(p) {
			continue
		}
		v, err := e.Version(p)
		if err != nil {
			continue
		}
		if best.Path == "" || CompareVersions(v, best.Version) > 0 {
			best = Resolution{Path: p, Version: v}
		}
	}
	return best, best.Path != ""
}

// cacheExecutables são os executáveis possíveis dentro de uma pasta de build do Puppeteer/Playwright.
// Só Chromium/Chrome: Firefox e WebKit do Playwright não falam CDP.
var cacheExecutables = []string{
	"chrome-linux64/chrome",
	"chrome-linux/chrome",
	"chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
	"chrome-mac-x64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
	"chrome-mac-arm64/Chromium.app/Contents/MacOS/Chromium",
	"chrome-mac/Chromium.app/Contents/MacOS/Chromium",
	"chrome-win64/chrome.exe",
	"chrome-win/chrome.exe",
}

// CacheCandidates lista executáveis nos caches padrão do Puppeteer e do Playwright.
// glob é injetado (filepath.Glob em produção).
func CacheCandidates(goos, home string, getenv func(string) string, glob func(string) ([]string, error)) []string {
	var buildDirs []string
	add := func(pattern string) {
		matches, _ := glob(pattern)
		buildDirs = append(buildDirs, matches...)
	}

	puppeteer := getenv("PUPPETEER_CACHE_DIR")
	if puppeteer == "" {
		puppeteer = filepath.Join(home, ".cache", "puppeteer")
	}
	add(filepath.Join(puppeteer, "chrome", "*"))
	add(filepath.Join(puppeteer, "chromium", "*"))

	playwright := getenv("PLAYWRIGHT_BROWSERS_PATH")
	if playwright == "" || playwright == "0" {
		switch goos {
		case "darwin":
			playwright = filepath.Join(home, "Library", "Caches", "ms-playwright")
		case "windows":
			local := getenv("LOCALAPPDATA")
			if local == "" {
				local = filepath.Join(home, "AppData", "Local")
			}
			playwright = filepath.Join(local, "ms-playwright")
		default:
			playwright = filepath.Join(home, ".cache", "ms-playwright")
		}
	}
	// chromium-<rev> (o browser completo); chromium_headless_shell-*, firefox-* e webkit-* ficam de fora.
	add(filepath.Join(playwright, "chromium-*"))

	var out []string
	for _, dir := range buildDirs {
		for _, rel := range cacheExecutables {
			out = append(out, filepath.Join(dir, filepath.FromSlash(rel)))
		}
	}
	return out
}

// SystemCandidates lista Chrome e Edge instalados no sistema (usado só com --allow-system-browser).
func SystemCandidates(goos string, getenv func(string) string, lookPath func(string) (string, error), rodLookPath func() (string, bool)) []string {
	var out []string
	if p, ok := rodLookPath(); ok {
		out = append(out, p)
	}
	switch goos {
	case "darwin":
		out = append(out, "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge")
	case "windows":
		for _, base := range []string{getenv("ProgramFiles(x86)"), getenv("ProgramFiles"), getenv("LOCALAPPDATA")} {
			if base != "" {
				out = append(out, filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
			}
		}
	default:
		for _, name := range []string{"microsoft-edge", "microsoft-edge-stable"} {
			if p, err := lookPath(name); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}
