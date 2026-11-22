package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sudabon/subcli/internal/config"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/session"
)

// fakeCLIEnv gives the shell fixtures access to their utilities without inheriting credentials.
func fakeCLIEnv() []string {
	return []string{"PATH=" + os.Getenv("PATH")}
}

func testDeps(t *testing.T) Deps {
	t.Helper()
	root := t.TempDir()
	return Deps{
		LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
		DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true, StdoutIsTTY: true} },
		LookPath:      fakeBinLookPath(t),
		ExecReplace:   func(string, []string, []string) error { return errors.New("unexpected exec") },
		RunInteractive: func(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) (int, error) {
			return 1, errors.New("unexpected interactive command")
		},
		RunCapture: func(context.Context, string, []string, []string) (string, error) {
			return "", errors.New("unexpected capture")
		},
		Environ:        fakeCLIEnv,
		Getwd:          func() (string, error) { return root, nil },
		OpenStore:      func() (*session.Store, error) { return session.Open(filepath.Join(root, "sessions.db")) },
		Now:            func() time.Time { return time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC) },
		ComplianceText: defaultComplianceText,
		Stdin:          strings.NewReader(""),
		Stdout:         io.Discard,
		Stderr:         io.Discard,
		Logger:         logging.NewWriter(io.Discard),
	}
}
