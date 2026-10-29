//go:build darwin && cgo && !server

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/mac"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/ui"
	"github.com/itbm/bench/internal/vscode"
	"github.com/itbm/bench/internal/windows"
	"github.com/itbm/bench/internal/workspace"
)

const appAbout = "Runs Bench. If Bench is already running, brings it forward instead."

// errNotTrusted explains how to give Bench Accessibility permission.
var errNotTrusted = errors.New("Bench needs Accessibility permission to move other apps' windows: " +
	"allow it in System Settings ▸ Privacy & Security ▸ Accessibility, then try again")

// errBenchRunning is bench arrange's error while the app, or another bench
// arrange, is running: either holds the app's single-instance lock.
var errBenchRunning = errors.New("Bench, or another bench arrange, is running, and it arranges the same windows: " +
	"quit it, then try again")

// init keeps main on the main thread, as AppKit needs, and as bench
// arrange's run loop does (mac.RunMainLoop). Wails does this too.
func init() { runtime.LockOSThread() }

func appFlags(*flag.FlagSet) func(*cli, []string) error {
	return func(c *cli, args []string) error {
		if len(args) > 0 {
			return usageErrorf("unexpected argument %q; see bench help for the commands", args[0])
		}
		return runApp(c)
	}
}

// runApp runs the macOS app. It doesn't return once the app is running: the
// process exits when Bench quits.
func runApp(c *cli) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	p, err := mac.New()
	if err != nil {
		return err
	}
	defer p.Close()
	return ui.Run(ui.Options{
		Platform:    p,
		Attach:      p.AttachWindow,
		Fill:        p.FillWindow,
		NewTerms:    func(cb term.Callbacks) term.Host { return term.NewManager(cb) },
		StartVSCode: vscodeStarter(dir),
		Dir:         dir,
		Watchdog:    true,
		Lock:        func() (func(), error) { return lockSupportDir(c.stderr, dir) },
		// If the watchdog didn't stop the server a Bench that crashed left
		// running, stop it now.
		Recover: func() {
			if err := stopVSCode(dir); err != nil {
				fmt.Fprintf(c.stderr, "bench: %v\n", err)
			}
		},
	})
}

// vscodeStarter returns the function that starts `code serve-web` for VS
// Code for the Web items, recording it in the support folder dir, so that it
// can be stopped if Bench crashes, and keeping its token there.
func vscodeStarter(dir string) func(ctx context.Context) (workspace.VSCodeServer, error) {
	return func(ctx context.Context) (workspace.VSCodeServer, error) {
		s, err := vscode.Start(ctx, vscodeOptions(dir))
		if err != nil {
			return nil, err
		}
		return s, nil
	}
}

// arrange is `bench arrange <tab>`: it arranges a tab with a headless
// engine, then waits for Control-C and puts everything back. It refuses
// while the app is running, and holds the app's single-instance lock so
// that the app can't start until it has finished: they'd fight over the
// same windows, and arranging from the app's support folder would put back
// the windows that the app has parked. It also holds the support folder's
// lock, as the app does.
func arrange(c *cli, tab, flagValue string) (err error) {
	setup, err := prepareArrange(c, tab, flagValue)
	if err != nil {
		return err
	}
	unlockInstance, err := lockInstance(mac.TemporaryDirectory() + ui.UniqueID + ".lock")
	if errors.Is(err, errLocked) {
		err = errBenchRunning
	}
	if err != nil {
		if setup.Temporary {
			err = errors.Join(err, os.RemoveAll(setup.Dir))
		}
		return err
	}
	defer unlockInstance()

	// The deferred calls below run in reverse order: the support folder's
	// lock goes once the engine has put everything back, then the
	// watchdog, which takes the lock next, has its moment to finish, and
	// only then can a temporary folder go (finishArrange). Control-C and
	// the other quit signals, once caught below, stay caught until all
	// that has happened.
	stopSignals := func() {}
	defer func() { stopSignals() }()
	watchdogDone := true
	defer func() { err = errors.Join(err, finishArrange(c.stderr, setup, watchdogDone)) }()
	p, err := mac.New()
	if err != nil {
		return err
	}
	defer p.Close()
	if !p.Trusted(true) {
		return errNotTrusted
	}
	// If this process crashes, the watchdog puts the windows back.
	if wd, err := ui.StartWatchdog(setup.Dir); err != nil {
		fmt.Fprintf(c.stderr, "bench arrange: %v\n", err)
	} else {
		defer func() {
			if err := wd.Close(); err != nil {
				watchdogDone = false
				fmt.Fprintf(c.stderr, "bench arrange: letting the watchdog go: %v\n", err)
			}
		}()
	}
	unlockDir, err := lockSupportDir(c.stderr, setup.Dir)
	if err != nil {
		return err
	}
	defer unlockDir()

	e, err := workspace.New(workspace.Options{
		Platform: p,
		Emitter:  consoleEmitter{w: c.stderr},
		Dir:      setup.Dir,
		Headless: true,
	})
	if err != nil {
		return err
	}
	// The signals stay caught until arrange returns, so that neither a
	// second Control-C nor the terminal closing ends this process while it
	// puts everything back and cleans up.
	ctx, stop := signal.NotifyContext(context.Background(), quitSignals()...)
	stopSignals = stop

	// macOS updates its list of running apps, and reports their windows,
	// only while the main thread runs its run loop. Without it, the apps
	// that arranging opens would never be seen to start. So this goroutine,
	// on the main thread, runs the loop while another arranges.
	var arrangeErr, closeErr error
	whileLooping(mac.RunMainLoop, func() {
		fmt.Fprintf(c.stdout, "Arranging %s…\n", tab)
		arrangeErr = e.ArrangeOnce(ctx, tab)
		if arrangeErr == nil {
			fmt.Fprintf(c.stdout, "Arranged %s. Press Control-C to put everything back.\n", tab)
			<-ctx.Done()
		}
		closeErr = e.Close()
	})
	if closeErr == nil {
		fmt.Fprintln(c.stdout, "Put everything back.")
	}
	if errors.Is(arrangeErr, context.Canceled) {
		arrangeErr = nil
	}
	return errors.Join(arrangeErr, closeErr)
}

// restore is `bench restore`. It refuses while Bench or bench arrange runs
// from the same support folder (restoreNow).
func restore(c *cli, dir string) error {
	done, err := restoreNow(dir, putBack)
	if err != nil {
		return err
	}
	summary := restoredSummary(done)
	if summary == "" {
		summary = "There's nothing to put back."
	}
	fmt.Fprintln(c.stdout, summary)
	return nil
}

// watchdog is `bench watchdog`. Bench handles Control-C and a closed
// terminal itself, and starts the watchdog in a process group of its own,
// out of the terminal's reach. In case it's run some other way, the
// watchdog ignores them too, and waits for its pipe.
func watchdog(c *cli, dir string) error {
	signal.Ignore(os.Interrupt, syscall.SIGHUP)
	return waitAndRestore(os.Stdin, dir, putBack, func() {})
}

// putBack restores the windows in a parked-windows file.
func putBack(path string) (windows.Restored, error) {
	p, err := mac.New()
	if err != nil {
		return windows.Restored{}, err
	}
	defer p.Close()
	if !p.Trusted(false) {
		return windows.Restored{}, errNotTrusted
	}
	return windows.RestoreFromFileReport(p, path)
}

// consoleEmitter is the headless engine's Emitter: it prints toasts and
// drops everything else, as there's no frontend.
type consoleEmitter struct{ w io.Writer }

func (consoleEmitter) State(workspace.View)                {}
func (e consoleEmitter) Toast(message string)              { fmt.Fprintln(e.w, message) }
func (consoleEmitter) Command(workspace.Command)           {}
func (consoleEmitter) TermData(workspace.TermData)         {}
func (consoleEmitter) PopOutTerminal(string, string) error { return nil }
func (consoleEmitter) CloseTerminalWindow(string)          {}
func (consoleEmitter) MenuBar(workspace.MenuBar)           {}
func (consoleEmitter) BringForward()                       {}
func (consoleEmitter) ShortcutsChanged()                   {}
func (consoleEmitter) FillScreen()                         {}
