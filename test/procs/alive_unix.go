//go:build !windows

package procs

import "syscall"

// alive usa o sinal 0: só confere se o processo existe.
func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }
