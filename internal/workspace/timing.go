package workspace

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"
)

// This file times each arrangement of the current tab, from what asked
// for it to the last frame set, and keeps the latest times for Help ▸
// Report a Problem… and the log (decision 67). The clock is the engine's,
// whose times carry the monotonic clock's reading, which Time.Sub uses, so
// a change of the wall clock doesn't count.

// timedArranges is how many tab switches Bench keeps the times of, and how
// many other arrangements: the latest of each. They're kept in memory
// only.
const timedArranges = 50

// The reasons for an arrangement that Report a Problem and the log give.
// An arrangement for a platform event has the event's kind as its reason,
// such as bench-moved or window-moved.
const (
	// reasonTabShortcut is a tab's shortcut, next_tab's, previous_tab's,
	// or last_tab's, which work from any app, and the end of a hold after
	// last_tab's shortcut, as its keys are let go of (decision 60).
	reasonTabShortcut = "tab-shortcut"
	// reasonTab is SwitchTab, StepTab, or LastTab: a click on a tab, a
	// row of the jump palette, a card of the overview, a key in Bench's
	// window, the Tab menu, or the menu bar item.
	reasonTab = "tab"
	// reasonLink is a bench:// link's switch: tab, next, previous, or last
	// (FollowLink, decision 77).
	reasonLink = "link"
	// reasonScript is a script's switch: bench tab, next, previous, or last
	// (RunScript, decision 77).
	reasonScript = "script"
	// reasonSwitch is any other tab switch, such as Bench's first.
	reasonSwitch = "switch"
	// reasonPaneArea is a new pane area, as Bench's window changed size.
	reasonPaneArea = "pane-area"
	// reasonRearrange is Rearrange, from its shortcut, a menu, or the jump
	// palette.
	reasonRearrange = "rearrange"
	// reasonSettle is the end of the wait for the screens to settle after
	// a wake, an unlock, or a change of displays (decision 56).
	reasonSettle = "settle"
	// reasonAppsOpened is a tab switch's wait for the apps it opened.
	reasonAppsOpened = "apps-opened"
	// reasonTimer is the end of another wait.
	reasonTimer = "timer"
	// reasonOther is anything else, such as a change made in Bench's
	// window or menus.
	reasonOther = "other"
)

// lockHold is what the engine notes about the current hold of its lock,
// to time what it arranges (see unlock).
type lockHold struct {
	// at is when what took the lock was asked for, or zero if that isn't
	// known, and reason why, or "" until something says (because).
	at     time.Time
	reason string
	// arranged is set once an arrangement in this hold has applied a plan,
	// end is when the last one ended, and windows is the most windows that
	// one of their plans asked the platform to change.
	arranged bool
	end      time.Time
	windows  int
}

// arrangeTime is one arrangement's time.
type arrangeTime struct {
	reason  string
	took    time.Duration
	windows int
}

// isSwitch reports whether an arrangement's reason is a tab switch's. The
// engine keeps the switches' times apart from the others' (Engine.switches),
// so that the many arrangements as Bench's window is dragged, one a frame,
// don't push them out.
func isSwitch(reason string) bool {
	switch reason {
	case reasonTab, reasonTabShortcut, reasonLink, reasonScript, reasonSwitch:
		return true
	}
	return false
}

// arrangeTimes keeps the latest timedArranges arrangements' times in a
// ring, which never grows.
type arrangeTimes struct {
	ring [timedArranges]arrangeTime
	next int // Where the next one goes.
	n    int // How many there are.
}

// add keeps an arrangement's time, in place of the oldest once the ring is
// full.
func (r *arrangeTimes) add(t arrangeTime) {
	r.ring[r.next] = t
	r.next = (r.next + 1) % timedArranges
	r.n = min(r.n+1, timedArranges)
}

// ArrangeTiming is how long the latest arrangements for one reason took,
// in a ProblemReport: from what asked for each (a tab's shortcut, a click
// in Bench's window, a menu, an event, or the end of a wait), as the
// engine got it, to the last frame set.
type ArrangeTiming struct {
	Reason string
	// Count is how many of the latest arrangements had the reason.
	Count int
	// Median and Worst are how long they took.
	Median, Worst time.Duration
	// Windows is the most windows that one of them asked the platform to
	// change.
	Windows int
}

// summary returns the times kept, by reason: the reasons with the most
// arrangements first, then by name.
func (r *arrangeTimes) summary() []ArrangeTiming {
	took := map[string][]time.Duration{}
	most := map[string]int{}
	for _, t := range r.ring[:r.n] {
		took[t.reason] = append(took[t.reason], t.took)
		most[t.reason] = max(most[t.reason], t.windows)
	}
	out := make([]ArrangeTiming, 0, len(took))
	for reason, ds := range took {
		slices.Sort(ds)
		out = append(out, ArrangeTiming{Reason: reason, Count: len(ds), Median: median(ds), Worst: ds[len(ds)-1], Windows: most[reason]})
	}
	slices.SortFunc(out, func(a, b ArrangeTiming) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Reason, b.Reason))
	})
	return out
}

// median returns the middle of sorted durations, or the mean of the two
// in the middle.
func median(sorted []time.Duration) time.Duration {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// lockFor takes the lock for work asked for now, for a reason.
func (e *Engine) lockFor(reason string) { e.lockAt(e.clk.Now(), reason) }

// lockAt takes the lock for work asked for at a time, for a reason, or ""
// for one that the work gives later (because), if it arranges. The time is
// taken before the lock, so that the wait for it counts.
func (e *Engine) lockAt(at time.Time, reason string) {
	e.mu.Lock()
	e.held = lockHold{at: at, reason: reason}
}

// because gives the reason for what this hold of the lock arranges, unless
// what took it has given one.
func (e *Engine) because(reason string) {
	if e.held.reason == "" {
		e.held.reason = reason
	}
}

// noteArrangeStartLocked notes, as an arrangement starts, when it was asked
// for, if what took the lock didn't say.
func (e *Engine) noteArrangeStartLocked() {
	if e.held.at.IsZero() {
		e.held.at = e.clk.Now()
	}
}

// noteArrangedLocked notes that an arrangement that applied a plan has
// ended now.
func (e *Engine) noteArrangedLocked() {
	e.held.arranged = true
	e.held.end = e.clk.Now()
}

// keepArrangeTimeLocked keeps the time of what this hold of the lock
// arranged, with the switches' or the others' (isSwitch), and logs it, at
// debug level, as one line. It runs as the lock is released.
func (e *Engine) keepArrangeTimeLocked() {
	h := e.held
	reason := cmp.Or(h.reason, reasonOther)
	took := max(h.end.Sub(h.at), 0)
	t := arrangeTime{reason: reason, took: took, windows: h.windows}
	if isSwitch(reason) {
		e.switches.add(t)
	} else {
		e.arranges.add(t)
	}
	if e.log.Enabled(context.Background(), slog.LevelDebug) {
		e.log.Debug("arranged", "reason", reason, "ms", float64(took.Microseconds())/1000, "windows", h.windows)
	}
}
