package install

import (
	"os"
	"strings"
	"testing"
)

// FuzzParseDownloadURL: JSON qualquer não derruba o parser; sucesso sempre traz uma URL.
func FuzzParseDownloadURL(f *testing.F) {
	if data, err := os.ReadFile("testdata/154.0.8037.92.json"); err == nil {
		f.Add(string(data), "mac-arm64")
	}
	f.Add(`{"version":"1","downloads":{"chrome":[{"platform":"linux64","url":"u"}]}}`, "linux64")
	f.Add(`{"downloads":{"chrome":null}}`, "win64")
	f.Add(`[]`, "")
	f.Fuzz(func(t *testing.T, data, platform string) {
		url, err := ParseDownloadURL([]byte(data), platform)
		if err == nil && url == "" {
			t.Fatal("success without URL")
		}
		_ = ParseVersion(data)
		_ = CompareVersions(platform, data)
		if strings.Contains(platform, "\x00") {
			return
		}
	})
}
