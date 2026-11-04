// Package platform describes what Bench needs from macOS, so that everything
// else can be built and tested on any OS. The real implementation is in
// internal/mac; Fake (fake.go) stands in for it in tests, and NewDemo
// (demo.go) seeds a Fake with the prototype's apps for the browser preview.
//
// Coordinates are global, in points, with the origin at the top-left of the
// primary display and y increasing downwards (the Accessibility API's
// convention), except where a method says it uses Bench's window content
// coordinates (top-left of the window's content area, y downwards).
package platform

import (
	"errors"
	"time"

	"github.com/itbm/bench/internal/layout"
)

// WindowID is a window's CGWindowID. It stays the same while the window
// exists, even as its title changes.
type WindowID uint32

// Accessibility subroles that matter to Bench. Only standard windows become
// items; the rest stay floating above Bench.
const (
	SubroleStandard     = "AXStandardWindow"
	SubroleDialog       = "AXDialog"
	SubroleSystemDialog = "AXSystemDialog"
	SubroleFloating     = "AXFloatingWindow"
)

// Screen is a display.
type Screen struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Frame   layout.Rect `json:"frame"`   // The whole display.
	Visible layout.Rect `json:"visible"` // Without the menu bar and the Dock.
	Primary bool        `json:"primary"`
}

// App is a running app.
type App struct {
	PID      int    `json:"pid"`
	BundleID string `json:"bundleId"`
	Name     string `json:"name"`
	Hidden   bool   `json:"hidden"`
	Active   bool   `json:"active"` // Frontmost.
	// Regular apps appear in the Dock. Bench ignores the others (agents,
	// menu bar extras, background helpers).
	Regular bool `json:"regular"`
}

// AppInfo is an installed app.
type AppInfo struct {
	BundleID string `json:"bundleId"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	// WebApp names the browser that made the app from a website, such as
	// "Safari" or "Chrome", or is "" for an ordinary app.
	WebApp string `json:"webApp,omitempty"`
	// WebURL is the address a web app opens, when its browser records one,
	// or "".
	WebURL string `json:"webUrl,omitempty"`
}

// Window is another app's window, as the Accessibility API reports it.
type Window struct {
	ID         WindowID    `json:"id"`
	PID        int         `json:"pid"`
	BundleID   string      `json:"bundleId"`
	App        string      `json:"app"` // The app's name.
	Title      string      `json:"title"`
	Subrole    string      `json:"subrole"`
	Frame      layout.Rect `json:"frame"`
	Minimised  bool        `json:"minimised"`
	FullScreen bool        `json:"fullScreen"`
	// Focused is true for the focused window of the frontmost app.
	Focused bool `json:"focused"`
	// Created is the order in which windows were first seen, starting at 1.
	// It lets Bench match windows to remembered items after an app restarts.
	Created int `json:"created"`
}

// Standard reports whether w is a standard window, which can become an item.
func (w Window) Standard() bool { return w.Subrole == SubroleStandard }

// WebView describes a Web item's web view.
type WebView struct {
	URL string `json:"url"`
	// Name is the item's name, such as "Slack", which the prompts that its
	// pages bring up, such as for the camera, use to say which item asks.
	Name string `json:"name,omitempty"`
	// Profile names the persistent data store. Web views with the same
	// profile share logins. The mac implementation derives a stable,
	// name-based UUID from it for +[WKWebsiteDataStore dataStoreForIdentifier:].
	Profile string `json:"profile"`
	// Zoom is the page's zoom, as a scale such as 1.25 for 125%, or 0 for
	// actual size (1). ZoomWebView changes it in a view that exists.
	Zoom float64 `json:"zoom,omitempty"`
}

// CaptureState is how a Web item's page uses the camera, or the
// microphone: not at all, live, or muted (the camera turned off), as
// WebKit's cameraCaptureState and microphoneCaptureState say.
type CaptureState string

// The capture states.
const (
	CaptureNone   CaptureState = ""
	CaptureActive CaptureState = "active"
	CaptureMuted  CaptureState = "muted"
)

// CaptureDevice is the camera or the microphone, for SetCaptureState.
type CaptureDevice string

// The capture devices.
const (
	Camera     CaptureDevice = "camera"
	Microphone CaptureDevice = "microphone"
)

// WebState is what a Web item's page can do and is doing now, for a
// WebStateChanged event: whether its view can go back or forward in its
// history, and how the page uses the camera and the microphone.
type WebState struct {
	CanGoBack    bool         `json:"canGoBack,omitempty"`
	CanGoForward bool         `json:"canGoForward,omitempty"`
	Camera       CaptureState `json:"camera,omitempty"`
	Microphone   CaptureState `json:"microphone,omitempty"`
}

// PreviewTarget is what a live preview shows: a window, or a Web item's view.
type PreviewTarget struct {
	Window    WindowID `json:"window,omitempty"`
	WebViewID string   `json:"webView,omitempty"`
	Title     string   `json:"title"`
}

// Point is a point in global coordinates.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// DropGuide is what the drop guide shows over one of Bench's panes while
// the user drags a window there (ShowDropGuide): the pane's outline, a
// target in its middle and, if it can be split, one at each edge, and the
// part of the pane the window would take, with a label. Rectangles are in
// global coordinates.
type DropGuide struct {
	// Pane is the pane's frame, its header included, and Body the part
	// below the header, which the pane's window fills.
	Pane layout.Rect `json:"pane"`
	Body layout.Rect `json:"body"`
	// Target is where the window would go: the pane's body for the
	// middle, or the half of it on the side of an edge, which a split
	// would give it.
	Target layout.Rect `json:"target"`
	// Zone is the target the pointer is on: "center", "left", "right",
	// "top", or "bottom" (layout.Zone).
	Zone string `json:"zone"`
	// Splits is set when the pane can be split, so that its edges are
	// targets too.
	Splits bool `json:"splits"`
	// Label says what dropping the window there does, such as "Add to
	// this pane".
	Label string `json:"label"`
}

// Page describes the page a Web item's view shows, for a WebURL event. A
// blob: page is described by the origin inside its address, such as
// https://app.slack.com for blob:https://app.slack.com/1234. The zero Page
// is no page at all.
type Page struct {
	// Host is the page's host, such as app.slack.com, or "" for a page with
	// no web address, such as about:blank or a data: page.
	Host string `json:"host,omitempty"`
	// Site is the end of Host that names its site: its registrable domain,
	// such as slack.com, or the whole host for an IP address or a local
	// one.
	Site string `json:"site,omitempty"`
	// Secure is set for an https page whose content all came over secure
	// connections, and for a page on the Mac itself (localhost, or a
	// loopback address such as 127.0.0.1). A page with no web address is
	// secure if WebKit says all its content is.
	Secure bool `json:"secure,omitempty"`
	// OffSite is set for a page on another site than the item's own
	// address, and not one related to it, and for a page with no web
	// address, which could be anyone's.
	OffSite bool `json:"offSite,omitempty"`
}

// Notification is a notification posted for an item: for a Web item, what
// its page posted, and for a terminal, what its program asked for, or that
// it rang the bell or finished.
type Notification struct {
	ItemID string `json:"itemId"`
	Title  string `json:"title"`
	// Subtitle says where the item is, such as its tab's name. A Web
	// item's notification has the site of its page instead, which only
	// pages of the item's own site can post from, as the title and body
	// are whatever the page says.
	Subtitle string `json:"subtitle,omitempty"`
	Body     string `json:"body"`
}

// SystemKeys are keys that macOS keeps for itself without saying so: Bench
// can register a global shortcut that one of them uses, but the key press
// may never reach it.
type SystemKeys struct {
	// InputSources are the shortcuts that switch input sources, in
	// canonical form (shortcuts.Parse), such as "ctrl+space" and
	// "ctrl+alt+space": those turned on in System Settings ▸ Keyboard ▸
	// Keyboard Shortcuts ▸ Input Sources, while more than one input source
	// is turned on, since they do nothing otherwise.
	InputSources []string `json:"inputSources,omitempty"`
	// VoiceOver is set while VoiceOver is on. It uses Control and Option,
	// its VO keys, with every key.
	VoiceOver bool `json:"voiceOver,omitempty"`
}

// Modifiers is a set of modifier keys, as HeldModifiers reports them.
type Modifiers uint8

// The modifier keys.
const (
	ModControl Modifiers = 1 << iota
	ModOption
	ModShift
	ModCommand
)

// ModifiersNamed returns the modifier keys named as shortcuts.Modifiers
// names them: "ctrl", "alt", "shift", and "cmd". It ignores other names.
func ModifiersNamed(names []string) Modifiers {
	var m Modifiers
	for _, n := range names {
		switch n {
		case "ctrl":
			m |= ModControl
		case "alt":
			m |= ModOption
		case "shift":
			m |= ModShift
		case "cmd":
			m |= ModCommand
		}
	}
	return m
}

// SettingsPane is a pane of System Settings that OpenSettings opens.
type SettingsPane string

// The panes that OpenSettings opens.
const (
	// SettingsAccessibility is Privacy & Security ▸ Accessibility.
	SettingsAccessibility SettingsPane = "accessibility"
	// SettingsScreenRecording is Privacy & Security ▸ Screen Recording
	// (Screen & System Audio Recording in macOS 15).
	SettingsScreenRecording SettingsPane = "screen-recording"
	// SettingsLoginItems is General ▸ Login Items (Login Items &
	// Extensions in macOS 15).
	SettingsLoginItems SettingsPane = "login-items"
)

// EventKind says what happened.
type EventKind string

const (
	WindowCreated EventKind = "window-created"
	// WindowDestroyed: a window closed while its app kept running. When an
	// app quits, only AppTerminated is sent, so its items can wait for it to
	// open again.
	WindowDestroyed EventKind = "window-destroyed"
	WindowMoved     EventKind = "window-moved"
	WindowResized   EventKind = "window-resized"
	// WindowFocused: a window became the focused window of the frontmost app.
	WindowFocused   EventKind = "window-focused"
	WindowRetitled  EventKind = "window-retitled"
	WindowMinimised EventKind = "window-minimised"
	WindowRestored  EventKind = "window-restored" // Unminimised.

	AppLaunched   EventKind = "app-launched"
	AppTerminated EventKind = "app-terminated"
	AppActivated  EventKind = "app-activated"
	// LoggingOut: the user chose Log Out, Restart, or Shut Down, so the
	// apps that quit next are quitting because of it, not because the user
	// quit them (NSWorkspaceWillPowerOffNotification). If an app cancels
	// it, nothing says so: see LogOutGrace.
	LoggingOut EventKind = "logging-out"

	// ScreensChanged: a display was connected, disconnected, or rearranged.
	ScreensChanged EventKind = "screens-changed"
	// Woke: the Mac woke from sleep (NSWorkspaceDidWakeNotification).
	// ScreensWoke: the displays woke from sleep
	// (NSWorkspaceScreensDidWakeNotification). SessionActive: the user's
	// session became active again, as when they switch back to it with
	// fast user switching (NSWorkspaceSessionDidBecomeActiveNotification).
	// ScreenUnlocked: the user unlocked the screen (the
	// com.apple.screenIsUnlocked distributed notification, which Apple
	// doesn't document, so it may not come: ScreensWoke and SessionActive
	// come at the same times). macOS can move windows at each of these, as
	// displays come back.
	Woke           EventKind = "woke"
	ScreensWoke    EventKind = "screens-woke"
	SessionActive  EventKind = "session-active"
	ScreenUnlocked EventKind = "screen-unlocked"
	// DesktopChanged: the user went to another desktop (Space)
	// (NSWorkspaceActiveSpaceDidChangeNotification), and Bench's window is
	// on a desktop that's shown. With "Displays have separate Spaces" on,
	// as it is by default, that includes a change on another display,
	// while Bench's window stays where it was. It isn't sent while Bench's
	// window is on a desktop that isn't shown, or is hidden or minimised.
	DesktopChanged EventKind = "desktop-changed"
	// BenchMoved: Bench's window moved, resized, or changed screen. The mac
	// implementation throttles these to one per frame.
	BenchMoved EventKind = "bench-moved"

	// WebTitle: a Web item's page title changed (ItemID, Title). WebURL: a
	// new page came into view, the page changed its address itself (with
	// pushState or a link to a part of the page), or it loaded content that
	// isn't secure (ItemID, Body holds the address, Page describes the
	// page). It's sent for pages that have come into view, not those still
	// loading.
	WebTitle EventKind = "web-title"
	WebURL   EventKind = "web-url"
	// WebNotification: a Web item's page posted a notification (ItemID,
	// Title, Body). The engine decides whether to Notify.
	WebNotification EventKind = "web-notification"
	// NotificationClicked: the user clicked a notification (ItemID).
	NotificationClicked EventKind = "notification-clicked"
	// WebViewClosed: the user closed a popped-out Web item's window (ItemID).
	WebViewClosed EventKind = "web-view-closed"
	// WebFocused: the user clicked inside a Web item's web view in Bench's
	// window (ItemID). The click goes to the page, not to Bench's UI, so
	// this is how its pane gets focused.
	WebFocused EventKind = "web-focused"
	// WebStateChanged: whether a Web item's view can go back or forward,
	// or how its page uses the camera or microphone, changed (ItemID, Web,
	// with the whole state). It's sent for a popped-out view too.
	WebStateChanged EventKind = "web-state-changed"
	// PageIconChanged: the icon of a Web item's page changed (ItemID, and
	// HasIcon, whether it has one now, which PageIcon returns).
	PageIconChanged EventKind = "page-icon-changed"

	// BenchHidden: Bench's window was hidden (Bench > Hide) or minimised,
	// so its docked windows should go too. BenchShown: it came back.
	BenchHidden EventKind = "bench-hidden"
	BenchShown  EventKind = "bench-shown"

	// ScrollBarsChanged: macOS has changed whether it shows scroll bars
	// all the time (see ScrollBarsShown), as when the user changes System
	// Settings, or connects or disconnects a mouse.
	ScrollBarsChanged EventKind = "scroll-bars-changed"

	// SystemKeysChanged: what SystemKeys reports may have changed, as when
	// VoiceOver is turned on or off, or Bench comes forward after the user
	// may have changed the input sources' shortcuts in System Settings.
	SystemKeysChanged EventKind = "system-keys-changed"

	// StageManagerChanged: Stage Manager was turned on or off (see
	// StageManager).
	StageManagerChanged EventKind = "stage-manager-changed"

	// DockBadgesChanged: while WatchDockBadges is on, the badges that apps
	// show on their Dock icons changed (see DockBadges).
	DockBadgesChanged EventKind = "dock-badges-changed"

	// WindowDragged: while TrackDrags is on, the user is dragging a window
	// by its title bar: another app's (Window, with only its ID, PID, and
	// frame now: not whether it's in full screen or minimised), or one
	// of Bench's own popped-out windows, a web page's or a terminal's
	// (ItemID, with its frame in Window.Frame). Start is the window's
	// frame as the drag began, and Pointer where the pointer is. It's sent
	// as the pointer moves, and a few times a second while it rests, for
	// as long as the button is held. WindowDropped: the user let go of it,
	// with the same fields. A drag that resizes the window, from an edge,
	// is reported too: its size differs from Start's.
	WindowDragged EventKind = "window-dragged"
	WindowDropped EventKind = "window-dropped"

	// PopOutFocused: one of Bench's own popped-out windows, a web page's
	// or a terminal's, got the keyboard (ItemID). PopOutUnfocused: it lost
	// it.
	PopOutFocused   EventKind = "pop-out-focused"
	PopOutUnfocused EventKind = "pop-out-unfocused"
	// DockRequested: the user clicked Back to Bench in the title bar of a
	// popped-out web page's window (ItemID).
	DockRequested EventKind = "dock-requested"
)

// Event is something that happened on the platform. Which fields are set
// depends on Kind.
type Event struct {
	Kind   EventKind `json:"kind"`
	Window Window    `json:"window"`
	App    App       `json:"app"`
	ItemID string    `json:"itemId,omitempty"`
	Title  string    `json:"title,omitempty"`
	Body   string    `json:"body,omitempty"`
	// Page is set for WebURL.
	Page Page `json:"page"`
	// Web is set for WebStateChanged.
	Web WebState `json:"web,omitzero"`
	// HasIcon is set for PageIconChanged when the page has an icon.
	HasIcon bool `json:"hasIcon,omitempty"`
	// MouseDown is set for WindowMoved and WindowResized when a mouse button
	// was held with the pointer on the window as it changed: a person
	// dragging it. Apps that move or resize their own windows (Electron apps
	// often do, just after being resized) don't hold the mouse there.
	MouseDown bool `json:"mouseDown,omitempty"`
	// Start and Pointer are set for WindowDragged and WindowDropped: the
	// window's frame as the drag began, and where the pointer is.
	Start   layout.Rect `json:"start,omitzero"`
	Pointer Point       `json:"pointer,omitzero"`
}

// LogOutGrace is how long after macOS says the user is logging out,
// restarting, or shutting down (LoggingOut), Bench takes it that they are.
// Apps can take a while to quit, asking to save their documents, and if one
// cancels the log-out, nothing says so.
const LogOutGrace = 5 * time.Minute

// Errors that callers act on.
var (
	// ErrNoWindow: the window doesn't exist any more.
	ErrNoWindow = errors.New("window not found")
	// ErrOtherDesktop: the window exists but is on another desktop (Space),
	// where the Accessibility API can't see it.
	ErrOtherDesktop = errors.New("window is on another desktop")
	// ErrNotTrusted: Bench doesn't have Accessibility permission.
	ErrNotTrusted = errors.New("Bench needs Accessibility permission")
	// ErrNotInstalled: macOS knows no app with that bundle ID.
	ErrNotInstalled = errors.New("no app with that bundle ID is installed")
	// ErrUnsupported: this platform can't do that.
	ErrUnsupported = errors.New("not supported on this platform")
	// ErrNoIcon: there's no icon to give, for an app or a page.
	ErrNoIcon = errors.New("no icon")
	// ErrBusy: the platform is already doing that for the user, such as
	// showing the open panel that ChooseToAdd shows.
	ErrBusy = errors.New("already in progress")
)

// Platform is what Bench needs from macOS.
//
// Methods may be called from any goroutine. Implementations must not call
// back into Bench synchronously: they report what happens through Events,
// which is a buffered channel that's never closed while Bench runs.
type Platform interface {
	// Screens returns every display.
	Screens() []Screen
	// ScreenOfWindow returns the screen Bench's window is on (the one holding
	// most of it), or the primary screen if there's no window.
	ScreenOfWindow() Screen
	// ContentFrame returns Bench's window content area in global
	// coordinates, or a zero Rect if Bench has no window (bench arrange).
	ContentFrame() layout.Rect

	// RunningApps returns the running apps, apart from Bench itself.
	RunningApps() []App
	// OwnAppHidden reports whether Bench itself is hidden, as when another
	// app's Hide Others hides it.
	OwnAppHidden() bool
	// LoggingOut reports whether the user is logging out, restarting, or
	// shutting down, for Bench to quit then without asking anything: macOS
	// said so (a LoggingOut event) less than LogOutGrace ago, or, when it's
	// asked on the main thread while macOS asks Bench to quit, the quit
	// says so (its Apple Event's quit reason). It never waits for the main
	// thread. The browser preview's platform reports false.
	LoggingOut() bool
	// InstalledApps returns every installed app, from /Applications,
	// ~/Applications, and /System/Applications and their Utilities folders,
	// and the other folders in /Applications, one level down.
	InstalledApps() []AppInfo
	// OpenApp launches an app if it isn't running, or activates it, and
	// opens url in it if url isn't empty. It returns an error wrapping
	// ErrNotInstalled for an app that macOS doesn't know.
	OpenApp(bundleID, url string) error
	// DefaultBrowser returns the bundle ID of the app that opens https
	// links, or "" if there's none, or it's an agent (such as a browser
	// picker) that opens no windows of its own.
	DefaultBrowser() string
	// NewWindow opens a new window of a running app: it presses the app's
	// New Window menu item, or if there's none, activates the app and opens
	// it again, which makes most apps open a window if they have none.
	NewWindow(app App) error
	// Hide and Unhide hide and show an app (NSRunningApplication).
	Hide(app App) error
	Unhide(app App) error

	// Windows returns an app's windows on the current desktop: standard ones
	// and floating ones, with IDs, titles, and subroles.
	Windows(app App) ([]Window, error)
	// Window returns a window's current state. It returns ErrNoWindow if the
	// window has closed, or ErrOtherDesktop if it's on another desktop. With
	// ErrOtherDesktop, the Window returned may name the window's app (its ID,
	// PID, BundleID, and App), so that callers can tell whether the window is
	// still the one they knew; a zero BundleID means the app isn't known.
	Window(id WindowID) (Window, error)
	// SetFrame moves and sizes a window: size, then position, then size
	// again, because some apps clamp the size to fit the screen at the old
	// position. Call Window afterwards to see the frame the app accepted.
	SetFrame(w Window, r layout.Rect) error
	// Raise brings a window to the front of its app (kAXRaiseAction),
	// without activating the app.
	Raise(w Window) error
	// Park moves a window below the bottom of a screen, outside the visible
	// area, leaving a 1-point line on screen (see ParkFrame).
	Park(w Window, s Screen) error
	// Unminimise restores a minimised window.
	Unminimise(w Window) error
	// Focus activates a window's app and makes the window its main window.
	Focus(w Window) error
	// PressClose asks a window to close, as clicking its close button
	// would (kAXCloseButtonAttribute, pressed), so that its app can ask
	// about unsaved changes first, or refuse. For a window with no close
	// button, it focuses the window and presses its app's Close Window, or
	// Close, menu item. It returns once the window has been asked: if it
	// closes, WindowDestroyed says so later, or AppTerminated if its app
	// quits as its last window closes. An app that hides a window rather
	// than close it, to show it again later, as Slack and Music do, may
	// send nothing: Window then reports the window on another desktop
	// (ErrOtherDesktop), as it does every window that its app no longer
	// lists. It returns ErrNoWindow for a window that has gone.
	PressClose(w Window) error

	// Trusted reports whether Bench has Accessibility permission. With
	// prompt, macOS asks the user to grant it if it hasn't been.
	Trusted(prompt bool) bool
	// ScreenRecordingAllowed reports whether Bench may capture windows for
	// live previews. With request, macOS asks the user if it hasn't been
	// decided.
	ScreenRecordingAllowed(request bool) bool
	// OpenSettings opens System Settings at a pane, for the user to turn
	// on a permission for Bench there, or allow its login item. macOS asks
	// for a permission only once, so this is what takes the user to the
	// switch after that. It returns once macOS has been asked. The browser
	// preview's platform refuses, with ErrUnsupported.
	OpenSettings(pane SettingsPane) error
	// LoginItemNeedsApproval reports whether macOS has Bench's login item
	// but won't open Bench at login until the user allows it in System
	// Settings ▸ General ▸ Login Items, as when they turned Bench off there
	// (SMAppService's requires-approval status). The browser preview's
	// platform reports false.
	LoginItemNeedsApproval() bool

	// SetRaised raises Bench's window above the docked windows (true), or
	// returns it to its usual level just below normal windows (false), where
	// every normal-level window, docked ones included, is above it again.
	// Bench raises itself while a menu, dialog, or drag is open, so the
	// docked windows don't cover them.
	SetRaised(raised bool)

	// ShowWebView shows a Web item's web view inside Bench's window, at
	// frame in content coordinates, creating it on first use.
	ShowWebView(id string, view WebView, frame layout.Rect) error
	// HideWebView hides a web view, keeping it loaded.
	HideWebView(id string)
	// CloseWebView destroys a web view.
	CloseWebView(id string)
	// PopOutWebView moves a web view into a window of its own.
	PopOutWebView(id, title string) error
	// DockWebView moves a popped-out web view back into Bench's window,
	// hidden until the next ShowWebView.
	DockWebView(id string) error
	// FocusWebView activates Bench and gives a shown Web item's view the
	// keyboard focus, for when its pane is focused.
	FocusWebView(id string) error

	// ReloadWebView loads a web view's page again, or the address it was
	// given if no page has come into view. NavigateWebView goes back (step
	// -1) or forward (1) in its history, doing nothing where it can't
	// (WebStateChanged says where it can). ZoomWebView sets its page's zoom,
	// as a scale, 1 for actual size. Each returns once WebKit has been
	// asked, and an error for a web view that doesn't exist.
	ReloadWebView(id string) error
	NavigateWebView(id string, step int) error
	ZoomWebView(id string, zoom float64) error
	// FindInWebView finds text in a web view's page, ignoring case, after
	// the match that's selected, or before it if backwards, going round at
	// the end, and selects and shows the match: it reports whether there is
	// one. It waits for WebKit's answer, up to a few seconds, without
	// waiting on the main thread, so callers mustn't hold locks.
	FindInWebView(id, text string, backwards bool) (bool, error)
	// SetCaptureState mutes a page's use of the camera or microphone
	// (CaptureMuted), which turns the camera off, or turns it back on
	// (CaptureActive), or stops it (CaptureNone). It does nothing for a
	// device the page isn't using. WebStateChanged reports the change.
	SetCaptureState(id string, device CaptureDevice, state CaptureState) error
	// PauseWebViewMedia pauses every sound and video that the page
	// plays.
	PauseWebViewMedia(id string) error
	// PageIcon returns the icon of the page that a Web item's view shows,
	// its favicon, as a PNG 40 pixels square, or ErrNoIcon while it has
	// none: a PageIconChanged event says when it changes.
	PageIcon(id string) ([]byte, error)

	// AppIcon returns the icon of the app with a bundle ID, as a PNG 40
	// pixels square, for a tile 20 points square: ErrNotInstalled if macOS
	// knows no such app, or ErrNoIcon where there are no icons, as in the
	// browser preview. It doesn't wait for the main thread.
	AppIcon(bundleID string) ([]byte, error)

	// ShowPreview shows a small live preview of target under anchor (in
	// content coordinates). HidePreview hides it and stops capturing.
	ShowPreview(target PreviewTarget, anchor layout.Rect) error
	HidePreview()

	// Notify posts a notification for an item. Clicking it sends a
	// NotificationClicked event.
	Notify(n Notification) error
	// SetBadge shows count on Bench's Dock icon, or nothing for 0: how
	// many items need the user. The browser preview's platform keeps it
	// for tests.
	SetBadge(count int)
	// WatchDockBadges turns on (true) or off reading the badges that other
	// apps show on their Dock icons, such as Mail's count of unread
	// messages. The Dock's items are read through the Accessibility API:
	// at once, when an app comes to the front, and every 5 seconds. While
	// it's on, a DockBadgesChanged event says when what DockBadges reports
	// changes. It's off until it's turned on.
	WatchDockBadges(on bool)
	// DockBadges reports the badges that running apps show on their Dock
	// icons, by bundle ID, as their text: a count such as "3" or "99+", or
	// other text, such as "•". Apps without a badge are left out, and so
	// is every app while watching is off, or while Bench doesn't have
	// Accessibility permission. It never waits: it reports what the last
	// reading found.
	DockBadges() map[string]string

	// Events reports windows and apps changing, screens changing, Bench's
	// window moving, and web view activity.
	Events() <-chan Event

	// Minimise minimises a window (kAXMinimizedAttribute). Bench minimises
	// a window it would park when no place to park it keeps clear of other
	// displays (see ParkPlace).
	Minimise(w Window) error

	// FocusUI activates Bench and gives its own UI (the terminals, menus,
	// and dialogs it draws) the keyboard focus, taking it from any Web
	// item's view: for when a terminal's pane is focused.
	FocusUI()

	// OpenTextFile opens a file, such as config.toml, in the default text
	// editor: the app that opens plain text files, or TextEdit. It returns
	// once macOS has been asked. The browser preview's platform refuses,
	// with ErrUnsupported.
	OpenTextFile(path string) error
	// ShowInFinder opens a Finder window with a file selected. The browser
	// preview's platform refuses, with ErrUnsupported.
	ShowInFinder(path string) error
	// ChooseToAdd shows macOS's open panel for the user to choose an app,
	// or a program or script, starting in the folder start, and returns
	// the path chosen, or "" if they cancel. The panel is a window of its
	// own, never a sheet on Bench's window, which may be lowered below
	// docked windows while it's open. It waits for the user, so callers
	// mustn't hold locks. An app chosen is registered with macOS, so that
	// OpenApp knows its bundle ID wherever it is. One shows at a time:
	// asking again while it's open brings it to the front and returns an
	// error wrapping ErrBusy. The browser preview's platform refuses, with
	// ErrUnsupported.
	ChooseToAdd(start string) (string, error)

	// ScrollBarsShown reports whether macOS shows scroll bars all the time,
	// rather than only while scrolling: System Settings' Show scroll bars is
	// Always, or it's automatic and a mouse without a trackpad is
	// connected (NSScroller's preferred style is legacy). A
	// ScrollBarsChanged event says when it changes. The browser preview's
	// platform reports false, as with a trackpad.
	ScrollBarsShown() bool

	// SystemKeys reports the keys that macOS keeps for itself and that
	// Bench's global shortcuts may clash with: the input sources'
	// shortcuts, and VoiceOver's keys while it's on. A SystemKeysChanged
	// event says when they may have changed. The browser preview's
	// platform reports none.
	SystemKeys() SystemKeys

	// HeldModifiers reports the modifier keys held down now, wherever the
	// keyboard is, so that Bench can tell when the user lets go of a
	// global shortcut's modifiers (see decision 60). It needs no
	// permission, and never waits for the main thread. The browser
	// preview's platform reports what the page says is held.
	HeldModifiers() Modifiers

	// MouseHeld reports whether the left mouse button is held down now,
	// wherever the pointer is, so that Bench can tell when the user lets
	// go of a docked window's edge that they're dragging (see decision
	// 62). It needs no permission, and never waits for the main thread.
	// The browser preview's platform reports false.
	MouseHeld() bool

	// StageManager reports whether Stage Manager is on, which moves the
	// windows of apps that aren't in front to a strip at the side of the
	// screen, so that Bench can't keep them in their panes. A
	// StageManagerChanged event says when it changes. The browser
	// preview's platform reports false.
	StageManager() bool

	// TrackDrags turns on (true) or off watching the user drag windows by
	// their title bars, for them to be dropped onto Bench's panes: other
	// apps' windows, through a global monitor of the left mouse button,
	// which needs no permission, and Bench's own popped-out windows.
	// While it's on, WindowDragged and WindowDropped report drags. It's
	// off until it's turned on.
	TrackDrags(on bool)
	// ShowDropGuide shows the drop guide over a pane, in a panel above
	// every window that never takes the keyboard or the mouse, replacing
	// the one showing, if any. HideDropGuide hides it.
	ShowDropGuide(g DropGuide)
	HideDropGuide()
}
