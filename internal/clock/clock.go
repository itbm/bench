// Package clock lets code that waits be tested without waiting.
package clock

import (
	"sort"
	"sync"
	"time"
)

// Clock tells the time and runs functions later.
type Clock interface {
	Now() time.Time
	// AfterFunc calls f in its own goroutine after d.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is a pending call from AfterFunc.
type Timer interface {
	// Stop prevents the call if it hasn't happened yet, and reports whether
	// it did.
	Stop() bool
}

// Real returns the system clock.
func Real() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }

// Fake is a clock that only moves when told to. Its timers run synchronously,
// in the goroutine that calls Advance, in order of their due time.
type Fake struct {
	mu     sync.Mutex
	now    time.Time
	seq    int
	timers []*fakeTimer
}

type fakeTimer struct {
	c    *Fake
	due  time.Time
	seq  int
	f    func()
	done bool
}

// NewFake returns a fake clock set to start.
func NewFake(start time.Time) *Fake { return &Fake{now: start} }

// Now returns the fake time.
func (c *Fake) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// AfterFunc schedules f to run when the clock has advanced by d.
func (c *Fake) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	t := &fakeTimer{c: c, due: c.now.Add(d), seq: c.seq, f: f}
	c.timers = append(c.timers, t)
	return t
}

func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	if t.done {
		return false
	}
	t.done = true
	return true
}

// Advance moves the clock forward by d, running every timer that falls due,
// including timers scheduled by the timers it runs.
func (c *Fake) Advance(d time.Duration) {
	c.mu.Lock()
	end := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		next := c.nextDue(end)
		if next == nil {
			c.now = end
			c.mu.Unlock()
			return
		}
		next.done = true
		if next.due.After(c.now) {
			c.now = next.due
		}
		c.mu.Unlock()
		next.f()
	}
}

// nextDue returns the earliest pending timer due by end. c.mu must be held.
func (c *Fake) nextDue(end time.Time) *fakeTimer {
	live := c.timers[:0]
	for _, t := range c.timers {
		if !t.done {
			live = append(live, t)
		}
	}
	c.timers = live
	sort.Slice(c.timers, func(i, j int) bool {
		if c.timers[i].due.Equal(c.timers[j].due) {
			return c.timers[i].seq < c.timers[j].seq
		}
		return c.timers[i].due.Before(c.timers[j].due)
	})
	if len(c.timers) == 0 || c.timers[0].due.After(end) {
		return nil
	}
	return c.timers[0]
}

// Pending returns how many timers haven't run or been stopped.
func (c *Fake) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if !t.done {
			n++
		}
	}
	return n
}
