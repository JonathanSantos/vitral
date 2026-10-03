package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/browser/install"
	"github.com/JonathanSantos/vitral/internal/config"
)

// doctorConfig aponta tudo para pastas temporárias e para um mirror local (sem internet).
func doctorConfig(t *testing.T, f *fakeEnv, mirror string) config.Config {
	t.Helper()
	cfg := config.Defaults(f.Home)
	cfg.ProfileDir = filepath.Join(f.Home, "profile")
	cfg.BrowsersDir = filepath.Join(f.Home, "browsers")
	cfg.BrowserMirror = mirror
	cfg.BrowserPath = filepath.Join(f.Home, "nao-existe", "chrome")
	return cfg
}

func TestDoctorReportsEverythingOffline(t *testing.T) {
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mirror.Close()

	f := newFakeEnv(t, "linux")
	if err := Setup(f.Env, SetupOptions{}); err != nil {
		t.Fatal(err)
	}
	f.out.Reset()
	ok := Doctor(f.Env, doctorConfig(t, f, mirror.URL))
	out := f.out.String()
	if ok {
		t.Fatalf("a broken --browser-path must fail the doctor:\n%s", out)
	}
	for _, want := range []string{
		"✗ nenhum navegador disponível",
		"--browser-path",
		"✓ o download do navegador está acessível",
		"✓ perfil do navegador pode ser gravado",
		"✗ não testei abrir o navegador",
		"✓ Vitral instalado em",
		"✓ " + filepath.Join(f.Cwd, ".vscode", "mcp.json") + " aponta para",
		"Algo precisa de atenção",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestDoctorChecks(t *testing.T) {
	f := newFakeEnv(t, "linux")
	t.Run("profile not writable", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "arquivo")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if c := checkProfile(filepath.Join(file, "sub")); c.OK || c.Fix == "" {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("not installed, then old version, then ok", func(t *testing.T) {
		if c := checkInstalled(f.Env); c.OK || !strings.Contains(c.Text, "não está instalado") {
			t.Fatalf("%+v", c)
		}
		writeFile(t, InstalledPath(f.Env), "binary v-old")
		f.versions["binary v-old"] = "0.0.1"
		if c := checkInstalled(f.Env); c.OK || !strings.Contains(c.Text, "(0.0.1) é diferente") {
			t.Fatalf("%+v", c)
		}
		writeFile(t, InstalledPath(f.Env), "binary v-new")
		if c := checkInstalled(f.Env); !c.OK {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("mcp.json entries", func(t *testing.T) {
		if got := checkMCP(f.Env); len(got) != 1 || got[0].OK {
			t.Fatalf("no config: %+v", got)
		}
		mcp := filepath.Join(f.Cwd, ".vscode", "mcp.json")
		writeFile(t, mcp, `{"servers":{"vitral":{"type":"stdio","command":"/nao/existe"}}}`)
		if got := checkMCP(f.Env); got[0].OK || !strings.Contains(got[0].Text, "que não existe") {
			t.Fatalf("missing binary: %+v", got)
		}
		old := filepath.Join(f.Home, "velho")
		writeFile(t, old, "binary v-old")
		writeFile(t, mcp, `{"servers":{"vitral":{"type":"stdio","command":"`+old+`"}}}`)
		if got := checkMCP(f.Env); got[0].OK || !strings.Contains(got[0].Text, "versão 0.0.1") {
			t.Fatalf("old binary: %+v", got)
		}
		writeFile(t, mcp, `{"servers":{"vitral":{"type":"stdio","command":"`+InstalledPath(f.Env)+`"}}}`)
		if got := checkMCP(f.Env); !got[0].OK {
			t.Fatalf("ok: %+v", got)
		}
	})
	t.Run("browser check", func(t *testing.T) {
		cfg := doctorConfig(t, f, "")
		cfg.BrowserPath = ""
		res := install.Resolution{Source: install.SourceCache, Version: "120.0.0.1", Path: "/cache/chrome"}
		if c := checkBrowser(cfg, res, nil); !c.OK || !strings.Contains(c.Text, "será baixado no primeiro uso") || !strings.Contains(c.Text, "se o download falhar") {
			t.Fatalf("pending with fallback: %+v", c)
		}
		if c := checkBrowser(cfg, install.Resolution{}, &install.NotFoundError{NotInstalled: true}); !c.OK {
			t.Fatalf("pending without fallback: %+v", c)
		}
		cfg.BrowserPath = "/opt/chrome"
		if c := checkBrowser(cfg, install.Resolution{Source: install.SourceExplicit, Version: "1.2.3.4", Path: "/opt/chrome"}, nil); !c.OK || !strings.Contains(c.Text, "definido em --browser-path") {
			t.Fatalf("explicit: %+v", c)
		}
		if c := checkBrowser(cfg, install.Resolution{}, errors.New("x")); c.OK {
			t.Fatalf("error: %+v", c)
		}
	})
}

func TestSourceLabelAndReporter(t *testing.T) {
	labels := map[install.Source]string{
		install.SourceExplicit: "--browser-path", install.SourceManaged: "Chrome for Testing do Vitral",
		install.SourceCache: "Puppeteer", install.SourceSystem: "--allow-system-browser",
	}
	for src, want := range labels {
		if !strings.Contains(SourceLabel(src), want) {
			t.Errorf("%s: %s", src, SourceLabel(src))
		}
	}
	var buf bytes.Buffer
	NotFoundReporter(&buf, config.Defaults(t.TempDir()))(&install.NotFoundError{Unsupported: true})
	if !strings.Contains(buf.String(), "O Vitral não encontrou um navegador") {
		t.Fatalf("reporter: %s", buf.String())
	}
	if FormatNotFound(errors.New("outro"), "") != "outro" || platformOr("") != "<plataforma>" {
		t.Fatal("fallbacks")
	}
}

func TestMainDoctorAndInstallBrowserErrors(t *testing.T) {
	f := newFakeEnv(t, "linux")
	failLoad := func([]string) (config.Config, error) { return config.Config{}, errors.New("config ruim") }
	if handled, code := Main([]string{"doctor"}, f.Env, failLoad); !handled || code != 1 || !strings.Contains(f.out.String(), "config ruim") {
		t.Fatalf("doctor with bad config: %d %s", code, f.out.String())
	}
	if _, code := Main([]string{"install-browser", "--from"}, f.Env, failLoad); code != 1 {
		t.Fatal("install-browser --from without file must fail")
	}
	if _, code := Main([]string{"help"}, f.Env, failLoad); code != 0 || !strings.Contains(f.out.String(), "vitral setup") {
		t.Fatal("help")
	}
	opts, err := parseSetup([]string{"--global", "-y"})
	if err != nil || !opts.Global || !opts.Yes {
		t.Fatalf("parseSetup: %+v %v", opts, err)
	}
}

func TestInstallEdgeCases(t *testing.T) {
	f := newFakeEnv(t, "linux")
	if err := copyFile(filepath.Join(f.Home, "nada"), filepath.Join(f.Home, "dst")); err == nil {
		t.Fatal("copy of a missing file must fail")
	}
	// Rodando o próprio binário instalado: não copia de novo.
	dst := InstalledPath(f.Env)
	writeFile(t, dst, "binary v-new")
	f.Executable = dst
	res, err := installBinary(f.Env)
	if err != nil || res.Copied {
		t.Fatalf("same path: %+v %v", res, err)
	}
	printInstall(f.Env, res)
	if !strings.Contains(f.out.String(), "já está instalado") {
		t.Fatalf("output: %s", f.out.String())
	}
	// mcp.json quebrado: erro claro, nada gravado.
	bad := filepath.Join(f.Cwd, ".vscode", "mcp.json")
	writeFile(t, bad, "{ quebrado")
	if err := writeMCP(f.Env, bad, ServerEntry{Type: "stdio", Command: dst}); err == nil {
		t.Fatal("broken mcp.json must fail")
	}
	if ParseVersion("vitral") != "?" {
		t.Fatal("ParseVersion")
	}
}

func TestDefaultEnvAndRunCommand(t *testing.T) {
	e, err := DefaultEnv()
	if err != nil || e.Home == "" || e.Executable == "" || e.Run == nil {
		t.Fatalf("DefaultEnv: %+v %v", e, err)
	}
	out, err := runCommand("", "go", "env", "GOOS")
	if err != nil || out == "" {
		t.Fatalf("runCommand: %q %v", out, err)
	}
}
