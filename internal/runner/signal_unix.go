//go:build unix

package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type processGroup struct{ proc *os.Process }

func startProcessGroup(cmd *exec.Cmd) (processGroup, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	err := cmd.Start()
	return processGroup{cmd.Process}, err
}

func (processGroup) close() {}

// terminate escalates signals to the child process group:
// SIGINT -> grace -> SIGTERM -> grace -> SIGKILL.
func (g processGroup) terminate(done <-chan struct{}, grace time.Duration) {
	if g.proc == nil || g.proc.Pid <= 0 {
		return
	}
	pgid := -g.proc.Pid

	_ = syscall.Kill(pgid, syscall.SIGINT)
	if waitDoneOrTimer(done, grace) {
		return
	}

	_ = syscall.Kill(pgid, syscall.SIGTERM)
	if waitDoneOrTimer(done, grace) {
		return
	}

	_ = syscall.Kill(pgid, syscall.SIGKILL)
}

func waitDoneOrTimer(done <-chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// ExitCodeFromError returns the child exit status, including 128+signal on Unix.
func ExitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
	}
	return 1
}
