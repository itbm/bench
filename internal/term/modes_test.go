package term

import "testing"

func TestModes(t *testing.T) {
	tests := []struct {
		name   string
		output []string // fed in turn
		want   string   // restore's sequences
	}{
		{"a new terminal", nil, ""},
		{"text and other sequences", []string{"hello \x1b[31mred\x1b[0m \x1b[2J\x1b[H\x1b]0;title\x07"}, ""},
		{"a full-screen program starting", []string{"\x1b[?1049h\x1b[?1h\x1b=\x1b[?1000h\x1b[?1006h\x1b[?2004h\x1b[?25l"},
			"\x1b[?1049h\x1b[?1h\x1b=\x1b[?1000h\x1b[?1006h\x1b[?2004h\x1b[?25l"},
		{"several modes in one sequence", []string{"\x1b[?1049;1;2004h"}, "\x1b[?1049h\x1b[?1h\x1b[?2004h"},
		{"set, then reset", []string{"\x1b[?1049h\x1b[?1h\x1b=", "\x1b[?1049l\x1b[?1l\x1b>"}, ""},
		{"split between writes", []string{"\x1b", "[?", "10", "49", "h"}, "\x1b[?1049h"},
		{"the other alternate screens", []string{"\x1b[?47h"}, "\x1b[?1049h"},
		{"the mouse reporting set last counts", []string{"\x1b[?1003h\x1b[?1000h"}, "\x1b[?1000h"},
		{"resetting any mouse reporting turns it off", []string{"\x1b[?1003h\x1b[?1000l"}, ""},
		{"focus reports, no wrapping", []string{"\x1b[?1004h\x1b[?7l"}, "\x1b[?1004h\x1b[?7l"},
		{"a full reset", []string{"\x1b[?1049h\x1b[?2004h\x1bc"}, ""},
		{"a soft reset keeps the screen", []string{"\x1b[?1049h\x1b[?1h\x1b=\x1b[?25l\x1b[?7l\x1b[!p"}, "\x1b[?1049h"},
		{"a soft reset keeps the mouse, but not pasting or focus", []string{"\x1b[?1000h\x1b[?2004h\x1b[?1004h\x1b[!p"}, "\x1b[?1000h"},
		{"the keypad set as a mode", []string{"\x1b[?66h"}, "\x1b="},
		{"the encodings xterm.js ignores", []string{"\x1b[?1006h\x1b[?1015h\x1b[?1005h"}, "\x1b[?1006h"},
		{"resetting the encoding", []string{"\x1b[?1016h\x1b[?1006l"}, ""},
		{"a cancelled sequence", []string{"\x1b[?1049\x18h"}, ""},
		{"a new escape ends one", []string{"\x1b[?10\x1b[?2004h"}, "\x1b[?2004h"},
		{"a request isn't a change", []string{"\x1b[?1049$p"}, ""},
		{"a sequence that isn't private", []string{"\x1b[4h\x1b[20h"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m modes
			for _, p := range tt.output {
				m.feed([]byte(p))
			}
			if got := string(m.restore()); got != tt.want {
				t.Errorf("restore() = %q, want %q", got, tt.want)
			}
		})
	}
}
