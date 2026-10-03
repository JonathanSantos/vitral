package install

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type zipEntry struct {
	name    string
	content string
	mode    os.FileMode
}

func makeZip(t *testing.T, entries []zipEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chrome.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		fw, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

// chromeZip simula o zip do Chrome for Testing para linux64; o "executável" contém a versão.
func chromeZip(t *testing.T, version string) string {
	return makeZip(t, []zipEntry{
		{name: "chrome-linux64/", mode: os.ModeDir | 0o755},
		{name: "chrome-linux64/chrome", content: "chrome " + version, mode: 0o755},
		{name: "chrome-linux64/libfoo.so.1", content: "lib", mode: 0o644},
		{name: "chrome-linux64/libfoo.so", content: "libfoo.so.1", mode: os.ModeSymlink | 0o777},
	})
}

type events []string

func (e *events) Println(args ...any) { *e = append(*e, fmt.Sprint(args[0])) }

func testInstaller(t *testing.T) (*Installer, *events) {
	ev := &events{}
	i := &Installer{
		Root: filepath.Join(t.TempDir(), "browsers"), Version: "154.0.8037.92", Platform: "linux64",
		Client: http.DefaultClient, Progress: ev,
		VersionOf: func(exe string) (string, error) {
			data, err := os.ReadFile(exe)
			if err != nil {
				return "", err
			}
			return ParseVersion(string(data)), nil
		},
	}
	return i, ev
}

func TestInstallZip(t *testing.T) {
	i, ev := testInstaller(t)
	exe, err := i.InstallZip(chromeZip(t, "154.0.8037.92"))
	if err != nil {
		t.Fatal(err)
	}
	if exe != filepath.Join(i.Root, "154.0.8037.92", "chrome-linux64", "chrome") {
		t.Fatalf("exe = %s", exe)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Fatal("executable bit lost")
	}
	if runtime.GOOS != "windows" {
		if target, err := os.Readlink(filepath.Join(filepath.Dir(exe), "libfoo.so")); err != nil || target != "libfoo.so.1" {
			t.Fatalf("symlink not preserved: %q %v", target, err)
		}
	}
	if got := strings.Join(*ev, ","); got != "Unzip:,Downloaded:" {
		t.Fatalf("progress events = %s", got)
	}
	leftovers, _ := filepath.Glob(filepath.Join(i.Root, ".staging-*"))
	if len(leftovers) != 0 {
		t.Fatalf("staging not cleaned: %v", leftovers)
	}
}

func TestInstallZipRejects(t *testing.T) {
	tests := []struct {
		name string
		zip  func(t *testing.T) string
		want string
	}{
		{"wrong version", func(t *testing.T) string { return chromeZip(t, "120.0.0.1") }, "expects 154.0.8037.92"},
		{"wrong platform", func(t *testing.T) string {
			return makeZip(t, []zipEntry{{name: "chrome-win64/chrome.exe", content: "chrome 154.0.8037.92", mode: 0o755}})
		}, "not Chrome for Testing for linux64"},
		{"zip slip", func(t *testing.T) string {
			return makeZip(t, []zipEntry{{name: "../../evil", content: "x", mode: 0o644}})
		}, "unsafe path"},
		{"symlink escaping", func(t *testing.T) string {
			return makeZip(t, []zipEntry{{name: "chrome-linux64/x", content: "../../../etc/passwd", mode: os.ModeSymlink | 0o777}})
		}, "unsafe symlink"},
		{"does not run", func(t *testing.T) string {
			return makeZip(t, []zipEntry{{name: "chrome-linux64/chrome", content: "garbage", mode: 0o755}})
		}, "expects"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i, _ := testInstaller(t)
			_, err := i.InstallZip(tt.zip(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if _, err := os.Stat(i.Dir()); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("a failed install must not leave the version folder")
			}
		})
	}
}

func TestCleanupKeepsCurrentAndPrevious(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"150.0.1.1", "152.0.1.1", "153.0.2.2", "154.0.8037.92", ".staging-abc", "notes"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	Cleanup(root, "154.0.8037.92")
	entries, _ := os.ReadDir(root)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if got := strings.Join(names, ","); got != "153.0.2.2,154.0.8037.92,notes" {
		t.Fatalf("after cleanup: %s", got)
	}
}

// Mirror falso com o mesmo layout de paths do Chrome for Testing.
func TestInstallFromMirrorAndProbe(t *testing.T) {
	zipPath := chromeZip(t, "154.0.8037.92")
	var heads int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cft/154.0.8037.92/linux64/chrome-linux64.zip" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodHead {
			heads++
		}
		http.ServeFile(w, r, zipPath)
	}))
	defer srv.Close()

	i, ev := testInstaller(t)
	i.Mirror = srv.URL + "/cft/"
	url, err := i.Probe(context.Background())
	if err != nil || heads != 1 || !strings.HasSuffix(url, "chrome-linux64.zip") {
		t.Fatalf("probe: %s %v heads=%d", url, err, heads)
	}
	exe, err := i.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.Join(*ev, ","), "Download:") {
		t.Fatalf("progress events = %v", *ev)
	}

	i.Mirror = srv.URL + "/errado"
	if _, err := i.Probe(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("blocked probe: %v", err)
	}
}
