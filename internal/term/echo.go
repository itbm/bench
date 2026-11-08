package term

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

// Echo is a pretend session host for the browser preview: it never runs a
// process. It prints a prompt, echoes what's typed, and answers a few
// commands (help, ls, pwd, cd, echo, clear, bell, notify, agent) with canned
// output. Its folders are pretend ones, which it shows and reports to
// Callbacks.Folder with ~ for the home folder, and never checks. Its
// methods are safe for concurrent use, and like Manager's, they never call
// Callbacks themselves.
type Echo struct {
	cb Callbacks

	mu       sync.Mutex
	sessions map[string]*echoSession
	gens     generations

	statusOn atomic.Bool // agent status is on (see SetAgentStatus)
}

var _ Host = (*Echo)(nil)

// NewEcho returns an Echo host.
func NewEcho(cb Callbacks) *Echo {
	e := &Echo{
		cb:       cb.withDefaults(),
		sessions: make(map[string]*echoSession),
		gens:     make(generations),
	}
	e.statusOn.Store(true)
	return e
}

// Start starts a pretend session, or does nothing if it's already running.
// It shows opts.Preface first. If opts.Command is set, the session shows it
// being typed at the first prompt, and answers it as it would anything else.
// opts.Input is typed at the first prompt, as if the user had typed it. As
// with Manager, a closed session that Start replaces sends no more
// callbacks.
func (e *Echo) Start(id string, opts Options) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.sessions[id]; ok {
		return nil
	}
	s := newEchoSession(id, opts, e.cb, &e.statusOn)
	e.gens.replace(id, s.replaced)
	e.sessions[id] = s
	go s.run(opts.Preface, opts.Command, opts.Input)
	return nil
}

// Running reports whether the session exists: pretend sessions only end
// when closed.
func (e *Echo) Running(id string) bool {
	return e.get(id) != nil
}

// Busy reports false: a pretend session never runs anything, and answers
// each command as soon as it's typed.
func (e *Echo) Busy(id string) (string, bool) { return "", false }

// LookPath returns "": pretend sessions run no programs, and the preview
// looks for none on the computer it runs on.
func (e *Echo) LookPath(name string) string { return "" }

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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.title
}

// Snapshot returns the session's recent output, up to SnapshotLimit bytes,
// and where it ends.
func (e *Echo) Snapshot(id string) Snapshot {
	s := e.get(id)
	if s == nil {
		return Snapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{Data: s.out.Bytes(), Offset: s.offset}
}

// Attach returns the session's snapshot, as Snapshot does. A pretend
// session asks nothing, so there's nothing for its views to answer, and it
// keeps no record of them.
func (e *Echo) Attach(id, view string) Snapshot {
	return e.Snapshot(id)
}

// Detach does nothing (see Attach).
func (e *Echo) Detach(id, view string) {}

// DetachView does nothing (see Attach).
func (e *Echo) DetachView(view string) {}

// SetAgentStatus turns agent status on or off. While it's off, the agent
// command still prints its report, but nothing is passed on to
// Callbacks.Status, as with Manager.
func (e *Echo) SetAgentStatus(on bool) {
	e.statusOn.Store(on)
}

// Clear forgets the session's recent output, as Manager.Clear does.
func (e *Echo) Clear(id string) {
	s := e.get(id)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.out.Clear()
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

// Ack does nothing: a pretend session prints only a little, in answer to
// what's typed, so it never waits for its views.
func (e *Echo) Ack(id string, offset int64) {}

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
	"  cd      Change folder, such as cd code.\r\n" +
	"  echo    Print what follows it.\r\n" +
	"  clear   Clear the screen.\r\n" +
	"  bell    Ring the bell in a few seconds, such as bell 5.\r\n" +
	"  notify  Send a notification in a few seconds, such as notify 5 Done.\r\n" +
	"  agent   Report an agent's state in a few seconds, such as agent 5 done Tests passed.\r\n"

// echoAgentHelp is the answer to agent without a state.
const echoAgentHelp = "Type agent, a state, and what the agent says, such as agent blocked permission approve Bash.\r\n" +
	"  States: idle, working, done, blocked, error, clear.\r\n" +
	"  After blocked, a kind can come next: permission, question, auth.\r\n" +
	"  A number of seconds to wait can come before the state, such as agent 0 working.\r\n"

// echoAlertWait is how long bell, notify, and agent wait without a number
// of seconds, for the user to look at another tab first.
const echoAlertWait = 3 * time.Second

// maxEchoAlertWait is the longest bell, notify, and agent wait.
const maxEchoAlertWait = time.Minute

// echoListing is the answer to ls.
const echoListing = "README.md  cmd  docs  e2e  frontend  go.mod  go.sum  internal\r\n"

// echoSession is one pretend session.
type echoSession struct {
	id       string
	cb       Callbacks
	replaced *atomic.Bool // set when a later Start with the same ID replaces this session
	statusOn *atomic.Bool // the Echo's (see Echo.SetAgentStatus)
	shell    string
	input    chan []byte
	alerts   chan Alert    // bell's and notify's, once they're due
	reported chan struct{} // a report of agent's may be due (see reportLater)
	quit     chan struct{}
	once     sync.Once

	mu         sync.Mutex
	out        ring
	offset     int64  // how much output there has been
	folder     string // as shown, such as "~/code/bench"
	title      string
	cols, rows int
	reports    []echoReport // agent's, in the order they're due

	// Only the run goroutine uses these.
	line    []rune
	pending []byte // an incomplete UTF-8 character
	esc     int
	lastCR  bool
}

func newEchoSession(id string, opts Options, cb Callbacks, statusOn *atomic.Bool) *echoSession {
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
		statusOn: statusOn,
		shell:    shell,
		folder:   folder,
		title:    formatTitle(shell, folder),
		input:    make(chan []byte, inputQueueEcho),
		alerts:   make(chan Alert, 1),
		reported: make(chan struct{}, 1),
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

// run prints the preface, the banner, and the prompt, and types input, then
// answers input until the session ends, and reports Exit last of all.
func (s *echoSession) run(preface []byte, command, input string) {
	if len(preface) > 0 {
		s.emit(string(preface))
	}
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
	if input != "" {
		s.handle([]byte(input))
	}
	for {
		select {
		case <-s.quit:
			if !s.replaced.Load() {
				s.cb.Exit(s.id, nil)
			}
			return
		case data := <-s.input:
			s.handle(data)
		case a := <-s.alerts:
			s.alert(a)
		case <-s.reported:
			for _, text := range s.dueReports(time.Now()) {
				s.report(text)
			}
		}
	}
}

// alert rings the bell, or sends a notification, as a program would: it
// prints the BEL or OSC 9 sequence for the view, and reports it.
func (s *echoSession) alert(a Alert) {
	if a.Bell {
		s.emit("\a")
	} else {
		s.emit("\x1b]9;" + a.Body + "\a")
	}
	if !s.replaced.Load() {
		s.cb.Alert(s.id, a)
	}
}

// alertLater has the session send a, once wait has passed, unless it has
// ended by then.
func (s *echoSession) alertLater(a Alert, wait time.Duration) {
	time.AfterFunc(wait, func() {
		select {
		case s.alerts <- a:
		case <-s.quit:
		}
	})
}

// report prints an OSC 7501 report, whose text (after "7501;") is text, as
// a program such as Claude Code would, and passes it on as Manager would:
// the same reports are kept, and the same are ignored.
func (s *echoSession) report(text string) {
	seq := "\x1b]" + statusCode + ";" + text + stEnd
	s.emit(seq)
	st, id, ok := parseStatus(text, len(seq))
	if ok && id == "" && s.statusOn.Load() && !s.replaced.Load() {
		s.cb.Status(s.id, st)
	}
}

// echoReport is a report of agent's, and when it's due.
type echoReport struct {
	due  time.Time
	text string
}

// reportLater has the session print and pass on a report, once wait has
// passed, unless it has ended by then. Reports go in the order they're due,
// and those due at once in the order they were made, as a program's would.
func (s *echoSession) reportLater(text string, wait time.Duration) {
	r := echoReport{due: time.Now().Add(wait), text: text}
	s.mu.Lock()
	i := len(s.reports)
	for i > 0 && s.reports[i-1].due.After(r.due) {
		i--
	}
	s.reports = slices.Insert(s.reports, i, r)
	s.mu.Unlock()
	time.AfterFunc(wait, func() {
		// The run goroutine takes every report that's due, so one wake
		// that's waiting is enough.
		select {
		case s.reported <- struct{}{}:
		default:
		}
	})
}

// dueReports takes the reports due by now, in order.
func (s *echoSession) dueReports(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []string
	for len(s.reports) > 0 && !s.reports[0].due.After(now) {
		due = append(due, s.reports[0].text)
		s.reports = s.reports[1:]
	}
	return due
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
	at := s.offset
	s.offset += int64(len(b))
	s.mu.Unlock()
	if !s.replaced.Load() {
		s.cb.Data(s.id, at, b)
	}
}

func (s *echoSession) prompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		s.mu.Lock()
		defer s.mu.Unlock()
		return pretendPath(s.folder) + "\r\n"
	case "cd":
		s.changeFolder(words[1:])
		return ""
	case "echo":
		return strings.Join(words[1:], " ") + "\r\n"
	case "clear":
		return clearScreen
	case "bell", "notify":
		wait, rest := splitWait(words[1:])
		a := Alert{Bell: true}
		if words[0] == "notify" {
			a = Alert{Body: cleanText(strings.Join(rest, " "), maxNote)}
			if a.Body == "" {
				a.Body = "The pretend terminal has finished."
			}
		}
		s.alertLater(a, wait)
		return ""
	case "agent":
		return s.agent(words[1:])
	}
	return words[0] + ": the browser preview can't run commands. Type help to see what it can do.\r\n"
}

// splitWait reads the number of seconds that bell, notify, and agent can
// start with, and returns how long to wait, and the words after it.
func splitWait(words []string) (time.Duration, []string) {
	if len(words) > 0 {
		if n, err := strconv.Atoi(words[0]); err == nil && n >= 0 {
			return min(time.Duration(n)*time.Second, maxEchoAlertWait), words[1:]
		}
	}
	return echoAlertWait, words
}

// agent handles the agent command, which reports a state as an agent such
// as Claude Code would, with OSC 7501: agent [seconds] state [kind] [words].
// A kind is taken only after blocked, and only if it's one of the
// protocol's; the words that follow are what the agent says. It returns
// help if there's no state.
func (s *echoSession) agent(words []string) string {
	wait, words := splitWait(words)
	if len(words) == 0 || !knownState(words[0]) {
		return echoAgentHelp
	}
	state, words := words[0], words[1:]
	kind := ""
	if state == StateBlocked && len(words) > 0 && knownKind(words[0]) {
		kind, words = words[0], words[1:]
	}
	s.reportLater(statusReport(state, kind, strings.Join(words, " ")), wait)
	return ""
}

// statusReport returns the text of an OSC 7501 report (after "7501;") of
// state, with kind and msg if they aren't empty.
func statusReport(state, kind, msg string) string {
	text := "state=" + state
	if kind != "" {
		text += ":kind=" + kind
	}
	if msg != "" {
		text += ":msg=" + base64.StdEncoding.EncodeToString([]byte(msg))
	}
	return text
}

// changeFolder handles cd: it moves to the folder named, or to the home
// folder, as shells do, without checking that the folder exists, and
// reports the new title and folder.
func (s *echoSession) changeFolder(args []string) {
	s.mu.Lock()
	to := "~"
	if len(args) > 0 {
		to = args[0]
	}
	var abs string
	switch {
	case to == "~" || strings.HasPrefix(to, "~/"):
		abs = pretendPath(to)
	case path.IsAbs(to):
		abs = to
	default:
		abs = path.Join(pretendPath(s.folder), to)
	}
	folder := path.Clean(abs)
	if folder == pretendHome {
		folder = "~"
	} else if rest, ok := strings.CutPrefix(folder, pretendHome+"/"); ok {
		folder = "~/" + rest
	}
	changed := folder != s.folder
	s.folder = folder
	s.title = formatTitle(s.shell, folder)
	title := s.title
	s.mu.Unlock()
	if changed && !s.replaced.Load() {
		s.cb.Title(s.id, title, false)
		s.cb.Folder(s.id, folder)
	}
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
