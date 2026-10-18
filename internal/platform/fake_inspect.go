package platform

import (
	"slices"

	"github.com/itbm/bench/internal/layout"
)

// This file has the Fake's inspection methods, for tests to check what Bench
// did. None is logged in Calls.

// FakeWebView is what the Fake knows about a web view.
type FakeWebView struct {
	ID        string
	View      WebView     // From the latest ShowWebView.
	Frame     layout.Rect // From the latest ShowWebView, in content coordinates.
	Visible   bool
	PoppedOut bool
	Title     string // From PopOutWebView.
	Shows     int    // How many times ShowWebView was called for it.
}

// FakePreview is one ShowPreview call.
type FakePreview struct {
	Target PreviewTarget
	Anchor layout.Rect
}

// OpenedURL is a URL that OpenApp was asked to open.
type OpenedURL struct {
	BundleID string
	URL      string
}

// Calls returns the Platform method calls so far, oldest first.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// CallsTo returns the calls to the named methods, oldest first.
func (f *Fake) CallsTo(methods ...string) []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Call
	for _, c := range f.calls {
		if slices.Contains(methods, c.Method) {
			out = append(out, c)
		}
	}
	return out
}

// ResetCalls clears the call log.
func (f *Fake) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

// Frame returns a window's frame, wherever it is, and whether it exists.
func (f *Fake) Frame(id WindowID) (layout.Rect, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw := f.windows[id]
	if fw == nil {
		return layout.Rect{}, false
	}
	return fw.w.Frame, true
}

// WindowState returns a window's state, even on another desktop, and
// whether it exists.
func (f *Fake) WindowState(id WindowID) (Window, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fw := f.windows[id]
	if fw == nil {
		return Window{}, false
	}
	return f.snapshotLocked(fw), true
}

// ZOrder returns every window's ID, front to back.
func (f *Fake) ZOrder() []WindowID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.zorder)
}

// App returns an app as RunningApps would (with PID 0 if it isn't running),
// and whether the Fake knows it.
func (f *Fake) App(bundleID string) (App, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	if a == nil {
		return App{}, false
	}
	return f.appLocked(a), true
}

// IsRunning reports whether an app is running.
func (f *Fake) IsRunning(bundleID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	return a != nil && a.running
}

// IsLaunching reports whether an app has been asked to open and hasn't
// finished launching.
func (f *Fake) IsLaunching(bundleID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	return a != nil && a.launching
}

// IsHidden reports whether a running app is hidden.
func (f *Fake) IsHidden(bundleID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	return a != nil && a.running && a.hidden
}

// ActiveApp returns the frontmost app's bundle ID, or "" if none is.
func (f *Fake) ActiveApp() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active
}

// Raised reports whether Bench's window is raised (from SetRaised).
func (f *Fake) Raised() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.raised
}

// WebViews returns the web views that exist, by ID.
func (f *Fake) WebViews() map[string]FakeWebView {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]FakeWebView, len(f.webViews))
	for id, wv := range f.webViews {
		out[id] = *wv
	}
	return out
}

// Previews returns every ShowPreview call that succeeded, oldest first.
func (f *Fake) Previews() []FakePreview {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.previews)
}

// PreviewShown returns the preview showing now, if there is one.
func (f *Fake) PreviewShown() (FakePreview, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.preview == nil {
		return FakePreview{}, false
	}
	return *f.preview, true
}

// Notifications returns the notifications posted, oldest first.
func (f *Fake) Notifications() []Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.notifications)
}

// OpenedURLs returns the URLs OpenApp was asked to open, oldest first.
func (f *Fake) OpenedURLs() []OpenedURL {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.openedURLs)
}

// TrustPrompts returns how many times Trusted was called with prompt while
// Accessibility permission wasn't granted.
func (f *Fake) TrustPrompts() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.trustPrompts
}

// ScreenRecordingRequests returns how many times ScreenRecordingAllowed was
// called with request while Screen Recording permission wasn't granted.
func (f *Fake) ScreenRecordingRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recordingAsks
}

// Dropped returns how many events were dropped because the Events channel
// was full.
func (f *Fake) Dropped() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dropped
}

// DrainEvents returns the events waiting in the Events channel, oldest
// first, without waiting for more. Tests that read Events themselves
// shouldn't use it at the same time.
func (f *Fake) DrainEvents() []Event {
	var evs []Event
	for {
		select {
		case ev := <-f.events:
			evs = append(evs, ev)
		default:
			return evs
		}
	}
}
