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
	"time"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/windows"
)

// Emitter sends things to the frontend and the shell. The engine calls it
// without holding its lock; implementations must not block for long.
type Emitter interface {
	// State sends a bench:state event.
	State(v View)
	// Toast sends a bench:toast event.
	Toast(message string)
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
	// CurrentTab tells the shell the current tab's name, for the menu bar
	// item.
	CurrentTab(name string, tabs []string)
}

// VSCodeServer is a running `code serve-web` (vscode.Server).
type VSCodeServer interface {
	URL(folder string) string
	Stop() error
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
	// Web item is shown. Nil means VS Code items show an explanation.
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
	// saveDelay is how long after the last change state.json is saved.
	saveDelay = 500 * time.Millisecond
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
)

// firstRunFile marks the permissions guide as dismissed.
const firstRunFile = "first-run-done"

// Notice sources.
const (
	sourceConfig  = "config"
	sourceRestore = "restore"
	sourceState   = "state"
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
	cfgMu sync.Mutex
	tq    termQueue // Terminal output and events waiting to be sent.

	mu  sync.Mutex
	out []func()

	cfg      *config.Config
	writable bool
	problems []string
	model    *Model

	started, closed bool
	trusted         bool
	needRestore     bool // parked.json waits for Accessibility permission.
	screenRec       bool
	openAtLogin     bool
	firstRun        bool
	overlay         bool
	screen          platform.Screen
	area            layout.Rect

	running   map[string]platform.App // Running apps, by lower-case bundle ID.
	installed map[string]platform.AppInfo
	unmanaged map[platform.WindowID]platform.Window
	launching map[string]time.Time // Apps being opened, until when.
	openFail  map[string]bool      // Apps that Bench couldn't open.
	pending   []pendingPlace
	ownFocus  map[platform.WindowID]time.Time
	redocks   map[platform.WindowID][]time.Time // When Bench put back windows their apps moved.
	webShown  map[string]layout.Rect
	webMade   map[string]bool
	termOn    map[string]bool
	vs        vscodeState

	notices     map[string][]string
	noticeOrder []string

	wait    *waitState
	waitGen int

	dirty       bool
	stateTimer  clock.Timer
	stateSeq    int // Numbers state.json snapshots.
	saveGen     int
	saveTimer   clock.Timer
	trustTimer  clock.Timer
	screenGen   int
	screenTimer clock.Timer
	redockTimer clock.Timer
	lastTab     string
	lastTabs    []string
}

// pendingPlace sends an app's next new windows to a pane, after the add
// dialog asked for a new window or opened the app.
type pendingPlace struct {
	bundleID string
	tab      *Tab
	pane     string
	until    time.Time
	once     bool
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
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating Bench's support folder: %w", err)
	}
	e := newEngine(opts)
	e.trusted = e.p.Trusted(false)
	if e.trusted {
		e.restoreCrashed()
	} else {
		e.needRestore = true
	}
	e.win = windows.New(e.p, e.clk, filepath.Join(e.dir, config.ParkedFile))
	e.loadConfig()
	e.loadRunning()
	newTerms := opts.NewTerms
	if newTerms == nil {
		newTerms = func(cb term.Callbacks) term.Host { return term.NewManager(cb) }
	}
	e.terms = newTerms(e.termCallbacks())
	e.firstRun = !fileExists(filepath.Join(e.dir, firstRunFile))
	if opts.LoginItem != nil {
		e.openAtLogin, _ = opts.LoginItem.IsEnabled()
	}
	e.screen = e.p.ScreenOfWindow()
	e.screenRec = e.p.ScreenRecordingAllowed(false)
	if err := e.loadModel(); err != nil {
		return nil, err
	}
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
		launching: map[string]time.Time{},
		openFail:  map[string]bool{},
		ownFocus:  map[platform.WindowID]time.Time{},
		redocks:   map[platform.WindowID][]time.Time{},
		webShown:  map[string]layout.Rect{},
		webMade:   map[string]bool{},
		termOn:    map[string]bool{},
		notices:   map[string][]string{},
		tq:        termQueue{data: map[string][]byte{}},
	}
}

// restoreCrashed puts back windows left parked by a run that crashed.
func (e *Engine) restoreCrashed() {
	e.needRestore = false
	if err := windows.RestoreFromFile(e.p, filepath.Join(e.dir, config.ParkedFile)); err != nil {
		e.setNoticesLocked(sourceRestore, []string{"Bench couldn't put back every window it moved before it last quit: " + err.Error()})
	}
}

// loadConfig reads config.toml. If it has problems, Bench runs with the
// default config and leaves the file alone.
func (e *Engine) loadConfig() {
	cfg, problems, err := config.Load(e.cfgPath)
	switch {
	case err != nil:
		e.cfg = config.Default()
		e.problems = []string{"Bench couldn't read config.toml, so it's using the default config: " + err.Error()}
	case len(problems) > 0:
		e.cfg = config.Default()
		for _, p := range problems {
			e.problems = append(e.problems, "config.toml: "+p.Error())
		}
	default:
		e.cfg = cfg
		e.writable = true
	}
}

// loadModel restores the arrangement from state.json, or builds it from the
// config. While config.toml has problems, state.json is left alone, so that
// fixing the file brings the arrangement back.
func (e *Engine) loadModel() error {
	if e.writable && !e.opts.Headless {
		st, err := config.LoadState(e.statePath, e.cfg)
		if err != nil {
			e.setNoticesLocked(sourceState, []string{"Bench couldn't read state.json: " + err.Error()})
		}
		if st != nil {
			e.model = modelFromState(st, e.cfg, e.appName)
			e.dropDeadFloatingLocked()
			return nil
		}
	}
	m, err := modelFromConfig(e.cfg, e.appName)
	if err != nil {
		return fmt.Errorf("building the tabs from the config: %w", err)
	}
	e.model = m
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
	e.installed = map[string]platform.AppInfo{}
	for _, info := range list {
		e.installed[strings.ToLower(info.BundleID)] = info
	}
	return list
}

// Start begins handling platform events and arranges the current tab.
func (e *Engine) Start() {
	e.lock()
	defer e.unlock()
	if e.started || e.closed {
		return
	}
	e.beginLocked()
	e.switchLocked(e.model.Current, switchOpts{})
}

// beginLocked scans the running apps' windows and starts the event loop.
func (e *Engine) beginLocked() {
	e.started = true
	e.loadRunning()
	if e.trusted {
		e.scanLocked()
	} else {
		e.scheduleTrustCheckLocked()
	}
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

// Close puts every parked window back, ends terminal sessions, stops the VS
// Code server, and saves state.json. It's safe to call more than once. If
// Bench never got Accessibility permission, parked.json is left for the next
// run (or the watchdog) to restore.
func (e *Engine) Close() error {
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
	for _, t := range []clock.Timer{e.stateTimer, e.saveTimer, e.trustTimer, e.screenTimer} {
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

// flushState sends the view if it has changed since the last one.
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
	v := e.buildViewLocked()
	name, names, tabsChanged := e.tabNamesChangedLocked()
	e.unlock()
	e.em.State(v)
	if tabsChanged {
		e.em.CurrentTab(name, names)
	}
}

// tabNamesChangedLocked returns the current tab's name and every tab's,
// and whether they've changed since they were last sent.
func (e *Engine) tabNamesChangedLocked() (string, []string, bool) {
	names := make([]string, len(e.model.Tabs))
	for i, t := range e.model.Tabs {
		names[i] = t.Name
	}
	name := ""
	if t := e.model.CurrentTab(); t != nil {
		name = t.Name
	}
	if name == e.lastTab && slices.Equal(names, e.lastTabs) {
		return name, names, false
	}
	e.lastTab, e.lastTabs = name, names
	return name, names, true
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
	if len(notices) == 0 {
		delete(e.notices, source)
		return
	}
	if _, ok := e.notices[source]; !ok {
		e.noticeOrder = append(e.noticeOrder, source)
	}
	e.notices[source] = append([]string(nil), notices...)
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
func (nopEmitter) Command(Command)                     {}
func (nopEmitter) TermData(TermData)                   {}
func (nopEmitter) PopOutTerminal(string, string) error { return nil }
func (nopEmitter) CloseTerminalWindow(string)          {}
func (nopEmitter) CurrentTab(string, []string)         {}
