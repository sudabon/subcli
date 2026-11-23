package policy

import (
	"errors"
	"os"
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
		{"direct_tty", CallContext{StdinIsTTY: true, StdoutIsTTY: true, UnderAgent: false}},
		{"redirected_stdout", CallContext{StdinIsTTY: true}},
		{"unknown_ancestry", CallContext{StdinIsTTY: true, StdoutIsTTY: true, AncestryError: os.ErrPermission}},
		{"non_tty", CallContext{StdinIsTTY: false, UnderAgent: false}},
		{"under_agent_tty", CallContext{StdinIsTTY: true, StdoutIsTTY: true, UnderAgent: true, AgentHint: "env:CLAUDECODE"}},
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
				return cc.StdinIsTTY && cc.StdoutIsTTY && !cc.UnderAgent && cc.AncestryError == nil
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
		dec := Check("unknown", provider.AuthSubscription, OpOpen, CallContext{StdinIsTTY: true, StdoutIsTTY: true})
		if dec.Allow || dec.Reason == "" || !strings.HasPrefix(dec.DocURL, "https://") {
			t.Fatalf("unexpected decision for unknown provider: %+v", dec)
		}
	})
}

func TestNullDeviceIsNotTerminal(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	stdin, stdout := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = f, f
	defer func() { os.Stdin, os.Stdout = stdin, stdout }()
	cc := DetectCallContext()
	if cc.StdinIsTTY || cc.StdoutIsTTY {
		t.Fatalf("%s detected as a terminal: %+v", os.DevNull, cc)
	}
}

func TestIncompleteAncestry(t *testing.T) {
	_, _, err := detectUnderAgent(nil, 123, func(int32, int) ([]string, error) {
		return []string{"terminal"}, os.ErrPermission
	})
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("incomplete ancestry error = %v", err)
	}
}
