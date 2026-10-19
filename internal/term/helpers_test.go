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
	next      map[string]int64 // where the next chunk should start
	gaps      map[string]bool  // a chunk's offset didn't follow on from the one before
	titles    map[string][]string
	byShell   map[string][]bool
	exits     map[string][]error
	afterExit map[string]bool // Data or Title arrived after Exit
}

func newRecorder() *recorder {
	return &recorder{
		chunks:    map[string][][]byte{},
		next:      map[string]int64{},
		gaps:      map[string]bool{},
		titles:    map[string][]string{},
		byShell:   map[string][]bool{},
		exits:     map[string][]error{},
		afterExit: map[string]bool{},
	}
}

func (r *recorder) callbacks() Callbacks {
	return Callbacks{
		Data: func(id string, offset int64, data []byte) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if len(r.exits[id]) > 0 {
				r.afterExit[id] = true
			}
			if offset != r.next[id] {
				r.gaps[id] = true
			}
			r.next[id] = offset + int64(len(data))
			r.chunks[id] = append(r.chunks[id], data)
		},
		Title: func(id, title string, byShell bool) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if len(r.exits[id]) > 0 {
				r.afterExit[id] = true
			}
			r.titles[id] = append(r.titles[id], title)
			r.byShell[id] = append(r.byShell[id], byShell)
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

// offset returns where the next chunk of a session's output starts, and
// whether every chunk so far started where the one before ended.
func (r *recorder) offset(id string) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.next[id], !r.gaps[id]
}

// byShellList returns, for each title reported, whether the shell set it.
func (r *recorder) byShellList(id string) []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bool(nil), r.byShell[id]...)
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
