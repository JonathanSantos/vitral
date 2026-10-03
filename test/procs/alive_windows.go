//go:build windows

package procs

import "syscall"

// stillActive é o código de saída de um processo que ainda roda (STILL_ACTIVE).
const stillActive = 259

// alive abre o processo e lê o código de saída: no Windows não há sinal 0.
func alive(pid int) bool {
	const queryLimitedInformation = 0x1000
	h, err := syscall.OpenProcess(queryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	return syscall.GetExitCodeProcess(h, &code) == nil && code == stillActive
}
