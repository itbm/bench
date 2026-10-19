package term

import (
	"bytes"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Control characters that matter to the OSC parser.
const (
	bel = 0x07
	esc = 0x1b
	can = 0x18
	sub = 0x1a
)

// maxOSC is the longest OSC payload kept. Longer ones are skipped.
const maxOSC = 4096

// maxTitle is the longest title kept, in runes.
const maxTitle = 256

// OSC parser states.
const (
	oscGround    = iota // ordinary output
	oscEscape           // just after ESC
	oscString           // inside ESC ] ... (the payload)
	oscStringEsc        // ESC inside the payload: maybe ESC \
)

// oscParser finds operating system commands (ESC ] Ps ; Pt, ended by BEL or
// ESC \) in terminal output, even when they're split across reads. It keeps
// only what it needs: everything else passes by untouched. It isn't safe for
// concurrent use.
type oscParser struct {
	state    int
	buf      []byte
	overflow bool
}

// Feed scans p and calls fn with the code (Ps) and text (Pt) of each complete
// OSC sequence, in order.
func (o *oscParser) Feed(p []byte, fn func(code, text string)) {
	for len(p) > 0 {
		switch o.state {
		case oscGround:
			i := bytes.IndexByte(p, esc)
			if i < 0 {
				return
			}
			p = p[i+1:]
			o.state = oscEscape
		case oscEscape:
			c := p[0]
			p = p[1:]
			switch c {
			case ']':
				o.begin()
			case esc:
				// Still just after an ESC.
			default:
				o.state = oscGround
			}
		case oscString:
			i := bytes.IndexAny(p, "\a\x1b\x18\x1a")
			if i < 0 {
				o.add(p)
				return
			}
			o.add(p[:i])
			c := p[i]
			p = p[i+1:]
			switch c {
			case bel:
				o.end(fn)
			case esc:
				o.state = oscStringEsc
			default: // CAN or SUB cancels the sequence.
				o.state = oscGround
			}
		case oscStringEsc:
			if p[0] == '\\' {
				p = p[1:]
				o.end(fn)
				continue
			}
			// Any other ESC sequence abandons the OSC and starts afresh,
			// as xterm does; the byte is read again as following ESC.
			o.state = oscEscape
		}
	}
}

func (o *oscParser) begin() {
	o.state = oscString
	o.buf = o.buf[:0]
	o.overflow = false
}

func (o *oscParser) add(p []byte) {
	if o.overflow {
		return
	}
	if len(o.buf)+len(p) > maxOSC {
		o.overflow = true
		o.buf = o.buf[:0]
		return
	}
	o.buf = append(o.buf, p...)
}

func (o *oscParser) end(fn func(code, text string)) {
	o.state = oscGround
	if o.overflow {
		return
	}
	code, text, _ := strings.Cut(string(o.buf), ";")
	fn(code, text)
}

// cleanTitle makes a title from the session, such as an OSC 0 or 2 payload,
// safe to show: valid UTF-8, no control characters, trimmed, and not too
// long.
func cleanTitle(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxTitle {
		s = string([]rune(s)[:maxTitle])
	}
	return s
}

// parseFileURL returns the folder in an OSC 7 payload, such as
// "file://host/Users/me/code". The host isn't checked: the session is local.
func parseFileURL(s string) (string, bool) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "file" || !filepath.IsAbs(u.Path) {
		return "", false
	}
	return filepath.Clean(u.Path), true
}
