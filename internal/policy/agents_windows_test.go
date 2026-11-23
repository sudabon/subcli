//go:build windows

package policy

import (
	"errors"
	"os"
	"testing"
)

// Windows does not re-parent orphans, so a real process tree normally ends at
// an exited parent. That must be treated as the root, not as unverifiable.
func TestWindowsAncestryWalkReachesRoot(t *testing.T) {
	names, err := listAncestorProcessNames(int32(os.Getpid()), MaxAncestorDepth)
	if err != nil && !errors.Is(err, errAncestorDepth) {
		t.Fatalf("ancestry walk failed after %q: %v", names, err)
	}
	if len(names) == 0 {
		t.Fatal("ancestry walk returned no parent processes")
	}
}
