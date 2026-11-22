package session_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sudabon/subcli/internal/session"
)

func TestStore_PermissionsAndCRUD(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "subcli", "sessions.db")
	store, err := session.Open(dbPath)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer store.Close()

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(dbPath)
		if err != nil {
			t.Fatalf("stat db: %v", err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("db file perm = %04o, want 0600", perm)
		}
		dirInfo, err := os.Stat(filepath.Dir(dbPath))
		if err != nil {
			t.Fatalf("stat dir: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0o700 {
			t.Errorf("db dir perm = %04o, want 0700", perm)
		}
	}

	ctx := context.Background()
	t1 := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)
	t3 := t1.Add(10 * time.Minute)

	rec1, err := store.Create(ctx, session.Record{
		Provider:          "claude",
		ProviderSessionID: "claude-sess-1",
		Cwd:               "/workspace/repo",
		CreatedAt:         t1,
		LastRunAt:         t1,
		PromptPreview:     "最初のプロンプト",
	})
	if err != nil {
		t.Fatalf("Create rec1: %v", err)
	}
	if len(rec1.ID) < 8 || len(rec1.ID) > 12 {
		t.Errorf("generated ID %q length out of expected range [8,12]", rec1.ID)
	}

	rec2, err := store.Create(ctx, session.Record{
		Provider:          "codex",
		ProviderSessionID: "codex-thread-2",
		Cwd:               "/workspace/repo",
		CreatedAt:         t2,
		LastRunAt:         t2,
		PromptPreview:     "2番目のプロンプト",
	})
	if err != nil {
		t.Fatalf("Create rec2: %v", err)
	}

	// List should return rec2 before rec1 (most recent last_run_at first)
	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(list) != 2 || list[0].ID != rec2.ID || list[1].ID != rec1.ID {
		t.Fatalf("List order = %+v, want [%s, %s]", list, rec2.ID, rec1.ID)
	}

	// Touch rec1 to t3 so rec1 becomes most recent
	if err := store.Touch(ctx, rec1.ID, t3); err != nil {
		t.Fatalf("Touch error: %v", err)
	}
	got, err := store.Get(ctx, rec1.ID)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !got.LastRunAt.Equal(t3) {
		t.Errorf("LastRunAt = %v, want %v", got.LastRunAt, t3)
	}

	list, err = store.List(ctx)
	if err != nil {
		t.Fatalf("List after touch error: %v", err)
	}
	if list[0].ID != rec1.ID {
		t.Errorf("List[0].ID = %q after Touch, want %q", list[0].ID, rec1.ID)
	}

	// Delete rec2
	if err := store.Delete(ctx, rec2.ID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if _, err := store.Get(ctx, rec2.ID); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Get deleted record err = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, "nonexistent"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Delete nonexistent err = %v, want ErrNotFound", err)
	}
	if err := store.Touch(ctx, "nonexistent", t3); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Touch nonexistent err = %v, want ErrNotFound", err)
	}
}

func TestStore_PromptPreviewTruncationAndSecretMasking(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	store, err := session.Open(dbPath)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer store.Close()

	// 100 Japanese runes must truncate cleanly to 80 valid UTF-8 runes
	longJP := strings.Repeat("あいうえおかきくけこ", 10)
	rec, err := store.Create(context.Background(), session.Record{
		Provider:          "claude",
		ProviderSessionID: "sess-jp",
		Cwd:               "/tmp",
		PromptPreview:     longJP,
	})
	if err != nil {
		t.Fatalf("Create JP: %v", err)
	}
	if !utf8.ValidString(rec.PromptPreview) {
		t.Errorf("PromptPreview is invalid UTF-8: %q", rec.PromptPreview)
	}
	if gotRunes := utf8.RuneCountInString(rec.PromptPreview); gotRunes != session.MaxPromptPreviewRunes {
		t.Errorf("rune count = %d, want %d", gotRunes, session.MaxPromptPreviewRunes)
	}

	// Secret-like tokens in prompt must be masked before saving to SQLite
	secretPrompt := "check key sk-ant-secret1234567890 and AIzaSyDummySecretKey12345"
	recSec, err := store.Create(context.Background(), session.Record{
		Provider:          "codex",
		ProviderSessionID: "sess-sec",
		Cwd:               "/tmp",
		PromptPreview:     secretPrompt,
	})
	if err != nil {
		t.Fatalf("Create secret: %v", err)
	}
	if strings.Contains(recSec.PromptPreview, "sk-ant-secret1234567890") || strings.Contains(recSec.PromptPreview, "AIzaSyDummySecretKey12345") {
		t.Errorf("secret leaked in PromptPreview: %q", recSec.PromptPreview)
	}
	if !strings.Contains(recSec.PromptPreview, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in PromptPreview: %q", recSec.PromptPreview)
	}

	// Also confirm raw DB bytes contain no leaked secret
	rawDB, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read raw db: %v", err)
	}
	if strings.Contains(string(rawDB), "sk-ant-secret1234567890") || strings.Contains(string(rawDB), "AIzaSyDummySecretKey12345") {
		t.Errorf("raw sessions.db contains unmasked secret")
	}
}

func TestDefaultPath_RespectsXDGDataHome(t *testing.T) {
	customDir := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", customDir)

	got, err := session.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath error: %v", err)
	}
	want := filepath.Join(customDir, "subcli", "sessions.db")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}
