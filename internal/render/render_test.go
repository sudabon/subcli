package render_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/render"
)

func TestTTYRenderer_SubscriptionDisclaimerAndFormatting(t *testing.T) {
	var buf bytes.Buffer
	r := render.NewTTYRenderer(&buf, provider.AuthSubscription, true)
	cost := 0.0652
	now := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)

	events := []event.Event{
		{Provider: "claude", Kind: event.SessionStarted, SessionID: "s1", At: now},
		{Provider: "claude", Kind: event.ToolCall, ToolName: "Read", ToolInput: json.RawMessage(`{"file_path":"/workspace/README.md"}`), At: now},
		{Provider: "claude", Kind: event.ToolResult, Text: "ignored verbose result", At: now},
		{Provider: "claude", Kind: event.Text, Text: "Hello ", At: now},
		{Provider: "claude", Kind: event.Text, Text: "world", At: now},
		{Provider: "claude", Kind: event.Usage, Usage: &event.UsageInfo{InputTokens: 10, OutputTokens: 25, CostUSD: &cost}, At: now},
		{Provider: "claude", Kind: event.Completed, Text: "Hello world", At: now},
	}
	for _, ev := range events {
		if err := r.Render(ev); err != nil {
			t.Fatalf("Render error: %v", err)
		}
	}
	if err := r.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"\x1b[36m[claude]\x1b[0m",
		"tool: Read (/workspace/README.md)",
		"Hello world",
		"in=10 out=25 cost=$0.0652",
		render.SubscriptionCostDisclaimer,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("TTY output missing %q:\n%s", want, out)
		}
	}
}

func TestTTYRenderer_APIKeyNoDisclaimerAndNoColor(t *testing.T) {
	var buf bytes.Buffer
	r := render.NewTTYRenderer(&buf, provider.AuthAPIKey, false)
	cost := 0.0123

	if err := r.Render(event.Event{
		Provider: "codex",
		Kind:     event.Usage,
		Usage:    &event.UsageInfo{InputTokens: 100, OutputTokens: 50, CostUSD: &cost},
	}); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI codes when color=false: %q", out)
	}
	if !strings.Contains(out, "[codex] usage: in=100 out=50 cost=$0.0123") {
		t.Errorf("unexpected output: %q", out)
	}
	if strings.Contains(out, render.SubscriptionCostDisclaimer) {
		t.Errorf("API key mode should not print subscription disclaimer: %q", out)
	}
}

func TestJSONLRenderer(t *testing.T) {
	var buf bytes.Buffer
	r := render.NewJSONLRenderer(&buf)
	now := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)

	input := []event.Event{
		{Provider: "codex", Kind: event.SessionStarted, SessionID: "th-1", Raw: event.SafeRaw([]byte(`{"type":"thread.started"}`)), At: now},
		{Provider: "codex", Kind: event.Text, Text: "ok", Raw: event.SafeRaw([]byte(`{"type":"item.completed"}`)), At: now},
	}
	for _, ev := range input {
		if err := r.Render(ev); err != nil {
			t.Fatalf("Render error: %v", err)
		}
	}
	if err := r.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2:\n%s", len(lines), buf.String())
	}
	for i, line := range lines {
		var got event.Event
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d invalid JSON: %v (%s)", i, err, line)
		}
		if got.Kind != input[i].Kind || got.Provider != input[i].Provider {
			t.Errorf("line %d = (%q,%q), want (%q,%q)", i, got.Provider, got.Kind, input[i].Provider, input[i].Kind)
		}
	}
}

func TestNullOutputIsNotTerminal(t *testing.T) {
	out, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if render.IsTerminal(out) {
		t.Fatalf("%s output detected as a terminal", os.DevNull)
	}
}
