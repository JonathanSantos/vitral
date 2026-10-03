// Package cli tem os comandos para pessoas: ajuda, setup, doctor e version (docs/UX-TASKS.md, UX-4).
// O servidor MCP continua sendo o modo padrão quando o Vitral roda sem terminal (pelo VS Code).
package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"
)

// Env é tudo o que os comandos usam do mundo externo, para testar sem mexer na máquina real.
type Env struct {
	Home       string
	Cwd        string
	GOOS       string
	Executable string
	Getenv     func(string) string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	// Interactive indica stdin num terminal (dá para perguntar e esperar resposta).
	Interactive bool
	// Run executa um programa e devolve a saída (usado para ler versões e consultar o git).
	Run func(dir, name string, args ...string) (string, error)
}

// DefaultEnv monta o ambiente real do processo.
func DefaultEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Env{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return Env{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return Env{
		Home: home, Cwd: cwd, GOOS: runtime.GOOS, Executable: exe, Getenv: os.Getenv,
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Interactive: IsTerminal(os.Stdin),
		Run:         runCommand,
	}, nil
}

// IsTerminal diz se o arquivo é um terminal de verdade (e não um pipe, como quando o VS Code
// inicia o Vitral, nem /dev/null, que também é "char device").
func IsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func runCommand(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// readLine lê byte a byte até o fim da linha: sem buffer, várias perguntas seguidas não perdem respostas.
func readLine(r io.Reader) string {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n == 1 && buf[0] != '\n' {
			b.WriteByte(buf[0])
		}
		if err != nil || (n == 1 && buf[0] == '\n') {
			return b.String()
		}
	}
}

// confirm pergunta sim/não. Sem terminal, a resposta é não.
func (e Env) confirm(question string) bool {
	if !e.Interactive {
		return false
	}
	_, _ = io.WriteString(e.Stdout, question)
	answer := strings.ToLower(strings.TrimSpace(readLine(e.Stdin)))
	return answer == answerYes || answer == "sim" || answer == "y" || answer == "yes"
}
