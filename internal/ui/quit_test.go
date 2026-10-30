//go:build darwin || server

package ui

import (
	"io"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// quitShell returns a shell that asks f before quitting, as setUpNative
// sets it up, and counts the quits it makes without asking.
func quitShell(f *fakeEngine) (*appShell, *atomic.Int32) {
	s := newAppShell(nil, nil)
	quits := new(atomic.Int32)
	s.quitApp = func() { quits.Add(1) }
	s.askToQuit = f.AskToQuit
	s.askToQuitNow = f.AskToQuitNow
	s.askToCloseTerminal = f.AskToCloseTerminal
	return s, quits
}

// waitFor waits up to two seconds for cond.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func methods(calls []call) []string {
	var out []string
	for _, c := range calls {
		out = append(out, c.Method)
	}
	return out
}

func TestShouldQuitAsksTheEngine(t *testing.T) {
	// Nothing's running: Bench quits at once.
	f := newFakeEngine()
	f.mayQuit = true
	s, quits := quitShell(f)
	if !s.shouldQuit() {
		t.Error("shouldQuit() = false with nothing running, want true")
	}

	// A terminal is running something: the quit stops, and the engine
	// asks the user, whose Quit quits.
	f = newFakeEngine()
	s, quits = quitShell(f)
	if s.shouldQuit() {
		t.Error("shouldQuit() = true while a terminal is running something, want false")
	}
	if got := methods(f.Calls()); !slices.Equal(got, []string{"AskToQuitNow"}) {
		t.Errorf("engine calls = %v, want only AskToQuitNow", got)
	}
	time.Sleep(20 * time.Millisecond)
	if quits.Load() != 0 {
		t.Error("Bench quit without waiting for the user's answer")
	}

	// The user answers Quit: Bench quits, and the quit goes through.
	s.quit()
	waitFor(t, "the quit", func() bool { return quits.Load() == 1 })
	if !s.shouldQuit() {
		t.Error("shouldQuit() = false after the user's Quit, want true")
	}
	if got := methods(f.Calls()); len(got) != 1 {
		t.Errorf("engine calls = %v; the quit after the user's Quit asked again", got)
	}
}

func TestShouldQuitWhileLoggingOut(t *testing.T) {
	f := newFakeEngine() // A terminal is running something.
	s, _ := quitShell(f)
	s.loggingOut = func() bool { return true }
	if !s.shouldQuit() {
		t.Error("shouldQuit() = false while logging out, want true: it would stop the log out")
	}
	if got := f.Calls(); len(got) != 0 {
		t.Errorf("engine calls = %v while logging out, want none", got)
	}
}

func TestShouldQuitAsBenchQuits(t *testing.T) {
	f := newFakeEngine()
	s, _ := quitShell(f)
	s.quitting.Store(true)
	if !s.shouldQuit() {
		t.Error("shouldQuit() = false once Bench is quitting, want true")
	}
	// Before setUpNative, there's nothing to ask.
	if !newAppShell(nil, nil).shouldQuit() {
		t.Error("shouldQuit() = false with no engine to ask, want true")
	}
}

func TestShouldQuitWhileTheEngineIsBusy(t *testing.T) {
	// The engine is busy, so the main thread can't wait for it: the quit
	// stops, and the engine is asked on another goroutine. With nothing
	// running, Bench then quits.
	f := newFakeEngine()
	f.busyEngine, f.mayQuit = true, true
	s, quits := quitShell(f)
	if s.shouldQuit() {
		t.Error("shouldQuit() = true while the engine is busy, want false")
	}
	waitFor(t, "the quit", func() bool { return quits.Load() == 1 })
	if !s.passQuit.Load() {
		t.Error("the quit after asking doesn't go through")
	}
	if got := methods(f.Calls()); !slices.Equal(got, []string{"AskToQuitNow", "AskToQuit"}) {
		t.Errorf("engine calls = %v, want AskToQuitNow, then AskToQuit", got)
	}

	// With a terminal running something, the engine asks the user instead.
	f = newFakeEngine()
	f.busyEngine = true
	s, quits = quitShell(f)
	s.shouldQuit()
	waitFor(t, "the question", func() bool { return len(f.Calls()) == 2 })
	time.Sleep(20 * time.Millisecond)
	if quits.Load() != 0 || s.passQuit.Load() {
		t.Error("Bench quit while a terminal is running something")
	}
}

func TestShouldQuitWhenTheEngineIsStuck(t *testing.T) {
	old, savedLog := quitAskWait, logOutput
	quitAskWait, logOutput = 10*time.Millisecond, io.Discard
	t.Cleanup(func() { quitAskWait, logOutput = old, savedLog })

	f := newFakeEngine()
	f.busyEngine = true
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	s, quits := quitShell(f)
	s.askToQuit = func() bool { <-stuck; return false }
	if s.shouldQuit() {
		t.Error("shouldQuit() = true while the engine is busy, want false")
	}
	waitFor(t, "the quit", func() bool { return quits.Load() == 1 })
}

func TestTerminalWindowAsksBeforeClosing(t *testing.T) {
	f := newFakeEngine() // The terminal is running something.
	s, _ := quitShell(f)
	w4, w5 := &application.WebviewWindow{}, &application.WebviewWindow{}
	s.terms["w4"] = w4

	// The user closes w4's window: it stays open while Bench asks.
	if !s.keepTerminalOpen("w4", w4) {
		t.Error("the window of a terminal that's running something closed without asking")
	}
	// Bench closes w5's window itself, having forgotten it.
	if s.keepTerminalOpen("w5", w5) {
		t.Error("Bench asked as it closed a terminal's window itself")
	}
	if got := f.Calls(); !slices.EqualFunc(got, []call{{"AskToCloseTerminal", []any{"w4"}}}, func(a, b call) bool {
		return a.String() == b.String()
	}) {
		t.Errorf("engine calls = %v, want AskToCloseTerminal(w4)", got)
	}

	// Nothing's running in it, or Bench is quitting: it closes.
	f.mayQuit = true
	if s.keepTerminalOpen("w4", w4) {
		t.Error("the window of a terminal that's running nothing stayed open")
	}
	f.mayQuit = false
	s.quitting.Store(true)
	if s.keepTerminalOpen("w4", w4) {
		t.Error("a terminal's window stayed open as Bench quits")
	}
	if s.terms["w4"] != w4 {
		t.Error("asking forgot the window, so closing it wouldn't end the terminal")
	}
}
