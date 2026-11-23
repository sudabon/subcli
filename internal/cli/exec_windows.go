//go:build windows

package cli

import (
	"context"
	"os"

	"github.com/sudabon/subcli/internal/childcmd"
)

// execInteractive launches the official CLI inheriting stdio and waits for completion on Windows.
func execInteractive(binPath string, argv []string, env []string) error {
	var args []string
	if len(argv) > 1 {
		args = argv[1:]
	}
	cmd, err := childcmd.CommandContext(context.Background(), binPath, args...)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	code, err := runForeground(cmd)
	if err != nil {
		return err
	}
	if code != 0 {
		return &ExitError{Code: code, Silent: true}
	}
	return nil
}
