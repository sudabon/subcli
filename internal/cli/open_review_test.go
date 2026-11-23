package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/config"
	"github.com/sudabon/subcli/internal/memory"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/session"
)

func TestOpenSkipsGitTrackedMemory(t *testing.T) {
	repo := t.TempDir()
	if err := exec.Command("git", "init", "-q", repo).Run(); err != nil {
		t.Fatal(err)
	}
	store := memory.ForRoot(repo, true)
	if _, err := store.Init(memory.InitOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", repo, "add", "-f", "--", ".subcli/memory/MEMORY.md").Run(); err != nil {
		t.Fatal(err)
	}
	projections := []string{
		filepath.Join(store.ProjectionDir("claude"), "CLAUDE.md"),
		filepath.Join(store.ProjectionDir("agy"), "AGENTS.md"),
	}
	const previous = "existing projection"
	for _, path := range projections {
		if err := os.WriteFile(path, []byte(previous), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, explicit := range []bool{false, true} {
		deps := testDeps(t)
		var stderr bytes.Buffer
		deps.Stderr = &stderr
		deps.Getwd = func() (string, error) { return repo, nil }
		var launched []string
		deps.ExecReplace = func(_ string, argv, _ []string) error {
			launched = argv
			return nil
		}
		args := []string{"open", "codex"}
		if explicit {
			args = append(args, "--memory")
		}
		cmd := NewRootCmd(deps)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if explicit {
			if err == nil || !strings.Contains(err.Error(), ".subcli") || launched != nil {
				t.Fatalf("explicit attach: argv=%q, err=%v", launched, err)
			}
		} else if err != nil || !slices.Equal(launched, []string{"codex"}) || !strings.Contains(stderr.String(), ".subcli") {
			t.Fatalf("auto attach: argv=%q, stderr=%q, err=%v", launched, stderr.String(), err)
		}
		for _, path := range projections {
			data, err := os.ReadFile(path)
			if err != nil || string(data) != previous {
				t.Fatalf("%s was modified by %v: %q, err=%v", path, args, data, err)
			}
		}
	}
}

func TestCommandsRejectMissingAPIKeyBeforeLaunching(t *testing.T) {
	const key = "SUBCLI_TEST_REQUIRED_KEY"
	for _, args := range [][]string{
		{"open", "agy", "--memory"},
		{"run", "codex", "hello"},
		{"compare", "--providers", "claude,codex", "hello"},
		{"chain", "--steps", "claude,codex", "--prompt", "hello", "--then", "{{.Prev}}"},
		{"resume", "saved", "hello"},
	} {
		t.Run(args[0], func(t *testing.T) {
			deps := testDeps(t)
			deps.LoadConfig = func() (config.Config, error) {
				cfg := config.Default()
				cfg.Providers.Codex.Auth, cfg.Providers.Codex.APIKeyEnv = provider.AuthAPIKey, key
				cfg.Providers.Agy.Auth, cfg.Providers.Agy.APIKeyEnv = provider.AuthAPIKey, key
				return cfg, nil
			}
			cwd, err := deps.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			invocations := filepath.Join(cwd, "invocations.log")
			deps.Environ = func() []string {
				return []string{"FAKE_CLI_INVOCATION_LOG=" + invocations, "PATH=" + os.Getenv("PATH")}
			}
			deps.ExecReplace = func(string, []string, []string) error {
				t.Fatal("launched provider with missing key")
				return nil
			}
			if args[0] == "resume" {
				st, err := deps.OpenStore()
				if err != nil {
					t.Fatal(err)
				}
				defer st.Close()
				rec, err := st.Create(t.Context(), session.Record{
					ID: "saved", Provider: "codex", ProviderSessionID: "example", Cwd: cwd,
				})
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					current, err := st.Get(t.Context(), rec.ID)
					if err != nil || !current.LastRunAt.Equal(rec.LastRunAt) {
						t.Errorf("failed resume changed last run time: %+v, err=%v", current, err)
					}
				}()
			}
			cmd := NewRootCmd(deps)
			cmd.SetArgs(args)
			err = cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error=%v, want missing %s", err, key)
			}
			for _, path := range []string{invocations, filepath.Join(cwd, ".subcli")} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("unexpected side effect at %s: %v", path, err)
				}
			}
		})
	}
}

func TestDoctorReportsMissingAPIKeys(t *testing.T) {
	deps := testDeps(t)
	cfg := config.Default()
	cfg.Providers.Claude.Auth, cfg.Providers.Claude.APIKeyEnv = provider.AuthAPIKey, "CUSTOM_CLAUDE_KEY"
	cfg.Providers.Codex.Auth, cfg.Providers.Codex.APIKeyEnv = provider.AuthAPIKey, "CUSTOM_CODEX_KEY"
	cfg.Providers.Agy.Auth, cfg.Providers.Agy.APIKeyEnv = provider.AuthAPIKey, "CUSTOM_AGY_KEY"
	deps.LoadConfig = func() (config.Config, error) { return cfg, nil }
	deps.Environ = func() []string { return []string{"CUSTOM_CODEX_KEY=", "CUSTOM_AGY_KEY= "} }
	deps.RunCapture = func(context.Context, string, []string, []string) (string, error) {
		t.Fatal("doctor launched a provider with a missing API key")
		return "", nil
	}
	var stdout bytes.Buffer
	deps.Stdout = &stdout
	cmd := NewRootCmd(deps)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"CUSTOM_CLAUDE_KEY", "CUSTOM_CODEX_KEY", "CUSTOM_AGY_KEY"} {
		if !strings.Contains(stdout.String(), key) {
			t.Errorf("missing %s in doctor output: %s", key, stdout.String())
		}
	}
}

func TestOpenAgySubscriptionRejectsHeadlessArgs(t *testing.T) {
	deps := testDeps(t)
	var launched []string
	deps.ExecReplace = func(_ string, argv, _ []string) error {
		launched = argv
		return nil
	}
	cmd := NewRootCmd(deps)
	cmd.SetArgs([]string{"open", "agy", "--no-memory"})
	if err := cmd.Execute(); err != nil || !slices.Equal(launched, []string{"agy"}) {
		t.Fatalf("plain open agy: argv=%q, err=%v", launched, err)
	}

	for _, extra := range [][]string{
		{"-p", "task", "--output-format", "stream-json"},
		{"--prompt=task"},
		{"acp"},
		{"--remote-control"},
	} {
		deps := testDeps(t)
		deps.ExecReplace = func(_ string, argv, _ []string) error {
			t.Fatalf("launched agy with %q", argv)
			return nil
		}
		cmd := NewRootCmd(deps)
		cmd.SetArgs(append([]string{"open", "agy", "--no-memory", "--"}, extra...))
		if err := cmd.Execute(); err == nil {
			t.Errorf("open agy -- %q was accepted", extra)
		}
	}
}
