package runner_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/claude"
	"github.com/sudabon/subcli/internal/provider/codex"
	"github.com/sudabon/subcli/internal/runner"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found")
		}
		dir = parent
	}
}

func fakeEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	fakeBin := filepath.Join(repoRoot(t), "testdata", "bin")
	path := fakeBin + string(os.PathListSeparator) + os.Getenv("PATH")
	env := append([]string{}, os.Environ()...)
	env = provider.SetEnv(env, "PATH", path)
	for _, kv := range extra {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			env = provider.SetEnv(env, k, v)
		}
	}
	return env
}

func TestRunner_LongLineOver64KB(t *testing.T) {
	const payloadBytes = 512 * 1024 // 512KB (> 64KB bufio.Scanner default limit)
	fakeBin := filepath.Join(repoRoot(t), "testdata", "bin", "claude")

	var texts []string
	res, err := runner.Run(context.Background(), runner.Options{
		BinPath:  fakeBin,
		Provider: claude.New(""),
		Request:  provider.RunRequest{Prompt: "big", Perm: provider.PermReadOnly},
		Auth:     provider.AuthSubscription,
		BaseEnv: fakeEnv(t,
			"FAKE_CLI_MODE=longline",
			"FAKE_CLI_LONG_LINE_BYTES=524288",
		),
		OnEvent: func(ev event.Event) {
			if ev.Kind == event.Text {
				texts = append(texts, ev.Text)
			}
		},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0 (stderr=%q)", res.ExitCode, res.Stderr)
	}
	got := strings.Join(texts, "")
	if len(got) != payloadBytes {
		t.Fatalf("text length = %d, want %d", len(got), payloadBytes)
	}
}

func TestRunner_Stderr64KBRingBufferAndExitCodePropagation(t *testing.T) {
	fakeBin := filepath.Join(repoRoot(t), "testdata", "bin", "codex")
	var logBuf bytes.Buffer
	logger := logging.NewWriter(&logBuf)

	res, err := runner.Run(context.Background(), runner.Options{
		BinPath:  fakeBin,
		Provider: codex.New(""),
		Request:  provider.RunRequest{Prompt: "fail", Perm: provider.PermReadOnly},
		Auth:     provider.AuthSubscription,
		BaseEnv: fakeEnv(t,
			"FAKE_CLI_MODE=stderr_overflow",
			"FAKE_CLI_EXIT_CODE=7",
		),
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", res.ExitCode)
	}
	if !res.StderrTruncated {
		t.Errorf("expected StderrTruncated=true for >64KB stderr")
	}
	if len(res.Stderr) != runner.DefaultStderrCap {
		t.Errorf("len(Stderr) = %d, want %d", len(res.Stderr), runner.DefaultStderrCap)
	}
	if !strings.Contains(res.Stderr, "STDERR_TAIL_MARKER_END") {
		t.Errorf("Stderr missing tail marker")
	}
	if !strings.Contains(logBuf.String(), "STDERR_TAIL_MARKER_END") {
		t.Errorf("logger output missing stderr tail")
	}
}

func TestRunner_ChildEnvSanitization(t *testing.T) {
	fakeBin := filepath.Join(repoRoot(t), "testdata", "bin", "claude")
	envFile := filepath.Join(t.TempDir(), "env.txt")

	res, err := runner.Run(context.Background(), runner.Options{
		BinPath:  fakeBin,
		Provider: claude.New("MY_ANTHROPIC_KEY"),
		Request:  provider.RunRequest{Prompt: "check env", Perm: provider.PermReadOnly},
		Auth:     provider.AuthSubscription,
		BaseEnv: fakeEnv(t,
			"ANTHROPIC_API_KEY=sk-ant-should-be-stripped",
			"ANTHROPIC_AUTH_TOKEN=tok-should-be-stripped",
			"FAKE_CLI_RECORD_ENV="+envFile,
		),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", res.ExitCode)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read recorded env: %v", err)
	}
	envText := string(data)
	if strings.Contains(envText, "ANTHROPIC_API_KEY=") {
		t.Errorf("ANTHROPIC_API_KEY leaked to child in subscription mode")
	}
	if strings.Contains(envText, "ANTHROPIC_AUTH_TOKEN=") {
		t.Errorf("ANTHROPIC_AUTH_TOKEN leaked to child in subscription mode")
	}
}

func TestRunner_TimeoutAndSignalEscalation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal escalation test runs on Unix")
	}

	fakeBin := filepath.Join(repoRoot(t), "testdata", "bin", "claude")

	t.Run("ignores SIGINT and SIGTERM then killed by SIGKILL", func(t *testing.T) {
		sigLog := filepath.Join(t.TempDir(), "signals.log")
		ready := make(chan struct{}, 1)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			<-ready
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		res, err := runner.Run(ctx, runner.Options{
			BinPath:     fakeBin,
			Provider:    claude.New(""),
			Request:     provider.RunRequest{Prompt: "trap", Perm: provider.PermReadOnly},
			Auth:        provider.AuthSubscription,
			GracePeriod: 150 * time.Millisecond,
			BaseEnv: fakeEnv(t,
				"FAKE_CLI_MODE=signal",
				"FAKE_CLI_IGNORE_SIGINT=1",
				"FAKE_CLI_IGNORE_SIGTERM=1",
				"FAKE_CLI_SIGNAL_LOG="+sigLog,
			),
			OnEvent: func(ev event.Event) {
				if ev.Kind == event.SessionStarted {
					select {
					case ready <- struct{}{}:
					default:
					}
				}
			},
		})
		elapsed := time.Since(start)

		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if !res.Canceled || res.ExitCode == 0 {
			t.Errorf("res = %+v, want Canceled=true and non-zero ExitCode", res)
		}
		if elapsed < 250*time.Millisecond {
			t.Errorf("elapsed = %v, expected at least two grace intervals (~300ms)", elapsed)
		}
		data, readErr := os.ReadFile(sigLog)
		if readErr != nil {
			t.Fatalf("read signal log: %v", readErr)
		}
		sigs := string(data)
		if !strings.Contains(sigs, "SIGINT") || !strings.Contains(sigs, "SIGTERM") {
			t.Errorf("expected both SIGINT and SIGTERM before SIGKILL, got:\n%s", sigs)
		}
	})

	t.Run("timeout stops sleeping child", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		res, err := runner.Run(ctx, runner.Options{
			BinPath:     fakeBin,
			Provider:    claude.New(""),
			Request:     provider.RunRequest{Prompt: "sleep", Perm: provider.PermReadOnly},
			Auth:        provider.AuthSubscription,
			GracePeriod: 100 * time.Millisecond,
			BaseEnv: fakeEnv(t,
				"FAKE_CLI_MODE=sleep",
				"FAKE_CLI_SLEEP_SEC=30",
			),
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want context.DeadlineExceeded", err)
		}
		if !res.TimedOut || res.ExitCode == 0 {
			t.Errorf("res = %+v, want TimedOut=true and non-zero ExitCode", res)
		}
	})
}

func TestRunner_FixtureReplay(t *testing.T) {
	root := repoRoot(t)
	fakeBin := filepath.Join(root, "testdata", "bin", "claude")
	fixture := filepath.Join(root, "testdata", "claude", "tool_use.ndjson")

	var kinds []event.Kind
	res, err := runner.Run(context.Background(), runner.Options{
		BinPath:  fakeBin,
		Provider: claude.New(""),
		Request:  provider.RunRequest{Prompt: "replay", Perm: provider.PermReadOnly},
		Auth:     provider.AuthSubscription,
		BaseEnv: fakeEnv(t,
			"FAKE_CLI_MODE=fixture",
			"FAKE_CLI_FIXTURE="+fixture,
		),
		OnEvent: func(ev event.Event) {
			kinds = append(kinds, ev.Kind)
		},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", res.ExitCode)
	}

	seen := make(map[event.Kind]bool)
	for _, k := range kinds {
		seen[k] = true
	}
	for _, want := range []event.Kind{
		event.SessionStarted,
		event.ToolCall,
		event.ToolResult,
		event.Text,
		event.Usage,
		event.Completed,
	} {
		if !seen[want] {
			t.Errorf("missing expected event kind %q (got %v)", want, kinds)
		}
	}
}
