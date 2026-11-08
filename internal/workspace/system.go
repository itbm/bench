package workspace

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
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

// termOptions returns how to start a terminal's session. A plain shell
// starts in the folder it was last in, if it has moved, or in its own
// folder if that one has gone. A terminal with Run types it into the shell,
// and presses Return if it's to run now: for its first session after the
// add dialog or a launcher added it, and after Bench opens again if it
// starts again. Like a Command, it starts in its own folder, which it was
// written for.
func termOptions(it *Item) term.Options {
	opts := term.Options{Dir: it.Folder, Command: it.Command}
	if plainShell(it) && it.LastFolder != "" {
		opts.Dir, opts.Fallback = it.LastFolder, it.Folder
	}
	if it.Run != "" && it.Command == "" {
		opts.Input = it.Run
		if it.runNow || it.StartAgain {
			opts.Input += "\r"
		}
	}
	return opts
}

// plainShell reports whether a terminal runs just a shell: it has neither a
// Command nor a Run, which are written for their folder.
func plainShell(it *Item) bool {
	return it.Kind == KindBuiltin && it.Command == "" && it.Run == ""
}

// hereOf returns the folder a terminal's shell is in, as far as Bench
// knows: the last it reported, or else the one it started in.
func hereOf(it *Item) string {
	if it.LastFolder != "" {
		return it.LastFolder
	}
	return it.Folder
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

// RequestAccessibility asks macOS for Accessibility permission, and while
// Bench doesn't have it, opens System Settings at Accessibility's list too:
// macOS asks only once, and after an update the user must remove Bench
// from the list there and turn it on again (see permissions.go). If Bench
// has it now, it starts arranging.
func (e *Engine) RequestAccessibility() error {
	e.lock()
	defer e.unlock()
	switch {
	case e.p.Trusted(true):
		if !e.trusted {
			e.becameTrustedLocked()
		}
	case !e.opts.Demo && !e.opts.Headless:
		e.openSettingsLocked(platform.SettingsAccessibility)
	}
	e.touch()
	return nil
}

// RequestScreenRecording asks macOS for Screen Recording permission, for
// live previews, and while Bench doesn't have it, opens System Settings at
// Screen Recording's list too (askScreenRecordingLocked).
func (e *Engine) RequestScreenRecording() error {
	e.lock()
	defer e.unlock()
	e.askScreenRecordingLocked()
	e.touch()
	return nil
}

// SetOpenAtLogin turns Open at login on or off. If the user turned Bench
// off in System Settings ▸ General ▸ Login Items, turning it on leaves it
// off, as macOS needs them to allow it there: Bench opens Login Items, and
// says so.
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
	// macOS may refuse the login item, or take it without opening Bench at
	// login, until the user allows it.
	approval := on && checkErr == nil && !enabled && e.p.LoginItemNeedsApproval()
	e.lock()
	defer e.unlock()
	if checkErr == nil {
		e.openAtLogin = enabled
	} else if err == nil {
		e.openAtLogin = on
	}
	e.loginApproval = approval
	e.touch()
	if approval {
		e.openSettingsLocked(platform.SettingsLoginItems)
		return errors.New(loginApprovalText)
	}
	if err != nil {
		return fmt.Errorf("Bench couldn't change Open at login: %v.", err)
	}
	return nil
}

// DismissFirstRun closes the welcome guide for good.
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
// laptop. It needs the preview's platform (platform.NewDemo). The simulated
// screen changes at once, with nothing for macOS to settle, so Auto
// chooses the size class for it straight away, afresh. Choosing a screen
// is choosing the display for Bench's window (decision 70).
func (e *Engine) SetDemoScreen(screen string, fitWidth float64) error {
	ds, ok := e.p.(demoScreens)
	if !ok || !e.opts.Demo {
		return errNoDemo
	}
	e.lock()
	defer e.unlock()
	ds.SetDemoScreen(screen, fitWidth)
	e.screen = e.p.ScreenOfWindow()
	e.autoClass = ""
	e.stopReturnLocked()
	e.placeOnShow = nil
	if s := e.screen; s.ID != "" {
		if !sameDisplay(e.preferred, s) {
			e.forgetWentBackLocked()
		}
		e.setPreferredLocked(&config.ScreenState{ID: s.ID, Name: s.Name, Frame: s.Frame})
	}
	e.arrangeLocked(arrangeOpts{})
	e.touch()
	return nil
}

// SetDemoMonitor simulates unplugging the browser preview's monitor, with
// plugged false, or plugging it in again, as on a laptop with a monitor
// (platform.Fake.SetDemoMonitor). It needs the preview's platform. As on a
// Mac, Bench's window goes to the laptop's screen when the monitor goes,
// and once the monitor is back and the screens have settled, Bench goes
// back to it, if it's the display chosen for the window, with a toast that
// says so, whose Undo moves it back (decision 70). Unplugging needs the
// monitor, as the demo bar's Monitor gives it, and plugging in needs it
// unplugged.
func (e *Engine) SetDemoMonitor(plugged bool) error {
	dd, ok := e.p.(platform.DemoDisplays)
	if !ok || !e.opts.Demo {
		return errNoDemo
	}
	e.lock()
	defer e.unlock()
	if !dd.SetDemoMonitor(plugged) {
		if plugged {
			return errDemoPlugged
		}
		return errNoDemoMonitor
	}
	e.touch()
	return nil
}

// demoSystemKeys is what the browser preview's platform adds, for
// simulating the keys that macOS keeps for itself.
type demoSystemKeys interface {
	SetSystemKeys(k platform.SystemKeys)
}

// SetDemoVoiceOver simulates VoiceOver being turned on or off, in the
// browser preview, whose platform has it off: the terminals are then read
// out, and the shortcuts with ⌃⌥ clash, as on a Mac with VoiceOver on. It
// needs the preview's platform (platform.NewDemo).
func (e *Engine) SetDemoVoiceOver(on bool) error {
	dk, ok := e.p.(demoSystemKeys)
	if !ok || !e.opts.Demo {
		return errNoDemoVoiceOver
	}
	e.lock()
	defer e.unlock()
	k := e.p.SystemKeys()
	k.VoiceOver = on
	dk.SetSystemKeys(k)
	e.onSystemKeysChanged()
	return nil
}

// demoWindows is what the browser preview's platform adds, for simulating
// what the user does to other apps' windows.
type demoWindows interface {
	MinimiseWindow(id platform.WindowID) error
}

// DemoMinimise minimises the window that the focused pane shows, as the
// user would with Command-M, in the browser preview, whose windows have no
// controls of their own. The platform reports it, as macOS would
// (platform.WindowMinimised). It needs the preview's platform
// (platform.NewDemo).
func (e *Engine) DemoMinimise() error {
	dw, ok := e.p.(demoWindows)
	if !ok || !e.opts.Demo {
		return errNoDemoWindows
	}
	e.lock()
	defer e.unlock()
	g, ok := e.displayLocked().focusedGeom()
	if !ok {
		return errNoMinimise
	}
	it := e.model.Items[g.node.Active]
	if !dockable(it) {
		return errNoMinimise
	}
	return dw.MinimiseWindow(it.WindowID)
}
