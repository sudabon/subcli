package runner

import "testing"

func TestRingBufferExactCapacityOverwrite(t *testing.T) {
	for _, previous := range []string{"", "a", "abcd"} {
		buf := NewRingBuffer(4)
		_, _ = buf.Write([]byte(previous))
		_, _ = buf.Write([]byte("wxyz"))
		if buf.String() != "wxyz" || buf.Truncated() != (previous != "") {
			t.Errorf("after %q: contents=%q, truncated=%v", previous, buf.String(), buf.Truncated())
		}
	}
}
