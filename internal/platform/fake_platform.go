package platform

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/itbm/bench/internal/layout"
)

// This file has the Fake's Platform methods. Each logs a Call, then returns
// any error set with SetErr or SetWindowErr before doing anything.

// Screens returns every display.
func (f *Fake) Screens() []Screen {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Screens"})
	return slices.Clone(f.screens)
}

// ScreenOfWindow returns the screen set with SetBenchScreen, or the one
// holding most of the content frame, or the primary screen.
func (f *Fake) ScreenOfWindow() Screen {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "ScreenOfWindow"})
	return f.benchScreenLocked()
}

// ContentFrame returns the frame set with SetContentFrame, or else the
// visible area of the screen Bench is on.
func (f *Fake) ContentFrame() layout.Rect {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "ContentFrame"})
	if f.contentSet {
		return f.content
	}
	return f.benchScreenLocked().Visible
}

// RunningApps returns the running apps, in the order they started.
func (f *Fake) RunningApps() []App {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "RunningApps"})
	var running []*fakeApp
	for _, a := range f.apps {
		if a.running {
			running = append(running, a)
		}
	}
	slices.SortFunc(running, func(a, b *fakeApp) int { return a.seq - b.seq })
	apps := make([]App, len(running))
	for i, a := range running {
		apps[i] = f.appLocked(a)
	}
	return apps
}

// InstalledApps returns the installed apps, sorted by name, ignoring case.
func (f *Fake) InstalledApps() []AppInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "InstalledApps"})
	var infos []AppInfo
	for _, a := range f.apps {
		if a.installed {
			infos = append(infos, a.info)
		}
	}
	slices.SortFunc(infos, func(a, b AppInfo) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.BundleID, b.BundleID)
	})
	return infos
}

// OpenApp activates a running app (unhiding it, and sending AppActivated if
// it wasn't frontmost). If the app has no windows on the current desktop and
// url is empty, it opens one with WindowCreated, titled as NewWindow would
// title it, as most apps do when they're opened again.
//
// An installed app that isn't running launches after its launch delay, by
// the Fake's clock: it sends AppLaunched, opens its launch windows, each with
// WindowCreated, and then becomes frontmost with AppActivated, as the mac
// implementation opens apps with activation on. Its frontmost window becomes
// its focused window, without a WindowFocused event (the real platform
// starts watching a new app too late to see that reliably).
//
// A non-empty url is recorded (see OpenedURLs), and if the app has a window
// for it (AppURLWindow), the app opens it once it's running. OpenApp
// returns an error for an app that's neither installed nor running.
func (f *Fake) OpenApp(bundleID, url string) error {
	f.mu.Lock()
	f.logLocked(Call{Method: "OpenApp", BundleID: bundleID, Arg: url})
	if err := f.errLocked("OpenApp", 0); err != nil {
		f.mu.Unlock()
		return err
	}
	a := f.apps[bundleID]
	if a == nil || (!a.installed && !a.running) {
		f.mu.Unlock()
		return fmt.Errorf("opening %s: no app with that bundle ID is installed", bundleID)
	}
	if url != "" {
		f.openedURLs = appendKeeping(f.openedURLs, OpenedURL{BundleID: bundleID, URL: url}, f.history)
	}
	if a.running {
		f.activateLocked(a)
		switch {
		case url != "":
			f.openURLLocked(a, url)
		case !f.hasWindowsLocked(a):
			f.emitWindowLocked(WindowCreated, f.newWindowLocked(a, a.newTitle(), nil))
		}
		f.mu.Unlock()
		return nil
	}
	if url != "" {
		a.pendingURLs = append(a.pendingURLs, url)
	}
	if a.launching {
		f.mu.Unlock()
		return nil
	}
	a.launching = true
	delay := a.launchDelay
	f.mu.Unlock()

	if delay <= 0 {
		f.finishLaunch(bundleID)
		return nil
	}
	f.clk.AfterFunc(delay, func() { f.finishLaunch(bundleID) })
	return nil
}

// finishLaunch starts a launching app and opens its windows.
func (f *Fake) finishLaunch(bundleID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	if a == nil || !a.launching {
		return
	}
	f.launchLocked(a)
}

// launchLocked starts an app, sending AppLaunched and opening its launch
// windows and any URLs it was asked to open, then makes it frontmost.
func (f *Fake) launchLocked(a *fakeApp) {
	f.startLocked(a)
	f.emitLocked(Event{Kind: AppLaunched, App: f.appLocked(a)})
	for _, title := range a.launchWindows {
		f.emitWindowLocked(WindowCreated, f.newWindowLocked(a, title, nil))
	}
	urls := a.pendingURLs
	a.pendingURLs = nil
	for _, url := range urls {
		f.openURLLocked(a, url)
	}
	f.activateLocked(a)
	a.focused = f.frontWindowLocked(a)
}

// frontWindowLocked returns an app's frontmost window on the current
// desktop, or 0 if it has none.
func (f *Fake) frontWindowLocked(a *fakeApp) WindowID {
	for _, id := range f.zorder {
		if fw := f.windows[id]; fw.w.BundleID == a.bundleID && !fw.otherDesktop {
			return id
		}
	}
	return 0
}

// openURLLocked opens the window an app has for url, if any.
func (f *Fake) openURLLocked(a *fakeApp, url string) {
	if title, ok := a.urlWindows[url]; ok {
		f.emitWindowLocked(WindowCreated, f.newWindowLocked(a, title, nil))
	}
}

// activateLocked makes a running app frontmost and unhides it, sending
// AppActivated if it wasn't frontmost.
func (f *Fake) activateLocked(a *fakeApp) {
	a.hidden = false
	if f.active == a.bundleID {
		return
	}
	f.active = a.bundleID
	f.emitLocked(Event{Kind: AppActivated, App: f.appLocked(a)})
}

// NewWindow opens a standard window of a running app, titled
// DefaultNewWindowTitle (or the app's own, from AppNewWindowTitle), in
// front of the others, and sends WindowCreated.
func (f *Fake) NewWindow(app App) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "NewWindow", BundleID: app.BundleID})
	if err := f.errLocked("NewWindow", 0); err != nil {
		return err
	}
	a, err := f.runningAppLocked(app)
	if err != nil {
		return fmt.Errorf("opening a new window: %w", err)
	}
	f.emitWindowLocked(WindowCreated, f.newWindowLocked(a, a.newTitle(), nil))
	return nil
}

// newTitle is the title of a window NewWindow opens.
func (a *fakeApp) newTitle() string {
	if a.newWindowTitle != "" {
		return a.newWindowTitle
	}
	return DefaultNewWindowTitle
}

// hasWindowsLocked reports whether an app has any windows on the current
// desktop.
func (f *Fake) hasWindowsLocked(a *fakeApp) bool {
	for _, fw := range f.windows {
		if fw.w.BundleID == a.bundleID && !fw.otherDesktop {
			return true
		}
	}
	return false
}

// Hide hides a running app. If it was frontmost, no app is frontmost
// afterwards.
func (f *Fake) Hide(app App) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Hide", BundleID: app.BundleID})
	if err := f.errLocked("Hide", 0); err != nil {
		return err
	}
	a, err := f.runningAppLocked(app)
	if err != nil {
		return fmt.Errorf("hiding an app: %w", err)
	}
	a.hidden = true
	if f.active == a.bundleID {
		f.active = ""
	}
	return nil
}

// Unhide shows a hidden app, without activating it.
func (f *Fake) Unhide(app App) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Unhide", BundleID: app.BundleID})
	if err := f.errLocked("Unhide", 0); err != nil {
		return err
	}
	a, err := f.runningAppLocked(app)
	if err != nil {
		return fmt.Errorf("showing an app: %w", err)
	}
	a.hidden = false
	return nil
}

// Windows returns a running app's windows on the current desktop, in the
// order they were created. Hidden apps' windows, minimised windows, and
// full-screen windows are included.
func (f *Fake) Windows(app App) ([]Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Windows", BundleID: app.BundleID})
	if err := f.errLocked("Windows", 0); err != nil {
		return nil, err
	}
	a, err := f.runningAppLocked(app)
	if err != nil {
		return nil, fmt.Errorf("listing windows: %w", err)
	}
	var ws []Window
	for _, id := range f.windowIDsLocked(a.bundleID) {
		if fw := f.windows[id]; !fw.otherDesktop {
			ws = append(ws, f.snapshotLocked(fw))
		}
	}
	return ws, nil
}

// Window returns a window's current state, ErrNoWindow if it has closed, or
// ErrOtherDesktop if it's on another desktop.
func (f *Fake) Window(id WindowID) (Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Window", Window: id})
	if err := f.errLocked("Window", id); err != nil {
		return Window{}, err
	}
	fw, err := f.liveWindowLocked(id)
	if err != nil {
		return Window{}, err
	}
	return f.snapshotLocked(fw), nil
}

// SetFrame moves and sizes a window, clamping the size up to the window's
// minimum size (and down to whole steps, for a window set up with
// WithSizeStep). It leaves a full-screen window alone. Unless turned off with
// SetEmitOwnMoves, it sends WindowResized if the size changed and
// WindowMoved if the position changed (not for a minimised window).
func (f *Fake) SetFrame(w Window, r layout.Rect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "SetFrame", Window: w.ID, BundleID: w.BundleID, Rect: r})
	if err := f.errLocked("SetFrame", w.ID); err != nil {
		return err
	}
	fw, err := f.liveWindowLocked(w.ID)
	if err != nil {
		return err
	}
	if fw.w.FullScreen {
		return nil
	}
	f.moveLocked(fw, r, f.emitOwnMoves)
	return nil
}

// moveLocked sets a window's frame, as the window accepts it (see
// WithSizeStep and WithMinSize), and sends events for what changed if emit
// is set.
func (f *Fake) moveLocked(fw *fakeWindow, r layout.Rect, emit bool) {
	f.moveByLocked(fw, r, emit, false)
}

// moveByLocked is moveLocked, saying whether the mouse moved the window (a
// person dragging it) in the events it sends.
func (f *Fake) moveByLocked(fw *fakeWindow, r layout.Rect, emit, mouse bool) {
	r = fw.constrain(r)
	old := fw.w.Frame
	fw.w.Frame = r
	if !emit || fw.w.Minimised {
		return
	}
	send := func(kind EventKind) {
		ev := Event{Kind: kind, Window: f.snapshotLocked(fw), MouseDown: mouse}
		if a := f.apps[fw.w.BundleID]; a != nil {
			ev.App = f.appLocked(a)
		}
		f.emitLocked(ev)
	}
	if old.W != r.W || old.H != r.H {
		send(WindowResized)
	}
	if old.X != r.X || old.Y != r.Y {
		send(WindowMoved)
	}
}

// Raise brings a window to the front, without activating its app.
func (f *Fake) Raise(w Window) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Raise", Window: w.ID, BundleID: w.BundleID})
	if err := f.errLocked("Raise", w.ID); err != nil {
		return err
	}
	fw, err := f.liveWindowLocked(w.ID)
	if err != nil {
		return err
	}
	f.raiseLocked(fw.w.ID)
	return nil
}

func (f *Fake) raiseLocked(id WindowID) {
	f.zorder = slices.DeleteFunc(f.zorder, func(z WindowID) bool { return z == id })
	f.zorder = append([]WindowID{id}, f.zorder...)
}

// Park moves a window, keeping its current size, to the frame ParkFrame
// gives for s, as SetFrame would. A screen with no frame means
// ScreenOfWindow, as in the mac implementation. It leaves a full-screen
// window alone. The logged call's Rect is the frame it chose, and its Arg is
// the screen's ID.
func (f *Fake) Park(w Window, s Screen) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.Frame.W <= 0 || s.Frame.H <= 0 {
		s = f.benchScreenLocked()
	}
	call := Call{Method: "Park", Window: w.ID, BundleID: w.BundleID, Arg: s.ID}
	fw, err := f.liveWindowLocked(w.ID)
	if err == nil {
		call.Rect = ParkFrame(f.screens, s, fw.w.Frame)
	}
	f.logLocked(call)
	if injected := f.errLocked("Park", w.ID); injected != nil {
		return injected
	}
	if err != nil {
		return err
	}
	if fw.w.FullScreen {
		return nil
	}
	f.moveLocked(fw, call.Rect, f.emitOwnMoves)
	return nil
}

// Unminimise restores a minimised window and sends WindowRestored. It does
// nothing for a window that isn't minimised.
func (f *Fake) Unminimise(w Window) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Unminimise", Window: w.ID, BundleID: w.BundleID})
	if err := f.errLocked("Unminimise", w.ID); err != nil {
		return err
	}
	fw, err := f.liveWindowLocked(w.ID)
	if err != nil {
		return err
	}
	f.unminimiseLocked(fw)
	return nil
}

func (f *Fake) unminimiseLocked(fw *fakeWindow) {
	if !fw.w.Minimised {
		return
	}
	fw.w.Minimised = false
	f.emitWindowLocked(WindowRestored, fw)
}

// Focus activates a window's app (unhiding it, and sending AppActivated if
// it wasn't frontmost), restores the window if it's minimised, brings it to
// the front, makes it the app's focused window, and sends WindowFocused.
func (f *Fake) Focus(w Window) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Focus", Window: w.ID, BundleID: w.BundleID})
	if err := f.errLocked("Focus", w.ID); err != nil {
		return err
	}
	fw, err := f.liveWindowLocked(w.ID)
	if err != nil {
		return err
	}
	f.focusLocked(fw)
	return nil
}

func (f *Fake) focusLocked(fw *fakeWindow) {
	a := f.apps[fw.w.BundleID]
	if a == nil {
		return
	}
	f.activateLocked(a)
	f.unminimiseLocked(fw)
	f.raiseLocked(fw.w.ID)
	a.focused = fw.w.ID
	f.emitWindowLocked(WindowFocused, fw)
}

// Trusted reports whether Bench has Accessibility permission. A call with
// prompt while it isn't granted counts as a prompt (see TrustPrompts); it
// doesn't grant it.
func (f *Fake) Trusted(prompt bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Trusted", Arg: strconv.FormatBool(prompt)})
	if prompt && !f.trusted {
		f.trustPrompts++
	}
	return f.trusted
}

// ScreenRecordingAllowed reports whether Bench has Screen Recording
// permission. A call with request while it isn't granted counts as a request
// (see ScreenRecordingRequests); it doesn't grant it.
func (f *Fake) ScreenRecordingAllowed(request bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "ScreenRecordingAllowed", Arg: strconv.FormatBool(request)})
	if request && !f.screenRecording {
		f.recordingAsks++
	}
	return f.screenRecording
}

// SetRaised records whether Bench's window is raised (see Raised).
func (f *Fake) SetRaised(raised bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "SetRaised", Arg: strconv.FormatBool(raised)})
	f.raised = raised
}

// ShowWebView records a web view as shown at frame, creating it on first
// use. A later call with a different view updates the recorded view.
func (f *Fake) ShowWebView(id string, view WebView, frame layout.Rect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "ShowWebView", Rect: frame, Arg: id})
	if err := f.errLocked("ShowWebView", 0); err != nil {
		return err
	}
	wv := f.webViews[id]
	if wv == nil {
		wv = &FakeWebView{ID: id}
		f.webViews[id] = wv
	}
	wv.View = view
	wv.Frame = frame
	wv.Visible = true
	wv.Shows++
	return nil
}

// HideWebView records a web view as hidden. It does nothing for a web view
// that doesn't exist.
func (f *Fake) HideWebView(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "HideWebView", Arg: id})
	if wv := f.webViews[id]; wv != nil {
		wv.Visible = false
	}
}

// CloseWebView destroys a web view, so WebViews no longer lists it.
func (f *Fake) CloseWebView(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "CloseWebView", Arg: id})
	delete(f.webViews, id)
}

// PopOutWebView records a web view as in a window of its own, with title.
// It returns an error for a web view that doesn't exist.
func (f *Fake) PopOutWebView(id, title string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "PopOutWebView", Arg: id})
	if err := f.errLocked("PopOutWebView", 0); err != nil {
		return err
	}
	wv := f.webViews[id]
	if wv == nil {
		return fmt.Errorf("popping out web view %q: there's no such web view", id)
	}
	wv.PoppedOut = true
	wv.Title = title
	wv.Visible = true
	return nil
}

// DockWebView records a popped-out web view as back in Bench's window, and
// hidden. It returns an error for a web view that doesn't exist.
func (f *Fake) DockWebView(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "DockWebView", Arg: id})
	if err := f.errLocked("DockWebView", 0); err != nil {
		return err
	}
	wv := f.webViews[id]
	if wv == nil {
		return fmt.Errorf("docking web view %q: there's no such web view", id)
	}
	wv.PoppedOut = false
	wv.Visible = false
	return nil
}

// FocusWebView records that Bench gave a web view the keyboard focus (see
// FocusedWebView). Bench comes forward, so no other app is frontmost. It
// returns an error for a web view that doesn't exist.
func (f *Fake) FocusWebView(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "FocusWebView", Arg: id})
	if err := f.errLocked("FocusWebView", 0); err != nil {
		return err
	}
	if f.webViews[id] == nil {
		return fmt.Errorf("focusing web view %q: there's no such web view", id)
	}
	f.focusedWebView = id
	f.active = ""
	return nil
}

// FocusedWebView returns the web view last given the keyboard focus, or "".
func (f *Fake) FocusedWebView() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.focusedWebView
}

// ShowPreview records a preview as shown (see Previews and PreviewShown).
// It returns ErrNoWindow for a window that doesn't exist, and an error for a
// web view that doesn't exist. It doesn't check Screen Recording
// permission.
func (f *Fake) ShowPreview(target PreviewTarget, anchor layout.Rect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "ShowPreview", Window: target.Window, Rect: anchor, Arg: target.WebViewID})
	if err := f.errLocked("ShowPreview", target.Window); err != nil {
		return err
	}
	if target.Window != 0 {
		if _, err := f.liveWindowLocked(target.Window); err != nil {
			return err
		}
	}
	if target.WebViewID != "" && f.webViews[target.WebViewID] == nil {
		return fmt.Errorf("previewing web view %q: there's no such web view", target.WebViewID)
	}
	p := FakePreview{Target: target, Anchor: anchor}
	f.previews = appendKeeping(f.previews, p, f.history)
	f.preview = &p
	return nil
}

// HidePreview records the preview as hidden.
func (f *Fake) HidePreview() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "HidePreview"})
	f.preview = nil
}

// Notify records a notification (see Notifications).
func (f *Fake) Notify(n Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Notify", Arg: n.ItemID})
	if err := f.errLocked("Notify", 0); err != nil {
		return err
	}
	f.notifications = appendKeeping(f.notifications, n, f.history)
	return nil
}

// Events returns the Fake's event channel, which holds up to
// FakeEventBuffer events and is never closed.
func (f *Fake) Events() <-chan Event { return f.events }

// Minimise minimises a window and sends WindowMinimised. It does nothing for
// a window that's already minimised.
func (f *Fake) Minimise(w Window) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "Minimise", Window: w.ID, BundleID: w.BundleID})
	if err := f.errLocked("Minimise", w.ID); err != nil {
		return err
	}
	fw, err := f.liveWindowLocked(w.ID)
	if err != nil {
		return err
	}
	f.minimiseLocked(fw)
	return nil
}

// FocusUI records that Bench gave its own UI the keyboard focus: no web view
// has it, and no other app is frontmost.
func (f *Fake) FocusUI() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logLocked(Call{Method: "FocusUI"})
	f.focusedWebView = ""
	f.active = ""
}
