package runner

import "sync"

// DefaultStderrCap is the maximum number of trailing stderr bytes retained per run (64KB).
const DefaultStderrCap = 64 * 1024

// RingBuffer retains only the last cap bytes written to it.
type RingBuffer struct {
	mu        sync.Mutex
	buf       []byte
	cap       int
	pos       int
	full      bool
	truncated bool
}

// NewRingBuffer creates a fixed-capacity RingBuffer.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultStderrCap
	}
	return &RingBuffer{
		buf: make([]byte, capacity),
		cap: capacity,
	}
}

// Write implements io.Writer, always succeeding and overwriting oldest bytes when full.
func (r *RingBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(p)
	if n == 0 {
		return 0, nil
	}

	if n >= r.cap {
		copy(r.buf, p[n-r.cap:])
		r.pos = 0
		r.full = true
		if n > r.cap || r.pos > 0 {
			r.truncated = true
		}
		return n, nil
	}

	if r.full || r.pos+n > r.cap {
		r.truncated = true
	}

	first := r.cap - r.pos
	if n <= first {
		copy(r.buf[r.pos:], p)
		r.pos += n
		if r.pos == r.cap {
			r.pos = 0
			r.full = true
		}
	} else {
		copy(r.buf[r.pos:], p[:first])
		copy(r.buf[0:], p[first:])
		r.pos = n - first
		r.full = true
	}
	return n, nil
}

// Bytes returns a copy of the retained bytes in chronological order.
func (r *RingBuffer) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.full {
		out := make([]byte, r.pos)
		copy(out, r.buf[:r.pos])
		return out
	}

	out := make([]byte, r.cap)
	tailLen := r.cap - r.pos
	copy(out, r.buf[r.pos:])
	copy(out[tailLen:], r.buf[:r.pos])
	return out
}

// String returns the retained bytes as a string.
func (r *RingBuffer) String() string {
	return string(r.Bytes())
}

// Truncated reports whether older bytes were overwritten due to capacity limits.
func (r *RingBuffer) Truncated() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.truncated
}
