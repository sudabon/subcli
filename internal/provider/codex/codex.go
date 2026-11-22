package codex

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

const defaultAPIKeyEnv = "CODEX_API_KEY"

// Adapter implements provider.Provider for the official Codex CLI.
type Adapter struct {
	apiKeyEnv string
	now       func() time.Time
}

// New creates a Codex CLI provider adapter.
func New(apiKeyEnv string) *Adapter {
	if strings.TrimSpace(apiKeyEnv) == "" {
		apiKeyEnv = defaultAPIKeyEnv
	}
	return &Adapter{
		apiKeyEnv: apiKeyEnv,
		now:       time.Now,
	}
}

func (a *Adapter) ID() string     { return "codex" }
func (a *Adapter) Binary() string { return "codex" }

func (a *Adapter) Version(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, a.Binary(), "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// LoginArgv returns the Phase 0 verified official login subcommand.
func (a *Adapter) LoginArgv() []string {
	return []string{"login"}
}

// StatusArgv returns the Phase 0 verified official login status subcommand.
func (a *Adapter) StatusArgv() []string {
	return []string{"login", "status"}
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
// and injects bounded shared memory instructions via `-c developer_instructions=...`.
func (a *Adapter) InteractiveMemoryArgs(mem provider.MemoryContext, extra []string) []string {
	var args []string
	if dir := strings.TrimSpace(mem.MemoryDir); dir != "" {
		args = append(args, "--add-dir", dir)
	}
	if prompt := strings.TrimSpace(mem.InlinePrompt); prompt != "" && !hasCodexConfigKey(extra, "developer_instructions") {
		// JSON string encoding is valid TOML basic string syntax and escapes all control chars safely.
		quoted, err := json.Marshal(prompt)
		if err == nil {
			args = append(args, "-c", "developer_instructions="+string(quoted))
		}
	}
	return args
}

// InteractiveMemoryEnv returns env unchanged for Codex CLI.
func (a *Adapter) InteractiveMemoryEnv(env []string, _ provider.MemoryContext) []string {
	return env
}

func hasCodexConfigKey(args []string, key string) bool {
	prefix := key + "="
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if (arg == "-c" || arg == "--config") && i+1 < len(args) {
			if strings.HasPrefix(strings.TrimSpace(args[i+1]), prefix) {
				return true
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			val := strings.TrimPrefix(arg, "--config=")
			if strings.HasPrefix(strings.TrimSpace(val), prefix) {
				return true
			}
		}
		if strings.HasPrefix(arg, "-c=") {
			val := strings.TrimPrefix(arg, "-c=")
			if strings.HasPrefix(strings.TrimSpace(val), prefix) {
				return true
			}
		}
	}
	return false
}

// HeadlessArgv builds Phase 0 contract-compliant arguments for `codex exec` and
// `codex exec resume`.
func (a *Adapter) HeadlessArgv(req provider.RunRequest) ([]string, error) {
	sandboxMode, err := mapSandboxMode(req.Perm)
	if err != nil {
		return nil, err
	}

	if sessID := strings.TrimSpace(req.ProviderSessionID); sessID != "" {
		// Per Phase 0 contract (`codex exec resume --help`):
		// Usage: codex exec resume [OPTIONS] [SESSION_ID] [PROMPT]
		// `-s/--sandbox` and `-C/--cd` are not accepted by `exec resume`;
		// cwd is passed via exec.Cmd.Dir and sandbox via `-c sandbox_mode=...`.
		args := []string{
			"exec", "resume",
			"--json",
			"-c", fmt.Sprintf("sandbox_mode=%q", sandboxMode),
		}
		if model := strings.TrimSpace(req.Model); model != "" {
			args = append(args, "-m", model)
		}
		args = append(args, sessID, req.Prompt)
		return args, nil
	}

	args := []string{
		"exec",
		"--json",
		"--sandbox", sandboxMode,
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		args = append(args, "-m", model)
	}
	if cwd := strings.TrimSpace(req.Cwd); cwd != "" {
		args = append(args, "-C", cwd)
	}
	args = append(args, req.Prompt)
	return args, nil
}

func mapSandboxMode(perm provider.Perm) (string, error) {
	switch perm {
	case "", provider.PermReadOnly:
		return "read-only", nil
	case provider.PermEdit:
		return "workspace-write", nil
	default:
		return "", fmt.Errorf("不正な権限モード --perm=%q です（%q または %q を指定してください）", perm, provider.PermReadOnly, provider.PermEdit)
	}
}

func (a *Adapter) ChildEnv(base []string, auth provider.AuthMode) []string {
	switch auth {
	case provider.AuthSubscription:
		// Strip API keys so subscription login is used cleanly without unintended API billing.
		return provider.FilterEnv(base, "CODEX_API_KEY", "OPENAI_API_KEY")
	case provider.AuthAPIKey:
		cleaned := provider.FilterEnv(base, "CODEX_API_KEY", "OPENAI_API_KEY")
		if val, ok := provider.LookupEnvSlice(base, a.apiKeyEnv); ok && val != "" {
			return provider.SetEnv(cleaned, "CODEX_API_KEY", val)
		}
		return cleaned
	default:
		return provider.FilterEnv(base, "CODEX_API_KEY", "OPENAI_API_KEY")
	}
}

func (a *Adapter) NeedsPTY() bool { return false }

type codexLine struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Message  string          `json:"message"`
	Error    *codexError     `json:"error"`
	Usage    *codexUsage     `json:"usage"`
	Item     json.RawMessage `json:"item"`
}

type codexError struct {
	Message string `json:"message"`
}

type codexUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type codexItem struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Message          string          `json:"message"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregated_output"`
	Tool             string          `json:"tool"`
	Server           string          `json:"server"`
	Arguments        json.RawMessage `json:"arguments"`
	Result           json.RawMessage `json:"result"`
	Changes          json.RawMessage `json:"changes"`
}

// Parse normalizes a single JSONL line from `codex exec --json` into zero or
// more normalized events. Unknown or unparseable lines are preserved as
// event.Unknown without returning an error.
func (a *Adapter) Parse(line []byte) ([]event.Event, error) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return nil, nil
	}

	at := a.now()
	raw := event.SafeRaw(trimmed)

	var msg codexLine
	if err := json.Unmarshal(trimmed, &msg); err != nil {
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}, nil
	}

	switch msg.Type {
	case "thread.started":
		return []event.Event{{
			Provider:  a.ID(),
			Kind:      event.SessionStarted,
			SessionID: msg.ThreadID,
			Raw:       raw,
			At:        at,
		}}, nil

	case "item.started", "item.updated", "item.completed":
		return a.parseItemEvent(msg.Type, msg.Item, trimmed, raw, at), nil

	case "turn.completed":
		var events []event.Event
		if msg.Usage != nil {
			events = append(events, event.Event{
				Provider: a.ID(),
				Kind:     event.Usage,
				Usage: &event.UsageInfo{
					InputTokens:  msg.Usage.InputTokens,
					OutputTokens: msg.Usage.OutputTokens,
				},
				Raw: raw,
				At:  at,
			})
		}
		events = append(events, event.Event{
			Provider: a.ID(),
			Kind:     event.Completed,
			Raw:      raw,
			At:       at,
		})
		return events, nil

	case "turn.failed":
		errMsg := msg.Message
		if msg.Error != nil && msg.Error.Message != "" {
			errMsg = msg.Error.Message
		}
		return []event.Event{{
			Provider: a.ID(),
			Kind:     event.Error,
			Text:     errMsg,
			Raw:      raw,
			At:       at,
		}}, nil

	case "error":
		errMsg := msg.Message
		if errMsg == "" && msg.Error != nil {
			errMsg = msg.Error.Message
		}
		return []event.Event{{
			Provider: a.ID(),
			Kind:     event.Error,
			Text:     errMsg,
			Raw:      raw,
			At:       at,
		}}, nil

	default:
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}, nil
	}
}

func (a *Adapter) parseItemEvent(eventType string, rawItem json.RawMessage, trimmed []byte, raw json.RawMessage, at time.Time) []event.Event {
	if len(bytes.TrimSpace(rawItem)) == 0 {
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}
	}
	var item codexItem
	if err := json.Unmarshal(rawItem, &item); err != nil {
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}
	}

	switch item.Type {
	case "agent_message":
		if eventType == "item.completed" || item.Text != "" {
			return []event.Event{{
				Provider: a.ID(),
				Kind:     event.Text,
				Text:     item.Text,
				Raw:      raw,
				At:       at,
			}}
		}
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}

	case "command_execution":
		switch eventType {
		case "item.started":
			inputJSON, _ := json.Marshal(map[string]string{"command": item.Command})
			return []event.Event{{
				Provider:  a.ID(),
				Kind:      event.ToolCall,
				ToolName:  "command_execution",
				ToolInput: json.RawMessage(inputJSON),
				Raw:       raw,
				At:        at,
			}}
		case "item.completed":
			return []event.Event{{
				Provider: a.ID(),
				Kind:     event.ToolResult,
				ToolName: "command_execution",
				Text:     item.AggregatedOutput,
				Raw:      raw,
				At:       at,
			}}
		default:
			return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}
		}

	case "file_change", "mcp_tool_call", "web_search":
		toolName := item.Type
		if item.Tool != "" {
			toolName = item.Tool
			if item.Server != "" {
				toolName = item.Server + "/" + item.Tool
			}
		}
		switch eventType {
		case "item.started":
			toolInput := item.Arguments
			if len(bytes.TrimSpace(toolInput)) == 0 {
				toolInput = item.Changes
			}
			if len(bytes.TrimSpace(toolInput)) == 0 {
				toolInput = rawItem
			}
			return []event.Event{{
				Provider:  a.ID(),
				Kind:      event.ToolCall,
				ToolName:  toolName,
				ToolInput: copyRaw(toolInput),
				Raw:       raw,
				At:        at,
			}}
		case "item.completed":
			text := item.AggregatedOutput
			if text == "" {
				text = item.Text
			}
			if text == "" && len(bytes.TrimSpace(item.Result)) > 0 {
				text = string(bytes.TrimSpace(item.Result))
			}
			if text == "" && len(bytes.TrimSpace(item.Changes)) > 0 {
				text = string(bytes.TrimSpace(item.Changes))
			}
			return []event.Event{{
				Provider: a.ID(),
				Kind:     event.ToolResult,
				ToolName: toolName,
				Text:     text,
				Raw:      raw,
				At:       at,
			}}
		default:
			return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}
		}

	case "error":
		return []event.Event{{
			Provider: a.ID(),
			Kind:     event.Error,
			Text:     item.Message,
			Raw:      raw,
			At:       at,
		}}

	default:
		return []event.Event{event.NewUnknown(a.ID(), trimmed, at)}
	}
}

func copyRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return json.RawMessage(cp)
}
