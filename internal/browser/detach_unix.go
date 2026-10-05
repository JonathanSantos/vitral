//go:build !windows

package browser

import (
	"os/exec"
	"syscall"
)

// detach solta o navegador do terminal: fechar o terminal não fecha a janela da pessoa.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
