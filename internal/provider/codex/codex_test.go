package codex_test

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/codex"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "testdata", "codex", name)
}

func parseFixture(t *testing.T, a *codex.Adapter, name string) []event.Event {
	t.Helper()
	f, err := os.Open(fixturePath(t, name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	defer f.Close()

	var out []event.Event
	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			evs, err := a.Parse(line)
			if err != nil {
				t.Fatalf("Parse(%s) returned unexpected error: %v", name, err)
			}
			out = append(out, evs...)
		}
		if readErr != nil {
			break
		}
	}
	return out
}

func TestCodex_HeadlessArgv(t *testing.T) {
	a := codex.New("")

	t.Run("edit with cwd and model", func(t *testing.T) {
		argv, err := a.HeadlessArgv(provider.RunRequest{
			Prompt: "fix tests",
			Model:  "o3",
			Cwd:    "/path/to/repo",
			Perm:   provider.PermEdit,
		})
		if err != nil {
			t.Fatalf("HeadlessArgv error: %v", err)
		}
		want := []string{
			"exec",
			"--json",
			"--sandbox", "workspace-write",
			"-m", "o3",
			"--", "fix tests",
		}
		if !reflect.DeepEqual(argv, want) {
			t.Errorf("argv = %v, want %v", argv, want)
		}
		if flag, bad := provider.ContainsDangerousFlag(argv); bad {
			t.Errorf("unexpected dangerous flag %q", flag)
		}
	})

	t.Run("default read-only sandbox", func(t *testing.T) {
		argv, err := a.HeadlessArgv(provider.RunRequest{
			Prompt: "review code",
			Perm:   provider.PermReadOnly,
		})
		if err != nil {
			t.Fatalf("HeadlessArgv error: %v", err)
		}
		want := []string{
			"exec",
			"--json",
			"--sandbox", "read-only",
			"--", "review code",
		}
		if !reflect.DeepEqual(argv, want) {
			t.Errorf("argv = %v, want %v", argv, want)
		}
	})

	t.Run("resume follows Phase 0 exec resume contract", func(t *testing.T) {
		argv, err := a.HeadlessArgv(provider.RunRequest{
			Prompt:            "continue please",
			Model:             "gpt-5-codex",
			Cwd:               "/ignored-on-argv-used-as-cmd-dir",
			Perm:              provider.PermReadOnly,
			ProviderSessionID: "01a20085-2878-74a3-896c-b98dc5f74bbf",
		})
		if err != nil {
			t.Fatalf("HeadlessArgv resume error: %v", err)
		}
		want := []string{
			"exec", "resume",
			"--json",
			"-c", `sandbox_mode="read-only"`,
			"-m", "gpt-5-codex",
			"--", "01a20085-2878-74a3-896c-b98dc5f74bbf",
			"continue please",
		}
		if !reflect.DeepEqual(argv, want) {
			t.Errorf("argv = %v, want %v", argv, want)
		}
	})

	t.Run("invalid perm rejected", func(t *testing.T) {
		if _, err := a.HeadlessArgv(provider.RunRequest{
			Prompt: "hi",
			Perm:   provider.Perm("danger-full-access"),
		}); err == nil {
			t.Fatalf("expected error for invalid --perm")
		}
	})
}

func TestCodex_ParseFixtures(t *testing.T) {
	a := codex.New("")

	t.Run("success fixture", func(t *testing.T) {
		events := parseFixture(t, a, "success.ndjson")
		var sessionID, text string
		var usage *event.UsageInfo
		var hasCompleted bool
		var unknownCount int

		for _, ev := range events {
			if ev.Provider != "codex" {
				t.Errorf("ev.Provider = %q, want codex", ev.Provider)
			}
			switch ev.Kind {
			case event.SessionStarted:
				sessionID = ev.SessionID
			case event.Text:
				text += ev.Text
			case event.Usage:
				usage = ev.Usage
			case event.Completed:
				hasCompleted = true
			case event.Unknown:
				unknownCount++
			}
		}

		if sessionID != "01a20085-2878-74a3-896c-b98dc5f74bbf" {
			t.Errorf("session_id = %q, want 01a20085-2878-74a3-896c-b98dc5f74bbf", sessionID)
		}
		if text != "hello" {
			t.Errorf("text = %q, want hello", text)
		}
		if usage == nil || usage.InputTokens != 32223 || usage.OutputTokens != 5 || usage.CostUSD != nil {
			t.Errorf("usage = %+v, want input=32223 output=5 cost=nil", usage)
		}
		if !hasCompleted {
			t.Errorf("expected Completed event")
		}
		if unknownCount == 0 {
			t.Errorf("expected turn.started to be preserved as Unknown")
		}
	})

	t.Run("tool_use fixture", func(t *testing.T) {
		events := parseFixture(t, a, "tool_use.ndjson")
		var toolName, toolInput, toolResult, text string
		var hasSession, hasUsage, hasCompleted bool

		for _, ev := range events {
			switch ev.Kind {
			case event.SessionStarted:
				hasSession = ev.SessionID == "01a20085-4084-7e01-85bc-dff63b395744"
			case event.ToolCall:
				toolName = ev.ToolName
				toolInput = string(ev.ToolInput)
			case event.ToolResult:
				toolResult = ev.Text
			case event.Text:
				text += ev.Text
			case event.Usage:
				hasUsage = ev.Usage != nil && ev.Usage.InputTokens == 64523 && ev.Usage.OutputTokens == 44
			case event.Completed:
				hasCompleted = true
			}
		}

		if !hasSession || toolName != "command_execution" || !strings.Contains(toolInput, "cat README.md") ||
			!strings.Contains(toolResult, "# subcli") || text != "# subcli" || !hasUsage || !hasCompleted {
			t.Errorf("tool_use parse mismatch: session=%v tool=%q input=%q result=%q text=%q usage=%v completed=%v",
				hasSession, toolName, toolInput, toolResult, text, hasUsage, hasCompleted)
		}
	})

	t.Run("error fixture", func(t *testing.T) {
		events := parseFixture(t, a, "error.ndjson")
		var errMsgs []string
		for _, ev := range events {
			if ev.Kind == event.Error {
				errMsgs = append(errMsgs, ev.Text)
			}
		}
		if len(errMsgs) < 2 {
			t.Fatalf("expected multiple Error events from error.ndjson, got %v", errMsgs)
		}
		joined := strings.Join(errMsgs, "\n")
		if !strings.Contains(joined, "invalid-model-xyz-000") {
			t.Errorf("error messages missing model name: %s", joined)
		}
	})

	t.Run("unknown event types are preserved", func(t *testing.T) {
		cases := []string{
			`{"type":"future.event","data":42}`,
			`{"type":"item.completed","item":{"id":"x","type":"future_item"}}`,
		}
		for _, rawLine := range cases {
			evs, err := a.Parse([]byte(rawLine))
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", rawLine, err)
			}
			if len(evs) != 1 || evs[0].Kind != event.Unknown {
				t.Fatalf("Parse(%q) = %+v, want 1 Unknown event", rawLine, evs)
			}
		}
	})
}

func TestCodex_OnlyCompletedAgentMessageProducesText(t *testing.T) {
	a := codex.New("")
	var text string
	for _, line := range []string{
		`{"type":"item.started","item":{"type":"agent_message","text":"a"}}`,
		`{"type":"item.updated","item":{"type":"agent_message","text":"ab"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"abc"}}`,
	} {
		events, err := a.Parse([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range events {
			if ev.Kind == event.Text {
				text += ev.Text
			}
		}
	}
	if text != "abc" {
		t.Errorf("combined text = %q, want abc", text)
	}
}
