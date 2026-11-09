package platform

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
)

// FakeEventBuffer is the capacity of a Fake's Events channel. When it's full,
// the Fake drops new events and counts them (see Fake.Dropped) rather than
// block.
const FakeEventBuffer = 4096

// DefaultNewWindowTitle is the title of a window opened by NewWindow, unless
// the app has its own (see AppNewWindowTitle).
const DefaultNewWindowTitle = "New window"

// Fake is a complete, in-memory Platform for tests, with methods to script
// what the user and other apps do and to inspect what Bench asked for. It's
// safe for concurrent use.
//
// The Fake has three kinds of method:
//
//   - Platform methods, which behave like the real thing and are logged
//     (see Calls).
//   - Set-up methods (AddApp, InstallApp, AddWindow, SetScreens, and the
//     other Set methods), which change the world quietly, without events.
//   - Scripted actions (OpenWindow, CloseWindow, FocusWindow, QuitApp, and
//     so on), which act as the user or another app would, and send the
//     events the real platform would. They aren't logged in Calls.
//
// Timed behaviour, such as an app taking a while to launch, uses the clock
// passed to NewFake, so tests can move a clock.Fake forward by hand. As on a
// Mac, the methods that use the Accessibility API fail with ErrNotTrusted
// while Bench doesn't have Accessibility permission (see SetTrusted).
type Fake struct {
	clk clock.Clock

	mu sync.Mutex

	screens     []Screen
	benchScreen string // A screen ID, or "" for the one holding the content frame.
	content     layout.Rect
	contentSet  bool
	// focusedWebView is the web view last given the keyboard focus.
	focusedWebView string
	demoScreen     string
	demoUnplugged  bool
	demoFit        float64

	apps    map[string]*fakeApp
	appSeq  int
	nextPID int
	active  string // Bundle ID of the frontmost app, or "".
	// own is the bundle ID of Bench's own app (see SetOwnApp), or "".
	own string
	// defaultBrowser is the bundle ID DefaultBrowser returns.
	defaultBrowser string
	// chosen is the path ChooseToAdd returns (see SetChosen).
	chosen string
	// webApps counts the web apps that browsers have made (see
	// AppMakesWebApps).
	webApps int

	windows map[WindowID]*fakeWindow
	zorder  []WindowID // Front to back.
	nextID  WindowID
	created int

	trusted         bool
	screenRecording bool
	trustPrompts    int
	recordingAsks   int
	// scrollBars is what ScrollBarsShown reports.
	scrollBars bool
	// systemKeys is what SystemKeys reports.
	systemKeys SystemKeys
	// stageManager is what StageManager reports.
	stageManager bool
	// toolPrefs holds what ToolPreferences reports, by lower-case bundle
	// ID (SetToolPreferences).
	toolPrefs map[string]ToolPreferences
	// held is what HeldModifiers reports, and mouseHeld what MouseHeld
	// does.
	held      Modifiers
	mouseHeld bool
	// appIcons and pageIcons are what AppIcon and PageIcon return, by
	// lower-case bundle ID and by item ID.
	appIcons  map[string][]byte
	pageIcons map[string][]byte
	// loggingOut is what LoggingOut reports.
	loggingOut bool
	// loginApproval is what LoginItemNeedsApproval reports.
	loginApproval bool
	// tracking is whether TrackDrags turned drag tracking on, dropGuide
	// the drop guide showing, if any, and drags the frame each window being
	// dragged had as its drag began, by dragKey.
	tracking  bool
	dropGuide *DropGuide
	drags     map[string]layout.Rect
	// watchingBadges is whether WatchDockBadges turned reading the apps'
	// Dock badges on.
	watchingBadges bool

	raised        bool
	webViews      map[string]*FakeWebView
	previews      []FakePreview
	preview       *FakePreview
	notifications []Notification
	badge         int
	openedURLs    []OpenedURL
	// webPages describes the page at an address, for the WebURL events
	// that ShowWebView and LoadWebView send (see SetWebPages), or is nil.
	webPages func(address string) Page
	// webTitles gives the title of the page at an address, for the
	// WebTitle events that ShowWebView and LoadWebView send (see
	// SetWebTitles), or is nil.
	webTitles func(address string) string
	// webMade lists the web views made, in order (see WebViewsMade).
	webMade []string

	emitOwnMoves bool
	errs         map[string]error
	windowErrs   map[windowErrKey]error
	// hiddenFixed is set while SetFrame and Park leave a hidden app's
	// windows where they are, answering hiddenErr (see FixHiddenWindows).
	hiddenFixed bool
	hiddenErr   error

	calls []Call
	// history is how many calls, opened URLs, previews, and notifications
	// the Fake keeps, or 0 for all of them (see SetHistoryLimit).
	history int
	events  chan Event
	dropped int
}

// fakeApp is an app the Fake knows about: installed, running, or both.
type fakeApp struct {
	bundleID string
	name     string  // The running app's name.
	info     AppInfo // The installed app, if installed.

	installed bool
	running   bool
	launching bool
	pid       int
	seq       int // Launch order, for RunningApps.
	hidden    bool
	regular   bool
	focused   WindowID // The app's focused window.

	launchDelay    time.Duration
	launchWindows  []string
	newWindowTitle string
	urlWindows     map[string]string
	pendingURLs    []string
	// makesWebApps is set for a browser that makes a web app of each site
	// OpenApp asks it to open, makeDelay later.
	makesWebApps bool
	makeDelay    time.Duration
	// quitsWithLastWindow is set for an app that quits as its last window
	// closes (see AppQuitsWithLastWindow).
	quitsWithLastWindow bool
	// dockBadge is the badge the app shows on its Dock icon while it runs
	// (see SetDockBadge), or "".
	dockBadge string
}

// fakeWindow is a window and what the Fake knows about it beyond Window.
type fakeWindow struct {
	w            Window
	minW, minH   float64
	stepW, stepH float64
	otherDesktop bool
	beforeFull   layout.Rect // The frame before full screen.
	// closeAsks and closeHides are what PressClose does instead of closing
	// the window (see WithCloseAsks and WithCloseHides).
	closeAsks  bool
	closeHides bool
}

type windowErrKey struct {
	id     WindowID
	method string
}

// NewFake returns a Fake with one 1,512 by 982 point screen (the visible area
// starting below a 25-point menu bar), no apps, no default browser,
// Accessibility permission granted, and Screen Recording permission not
// granted. Bench's content frame is the screen's visible area until
// SetContentFrame changes it. A nil clk means clock.Real().
func NewFake(clk clock.Clock) *Fake {
	if clk == nil {
		clk = clock.Real()
	}
	return &Fake{
		clk: clk,
		screens: []Screen{{
			ID:      "1",
			Name:    "Built-in Retina Display",
			Frame:   layout.Rect{X: 0, Y: 0, W: 1512, H: 982},
			Visible: layout.Rect{X: 0, Y: 25, W: 1512, H: 957},
			Primary: true,
		}},
		apps:         map[string]*fakeApp{},
		nextPID:      1000,
		windows:      map[WindowID]*fakeWindow{},
		trusted:      true,
		webViews:     map[string]*FakeWebView{},
		appIcons:     map[string][]byte{},
		pageIcons:    map[string][]byte{},
		emitOwnMoves: true,
		errs:         map[string]error{},
		windowErrs:   map[windowErrKey]error{},
		events:       make(chan Event, FakeEventBuffer),
	}
}

// Compile-time check that Fake is a Platform.
var _ Platform = (*Fake)(nil)

// ---------- Options ----------

// AppOption changes how AddApp or InstallApp sets up an app.
type AppOption func(*appConfig)

type appConfig struct {
	installed      bool
	running        bool
	regular        bool
	hidden         bool
	path           string
	webApp         string
	webURL         string
	makeDelay      *time.Duration
	launchDelay    *time.Duration
	launchWindows  []string
	setLaunch      bool
	newWindowTitle string
	urlWindows     map[string]string
	quitsWithLast  bool
}

// AppNotRunning sets up an app that's installed but not running.
func AppNotRunning() AppOption { return func(c *appConfig) { c.running = false } }

// AppNotInstalled sets up a running app that InstalledApps doesn't list,
// such as a helper or an app run from a disk image.
func AppNotInstalled() AppOption { return func(c *appConfig) { c.installed = false } }

// AppNotRegular sets up an app that isn't in the Dock (an agent, menu bar
// extra, or background helper).
func AppNotRegular() AppOption { return func(c *appConfig) { c.regular = false } }

// AppHidden sets up a running app that's hidden.
func AppHidden() AppOption { return func(c *appConfig) { c.hidden = true } }

// AppPath sets the installed app's path. The default is
// /Applications/<name>.app.
func AppPath(path string) AppOption { return func(c *appConfig) { c.path = path } }

// AppWebApp sets up the installed app as a web app that the named browser
// made, such as "Chrome".
func AppWebApp(browser string) AppOption { return func(c *appConfig) { c.webApp = browser } }

// AppWebURL sets the address that the installed web app opens (see
// AppInfo.WebURL).
func AppWebURL(url string) AppOption { return func(c *appConfig) { c.webURL = url } }

// AppMakesWebApps sets up a browser that makes a web app of each http or
// https site that OpenApp asks it to open, after delay by the Fake's clock,
// as the user would by following the browser's steps: an installed app that
// isn't running, named after the site's host, with the browser's installed
// name as its WebApp, the address as its WebURL, and the browser's launch
// delay. Its bundle ID is the browser's, then ".WebApp." and a number, as
// Safari's are, and it opens one window, titled with its name, each time it
// launches. A browser makes one web app for each host. The default is none.
func AppMakesWebApps(delay time.Duration) AppOption {
	return func(c *appConfig) { c.makeDelay = &delay }
}

// AppLaunchDelay sets how long the app takes to launch after OpenApp, by the
// Fake's clock. The default, 0, launches it before OpenApp returns.
func AppLaunchDelay(d time.Duration) AppOption {
	return func(c *appConfig) { c.launchDelay = &d }
}

// AppLaunchWindows sets the titles of the standard windows the app opens
// each time it launches (through OpenApp or LaunchApp), in order. The
// default is none.
func AppLaunchWindows(titles ...string) AppOption {
	return func(c *appConfig) {
		c.launchWindows = slices.Clone(titles)
		c.setLaunch = true
	}
}

// AppNewWindowTitle sets the title of the windows NewWindow opens. The
// default is DefaultNewWindowTitle.
func AppNewWindowTitle(title string) AppOption {
	return func(c *appConfig) { c.newWindowTitle = title }
}

// AppQuitsWithLastWindow sets up an app that quits as its last window
// closes, as System Settings and Calculator do: PressClose on its last
// window quits it, sending AppTerminated alone, as QuitApp does.
func AppQuitsWithLastWindow() AppOption { return func(c *appConfig) { c.quitsWithLast = true } }

// AppURLWindow makes the app open a standard window with the given title
// when OpenApp asks it to open url.
func AppURLWindow(url, title string) AppOption {
	return func(c *appConfig) {
		if c.urlWindows == nil {
			c.urlWindows = map[string]string{}
		}
		c.urlWindows[url] = title
	}
}

// WindowOption changes how AddWindow or OpenWindow sets up a window.
type WindowOption func(*windowConfig)

type windowConfig struct {
	subrole      string
	frame        *layout.Rect
	minW, minH   float64
	stepW, stepH float64
	minimised    bool
	fullScreen   bool
	otherDesktop bool
	closeAsks    bool
	closeHides   bool
}

// WithSubrole sets the window's Accessibility subrole. The default is
// SubroleStandard.
func WithSubrole(subrole string) WindowOption {
	return func(c *windowConfig) { c.subrole = subrole }
}

// WithFrame sets the window's frame. The default cascades windows across
// the primary screen's visible area.
func WithFrame(r layout.Rect) WindowOption {
	return func(c *windowConfig) { c.frame = &r }
}

// WithMinSize sets the smallest size the window accepts. SetFrame clamps
// sizes up to it. The default is no minimum.
func WithMinSize(w, h float64) WindowOption {
	return func(c *windowConfig) { c.minW, c.minH = w, h }
}

// WithSizeStep makes the window size itself in steps, as a terminal sizes
// itself in whole characters: SetFrame, Park, and UserMoveWindow round its
// width and height down to whole steps (then up to its minimum size). A step
// of 0 means any size.
func WithSizeStep(w, h float64) WindowOption {
	return func(c *windowConfig) { c.stepW, c.stepH = w, h }
}

// WithMinimised sets up a minimised window.
func WithMinimised() WindowOption { return func(c *windowConfig) { c.minimised = true } }

// WithFullScreen sets up a window in native full screen, filling the screen
// Bench is on.
func WithFullScreen() WindowOption { return func(c *windowConfig) { c.fullScreen = true } }

// WithOtherDesktop sets up a window on another desktop (Space).
func WithOtherDesktop() WindowOption { return func(c *windowConfig) { c.otherDesktop = true } }

// WithCloseAsks sets up a window that asks before it closes, as one with
// unsaved changes does: PressClose leaves it open, as a real app would
// while it shows its sheet, and sends nothing. CloseWindow still closes it,
// as the user answering would.
func WithCloseAsks() WindowOption { return func(c *windowConfig) { c.closeAsks = true } }

// WithCloseHides sets up a window that its app hides rather than closes,
// as Slack and Music do: PressClose takes it off screen and sends nothing,
// and afterwards, as on a Mac, Windows leaves it out and Window returns
// ErrOtherDesktop.
func WithCloseHides() WindowOption { return func(c *windowConfig) { c.closeHides = true } }

// ---------- Set-up ----------

// SetScreens replaces the displays. If none is marked primary, the first
// one becomes primary. It doesn't send ScreensChanged; use EmitEvent for
// that.
func (f *Fake) SetScreens(screens ...Screen) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setScreensLocked(screens)
}

func (f *Fake) setScreensLocked(screens []Screen) {
	f.screens = slices.Clone(screens)
	if len(f.screens) == 0 {
		return
	}
	for _, s := range f.screens {
		if s.Primary {
			return
		}
	}
	f.screens[0].Primary = true
}

// SetBenchScreen sets which screen ScreenOfWindow reports, by ID. With "",
// or an ID that isn't one of the screens, it's the screen holding most of
// the content frame, or else the primary screen.
func (f *Fake) SetBenchScreen(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.benchScreen = id
}

// SetOwnApp makes the app with bundleID Bench itself: as on a Mac,
// RunningApps leaves it out, and OwnAppHidden reports whether it's hidden.
func (f *Fake) SetOwnApp(bundleID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.own = bundleID
}

// SetContentFrame sets Bench's window content area, in global coordinates.
// A zero Rect means Bench has no window, as for bench arrange.
func (f *Fake) SetContentFrame(r layout.Rect) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.content = r
	f.contentSet = true
}

// SetTrusted sets whether Bench has Accessibility permission. Without it,
// the Platform methods that use the Accessibility API (Windows, Window,
// SetFrame, Raise, Park, Unminimise, Focus, PressClose, NewWindow, and
// Minimise) fail with an error wrapping ErrNotTrusted, as they do on a Mac.
//
// It also decides whether DockBadges reports any badges, and sends
// DockBadgesChanged when that changes what it reports, as the next
// reading on a Mac would.
func (f *Fake) SetTrusted(trusted bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.badgesChangeLocked(func() { f.trusted = trusted })
}

// SetDefaultBrowser sets the bundle ID that DefaultBrowser returns: "", the
// default, for none.
func (f *Fake) SetDefaultBrowser(bundleID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.defaultBrowser = bundleID
}

// SetChosen sets the path that ChooseToAdd returns, as if the user chose
// it in the open panel: "", the default, as if they cancelled.
func (f *Fake) SetChosen(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chosen = path
}

// SetScreenRecordingAllowed sets whether Bench has Screen Recording
// permission.
func (f *Fake) SetScreenRecordingAllowed(allowed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.screenRecording = allowed
}

// SetScrollBarsShown sets whether macOS shows scroll bars all the time,
// as ScrollBarsShown reports. It's off by default, as with a trackpad. Send
// a ScrollBarsChanged event with EmitEvent for Bench to look again.
func (f *Fake) SetScrollBarsShown(shown bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scrollBars = shown
}

// SetSystemKeys sets the keys that macOS keeps for itself, as SystemKeys
// reports. There are none by default. Send a SystemKeysChanged event with
// EmitEvent for Bench to look again.
func (f *Fake) SetSystemKeys(k SystemKeys) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k.InputSources = append([]string(nil), k.InputSources...)
	f.systemKeys = k
}

// SetStageManager sets whether Stage Manager is on, as StageManager
// reports. It's off by default. Send a StageManagerChanged event with
// EmitEvent for Bench to look again.
func (f *Fake) SetStageManager(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stageManager = on
}

// SetToolPreferences sets the preferences that ToolPreferences reports for a
// window tool, by its bundle ID, whatever its case. Tools have none by
// default.
func (f *Fake) SetToolPreferences(bundleID string, p ToolPreferences) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.toolPrefs == nil {
		f.toolPrefs = map[string]ToolPreferences{}
	}
	f.toolPrefs[strings.ToLower(bundleID)] = ToolPreferences{
		Shortcuts: maps.Clone(p.Shortcuts),
		Numbers:   maps.Clone(p.Numbers),
		Texts:     maps.Clone(p.Texts),
	}
}

// SetDockBadge sets the badge that an app shows on its Dock icon while it
// runs, as DockBadges reports it: a count such as "3", other text such as
// "•", or "" for none. The app needn't be running yet: like a real app, it
// shows the badge again each time it opens. While WatchDockBadges is on
// and Bench has Accessibility permission, it sends DockBadgesChanged when
// what DockBadges reports changes, as do the app opening (LaunchApp and
// OpenApp) and quitting (QuitApp and PressClose), after their AppLaunched
// and AppTerminated. The methods that set up apps, such as AddApp and
// AddWindow, send nothing.
func (f *Fake) SetDockBadge(bundleID, label string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.appForSetupLocked(bundleID, bundleID)
	f.badgesChangeLocked(func() { a.dockBadge = label })
}

// SetHeldModifiers sets the modifier keys that HeldModifiers reports as
// held down. None are by default. The browser preview's page sets them as
// the user presses and lets go of them (Engine.DemoModifiers).
func (f *Fake) SetHeldModifiers(m Modifiers) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held = m
}

// SetMouseHeld sets whether MouseHeld reports the left mouse button held
// down. It isn't by default. DragWindow holds it, and DropWindow lets go.
func (f *Fake) SetMouseHeld(held bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mouseHeld = held
}

// SetAppIcon sets the icon that AppIcon returns for an app, or with nil,
// takes it away. Apps have none by default.
func (f *Fake) SetAppIcon(bundleID string, png []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if png == nil {
		delete(f.appIcons, strings.ToLower(bundleID))
		return
	}
	f.appIcons[strings.ToLower(bundleID)] = png
}

// SetPageIcon sets the icon that PageIcon returns for a Web item's page,
// or with nil, takes it away. Pages have none by default, and, as on the
// Mac, CloseWebView takes it away. It sends no event: send PageIconChanged
// with EmitEvent for that.
func (f *Fake) SetPageIcon(itemID string, png []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if png == nil {
		delete(f.pageIcons, itemID)
		return
	}
	f.pageIcons[itemID] = png
}

// SetLoggingOut sets whether the user is logging out, restarting, or
// shutting down, as LoggingOut reports. It's off by default. It sends no
// event: send LoggingOut with EmitEvent for that.
func (f *Fake) SetLoggingOut(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loggingOut = on
}

// SetLoginItemNeedsApproval sets whether macOS needs the user to allow
// Bench's login item, as LoginItemNeedsApproval reports. It's off by
// default.
func (f *Fake) SetLoginItemNeedsApproval(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loginApproval = on
}

// SetEmitOwnMoves sets whether SetFrame and Park send WindowMoved and
// WindowResized events, as the real Accessibility API does for Bench's own
// moves. It's on by default.
func (f *Fake) SetEmitOwnMoves(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emitOwnMoves = on
}

// FixHiddenWindows makes SetFrame and Park leave the windows of a hidden
// app where they are, and answer err, as the Accessibility API might on a
// Mac (decision 67): nil stands for a move that's said to be made but
// isn't. FixHiddenWindows(false, nil) moves them again.
func (f *Fake) FixHiddenWindows(on bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hiddenFixed, f.hiddenErr = on, err
}

// fixedLocked reports whether SetFrame and Park leave a window where it
// is, as its app is hidden (see FixHiddenWindows).
func (f *Fake) fixedLocked(fw *fakeWindow) bool {
	if !f.hiddenFixed {
		return false
	}
	a := f.apps[fw.w.BundleID]
	return a != nil && a.hidden
}

// SetHistoryLimit makes the Fake keep only the newest n of the calls it has
// logged (see Calls), and likewise of the URLs, previews, and notifications
// it records, so that a Fake that runs for a long time, as the browser
// preview's does, doesn't keep growing. An n of 0, the default, keeps
// everything. The limit applies from the next entry.
func (f *Fake) SetHistoryLimit(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.history = max(n, 0)
}

// SetWebPages makes ShowWebView and LoadWebView send a WebURL event when
// they give a web view an address to load, as the real web view does once the page comes
// into view, with the address in Body and the Page that describe gives for
// it. The Fake's web views load nothing, so it's for the browser preview,
// whose placeholders then show each page's host. describe is called with
// the Fake's lock held, so it mustn't call the Fake. A nil describe, the
// default, sends nothing.
func (f *Fake) SetWebPages(describe func(address string) Page) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webPages = describe
}

// SetWebTitles makes ShowWebView and LoadWebView send a WebTitle event
// when they give a web view an address to load, after its WebURL event, with the title that
// title gives for the address, as the real web view does once the page
// has loaded. A "" title sends nothing. The Fake's web views load nothing,
// so it's for the browser preview, whose pages then have titles. title is
// called with the Fake's lock held, so it mustn't call the Fake. A nil
// title, the default, sends nothing.
func (f *Fake) SetWebTitles(title func(address string) string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.webTitles = title
}

// SetErr makes the Platform method with the given name (such as "SetFrame")
// fail with err until it's set again; a nil err clears it. The call is still
// logged.
func (f *Fake) SetErr(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.errs, method)
		return
	}
	f.errs[method] = err
}

// SetWindowErr is SetErr for one window: the named method fails with err
// only when it's called for that window.
func (f *Fake) SetWindowErr(id WindowID, method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := windowErrKey{id: id, method: method}
	if err == nil {
		delete(f.windowErrs, key)
		return
	}
	f.windowErrs[key] = err
}

// AddApp sets up an app, by default installed (at /Applications/<name>.app)
// and running, regular, and not hidden. For an app the Fake already knows,
// it updates the app: name becomes the running app's name, and an installed
// app keeps its installed name and path unless AppPath is given. It returns
// the app as RunningApps would (with PID 0 if it isn't running). It sends no
// events.
func (f *Fake) AddApp(bundleID, name string, opts ...AppOption) App {
	c := appConfig{installed: true, running: true, regular: true}
	for _, o := range opts {
		o(&c)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.appForSetupLocked(bundleID, name)
	a.name = name
	if c.installed && !a.installed {
		a.installed = true
		a.info = AppInfo{BundleID: bundleID, Name: name, Path: defaultAppPath(name)}
	}
	if !c.installed {
		a.installed = false
	}
	f.configureLocked(a, c, true)
	if c.running && !a.running {
		f.startLocked(a)
	}
	if !c.running && a.running {
		f.stopLocked(a)
	}
	a.hidden = a.running && c.hidden
	return f.appLocked(a)
}

// InstallApp sets up an installed app that isn't running, with name as its
// installed name (and its running name, until AddApp gives another). For an
// app that's already running, it only records the installed app. It sends
// no events.
func (f *Fake) InstallApp(bundleID, name string, opts ...AppOption) {
	c := appConfig{installed: true, regular: true}
	for _, o := range opts {
		o(&c)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.appForSetupLocked(bundleID, name)
	a.installed = true
	a.info = AppInfo{BundleID: bundleID, Name: name, Path: defaultAppPath(name)}
	f.configureLocked(a, c, !a.running)
}

// appForSetupLocked returns the app with bundleID, creating it if needed.
func (f *Fake) appForSetupLocked(bundleID, name string) *fakeApp {
	a := f.apps[bundleID]
	if a == nil {
		a = &fakeApp{bundleID: bundleID, name: name, regular: true}
		f.apps[bundleID] = a
	}
	return a
}

// configureLocked applies the options that don't depend on whether the app
// is new. With setRegular, it also sets whether the app is regular.
func (f *Fake) configureLocked(a *fakeApp, c appConfig, setRegular bool) {
	if setRegular {
		a.regular = c.regular
	}
	if c.path != "" {
		a.info.Path = c.path
	}
	if c.webApp != "" {
		a.info.WebApp = c.webApp
	}
	if c.webURL != "" {
		a.info.WebURL = c.webURL
	}
	if c.makeDelay != nil {
		a.makesWebApps, a.makeDelay = true, *c.makeDelay
	}
	if c.launchDelay != nil {
		a.launchDelay = *c.launchDelay
	}
	if c.setLaunch {
		a.launchWindows = c.launchWindows
	}
	if c.newWindowTitle != "" {
		a.newWindowTitle = c.newWindowTitle
	}
	if c.quitsWithLast {
		a.quitsWithLastWindow = true
	}
	for url, title := range c.urlWindows {
		if a.urlWindows == nil {
			a.urlWindows = map[string]string{}
		}
		a.urlWindows[url] = title
	}
}

func defaultAppPath(name string) string { return "/Applications/" + name + ".app" }

// AddWindow sets up a window of an app, in front of the others, and returns
// it. If the app isn't known, it's added as a running app named after its
// bundle ID; if it isn't running, it's started. It sends no events.
func (f *Fake) AddWindow(bundleID, title string, opts ...WindowOption) Window {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := f.apps[bundleID]
	if a == nil {
		a = &fakeApp{bundleID: bundleID, name: bundleID, regular: true}
		f.apps[bundleID] = a
	}
	if !a.running {
		f.startLocked(a)
	}
	return f.snapshotLocked(f.newWindowLocked(a, title, opts))
}

// startLocked marks an app as running, with a new PID. An app with a Dock
// badge shows it again (see SetDockBadge).
func (f *Fake) startLocked(a *fakeApp) {
	f.appSeq++
	a.seq = f.appSeq
	a.pid = f.nextPID
	f.nextPID++
	a.running = true
	a.launching = false
	a.hidden = false
	a.focused = 0
}

// stopLocked marks an app as not running and destroys its windows. Its Dock
// badge goes with it.
func (f *Fake) stopLocked(a *fakeApp) {
	for _, id := range f.windowIDsLocked(a.bundleID) {
		f.removeWindowLocked(id)
	}
	a.running = false
	a.launching = false
	a.hidden = false
	a.pendingURLs = nil
	a.focused = 0
	if f.active == a.bundleID {
		f.active = ""
	}
}

// dockBadgesLocked returns what DockBadges reports: the badges of the
// running apps that have one, while WatchDockBadges is on and Bench has
// Accessibility permission.
func (f *Fake) dockBadgesLocked() map[string]string {
	badges := map[string]string{}
	if !f.watchingBadges || !f.trusted {
		return badges
	}
	for _, a := range f.apps {
		if a.running && a.dockBadge != "" {
			badges[a.bundleID] = a.dockBadge
		}
	}
	return badges
}

// badgesChangeLocked runs change, and sends DockBadgesChanged if that
// changed what DockBadges reports.
func (f *Fake) badgesChangeLocked(change func()) {
	before := f.dockBadgesLocked()
	change()
	f.badgesChangedSinceLocked(before)
}

// badgesChangedSinceLocked sends DockBadgesChanged if DockBadges reports
// other badges than before.
func (f *Fake) badgesChangedSinceLocked(before map[string]string) {
	if !maps.Equal(before, f.dockBadgesLocked()) {
		f.emitLocked(Event{Kind: DockBadgesChanged})
	}
}

// newWindowLocked creates a window of a running app, in front of the others.
func (f *Fake) newWindowLocked(a *fakeApp, title string, opts []WindowOption) *fakeWindow {
	c := windowConfig{subrole: SubroleStandard}
	for _, o := range opts {
		o(&c)
	}
	f.nextID++
	f.created++
	frame := f.cascadeLocked()
	if c.frame != nil {
		frame = *c.frame
	}
	fw := &fakeWindow{
		w: Window{
			ID:        f.nextID,
			Title:     title,
			Subrole:   c.subrole,
			Minimised: c.minimised,
			Created:   f.created,
		},
		minW:         c.minW,
		minH:         c.minH,
		stepW:        c.stepW,
		stepH:        c.stepH,
		otherDesktop: c.otherDesktop,
		closeAsks:    c.closeAsks,
		closeHides:   c.closeHides,
	}
	fw.w.Frame = fw.constrain(frame)
	fw.w.PID, fw.w.BundleID, fw.w.App = a.pid, a.bundleID, a.name
	if c.fullScreen {
		f.enterFullScreenLocked(fw)
	}
	f.windows[fw.w.ID] = fw
	f.zorder = append([]WindowID{fw.w.ID}, f.zorder...)
	return fw
}

// cascadeLocked returns a default frame for the next window, stepping
// across the primary screen's visible area.
func (f *Fake) cascadeLocked() layout.Rect {
	v := f.primaryLocked().Visible
	if v.W <= 0 || v.H <= 0 {
		v = layout.Rect{X: 0, Y: 25, W: 1512, H: 957}
	}
	step := float64(f.created % 8 * 24)
	return layout.Rect{
		X: v.X + 60 + step,
		Y: v.Y + 40 + step,
		W: max(200, min(900, v.W-240)),
		H: max(150, min(640, v.H-240)),
	}
}

// constrain returns the frame a window accepts for r: its size rounded down
// to whole steps, then up to its minimum size.
func (fw *fakeWindow) constrain(r layout.Rect) layout.Rect {
	if fw.stepW > 0 {
		r.W = math.Floor(r.W/fw.stepW) * fw.stepW
	}
	if fw.stepH > 0 {
		r.H = math.Floor(r.H/fw.stepH) * fw.stepH
	}
	r.W = max(r.W, fw.minW)
	r.H = max(r.H, fw.minH)
	return r
}

func (f *Fake) enterFullScreenLocked(fw *fakeWindow) {
	if fw.w.FullScreen {
		return
	}
	fw.beforeFull = fw.w.Frame
	fw.w.FullScreen = true
	fw.w.Frame = f.benchScreenLocked().Frame
}

func (f *Fake) leaveFullScreenLocked(fw *fakeWindow) {
	if !fw.w.FullScreen {
		return
	}
	fw.w.FullScreen = false
	fw.w.Frame = fw.beforeFull
}

func (f *Fake) removeWindowLocked(id WindowID) {
	fw := f.windows[id]
	if fw == nil {
		return
	}
	delete(f.windows, id)
	f.zorder = slices.DeleteFunc(f.zorder, func(z WindowID) bool { return z == id })
	if a := f.apps[fw.w.BundleID]; a != nil && a.focused == id {
		a.focused = 0
	}
}

// windowIDsLocked returns an app's windows, in creation order.
func (f *Fake) windowIDsLocked(bundleID string) []WindowID {
	var ids []WindowID
	for id, fw := range f.windows {
		if fw.w.BundleID == bundleID {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b WindowID) int { return f.windows[a].w.Created - f.windows[b].w.Created })
	return ids
}

// ---------- Snapshots and lookups ----------

// snapshotLocked returns a window's state as the platform reports it.
func (f *Fake) snapshotLocked(fw *fakeWindow) Window {
	w := fw.w
	a := f.apps[w.BundleID]
	w.Focused = a != nil && f.active == a.bundleID && a.focused == w.ID
	return w
}

// appLocked returns an app as RunningApps reports it.
func (f *Fake) appLocked(a *fakeApp) App {
	app := App{
		BundleID: a.bundleID,
		Name:     a.name,
		Regular:  a.regular,
	}
	if a.running {
		app.PID = a.pid
		app.Hidden = a.hidden
		app.Active = f.active == a.bundleID
	}
	return app
}

// runningAppLocked finds a running app by bundle ID, or by PID if app has no
// bundle ID.
func (f *Fake) runningAppLocked(app App) (*fakeApp, error) {
	if app.BundleID != "" {
		if a := f.apps[app.BundleID]; a != nil && a.running {
			return a, nil
		}
		return nil, fmt.Errorf("%s isn't running", app.BundleID)
	}
	for _, a := range f.apps {
		if a.running && a.pid == app.PID {
			return a, nil
		}
	}
	return nil, fmt.Errorf("no running app has process ID %d", app.PID)
}

// liveWindowLocked returns a window on the current desktop, or ErrNoWindow
// or ErrOtherDesktop.
func (f *Fake) liveWindowLocked(id WindowID) (*fakeWindow, error) {
	fw := f.windows[id]
	if fw == nil {
		return nil, fmt.Errorf("window %d: %w", id, ErrNoWindow)
	}
	if fw.otherDesktop {
		return nil, fmt.Errorf("window %d: %w", id, ErrOtherDesktop)
	}
	return fw, nil
}

// anyWindowLocked returns a window wherever it is, or ErrNoWindow.
func (f *Fake) anyWindowLocked(id WindowID) (*fakeWindow, error) {
	fw := f.windows[id]
	if fw == nil {
		return nil, fmt.Errorf("window %d: %w", id, ErrNoWindow)
	}
	return fw, nil
}

func (f *Fake) primaryLocked() Screen {
	for _, s := range f.screens {
		if s.Primary {
			return s
		}
	}
	if len(f.screens) > 0 {
		return f.screens[0]
	}
	return Screen{}
}

// benchScreenLocked is ScreenOfWindow.
func (f *Fake) benchScreenLocked() Screen {
	if f.benchScreen != "" {
		for _, s := range f.screens {
			if s.ID == f.benchScreen {
				return s
			}
		}
	}
	if f.contentSet && f.content.W > 0 && f.content.H > 0 {
		best, bestArea := Screen{}, 0.0
		for _, s := range f.screens {
			in := Intersect(s.Frame, f.content)
			if area := in.W * in.H; area > bestArea {
				best, bestArea = s, area
			}
		}
		if bestArea > 0 {
			return best
		}
	}
	return f.primaryLocked()
}

// ---------- Events ----------

// emitLocked sends an event without blocking, dropping it if the channel is
// full.
func (f *Fake) emitLocked(ev Event) {
	select {
	case f.events <- ev:
	default:
		f.dropped++
	}
}

// emitWindowLocked sends a window event with the window's state and its app.
func (f *Fake) emitWindowLocked(kind EventKind, fw *fakeWindow) {
	ev := Event{Kind: kind, Window: f.snapshotLocked(fw)}
	if a := f.apps[fw.w.BundleID]; a != nil {
		ev.App = f.appLocked(a)
	}
	f.emitLocked(ev)
}

// EmitEvent sends any event, such as WebTitle, WebNotification,
// NotificationClicked, ScreensChanged, or BenchMoved. Like every event the
// Fake sends, it's dropped (and counted) if the channel is full.
func (f *Fake) EmitEvent(ev Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emitLocked(ev)
}

// ---------- Calls ----------

// Call is one Platform method call, as the Fake logged it. Only the fields
// that the method takes are set.
type Call struct {
	Method string
	// Window is the window's ID, for methods that take a window.
	Window WindowID
	// BundleID is the app's bundle ID, for methods that take an app or a
	// window.
	BundleID string
	// Rect is SetFrame's frame, the frame Park chose, ShowWebView's frame,
	// or ShowPreview's anchor.
	Rect layout.Rect
	// Arg holds any other argument as text: OpenApp's URL, Park's screen
	// ID, a web view ID, with KeepWebViewRunning's flag after a space,
	// SetRaised's and Trusted's flag, Notify's item ID,
	// SetBadge's count, OpenTextFile's and ShowInFinder's path,
	// ChooseToAdd's folder, or OpenSettings' pane.
	Arg string
}

// String formats a call for test failures, such as "SetFrame(w3 com.app
// {0 25 800 600})".
func (c Call) String() string {
	var parts []string
	if c.Window != 0 {
		parts = append(parts, fmt.Sprintf("w%d", c.Window))
	}
	if c.BundleID != "" {
		parts = append(parts, c.BundleID)
	}
	if c.Rect != (layout.Rect{}) {
		parts = append(parts, fmt.Sprintf("{%g %g %g %g}", c.Rect.X, c.Rect.Y, c.Rect.W, c.Rect.H))
	}
	if c.Arg != "" {
		parts = append(parts, fmt.Sprintf("%q", c.Arg))
	}
	return c.Method + "(" + strings.Join(parts, " ") + ")"
}

func (f *Fake) logLocked(c Call) { f.calls = appendKeeping(f.calls, c, f.history) }

// appendKeeping appends v to s, then drops the oldest entries beyond the
// newest limit; a limit of 0 keeps them all. It moves the entries it keeps
// to the front rather than reslicing, so that the array doesn't grow.
func appendKeeping[T any](s []T, v T, limit int) []T {
	s = append(s, v)
	if limit <= 0 || len(s) <= limit {
		return s
	}
	n := copy(s, s[len(s)-limit:])
	clear(s[n:])
	return s[:n]
}

// axMethods are the Platform methods that use the Accessibility API, so fail
// without Accessibility permission.
var axMethods = map[string]bool{
	"Windows": true, "Window": true, "SetFrame": true, "Raise": true, "Park": true,
	"Unminimise": true, "Focus": true, "NewWindow": true, "Minimise": true, "PressClose": true,
}

// errLocked returns the error set for a method, for any window or for id,
// or else ErrNotTrusted for a method that needs Accessibility permission
// while Bench doesn't have it.
func (f *Fake) errLocked(method string, id WindowID) error {
	if id != 0 {
		if err := f.windowErrs[windowErrKey{id: id, method: method}]; err != nil {
			return err
		}
	}
	if err := f.errs[method]; err != nil {
		return err
	}
	if !f.trusted && axMethods[method] {
		return fmt.Errorf("%s: %w", method, ErrNotTrusted)
	}
	return nil
}
