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
// changed and WindowMoved if the position changed, with MouseDown set,
// whatever SetEmitOwnMoves says.
func (f *Fake) UserMoveWindow(id WindowID, frame layout.Rect) error {
	return f.moveWindowBy(id, frame, true)
}

// AppMoveWindow moves or resizes a window as its app would by itself, as
// Electron apps often do just after being resized: like UserMoveWindow, but
// without MouseDown.
func (f *Fake) AppMoveWindow(id WindowID, frame layout.Rect) error {
	return f.moveWindowBy(id, frame, false)
}

func (f *Fake) moveWindowBy(id WindowID, frame layout.Rect, mouse bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.liveWindowLocked(id)
	if err != nil {
		return err
	}
	if fw.w.FullScreen {
		return fmt.Errorf("moving window %d: it's in full screen", id)
	}
	f.moveByLocked(fw, frame, true, mouse)
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
	f.minimiseLocked(fw)
	return nil
}

func (f *Fake) minimiseLocked(fw *fakeWindow) {
	if fw.w.Minimised {
		return
	}
	fw.w.Minimised = true
	f.emitWindowLocked(WindowMinimised, fw)
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

// DragWindow drags a window by its title bar, as the user would, to frame,
// with the pointer at pointer: like UserMoveWindow, it moves the window and
// sends WindowMoved (and WindowResized, if its size changed) with MouseDown
// set, and then, while TrackDrags is on, WindowDragged, with Start the
// window's frame as this drag began. A frame of a different size is a drag
// from one of the window's edges. DropWindow ends the drag.
func (f *Fake) DragWindow(id WindowID, frame layout.Rect, pointer Point) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.liveWindowLocked(id)
	if err != nil {
		return err
	}
	if fw.w.FullScreen {
		return fmt.Errorf("dragging window %d: it's in full screen", id)
	}
	start := f.dragStartLocked(windowDragKey(id), fw.w.Frame)
	f.moveByLocked(fw, frame, true, true)
	f.emitDragLocked(WindowDragged, Event{Window: f.snapshotLocked(fw)}, start, pointer)
	return nil
}

// DropWindow lets go of a window that DragWindow dragged, with the pointer
// at pointer, and sends WindowDropped while TrackDrags is on. It returns an
// error for a window that isn't being dragged.
func (f *Fake) DropWindow(id WindowID, pointer Point) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw, err := f.liveWindowLocked(id)
	if err != nil {
		return err
	}
	start, ok := f.drags[windowDragKey(id)]
	if !ok {
		return fmt.Errorf("dropping window %d: it isn't being dragged", id)
	}
	delete(f.drags, windowDragKey(id))
	f.emitDragLocked(WindowDropped, Event{Window: f.snapshotLocked(fw)}, start, pointer)
	return nil
}

// DragPopOut drags one of Bench's own popped-out windows, a web page's or a
// terminal's, by its title bar, to frame, with the pointer at pointer, and
// sends WindowDragged for the item while TrackDrags is on. The Fake doesn't
// keep those windows: the first drag's frame is taken as where the window
// was, unless from says otherwise. DropPopOut ends the drag.
func (f *Fake) DragPopOut(itemID string, from, frame layout.Rect, pointer Point) {
	f.mu.Lock()
	defer f.mu.Unlock()
	start := f.dragStartLocked(itemID, from)
	f.emitDragLocked(WindowDragged, Event{ItemID: itemID, Window: Window{Frame: frame}}, start, pointer)
}

// DropPopOut lets go of a popped-out window that DragPopOut dragged, at
// frame, with the pointer at pointer, and sends WindowDropped while
// TrackDrags is on.
func (f *Fake) DropPopOut(itemID string, frame layout.Rect, pointer Point) {
	f.mu.Lock()
	defer f.mu.Unlock()
	start, ok := f.drags[itemID]
	if !ok {
		start = frame
	}
	delete(f.drags, itemID)
	f.emitDragLocked(WindowDropped, Event{ItemID: itemID, Window: Window{Frame: frame}}, start, pointer)
}

// FocusPopOut sends PopOutFocused for one of Bench's own popped-out windows
// getting the keyboard, as when the user clicks it, or PopOutUnfocused, with
// focused false, for it losing the keyboard.
func (f *Fake) FocusPopOut(itemID string, focused bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kind := PopOutUnfocused
	if focused {
		kind = PopOutFocused
	}
	f.emitLocked(Event{Kind: kind, ItemID: itemID})
}

// PressBackToBench sends DockRequested, as the user clicking Back to Bench
// in a popped-out web page's title bar does.
func (f *Fake) PressBackToBench(itemID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emitLocked(Event{Kind: DockRequested, ItemID: itemID})
}

// SetWebState sets what a web view's page can do and is doing, as when it
// goes to another page, or joins or leaves a call, and sends
// WebStateChanged if that has changed. It returns an error for a web view
// that doesn't exist.
func (f *Fake) SetWebState(id string, s WebState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	wv := f.webViews[id]
	if wv == nil {
		return fmt.Errorf("setting the state of web view %q: there's no such web view", id)
	}
	f.setWebStateLocked(wv, s)
	return nil
}

// setWebStateLocked sets a web view's state, and sends WebStateChanged if
// it has changed.
func (f *Fake) setWebStateLocked(wv *FakeWebView, s WebState) {
	if wv.State == s {
		return
	}
	wv.State = s
	f.emitLocked(Event{Kind: WebStateChanged, ItemID: wv.ID, Web: s})
}

// SetPageText sets the text that FindInWebView finds in a web view's page.
// It returns an error for a web view that doesn't exist.
func (f *Fake) SetPageText(id, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	wv := f.webViews[id]
	if wv == nil {
		return fmt.Errorf("setting the text of web view %q: there's no such web view", id)
	}
	wv.Text = text
	return nil
}

// windowDragKey is the key in Fake.drags of another app's window.
func windowDragKey(id WindowID) string { return fmt.Sprintf("window:%d", id) }

// dragStartLocked returns the frame a drag began with, recording frame as
// that if the drag is new.
func (f *Fake) dragStartLocked(key string, frame layout.Rect) layout.Rect {
	if start, ok := f.drags[key]; ok {
		return start
	}
	if f.drags == nil {
		f.drags = map[string]layout.Rect{}
	}
	f.drags[key] = frame
	return frame
}

// emitDragLocked sends a drag event, while TrackDrags is on. As on a Mac,
// another app's window has only its ID, process, and frame: not its app,
// title, or whether it's in full screen.
func (f *Fake) emitDragLocked(kind EventKind, ev Event, start layout.Rect, pointer Point) {
	if !f.tracking {
		return
	}
	ev.Kind, ev.Start, ev.Pointer = kind, start, pointer
	ev.Window = Window{ID: ev.Window.ID, PID: ev.Window.PID, Frame: ev.Window.Frame}
	f.emitLocked(ev)
}
