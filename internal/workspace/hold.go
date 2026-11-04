package workspace

import (
	"slices"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// The hold after last_tab's shortcut (decision 60), as Command-Tab works
// for apps. The shortcut doesn't switch tabs while its modifiers are still
// held: Bench waits for the user to let go of them, which it sees by
// asking the platform which are held (Platform.HeldModifiers) every
// holdPoll. Each press of the shortcut meanwhile chooses the next tab in
// the order they were last used, and letting go switches to the tab
// chosen. A quick tap chooses the last used tab, as the shortcut always
// did. Held past holdDelay, Bench shows every tab in that order, with the
// chosen one marked (View.Hold), in the overview of every tab. A hold ends
// without switching after holdLimit, once the Mac has slept, and as it
// wakes, as the user can't still be holding the keys.

const (
	// holdDelay is how long last_tab's modifiers must stay held after its
	// shortcut before Bench shows every tab to choose from.
	holdDelay = 300 * time.Millisecond
	// holdPoll is how often Bench asks whether they're still held.
	holdPoll = 20 * time.Millisecond
	// holdLimit is the longest a hold lasts. Past it, Bench gives up
	// without switching, as Escape does: it may have missed the user
	// letting go.
	holdLimit = time.Minute
)

// The bench:commands that open the overview of every tab: hold-tabs, for
// a hold that shows the tabs, and toggle-all-tabs, for the show_all_tabs
// shortcut, which closes it if it's open.
const (
	holdTabsCommand      = "hold-tabs"
	toggleAllTabsCommand = "toggle-all-tabs"
)

// The bench:commands that a hold that shows the tabs sends for
// focus_pane_modifiers with Up and Down: the frontend, which knows how many
// cards each row has, moves the choice to the card above or below
// (StepHold).
const (
	holdUpCommand   = "hold-up"
	holdDownCommand = "hold-down"
)

// holdSlept is how far the wall clock may run ahead of the monotonic clock
// during a hold. The monotonic clock, which timers and Time.Sub use, stops
// while the Mac sleeps, and the wall clock doesn't: a hold that has run
// further ahead has slept, and the user can't still be holding the keys.
const holdSlept = time.Second

// holdState is a hold after last_tab's shortcut.
type holdState struct {
	// id tells this hold from the ones before it (HoldView.ID).
	id int
	// tabs are the tabs to choose from, most recently used first: the
	// current tab, then the last used tab (recentTabsLocked).
	tabs []*Tab
	// sel is the chosen tab's index in tabs.
	sel int
	// none is set when there was no last used tab as the hold began
	// (lastTabLocked): a quick tap then says so, as LastTab does, and only
	// a hold that shows the tabs switches.
	none bool
	// mods are last_tab's modifiers, which the user holds.
	mods platform.Modifiers
	// start is when the hold began, and shown is set once Bench shows the
	// tabs.
	start time.Time
	shown bool
	// timer asks again whether the modifiers are held.
	timer clock.Timer
}

// noteLeftTabLocked notes that the user has left a tab, which is the last
// used tab now: first in the order that the hold shows (recentTabsLocked),
// after the current tab.
func (e *Engine) noteLeftTabLocked(t *Tab) {
	e.lastTab = t
	recent := make([]*Tab, 0, len(e.recent)+1)
	recent = append(recent, t)
	for _, r := range e.recent {
		if r != t && e.model.IndexOf(r) >= 0 {
			recent = append(recent, r)
		}
	}
	e.recent = recent
}

// recentTabsLocked returns every tab, most recently used first: the
// current tab, the last used tab (lastTabLocked), then the others in the
// order the user left them, and last the tabs not used since Bench opened,
// in the tab bar's order.
func (e *Engine) recentTabsLocked() []*Tab {
	m := e.model
	tabs := make([]*Tab, 0, len(m.Tabs))
	seen := make(map[*Tab]bool, len(m.Tabs))
	add := func(t *Tab) {
		if t != nil && !seen[t] && m.IndexOf(t) >= 0 {
			seen[t] = true
			tabs = append(tabs, t)
		}
	}
	add(m.CurrentTab())
	if i := e.lastTabLocked(); i >= 0 {
		add(m.Tabs[i])
	}
	for _, t := range e.recent {
		add(t)
	}
	for _, t := range m.Tabs {
		add(t)
	}
	return tabs
}

// startHoldLocked starts a hold after last_tab's shortcut, if its
// modifiers are still held, and reports whether it did. It doesn't while a
// menu or dialog is open, when the shortcut has no modifiers, or while
// Bench is hidden and tab shortcuts leave it hidden
// (settings.show_on_tab_shortcut), as Bench couldn't show the tabs then.
// The shortcut goes back to the last used tab at once instead. A hold
// starts before Bench has switched tabs too, with the first other tab in
// the tab bar chosen, as Command-Tab works from the first press.
func (e *Engine) startHoldLocked() bool {
	if !e.started || e.overlay || e.closed {
		return false
	}
	if (e.benchHidden || e.p.OwnAppHidden()) && !e.cfg.Settings.ShowOnTabShortcut {
		return false
	}
	mods := platform.ModifiersNamed(shortcuts.Modifiers(e.cfg.Shortcuts.LastTab))
	if mods == 0 || e.p.HeldModifiers()&mods != mods {
		return false
	}
	tabs := e.recentTabsLocked()
	if len(tabs) < 2 {
		return false
	}
	e.holds++
	h := &holdState{id: e.holds, tabs: tabs, sel: 1, none: e.lastTabLocked() < 0, mods: mods, start: e.clk.Now()}
	e.hold = h
	h.timer = e.after(holdPoll, func() { e.pollHoldLocked(h) })
	return true
}

// pollHoldLocked asks whether the hold's modifiers are still held. Once
// they aren't, it switches to the chosen tab. Held past holdDelay, it
// shows the tabs, unless a menu or dialog is open. Past holdLimit, or once
// the Mac has slept, it gives up first: the keys seen let go of after a
// sleep were let go of before it, or in it.
func (e *Engine) pollHoldLocked(h *holdState) {
	if e.hold != h {
		return
	}
	h.timer = nil
	now := e.clk.Now()
	held := now.Sub(h.start)
	if held >= holdLimit || now.Round(0).Sub(h.start.Round(0))-held > holdSlept {
		e.endHoldLocked(false)
		return
	}
	if e.p.HeldModifiers()&h.mods != h.mods {
		e.endHoldLocked(true)
		return
	}
	if !h.shown && held >= holdDelay && !e.overlay {
		h.shown = true
		// Bench takes the keyboard to show the tabs. Escape gives it back
		// (see noteJumpLocked).
		e.noteJumpLocked()
		e.touch()
		em := e.em
		e.later(func() { em.Command(Command{Name: holdTabsCommand}) })
	}
	h.timer = e.after(holdPoll, func() { e.pollHoldLocked(h) })
}

// stepHoldLocked chooses the next tab of the hold (step 1) or the previous
// one (-1), wrapping, and passing over tabs deleted since it began. A
// longer step is a row's worth of the overview's cards, for the card
// below (a row's length) or above (minus it): it stops at the first and
// last cards, as the arrow keys do in the overview without a hold.
func (e *Engine) stepHoldLocked(step int) {
	h := e.hold
	n := len(h.tabs)
	if step < -1 || step > 1 {
		// The tabs still there, as the overview draws them.
		var shown []int
		for k, t := range h.tabs {
			if e.model.IndexOf(t) >= 0 {
				shown = append(shown, k)
			}
		}
		if at := slices.Index(shown, h.sel); at >= 0 {
			h.sel = shown[max(0, min(at+step, len(shown)-1))]
			if h.shown {
				e.touch()
			}
			return
		}
		// The tab chosen has been deleted: the next or previous one.
		step = max(-1, min(step, 1))
	}
	for range n {
		h.sel = ((h.sel+step)%n + n) % n
		if e.model.IndexOf(h.tabs[h.sel]) >= 0 {
			break
		}
	}
	if h.shown {
		e.touch()
	}
}

// endHoldLocked ends the hold, if there is one, and with choose, switches
// to the tab chosen, if it's still there. The switch gives the keyboard to
// that tab, so the window that had it before the hold doesn't get it back.
// A quick tap before Bench has switched tabs says there's no tab to go
// back to, as LastTab does without a hold.
func (e *Engine) endHoldLocked(choose bool) {
	h := e.hold
	if h == nil {
		return
	}
	e.hold = nil
	if h.timer != nil {
		h.timer.Stop()
	}
	if h.shown {
		e.touch()
	}
	if !choose {
		return
	}
	if h.none && !h.shown {
		e.toastLocked(errNoLastTab.Error())
		return
	}
	if i := e.model.IndexOf(h.tabs[h.sel]); i >= 0 {
		if h.shown {
			e.jump = jumpNote{}
		}
		e.switchToLocked(i)
	}
}

// holdShortcutLocked handles a global shortcut during a hold, and reports
// whether it did.
//
// Before the hold shows the tabs, any shortcut but last_tab's ends it
// first, switching to the last used tab as a quick tap does, so that it
// acts on that tab, as it did when the tap switched at once.
//
// While the hold shows the tabs, the user is holding last_tab's modifiers,
// which Bench's other shortcuts share by default: next_tab and
// previous_tab, and focus_pane_modifiers with Left and Right, move the
// choice, as the arrow keys do in the overview, and with Up and Down ask
// the frontend to move it a row (holdUpCommand); send_to_last_tab, the
// shortcut with Shift added by default, moves it back, as Command-Shift-Tab
// does; and zoom_pane, whose key is Return, switches at once, as Return
// does. A tab's own shortcut switches to it, ending the hold
// (switchLocked), last_tab's chooses the next tab (LastTab), and jump and
// show_all_tabs end it, so it doesn't handle those. The rest do nothing
// until the hold ends.
func (e *Engine) holdShortcutLocked(action, arg string) bool {
	if e.hold == nil {
		return false
	}
	if !e.hold.shown {
		if action != shortcuts.ActionLastTab {
			e.endHoldLocked(true)
		}
		return false
	}
	switch action {
	case shortcuts.ActionTab, shortcuts.ActionLastTab, shortcuts.ActionJump, shortcuts.ActionShowAllTabs:
		return false
	case shortcuts.ActionFocusPane:
		switch arg {
		case "left":
			e.stepHoldLocked(-1)
		case "right":
			e.stepHoldLocked(1)
		case "up", "down":
			name := holdUpCommand
			if arg == "down" {
				name = holdDownCommand
			}
			em := e.em
			e.later(func() { em.Command(Command{Name: name}) })
		}
	case shortcuts.ActionNextTab:
		e.stepHoldLocked(1)
	case shortcuts.ActionPreviousTab, shortcuts.ActionSendToLastTab:
		e.stepHoldLocked(-1)
	case shortcuts.ActionZoomPane:
		e.endHoldLocked(true)
	}
	return true
}

// holdViewLocked returns the hold's tabs, by index, and the chosen one's,
// while Bench shows them, or nil.
func (e *Engine) holdViewLocked() *HoldView {
	h := e.hold
	if h == nil || !h.shown {
		return nil
	}
	v := &HoldView{ID: h.id, Tabs: make([]int, 0, len(h.tabs)), Selected: -1}
	for k, t := range h.tabs {
		i := e.model.IndexOf(t)
		if i < 0 {
			continue
		}
		v.Tabs = append(v.Tabs, i)
		if k == h.sel {
			v.Selected = i
		}
	}
	return v
}

// StepHold moves the choice of the hold that shows the tabs, for the arrow
// keys in the overview: to the next tab (step 1) or the previous one (-1),
// wrapping, or by a row of cards as the overview draws them, to the card
// below (the row's length) or above (minus it), stopping at the first and
// last (stepHoldLocked). It does nothing if there's no such hold, as it
// may have ended just before.
func (e *Engine) StepHold(step int) error {
	e.lock()
	defer e.unlock()
	if e.hold == nil || !e.hold.shown || step == 0 {
		return nil
	}
	e.stepHoldLocked(step)
	return nil
}

// CancelHold ends the hold without switching tabs, for Escape in the
// overview, or a click outside it. It does nothing if there's no hold.
func (e *Engine) CancelHold() error {
	e.lock()
	defer e.unlock()
	e.endHoldLocked(false)
	return nil
}

// demoKeys is what the browser preview's platform adds, for the modifier
// keys that the page says are held.
type demoKeys interface {
	SetHeldModifiers(m platform.Modifiers)
}

// DemoModifiers says which modifier keys are held down in the browser
// preview's page, by the names shortcuts.Modifiers gives them, so that
// the hold after last_tab's shortcut works there as on a Mac. It needs the
// preview's platform (platform.NewDemo).
func (e *Engine) DemoModifiers(held []string) error {
	dk, ok := e.p.(demoKeys)
	if !ok || !e.opts.Demo {
		return errNoDemoKeys
	}
	dk.SetHeldModifiers(platform.ModifiersNamed(held))
	return nil
}
