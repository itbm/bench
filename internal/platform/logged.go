package platform

import (
	"errors"
	"log/slog"
	"net/url"
	"strings"

	"github.com/itbm/bench/internal/layout"
)

// Logged returns p with every call that changes something, every window it
// reads, and every event it reports logged at debug level. Bench uses it when
// BENCH_DEBUG is set, to see on a real Mac what it asked macOS to do and what
// macOS said back. The returned Platform also forwards Close, SetDemoScreen,
// DemoScreen, MinimiseWindow, AppMoveWindow, StartAgent, StopAgent,
// SetHeldModifiers, and SetSystemKeys when p has them. If p is a DebugLogger, it gets log too. HeldModifiers and MouseHeld
// aren't logged: the hold after the last_tab shortcut asks the first every
// 20 ms, and a drag of a docked window's edge asks the second every 50 ms.
//
// The log leaves out window titles, which can name private documents and
// messages, the token in a VS Code for the Web address (see redactURL),
// which would let anyone who reads the log into the server, and the
// addresses of Web items' pages, which can be private too. A DebugLogger
// names only the host of the page that each of its decisions is about.
func Logged(p Platform, log *slog.Logger) Platform {
	if d, ok := p.(DebugLogger); ok {
		d.SetDebugLog(log)
	}
	l := &logged{Platform: p, log: log, events: make(chan Event, 4096)}
	go l.forward()
	return l
}

// DebugLogger is a Platform that can log decisions of its own at debug
// level, such as what the macOS platform's Web items' web views do with
// each navigation, which no call or event shows.
type DebugLogger interface {
	// SetDebugLog has the platform log its decisions on log.
	SetDebugLog(log *slog.Logger)
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
		if ev.Kind == WebURL {
			attrs = append(attrs, "secure", ev.Page.Secure, "offSite", ev.Page.OffSite)
		}
		if ev.Kind == WebStateChanged {
			attrs = append(attrs, "canGoBack", ev.Web.CanGoBack, "canGoForward", ev.Web.CanGoForward,
				"camera", string(ev.Web.Camera), "microphone", string(ev.Web.Microphone))
		}
		if ev.MouseDown {
			attrs = append(attrs, "mouseDown", true)
		}
		if ev.Kind == WindowDragged || ev.Kind == WindowDropped {
			// A popped-out window's drag names its item, not its window.
			if ev.Window.ID == 0 {
				attrs = append(attrs, "frame", ev.Window.Frame)
			}
			attrs = append(attrs, "start", ev.Start, "pointer", ev.Pointer)
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

func (l *logged) PressClose(w Window) error {
	return l.call(l.Platform.PressClose(w), "PressClose", "window", w.ID, "app", w.BundleID)
}

func (l *logged) SetRaised(raised bool) {
	l.Platform.SetRaised(raised)
	l.log.Debug("SetRaised", "raised", raised)
}

func (l *logged) TrackDrags(on bool) {
	l.Platform.TrackDrags(on)
	l.log.Debug("TrackDrags", "on", on)
}

func (l *logged) WatchDockBadges(on bool) {
	l.Platform.WatchDockBadges(on)
	l.log.Debug("WatchDockBadges", "on", on)
}

// DockBadges logs how many apps show a badge, which Bench asks only when
// that may have changed.
func (l *logged) DockBadges() map[string]string {
	badges := l.Platform.DockBadges()
	l.log.Debug("DockBadges", "apps", len(badges))
	return badges
}

func (l *logged) ShowDropGuide(g DropGuide) {
	l.Platform.ShowDropGuide(g)
	l.log.Debug("ShowDropGuide", "pane", g.Pane, "zone", g.Zone)
}

func (l *logged) HideDropGuide() {
	l.Platform.HideDropGuide()
	l.log.Debug("HideDropGuide")
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

func (l *logged) ReloadWebView(id string) error {
	return l.call(l.Platform.ReloadWebView(id), "ReloadWebView", "item", id)
}

func (l *logged) NavigateWebView(id string, step int) error {
	return l.call(l.Platform.NavigateWebView(id, step), "NavigateWebView", "item", id, "step", step)
}

func (l *logged) ZoomWebView(id string, zoom float64) error {
	return l.call(l.Platform.ZoomWebView(id, zoom), "ZoomWebView", "item", id, "zoom", zoom)
}

// FindInWebView logs how long the text was, not the text, which can be
// private, and whether the page has it.
func (l *logged) FindInWebView(id, text string, backwards bool) (bool, error) {
	found, err := l.Platform.FindInWebView(id, text, backwards)
	return found, l.call(err, "FindInWebView", "item", id, "length", len([]rune(text)), "backwards", backwards, "found", found)
}

func (l *logged) SetCaptureState(id string, device CaptureDevice, state CaptureState) error {
	return l.call(l.Platform.SetCaptureState(id, device, state), "SetCaptureState", "item", id, "device", string(device), "state", string(state))
}

func (l *logged) PauseWebViewMedia(id string) error {
	return l.call(l.Platform.PauseWebViewMedia(id), "PauseWebViewMedia", "item", id)
}

// PageIcon and AppIcon are asked for each time the UI draws an icon that
// isn't cached, so only their failures are logged, other than having none.
func (l *logged) PageIcon(id string) ([]byte, error) {
	png, err := l.Platform.PageIcon(id)
	if err != nil && !errors.Is(err, ErrNoIcon) {
		_ = l.call(err, "PageIcon", "item", id)
	}
	return png, err
}

func (l *logged) AppIcon(bundleID string) ([]byte, error) {
	png, err := l.Platform.AppIcon(bundleID)
	if err != nil && !errors.Is(err, ErrNoIcon) {
		_ = l.call(err, "AppIcon", "bundle", bundleID)
	}
	return png, err
}

func (l *logged) FocusUI() {
	l.Platform.FocusUI()
	l.log.Debug("FocusUI")
}

func (l *logged) OpenTextFile(path string) error {
	return l.call(l.Platform.OpenTextFile(path), "OpenTextFile", "path", path)
}

func (l *logged) ShowInFinder(path string) error {
	return l.call(l.Platform.ShowInFinder(path), "ShowInFinder", "path", path)
}

func (l *logged) OpenSettings(pane SettingsPane) error {
	return l.call(l.Platform.OpenSettings(pane), "OpenSettings", "pane", string(pane))
}

// LoginItemNeedsApproval logs what macOS says, which Bench asks only as
// Open at login changes.
func (l *logged) LoginItemNeedsApproval() bool {
	needs := l.Platform.LoginItemNeedsApproval()
	l.log.Debug("LoginItemNeedsApproval", "needs", needs)
	return needs
}

func (l *logged) ChooseToAdd(start string) (string, error) {
	path, err := l.Platform.ChooseToAdd(start)
	return path, l.call(err, "ChooseToAdd", "start", start, "chosen", path)
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

// MinimiseWindow passes on to the platform underneath (the browser preview's
// demo platform, whose demo bar's Minimise window stands in for Command-M),
// and logs it. A platform without the method refuses it.
func (l *logged) MinimiseWindow(id WindowID) error {
	d, ok := l.Platform.(interface{ MinimiseWindow(WindowID) error })
	if !ok {
		return ErrUnsupported
	}
	return l.call(d.MinimiseWindow(id), "MinimiseWindow", "window", id)
}

// AppMoveWindow passes on to the platform underneath (the browser preview's
// demo platform, whose demo bar's Keep moving window stands in for a window
// tool), and logs it. A platform without the method refuses it.
func (l *logged) AppMoveWindow(id WindowID, frame layout.Rect) error {
	d, ok := l.Platform.(interface {
		AppMoveWindow(WindowID, layout.Rect) error
	})
	if !ok {
		return ErrUnsupported
	}
	return l.call(d.AppMoveWindow(id, frame), "AppMoveWindow", "window", id, "frame", frame)
}

// StartAgent and StopAgent pass on to the platform underneath (the browser
// preview's demo platform, whose demo bar's Rectangle starts and stops a
// window tool), and log it. A platform without them refuses them.
func (l *logged) StartAgent(bundleID string) error {
	d, ok := l.Platform.(interface{ StartAgent(string) error })
	if !ok {
		return ErrUnsupported
	}
	return l.call(d.StartAgent(bundleID), "StartAgent", "app", bundleID)
}

func (l *logged) StopAgent(bundleID string) error {
	d, ok := l.Platform.(interface{ StopAgent(string) error })
	if !ok {
		return ErrUnsupported
	}
	return l.call(d.StopAgent(bundleID), "StopAgent", "app", bundleID)
}

// SetHeldModifiers passes on to the platform underneath (the browser
// preview's demo platform, whose page says which modifier keys it holds), if
// it has the method.
func (l *logged) SetHeldModifiers(m Modifiers) {
	if d, ok := l.Platform.(interface{ SetHeldModifiers(Modifiers) }); ok {
		d.SetHeldModifiers(m)
	}
}

// SetSystemKeys passes on to the platform underneath (the browser preview's
// demo platform, whose demo bar simulates VoiceOver), if it has the method.
func (l *logged) SetSystemKeys(k SystemKeys) {
	if d, ok := l.Platform.(interface{ SetSystemKeys(SystemKeys) }); ok {
		d.SetSystemKeys(k)
	}
}

// SetBadge logs the count, which changes only as items come to need the user,
// or stop needing them.
func (l *logged) SetBadge(count int) {
	l.Platform.SetBadge(count)
	l.log.Debug("SetBadge", "count", count)
}

// ScrollBarsShown logs what macOS says, which Bench asks only at startup
// and when it changes.
func (l *logged) ScrollBarsShown() bool {
	shown := l.Platform.ScrollBarsShown()
	l.log.Debug("ScrollBarsShown", "shown", shown)
	return shown
}

// SystemKeys logs what macOS keeps for itself, which Bench asks only as it
// registers its shortcuts.
func (l *logged) SystemKeys() SystemKeys {
	k := l.Platform.SystemKeys()
	l.log.Debug("SystemKeys", "inputSources", k.InputSources, "voiceOver", k.VoiceOver)
	return k
}

// StageManager logs what macOS says, which Bench asks only at startup and
// when it changes.
func (l *logged) StageManager() bool {
	on := l.Platform.StageManager()
	l.log.Debug("StageManager", "on", on)
	return on
}

// ToolPreferences logs how many preferences of each kind it read, and
// leaves out the rest, which is the tool's business.
func (l *logged) ToolPreferences(bundleID string) ToolPreferences {
	p := l.Platform.ToolPreferences(bundleID)
	l.log.Debug("ToolPreferences", "app", bundleID, "shortcuts", len(p.Shortcuts), "numbers", len(p.Numbers), "texts", len(p.Texts))
	return p
}

// LoggingOut logs what macOS says, which Bench asks only as it's about to
// quit.
func (l *logged) LoggingOut() bool {
	out := l.Platform.LoggingOut()
	l.log.Debug("LoggingOut", "loggingOut", out)
	return out
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
