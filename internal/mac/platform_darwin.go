//go:build darwin && cgo && !server

package mac

/*
#cgo CFLAGS: -fobjc-arc -mmacosx-version-min=14.0
#cgo LDFLAGS: -mmacosx-version-min=14.0 -framework Foundation -framework CoreFoundation -framework AppKit -framework ApplicationServices -framework CoreServices -framework CoreGraphics -framework WebKit -framework ScreenCaptureKit -framework CoreMedia -framework CoreVideo -framework QuartzCore -framework UserNotifications -framework UniformTypeIdentifiers -framework Carbon -framework ServiceManagement
#include <stdlib.h>
#include <string.h>
#include "bridge_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/icons"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// Platform is the macOS implementation of platform.Platform. Make one with
// New; there can be one at a time, since it owns process-wide observers.
// Its methods are safe for concurrent use.
type Platform struct {
	events *eventQueue
	moved  *throttle
	// notifyLimit holds back Web items' pages that post notifications too
	// often.
	notifyLimit *notifyLimiter

	// Written by native callbacks on the main thread, read anywhere.
	screens      atomic.Pointer[[]platform.Screen]
	content      atomic.Pointer[layout.Rect]
	scrollBars   atomic.Bool
	systemKeys   atomic.Pointer[platform.SystemKeys]
	stageManager atomic.Bool
	// poweringOff is when macOS last said that the user is logging out,
	// restarting, or shutting down, in Unix nanoseconds, or 0.
	poweringOff atomic.Int64

	uaSuffix  string
	hasBundle bool

	mu       sync.Mutex
	webViews map[string]bool // Web views that exist: shown and not closed.
	closed   bool
	// choosing gets the path chosen in the open panel that ChooseToAdd
	// shows, while it's open.
	choosing chan string
	// finds hands FindInWebView WebKit's answers.
	finds finds

	// pageIcons fetches and keeps Web items' page icons.
	pageIcons *icons.Pages
	// iconMu guards appIcons, the apps' icons drawn so far, by lower-case
	// bundle ID.
	iconMu   sync.Mutex
	appIcons map[string][]byte
}

// Compile-time checks that Platform is a platform.Platform, and logs its
// own decisions when given a log.
var (
	_ platform.Platform    = (*Platform)(nil)
	_ platform.DebugLogger = (*Platform)(nil)
)

var (
	// openMu serialises New and Close.
	openMu sync.Mutex
	// current is the open Platform, which native callbacks report to.
	current atomic.Pointer[Platform]
)

// New starts the macOS platform: it reads the screens, and starts watching
// apps and windows. Watching windows needs Accessibility permission; until
// Bench has it, the window methods return errors wrapping
// platform.ErrNotTrusted, and watching starts once it's granted.
//
// Events about windows arrive only while the main thread runs its run loop,
// as it does in the app, and in `bench arrange` through RunMainLoop. Call
// AttachWindow once Bench's window exists, and Close when Bench quits.
func New() (*Platform, error) {
	openMu.Lock()
	defer openMu.Unlock()
	if current.Load() != nil {
		return nil, errors.New("starting the macOS platform: it's already running; close it first")
	}
	p := &Platform{
		events:      newEventQueue(EventBuffer, overflowLimit),
		notifyLimit: newNotifyLimiter(nil),
		uaSuffix:    userAgentSuffix(int(C.benchOSMajorVersion())),
		hasBundle:   C.benchHasBundleID() != 0,
		webViews:    map[string]bool{},
		appIcons:    map[string][]byte{},
	}
	p.pageIcons = icons.NewPages(func(itemID string, has bool) {
		p.events.push(platform.Event{Kind: platform.PageIconChanged, ItemID: itemID, HasIcon: has})
	})
	p.moved = newThrottle(nil, benchMovedInterval, func() {
		p.events.push(platform.Event{Kind: platform.BenchMoved})
	})
	none := []platform.Screen{}
	p.screens.Store(&none)
	p.content.Store(&layout.Rect{})
	p.systemKeys.Store(&platform.SystemKeys{})
	current.Store(p)
	C.benchRefreshScreens()
	C.benchStart()
	return p, nil
}

// AttachWindow tells the platform which window is Bench's: pass Wails'
// window.NativeWindow(), an NSWindow. The platform puts it just below
// normal windows, turns off native full screen (so the green button
// zooms), adds the container for web views, and starts reporting its
// content frame and BenchMoved events. It's safe to call from any
// goroutine; the work happens on the main thread.
func (p *Platform) AttachWindow(nsWindow unsafe.Pointer) {
	if nsWindow == nil {
		return
	}
	C.benchAttachWindow(nsWindow)
}

// FillWindow zooms Bench's window, Wails' window.NativeWindow(), to fill
// its screen's visible area at once, without Wails' animation, unless it
// fills it already. The frame the window had is the one that the green
// button gives back. Call it on the main thread before the window is first
// shown, where it runs at once; from another goroutine, it runs on the
// main thread later.
func (p *Platform) FillWindow(nsWindow unsafe.Pointer) {
	if nsWindow == nil {
		return
	}
	C.benchFillWindow(nsWindow)
}

// TagPopOutWindow marks one of Bench's windows, Wails'
// window.NativeWindow(), as the popped-out window of an item, a terminal's,
// for the platform to report when it gets and loses the keyboard
// (PopOutFocused and PopOutUnfocused) and when the user drags it
// (WindowDragged and WindowDropped), as it does for popped-out web pages'
// windows. It runs on the main thread, later if it's called from another
// goroutine.
func (p *Platform) TagPopOutWindow(nsWindow unsafe.Pointer, itemID string) {
	if nsWindow == nil || itemID == "" {
		return
	}
	cid := C.CString(itemID)
	defer C.free(unsafe.Pointer(cid))
	C.benchTagPopOut(nsWindow, cid)
}

// SetWindowAppearance gives one of Bench's windows, Wails'
// window.NativeWindow(), the appearance setting: system leaves it to
// follow macOS, and light and dark fix it. Its background follows the
// appearance it has then, as the UI's does. Web items' web views in it
// keep following macOS. Bench's panels, the drop guide and the live
// preview, take the setting too, the next time they show. It runs on the
// main thread, later if it's called from another goroutine.
func (p *Platform) SetWindowAppearance(nsWindow unsafe.Pointer, appearance string) {
	C.benchSetPanelsAppearance(C.int(appearanceCode(appearance)))
	if nsWindow == nil {
		return
	}
	C.benchSetWindowAppearance(nsWindow, C.int(appearanceCode(appearance)))
}

// SystemDark reports whether macOS is in dark mode now, for Bench's
// windows to open with the system appearance's background. It reads the
// user's defaults, as Wails does once the app runs, so it works before.
func (p *Platform) SystemDark() bool {
	return C.benchSystemDark() != 0
}

// Close stops watching apps and windows, closes web views and the preview,
// and detaches Bench's window. The Events channel stays open, with no more
// events. It's safe to call more than once.
func (p *Platform) Close() {
	openMu.Lock()
	defer openMu.Unlock()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.webViews = map[string]bool{}
	// The open panel's answer won't come now.
	if p.choosing != nil {
		p.choosing <- ""
		p.choosing = nil
	}
	p.mu.Unlock()
	// Nor will the answers of finds in pages, nor pages' icons.
	p.finds.close()
	p.pageIcons.Close()

	p.moved.stop()
	C.benchStop()
	current.CompareAndSwap(p, nil)
	webLog.Store(nil)
	p.events.close()
}

// SetDebugLog logs, at debug level on log, what Web items' web views
// decide about each navigation and new window, naming only the host each
// one goes to (see logDecision). A nil log stops it. platform.Logged calls
// it when BENCH_DEBUG is set.
func (p *Platform) SetDebugLog(log *slog.Logger) {
	webLog.Store(log)
}

// Screens returns every display, the primary one first.
func (p *Platform) Screens() []platform.Screen {
	return slices.Clone(*p.screens.Load())
}

// ScreenOfWindow returns the screen holding most of Bench's window, or the
// primary screen.
func (p *Platform) ScreenOfWindow() platform.Screen {
	return screenOfRect(p.Screens(), p.ContentFrame())
}

// ContentFrame returns Bench's window content area in global coordinates,
// or a zero Rect before AttachWindow. It never waits for the main thread.
func (p *Platform) ContentFrame() layout.Rect {
	return *p.content.Load()
}

// ScrollBarsShown reports whether macOS shows scroll bars all the time
// (NSScroller's preferred style is legacy). The main thread reads it when
// the platform starts, and again when macOS says it has changed; until it
// first does, it's false. It never waits for the main thread.
func (p *Platform) ScrollBarsShown() bool {
	return p.scrollBars.Load()
}

// SystemKeys reports the input sources' shortcuts that are turned on,
// while more than one input source is, and whether VoiceOver is on. The
// main thread reads them when the platform starts; when Bench becomes
// active, as the user may have changed the shortcuts in System Settings
// meanwhile; when input sources are turned on or off; and when VoiceOver
// is. Until it first does, there are none. It never waits for the main
// thread.
func (p *Platform) SystemKeys() platform.SystemKeys {
	k := *p.systemKeys.Load()
	k.InputSources = slices.Clone(k.InputSources)
	return k
}

// StageManager reports whether Stage Manager is on: GloballyEnabled in
// com.apple.WindowManager, which Control Centre and System Settings ▸
// Desktop & Dock change. The main thread reads it when the platform
// starts, and again each time Bench becomes active, as nothing says when
// it changes; until it first does, it's false. It never waits for the main
// thread.
func (p *Platform) StageManager() bool {
	return p.stageManager.Load()
}

// LoggingOut reports whether the user is logging out, restarting, or
// shutting down: macOS said so (NSWorkspaceWillPowerOffNotification) less
// than platform.LogOutGrace ago, or, on the main thread, the quit that it's
// asking Bench for, as Wails' ShouldQuit runs, says so. It never waits for
// the main thread.
func (p *Platform) LoggingOut() bool {
	if at := p.poweringOff.Load(); at != 0 && time.Since(time.Unix(0, at)) < platform.LogOutGrace {
		return true
	}
	return C.benchQuitForLogOut() != 0
}

// RunningApps returns the running apps, apart from Bench itself.
func (p *Platform) RunningApps() []platform.App {
	var infos *C.BenchAppInfo
	var n C.int
	if C.benchRunningApps(&infos, &n) != C.BENCH_OK {
		return nil
	}
	defer C.benchFreeAppInfos(infos, n)
	all := unsafe.Slice(infos, int(n))
	running := make([]platform.App, 0, len(all))
	for i := range all {
		running = append(running, appFromC(&all[i]))
	}
	return running
}

// OwnAppHidden reports whether Bench itself is hidden, as when another
// app's Hide Others hides it. It never waits for the main thread.
func (p *Platform) OwnAppHidden() bool {
	return C.benchOwnAppHidden() != 0
}

// InstalledApps returns every installed app, from /Applications,
// ~/Applications, and /System/Applications and their Utilities folders, the
// other folders in /Applications, one level down, and the web apps that
// browsers keep in ~/Applications.
func (p *Platform) InstalledApps() []platform.AppInfo {
	home, _ := os.UserHomeDir()
	return apps.Scan(apps.Dirs(home))
}

// OpenApp launches an app, or activates it if it's running, and opens url
// in it if url isn't empty. It returns once macOS has been asked; the app
// launches in the background.
func (p *Platform) OpenApp(bundleID, url string) error {
	if bundleID == "" {
		return errors.New("opening an app: no bundle ID")
	}
	cid := C.CString(bundleID)
	defer C.free(unsafe.Pointer(cid))
	var curl *C.char
	if url != "" {
		curl = C.CString(url)
		defer C.free(unsafe.Pointer(curl))
	}
	action := "opening " + bundleID
	if url != "" {
		action = fmt.Sprintf("opening %s in %s", url, bundleID)
	}
	return statusError(int(C.benchOpenApp(cid, curl)), action)
}

// DefaultBrowser returns the bundle ID of the app that opens https links,
// as LaunchServices says, or "" if there's none, or it's an agent (such as
// a browser picker) that opens no windows of its own.
func (p *Platform) DefaultBrowser() string {
	id := C.benchDefaultBrowser()
	if id == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(id))
	return C.GoString(id)
}

// OpenTextFile opens a file in the default text editor: the app that opens
// plain text files, or TextEdit if there's none. It returns once macOS has
// been asked; the app opens in the background.
func (p *Platform) OpenTextFile(path string) error {
	if path == "" {
		return errors.New("opening a file: no path")
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	return statusError(int(C.benchOpenTextFile(cpath)), "opening "+path)
}

// ShowInFinder opens a Finder window with a file selected.
func (p *Platform) ShowInFinder(path string) error {
	if path == "" {
		return errors.New("showing a file in Finder: no path")
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	return statusError(int(C.benchShowInFinder(cpath)), "showing "+path+" in Finder")
}

// ChooseToAdd shows macOS's open panel, as a window of its own, for the
// user to choose an app, or a program or script, starting in the folder
// start, and waits for them: it returns the path chosen, or "" if they
// cancel. An app chosen is registered with LaunchServices, so that OpenApp
// finds it by its bundle ID, wherever it is. While the panel is open,
// asking again brings it to the front and returns an error wrapping
// platform.ErrBusy. It must not be called on the main thread, which shows
// the panel.
func (p *Platform) ChooseToAdd(start string) (string, error) {
	answer := make(chan string, 1)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return "", errors.New("choosing an app or program: the platform is closed")
	}
	if p.choosing != nil {
		p.mu.Unlock()
		C.benchFrontChooser()
		return "", fmt.Errorf("choosing an app or program: %w", platform.ErrBusy)
	}
	p.choosing = answer
	p.mu.Unlock()
	cstart := C.CString(start)
	defer C.free(unsafe.Pointer(cstart))
	if err := statusError(int(C.benchChooseToAdd(cstart)), "showing the open panel"); err != nil {
		p.mu.Lock()
		if p.choosing == answer {
			p.choosing = nil
		}
		p.mu.Unlock()
		return "", err
	}
	return <-answer, nil
}

// choseToAdd hands ChooseToAdd the path chosen in its open panel, or "" if
// the user cancelled. The main thread calls it, so it never waits.
func (p *Platform) choseToAdd(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.choosing != nil {
		p.choosing <- path
		p.choosing = nil
	}
}

// NewWindow opens a new window of a running app. It presses the app's New
// Window menu item; if the app has none, or it can't be pressed, it
// activates the app and opens it again, which makes most apps open a
// window if they have none.
func (p *Platform) NewWindow(app platform.App) error {
	pid, err := p.resolve(app, "opening a new window")
	if err != nil {
		return err
	}
	if C.benchPressNewWindow(C.int32_t(pid)) == C.BENCH_OK {
		return nil
	}
	bundleID := app.BundleID
	if bundleID == "" {
		bundleID = p.bundleIDOf(pid)
	}
	if bundleID == "" {
		return fmt.Errorf("opening a new window of %s: %w", appLabel(app), errNoMenuItem)
	}
	return p.OpenApp(bundleID, "")
}

// Hide hides an app.
func (p *Platform) Hide(app platform.App) error {
	return p.setHidden(app, true)
}

// Unhide shows a hidden app, without activating it.
func (p *Platform) Unhide(app platform.App) error {
	return p.setHidden(app, false)
}

// setHidden hides (hide) or shows an app.
func (p *Platform) setHidden(app platform.App, hide bool) error {
	action := "showing " + appLabel(app)
	if hide {
		action = "hiding " + appLabel(app)
	}
	pid, err := p.resolve(app, action)
	if err != nil {
		return err
	}
	return statusError(int(C.benchHideApp(C.int32_t(pid), cBool(hide))), action)
}

// Windows returns an app's windows on the current desktop, standard and
// floating, in the order they were first seen. It includes windows that the
// Accessibility API can leave out of the app's list but that Bench has seen:
// those in native full screen, minimised ones, and a hidden app's.
func (p *Platform) Windows(app platform.App) ([]platform.Window, error) {
	action := "listing the windows of " + appLabel(app)
	pid, err := p.resolve(app, action)
	if err != nil {
		return nil, err
	}
	var infos *C.BenchWindowInfo
	var n C.int
	status := C.benchCopyWindows(C.int32_t(pid), &infos, &n)
	if err := statusError(int(status), action); err != nil {
		return nil, err
	}
	defer C.benchFreeWindowInfos(infos, n)
	all := unsafe.Slice(infos, int(n))
	windows := make([]platform.Window, 0, len(all))
	for i := range all {
		windows = append(windows, windowFromC(&all[i]))
	}
	slices.SortStableFunc(windows, func(a, b platform.Window) int { return a.Created - b.Created })
	return windows, nil
}

// Window returns a window's current state. It returns an error wrapping
// platform.ErrNoWindow if the window has closed, or
// platform.ErrOtherDesktop if the window server still has it but the
// Accessibility API can't see it, because it's on another desktop. With
// ErrOtherDesktop, the Window names the app that owns the window (ID, PID,
// BundleID, and App), which callers check is still the app they knew: window
// IDs start again each login, so a remembered one can name another app's
// window.
func (p *Platform) Window(id platform.WindowID) (platform.Window, error) {
	return p.window(id, 0)
}

// window is Window, with the window's process ID if the caller knows it (or
// 0), which finds a window from an earlier run without asking the window
// server for its owner.
func (p *Platform) window(id platform.WindowID, pidHint int) (platform.Window, error) {
	var info C.BenchWindowInfo
	status := C.benchGetWindow(C.uint32_t(id), C.int32_t(pidHint), &info)
	defer C.benchFreeWindowInfo(&info)
	if err := statusError(int(status), fmt.Sprintf("reading window %d", id)); err != nil {
		if status == C.BENCH_ERR_OTHER_DESKTOP {
			// The native side names the app that owns the window, so that
			// callers can tell another app's window from the one they knew.
			return windowFromC(&info), err
		}
		return platform.Window{}, err
	}
	return windowFromC(&info), nil
}

// SetFrame moves and sizes a window: size, then position, then size again.
// It leaves a window in native full screen alone.
func (p *Platform) SetFrame(w platform.Window, r layout.Rect) error {
	status := C.benchSetFrame(C.uint32_t(w.ID), C.int32_t(w.PID),
		C.double(r.X), C.double(r.Y), C.double(r.W), C.double(r.H))
	return statusError(int(status), fmt.Sprintf("moving window %d", w.ID))
}

// Raise brings a window to the front of its app, without activating it.
func (p *Platform) Raise(w platform.Window) error {
	status := C.benchRaise(C.uint32_t(w.ID), C.int32_t(w.PID))
	return statusError(int(status), fmt.Sprintf("raising window %d", w.ID))
}

// Park moves a window, keeping its size, below the bottom of s, outside
// the visible area (see platform.ParkFrame). It leaves a window in native
// full screen alone. A screen with no frame means ScreenOfWindow.
func (p *Platform) Park(w platform.Window, s platform.Screen) error {
	live, err := p.window(w.ID, w.PID)
	if err != nil {
		return fmt.Errorf("parking window %d: %w", w.ID, errors.Unwrap(err))
	}
	if live.FullScreen {
		return nil
	}
	if s.Frame.W <= 0 || s.Frame.H <= 0 {
		s = p.ScreenOfWindow()
	}
	return p.SetFrame(live, platform.ParkFrame(p.Screens(), s, live.Frame))
}

// Unminimise restores a minimised window.
func (p *Platform) Unminimise(w platform.Window) error {
	status := C.benchSetMinimised(C.uint32_t(w.ID), C.int32_t(w.PID), 0)
	return statusError(int(status), fmt.Sprintf("restoring window %d", w.ID))
}

// Focus activates a window's app and makes the window its main window,
// restoring it first if it's minimised.
func (p *Platform) Focus(w platform.Window) error {
	status := C.benchFocus(C.uint32_t(w.ID), C.int32_t(w.PID))
	return statusError(int(status), fmt.Sprintf("focusing window %d", w.ID))
}

// PressClose asks a window to close: it presses the window's close button,
// as a click would, so that its app can ask about unsaved changes first. A
// window without one is focused, and its app's Close Window (or Close) menu
// item pressed, which closes the app's main window. It returns once the
// window has been asked; its observer reports it if it closes.
func (p *Platform) PressClose(w platform.Window) error {
	action := fmt.Sprintf("closing window %d", w.ID)
	status := C.benchPressClose(C.uint32_t(w.ID), C.int32_t(w.PID))
	if status != C.BENCH_ERR_NO_CLOSE_BUTTON {
		return statusError(int(status), action)
	}
	pid, err := p.resolve(platform.App{PID: w.PID, BundleID: w.BundleID}, action)
	if err != nil {
		return err
	}
	if err := p.Focus(w); err != nil {
		return err
	}
	if C.benchPressCloseItem(C.int32_t(pid)) != C.BENCH_OK {
		return fmt.Errorf("%s: %w", action, errNoCloseButton)
	}
	return nil
}

// Trusted reports whether Bench has Accessibility permission. With prompt,
// macOS asks the user to grant it if it hasn't been.
func (p *Platform) Trusted(prompt bool) bool {
	return C.benchTrusted(cBool(prompt)) != 0
}

// ScreenRecordingAllowed reports whether Bench may capture windows for live
// previews. With request, macOS asks the user if it hasn't been decided.
func (p *Platform) ScreenRecordingAllowed(request bool) bool {
	return C.benchScreenRecordingAllowed(cBool(request)) != 0
}

// OpenSettings opens System Settings at a pane: Privacy & Security's list
// for a permission, by its address, or General ▸ Login Items, with
// SMAppService. It returns once macOS has been asked.
func (p *Platform) OpenSettings(pane platform.SettingsPane) error {
	if pane == platform.SettingsLoginItems {
		C.benchOpenLoginItems()
		return nil
	}
	address, err := settingsURL(pane)
	if err != nil {
		return err
	}
	caddr := C.CString(address)
	defer C.free(unsafe.Pointer(caddr))
	return statusError(int(C.benchOpenSettingsURL(caddr)), "opening System Settings")
}

// LoginItemNeedsApproval reports whether macOS has Bench's login item but
// needs the user to allow it in System Settings ▸ General ▸ Login Items
// (SMAppService's requires-approval status). Wails' Autostart reports that
// as not enabled, without saying why. It's false for a binary that isn't
// in an app bundle, which Wails opens at login with a LaunchAgent instead.
func (p *Platform) LoginItemNeedsApproval() bool {
	return C.benchLoginItemNeedsApproval() != 0
}

// SetRaised raises Bench's window above the docked windows and hides the
// web views (true), or puts the window back just below normal windows and
// shows the web views that were showing (false).
func (p *Platform) SetRaised(raised bool) {
	C.benchSetRaised(cBool(raised))
}

// ShowWebView shows a Web item's web view at frame (content coordinates),
// creating it on first use. A later call with a different address loads
// it; one with a different profile makes a new web view in that profile's
// data store. A popped-out web view stays in its own window.
func (p *Platform) ShowWebView(id string, view platform.WebView, frame layout.Rect) error {
	if id == "" {
		return errors.New("showing a web view: no item ID")
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("showing a web view: the platform is closed")
	}
	p.webViews[id] = true
	p.mu.Unlock()

	cid, cname, curl := C.CString(id), C.CString(view.Name), C.CString(view.URL)
	cuuid, cua := C.CString(ProfileUUID(view.Profile)), C.CString(p.uaSuffix)
	defer freeAll(cid, cname, curl, cuuid, cua)
	C.benchWebShow(cid, cname, curl, cuuid, cua, C.double(view.Zoom), C.double(frame.X), C.double(frame.Y), C.double(frame.W), C.double(frame.H))
	return nil
}

// HideWebView hides a web view, keeping it loaded.
func (p *Platform) HideWebView(id string) {
	if !p.hasWebView(id) {
		return
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebHide(cid)
}

// CloseWebView destroys a web view.
func (p *Platform) CloseWebView(id string) {
	p.mu.Lock()
	known := p.webViews[id]
	delete(p.webViews, id)
	p.mu.Unlock()
	if !known {
		return
	}
	p.pageIcons.Forget(id)
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebClose(cid)
}

// PageIcon returns the icon of the page that a Web item's view shows, as
// icons.Pages fetched it once the page had loaded, or platform.ErrNoIcon.
func (p *Platform) PageIcon(id string) ([]byte, error) {
	if png, ok := p.pageIcons.Icon(id); ok {
		return png, nil
	}
	return nil, platform.ErrNoIcon
}

// foundIconLinks takes the <link> elements of the page that a Web item's
// view has loaded, as the page's script read them, for its icon.
func (p *Platform) foundIconLinks(itemID, pageURL, links string) {
	if !p.hasWebView(itemID) {
		return
	}
	p.pageIcons.Found(itemID, pageURL, iconLinks(links))
}

// AppIcon returns the icon of the app with a bundle ID, as a PNG
// icons.Pixels square, which macOS draws the first time it's asked for,
// on the calling thread, never the main thread, and Bench then keeps:
// platform.ErrNotInstalled if macOS knows no such app.
func (p *Platform) AppIcon(bundleID string) ([]byte, error) {
	if !validBundleID(bundleID) {
		return nil, fmt.Errorf("drawing the icon of %q: %w", bundleID, platform.ErrNotInstalled)
	}
	key := strings.ToLower(bundleID)
	p.iconMu.Lock()
	png, ok := p.appIcons[key]
	p.iconMu.Unlock()
	if ok {
		return png, nil
	}
	cid := C.CString(bundleID)
	defer C.free(unsafe.Pointer(cid))
	var data unsafe.Pointer
	var length C.int
	status := int(C.benchAppIcon(cid, C.int(icons.Pixels), &data, &length))
	if err := statusError(status, "drawing the icon of "+bundleID); err != nil {
		return nil, err
	}
	defer C.free(data)
	png = C.GoBytes(data, length)
	p.iconMu.Lock()
	p.appIcons[key] = png
	p.iconMu.Unlock()
	return png, nil
}

// PopOutWebView moves a web view into a window of its own, titled title.
// Closing that window sends WebViewClosed.
func (p *Platform) PopOutWebView(id, title string) error {
	if !p.hasWebView(id) {
		return fmt.Errorf("popping out web view %q: there's no such web view", id)
	}
	cid, ctitle := C.CString(id), C.CString(title)
	defer freeAll(cid, ctitle)
	C.benchWebPopOut(cid, ctitle)
	return nil
}

// DockWebView moves a popped-out web view back into Bench's window, hidden
// until the next ShowWebView.
func (p *Platform) DockWebView(id string) error {
	if !p.hasWebView(id) {
		return fmt.Errorf("docking web view %q: there's no such web view", id)
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebDock(cid)
	return nil
}

// FocusWebView activates Bench and gives a Web item's web view the keyboard
// focus, as when its pane is focused. While Bench is raised (a menu or
// dialog is open), the web view gets the focus when Bench is lowered. A
// popped-out web view's window comes forward instead.
func (p *Platform) FocusWebView(id string) error {
	if !p.hasWebView(id) {
		return fmt.Errorf("focusing web view %q: there's no such web view", id)
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebFocus(cid)
	return nil
}

// ReloadWebView loads a web view's page again, or the address it was given
// if no page has come into view.
func (p *Platform) ReloadWebView(id string) error {
	if !p.hasWebView(id) {
		return fmt.Errorf("reloading web view %q: there's no such web view", id)
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebReload(cid)
	return nil
}

// NavigateWebView goes back (step -1) or forward (1) in a web view's
// history. WebKit does nothing where it can't: WebStateChanged says where
// it can.
func (p *Platform) NavigateWebView(id string, step int) error {
	if !p.hasWebView(id) {
		return fmt.Errorf("navigating web view %q: there's no such web view", id)
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebNavigate(cid, C.int(max(-1, min(step, 1))))
	return nil
}

// ZoomWebView sets a web view's page zoom (WKWebView's pageZoom), as a
// scale, 1 or 0 for actual size.
func (p *Platform) ZoomWebView(id string, zoom float64) error {
	if !p.hasWebView(id) {
		return fmt.Errorf("zooming web view %q: there's no such web view", id)
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebZoom(cid, C.double(zoom))
	return nil
}

// FindInWebView finds text in a web view's page, as find in page does in a
// browser, and reports whether there's a match, which WebKit selects and
// scrolls to. It waits for WebKit's answer, which the main thread gives,
// for up to findTimeout, so it mustn't be called on the main thread.
func (p *Platform) FindInWebView(id, text string, backwards bool) (bool, error) {
	if !p.hasWebView(id) {
		return false, fmt.Errorf("finding in web view %q: there's no such web view", id)
	}
	if text == "" {
		return false, nil
	}
	request, answer, ok := p.finds.ask()
	if !ok {
		return false, fmt.Errorf("finding in web view %q: %w", id, errFindClosed)
	}
	cid, ctext := C.CString(id), C.CString(clip(text, maxFind))
	C.benchWebFind(cid, ctext, cBool(backwards), C.uint64_t(request))
	freeAll(cid, ctext)
	found, err := p.finds.wait(request, answer, findTimeout)
	if err != nil {
		return false, fmt.Errorf("finding in web view %q: %w", id, err)
	}
	return found, nil
}

// SetCaptureState mutes a page's use of the camera or microphone, or turns
// it back on, or stops it, with WKWebView's setCameraCaptureState: and
// setMicrophoneCaptureState:. It does nothing for a device the page isn't
// using. KVO reports the change, as WebStateChanged.
func (p *Platform) SetCaptureState(id string, device platform.CaptureDevice, state platform.CaptureState) error {
	kind, ok := deviceKind(device)
	if !ok {
		return fmt.Errorf("setting the capture state of web view %q: there's no device %q", id, device)
	}
	code, ok := captureCode(state)
	if !ok {
		return fmt.Errorf("setting the capture state of web view %q: there's no state %q", id, state)
	}
	if !p.hasWebView(id) {
		return fmt.Errorf("setting the capture state of web view %q: there's no such web view", id)
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebSetCapture(cid, C.int(kind), C.int(code))
	return nil
}

// PauseWebViewMedia pauses every sound and video that a web view's page,
// and its popups' pages, play (pauseAllMediaPlaybackWithCompletionHandler:).
func (p *Platform) PauseWebViewMedia(id string) error {
	if !p.hasWebView(id) {
		return fmt.Errorf("pausing web view %q: there's no such web view", id)
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	C.benchWebPauseMedia(cid)
	return nil
}

// FocusUI activates Bench and gives Bench's own UI (its terminals, menus,
// and dialogs) the keyboard focus, taking it from any Web item's web view:
// for when a terminal's pane is focused. A page's own focus calls can't do
// that, since the web views are separate views in Bench's window.
func (p *Platform) FocusUI() {
	C.benchFocusUI()
}

// ShowPreview shows a live preview of target under anchor (content
// coordinates). The panel appears once the first frame arrives, so a
// capture that fails shows nothing. It returns an error wrapping
// platform.ErrNoWindow for a window that doesn't exist, and an error for a
// window when Bench doesn't have Screen Recording permission (it doesn't
// ask; ScreenRecordingAllowed does).
func (p *Platform) ShowPreview(target platform.PreviewTarget, anchor layout.Rect) error {
	content := p.ContentFrame()
	if content.W <= 0 || content.H <= 0 {
		return errors.New("showing a preview: Bench has no window")
	}
	panel := GlobalFromAppKit(previewFrame(anchor, content), primaryHeight(p.Screens()))
	ctitle := C.CString(target.Title)
	defer C.free(unsafe.Pointer(ctitle))
	switch {
	case target.Window != 0:
		// Without permission, ScreenCaptureKit would ask for it on every
		// hover; Settings asks instead, when previews are turned on.
		if !p.ScreenRecordingAllowed(false) {
			return fmt.Errorf("showing a preview: %w", errNoScreenRecording)
		}
		w, err := p.Window(target.Window)
		if err != nil {
			return fmt.Errorf("showing a preview: %w", errors.Unwrap(err))
		}
		cw, ch := captureSize(w.Frame.W, w.Frame.H)
		C.benchPreviewShowWindow(C.uint32_t(target.Window), ctitle,
			C.double(panel.X), C.double(panel.Y), C.double(panel.W), C.double(panel.H), C.int(cw), C.int(ch))
	case target.WebViewID != "":
		if !p.hasWebView(target.WebViewID) {
			return fmt.Errorf("previewing web view %q: there's no such web view", target.WebViewID)
		}
		cid := C.CString(target.WebViewID)
		defer C.free(unsafe.Pointer(cid))
		C.benchPreviewShowWeb(cid, ctitle, C.double(panel.X), C.double(panel.Y), C.double(panel.W), C.double(panel.H))
	default:
		return errors.New("showing a preview: no window or web view to show")
	}
	return nil
}

// HidePreview hides the live preview and stops capturing.
func (p *Platform) HidePreview() {
	C.benchPreviewHide()
}

// TrackDrags turns watching the user drag windows by their title bars on
// or off: other apps' windows, through a global monitor of the left mouse
// button, which needs no permission, and Bench's own popped-out windows.
func (p *Platform) TrackDrags(on bool) {
	C.benchTrackDrags(cBool(on))
}

// ShowDropGuide shows the drop guide over a pane (see guideMarks), in a
// panel above every window that lets the mouse through.
func (p *Platform) ShowDropGuide(g platform.DropGuide) {
	if g.Pane.W <= 0 || g.Pane.H <= 0 {
		return
	}
	panel := GlobalFromAppKit(g.Pane, primaryHeight(p.Screens()))
	items := guideMarks(g)
	marks := make([]C.BenchGuideMark, len(items))
	for i, m := range items {
		marks[i] = C.BenchGuideMark{kind: C.int(m.kind), side: C.int(m.side),
			x: C.double(m.rect.X), y: C.double(m.rect.Y), w: C.double(m.rect.W), h: C.double(m.rect.H)}
	}
	clabel := C.CString(g.Label)
	defer C.free(unsafe.Pointer(clabel))
	C.benchShowDropGuide(C.double(panel.X), C.double(panel.Y), C.double(panel.W), C.double(panel.H),
		unsafe.SliceData(marks), C.int(len(marks)), clabel)
}

// HideDropGuide hides the drop guide.
func (p *Platform) HideDropGuide() {
	C.benchHideDropGuide()
}

// Notify posts a notification for an item, asking for permission the first
// time. Clicking it sends NotificationClicked. A bare binary (not in an app
// bundle) can't post notifications, so there Notify returns an error
// wrapping platform.ErrUnsupported.
func (p *Platform) Notify(n platform.Notification) error {
	if !p.hasBundle {
		return fmt.Errorf("posting a notification: %w", platform.ErrUnsupported)
	}
	cid, ctitle, csubtitle, cbody := C.CString(n.ItemID), C.CString(n.Title), C.CString(n.Subtitle), C.CString(n.Body)
	defer freeAll(cid, ctitle, csubtitle, cbody)
	C.benchNotify(cid, ctitle, csubtitle, cbody)
	return nil
}

// SetBadge shows count on Bench's Dock icon, or takes the badge away for 0.
func (p *Platform) SetBadge(count int) {
	C.benchSetBadge(C.int(max(count, 0)))
}

// Events reports windows and apps changing, screens changing, Bench's
// window moving, and web view activity. The channel is never closed.
func (p *Platform) Events() <-chan platform.Event {
	return p.events.C()
}

// Minimise minimises a window, for one that Bench can't park clear of other
// displays.
func (p *Platform) Minimise(w platform.Window) error {
	status := C.benchSetMinimised(C.uint32_t(w.ID), C.int32_t(w.PID), 1)
	return statusError(int(status), fmt.Sprintf("minimising window %d", w.ID))
}

// resolve finds a running app's process ID: app.PID if that process is
// still the app, or else the first running instance of app.BundleID.
func (p *Platform) resolve(app platform.App, action string) (int, error) {
	var cid *C.char
	if app.BundleID != "" {
		cid = C.CString(app.BundleID)
		defer C.free(unsafe.Pointer(cid))
	}
	if pid := int(C.benchResolvePID(C.int32_t(app.PID), cid)); pid > 0 {
		return pid, nil
	}
	return 0, fmt.Errorf("%s: %w", action, errNotRunning)
}

// bundleIDOf returns the bundle ID of a running app, or "".
func (p *Platform) bundleIDOf(pid int) string {
	for _, a := range p.RunningApps() {
		if a.PID == pid {
			return a.BundleID
		}
	}
	return ""
}

// hasWebView reports whether a web view exists.
func (p *Platform) hasWebView(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.webViews[id]
}

// appLabel names an app in messages.
func appLabel(app platform.App) string {
	switch {
	case app.Name != "":
		return app.Name
	case app.BundleID != "":
		return app.BundleID
	default:
		return fmt.Sprintf("process %d", app.PID)
	}
}

// goClipped copies a C string, clipped to n runes. It reads no more of it
// than those runes can take (see clipBytes): other apps choose their window
// titles and names, and one could be megabytes long.
func goClipped(s *C.char, n int) string {
	if s == nil {
		return ""
	}
	length := C.strnlen(s, C.size_t(clipBytes(n)))
	return clip(C.GoStringN(s, C.int(length)), n)
}

// cBool converts a Go bool to a C int.
func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// freeAll frees C strings.
func freeAll(strs ...*C.char) {
	for _, s := range strs {
		C.free(unsafe.Pointer(s))
	}
}
