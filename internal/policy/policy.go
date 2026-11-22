package policy

import (
	"fmt"
	"os"

	"github.com/sudabon/subcli/internal/provider"
)

// Op identifies a subcli operation gated by policy.Check.
type Op string

const (
	OpLogin   Op = "login"
	OpOpen    Op = "open"
	OpRun     Op = "run"
	OpResume  Op = "resume"
	OpCompare Op = "compare"
	OpChain   Op = "chain"
)

// CallContext captures runtime execution signals used for policy evaluation.
type CallContext struct {
	StdinIsTTY bool
	UnderAgent bool
	AgentHint  string
}

// Decision represents the outcome of a policy check.
type Decision struct {
	Allow  bool
	Reason string
	DocURL string
}

// Check evaluates whether the requested operation is permitted under subcli's
// terms-compliance policy matrix and execution context rules.
func Check(prov string, auth provider.AuthMode, op Op, cc CallContext) Decision {
	rule, ok := policyMatrix[matrixKey{Provider: prov, Auth: auth, Op: op}]
	if !ok {
		return Decision{
			Allow:  false,
			Reason: fmt.Sprintf("未定義のプロバイダ・認証モード・操作の組み合わせ（provider=%q, auth=%q, op=%q）は許可されていません。", prov, auth, op),
			DocURL: urlAntigravityFAQ,
		}
	}

	if !rule.Allow {
		return Decision{
			Allow:  false,
			Reason: rule.Reason,
			DocURL: rule.DocURL,
		}
	}

	if rule.RequireDirectTTY {
		docURL := rule.DocURL
		if docURL == "" {
			docURL = urlAntigravityFAQ
		}
		if cc.UnderAgent {
			hint := cc.AgentHint
			if hint == "" {
				hint = "AI agent environment"
			}
			return Decision{
				Allow:  false,
				Reason: fmt.Sprintf("他の AI エージェント配下（検出理由: %s）からは %s（subscription）の %s を実行できません。対話ターミナルから人間が直接起動してください。", hint, prov, op),
				DocURL: docURL,
			}
		}
		if !cc.StdinIsTTY {
			return Decision{
				Allow:  false,
				Reason: fmt.Sprintf("標準入力が TTY ではない環境からは %s（subscription）の %s を実行できません。対話ターミナルから人間が直接起動してください。", prov, op),
				DocURL: docURL,
			}
		}
	}

	return Decision{Allow: true}
}

// DetectCallContext inspects stdin TTY state and detects whether subcli is
// running underneath another AI coding agent.
func DetectCallContext() CallContext {
	under, hint := DetectUnderAgent()
	return CallContext{
		StdinIsTTY: isTerminal(os.Stdin),
		UnderAgent: under,
		AgentHint:  hint,
	}
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
