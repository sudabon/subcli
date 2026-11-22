package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sudabon/subcli/internal/event"
)

// AuthMode specifies how a provider authenticates.
type AuthMode string

const (
	AuthSubscription AuthMode = "subscription"
	AuthAPIKey       AuthMode = "api_key"
)

// IsValid reports whether the auth mode is one of the supported values.
func (m AuthMode) IsValid() bool {
	return m == AuthSubscription || m == AuthAPIKey
}

// Perm specifies the permission level for headless execution.
type Perm string

const (
	PermReadOnly Perm = "read-only"
	PermEdit     Perm = "edit"
)

// RunRequest describes a headless execution request for a provider.
type RunRequest struct {
	Prompt            string
	Model             string
	Cwd               string
	Perm              Perm
	ProviderSessionID string
}

// ErrUnsupported indicates that a provider does not support the requested operation.
var ErrUnsupported = errors.New("operation unsupported by provider")

// Provider adapts an official CLI for subcli.
type Provider interface {
	ID() string
	Binary() string
	Version(ctx context.Context) (string, error)
	LoginArgv() []string
	InteractiveArgv(extra []string) []string
	HeadlessArgv(req RunRequest) ([]string, error)
	ChildEnv(base []string, auth AuthMode) []string
	NeedsPTY() bool
	Parse(line []byte) ([]event.Event, error)
}

// StatusChecker is an optional interface implemented by providers whose official
// CLI exposes a dedicated authentication status subcommand.
type StatusChecker interface {
	StatusArgv() []string
}

// MemoryContext carries repository-local shared memory paths and bounded inline
// instructions prepared by subcli before launching an interactive session.
type MemoryContext struct {
	RepoRoot      string
	MemoryDir     string
	MemoryFile    string
	ProjectionDir string
	InlinePrompt  string
}

// MemoryAdapter is an optional interface implemented by providers that support
// attaching repository-local shared memory during interactive sessions.
type MemoryAdapter interface {
	InteractiveMemoryArgs(mem MemoryContext, extra []string) []string
	InteractiveMemoryEnv(env []string, mem MemoryContext) []string
}

// Registry holds registered providers in deterministic insertion order.
type Registry struct {
	order     []string
	providers map[string]Provider
}

// NewRegistry builds a Registry from the given providers.
func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{
		providers: make(map[string]Provider, len(ps)),
	}
	for _, p := range ps {
		r.Register(p)
	}
	return r
}

// Register adds or replaces a provider in the registry.
func (r *Registry) Register(p Provider) {
	if _, exists := r.providers[p.ID()]; !exists {
		r.order = append(r.order, p.ID())
	}
	r.providers[p.ID()] = p
}

// Get returns the provider with the given ID.
func (r *Registry) Get(id string) (Provider, bool) {
	p, ok := r.providers[id]
	return p, ok
}

// List returns all registered providers in deterministic order.
func (r *Registry) List() []Provider {
	out := make([]Provider, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.providers[id])
	}
	return out
}

var dangerousFlagTokens = []string{
	"bypassPermissions",
	"danger-full-access",
	"--dangerously-skip-permissions",
	"--allow-dangerously-skip-permissions",
	"--dangerously-bypass-approvals-and-sandbox",
	"--dangerously-bypass-hook-trust",
}

// ContainsDangerousFlag reports whether args include any flag or value that
// bypasses official CLI permission or sandbox checks.
func ContainsDangerousFlag(args []string) (string, bool) {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		for _, banned := range dangerousFlagTokens {
			if strings.Contains(lower, strings.ToLower(banned)) {
				return banned, true
			}
		}
	}
	return "", false
}

// ValidateExtraArgs rejects any dangerous permission-bypass flags in extra args.
func ValidateExtraArgs(args []string) error {
	if flag, bad := ContainsDangerousFlag(args); bad {
		return fmt.Errorf("権限確認をバイパスする危険フラグ (%s) は subcli から渡せません", flag)
	}
	return nil
}

// LookupEnvSlice returns the value of key from a KEY=VALUE slice.
func LookupEnvSlice(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix), true
		}
	}
	return "", false
}

// FilterEnv removes all entries whose key matches any name in removeKeys.
func FilterEnv(base []string, removeKeys ...string) []string {
	removeSet := make(map[string]struct{}, len(removeKeys))
	for _, k := range removeKeys {
		removeSet[k] = struct{}{}
	}
	out := make([]string, 0, len(base))
	for _, entry := range base {
		k, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, drop := removeSet[k]; drop {
				continue
			}
		}
		out = append(out, entry)
	}
	return out
}

// SetEnv returns a new env slice with key set to value (replacing any prior value).
func SetEnv(base []string, key, value string) []string {
	out := FilterEnv(base, key)
	return append(out, key+"="+value)
}
