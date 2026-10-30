// Package workspace holds Bench's model (tabs, items, moves, rules) and the
// engine that runs it: it reacts to platform events, switches tabs, arranges
// windows through the windows package, keeps state.json up to date, and sends
// the View to the frontend.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/update"
	"github.com/itbm/bench/internal/windows"
)

// Emitter sends things to the frontend and the shell. The engine calls it
// without holding its lock; implementations must not block for long.
type Emitter interface {
	// State sends a bench:state event.
	State(v View)
	// Toast sends a bench:toast event.
	Toast(message string)
	// ActionToast sends a bench:toast event with a button, such as Undo.
	ActionToast(t Toast)
	// Command sends a bench:command event, and brings Bench's window
	// forward if the command needs it (such as opening the palette).
	Command(c Command)
	// TermData sends a term:data event.
	TermData(d TermData)
	// PopOutTerminal gives a terminal item a window of its own, and
	// CloseTerminalWindow closes it. In the browser preview these do
	// nothing and return nil.
	PopOutTerminal(itemID, title string) error
	CloseTerminalWindow(itemID string)
	// MenuBar tells the shell what the menu bar item shows: the current
	// tab's name, unless the menu_bar setting says otherwise, and the tabs
	// in its menu, with their shortcuts.
	MenuBar(m MenuBar)
	// BringForward asks the shell to show Bench's window, un-minimising it,
	// and activate Bench, as the menu bar item's Show Bench does. In the
	// browser preview it does nothing.
	BringForward()
	// ShortcutsChanged tells the shell that the config's [shortcuts] have
	// changed, as when config.toml is reloaded or Settings ▸ Keyboard is
	// saved, or that the keys macOS keeps for itself may have, for it to
	// register the global shortcuts again (ConfigCopy has the new ones),
	// and report what it found (ReportShortcuts).
	ShortcutsChanged()
	// CheckShortcuts asks the shell whether each field of a [shortcuts]
	// table, by the fields' names in config.toml, would work, without
	// keeping any of them registered, for Settings ▸ Keyboard as the user
	// records shortcuts (Engine.CheckShortcuts). It returns a row for each
	// field, as ReportShortcuts has them. In the headless engine it returns
	// nil.
	CheckShortcuts(fields map[string]string) []ShortcutView
	// PauseShortcuts asks the shell to release the global shortcuts, so
	// that the keys reach Settings ▸ Keyboard as it records one, or, with
	// paused false, to register them again.
	PauseShortcuts(paused bool)
	// FillScreen asks the shell to zoom Bench's window to fill its screen,
	// unless it fills it already. In the browser preview it does nothing.
	FillScreen()
	// Quit asks the shell to quit Bench without asking about terminals
	// that are running something, as the user has answered that already.
	// In the browser preview it does nothing.
	Quit()
	// Relaunch asks the shell to quit Bench as Quit does, and to open it
	// again once it has quit, for macOS to apply Screen Recording
	// permission. In the browser preview it does nothing.
	Relaunch()
	// Appearance tells the shell the appearance setting, for Bench's
	// windows: system, light, or dark (the config.Appearance constants).
	// It's sent with the first view, and whenever the setting changes. In
	// the browser preview it does nothing: the frontend follows the
	// View's settings.
	Appearance(appearance string)
}

// VSCodeServer is a running `code serve-web` (vscode.Server).
type VSCodeServer interface {
	URL(folder string) string
	Stop() error
	// Done is closed when the server has ended, whether by Stop or not.
	Done() <-chan struct{}
}

// LoginItem turns Open at login on and off (Wails' Autostart manager).
type LoginItem interface {
	IsEnabled() (bool, error)
	Enable() error
	Disable() error
}

// Options configure an Engine.
type Options struct {
	Platform platform.Platform
	Clock    clock.Clock // Nil means clock.Real().
	// NewTerms makes the terminal host, given the engine's callbacks.
	// Nil means term.NewManager.
	NewTerms func(cb term.Callbacks) term.Host
	Emitter  Emitter
	// Dir is the support folder holding config.toml, state.json, and
	// parked.json. It's created if needed.
	Dir string
	// StartVSCode starts `code serve-web` the first time a VS Code for the
	// Web item is shown, and again after it fails or stops. Nil means VS
	// Code items show an explanation.
	StartVSCode func(ctx context.Context) (VSCodeServer, error)
	// LoginItem is nil where Open at login isn't available.
	LoginItem LoginItem
	// Demo is true for the browser preview.
	Demo bool
	// Headless is true for `bench arrange`: there's no Bench window, so the
	// pane area is the screen's visible area in global coordinates, Web and
	// Built-in items are skipped, and nothing is persisted.
	Headless bool
	// Log receives debug messages about the engine's decisions, such as
	// a window being undocked or put back. Nil means no logging.
	Log *slog.Logger
	// OpenLink opens a web address in the default browser, for links that
	// terminal programs print. Nil means links don't open.
	OpenLink func(address string) error
	// ChooseFolder shows a folder picker starting in a folder, a full path,
	// and returns the full path of the folder the user chose, or "" if
	// they cancelled. It waits for the user. Nil means there's no picker,
	// as in the browser preview.
	ChooseFolder func(start string) (string, error)
	// Version is Bench's version, as `bench version` prints it, such as
	// "0.2.0 (4f2c1a9e0b7d)". Bench records it in the support folder
	// while it has Accessibility permission, to tell after an update that
	// macOS has stopped applying it (see permissions.go), and compares it
	// with the latest release (see updates.go).
	Version string
	// LatestRelease asks GitHub for Bench's latest release (update.Latest),
	// for the check_for_updates setting. It waits for the network, so the
	// engine calls it without its lock, on a goroutine of its own. Nil
	// means Bench doesn't look, as in the browser preview.
	LatestRelease func(ctx context.Context) (update.Release, error)
	// Updater installs new versions of Bench: Sparkle, in a Bench.app that
	// has it (mac.Updater). With one, it looks for new versions in place
	// of LatestRelease, and the notice of one offers to install it (see
	// updates.go). Nil means Bench only says that there's a new version.
	Updater Updater
}

// ConfigInfo is what the shell needs from the config.
type ConfigInfo struct {
	Path          string
	Shortcuts     shortcuts.Config
	LaunchTimeout time.Duration
}

// Timings. Every wait uses the engine's clock, so tests can drive them.
const (
	// frame is the shortest time between two bench:state events, and
	// between two batches of a terminal's output.
	frame = 16 * time.Millisecond
	// checkEvery is how often a tab switch checks for windows it's waiting
	// for.
	checkEvery = 250 * time.Millisecond
	// settleWait is how long to wait after everything is ready before
	// arranging again, because apps often resize their windows on launch.
	settleWait = 750 * time.Millisecond
	// saveDelay is how long after the last change state.json is saved, and
	// saveMaxDelay the longest any change waits, however often changes come.
	saveDelay    = 500 * time.Millisecond
	saveMaxDelay = 3 * time.Second
	// retitleSaveDelay is how long a new title or address waits to be saved
	// when nothing else has changed (see retitled).
	retitleSaveDelay = 30 * time.Second
	// trustEvery is how often Bench checks for Accessibility permission
	// until it has it.
	trustEvery = 2 * time.Second
	// screenSettle is how long to wait for macOS to settle after the
	// screens change.
	screenSettle = 1500 * time.Millisecond
	// ownFocusWindow is how long after Bench focuses a window a focus event
	// for it counts as Bench's own.
	ownFocusWindow = 500 * time.Millisecond
	// redockDelay is how long Bench waits, after an app moves or resizes a
	// docked window by itself, before putting it back, so that a burst of
	// changes is answered once.
	redockDelay = 300 * time.Millisecond
	// redockLimit is how many times in redockMemory Bench puts back one
	// window that its app keeps moving, before it stops fighting the app.
	redockLimit  = 3
	redockMemory = 10 * time.Second
	// quietFocus is how long after Bench hides or opens an app, or the app
	// it opened finishes launching, focus events for that app's windows in
	// other tabs are put down to Bench: macOS activates the next app when
	// one is hidden, and an app that's opened is activated.
	quietFocus = time.Second
	// dividerGrace is how long after the user drags a divider moves and
	// resizes of docked windows aren't taken for the user dragging them:
	// the mouse is held down near them while Bench resizes them.
	dividerGrace = time.Second
	// goneGrace is how long Bench waits, after it can't find a window whose
	// app is still running, before it removes the window's item: an app
	// that's quitting closes its windows first.
	goneGrace = 1500 * time.Millisecond
	// launchSettle is how long after an app that the add dialog opened has
	// launched Bench waits for a window to put in the pane, before it opens
	// a new one.
	launchSettle = 1500 * time.Millisecond
	// jumpRefocusDelay is how long Bench waits, once it's lowered after the
	// jump palette, before it gives the keyboard back to the App window
	// that had it. The palette closes, lowering Bench, just before it runs
	// the row chosen, which reaches Go as a call of its own and may come
	// second: a row that focuses something, such as a tab, does so first.
	jumpRefocusDelay = 150 * time.Millisecond
)

// firstRunFile marks the welcome guide as dismissed.
const firstRunFile = "first-run-done"

// Notice sources.
const (
	sourceConfig  = "config"
	sourceRestore = "restore"
	sourceState   = "state"
	sourceWindows = "windows"
)

// Engine runs Bench. Its methods are safe for concurrent use.
//
// One mutex guards everything. Bound methods, platform events, and timers
// all take it. The engine never calls the Emitter while holding it: calls
// are queued in out and made by unlock, and bench:state is coalesced by a
// timer that sends at most one View per frame.
type Engine struct {
	opts      Options
	log       *slog.Logger
	p         platform.Platform
	clk       clock.Clock
	em        Emitter
	terms     term.Host
	win       *windows.Manager
	dir       string
	cfgPath   string
	statePath string
	ownPID    int
	manual    bool // Tests feed events themselves instead of the loop.

	ctx    context.Context
	cancel context.CancelFunc
	stop   chan struct{}
	looped chan struct{}

	emitMu sync.Mutex // Keeps bench:state events in order.
	// saveMu keeps state.json saves in order, and guards writtenSeq: the
	// number of the newest snapshot written (see snapshotLocked).
	saveMu     sync.Mutex
	writtenSeq int
	// cfgMu is held across each config change and its save, taken before
	// mu, so that config.toml is written in the order the changes were made.
	// Close takes it too, so as not to quit in the middle of a save. It
	// guards cfgSum: config.toml's Sum when Bench last read or wrote it,
	// which a save checks the file still has (see saveConfig).
	cfgMu  sync.Mutex
	cfgSum config.Sum
	tq     termQueue // Terminal output and events waiting to be sent.
	// focusSeq counts the times the keyboard focus has moved, and focusMu
	// keeps focus-terminal commands in that order, so that one queued
	// before a newer focus is dropped (see sendFocusTerminal).
	focusSeq atomic.Uint64
	focusMu  sync.Mutex

	mu  sync.Mutex
	out []func()

	cfg *config.Config
	// writable is false while config.toml has problems, or can't be read
	// (cantRead): Bench runs with the last good config, or the default one,
	// or the one it had (using says which, for the notice), and doesn't
	// save config.toml. problems lists them; backupOK is set if
	// config.toml.bak loads without problems, for the notice to offer it.
	writable bool
	using    string
	cantRead bool
	problems []string
	backupOK bool
	// followsGood is set while config.toml has problems if the model follows
	// a config that loaded without them, so that Bench can save state.json
	// for it: the last good config, if state.json had its tabs when Bench
	// started, or the config Bench had when a reload found problems. Not
	// the default config, whose tabs aren't the user's.
	followsGood bool
	// outside says whether config.toml has changed outside Bench since
	// Bench last read or wrote it (see checkConfigFile and saveConfig).
	outside outsideChange
	model   *Model

	// stateHeld stops state.json being saved while config.toml is behind the
	// config: from a change to the tabs until config.toml is saved with it
	// (see saveEdit).
	stateHeld bool
	// aside is what Bench couldn't keep of the arrangement, at startup or
	// when it reloaded config.toml, for the notices that say so, until the
	// user restores it or dismisses them (see setaside.go).
	aside *setAsideState

	started, closed bool
	trusted         bool
	needRestore     bool // parked.json waits for Accessibility permission.
	// trustedVersion is the version of Bench that last had Accessibility
	// permission, as the support folder records it, or "" (see
	// permissions.go).
	trustedVersion string
	screenRec      bool
	// screenRecAtStart is whether Bench had Screen Recording permission as
	// it opened, and screenRecAsked whether it has asked macOS for it
	// since: macOS applies it only as Bench opens.
	screenRecAtStart bool
	screenRecAsked   bool
	scrollBars       bool // macOS shows scroll bars all the time (Platform.ScrollBarsShown).
	openAtLogin      bool
	// loginApproval is set while macOS has Bench's login item but needs
	// the user to allow it (Platform.LoginItemNeedsApproval).
	loginApproval bool
	firstRun      bool
	overlay       bool
	// closePress holds the App items whose windows Close is to ask to
	// close once Bench is lowered (see close.go).
	closePress  []string
	benchHidden bool   // Bench's window is hidden or minimised.
	benchActive bool   // Bench is the active app.
	activeApp   string // the active app's bundle ID, as AppActivated last said
	screen      platform.Screen
	area        layout.Rect
	// titleBar is the density that the window's title bar was made with
	// at launch (WindowStart), or "" if Bench has no window of its own.
	titleBar string
	// headers holds whether each displayed pane goes without its header,
	// under pane_headers = "auto", by pane ID, and headerTimer arranges the
	// tab again at headerDue, when one is due to come or go (see
	// headers.go).
	headers     map[string]paneHeader
	headerTimer clock.Timer
	headerDue   time.Time
	// zoom is the pane that fills its tab's pane area for now (ZoomPane),
	// and split the empty pane that SplitPane made for the add dialog to
	// fill (see panes.go).
	zoom  paneZoom
	split emptySplit
	// lastTab is the tab that was current before the current one, which
	// LastTab goes back to, or nil (see lastTabLocked).
	lastTab *Tab
	// filled is whether Bench's window filled its screen when the screens
	// were last settled, for it to fill the screen again after they change
	// (see refillLocked); refillOnShow is set when they changed while the
	// window was hidden or minimised, to look again once it's back.
	// lastScreen is the screen it was on then, which state.json saves, for
	// it to open there next time.
	filled       bool
	refillOnShow bool
	lastScreen   *config.ScreenState
	// dialogs holds the sizes that the user gave Bench's dialogs, by kind
	// (dialogKinds), which state.json saves (SetDialogSize).
	dialogs map[string]config.DialogSize
	// undo holds the changes that Undo can take back, the latest last, and
	// undoSeq numbers them; closedTabs holds the tabs that Reopen closed
	// tab can bring back, the latest first, which state.json saves (see
	// undo.go).
	undo       []*undoStep
	undoSeq    int
	closedTabs []*config.ClosedTab

	running   map[string]platform.App // Running apps, by lower-case bundle ID.
	installed map[string]platform.AppInfo
	// browsed holds the apps the user chose with the add dialog's Browse…,
	// which a scan of the installed apps may not find, by lower-case
	// bundle ID, and browseDir is the folder of what they last chose (see
	// browse.go).
	browsed   map[string]platform.AppInfo
	browseDir string
	unmanaged map[platform.WindowID]platform.Window
	launching map[string]time.Time // Apps being opened, until when.
	openFail  map[string]bool      // Apps that Bench couldn't open.
	notFound  map[string]bool      // Apps that macOS says aren't installed, until they launch.
	// quit holds the apps the user quit while Bench was running, by
	// lower-case bundle ID, until they next open: Bench doesn't open them
	// for a tab (see openMissingLocked). It's saved in state.json.
	quit map[string]bool
	// seen is when Bench last saw each app with items running, by
	// lower-case bundle ID. It's saved in state.json, so that the places of
	// an app that hasn't run for a long time can be forgotten (see
	// forgetStaleLocked).
	seen map[string]time.Time
	// noWindow holds the apps, with the url Bench opened them with
	// (openKey), that a tab switch or Open opened for an item and that
	// opened no window for it, until they next launch.
	noWindow map[string]bool
	// askedFor holds, by openKey, when a tab switch last asked an app for
	// a window, and for which items, until one of them gets a window or the
	// app next launches after the launch timeout (a launch before it may be
	// the answer): a switch soon after, before the launch timeout,
	// waits for that window without asking again, and one after it takes a
	// running app for one that gives no window (see openMissingLocked).
	askedFor map[string]askRecord
	// loggingOut is until when apps that quit are taken to be quitting
	// because the user is logging out, restarting, or shutting down.
	loggingOut time.Time
	// reopening follows the apps that have places to fill as they open
	// again, by lower-case bundle ID (see reopen.go).
	reopening map[string]*reopenState
	// scanned is when Bench last scanned the windows already open, at
	// startup, at login, or when Accessibility permission arrives: every
	// app's period of opening again lasts until a while after it, as apps
	// go on opening for a while after a login (see reopenDueLocked).
	scanned  time.Time
	pending  []pendingPlace
	making   *makingWebApp // The web app a dialog waits for, from AddWebsite or MakeWebApp.
	ownFocus map[platform.WindowID]time.Time
	quiet    map[string]time.Time              // Apps whose focus events are Bench's own until then, by lower-case bundle ID.
	redocks  map[platform.WindowID][]time.Time // When Bench put back windows their apps moved.
	sizedAt  time.Time                         // When the user last dragged a divider.
	webShown map[string]layout.Rect
	webMade  map[string]bool
	// pageIcons counts the page icons Web items' views have reported, to
	// number each (see icons.go).
	pageIcons int
	// webFocus is a Web item whose view should get the keyboard focus once
	// Bench is no longer raised (its view is hidden while it's raised).
	webFocus string
	// focusOnShow is set when the focused pane was to get the keyboard
	// focus while Bench's window was hidden: it does once it's shown.
	focusOnShow bool
	termOn      map[string]bool
	vs          vscodeState

	// jump is the App window that had the keyboard when the jump shortcut
	// opened the palette, to give it back to when the palette closes
	// (see noteJumpLocked), and jumpTimer waits to do that.
	jump      jumpNote
	jumpTimer clock.Timer

	// front is the window the user last gave the keyboard to outside
	// Bench's main window, for AddFrontWindow, and keyPopOut the popped-out
	// item whose window has the keyboard now, for Window ▸ Back to Bench,
	// or "". drag is the window being dragged by its title bar, if any,
	// and tracking whether the platform was last told to watch drags, once
	// trackingSent (see docking.go).
	front        frontNote
	keyPopOut    string
	drag         *dragState
	tracking     bool
	trackingSent bool
	// appearance is the appearance setting that the shell last heard,
	// once appearanceSent (see appearance.go).
	appearance     string
	appearanceSent bool

	notices     map[string][]NoticeView
	noticeOrder []string
	// keyStates is what the shell last reported about each field of the
	// [shortcuts] table, by name (ReportShortcuts).
	keyStates map[string]ShortcutView
	// keysPaused is true while Settings ▸ Keyboard has the global
	// shortcuts released, to record one (PauseShortcuts). It's atomic so
	// that the shell is told the latest, whichever order calls come in.
	keysPaused atomic.Bool

	wait    *waitState
	waitGen int

	dirty       bool
	stateTimer  clock.Timer
	stateSeq    int // Numbers state.json snapshots.
	saveGen     int
	saveTimer   clock.Timer
	saveDue     time.Time // The latest the pending save may be.
	trustTimer  clock.Timer
	screenGen   int
	screenTimer clock.Timer
	redockTimer clock.Timer
	// lastMenuBar is what the menu bar item was last told to show, once
	// menuBarSent.
	lastMenuBar MenuBar
	menuBarSent bool
	// badge is the count on the Dock icon: the items marked as needing
	// the user (see attention.go).
	badge int
	// updates is what Bench knows of its releases, which state.json saves,
	// and updateTimer looks again when that's due (see updates.go).
	updates     config.UpdatesState
	updateTimer clock.Timer
}

// jumpNote is the App item that the focused pane showed, with its app
// frontmost, when the jump shortcut was pressed, and focusSeq then.
type jumpNote struct {
	itemID string
	seq    uint64
}

// pendingPlace sends an app's next new windows to a pane, after the add
// dialog asked for a new window (once) or opened the app.
type pendingPlace struct {
	bundleID string
	tab      *Tab
	pane     string
	until    time.Time
	once     bool
	// launch follows the windows of an app the add dialog opened: if none
	// of them goes to the pane, Bench opens a new window.
	launch *launchState
	// item is a placeholder in the pane, for a web app that has taken a
	// Web item's place (see openWebAppForLocked), that the window goes to
	// while it has none, leaving what the pane shows as it is.
	item string
}

// New loads the config and state from opts.Dir (writing the default config
// on first run), restores windows left parked by a crashed run, and builds
// the model. It doesn't touch any window until Start.
func New(opts Options) (*Engine, error) {
	if opts.Platform == nil {
		return nil, errors.New("starting the workspace: there's no platform")
	}
	if opts.Dir == "" {
		return nil, errors.New("starting the workspace: there's no support folder")
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating Bench's support folder: %w", err)
	}
	e := newEngine(opts)
	e.win = windows.New(e.p, e.clk, filepath.Join(e.dir, config.ParkedFile))
	e.trusted = e.p.Trusted(false)
	if e.trusted {
		e.restoreCrashed()
	} else {
		e.needRestore = true
	}
	e.loadConfig()
	e.loadRunning()
	newTerms := opts.NewTerms
	if newTerms == nil {
		newTerms = func(cb term.Callbacks) term.Host { return term.NewManager(cb) }
	}
	e.terms = newTerms(e.termCallbacks())
	e.firstRun = !fileExists(filepath.Join(e.dir, firstRunFile))
	e.trustedVersion = e.readTrustedVersion()
	if e.trusted {
		e.noteTrustedLocked()
	}
	if opts.LoginItem != nil {
		e.openAtLogin, _ = opts.LoginItem.IsEnabled()
		e.loginApproval = !e.openAtLogin && e.p.LoginItemNeedsApproval()
	}
	e.screen = e.p.ScreenOfWindow()
	e.screenRec = e.p.ScreenRecordingAllowed(false)
	e.screenRecAtStart = e.screenRec
	e.scrollBars = e.p.ScrollBarsShown()
	e.stageManagerNoticeLocked()
	if err := e.loadModel(); err != nil {
		return nil, err
	}
	// The shell opens the window filling its screen with this setting (see
	// WindowStart).
	e.filled = e.cfg.Settings.FillScreen && !opts.Headless && !opts.Demo
	return e, nil
}

// newEngine returns an engine with its maps made and its options applied.
func newEngine(opts Options) *Engine {
	clk := opts.Clock
	if clk == nil {
		clk = clock.Real()
	}
	em := opts.Emitter
	if em == nil {
		em = nopEmitter{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	log := opts.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Engine{
		opts:      opts,
		log:       log,
		p:         opts.Platform,
		clk:       clk,
		em:        em,
		dir:       opts.Dir,
		cfgPath:   filepath.Join(opts.Dir, config.ConfigFile),
		statePath: filepath.Join(opts.Dir, config.StateFile),
		ownPID:    os.Getpid(),
		ctx:       ctx,
		cancel:    cancel,
		stop:      make(chan struct{}),
		running:   map[string]platform.App{},
		unmanaged: map[platform.WindowID]platform.Window{},
		headers:   map[string]paneHeader{},
		launching: map[string]time.Time{},
		openFail:  map[string]bool{},
		notFound:  map[string]bool{},
		quit:      map[string]bool{},
		seen:      map[string]time.Time{},
		noWindow:  map[string]bool{},
		askedFor:  map[string]askRecord{},
		reopening: map[string]*reopenState{},
		ownFocus:  map[platform.WindowID]time.Time{},
		quiet:     map[string]time.Time{},
		redocks:   map[platform.WindowID][]time.Time{},
		webShown:  map[string]layout.Rect{},
		webMade:   map[string]bool{},
		termOn:    map[string]bool{},
		notices:   map[string][]NoticeView{},
		tq:        termQueue{data: map[string][]byte{}},
	}
}

// restoreCrashed puts back windows left parked by a run that crashed, or
// that the last run couldn't put back as it quit. What still can't be put
// back stays recorded, for RestoreAll to try again.
func (e *Engine) restoreCrashed() {
	e.needRestore = false
	if err := e.win.RestoreCrashed(); err != nil {
		e.setNoticesLocked(sourceRestore, []string{"Bench couldn't put back every window it moved before it last quit: " + err.Error()})
	}
}

// loadConfig reads config.toml, writing the default config there on first
// run, and records its Sum. A config that loads without problems is kept as
// the last good one too. If the file has problems, or can't be read, Bench
// runs with the last good config, so that the user's tabs stay, or with the
// default config if there's none, and leaves the file alone.
func (e *Engine) loadConfig() {
	cfg, problems, data, err := config.LoadFile(e.cfgPath, func() []byte { return config.Format(e.defaultConfig()) })
	if err == nil && len(problems) == 0 {
		e.cfg, e.writable, e.cfgSum = cfg, true, config.SumOf(data)
		e.keepGood(data)
		return
	}
	e.cfg = config.LoadGood(e.goodPath())
	using := usingGood
	if e.cfg == nil {
		e.cfg = e.defaultConfig()
		using = usingDefault
	}
	if err != nil {
		e.setProblemsLocked(using, true, []string{cantReadText(err)})
	} else {
		e.cfgSum = config.SumOf(data)
		e.setProblemsLocked(using, false, problemTexts(problems))
	}
	// Until loadModel finds whether state.json fits it.
	e.followsGood = using == usingGood
}

// defaultConfig is the config Bench starts with: one tab holding the
// default browser, since a Mac may have no other app that Bench could put
// in a tab. The browser preview, whose apps are all there, starts with the
// example config instead.
func (e *Engine) defaultConfig() *config.Config {
	if e.opts.Demo {
		return config.Example()
	}
	return config.DefaultFor(e.p.DefaultBrowser())
}

// loadModel restores the arrangement from state.json, or builds it from the
// config, keeping the arrangement of each tab that the config still has. If
// Bench couldn't keep some or all of it, a notice says so. While
// config.toml has problems, state.json is restored only if it has the tabs
// of the last good config that Bench runs with, and then Bench goes on
// saving it (followsGood). Otherwise it's left alone: that config may not be
// the user's, such as the default config, and a tab that shares a name with
// one of theirs isn't theirs. Once the file is fixed, the notice offers to
// restore the arrangement.
func (e *Engine) loadModel() error {
	usingGood := e.followsGood
	e.followsGood = false
	if !e.opts.Headless {
		var st *config.State
		var aside *config.SetAside
		var err error
		if e.writable {
			st, aside, err = config.LoadState(e.statePath, e.cfg)
		} else {
			st, aside, err = config.PeekState(e.statePath)
		}
		switch {
		case err != nil && st == nil:
			e.setNoticesLocked(sourceState, []string{"Bench couldn't read state.json: " + err.Error()})
		case err != nil:
			e.setNoticesLocked(sourceState, []string{"Bench couldn't keep a copy of state.json: " + err.Error()})
		}
		var from []int
		if st != nil {
			from = stateTabs(st, e.cfg)
			switch {
			case e.writable:
			case sameTabs(st, e.cfg):
				e.followsGood = usingGood
			default:
				from = slices.Repeat([]int{-1}, len(e.cfg.Tabs))
			}
		}
		e.noteSetAsideLocked(aside, st, from)
		if st != nil {
			e.model = modelFromTabs(st, e.cfg, from, e.appName)
			e.model.hidden = e.hiddenLocked
			e.dropDeadFloatingLocked()
			e.loadAppStates(st.Apps, st.Ran)
			e.lastScreen = st.Screen
			e.dialogs = st.Dialogs
			e.closedTabs = closedTabsOf(st.Closed)
			if st.Updates != nil {
				e.updates = *st.Updates
			}
			return nil
		}
	}
	m, err := modelFromConfig(e.cfg, e.appName)
	if err != nil {
		return fmt.Errorf("building the tabs from the config: %w", err)
	}
	e.model = m
	e.model.hidden = e.hiddenLocked
	return nil
}

// loadRunning records the running apps.
func (e *Engine) loadRunning() {
	e.running = map[string]platform.App{}
	for _, app := range e.p.RunningApps() {
		if app.BundleID != "" {
			e.running[strings.ToLower(app.BundleID)] = app
		}
	}
}

// appName returns an app's name: the running app's, else the installed
// app's, else one made from its bundle ID.
func (e *Engine) appName(bundleID string) string {
	key := strings.ToLower(bundleID)
	if app, ok := e.running[key]; ok && app.Name != "" {
		return apps.DisplayName(app.Name)
	}
	if e.installed == nil {
		e.loadInstalled()
	}
	if info, ok := e.installed[key]; ok && info.Name != "" {
		return apps.DisplayName(info.Name)
	}
	return apps.NameFromBundleID(bundleID)
}

// loadInstalled records the installed apps.
func (e *Engine) loadInstalled() []platform.AppInfo {
	list := e.p.InstalledApps()
	e.setInstalledLocked(list)
	return list
}

// setInstalledLocked records the installed apps from a scan made without
// the lock, and the apps the user chose with Browse… that it didn't find.
func (e *Engine) setInstalledLocked(list []platform.AppInfo) {
	e.installed = map[string]platform.AppInfo{}
	for _, info := range list {
		e.installed[strings.ToLower(info.BundleID)] = info
	}
	for key, info := range e.browsed {
		if _, ok := e.installed[key]; !ok {
			e.installed[key] = info
		}
	}
}

// Start begins handling platform events, arranges the current tab, and
// opens the windows of popped-out Web and Built-in items that had them.
func (e *Engine) Start() {
	e.lock()
	defer e.unlock()
	if e.started || e.closed {
		return
	}
	e.beginLocked()
	e.switchLocked(e.model.Current, switchOpts{})
	e.reopenOwnWindowsLocked()
	e.scheduleUpdateCheckLocked()
}

// reopenOwnWindowsLocked gives the popped-out Web and Built-in items that
// had windows of their own when Bench last quit their windows again.
func (e *Engine) reopenOwnWindowsLocked() {
	if e.opts.Headless {
		return
	}
	for _, id := range e.model.Floating {
		it := e.model.Items[id]
		if it == nil || !it.OwnWindow {
			continue
		}
		switch it.Kind {
		case KindWeb:
			if it.VSCode != "" && e.vs.server == nil {
				// VS Code for the Web has no address until its server has
				// started. The item keeps OwnWindow meanwhile, so that
				// state.json still records the window, and gets the window
				// once the server is up (see reopenVSCodeWindowsLocked).
				e.ensureVSCodeLocked()
				continue
			}
			e.reopenWebWindowLocked(it)
		case KindBuiltin:
			e.startTerminalLocked(it)
			em, title := e.em, labelOf(it)
			e.later(func() {
				if err := em.PopOutTerminal(id, title); err != nil {
					e.lock()
					defer e.unlock()
					if it := e.model.Items[id]; it != nil {
						it.OwnWindow = false
						e.changed()
					}
				}
			})
		}
	}
}

// reopenVSCodeWindowsLocked gives the popped-out VS Code for the Web items
// that waited for the server at startup their windows again, once it has
// started.
func (e *Engine) reopenVSCodeWindowsLocked() {
	if e.opts.Headless || e.vs.server == nil {
		return
	}
	for _, id := range e.model.Floating {
		if it := e.model.Items[id]; it != nil && it.VSCode != "" && it.OwnWindow && !e.webMade[id] {
			e.reopenWebWindowLocked(it)
		}
	}
}

// reopenWebWindowLocked gives a popped-out Web item its own window again.
func (e *Engine) reopenWebWindowLocked(it *Item) {
	it.OwnWindow = false
	if err := e.popOutWebLocked(it); err != nil {
		e.log.Warn("couldn't reopen a web page's window", "item", it.ID, "error", err)
	}
	e.changed()
}

// beginLocked scans the running apps' windows and starts the event loop.
func (e *Engine) beginLocked() {
	e.started = true
	e.loadRunning()
	e.noteRunningAppsLocked()
	if e.trusted {
		e.scanLocked()
	} else {
		e.scheduleTrustCheckLocked()
	}
	// The apps that macOS reopens at login get a while to open first.
	e.after(e.reopenSpan(reopenAfterScan), e.forgetStaleLocked)
	e.scheduleStaleCheckLocked()
	if !e.manual {
		e.looped = make(chan struct{})
		go e.loop()
	}
}

// loop handles platform events until Close.
func (e *Engine) loop() {
	defer close(e.looped)
	events := e.p.Events()
	for {
		select {
		case <-e.stop:
			return
		case ev := <-events:
			e.handle(ev)
		}
	}
}

// Close puts every parked window back, keeps the terminals' output (see
// output.go), ends terminal sessions, stops the VS Code server, and saves
// state.json, unless it's held (see saveEdit). It's safe to call more than
// once.
// Windows it couldn't put back stay in parked.json, for the watchdog or the
// next run; if Bench never got Accessibility permission, parked.json is left
// as it is.
func (e *Engine) Close() error {
	// A save from Tab properties or Settings in progress writes config.toml
	// and then state.json (see saveEdit), so let it finish first: quitting
	// between the two would leave a state.json that the next start sets
	// aside.
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	if e.closed {
		e.unlock()
		return nil
	}
	e.closed = true
	e.cancel()
	e.stopTimersLocked()
	var st *config.State
	var seq int
	if e.persistent() {
		st, seq = e.snapshotLocked()
	}
	output := e.outputPlanLocked(st != nil)
	var errs []error
	// Without Accessibility permission Bench has moved nothing, and
	// RestoreAll would clear parked.json, where a crashed run's windows are
	// still waiting to be put back.
	e.log.Debug("closing: putting windows back", "skipped", e.needRestore)
	if !e.needRestore {
		if err := e.win.RestoreAll(); err != nil {
			errs = append(errs, fmt.Errorf("putting windows back: %w", err))
		}
	}
	srv := e.vs.server
	e.vs.server = nil
	looped := e.looped
	e.unlock()

	close(e.stop)
	if looped != nil {
		<-looped
	}
	if err := e.saveOutput(output); err != nil {
		errs = append(errs, err)
	}
	e.terms.CloseAll()
	if srv != nil {
		if err := srv.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stopping VS Code for the Web: %w", err))
		}
	}
	if st != nil {
		if err := e.writeState(st, seq); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// stopTimersLocked stops every pending timer.
func (e *Engine) stopTimersLocked() {
	for _, t := range []clock.Timer{e.stateTimer, e.saveTimer, e.trustTimer, e.screenTimer, e.updateTimer} {
		if t != nil {
			t.Stop()
		}
	}
	e.cancelWaitLocked()
}

// lock takes the engine's lock.
func (e *Engine) lock() { e.mu.Lock() }

// unlock releases the lock, then makes the Emitter calls queued while it
// was held.
func (e *Engine) unlock() {
	out := e.out
	e.out = nil
	e.mu.Unlock()
	for _, f := range out {
		f()
	}
}

// later queues a call to make once the lock is released.
func (e *Engine) later(f func()) { e.out = append(e.out, f) }

// toastLocked queues a toast.
func (e *Engine) toastLocked(msg string) {
	em := e.em
	e.later(func() { em.Toast(msg) })
}

// after runs f with the lock held after d, unless the engine has closed.
func (e *Engine) after(d time.Duration, f func()) clock.Timer {
	return e.clk.AfterFunc(d, func() {
		e.lock()
		defer e.unlock()
		if !e.closed {
			f()
		}
	})
}

// touch marks the view as changed, so that a bench:state goes out within a
// frame.
func (e *Engine) touch() {
	e.dirty = true
	if e.stateTimer == nil && !e.closed {
		e.stateTimer = e.clk.AfterFunc(frame, e.flushState)
	}
}

// changed marks the view as changed and saves state.json soon.
func (e *Engine) changed() {
	e.touch()
	e.scheduleSaveLocked()
}

// retitled marks the view as changed by a new title or address, which
// state.json keeps but which needn't be saved soon. Pages, apps, and
// terminal programs can change them many times a second, so they don't put
// off a save that's due: they're saved with the next change, or after
// retitleSaveDelay.
func (e *Engine) retitled() {
	e.touch()
	e.scheduleLazySaveLocked()
}

// flushState sends the view if it has changed since the last one, and the
// Dock icon's badge if that has. Marks come off the items the user can see
// first (see attention.go).
func (e *Engine) flushState() {
	e.emitMu.Lock()
	defer e.emitMu.Unlock()
	e.lock()
	e.stateTimer = nil
	if !e.dirty || e.closed {
		e.unlock()
		return
	}
	e.dirty = false
	e.clearSeenLocked()
	v := e.buildViewLocked()
	bar, barChanged := e.menuBarChangedLocked(v)
	badge, badgeChanged := e.badgeChangedLocked()
	track, trackChanged := e.dragTrackingChangedLocked()
	look, lookChanged := e.appearanceChangedLocked()
	e.unlock()
	e.em.State(v)
	if barChanged {
		e.em.MenuBar(bar)
	}
	if badgeChanged {
		e.p.SetBadge(badge)
	}
	if trackChanged {
		e.p.TrackDrags(track)
	}
	if lookChanged {
		e.em.Appearance(look)
	}
}

// View returns the current view.
func (e *Engine) View() View {
	e.lock()
	defer e.unlock()
	return e.buildViewLocked()
}

// Problems returns the config's problems, if it had any when loaded.
func (e *Engine) Problems() []string {
	e.lock()
	defer e.unlock()
	return append([]string(nil), e.problems...)
}

// ConfigCopy returns what the shell needs from the config.
func (e *Engine) ConfigCopy() ConfigInfo {
	e.lock()
	defer e.unlock()
	return ConfigInfo{
		Path:          e.cfgPath,
		Shortcuts:     e.cfg.Shortcuts,
		LaunchTimeout: e.cfg.Settings.LaunchTimeout.Duration,
	}
}

// SetNotices replaces the notices from one source (such as "shortcuts"),
// shown in the status bar.
func (e *Engine) SetNotices(source string, notices []string) {
	e.lock()
	defer e.unlock()
	e.setNoticesLocked(source, notices)
	e.touch()
}

// setNoticesLocked replaces one source's notices, keeping the order in
// which sources first reported any.
func (e *Engine) setNoticesLocked(source string, notices []string) {
	views := make([]NoticeView, len(notices))
	for i, text := range notices {
		views[i] = NoticeView{Text: text}
	}
	e.setNoticeViewsLocked(source, views)
}

// setNoticeViewsLocked replaces one source's notices, which may have
// buttons, keeping the order in which sources first reported any.
func (e *Engine) setNoticeViewsLocked(source string, notices []NoticeView) {
	if len(notices) == 0 {
		delete(e.notices, source)
		return
	}
	if !slices.Contains(e.noticeOrder, source) {
		e.noticeOrder = append(e.noticeOrder, source)
	}
	e.notices[source] = append([]NoticeView(nil), notices...)
}

// launchTimeout returns how long to wait for apps to open.
func (e *Engine) launchTimeout() time.Duration {
	if d := e.cfg.Settings.LaunchTimeout.Duration; d > 0 {
		return d
	}
	return config.DefaultSettings().LaunchTimeout.Duration
}

// fileExists reports whether a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// nopEmitter is the Emitter when none is given, as for bench arrange.
type nopEmitter struct{}

func (nopEmitter) State(View)                          {}
func (nopEmitter) Toast(string)                        {}
func (nopEmitter) ActionToast(Toast)                   {}
func (nopEmitter) Command(Command)                     {}
func (nopEmitter) TermData(TermData)                   {}
func (nopEmitter) PopOutTerminal(string, string) error { return nil }
func (nopEmitter) CloseTerminalWindow(string)          {}
func (nopEmitter) MenuBar(MenuBar)                     {}
func (nopEmitter) BringForward()                       {}
func (nopEmitter) ShortcutsChanged()                   {}
func (nopEmitter) PauseShortcuts(bool)                 {}
func (nopEmitter) FillScreen()                         {}
func (nopEmitter) Quit()                               {}
func (nopEmitter) Relaunch()                           {}
func (nopEmitter) Appearance(string)                   {}

func (nopEmitter) CheckShortcuts(map[string]string) []ShortcutView { return nil }
