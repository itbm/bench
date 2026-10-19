package platform

import (
	"log/slog"
	"net/url"
	"strings"

	"github.com/itbm/bench/internal/layout"
)

// Logged returns p with every call that changes something, every window it
// reads, and every event it reports logged at debug level. Bench uses it when
// BENCH_DEBUG is set, to see on a real Mac what it asked macOS to do and what
// macOS said back. The returned Platform also forwards Close, SetDemoScreen,
// and DemoScreen when p has them.
//
// The log leaves out window titles, which can name private documents and
// messages, and the token in a VS Code for the Web address (see redactURL),
// which would let anyone who reads the log into the server.
func Logged(p Platform, log *slog.Logger) Platform {
	l := &logged{Platform: p, log: log, events: make(chan Event, 4096)}
	go l.forward()
	return l
}

type logged struct {
	Platform
	log    *slog.Logger
	events chan Event
}

// forward logs each event and passes it on. The channel it reads is never
// closed while Bench runs, and neither is the one it writes.
func (l *logged) forward() {
	for ev := range l.Platform.Events() {
		attrs := []any{"kind", ev.Kind}
		if ev.Window.ID != 0 {
			attrs = append(attrs, "window", ev.Window.ID, "app", ev.Window.BundleID,
				"frame", ev.Window.Frame)
		}
		if ev.App.BundleID != "" {
			attrs = append(attrs, "pid", ev.App.PID, "bundle", ev.App.BundleID)
		}
		if ev.ItemID != "" {
			attrs = append(attrs, "item", ev.ItemID)
		}
		if ev.MouseDown {
			attrs = append(attrs, "mouseDown", true)
		}
		l.log.Debug("event", attrs...)
		l.events <- ev
	}
}

func (l *logged) Events() <-chan Event { return l.events }

// call logs a call and its error, if any.
func (l *logged) call(err error, method string, attrs ...any) error {
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	l.log.Debug(method, attrs...)
	return err
}

func (l *logged) OpenApp(bundleID, url string) error {
	return l.call(l.Platform.OpenApp(bundleID, url), "OpenApp", "bundle", bundleID, "url", redactURL(url))
}

func (l *logged) NewWindow(app App) error {
	return l.call(l.Platform.NewWindow(app), "NewWindow", "bundle", app.BundleID)
}

func (l *logged) Hide(app App) error {
	return l.call(l.Platform.Hide(app), "Hide", "bundle", app.BundleID)
}

func (l *logged) Unhide(app App) error {
	return l.call(l.Platform.Unhide(app), "Unhide", "bundle", app.BundleID)
}

func (l *logged) Window(id WindowID) (Window, error) {
	w, err := l.Platform.Window(id)
	return w, l.call(err, "Window", "window", id, "frame", w.Frame, "minimised", w.Minimised, "fullScreen", w.FullScreen)
}

func (l *logged) SetFrame(w Window, r layout.Rect) error {
	return l.call(l.Platform.SetFrame(w, r), "SetFrame", "window", w.ID, "app", w.BundleID, "to", r)
}

func (l *logged) Raise(w Window) error {
	return l.call(l.Platform.Raise(w), "Raise", "window", w.ID, "app", w.BundleID)
}

func (l *logged) Park(w Window, s Screen) error {
	return l.call(l.Platform.Park(w, s), "Park", "window", w.ID, "app", w.BundleID, "screen", s.ID)
}

func (l *logged) Unminimise(w Window) error {
	return l.call(l.Platform.Unminimise(w), "Unminimise", "window", w.ID)
}

func (l *logged) Focus(w Window) error {
	return l.call(l.Platform.Focus(w), "Focus", "window", w.ID, "app", w.BundleID)
}

func (l *logged) SetRaised(raised bool) {
	l.Platform.SetRaised(raised)
	l.log.Debug("SetRaised", "raised", raised)
}

func (l *logged) ShowWebView(id string, view WebView, frame layout.Rect) error {
	return l.call(l.Platform.ShowWebView(id, view, frame), "ShowWebView", "item", id, "url", redactURL(view.URL), "frame", frame)
}

func (l *logged) HideWebView(id string) {
	l.Platform.HideWebView(id)
	l.log.Debug("HideWebView", "item", id)
}

func (l *logged) PopOutWebView(id, title string) error {
	return l.call(l.Platform.PopOutWebView(id, title), "PopOutWebView", "item", id)
}

func (l *logged) DockWebView(id string) error {
	return l.call(l.Platform.DockWebView(id), "DockWebView", "item", id)
}

func (l *logged) FocusWebView(id string) error {
	return l.call(l.Platform.FocusWebView(id), "FocusWebView", "item", id)
}

func (l *logged) FocusUI() {
	l.Platform.FocusUI()
	l.log.Debug("FocusUI")
}

// Close closes the platform underneath, if it can be closed.
func (l *logged) Close() {
	if c, ok := l.Platform.(interface{ Close() }); ok {
		c.Close()
	}
}

// SetDemoScreen passes on to the platform underneath (the browser preview's
// demo platform), if it has the method.
func (l *logged) SetDemoScreen(name string, fitWidth float64) {
	if d, ok := l.Platform.(interface{ SetDemoScreen(string, float64) }); ok {
		d.SetDemoScreen(name, fitWidth)
	}
}

// DemoScreen returns the simulated screen of the platform underneath (the
// browser preview's demo platform), or "" if it has none, so that the
// engine still finds it through DemoScreens.
func (l *logged) DemoScreen() string {
	if d, ok := l.Platform.(interface{ DemoScreen() string }); ok {
		return d.DemoScreen()
	}
	return ""
}

func (l *logged) Minimise(w Window) error {
	return l.call(l.Platform.Minimise(w), "Minimise", "window", w.ID, "app", w.BundleID)
}

// redactURL returns an address for the log, with the value of any tkn query
// parameter replaced. That parameter is VS Code for the Web's connection
// token, the only thing that keeps other local users out of the server,
// and the log may be in a file that anyone can read. An address whose query
// doesn't parse loses all of it, in case the token is in there.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		before, _, _ := strings.Cut(raw, "?")
		return before
	}
	if u.RawQuery == "" {
		return raw
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		u.RawQuery = ""
		return u.String()
	}
	if !q.Has("tkn") {
		return raw
	}
	q.Set("tkn", "redacted")
	u.RawQuery = q.Encode()
	return u.String()
}
