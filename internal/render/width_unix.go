//go:build !windows

package render

import (
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// TerminalWidth returns the column width of w if it is a TTY, falling back to
// the COLUMNS environment variable and finally DefaultTerminalWidth.
func TerminalWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok && f != nil {
		if ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ); err == nil && ws.Col > 0 {
			return int(ws.Col)
		}
	}
	if cols := strings.TrimSpace(os.Getenv("COLUMNS")); cols != "" {
		if n, err := strconv.Atoi(cols); err == nil && n > 0 {
			return n
		}
	}
	return DefaultTerminalWidth
}
