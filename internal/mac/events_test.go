package mac

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func windowEvent(kind platform.EventKind, id platform.WindowID, x float64) platform.Event {
	return platform.Event{Kind: kind, Window: platform.Window{ID: id, Frame: layout.Rect{X: x}}}
}

// receive reads n events, failing the test if they don't arrive in time.
func receive(t *testing.T, q *eventQueue, n int) []platform.Event {
	t.Helper()
	var got []platform.Event
	timeout := time.After(5 * time.Second)
	for len(got) < n {
		select {
		case ev := <-q.C():
			got = append(got, ev)
		case <-timeout:
			t.Fatalf("received %d events, want %d", len(got), n)
		}
	}
	return got
}

// expectNone fails the test if another event arrives soon.
func expectNone(t *testing.T, q *eventQueue) {
	t.Helper()
	select {
	case ev := <-q.C():
		t.Fatalf("unexpected event %+v", ev)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEventQueueDeliversInOrder(t *testing.T) {
	q := newEventQueue(2, 100)
	defer q.close()
	kinds := []platform.EventKind{
		platform.WindowCreated, platform.WindowFocused, platform.WindowRetitled,
		platform.AppLaunched, platform.WindowDestroyed, platform.AppTerminated,
	}
	for i, k := range kinds {
		q.push(windowEvent(k, platform.WindowID(i+1), 0))
	}
	got := receive(t, q, len(kinds))
	for i, ev := range got {
		if ev.Kind != kinds[i] || ev.Window.ID != platform.WindowID(i+1) {
			t.Errorf("event %d = %s of %d, want %s of %d", i, ev.Kind, ev.Window.ID, kinds[i], i+1)
		}
	}
	expectNone(t, q)
	if d := q.Dropped(); d != 0 {
		t.Errorf("Dropped = %d, want 0", d)
	}
}

func TestEventQueueCoalescesMovesWhenFull(t *testing.T) {
	// The queue starts feeding only once everything is queued, so that
	// what waits in the overflow list is certain.
	q := newIdleEventQueue(1, 100)
	defer q.close()

	// The channel takes the first event; the rest wait.
	q.push(windowEvent(platform.WindowCreated, 1, 0))
	q.push(windowEvent(platform.WindowMoved, 1, 10))
	q.push(windowEvent(platform.WindowMoved, 2, 20))
	q.push(windowEvent(platform.WindowResized, 1, 30)) // Replaces the move of window 1 at 10.
	q.push(windowEvent(platform.WindowMoved, 1, 40))   // Replaces the resize.
	q.push(platform.Event{Kind: platform.BenchMoved})
	q.push(platform.Event{Kind: platform.BenchMoved}) // Replaces the earlier one.
	q.push(windowEvent(platform.WindowRetitled, 1, 0))
	q.start()

	type summary struct {
		kind platform.EventKind
		id   platform.WindowID
		x    float64
	}
	want := []summary{
		{platform.WindowCreated, 1, 0},
		{platform.WindowMoved, 1, 40},
		{platform.WindowMoved, 2, 20},
		{platform.BenchMoved, 0, 0},
		{platform.WindowRetitled, 1, 0},
	}
	got := receive(t, q, len(want))
	for i, ev := range got {
		if s := (summary{ev.Kind, ev.Window.ID, ev.Window.Frame.X}); s != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, s, want[i])
		}
	}
	expectNone(t, q)
	if d := q.Dropped(); d != 3 {
		t.Errorf("Dropped = %d, want 3", d)
	}
}

func TestEventQueueCoalescingKeepsTheUsersDrag(t *testing.T) {
	q := newIdleEventQueue(1, 100)
	defer q.close()
	drag := windowEvent(platform.WindowMoved, 1, 10)
	drag.MouseDown = true
	q.push(windowEvent(platform.WindowCreated, 9, 0)) // Fills the channel.
	q.push(drag)
	q.push(windowEvent(platform.WindowMoved, 1, 20)) // The app's own move replaces the drag.
	q.start()
	got := receive(t, q, 2)
	if ev := got[1]; ev.Kind != platform.WindowMoved || ev.Window.Frame.X != 20 || !ev.MouseDown {
		t.Errorf("coalesced move = %s at %v, MouseDown %v; want the newer frame with MouseDown", ev.Kind, ev.Window.Frame.X, ev.MouseDown)
	}
	expectNone(t, q)
}

func TestEventQueueCoalescingKeepsOrderForAWindow(t *testing.T) {
	q := newIdleEventQueue(1, 100)
	defer q.close()
	q.push(windowEvent(platform.WindowCreated, 9, 0)) // Fills the channel.
	q.push(windowEvent(platform.WindowMoved, 1, 10))
	q.push(windowEvent(platform.WindowMinimised, 1, 0))
	q.push(windowEvent(platform.WindowMoved, 1, 20)) // Mustn't jump ahead of the minimise.
	q.push(windowEvent(platform.WindowMoved, 2, 30))
	q.push(windowEvent(platform.WindowFocused, 2, 0))
	q.push(windowEvent(platform.WindowMoved, 2, 40))
	q.push(windowEvent(platform.WindowMoved, 1, 50)) // Replaces the move at 20.
	q.start()

	type summary struct {
		kind platform.EventKind
		id   platform.WindowID
		x    float64
	}
	want := []summary{
		{platform.WindowCreated, 9, 0},
		{platform.WindowMoved, 1, 10},
		{platform.WindowMinimised, 1, 0},
		{platform.WindowMoved, 1, 50},
		{platform.WindowMoved, 2, 30},
		{platform.WindowFocused, 2, 0},
		{platform.WindowMoved, 2, 40},
	}
	got := receive(t, q, len(want))
	for i, ev := range got {
		if s := (summary{ev.Kind, ev.Window.ID, ev.Window.Frame.X}); s != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, s, want[i])
		}
	}
	expectNone(t, q)
}

func TestEventQueueCoalescesMovesAndResizesTogether(t *testing.T) {
	q := newIdleEventQueue(1, 100)
	defer q.close()
	q.push(windowEvent(platform.WindowCreated, 9, 0)) // Fills the channel.
	// A drag on the window's left edge moves and resizes it in turn.
	q.push(windowEvent(platform.WindowMoved, 1, 10))
	q.push(windowEvent(platform.WindowResized, 1, 20))
	q.push(windowEvent(platform.WindowMoved, 2, 5))
	q.push(windowEvent(platform.WindowMoved, 1, 30)) // Mustn't come before the frame at 20.
	q.start()
	got := receive(t, q, 3)
	if ev := got[1]; ev.Window.ID != 1 || ev.Window.Frame.X != 30 {
		t.Errorf("event 1 = %s of window %d at %v, want window 1's latest frame, at 30", ev.Kind, ev.Window.ID, ev.Window.Frame.X)
	}
	if ev := got[2]; ev.Window.ID != 2 {
		t.Errorf("event 2 = %s of window %d, want window 2's move", ev.Kind, ev.Window.ID)
	}
	expectNone(t, q)
}

func TestEventQueueCoalescesWebTitlesWhenFull(t *testing.T) {
	q := newIdleEventQueue(1, 100)
	defer q.close()
	web := func(kind platform.EventKind, item, text string) platform.Event {
		ev := platform.Event{Kind: kind, ItemID: item}
		if kind == platform.WebURL {
			ev.Body = text
		} else {
			ev.Title = text
		}
		return ev
	}
	q.push(windowEvent(platform.WindowCreated, 9, 0)) // Fills the channel.
	// A page retitles itself, and navigates, in a tight loop.
	for i := range 1000 {
		q.push(web(platform.WebTitle, "w1", fmt.Sprint("title ", i)))
		q.push(web(platform.WebURL, "w1", fmt.Sprint("https://example.com/", i)))
	}
	q.push(web(platform.WebTitle, "w2", "Other"))
	q.push(web(platform.WebNotification, "w1", "Hello"))
	q.push(web(platform.WebTitle, "w1", "After")) // Mustn't jump ahead of the notification.
	q.push(windowEvent(platform.WindowCreated, 10, 0))
	q.start()

	type summary struct {
		kind       platform.EventKind
		item, text string
	}
	want := []summary{
		{platform.WindowCreated, "", ""},
		{platform.WebTitle, "w1", "title 999"},
		{platform.WebURL, "w1", "https://example.com/999"},
		{platform.WebTitle, "w2", "Other"},
		{platform.WebNotification, "w1", "Hello"},
		{platform.WebTitle, "w1", "After"},
		{platform.WindowCreated, "", ""},
	}
	got := receive(t, q, len(want))
	for i, ev := range got {
		s := summary{ev.Kind, ev.ItemID, ev.Title + ev.Body}
		if s != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, s, want[i])
		}
	}
	expectNone(t, q)
}

func TestEventQueueKeepsImportantEventsWhenFull(t *testing.T) {
	q := newEventQueue(4, 1000)
	defer q.close()
	const n = 500
	for i := range n {
		q.push(windowEvent(platform.WindowCreated, platform.WindowID(i+1), 0))
	}
	got := receive(t, q, n)
	for i, ev := range got {
		if ev.Window.ID != platform.WindowID(i+1) {
			t.Fatalf("event %d is for window %d, want %d", i, ev.Window.ID, i+1)
		}
	}
}

func TestEventQueueLimit(t *testing.T) {
	q := newIdleEventQueue(1, 2)
	defer q.close()
	for i := range 5 {
		q.push(windowEvent(platform.WindowCreated, platform.WindowID(i+1), 0))
	}
	q.start()
	got := receive(t, q, 3)
	for i, ev := range got {
		if ev.Window.ID != platform.WindowID(i+1) {
			t.Errorf("event %d is for window %d, want %d", i, ev.Window.ID, i+1)
		}
	}
	expectNone(t, q)
	if d := q.Dropped(); d != 2 {
		t.Errorf("Dropped = %d, want 2", d)
	}
}

func TestEventQueueNeverBlocks(t *testing.T) {
	q := newEventQueue(1, 10)
	defer q.close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 10000 {
			q.push(windowEvent(platform.WindowMoved, platform.WindowID(i%3), float64(i)))
			q.push(windowEvent(platform.WindowCreated, platform.WindowID(i), 0))
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("push blocked with nobody reading")
	}
}

func TestEventQueueConcurrentPushes(t *testing.T) {
	q := newEventQueue(8, 100000)
	defer q.close()
	const writers, each = 8, 200
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				q.push(windowEvent(platform.WindowCreated, platform.WindowID(w*each+i+1), 0))
			}
		}()
	}
	got := receive(t, q, writers*each)
	wg.Wait()
	seen := map[platform.WindowID]bool{}
	for _, ev := range got {
		if seen[ev.Window.ID] {
			t.Fatalf("window %d delivered twice", ev.Window.ID)
		}
		seen[ev.Window.ID] = true
	}
}

func TestEventQueueClose(t *testing.T) {
	q := newEventQueue(1, 10)
	q.push(windowEvent(platform.WindowCreated, 1, 0))
	q.close()
	q.close()
	q.push(windowEvent(platform.WindowCreated, 2, 0))
	if got := receive(t, q, 1); got[0].Window.ID != 1 {
		t.Errorf("got window %d, want 1", got[0].Window.ID)
	}
	expectNone(t, q)
}

func TestCoalescible(t *testing.T) {
	tests := []struct {
		kind platform.EventKind
		want bool
	}{
		{platform.WindowMoved, true},
		{platform.WindowResized, true},
		{platform.BenchMoved, true},
		{platform.WindowCreated, false},
		{platform.WindowDestroyed, false},
		{platform.WindowRetitled, false},
		{platform.ScreensChanged, false},
		{platform.WebTitle, true},
		{platform.WebURL, true},
		{platform.WebNotification, false},
		{platform.WebFocused, false},
	}
	for _, tt := range tests {
		if got := coalescible(tt.kind); got != tt.want {
			t.Errorf("coalescible(%s) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

// counter counts calls, safely across goroutines.
type counter struct {
	mu sync.Mutex
	n  int
}

func (c *counter) inc() { c.mu.Lock(); c.n++; c.mu.Unlock() }

func (c *counter) get() int { c.mu.Lock(); defer c.mu.Unlock(); return c.n }

func TestThrottle(t *testing.T) {
	type step struct {
		advance time.Duration
		trigger int // How many triggers after advancing.
		want    int // Total calls afterwards.
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"first call is immediate", []step{{0, 1, 1}}},
		{"a second call in the interval waits for its end", []step{
			{0, 1, 1},
			{5 * time.Millisecond, 1, 1},
			{10 * time.Millisecond, 0, 1},
			{time.Millisecond, 0, 2},
		}},
		{"many calls in the interval make one more call", []step{
			{0, 1, 1},
			{time.Millisecond, 50, 1},
			{20 * time.Millisecond, 0, 2},
			{time.Second, 0, 2},
		}},
		{"after a quiet interval the call is immediate again", []step{
			{0, 1, 1},
			{16 * time.Millisecond, 1, 2},
			{100 * time.Millisecond, 1, 3},
		}},
		{"a steady stream gives one call per interval", []step{
			{0, 1, 1},
			{4 * time.Millisecond, 1, 1},
			{4 * time.Millisecond, 1, 1},
			{4 * time.Millisecond, 1, 1},
			{4 * time.Millisecond, 1, 2},
			{4 * time.Millisecond, 1, 2},
			{12 * time.Millisecond, 0, 3},
			{time.Second, 0, 3},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clk := clock.NewFake(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
			var calls counter
			th := newThrottle(clk, benchMovedInterval, calls.inc)
			for i, s := range tt.steps {
				clk.Advance(s.advance)
				for range s.trigger {
					th.trigger()
				}
				if got := calls.get(); got != s.want {
					t.Fatalf("after step %d: %d calls, want %d", i, got, s.want)
				}
			}
		})
	}
}

func TestThrottleStop(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	var calls counter
	th := newThrottle(clk, benchMovedInterval, calls.inc)
	th.trigger()
	th.trigger()
	th.stop()
	clk.Advance(time.Second)
	th.trigger()
	if got := calls.get(); got != 1 {
		t.Errorf("%d calls, want 1", got)
	}
	if n := clk.Pending(); n != 0 {
		t.Errorf("%d timers pending, want 0", n)
	}
}

func TestThrottleRealClock(t *testing.T) {
	fired := make(chan struct{}, 10)
	th := newThrottle(nil, 5*time.Millisecond, func() { fired <- struct{}{} })
	defer th.stop()
	th.trigger()
	th.trigger()
	for i := range 2 {
		select {
		case <-fired:
		case <-time.After(5 * time.Second):
			t.Fatalf("call %d didn't happen", i+1)
		}
	}
}
