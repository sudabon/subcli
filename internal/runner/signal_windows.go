//go:build windows

package runner

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

func configureProcAttr(_ *exec.Cmd) {}

func terminateProcessGroup(proc *os.Process, done <-chan struct{}, grace time.Duration) {
	if proc == nil {
		return
	}
	_ = proc.Signal(os.Interrupt)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-done:
		return
	case <-timer.C:
		_ = proc.Kill()
	}
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}
