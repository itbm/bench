//go:build darwin || server

package ui

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
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
// subcommand, stamped as Forward stamps it, to its RunScript, which checks
// allow_scripts, and anything else brings Bench forward, as opening Bench
// again does.
func TestSecondInstanceRoutesLinksAndScripts(t *testing.T) {
	f := newFakeEngine()
	r := &runner{}
	var forwards atomic.Int32
	forward := func() { forwards.Add(1) }
	r.forward.Store(&forward)
	// Before the engine exists, links and scripts are dropped, and don't
	// bring Bench forward either.
	r.secondInstance(application.SecondInstanceData{Args: []string{"/Applications/Bench.app/Contents/MacOS/Bench", "next"},
		AdditionalData: newStamp(time.Now())})
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
		r.secondInstance(application.SecondInstanceData{Args: args, AdditionalData: newStamp(time.Now())})
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

// TestSecondInstanceTakesEachScriptCommandOnce checks that a script
// subcommand is carried out only once, and only soon after Forward sent it,
// as its stamp says: any app can record the notification that carries it,
// encrypted, and post it again. Links and bringing Bench forward need no
// stamp, as any page can open a link or open Bench.
func TestSecondInstanceTakesEachScriptCommandOnce(t *testing.T) {
	f := newFakeEngine()
	r := &runner{}
	var forwards atomic.Int32
	forward := func() { forwards.Add(1) }
	r.forward.Store(&forward)
	r.scripts.Store(&scriptTarget{followLink: f.FollowLink, runScript: f.RunScript})
	stamp := func(sent time.Time, id string) map[string]string {
		return map[string]string{"sent": strconv.FormatInt(sent.UnixNano(), 10), "id": id}
	}
	launch := []string{"bench", "launch", "--", "Deploy"}
	now := time.Now()
	fresh := application.SecondInstanceData{Args: launch, AdditionalData: stamp(now, "a")}
	for _, data := range []application.SecondInstanceData{
		fresh,
		fresh,          // Recorded and posted again.
		{Args: launch}, // No stamp.
		{Args: launch, AdditionalData: map[string]string{"id": "b"}},
		{Args: launch, AdditionalData: map[string]string{"sent": "soon", "id": "c"}},
		{Args: launch, AdditionalData: stamp(now, "")},
		{Args: launch, AdditionalData: stamp(now.Add(-time.Minute), "d")},
		{Args: launch, AdditionalData: stamp(now.Add(time.Minute), "e")},
		{Args: []string{"bench", "next"}, AdditionalData: stamp(now, "f")},
		{Args: []string{"Bench", "bench://tab/Code"}},
		{Args: []string{"Bench", "bench://tab/Code"}},
		{Args: []string{"Bench"}},
	} {
		r.secondInstance(data)
	}
	want := []call{
		{Method: "RunScript", Args: []any{[]string{"launch", "--", "Deploy"}}},
		{Method: "RunScript", Args: []any{[]string{"next"}}},
		{Method: "FollowLink", Args: []any{"bench://tab/Code"}},
		{Method: "FollowLink", Args: []any{"bench://tab/Code"}},
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(f.Calls()) < len(want) || forwards.Load() < 1 {
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Give a call that shouldn't come the time to.
	time.Sleep(50 * time.Millisecond)
	got := f.Calls()
	count := func(c call, in []call) int {
		n := 0
		for _, g := range in {
			if reflect.DeepEqual(g, c) {
				n++
			}
		}
		return n
	}
	if len(got) != len(want) {
		t.Errorf("calls %+v, want %+v", got, want)
	}
	for _, w := range want {
		if count(w, got) != count(w, want) {
			t.Errorf("%v came %d times, want %d: %+v", w, count(w, got), count(w, want), got)
		}
	}
	if n := forwards.Load(); n != 1 {
		t.Errorf("brought Bench forward %d times, want 1", n)
	}
}

// TestStampGuard checks Forward's stamps, which reach the running Bench in
// the JSON that Wails encrypts, and how long the running Bench remembers
// the IDs it has taken: long enough that no copy of a subcommand can be
// fresh once it forgets, even one sent a little ahead of its clock.
func TestStampGuard(t *testing.T) {
	t0 := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	stamp := newStamp(t0)
	if again := newStamp(t0); again[stampID] == stamp[stampID] || len(stamp[stampID]) < 16 {
		t.Errorf("stamps %v and %v: want a new, long ID each time", stamp, again)
	}
	data, err := json.Marshal(application.SecondInstanceData{Args: []string{"bench", "next"}, AdditionalData: stamp})
	if err != nil {
		t.Fatal(err)
	}
	var got application.SecondInstanceData
	if err := json.Unmarshal(data, &got); err != nil || !reflect.DeepEqual(got.AdditionalData, stamp) {
		t.Fatalf("the stamp in Wails' JSON: %s, %v", data, err)
	}

	var g stampGuard
	if err := g.take(got.AdditionalData, t0.Add(time.Second)); err != nil {
		t.Errorf("a fresh stamp: %v", err)
	}
	if err := g.take(got.AdditionalData, t0.Add(2*time.Second)); !errors.Is(err, errStampTaken) {
		t.Errorf("the same stamp again = %v", err)
	}
	if err := g.take(got.AdditionalData, t0.Add(stampFresh+time.Second)); !errors.Is(err, errStampOld) {
		t.Errorf("the same stamp, old = %v", err)
	}

	// A stamp from a little ahead is fresh until stampFresh after it was
	// sent, so its ID is kept that long after it came.
	ahead := newStamp(t0.Add(stampFresh))
	if err := g.take(ahead, t0); err != nil {
		t.Errorf("a stamp from %v ahead: %v", stampFresh, err)
	}
	if err := g.take(ahead, t0.Add(2*stampFresh)); !errors.Is(err, errStampTaken) {
		t.Errorf("the stamp from ahead, again at its last fresh moment = %v", err)
	}
	// Once no copy can be fresh, its ID goes.
	if err := g.take(newStamp(t0.Add(3*stampFresh)), t0.Add(3*stampFresh)); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.seen[ahead[stampID]]; ok || len(g.seen) != 1 {
		t.Errorf("the guard still has %d IDs, want only the latest", len(g.seen))
	}
}
