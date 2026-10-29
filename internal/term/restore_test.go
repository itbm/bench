package term

import (
	"strings"
	"testing"
)

func TestRestorable(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"text", "make\r\nok\r\n", "make\r\nok\r\n"},
		{"colours", "\x1b[31mred\x1b[0m \x1b[38;5;208;1mbold\x1b[m", "\x1b[31mred\x1b[0m \x1b[38;5;208;1mbold\x1b[m"},
		{"colours with colons", "\x1b[38:2::1:2:3mx", "\x1b[38:2::1:2:3mx"},
		{"cursor and erasing", "a\x1b[2A\x1b[K\x1b[3;4H\x1b[2J\x1b[1Pb", "a\x1b[2A\x1b[K\x1b[3;4H\x1b[2J\x1b[1Pb"},
		{"tabs, backspace, and carriage return", "a\tb\bc\r50%\r100%", "a\tb\bc\r50%\r100%"},
		{"save and restore the cursor", "\x1b7x\x1b8", "\x1b7x\x1b8"},
		{"line drawing", "\x1b(0qqq\x1b(B", "\x1b(0qqq\x1b(B"},
		{"bell and ENQ", "done\x07\x05!", "done!"},
		{"device attributes", "a\x1b[c\x1b[>c\x1b[=cb", "ab"},
		{"cursor position report", "a\x1b[6n\x1b[5n\x1b[?6nb", "ab"},
		{"window size", "a\x1b[14t\x1b[18tb", "ab"},
		{"terminal version", "a\x1b[>0qb", "ab"},
		{"mode report", "a\x1b[?2004$pb", "ab"},
		{"keyboard protocol", "a\x1b[?u\x1b[>1ub", "ab"},
		{"ESC Z", "a\x1bZb", "ab"},
		{"titles", "a\x1b]0;title\x07\x1b]2;t\x1b\\b", "ab"},
		{"colour queries", "a\x1b]11;?\x07\x1b]4;1;?\x1b\\b", "ab"},
		{"clipboard", "a\x1b]52;c;c2VjcmV0\x07b", "ab"},
		{"links", "\x1b]8;;https://example.com\x07link\x1b]8;;\x07", "link"},
		{"DCS queries", "a\x1bP$qm\x1b\\\x1bP+q544e\x1b\\b", "ab"},
		{"APC", "a\x1b_Gi=1,a=q;AAAA\x1b\\b", "ab"},
		{"OSC cancelled", "a\x1b]0;x\x18b", "ab"},
		{"modes", "a\x1b[?1000h\x1b[?1006h\x1b[?2004h\x1b[?1h\x1b=\x1b[?25l\x1b[4hb", "ab"},
		{"scrolling region", "a\x1b[1;10rb", "ab"},
		{"tab stops", "a\x1b[3gb", "ab"},
		{"full reset", "a\x1bcb", "ab"},
		{"alternate screen", "$ vim\r\n\x1b[?1049h\x1b[Hfile\x1b[?1049l$ ", "$ vim\r\n$ "},
		{"alternate screen at the end", "$ vim\r\n\x1b[?1049h\x1b[Hfile", "$ vim\r\n"},
		{"old alternate screen", "a\x1b[?47hx\x1b[?47lb\x1b[?1047hy\x1b[?1047lc", "abc"},
		{"reset leaves the alternate screen", "\x1b[?1049hx\x1bcb", "b"},
		{"C1 sequences", "a\u009b6n\u009d0;t\u0007\u009bm\u009cb", "a\x1b[mb"},
		{"C1 string ended by C1 ST", "a\u0090$qm\u009cb", "ab"},
		{"not UTF-8", "a\xffb", "ab"},
		{"unfinished", "a\x1b[31", "a"},
		{"ESC at the end", "a\x1b", "a"},
		{"broken off by a line feed", "a\x1b[3\nb", "a\nb"},
		{"too long", "a\x1b[" + strings.Repeat("1;", maxSequence) + "mb", "ab"},
	}
	for _, tt := range tests {
		if got := string(Restorable([]byte(tt.in), SnapshotLimit)); got != tt.want {
			t.Errorf("%s: Restorable(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestRestorableLimit(t *testing.T) {
	in := strings.Repeat("line of output\r\n", 100)
	got := string(Restorable([]byte(in), 100))
	if len(got) > 100 || !strings.HasPrefix(got, "line of output\r\n") || !strings.HasSuffix(in, got) {
		t.Errorf("Restorable(100 lines, 100) = %q, want the last whole lines, at most 100 bytes", got)
	}
	long := "é" + strings.Repeat("é", 100)
	got = string(Restorable([]byte(long), 51))
	if !strings.HasPrefix(got, "é") || !strings.HasSuffix(long, got) {
		t.Errorf("Restorable(one long line, 51) = %q, want it to start with a whole character", got)
	}
}

func TestFaint(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "\x1b[0;2m\x1b[0m\x1b(B\x0f"},
		{"text\r\n", "\x1b[0;2mtext\r\n\x1b[0m\x1b(B\x0f"},
		{"a\x1b[31mb\x1b[0mc", "\x1b[0;2ma\x1b[31m\x1b[2mb\x1b[0m\x1b[2mc\x1b[0m\x1b(B\x0f"},
		{"a\x1b[2Ab", "\x1b[0;2ma\x1b[2Ab\x1b[0m\x1b(B\x0f"},
		{"a\x1b(0q", "\x1b[0;2ma\x1b(0q\x1b[0m\x1b(B\x0f"},
	}
	for _, tt := range tests {
		if got := string(Faint([]byte(tt.in))); got != tt.want {
			t.Errorf("Faint(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
