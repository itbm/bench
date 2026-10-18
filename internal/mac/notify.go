package mac

import (
	"sync"
	"time"

	"github.com/itbm/bench/internal/clock"
)

// A Web item's page may post notifyBurst notifications at once, and then one
// every notifyEvery, so that a page posting in a loop can't flood
// Notification Centre (with a sound each time).
const (
	notifyBurst = 5
	notifyEvery = 6 * time.Second
)

// notifyLimiter limits how often each Web item posts notifications. Its
// methods are safe for concurrent use, and never block for longer than a
// moment.
type notifyLimiter struct {
	clk clock.Clock

	mu    sync.Mutex
	items map[string]*notifyBudget
}

// notifyBudget is a token bucket: tokens notifications are available, as of
// at.
type notifyBudget struct {
	tokens float64
	at     time.Time
}

// newNotifyLimiter returns a limiter. A nil clk means clock.Real().
func newNotifyLimiter(clk clock.Clock) *notifyLimiter {
	if clk == nil {
		clk = clock.Real()
	}
	return &notifyLimiter{clk: clk, items: map[string]*notifyBudget{}}
}

// allow reports whether an item may post a notification now, and if so
// counts it.
func (l *notifyLimiter) allow(itemID string) bool {
	now := l.clk.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.items[itemID]
	if b == nil {
		b = &notifyBudget{tokens: notifyBurst, at: now}
		l.items[itemID] = b
	}
	b.tokens = min(notifyBurst, b.tokens+float64(now.Sub(b.at))/float64(notifyEvery))
	b.at = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
