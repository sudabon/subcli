package policy

import (
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/provider"
)

func TestPolicyMatrix(t *testing.T) {
	providers := []string{"claude", "codex", "agy"}
	authModes := []provider.AuthMode{provider.AuthSubscription, provider.AuthAPIKey}
	ops := []Op{OpLogin, OpOpen, OpRun, OpResume, OpCompare, OpChain}
	contexts := []struct {
		name string
		cc   CallContext
	}{
		{"direct_tty", CallContext{StdinIsTTY: true, UnderAgent: false}},
		{"non_tty", CallContext{StdinIsTTY: false, UnderAgent: false}},
		{"under_agent_tty", CallContext{StdinIsTTY: true, UnderAgent: true, AgentHint: "env:CLAUDECODE"}},
		{"under_agent_non_tty", CallContext{StdinIsTTY: false, UnderAgent: true, AgentHint: "process:codex"}},
	}

	expectedAllow := func(prov string, auth provider.AuthMode, op Op, cc CallContext) bool {
		switch {
		case auth == provider.AuthAPIKey && op == OpLogin:
			return false
		case prov == "claude" || prov == "codex":
			return true
		case prov == "agy" && auth == provider.AuthAPIKey:
			return true
		case prov == "agy" && auth == provider.AuthSubscription:
			if op == OpLogin || op == OpOpen {
				return cc.StdinIsTTY && !cc.UnderAgent
			}
			return false
		default:
			return false
		}
	}

	for _, prov := range providers {
		for _, auth := range authModes {
			for _, op := range ops {
				for _, ctx := range contexts {
					name := prov + "/" + string(auth) + "/" + string(op) + "/" + ctx.name
					t.Run(name, func(t *testing.T) {
						got := Check(prov, auth, op, ctx.cc)
						want := expectedAllow(prov, auth, op, ctx.cc)
						if got.Allow != want {
							t.Fatalf("Check(%s, %s, %s, %+v).Allow = %v, want %v (reason=%q)", prov, auth, op, ctx.cc, got.Allow, want, got.Reason)
						}
						if !got.Allow {
							if strings.TrimSpace(got.Reason) == "" {
								t.Errorf("denied decision must include a Japanese reason")
							}
							if !strings.HasPrefix(got.DocURL, "https://") {
								t.Errorf("denied decision DocURL = %q, want https:// URL", got.DocURL)
							}
							if prov == "agy" && auth == provider.AuthSubscription && (op == OpLogin || op == OpOpen) && ctx.cc.UnderAgent {
								if !strings.Contains(got.Reason, ctx.cc.AgentHint) {
									t.Errorf("expected AgentHint %q in reason %q", ctx.cc.AgentHint, got.Reason)
								}
							}
						}
					})
				}
			}
		}
	}

	// Unknown combinations must be denied safely.
	t.Run("unknown_provider", func(t *testing.T) {
		dec := Check("unknown", provider.AuthSubscription, OpOpen, CallContext{StdinIsTTY: true})
		if dec.Allow || dec.Reason == "" || !strings.HasPrefix(dec.DocURL, "https://") {
			t.Fatalf("unexpected decision for unknown provider: %+v", dec)
		}
	})
}
