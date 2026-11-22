package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
)

func TestDoctorCmd_OutputsAndComplianceWarning(t *testing.T) {
	run := func(now time.Time, complianceMD string) (string, error) {
		var stdout, stderr bytes.Buffer
		var logBuf bytes.Buffer
		deps := testDeps(t)
		deps.LookPath = func(file string) (string, error) {
			if file == "codex" {
				return "", errors.New("not found")
			}
			return "/usr/local/bin/" + file, nil
		}
		deps.RunCapture = func(_ context.Context, binPath string, args []string, _ []string) (string, error) {
			if len(args) == 1 && args[0] == "--version" {
				return binPath + " 1.0.0", nil
			}
			if strings.HasSuffix(binPath, "/claude") && reflect.DeepEqual(args, []string{"auth", "status"}) {
				return "Logged in as user@example.com", nil
			}
			return "", errors.New("unexpected command")
		}
		deps.Environ = func() []string { return []string{"PATH=/usr/local/bin"} }
		deps.Now = func() time.Time { return now }
		deps.ComplianceText = func() (string, string) {
			return complianceMD, "docs/COMPLIANCE.md"
		}
		deps.Logger = logging.NewWriter(&logBuf)
		deps.Stdout = &stdout
		deps.Stderr = &stderr
		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"doctor"})
		err := cmd.Execute()
		return stdout.String(), err
	}

	t.Run("fresh compliance date and agy unconfirmed message", func(t *testing.T) {
		out, err := run(
			time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC),
			"# Compliance\nConfirmed-Date: 2026-11-22\n",
		)
		if err != nil {
			t.Fatalf("doctor error: %v", err)
		}
		for _, want := range []string{
			"claude (claude): found",
			"Logged in as user@example.com",
			"codex (codex): not found",
			"agy (agy): found",
			"未確認（subcli login agy で確認してください）",
			"confirmed_date: 2026-11-22",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("doctor output missing %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "WARNING:") {
			t.Errorf("did not expect warning for fresh compliance date:\n%s", out)
		}
	})

	t.Run("stale compliance date emits warning", func(t *testing.T) {
		out, err := run(
			time.Date(2026, 12, 25, 0, 0, 0, 0, time.UTC),
			"# Compliance\nConfirmed-Date: 2026-11-22\n",
		)
		if err != nil {
			t.Fatalf("doctor error: %v", err)
		}
		if !strings.Contains(out, "WARNING:") || !strings.Contains(out, "30 日以上") {
			t.Errorf("expected 30-day stale compliance warning:\n%s", out)
		}
	})
}

func TestDoctorCmd_JSONAuthStatusFormatting(t *testing.T) {
	runWithClaudeStatus := func(claudeOut string, claudeErr error) (string, error) {
		var stdout, stderr, logBuf bytes.Buffer
		deps := testDeps(t)
		deps.LookPath = func(file string) (string, error) {
			if file == "claude" {
				return "/usr/local/bin/claude", nil
			}
			return "", errors.New("not found")
		}
		deps.RunCapture = func(_ context.Context, _ string, args []string, _ []string) (string, error) {
			if len(args) == 1 && args[0] == "--version" {
				return "2.1.330 (Claude Code)", nil
			}
			if reflect.DeepEqual(args, []string{"auth", "status"}) {
				return claudeOut, claudeErr
			}
			return "", errors.New("unexpected command")
		}
		deps.Environ = func() []string { return []string{"PATH=/usr/local/bin"} }
		deps.Now = func() time.Time { return time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC) }
		deps.ComplianceText = func() (string, string) {
			return "# Compliance\nConfirmed-Date: 2026-11-22\n", "docs/COMPLIANCE.md"
		}
		deps.Logger = logging.NewWriter(&logBuf)
		deps.Stdout = &stdout
		deps.Stderr = &stderr
		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"doctor"})
		err := cmd.Execute()
		return stdout.String(), err
	}

	t.Run("pretty-printed logged-in JSON summarizes auth method and subscription", func(t *testing.T) {
		out, err := runWithClaudeStatus("{\n  \"loggedIn\": true,\n  \"authMethod\": \"claude.ai\",\n  \"apiProvider\": \"firstParty\",\n  \"subscriptionType\": \"pro\"\n}", nil)
		if err != nil {
			t.Fatalf("doctor error: %v", err)
		}
		if !strings.Contains(out, "login:   Logged in (claude.ai, pro)") {
			t.Errorf("unexpected doctor output:\n%s", out)
		}
	})

	t.Run("pretty-printed logged-out JSON reports attention required even on exit 0", func(t *testing.T) {
		out, err := runWithClaudeStatus("{\n  \"loggedIn\": false,\n  \"authMethod\": \"none\"\n}", nil)
		if err != nil {
			t.Fatalf("doctor error: %v", err)
		}
		if !strings.Contains(out, "login:   要確認 (Not logged in)") {
			t.Errorf("unexpected doctor output:\n%s", out)
		}
	})

	t.Run("generic multi-line JSON is compacted instead of showing bare brace", func(t *testing.T) {
		out, err := runWithClaudeStatus("{\n  \"status\": \"authenticated\"\n}", nil)
		if err != nil {
			t.Fatalf("doctor error: %v", err)
		}
		if !strings.Contains(out, `login:   {"status":"authenticated"}`) {
			t.Errorf("unexpected doctor output:\n%s", out)
		}
	})

	t.Run("leading warning line before JSON still extracts login summary", func(t *testing.T) {
		out, err := runWithClaudeStatus("Warning: update available\n{\n  \"loggedIn\": true,\n  \"authMethod\": \"claude.ai\"\n}", nil)
		if err != nil {
			t.Fatalf("doctor error: %v", err)
		}
		if !strings.Contains(out, "login:   Logged in (claude.ai)") {
			t.Errorf("unexpected doctor output:\n%s", out)
		}
	})
}

func TestLoginCmd(t *testing.T) {
	t.Run("allowed codex login launches official command and reports exit code", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotBin string
		var gotArgs []string
		var gotEnv []string

		deps := testDeps(t)
		deps.LookPath = func(file string) (string, error) { return "/usr/local/bin/" + file, nil }
		deps.RunInteractive = func(_ context.Context, binPath string, args []string, env []string, _ io.Reader, _, _ io.Writer) (int, error) {
			gotBin = binPath
			gotArgs = args
			gotEnv = env
			return 0, nil
		}
		deps.Environ = func() []string { return []string{"PATH=/usr/local/bin", "OPENAI_API_KEY=sk-should-be-stripped"} }
		deps.Stdout = &stdout
		deps.Stderr = &stderr

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"login", "codex"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("login codex error: %v", err)
		}
		if gotBin != "/usr/local/bin/codex" || !reflect.DeepEqual(gotArgs, []string{"login"}) {
			t.Errorf("launched (%q, %v), want (/usr/local/bin/codex, [login])", gotBin, gotArgs)
		}
		if _, leaked := provider.LookupEnvSlice(gotEnv, "OPENAI_API_KEY"); leaked {
			t.Errorf("OPENAI_API_KEY should be stripped in subscription mode")
		}
		if !strings.Contains(stdout.String(), "codex login exited with code 0") {
			t.Errorf("unexpected stdout: %q", stdout.String())
		}
	})

	t.Run("non-zero login exit code propagated", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		deps := testDeps(t)
		deps.LookPath = func(file string) (string, error) { return "/usr/local/bin/" + file, nil }
		deps.RunInteractive = func(_ context.Context, _ string, _ []string, _ []string, _ io.Reader, _, _ io.Writer) (int, error) {
			return 2, nil
		}
		deps.Environ = func() []string { return nil }
		deps.Stdout = &stdout
		deps.Stderr = &stderr
		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"login", "claude"})
		err := cmd.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 2 {
			t.Fatalf("expected ExitError code 2, got %v", err)
		}
		if !strings.Contains(stdout.String(), "claude login exited with code 2") {
			t.Errorf("unexpected stdout: %q", stdout.String())
		}
	})

	t.Run("agy subscription login denied under agent without launching CLI", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		called := false
		deps := testDeps(t)
		deps.DetectContext = func() policy.CallContext {
			return policy.CallContext{StdinIsTTY: true, UnderAgent: true, AgentHint: "env:CLAUDECODE"}
		}
		deps.LookPath = func(file string) (string, error) {
			called = true
			return "/usr/local/bin/" + file, nil
		}
		deps.RunInteractive = func(_ context.Context, _ string, _ []string, _ []string, _ io.Reader, _, _ io.Writer) (int, error) {
			called = true
			return 0, nil
		}
		deps.Environ = func() []string { return nil }
		deps.Stdout = &stdout
		deps.Stderr = &stderr

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"login", "agy"})
		err := cmd.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code == 0 {
			t.Fatalf("expected non-zero ExitError, got %v", err)
		}
		if called {
			t.Fatalf("CLI must not be looked up or launched when policy denies login")
		}
		if !strings.Contains(stderr.String(), "env:CLAUDECODE") || !strings.Contains(stderr.String(), "https://") {
			t.Errorf("expected denial reason with AgentHint and DocURL on stderr, got:\n%s", stderr.String())
		}
	})
}

func TestOpenCmd(t *testing.T) {
	t.Run("direct TTY open forwards extra args and strips API keys", func(t *testing.T) {
		var gotBin string
		var gotArgv []string
		var gotEnv []string

		deps := testDeps(t)
		deps.LookPath = func(file string) (string, error) { return "/usr/local/bin/" + file, nil }
		deps.ExecReplace = func(binPath string, argv []string, env []string) error {
			gotBin = binPath
			gotArgv = argv
			gotEnv = env
			return nil
		}
		deps.Environ = func() []string { return []string{"PATH=/usr/bin", "ANTHROPIC_API_KEY=sk-ant-should-be-stripped"} }
		deps.Stdout = &bytes.Buffer{}
		deps.Stderr = &bytes.Buffer{}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "claude", "--", "--model", "sonnet"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("open claude error: %v", err)
		}
		if gotBin != "/usr/local/bin/claude" {
			t.Errorf("bin = %q, want /usr/local/bin/claude", gotBin)
		}
		wantArgv := []string{"claude", "--model", "sonnet"}
		if !reflect.DeepEqual(gotArgv, wantArgv) {
			t.Errorf("argv = %v, want %v", gotArgv, wantArgv)
		}
		if _, leaked := provider.LookupEnvSlice(gotEnv, "ANTHROPIC_API_KEY"); leaked {
			t.Errorf("ANTHROPIC_API_KEY should be stripped in subscription mode")
		}
	})

	t.Run("open agy under agent is denied before exec", func(t *testing.T) {
		var stderr bytes.Buffer
		execCalled := false
		deps := testDeps(t)
		deps.DetectContext = func() policy.CallContext {
			return policy.CallContext{StdinIsTTY: true, UnderAgent: true, AgentHint: "env:CLAUDECODE"}
		}
		deps.LookPath = func(file string) (string, error) { return "/usr/local/bin/" + file, nil }
		deps.ExecReplace = func(_ string, _ []string, _ []string) error {
			execCalled = true
			return nil
		}
		deps.Environ = func() []string { return nil }
		deps.Stdout = &bytes.Buffer{}
		deps.Stderr = &stderr

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "agy"})
		err := cmd.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code == 0 {
			t.Fatalf("expected non-zero ExitError, got %v", err)
		}
		if execCalled {
			t.Fatalf("ExecReplace must not be called when policy denies")
		}
		if !strings.Contains(stderr.String(), "env:CLAUDECODE") || !strings.Contains(stderr.String(), "https://") {
			t.Errorf("stderr missing reason/URL: %s", stderr.String())
		}
	})

	t.Run("dangerous flag rejected in extra args", func(t *testing.T) {
		execCalled := false
		deps := testDeps(t)
		deps.LookPath = func(file string) (string, error) { return "/usr/local/bin/" + file, nil }
		deps.ExecReplace = func(_ string, _ []string, _ []string) error {
			execCalled = true
			return nil
		}
		deps.Environ = func() []string { return nil }
		deps.Stdout = &bytes.Buffer{}
		deps.Stderr = &bytes.Buffer{}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "claude", "--", "--dangerously-skip-permissions"})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected error for dangerous flag")
		}
		if execCalled {
			t.Fatalf("ExecReplace must not be called when dangerous flag is supplied")
		}
	})
}
