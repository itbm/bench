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
	// Screen is the screen Bench is on. Windows are parked on it. A screen
	// with no frame means the platform's ScreenOfWindow.
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
