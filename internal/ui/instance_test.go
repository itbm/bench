//go:build darwin || server

package ui

import (
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TestSecondInstanceDoesNotWait checks that Wails' OnSecondInstanceLaunch
// returns while Bench is still coming forward. Wails calls it from the one
// reader of a one-slot channel that the main thread fills, and coming
// forward waits for the main thread, so waiting here would hang Bench once
// three Benches start close together.
func TestSecondInstanceDoesNotWait(t *testing.T) {
	r := &runner{}
	r.secondInstance(application.SecondInstanceData{}) // Before the shell exists.

	block := make(chan struct{})
	var calls atomic.Int32
	forward := func() {
		calls.Add(1)
		<-block // The main thread is busy.
	}
	r.forward.Store(&forward)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 3 {
			r.secondInstance(application.SecondInstanceData{Args: []string{"bench"}})
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnSecondInstanceLaunch waited for Bench to come forward")
	}
	close(block)
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() != 3 {
		if time.Now().After(deadline) {
			t.Fatalf("brought Bench forward %d times, want 3", calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSecondInstanceRoutesLinksAndScripts checks what another Bench's
// arguments do: a bench:// link goes to the engine's FollowLink, a script
// subcommand to its RunScript, which checks allow_scripts, and anything
// else brings Bench forward, as opening Bench again does.
func TestSecondInstanceRoutesLinksAndScripts(t *testing.T) {
	f := newFakeEngine()
	r := &runner{}
	var forwards atomic.Int32
	forward := func() { forwards.Add(1) }
	r.forward.Store(&forward)
	// Before the engine exists, links and scripts are dropped, and don't
	// bring Bench forward either.
	r.secondInstance(application.SecondInstanceData{Args: []string{"/Applications/Bench.app/Contents/MacOS/Bench", "next"}})
	r.followLink("bench://next")
	r.scripts.Store(&scriptTarget{followLink: f.FollowLink, runScript: f.RunScript})

	for _, args := range [][]string{
		{"Bench", "bench://tab/Code"},
		{"Bench", "-psn_0_12345", "BENCH://next"},
		{"bench", "tab", "--", "Code"},
		{"bench", "move-to-tab", "--", "Mail"},
		{"bench", "launch", "--", "Claude"},
		{"bench", "last"},
		{"bench"},
		{"bench", "-psn_0_1"},
		{},
		{"bench", "bench://next", "bench://previous"},
		{"bench", "jump"},
		{"bench", "version"},
	} {
		r.secondInstance(application.SecondInstanceData{Args: args})
	}
	r.followLink("bench://overview")

	want := []call{
		{Method: "FollowLink", Args: []any{"bench://tab/Code"}},
		{Method: "FollowLink", Args: []any{"BENCH://next"}},
		{Method: "RunScript", Args: []any{[]string{"tab", "--", "Code"}}},
		{Method: "RunScript", Args: []any{[]string{"move-to-tab", "--", "Mail"}}},
		{Method: "RunScript", Args: []any{[]string{"launch", "--", "Claude"}}},
		{Method: "RunScript", Args: []any{[]string{"last"}}},
		{Method: "FollowLink", Args: []any{"bench://overview"}},
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(f.Calls()) < len(want) || forwards.Load() < 6 {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := f.Calls()
	if len(got) != len(want) {
		t.Fatalf("calls %+v, want %+v", got, want)
	}
	// The calls run on goroutines of their own, so their order isn't
	// fixed.
	for _, w := range want {
		found := false
		for _, g := range got {
			if reflect.DeepEqual(g, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("no call %+v in %+v", w, got)
		}
	}
	if n := forwards.Load(); n != 6 {
		t.Errorf("brought Bench forward %d times, want 6: for no arguments, the -psn_ one alone, none at all, two links, jump, and version", n)
	}
}
