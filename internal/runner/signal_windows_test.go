package runner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The helper intentionally exits while its own child still holds stdout.
func TestWindowsJobHelper(t *testing.T) {
	switch os.Getenv("SUBCLI_TEST_WINDOWS_HELPER") {
	case "child":
		cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsJobHelper$")
		cmd.Env = append(os.Environ(), "SUBCLI_TEST_WINDOWS_HELPER=grandchild")
		cmd.Stdout = os.Stdout
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		fmt.Println(cmd.Process.Pid)
		os.Exit(0)
	case "grandchild":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}

func TestWindowsJobTerminatesDescendant(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsJobHelper$")
	cmd.Env = append(os.Environ(), "SUBCLI_TEST_WINDOWS_HELPER=child")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	group, err := startProcessGroup(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer group.close()
	defer cmd.Wait()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	// Hold the handle so we can independently confirm the descendant exited.
	descendant, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(descendant)
	group.terminate(make(chan struct{}), 20*time.Millisecond)
	status, err := windows.WaitForSingleObject(descendant, 3000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant still running: status %d, error %v", status, err)
	}
}
