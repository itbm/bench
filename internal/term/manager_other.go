//go:build !darwin && !linux

package term

import (
	"errors"
	"fmt"
)

// errUnsupported is returned by Manager.Start where pseudo-terminals aren't
// supported.
var errUnsupported = errors.New("terminals need macOS or Linux")

// Manager runs real sessions with creack/pty. On this system it can't start
// any: Start returns an error, and the other methods behave as if there are
// no sessions.
type Manager struct{}

var _ Host = (*Manager)(nil)

// NewManager returns a manager.
func NewManager(cb Callbacks) *Manager { return &Manager{} }

// Start returns an error: terminals aren't supported here.
func (m *Manager) Start(id string, opts Options) error {
	return fmt.Errorf("start terminal: %w", errUnsupported)
}

// Running reports false.
func (m *Manager) Running(id string) bool { return false }

// Write returns ErrNoSession.
func (m *Manager) Write(id string, data []byte) error {
	return fmt.Errorf("write to terminal %s: %w", id, ErrNoSession)
}

// Resize returns ErrNoSession.
func (m *Manager) Resize(id string, cols, rows int) error {
	return fmt.Errorf("resize terminal %s: %w", id, ErrNoSession)
}

// Title returns "".
func (m *Manager) Title(id string) string { return "" }

// Snapshot returns nothing.
func (m *Manager) Snapshot(id string) Snapshot { return Snapshot{} }

// Close does nothing.
func (m *Manager) Close(id string) {}

// CloseAll does nothing.
func (m *Manager) CloseAll() {}

// Ack does nothing.
func (m *Manager) Ack(id string, offset int64) {}
