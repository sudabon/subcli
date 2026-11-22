package childcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// npm's cmd-shim ends in one of these two forms. Resolve its target so user
// arguments are passed directly to a native executable without shell parsing.
var (
	nodeTarget = regexp.MustCompile(`(?i)"%_prog%"[ \t]+"%dp0%\\([^"%\r\n]+)"[ \t]+%\*$`)
	exeTarget  = regexp.MustCompile(`(?i)(?:^|\n)"%dp0%\\([^"%\r\n]+\.exe)"[ \t]+%\*$`)
)

func CommandContext(ctx context.Context, path string, args ...string) (*exec.Cmd, error) {
	path, err := exec.LookPath(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bat", ".cmd":
		path, args, err = resolveShim(path, args)
		if err != nil {
			return nil, err
		}
	}
	return exec.CommandContext(ctx, path, args...), nil
}

func resolveShim(path string, args []string) (string, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	dir := filepath.Dir(path)
	shim := strings.TrimSpace(string(data))
	if match := exeTarget.FindStringSubmatch(shim); match != nil {
		return filepath.Join(dir, match[1]), args, nil
	}
	if match := nodeTarget.FindStringSubmatch(shim); match != nil && strings.Contains(strings.ToLower(shim), `set "_prog=node"`) {
		entry, err := filepath.Abs(filepath.Join(dir, match[1]))
		if err != nil {
			return "", nil, err
		}
		node := filepath.Join(dir, "node.exe")
		if _, err := os.Stat(node); os.IsNotExist(err) {
			node, err = exec.LookPath("node.exe")
			if err != nil {
				return "", nil, err
			}
		} else if err != nil {
			return "", nil, err
		}
		return node, append([]string{entry}, args...), nil
	}
	return "", nil, fmt.Errorf("unsupported batch launcher %q: use the official executable or npm shim", path)
}
