package windows

import (
	"testing"

	"github.com/itbm/bench/internal/platform"
)

// TestRestoreFromFileReport checks that restoring after a crash reports
// what it put back, not what the file listed: windows that have closed and
// apps that have quit since are counted apart, so that bench restore
// doesn't claim to have put them back.
func TestRestoreFromFileReport(t *testing.T) {
	w := newWorld(t)
	w.f.AddApp(finderID, "Finder")
	w.f.AddApp(zedID, "Zed")
	docs := w.window(braveID, "docs", 0)
	pr := w.window(braveID, "pr", 1)
	issues := w.window(braveID, "issues", 2)
	noErrors(t, w.m.Apply(Plan{Screen: monitor, Park: []platform.Window{docs, pr, issues}}))
	noErrors(t, w.m.Apply(Plan{Hide: []platform.App{w.app(finderID), w.app(zedID)}}))

	// Bench crashes; then two of the windows close, and Zed quits.
	w.m = nil
	w.f.CloseWindow(pr.ID)
	w.f.CloseWindow(issues.ID)
	w.f.QuitApp(zedID)

	got, err := RestoreFromFileReport(w.f, w.path)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Restored{Windows: 1, Closed: 2, Apps: 1, Quit: 1}); got != want {
		t.Errorf("RestoreFromFileReport = %+v, want %+v", got, want)
	}
	if w.frame(t, docs.ID) != startFrame || w.f.IsHidden(finderID) {
		t.Error("the window and the app that were still there weren't put back")
	}

	// Nothing is left, so a second try reports nothing.
	if got, err := RestoreFromFileReport(w.f, w.path); err != nil || got != (Restored{}) {
		t.Errorf("RestoreFromFileReport again = %+v, %v; want nothing", got, err)
	}
}
