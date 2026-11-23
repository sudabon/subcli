package render_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/render"
)

func TestRenderCompare_SideBySideLayoutOnWideTerminal(t *testing.T) {
	var buf bytes.Buffer
	cost := 0.015
	cols := []render.CompareColumn{
		{
			Index:    0,
			Provider: "claude",
			Auth:     provider.AuthSubscription,
			Text:     "Claude の回答です。\n2 行目。",
			Usage:    &event.UsageInfo{InputTokens: 10, OutputTokens: 20, CostUSD: &cost},
		},
		{
			Index:    1,
			Provider: "codex",
			Auth:     provider.AuthSubscription,
			Text:     "Codex response line 1.",
			Usage:    &event.UsageInfo{InputTokens: 50, OutputTokens: 15},
		},
	}

	err := render.RenderCompare(&buf, cols, render.CompareFormatOptions{
		Width: 120,
		Color: false,
	})
	if err != nil {
		t.Fatalf("RenderCompare error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected side-by-side table with header/separator/body, got:\n%s", out)
	}
	if !strings.Contains(lines[0], "[claude]") || !strings.Contains(lines[0], "[codex]") || !strings.Contains(lines[0], "│") {
		t.Errorf("header line should show both providers side-by-side: %q", lines[0])
	}
	if !strings.Contains(lines[1], "─┼─") {
		t.Errorf("separator line missing ─┼─: %q", lines[1])
	}
	if !strings.Contains(out, "Claude の回答です。") || !strings.Contains(out, "Codex response line 1.") {
		t.Errorf("output missing provider texts:\n%s", out)
	}
	if !strings.Contains(out, render.SubscriptionCostDisclaimer) {
		t.Errorf("output missing subscription cost disclaimer:\n%s", out)
	}
}

func TestRenderCompare_NarrowTerminalFallsBackToStackedLayout(t *testing.T) {
	var buf bytes.Buffer
	cols := []render.CompareColumn{
		{
			Index:    0,
			Provider: "claude",
			Auth:     provider.AuthSubscription,
			Text:     "claude answer",
		},
		{
			Index:    1,
			Provider: "codex",
			Auth:     provider.AuthSubscription,
			Text:     "codex answer",
			ExitCode: 2,
			ErrText:  "boom",
		},
	}

	err := render.RenderCompare(&buf, cols, render.CompareFormatOptions{
		Width: 80, // < DefaultSideBySideMinWidth (100)
		Color: false,
	})
	if err != nil {
		t.Fatalf("RenderCompare error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "─┼─") {
		t.Errorf("narrow terminal should fall back to vertical layout, not side-by-side:\n%s", out)
	}
	for _, want := range []string{
		"=== [claude] ===",
		"claude answer",
		"=== [codex] ===",
		"codex answer",
		"error: boom",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stacked output missing %q:\n%s", want, out)
		}
	}
}

func TestCompareJSONLRenderer_IncludesProviderAndIndex(t *testing.T) {
	var buf bytes.Buffer
	r := render.NewCompareJSONLRenderer(&buf)
	now := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)

	if err := r.Render(0, event.Event{
		Provider: "claude",
		Kind:     event.Text,
		Text:     "hi claude",
		Raw:      event.SafeRaw([]byte(`{"type":"assistant"}`)),
		At:       now,
	}); err != nil {
		t.Fatalf("Render 0 error: %v", err)
	}
	if err := r.Render(1, event.Event{
		Provider: "codex",
		Kind:     event.Text,
		Text:     "hi codex",
		Raw:      event.SafeRaw([]byte(`{"type":"item.completed"}`)),
		At:       now,
	}); err != nil {
		t.Fatalf("Render 1 error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}

	for i, wantProv := range []string{"claude", "codex"} {
		// Compatible with both CompareJSONLEvent and plain event.Event
		var ce render.CompareJSONLEvent
		if err := json.Unmarshal([]byte(lines[i]), &ce); err != nil {
			t.Fatalf("line %d invalid JSON: %v", i, err)
		}
		if ce.CompareIndex != i || ce.Provider != wantProv || ce.Kind != event.Text {
			t.Errorf("line %d = %+v, want index=%d provider=%q kind=text", i, ce, i, wantProv)
		}
		var plain event.Event
		if err := json.Unmarshal([]byte(lines[i]), &plain); err != nil {
			t.Fatalf("line %d failed to unmarshal as event.Event: %v", i, err)
		}
		if plain.Provider != wantProv {
			t.Errorf("plain.Provider = %q, want %q", plain.Provider, wantProv)
		}
	}
}

func TestChainJSONLRenderer_IncludesStepAndProvider(t *testing.T) {
	var buf bytes.Buffer
	r := render.NewChainJSONLRenderer(&buf)
	if err := r.Render(2, event.Event{
		Provider: "codex",
		Kind:     event.Text,
		Text:     "review done",
		Raw:      event.SafeRaw([]byte(`{"ok":true}`)),
	}); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	var ce render.ChainJSONLEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &ce); err != nil {
		t.Fatalf("unmarshal ChainJSONLEvent: %v", err)
	}
	if ce.Step != 2 || ce.Provider != "codex" || ce.Text != "review done" {
		t.Errorf("unexpected ChainJSONLEvent: %+v", ce)
	}
}
