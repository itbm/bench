package platform

import (
	"fmt"

	"github.com/itbm/bench/internal/layout"
)

// This file has the Fake's scripted actions: what the user and other apps
// do. Each sends the events the real platform would, and none is logged in
// Calls.

// LaunchApp starts an installed app straight away, as if the user opened it
// from the Dock, ignoring its launch delay. Like OpenApp, it sends
// AppLaunched, then WindowCreated for each of its launch windows, then
// AppActivated. It returns an error for an app that isn't installed or is
// already running.
func (f *Fake) LaunchApp(bundleID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	switch {
	case a == nil || (!a.installed && !a.running):
		return fmt.Errorf("launching %s: no app with that bundle ID is installed", bundleID)
	case a.running:
		return fmt.Errorf("launching %s: it's already running", bundleID)
	}
	f.launchLocked(a)
	return nil
}

// QuitApp quits a running app: its windows are destroyed (Window returns
// ErrNoWindow for them) and it sends AppTerminated. Like the real platform,
// it doesn't send WindowDestroyed for the windows.
func (f *Fake) QuitApp(bundleID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	if a == nil || !a.running {
		return fmt.Errorf("quitting %s: it isn't running", bundleID)
	}
	app := f.appLocked(a)
	f.stopLocked(a)
	app.Active, app.Hidden = false, false
	f.emitLocked(Event{Kind: AppTerminated, App: app})
	return nil
}

// ActivateApp makes a running app frontmost, as if the user switched to it,
// unhiding it and sending AppActivated if it wasn't frontmost.
func (f *Fake) ActivateApp(bundleID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	if a == nil || !a.running {
		return fmt.Errorf("activating %s: it isn't running", bundleID)
	}
	f.activateLocked(a)
	return nil
}

// OpenWindow opens a window of a running app, in front of the others, as if
// the app or the user did, and sends WindowCreated. It doesn't focus it.
func (f *Fake) OpenWindow(bundleID, title string, opts ...WindowOption) (Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	if a == nil || !a.running {
		return Window{}, fmt.Errorf("opening a window of %s: it isn't running", bundleID)
	}
	fw := f.newWindowLocked(a, title, opts)
	f.emitWindowLocked(WindowCreated, fw)
	return f.snapshotLocked(fw), nil
}

// CloseWindow closes a window and sends WindowDestroyed with its last state.
func (f *Fake) CloseWindow(id WindowID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.anyWindowLocked(id)
	if err != nil {
		return err
	}
	f.removeWindowLocked(id)
	f.emitWindowLocked(WindowDestroyed, fw)
	return nil
}

// RetitleWindow changes a window's title and sends WindowRetitled.
func (f *Fake) RetitleWindow(id WindowID, title string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.anyWindowLocked(id)
	if err != nil {
		return err
	}
	fw.w.Title = title
	f.emitWindowLocked(WindowRetitled, fw)
	return nil
}

// FocusWindow focuses a window as if the user clicked it: like Focus, it
// activates the app (sending AppActivated if it wasn't frontmost), restores
// the window if it's minimised, brings it to the front, and sends
// WindowFocused.
func (f *Fake) FocusWindow(id WindowID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.liveWindowLocked(id)
	if err != nil {
		return err
	}
	f.focusLocked(fw)
	return nil
}

// UserMoveWindow moves or resizes a window as the user would by dragging it,
// as the window accepts it (see WithSizeStep and WithMinSize). It sends WindowResized if the size
// changed and WindowMoved if the position changed, whatever
// SetEmitOwnMoves says.
func (f *Fake) UserMoveWindow(id WindowID, frame layout.Rect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.liveWindowLocked(id)
	if err != nil {
		return err
	}
	if fw.w.FullScreen {
		return fmt.Errorf("moving window %d: it's in full screen", id)
	}
	f.moveLocked(fw, frame, true)
	return nil
}

// MoveToOtherDesktop moves a window to another desktop (Space), as the user
// would from Mission Control. It sends WindowMoved with the window's last
// state; afterwards Windows leaves it out and Window returns
// ErrOtherDesktop.
func (f *Fake) MoveToOtherDesktop(id WindowID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.anyWindowLocked(id)
	if err != nil {
		return err
	}
	if fw.otherDesktop {
		return nil
	}
	fw.otherDesktop = true
	f.emitWindowLocked(WindowMoved, fw)
	return nil
}

// ReturnToDesktop brings a window back from another desktop, and sends
// WindowMoved.
func (f *Fake) ReturnToDesktop(id WindowID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.anyWindowLocked(id)
	if err != nil {
		return err
	}
	if !fw.otherDesktop {
		return nil
	}
	fw.otherDesktop = false
	f.emitWindowLocked(WindowMoved, fw)
	return nil
}

// MinimiseWindow minimises a window as the user would, and sends
// WindowMinimised. It does nothing for a window that's already minimised.
func (f *Fake) MinimiseWindow(id WindowID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.liveWindowLocked(id)
	if err != nil {
		return err
	}
	if fw.w.Minimised {
		return nil
	}
	fw.w.Minimised = true
	f.emitWindowLocked(WindowMinimised, fw)
	return nil
}

// SetFullScreen puts a window into native full screen, filling the screen
// Bench is on, or takes it out again, back to its earlier frame. It sends
// WindowResized when that changes anything.
func (f *Fake) SetFullScreen(id WindowID, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.liveWindowLocked(id)
	if err != nil {
		return err
	}
	if fw.w.FullScreen == on {
		return nil
	}
	if on {
		f.enterFullScreenLocked(fw)
	} else {
		f.leaveFullScreenLocked(fw)
	}
	f.emitWindowLocked(WindowResized, fw)
	return nil
}
