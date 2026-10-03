// Package install baixa, instala e escolhe o navegador do Vitral.
// O padrão é um Chrome for Testing próprio, em versão fixada; o Chrome do usuário só com opt-in.
package install

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// PinnedVersion é a versão do Chrome for Testing que o Vitral usa. Trocar aqui atualiza todo mundo.
const PinnedVersion = "154.0.8037.92"

// DefaultBaseURL é a raiz oficial dos downloads; mirrors seguem o mesmo layout de paths.
const DefaultBaseURL = "https://storage.googleapis.com/chrome-for-testing-public"

// versionEndpoint é o JSON oficial com as URLs de download de uma versão.
const versionEndpoint = "https://googlechromelabs.github.io/chrome-for-testing/%s.json"

// Platform traduz GOOS/GOARCH para a plataforma do Chrome for Testing. ok=false: não há build.
func Platform(goos, goarch string) (string, bool) {
	switch goos + "/" + goarch {
	case "darwin/arm64":
		return "mac-arm64", true
	case "darwin/amd64":
		return "mac-x64", true
	case "linux/amd64":
		return "linux64", true
	case "linux/arm64":
		return "linux-arm64", true
	case "windows/amd64":
		return "win64", true
	}
	return "", false
}

// ZipURL monta a URL do zip no layout oficial (usado com mirror ou se o JSON estiver inacessível).
func ZipURL(base, version, platform string) string {
	return strings.TrimRight(base, "/") + "/" + path.Join(version, platform, "chrome-"+platform+".zip")
}

// VersionURL é o endpoint JSON da versão.
func VersionURL(version string) string {
	return fmt.Sprintf(versionEndpoint, version)
}

type versionDownloads struct {
	Version   string `json:"version"`
	Downloads struct {
		Chrome []struct {
			Platform string `json:"platform"`
			URL      string `json:"url"`
		} `json:"chrome"`
	} `json:"downloads"`
}

// ParseDownloadURL lê o JSON de uma versão e devolve a URL do Chrome para a plataforma.
func ParseDownloadURL(data []byte, platform string) (string, error) {
	var v versionDownloads
	if err := json.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("invalid Chrome for Testing JSON: %w", err)
	}
	for _, d := range v.Downloads.Chrome {
		if d.Platform == platform && d.URL != "" {
			return d.URL, nil
		}
	}
	return "", fmt.Errorf("Chrome for Testing %s has no build for %s", v.Version, platform)
}

// ExecutableRel é o caminho do executável dentro do zip extraído.
func ExecutableRel(platform string) string {
	top := "chrome-" + platform
	switch {
	case strings.HasPrefix(platform, "mac"):
		return path.Join(top, "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing")
	case strings.HasPrefix(platform, "win"):
		return path.Join(top, "chrome.exe")
	}
	return path.Join(top, "chrome")
}

// ParseVersion extrai "154.0.8037.92" de "Google Chrome for Testing 154.0.8037.92".
func ParseVersion(out string) string {
	for _, f := range strings.Fields(out) {
		if len(versionParts(f)) >= 2 {
			return f
		}
	}
	return ""
}

func versionParts(v string) []int {
	var parts []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		parts = append(parts, n)
	}
	return parts
}

// CompareVersions compara versões numéricas com pontos ("120.0.1" < "121.0.0").
func CompareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
