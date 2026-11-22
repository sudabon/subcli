package policy

import (
	"errors"
	"os"
	"testing"
)

func TestDetectUnderAgent_EnvVars(t *testing.T) {
	tests := []struct {
		name     string
		environ  []string
		wantHint string
	}{
		{"claude code", []string{"PATH=/usr/bin", "CLAUDECODE=1"}, "env:CLAUDECODE"},
		{"claude entrypoint", []string{"CLAUDE_CODE_ENTRYPOINT=cli"}, "env:CLAUDE_CODE_ENTRYPOINT"},
		{"codex sandbox", []string{"CODEX_SANDBOX=seatbelt"}, "env:CODEX_SANDBOX"},
		{"codex sandbox prefix", []string{"CODEX_SANDBOX_EXTRA=1"}, "env:CODEX_SANDBOX_EXTRA"},
		{"codex thread", []string{"CODEX_THREAD_ID=abc"}, "env:CODEX_THREAD_ID"},
		{"antigravity agent", []string{"ANTIGRAVITY_AGENT=1"}, "env:ANTIGRAVITY_AGENT"},
		{"generic ai agent", []string{"AI_AGENT=true"}, "env:AI_AGENT"},
	}

	noAncestors := func(int32, int) ([]string, error) { return []string{"zsh", "login"}, nil }

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			under, hint, err := detectUnderAgent(tc.environ, 1234, noAncestors)
			if err != nil {
				t.Fatal(err)
			}
			if !under {
				t.Fatalf("expected UnderAgent=true for %v", tc.environ)
			}
			if hint != tc.wantHint {
				t.Errorf("hint = %q, want %q", hint, tc.wantHint)
			}
		})
	}
}

func TestDetectUnderAgent_ProcessChain(t *testing.T) {
	tests := []struct {
		name      string
		ancestors []string
		wantUnder bool
		wantHint  string
		wantErr   error
	}{
		{
			name:      "direct parent claude",
			ancestors: []string{"claude", "zsh"},
			wantUnder: true,
			wantHint:  "process:claude",
		},
		{
			name:      "windows exe suffix normalized",
			ancestors: []string{"cmd.exe", "CODEX.EXE"},
			wantUnder: true,
			wantHint:  "process:codex",
		},
		{
			name:      "cursor-agent at depth 10 detected",
			ancestors: []string{"sh", "sh", "sh", "sh", "sh", "sh", "sh", "sh", "sh", "cursor-agent"},
			wantUnder: true,
			wantHint:  "process:cursor-agent",
		},
		{
			name:      "unverified ancestry beyond depth 10",
			wantErr:   errAncestorDepth,
			ancestors: []string{"sh", "sh", "sh", "sh", "sh", "sh", "sh", "sh", "sh", "sh", "claude"},
			wantUnder: false,
			wantHint:  "",
		},
		{
			name:      "non-agent terminal chain",
			ancestors: []string{"zsh", "tmux", "iTerm2", "launchd"},
			wantUnder: false,
			wantHint:  "",
		},
	}

	cleanEnv := []string{"PATH=/usr/bin", "TERM=xterm-256color", "CLAUDECODE="}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			under, hint, err := detectUnderAgent(cleanEnv, 9999, func(int32, int) ([]string, error) {
				return tc.ancestors, nil
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if under != tc.wantUnder || hint != tc.wantHint {
				t.Fatalf("detectUnderAgent() = (%v, %q), want (%v, %q)", under, hint, tc.wantUnder, tc.wantHint)
			}
		})
	}
}

func TestListAncestorProcessNames_CurrentProcessSanity(t *testing.T) {
	names, err := listAncestorProcessNames(int32(os.Getpid()), MaxAncestorDepth)
	if err != nil && !errors.Is(err, errAncestorDepth) {
		t.Fatalf("listAncestorProcessNames() error: %v", err)
	}
	if len(names) > MaxAncestorDepth {
		t.Errorf("returned %d ancestors, exceeds max %d", len(names), MaxAncestorDepth)
	}
}

func TestListAncestorProcessNamesDepthExhausted(t *testing.T) {
	_, err := listAncestorProcessNames(int32(os.Getpid()), 0)
	if !errors.Is(err, errAncestorDepth) {
		t.Fatalf("error = %v, want depth limit", err)
	}
}
