package term

import (
	"strconv"
	"strings"
)

// This file follows the terminal modes a program sets, so that a snapshot
// whose start has been dropped can set them again (see ring.Bytes).

// maxModeParams is the most parameters of one mode sequence that count.
const maxModeParams = 16

// Parser states for modes.feed.
const (
	modeGround  = iota
	modeEscape  // after ESC
	modeCSI     // after ESC [
	modePrivate // reading the parameters of ESC [ ?
	modeBang    // after ESC [ !
	modeSkip    // the rest of a sequence that doesn't matter here
)

// modes is the state of the terminal modes that change what the keys, the
// mouse, and pasting send, and which screen is drawn on: the ones a
// full-screen program such as vim or htop sets as it starts, and that a
// terminal replaying only its recent output would miss. Its zero value is a
// new terminal's. It's a plain value, so copying it copies the state.
type modes struct {
	cursorKeys bool // application cursor keys (DECSET 1)
	keypad     bool // application keypad (ESC =, or DECSET 66)
	altScreen  bool // the alternate screen (DECSET 47, 1047, or 1049)
	mouse      int  // the mouse reporting set last (DECSET 9, 1000, 1002, or 1003), or 0
	mouseCodes int  // the mouse report encoding set last (DECSET 1006 or 1016), or 0
	focus      bool // focus reports (DECSET 1004)
	paste      bool // bracketed paste (DECSET 2004)
	noCursor   bool // the cursor is hidden (DECRST 25)
	noWrap     bool // auto-wrap is off (DECRST 7)

	// The parser's state, carried from one feed to the next, since a
	// sequence can be split between reads.
	state  int
	params [maxModeParams]int
	count  int // parameters finished
	param  int // the parameter being read
}

// feed follows the mode changes in a stretch of output.
func (m *modes) feed(p []byte) {
	for _, c := range p {
		m.step(c)
	}
}

// step handles one byte of output.
func (m *modes) step(c byte) {
	switch c {
	case 0x1b:
		m.state = modeEscape
		return
	case 0x18, 0x1a: // CAN and SUB cancel a sequence.
		m.state = modeGround
		return
	}
	switch m.state {
	case modeEscape:
		m.state = modeGround
		switch c {
		case '[':
			m.state = modeCSI
		case '=':
			m.keypad = true
		case '>':
			m.keypad = false
		case 'c': // RIS: a full reset.
			*m = modes{}
		}
	case modeCSI:
		switch {
		case c == '?':
			m.state, m.count, m.param = modePrivate, 0, 0
		case c == '!':
			m.state = modeBang
		case c >= 0x40 && c <= 0x7e:
			m.state = modeGround
		case c >= 0x20:
			m.state = modeSkip
		}
	case modePrivate:
		switch {
		case c >= '0' && c <= '9':
			m.param = min(m.param*10+int(c-'0'), 99999)
		case c == ';':
			m.push()
		case c == 'h' || c == 'l':
			m.push()
			for _, n := range m.params[:m.count] {
				m.set(n, c == 'h')
			}
			m.state = modeGround
		case c >= 0x40 && c <= 0x7e:
			m.state = modeGround
		case c >= 0x20:
			m.state = modeSkip
		}
	case modeBang:
		if c == 'p' {
			// DECSTR: a soft reset. In xterm.js, it also turns off
			// bracketed paste and focus reports.
			m.cursorKeys, m.keypad, m.noCursor, m.noWrap = false, false, false, false
			m.paste, m.focus = false, false
		}
		m.state = modeGround
		if c >= 0x20 && c < 0x40 {
			m.state = modeSkip
		}
	case modeSkip:
		if c >= 0x40 && c <= 0x7e {
			m.state = modeGround
		}
	}
}

// push finishes the parameter being read.
func (m *modes) push() {
	if m.count < maxModeParams {
		m.params[m.count] = m.param
		m.count++
	}
	m.param = 0
}

// set sets (DECSET) or resets (DECRST) a private mode. As in xterm.js, the
// mouse reporting and report encoding set last are the ones in force, and
// resetting any of them turns it off. xterm.js ignores the UTF-8 and urxvt
// encodings (1005 and 1015), so they're ignored here too: setting one again
// would turn off an SGR encoding that's in force.
func (m *modes) set(n int, on bool) {
	switch n {
	case 1:
		m.cursorKeys = on
	case 66:
		m.keypad = on
	case 7:
		m.noWrap = !on
	case 25:
		m.noCursor = !on
	case 47, 1047, 1049:
		m.altScreen = on
	case 9, 1000, 1002, 1003:
		m.mouse = 0
		if on {
			m.mouse = n
		}
	case 1004:
		m.focus = on
	case 1006, 1016:
		m.mouseCodes = 0
		if on {
			m.mouseCodes = n
		}
	case 2004:
		m.paste = on
	}
}

// restore returns the sequences that set these modes in a new terminal, or
// nothing if they're all as a new terminal has them. The alternate screen
// comes first, since switching to it starts a clean screen.
func (m *modes) restore() []byte {
	var b strings.Builder
	decset := func(n int, on bool) {
		b.WriteString("\x1b[?")
		b.WriteString(strconv.Itoa(n))
		if on {
			b.WriteByte('h')
		} else {
			b.WriteByte('l')
		}
	}
	if m.altScreen {
		decset(1049, true)
	}
	if m.cursorKeys {
		decset(1, true)
	}
	if m.keypad {
		b.WriteString("\x1b=")
	}
	if m.mouse != 0 {
		decset(m.mouse, true)
	}
	if m.mouseCodes != 0 {
		decset(m.mouseCodes, true)
	}
	if m.focus {
		decset(1004, true)
	}
	if m.paste {
		decset(2004, true)
	}
	if m.noCursor {
		decset(25, false)
	}
	if m.noWrap {
		decset(7, false)
	}
	if b.Len() == 0 {
		return nil
	}
	return []byte(b.String())
}
