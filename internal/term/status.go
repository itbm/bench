package term

import (
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file reads the Program Status Protocol (OSC 7501, revision 0.4), with
// which a program, such as Claude Code or Codex, tells the terminal what it's
// doing: ESC ] 7501 ; key=value:key=value ST. A program can ask whether the
// terminal reads it with a probe, ESC ] 7501 ; ? ST, which xterm.js ignores,
// so sessions answer it themselves.

// statusCode is the protocol's OSC number.
const statusCode = "7501"

// Limits from the protocol. A report that breaks one is ignored whole.
const (
	maxStatusSeq       = 4096 // the whole sequence, ESC ] through ST
	maxStatusKey       = 16
	maxStatusMsg       = 2732 // msg, as base64
	maxStatusMsgText   = 2048 // msg, decoded, in bytes
	maxStatusTitle     = 256  // title, as base64
	maxStatusTitleText = 192  // title, decoded, in bytes
	maxStatusApp       = 32
	maxStatusID        = 128 // the whole id
	maxStatusSegment   = 32  // each of an id's parts, and app
	maxStatusDepth     = 8   // the parts in an id
)

// The states a program reports. StateClear isn't a state: it removes what
// the program reported before.
const (
	StateIdle    = "idle"    // at rest, waiting for the user's next instruction
	StateWorking = "working" // running
	StateDone    = "done"    // finished a piece of work, which the user hasn't seen yet
	StateBlocked = "blocked" // can't carry on until the user does something (see Status.Kind)
	StateError   = "error"   // failed and stopped
	StateClear   = "clear"   // forget what the program reported
)

// StateEnded is Bench's own, and no program can report it: the program that
// reported has gone, though the shell it ran in carries on. What it
// reported last goes if it was working, blocked, or idle, and stays if it
// was done or error, as the protocol says.
const StateEnded = "ended"

// The kinds of help a blocked program needs.
const (
	KindPermission = "permission" // approval to do something
	KindQuestion   = "question"   // an answer the user must type
	KindAuth       = "auth"       // a login, token, or other credential
)

// statusReply answers the probe, without its terminator, which is the
// probe's own. It lists every state and kind, which are the protocol's
// defaults. It isn't a probe itself, so a terminal that echoes it back
// can't start a loop.
const statusReply = "\x1b]7501;?:states=idle,working,done,blocked,error:kinds=permission,question,auth"

// da1Reply answers a request for primary device attributes (CSI c) as
// xterm.js 6 does: a VT100 with advanced video. Programs see the same
// terminal whether a view or a session answers.
const da1Reply = "\x1b[?1;2c"

// statusSpace is the white space taken from around a report's keys and
// values.
const statusSpace = " \t\n\v\f\r"

// Status is what a program reports about itself with OSC 7501. It comes
// from the program, which already controls the screen, so none of it is to
// be trusted: App is the name the program gives itself, never a label to
// show for the terminal, and Title and Msg are text to show as it is, never
// markup.
type Status struct {
	// State is one of the State constants.
	State string
	// Kind is what a blocked program needs: KindPermission,
	// KindQuestion, KindAuth, or "" if it didn't say.
	Kind string
	// App is the program's name for itself, such as "claude-code", or
	// "" if it gave none.
	App string
	// Title is the program's label for what it reports, and Msg is one
	// line on what it's doing, waiting for, or has finished. Both are
	// valid UTF-8 without control characters, and the characters that
	// change the direction of text, or hide in it, are taken out.
	Title, Msg string
	// Progress is how far a working or blocked program has got, from 0
	// to 100, or -1 if it didn't say.
	Progress int
}

// parseStatus reads an OSC 7501 report: text is all after "7501;", and seq
// is the length of the whole sequence, ESC ] through ST. It returns the
// report and the id of the record it's about ("" for the program as a
// whole), or false if the report is to be ignored: it has no state, or one
// that isn't known, or it breaks one of the protocol's limits, or its
// base64 doesn't decode, or the text in it has control characters or isn't
// UTF-8, or its id isn't one. Every pair is checked, even those a later
// one replaces, as the protocol asks.
func parseStatus(text string, seq int) (st Status, id string, ok bool) {
	if seq > maxStatusSeq {
		return Status{}, "", false
	}
	var kind, progress, app string
	for pair := range strings.SplitSeq(text, ":") {
		key, value, found := strings.Cut(pair, "=")
		key, value = strings.Trim(key, statusSpace), strings.Trim(value, statusSpace)
		if !found || key == "" {
			continue // A malformed pair is skipped.
		}
		if len(key) > maxStatusKey {
			return Status{}, "", false
		}
		if !allBytes(key, isKeyByte) || !allBytes(value, isValueByte) {
			continue
		}
		good := true
		switch key {
		case "state":
			st.State = value
		case "id":
			id, good = value, validStatusID(value)
		case "kind":
			kind = value
		case "progress":
			progress = value
		case "app":
			app, good = value, len(value) <= maxStatusApp
		case "title":
			st.Title, good = statusText(value, maxStatusTitle, maxStatusTitleText)
		case "msg":
			st.Msg, good = statusText(value, maxStatusMsg, maxStatusMsgText)
		}
		if !good {
			return Status{}, "", false
		}
	}
	if !knownState(st.State) {
		return Status{}, "", false
	}
	if st.State == StateBlocked && knownKind(kind) {
		st.Kind = kind
	}
	st.Progress = -1
	if st.State == StateWorking || st.State == StateBlocked {
		st.Progress = statusProgress(progress)
	}
	if validStatusSegment(app) {
		st.App = app
	}
	st.Title, st.Msg = disarm(st.Title), disarm(st.Msg)
	return st, id, true
}

// isProbe reports whether an OSC 7501 text is the probe, "?". The answer
// starts with "?" too, but has more after it.
func isProbe(text string) bool {
	return text == "?"
}

// knownState reports whether a program can report state: one of the
// protocol's states, or clear.
func knownState(state string) bool {
	switch state {
	case StateIdle, StateWorking, StateDone, StateBlocked, StateError, StateClear:
		return true
	}
	return false
}

// knownKind reports whether kind is one of the protocol's kinds.
func knownKind(kind string) bool {
	switch kind {
	case KindPermission, KindQuestion, KindAuth:
		return true
	}
	return false
}

// statusProgress reads a progress value: a whole number from 0 to 100, or
// -1 for anything else.
func statusProgress(s string) int {
	if s == "" || len(s) > len("100") || !allBytes(s, isDigit) {
		return -1
	}
	n := 0
	for i := range len(s) {
		n = n*10 + int(s[i]-'0')
	}
	if n > 100 {
		return -1
	}
	return n
}

// statusText decodes a title or msg value: standard base64, with or without
// its padding, of at most encoded bytes, which decodes to at most decoded
// bytes of UTF-8 without control characters. The encoded size is checked
// first, so that nothing too long is decoded.
func statusText(value string, encoded, decoded int) (string, bool) {
	if len(value) > encoded {
		return "", false
	}
	enc := base64.StdEncoding
	if len(value)%4 != 0 {
		// Without its padding; any '=' left is out of place.
		enc = base64.RawStdEncoding
	}
	raw, err := enc.DecodeString(value)
	if err != nil || len(raw) > decoded || !utf8.Valid(raw) {
		return "", false
	}
	text := string(raw)
	if strings.ContainsFunc(text, unicode.IsControl) {
		return "", false
	}
	return text, true
}

// validStatusID reports whether id names a record: 1 to 8 parts, each of
// [A-Za-z0-9_.+-]{1,32}, joined by /, and 128 bytes at most.
func validStatusID(id string) bool {
	if id == "" || len(id) > maxStatusID || strings.Count(id, "/") >= maxStatusDepth {
		return false
	}
	for seg := range strings.SplitSeq(id, "/") {
		if !validStatusSegment(seg) {
			return false
		}
	}
	return true
}

// validStatusSegment reports whether s is a name the protocol allows, as for
// app and each part of an id: [A-Za-z0-9_.+-]{1,32}.
func validStatusSegment(s string) bool {
	return s != "" && len(s) <= maxStatusSegment && allBytes(s, isNameByte)
}

// disarm takes out of text from a program the invisible characters that
// change the direction of text, or hide in it, so that it can't make what
// Bench shows read other than as it is: the Arabic letter mark, the
// left-to-right and right-to-left marks, embeddings, overrides, and
// isolates, the word joiner and the invisible operators, and the byte order
// mark. The zero-width joiner and non-joiner (U+200D and U+200C) stay, as
// some scripts and emoji need them.
func disarm(text string) string {
	return strings.Map(func(r rune) rune {
		if invisible(r) {
			return -1
		}
		return r
	}, text)
}

// invisible reports whether disarm takes r out.
func invisible(r rune) bool {
	switch {
	case r == 0x061c, r == 0x200e, r == 0x200f, r == 0xfeff:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2060 && r <= 0x2064, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// allBytes reports whether every byte of s is one that ok accepts.
func allBytes(s string, ok func(byte) bool) bool {
	for i := range len(s) {
		if !ok(s[i]) {
			return false
		}
	}
	return true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isKeyByte reports whether c can be in a key: [a-z].
func isKeyByte(c byte) bool { return c >= 'a' && c <= 'z' }

// isNameByte reports whether c can be in a name: [A-Za-z0-9_.+-].
func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || isDigit(c) || strings.IndexByte("_.+-", c) >= 0
}

// isValueByte reports whether c can be in a value: [A-Za-z0-9_.,+/=-].
func isValueByte(c byte) bool {
	return isNameByte(c) || strings.IndexByte(",/=", c) >= 0
}
