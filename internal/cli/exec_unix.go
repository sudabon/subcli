//go:build unix

package cli

import "syscall"

// execInteractive replaces the current subcli process with the official CLI binary
// so subcli does not sit in the interactive stdio path.
func execInteractive(binPath string, argv []string, env []string) error {
	return syscall.Exec(binPath, argv, env)
}
