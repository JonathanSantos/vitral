package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// reportName guarda o pacote no nome do arquivo (internal/flow -> internal_flow.json).
func reportName(pkg string) string { return strings.ReplaceAll(pkg, "/", "_") + ".json" }

func pkgFromReport(name string) string {
	return strings.ReplaceAll(strings.TrimSuffix(name, ".json"), "_", "/")
}

// runMutate roda o Gremlins nos pacotes alterados (ou em todos, com -all) do escopo.
func runMutate(args []string) error {
	fs := flag.NewFlagSet("mutate", flag.ContinueOnError)
	thresholds := fs.String("thresholds", ".quality/thresholds.yaml", "thresholds file")
	base := fs.String("base", "main", "base branch/commit to compare with")
	all := fs.Bool("all", false, "mutate every package in scope (nightly)")
	out := fs.String("out", ".quality/out/mutation", "where to write the reports")
	gremlins := fs.String("gremlins", "gremlins", "command that runs gremlins")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile(*thresholds)
	if err != nil {
		return err
	}
	th, err := ParseThresholds(data)
	if err != nil {
		return err
	}
	pkgs := th.Mutation.Packages
	if !*all {
		files, err := changedFiles(*base)
		if err != nil {
			return err
		}
		pkgs = ChangedPackages(files, th.Mutation.Packages)
	}
	if err := os.RemoveAll(*out); err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o750); err != nil {
		return err
	}
	if len(pkgs) == 0 {
		fmt.Println("mutação: nenhum pacote do escopo foi alterado")
		return nil
	}
	coef := th.Mutation.TimeoutCoefficient
	if coef == 0 {
		coef = 20
	}
	cmdParts := strings.Fields(*gremlins)
	for _, pkg := range pkgs {
		fmt.Println("mutação:", pkg)
		report := filepath.Join(*out, reportName(pkg))
		args := append(append([]string{}, cmdParts[1:]...), "unleash", "--integration=false", fmt.Sprintf("--timeout-coefficient=%d", coef), "--output", report, "./"+pkg)
		cmd := exec.Command(cmdParts[0], args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("gremlins %s: %w", pkg, err)
		}
	}
	return nil
}

// changedFiles: diferença para o ponto em que a branch saiu da base, mais o que não foi commitado.
func changedFiles(base string) ([]string, error) {
	mb, err := exec.Command("git", "merge-base", "HEAD", base).Output()
	if err != nil {
		return nil, fmt.Errorf("git merge-base HEAD %s: %w (use -all or -base)", base, err)
	}
	var files []string
	for _, args := range [][]string{
		{"diff", "--name-only", strings.TrimSpace(string(mb))},
		{"ls-files", "--others", "--exclude-standard"},
	} {
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			return nil, err
		}
		files = append(files, strings.Fields(string(out))...)
	}
	return files, nil
}

// loadMutation lê os relatórios gerados pelo mutate.
func loadMutation(dir, exclusionsFile string) ([]MutationResult, error) {
	var exclusions []Exclusion
	data, err := os.ReadFile(exclusionsFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if exclusions, err = ParseExclusions(data); err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []MutationResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []MutationResult
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		r, err := ParseMutation(pkgFromReport(e.Name()), raw, exclusions)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}
