package term

import (
	"reflect"
	"strings"
	"testing"
)

type oscCall struct{ code, text string }

// feedAll runs the parser over chunks and returns the calls it makes.
func feedAll(chunks ...string) []oscCall {
	var p oscParser
	var calls []oscCall
	for _, c := range chunks {
		p.Feed([]byte(c), func(code, text string) {
			calls = append(calls, oscCall{code, text})
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
