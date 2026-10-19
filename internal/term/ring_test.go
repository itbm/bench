package term

import (
	"bytes"
	"strings"
	"testing"
)

func TestRing(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{"empty", 8, nil, ""},
		{"under the limit", 8, []string{"abc", "de"}, "abcde"},
		{"exactly the limit", 8, []string{"abcd", "efgh"}, "abcdefgh"},
		{"wraps", 8, []string{"abcdef", "ghij"}, "cdefghij"},
		{"wraps many times", 4, []string{"ab", "cd", "ef", "gh", "i"}, "fghi"},
		{"one write longer than the limit", 4, []string{"abcdefghij"}, "ghij"},
		{"long write after wrapping", 4, []string{"abc", "defghijk"}, "hijk"},
		{"starts after the first new line once wrapped", 8, []string{"12\n45678", "9"}, "456789"},
		{"keeps the first line when exactly full", 8, []string{"12\n4", "5678"}, "12\n45678"},
		{"one write of exactly the limit keeps it all", 8, []string{"12\n45678"}, "12\n45678"},
		{"empty writes change nothing", 4, []string{"", "ab", ""}, "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRing(tt.limit)
			for _, w := range tt.writes {
				r.Write([]byte(w))
			}
			if got := string(r.Bytes()); got != tt.want {
				t.Errorf("Bytes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRingStartsAtLine(t *testing.T) {
	r := newRing(16)
	r.Write([]byte("0123456789\nabcdefghij\nkl"))
	// The ring keeps the last 16 bytes, "89\nabcdefghij\nkl", and the
	// snapshot leaves out the partial line before the first new line.
	if got, want := string(r.Bytes()), "abcdefghij\nkl"; got != want {
		t.Errorf("Bytes() = %q, want %q", got, want)
	}
}

func TestRingSkipsPartialCharacter(t *testing.T) {
	r := newRing(5)
	// "é" is two bytes; dropping the first byte of "éabcd" leaves a
	// continuation byte at the front, which Bytes leaves out.
	r.Write([]byte("xéabcd"))
	if got, want := string(r.Bytes()), "abcd"; got != want {
		t.Errorf("Bytes() = %q, want %q", got, want)
	}
}

func TestRingBounded(t *testing.T) {
	r := newRing(SnapshotLimit)
	line := strings.Repeat("x", 99) + "\n"
	for range 5000 {
		r.Write([]byte(line))
	}
	got := r.Bytes()
	if len(got) > SnapshotLimit {
		t.Fatalf("len(Bytes()) = %d, want at most %d", len(got), SnapshotLimit)
	}
	if len(got) < SnapshotLimit-len(line) {
		t.Errorf("len(Bytes()) = %d, want nearly %d", len(got), SnapshotLimit)
	}
	if !bytes.HasPrefix(got, []byte(line)) {
		t.Errorf("Bytes() doesn't start at a whole line: %q...", got[:20])
	}
	if r.Len() != SnapshotLimit {
		t.Errorf("Len() = %d, want %d", r.Len(), SnapshotLimit)
	}
}

func TestRingCopies(t *testing.T) {
	r := newRing(8)
	p := []byte("abc")
	r.Write(p)
	p[0] = 'z'
	got := r.Bytes()
	got[1] = 'z'
	if s := string(r.Bytes()); s != "abc" {
		t.Errorf("Bytes() = %q after changing the caller's slices, want %q", s, "abc")
	}
}

func TestRingRestoresModesOfDroppedOutput(t *testing.T) {
	const start = "\x1b[?1049h\x1b[?1h\x1b[?1000h\x1b[?1006h\x1b[?2004h"
	tests := []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{"nothing dropped needs nothing", 64, []string{start, "screen\n"}, start + "screen\n"},
		{"a program's start-up dropped", 16, []string{start, "line one\nline two\n"}, start + "line two\n"},
		{"in one long write", 16, []string{start + "line one\nline two\n"}, start + "line two\n"},
		{"in the partial line left out", 16, []string{"old\n", "\x1b[?2004hab\nline two\n"}, "\x1b[?2004hline two\n"},
		{"undone before the drop", 16, []string{"\x1b[?2004h\x1b[?2004l", "line one\nline two\n"}, "line two\n"},
		{"changed after the drop", 24, []string{"\x1b[?1049h", "xxxxxxxxxxxx\n\x1b[?1049lshell\n"}, "\x1b[?1049h\x1b[?1049lshell\n"},
		{"split by the drop", 16, []string{"\x1b[?20", "04hxxxxx\nline two\n"}, "\x1b[?2004hline two\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRing(tt.limit)
			for _, w := range tt.writes {
				r.Write([]byte(w))
			}
			if got := string(r.Bytes()); got != tt.want {
				t.Errorf("Bytes() = %q, want %q", got, tt.want)
			}
		})
	}
}
