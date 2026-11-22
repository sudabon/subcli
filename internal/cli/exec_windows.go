//go:build windows

package cli

import (
	"errors"
	"os"
	"os/exec"
)

// execInteractive launches the official CLI inheriting stdio and waits for completion on Windows.
func execInteractive(binPath string, argv []string, env []string) error {
	var args []string
	if len(argv) > 1 {
		args = argv[1:]
	}
	cmd := exec.Command(binPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	err := cmd.Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitError{Code: exitErr.ExitCode(), Silent: true}
	}
	return err
}
