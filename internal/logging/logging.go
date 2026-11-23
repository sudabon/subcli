package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const RedactedPlaceholder = "[REDACTED]"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9._-]{8,}`),
	regexp.MustCompile(`AIza[A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|auth[_-]?token|access[_-]?token|secret|password)\s*[=:]\s*)([^\s"',;]+)`),
}

// Redact masks API key and token-like substrings before writing to logs.
func Redact(input string) string {
	out := input
	for i, re := range secretPatterns {
		if i < 2 {
			out = re.ReplaceAllString(out, RedactedPlaceholder)
		} else {
			out = re.ReplaceAllString(out, "${1}"+RedactedPlaceholder)
		}
	}
	return out
}

// DefaultPath returns $XDG_STATE_HOME/subcli/subcli.log or ~/.local/state/subcli/subcli.log.
func DefaultPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); xdg != "" {
		return filepath.Join(xdg, "subcli", "subcli.log"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("状態ディレクトリの取得に失敗しました: %w", err)
	}
	return filepath.Join(home, ".local", "state", "subcli", "subcli.log"), nil
}

// Logger writes masked single-line structured log entries without dumping process environments.
type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	closer io.Closer
	now    func() time.Time
}

// OpenDefault opens the log file at DefaultPath.
func OpenDefault() (*Logger, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return OpenFile(path)
}

// OpenFile opens or creates the log file at path with 0600 permissions.
func OpenFile(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("ログディレクトリの作成に失敗しました: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ログファイル %s を開けません: %w", path, err)
	}
	return &Logger{
		out:    f,
		closer: f,
		now:    time.Now,
	}, nil
}

// NewWriter creates a Logger backed by an arbitrary io.Writer (useful in tests).
func NewWriter(w io.Writer) *Logger {
	return &Logger{
		out: w,
		now: time.Now,
	}
}

// Close closes the underlying log file if owned by the Logger.
func (l *Logger) Close() error {
	if l == nil || l.closer == nil {
		return nil
	}
	return l.closer.Close()
}

// Field is a key/value log attribute. Values are always masked before writing.
type Field struct {
	Key   string
	Value string
}

// Info logs an informational event with redacted message and fields.
func (l *Logger) Info(msg string, fields ...Field) {
	l.log("INFO", msg, fields...)
}

// Warn logs a warning event with redacted message and fields.
func (l *Logger) Warn(msg string, fields ...Field) {
	l.log("WARN", msg, fields...)
}

// Error logs an error event with redacted message and fields.
func (l *Logger) Error(msg string, fields ...Field) {
	l.log("ERROR", msg, fields...)
}

func (l *Logger) log(level, msg string, fields ...Field) {
	if l == nil || l.out == nil {
		return
	}
	ts := l.now().UTC().Format(time.RFC3339)
	var b strings.Builder
	b.WriteString(ts)
	b.WriteByte(' ')
	b.WriteString(level)
	b.WriteByte(' ')
	b.WriteString(sanitizeLine(Redact(msg)))
	for _, f := range fields {
		b.WriteByte(' ')
		b.WriteString(sanitizeLine(f.Key))
		b.WriteByte('=')
		b.WriteString(fmt.Sprintf("%q", sanitizeLine(Redact(f.Value))))
	}
	b.WriteByte('\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = io.WriteString(l.out, b.String())
}

func sanitizeLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
