//go:build unix

package cli

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
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
	cmd.Env = append(fakeCLIEnv(),
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

func TestRunForegroundSurvivesInterrupt(t *testing.T) {
	if os.Getenv("SUBCLI_TEST_FOREGROUND_HELPER") == "1" {
		// The child interrupts subcli the way a terminal Ctrl+C would.
		code, err := runForeground(exec.Command("/bin/sh", "-c", "kill -INT $PPID; sleep 0.2; exit 7"))
		if err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(2)
		}
		os.Stdout.WriteString("code=" + strconv.Itoa(code) + "\n")
		os.Exit(0)
	}

	if signal.Ignored(os.Interrupt) {
		t.Skip("SIGINT is already ignored, so the helper would survive without runForeground")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunForegroundSurvivesInterrupt$")
	cmd.Env = append(fakeCLIEnv(), "SUBCLI_TEST_FOREGROUND_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper did not survive SIGINT: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "code=7") {
		t.Errorf("helper output = %q, want child exit code 7", out)
	}
}
