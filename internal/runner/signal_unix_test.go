//go:build unix

package runner_test

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/sudabon/subcli/internal/runner"
)

func TestExitCodeFromErrorReportsSignal(t *testing.T) {
	if got := runner.ExitCodeFromError(nil); got != 0 {
		t.Errorf("nil error = %d, want 0", got)
	}
	if got := runner.ExitCodeFromError(exec.Command("/bin/sh", "-c", "exit 3").Run()); got != 3 {
		t.Errorf("exit 3 = %d, want 3", got)
	}
	err := exec.Command("/bin/sh", "-c", "kill -TERM $$").Run()
	if got, want := runner.ExitCodeFromError(err), 128+int(syscall.SIGTERM); got != want {
		t.Errorf("SIGTERM exit = %d, want %d (err=%v)", got, want, err)
	}
}
