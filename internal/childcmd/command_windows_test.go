package childcmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestWindowsNpmShimLiteralArguments(t *testing.T) {
	if _, err := exec.LookPath("node.exe"); err != nil {
		t.Skip("requires Node.js")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "record.js")
	if err := os.WriteFile(script, []byte("console.log(JSON.stringify(process.argv.slice(2)))"), 0600); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "provider.cmd")
	const contents = "@ECHO off\r\n" +
		"SET \"_prog=node\"\r\n" +
		"endLocal & goto #_undefined_# 2>NUL || title %COMSPEC% & \"%_prog%\"  \"%dp0%\\record.js\" %*\r\n"
	if err := os.WriteFile(shim, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"", "two words", "literal & quoted \"text\"\nsecond line", "%PATH%", "backslash\\"}
	cmd, err := CommandContext(context.Background(), shim, args...)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Dir = t.TempDir()
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, args) {
		t.Fatalf("arguments = %q, want %q", got, args)
	}
}

func TestWindowsUnknownBatchIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.cmd")
	if err := os.WriteFile(path, []byte("@echo off"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CommandContext(context.Background(), path, "hello"); err == nil {
		t.Fatal("expected an unsupported batch launcher error")
	}
}
