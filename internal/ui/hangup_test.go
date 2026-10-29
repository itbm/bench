//go:build server && unix

package ui

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestQuitOnSignalsKeepsNohup checks that a Bench started with SIGHUP
// ignored, as nohup starts it, keeps running when its terminal closes.
// Listening for a signal stops it being ignored, so quitOnSignals mustn't
// listen then.
func TestQuitOnSignalsKeepsNohup(t *testing.T) {
	if !signal.Ignored(syscall.SIGHUP) {
		signal.Ignore(syscall.SIGHUP)
		t.Cleanup(func() {
			// signal.Reset leaves a signal ignored; listening for it, then
			// stopping, puts back Go's own handling, which ends the process.
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGHUP)
			signal.Stop(c)
			if signal.Ignored(syscall.SIGHUP) {
				t.Error("SIGHUP is still ignored after the test")
			}
		})
	}
	stop := quitOnSignals(nil, syscall.SIGHUP) // Nothing should quit.
	defer stop()
	if !signal.Ignored(syscall.SIGHUP) {
		t.Error("quitOnSignals stopped SIGHUP being ignored: Bench started with nohup would quit when its terminal closes")
	}
}

// TestQuitOnSignalsQuitsOnce checks that the first signal quits, and that
// later ones neither quit again nor end Bench before it has put every
// window back. SIGUSR1 stands in for the signals Bench quits on, as its
// default action would end the test too.
func TestQuitOnSignalsQuitsOnce(t *testing.T) {
	var quits atomic.Int32
	stop := quitOnSignals(func() { quits.Add(1) }, syscall.SIGUSR1)
	defer stop()
	for range 3 {
		if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for quits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // Time for any later quit.
	if n := quits.Load(); n != 1 {
		t.Errorf("quit %d times for three signals, want once", n)
	}
}

// TestQuitSignals checks that the preview quits without asking on SIGHUP,
// and leaves SIGINT and SIGTERM to Wails, which never asks in the preview.
func TestQuitSignals(t *testing.T) {
	got := quitSignals()
	if len(got) != 1 || got[0] != syscall.SIGHUP {
		t.Errorf("quitSignals() = %v, want only SIGHUP in the preview", got)
	}
}
