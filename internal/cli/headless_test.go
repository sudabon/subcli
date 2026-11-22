package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sudabon/subcli/internal/config"
	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/render"
	"github.com/sudabon/subcli/internal/session"
)

func testRepoRoot(t *testing.T) string {
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

func fakeBinLookPath(t *testing.T) func(string) (string, error) {
	t.Helper()
	binDir := filepath.Join(testRepoRoot(t), "testdata", "bin")
	return func(file string) (string, error) {
		p := filepath.Join(binDir, file)
		if _, err := os.Stat(p); err != nil {
			return "", err
		}
		return p, nil
	}
}

func TestSessionsCmd_ListShowRmAndMissingID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	openStore := func() (*session.Store, error) {
		return session.Open(dbPath)
	}

	// Pre-populate a record
	st, err := openStore()
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	t0 := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	rec, err := st.Create(context.Background(), session.Record{
		ID:                "abc12345",
		Provider:          "claude",
		ProviderSessionID: "claude-real-id",
		Cwd:               "/workspace/subcli",
		CreatedAt:         t0,
		LastRunAt:         t0,
		PromptPreview:     "README を要約して",
	})
	_ = st.Close()
	if err != nil {
		t.Fatalf("create record: %v", err)
	}

	execCmd := func(args ...string) (string, string, error) {
		var stdout, stderr bytes.Buffer
		deps := Deps{
			OpenStore: openStore,
			Logger:    logging.NewWriter(&bytes.Buffer{}),
			Stdout:    &stdout,
			Stderr:    &stderr,
		}
		cmd := NewRootCmd(deps)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return stdout.String(), stderr.String(), err
	}

	// Bare `sessions` behaves like `sessions list`
	out, _, err := execCmd("sessions")
	if err != nil {
		t.Fatalf("sessions error: %v", err)
	}
	if !strings.Contains(out, rec.ID) || !strings.Contains(out, "claude") || !strings.Contains(out, "README を要約して") {
		t.Errorf("sessions output unexpected:\n%s", out)
	}

	// `sessions list`
	outList, _, err := execCmd("sessions", "list")
	if err != nil {
		t.Fatalf("sessions list error: %v", err)
	}
	if outList != out {
		t.Errorf("sessions and sessions list outputs differ:\n%s\nvs\n%s", out, outList)
	}

	// `sessions show <id>`
	outShow, _, err := execCmd("sessions", "show", rec.ID)
	if err != nil {
		t.Fatalf("sessions show error: %v", err)
	}
	for _, want := range []string{"id: abc12345", "provider: claude", "provider_session_id: claude-real-id", "cwd: /workspace/subcli"} {
		if !strings.Contains(outShow, want) {
			t.Errorf("sessions show missing %q:\n%s", want, outShow)
		}
	}

	// `sessions show` missing ID returns error
	if _, _, err := execCmd("sessions", "show", "missing-id"); err == nil {
		t.Fatalf("expected error for missing session ID on show")
	}

	// `sessions rm <id>`
	outRm, _, err := execCmd("sessions", "rm", rec.ID)
	if err != nil {
		t.Fatalf("sessions rm error: %v", err)
	}
	if !strings.Contains(outRm, rec.ID) {
		t.Errorf("sessions rm output unexpected: %s", outRm)
	}

	// `sessions rm` missing ID returns error
	if _, _, err := execCmd("sessions", "rm", rec.ID); err == nil {
		t.Fatalf("expected error when removing already-deleted session ID")
	}
}

func TestRunCmd_InvalidPermAndPolicyCheck(t *testing.T) {
	t.Run("rejects invalid --perm before launching child", func(t *testing.T) {
		lookedUp := false
		deps := Deps{
			LoadConfig: func() (config.Config, error) { return config.Default(), nil },
			LookPath: func(file string) (string, error) {
				lookedUp = true
				return "/usr/bin/" + file, nil
			},
			Logger: logging.NewWriter(&bytes.Buffer{}),
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
		}
		for _, badPerm := range []string{"full", "bypass", "bypassPermissions", "danger-full-access"} {
			cmd := NewRootCmd(deps)
			cmd.SetArgs([]string{"run", "claude", "--perm", badPerm, "hello"})
			if err := cmd.Execute(); err == nil {
				t.Errorf("expected error for --perm=%q", badPerm)
			}
		}
		if lookedUp {
			t.Fatalf("LookPath must not be called when --perm is invalid")
		}
	})

	t.Run("policy denies agy subscription run before launching child", func(t *testing.T) {
		lookedUp := false
		var stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true, UnderAgent: false} },
			LookPath: func(file string) (string, error) {
				lookedUp = true
				return "/usr/bin/" + file, nil
			},
			Logger: logging.NewWriter(&bytes.Buffer{}),
			Stdout: &bytes.Buffer{},
			Stderr: &stderr,
		}
		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"run", "agy", "hello"})
		err := cmd.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code == 0 {
			t.Fatalf("expected non-zero ExitError for agy subscription run, got %v", err)
		}
		if lookedUp {
			t.Fatalf("LookPath must not be called when policy denies run")
		}
		if !strings.Contains(stderr.String(), "https://") {
			t.Errorf("expected policy denial with DocURL on stderr, got: %s", stderr.String())
		}
	})
}

func TestRunSessionsResume_RoundTrip_ClaudeAndCodex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	openStore := func() (*session.Store, error) {
		return session.Open(dbPath)
	}
	workDir := t.TempDir()

	t.Run("claude run -> sessions -> resume round-trip", func(t *testing.T) {
		argsFile := filepath.Join(t.TempDir(), "claude-args.txt")
		envFile := filepath.Join(t.TempDir(), "claude-env.txt")
		now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

		baseEnv := append([]string{}, os.Environ()...)
		baseEnv = provider.SetEnv(baseEnv, "ANTHROPIC_API_KEY", "sk-ant-should-not-leak")
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_RECORD_ARGS", argsFile)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_RECORD_ENV", envFile)

		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return baseEnv },
			Getwd:         func() (string, error) { return workDir, nil },
			OpenStore:     openStore,
			Now:           func() time.Time { return now },
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		// 1. run claude
		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"run", "claude", "--model", "claude-sonnet-4", "README を要約して"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("run claude error: %v (stderr=%s)", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "hello from fake claude") {
			t.Errorf("run stdout missing fake output:\n%s", stdout.String())
		}
		if !strings.Contains(stdout.String(), render.SubscriptionCostDisclaimer) {
			t.Errorf("run stdout missing subscription reference-cost disclaimer:\n%s", stdout.String())
		}

		recordedArgs, _ := os.ReadFile(argsFile)
		if !strings.Contains(string(recordedArgs), "--permission-mode\nplan\n") || !strings.Contains(string(recordedArgs), "--model\nclaude-sonnet-4\n") {
			t.Errorf("unexpected recorded claude args:\n%s", string(recordedArgs))
		}
		recordedEnv, _ := os.ReadFile(envFile)
		if strings.Contains(string(recordedEnv), "ANTHROPIC_API_KEY=") {
			t.Errorf("ANTHROPIC_API_KEY leaked in subscription mode")
		}

		// Extract saved local session ID from store
		st, err := openStore()
		if err != nil {
			t.Fatalf("openStore: %v", err)
		}
		list, err := st.List(context.Background())
		_ = st.Close()
		if err != nil || len(list) != 1 {
			t.Fatalf("expected 1 saved session, got %d (err=%v)", len(list), err)
		}
		localID := list[0].ID
		if list[0].Provider != "claude" || list[0].ProviderSessionID != "fake-claude-sess-001" {
			t.Errorf("saved session mismatch: %+v", list[0])
		}

		// 2. resume <local-id>
		later := now.Add(15 * time.Minute)
		stdout.Reset()
		stderr.Reset()
		deps.Now = func() time.Time { return later }

		resumeCmd := NewRootCmd(deps)
		resumeCmd.SetArgs([]string{"resume", localID, "続きを書いて"})
		if err := resumeCmd.Execute(); err != nil {
			t.Fatalf("resume claude error: %v (stderr=%s)", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "resumed:fake-claude-sess-001") {
			t.Errorf("resume stdout missing resumed text:\n%s", stdout.String())
		}
		recordedResumeArgs, _ := os.ReadFile(argsFile)
		if !strings.Contains(string(recordedResumeArgs), "--resume\nfake-claude-sess-001\n") {
			t.Errorf("resume did not pass --resume flag:\n%s", string(recordedResumeArgs))
		}

		// Confirm last_run_at updated
		st, _ = openStore()
		updated, err := st.Get(context.Background(), localID)
		_ = st.Close()
		if err != nil {
			t.Fatalf("Get updated session: %v", err)
		}
		if !updated.LastRunAt.Equal(later) {
			t.Errorf("LastRunAt = %v, want %v", updated.LastRunAt, later)
		}
	})

	t.Run("codex run --json -> resume round-trip and --no-store", func(t *testing.T) {
		argsFile := filepath.Join(t.TempDir(), "codex-args.txt")
		cwdFile := filepath.Join(t.TempDir(), "codex-cwd.txt")
		baseEnv := append([]string{}, os.Environ()...)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_RECORD_ARGS", argsFile)
		baseEnv = provider.SetEnv(baseEnv, "FAKE_CLI_RECORD_CWD", cwdFile)

		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      fakeBinLookPath(t),
			Environ:       func() []string { return baseEnv },
			Getwd:         func() (string, error) { return workDir, nil },
			OpenStore:     openStore,
			Logger:        logging.NewWriter(&bytes.Buffer{}),
			Stdout:        &stdout,
			Stderr:        &stderr,
		}

		// 1. run codex with --json
		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"run", "codex", "--json", "--perm", "edit", "--cwd", workDir, "テスト"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("run codex error: %v (stderr=%s)", err, stderr.String())
		}

		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if len(lines) < 3 {
			t.Fatalf("expected >=3 JSONL lines, got %d:\n%s", len(lines), stdout.String())
		}
		for i, line := range lines {
			var ev event.Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatalf("JSONL line %d invalid JSON: %v (%s)", i, err, line)
			}
			if ev.Provider != "codex" {
				t.Errorf("line %d provider = %q, want codex", i, ev.Provider)
			}
		}

		st, _ := openStore()
		list, _ := st.List(context.Background())
		_ = st.Close()
		var codexID string
		for _, r := range list {
			if r.Provider == "codex" {
				codexID = r.ID
			}
		}
		if codexID == "" {
			t.Fatalf("expected codex session to be saved")
		}

		// 2. resume codex session
		stdout.Reset()
		stderr.Reset()
		resCmd := NewRootCmd(deps)
		resCmd.SetArgs([]string{"resume", codexID, "もっと詳しく"})
		if err := resCmd.Execute(); err != nil {
			t.Fatalf("resume codex error: %v (stderr=%s)", err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "resumed:fake-codex-thread-001") {
			t.Errorf("resume codex stdout unexpected:\n%s", stdout.String())
		}
		recordedArgs, _ := os.ReadFile(argsFile)
		if !strings.Contains(string(recordedArgs), "exec\nresume\n--json\n") {
			t.Errorf("unexpected codex resume args:\n%s", string(recordedArgs))
		}
		recordedCwd, _ := os.ReadFile(cwdFile)
		resolvedWorkDir, _ := filepath.EvalSymlinks(workDir)
		resolvedRecorded, _ := filepath.EvalSymlinks(strings.TrimSpace(string(recordedCwd)))
		if resolvedRecorded != resolvedWorkDir {
			t.Errorf("codex resume cwd = %q, want %q", resolvedRecorded, resolvedWorkDir)
		}

		// 3. --no-store does not add a record
		beforeCount := len(list)
		stdout.Reset()
		stderr.Reset()
		noStoreCmd := NewRootCmd(deps)
		noStoreCmd.SetArgs([]string{"run", "codex", "--no-store", "一時実行"})
		if err := noStoreCmd.Execute(); err != nil {
			t.Fatalf("run --no-store error: %v", err)
		}
		st, _ = openStore()
		afterList, _ := st.List(context.Background())
		_ = st.Close()
		if len(afterList) != beforeCount {
			t.Errorf("--no-store should not persist session: before=%d after=%d", beforeCount, len(afterList))
		}
	})

	t.Run("resume nonexistent local session ID fails without launching CLI", func(t *testing.T) {
		lookedUp := false
		deps := Deps{
			OpenStore: openStore,
			LookPath: func(file string) (string, error) {
				lookedUp = true
				return "", errors.New("should not be called")
			},
			Logger: logging.NewWriter(&bytes.Buffer{}),
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
		}
		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"resume", "does-not-exist", "hello"})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected error for nonexistent session ID")
		}
		if lookedUp {
			t.Fatalf("LookPath must not be called when session ID does not exist")
		}
	})
}
