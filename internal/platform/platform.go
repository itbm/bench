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
	// Profile names the persistent data store. Web views with the same
	// profile share logins. The mac implementation derives a stable,
	// name-based UUID from it for +[WKWebsiteDataStore dataStoreForIdentifier:].
	Profile string `json:"profile"`
}

// PreviewTarget is what a live preview shows: a window, or a Web item's view.
type PreviewTarget struct {
	Window    WindowID `json:"window,omitempty"`
	WebViewID string   `json:"webView,omitempty"`
	Title     string   `json:"title"`
}

// Notification is a notification posted for a Web item.
type Notification struct {
	ItemID string `json:"itemId"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

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

	// ScreensChanged: a display was connected, disconnected, or rearranged.
	ScreensChanged EventKind = "screens-changed"
	// BenchMoved: Bench's window moved, resized, or changed screen. The mac
	// implementation throttles these to one per frame.
	BenchMoved EventKind = "bench-moved"

	// WebTitle: a Web item's page title changed (ItemID, Title). WebURL: it
	// navigated (ItemID, Body holds the address).
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

	// BenchHidden: Bench's window was hidden (Bench > Hide) or minimised,
	// so its docked windows should go too. BenchShown: it came back.
	BenchHidden EventKind = "bench-hidden"
	BenchShown  EventKind = "bench-shown"
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
	// MouseDown is set for WindowMoved and WindowResized when a mouse button
	// was held with the pointer on the window as it changed: a person
	// dragging it. Apps that move or resize their own windows (Electron apps
	// often do, just after being resized) don't hold the mouse there.
	MouseDown bool `json:"mouseDown,omitempty"`
}

// Errors that callers act on.
var (
	// ErrNoWindow: the window doesn't exist any more.
	ErrNoWindow = errors.New("window not found")
	// ErrOtherDesktop: the window exists but is on another desktop (Space),
	// where the Accessibility API can't see it.
	ErrOtherDesktop = errors.New("window is on another desktop")
	// ErrNotTrusted: Bench doesn't have Accessibility permission.
	ErrNotTrusted = errors.New("Bench needs Accessibility permission")
	// ErrUnsupported: this platform can't do that.
	ErrUnsupported = errors.New("not supported on this platform")
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

	// RunningApps returns the running apps.
	RunningApps() []App
	// InstalledApps returns every installed app, from /Applications,
	// ~/Applications, and /System/Applications and their Utilities folders.
	InstalledApps() []AppInfo
	// OpenApp launches an app if it isn't running, or activates it, and
	// opens url in it if url isn't empty.
	OpenApp(bundleID, url string) error
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

	// Trusted reports whether Bench has Accessibility permission. With
	// prompt, macOS asks the user to grant it if it hasn't been.
	Trusted(prompt bool) bool
	// ScreenRecordingAllowed reports whether Bench may capture windows for
	// live previews. With request, macOS asks the user if it hasn't been
	// decided.
	ScreenRecordingAllowed(request bool) bool

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

	// ShowPreview shows a small live preview of target under anchor (in
	// content coordinates). HidePreview hides it and stops capturing.
	ShowPreview(target PreviewTarget, anchor layout.Rect) error
	HidePreview()

	// Notify posts a notification for a Web item. Clicking it sends a
	// NotificationClicked event.
	Notify(n Notification) error

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
}
