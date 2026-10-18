package mac

import (
	"sync"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/platform"
)

// EventBuffer is the capacity of the Platform's Events channel.
const EventBuffer = 4096

// overflowLimit bounds how many events wait while the Events channel is
// full. Past it, new events are dropped.
const overflowLimit = 65536

// benchMovedInterval is the least time between two BenchMoved events: about
// one frame.
const benchMovedInterval = 16 * time.Millisecond

// eventQueue hands events from native callbacks to the Events channel
// without ever blocking the caller, so that callbacks on the main thread
// can't stall AppKit.
//
// An event goes straight onto the channel when there's room. When the
// channel is full, it waits in an overflow list that a goroutine feeds into
// the channel in order. Moves and resizes (and BenchMoved) are coalesced
// there: a newer one replaces a waiting one for the same window, since only
// the latest frame matters.
//
// Its methods are safe for concurrent use. Its lock is held only for
// moments, never while waiting.
type eventQueue struct {
	out   chan platform.Event
	limit int
	wake  chan struct{}
	done  chan struct{}

	mu       sync.Mutex
	overflow []platform.Event
	sending  bool // The feeding goroutine holds an event from overflow.
	closed   bool
	dropped  int
}

// newEventQueue returns a running queue whose channel holds capacity
// events, with up to limit more waiting in its overflow list.
func newEventQueue(capacity, limit int) *eventQueue {
	q := newIdleEventQueue(capacity, limit)
	q.start()
	return q
}

// newIdleEventQueue returns a queue that doesn't feed its overflow list
// into the channel until start.
func newIdleEventQueue(capacity, limit int) *eventQueue {
	return &eventQueue{
		out:   make(chan platform.Event, capacity),
		limit: limit,
		wake:  make(chan struct{}, 1),
		done:  make(chan struct{}),
	}
}

// start begins feeding the overflow list into the channel.
func (q *eventQueue) start() { go q.feed() }

// C returns the channel events arrive on. It's never closed.
func (q *eventQueue) C() <-chan platform.Event { return q.out }

// push queues ev without blocking.
func (q *eventQueue) push(ev platform.Event) {
	q.mu.Lock()
	if q.closed {
		q.dropped++
		q.mu.Unlock()
		return
	}
	if len(q.overflow) == 0 && !q.sending {
		select {
		case q.out <- ev:
			q.mu.Unlock()
			return
		default:
		}
	}
	added := q.addLocked(ev)
	q.mu.Unlock()
	if added {
		select {
		case q.wake <- struct{}{}:
		default:
		}
	}
}

// addLocked puts ev in the overflow list, coalescing it with a waiting
// event of the same kind for the same window if it can be. It reports
// whether the list grew. q.mu must be held.
func (q *eventQueue) addLocked(ev platform.Event) bool {
	if coalescible(ev.Kind) {
		for i := len(q.overflow) - 1; i >= 0; i-- {
			if q.overflow[i].Kind == ev.Kind && q.overflow[i].Window.ID == ev.Window.ID {
				q.overflow[i] = ev
				q.dropped++
				return false
			}
		}
	}
	if len(q.overflow) >= q.limit {
		q.dropped++
		return false
	}
	q.overflow = append(q.overflow, ev)
	return true
}

// feed moves events from the overflow list to the channel, waiting for
// room, until the queue is closed.
func (q *eventQueue) feed() {
	for {
		select {
		case <-q.wake:
		case <-q.done:
			return
		}
		for {
			ev, ok := q.next()
			if !ok {
				break
			}
			select {
			case q.out <- ev:
			case <-q.done:
				return
			}
		}
	}
}

// next takes the oldest waiting event, marking it as being sent, or reports
// false when none is waiting.
func (q *eventQueue) next() (platform.Event, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.overflow) == 0 {
		q.overflow = nil
		q.sending = false
		return platform.Event{}, false
	}
	ev := q.overflow[0]
	q.overflow[0] = platform.Event{}
	q.overflow = q.overflow[1:]
	q.sending = true
	return ev, true
}

// Dropped returns how many events were dropped or replaced by newer ones.
func (q *eventQueue) Dropped() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.dropped
}

// close stops the feeding goroutine and drops later events. The channel
// stays open. It's safe to call more than once.
func (q *eventQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	close(q.done)
}

// coalescible reports whether only the latest event of a kind matters, so
// that older ones can be dropped when the queue is full.
func coalescible(kind platform.EventKind) bool {
	switch kind {
	case platform.WindowMoved, platform.WindowResized, platform.BenchMoved:
		return true
	}
	return false
}

// throttle calls a function at most once per interval: at once if it
// hasn't been called for an interval, and otherwise once more at the end of
// the interval, however many times it was triggered in between. The last
// trigger is never lost.
//
// Its methods are safe for concurrent use and never block: trigger is
// called from native callbacks.
type throttle struct {
	clk   clock.Clock
	every time.Duration
	fire  func()

	mu      sync.Mutex
	fired   bool
	last    time.Time
	pending bool
	stopped bool
	timer   clock.Timer
}

// newThrottle returns a throttle that calls fire at most once per every. A
// nil clk means clock.Real(). fire must not block.
func newThrottle(clk clock.Clock, every time.Duration, fire func()) *throttle {
	if clk == nil {
		clk = clock.Real()
	}
	return &throttle{clk: clk, every: every, fire: fire}
}

// trigger asks for a call.
func (t *throttle) trigger() {
	t.mu.Lock()
	if t.stopped || t.pending {
		t.mu.Unlock()
		return
	}
	now := t.clk.Now()
	if wait := t.every - now.Sub(t.last); t.fired && wait > 0 {
		t.pending = true
		t.timer = t.clk.AfterFunc(wait, t.flush)
		t.mu.Unlock()
		return
	}
	t.fired, t.last = true, now
	t.mu.Unlock()
	t.fire()
}

// flush makes the call that a trigger during the interval asked for.
func (t *throttle) flush() {
	t.mu.Lock()
	if t.stopped || !t.pending {
		t.mu.Unlock()
		return
	}
	t.pending = false
	t.fired, t.last = true, t.clk.Now()
	t.mu.Unlock()
	t.fire()
}

// stop cancels any pending call and ignores later triggers.
func (t *throttle) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
	if t.timer != nil {
		t.timer.Stop()
	}
}
