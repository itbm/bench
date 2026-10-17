// Package windows docks, stacks, parks, and hides other apps' windows, using
// a platform.Platform. It decides nothing about which window belongs where;
// the workspace engine works that out and hands it a Plan.
package windows

import (
	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file fixes the package's exported API so that other packages can build
// against it. Delete it once everything is implemented in its own file.

// Item statuses that Apply reports for windows. They match the workspace's
// item statuses.
const (
	StatusReady       = "ready"
	StatusNeedsRoom   = "needs-room"
	StatusFullScreen  = "full-screen"
	StatusUnavailable = "unavailable" // On another desktop.
	StatusMissing     = "missing"     // The window has closed.
)

// Stack is one visible pane's App windows.
type Stack struct {
	PaneID string
	// Body is the pane's body in global coordinates.
	Body layout.Rect
	// Windows are the pane's App windows (excluding undocked ones), in
	// sub-tab order.
	Windows []platform.Window
	// Shown is the window to show on top, or 0 when the pane shows a Web or
	// Built-in item (or nothing), in which case every window in Windows is
	// parked so it doesn't cover Bench's own view.
	Shown platform.WindowID
}

// Plan is what a tab switch or rearrangement should look like.
type Plan struct {
	// Screen is the screen Bench is on. Windows are parked on it.
	Screen platform.Screen
	// Stacks are the visible panes. In each, the inactive windows go in the
	// same frame directly behind the shown one, so switching sub-tabs only
	// raises a window. An inactive window that can't shrink to the body is
	// parked instead.
	Stacks []Stack
	// Park are windows that belong to other tabs, whose apps also have
	// items in this tab.
	Park []platform.Window
	// Hide are apps whose every item is in other tabs.
	Hide []platform.App
	// Unhide are apps with items in this tab. Apply unhides them before
	// docking, if Bench hid them or they're hidden.
	Unhide []platform.App
}

// Result is what Apply did.
type Result struct {
	// Status for every window in Plan.Stacks: StatusReady, or another
	// status from this package.
	Status map[platform.WindowID]string
	// Errors that didn't stop the rest of the plan.
	Errors []error
}

// Manager applies plans and remembers what it did, so that it can tell its
// own moves from the user's, and put everything back on quit.
type Manager struct{}

// New returns a manager. parkedPath is where it records parked windows
// (config.ParkedFile in the support folder), so that RestoreFromFile can put
// them back after a crash; "" means don't record.
func New(p platform.Platform, clk clock.Clock, parkedPath string) *Manager {
	panic("not implemented")
}

// Apply carries out a plan:
//   - Unhide the apps in Unhide.
//   - For each stack: unminimise minimised windows; leave full-screen
//     windows alone (StatusFullScreen); set every window's frame to the
//     body; read the frame back with Platform.Window, and if a window
//     couldn't shrink to fit, park it if it's inactive, or place it at the
//     body's top-left if it's shown (StatusNeedsRoom); then raise the
//     inactive windows and finally the shown one. With Shown == 0, park
//     every window in the stack.
//   - Park the windows in Park, and hide the apps in Hide.
//   - Skip calls that wouldn't change anything (a window already in its
//     frame, an app already hidden), except that raising always happens.
//
// ErrNoWindow gives StatusMissing, and ErrOtherDesktop StatusUnavailable.
func (m *Manager) Apply(plan Plan) Result { panic("not implemented") }

// OwnChange reports whether a WindowMoved or WindowResized event was caused
// by the manager: the window's frame matches (within 2 pt) a frame the
// manager set or parked it at in the last 2 seconds, or the event arrived
// within 250 ms of the manager setting that window's frame.
func (m *Manager) OwnChange(ev platform.Event) bool { panic("not implemented") }

// Docked returns the body frame the manager last docked a window in.
func (m *Manager) Docked(id platform.WindowID) (layout.Rect, bool) { panic("not implemented") }

// Forget drops what the manager knows about a window (it closed, or the user
// dragged it out of its pane). A parked window that's forgotten stays where
// it is.
func (m *Manager) Forget(id platform.WindowID) { panic("not implemented") }

// IsParked reports whether the manager has parked a window.
func (m *Manager) IsParked(id platform.WindowID) bool { panic("not implemented") }

// Unpark moves a parked window back on screen (to where it was before it was
// parked, or else centred on the screen's visible area), for windows that
// leave Bench (pop out, a deleted tab).
func (m *Manager) Unpark(id platform.WindowID) error { panic("not implemented") }

// RestoreAll puts every parked window back on screen, unhides the apps the
// manager hid, and clears the parked-windows file. Bench calls it on quit.
func (m *Manager) RestoreAll() error { panic("not implemented") }

// RestoreFromFile puts back the windows recorded in a parked-windows file
// left by a run that crashed, then removes the file. A missing file is fine.
func RestoreFromFile(p platform.Platform, path string) error { panic("not implemented") }
