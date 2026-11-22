package policy

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

// MaxAncestorDepth limits parent process chain traversal to 10 hops (spce.md §7.3).
const MaxAncestorDepth = 10

// knownAgentEnvVars lists environment variables confirmed during Phase 0
// (docs/cli-contracts/research-notes.md §2.4). Only variable names are checked;
// values are never logged or retained.
var knownAgentEnvVars = []string{
	// Claude Code
	"CLAUDECODE",
	"CLAUDE_CODE_ENTRYPOINT",
	"CLAUDE_CODE_SESSION_ID",
	"CLAUDE_PID",
	// Codex CLI
	"CODEX_SANDBOX",
	"CODEX_SANDBOX_NETWORK_DISABLED",
	"CODEX_THREAD_ID",
	"CODEX_SESSION_ID",
	"CODEX_CI",
	// Antigravity CLI
	"ANTIGRAVITY_AGENT",
	"ANTIGRAVITY_CONVERSATION_ID",
	"ANTIGRAVITY_PROJECT_ID",
	"ANTIGRAVITY_TRAJECTORY_ID",
	// Generic AI agent marker
	"AI_AGENT",
}

// knownAgentEnvPrefixes matches families of agent environment variables such as CODEX_SANDBOX*.
var knownAgentEnvPrefixes = []string{
	"CODEX_SANDBOX",
}

// knownAgentProcessNames lists known AI coding agent process basenames (lowercased, without .exe).
var knownAgentProcessNames = map[string]struct{}{
	"claude":       {},
	"codex":        {},
	"agy":          {},
	"opencode":     {},
	"openclaw":     {},
	"cursor-agent": {},
}

// DetectUnderAgent reports whether the current process appears to be running
// underneath another AI agent, returning a human-readable detection hint.
func DetectUnderAgent() (bool, string) {
	return detectUnderAgent(os.Environ(), int32(os.Getpid()), listAncestorProcessNames)
}

func detectUnderAgent(environ []string, pid int32, ancestorsFn func(pid int32, maxDepth int) ([]string, error)) (bool, string) {
	if envName, ok := detectAgentEnv(environ); ok {
		return true, "env:" + envName
	}
	if ancestorsFn != nil {
		names, err := ancestorsFn(pid, MaxAncestorDepth)
		if err == nil {
			limit := len(names)
			if limit > MaxAncestorDepth {
				limit = MaxAncestorDepth
			}
			for _, raw := range names[:limit] {
				norm := normalizeProcessName(raw)
				if _, match := knownAgentProcessNames[norm]; match {
					return true, "process:" + norm
				}
			}
		}
	}
	return false, ""
}

func detectAgentEnv(environ []string) (string, bool) {
	present := make(map[string]struct{}, len(environ))
	orderedKeys := make([]string, 0, len(environ))
	for _, entry := range environ {
		k, v, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(v) == "" {
			continue
		}
		if _, seen := present[k]; !seen {
			present[k] = struct{}{}
			orderedKeys = append(orderedKeys, k)
		}
	}

	for _, name := range knownAgentEnvVars {
		if _, ok := present[name]; ok {
			return name, true
		}
	}
	for _, k := range orderedKeys {
		for _, prefix := range knownAgentEnvPrefixes {
			if strings.HasPrefix(k, prefix) {
				return k, true
			}
		}
	}
	return "", false
}

func normalizeProcessName(raw string) string {
	base := filepath.Base(strings.TrimSpace(raw))
	lower := strings.ToLower(base)
	return strings.TrimSuffix(lower, ".exe")
}

func listAncestorProcessNames(pid int32, maxDepth int) ([]string, error) {
	cur, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}
	visited := map[int32]struct{}{pid: {}}
	names := make([]string, 0, maxDepth)

	for depth := 0; depth < maxDepth; depth++ {
		ppid, err := cur.Ppid()
		if err != nil || ppid <= 0 {
			break
		}
		if _, seen := visited[ppid]; seen {
			break
		}
		visited[ppid] = struct{}{}

		parent, err := process.NewProcess(ppid)
		if err != nil {
			break
		}
		if name, err := parent.Name(); err == nil && name != "" {
			names = append(names, name)
		}
		if ppid == 1 {
			break
		}
		cur = parent
	}
	return names, nil
}
