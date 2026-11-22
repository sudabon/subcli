//go:build windows

package render

import (
	"io"
	"os"
	"strconv"
	"strings"
)

// TerminalWidth returns the column width from COLUMNS or DefaultTerminalWidth.
func TerminalWidth(_ io.Writer) int {
	if cols := strings.TrimSpace(os.Getenv("COLUMNS")); cols != "" {
		if n, err := strconv.Atoi(cols); err == nil && n > 0 {
			return n
		}
	}
	return DefaultTerminalWidth
}
