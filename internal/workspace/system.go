package workspace

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/term"
)

// This file has the bound methods for the frontend's start, terminals,
// permissions, dialogs, and the shell.

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

// TermInput sends keystrokes to a terminal. Only when its session has ended,
// or the host no longer has it, does it take the engine's lock, so that
// typing doesn't wait for an arrange: Return in a terminal whose command has
// finished runs it again.
func (e *Engine) TermInput(itemID, data string) error {
	err := e.terms.Write(itemID, []byte(data))
	if errors.Is(err, term.ErrEnded) || errors.Is(err, term.ErrNoSession) {
		e.lock()
		rerun := e.rerunLocked(itemID, data)
		e.unlock()
		if rerun {
			return nil
		}
	}
	return termError(err)
}

// TermResize sets a terminal's size, in character cells.
func (e *Engine) TermResize(itemID string, cols, rows int) error {
	return termError(e.terms.Resize(itemID, cols, rows))
}

// TermSnapshot returns a terminal's recent output, for a new terminal view
// to replay, and where it ends.
func (e *Engine) TermSnapshot(itemID string) (TermSnapshot, error) {
	e.lock()
	it := e.model.Items[itemID]
	e.unlock()
	if it == nil || it.Kind != KindBuiltin {
		return TermSnapshot{}, errNoTerminal
	}
	snap := e.terms.Snapshot(itemID)
	return TermSnapshot{Data: base64.StdEncoding.EncodeToString(snap.Data), Offset: snap.Offset}, nil
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

// dialogKinds are the dialogs whose size Bench remembers, as the frontend
// names them: the jump palette, the add dialog, Tab properties, Settings,
// and the welcome guide.
var dialogKinds = []string{"palette", "add", "editor", "settings", "guide"}

// maxDialogSide is the widest or tallest, in points, that a dialog's
// remembered size can be: wider than any screen.
const maxDialogSide = 20000

// SetDialogSize remembers the size that the user gave a dialog by resizing
// it, by kind (dialogKinds), for it to open at that size next time, after a
// restart too: state.json saves it. The frontend keeps the dialog inside
// the window, so a size can be larger than the window it opens in.
func (e *Engine) SetDialogSize(kind string, w, h float64) error {
	if !slices.Contains(dialogKinds, kind) {
		return errNoDialog
	}
	if !(w > 0 && h > 0 && w <= maxDialogSide && h <= maxDialogSide) {
		return errDialogSize
	}
	size := config.DialogSize{W: math.Round(w), H: math.Round(h)}
	e.lock()
	defer e.unlock()
	if e.dialogs[kind] == size {
		return nil
	}
	if e.dialogs == nil {
		e.dialogs = map[string]config.DialogSize{}
	}
	e.dialogs[kind] = size
	e.touch()
	e.scheduleLazySaveLocked()
	return nil
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
