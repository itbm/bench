package clock

import (
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

var start = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func TestFakeRunsTimersInDueOrder(t *testing.T) {
	c := NewFake(start)
	var got []string
	c.AfterFunc(3*time.Second, func() { got = append(got, "c") })
	c.AfterFunc(time.Second, func() { got = append(got, "a") })
	c.AfterFunc(2*time.Second, func() { got = append(got, "b") })
	c.AfterFunc(2*time.Second, func() { got = append(got, "b2") }) // Same time: scheduling order.

	c.Advance(2 * time.Second)
	if want := []string{"a", "b", "b2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after 2s ran %v, want %v", got, want)
	}
	if n := c.Pending(); n != 1 {
		t.Fatalf("Pending() = %d, want 1", n)
	}
	c.Advance(time.Second)
	if want := []string{"a", "b", "b2", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after 3s ran %v, want %v", got, want)
	}
}

func TestFakeNowDuringTimer(t *testing.T) {
	c := NewFake(start)
	var at time.Time
	c.AfterFunc(1500*time.Millisecond, func() { at = c.Now() })
	c.Advance(5 * time.Second)
	if want := start.Add(1500 * time.Millisecond); !at.Equal(want) {
		t.Fatalf("Now() inside timer = %v, want %v", at, want)
	}
	if want := start.Add(5 * time.Second); !c.Now().Equal(want) {
		t.Fatalf("Now() after Advance = %v, want %v", c.Now(), want)
	}
}

func TestFakeRunsTimersScheduledByTimers(t *testing.T) {
	c := NewFake(start)
	var ticks int
	var tick func()
	tick = func() {
		ticks++
		c.AfterFunc(250*time.Millisecond, tick)
	}
	c.AfterFunc(250*time.Millisecond, tick)
	c.Advance(time.Second)
	if ticks != 4 {
		t.Fatalf("ticks = %d, want 4", ticks)
	}
	if n := c.Pending(); n != 1 {
		t.Fatalf("Pending() = %d, want 1", n)
	}
}

func TestFakeStop(t *testing.T) {
	c := NewFake(start)
	ran := false
	tm := c.AfterFunc(time.Second, func() { ran = true })
	if !tm.Stop() {
		t.Fatal("Stop() = false for a pending timer")
	}
	if tm.Stop() {
		t.Fatal("Stop() = true for a stopped timer")
	}
	c.Advance(2 * time.Second)
	if ran {
		t.Fatal("a stopped timer ran")
	}

	done := c.AfterFunc(time.Second, func() {})
	c.Advance(time.Second)
	if done.Stop() {
		t.Fatal("Stop() = true for a timer that has run")
	}
}

func TestRealAfterFunc(t *testing.T) {
	var fired atomic.Bool
	ch := make(chan struct{})
	Real().AfterFunc(time.Millisecond, func() { fired.Store(true); close(ch) })
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("the real clock's timer didn't run")
	}
	if !fired.Load() {
		t.Fatal("timer didn't run")
	}
}
