package mac

import (
	"errors"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/platform"
)

// This file decides when Bench reads the badges that apps show on their
// Dock icons, and keeps what it found, for WatchDockBadges and DockBadges.
// It's pure Go, so it builds and is tested on any OS; the reading itself is
// native (benchCopyDockBadges).

// dockBadgeEvery is how often the Dock's badges are read while Bench watches
// them. Apps change their badges without telling anyone, so Bench has to
// look.
const dockBadgeEvery = 5 * time.Second

// dockBadgeGap is the least time between two readings that apps coming to
// the front ask for, so that switching quickly between apps doesn't ask
// the Dock about every one of them.
const dockBadgeGap = time.Second

// maxBadge is the most runes of a badge that Bench keeps. Apps choose their
// badges' text: a count, as most do, is a few digits.
const maxBadge = 16

// badgeWatcher reads the badges that apps show on their Dock icons while
// it's on: at once when it's turned on, again soon after an app comes to
// the front (poke), and every dockBadgeEvery. It keeps what the last
// reading found, and calls changed when that differs from what it had. A
// reading that fails keeps what it had, since the Dock may be restarting or
// busy, except that without Accessibility permission there are no badges.
//
// Its methods are safe for concurrent use and never block: poke is called
// from native callbacks on the main thread. Readings run on the clock's
// timer goroutines, one at a time.
type badgeWatcher struct {
	clk     clock.Clock
	read    func() (map[string]string, error)
	changed func()

	mu     sync.Mutex
	on     bool
	closed bool
	// gen counts the times watching was turned on, so that a reading that
	// began before it was turned off and on again is dropped.
	gen     int
	badges  map[string]string
	timer   clock.Timer
	reading bool
	// again is set when an app came to the front during a reading, which
	// may have missed what that app did.
	again bool
	// last is when the last reading finished.
	last time.Time
}

// newBadgeWatcher returns a watcher that's off, which reads the badges with
// read and calls changed when they change. A nil clk means clock.Real().
// changed must not block.
func newBadgeWatcher(clk clock.Clock, read func() (map[string]string, error), changed func()) *badgeWatcher {
	if clk == nil {
		clk = clock.Real()
	}
	return &badgeWatcher{clk: clk, read: read, changed: changed}
}

// set turns watching on or off. Turning it off forgets the badges, calling
// changed if there were any.
func (w *badgeWatcher) set(on bool) {
	w.mu.Lock()
	if w.closed || on == w.on {
		w.mu.Unlock()
		return
	}
	w.on = on
	w.gen++
	w.stopTimerLocked()
	had := len(w.badges) > 0
	w.badges = nil
	if on {
		w.scheduleLocked(0)
	}
	w.mu.Unlock()
	if had {
		w.changed()
	}
}

// poke asks for a reading soon, while watching is on: an app came to the
// front, as the user goes to read what its badge counts.
func (w *badgeWatcher) poke() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.on {
		return
	}
	if w.reading {
		w.again = true
		return
	}
	if w.timer != nil && !w.timer.Stop() {
		// The timer has fired, and its reading is about to begin.
		return
	}
	w.scheduleLocked(max(dockBadgeGap-w.clk.Now().Sub(w.last), 0))
}

// current returns what the last reading found, by bundle ID: none while
// watching is off.
func (w *badgeWatcher) current() map[string]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return maps.Clone(w.badges)
}

// close turns watching off for good, without calling changed.
func (w *badgeWatcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed, w.on = true, false
	w.gen++
	w.stopTimerLocked()
	w.badges = nil
}

func (w *badgeWatcher) stopTimerLocked() {
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

// scheduleLocked reads the badges after d.
func (w *badgeWatcher) scheduleLocked(d time.Duration) {
	gen := w.gen
	w.timer = w.clk.AfterFunc(d, func() { w.run(gen) })
}

// run reads the badges, for the watching that gen counts, and schedules the
// next reading.
func (w *badgeWatcher) run(gen int) {
	w.mu.Lock()
	if !w.on || gen != w.gen {
		w.mu.Unlock()
		return
	}
	w.timer = nil
	if w.reading {
		// A reading that began before watching was turned off and on
		// again is still running. The next one follows it.
		w.mu.Unlock()
		return
	}
	w.reading, w.again = true, false
	w.mu.Unlock()

	found, err := w.read()

	w.mu.Lock()
	w.reading = false
	w.last = w.clk.Now()
	if !w.on {
		w.mu.Unlock()
		return
	}
	if gen != w.gen {
		// Watching was turned off and on again during the reading, so
		// what it found may be from before. Read again.
		if w.timer == nil {
			w.scheduleLocked(0)
		}
		w.mu.Unlock()
		return
	}
	changed := false
	switch {
	case err == nil:
		found = cleanBadges(found)
		changed = !maps.Equal(found, w.badges)
		w.badges = found
	case errors.Is(err, platform.ErrNotTrusted):
		changed = len(w.badges) > 0
		w.badges = nil
	}
	next := dockBadgeEvery
	if w.again {
		next = dockBadgeGap
	}
	w.scheduleLocked(next)
	w.mu.Unlock()
	if changed {
		w.changed()
	}
}

// cleanBadges trims the space around each badge, clips it to maxBadge
// runes, and leaves out those with no text or no bundle ID.
func cleanBadges(found map[string]string) map[string]string {
	clean := make(map[string]string, len(found))
	for bundleID, label := range found {
		label = clip(strings.TrimSpace(label), maxBadge)
		if bundleID != "" && label != "" {
			clean[bundleID] = label
		}
	}
	return clean
}
