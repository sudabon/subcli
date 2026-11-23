package event_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sudabon/subcli/internal/event"
)

func TestEvent_JSONRoundTrip(t *testing.T) {
	cost := 0.0651662
	ts := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		ev   event.Event
	}{
		{
			name: "session_started",
			ev: event.Event{
				Provider:  "claude",
				Kind:      event.SessionStarted,
				SessionID: "sess-123",
				Raw:       event.SafeRaw([]byte(`{"type":"system","subtype":"init","session_id":"sess-123"}`)),
				At:        ts,
			},
		},
		{
			name: "text",
			ev: event.Event{
				Provider: "codex",
				Kind:     event.Text,
				Text:     "hello world",
				Raw:      event.SafeRaw([]byte(`{"type":"item.completed","item":{"type":"agent_message","text":"hello world"}}`)),
				At:       ts,
			},
		},
		{
			name: "tool_call",
			ev: event.Event{
				Provider:  "claude",
				Kind:      event.ToolCall,
				ToolName:  "Read",
				ToolInput: json.RawMessage(`{"file_path":"README.md"}`),
				Raw:       event.SafeRaw([]byte(`{"type":"assistant"}`)),
				At:        ts,
			},
		},
		{
			name: "usage with cost",
			ev: event.Event{
				Provider: "claude",
				Kind:     event.Usage,
				Usage: &event.UsageInfo{
					InputTokens:  100,
					OutputTokens: 42,
					CostUSD:      &cost,
				},
				Raw: event.SafeRaw([]byte(`{"type":"result"}`)),
				At:  ts,
			},
		},
		{
			name: "usage without cost",
			ev: event.Event{
				Provider: "codex",
				Kind:     event.Usage,
				Usage: &event.UsageInfo{
					InputTokens:  32223,
					OutputTokens: 5,
				},
				Raw: event.SafeRaw([]byte(`{"type":"turn.completed"}`)),
				At:  ts,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.ev)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			var got event.Event
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if got.Provider != tc.ev.Provider || got.Kind != tc.ev.Kind {
				t.Errorf("got (%q, %q), want (%q, %q)", got.Provider, got.Kind, tc.ev.Provider, tc.ev.Kind)
			}
			if got.SessionID != tc.ev.SessionID || got.Text != tc.ev.Text || got.ToolName != tc.ev.ToolName {
				t.Errorf("field mismatch: got %+v, want %+v", got, tc.ev)
			}
			if string(got.Raw) != string(tc.ev.Raw) {
				t.Errorf("Raw = %s, want %s", string(got.Raw), string(tc.ev.Raw))
			}
			if tc.ev.Usage != nil {
				if got.Usage == nil {
					t.Fatalf("Usage is nil, want %+v", tc.ev.Usage)
				}
				if got.Usage.InputTokens != tc.ev.Usage.InputTokens || got.Usage.OutputTokens != tc.ev.Usage.OutputTokens {
					t.Errorf("Usage tokens = (%d, %d), want (%d, %d)", got.Usage.InputTokens, got.Usage.OutputTokens, tc.ev.Usage.InputTokens, tc.ev.Usage.OutputTokens)
				}
				if (tc.ev.Usage.CostUSD == nil) != (got.Usage.CostUSD == nil) {
					t.Errorf("CostUSD presence mismatch: got %v, want %v", got.Usage.CostUSD, tc.ev.Usage.CostUSD)
				}
			}
		})
	}
}

func TestEvent_UnknownPreservesRaw(t *testing.T) {
	ts := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)

	t.Run("unknown JSON event preserves raw payload verbatim", func(t *testing.T) {
		rawLine := `{"type":"future_event_v9","nested":{"flag":true},"extra_field":123}`
		ev := event.NewUnknown("claude", []byte(rawLine), ts)
		if ev.Kind != event.Unknown {
			t.Fatalf("Kind = %q, want %q", ev.Kind, event.Unknown)
		}
		if string(ev.Raw) != rawLine {
			t.Fatalf("Raw = %s, want %s", string(ev.Raw), rawLine)
		}

		encoded, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal unknown event: %v", err)
		}
		var decoded event.Event
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshal unknown event: %v", err)
		}
		if decoded.Kind != event.Unknown {
			t.Errorf("decoded.Kind = %q, want %q", decoded.Kind, event.Unknown)
		}
		if string(decoded.Raw) != rawLine {
			t.Errorf("decoded.Raw = %s, want %s", string(decoded.Raw), rawLine)
		}
	})

	t.Run("non-JSON noise line is safely wrapped as valid JSON raw", func(t *testing.T) {
		noise := []byte("plain warning line from cli")
		ev := event.NewUnknown("codex", noise, ts)
		encoded, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal non-JSON unknown event: %v", err)
		}
		var decoded event.Event
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("unmarshal non-JSON unknown event: %v", err)
		}
		var rawStr string
		if err := json.Unmarshal(decoded.Raw, &rawStr); err != nil {
			t.Fatalf("Raw is not a valid JSON string: %v (%s)", err, string(decoded.Raw))
		}
		if rawStr != string(noise) {
			t.Errorf("rawStr = %q, want %q", rawStr, string(noise))
		}
	})
}
