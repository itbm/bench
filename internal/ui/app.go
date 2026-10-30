//go:build darwin || server

// Package ui is Bench's app shell, built on Wails v3: the windows (the main
// window, popped-out terminals' windows, and Acknowledgements), the bound
// Service the frontend calls, the events Go sends it, global shortcuts, the
// app menu, the menu bar item, and Open at login.
//
// The same code runs in two builds. In the macOS app (darwin without the
// server tag), Run opens a native window. In the browser preview (the server
// tag), Wails serves the UI over HTTP instead, and Run leaves out everything
// native: windows, menus, the menu bar item, global shortcuts (the frontend
// handles keys itself), and Open at login.
package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/itbm/bench/frontend"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/update"
	"github.com/itbm/bench/internal/workspace"
)

// UniqueID identifies Bench to Wails' single-instance lock, which on macOS
// is an flock(2) lock on <UniqueID>.lock in NSTemporaryDirectory().
const UniqueID = "com.github.itbm.bench"

// shutdownWait is how long quitting waits for the engine to put windows back
// before letting the app exit. If it runs out, the watchdog puts them back.
const shutdownWait = 10 * time.Second

// Options configure Run.
type Options struct {
	// Platform is the macOS platform (mac.Platform) in the app, or the demo
	// platform (platform.NewDemo) in the browser preview. If it has a
	// Close method, Run calls it when Bench quits.
	Platform platform.Platform
	// Attach is given the main window's NSWindow once the window exists
	// (mac.Platform.AttachWindow). Nil in the preview.
	Attach func(nsWindow unsafe.Pointer)
	// Fill is given the main window's NSWindow on the main thread, before
	// the window is first shown, when the fill_screen setting is on, to
	// zoom it to fill its screen at once (mac.Platform.FillWindow). Nil in
	// the preview.
	Fill func(nsWindow unsafe.Pointer)
	// TagPopOut is given a popped-out terminal's NSWindow, with its item's
	// ID, once the window exists, for the platform to report when it has
	// the keyboard and when the user drags it, as it does for popped-out
	// web pages' windows (mac.Platform.TagPopOutWindow). Nil in the
	// preview.
	TagPopOut func(nsWindow unsafe.Pointer, itemID string)
	// Appearance is given each of Bench's windows' NSWindow, once the
	// window exists and whenever the appearance setting changes, with the
	// setting (system, light, or dark), for the window to look that way
	// and keep a background that matches
	// (mac.Platform.SetWindowAppearance). Nil in the preview.
	Appearance func(nsWindow unsafe.Pointer, appearance string)
	// SystemDark reports whether macOS is dark now, for Bench's windows to
	// open with the system appearance's background: Wails can't say until
	// the app runs, and the main window is made before
	// (mac.Platform.SystemDark). Nil in the preview.
	SystemDark func() bool
	// NewTerms makes the terminal host: term.NewManager in the app, or
	// term.NewEcho in the preview. Nil means term.NewManager.
	NewTerms func(cb term.Callbacks) term.Host
	// StartVSCode starts `code serve-web` for VS Code for the Web items.
	// Nil means VS Code items show an explanation.
	StartVSCode func(ctx context.Context) (workspace.VSCodeServer, error)
	// Dir is the support folder, holding config.toml, state.json, and
	// parked.json.
	Dir string
	// Demo is true for the browser preview.
	Demo bool
	// Port is the preview's HTTP port; 0 means 8080. The app ignores it.
	Port int
	// Watchdog starts `bench watchdog --dir <Dir>` (this executable), which
	// puts parked windows back if Bench crashes. For the macOS app only.
	Watchdog bool
	// Recover, if not nil, puts right what a Bench that crashed left
	// behind, other than parked windows, which the engine puts back
	// itself: cmd/bench stops the VS Code for the Web server it left
	// running. Run calls it once it knows that no other Bench is running,
	// before it starts the engine.
	Recover func()
	// Version is Bench's version, with the commit it was built from if
	// that's known, for the log, Help ▸ Report a Problem…, the
	// check_for_updates setting, and the version that last had
	// Accessibility permission.
	Version string
	// LogFile, if not "", is where Bench keeps its log, as well as
	// standard error: ~/Library/Logs/Bench/bench.log in the app (see
	// openLogFile).
	LogFile string
	// Lock, if not nil, takes the support folder's lock, which every
	// process that acts on parked.json holds while it does (see cmd/bench),
	// waiting for a watchdog or bench restore that holds it to finish. It
	// returns the function that lets it go. Run calls it once it knows that
	// no other Bench is running, before Recover, and lets the lock go as it
	// quits, once the engine has put every window back and before it lets
	// the watchdog go, which takes the lock next.
	Lock func() (unlock func(), err error)
}

// newEngine makes the engine. Tests replace it.
var newEngine = func(opts workspace.Options) (engine, error) {
	e, err := workspace.New(opts)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// Run builds the engine and the Wails app, and runs the app until Bench
// quits. In the macOS app it doesn't return: the process exits when the app
// quits, once Bench has put every window back. A second Bench just brings
// the first forward and exits.
func Run(opts Options) error {
	if opts.Platform == nil {
		return errors.New("running Bench: there's no platform")
	}
	if opts.Dir == "" {
		return errors.New("running Bench: there's no support folder")
	}
	r := &runner{opts: opts}
	if !nativeShell {
		ignoreServerEnv()
	}
	r.keepLog()
	if os.Getenv(debugEnv) != "" {
		r.debug = slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))
		r.opts.Platform = platform.Logged(r.opts.Platform, r.debug)
	}
	app := application.New(r.appOptions())
	return r.run(app)
}

// debugEnv turns on debug logging: every platform call and event, and the
// engine's decisions, go to the log, and the latest view is written to
// view.json in the support folder (see scripts/macos-demo).
const debugEnv = "BENCH_DEBUG"

// ServerEnv are the environment variables with which Wails' server mode
// would serve somewhere other than Options says: another host (even every
// interface) or another port. The browser preview ignores them, so that it
// serves on localhost, on the port that cmd/bench checks and announces and
// that previewGuard expects.
var ServerEnv = []string{"WAILS_SERVER_HOST", "WAILS_SERVER_PORT"}

// ignoreServerEnv unsets ServerEnv.
func ignoreServerEnv() {
	for _, name := range ServerEnv {
		_ = os.Unsetenv(name)
	}
}

// keepLog sends the log to the log file too, if Options name one, from now
// on: Wails' warnings and errors, Bench's own, the engine's, and, with
// BENCH_DEBUG, its debug messages. If it can't be opened, the log goes to
// standard error alone, as it always does. It starts before
// application.New, so as to keep what Wails says there, but the file is
// only added to until holdLog: a second Bench ends in application.New, once
// it has brought the first forward, and mustn't say that it started, or
// move the file aside from under the first.
func (r *runner) keepLog() {
	if r.opts.LogFile == "" {
		return
	}
	f, err := openLogFile(r.opts.LogFile)
	if err != nil {
		warn(fmt.Errorf("keeping a log: %w", err))
		return
	}
	r.logFile = f
	logOutput = io.MultiWriter(logOutput, f)
}

// holdLog makes the log file this Bench's, once it's the only Bench
// running (see logFile.hold), and starts the launch in it with Bench's
// version.
func (r *runner) holdLog() {
	if r.logFile == nil {
		return
	}
	r.logFile.hold()
	slog.New(slog.NewTextHandler(r.logFile, nil)).Info("Bench started", "version", r.opts.Version)
}

// runner holds what Run builds.
type runner struct {
	opts     Options
	logFile  *logFile     // Nil without one (see keepLog).
	debug    *slog.Logger // Nil unless BENCH_DEBUG is set.
	app      *application.App
	native   *appShell // Nil in the preview.
	emitter  *emitter
	engine   engine
	watchdog io.Closer
	unlock   func() // Lets the support folder's lock go; nil without one.

	// forward brings Bench forward, once the shell exists, for another
	// Bench that starts. Wails can ask for that as soon as the app exists.
	forward atomic.Pointer[func()]

	shutdownOnce sync.Once
}

// appOptions returns the Wails options. In the app, Bench is a single
// instance: a second one brings the first forward and exits, before it
// builds an engine that could move windows.
func (r *runner) appOptions() application.Options {
	opts := application.Options{
		Name:        appName,
		Description: "Gathers any set of apps into one window with tabs.",
		Assets: application.AssetOptions{
			Handler:        iconHandler(r.opts.Platform, application.AssetFileServerFS(frontend.FS)),
			DisableLogging: true,
		},
		Logger:     newLogger(logOutput),
		OnShutdown: r.shutdownOnQuit,
	}
	if nativeShell {
		opts.SingleInstance = &application.SingleInstanceOptions{
			UniqueID:               UniqueID,
			OnSecondInstanceLaunch: r.secondInstance,
		}
		opts.ShouldQuit = r.shouldQuit
		// Wails would quit on SIGINT and SIGTERM as the Quit menu item
		// does, which asks about terminals that are running something;
		// quitOnSignals quits without asking instead.
		opts.DisableDefaultSignalHandler = true
	} else {
		port := r.opts.Port
		if port == 0 {
			port = previewPort
		}
		opts.Server = application.ServerOptions{Host: "localhost", Port: port}
		opts.Assets.Middleware = previewGuard(port)
	}
	return opts
}

// shouldQuit is Wails' ShouldQuit, which the shell answers
// (appShell.shouldQuit). Wails asks it only once the app runs, by when the
// shell exists.
func (r *runner) shouldQuit() bool {
	if r.native == nil {
		return true
	}
	return r.native.shouldQuit()
}

// secondInstance is Wails' OnSecondInstanceLaunch, which brings Bench
// forward. Wails calls it from the one goroutine that reads a one-slot
// channel, which the main thread fills each time another Bench starts. So
// it must return without waiting for the main thread, as bringing Bench
// forward does: otherwise, once two more Benches start meanwhile, the main
// thread would block on the channel, and nothing could ever run on it again.
func (r *runner) secondInstance(application.SecondInstanceData) {
	if forward := r.forward.Load(); forward != nil {
		go (*forward)()
	}
}

// run builds the engine and the shell around it, and runs the app. Only
// the one Bench that runs gets here: Wails ends a second Bench in
// application.New.
func (r *runner) run(app *application.App) error {
	r.holdLog()
	r.app = app
	preview := &previewShell{}
	var sh shell = preview
	if nativeShell {
		r.native = newAppShell(app, r.opts.SystemDark)
		sh = r.native
		forward := r.native.bringForward
		r.forward.Store(&forward)
	}
	r.emitter = newEmitter(app.Event, sh)
	if r.debug != nil {
		r.emitter.viewFile = filepath.Join(r.opts.Dir, "view.json")
	}
	// The engine's warnings, and the notices, always go to the log: its
	// decisions too with BENCH_DEBUG.
	log := r.debug
	if log == nil {
		log = slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}
	r.emitter.log = log

	if r.opts.Lock != nil {
		unlock, err := r.opts.Lock()
		if err != nil {
			return fmt.Errorf("starting Bench: %w", err)
		}
		r.unlock = unlock
	}
	if r.opts.Recover != nil {
		r.opts.Recover()
	}
	e, err := newEngine(workspace.Options{
		Platform:      r.opts.Platform,
		NewTerms:      r.opts.NewTerms,
		Emitter:       r.emitter,
		Dir:           r.opts.Dir,
		StartVSCode:   r.opts.StartVSCode,
		LoginItem:     loginItem(app),
		Demo:          r.opts.Demo,
		Log:           log,
		OpenLink:      linkOpener(app),
		Version:       r.opts.Version,
		LatestRelease: latestRelease(r.opts.Version),
		ChooseFolder: folderChooser(app, func() *application.WebviewWindow {
			if r.native == nil {
				return nil
			}
			return r.native.main
		}),
	})
	if err != nil {
		r.letLockGo()
		return fmt.Errorf("starting Bench: %w", err)
	}
	r.engine = e
	// The global shortcuts. The preview's run nothing (previewRegistrar).
	var reg registrar = previewRegistrar{}
	if nativeShell {
		reg = app.GlobalShortcut
	}
	keys := &shortcutSet{reg: reg, e: e, system: r.opts.Platform.SystemKeys}
	r.emitter.keys = keys
	preview.registerShortcuts = keys.register
	app.RegisterService(application.NewService(newService(e)))

	if r.opts.Watchdog {
		if r.watchdog, err = StartWatchdog(r.opts.Dir); err != nil {
			warn(err)
		}
	}
	if r.native != nil {
		r.setUpNative(keys)
	}

	e.Start()
	if r.native == nil {
		// The app registers them once it has started (setUpNative).
		keys.register()
	}
	quit := app.Quit // It waits for the main thread itself.
	if r.native != nil {
		quit = r.native.quit
	}
	stopSignals := quitOnSignals(quit, quitSignals()...)
	err = app.Run()
	stopSignals()
	r.shutdown()
	if err != nil {
		return fmt.Errorf("running Bench: %w", err)
	}
	return nil
}

// setUpNative opens the main window and sets up the app menu, the menu bar
// item, and global shortcuts (keys).
func (r *runner) setUpNative(keys *shortcutSet) {
	n := r.native
	n.switchTab = r.engine.SwitchTab
	n.toast = r.emitter.Toast
	n.openPalette = func() { r.emitter.Command(paletteCommand) }
	n.openSettings = func() { r.emitter.Command(settingsCommand) }
	n.terminalClosed = r.engine.TerminalWindowClosed
	n.askToCloseTerminal = r.engine.AskToCloseTerminal
	n.tagPopOut = r.opts.TagPopOut
	n.applyAppearance = r.opts.Appearance
	n.askToQuit = r.engine.AskToQuit
	n.askToQuitNow = r.engine.AskToQuitNow
	n.loggingOut = r.opts.Platform.LoggingOut
	n.registerShortcuts = keys.register

	n.openMain(r.engine.WindowStart(), nativeWindow(r.opts.Attach), nativeWindow(r.opts.Fill))

	n.menu = newAppMenu(menuActions{
		acknowledgements: n.openAcknowledgements,
		settings:         n.openSettings,
		command:          r.emitter.Command,
		act: func(f func(e engine) error) {
			if err := f(r.engine); err != nil {
				r.emitter.Toast(err.Error())
			}
		},
		fillScreen:    n.fillScreen,
		openLink:      r.openLink,
		reportProblem: r.reportProblem,
	})
	r.app.Menu.Set(n.menu.root)

	// Wails calls application event listeners on their own goroutines, so
	// these can wait for the main thread.
	r.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		n.menu.start()
		n.startTray()
		keys.register()
	})
	r.app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		n.reopen()
	})
}

// latestRelease returns what asks GitHub for Bench's latest release, for
// the check_for_updates setting, saying which version asks: nil in the
// preview, which looks for none. The engine gives each look a deadline.
func latestRelease(version string) func(ctx context.Context) (update.Release, error) {
	if !nativeShell {
		return nil
	}
	return func(ctx context.Context) (update.Release, error) {
		return update.Latest(ctx, http.DefaultClient, update.LatestAPI, version)
	}
}

// nativeWindow returns a function that gives f a window's NSWindow, if it
// has one, or nil if f is nil.
func nativeWindow(f func(nsWindow unsafe.Pointer)) func(*application.WebviewWindow) {
	if f == nil {
		return nil
	}
	return func(w *application.WebviewWindow) {
		if ns := w.NativeWindow(); ns != nil {
			f(ns)
		}
	}
}

// quitSignals are the signals that quit Bench without asking about its
// terminals: SIGHUP, which comes as the terminal Bench runs in closes, and,
// in the macOS app, SIGINT and SIGTERM, which Wails would otherwise handle
// by quitting as the Quit menu item does, asking first. In the preview,
// Wails handles SIGINT and SIGTERM itself, and never asks.
func quitSignals() []os.Signal {
	if nativeShell {
		return []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM}
	}
	return []os.Signal{syscall.SIGHUP}
}

// quitOnSignals calls quit, which quits the app cleanly, putting every
// window back, when the first of signals comes; without it, SIGHUP would
// end Bench at once. Later signals are dropped, so that a second one
// doesn't end Bench before it has put every window back. A signal that
// Bench started with ignored, as nohup starts it ignoring SIGHUP, stays
// ignored, so that Bench keeps running when its terminal closes: listening
// for it would stop it being ignored. It returns a function that stops
// listening.
func quitOnSignals(quit func(), signals ...os.Signal) (stop func()) {
	var listen []os.Signal
	for _, sig := range signals {
		if !signal.Ignored(sig) {
			listen = append(listen, sig)
		}
	}
	if len(listen) == 0 {
		return func() {}
	}
	// Go drops a signal that would block the channel, so once the first
	// has been taken, every later one fills the channel or is dropped.
	c := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(c, listen...)
	go func() {
		select {
		case <-c:
			quit()
		case <-done:
		}
	}()
	return func() {
		signal.Stop(c)
		close(done)
	}
}

// shutdownOnQuit is Wails' shutdown hook. On macOS it runs on the main
// thread, which mustn't wait for anything that needs the main thread, so it
// puts windows back on another goroutine and waits for that, but only up to
// shutdownWait.
func (r *runner) shutdownOnQuit() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.shutdown()
	}()
	select {
	case <-done:
	case <-time.After(shutdownWait):
		warn(errors.New("quitting before every window was put back; the watchdog will finish"))
	}
}

// shutdown stops events reaching the frontend, closes the engine (which puts
// every parked window back and empties parked.json), then lets the support
// folder's lock go and the watchdog after it, and closes the platform. It
// runs once; later calls wait for the first.
func (r *runner) shutdown() {
	r.shutdownOnce.Do(func() {
		if r.native != nil {
			r.native.quitting.Store(true)
		}
		if r.emitter != nil {
			r.emitter.close()
		}
		if r.engine != nil {
			if err := r.engine.Close(); err != nil {
				warn(fmt.Errorf("quitting: %w", err))
			}
		}
		// The watchdog takes the lock before it looks at parked.json, so
		// holding it while closing the watchdog would only keep it waiting.
		r.letLockGo()
		if r.watchdog != nil {
			if err := r.watchdog.Close(); err != nil {
				warn(fmt.Errorf("quitting: letting the watchdog go: %w", err))
			}
		}
		if c, ok := r.opts.Platform.(interface{ Close() }); ok {
			c.Close()
		}
	})
}

// letLockGo lets the support folder's lock go, if Run took it.
func (r *runner) letLockGo() {
	if r.unlock != nil {
		r.unlock()
		r.unlock = nil
	}
}

// loginItem returns Wails' Autostart manager as Open at login, or nil where
// it isn't supported (the preview).
func loginItem(app *application.App) workspace.LoginItem {
	if !nativeShell {
		return nil
	}
	if _, err := app.Autostart.IsEnabled(); errors.Is(err, application.ErrAutostartNotSupported) {
		return nil
	}
	return app.Autostart
}

// warn reports a problem that doesn't stop Bench, in the log.
func warn(err error) {
	slog.New(slog.NewTextHandler(logOutput, nil)).Warn(err.Error())
}
