//go:build unix

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecInteractive_UnixReplacesProcess(t *testing.T) {
	if os.Getenv("SUBCLI_TEST_EXEC_HELPER") == "1" {
		binPath := os.Getenv("SUBCLI_TEST_EXEC_BIN")
		if err := execInteractive(binPath, []string{"fake-cli", "--flag", "value"}, []string{"SUBCLI_EXEC_OK=yes"}); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(2)
		}
		os.Exit(3)
	}

	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "fake-cli")
	script := "#!/bin/sh\necho \"pid=$$ env=$SUBCLI_EXEC_OK args=$*\"\n"
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestExecInteractive_UnixReplacesProcess$")
	cmd.Env = append(os.Environ(),
		"SUBCLI_TEST_EXEC_HELPER=1",
		"SUBCLI_TEST_EXEC_BIN="+fakeBin,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, string(out))
	}
	got := string(out)
	if !strings.Contains(got, "env=yes args=--flag value") {
		t.Errorf("unexpected exec output: %q", got)
	}
}
