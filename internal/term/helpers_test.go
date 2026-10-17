package term

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder collects what callbacks report, for tests.
type recorder struct {
	mu        sync.Mutex
	chunks    map[string][][]byte
	titles    map[string][]string
	exits     map[string][]error
	afterExit map[string]bool // Data or Title arrived after Exit
}

func newRecorder() *recorder {
	return &recorder{
		chunks:    map[string][][]byte{},
		titles:    map[string][]string{},
		exits:     map[string][]error{},
		afterExit: map[string]bool{},
	}
}

func (r *recorder) callbacks() Callbacks {
	return Callbacks{
		Data: func(id string, data []byte) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if len(r.exits[id]) > 0 {
				r.afterExit[id] = true
			}
			r.chunks[id] = append(r.chunks[id], data)
		},
		Title: func(id, title string) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if len(r.exits[id]) > 0 {
				r.afterExit[id] = true
			}
			r.titles[id] = append(r.titles[id], title)
		},
		Exit: func(id string, err error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.exits[id] = append(r.exits[id], err)
		},
	}
}

// output returns everything a session has printed so far.
func (r *recorder) output(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(bytes.Join(r.chunks[id], nil))
}

func (r *recorder) titleList(id string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.titles[id]...)
}

func (r *recorder) exitList(id string) []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]error(nil), r.exits[id]...)
}

func (r *recorder) exitCount(id string) int {
	return len(r.exitList(id))
}

// checkTidy fails the test if a session reported Exit more than once, or
// reported anything after Exit.
func (r *recorder) checkTidy(t *testing.T, ids ...string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		if n := len(r.exits[id]); n > 1 {
			t.Errorf("session %s: Exit called %d times, want at most once", id, n)
		}
		if r.afterExit[id] {
			t.Errorf("session %s: a callback arrived after Exit", id)
		}
	}
}

// waitFor polls cond until it's true, or fails the test after timeout.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitOutput waits until a session's output contains want.
func waitOutput(t *testing.T, r *recorder, id, want string, timeout time.Duration) {
	t.Helper()
	waitFor(t, timeout, "output containing "+quote(want), func() bool {
		return strings.Contains(r.output(id), want)
	})
}

func quote(s string) string { return "\"" + s + "\"" }
