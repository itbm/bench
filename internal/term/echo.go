package term

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

// Echo is a pretend session host for the browser preview: it never runs a
// process. It prints a prompt, echoes what's typed, and answers a few
// commands (help, ls, pwd, echo, clear) with canned output. Its methods are
// safe for concurrent use, and like Manager's, they never call Callbacks
// themselves.
type Echo struct {
	cb Callbacks

	mu       sync.Mutex
	sessions map[string]*echoSession
	gens     generations
}

var _ Host = (*Echo)(nil)

// NewEcho returns an Echo host.
func NewEcho(cb Callbacks) *Echo {
	return &Echo{
		cb:       cb.withDefaults(),
		sessions: make(map[string]*echoSession),
		gens:     make(generations),
	}
}

// Start starts a pretend session, or does nothing if it's already running.
// If opts.Command is set, the session shows it being typed at the first
// prompt, and answers it as it would anything else. As with Manager, a
// closed session that Start replaces sends no more callbacks.
func (e *Echo) Start(id string, opts Options) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.sessions[id]; ok {
		return nil
	}
	s := newEchoSession(id, opts, e.cb)
	e.gens.replace(id, s.replaced)
	e.sessions[id] = s
	go s.run(opts.Command)
	return nil
}

// Running reports whether the session exists: pretend sessions only end
// when closed.
func (e *Echo) Running(id string) bool {
	return e.get(id) != nil
}

// Write sends typed input to the session.
func (e *Echo) Write(id string, data []byte) error {
	s := e.get(id)
	if s == nil {
		return fmt.Errorf("write to terminal %s: %w", id, ErrNoSession)
	}
	select {
	case s.input <- bytes.Clone(data):
		return nil
	default:
		return fmt.Errorf("write to terminal %s: %w", id, ErrBusy)
	}
}

// Resize records the terminal's size; pretend sessions don't use it.
func (e *Echo) Resize(id string, cols, rows int) error {
	s := e.get(id)
	if s == nil {
		return fmt.Errorf("resize terminal %s: %w", id, ErrNoSession)
	}
	if err := checkSize(cols, rows); err != nil {
		return err
	}
	s.mu.Lock()
	s.cols, s.rows = clampCells(cols), clampCells(rows)
	s.mu.Unlock()
	return nil
}

// Title returns "<shell> — <folder>", such as "zsh — ~/code/bench", or "" if
// there's no such session.
func (e *Echo) Title(id string) string {
	s := e.get(id)
	if s == nil {
		return ""
	}
	return s.title
}

// Snapshot returns the session's recent output, up to SnapshotLimit bytes.
func (e *Echo) Snapshot(id string) []byte {
	s := e.get(id)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.Bytes()
}

// Close ends a pretend session. Exit is called from its goroutine.
func (e *Echo) Close(id string) {
	e.mu.Lock()
	s := e.sessions[id]
	delete(e.sessions, id)
	e.mu.Unlock()
	if s != nil {
		s.end()
	}
}

// CloseAll ends every pretend session.
func (e *Echo) CloseAll() {
	e.mu.Lock()
	all := e.sessions
	e.sessions = make(map[string]*echoSession)
	e.mu.Unlock()
	for _, s := range all {
		s.end()
	}
}

func (e *Echo) get(id string) *echoSession {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sessions[id]
}

// Escape sequence states for typed input.
const (
	escNone  = iota
	escStart // after ESC
	escCSI   // after ESC [
	escSS3   // after ESC O
)

const (
	// maxLine is the longest line a pretend session keeps, in runes.
	maxLine = 4096
	// pretendHome stands in for the home folder in pwd's answer, so the
	// preview doesn't show the real one.
	pretendHome = "/Users/you"
	// clearScreen moves to the top left, then clears the screen and the
	// scrollback.
	clearScreen = "\x1b[H\x1b[2J\x1b[3J"
)

// echoBanner is the first thing a pretend session prints.
const echoBanner = "\x1b[2mThis is a pretend terminal: the browser preview doesn't run a shell. " +
	"Type help to see what it can do.\x1b[0m\r\n"

// echoHelp is the answer to help.
const echoHelp = "This pretend terminal understands these commands:\r\n" +
	"  help    Show this list.\r\n" +
	"  ls      List the files in this folder.\r\n" +
	"  pwd     Show this folder.\r\n" +
	"  echo    Print what follows it.\r\n" +
	"  clear   Clear the screen.\r\n"

// echoListing is the answer to ls.
const echoListing = "README.md  cmd  docs  e2e  frontend  go.mod  go.sum  internal\r\n"

// echoSession is one pretend session.
type echoSession struct {
	id       string
	cb       Callbacks
	replaced *atomic.Bool // set when a later Start with the same ID replaces this session
	folder   string       // as shown, such as "~/code/bench"
	title    string
	input    chan []byte
	quit     chan struct{}
	once     sync.Once

	mu         sync.Mutex
	out        ring
	cols, rows int

	// Only the run goroutine uses these.
	line    []rune
	pending []byte // an incomplete UTF-8 character
	esc     int
	lastCR  bool
}

func newEchoSession(id string, opts Options, cb Callbacks) *echoSession {
	shell := "zsh"
	if opts.Shell != "" {
		shell = filepath.Base(opts.Shell)
	}
	folder := echoFolder(opts.Dir, homeDir())
	cols, rows := opts.size()
	return &echoSession{
		id:       id,
		cb:       cb,
		replaced: new(atomic.Bool),
		folder:   folder,
		title:    formatTitle(shell, folder),
		input:    make(chan []byte, inputQueueEcho),
		quit:     make(chan struct{}),
		out:      newRing(SnapshotLimit),
		cols:     cols,
		rows:     rows,
	}
}

// inputQueueEcho is how many writes can wait for a pretend session.
const inputQueueEcho = 1024

// echoFolder returns the folder to show for dir, with the home folder as ~.
// Relative folders are taken to be in the home folder, as Manager does.
func echoFolder(dir, home string) string {
	switch {
	case dir == "" || dir == "~":
		return "~"
	case strings.HasPrefix(dir, "~/"):
		return path.Clean(dir)
	case filepath.IsAbs(dir):
		return abbreviate(dir, home)
	}
	return path.Clean("~/" + filepath.ToSlash(dir))
}

// run prints the banner and prompt, then answers input until the session
// ends, and reports Exit last of all.
func (s *echoSession) run(command string) {
	var b strings.Builder
	b.WriteString(echoBanner)
	b.WriteString(s.prompt())
	if command != "" {
		b.WriteString(command)
		b.WriteString("\r\n")
		b.WriteString(s.answer(command))
		b.WriteString(s.prompt())
	}
	s.emit(b.String())
	for {
		select {
		case <-s.quit:
			if !s.replaced.Load() {
				s.cb.Exit(s.id, nil)
			}
			return
		case data := <-s.input:
			s.handle(data)
		}
	}
}

// end stops the session's goroutine.
func (s *echoSession) end() {
	s.once.Do(func() { close(s.quit) })
}

// emit records output for Snapshot and passes it on.
func (s *echoSession) emit(text string) {
	if text == "" {
		return
	}
	b := []byte(text)
	s.mu.Lock()
	s.out.Write(b)
	s.mu.Unlock()
	if !s.replaced.Load() {
		s.cb.Data(s.id, b)
	}
}

func (s *echoSession) prompt() string {
	return s.folder + " $ "
}

// handle answers one write of typed input.
func (s *echoSession) handle(data []byte) {
	var out strings.Builder
	s.pending = append(s.pending, data...)
	p := s.pending
	for len(p) > 0 && utf8.FullRune(p) {
		r, n := utf8.DecodeRune(p)
		p = p[n:]
		if r == utf8.RuneError && n == 1 {
			continue
		}
		s.key(r, &out)
	}
	s.pending = append(s.pending[:0], p...)
	s.emit(out.String())
}

// key handles one typed character, writing what the terminal should show.
func (s *echoSession) key(r rune, out *strings.Builder) {
	if s.esc != escNone {
		s.escape(r)
		return
	}
	if r == '\n' && s.lastCR {
		// The second half of a pasted CR LF.
		s.lastCR = false
		return
	}
	s.lastCR = r == '\r'
	switch r {
	case '\r', '\n':
		line := string(s.line)
		s.line = s.line[:0]
		out.WriteString("\r\n")
		out.WriteString(s.answer(line))
		out.WriteString(s.prompt())
	case esc:
		s.esc = escStart
	case 0x7f, 0x08: // Backspace.
		if len(s.line) > 0 {
			s.line = s.line[:len(s.line)-1]
			out.WriteString("\b \b")
		}
	case 0x03: // Control-C.
		s.line = s.line[:0]
		out.WriteString("^C\r\n")
		out.WriteString(s.prompt())
	case 0x15: // Control-U.
		out.WriteString(strings.Repeat("\b \b", len(s.line)))
		s.line = s.line[:0]
	case 0x0c: // Control-L.
		out.WriteString(clearScreen)
		out.WriteString(s.prompt())
		out.WriteString(string(s.line))
	default:
		if unicode.IsControl(r) || len(s.line) >= maxLine {
			return
		}
		s.line = append(s.line, r)
		out.WriteRune(r)
	}
}

// escape skips an escape sequence, such as an arrow key, one character at a
// time.
func (s *echoSession) escape(r rune) {
	switch s.esc {
	case escStart:
		switch r {
		case '[':
			s.esc = escCSI
		case 'O':
			s.esc = escSS3
		default:
			s.esc = escNone
		}
	case escCSI:
		// Parameters and intermediates (0x20 to 0x3f) carry on; a final
		// character, or anything unexpected, ends the sequence.
		if r < 0x20 || r >= 0x40 {
			s.esc = escNone
		}
	default:
		s.esc = escNone
	}
}

// answer returns the canned output for a line, ending in a new line, or
// nothing.
func (s *echoSession) answer(line string) string {
	words := splitWords(line)
	if len(words) == 0 {
		return ""
	}
	switch words[0] {
	case "help":
		return echoHelp
	case "ls":
		return echoListing
	case "pwd":
		return pretendPath(s.folder) + "\r\n"
	case "echo":
		return strings.Join(words[1:], " ") + "\r\n"
	case "clear":
		return clearScreen
	}
	return words[0] + ": the browser preview can't run commands. Type help to see what it can do.\r\n"
}

// pretendPath shows folder as a full path, with a pretend home folder.
func pretendPath(folder string) string {
	if folder == "~" {
		return pretendHome
	}
	if rest, ok := strings.CutPrefix(folder, "~/"); ok {
		return pretendHome + "/" + rest
	}
	return folder
}

// splitWords splits a line into words as a shell would, roughly: spaces
// separate words, and single or double quotes keep spaces in them.
func splitWords(line string) []string {
	var words []string
	var cur strings.Builder
	inWord := false
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}
