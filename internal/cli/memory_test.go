package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sudabon/subcli/internal/config"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/memory"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
)

func TestMemoryCommands_Lifecycle(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(repo, "sub", "dir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	execCmd := func(args ...string) (string, string, error) {
		var stdout, stderr bytes.Buffer
		deps := Deps{
			LoadConfig: func() (config.Config, error) { return config.Default(), nil },
			Getwd:      func() (string, error) { return subdir, nil },
			Now:        func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) },
			Logger:     logging.NewWriter(&bytes.Buffer{}),
			Stdout:     &stdout,
			Stderr:     &stderr,
		}
		cmd := NewRootCmd(deps)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return stdout.String(), stderr.String(), err
	}

	// 1. status before init
	out, _, err := execCmd("memory", "status")
	if err != nil {
		t.Fatalf("memory status error: %v", err)
	}
	if !strings.Contains(out, "initialized: false") || !strings.Contains(out, "gitignored:  false") {
		t.Errorf("unexpected pre-init status:\n%s", out)
	}

	// 2. init from subdirectory creates <repo-root>/.subcli and updates <repo-root>/.gitignore
	out, _, err = execCmd("memory", "init")
	if err != nil {
		t.Fatalf("memory init error: %v", err)
	}
	if !strings.Contains(out, filepath.Join(repo, ".subcli", "memory", "MEMORY.md")) {
		t.Errorf("unexpected init stdout:\n%s", out)
	}
	giBytes, err := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if err != nil || !strings.Contains(string(giBytes), ".subcli/") {
		t.Fatalf("expected .gitignore at repo root to contain .subcli/, err=%v content=%q", err, string(giBytes))
	}

	// 3. add main + topic entries
	if _, _, err := execCmd("memory", "add", "Use Go 1.26 modules"); err != nil {
		t.Fatalf("memory add main error: %v", err)
	}
	if _, _, err := execCmd("memory", "add", "--topic", "arch", "Keep policy checks before exec"); err != nil {
		t.Fatalf("memory add topic error: %v", err)
	}

	// 4. show main and topic
	out, _, err = execCmd("memory", "show")
	if err != nil {
		t.Fatalf("memory show error: %v", err)
	}
	if !strings.Contains(out, "Use Go 1.26 modules") || !strings.Contains(out, "- [arch](./arch.md)") {
		t.Errorf("memory show missing expected content:\n%s", out)
	}

	out, _, err = execCmd("memory", "show", "arch")
	if err != nil {
		t.Fatalf("memory show arch error: %v", err)
	}
	if !strings.Contains(out, "Keep policy checks before exec") {
		t.Errorf("memory show arch missing entry:\n%s", out)
	}

	// 5. list and path
	out, _, err = execCmd("memory", "list")
	if err != nil {
		t.Fatalf("memory list error: %v", err)
	}
	if !strings.Contains(out, "MEMORY.md") || !strings.Contains(out, "arch.md") {
		t.Errorf("memory list output unexpected:\n%s", out)
	}
	out, _, err = execCmd("memory", "path")
	if err != nil {
		t.Fatalf("memory path error: %v", err)
	}
	if strings.TrimSpace(out) != filepath.Join(repo, ".subcli", "memory") {
		t.Errorf("memory path = %q, want %q", strings.TrimSpace(out), filepath.Join(repo, ".subcli", "memory"))
	}

	// 6. import from custom source dir
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "MEMORY.md"), []byte("Imported rule from claude\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err = execCmd("memory", "import", "claude", "--source", srcDir)
	if err != nil {
		t.Fatalf("memory import error: %v", err)
	}
	if !strings.Contains(out, "1 件") {
		t.Errorf("unexpected import output:\n%s", out)
	}

	// 7. sync & rm
	if _, _, err := execCmd("memory", "sync"); err != nil {
		t.Fatalf("memory sync error: %v", err)
	}
	if _, _, err := execCmd("memory", "rm", "arch"); err != nil {
		t.Fatalf("memory rm error: %v", err)
	}
	if _, _, err := execCmd("memory", "show", "arch"); err == nil {
		t.Errorf("expected error after removing topic arch")
	}
}

func TestOpenCmd_SharedMemoryIntegration(t *testing.T) {
	setupRepo := func(t *testing.T) string {
		t.Helper()
		repo := t.TempDir()
		if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		store := memory.ForRoot(repo, true)
		if _, err := store.AddEntry("Shared repo rule: prefer table tests", "", time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC), memory.InitOptions{}); err != nil {
			t.Fatal(err)
		}
		return repo
	}

	t.Run("claude open attaches autoMemoryDirectory, add-dir, and env", func(t *testing.T) {
		repo := setupRepo(t)
		var gotArgv, gotEnv []string
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      func(file string) (string, error) { return "/usr/local/bin/" + file, nil },
			Getwd:         func() (string, error) { return repo, nil },
			ExecReplace: func(_ string, argv []string, env []string) error {
				gotArgv = argv
				gotEnv = env
				return nil
			},
			Environ: func() []string { return []string{"PATH=/usr/bin"} },
			Logger:  logging.NewWriter(&bytes.Buffer{}),
			Stdout:  &bytes.Buffer{},
			Stderr:  &bytes.Buffer{},
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "claude", "--", "--model", "sonnet"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("open claude error: %v", err)
		}

		memDir := filepath.Join(repo, ".subcli", "memory")
		projDir := filepath.Join(repo, ".subcli", "projections", "claude")
		joined := strings.Join(gotArgv, "\n")
		if !strings.Contains(joined, "--add-dir\n"+memDir) {
			t.Errorf("argv missing --add-dir %s: %v", memDir, gotArgv)
		}
		if !strings.Contains(joined, "--add-dir\n"+projDir) {
			t.Errorf("argv missing --add-dir %s: %v", projDir, gotArgv)
		}
		// Verify --settings JSON parses cleanly with autoMemoryDirectory.
		settingsIdx := -1
		for i, a := range gotArgv {
			if a == "--settings" && i+1 < len(gotArgv) {
				settingsIdx = i + 1
				break
			}
		}
		if settingsIdx < 0 {
			t.Fatalf("argv missing --settings: %v", gotArgv)
		}
		var parsed map[string]string
		if err := json.Unmarshal([]byte(gotArgv[settingsIdx]), &parsed); err != nil {
			t.Fatalf("invalid --settings JSON %q: %v", gotArgv[settingsIdx], err)
		}
		if parsed["autoMemoryDirectory"] != memDir {
			t.Errorf("autoMemoryDirectory = %q, want %q", parsed["autoMemoryDirectory"], memDir)
		}
		// Extra args must be preserved at the end.
		if len(gotArgv) < 2 || gotArgv[len(gotArgv)-2] != "--model" || gotArgv[len(gotArgv)-1] != "sonnet" {
			t.Errorf("extra args not forwarded at end: %v", gotArgv)
		}
		if val, ok := provider.LookupEnvSlice(gotEnv, "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD"); !ok || val != "1" {
			t.Errorf("CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD = (%q, %v), want (1, true)", val, ok)
		}
	})

	t.Run("codex open attaches add-dir and developer_instructions", func(t *testing.T) {
		repo := setupRepo(t)
		var gotArgv []string
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      func(file string) (string, error) { return "/usr/local/bin/" + file, nil },
			Getwd:         func() (string, error) { return repo, nil },
			ExecReplace: func(_ string, argv []string, _ []string) error {
				gotArgv = argv
				return nil
			},
			Environ: func() []string { return []string{"PATH=/usr/bin"} },
			Logger:  logging.NewWriter(&bytes.Buffer{}),
			Stdout:  &bytes.Buffer{},
			Stderr:  &bytes.Buffer{},
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "codex", "--", "-m", "o3"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("open codex error: %v", err)
		}

		memDir := filepath.Join(repo, ".subcli", "memory")
		joined := strings.Join(gotArgv, "\n")
		if !strings.Contains(joined, "--add-dir\n"+memDir) {
			t.Errorf("argv missing --add-dir %s: %v", memDir, gotArgv)
		}
		if !strings.Contains(joined, "-c\ndeveloper_instructions=") || !strings.Contains(joined, "Shared repo rule: prefer table tests") {
			t.Errorf("argv missing developer_instructions with shared memory: %v", gotArgv)
		}
	})

	t.Run("agy open attaches projection directory via add-dir", func(t *testing.T) {
		repo := setupRepo(t)
		var gotArgv []string
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true, UnderAgent: false} },
			LookPath:      func(file string) (string, error) { return "/usr/local/bin/" + file, nil },
			Getwd:         func() (string, error) { return repo, nil },
			ExecReplace: func(_ string, argv []string, _ []string) error {
				gotArgv = argv
				return nil
			},
			Environ: func() []string { return []string{"PATH=/usr/bin"} },
			Logger:  logging.NewWriter(&bytes.Buffer{}),
			Stdout:  &bytes.Buffer{},
			Stderr:  &bytes.Buffer{},
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "agy"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("open agy error: %v", err)
		}

		projDir := filepath.Join(repo, ".subcli", "projections", "agy")
		want := []string{"agy", "--add-dir", projDir}
		if strings.Join(gotArgv, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("argv = %v, want %v", gotArgv, want)
		}
		// Verify projected AGENTS.md contains the shared memory entry.
		data, err := os.ReadFile(filepath.Join(projDir, "AGENTS.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "Shared repo rule: prefer table tests") {
			t.Errorf("agy AGENTS.md projection missing shared entry:\n%s", string(data))
		}
	})

	t.Run("--no-memory skips attaching even when initialized", func(t *testing.T) {
		repo := setupRepo(t)
		var gotArgv []string
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      func(file string) (string, error) { return "/usr/local/bin/" + file, nil },
			Getwd:         func() (string, error) { return repo, nil },
			ExecReplace: func(_ string, argv []string, _ []string) error {
				gotArgv = argv
				return nil
			},
			Environ: func() []string { return nil },
			Logger:  logging.NewWriter(&bytes.Buffer{}),
			Stdout:  &bytes.Buffer{},
			Stderr:  &bytes.Buffer{},
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "--no-memory", "agy"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("open --no-memory agy error: %v", err)
		}
		if len(gotArgv) != 1 || gotArgv[0] != "agy" {
			t.Errorf("argv = %v, want [agy]", gotArgv)
		}
	})

	t.Run("--memory initializes .subcli and .gitignore on demand", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		var gotArgv []string
		deps := Deps{
			LoadConfig:    func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext { return policy.CallContext{StdinIsTTY: true} },
			LookPath:      func(file string) (string, error) { return "/usr/local/bin/" + file, nil },
			Getwd:         func() (string, error) { return repo, nil },
			ExecReplace: func(_ string, argv []string, _ []string) error {
				gotArgv = argv
				return nil
			},
			Environ: func() []string { return nil },
			Logger:  logging.NewWriter(&bytes.Buffer{}),
			Stdout:  &bytes.Buffer{},
			Stderr:  &bytes.Buffer{},
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "--memory", "agy"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("open --memory agy error: %v", err)
		}
		if _, err := os.Stat(filepath.Join(repo, ".subcli", "memory", "MEMORY.md")); err != nil {
			t.Fatalf("expected MEMORY.md to be created: %v", err)
		}
		if gi, err := os.ReadFile(filepath.Join(repo, ".gitignore")); err != nil || !strings.Contains(string(gi), ".subcli/") {
			t.Fatalf("expected .gitignore to include .subcli/, err=%v", err)
		}
		if len(gotArgv) != 3 || gotArgv[1] != "--add-dir" {
			t.Errorf("expected --add-dir projection arg, got %v", gotArgv)
		}
	})

	t.Run("policy denial happens BEFORE --memory creates any files", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		deps := Deps{
			LoadConfig: func() (config.Config, error) { return config.Default(), nil },
			DetectContext: func() policy.CallContext {
				return policy.CallContext{StdinIsTTY: true, UnderAgent: true, AgentHint: "env:CLAUDECODE"}
			},
			LookPath: func(file string) (string, error) { return "/usr/local/bin/" + file, nil },
			Getwd:    func() (string, error) { return repo, nil },
			ExecReplace: func(_ string, _ []string, _ []string) error {
				t.Fatal("ExecReplace must not be called")
				return nil
			},
			Environ: func() []string { return nil },
			Logger:  logging.NewWriter(&bytes.Buffer{}),
			Stdout:  &bytes.Buffer{},
			Stderr:  &bytes.Buffer{},
		}

		cmd := NewRootCmd(deps)
		cmd.SetArgs([]string{"open", "--memory", "agy"})
		err := cmd.Execute()
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code == 0 {
			t.Fatalf("expected policy denial ExitError, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(repo, ".subcli")); !os.IsNotExist(statErr) {
			t.Errorf(".subcli must not be created when policy denies open")
		}
		if _, statErr := os.Stat(filepath.Join(repo, ".gitignore")); !os.IsNotExist(statErr) {
			t.Errorf(".gitignore must not be created when policy denies open")
		}
	})
}
