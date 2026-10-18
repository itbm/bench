//go:build server && unix

package ui

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
)

// TestQuitOnHangupKeepsNohup checks that a Bench started with SIGHUP
// ignored, as nohup starts it, keeps running when its terminal closes.
// Listening for a signal stops it being ignored, so quitOnHangup mustn't
// listen then.
func TestQuitOnHangupKeepsNohup(t *testing.T) {
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
	stop := quitOnHangup(nil) // Nothing should quit, so no app is needed.
	defer stop()
	if !signal.Ignored(syscall.SIGHUP) {
		t.Error("quitOnHangup stopped SIGHUP being ignored: Bench started with nohup would quit when its terminal closes")
	}
}
