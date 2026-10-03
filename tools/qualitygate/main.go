// Comando qualitygate compara cobertura e demais métricas com .quality/thresholds.yaml.
// Ferramenta de desenvolvimento: nunca é importada pelo binário do Vitral.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	thresholds := flag.String("thresholds", ".quality/thresholds.yaml", "thresholds file")
	unit := flag.String("unit", "", "comma separated coverprofiles from unit tests")
	combined := flag.String("combined", "", "comma separated coverprofiles for combined coverage (unit + integration + e2e)")
	tokensFile := flag.String("tokens", "", "token report from the e2e (JSON key -> tokens)")
	tokenBase := flag.String("token-baseline", ".quality/token-baseline.json", "versioned token baseline")
	flag.Parse()

	if err := run(*thresholds, split(*unit), split(*combined), *tokensFile, *tokenBase); err != nil {
		fmt.Fprintln(os.Stderr, "qualitygate:", err)
		os.Exit(1)
	}
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func run(thresholdsPath string, unitFiles, combinedFiles []string, tokensFile, tokenBase string) error {
	data, err := os.ReadFile(thresholdsPath)
	if err != nil {
		return err
	}
	th, err := ParseThresholds(data)
	if err != nil {
		return err
	}
	pkgs, err := modulePackages(th.Module)
	if err != nil {
		return err
	}
	in := Input{Thresholds: th, Packages: pkgs, Now: time.Now()}
	if in.Unit, err = load(th.Module, unitFiles); err != nil {
		return err
	}
	// A cobertura combinada inclui os unitários.
	if len(combinedFiles) > 0 {
		if in.Combined, err = load(th.Module, append(unitFiles, combinedFiles...)); err != nil {
			return err
		}
	}
	if tokensFile != "" {
		if in.TokenNow, err = readTokens(tokensFile, true); err != nil {
			return err
		}
		if in.TokenBase, err = readTokens(tokenBase, false); err != nil {
			return err
		}
	}
	if in.Quarantine, err = scanQuarantines("."); err != nil {
		return err
	}
	report := Evaluate(in)
	fmt.Print(Format(report))
	if !report.OK() {
		os.Exit(1)
	}
	return nil
}

func load(module string, files []string) (map[string]Coverage, error) {
	if len(files) == 0 {
		return nil, nil
	}
	readers := make([]io.Reader, 0, len(files))
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		defer fh.Close()
		readers = append(readers, fh)
	}
	return ParseProfiles(module, readers...)
}

// readTokens lê um relatório de tokens; a base pode não existir ainda.
func readTokens(file string, required bool) (map[string]int, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) && !required {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return out, nil
}

// modulePackages lista os pacotes com código Go (go list), relativos ao módulo.
func modulePackages(module string) ([]string, error) {
	out, err := exec.Command("go", "list", "./...").Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	var pkgs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if rel := strings.TrimPrefix(line, module+"/"); rel != line {
			pkgs = append(pkgs, rel)
		}
	}
	return pkgs, nil
}

func scanQuarantines(root string) ([]Quarantine, error) {
	var out []Quarantine
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		out = append(out, FindQuarantines(p, f)...)
		return nil
	})
	return out, err
}
