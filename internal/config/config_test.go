package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/provider"
)

func TestLoadFromPath_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[providers.claude]
auth = "subscription"
api_key_env = "MY_ANTHROPIC_KEY"
max_concurrency = 3

[providers.codex]
auth = "api_key"
api_key_env = "MY_OPENAI_KEY"
max_concurrency = 4

[providers.agy]
auth = "api_key"
api_key_env = "MY_GEMINI_KEY"
max_concurrency = 1
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath() unexpected error: %v", err)
	}

	if cfg.Providers.Claude.Auth != provider.AuthSubscription || cfg.Providers.Claude.APIKeyEnv != "MY_ANTHROPIC_KEY" || cfg.Providers.Claude.MaxConcurrency != 3 {
		t.Errorf("unexpected claude config: %+v", cfg.Providers.Claude)
	}
	if cfg.Providers.Codex.Auth != provider.AuthAPIKey || cfg.Providers.Codex.APIKeyEnv != "MY_OPENAI_KEY" || cfg.Providers.Codex.MaxConcurrency != 4 {
		t.Errorf("unexpected codex config: %+v", cfg.Providers.Codex)
	}
	if cfg.Providers.Agy.Auth != provider.AuthAPIKey || cfg.Providers.Agy.APIKeyEnv != "MY_GEMINI_KEY" || cfg.Providers.Agy.MaxConcurrency != 1 {
		t.Errorf("unexpected agy config: %+v", cfg.Providers.Agy)
	}
}

func TestLoadFromPath_MissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.toml")
	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath() unexpected error: %v", err)
	}

	for _, id := range []string{"claude", "codex", "agy"} {
		pc, err := cfg.Provider(id)
		if err != nil {
			t.Fatalf("Provider(%q) error: %v", id, err)
		}
		if pc.Auth != provider.AuthSubscription {
			t.Errorf("%s default auth = %q, want %q", id, pc.Auth, provider.AuthSubscription)
		}
		if pc.MaxConcurrency != DefaultMaxConcurrency {
			t.Errorf("%s default max_concurrency = %d, want %d", id, pc.MaxConcurrency, DefaultMaxConcurrency)
		}
	}
}

func TestLoadFromReader_InvalidConfigsRejected(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{
			name: "invalid auth mode",
			toml: `
[providers.claude]
auth = "oauth"
`,
		},
		{
			name: "zero max_concurrency",
			toml: `
[providers.codex]
max_concurrency = 0
`,
		},
		{
			name: "negative max_concurrency",
			toml: `
[providers.agy]
max_concurrency = -2
`,
		},
		{
			name: "multi-account array rejected",
			toml: `
[[providers.claude.accounts]]
auth = "subscription"
`,
		},
		{
			name: "second provider alias rejected",
			toml: `
[providers.claude_work]
auth = "subscription"
`,
		},
		{
			name: "raw api_key field rejected",
			toml: `
[providers.claude]
auth = "api_key"
api_key = "sk-test-raw-value"
`,
		},
		{
			name: "raw secret in api_key_env rejected",
			toml: `
[providers.agy]
auth = "api_key"
api_key_env = "AIzaSyExampleKey12345"
`,
		},
		{
			name: "unknown top-level key rejected",
			toml: `
unknown_section = true
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFromReader(strings.NewReader(tc.toml))
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestDefaultPath_RespectsXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error: %v", err)
	}
	want := filepath.Join(xdg, "subcli", "config.toml")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}
