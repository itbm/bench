package mac

import (
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/platform"
)

// dockForTest stands in for the Dock: read returns its badges, or its
// error, and counts the readings.
type dockForTest struct {
	badges  map[string]string
	err     error
	reads   int
	changes int
	during  func() // Runs during a reading, if set.
}

func (d *dockForTest) read() (map[string]string, error) {
	d.reads++
	if d.during != nil {
		f := d.during
		d.during = nil
		f()
	}
	if d.err != nil {
		return nil, d.err
	}
	return maps.Clone(d.badges), nil
}

func newWatcherForTest() (*badgeWatcher, *dockForTest, *clock.Fake) {
	clk := clock.NewFake(time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC))
	d := &dockForTest{badges: map[string]string{}}
	return newBadgeWatcher(clk, d.read, func() { d.changes++ }), d, clk
}

func TestBadgeWatcherReadsWhileOn(t *testing.T) {
	w, d, clk := newWatcherForTest()
	d.badges["com.apple.mail"] = "8"
	clk.Advance(time.Minute)
	if d.reads != 0 || len(w.current()) != 0 {
		t.Fatalf("a watcher that's off read %d times and has %v, want nothing", d.reads, w.current())
	}

	w.set(true)
	clk.Advance(0)
	if d.reads != 1 || d.changes != 1 {
		t.Fatalf("turned on: %d readings, %d changes, want 1 and 1", d.reads, d.changes)
	}
	if got := w.current(); got["com.apple.mail"] != "8" || len(got) != 1 {
		t.Errorf("current() = %v, want Mail's 8", got)
	}

	// The same badges again aren't a change.
	clk.Advance(dockBadgeEvery)
	if d.reads != 2 || d.changes != 1 {
		t.Errorf("after %v: %d readings, %d changes, want 2 and 1", dockBadgeEvery, d.reads, d.changes)
	}
	d.badges["com.tinyspeck.slackmacgap"] = "•"
	clk.Advance(dockBadgeEvery - time.Millisecond)
	if d.reads != 2 {
		t.Errorf("read %d times before %v had passed, want 2", d.reads, dockBadgeEvery)
	}
	clk.Advance(time.Millisecond)
	if d.reads != 3 || d.changes != 2 || len(w.current()) != 2 {
		t.Errorf("a new badge: %d readings, %d changes, %v; want 3, 2, and both badges", d.reads, d.changes, w.current())
	}

	// current is a copy.
	w.current()["com.example.Other"] = "1"
	if len(w.current()) != 2 {
		t.Error("changing what current() returned changed the watcher's badges")
	}

	w.set(false)
	if d.changes != 3 || len(w.current()) != 0 {
		t.Errorf("turned off: %d changes, %v; want 3 and no badges", d.changes, w.current())
	}
	clk.Advance(time.Minute)
	if d.reads != 3 {
		t.Errorf("read %d times after being turned off, want 3", d.reads)
	}
	// Turning it off again, with no badges, isn't a change.
	w.set(false)
	if d.changes != 3 {
		t.Errorf("turning off twice made %d changes, want 3", d.changes)
	}
}

func TestBadgeWatcherFailures(t *testing.T) {
	w, d, clk := newWatcherForTest()
	d.badges["com.apple.mail"] = "8"
	w.set(true)
	clk.Advance(0)

	// The Dock not answering, or restarting, keeps what Bench had.
	d.err = fmt.Errorf("reading the Dock's badges: %w", errors.New("cannot complete"))
	clk.Advance(dockBadgeEvery)
	if d.reads != 2 || d.changes != 1 || w.current()["com.apple.mail"] != "8" {
		t.Errorf("a failed reading: %d readings, %d changes, %v; want 2, 1, and Mail's badge kept", d.reads, d.changes, w.current())
	}
	// Losing Accessibility permission leaves no badges.
	d.err = fmt.Errorf("reading the Dock's badges: %w", platform.ErrNotTrusted)
	clk.Advance(dockBadgeEvery)
	if d.changes != 2 || len(w.current()) != 0 {
		t.Errorf("without Accessibility: %d changes, %v; want 2 and no badges", d.changes, w.current())
	}
	clk.Advance(dockBadgeEvery)
	if d.reads != 4 || d.changes != 2 {
		t.Errorf("still without Accessibility: %d readings, %d changes, want 4 and 2", d.reads, d.changes)
	}
	d.err = nil
	clk.Advance(dockBadgeEvery)
	if d.changes != 3 || w.current()["com.apple.mail"] != "8" {
		t.Errorf("with Accessibility again: %d changes, %v; want 3 and Mail's badge", d.changes, w.current())
	}
}

func TestBadgeWatcherPoke(t *testing.T) {
	w, d, clk := newWatcherForTest()
	w.poke()
	clk.Advance(time.Minute)
	if d.reads != 0 {
		t.Fatalf("a poke while off read %d times, want 0", d.reads)
	}
	w.set(true)
	clk.Advance(0)

	// An app coming to the front just after a reading waits for the gap.
	clk.Advance(200 * time.Millisecond)
	w.poke()
	w.poke()
	clk.Advance(dockBadgeGap - 200*time.Millisecond - time.Millisecond)
	if d.reads != 1 {
		t.Errorf("read %d times before the gap had passed, want 1", d.reads)
	}
	clk.Advance(time.Millisecond)
	if d.reads != 2 {
		t.Errorf("read %d times after a poke and the gap, want 2", d.reads)
	}
	// A while after the last reading, a poke reads at once.
	clk.Advance(3 * time.Second)
	w.poke()
	clk.Advance(0)
	if d.reads != 3 {
		t.Errorf("read %d times after a later poke, want 3", d.reads)
	}
	// The next reading is a whole interval after that one.
	clk.Advance(dockBadgeEvery - time.Millisecond)
	if d.reads != 3 {
		t.Errorf("read %d times before the interval, want 3", d.reads)
	}
	clk.Advance(time.Millisecond)
	if d.reads != 4 {
		t.Errorf("read %d times after the interval, want 4", d.reads)
	}

	// A poke during a reading has the next one come after the gap.
	d.during = w.poke
	clk.Advance(dockBadgeEvery)
	if d.reads != 5 {
		t.Fatalf("read %d times, want 5", d.reads)
	}
	clk.Advance(dockBadgeGap)
	if d.reads != 6 {
		t.Errorf("read %d times after a poke during a reading, want 6", d.reads)
	}
}

func TestBadgeWatcherTurnedOffAndOnWhileReading(t *testing.T) {
	w, d, clk := newWatcherForTest()
	d.badges["com.apple.mail"] = "8"
	w.set(true)
	d.during = func() {
		w.set(false)
		d.badges["com.apple.mail"] = "9"
		w.set(true)
	}
	clk.Advance(0)
	// The first reading's answer is dropped, and a second one follows.
	if d.reads != 2 || w.current()["com.apple.mail"] != "9" {
		t.Errorf("%d readings, %v; want 2 and Mail's 9", d.reads, w.current())
	}
	if clk.Pending() != 1 {
		t.Errorf("%d timers pending, want 1, for the next reading", clk.Pending())
	}
}

func TestBadgeWatcherClose(t *testing.T) {
	w, d, clk := newWatcherForTest()
	d.badges["com.apple.mail"] = "8"
	w.set(true)
	clk.Advance(0)
	w.close()
	if len(w.current()) != 0 || d.changes != 1 {
		t.Errorf("closed: %v, %d changes; want no badges, and no change reported", w.current(), d.changes)
	}
	w.set(true)
	w.poke()
	clk.Advance(time.Minute)
	if d.reads != 1 {
		t.Errorf("a closed watcher read %d times, want 1", d.reads)
	}
}

func TestCleanBadges(t *testing.T) {
	got := cleanBadges(map[string]string{
		"com.apple.mail":            " 8 ",
		"com.tinyspeck.slackmacgap": "•",
		"com.example.Empty":         "  ",
		"":                          "3",
		"com.example.Long":          strings.Repeat("9", 40),
	})
	want := map[string]string{
		"com.apple.mail":            "8",
		"com.tinyspeck.slackmacgap": "•",
		"com.example.Long":          strings.Repeat("9", maxBadge),
	}
	if !maps.Equal(got, want) {
		t.Errorf("cleanBadges = %v, want %v", got, want)
	}
}
