package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/config"
	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/render"
)

func TestCompareCmd_ClaudeAndCodex_TTYAndJSONL(t *testing.T) {
	workDir := t.TempDir()

	t.Run("side-by-side TTY on wide terminal", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return os.Environ() },
			Getwd:         func() (string, error) { return workDir, nil },
			TermWidth:     func() int { return 120 },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"compare", "--providers", "claude,codex", "要約して"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("compare error: %v (stderr=%s)", err, stderr.String())
		}

		out := stdout.String()
		for _, want := range []string{
			"[claude]",
			"[codex]",
			"─┼─",
			"hello from fake claude",
			"hello from fake codex",
			render.SubscriptionCostDisclaimer,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("wide compare output missing %q:\n%s", want, out)
			}
		}
	})

	t.Run("stacked fallback on narrow terminal", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return os.Environ() },
			Getwd:         func() (string, error) { return workDir, nil },
			TermWidth:     func() int { return 80 },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"compare", "--providers", "claude,codex", "要約して"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("compare narrow error: %v", err)
		}

		out := stdout.String()
		if strings.Contains(out, "─┼─") {
			t.Errorf("narrow output should stack vertically, not side-by-side:\n%s", out)
		}
		if !strings.Contains(out, "=== [claude] ===") || !strings.Contains(out, "=== [codex] ===") {
			t.Errorf("narrow output missing stacked headers:\n%s", out)
		}
	})

	t.Run("--json outputs normalized JSONL with provider tags", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return os.Environ() },
			Getwd:         func() (string, error) { return workDir, nil },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"compare", "--providers", "claude,codex", "--json", "要約して"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("compare --json error: %v (stderr=%s)", err, stderr.String())
		}

		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if len(lines) < 4 {
			t.Fatalf("expected >=4 JSONL lines, got %d:\n%s", len(lines), stdout.String())
		}
		seenProviders := map[string]bool{}
		for i, line := range lines {
			var ev render.CompareJSONLEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatalf("line %d invalid JSON: %v (%s)", i, err, line)
			}
			if ev.Provider == "" {
				t.Errorf("line %d missing provider: %s", i, line)
			}
			seenProviders[ev.Provider] = true
		}
		if !seenProviders["claude"] || !seenProviders["codex"] {
			t.Errorf("expected events from both claude and codex, got %v", seenProviders)
		}
	})
}

func TestCompareCmd_AgySubscriptionDeniedBeforeLaunchingAnyProcess(t *testing.T) {
	invLog := filepath.Join(t.TempDir(), "invocations.log")
	baseEnv := provider.SetEnv(os.Environ(), "FAKE_CLI_INVOCATION_LOG", invLog)
	lookedUp := false

	var stdout, stderr bytes.Buffer
	deps := Deps{
		LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
		DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true, UnderAgent: false} },
		LookPath: func(file string) (string, error) {
			lookedUp = true
			return fakeBinLookPath(t)(file)
		},
		Environ: func() []string { return baseEnv },
		Getwd:   func() (string, error) { return t.TempDir(), nil },
		Logger:  logging.NewWriter(&bytes.Buffer{}),
		Stdout:  &stdout,
		Stderr:  &stderr,
	}

	cmd := NewRootCmd(deps)
	cmd.SetArgs([]string{"compare", "--providers", "claude,agy", "比較して"})
	err := cmd.Execute()

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("expected non-zero ExitError when agy(subscription) is included, got %v", err)
	}
	if lookedUp {
		t.Fatalf("LookPath must not be called when policy rejects a compare participant")
	}
	if _, statErr := os.Stat(invLog); !os.IsNotExist(statErr) {
		data, _ := os.ReadFile(invLog)
		t.Fatalf("no process (including claude) may start when agy(subscription) is rejected, got:\n%s", string(data))
	}
	errOut := stderr.String()
	if !strings.Contains(errOut, "agy") || !strings.Contains(errOut, "https://") {
		t.Errorf("expected denial reason and official URL on stderr, got:\n%s", errOut)
	}
}

func TestCompareCmd_EnforcesConfigMaxConcurrencyAndPartialFailureExitCode(t *testing.T) {
	t.Run("respects max_concurrency from config", func(t *testing.T) {
		concDir := t.TempDir()
		baseEnv := append([]string{}, os.Environ()...)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_MODE", "concurrency")
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_CONCURRENCY_DIR", concDir)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_SLEEP_SEC", "0.15")

		cfg := config.Default()
		cfg.Providers.Claude.MaxConcurrency = 1

		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return cfg, nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return baseEnv },
			Getwd:         func() (string, error) { return t.TempDir(), nil },
			TermWidth:     func() int { return 120 },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"compare", "--providers", "claude,claude,claude", "並列チェック"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("compare error: %v (stderr=%s)", err, stderr.String())
		}

		data, err := os.ReadFile(filepath.Join(concDir, "counts.claude"))
		if err != nil {
			t.Fatalf("read counts.claude: %v", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			n, convErr := strconv.Atoi(strings.TrimSpace(line))
			if convErr != nil {
				t.Fatalf("parse count %q: %v", line, convErr)
			}
			if n > 1 {
				t.Fatalf("claude concurrency reached %d, exceeded config max_concurrency=1", n)
			}
		}
	})

	t.Run("returns non-zero exit code when one provider fails after displaying all results", func(t *testing.T) {
		baseEnv := append([]string{}, os.Environ()...)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_CLAUDE_MODE", "exit")
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_CLAUDE_EXIT_CODE", "5")
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_STDERR_MSG", "claude failure")

		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return baseEnv },
			Getwd:         func() (string, error) { return t.TempDir(), nil },
			TermWidth:     func() int { return 120 },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"compare", "--providers", "claude,codex", "比較"})
		err := cmd.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 5 {
			t.Fatalf("expected ExitError code 5, got %v", err)
		}
		if !strings.Contains(stdout.String(), "hello from fake codex") {
			t.Errorf("expected codex output to still be displayed:\n%s", stdout.String())
		}
		if !strings.Contains(stderr.String(), "claude failure") {
			t.Errorf("expected claude stderr on stderr:\n%s", stderr.String())
		}
	})
}

func TestChainCmd_TwoAndThreeSteps_AndJSONL(t *testing.T) {
	workDir := t.TempDir()

	t.Run("2-step chain expands Prev from claude to codex", func(t *testing.T) {
		codexArgs := filepath.Join(t.TempDir(), "codex.args")
		baseEnv := append([]string{}, os.Environ()...)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_CLAUDE_TEXT", "claude-plan-abc")
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_CODEX_TEXT", "codex-review-xyz")
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_CODEX_RECORD_ARGS", codexArgs)

		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return baseEnv },
			Getwd:         func() (string, error) { return workDir, nil },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{
			"chain",
			"--steps", "claude,codex",
			"--prompt", "実装案を出して",
			"--then", "次の案をレビューして、改善点を挙げて: {{.Prev}}",
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("chain error: %v (stderr=%s)", err, stderr.String())
		}

		out := stdout.String()
		for _, want := range []string{
			"=== Step 1/2 [claude] ===",
			"claude-plan-abc",
			"=== Step 2/2 [codex] ===",
			"codex-review-xyz",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("chain output missing %q:\n%s", want, out)
			}
		}

		argsBytes, err := os.ReadFile(codexArgs)
		if err != nil {
			t.Fatalf("read codex args: %v", err)
		}
		if !strings.Contains(string(argsBytes), "次の案をレビューして、改善点を挙げて: claude-plan-abc") {
			t.Errorf("codex did not receive expanded --then template (including comma):\n%s", string(argsBytes))
		}
	})

	t.Run("3-step chain with --json emits step-tagged JSONL", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return os.Environ() },
			Getwd:         func() (string, error) { return workDir, nil },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{
			"chain",
			"--steps", "claude,codex,claude",
			"--prompt", "案を出して",
			"--then", "レビュー: {{.Prev}}",
			"--then", "統合: {{.Prev}}",
			"--json",
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("chain --json error: %v (stderr=%s)", err, stderr.String())
		}

		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		seenSteps := map[int]string{}
		for i, line := range lines {
			var ce render.ChainJSONLEvent
			if err := json.Unmarshal([]byte(line), &ce); err != nil {
				t.Fatalf("line %d invalid JSON: %v", i, err)
			}
			if ce.Kind == event.Text {
				seenSteps[ce.Step] = ce.Provider
			}
		}
		if seenSteps[1] != "claude" || seenSteps[2] != "codex" || seenSteps[3] != "claude" {
			t.Errorf("unexpected step/provider mapping in JSONL: %v", seenSteps)
		}
	})
}

func TestChainCmd_AgySubscriptionDeniedAndStepLimitsBeforeLaunch(t *testing.T) {
	t.Run("agy(subscription) in chain denied before launching any process", func(t *testing.T) {
		invLog := filepath.Join(t.TempDir(), "invocations.log")
		baseEnv := provider.SetEnv(os.Environ(), "FAKE_CLI_INVOCATION_LOG", invLog)
		lookedUp := false

		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath: func(file string) (string, error) {
				lookedUp = true
				return fakeBinLookPath(t)(file)
			},
			Environ: func() []string { return baseEnv },
			Getwd:   func() (string, error) { return t.TempDir(), nil },
			Logger:  logging.NewWriter(&bytes.Buffer{}),
			Stdout:  &stdout,
			Stderr:  &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{
			"chain",
			"--steps", "claude,agy",
			"--prompt", "案を出して",
			"--then", "確認: {{.Prev}}",
		})
		err := cmd.Execute()

		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code == 0 {
			t.Fatalf("expected non-zero ExitError for agy(subscription) in chain, got %v", err)
		}
		if lookedUp {
			t.Fatalf("LookPath must not be called when policy denies chain")
		}
		if _, statErr := os.Stat(invLog); !os.IsNotExist(statErr) {
			data, _ := os.ReadFile(invLog)
			t.Fatalf("no process (including claude) may start when agy(subscription) is rejected, got:\n%s", string(data))
		}
		errOut := stderr.String()
		if !strings.Contains(errOut, "agy") || !strings.Contains(errOut, "https://") {
			t.Errorf("expected denial reason and official URL on stderr, got:\n%s", errOut)
		}
	})

	t.Run("rejects 1 step and 4 steps without launching any process", func(t *testing.T) {
		lookedUp := false
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath: func(file string) (string, error) {
				lookedUp = true
				return "", errors.New("should not be called")
			},
			Logger: logging.NewWriter(&bytes.Buffer{}),
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
		}

		for _, steps := range []string{"claude", "claude,codex,claude,codex"} {
			cmd := NewRootCmd(deps)
			cmd.SetArgs([]string{
				"chain",
				"--steps", steps,
				"--prompt", "案を出して",
				"--then", "次: {{.Prev}}",
			})
			if err := cmd.Execute(); err == nil {
				t.Errorf("expected error for --steps=%q", steps)
			}
		}
		if lookedUp {
			t.Fatalf("LookPath must not be called when step count is out of bounds")
		}
	})

	t.Run("step 1 failure stops chain and never launches step 2", func(t *testing.T) {
		invLog := filepath.Join(t.TempDir(), "invocations.log")
		baseEnv := append([]string{}, os.Environ()...)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_INVOCATION_LOG", invLog)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_CLAUDE_MODE", "exit")
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_CLAUDE_EXIT_CODE", "6")
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_STDERR_MSG", "step1 failed hard")

		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return baseEnv },
			Getwd:         func() (string, error) { return t.TempDir(), nil },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{
			"chain",
			"--steps", "claude,codex",
			"--prompt", "案を出して",
			"--then", "次: {{.Prev}}",
		})
		err := cmd.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 6 {
			t.Fatalf("expected ExitError code 6, got %v", err)
		}
		data, _ := os.ReadFile(invLog)
		if strings.Contains(string(data), "codex:") {
			t.Fatalf("step 2 (codex) must not be launched after step 1 failure:\n%s", string(data))
		}
		if !strings.Contains(stderr.String(), "step1 failed hard") {
			t.Errorf("stderr missing step 1 error output:\n%s", stderr.String())
		}
	})
}
