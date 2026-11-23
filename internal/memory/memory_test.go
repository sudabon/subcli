package memory

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFindRepoRoot(t *testing.T) {
	t.Run("finds git root from nested subdirectory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		nested := filepath.Join(root, "a", "b", "c")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}

		gotRoot, isGit, err := FindRepoRoot(nested)
		if err != nil {
			t.Fatalf("FindRepoRoot error: %v", err)
		}
		if !isGit || gotRoot != root {
			t.Errorf("FindRepoRoot = (%q, %v), want (%q, true)", gotRoot, isGit, root)
		}
	})

	t.Run("supports git worktree file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /tmp/somewhere\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		nested := filepath.Join(root, "pkg")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}

		gotRoot, isGit, err := FindRepoRoot(nested)
		if err != nil {
			t.Fatalf("FindRepoRoot error: %v", err)
		}
		if !isGit || gotRoot != root {
			t.Errorf("FindRepoRoot = (%q, %v), want (%q, true)", gotRoot, isGit, root)
		}
	})

	t.Run("falls back to startDir when no git repo", func(t *testing.T) {
		dir := t.TempDir()
		gotRoot, isGit, err := FindRepoRoot(dir)
		if err != nil {
			t.Fatalf("FindRepoRoot error: %v", err)
		}
		if isGit || gotRoot != dir {
			t.Errorf("FindRepoRoot = (%q, %v), want (%q, false)", gotRoot, isGit, dir)
		}
	})
}

func TestInitAndGitignoreIdempotency(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}

	res1, err := store.Init(InitOptions{})
	if err != nil {
		t.Fatalf("first Init error: %v", err)
	}
	if !res1.CreatedMemory || !res1.UpdatedGitignore {
		t.Errorf("first Init = %+v, want CreatedMemory=true, UpdatedGitignore=true", res1)
	}

	giData, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	giText := string(giData)
	if !strings.Contains(giText, "node_modules/\n\n# subcli") || !strings.Contains(giText, "\n.subcli/\n") {
		t.Errorf("unexpected .gitignore content:\n%q", giText)
	}

	// Second Init must be completely idempotent and preserve existing MEMORY.md.
	if err := os.WriteFile(store.MemoryFile, []byte("# Custom\nkeep me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res2, err := store.Init(InitOptions{})
	if err != nil {
		t.Fatalf("second Init error: %v", err)
	}
	if res2.CreatedMemory || res2.UpdatedGitignore {
		t.Errorf("second Init = %+v, want CreatedMemory=false, UpdatedGitignore=false", res2)
	}
	mainContent, err := store.ReadMain()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mainContent, "keep me") {
		t.Errorf("Init overwrote existing MEMORY.md: %q", mainContent)
	}

	// Projections must exist and reflect MEMORY.md.
	for _, proj := range []string{
		filepath.Join(store.ProjectionDir("claude"), "CLAUDE.md"),
		filepath.Join(store.ProjectionDir("agy"), "AGENTS.md"),
	} {
		data, readErr := os.ReadFile(proj)
		if readErr != nil {
			t.Fatalf("missing projection %s: %v", proj, readErr)
		}
		if !strings.Contains(string(data), "keep me") {
			t.Errorf("projection %s missing memory content", proj)
		}
	}

	if runtime.GOOS != "windows" {
		if info, err := os.Stat(store.BaseDir); err == nil && info.Mode().Perm() != 0o700 {
			t.Errorf(".subcli perm = %o, want 700", info.Mode().Perm())
		}
		if info, err := os.Stat(store.MemoryFile); err == nil && info.Mode().Perm() != 0o600 {
			t.Errorf("MEMORY.md perm = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestAddEntryTopicRedactionAndRemove(t *testing.T) {
	root := t.TempDir()
	store := ForRoot(root, true)
	now := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

	// Main entry with secret must be redacted.
	res, err := store.AddEntry("Use API key sk-ant-secret1234567890abcd for staging", "", now, InitOptions{})
	if err != nil {
		t.Fatalf("AddEntry main error: %v", err)
	}
	if !res.Redacted {
		t.Errorf("expected secret to be flagged as redacted")
	}
	mainText, err := store.ReadMain()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mainText, "sk-ant-secret1234567890abcd") {
		t.Errorf("raw secret leaked into MEMORY.md: %s", mainText)
	}
	if !strings.Contains(mainText, "[2026-11-22]") {
		t.Errorf("date prefix missing in MEMORY.md: %s", mainText)
	}

	// Topic entry creates topic file and index link once.
	_, err = store.AddEntry("Always run go test -race ./...", "testing", now, InitOptions{})
	if err != nil {
		t.Fatalf("AddEntry topic error: %v", err)
	}
	_, err = store.AddEntry("Use fake binaries under testdata/bin", "testing", now, InitOptions{})
	if err != nil {
		t.Fatalf("second AddEntry topic error: %v", err)
	}

	topicText, err := store.ReadTopic("testing")
	if err != nil {
		t.Fatalf("ReadTopic error: %v", err)
	}
	if !strings.Contains(topicText, "go test -race") || !strings.Contains(topicText, "testdata/bin") {
		t.Errorf("unexpected topic content:\n%s", topicText)
	}
	mainText, _ = store.ReadMain()
	if count := strings.Count(mainText, "- [testing](./testing.md)"); count != 1 {
		t.Errorf("expected exactly 1 topic link in MEMORY.md, got %d:\n%s", count, mainText)
	}

	// Invalid topic names must be rejected.
	for _, bad := range []string{"", "../escape", "a/b", "MEMORY", "memory", ".hidden"} {
		if _, err := store.AddEntry("hi", bad, now, InitOptions{}); bad != "" && err == nil {
			t.Errorf("ValidateTopic(%q) succeeded, want error", bad)
		}
	}

	// BuildProviderContext should include topic listing.
	ctx, err := store.BuildProviderContext("codex")
	if err != nil {
		t.Fatalf("BuildProviderContext error: %v", err)
	}
	if !strings.Contains(ctx.InlinePrompt, "testing.md") {
		t.Errorf("InlinePrompt missing topic file listing:\n%s", ctx.InlinePrompt)
	}

	// RemoveTopic deletes file and cleans index link.
	if err := store.RemoveTopic("testing"); err != nil {
		t.Fatalf("RemoveTopic error: %v", err)
	}
	if _, err := store.ReadTopic("testing"); err == nil {
		t.Errorf("expected error reading removed topic")
	}
	mainText, _ = store.ReadMain()
	if strings.Contains(mainText, "testing.md") {
		t.Errorf("topic link still present in MEMORY.md after removal:\n%s", mainText)
	}
}

func TestProjectionTruncation(t *testing.T) {
	root := t.TempDir()
	store := ForRoot(root, true)
	if _, err := store.Init(InitOptions{SkipGitignore: true}); err != nil {
		t.Fatal(err)
	}

	bigLine := strings.Repeat("あいうえお0123456789", 20) + "\n"
	bigBody := strings.Repeat(bigLine, 120) // > 30KB
	if err := os.WriteFile(store.MemoryFile, []byte(bigBody), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, err := store.BuildProviderContext("codex")
	if err != nil {
		t.Fatalf("BuildProviderContext error: %v", err)
	}
	if len(ctx.InlinePrompt) > MaxInlineArgBytes {
		t.Errorf("InlinePrompt len = %d, exceeds cap %d", len(ctx.InlinePrompt), MaxInlineArgBytes)
	}
	if !strings.Contains(ctx.InlinePrompt, "truncated by subcli") {
		t.Errorf("expected truncation marker in InlinePrompt")
	}

	agyData, err := os.ReadFile(filepath.Join(store.ProjectionDir("agy"), "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(agyData) > MaxProjectionBytes {
		t.Errorf("agy AGENTS.md len = %d, exceeds cap %d", len(agyData), MaxProjectionBytes)
	}
}

func TestImportMarkdownDirAndDefaultPaths(t *testing.T) {
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "MEMORY.md"), []byte("Remember token Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "notes.md"), []byte("Prefer table-driven tests.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "ignored.json"), []byte(`{"secret":"ignore"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	store := ForRoot(root, true)
	res, err := store.ImportMarkdownDir(srcDir, "claude", time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC), InitOptions{})
	if err != nil {
		t.Fatalf("ImportMarkdownDir error: %v", err)
	}
	if len(res.ImportedFiles) != 2 {
		t.Errorf("ImportedFiles = %v, want 2 markdown files", res.ImportedFiles)
	}
	if !res.Redacted {
		t.Errorf("expected Bearer token to be redacted during import")
	}

	imported, err := store.ReadTopic("imported-claude")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(imported, "eyJhbGciOiJIUzI1NiJ9") {
		t.Errorf("Bearer token leaked into imported file:\n%s", imported)
	}
	if !strings.Contains(imported, "Prefer table-driven tests.") {
		t.Errorf("expected markdown content in imported file:\n%s", imported)
	}
	if strings.Contains(imported, "ignored.json") {
		t.Errorf("non-markdown file should not be imported")
	}

	// DefaultImportDir mapping check.
	claudePath, err := DefaultImportDir("claude", "/Users/alice/work/subcli", "/Users/alice", nil)
	if err != nil {
		t.Fatal(err)
	}
	wantClaude := filepath.Join("/Users/alice", ".claude", "projects", "-Users-alice-work-subcli", "memory")
	if claudePath != wantClaude {
		t.Errorf("DefaultImportDir(claude) = %q, want %q", claudePath, wantClaude)
	}

	codexPath, err := DefaultImportDir("codex", "/Users/alice/work/subcli", "/Users/alice", func(k string) string {
		if k == "CODEX_HOME" {
			return "/custom/codex"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if codexPath != filepath.Join("/custom/codex", "memories") {
		t.Errorf("DefaultImportDir(codex) = %q, want /custom/codex/memories", codexPath)
	}
}

func TestTrackedMemoryUsesRepositoryIndex(t *testing.T) {
	root := t.TempDir()
	store := ForRoot(root, true)
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s: %v", args, out, err)
		}
	}
	runGit("init", "-q")
	if _, err := store.Init(InitOptions{}); err != nil {
		t.Fatal(err)
	}
	if tracked, err := store.HasTrackedMemory(); err != nil || tracked {
		t.Fatalf("local memory: tracked=%v err=%v", tracked, err)
	}
	runGit("add", "-f", "--", ".subcli/projections/agy/AGENTS.md")
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "other-index"))
	if tracked, err := store.HasTrackedMemory(); err != nil || !tracked {
		t.Fatalf("tracked projection: tracked=%v err=%v", tracked, err)
	}
}

func TestStoreRejectsLinkedPaths(t *testing.T) {
	for _, rel := range []string{".subcli", ".subcli/memory", ".subcli/memory/MEMORY.md", ".subcli/projections", ".subcli/projections/claude", ".subcli/projections/agy/AGENTS.md"} {
		t.Run(rel, func(t *testing.T) {
			store := ForRoot(t.TempDir(), false)
			if _, err := store.Init(InitOptions{}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(store.RepoRoot, rel)
			moved := filepath.Join(t.TempDir(), "local")
			if err := os.Rename(path, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(moved, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := store.BuildProviderContext("codex"); err == nil {
				t.Error("generated context through linked path")
			}
			if _, err := store.Status(); err == nil {
				t.Error("reported status for linked path")
			}
			if _, err := store.Init(InitOptions{}); err == nil {
				t.Error("initialized through linked path")
			}
			if rel == ".subcli" || strings.HasPrefix(rel, ".subcli/memory") {
				if _, err := store.ReadMain(); err == nil {
					t.Error("read linked memory")
				}
			}
		})
	}
}

func TestImportSkipsLinkedMarkdown(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "entry.md"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("entry.md", filepath.Join(source, "linked.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := ForRoot(t.TempDir(), false)
	res, err := store.ImportMarkdownDir(source, "claude", time.Time{}, InitOptions{})
	if err != nil || len(res.ImportedFiles) != 1 || res.ImportedFiles[0] != "entry.md" {
		t.Fatalf("imported %v, err=%v", res.ImportedFiles, err)
	}
	if _, err := os.Lstat(filepath.Join(store.RepoRoot, ".gitignore")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unexpected .gitignore outside Git: %v", err)
	}
}

func TestManualMemoryRedactedInAllProjections(t *testing.T) {
	store := ForRoot(t.TempDir(), false)
	if _, err := store.Init(InitOptions{}); err != nil {
		t.Fatal(err)
	}
	const example = "sk-ant-example1234567890abcd"
	if err := os.WriteFile(store.MemoryFile, []byte("Example "+example), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, err := store.BuildProviderContext("codex")
	if err != nil {
		t.Fatal(err)
	}
	check := func(name, body string) {
		t.Helper()
		if !strings.Contains(body, "Example ") || strings.Contains(body, example) {
			t.Errorf("%s omitted content or included the raw example key", name)
		}
	}
	check("inline", ctx.InlinePrompt)
	for name, file := range map[string]string{"claude": "CLAUDE.md", "agy": "AGENTS.md"} {
		data, err := os.ReadFile(filepath.Join(store.ProjectionDir(name), file))
		if err != nil {
			t.Fatal(err)
		}
		check(name, string(data))
	}
}

func TestPrivateWriteReplacesExistingFile(t *testing.T) {
	store := ForRoot(t.TempDir(), false)
	if _, err := store.Init(InitOptions{}); err != nil {
		t.Fatal(err)
	}
	before, err := store.statFile(store.MemoryFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePrivateFile(store.MemoryFile, []byte("replaced")); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(store.MemoryFile)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("existing file was modified in place")
	}
	if runtime.GOOS != "windows" && after.Mode().Perm() != 0o600 {
		t.Errorf("replacement permissions = %o, want 600", after.Mode().Perm())
	}
}

func TestEncodeClaudeProjectKey(t *testing.T) {
	if key := EncodeClaudeProjectKey("/repos/my_project.v1/開発"); key != "-repos-my-project-v1---" {
		t.Errorf("unexpected project key %q", key)
	}
}
