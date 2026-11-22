package agy

import (
	"context"
	"os/exec"
	"strings"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
)

const defaultAPIKeyEnv = "GEMINI_API_KEY"

// Adapter implements provider.Provider for the official Antigravity CLI.
type Adapter struct {
	apiKeyEnv string
}

// New creates an Antigravity CLI provider adapter.
func New(apiKeyEnv string) *Adapter {
	if strings.TrimSpace(apiKeyEnv) == "" {
		apiKeyEnv = defaultAPIKeyEnv
	}
	return &Adapter{apiKeyEnv: apiKeyEnv}
}

func (a *Adapter) ID() string     { return "agy" }
func (a *Adapter) Binary() string { return "agy" }

func (a *Adapter) Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, a.Binary(), "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// LoginArgv returns nil because agy launches its interactive sign-in flow directly
// without a dedicated login subcommand (verified in Phase 0).
func (a *Adapter) LoginArgv() []string {
	return nil
}

func (a *Adapter) InteractiveArgv(extra []string) []string {
	if len(extra) == 0 {
		return nil
	}
	out := make([]string, len(extra))
	copy(out, extra)
	return out
}

// InteractiveMemoryArgs attaches the generated Antigravity rule projection
// directory via --add-dir so AGENTS.md is discovered at interactive startup.
func (a *Adapter) InteractiveMemoryArgs(mem provider.MemoryContext, _ []string) []string {
	if projDir := strings.TrimSpace(mem.ProjectionDir); projDir != "" {
		return []string{"--add-dir", projDir}
	}
	return nil
}

// InteractiveMemoryEnv returns env unchanged for Antigravity CLI.
func (a *Adapter) InteractiveMemoryEnv(env []string, _ provider.MemoryContext) []string {
	return env
}

func (a *Adapter) HeadlessArgv(_ provider.RunRequest) ([]string, error) {
	return nil, provider.ErrUnsupported
}

func (a *Adapter) ChildEnv(base []string, auth provider.AuthMode) []string {
	switch auth {
	case provider.AuthSubscription:
		return provider.FilterEnv(base, "GEMINI_API_KEY", "GOOGLE_API_KEY")
	case provider.AuthAPIKey:
		cleaned := provider.FilterEnv(base, "GEMINI_API_KEY", "GOOGLE_API_KEY")
		if val, ok := provider.LookupEnvSlice(base, a.apiKeyEnv); ok && val != "" {
			return provider.SetEnv(cleaned, "GEMINI_API_KEY", val)
		}
		return cleaned
	default:
		return provider.FilterEnv(base, "GEMINI_API_KEY", "GOOGLE_API_KEY")
	}
}

func (a *Adapter) NeedsPTY() bool { return false }

func (a *Adapter) Parse(_ []byte) ([]event.Event, error) {
	return nil, provider.ErrUnsupported
}
