package agy

import (
	"context"
	"fmt"
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

// InteractiveAuthArgs pins API-key sessions to the Gemini backend. For
// subscriptions, only options for a local interactive session are forwarded.
func (a *Adapter) InteractiveAuthArgs(auth provider.AuthMode, extra []string) ([]string, error) {
	switch auth {
	case provider.AuthAPIKey:
		for _, arg := range extra {
			name, _, _ := strings.Cut(arg, "=")
			if name == "--settings" || name == "-settings" {
				return nil, fmt.Errorf("agy の API キーモードでは --settings は subcli が設定します")
			}
		}
		return append([]string{"--settings", `{"modelProvider":"gemini"}`}, extra...), nil
	case provider.AuthSubscription:
		if err := validateInteractiveOptions(extra); err != nil {
			return nil, err
		}
		return a.InteractiveArgv(extra), nil
	default:
		return nil, fmt.Errorf("agy の認証モード %q は未対応です", auth)
	}
}

// Match the option arity in the recorded agy CLI contract. Reject subcommands
// and unrecognized options so a newer CLI cannot silently select a remote or
// non-interactive entry point in subscription mode.
func validateInteractiveOptions(args []string) error {
	for i := 0; i < len(args); i++ {
		option, _, inline := strings.Cut(args[i], "=")
		name := strings.TrimPrefix(strings.TrimPrefix(option, "-"), "-")
		if option != name {
			switch name {
			case "c", "continue", "new-project", "sandbox":
				continue
			case "add-dir", "agent", "conversation", "effort", "i", "log-file", "mode", "model", "project", "prompt-interactive":
				if !inline {
					if i+1 == len(args) || strings.HasPrefix(args[i+1], "-") {
						return fmt.Errorf("agy の %s に値を指定してください", option)
					}
					i++
				}
				continue
			}
		}
		return fmt.Errorf("agy のサブスクリプションでは対話起動用のオプションのみ指定できます")
	}
	return nil
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
