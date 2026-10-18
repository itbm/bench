//go:build darwin || server

// Package ui is Bench's app shell, built on Wails v3: the main window, the
// bound Service the frontend calls, the events Go sends it, global
// shortcuts, the app menu, the menu bar item, and Open at login.
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
	"sync"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/itbm/bench/frontend"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/workspace"
)

// uniqueID identifies Bench to Wails' single-instance lock.
const uniqueID = "com.github.itbm.bench"

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
	app := application.New(r.appOptions())
	return r.run(app)
}

// runner holds what Run builds.
type runner struct {
	opts     Options
	app      *application.App
	native   *appShell // Nil in the preview.
	emitter  *emitter
	engine   engine
	watchdog io.Closer

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
			Handler:        application.AssetFileServerFS(frontend.FS),
			DisableLogging: true,
		},
		Logger:     newLogger(logOutput),
		OnShutdown: r.shutdownOnQuit,
		Server:     application.ServerOptions{Port: r.opts.Port},
	}
	if nativeShell {
		opts.SingleInstance = &application.SingleInstanceOptions{
			UniqueID: uniqueID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if r.native != nil {
					r.native.bringForward()
				}
			},
		}
	}
	return opts
}

// run builds the engine and the shell around it, and runs the app.
func (r *runner) run(app *application.App) error {
	r.app = app
	var sh shell = previewShell{}
	if nativeShell {
		r.native = newAppShell(app)
		sh = r.native
	}
	r.emitter = newEmitter(app.Event, sh)

	e, err := newEngine(workspace.Options{
		Platform:    r.opts.Platform,
		NewTerms:    r.opts.NewTerms,
		Emitter:     r.emitter,
		Dir:         r.opts.Dir,
		StartVSCode: r.opts.StartVSCode,
		LoginItem:   loginItem(app),
		Demo:        r.opts.Demo,
	})
	if err != nil {
		return fmt.Errorf("starting Bench: %w", err)
	}
	r.engine = e
	app.RegisterService(application.NewService(newService(e)))

	if r.opts.Watchdog {
		if r.watchdog, err = StartWatchdog(r.opts.Dir); err != nil {
			warn(err)
		}
	}
	if r.native != nil {
		r.setUpNative()
	}

	e.Start()
	err = app.Run()
	r.shutdown()
	if err != nil {
		return fmt.Errorf("running Bench: %w", err)
	}
	return nil
}

// setUpNative opens the main window and sets up the app menu, the menu bar
// item, and global shortcuts.
func (r *runner) setUpNative() {
	n := r.native
	n.switchTab = r.engine.SwitchTab
	n.toast = r.emitter.Toast

	var attach func(*application.WebviewWindow)
	if r.opts.Attach != nil {
		attach = func(w *application.WebviewWindow) {
			if ns := w.NativeWindow(); ns != nil {
				r.opts.Attach(ns)
			}
		}
	}
	n.openMain(attach)

	r.app.Menu.Set(appMenu(menuActions{
		settings:   func() { r.emitter.Command(settingsCommand) },
		fillScreen: n.fillScreen,
		rearrange: func() {
			if err := r.engine.Rearrange(); err != nil {
				r.emitter.Toast(err.Error())
			}
		},
	}))

	// Wails calls application event listeners on their own goroutines, so
	// these can wait for the main thread.
	r.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		n.startTray()
		registerShortcuts(r.app.GlobalShortcut, r.engine)
	})
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
// every parked window back and empties parked.json), then lets the watchdog
// go and closes the platform. It runs once; later calls wait for the first.
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

// warn reports a problem that doesn't stop Bench, on standard error.
func warn(err error) {
	fmt.Fprintf(logOutput, "bench: %v\n", err)
}
