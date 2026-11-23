package claude_test

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/claude"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "testdata", "claude", name)
}

func parseFixture(t *testing.T, a *claude.Adapter, name string) []event.Event {
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

func TestClaude_HeadlessArgv(t *testing.T) {
	a := claude.New("")

	t.Run("read-only with model", func(t *testing.T) {
		argv, err := a.HeadlessArgv(provider.RunRequest{
			Prompt: "summarize README",
			Model:  "claude-sonnet-4",
			Perm:   provider.PermReadOnly,
		})
		if err != nil {
			t.Fatalf("HeadlessArgv error: %v", err)
		}
		want := []string{
			"-p",
			"--output-format", "stream-json",
			"--verbose",
			"--permission-mode", "plan",
			"--model", "claude-sonnet-4",
			"--", "summarize README",
		}
		if !reflect.DeepEqual(argv, want) {
			t.Errorf("argv = %v, want %v", argv, want)
		}
		if flag, bad := provider.ContainsDangerousFlag(argv); bad {
			t.Errorf("unexpected dangerous flag %q", flag)
		}
	})

	t.Run("edit with resume", func(t *testing.T) {
		argv, err := a.HeadlessArgv(provider.RunRequest{
			Prompt:            "continue",
			Perm:              provider.PermEdit,
			ProviderSessionID: "sess-abc-123",
		})
		if err != nil {
			t.Fatalf("HeadlessArgv error: %v", err)
		}
		want := []string{
			"-p",
			"--output-format", "stream-json",
			"--verbose",
			"--permission-mode", "acceptEdits",
			"--resume", "sess-abc-123",
			"--", "continue",
		}
		if !reflect.DeepEqual(argv, want) {
			t.Errorf("argv = %v, want %v", argv, want)
		}
	})

	t.Run("invalid perm rejected", func(t *testing.T) {
		if _, err := a.HeadlessArgv(provider.RunRequest{
			Prompt: "hi",
			Perm:   provider.Perm("bypass"),
		}); err == nil {
			t.Fatalf("expected error for invalid --perm")
		}
	})
}

func TestClaude_ParseFixtures(t *testing.T) {
	a := claude.New("")

	t.Run("success fixture", func(t *testing.T) {
		events := parseFixture(t, a, "success.ndjson")
		var gotSession, gotText, gotCompleted string
		var gotUsage *event.UsageInfo
		var unknownCount int

		for _, ev := range events {
			if ev.Provider != "claude" {
				t.Errorf("ev.Provider = %q, want claude", ev.Provider)
			}
			switch ev.Kind {
			case event.SessionStarted:
				gotSession = ev.SessionID
			case event.Text:
				gotText += ev.Text
			case event.Usage:
				gotUsage = ev.Usage
			case event.Completed:
				gotCompleted = ev.Text
			case event.Unknown:
				unknownCount++
				if len(ev.Raw) == 0 {
					t.Errorf("unknown event has empty Raw")
				}
			}
		}

		if gotSession != "21c5723c-9da7-4d39-aeff-13fc2ecb542e" {
			t.Errorf("session_id = %q, want 21c5723c-9da7-4d39-aeff-13fc2ecb542e", gotSession)
		}
		if gotText != "hello" {
			t.Errorf("text = %q, want hello", gotText)
		}
		if gotCompleted != "hello" {
			t.Errorf("completed = %q, want hello", gotCompleted)
		}
		if gotUsage == nil || gotUsage.InputTokens != 2 || gotUsage.OutputTokens != 167 || gotUsage.CostUSD == nil || *gotUsage.CostUSD <= 0 {
			t.Errorf("usage = %+v, want input=2 output=167 cost>0", gotUsage)
		}
		if unknownCount == 0 {
			t.Errorf("expected unknown events for thinking_tokens / thinking / rate_limit_event")
		}
	})

	t.Run("tool_use fixture", func(t *testing.T) {
		events := parseFixture(t, a, "tool_use.ndjson")
		var toolName, toolResult, text string
		var hasSession, hasUsage, hasCompleted bool

		for _, ev := range events {
			switch ev.Kind {
			case event.SessionStarted:
				hasSession = ev.SessionID == "f8ec8c04-e34d-4500-a1f6-cbba6205e0c3"
			case event.ToolCall:
				toolName = ev.ToolName
				if !strings.Contains(string(ev.ToolInput), "README.md") {
					t.Errorf("ToolInput = %s, want README.md", string(ev.ToolInput))
				}
			case event.ToolResult:
				toolResult = ev.Text
			case event.Text:
				text += ev.Text
			case event.Usage:
				hasUsage = ev.Usage != nil && ev.Usage.OutputTokens == 273
			case event.Completed:
				hasCompleted = true
			}
		}

		if !hasSession || toolName != "Read" || !strings.Contains(toolResult, "# subcli") || text != "# subcli" || !hasUsage || !hasCompleted {
			t.Errorf("tool_use parse mismatch: session=%v tool=%q result=%q text=%q usage=%v completed=%v",
				hasSession, toolName, toolResult, text, hasUsage, hasCompleted)
		}
	})

	t.Run("error fixture detects is_error even when subtype is success", func(t *testing.T) {
		events := parseFixture(t, a, "error.ndjson")
		var errEv *event.Event
		var hasCompleted bool
		for i := range events {
			switch events[i].Kind {
			case event.Error:
				errEv = &events[i]
			case event.Completed:
				hasCompleted = true
			}
		}
		if errEv == nil {
			t.Fatalf("expected Error event from error.ndjson")
		}
		if hasCompleted {
			t.Errorf("error.ndjson should emit Error, not Completed")
		}
		if !strings.Contains(errEv.Text, "invalid-model-xyz-000") {
			t.Errorf("error text = %q, want mention of invalid-model-xyz-000", errEv.Text)
		}
	})

	t.Run("string message content is accepted", func(t *testing.T) {
		// The Messages API allows content to be a plain string instead of blocks.
		evs, err := a.Parse([]byte(`{"type":"user","message":{"role":"user","content":"hi"}}`))
		if err != nil || len(evs) != 1 || evs[0].Kind != event.Unknown {
			t.Fatalf("user string content = %+v, %v; want 1 Unknown event and no error", evs, err)
		}
		evs, err = a.Parse([]byte(`{"type":"assistant","message":{"content":"hello"}}`))
		if err != nil || len(evs) != 1 || evs[0].Kind != event.Text || evs[0].Text != "hello" {
			t.Fatalf("assistant string content = %+v, %v; want 1 Text event", evs, err)
		}
		evs, err = a.Parse([]byte(`{"type":"assistant","message":{"content":null}}`))
		if err != nil || len(evs) != 1 || evs[0].Kind != event.Unknown {
			t.Fatalf("null content = %+v, %v; want 1 Unknown event", evs, err)
		}
	})

	t.Run("unknown event types are preserved", func(t *testing.T) {
		cases := []string{
			`{"type":"brand_new_event","payload":{"k":"v"}}`,
		}
		for _, rawLine := range cases {
			evs, err := a.Parse([]byte(rawLine))
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", rawLine, err)
			}
			if len(evs) != 1 || evs[0].Kind != event.Unknown {
				t.Fatalf("Parse(%q) = %+v, want 1 Unknown event", rawLine, evs)
			}
			if len(evs[0].Raw) == 0 {
				t.Errorf("Parse(%q) Raw is empty", rawLine)
			}
		}
	})
}
