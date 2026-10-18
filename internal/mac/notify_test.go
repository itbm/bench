package mac

import (
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
)

func TestNotifyLimiter(t *testing.T) {
	clk := clock.NewFake(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	l := newNotifyLimiter(clk)
	for i := range notifyBurst {
		if !l.allow("w1") {
			t.Fatalf("notification %d of a burst was refused", i+1)
		}
	}
	if l.allow("w1") {
		t.Errorf("a page posting in a loop should be held back after a burst")
	}
	// Another item has a budget of its own.
	if !l.allow("w2") {
		t.Errorf("w2 was refused because of w1")
	}
	clk.Advance(notifyEvery - time.Millisecond)
	if l.allow("w1") {
		t.Errorf("allowed again too soon")
	}
	clk.Advance(time.Millisecond)
	if !l.allow("w1") {
		t.Errorf("one more should be allowed after %v", notifyEvery)
	}
	if l.allow("w1") {
		t.Errorf("only one more")
	}
	// After a quiet spell the whole burst is available again, and no more.
	clk.Advance(time.Hour)
	for i := range notifyBurst {
		if !l.allow("w1") {
			t.Fatalf("after a quiet hour, notification %d was refused", i+1)
		}
	}
	if l.allow("w1") {
		t.Errorf("the budget shouldn't grow past a burst")
	}
}
