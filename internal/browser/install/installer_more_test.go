package install

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewInstallerDefaults(t *testing.T) {
	i := NewInstaller("/r", "linux64", "", nil)
	if i.Version != PinnedVersion || i.VersionJSON != VersionURL(PinnedVersion) || i.Client == nil || i.VersionOf == nil {
		t.Fatalf("defaults: %+v", i)
	}
	if i.Dir() != filepath.Join("/r", PinnedVersion) {
		t.Fatalf("dir = %s", i.Dir())
	}
}

// DownloadURL: sem mirror pergunta ao JSON oficial; se ele falhar, usa o layout oficial.
func TestDownloadURLFromJSON(t *testing.T) {
	fixture, err := os.ReadFile("testdata/154.0.8037.92.json")
	if err != nil {
		t.Fatal(err)
	}
	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	i, _ := testInstaller(t)
	i.Version, i.Platform, i.VersionJSON = "154.0.8037.92", "mac-arm64", srv.URL
	url, err := i.DownloadURL(context.Background())
	if err != nil || url != ZipURL(DefaultBaseURL, "154.0.8037.92", "mac-arm64") {
		t.Fatalf("from JSON: %s %v", url, err)
	}
	status = http.StatusInternalServerError
	url, err = i.DownloadURL(context.Background())
	if err != nil || url != ZipURL(DefaultBaseURL, "154.0.8037.92", "mac-arm64") {
		t.Fatalf("JSON down should fall back to the official layout: %s %v", url, err)
	}
}

func TestInstallDownloadErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/ok/") && r.Method == http.MethodGet {
			w.Header().Set("Content-Length", "10")
			_, _ = io.WriteString(w, "not a zip!")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	i, ev := testInstaller(t)
	i.Mirror = srv.URL + "/ok"
	if _, err := i.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "open zip") {
		t.Fatalf("a broken zip must fail: %v", err)
	}
	if strings.Join(*ev, ",") != "Download:,Progress:,Unzip:" {
		t.Fatalf("progress events = %v", *ev)
	}
	i.Mirror = srv.URL + "/nada/"
	i.Version = "1.0.0.0"
	if _, err := i.Install(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("404 must fail: %v", err)
	}
	if _, err := os.Stat(i.Dir()); err == nil {
		t.Fatal("failed install must not leave the version folder")
	}
}

func TestRunVersionAndManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if v, err := RunVersion(write("ok", "echo 'Google Chrome for Testing 154.0.8037.92'")); err != nil || v != "154.0.8037.92" {
		t.Fatalf("ok: %q %v", v, err)
	}
	if _, err := RunVersion(write("silent", "true")); err == nil {
		t.Fatal("no version in output must fail")
	}
	if _, err := RunVersion(write("broken", "exit 3")); err == nil {
		t.Fatal("failing binary must fail")
	}
	if _, err := RunVersion(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing binary must fail")
	}
	if err := os.WriteFile(filepath.Join(dir, "154.0.8037.92.manifest"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if v := manifestVersion(dir); v != "154.0.8037.92" {
		t.Fatalf("manifest version = %q", v)
	}
	if v := manifestVersion(t.TempDir()); v != "" {
		t.Fatalf("empty dir = %q", v)
	}
}

func TestSystemCandidatesLinux(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "microsoft-edge" {
			return "/usr/bin/microsoft-edge", nil
		}
		return "", os.ErrNotExist
	}
	got := SystemCandidates("linux", func(string) string { return "" }, look, func() (string, bool) { return "/usr/bin/google-chrome", true })
	if len(got) != 2 || got[0] != "/usr/bin/google-chrome" || got[1] != "/usr/bin/microsoft-edge" {
		t.Fatalf("linux: %v", got)
	}
}
