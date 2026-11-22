//go:build unix

// Package childcmd builds provider commands with literal argument boundaries.
package childcmd

import (
	"context"
	"os/exec"
)

func CommandContext(ctx context.Context, path string, args ...string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, path, args...), nil
}
