package term

import (
	"bytes"
	"unicode/utf8"
)

// lineSearch is how far into a wrapped snapshot to look for the end of the
// first, partial, line.
const lineSearch = 4096

// ring keeps the most recent output of a session, up to a limit. It isn't
// safe for concurrent use; sessions guard it with their own mutex.
type ring struct {
	limit   int
	buf     []byte // grows up to limit, then wraps
	pos     int    // where the next byte goes, once full
	full    bool   // buf has reached limit
	dropped bool   // some output has been dropped to make room; implies full
}

// newRing returns a ring that keeps up to limit bytes.
func newRing(limit int) ring {
	return ring{limit: max(limit, 1)}
}

// Write appends p, dropping the oldest output beyond the limit.
func (r *ring) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	if len(p) > r.limit {
		p = p[len(p)-r.limit:]
		r.dropped = true
	}
	if !r.full {
		room := r.limit - len(r.buf)
		if len(p) < room {
			r.buf = append(r.buf, p...)
			return
		}
		r.buf = append(r.buf, p[:room]...)
		p = p[room:]
		r.full = true
		r.pos = 0
	}
	if len(p) > 0 {
		r.dropped = true
	}
	for len(p) > 0 {
		n := copy(r.buf[r.pos:r.limit], p)
		r.pos = (r.pos + n) % r.limit
		p = p[n:]
	}
}

// Len returns how many bytes the ring holds.
func (r *ring) Len() int {
	if r.full {
		return r.limit
	}
	return len(r.buf)
}

// Bytes returns a copy of what the ring holds, oldest first. Once output has
// been dropped, the copy starts at the beginning of a line if one starts near
// the front, and never in the middle of a UTF-8 character, so that a terminal
// replaying it doesn't show a fragment of an escape sequence.
func (r *ring) Bytes() []byte {
	if !r.dropped {
		// Nothing has wrapped yet, so buf is in order.
		return bytes.Clone(r.buf)
	}
	out := make([]byte, 0, r.limit)
	out = append(out, r.buf[r.pos:r.limit]...)
	out = append(out, r.buf[:r.pos]...)
	if i := bytes.IndexByte(out[:min(len(out), lineSearch)], '\n'); i >= 0 {
		return out[i+1:]
	}
	i := 0
	for i < len(out) && i < utf8.UTFMax && !utf8.RuneStart(out[i]) {
		i++
	}
	return out[i:]
}
