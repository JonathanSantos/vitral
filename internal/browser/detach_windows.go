//go:build windows

package browser

import (
	"os/exec"
	"syscall"
)

// detach solta o navegador do console: fechar o terminal não fecha a janela da pessoa.
func detach(cmd *exec.Cmd) {
	const detachedProcess, newProcessGroup = 0x00000008, 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | newProcessGroup}
}
