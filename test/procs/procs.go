// Package procs confere que processos lançados pelos testes terminaram (docs/TESTING.md, seção 9).
package procs

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Alive diz se o processo ainda existe (no Windows não há sinal 0: devolve false).
func Alive(pid int) bool {
	if pid <= 0 || runtime.GOOS == "windows" {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// WithArg lista PIDs cuja linha de comando contém arg (ex.: o perfil temporário do teste).
func WithArg(arg string) []string {
	if runtime.GOOS == "windows" || arg == "" {
		return nil
	}
	out, err := exec.Command("pgrep", "-f", arg).Output()
	if err != nil {
		return nil // pgrep sai com 1 quando não acha nada
	}
	return strings.Fields(string(out))
}

// WaitGone espera os processos sumirem; devolve erro descrevendo o que sobrou.
func WaitGone(timeout time.Duration, pids []int, arg string) error {
	deadline := time.Now().Add(timeout)
	for {
		var alive []int
		for _, p := range pids {
			if Alive(p) {
				alive = append(alive, p)
			}
		}
		leftovers := WithArg(arg)
		if len(alive) == 0 && len(leftovers) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("processes still running: pids %v, matching %q: %v", alive, arg, leftovers)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
