package provider_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/agy"
	"github.com/sudabon/subcli/internal/provider/claude"
	"github.com/sudabon/subcli/internal/provider/codex"
)

func TestProviders_ArgvAndHeadlessStub(t *testing.T) {
	c := claude.New("MY_ANTHROPIC_KEY")
	x := codex.New("MY_OPENAI_KEY")
	a := agy.New("MY_GEMINI_KEY")

	if got := c.LoginArgv(); !reflect.DeepEqual(got, []string{"auth", "login"}) {
		t.Errorf("claude.LoginArgv() = %v, want [auth login]", got)
	}
	if got := c.StatusArgv(); !reflect.DeepEqual(got, []string{"auth", "status"}) {
		t.Errorf("claude.StatusArgv() = %v, want [auth status]", got)
	}
	if got := x.LoginArgv(); !reflect.DeepEqual(got, []string{"login"}) {
		t.Errorf("codex.LoginArgv() = %v, want [login]", got)
	}
	if got := x.StatusArgv(); !reflect.DeepEqual(got, []string{"login", "status"}) {
		t.Errorf("codex.StatusArgv() = %v, want [login status]", got)
	}
	if got := a.LoginArgv(); len(got) != 0 {
		t.Errorf("agy.LoginArgv() = %v, want empty", got)
	}
	if _, ok := provider.Provider(a).(provider.StatusChecker); ok {
		t.Errorf("agy should not implement StatusChecker when no official status subcommand exists")
	}

	extra := []string{"--model", "test-model"}
	for _, p := range []provider.Provider{c, x, a} {
		got := p.InteractiveArgv(extra)
		if !reflect.DeepEqual(got, extra) {
			t.Errorf("%s.InteractiveArgv() = %v, want %v", p.ID(), got, extra)
		}
	}
	for _, p := range []provider.Provider{c, x} {
		if argv, err := p.HeadlessArgv(provider.RunRequest{Prompt: "hi", Perm: provider.PermReadOnly}); err != nil || len(argv) == 0 {
			t.Errorf("%s.HeadlessArgv() = (%v, %v), want non-empty argv and nil error", p.ID(), argv, err)
		}
	}
	if _, err := a.HeadlessArgv(provider.RunRequest{Prompt: "hi"}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("agy.HeadlessArgv() err = %v, want ErrUnsupported", err)
	}
}

func TestProviders_ChildEnv(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=sk-ant-existing",
		"ANTHROPIC_AUTH_TOKEN=tok-existing",
		"MY_ANTHROPIC_KEY=sk-ant-custom",
		"CODEX_API_KEY=sk-codex-existing",
		"OPENAI_API_KEY=sk-openai-existing",
		"MY_OPENAI_KEY=sk-openai-custom",
		"GEMINI_API_KEY=AIzaExisting",
		"GOOGLE_API_KEY=AIzaGoogleExisting",
		"MY_GEMINI_KEY=AIzaCustom",
	}

	t.Run("claude subscription strips API keys", func(t *testing.T) {
		env := claude.New("MY_ANTHROPIC_KEY").ChildEnv(base, provider.AuthSubscription)
		if _, ok := provider.LookupEnvSlice(env, "ANTHROPIC_API_KEY"); ok {
			t.Errorf("ANTHROPIC_API_KEY should be removed in subscription mode")
		}
		if _, ok := provider.LookupEnvSlice(env, "ANTHROPIC_AUTH_TOKEN"); ok {
			t.Errorf("ANTHROPIC_AUTH_TOKEN should be removed in subscription mode")
		}
		if val, ok := provider.LookupEnvSlice(env, "PATH"); !ok || val != "/usr/bin" {
			t.Errorf("PATH should be preserved, got %q (%v)", val, ok)
		}
	})

	t.Run("claude api_key maps configured env var", func(t *testing.T) {
		env := claude.New("MY_ANTHROPIC_KEY").ChildEnv(base, provider.AuthAPIKey)
		if val, ok := provider.LookupEnvSlice(env, "ANTHROPIC_API_KEY"); !ok || val != "sk-ant-custom" {
			t.Errorf("ANTHROPIC_API_KEY = %q (%v), want sk-ant-custom", val, ok)
		}
	})

	t.Run("codex subscription strips API keys", func(t *testing.T) {
		env := codex.New("MY_OPENAI_KEY").ChildEnv(base, provider.AuthSubscription)
		if _, ok := provider.LookupEnvSlice(env, "CODEX_API_KEY"); ok {
			t.Errorf("CODEX_API_KEY should be removed in subscription mode")
		}
		if _, ok := provider.LookupEnvSlice(env, "OPENAI_API_KEY"); ok {
			t.Errorf("OPENAI_API_KEY should be removed in subscription mode")
		}
	})

	t.Run("codex api_key maps configured env var to CODEX_API_KEY", func(t *testing.T) {
		env := codex.New("MY_OPENAI_KEY").ChildEnv(base, provider.AuthAPIKey)
		if val, ok := provider.LookupEnvSlice(env, "CODEX_API_KEY"); !ok || val != "sk-openai-custom" {
			t.Errorf("CODEX_API_KEY = %q (%v), want sk-openai-custom", val, ok)
		}
		if _, ok := provider.LookupEnvSlice(env, "OPENAI_API_KEY"); ok {
			t.Errorf("OPENAI_API_KEY should be stripped so only CODEX_API_KEY is passed")
		}
	})

	t.Run("agy subscription strips GEMINI_API_KEY and GOOGLE_API_KEY", func(t *testing.T) {
		env := agy.New("MY_GEMINI_KEY").ChildEnv(base, provider.AuthSubscription)
		if _, ok := provider.LookupEnvSlice(env, "GEMINI_API_KEY"); ok {
			t.Errorf("GEMINI_API_KEY should be removed in subscription mode")
		}
		if _, ok := provider.LookupEnvSlice(env, "GOOGLE_API_KEY"); ok {
			t.Errorf("GOOGLE_API_KEY should be removed in subscription mode")
		}
	})

	t.Run("agy api_key maps configured env var to GEMINI_API_KEY", func(t *testing.T) {
		env := agy.New("MY_GEMINI_KEY").ChildEnv(base, provider.AuthAPIKey)
		if val, ok := provider.LookupEnvSlice(env, "GEMINI_API_KEY"); !ok || val != "AIzaCustom" {
			t.Errorf("GEMINI_API_KEY = %q (%v), want AIzaCustom", val, ok)
		}
	})
}

func TestProviders_InteractiveMemoryAdapter(t *testing.T) {
	mem := provider.MemoryContext{
		RepoRoot:      "/repo",
		MemoryDir:     "/repo/.subcli/memory",
		MemoryFile:    "/repo/.subcli/memory/MEMORY.md",
		ProjectionDir: "/repo/.subcli/projections/test",
		InlinePrompt:  "Line 1\nLine \"2\"",
	}

	t.Run("claude respects user --settings override", func(t *testing.T) {
		c := claude.New("")
		args := c.InteractiveMemoryArgs(mem, []string{"--settings", `{"model":"opus"}`})
		for _, a := range args {
			if a == "--settings" {
				t.Errorf("claude should not inject --settings when user already supplied one: %v", args)
			}
		}
		env := c.InteractiveMemoryEnv([]string{"PATH=/usr/bin"}, mem)
		if v, ok := provider.LookupEnvSlice(env, "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD"); !ok || v != "1" {
			t.Errorf("expected CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1, got (%q, %v)", v, ok)
		}
	})

	t.Run("codex respects user developer_instructions override", func(t *testing.T) {
		x := codex.New("")
		args := x.InteractiveMemoryArgs(mem, []string{"-c", `developer_instructions="custom"`})
		for _, a := range args {
			if a == "-c" {
				t.Errorf("codex should not inject -c developer_instructions when user already supplied one: %v", args)
			}
		}
	})

	t.Run("agy returns --add-dir for projection directory", func(t *testing.T) {
		a := agy.New("")
		args := a.InteractiveMemoryArgs(mem, nil)
		want := []string{"--add-dir", mem.ProjectionDir}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("agy.InteractiveMemoryArgs = %v, want %v", args, want)
		}
	})
}
