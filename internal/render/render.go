package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
)

// SubscriptionCostDisclaimer is required by spec when rendering reference cost in subscription mode.
const SubscriptionCostDisclaimer = "参考値（実際の請求ではありません）"

// Renderer renders normalized events to an output stream.
type Renderer interface {
	Render(ev event.Event) error
	Flush() error
}

// IsTerminal reports whether w is backed by an interactive terminal character device.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// JSONLRenderer writes each normalized Event as a single JSON line.
type JSONLRenderer struct {
	w io.Writer
}

// NewJSONLRenderer creates a JSONLRenderer writing to w.
func NewJSONLRenderer(w io.Writer) *JSONLRenderer {
	return &JSONLRenderer{w: w}
}

// Render encodes ev as one compact JSON object followed by a newline.
func (r *JSONLRenderer) Render(ev event.Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = r.w.Write(data)
	return err
}

// Flush is a no-op for JSONLRenderer because every Render writes a complete line.
func (r *JSONLRenderer) Flush() error { return nil }

// TTYRenderer formats normalized events for human terminal viewing with
// provider-colored prefixes, streamed text, 1-line tool call summaries, and usage.
type TTYRenderer struct {
	w               io.Writer
	auth            provider.AuthMode
	color           bool
	inText          bool
	lastTextEndedNL bool
}

// NewTTYRenderer creates a TTYRenderer.
func NewTTYRenderer(w io.Writer, auth provider.AuthMode, color bool) *TTYRenderer {
	return &TTYRenderer{
		w:     w,
		auth:  auth,
		color: color,
	}
}

// Render formats and writes a single normalized event.
func (r *TTYRenderer) Render(ev event.Event) error {
	switch ev.Kind {
	case event.Text:
		if ev.Text == "" {
			return nil
		}
		if !r.inText {
			if _, err := fmt.Fprintf(r.w, "%s ", r.prefix(ev.Provider)); err != nil {
				return err
			}
			r.inText = true
		}
		if _, err := io.WriteString(r.w, ev.Text); err != nil {
			return err
		}
		r.lastTextEndedNL = strings.HasSuffix(ev.Text, "\n")
		return nil

	case event.ToolCall:
		if err := r.endTextLine(); err != nil {
			return err
		}
		summary := summarizeToolInput(ev.ToolInput)
		if summary != "" {
			_, err := fmt.Fprintf(r.w, "%s tool: %s (%s)\n", r.prefix(ev.Provider), ev.ToolName, summary)
			return err
		}
		_, err := fmt.Fprintf(r.w, "%s tool: %s\n", r.prefix(ev.Provider), ev.ToolName)
		return err

	case event.Usage:
		if ev.Usage == nil {
			return nil
		}
		if err := r.endTextLine(); err != nil {
			return err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "in=%d out=%d", ev.Usage.InputTokens, ev.Usage.OutputTokens)
		if ev.Usage.CostUSD != nil {
			fmt.Fprintf(&b, " cost=$%.4f", *ev.Usage.CostUSD)
			if r.auth == provider.AuthSubscription {
				fmt.Fprintf(&b, " [%s]", SubscriptionCostDisclaimer)
			}
		}
		_, err := fmt.Fprintf(r.w, "%s usage: %s\n", r.prefix(ev.Provider), b.String())
		return err

	case event.Error:
		if err := r.endTextLine(); err != nil {
			return err
		}
		_, err := fmt.Fprintf(r.w, "%s error: %s\n", r.prefix(ev.Provider), ev.Text)
		return err

	default:
		return nil
	}
}

// Flush ensures streamed text terminates with a newline.
func (r *TTYRenderer) Flush() error {
	return r.endTextLine()
}

func (r *TTYRenderer) endTextLine() error {
	if !r.inText {
		return nil
	}
	r.inText = false
	if r.lastTextEndedNL {
		r.lastTextEndedNL = false
		return nil
	}
	_, err := io.WriteString(r.w, "\n")
	return err
}

func (r *TTYRenderer) prefix(prov string) string {
	label := "[" + prov + "]"
	if !r.color {
		return label
	}
	var code string
	switch prov {
	case "claude":
		code = "\x1b[36m" // cyan
	case "codex":
		code = "\x1b[32m" // green
	case "agy":
		code = "\x1b[33m" // yellow
	default:
		code = "\x1b[37m" // white
	}
	return code + label + "\x1b[0m"
}

func summarizeToolInput(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(trimmed, &m); err == nil {
		for _, key := range []string{"command", "file_path", "path", "pattern", "query", "url"} {
			if val, ok := m[key]; ok {
				if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
					return singleLineTruncate(s, 80)
				}
			}
		}
	}
	return singleLineTruncate(string(trimmed), 80)
}

func singleLineTruncate(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return s
}
