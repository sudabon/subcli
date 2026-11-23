package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sudabon/subcli/internal/logging"
	_ "modernc.org/sqlite"
)

const (
	// MaxPromptPreviewRunes is the maximum Unicode rune length stored for prompt previews.
	MaxPromptPreviewRunes = 80
)

// ErrNotFound is returned when a requested local session ID does not exist.
var ErrNotFound = errors.New("session not found")

// Record stores non-sensitive metadata for a headless session.
type Record struct {
	ID                string
	Provider          string
	ProviderSessionID string
	Cwd               string
	CreatedAt         time.Time
	LastRunAt         time.Time
	PromptPreview     string
}

// Store persists local session metadata in a SQLite database with 0600 permissions.
type Store struct {
	db   *sql.DB
	path string
}

// DefaultPath returns $XDG_DATA_HOME/subcli/sessions.db or
// ~/.local/share/subcli/sessions.db when XDG_DATA_HOME is unset.
func DefaultPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "subcli", "sessions.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("データディレクトリの取得に失敗しました: %w", err)
	}
	return filepath.Join(home, ".local", "share", "subcli", "sessions.db"), nil
}

// OpenDefault opens or initializes the SQLite session store at DefaultPath.
func OpenDefault() (*Store, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

// Open opens or initializes the SQLite session store at path with 0700 directory
// and 0600 file permissions.
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("セッションディレクトリの作成に失敗しました: %w", err)
	}
	_ = os.Chmod(dir, 0o700)

	// Pre-create with 0600 so the database file is never world- or group-readable.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("セッション DB ファイル %s の作成に失敗しました: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("セッション DB ファイルのクローズに失敗しました: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("セッション DB のパーミッション設定に失敗しました: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("セッション DB を開けません: %w", err)
	}
	db.SetMaxOpenConns(1)

	const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	provider TEXT NOT NULL,
	provider_session_id TEXT NOT NULL,
	cwd TEXT NOT NULL,
	created_at TEXT NOT NULL,
	last_run_at TEXT NOT NULL,
	prompt_preview TEXT NOT NULL
);`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("セッションテーブルの初期化に失敗しました: %w", err)
	}
	_ = os.Chmod(path, 0o600)

	return &Store{db: db, path: path}, nil
}

// Close closes the underlying SQLite connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Path returns the filesystem path of the SQLite database.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// FormatPromptPreview redacts secret-like substrings, normalizes newlines, and
// truncates to MaxPromptPreviewRunes Unicode runes.
func FormatPromptPreview(prompt string) string {
	redacted := logging.Redact(prompt)
	redacted = strings.ReplaceAll(redacted, "\r\n", " ")
	redacted = strings.ReplaceAll(redacted, "\n", " ")
	redacted = strings.ReplaceAll(redacted, "\r", " ")
	redacted = strings.TrimSpace(redacted)

	runes := []rune(redacted)
	if len(runes) > MaxPromptPreviewRunes {
		return string(runes[:MaxPromptPreviewRunes])
	}
	return redacted
}

// Create inserts a new session record and returns the persisted Record.
func (s *Store) Create(ctx context.Context, rec Record) (Record, error) {
	if strings.TrimSpace(rec.Provider) == "" {
		return Record{}, errors.New("provider is required")
	}
	if strings.TrimSpace(rec.ProviderSessionID) == "" {
		return Record{}, errors.New("provider_session_id is required")
	}

	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	} else {
		rec.CreatedAt = rec.CreatedAt.UTC()
	}
	if rec.LastRunAt.IsZero() {
		rec.LastRunAt = rec.CreatedAt
	} else {
		rec.LastRunAt = rec.LastRunAt.UTC()
	}
	rec.PromptPreview = FormatPromptPreview(rec.PromptPreview)

	if strings.TrimSpace(rec.ID) == "" {
		id, err := generateID()
		if err != nil {
			return Record{}, err
		}
		rec.ID = id
	}

	const q = `
INSERT INTO sessions (id, provider, provider_session_id, cwd, created_at, last_run_at, prompt_preview)
VALUES (?, ?, ?, ?, ?, ?, ?);`
	_, err := s.db.ExecContext(ctx, q,
		rec.ID,
		rec.Provider,
		rec.ProviderSessionID,
		rec.Cwd,
		rec.CreatedAt.Format(time.RFC3339Nano),
		rec.LastRunAt.Format(time.RFC3339Nano),
		rec.PromptPreview,
	)
	if err != nil {
		return Record{}, fmt.Errorf("セッションの保存に失敗しました: %w", err)
	}
	return rec, nil
}

// Get retrieves a session record by local ID.
func (s *Store) Get(ctx context.Context, id string) (Record, error) {
	const q = `
SELECT id, provider, provider_session_id, cwd, created_at, last_run_at, prompt_preview
FROM sessions
WHERE id = ?;`
	row := s.db.QueryRowContext(ctx, q, id)
	rec, err := scanRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, ErrNotFound
		}
		return Record{}, fmt.Errorf("セッション %q の取得に失敗しました: %w", id, err)
	}
	return rec, nil
}

// List returns all saved sessions ordered by most recent run first.
func (s *Store) List(ctx context.Context) ([]Record, error) {
	const q = `
SELECT id, provider, provider_session_id, cwd, created_at, last_run_at, prompt_preview
FROM sessions
ORDER BY last_run_at DESC, created_at DESC;`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("セッション一覧の取得に失敗しました: %w", err)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("セッション行の読み取りに失敗しました: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("セッション一覧の走査に失敗しました: %w", err)
	}
	return out, nil
}

// Touch updates the last_run_at timestamp for an existing session.
func (s *Store) Touch(ctx context.Context, id string, lastRunAt time.Time) error {
	if lastRunAt.IsZero() {
		lastRunAt = time.Now().UTC()
	} else {
		lastRunAt = lastRunAt.UTC()
	}
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_run_at = ? WHERE id = ?;`, lastRunAt.Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("セッション %q の更新に失敗しました: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a session record by local ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?;`, id)
	if err != nil {
		return fmt.Errorf("セッション %q の削除に失敗しました: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRecord(sc scanner) (Record, error) {
	var rec Record
	var createdStr, lastRunStr string
	if err := sc.Scan(
		&rec.ID,
		&rec.Provider,
		&rec.ProviderSessionID,
		&rec.Cwd,
		&createdStr,
		&lastRunStr,
		&rec.PromptPreview,
	); err != nil {
		return Record{}, err
	}
	var err error
	rec.CreatedAt, err = time.Parse(time.RFC3339Nano, createdStr)
	if err != nil {
		return Record{}, err
	}
	rec.LastRunAt, err = time.Parse(time.RFC3339Nano, lastRunStr)
	if err != nil {
		return Record{}, err
	}
	return rec, nil
}

func generateID() (string, error) {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("ローカルセッション ID の生成に失敗しました: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
