//go:build darwin || server

package ui

import (
	"slices"
	"testing"
)

// orderEngine is a fake engine that records when it's closed.
type orderEngine struct {
	*fakeEngine
	order *[]string
}

func (e orderEngine) Close() error {
	*e.order = append(*e.order, "engine")
	return e.fakeEngine.Close()
}

// closerFunc is an io.Closer.
type closerFunc func() error

func (f closerFunc) Close() error { return f() }

// TestShutdownLetsTheLockGoBeforeTheWatchdog checks the order of quitting:
// the engine puts every window back while Bench holds the support folder's
// lock, then the lock goes, so that the watchdog, which takes it before it
// looks at parked.json, can finish while quitting waits for it.
func TestShutdownLetsTheLockGoBeforeTheWatchdog(t *testing.T) {
	var order []string
	r := &runner{
		engine:   orderEngine{newFakeEngine(), &order},
		watchdog: closerFunc(func() error { order = append(order, "watchdog"); return nil }),
		unlock:   func() { order = append(order, "unlock") },
	}
	r.shutdown()
	r.shutdown()
	if want := []string{"engine", "unlock", "watchdog"}; !slices.Equal(order, want) {
		t.Errorf("quitting did %v, want %v", order, want)
	}
}
