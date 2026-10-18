package workspace

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/itbm/bench/internal/term"
)

// This file has the bound methods for the frontend's start, terminals,
// permissions, and the shell.

// Ready returns the current view. The frontend calls it on load. It lowers
// Bench's window, in case a previous page load left it raised.
func (e *Engine) Ready() (View, error) {
	e.lock()
	defer e.unlock()
	e.setOverlayLocked(false)
	return e.buildViewLocked(), nil
}

// termOptions returns how to start a terminal's session.
func termOptions(it *Item) term.Options {
	return term.Options{Dir: it.Folder, Command: it.Command}
}

// termError turns a terminal host's error into a sentence for the user.
func termError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, term.ErrNoSession):
		return errNoTerminal
	case errors.Is(err, term.ErrEnded):
		return errTermEnded
	case errors.Is(err, term.ErrBusy):
		return errors.New("That terminal isn't reading what's typed.")
	}
	return errors.New("That terminal size isn't valid.")
}

// TermInput sends keystrokes to a terminal.
func (e *Engine) TermInput(itemID, data string) error {
	return termError(e.terms.Write(itemID, []byte(data)))
}

// TermResize sets a terminal's size, in character cells.
func (e *Engine) TermResize(itemID string, cols, rows int) error {
	return termError(e.terms.Resize(itemID, cols, rows))
}

// TermSnapshot returns a terminal's recent output, base64-encoded, for a new
// terminal view to replay.
func (e *Engine) TermSnapshot(itemID string) (string, error) {
	e.lock()
	it := e.model.Items[itemID]
	e.unlock()
	if it == nil || it.Kind != KindBuiltin {
		return "", errNoTerminal
	}
	return base64.StdEncoding.EncodeToString(e.terms.Snapshot(itemID)), nil
}

// RequestAccessibility asks macOS for Accessibility permission. If Bench
// has it now, it starts arranging.
func (e *Engine) RequestAccessibility() error {
	e.lock()
	defer e.unlock()
	if e.p.Trusted(true) && !e.trusted {
		e.becameTrustedLocked()
	}
	e.touch()
	return nil
}

// RequestScreenRecording asks macOS for Screen Recording permission, for
// live previews.
func (e *Engine) RequestScreenRecording() error {
	e.lock()
	defer e.unlock()
	e.screenRec = e.p.ScreenRecordingAllowed(true)
	e.touch()
	return nil
}

// SetOpenAtLogin turns Open at login on or off.
func (e *Engine) SetOpenAtLogin(on bool) error {
	li := e.opts.LoginItem
	if li == nil {
		return errNoLoginItem
	}
	var err error
	if on {
		err = li.Enable()
	} else {
		err = li.Disable()
	}
	enabled, checkErr := li.IsEnabled()
	e.lock()
	defer e.unlock()
	if checkErr == nil {
		e.openAtLogin = enabled
	} else if err == nil {
		e.openAtLogin = on
	}
	e.touch()
	if err != nil {
		return fmt.Errorf("Bench couldn't change Open at login: %v.", err)
	}
	return nil
}

// DismissFirstRun closes the permissions guide for good.
func (e *Engine) DismissFirstRun() error {
	err := e.markFirstRunDone()
	e.lock()
	defer e.unlock()
	e.firstRun = false
	e.touch()
	return err
}

// demoScreens is what the browser preview's platform adds, for simulating
// screens.
type demoScreens interface {
	SetDemoScreen(name string, fitWidth float64)
}

// SetDemoScreen simulates a screen in the browser preview: fit, monitor, or
// laptop. It needs the preview's platform (platform.NewDemo).
func (e *Engine) SetDemoScreen(screen string, fitWidth float64) error {
	ds, ok := e.p.(demoScreens)
	if !ok || !e.opts.Demo {
		return errNoDemo
	}
	e.lock()
	defer e.unlock()
	ds.SetDemoScreen(screen, fitWidth)
	e.screen = e.p.ScreenOfWindow()
	e.arrangeLocked(arrangeOpts{})
	e.touch()
	return nil
}
