package install

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEnv: arquivos que existem, versões que cada um informa ao rodar e o resultado do download.
type fakeEnv struct {
	files      map[string]string // caminho -> versão ("" = existe mas não roda)
	installErr error
	installed  bool
}

func (f *fakeEnv) env(e Env) Env {
	e.Exists = func(p string) bool { _, ok := f.files[p]; return ok }
	e.Version = func(p string) (string, error) {
		v, ok := f.files[p]
		if !ok || v == "" {
			return "", errors.New("does not run")
		}
		return v, nil
	}
	return e
}

func TestResolve(t *testing.T) {
	const managedPath = "/home/u/.vitral/browsers/154/chrome"
	install := func(f *fakeEnv) func() (string, error) {
		return func() (string, error) {
			f.installed = true
			if f.installErr != nil {
				return "", f.installErr
			}
			f.files[managedPath] = "154.0.8037.92"
			return managedPath, nil
		}
	}
	caches := []string{"/pp/chrome/linux-120/chrome-linux64/chrome", "/pw/chromium-1100/chrome-linux/chrome", "/pw/chromium-1200/chrome-linux/chrome"}
	system := []string{"/usr/bin/google-chrome", "/usr/bin/microsoft-edge"}

	tests := []struct {
		name        string
		files       map[string]string
		installErr  error
		cfg         Env
		dry         bool
		wantSource  Source
		wantPath    string
		wantInstall bool
		wantErr     string
	}{
		{
			name:       "explicit path wins",
			files:      map[string]string{"/opt/chrome": "130.0.1.1", managedPath: "154.0.8037.92"},
			cfg:        Env{ExplicitPath: "/opt/chrome", Supported: true},
			wantSource: SourceExplicit, wantPath: "/opt/chrome",
		},
		{
			name:    "broken explicit path is an error, not a silent fallback",
			files:   map[string]string{managedPath: "154.0.8037.92"},
			cfg:     Env{ExplicitPath: "/opt/missing", Supported: true},
			wantErr: "browser at /opt/missing does not run",
		},
		{
			name:       "managed already installed",
			files:      map[string]string{managedPath: "154.0.8037.92", caches[0]: "120.0.1.1"},
			cfg:        Env{Supported: true},
			wantSource: SourceManaged, wantPath: managedPath,
		},
		{
			name:       "managed missing is downloaded",
			files:      map[string]string{},
			cfg:        Env{Supported: true},
			wantSource: SourceManaged, wantPath: managedPath, wantInstall: true,
		},
		{
			name:       "download fails: newest cache that runs",
			files:      map[string]string{caches[0]: "120.0.6099.0", caches[1]: "", caches[2]: "121.0.6167.0"},
			installErr: errors.New("proxy blocked"),
			cfg:        Env{Supported: true},
			wantSource: SourceCache, wantPath: caches[2], wantInstall: true,
		},
		{
			name:       "unsupported platform skips straight to caches",
			files:      map[string]string{caches[0]: "120.0.6099.0"},
			cfg:        Env{Supported: false},
			wantSource: SourceCache, wantPath: caches[0],
		},
		{
			name:       "system browser is never used without opt-in",
			files:      map[string]string{system[0]: "140.0.1.1"},
			installErr: errors.New("offline"),
			cfg:        Env{Supported: true},
			wantErr:    "system browser not allowed",
		},
		{
			name:       "system browser with opt-in, first that runs",
			files:      map[string]string{system[0]: "", system[1]: "140.0.1.1"},
			installErr: errors.New("offline"),
			cfg:        Env{Supported: true, AllowSystem: true},
			wantSource: SourceSystem, wantPath: system[1], wantInstall: true,
		},
		{
			name:    "dry mode (doctor) does not download",
			files:   map[string]string{},
			cfg:     Env{Supported: true},
			dry:     true,
			wantErr: "Chrome for Testing not downloaded yet",
		},
		{
			name:       "nothing at all",
			files:      map[string]string{},
			installErr: errors.New("HTTP 403"),
			cfg:        Env{Supported: true, AllowSystem: true},
			wantErr:    "download failed: HTTP 403",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeEnv{files: tt.files, installErr: tt.installErr}
			e := tt.cfg
			e.ManagedPath, e.CachePaths, e.SystemPaths = managedPath, caches, system
			if !tt.dry {
				e.Install = install(f)
			}
			r, err := Resolve(f.env(e))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				var nf *NotFoundError
				if !errors.As(err, &nf) {
					t.Fatalf("want *NotFoundError, got %T", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.Source != tt.wantSource || r.Path != tt.wantPath || r.Reason == "" || r.Version == "" {
				t.Fatalf("got %+v", r)
			}
			if f.installed != tt.wantInstall {
				t.Fatalf("installed = %v", f.installed)
			}
		})
	}
}

func TestCacheCandidates(t *testing.T) {
	dirs := map[string][]string{
		filepath.Join("/h", ".cache", "puppeteer", "chrome", "*"):               {"/h/.cache/puppeteer/chrome/linux-131.0.6778.204"},
		filepath.Join("/h", ".cache", "ms-playwright", "chromium-*"):            {"/h/.cache/ms-playwright/chromium-1148"},
		filepath.Join("/h", "Library", "Caches", "ms-playwright", "chromium-*"): {"/h/Library/Caches/ms-playwright/chromium-1148"},
	}
	glob := func(p string) ([]string, error) { return dirs[p], nil }
	none := func(string) string { return "" }

	linux := CacheCandidates("linux", "/h", none, glob)
	want := filepath.Join("/h/.cache/puppeteer/chrome/linux-131.0.6778.204", "chrome-linux64", "chrome")
	if !contains(linux, want) || !contains(linux, filepath.Join("/h/.cache/ms-playwright/chromium-1148", "chrome-linux", "chrome")) {
		t.Fatalf("linux candidates missing: %v", linux)
	}
	for _, c := range linux {
		if strings.Contains(c, "firefox") || strings.Contains(c, "webkit") || strings.Contains(c, "headless_shell") {
			t.Fatalf("non-CDP browser in candidates: %s", c)
		}
	}
	mac := CacheCandidates("darwin", "/h", none, glob)
	if !contains(mac, filepath.Join("/h/Library/Caches/ms-playwright/chromium-1148", "chrome-mac", "Chromium.app", "Contents", "MacOS", "Chromium")) {
		t.Fatalf("mac candidates: %v", mac)
	}
	custom := CacheCandidates("linux", "/h", func(k string) string {
		return map[string]string{"PUPPETEER_CACHE_DIR": "/pp", "PLAYWRIGHT_BROWSERS_PATH": "/pw"}[k]
	}, func(p string) ([]string, error) { return []string{p}, nil })
	if !strings.HasPrefix(custom[0], "/pp") || !contains(custom, filepath.Join("/pw", "chromium-*", "chrome-linux", "chrome")) {
		t.Fatalf("env overrides ignored: %v", custom[:2])
	}
}

func TestSystemCandidates(t *testing.T) {
	rod := func() (string, bool) { return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", true }
	noLook := func(string) (string, error) { return "", errors.New("nope") }
	got := SystemCandidates("darwin", func(string) string { return "" }, noLook, rod)
	if len(got) != 2 || !strings.Contains(got[1], "Microsoft Edge") {
		t.Fatalf("darwin: %v", got)
	}
	win := SystemCandidates("windows", func(k string) string {
		return map[string]string{"ProgramFiles": `C:\PF`}[k]
	}, noLook, func() (string, bool) { return "", false })
	if len(win) != 1 || !strings.HasSuffix(win[0], "msedge.exe") {
		t.Fatalf("windows: %v", win)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
