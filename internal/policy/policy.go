package policy

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
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
	StdinIsTTY    bool
	StdoutIsTTY   bool
	AncestryError error
	UnderAgent    bool
	AgentHint     string
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
		if cc.AncestryError != nil {
			return Decision{
				Reason: fmt.Sprintf("祖先プロセスを確認できないため %s（subscription）の %s を実行できません: %v", prov, op, cc.AncestryError),
				DocURL: docURL,
			}
		}
		if !cc.StdinIsTTY || !cc.StdoutIsTTY {
			return Decision{
				Allow:  false,
				Reason: fmt.Sprintf("標準入力と標準出力の両方が TTY の場合に限り %s（subscription）の %s を実行できます。対話ターミナルから人間が直接起動してください。", prov, op),
				DocURL: docURL,
			}
		}
	}

	return Decision{Allow: true}
}

// DetectCallContext inspects stdin and stdout TTY state and detects whether subcli is
// running underneath another AI coding agent.
func DetectCallContext() CallContext {
	under, hint, err := detectUnderAgent(os.Environ(), int32(os.Getpid()), listAncestorProcessNames)
	return CallContext{
		StdinIsTTY:    isTerminal(os.Stdin),
		StdoutIsTTY:   isTerminal(os.Stdout),
		AncestryError: err,
		UnderAgent:    under,
		AgentHint:     hint,
	}
}

func isTerminal(f *os.File) bool {
	return f != nil && isatty.IsTerminal(f.Fd())
}
