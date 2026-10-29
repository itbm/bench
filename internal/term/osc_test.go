package term

import (
	"reflect"
	"strings"
	"testing"
)

type oscCall struct{ code, text string }

// bellCall stands for a bell among the calls feedAll returns.
var bellCall = oscCall{code: "BEL"}

// feedAll runs the parser over chunks and returns the calls it makes, with
// bellCall for each bell.
func feedAll(chunks ...string) []oscCall {
	var p oscParser
	var calls []oscCall
	for _, c := range chunks {
		p.Feed([]byte(c), func(code, text string) {
			calls = append(calls, oscCall{code, text})
		}, func() {
			calls = append(calls, bellCall)
		})
	}
	return calls
}

func TestOSCParser(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []oscCall
	}{
		{"none", "plain output\r\n\x1b[31mred\x1b[0m", nil},
		{"title ended by BEL", "\x1b]0;my title\x07", []oscCall{{"0", "my title"}}},
		{"title ended by ST", "\x1b]2;my title\x1b\\", []oscCall{{"2", "my title"}}},
		{"folder", "\x1b]7;file://mac.local/Users/me/code\x07", []oscCall{{"7", "file://mac.local/Users/me/code"}}},
		{"among other output", "a\x1b[1mb\x1b]0;t1\x07c\x1b]2;t2\x1b\\d", []oscCall{{"0", "t1"}, {"2", "t2"}}},
		{"no text", "\x1b]0\x07", []oscCall{{"0", ""}}},
		{"empty title", "\x1b]0;\x07", []oscCall{{"0", ""}}},
		{"text with semicolons", "\x1b]0;a;b\x07", []oscCall{{"0", "a;b"}}},
		{"cancelled by CAN", "\x1b]0;lost\x18\x1b]0;kept\x07", []oscCall{{"0", "kept"}}},
		{"abandoned by another escape", "\x1b]0;lost\x1b[0m\x1b]0;kept\x07", []oscCall{{"0", "kept"}}},
		{"ESC ESC ]", "\x1b\x1b]0;t\x07", []oscCall{{"0", "t"}}},
		{"too long", "\x1b]0;" + strings.Repeat("x", maxOSC+1) + "\x07\x1b]0;short\x07", []oscCall{{"0", "short"}}},
		{"bell", "done\x07", []oscCall{bellCall}},
		{"bells around a title", "\x07\x1b]0;t\x07\x07", []oscCall{bellCall, {"0", "t"}, bellCall}},
		{"bell inside a CSI sequence", "\x1b[\x07m", []oscCall{bellCall}},
		{"bell after ESC", "\x1b\x07", []oscCall{bellCall}},
		{"notification", "\x1b]9;Claude needs your permission\x07", []oscCall{{"9", "Claude needs your permission"}}},
		{"rxvt notification", "\x1b]777;notify;Codex;Done\x1b\\", []oscCall{{"777", "notify;Codex;Done"}}},
		{"no bell inside DCS", "\x1bP+q\x07544e\x1b\\\x07", []oscCall{bellCall}},
		{"no bell inside APC", "\x1b_G\x07a=q\x1b\\", nil},
		{"DCS cancelled by CAN", "\x1bPq\x18\x07", []oscCall{bellCall}},
		{"OSC after DCS", "\x1bPq\x1b\\\x1b]0;t\x07", []oscCall{{"0", "t"}}},
		{"tmux passthrough", "\x1bPtmux;\x1b\x1b]9;hi\x07\x1b\\", []oscCall{{"9", "hi"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := feedAll(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Feed(%q) calls = %q, want %q", tt.in, got, tt.want)
			}
			// The same result however the input is split across reads.
			for i := 1; i < len(tt.in) && len(tt.in) < 200; i++ {
				if got := feedAll(tt.in[:i], tt.in[i:]); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("split at %d: calls = %q, want %q", i, got, tt.want)
				}
			}
		})
	}
}

func TestOSCParserByteAtATime(t *testing.T) {
	in := "x\x1b]0;one\x1b\\y\x1b]2;two\x07z"
	chunks := strings.Split(in, "")
	got := feedAll(chunks...)
	want := []oscCall{{"0", "one"}, {"2", "two"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"my title", "my title"},
		{"  padded  ", "padded"},
		{"tab\there", "tabhere"},
		{"bad\xffbyte", "badbyte"},
		{"vim — notes.md", "vim — notes.md"},
		{strings.Repeat("é", maxTitle+10), strings.Repeat("é", maxTitle)},
	}
	for _, tt := range tests {
		if got := cleanTitle(tt.in); got != tt.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseFileURL(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"file://mac.local/Users/me/code", "/Users/me/code", true},
		{"file:///Users/me/code/", "/Users/me/code", true},
		{"file://host/Users/me/my%20project", "/Users/me/my project", true},
		{"file://host", "", false},
		{"http://host/Users/me", "", false},
		{"not a url %zz", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := parseFileURL(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("parseFileURL(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestNotification(t *testing.T) {
	tests := []struct {
		name, code, text string
		want             Alert
		wantOK           bool
	}{
		{"OSC 9", "9", "Claude needs your permission", Alert{Body: "Claude needs your permission"}, true},
		{"OSC 9 with semicolons", "9", "Done; 3 tests failed", Alert{Body: "Done; 3 tests failed"}, true},
		{"OSC 9 starting with a number", "9", "4 tests failed", Alert{Body: "4 tests failed"}, true},
		{"ConEmu progress", "9", "4;1;50", Alert{}, false},
		{"ConEmu command", "9", "9;C:\\code", Alert{}, false},
		{"empty OSC 9", "9", "  ", Alert{}, false},
		{"OSC 777", "777", "notify;Codex;Turn complete", Alert{Title: "Codex", Body: "Turn complete"}, true},
		{"OSC 777 title only", "777", "notify;Codex", Alert{Title: "Codex"}, true},
		{"OSC 777 body with semicolons", "777", "notify;Build;a;b", Alert{Title: "Build", Body: "a;b"}, true},
		{"OSC 777 other command", "777", "preexec", Alert{}, false},
		{"control characters", "9", "a\tb\x1bc", Alert{Body: "abc"}, true},
		{"too long", "9", strings.Repeat("x", maxNote+5), Alert{Body: strings.Repeat("x", maxNote)}, true},
		{"a title", "0", "zsh", Alert{}, false},
	}
	for _, tt := range tests {
		got, ok := notification(tt.code, tt.text)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("%s: notification(%q, %q) = %+v, %v; want %+v, %v", tt.name, tt.code, tt.text, got, ok, tt.want, tt.wantOK)
		}
	}
}
