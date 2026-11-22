package runner_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/claude"
	"github.com/sudabon/subcli/internal/provider/codex"
	"github.com/sudabon/subcli/internal/runner"
)

func TestRunChain_TwoStepSuccess(t *testing.T) {
	root := repoRoot(t)
	claudeBin := filepath.Join(root, "testdata", "bin", "claude")
	codexBin := filepath.Join(root, "testdata", "bin", "codex")

	claudeArgsFile := filepath.Join(t.TempDir(), "claude.args")
	codexArgsFile := filepath.Join(t.TempDir(), "codex.args")
	env := fakeEnv(t,
		"FAKE_CLI_CLAUDE_RECORD_ARGS="+claudeArgsFile,
		"FAKE_CLI_CODEX_RECORD_ARGS="+codexArgsFile,
		"FAKE_CLI_CLAUDE_TEXT=draft proposal v1",
		"FAKE_CLI_CODEX_TEXT=reviewed proposal v1",
	)

	steps := []runner.ChainStep{
		{
			Provider: claude.New(""),
			BinPath:  claudeBin,
			Request:  provider.RunRequest{Perm: provider.PermReadOnly},
			Auth:     provider.AuthSubscription,
			BaseEnv:  env,
		},
		{
			Provider: codex.New(""),
			BinPath:  codexBin,
			Request:  provider.RunRequest{Perm: provider.PermReadOnly},
			Auth:     provider.AuthSubscription,
			BaseEnv:  env,
		},
	}

	results, err := runner.RunChain(
		context.Background(),
		steps,
		"実装案を出して",
		[]string{"次の案をレビューして: {{.Prev}}"},
		runner.ChainOptions{},
	)
	if err != nil {
		t.Fatalf("RunChain 2-step error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Text != "draft proposal v1" {
		t.Errorf("step 0 text = %q, want draft proposal v1", results[0].Text)
	}
	if results[1].Prompt != "次の案をレビューして: draft proposal v1" {
		t.Errorf("step 1 prompt = %q, want expanded template", results[1].Prompt)
	}
	if results[1].Text != "reviewed proposal v1" {
		t.Errorf("step 1 text = %q, want reviewed proposal v1", results[1].Text)
	}

	codexArgs, err := os.ReadFile(codexArgsFile)
	if err != nil {
		t.Fatalf("read codex args: %v", err)
	}
	if !strings.Contains(string(codexArgs), "次の案をレビューして: draft proposal v1") {
		t.Errorf("codex CLI did not receive expanded prompt:\n%s", string(codexArgs))
	}
}

func TestRunChain_ThreeStepSharedAndPerStepTemplates(t *testing.T) {
	root := repoRoot(t)
	claudeBin := filepath.Join(root, "testdata", "bin", "claude")
	codexBin := filepath.Join(root, "testdata", "bin", "codex")

	t.Run("3 steps with 2 distinct --then templates", func(t *testing.T) {
		claudeAppend := filepath.Join(t.TempDir(), "claude.calls")
		env := fakeEnv(t,
			"FAKE_CLI_CLAUDE_APPEND_ARGS="+claudeAppend,
			"FAKE_CLI_CLAUDE_TEXT=step1-out",
			"FAKE_CLI_CODEX_TEXT=step2-out",
		)

		steps := []runner.ChainStep{
			{Provider: claude.New(""), BinPath: claudeBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
			{Provider: codex.New(""), BinPath: codexBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
			{Provider: claude.New(""), BinPath: claudeBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
		}

		results, err := runner.RunChain(
			context.Background(),
			steps,
			"初期依頼",
			[]string{"レビュー: {{.Prev}}", "最終まとめ: {{.Prev}}"},
			runner.ChainOptions{},
		)
		if err != nil {
			t.Fatalf("RunChain 3-step error: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("len(results) = %d, want 3", len(results))
		}
		if results[1].Prompt != "レビュー: step1-out" {
			t.Errorf("step 1 prompt = %q", results[1].Prompt)
		}
		if results[2].Prompt != "最終まとめ: step2-out" {
			t.Errorf("step 2 prompt = %q", results[2].Prompt)
		}
	})

	t.Run("3 steps reusing single --then template", func(t *testing.T) {
		env := fakeEnv(t,
			"FAKE_CLI_CLAUDE_TEXT=c-out",
			"FAKE_CLI_CODEX_TEXT=x-out",
		)
		steps := []runner.ChainStep{
			{Provider: claude.New(""), BinPath: claudeBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
			{Provider: codex.New(""), BinPath: codexBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
			{Provider: claude.New(""), BinPath: claudeBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
		}

		results, err := runner.RunChain(
			context.Background(),
			steps,
			"start",
			[]string{"improve: {{.Prev}}"},
			runner.ChainOptions{},
		)
		if err != nil {
			t.Fatalf("RunChain 3-step single template error: %v", err)
		}
		if results[1].Prompt != "improve: c-out" || results[2].Prompt != "improve: x-out" {
			t.Errorf("unexpected prompts: step1=%q step2=%q", results[1].Prompt, results[2].Prompt)
		}
	})
}

func TestRunChain_RejectsInvalidStepCountAndTemplatesBeforeLaunch(t *testing.T) {
	root := repoRoot(t)
	claudeBin := filepath.Join(root, "testdata", "bin", "claude")
	invLog := filepath.Join(t.TempDir(), "invocations.log")
	env := fakeEnv(t, "FAKE_CLI_INVOCATION_LOG="+invLog)

	mkStep := func() runner.ChainStep {
		return runner.ChainStep{
			Provider: claude.New(""),
			BinPath:  claudeBin,
			Request:  provider.RunRequest{Perm: provider.PermReadOnly},
			Auth:     provider.AuthSubscription,
			BaseEnv:  env,
		}
	}

	cases := []struct {
		name   string
		steps  []runner.ChainStep
		prompt string
		thens  []string
	}{
		{name: "0 steps", steps: nil, prompt: "p", thens: []string{"{{.Prev}}"}},
		{name: "1 step", steps: []runner.ChainStep{mkStep()}, prompt: "p", thens: []string{"{{.Prev}}"}},
		{name: "4 steps", steps: []runner.ChainStep{mkStep(), mkStep(), mkStep(), mkStep()}, prompt: "p", thens: []string{"{{.Prev}}"}},
		{name: "missing template", steps: []runner.ChainStep{mkStep(), mkStep()}, prompt: "p", thens: nil},
		{name: "invalid template syntax", steps: []runner.ChainStep{mkStep(), mkStep()}, prompt: "p", thens: []string{"{{.Prev"}},
		{name: "unknown template field", steps: []runner.ChainStep{mkStep(), mkStep()}, prompt: "p", thens: []string{"{{.DoesNotExist}}"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := runner.RunChain(context.Background(), tc.steps, tc.prompt, tc.thens, runner.ChainOptions{}); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}

	if _, err := os.Stat(invLog); !os.IsNotExist(err) {
		data, _ := os.ReadFile(invLog)
		t.Fatalf("expected zero CLI invocations on validation failure, got:\n%s", string(data))
	}
}

func TestRunChain_AbortsOnStepFailureOrEmptyOutput(t *testing.T) {
	root := repoRoot(t)
	claudeBin := filepath.Join(root, "testdata", "bin", "claude")
	codexBin := filepath.Join(root, "testdata", "bin", "codex")

	t.Run("step 1 non-zero exit does not start step 2", func(t *testing.T) {
		invLog := filepath.Join(t.TempDir(), "invocations.log")
		env := fakeEnv(t,
			"FAKE_CLI_INVOCATION_LOG="+invLog,
			"FAKE_CLI_CLAUDE_MODE=exit",
			"FAKE_CLI_CLAUDE_EXIT_CODE=4",
			"FAKE_CLI_STDERR_MSG=step1 exploded",
		)

		steps := []runner.ChainStep{
			{Provider: claude.New(""), BinPath: claudeBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
			{Provider: codex.New(""), BinPath: codexBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
		}

		results, err := runner.RunChain(context.Background(), steps, "start", []string{"next: {{.Prev}}"}, runner.ChainOptions{})
		if err == nil {
			t.Fatalf("expected error when step 1 fails")
		}
		var chainErr *runner.ChainError
		if !errors.As(err, &chainErr) || chainErr.StepIndex != 0 || chainErr.Result.ExitCode != 4 {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("len(results) = %d, want 1", len(results))
		}

		data, _ := os.ReadFile(invLog)
		if strings.Contains(string(data), "codex:") {
			t.Fatalf("step 2 (codex) must not be launched when step 1 fails:\n%s", string(data))
		}
	})

	t.Run("step 1 empty text output stops before step 2", func(t *testing.T) {
		invLog := filepath.Join(t.TempDir(), "invocations.log")
		env := fakeEnv(t,
			"FAKE_CLI_INVOCATION_LOG="+invLog,
			"FAKE_CLI_CLAUDE_MODE=empty_text",
		)

		steps := []runner.ChainStep{
			{Provider: claude.New(""), BinPath: claudeBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
			{Provider: codex.New(""), BinPath: codexBin, Request: provider.RunRequest{Perm: provider.PermReadOnly}, Auth: provider.AuthSubscription, BaseEnv: env},
		}

		_, err := runner.RunChain(context.Background(), steps, "start", []string{"next: {{.Prev}}"}, runner.ChainOptions{})
		if err == nil {
			t.Fatalf("expected error when step 1 produces empty output text")
		}
		data, _ := os.ReadFile(invLog)
		if strings.Contains(string(data), "codex:") {
			t.Fatalf("step 2 (codex) must not be launched when step 1 output is empty:\n%s", string(data))
		}
	})
}
