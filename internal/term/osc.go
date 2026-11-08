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
	del = 0x7f
)

// The two ways to end an OSC sequence: BEL, as xterm first had it, or ST
// (ESC \).
const (
	belEnd = "\a"
	stEnd  = "\x1b\\"
)

// oscEnd returns what ended an OSC sequence: BEL if bel is set, or ST.
func oscEnd(bel bool) string {
	if bel {
		return belEnd
	}
	return stEnd
}

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
	oscOther            // inside another string (DCS, SOS, PM, or APC), which only ESC \ ends
	oscOtherEsc         // ESC inside another string: maybe ESC \
	oscCSI              // inside a control sequence (ESC [ ...), up to its final byte
)

// oscHooks are what oscParser.Feed calls, in order, as it finds things in
// output. None may be nil.
type oscHooks struct {
	// osc is called with the code (Ps) and text (Pt) of each complete OSC
	// sequence. bel is set if BEL ended it, rather than ST.
	osc func(code, text string, bel bool)
	// bell is called for each BEL that rings the bell.
	bell func()
	// da1 is called for each request for primary device attributes, which
	// xterm.js answers: CSI c, or CSI 0 c, without a prefix, such as the >
	// of DA2's CSI > c, or intermediates.
	da1 func()
	// reset is called for each full reset (RIS, ESC c).
	reset func()
}

// oscParser finds operating system commands (ESC ] Ps ; Pt, ended by BEL or
// ESC \) in terminal output, the bell (BEL) outside them, requests for
// device attributes, and full resets, even when they're split across reads.
// It keeps only what it needs: everything else passes by untouched. It isn't
// safe for concurrent use.
type oscParser struct {
	state    int
	buf      []byte
	overflow bool
	// csiDA1 is set while the control sequence being read can still be a
	// request for primary device attributes, and csiRest once its first
	// parameter has ended.
	csiDA1  bool
	csiRest bool
}

// Feed scans p and calls h's hooks for what it finds, in order: each
// complete OSC sequence, each BEL that rings the bell, each request for
// primary device attributes, and each full reset. A BEL that ends an OSC
// sequence doesn't ring the bell, nor does one inside the other strings a
// program can send (DCS, SOS, PM, and APC), as in xterm.
func (o *oscParser) Feed(p []byte, h oscHooks) {
	for len(p) > 0 {
		switch o.state {
		case oscGround:
			i := bytes.IndexAny(p, "\a\x1b")
			if i < 0 {
				return
			}
			c := p[i]
			p = p[i+1:]
			if c == bel {
				h.bell()
				continue
			}
			o.state = oscEscape
		case oscEscape:
			c := p[0]
			p = p[1:]
			switch c {
			case ']':
				o.begin()
			case '[':
				o.state, o.csiDA1, o.csiRest = oscCSI, true, false
			case 'c':
				o.state = oscGround
				h.reset()
			case 'P', 'X', '^', '_':
				o.state = oscOther
			case esc:
				// Still just after an ESC.
			case bel:
				// A control character inside an escape sequence acts, and
				// the sequence goes on.
				h.bell()
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
				o.end(h, true)
			case esc:
				o.state = oscStringEsc
			default: // CAN or SUB cancels the sequence.
				o.state = oscGround
			}
		case oscStringEsc:
			if p[0] == '\\' {
				p = p[1:]
				o.end(h, false)
				continue
			}
			// Any other ESC sequence abandons the OSC and starts afresh,
			// as xterm does; the byte is read again as following ESC.
			o.state = oscEscape
		case oscOther:
			i := bytes.IndexAny(p, "\x1b\x18\x1a")
			if i < 0 {
				return
			}
			c := p[i]
			p = p[i+1:]
			if c == esc {
				o.state = oscOtherEsc
			} else {
				o.state = oscGround
			}
		case oscOtherEsc:
			if p[0] == '\\' {
				p = p[1:]
				o.state = oscGround
				continue
			}
			o.state = oscEscape
		case oscCSI:
			p = o.csiParams(p)
			if len(p) > 0 {
				o.csi(p[0], h)
				p = p[1:]
			}
		}
	}
}

// csiParams reads the parameters at the start of p, which are most of a
// control sequence, such as the 38;5;208 of ESC [ 38;5;208 m, and returns
// the rest.
func (o *oscParser) csiParams(p []byte) []byte {
	for i, c := range p {
		switch {
		case c == ';' || c == ':':
			o.csiRest = true
		case c >= '1' && c <= '9':
			if !o.csiRest {
				o.csiDA1 = false
			}
		case c != '0':
			return p[i:]
		}
	}
	return nil
}

// csi reads one byte of a control sequence (ESC [ ...), other than its
// parameters, as xterm.js does, to find requests for primary device
// attributes.
func (o *oscParser) csi(c byte, h oscHooks) {
	switch {
	case c == esc:
		o.state = oscEscape
	case c == can || c == sub:
		o.state = oscGround
	case c == bel:
		// A control character inside a control sequence acts, and the
		// sequence goes on.
		h.bell()
	case c < 0x20 || c == del:
		// Other control characters act or are ignored, but don't matter
		// here.
	case c >= 0x80:
		// xterm.js reads characters, not bytes, and one past ASCII ends
		// the sequence unfinished.
		o.state = oscGround
	case c >= 0x40:
		// The final byte.
		o.state = oscGround
		if c == 'c' && o.csiDA1 {
			h.da1()
		}
	default:
		// A prefix (< = > ?) or an intermediate (0x20 to 0x2f).
		o.csiDA1 = false
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

func (o *oscParser) end(h oscHooks, bel bool) {
	o.state = oscGround
	if o.overflow {
		return
	}
	code, text, _ := strings.Cut(string(o.buf), ";")
	h.osc(code, text, bel)
}

// cleanTitle makes a title from the session, such as an OSC 0 or 2 payload,
// safe to show: valid UTF-8, no control characters, trimmed, and not too
// long.
func cleanTitle(s string) string {
	return cleanText(s, maxTitle)
}

// cleanText makes text from the session safe to show, as cleanTitle does,
// cut to at most limit runes.
func cleanText(s string, limit int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > limit {
		s = string([]rune(s)[:limit])
	}
	return s
}

// maxNote is the longest notification text kept, in runes.
const maxNote = 1024

// notification returns the desktop notification in an OSC sequence, if it's
// one: iTerm2's OSC 9 (ESC ] 9 ; body), which Claude Code and Codex send,
// and rxvt's OSC 777 (ESC ] 777 ; notify ; title ; body), which other
// terminals have taken up. ConEmu numbers commands of its own in OSC 9, such
// as its progress reports (ESC ] 9 ; 4 ; state ; percent), which aren't
// notifications.
func notification(code, text string) (Alert, bool) {
	var a Alert
	switch code {
	case "9":
		if first, _, _ := strings.Cut(text, ";"); first != "" && strings.Trim(first, "0123456789") == "" {
			return Alert{}, false
		}
		a.Body = cleanText(text, maxNote)
	case "777":
		kind, rest, _ := strings.Cut(text, ";")
		if kind != "notify" {
			return Alert{}, false
		}
		title, body, _ := strings.Cut(rest, ";")
		a.Title, a.Body = cleanTitle(title), cleanText(body, maxNote)
	default:
		return Alert{}, false
	}
	return a, a.Title != "" || a.Body != ""
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
