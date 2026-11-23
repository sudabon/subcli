package policy_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/agy"
	"github.com/sudabon/subcli/internal/provider/claude"
	"github.com/sudabon/subcli/internal/provider/codex"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod from %s", dir)
		}
		dir = parent
	}
}

// TestNoCredentialAccess verifies that implementation files under cmd/ and internal/
// never reference official CLI credential files, OAuth token helpers, or OS keychain APIs.
// The banned list lives strictly inside this test file.
func TestNoCredentialAccess(t *testing.T) {
	banned := []string{
		".credentials.json",
		"credentials.json",
		".claude.json",
		"auth.json",
		"CLAUDE_CODE_OAUTH_TOKEN",
		"setup-token",
		"SecKeychain",
		"SecItemCopyMatching",
		"go-keyring",
		"zalando/go-keyring",
		"keychain",
		"CredentialManager",
		"antigravity-cli/settings.json",
		"settings.json",
	}

	root := repoRoot(t)
	for _, sub := range []string{"cmd", "internal"} {
		scanRoot := filepath.Join(root, sub)
		if _, err := os.Stat(scanRoot); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(scanRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(data)
			rel, _ := filepath.Rel(root, path)
			for _, token := range banned {
				if strings.Contains(content, token) {
					t.Errorf("forbidden credential/keychain reference %q found in %s", token, rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", scanRoot, err)
		}
	}
}

// TestNetHTTPOnlyInGeminiAPI ensures net/http is never imported outside internal/geminiapi.
func TestNetHTTPOnlyInGeminiAPI(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}} {{join .Deps " "}}`, "./cmd/...", "./internal/...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, string(out))
	}

	const allowedPrefix = "github.com/sudabon/subcli/internal/geminiapi"
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		pkg := fields[0]
		if pkg == allowedPrefix || strings.HasPrefix(pkg, allowedPrefix+"/") {
			continue
		}
		for _, dep := range fields[1:] {
			if dep == "net/http" {
				t.Errorf("package %s depends on net/http, which is only allowed under %s", pkg, allowedPrefix)
			}
		}
	}
}

// TestNoDangerousFlags verifies that provider adapters never emit permission-bypass flags
// and that extra-arg validation rejects every known dangerous flag.
func TestNoDangerousFlags(t *testing.T) {
	providers := []provider.Provider{
		claude.New(""),
		codex.New(""),
		agy.New(""),
	}

	for _, p := range providers {
		if flag, bad := provider.ContainsDangerousFlag(p.LoginArgv()); bad {
			t.Errorf("%s.LoginArgv() contains dangerous flag %q", p.ID(), flag)
		}
		if flag, bad := provider.ContainsDangerousFlag(p.InteractiveArgv([]string{"--model", "test"})); bad {
			t.Errorf("%s.InteractiveArgv() contains dangerous flag %q", p.ID(), flag)
		}
		if ma, ok := p.(provider.MemoryAdapter); ok {
			memArgs := ma.InteractiveMemoryArgs(provider.MemoryContext{
				RepoRoot:      "/repo",
				MemoryDir:     "/repo/.subcli/memory",
				MemoryFile:    "/repo/.subcli/memory/MEMORY.md",
				ProjectionDir: "/repo/.subcli/projections/" + p.ID(),
				InlinePrompt:  "shared repo memory",
			}, nil)
			if flag, bad := provider.ContainsDangerousFlag(memArgs); bad {
				t.Errorf("%s.InteractiveMemoryArgs() contains dangerous flag %q", p.ID(), flag)
			}
		}
		for _, perm := range []provider.Perm{provider.PermReadOnly, provider.PermEdit} {
			argv, _ := p.HeadlessArgv(provider.RunRequest{
				Prompt: "hello",
				Model:  "test",
				Cwd:    ".",
				Perm:   perm,
			})
			if flag, bad := provider.ContainsDangerousFlag(argv); bad {
				t.Errorf("%s.HeadlessArgv(%s) contains dangerous flag %q", p.ID(), perm, flag)
			}
		}
	}

	dangerousInputs := [][]string{
		{"--dangerously-skip-permissions"},
		{"--allow-dangerously-skip-permissions"},
		{"--permission-mode", "bypassPermissions"},
		{"--permission-mode=bypassPermissions"},
		{"--sandbox", "danger-full-access"},
		{"--sandbox=danger-full-access"},
		{"--dangerously-bypass-approvals-and-sandbox"},
		{"--dangerously-bypass-hook-trust"},
	}
	for _, args := range dangerousInputs {
		if err := provider.ValidateExtraArgs(args); err == nil {
			t.Errorf("ValidateExtraArgs(%v) succeeded, want rejection", args)
		}
	}
}
