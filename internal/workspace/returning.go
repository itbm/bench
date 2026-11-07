package workspace

import (
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// This file has the display that the user chose for Bench's window, and
// going back to it (decision 70). When a display goes away, as when the
// user unplugs it or macOS turns it off while it sleeps, macOS moves the
// windows on it onto another display. Bench remembers the display that the
// user last chose, which state.json saves apart from the one the window is
// on: the user chooses a display by moving the window, within it or onto
// it, or by filling the screen on it, but not while the screens settle, as
// macOS moves the window then, and not by Bench's own moves, nor by the
// zoom with which Bench fills the screen again. When that display comes
// back, and the screens have settled, Bench moves its window back to it,
// fills the screen again if it filled it, and says so, with Undo, which
// moves it back to where it was. If macOS has put the window back itself,
// there's nothing more to do, and nothing to say. The move back isn't one
// of the changes that Tab ▸ Undo takes back (undo.go): only its toast's
// Undo, and the recent messages', take it back.

// returnRetry is how often Bench looks again, while a hold after last_tab's
// shortcut, a tab switch, or one of Bench's menus or dialogs holds up the
// move back, whether it has ended.
const returnRetry = 250 * time.Millisecond

// ownFill is how long, at most, the moves of Bench's window after Bench
// asks the shell to fill the screen again (refillLocked) are put down to
// the zoom, which animates, rather than to the user, unless the window
// fills the screen before.
const ownFill = 2 * time.Second

// benchPlace is where Bench's window was before Bench moved it back to the
// display that the user chose, for Undo: the display, and, as state.json
// keeps it, where the window was on it if it didn't fill it; and whether
// it filled it.
type benchPlace struct {
	screen config.ScreenState
	filled bool
}

// wentBack is Bench's latest move back to the display that the user chose
// (goBackLocked), which its toast's Undo takes back: step is the number
// that the toast's Undo has, counted with the changes that Undo takes back
// (undoSeq), from is where the window was, name names the move for the
// recent messages, and undone is the toast after Undo.
type wentBack struct {
	step   int
	from   benchPlace
	name   undoName
	undone string
}

// sameDisplay reports whether a display is the one saved: the one with its
// ID, or else, as macOS can give a display a new ID, the one with its name
// and frame. Two displays of the same model have the same name, so the
// name alone isn't enough, and another display can be where it was, so the
// frame alone isn't either.
func sameDisplay(saved *config.ScreenState, s platform.Screen) bool {
	if saved == nil || s.ID == "" {
		return false
	}
	return s.ID == saved.ID || (saved.Name != "" && s.Name == saved.Name && s.Frame == saved.Frame)
}

// connectedScreen returns the display saved, if it's connected: by its ID,
// or else by its name and frame (see sameDisplay).
func connectedScreen(screens []platform.Screen, saved *config.ScreenState) (platform.Screen, bool) {
	if saved == nil {
		return platform.Screen{}, false
	}
	for _, s := range screens {
		if s.ID != "" && s.ID == saved.ID {
			return s, true
		}
	}
	for _, s := range screens {
		if sameDisplay(saved, s) {
			return s, true
		}
	}
	return platform.Screen{}, false
}

// displayName is a display's name, for the toasts.
func displayName(name string) string {
	if name == "" {
		return otherDisplay
	}
	return name
}

// notePreferredAwayLocked notes, as the displays change, whether the
// display that the user chose has gone, for Bench to go back to it when it
// comes back. Bench's move back to it can't be taken back once it has
// gone again: the window is off it already.
func (e *Engine) notePreferredAwayLocked() {
	if e.preferred != nil {
		if _, ok := connectedScreen(e.p.Screens(), e.preferred); !ok {
			e.preferredAway = true
			e.forgetWentBackLocked()
		}
	}
}

// noteUserMoveLocked notes, as Bench's window moves within its screen
// while the screens are settled, that the user chose the screen for it,
// unless the move is the zoom with which Bench fills the screen again
// (refillLocked): until the window fills the screen, or ownFill has
// passed, the moves are the zoom's.
func (e *Engine) noteUserMoveLocked() {
	if e.clk.Now().Before(e.ownFillUntil) {
		if e.filled {
			e.ownFillUntil = time.Time{}
		}
		return
	}
	e.notePreferredLocked()
}

// notePreferredLocked records, as the user moves Bench's window, the
// display it's on, e.screen, as the one that the user chose for it, with
// where the window is on it as noteBenchPlaceLocked recorded it, whether or
// not the display chosen before is connected. A move while Bench is to go
// back, once a hold, a tab switch, or a menu or dialog has ended, isn't a
// choice. Choosing another display than the one Bench went back to forgets
// that move, for Undo.
func (e *Engine) notePreferredLocked() {
	if e.opts.Headless || e.screen.ID == "" || e.returnDue {
		return
	}
	s := e.screen
	now := &config.ScreenState{ID: s.ID, Name: s.Name, Frame: s.Frame}
	if e.lastScreen != nil && e.lastScreen.ID == s.ID {
		*now = *e.lastScreen
	}
	if !sameDisplay(e.preferred, s) {
		e.forgetWentBackLocked()
	}
	e.setPreferredLocked(now)
}

// setPreferredLocked makes a display the one that the user chose for
// Bench's window, saving it in state.json when it changes.
func (e *Engine) setPreferredLocked(s *config.ScreenState) {
	e.preferredAway = false
	if e.preferred != nil && *e.preferred == *s {
		return
	}
	e.preferred = s
	e.scheduleLazySaveLocked()
}

// screensSettledLocked runs as the screens settle, with e.screen up to date
// and was the screen that Bench's window was on before: changed says that
// the displays changed meanwhile. The display that the user chose is back,
// for Bench to go back to it (goBackLocked, from refillLocked), if it was
// away and is connected again, or if the displays changed and macOS moved
// the window off it, which a display that goes and comes back before Bench
// reads the displays in between makes it do: as when both changes come
// while the engine is busy, or macOS reports one change for both, as it
// can as the Mac wakes. If the window went to another display without the
// displays changing, and not by Bench's own move, the user moved it there:
// Bench doesn't go back, and the display is the one chosen now, which it
// returns as across, for the caller to note once refillLocked has noted
// where the window is.
func (e *Engine) screensSettledLocked(changed bool, was platform.Screen) (across bool) {
	own := e.ownMove
	e.ownMove = false
	if changed {
		if _, ok := connectedScreen(e.p.Screens(), e.preferred); ok {
			movedOff := !own && sameDisplay(e.preferred, was) && !sameDisplay(e.preferred, e.screen)
			if e.preferredAway || movedOff {
				e.preferredAway = false
				e.returnDue = true
			}
		}
	}
	if changed || was.ID == "" || was.ID == e.screen.ID {
		return false
	}
	if own {
		return false
	}
	e.stopReturnLocked()
	return true
}

// stopReturnLocked forgets a move back that's due.
func (e *Engine) stopReturnLocked() {
	e.returnDue = false
	if e.returnTimer != nil {
		e.returnTimer.Stop()
		e.returnTimer = nil
	}
}

// goBackLocked moves Bench's window back to the display that the user
// chose for it, now that it's back, unless the window is on it already,
// as when macOS has put it back. It fills that display again if the window
// filled it, or else puts the window where it was on it, if that's still
// on the display, or else at its usual size in the middle. While a hold
// after last_tab's shortcut or a tab switch is running, or one of Bench's
// menus or dialogs is open (menuOrDialogLocked), the move waits until it
// has ended, so that the window doesn't move from under the switch, or
// from under a native menu, which stays where it opened (decision 71). It
// reports whether the window moves, now or then; if it does, there's no
// filling of the screen it's on now to do.
func (e *Engine) goBackLocked() bool {
	target, ok := connectedScreen(e.p.Screens(), e.preferred)
	if !ok || target.ID == e.screen.ID {
		e.stopReturnLocked()
		return false
	}
	if e.hold != nil || e.wait != nil || e.menuOrDialogLocked() {
		if e.returnTimer == nil {
			e.returnTimer = e.after(returnRetry, func() {
				e.returnTimer = nil
				if e.returnDue && e.screenTimer == nil {
					e.refillLocked()
				}
			})
		}
		return true
	}
	e.stopReturnLocked()
	from := e.screen
	place := benchPlace{screen: config.ScreenState{ID: from.ID, Name: from.Name, Frame: from.Frame}, filled: e.filled}
	if e.lastScreen != nil && e.lastScreen.ID == from.ID {
		place.screen = *e.lastScreen
	}
	frame, left := openFrame(target, e.preferred)
	fill := !left && e.preferred.Window == (layout.Rect{})
	e.log.Info("moving Bench back to the display it was on", "display", target.Name, "from", from.Name, "fill", fill)
	e.moveBenchLocked(target, frame, fill)
	// The display may have come back with a new ID: it's found by that
	// from now on.
	chosen := *e.preferred
	chosen.ID, chosen.Name, chosen.Frame = target.ID, target.Name, target.Frame
	e.setPreferredLocked(&chosen)
	to := displayName(target.Name)
	e.undoSeq++
	e.wentBack = &wentBack{step: e.undoSeq, from: place,
		name: undoName{"Move to Display", "moved Bench back to " + to}, undone: "Back on " + displayName(from.Name) + "."}
	e.touch()
	e.undoToastLocked("Back on "+to+".", e.undoSeq)
	return true
}

// forgetWentBackLocked forgets Bench's latest move back to the display
// that the user chose, for Undo.
func (e *Engine) forgetWentBackLocked() {
	if e.wentBack != nil {
		e.wentBack = nil
		e.touch()
	}
}

// wentBackLocked returns Bench's latest move back to the display that the
// user chose, while Undo can take it back, for the View, or nil.
func (e *Engine) wentBackLocked() *UndoView {
	w := e.wentBack
	if w == nil {
		return nil
	}
	return &UndoView{Step: w.step, Name: w.name.name, Did: w.name.did}
}

// moveBenchLocked moves Bench's window to a display, s: its content area
// to frame, in global coordinates, then, with fill, filling the display's
// visible area. It records the move as noteBenchPlaceLocked would, for
// Bench to fill the screen again after the screens change, and waits for
// the screens to settle, as after the user moves the window to another
// display, so that the moves macOS makes meanwhile aren't the user's, and
// the size class is worked out again for the display. This is Bench's own
// move: it isn't the user's choice of display (see screensSettledLocked).
func (e *Engine) moveBenchLocked(s platform.Screen, frame layout.Rect, fill bool) {
	if dd, ok := e.p.(platform.DemoDisplays); ok && e.opts.Demo {
		dd.MoveDemoBench(s.ID)
	} else {
		e.filled = fill
		window := frame
		if fill {
			window = layout.Rect{}
		}
		e.noteScreenLocked(s, window)
		em := e.em
		e.later(func() { em.MoveToScreen(frame, fill) })
	}
	e.ownMove = true
	e.scheduleScreenSettleLocked(true)
}

// undoWentBackLocked takes back Bench's latest move back to the display
// that the user chose, for its toast's Undo: the window goes back to where
// it was, and that display is the one chosen, as the user has chosen it by
// undoing. While the window is hidden or minimised, moving it could bring
// it back, so the move waits until it's shown (refillLocked). It returns
// the toast after Undo, or an error if that display has gone since, as
// when the lid has closed, which forgets the move too.
func (e *Engine) undoWentBackLocked() (string, error) {
	w := e.wentBack
	e.forgetWentBackLocked()
	s, ok := connectedScreen(e.p.Screens(), &w.from.screen)
	if !ok {
		return "", errCantGoBackTo(w.from.screen.Name)
	}
	e.stopReturnLocked()
	place := w.from
	if e.benchHidden {
		e.placeOnShow, e.refillOnShow = &place, true
	} else {
		e.putBackLocked(&place)
	}
	chosen := place.screen
	chosen.ID, chosen.Name, chosen.Frame = s.ID, s.Name, s.Frame
	e.setPreferredLocked(&chosen)
	return w.undone, nil
}

// putBackLocked moves Bench's window back to where it was before it went
// back to the display that the user chose, for Undo. It reports whether
// that display is still connected; if it isn't, there's nowhere to put the
// window.
func (e *Engine) putBackLocked(p *benchPlace) bool {
	s, ok := connectedScreen(e.p.Screens(), &p.screen)
	if !ok {
		return false
	}
	frame, _ := openFrame(s, &p.screen)
	e.moveBenchLocked(s, frame, p.filled)
	return true
}
