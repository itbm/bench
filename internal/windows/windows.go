// Package windows docks, stacks, parks, and hides other apps' windows, using
// a platform.Platform. It decides nothing about which window belongs where;
// the workspace engine works that out and hands it a Plan.
package windows

import (
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// Item statuses that Apply reports for windows. They match the workspace's
// item statuses.
const (
	StatusReady       = "ready"
	StatusNeedsRoom   = "needs-room"
	StatusFullScreen  = "full-screen"
	StatusUnavailable = "unavailable" // On another desktop.
	StatusMissing     = "missing"     // The window has closed.
	// StatusMinimised is for a window in a stack that's minimised, but not
	// by the manager: the user minimised it, so it stays in the Dock.
	StatusMinimised = "minimised"
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
	// parked so it doesn't cover Bench's own view. The others are parked
	// too when Shown is in native full screen, where the pane says so.
	Shown platform.WindowID
	// Restore are windows in Windows to bring back from the Dock if
	// they're minimised: the user has asked to see them. Apply leaves
	// the stack's other windows that are minimised, other than those the
	// manager minimised itself, in the Dock (StatusMinimised).
	Restore []platform.WindowID
}

// Plan is what a tab switch or rearrangement should look like.
type Plan struct {
	// Screen is the screen Bench is on. Windows are parked on it. A screen
	// with no frame means the platform's ScreenOfWindow.
	Screen platform.Screen
	// Stacks are the visible panes. In each, the inactive windows go in the
	// same frame directly behind the shown one, so switching sub-tabs only
	// raises a window. An inactive window that can't shrink to the body is
	// parked instead.
	Stacks []Stack
	// Park are windows to put out of sight whose apps can't be hidden,
	// because they also have windows that should stay: windows that belong
	// to other tabs, whose apps also have items in this tab.
	Park []platform.Window
	// Hide are apps to hide: those whose every item is in other tabs.
	Hide []platform.App
	// Unhide are apps with items in this tab. Apply unhides them before
	// docking, if Bench hid them or they're hidden.
	Unhide []platform.App
	// Unpark are windows that aren't in a stack but should be on screen,
	// such as this tab's undocked windows. Apply puts back the ones the
	// manager has parked, to where they were before, as Unpark does.
	Unpark []platform.Window
}

// Result is what Apply did.
type Result struct {
	// Status for every window in Plan.Stacks: StatusReady, or another
	// status from this package.
	Status map[platform.WindowID]string
	// Errors that didn't stop the rest of the plan.
	Errors []error
	// FileErr is set while the parked-windows file is out of date because
	// it couldn't be written, so windows may be parked with no record to
	// put them back after a crash. Each Apply tries to write it again. A
	// new failure is also in Errors.
	FileErr error
}
