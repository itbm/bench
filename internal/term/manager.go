//go:build darwin || linux

package term

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// closeAllWait is how long CloseAll waits for processes to end after
// hanging them up.
const closeAllWait = time.Second

// Manager runs real sessions with creack/pty. Its methods are safe for
// concurrent use. They never call Callbacks themselves: callbacks come from
// each session's own goroutines, so a caller may hold a lock that its
// callbacks also take.
type Manager struct {
	cb         Callbacks
	titleEvery time.Duration

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
		sessions:   make(map[string]*session),
		gens:       make(generations),
	}
}

// Start starts a session running a login shell ($SHELL -l), or Command
// through it ($SHELL -l -c), in opts.Dir. It does nothing if the session is
// already running.
//
// A session that has ended, or been closed, is replaced. Once it has, the old
// session's callbacks stop, including an Exit it hasn't reported yet, so
// that everything reported for id is about the new session.
func (m *Manager) Start(id string, opts Options) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.sessions[id]; s != nil && s.running() {
		return nil
	}
	replaced := new(atomic.Bool)
	s, err := startSession(id, opts, m.cb, m.titleEvery, replaced)
	if err != nil {
		return fmt.Errorf("start terminal: %w", err)
	}
	// Only now that the new session exists does the old one go quiet.
	m.gens.replace(id, replaced)
	m.sessions[id] = s
	return nil
}

// Running reports whether the session's process is running.
func (m *Manager) Running(id string) bool {
	s := m.get(id)
	return s != nil && s.running()
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

// Snapshot returns the session's recent output, up to SnapshotLimit bytes.
// It's kept after the process ends, until Close.
func (m *Manager) Snapshot(id string) []byte {
	s := m.get(id)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.Bytes()
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
