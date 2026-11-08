package workspace

import (
	"time"

	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// This file has the marks that say an item needs the user: a terminal whose
// program rang the bell or asked for a notification, or whose command
// finished, or a Web item whose page posted a notification, while the user
// couldn't see it. Its sub-tab and its tab show the mark until it's shown,
// and the Dock icon counts the items marked. Terminals also post
// notifications, as pages do, when the notify_terminals setting is on.

// notifyGap is the shortest time between two notifications for one
// terminal, so that a program that rings the bell, or asks for
// notifications, over and over doesn't flood Notification Centre. Its mark
// still says why it last needed the user.
const notifyGap = 5 * time.Second

// Why a terminal needs the user, for its mark and its notification.
const (
	bellText     = "Rang the bell"
	finishedText = "Its command has finished"
	failedText   = "Its command ended with an error"
	notePrefix   = "Notification: "
)

// noteText returns a notification's text in one line: its title and its
// body, or whichever it has.
func noteText(title, body string) string {
	switch {
	case title == "":
		return body
	case body == "":
		return title
	}
	return title + ": " + body
}

// seenLocked reports whether the user can see an item now, with Bench's
// window shown: the item is what a displayed pane of the current tab shows,
// while Bench is active, or an app with a window docked in the current tab
// is, as when the user types in Zed beside a terminal; or it's in a window
// of its own, while Bench is active. In the browser preview, whose platform
// says nothing about the active app, Bench always is. The drop-down is seen
// while it shows, over any app, even with Bench hidden.
func (e *Engine) seenLocked(it *Item) bool {
	if e.isDropdownLocked(it) {
		return e.dropShown
	}
	if e.benchHidden {
		return false
	}
	active := e.benchActive || e.opts.Demo
	if e.model.IsFloating(it.ID) {
		return it.OwnWindow && active
	}
	if !active && !e.inCurrentTabLocked(e.activeApp) {
		return false
	}
	for _, g := range e.displayLocked().panes {
		if g.node.Active == it.ID {
			return true
		}
	}
	return false
}

// markLocked marks an item in a tab, or the drop-down, as needing the user,
// for why, unless the user can see it. Another popped-out item has no
// sub-tab to mark. The drop-down's mark shows on the menu bar item. It
// reports whether the user can't see the item, and whether it was marked
// already. A new reason takes the place of the agent's words (see
// agent.go), if the mark had them.
func (e *Engine) markLocked(it *Item, why string) (unseen, already bool) {
	if e.seenLocked(it) {
		return false, false
	}
	already = it.attention != ""
	if ti, _ := e.model.Where(it.ID); (ti >= 0 || e.isDropdownLocked(it)) && it.attention != why {
		it.attention = why
		it.agent.marked = false
		e.touch()
	}
	return true, already
}

// clearSeenLocked takes the mark off the items the user can see now, and
// off those that have left their tabs, other than the drop-down, and
// reports whether it took any off. It also forgets that the agents in the
// terminals the user can see have finished, or stopped with an error (see
// agent.go).
// Arranging calls it, and so does sending the view, so that showing an item
// in any way takes its mark off.
func (e *Engine) clearSeenLocked() bool {
	cleared := false
	for _, it := range e.model.Items {
		if it.attention != "" && (e.seenLocked(it) || (e.model.IsFloating(it.ID) && !e.isDropdownLocked(it))) {
			it.attention = ""
			it.agent.marked = false
			cleared = true
		}
		if e.agentSeenLocked(it) {
			cleared = true
		}
	}
	return cleared
}

// terminalAlertLocked marks a terminal whose program rang the bell or asked
// for a notification while the user couldn't see it, and posts a
// notification for it: always for one its program asked for, but for the
// bell only when it marks the terminal, not again while it's marked. An
// alert that's part of what the terminal's agent reported changes nothing
// more (see agentEchoLocked), and nor does a bell while the mark has the
// agent's words, which stay.
func (e *Engine) terminalAlertLocked(it *Item, a term.Alert) {
	if e.agentEchoLocked(it, a) || (a.Bell && it.agent.marked && it.attention != "") {
		return
	}
	why := bellText
	if !a.Bell {
		why = notePrefix + noteText(a.Title, a.Body)
	}
	unseen, already := e.markLocked(it, why)
	switch {
	case !unseen:
	case a.Bell && !already:
		e.notifyTerminalLocked(it, bellText+".")
	case !a.Bell:
		e.notifyTerminalLocked(it, noteText(a.Title, a.Body))
	}
}

// commandEndedLocked marks a terminal whose command has ended while the
// user couldn't see it, and posts a notification to say so.
func (e *Engine) commandEndedLocked(it *Item, failed bool) {
	why := finishedText
	if failed {
		why = failedText
	}
	if unseen, _ := e.markLocked(it, why); unseen {
		e.notifyTerminalLocked(it, why+".")
	}
}

// notifyTerminalLocked posts a notification for a terminal, with its
// sub-tab's title, the tab it's in (or "Drop-down" for the drop-down), and
// body, if the notify_terminals setting is on, and Bench hasn't posted one
// for it within notifyGap.
// Clicking it shows the terminal (see showAnywhereLocked).
func (e *Engine) notifyTerminalLocked(it *Item, body string) {
	if !e.cfg.Settings.NotifyTerminals {
		return
	}
	now := e.clk.Now()
	if !it.notified.IsZero() && now.Sub(it.notified) < notifyGap {
		return
	}
	it.notified = now
	n := platform.Notification{ItemID: it.ID, Title: e.shownTitleLocked(it), Body: body}
	if ti, _ := e.model.Where(it.ID); ti >= 0 {
		n.Subtitle = e.model.Tabs[ti].Name
	} else if e.isDropdownLocked(it) {
		n.Subtitle = dropdownSubtitle
	}
	if n.Title == "" {
		n.Title = it.App
	}
	if err := e.p.Notify(n); err != nil {
		e.log.Debug("couldn't post a notification", "item", it.ID, "error", err)
	}
}

// attentionCountLocked returns how many items are marked as needing the
// user, for the Dock icon.
func (e *Engine) attentionCountLocked() int {
	n := 0
	for _, it := range e.model.Items {
		if it.attention != "" {
			n++
		}
	}
	return n
}

// badgeChangedLocked returns the count for the Dock icon's badge, and
// whether it has changed since it was last set.
func (e *Engine) badgeChangedLocked() (int, bool) {
	n := e.attentionCountLocked()
	if n == e.badge {
		return n, false
	}
	e.badge = n
	return n, true
}
