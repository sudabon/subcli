package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/provider"
)

const (
	// DefaultSideBySideMinWidth is the minimum terminal column width required
	// for side-by-side compare layout before falling back to stacked vertical blocks.
	DefaultSideBySideMinWidth = 100
	// DefaultTerminalWidth is used when terminal width cannot be queried.
	DefaultTerminalWidth = 120
	minColumnWidth       = 20
)

// CompareColumn holds one provider's completed output for compare TTY rendering.
type CompareColumn struct {
	Index    int
	Provider string
	Auth     provider.AuthMode
	Text     string
	Tools    []string
	Usage    *event.UsageInfo
	ExitCode int
	ErrText  string
}

// CompareFormatOptions configures side-by-side vs stacked compare rendering.
type CompareFormatOptions struct {
	Width         int
	MinSideBySide int
	Color         bool
}

// CompareJSONLEvent extends normalized event.Event with compare_index while
// preserving every standard event.Event field (including "provider") at top level.
type CompareJSONLEvent struct {
	CompareIndex int `json:"compare_index"`
	event.Event
}

// CompareJSONLRenderer writes provider-tagged compare events as JSONL.
type CompareJSONLRenderer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewCompareJSONLRenderer creates a CompareJSONLRenderer writing to w.
func NewCompareJSONLRenderer(w io.Writer) *CompareJSONLRenderer {
	return &CompareJSONLRenderer{w: w}
}

// Render encodes a compare event as one compact JSON line.
func (r *CompareJSONLRenderer) Render(index int, ev event.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := json.Marshal(CompareJSONLEvent{
		CompareIndex: index,
		Event:        ev,
	})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = r.w.Write(data)
	return err
}

// ChainJSONLEvent extends normalized event.Event with a 1-based step number.
type ChainJSONLEvent struct {
	Step int `json:"step"`
	event.Event
}

// ChainJSONLRenderer writes step-tagged chain events as JSONL.
type ChainJSONLRenderer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewChainJSONLRenderer creates a ChainJSONLRenderer writing to w.
func NewChainJSONLRenderer(w io.Writer) *ChainJSONLRenderer {
	return &ChainJSONLRenderer{w: w}
}

// Render encodes a chain event as one compact JSON line (step is 1-based).
func (r *ChainJSONLRenderer) Render(step int, ev event.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := json.Marshal(ChainJSONLEvent{
		Step:  step,
		Event: ev,
	})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = r.w.Write(data)
	return err
}

// WriteChainStepHeader prints a human-readable header before each chain step.
func WriteChainStepHeader(w io.Writer, stepIndex, totalSteps int, provID string, color bool) error {
	if stepIndex > 0 {
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	label := providerLabel(provID, 0, false, color)
	_, err := fmt.Fprintf(w, "=== Step %d/%d %s ===\n", stepIndex+1, totalSteps, label)
	return err
}

// RenderCompare formats compare columns side-by-side when terminal width is
// sufficient (>= MinSideBySide, default 100) and len(cols) >= 2, or falls back
// to vertically stacked provider sections on narrow terminals.
func RenderCompare(w io.Writer, cols []CompareColumn, opts CompareFormatOptions) error {
	if len(cols) == 0 {
		return nil
	}

	width := opts.Width
	if width <= 0 {
		width = TerminalWidth(w)
	}
	minWide := opts.MinSideBySide
	if minWide <= 0 {
		minWide = DefaultSideBySideMinWidth
	}

	// Count provider occurrences so duplicate providers (e.g. claude,claude) get numbered labels.
	counts := make(map[string]int, len(cols))
	for _, c := range cols {
		counts[c.Provider]++
	}
	seen := make(map[string]int, len(cols))
	headers := make([]string, len(cols))
	for i, c := range cols {
		seen[c.Provider]++
		headers[i] = providerLabel(c.Provider, seen[c.Provider], counts[c.Provider] > 1, opts.Color)
	}

	const divider = " │ "
	const dividerWidth = 3

	useSideBySide := len(cols) >= 2 && width >= minWide
	colWidth := 0
	if useSideBySide {
		colWidth = (width - dividerWidth*(len(cols)-1)) / len(cols)
		if colWidth < minColumnWidth {
			useSideBySide = false
		}
	}

	if !useSideBySide {
		return renderCompareStacked(w, cols, headers)
	}
	return renderCompareSideBySide(w, cols, headers, colWidth, divider)
}

func renderCompareStacked(w io.Writer, cols []CompareColumn, headers []string) error {
	for i, c := range cols {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "=== %s ===\n", headers[i]); err != nil {
			return err
		}
		for _, line := range buildColumnLines(c) {
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderCompareSideBySide(w io.Writer, cols []CompareColumn, headers []string, colWidth int, divider string) error {
	// Header row
	for i, h := range headers {
		if i > 0 {
			if _, err := io.WriteString(w, divider); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, padDisplayRight(truncateDisplay(h, colWidth), colWidth)); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	// Separator row
	sepSeg := strings.Repeat("─", colWidth)
	for i := range cols {
		if i > 0 {
			if _, err := io.WriteString(w, "─┼─"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, sepSeg); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	// Wrap each column's body lines to colWidth.
	wrapped := make([][]string, len(cols))
	maxRows := 0
	for i, c := range cols {
		rawLines := buildColumnLines(c)
		var lines []string
		for _, rl := range rawLines {
			lines = append(lines, wrapDisplayWidth(rl, colWidth)...)
		}
		if len(lines) == 0 {
			lines = []string{""}
		}
		wrapped[i] = lines
		if len(lines) > maxRows {
			maxRows = len(lines)
		}
	}

	for row := 0; row < maxRows; row++ {
		for i := range cols {
			if i > 0 {
				if _, err := io.WriteString(w, divider); err != nil {
					return err
				}
			}
			cell := ""
			if row < len(wrapped[i]) {
				cell = wrapped[i][row]
			}
			if i < len(cols)-1 {
				cell = padDisplayRight(cell, colWidth)
			}
			if _, err := io.WriteString(w, cell); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func buildColumnLines(c CompareColumn) []string {
	var lines []string
	for _, t := range c.Tools {
		lines = append(lines, "tool: "+t)
	}
	if c.Text != "" {
		normalized := strings.ReplaceAll(c.Text, "\r\n", "\n")
		normalized = strings.TrimRight(normalized, "\n")
		lines = append(lines, strings.Split(normalized, "\n")...)
	}
	if strings.TrimSpace(c.ErrText) != "" {
		for _, el := range strings.Split(strings.TrimRight(c.ErrText, "\n"), "\n") {
			lines = append(lines, "error: "+el)
		}
	} else if c.ExitCode != 0 {
		lines = append(lines, fmt.Sprintf("error: exit code %d", c.ExitCode))
	}
	if c.Usage != nil {
		lines = append(lines, formatUsageLines(c.Usage, c.Auth)...)
	}
	if len(lines) == 0 {
		lines = append(lines, "(no output)")
	}
	return lines
}

func formatUsageLines(u *event.UsageInfo, auth provider.AuthMode) []string {
	var b strings.Builder
	fmt.Fprintf(&b, "usage: in=%d out=%d", u.InputTokens, u.OutputTokens)
	if u.CostUSD != nil {
		fmt.Fprintf(&b, " cost=$%.4f", *u.CostUSD)
	}
	lines := []string{b.String()}
	if u.CostUSD != nil && auth == provider.AuthSubscription {
		lines = append(lines, fmt.Sprintf("  [%s]", SubscriptionCostDisclaimer))
	}
	return lines
}

func providerLabel(prov string, seq int, numbered bool, color bool) string {
	label := "[" + prov + "]"
	if numbered {
		label = fmt.Sprintf("[%s #%d]", prov, seq)
	}
	if !color {
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

// SummarizeToolEvent formats a tool_call event into a concise single-line summary.
func SummarizeToolEvent(ev event.Event) string {
	summary := summarizeToolInput(ev.ToolInput)
	if summary != "" {
		return fmt.Sprintf("%s (%s)", ev.ToolName, summary)
	}
	return ev.ToolName
}

func displayWidth(s string) int {
	w := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		w += runeDisplayWidth(r)
	}
	return w
}

func runeDisplayWidth(r rune) int {
	if r < 0x20 || r == 0x7f {
		return 0
	}
	if (r >= 0x1100 && r <= 0x115f) ||
		r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) ||
		(r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) ||
		(r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) ||
		(r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x20000 && r <= 0x2fffd) ||
		(r >= 0x30000 && r <= 0x3fffd) {
		return 2
	}
	return 1
}

func padDisplayRight(s string, width int) string {
	w := displayWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func truncateDisplay(s string, maxWidth int) string {
	if displayWidth(s) <= maxWidth {
		return s
	}
	var b strings.Builder
	w := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			b.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			b.WriteRune(r)
			continue
		}
		rw := runeDisplayWidth(r)
		if w+rw > maxWidth {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

func wrapDisplayWidth(s string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{s}
	}
	if s == "" {
		return []string{""}
	}
	var out []string
	var cur strings.Builder
	curW := 0
	for _, r := range s {
		rw := runeDisplayWidth(r)
		if curW+rw > maxWidth && curW > 0 {
			out = append(out, cur.String())
			cur.Reset()
			curW = 0
		}
		cur.WriteRune(r)
		curW += rw
	}
	if cur.Len() > 0 || len(out) == 0 {
		out = append(out, cur.String())
	}
	return out
}
