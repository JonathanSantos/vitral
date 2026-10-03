package install

import (
	"os"
	"strings"
	"testing"
)

func TestPlatform(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
		ok                 bool
	}{
		{"darwin", "arm64", "mac-arm64", true},
		{"darwin", "amd64", "mac-x64", true},
		{"linux", "amd64", "linux64", true},
		{"linux", "arm64", "linux-arm64", true},
		{"windows", "amd64", "win64", true},
		{"windows", "arm64", "", false},
		{"freebsd", "amd64", "", false},
		{"linux", "386", "", false},
	}
	for _, tt := range tests {
		got, ok := Platform(tt.goos, tt.goarch)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Platform(%s,%s) = %q,%v", tt.goos, tt.goarch, got, ok)
		}
	}
}

func TestParseDownloadURLFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/154.0.8037.92.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"linux64", "linux-arm64", "mac-arm64", "mac-x64", "win64"} {
		url, err := ParseDownloadURL(data, platform)
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		if want := ZipURL(DefaultBaseURL, "154.0.8037.92", platform); url != want {
			t.Errorf("%s: url %s, want the official layout %s", platform, url, want)
		}
	}
	if _, err := ParseDownloadURL(data, "solaris"); err == nil || !strings.Contains(err.Error(), "no build for solaris") {
		t.Fatalf("unknown platform: %v", err)
	}
	if _, err := ParseDownloadURL([]byte("{"), "linux64"); err == nil {
		t.Fatal("broken JSON must fail")
	}
}

func TestURLsAndPaths(t *testing.T) {
	if got := ZipURL("https://mirror.corp/cft/", "1.2.3.4", "win64"); got != "https://mirror.corp/cft/1.2.3.4/win64/chrome-win64.zip" {
		t.Fatalf("mirror url = %s", got)
	}
	if VersionURL("1.2.3.4") != "https://googlechromelabs.github.io/chrome-for-testing/1.2.3.4.json" {
		t.Fatal("version endpoint")
	}
	tests := map[string]string{
		"linux64":   "chrome-linux64/chrome",
		"win64":     "chrome-win64/chrome.exe",
		"mac-arm64": "chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
	}
	for platform, want := range tests {
		if got := ExecutableRel(platform); got != want {
			t.Errorf("ExecutableRel(%s) = %s", platform, got)
		}
	}
}

func TestVersions(t *testing.T) {
	if ParseVersion("Google Chrome for Testing 154.0.8037.92 \n") != "154.0.8037.92" || ParseVersion("Chromium 120.0.6099.0 built on Debian") != "120.0.6099.0" || ParseVersion("nada") != "" {
		t.Fatal("ParseVersion")
	}
	tests := []struct {
		a, b string
		want int
	}{
		{"120.0.1", "121.0.0", -1},
		{"121.0.10", "121.0.9", 1},
		{"154.0.8037.92", "154.0.8037.92", 0},
		{"1.2", "1.2.0", 0},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%s,%s)=%d", tt.a, tt.b, got)
		}
	}
}
