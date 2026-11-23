package event

import (
	"bytes"
	"encoding/json"
	"time"
)

// Kind identifies the normalized event type emitted by provider parsers.
type Kind string

const (
	SessionStarted Kind = "session_started"
	Text           Kind = "text"
	ToolCall       Kind = "tool_call"
	ToolResult     Kind = "tool_result"
	Usage          Kind = "usage"
	Completed      Kind = "completed"
	Error          Kind = "error"
	Unknown        Kind = "unknown"
)

// Event represents a provider-agnostic normalized event.
type Event struct {
	Provider      string          `json:"provider"`
	Kind          Kind            `json:"kind"`
	TerminalError bool            `json:"-"` // The provider reports that the turn failed.
	SessionID     string          `json:"session_id,omitempty"`
	Text          string          `json:"text,omitempty"`
	ToolName      string          `json:"tool_name,omitempty"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
	Usage         *UsageInfo      `json:"usage,omitempty"`
	Raw           json.RawMessage `json:"raw"`
	At            time.Time       `json:"at"`
}

// UsageInfo records token usage and optional reference cost.
type UsageInfo struct {
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
}

// SafeRaw converts an arbitrary raw output line into valid json.RawMessage so
// Event always marshals cleanly to JSON even when a CLI emits non-JSON noise.
func SafeRaw(line []byte) json.RawMessage {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return json.RawMessage("null")
	}
	if json.Valid(trimmed) {
		cp := make([]byte, len(trimmed))
		copy(cp, trimmed)
		return json.RawMessage(cp)
	}
	encoded, err := json.Marshal(string(trimmed))
	if err != nil {
		return json.RawMessage("null")
	}
	return json.RawMessage(encoded)
}

// NewUnknown builds an Unknown event preserving the original line in Raw.
func NewUnknown(provider string, line []byte, at time.Time) Event {
	return Event{
		Provider: provider,
		Kind:     Unknown,
		Raw:      SafeRaw(line),
		At:       at,
	}
}
