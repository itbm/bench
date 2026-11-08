package term

import (
	"encoding/base64"
	"strings"
	"testing"
)

// b64 returns s as standard base64, with padding, as programs send title
// and msg.
func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// seqLen returns the length of the whole OSC 7501 sequence with text, ended
// by ST.
func seqLen(text string) int { return len("\x1b]7501;") + len(text) + len(stEnd) }

func TestParseStatus(t *testing.T) {
	st := func(state string) Status { return Status{State: state, Progress: -1} }
	with := func(s Status, f func(*Status)) Status { f(&s); return s }
	seg32 := strings.Repeat("a", 32)
	id128 := seg32 + "/" + seg32 + "/" + seg32 + "/" + strings.Repeat("a", 29)
	// The longest text whose sequence, ended by ST, is 4096 bytes.
	longest := maxStatusSeq - seqLen("")

	tests := []struct {
		name   string
		text   string
		want   Status
		wantID string
		ok     bool
	}{
		// What Claude Code sends.
		{"Claude Code idle", "state=idle:app=claude-code", with(st(StateIdle), func(s *Status) { s.App = "claude-code" }), "", true},
		{"Claude Code working", "state=working:app=claude-code:progress=40",
			with(st(StateWorking), func(s *Status) { s.App = "claude-code"; s.Progress = 40 }), "", true},
		{"Claude Code waiting for permission", "state=blocked:app=claude-code:kind=permission:msg=YXBwcm92ZSBCYXNoOiBucG0gdGVzdA==",
			with(st(StateBlocked), func(s *Status) { s.App = "claude-code"; s.Kind = KindPermission; s.Msg = "approve Bash: npm test" }), "", true},
		{"Claude Code asking a question", "state=blocked:app=claude-code:kind=question:msg=YW5zd2VyOiBXaGljaCBkYXRhYmFzZT8gKFBvc3RncmVzIMK3IFNRTGl0ZSk=",
			with(st(StateBlocked), func(s *Status) {
				s.App = "claude-code"
				s.Kind = KindQuestion
				s.Msg = "answer: Which database? (Postgres · SQLite)"
			}), "", true},
		{"Claude Code done", "state=done:app=claude-code", with(st(StateDone), func(s *Status) { s.App = "claude-code" }), "", true},
		{"Claude Code error", "state=error:app=claude-code:msg=QVBJIEVycm9yOiA1MDAgSW50ZXJuYWwgc2VydmVyIGVycm9y",
			with(st(StateError), func(s *Status) { s.App = "claude-code"; s.Msg = "API Error: 500 Internal server error" }), "", true},
		{"Claude Code subagent", "state=working:app=claude-code:id=a3f9c2:title=RXhwbG9yZSBhdXRoIGZsb3c=",
			with(st(StateWorking), func(s *Status) { s.App = "claude-code"; s.Title = "Explore auth flow" }), "a3f9c2", true},
		{"Claude Code subagent clear", "state=clear:id=a3f9c2", st(StateClear), "a3f9c2", true},
		{"Claude Code clear", "state=clear", st(StateClear), "", true},
		// What Codex sends: a kind, but no words.
		{"Codex waiting to sign in", "state=blocked:app=codex:kind=auth", with(st(StateBlocked), func(s *Status) { s.App = "codex"; s.Kind = KindAuth }), "", true},
		{"Codex 0.163 waiting", "state=blocked:app=codex", with(st(StateBlocked), func(s *Status) { s.App = "codex" }), "", true},
		// The protocol's examples.
		{"spec terraform", "state=blocked:kind=permission:app=terraform:msg=QXBwbHkgMyB0byBhZGQsIDEgdG8gY2hhbmdlLCAwIHRvIGRlc3Ryb3k/",
			with(st(StateBlocked), func(s *Status) {
				s.Kind = KindPermission
				s.App = "terraform"
				s.Msg = "Apply 3 to add, 1 to change, 0 to destroy?"
			}), "", true},
		{"spec several records", "state=working:id=us-east:title=VVMgRWFzdA==:progress=40:msg=UHVzaGluZyBpbWFnZQ==",
			with(st(StateWorking), func(s *Status) { s.Title = "US East"; s.Progress = 40; s.Msg = "Pushing image" }), "us-east", true},

		// Syntax.
		{"no state", "app=claude-code", Status{}, "", false},
		{"nothing", "", Status{}, "", false},
		{"probe", "?", Status{}, "", false},
		{"answer to the probe", statusReply[len("\x1b]7501;"):], Status{}, "", false},
		{"unknown state", "state=sleeping", Status{}, "", false},
		{"ended isn't a state", "state=ended", Status{}, "", false},
		{"state in capitals", "state=Done", Status{}, "", false},
		{"white space taken off", " state = working : progress = 5 ", with(st(StateWorking), func(s *Status) { s.Progress = 5 }), "", true},
		{"unknown key ignored", "state=done:future=1", st(StateDone), "", true},
		{"last value counts", "state=idle:state=done", st(StateDone), "", true},
		{"pair without = skipped", "state=done:junk:app=x", with(st(StateDone), func(s *Status) { s.App = "x" }), "", true},
		{"empty key skipped", "state=done:=x", st(StateDone), "", true},
		{"key in capitals skipped", "state=done:App=x", st(StateDone), "", true},
		{"value with a bad byte skipped", "state=done:state=bo!gus", st(StateDone), "", true},
		{"value with a semicolon skipped", "state=working:msg=abc;def", st(StateWorking), "", true},
		{"value with a space skipped", "state=blocked:app=a b:kind=auth", with(st(StateBlocked), func(s *Status) { s.Kind = KindAuth }), "", true},
		{"key of 16 bytes", "state=done:" + strings.Repeat("k", 16) + "=1", st(StateDone), "", true},
		{"key of 17 bytes", "state=done:" + strings.Repeat("k", 17) + "=1", Status{}, "", false},
		{"key of 17 bytes in capitals", "state=done:" + strings.Repeat("K", 17) + "=1", Status{}, "", false},

		// Keys.
		{"kind only while blocked", "state=working:kind=auth", st(StateWorking), "", true},
		{"unknown kind", "state=blocked:kind=approval", st(StateBlocked), "", true},
		{"progress 0", "state=working:progress=0", with(st(StateWorking), func(s *Status) { s.Progress = 0 }), "", true},
		{"progress 100", "state=blocked:progress=100", with(st(StateBlocked), func(s *Status) { s.Progress = 100 }), "", true},
		{"progress 101", "state=working:progress=101", st(StateWorking), "", true},
		{"progress -1", "state=working:progress=-1", st(StateWorking), "", true},
		{"progress +5", "state=working:progress=+5", st(StateWorking), "", true},
		{"progress 4.5", "state=working:progress=4.5", st(StateWorking), "", true},
		{"progress 0100", "state=working:progress=0100", st(StateWorking), "", true},
		{"progress only while working or blocked", "state=done:progress=40", st(StateDone), "", true},
		{"app of 32 bytes", "state=done:app=" + seg32, with(st(StateDone), func(s *Status) { s.App = seg32 }), "", true},
		{"app of 33 bytes", "state=done:app=" + seg32 + "a", Status{}, "", false},
		{"app with a bad byte", "state=done:app=a,b", st(StateDone), "", true},
		{"app with a bad byte, then a good app", "state=done:app=a,b:app=ok", with(st(StateDone), func(s *Status) { s.App = "ok" }), "", true},
		{"id 8 deep", "state=done:id=a/b/c/d/e/f/g/h", st(StateDone), "a/b/c/d/e/f/g/h", true},
		{"id 9 deep", "state=done:id=a/b/c/d/e/f/g/h/i", Status{}, "", false},
		{"id of 128 bytes", "state=done:id=" + id128, st(StateDone), id128, true},
		{"id of 129 bytes", "state=done:id=" + id128 + "a", Status{}, "", false},
		{"id part of 33 bytes", "state=done:id=" + seg32 + "a", Status{}, "", false},
		{"id with an empty part", "state=done:id=a//b", Status{}, "", false},
		{"id ending in /", "state=done:id=a/", Status{}, "", false},
		{"empty id", "state=done:id=", Status{}, "", false},
		{"id with a comma", "state=done:id=a,b", Status{}, "", false},
		{"bad id, then a good one", "state=done:id=a//b:id=ok", Status{}, "", false},

		// Text.
		{"msg without padding", "state=done:msg=QQ", with(st(StateDone), func(s *Status) { s.Msg = "A" }), "", true},
		{"msg with padding", "state=done:msg=QQ==", with(st(StateDone), func(s *Status) { s.Msg = "A" }), "", true},
		{"msg with half its padding", "state=done:msg=QQ=", Status{}, "", false},
		{"msg with padding inside", "state=done:msg=QQ==QQ", Status{}, "", false},
		{"msg that isn't base64", "state=done:msg=a", Status{}, "", false},
		{"empty msg", "state=done:msg=", st(StateDone), "", true},
		{"msg of UTF-8", "state=done:msg=" + b64("héllo ✓"), with(st(StateDone), func(s *Status) { s.Msg = "héllo ✓" }), "", true},
		{"msg with a new line", "state=done:msg=" + b64("a\nb"), Status{}, "", false},
		{"msg with a tab", "state=done:msg=" + b64("a\tb"), Status{}, "", false},
		{"msg with ESC", "state=done:msg=" + b64("a\x1bb"), Status{}, "", false},
		{"msg with DEL", "state=done:msg=" + b64("a\x7fb"), Status{}, "", false},
		{"msg with a C1 control", "state=done:msg=" + b64("a\u0085b"), Status{}, "", false},
		{"msg that isn't UTF-8", "state=done:msg=" + b64("a\xffb"), Status{}, "", false},
		{"msg of 2048 bytes", "state=done:msg=" + b64(strings.Repeat("a", 2048)), with(st(StateDone), func(s *Status) { s.Msg = strings.Repeat("a", 2048) }), "", true},
		{"msg of 2049 bytes", "state=done:msg=" + strings.Repeat("YWFh", 683), Status{}, "", false},
		{"msg of 2733 bytes of base64", "state=done:msg=" + strings.Repeat("YWFh", 683) + "Y", Status{}, "", false},
		{"bad msg, then a good one", "state=done:msg=a:msg=QQ==", Status{}, "", false},
		{"title of 192 bytes", "state=done:title=" + b64(strings.Repeat("a", 192)), with(st(StateDone), func(s *Status) { s.Title = strings.Repeat("a", 192) }), "", true},
		{"title of 193 bytes", "state=done:title=" + b64(strings.Repeat("a", 193)), Status{}, "", false},
		{"title of 257 bytes of base64", "state=done:title=" + strings.Repeat("YWFh", 64) + "Y", Status{}, "", false},
		{"title with a control character", "state=done:title=" + b64("a\x00"), Status{}, "", false},

		// Invisible characters are taken out of the text.
		{"msg with an override", "state=blocked:msg=" + b64("\u202eevil"), with(st(StateBlocked), func(s *Status) { s.Msg = "evil" }), "", true},
		{"title with isolates", "state=done:title=" + b64("\u2066a\u2069b"), with(st(StateDone), func(s *Status) { s.Title = "ab" }), "", true},
		{"joiners kept", "state=done:msg=" + b64("👩\u200d💻 \u200c"), with(st(StateDone), func(s *Status) { s.Msg = "👩\u200d💻 \u200c" }), "", true},

		// The whole sequence.
		{"sequence of 4096 bytes", "state=done:x=" + strings.Repeat("a", longest-len("state=done:x=")), st(StateDone), "", true},
		{"sequence of 4097 bytes", "state=done:x=" + strings.Repeat("a", longest-len("state=done:x=")+1), Status{}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, id, ok := parseStatus(tt.text, seqLen(tt.text))
			if ok != tt.ok || got != tt.want || id != tt.wantID {
				t.Errorf("parseStatus(%q) = %+v, %q, %v; want %+v, %q, %v", tt.text, got, id, ok, tt.want, tt.wantID, tt.ok)
			}
		})
	}
}

func TestParseStatusCountsTheTerminator(t *testing.T) {
	// The same text fits ended by BEL, but not by ST, which is a byte
	// longer.
	text := "state=done:x=" + strings.Repeat("a", maxStatusSeq-len("\x1b]7501;")-len(belEnd)-len("state=done:x="))
	if _, _, ok := parseStatus(text, len("\x1b]7501;")+len(text)+len(belEnd)); !ok {
		t.Error("a sequence of 4096 bytes ended by BEL was ignored")
	}
	if _, _, ok := parseStatus(text, seqLen(text)); ok {
		t.Error("a sequence of 4097 bytes ended by ST was read")
	}
}

func TestIsProbe(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"?", true},
		{"? ", false},
		{"?:states=idle", false},
		{statusReply[len("\x1b]7501;"):], false},
		{"state=idle", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isProbe(tt.text); got != tt.want {
			t.Errorf("isProbe(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestDisarm(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain words", "plain words"},
		{"\u061cArabic letter mark", "Arabic letter mark"},
		{"marks\u200e\u200f", "marks"},
		{"\u202aembeddings\u202b and overrides\u202c\u202d\u202e", "embeddings and overrides"},
		{"word\u2060joiner and \u2061\u2062\u2063\u2064operators", "wordjoiner and operators"},
		{"\u2066isolates\u2067\u2068\u2069", "isolates"},
		{"\ufeffbyte order mark", "byte order mark"},
		{"joiners\u200c\u200d stay", "joiners\u200c\u200d stay"},
		{"a\u2065b", "a\u2065b"}, // U+2065 has no name, and isn't one of them.
	}
	for _, tt := range tests {
		if got := disarm(tt.in); got != tt.want {
			t.Errorf("disarm(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAnswers(t *testing.T) {
	// Programs read these exact bytes.
	if want := "\x1b]7501;?:states=idle,working,done,blocked,error:kinds=permission,question,auth"; statusReply != want {
		t.Errorf("statusReply = %q, want %q", statusReply, want)
	}
	if want := "\x1b[?1;2c"; da1Reply != want {
		t.Errorf("da1Reply = %q, want %q, as xterm.js answers", da1Reply, want)
	}
}
