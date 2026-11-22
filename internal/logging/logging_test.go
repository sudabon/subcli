package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedact_MasksKnownSecretPatterns(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		forbidden []string
	}{
		{
			name:      "openai and anthropic sk keys",
			input:     "failed with key sk-proj-abcd1234EFGH5678 and sk-ant-api03-999988887777",
			forbidden: []string{"sk-proj-abcd1234EFGH5678", "sk-ant-api03-999988887777"},
		},
		{
			name:      "google AIza key",
			input:     "gemini error AIzaSyD1234567890abcdefg",
			forbidden: []string{"AIzaSyD1234567890abcdefg"},
		},
		{
			name:      "bearer token",
			input:     "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig",
			forbidden: []string{"eyJhbGciOiJIUzI1NiJ9.payload.sig"},
		},
		{
			name:      "key value assignment",
			input:     "api_key=super_secret_token_999",
			forbidden: []string{"super_secret_token_999"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Redact(tc.input)
			if !strings.Contains(got, RedactedPlaceholder) {
				t.Errorf("Redact(%q) = %q, expected %q", tc.input, got, RedactedPlaceholder)
			}
			for _, f := range tc.forbidden {
				if strings.Contains(got, f) {
					t.Errorf("Redact(%q) leaked secret %q in %q", tc.input, f, got)
				}
			}
		})
	}
}

func TestLogger_WritesMaskedEntryToXDGStateHome(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("SUBCLI_TEST_ENV_SECRET", "env-should-not-be-dumped")

	logger, err := OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault() error: %v", err)
	}
	logger.Error("upstream failed sk-ant-secret123456789", Field{Key: "detail", Value: "AIzaSySecretKey987654321"})
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	logPath := filepath.Join(stateDir, "subcli", "subcli.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	content := string(data)

	for _, leaked := range []string{"sk-ant-secret123456789", "AIzaSySecretKey987654321", "env-should-not-be-dumped", "SUBCLI_TEST_ENV_SECRET"} {
		if strings.Contains(content, leaked) {
			t.Errorf("log file leaked %q:\n%s", leaked, content)
		}
	}
	if !strings.Contains(content, RedactedPlaceholder) {
		t.Errorf("log file missing redaction marker:\n%s", content)
	}
}
