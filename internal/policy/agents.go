package policy

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

// MaxAncestorDepth limits parent process chain traversal to 10 hops.
const MaxAncestorDepth = 10

var errAncestorDepth = errors.New("ancestor depth limit reached before the process-tree root")

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

func detectUnderAgent(environ []string, pid int32, ancestorsFn func(pid int32, maxDepth int) ([]string, error)) (bool, string, error) {
	if envName, ok := detectAgentEnv(environ); ok {
		return true, "env:" + envName, nil
	}
	names, err := ancestorsFn(pid, MaxAncestorDepth)
	if len(names) > MaxAncestorDepth {
		names = names[:MaxAncestorDepth]
		err = errAncestorDepth
	}
	for _, raw := range names {
		norm := normalizeProcessName(raw)
		if _, match := knownAgentProcessNames[norm]; match {
			return true, "process:" + norm, nil
		}
	}
	return false, "", err
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

	if runtime.GOOS != "windows" && pid == 1 {
		return names, nil
	}
	for depth := 0; depth < maxDepth; depth++ {
		ppid, err := cur.Ppid()
		if err != nil {
			return names, err
		}
		// PID 4 is the Windows System process and may be inaccessible.
		if runtime.GOOS == "windows" && ppid == 4 {
			return names, nil
		}
		if ppid <= 0 {
			return names, fmt.Errorf("parent PID unavailable for %d", cur.Pid)
		}
		if _, seen := visited[ppid]; seen {
			return names, fmt.Errorf("cyclic process ancestry at PID %d", ppid)
		}
		visited[ppid] = struct{}{}

		parent, err := process.NewProcess(ppid)
		if runtime.GOOS == "windows" && errors.Is(err, process.ErrorProcessNotRunning) {
			// Windows never re-parents orphans, so an exited parent (for example
			// the userinit.exe that started explorer.exe) is the tree root.
			return names, nil
		}
		if err != nil {
			return names, err
		}
		if runtime.GOOS == "windows" && startedAfter(parent, cur) {
			// The parent PID was reused by a newer, unrelated process.
			return names, nil
		}
		name, err := parent.Name()
		if err != nil {
			return names, err
		}
		if name == "" {
			return names, errors.New("ancestor process name unavailable")
		}
		names = append(names, name)
		if runtime.GOOS != "windows" && ppid == 1 {
			return names, nil
		}
		cur = parent
	}
	return names, errAncestorDepth
}

// startedAfter reports whether parent provably started after child. Unknown
// creation times return false so the walk continues instead of stopping early.
func startedAfter(parent, child *process.Process) bool {
	parentStart, err := parent.CreateTime()
	if err != nil {
		return false
	}
	childStart, err := child.CreateTime()
	if err != nil {
		return false
	}
	return parentStart > childStart
}
