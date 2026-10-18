//go:build darwin || server

package ui

import (
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
