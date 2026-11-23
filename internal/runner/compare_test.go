package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/claude"
	"github.com/sudabon/subcli/internal/provider/codex"
	"github.com/sudabon/subcli/internal/runner"
)

func readMaxCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read count log %s: %v", path, err)
	}
	maxVal := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			t.Fatalf("parse count %q: %v", line, err)
		}
		if n > maxVal {
			maxVal = n
		}
	}
	return maxVal
}

func TestRunCompare_EnforcesPerProviderMaxConcurrency(t *testing.T) {
	root := repoRoot(t)
	claudeBin := filepath.Join(root, "testdata", "bin", "claude")
	codexBin := filepath.Join(root, "testdata", "bin", "codex")

	t.Run("same provider capped at max_concurrency=2 across 5 parallel requests", func(t *testing.T) {
		concDir := t.TempDir()
		env := fakeEnv(t,
			"FAKE_CLI_MODE=concurrency",
			"FAKE_CLI_CONCURRENCY_DIR="+concDir,
			"FAKE_CLI_SLEEP_SEC=0.15",
		)

		targets := make([]runner.CompareTarget, 5)
		for i := range targets {
			targets[i] = runner.CompareTarget{
				Provider:       claude.New(""),
				BinPath:        claudeBin,
				Request:        provider.RunRequest{Prompt: "hello", Perm: provider.PermReadOnly},
				Auth:           provider.AuthSubscription,
				MaxConcurrency: 2,
				BaseEnv:        env,
			}
		}

		var observedEvents []runner.CompareEvent
		outcomes := runner.RunCompare(context.Background(), targets, runner.CompareOptions{
			OnEvent: func(ce runner.CompareEvent) {
				observedEvents = append(observedEvents, ce)
			},
		})

		if len(outcomes) != 5 {
			t.Fatalf("len(outcomes) = %d, want 5", len(outcomes))
		}
		for i, out := range outcomes {
			if out.Err != nil || out.Result.ExitCode != 0 {
				t.Errorf("outcome[%d] failed: err=%v exit=%d stderr=%s", i, out.Err, out.Result.ExitCode, out.Result.Stderr)
			}
			if !strings.HasPrefix(out.Text, "conc-claude-") {
				t.Errorf("outcome[%d].Text = %q, want prefix conc-claude-", i, out.Text)
			}
		}
		if len(observedEvents) == 0 {
			t.Errorf("expected OnEvent to receive normalized events")
		}

		maxClaude := readMaxCount(t, filepath.Join(concDir, "counts.claude"))
		if maxClaude > 2 {
			t.Fatalf("claude concurrent processes reached %d, must not exceed max_concurrency=2", maxClaude)
		}
		if maxClaude < 2 {
			t.Errorf("expected claude to reach concurrency 2 during 5 parallel tasks, got max %d", maxClaude)
		}
	})

	t.Run("semaphores are isolated per provider", func(t *testing.T) {
		concDir := t.TempDir()
		env := fakeEnv(t,
			"FAKE_CLI_MODE=concurrency",
			"FAKE_CLI_CONCURRENCY_DIR="+concDir,
			"FAKE_CLI_SLEEP_SEC=0.18",
		)

		targets := []runner.CompareTarget{
			{
				Provider:       claude.New(""),
				BinPath:        claudeBin,
				Request:        provider.RunRequest{Prompt: "p", Perm: provider.PermReadOnly},
				Auth:           provider.AuthSubscription,
				MaxConcurrency: 1,
				BaseEnv:        env,
			},
			{
				Provider:       claude.New(""),
				BinPath:        claudeBin,
				Request:        provider.RunRequest{Prompt: "p", Perm: provider.PermReadOnly},
				Auth:           provider.AuthSubscription,
				MaxConcurrency: 1,
				BaseEnv:        env,
			},
			{
				Provider:       codex.New(""),
				BinPath:        codexBin,
				Request:        provider.RunRequest{Prompt: "p", Perm: provider.PermReadOnly},
				Auth:           provider.AuthSubscription,
				MaxConcurrency: 1,
				BaseEnv:        env,
			},
			{
				Provider:       codex.New(""),
				BinPath:        codexBin,
				Request:        provider.RunRequest{Prompt: "p", Perm: provider.PermReadOnly},
				Auth:           provider.AuthSubscription,
				MaxConcurrency: 1,
				BaseEnv:        env,
			},
		}

		outcomes := runner.RunCompare(context.Background(), targets, runner.CompareOptions{})
		for i, out := range outcomes {
			if out.Err != nil || out.Result.ExitCode != 0 {
				t.Fatalf("outcome[%d] failed: err=%v exit=%d", i, out.Err, out.Result.ExitCode)
			}
		}

		if maxClaude := readMaxCount(t, filepath.Join(concDir, "counts.claude")); maxClaude > 1 {
			t.Fatalf("claude concurrent processes = %d, want <= 1", maxClaude)
		}
		if maxCodex := readMaxCount(t, filepath.Join(concDir, "counts.codex")); maxCodex > 1 {
			t.Fatalf("codex concurrent processes = %d, want <= 1", maxCodex)
		}
		if maxTotal := readMaxCount(t, filepath.Join(concDir, "counts.total")); maxTotal < 2 {
			t.Errorf("expected claude and codex to run concurrently across separate semaphores, got max total=%d", maxTotal)
		}
	})
}

func TestRunCompare_PartialFailureStillCollectsOtherProvider(t *testing.T) {
	root := repoRoot(t)
	claudeBin := filepath.Join(root, "testdata", "bin", "claude")
	codexBin := filepath.Join(root, "testdata", "bin", "codex")

	env := fakeEnv(t,
		"FAKE_CLI_CLAUDE_MODE=exit",
		"FAKE_CLI_CLAUDE_EXIT_CODE=3",
		"FAKE_CLI_STDERR_MSG=claude boom",
	)

	var seenCodexText bool
	outcomes := runner.RunCompare(context.Background(), []runner.CompareTarget{
		{
			Provider:       claude.New(""),
			BinPath:        claudeBin,
			Request:        provider.RunRequest{Prompt: "compare", Perm: provider.PermReadOnly},
			Auth:           provider.AuthSubscription,
			MaxConcurrency: 2,
			BaseEnv:        env,
		},
		{
			Provider:       codex.New(""),
			BinPath:        codexBin,
			Request:        provider.RunRequest{Prompt: "compare", Perm: provider.PermReadOnly},
			Auth:           provider.AuthSubscription,
			MaxConcurrency: 2,
			BaseEnv:        env,
		},
	}, runner.CompareOptions{
		OnEvent: func(ce runner.CompareEvent) {
			if ce.Event.Provider == "codex" && ce.Event.Kind == event.Text {
				seenCodexText = true
			}
		},
	})

	if outcomes[0].Result.ExitCode != 3 {
		t.Errorf("claude ExitCode = %d, want 3", outcomes[0].Result.ExitCode)
	}
	if !strings.Contains(outcomes[0].Result.Stderr, "claude boom") {
		t.Errorf("claude stderr = %q, want claude boom", outcomes[0].Result.Stderr)
	}
	if outcomes[1].Err != nil || outcomes[1].Result.ExitCode != 0 {
		t.Errorf("codex failed unexpectedly: err=%v exit=%d", outcomes[1].Err, outcomes[1].Result.ExitCode)
	}
	if outcomes[1].Text != "hello from fake codex" || !seenCodexText {
		t.Errorf("codex text = %q (seen=%v), want hello from fake codex", outcomes[1].Text, seenCodexText)
	}
}
