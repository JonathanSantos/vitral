package procs

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestAliveAndWaitGone(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Fatal("the test process itself should be alive")
	}
	if Alive(0) || Alive(-1) {
		t.Fatal("invalid pids are never alive")
	}
	name, args := "sleep", []string{"30"}
	if runtime.GOOS == "windows" {
		name, args = "ping", []string{"-n", "30", "127.0.0.1"}
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Skipf("no %s to start: %v", name, err)
	}
	pid := cmd.Process.Pid
	if err := WaitGone(200*time.Millisecond, []int{pid}, ""); err == nil {
		t.Fatal("a running process should be reported")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if err := WaitGone(5*time.Second, []int{pid}, ""); err != nil {
		t.Fatalf("killed process still reported: %v", err)
	}
}
