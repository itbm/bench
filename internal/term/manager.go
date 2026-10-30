//go:build darwin || linux

package term

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// closeAllWait is how long CloseAll waits for processes to end after
// hanging them up.
const closeAllWait = time.Second

// inputWait is the longest a session waits for its shell's line editor
// before it types Options.Input anyway, for a shell that may have none,
// such as sh or dash.
const inputWait = time.Second

// editorWait is the longest a session waits for the line editor of a shell
// that has one (editorShells), before it types Options.Input anyway. Its
// start-up files can take several seconds, as when they load nvm or conda,
// or rebuild completions, and the line editor normally has the terminal
// long before this.
const editorWait = 15 * time.Second

// editorShells are the shells, by name, that always start a line editor at
// their first prompt.
var editorShells = []string{"zsh", "bash", "fish"}

// Manager runs real sessions with creack/pty. Its methods are safe for
// concurrent use. They never call Callbacks themselves: callbacks come from
// each session's own goroutines, so a caller may hold a lock that its
// callbacks also take.
type Manager struct {
	cb         Callbacks
	titleEvery time.Duration
	flowWait   time.Duration // how long a session waits for its views (see flowWindow)
	inputWait  time.Duration // how long a session waits to type Options.Input (see inputWait)
	editorWait time.Duration // the same, for a shell with a line editor (see editorWait)

	mu       sync.Mutex
	sessions map[string]*session
	gens     generations
}

var _ Host = (*Manager)(nil)

// NewManager returns a manager.
func NewManager(cb Callbacks) *Manager {
	if os.Getenv("LANG") == "" {
		// Sessions will need a LANG, and working it out can take a
		// moment, so it starts now rather than in the first Start.
		go defaultLang()
	}
	return &Manager{
		cb:         cb.withDefaults(),
		titleEvery: time.Second,
		flowWait:   time.Second,
		inputWait:  inputWait,
		editorWait: editorWait,
		sessions:   make(map[string]*session),
		gens:       make(generations),
	}
}

// Start starts a session running a login shell ($SHELL -l), or Command
// through it ($SHELL -l -c), in opts.Dir, and types opts.Input into the
// shell once it's ready. It does nothing if the session is already
// running.
//
// A session that has ended, or been closed, is replaced. Once it has, the old
// session's callbacks stop, including an Exit it hasn't reported yet, so
// that everything reported for id is about the new session.
func (m *Manager) Start(id string, opts Options) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var from int64
	if old := m.sessions[id]; old != nil {
		if old.running() {
			return nil
		}
		// The new session's output carries on from the old one's.
		from = old.end()
	}
	// The shell decides how long the session waits to type opts.Input.
	shell, err := chooseShell(opts.Shell, os.Getenv("SHELL"))
	if err != nil {
		return fmt.Errorf("start terminal: %w", err)
	}
	opts.Shell = shell
	replaced := new(atomic.Bool)
	s, err := startSession(id, opts, m.cb, m.titleEvery, m.flowWait, m.inputWaitFor(shell), replaced, from)
	if err != nil {
		return fmt.Errorf("start terminal: %w", err)
	}
	// Only now that the new session exists does the old one go quiet.
	m.gens.replace(id, replaced)
	m.sessions[id] = s
	return nil
}

// inputWaitFor returns the longest a session of shell waits for its line
// editor before it types Options.Input anyway: long for a shell that has
// one, so that the input waits for its start-up files, however slow, and
// short for any other, which may never start one.
func (m *Manager) inputWaitFor(shell string) time.Duration {
	if slices.Contains(editorShells, filepath.Base(shell)) {
		return m.editorWait
	}
	return m.inputWait
}

// LookPath returns the path of the program that a terminal's shell runs for
// the command name, as an interactive login shell finds it on the user's
// PATH, or "" if it finds none, or doesn't answer within a few seconds.
func (m *Manager) LookPath(name string) string {
	return findInShell(name, os.Getenv("SHELL"), lookWait)
}

// Running reports whether the session's process is running.
func (m *Manager) Running(id string) bool {
	s := m.get(id)
	return s != nil && s.running()
}

// Busy reports whether the session is running something, and what (see
// Host.Busy).
func (m *Manager) Busy(id string) (string, bool) {
	s := m.get(id)
	if s == nil {
		return "", false
	}
	return s.busy()
}

// Write sends input (keystrokes) to the session. It doesn't wait for the
// process to read it.
func (m *Manager) Write(id string, data []byte) error {
	s := m.get(id)
	if s == nil {
		return fmt.Errorf("write to terminal %s: %w", id, ErrNoSession)
	}
	if err := s.send(data); err != nil {
		return fmt.Errorf("write to terminal %s: %w", id, err)
	}
	return nil
}

// Resize sets the terminal's size, in character cells. Resizing a session
// that has ended does nothing.
func (m *Manager) Resize(id string, cols, rows int) error {
	s := m.get(id)
	if s == nil {
		return fmt.Errorf("resize terminal %s: %w", id, ErrNoSession)
	}
	return s.resize(cols, rows)
}

// Title returns the session's title, or "" if there's no such session. It
// stays as it was once the session ends.
func (m *Manager) Title(id string) string {
	s := m.get(id)
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.title
}

// Snapshot returns the session's recent output, up to SnapshotLimit bytes,
// and where it ends. It's kept after the process ends, until Close.
func (m *Manager) Snapshot(id string) Snapshot {
	s := m.get(id)
	if s == nil {
		return Snapshot{}
	}
	return s.snapshot()
}

// Clear forgets the session's recent output, as a view clears its
// scrollback, so that Snapshot returns none of it. Offsets carry on.
func (m *Manager) Clear(id string) {
	if s := m.get(id); s != nil {
		s.clear()
	}
}

// Close ends a session: it sends SIGHUP to its process and closes its pty,
// and forgets it. Exit is called when the process has ended.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	s := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if s != nil {
		s.hangUp()
	}
}

// CloseAll closes every session, then waits up to a second for their
// processes to end.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	all := make([]*session, 0, len(m.sessions))
	for id, s := range m.sessions {
		all = append(all, s)
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	for _, s := range all {
		s.hangUp()
	}
	deadline := time.NewTimer(closeAllWait)
	defer deadline.Stop()
	for _, s := range all {
		select {
		case <-s.exited:
		case <-deadline.C:
			return
		}
	}
}

func (m *Manager) get(id string) *session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// Ack records that a view has drawn the session's output up to offset, so
// that the session reads more if it was waiting for its views.
func (m *Manager) Ack(id string, offset int64) {
	if s := m.get(id); s != nil {
		s.ack(offset)
	}
}
