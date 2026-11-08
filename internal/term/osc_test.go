package term

import (
	"reflect"
	"strings"
	"testing"
)

type oscCall struct{ code, text string }

// bellCall, da1Call, and resetCall stand for a bell, a request for primary
// device attributes, and a full reset among the calls feedAll returns.
var (
	bellCall  = oscCall{code: "BEL"}
	da1Call   = oscCall{code: "DA1"}
	resetCall = oscCall{code: "RIS"}
)

// feedAll runs the parser over chunks and returns the calls it makes, with
// bellCall for each bell, da1Call for each request for primary device
// attributes, and resetCall for each full reset.
func feedAll(chunks ...string) []oscCall {
	var p oscParser
	var calls []oscCall
	add := func(c oscCall) func() { return func() { calls = append(calls, c) } }
	for _, c := range chunks {
		p.Feed([]byte(c), oscHooks{
			osc: func(code, text string, _ bool) {
				calls = append(calls, oscCall{code, text})
			},
			bell:  add(bellCall),
			da1:   add(da1Call),
			reset: add(resetCall),
		})
	}
	return calls
}

// feedEnds runs the parser over chunks and returns what ended each OSC
// sequence it finds: belEnd or stEnd.
func feedEnds(chunks ...string) []string {
	var p oscParser
	var ends []string
	for _, c := range chunks {
		p.Feed([]byte(c), oscHooks{
			osc:   func(_, _ string, bel bool) { ends = append(ends, oscEnd(bel)) },
			bell:  func() {},
			da1:   func() {},
			reset: func() {},
		})
	}
	return ends
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
		{"program status", "\x1b]7501;state=working:app=claude-code\x07", []oscCall{{"7501", "state=working:app=claude-code"}}},
		{"device attributes", "\x1b[c", []oscCall{da1Call}},
		{"device attributes with 0", "\x1b[0c", []oscCall{da1Call}},
		{"device attributes with 00", "\x1b[00c", []oscCall{da1Call}},
		{"device attributes with more parameters", "\x1b[0;1c", []oscCall{da1Call}},
		{"device attributes with 1", "\x1b[1c", nil},
		{"device attributes with 10", "\x1b[10c", nil},
		{"secondary device attributes", "\x1b[>c", nil},
		{"tertiary device attributes", "\x1b[=c", nil},
		{"answer to device attributes", "\x1b[?1;2c", nil},
		{"intermediate", "\x1b[ c", nil},
		{"prefix after a parameter", "\x1b[0?c", nil},
		{"other control sequences", "\x1b[31mc\x1b[2Jc", nil},
		{"bell inside device attributes", "\x1b[\x07c", []oscCall{bellCall, da1Call}},
		{"other control character inside device attributes", "\x1b[\x0d0c", []oscCall{da1Call}},
		{"control sequence cancelled by CAN", "\x1b[\x18c", nil},
		{"control sequence abandoned by another escape", "\x1b[1\x1b[c", []oscCall{da1Call}},
		{"control sequence ended by a character past ASCII", "\x1b[éc", nil},
		{"device attributes after an OSC", "\x1b]0;\x1b[c\x07", []oscCall{da1Call, bellCall}},
		{"device attributes after DCS", "\x1bPq\x1b[c", []oscCall{da1Call}},
		{"probes Claude Code sends", "\x1b[>0q\x1b[?u\x1b]7501;?\x07\x1b[c", []oscCall{{"7501", "?"}, da1Call}},
		{"full reset", "a\x1bcb", []oscCall{resetCall}},
		{"full reset abandons an OSC", "\x1b]0;lost\x1bc", []oscCall{resetCall}},
		{"character set isn't a full reset", "\x1b(c", nil},
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

func TestOSCParserTerminators(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"BEL", "\x1b]7501;?\x07", []string{belEnd}},
		{"ST", "\x1b]7501;?\x1b\\", []string{stEnd}},
		{"both", "\x1b]0;a\x07b\x1b]2;c\x1b\\", []string{belEnd, stEnd}},
		{"abandoned", "\x1b]0;a\x1b[m\x1b]0;b\x1b\\", []string{stEnd}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < len(tt.in); i++ {
				if got := feedEnds(tt.in[:i], tt.in[i:]); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("split at %d: ends = %q, want %q", i, got, tt.want)
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
