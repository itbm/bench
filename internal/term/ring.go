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
	// lost follows the modes set by the output that has been dropped, so
	// that Bytes can set them again.
	lost modes
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
	if len(p) >= r.limit {
		// Everything the ring holds goes, oldest first, then the front of
		// p if it's longer than the limit.
		older, newer := r.ordered()
		r.drop(older)
		r.drop(newer)
		r.drop(p[:len(p)-r.limit])
		r.buf = append(r.buf[:0], p[len(p)-r.limit:]...)
		r.pos, r.full = 0, true
		return
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
	for len(p) > 0 {
		end := min(r.pos+len(p), r.limit)
		r.drop(r.buf[r.pos:end])
		n := copy(r.buf[r.pos:end], p)
		r.pos = end % r.limit
		p = p[n:]
	}
}

// ordered returns what the ring holds, oldest first, in two parts.
func (r *ring) ordered() (older, newer []byte) {
	if !r.full {
		return r.buf, nil
	}
	return r.buf[r.pos:], r.buf[:r.pos]
}

// drop notes output that's about to be dropped.
func (r *ring) drop(p []byte) {
	if len(p) == 0 {
		return
	}
	r.dropped = true
	r.lost.feed(p)
}

// Clear drops everything the ring holds, as clearing a terminal drops its
// scrollback, but follows the modes that it set, so that Bytes still sets
// them. What's written next starts the ring again, at the start of a line.
func (r *ring) Clear() {
	older, newer := r.ordered()
	r.lost.feed(older)
	r.lost.feed(newer)
	r.buf, r.pos, r.full, r.dropped = r.buf[:0], 0, false, false
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
// replaying it doesn't show a fragment of an escape sequence. It then starts
// with the sequences that set the terminal modes that the dropped output
// set, such as the alternate screen or application cursor keys, so that a
// terminal replaying it ends up in the modes the program left it in. So does
// a copy after Clear, for the output cleared.
func (r *ring) Bytes() []byte {
	if !r.dropped {
		// Nothing has wrapped since the ring started, or was cleared, so
		// buf is in order, after the modes that cleared output set.
		return append(r.lost.restore(), r.buf...)
	}
	out := make([]byte, 0, r.limit)
	older, newer := r.ordered()
	out = append(out, older...)
	out = append(out, newer...)
	start := 0
	if i := bytes.IndexByte(out[:min(len(out), lineSearch)], '\n'); i >= 0 {
		start = i + 1
	} else {
		for start < len(out) && start < utf8.UTFMax && !utf8.RuneStart(out[start]) {
			start++
		}
	}
	// The part left out counts as dropped too. lost is a copy, so the
	// ring's own state doesn't change.
	lost := r.lost
	lost.feed(out[:start])
	prefix := lost.restore()
	if len(prefix) == 0 {
		return out[start:]
	}
	return append(prefix, out[start:]...)
}
