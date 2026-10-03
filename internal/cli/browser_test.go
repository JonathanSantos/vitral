package cli

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/browser/install"
	"github.com/JonathanSantos/vitral/internal/config"
)

func TestFormatNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "download blocked, no system opt-in",
			err:  &install.NotFoundError{InstallErr: errors.New("HTTP 403")},
			want: []string{
				"não encontrou um navegador", "falhou: HTTP 403", "não é usado sem permissão",
				"VITRAL_BROWSER_MIRROR", "vitral install-browser --from", "--allow-system-browser", "vitral doctor",
				"chrome-for-testing-public/" + install.PinnedVersion + "/linux64/chrome-linux64.zip",
			},
		},
		{
			name: "unsupported platform with system allowed",
			err:  &install.NotFoundError{Unsupported: true, SystemAllowed: true},
			want: []string{"Não existe Chrome for Testing", "Não achei Chrome nem Edge"},
		},
		{
			name: "broken explicit path",
			err:  &install.NotFoundError{ExplicitPath: "/opt/chrome", ExplicitErr: errors.New("exit 1")},
			want: []string{"--browser-path", "/opt/chrome", "remova --browser-path"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatNotFound(tt.err, "linux64")
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
		})
	}
}

func TestParseInstallBrowser(t *testing.T) {
	zipFile := filepath.Join(t.TempDir(), "c.zip")
	if err := os.WriteFile(zipFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, rest, err := parseInstallBrowser([]string{"--browser-mirror", "https://m", "--from", zipFile})
	if err != nil || path != zipFile || len(rest) != 2 {
		t.Fatalf("%q %v %v", path, rest, err)
	}
	if _, _, err := parseInstallBrowser([]string{"--from"}); err == nil {
		t.Fatal("missing file must fail")
	}
	if _, _, err := parseInstallBrowser([]string{"--from=/nao/existe.zip"}); err == nil {
		t.Fatal("nonexistent file must fail")
	}
}

// install-browser --from de ponta a ponta, com um "Chrome" de mentira que responde --version.
func TestInstallBrowserFromZip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake browser is a shell script")
	}
	platform, ok := install.Platform(runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("no Chrome for Testing build for this platform")
	}
	zipPath := filepath.Join(t.TempDir(), "chrome.zip")
	f, _ := os.Create(zipPath)
	w := zip.NewWriter(f)
	h := &zip.FileHeader{Name: install.ExecutableRel(platform), Method: zip.Deflate}
	h.SetMode(0o755)
	fw, _ := w.CreateHeader(h)
	if _, err := fw.Write([]byte("#!/bin/sh\necho 'Google Chrome for Testing " + install.PinnedVersion + "'\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	fe := newFakeEnv(t, runtime.GOOS)
	browsers := filepath.Join(fe.Home, ".vitral", "browsers")
	load := func(args []string) (config.Config, error) {
		cfg := config.Defaults(fe.Home)
		cfg.BrowsersDir = browsers
		return cfg, nil
	}
	handled, code := Main([]string{"install-browser", "--from", zipPath}, fe.Env, load)
	if !handled || code != 0 {
		t.Fatalf("code=%d out=%s", code, fe.out.String())
	}
	exe := filepath.Join(browsers, install.PinnedVersion, filepath.FromSlash(install.ExecutableRel(platform)))
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("not installed: %v\n%s", err, fe.out.String())
	}
	if !strings.Contains(fe.out.String(), "navegador instalado") {
		t.Fatalf("output: %s", fe.out.String())
	}

	// Rodar de novo sem --from: já instalado, nada a baixar.
	fe.out.Reset()
	if _, code := Main([]string{"install-browser"}, fe.Env, load); code != 0 || !strings.Contains(fe.out.String(), "já está instalado") {
		t.Fatalf("second run: %d %s", code, fe.out.String())
	}
}
