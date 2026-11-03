package term

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// This file makes a session's output safe to keep between launches, and to
// show again before a new session's own (see Options.Preface).

// maxSequence is the longest control sequence kept, in bytes. Longer ones
// are left out.
const maxSequence = 256

// keptFinals are the final bytes of the CSI sequences that output keeps:
// those that move the cursor, erase, insert, delete, scroll, and set colours
// (SGR, m). Others ask the terminal something, such as where the cursor is
// (n), what it is (c), or how big its window is (t), or change how it works
// from then on, as setting a scrolling region (r) or modes (h and l) do.
const keptFinals = "@ABCDEFGHIJKLMPSTXZ`abdefmsu"

// Restorable returns output as Bench keeps it between launches: the text the
// program drew, with the sequences that colour it, move the cursor, and
// erase, so that a terminal drawing it shows what the program showed.
// Everything else goes: sequences that ask the terminal something, whose
// answers would reach the new session as if they'd been typed; those that
// change the terminal's modes, such as mouse reporting, or set its title or
// the clipboard; the bell; and what full-screen programs, such as vim, drew
// on the alternate screen, which went as they ended. Control sequences that
// start with a C1 character, as xterm.js reads them, count the same as with
// ESC. The result is at most limit bytes, from the end of output, starting
// at a line if one starts in it.
func Restorable(output []byte, limit int) []byte {
	var r restorer
	p := output
	for len(p) > 0 {
		c, n := utf8.DecodeRune(p)
		switch {
		case c == utf8.RuneError && n == 1:
			p = p[1:] // Not UTF-8.
		case c == esc:
			if len(p) < 2 {
				p = nil
				break
			}
			p = r.escape(p[1], p[2:])
		case c >= 0x80 && c <= 0x9f:
			// A C1 control, the same as ESC and the character 0x40 below
			// it.
			p = r.escape(byte(c-0x40), p[n:])
		case c < 0x20 || c == 0x7f:
			// Backspace, tab, line feeds, carriage return, and shifting
			// between character sets draw; other controls, such as the
			// bell and ENQ, which asks the terminal to answer, go.
			if strings.ContainsRune("\b\t\n\v\f\r\x0e\x0f", c) {
				r.add(p[:1])
			}
			p = p[1:]
		default:
			r.add(p[:n])
			p = p[n:]
		}
	}
	return lastLines(r.out, limit)
}

// restorer collects what Restorable keeps.
type restorer struct {
	out []byte
	alt bool // on the alternate screen, whose output goes
}

// add keeps b, unless it's drawn on the alternate screen.
func (r *restorer) add(b ...[]byte) {
	if r.alt {
		return
	}
	for _, part := range b {
		r.out = append(r.out, part...)
	}
}

// escape handles an escape sequence: intro is the character after ESC, and
// p what follows it. It returns what follows the sequence.
func (r *restorer) escape(intro byte, p []byte) []byte {
	switch intro {
	case '[':
		return r.csi(p)
	case ']':
		// OSC: a title, a folder, a colour, a link, or the clipboard,
		// ended by BEL or ST.
		return skipString(p, true)
	case 'P', 'X', '^', '_':
		// DCS, SOS, PM, and APC, which only ST ends.
		return skipString(p, false)
	case '(', ')':
		// Choose the character set for G0 or G1: ASCII, or line drawing.
		if len(p) > 0 && (p[0] == 'B' || p[0] == '0') {
			r.add([]byte{esc, intro, p[0]})
		}
		if len(p) > 0 {
			p = p[1:]
		}
		return p
	case '*', '+', '#', ' ', '%':
		// Other character sets, screen tests such as filling the screen
		// with E, and how controls are sent: none of them draw output.
		if len(p) > 0 {
			p = p[1:]
		}
		return p
	case '7', '8', 'D', 'E', 'M':
		// Save and restore the cursor, index, next line, and reverse
		// index.
		r.add([]byte{esc, intro})
	case 'c':
		// A full reset, which leaves the alternate screen.
		r.alt = false
	}
	// Others, such as ESC Z, which asks what the terminal is, and the
	// keypad modes, go.
	return p
}

// csi handles a control sequence, whose parameters start p, keeping it if
// it draws, and following the alternate screen. It returns what follows it.
func (r *restorer) csi(p []byte) []byte {
	i := 0
	for i < len(p) && p[i] >= 0x20 && p[i] <= 0x3f {
		i++
	}
	if i == len(p) {
		return nil
	}
	final := p[i]
	if final < 0x40 || final > 0x7e {
		// Not a sequence after all: a control or another escape ends it,
		// and is read again.
		if final == 0x18 || final == 0x1a {
			return p[i+1:]
		}
		return p[i:]
	}
	params, rest := p[:i], p[i+1:]
	if len(params) > maxSequence {
		return rest
	}
	if len(params) > 0 && params[0] == '?' && (final == 'h' || final == 'l') {
		for _, n := range strings.Split(string(params[1:]), ";") {
			switch n {
			case "47", "1047", "1049":
				r.alt = final == 'h'
			}
		}
		return rest
	}
	plain := bytes.IndexFunc(params, func(c rune) bool { return c < '0' || c > ';' }) < 0
	if plain && strings.IndexByte(keptFinals, final) >= 0 {
		r.add([]byte{esc, '['}, params, []byte{final})
	}
	return rest
}

// skipString returns what follows a string that starts p: one ended by ST
// (ESC \, or the C1 character), by BEL if bell is set, as OSC strings may
// be, or cancelled by CAN or SUB. Another escape sequence also ends it, as
// in xterm, and is read again.
func skipString(p []byte, bell bool) []byte {
	for i := 0; i < len(p); i++ {
		switch c := p[i]; {
		case c == esc:
			if i+1 < len(p) && p[i+1] == '\\' {
				return p[i+2:]
			}
			return p[i:]
		case c == bel && bell, c == 0x18, c == 0x1a:
			return p[i+1:]
		case c == 0xc2 && i+1 < len(p) && p[i+1] == 0x9c:
			return p[i+2:]
		}
	}
	return nil
}

// lastLines returns the end of b, at most limit bytes of it, from the start
// of a line if one starts there, or else from the start of a character.
func lastLines(b []byte, limit int) []byte {
	if len(b) <= limit {
		return b
	}
	b = b[len(b)-limit:]
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[i+1:]
	}
	for len(b) > 0 && !utf8.RuneStart(b[0]) {
		b = b[1:]
	}
	return b
}

// Greyed returns restorable output (see Restorable) as a terminal shows it
// again after Bench reopens: text in the terminal's own colour is bright
// black (SGR 90), which is grey in both of Bench's sets of colours, and
// readable on either, so that it doesn't look like the new session's. The
// colours that the program chose stay, at full strength: faint (SGR 2),
// which halves a colour, would leave text too pale to read on a light
// background, so Greyed leaves it out. Each colour sequence that goes back
// to the terminal's own colour ends with bright black again; other
// sequences, and those with a private marker, pass as they are. The output
// ends with the colours and the character set as a new terminal has them.
func Greyed(output []byte) []byte {
	var b bytes.Buffer
	b.WriteString("\x1b[0;90m")
	for p := output; len(p) > 0; {
		i := bytes.Index(p, []byte("\x1b["))
		if i < 0 {
			b.Write(p)
			break
		}
		b.Write(p[:i])
		p = p[i:]
		end := 2
		for end < len(p) && p[end] >= 0x20 && p[end] <= 0x3f {
			end++
		}
		if end == len(p) {
			b.Write(p)
			break
		}
		params := p[2:end]
		if p[end] == 'm' && bytes.IndexFunc(params, func(c rune) bool { return c < '0' || c > ';' }) < 0 {
			b.WriteString(greyedSGR(string(params)))
		} else {
			b.Write(p[:end+1])
		}
		p = p[end+1:]
	}
	b.WriteString("\x1b[0m\x1b(B\x0f")
	return b.Bytes()
}

// greyedSGR returns the SGR sequence with params as Greyed draws it:
// without faint, and with bright black at its end if it leaves the text in
// the terminal's own colour (0, an empty parameter, or 39, after any
// other colour). A sequence that only made text faint goes.
func greyedSGR(params string) string {
	fields := strings.Split(params, ";")
	kept := make([]string, 0, len(fields)+1)
	own := false
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		code, _, colon := strings.Cut(field, ":")
		if code = strings.TrimLeft(code, "0"); code == "" {
			code = "0"
		}
		switch code {
		case "0", "39":
			own = true
		case "2":
			if !colon {
				continue
			}
		case "30", "31", "32", "33", "34", "35", "36", "37",
			"90", "91", "92", "93", "94", "95", "96", "97":
			own = false
		case "38", "48", "58":
			// The text's, background's, or underline's colour, from the
			// 256 (5;n) or as red, green, and blue (2;r;g;b). With colons,
			// the colour is part of the parameter.
			if code == "38" {
				own = false
			}
			if !colon && i+1 < len(fields) {
				n := 0
				switch fields[i+1] {
				case "5":
					n = 2
				case "2":
					n = 4
				}
				n = min(n, len(fields)-1-i)
				kept = append(kept, fields[i:i+1+n]...)
				i += n
				continue
			}
		}
		if field == "" {
			field = "0"
		}
		kept = append(kept, field)
	}
	if own {
		kept = append(kept, "90")
	}
	if len(kept) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(kept, ";") + "m"
}
