package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
)

const defaultAPIKeyEnv = "ANTHROPIC_API_KEY"

// Adapter implements provider.Provider for the official Claude CLI.
type Adapter struct {
	apiKeyEnv string
	now       func() time.Time
}

// New creates a Claude provider adapter.
func New(apiKeyEnv string) *Adapter {
	if strings.TrimSpace(apiKeyEnv) == "" {
		apiKeyEnv = defaultAPIKeyEnv
	}
	return &Adapter{
		apiKeyEnv: apiKeyEnv,
		now:       time.Now,
	}
}

func (a *Adapter) ID() string     { return "claude" }
func (a *Adapter) Binary() string { return "claude" }

func (a *Adapter) Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, a.Binary(), "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// LoginArgv returns the Phase 0 verified official login subcommand.
func (a *Adapter) LoginArgv() []string {
	return []string{"auth", "login"}
}

// StatusArgv returns the Phase 0 verified official authentication status subcommand.
func (a *Adapter) StatusArgv() []string {
	return []string{"auth", "status"}
}

func (a *Adapter) InteractiveArgv(extra []string) []string {
	if len(extra) == 0 {
		return nil
	}
	out := make([]string, len(extra))
	copy(out, extra)
	return out
}

// InteractiveMemoryArgs attaches the repository-local shared memory directory
// and projection directory for interactive Claude Code sessions.
func (a *Adapter) InteractiveMemoryArgs(mem provider.MemoryContext, extra []string) []string {
	var args []string
	if dir := strings.TrimSpace(mem.MemoryDir); dir != "" {
		args = append(args, "--add-dir", dir)
	}
	if projDir := strings.TrimSpace(mem.ProjectionDir); projDir != "" {
		args = append(args, "--add-dir", projDir)
	}
	if dir := strings.TrimSpace(mem.MemoryDir); dir != "" && !hasFlag(extra, "--settings") {
		payload, err := json.Marshal(map[string]string{
			"autoMemoryDirectory": dir,
		})
		if err == nil {
			args = append(args, "--settings", string(payload))
		}
	}
	return args
}

// InteractiveMemoryEnv enables CLAUDE.md discovery in --add-dir directories so
// the generated shared memory projection is loaded automatically at startup.
func (a *Adapter) InteractiveMemoryEnv(env []string, mem provider.MemoryContext) []string {
	if strings.TrimSpace(mem.ProjectionDir) == "" {
		return env
	}
	return provider.SetEnv(env, "CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD", "1")
}

func hasFlag(args []string, name string) bool {
	prefix := name + "="
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

// HeadlessArgv builds Phase 0 contract-compliant arguments for headless and resume runs.
func (a *Adapter) HeadlessArgv(req provider.RunRequest) ([]string, error) {
	permMode, err := mapPermissionMode(req.Perm)
	if err != nil {
		return nil, err
	}

	args := []string{
		"-p", req.Prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", permMode,
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "--model", model)
	}
	if sessID := strings.TrimSpace(req.ProviderSessionID); sessID != "" {
		args = append(args, "--resume", sessID)
	}
	return args, nil
}

func mapPermissionMode(perm provider.Perm) (string, error) {
	switch perm {
	case "", provider.PermReadOnly:
		return "plan", nil
	case provider.PermEdit:
		return "acceptEdits", nil
	default:
		return "", fmt.Errorf("不正な権限モード --perm=%q です（%q または %q を指定してください）", perm, provider.PermReadOnly, provider.PermEdit)
	}
}

func (a *Adapter) ChildEnv(base []string, auth provider.AuthMode) []string {
	switch auth {
	case provider.AuthSubscription:
		// Strip API key env vars so subscription mode never bills API usage unexpectedly.
		return provider.FilterEnv(base, "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN")
	case provider.AuthAPIKey:
		cleaned := provider.FilterEnv(base, "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN")
		if val, ok := provider.LookupEnvSlice(base, a.apiKeyEnv); ok && val != "" {
			return provider.SetEnv(cleaned, "ANTHROPIC_API_KEY", val)
		}
		return cleaned
	default:
		return provider.FilterEnv(base, "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN")
	}
}

func (a *Adapter) NeedsPTY() bool { return false }

type streamLine struct {
	Type           string          `json:"type"`
	Subtype        string          `json:"subtype"`
	SessionID      string          `json:"session_id"`
	IsError        bool            `json:"is_error"`
	Result         string          `json:"result"`
	TerminalReason string          `json:"terminal_reason"`
	Errors         []string        `json:"errors"`
	TotalCostUSD   *float64        `json:"total_cost_usd"`
	Usage          *claudeUsage    `json:"usage"`
	Message        *claudeMessage  `json:"message"`
}

type claudeUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type claudeMessage struct {
	Content []claudeContentBlock `json:"content"`
}

type claudeContentBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
}

// Parse normalizes a single stream-json NDJSON line from the claude CLI into
// zero or more normalized events. Unknown or unparseable lines are preserved as
// event.Unknown without returning an error.
func (a *Adapter) Parse(line []byte) ([]event.Event, error) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return nil, nil
	}

	at := a.now()
	raw := event.SafeRaw(trimmed)

	var msg streamLine
	if err := json.Unmarshal(trimmed, &msg); err != nil {
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}, nil
	}

	switch msg.Type {
	case "system":
		if msg.Subtype == "init" {
			return []event.Event{{
				Provider:  a.ID(),
				Kind:      event.SessionStarted,
				SessionID: msg.SessionID,
				Raw:       raw,
				At:        at,
			}}, nil
		}
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}, nil

	case "assistant":
		if msg.Message == nil || len(msg.Message.Content) == 0 {
			return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}, nil
		}
		events := make([]event.Event, 0, len(msg.Message.Content))
		for _, block := range msg.Message.Content {
			switch block.Type {
			case "text":
				events = append(events, event.Event{
					Provider:  a.ID(),
					Kind:      event.Text,
					SessionID: msg.SessionID,
					Text:      block.Text,
					Raw:       raw,
					At:        at,
				})
			case "tool_use":
				events = append(events, event.Event{
					Provider:  a.ID(),
					Kind:      event.ToolCall,
					SessionID: msg.SessionID,
					ToolName:  block.Name,
					ToolInput: copyRaw(block.Input),
					Raw:       raw,
					At:        at,
				})
			default:
				events = append(events, event.NewUnknown(a.ID(), trimmed, at))
			}
		}
		return events, nil

	case "user":
		if msg.Message == nil || len(msg.Message.Content) == 0 {
			return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}, nil
		}
		var events []event.Event
		for _, block := range msg.Message.Content {
			if block.Type == "tool_result" {
				events = append(events, event.Event{
					Provider:  a.ID(),
					Kind:      event.ToolResult,
					SessionID: msg.SessionID,
					Text:      extractToolResultText(block.Content),
					Raw:       raw,
					At:        at,
				})
			}
		}
		if len(events) == 0 {
			return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}, nil
		}
		return events, nil

	case "result":
		var events []event.Event
		if msg.Usage != nil || msg.TotalCostUSD != nil {
			usage := &event.UsageInfo{
				CostUSD: msg.TotalCostUSD,
			}
			if msg.Usage != nil {
				usage.InputTokens = msg.Usage.InputTokens
				usage.OutputTokens = msg.Usage.OutputTokens
			}
			events = append(events, event.Event{
				Provider:  a.ID(),
				Kind:      event.Usage,
				SessionID: msg.SessionID,
				Usage:     usage,
				Raw:       raw,
				At:        at,
			})
		}

		// Per Phase 0 findings (research-notes.md §3.3), error results can carry
		// subtype="success" with is_error=true, so both must be inspected.
		isErr := msg.IsError || (msg.Subtype != "" && msg.Subtype != "success")
		if isErr {
			errText := strings.TrimSpace(msg.Result)
			if errText == "" && len(msg.Errors) > 0 {
				errText = strings.Join(msg.Errors, "; ")
			}
			if errText == "" {
				errText = msg.TerminalReason
			}
			if errText == "" {
				errText = msg.Subtype
			}
			events = append(events, event.Event{
				Provider:  a.ID(),
				Kind:      event.Error,
				SessionID: msg.SessionID,
				Text:      errText,
				Raw:       raw,
				At:        at,
			})
		} else {
			events = append(events, event.Event{
				Provider:  a.ID(),
				Kind:      event.Completed,
				SessionID: msg.SessionID,
				Text:      msg.Result,
				Raw:       raw,
				At:        at,
			})
		}
		return events, nil

	default:
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}, nil
	}
}

func extractToolResultText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(trimmed, &blocks); err == nil && len(blocks) > 0 {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return string(trimmed)
}

func copyRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return json.RawMessage(cp)
}
