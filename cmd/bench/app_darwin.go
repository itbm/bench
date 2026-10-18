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

func appFlags(*flag.FlagSet) func(*cli, []string) error {
	return func(c *cli, args []string) error {
		if len(args) > 0 {
			return usageErrorf("unexpected argument %q; see bench help for the commands", args[0])
		}
		return runApp()
	}
}

// runApp runs the macOS app. It doesn't return once the app is running: the
// process exits when Bench quits.
func runApp() error {
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
		NewTerms:    func(cb term.Callbacks) term.Host { return term.NewManager(cb) },
		StartVSCode: startVSCode,
		Dir:         dir,
		Watchdog:    true,
	})
}

// startVSCode starts `code serve-web` for VS Code for the Web items.
func startVSCode(ctx context.Context) (workspace.VSCodeServer, error) {
	s, err := vscode.Start(ctx, vscode.Options{})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// arrange is `bench arrange <tab>`: it arranges a tab with a headless
// engine, then waits for Control-C and puts everything back.
func arrange(c *cli, tab, flagValue string) (err error) {
	setup, err := prepareArrange(c, tab, flagValue)
	if err != nil {
		return err
	}
	if setup.Temporary {
		defer func() { err = errors.Join(err, os.RemoveAll(setup.Dir)) }()
	}
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
		defer wd.Close()
	}

	e, err := workspace.New(workspace.Options{
		Platform: p,
		Emitter:  consoleEmitter{w: c.stderr},
		Dir:      setup.Dir,
		Headless: true,
	})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(c.stdout, "Arranging %s…\n", tab)
	arrangeErr := e.ArrangeOnce(ctx, tab)
	if arrangeErr == nil {
		fmt.Fprintf(c.stdout, "Arranged %s. Press Control-C to put everything back.\n", tab)
		<-ctx.Done()
	}
	stop()
	closeErr := e.Close()
	if closeErr == nil {
		fmt.Fprintln(c.stdout, "Put everything back.")
	}
	if errors.Is(arrangeErr, context.Canceled) {
		arrangeErr = nil
	}
	return errors.Join(arrangeErr, closeErr)
}

// restore is `bench restore`.
func restore(c *cli, dir string) error {
	f, err := restoreParked(dir, putBack)
	if err != nil {
		return err
	}
	if f.Empty() {
		fmt.Fprintln(c.stdout, "There's nothing to put back.")
		return nil
	}
	fmt.Fprintf(c.stdout, "Put back %s.\n", parkedSummary(f))
	return nil
}

// watchdog is `bench watchdog`. Bench handles Control-C and a closed
// terminal itself, so the watchdog ignores them and waits for its pipe.
func watchdog(c *cli, dir string) error {
	signal.Ignore(os.Interrupt, syscall.SIGHUP)
	return waitAndRestore(os.Stdin, dir, putBack)
}

// putBack restores the windows in a parked-windows file.
func putBack(path string) error {
	p, err := mac.New()
	if err != nil {
		return err
	}
	defer p.Close()
	if !p.Trusted(false) {
		return errNotTrusted
	}
	return windows.RestoreFromFile(p, path)
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
func (consoleEmitter) CurrentTab(string, []string)         {}
