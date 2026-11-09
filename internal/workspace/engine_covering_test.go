package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/windows"
)

// These tests cover what Bench does, as it comes forward, with windows that
// aren't in a tab and cover it (decision 68): it hides their app, or parks
// only them when the app has other windows to keep showing, says so once
// for each app, with Keep over Bench, which adds a float rule, and puts
// them back when the app comes forward, on quit, and after a crash.

// second is a display to the right of the monitor.
var second = platform.Screen{ID: "second", Name: "Second", Frame: layout.Rect{X: 2560, W: 1000, H: 800},
	Visible: layout.Rect{X: 2560, W: 1000, H: 800}}

// downloadsFrame is where the standard world's Finder window is: over
// Bench, which fills the monitor.
var downloadsFrame = layout.Rect{X: 100, Y: 100, W: 600, H: 400}

// twoFinderWindows is the standard world, with a second display and a
// second Finder window, Documents, on it.
func twoFinderWindows(p *platform.Fake) {
	standardWorld(p)
	p.SetScreens(monitor, second)
	p.AddWindow(finderID, "Documents", platform.WithFrame(layout.Rect{X: 2700, Y: 100, W: 500, H: 400}))
}

// comeForward makes Bench the active app, as a click on it does.
func (h *harness) comeForward() {
	h.t.Helper()
	mustOK(h.t, h.p.ActivateApp(benchID))
	h.drain()
}

// coveredToasts returns the toasts that say Bench hid something because it
// covered Bench.
func (h *harness) coveredToasts() []string {
	h.em.mu.Lock()
	defer h.em.mu.Unlock()
	return slices.DeleteFunc(slices.Clone(h.em.toasts), func(s string) bool { return !strings.Contains(s, "covered Bench") })
}

// lastAction returns the latest toast's button.
func (h *harness) lastAction() ToastAction {
	h.em.mu.Lock()
	defer h.em.mu.Unlock()
	if len(h.em.actions) == 0 {
		return ToastAction{}
	}
	return h.em.actions[len(h.em.actions)-1]
}

// isParked reports whether a window is parked below the bottom of the
// monitor, where only its top edge shows.
func (h *harness) isParked(id platform.WindowID) bool {
	return h.e.win.IsParked(id) && h.frame(id).Y >= monitor.Frame.H-1
}

// parkedFile reads parked.json.
func (h *harness) parkedFile() string {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.dir, config.ParkedFile))
	if err != nil {
		h.t.Fatalf("parked.json: %v", err)
	}
	return string(data)
}

func TestComingForwardHidesAnAppThatCoversBench(t *testing.T) {
	h := newHarness(t, setup{})
	h.comeForward()
	if !h.p.IsHidden(finderID) {
		t.Fatal("Finder's only window covers Bench, so Finder should be hidden")
	}
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid Finder because it covered Bench."}) {
		t.Errorf("toasts: %q", got)
	}
	if a := h.lastAction(); a != (ToastAction{Label: "Keep over Bench", Method: "KeepOverBench", Item: finderID}) {
		t.Errorf("the toast's button: %+v", a)
	}
	if !strings.Contains(h.parkedFile(), finderID) {
		t.Error("parked.json should list Finder, for a crash")
	}
}

func TestComingForwardParksOnlyTheWindowOverBench(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads, documents := h.windowID(finderID, "Downloads"), h.windowID(finderID, "Documents")
	docsFrame := h.frame(documents)
	h.comeForward()
	if h.p.IsHidden(finderID) {
		t.Fatal("Finder has a window on the other display, so it should stay shown")
	}
	if !h.isParked(downloads) {
		t.Errorf("Downloads covered Bench, so it should be parked: %v", h.frame(downloads))
	}
	if h.e.win.IsParked(documents) || h.frame(documents) != docsFrame {
		t.Errorf("Documents should stay where it is: %v", h.frame(documents))
	}
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid the Finder window “Downloads” because it covered Bench."}) {
		t.Errorf("toasts: %q", got)
	}
	if a := h.lastAction(); a.Method != "KeepOverBench" || a.Item != finderID {
		t.Errorf("the toast's button: %+v", a)
	}
	if !strings.Contains(h.parkedFile(), "Downloads") {
		t.Error("parked.json should list Downloads, for a crash")
	}
}

// A window that a float rule keeps over Bench, such as picture in picture,
// is one to keep showing too: only the app's other window over Bench goes.
func TestComingForwardKeepsAFloatingWindowOfTheSameApp(t *testing.T) {
	cfg := exampleConfig(t) + "\n[[rule]]\napp = \"" + finderID + "\"\ntitle = \"Info\"\nfloat = true\n"
	h := newHarness(t, setup{config: cfg, world: func(p *platform.Fake) {
		standardWorld(p)
		p.AddWindow(finderID, "Downloads Info", platform.WithFrame(layout.Rect{X: 900, Y: 200, W: 260, H: 400}))
	}})
	info := h.windowID(finderID, "Downloads Info")
	h.comeForward()
	if h.p.IsHidden(finderID) {
		t.Fatal("Finder's Info window floats over Bench by a rule, so Finder should stay shown")
	}
	if !h.e.win.IsParked(h.windowID(finderID, "Downloads")) {
		t.Error("Downloads covered Bench, so it should be parked")
	}
	if h.e.win.IsParked(info) {
		t.Error("the Info window floats by a rule, so it should stay")
	}
}

func TestComingForwardParksEachWindowOverBench(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		twoFinderWindows(p)
		p.AddWindow(finderID, "Desktop", platform.WithFrame(layout.Rect{X: 800, Y: 300, W: 500, H: 400}))
	}})
	h.comeForward()
	for _, title := range []string{"Downloads", "Desktop"} {
		if !h.e.win.IsParked(h.windowID(finderID, title)) {
			t.Errorf("%s covered Bench, so it should be parked", title)
		}
	}
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid 2 Finder windows because they covered Bench."}) {
		t.Errorf("toasts: %q", got)
	}
}

func TestParkedCoveringWindowsComeBackWhenTheAppComesForward(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	if !h.e.win.IsParked(downloads) {
		t.Fatal("Downloads should be parked")
	}
	// The user clicks Documents, on the other display.
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if h.e.win.IsParked(downloads) || h.frame(downloads) != downloadsFrame {
		t.Errorf("Downloads should be back where it was: %v", h.frame(downloads))
	}
	if strings.Contains(h.parkedFile(), "Downloads") {
		t.Error("parked.json should no longer list Downloads")
	}
	// Bench again: it goes again, without a second toast.
	h.comeForward()
	if !h.e.win.IsParked(downloads) {
		t.Error("Downloads covered Bench again, so it should be parked again")
	}
	if got := h.coveredToasts(); len(got) != 1 {
		t.Errorf("Bench should say so once: %q", got)
	}
}

func TestParkedCoveringWindowsComeBackOnQuit(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	if !h.e.win.IsParked(downloads) {
		t.Fatal("Downloads should be parked")
	}
	mustOK(t, h.e.Close())
	if h.frame(downloads) != downloadsFrame {
		t.Errorf("quitting should put Downloads back: %v", h.frame(downloads))
	}
}

func TestParkedCoveringWindowsComeBackAfterACrash(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	if !h.e.win.IsParked(downloads) {
		t.Fatal("Downloads should be parked")
	}
	// Bench crashes: what's left is parked.json, which bench restore and
	// the watchdog read.
	mustOK(t, windows.RestoreFromFile(h.p, filepath.Join(h.dir, config.ParkedFile)))
	if h.frame(downloads) != downloadsFrame {
		t.Errorf("restoring after a crash should put Downloads back: %v", h.frame(downloads))
	}
}

// A covering window that closes while it's parked is forgotten, so that
// nothing tries to put it back.
func TestAClosedCoveringWindowIsForgotten(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	mustOK(t, h.p.CloseWindow(downloads))
	h.drain()
	if h.e.win.IsParked(downloads) || len(h.e.covered) != 0 {
		t.Errorf("a closed window should be forgotten: parked %v, covered %v", h.e.win.IsParked(downloads), h.e.covered)
	}
}

func TestCoveringToastOncePerApp(t *testing.T) {
	const calculatorID = "com.apple.calculator"
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetScreens(monitor, second)
		p.AddApp(calculatorID, "Calculator")
		p.AddWindow(calculatorID, "Calculator", platform.WithFrame(layout.Rect{X: 2700, Y: 200, W: 230, H: 400}))
	}})
	h.comeForward()
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if h.p.IsHidden(finderID) {
		t.Fatal("Finder should show again when it comes forward")
	}
	h.comeForward()
	if !h.p.IsHidden(finderID) {
		t.Fatal("Finder covered Bench again, so it should be hidden again")
	}
	if got := h.coveredToasts(); len(got) != 1 {
		t.Errorf("Bench should say so once for Finder: %q", got)
	}

	// Another app gets its own toast, once its window covers Bench.
	mustOK(t, h.p.ActivateApp(calculatorID))
	mustOK(t, h.p.AppMoveWindow(h.windowID(calculatorID, "Calculator"), layout.Rect{X: 300, Y: 200, W: 230, H: 400}))
	h.drain()
	h.comeForward()
	if !h.p.IsHidden(calculatorID) {
		t.Fatal("Calculator covered Bench, so it should be hidden")
	}
	if got := h.coveredToasts(); len(got) != 2 || got[1] != "Hid Calculator because it covered Bench." {
		t.Errorf("toasts: %q", got)
	}

	// state.json remembers, after a restart.
	h.restart()
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	h.comeForward()
	if !h.p.IsHidden(finderID) {
		t.Fatal("Finder covered Bench, so it should be hidden")
	}
	if got := h.coveredToasts(); len(got) != 2 {
		t.Errorf("Bench said so again after a restart: %q", got)
	}
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	mustOK(t, err)
	if !strings.Contains(string(data), `"covering": [`) || !strings.Contains(string(data), `"com.apple.finder"`) {
		t.Errorf("state.json should list the apps Bench told about: %s", data)
	}
}

// When two apps cover Bench for the first time together, the first says
// so, and the other gets its toast the next time.
func TestCoveringToastOneAppAtATime(t *testing.T) {
	const calculatorID = "com.apple.calculator"
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.AddApp(calculatorID, "Calculator")
		p.AddWindow(calculatorID, "Calculator", platform.WithFrame(layout.Rect{X: 300, Y: 200, W: 230, H: 400}))
	}})
	h.comeForward()
	if !h.p.IsHidden(finderID) || !h.p.IsHidden(calculatorID) {
		t.Fatal("both apps covered Bench, so both should be hidden")
	}
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid Finder because it covered Bench."}) {
		t.Errorf("toasts: %q", got)
	}
	mustOK(t, h.p.ActivateApp(calculatorID))
	h.drain()
	h.comeForward()
	if got := h.coveredToasts(); len(got) != 2 || got[1] != "Hid Calculator because it covered Bench." {
		t.Errorf("toasts: %q", got)
	}
}

func TestKeepOverBenchAddsAFloatRule(t *testing.T) {
	h := newHarness(t, setup{})
	h.comeForward()
	if !h.p.IsHidden(finderID) {
		t.Fatal("Finder should be hidden")
	}
	mustOK(t, h.e.KeepOverBench(finderID))
	if h.p.IsHidden(finderID) {
		t.Error("Keep over Bench should show Finder again")
	}
	if got := h.em.lastToast(); got != "Added a rule to config.toml: Finder windows now stay over Bench, and new ones stay out of your tabs." {
		t.Errorf("toast: %q", got)
	}
	step := h.em.lastUndo(t)
	cfg := savedConfig(t, h)
	if !slices.Contains(cfg.Rules, config.Rule{App: finderID, Float: true}) {
		t.Errorf("config.toml's rules: %+v", cfg.Rules)
	}
	data, _ := os.ReadFile(filepath.Join(h.dir, config.ConfigFile))
	if !strings.Contains(string(data), "[[rule]]\napp = \"com.apple.finder\"\nfloat = true\n") {
		t.Errorf("config.toml:\n%s", data)
	}
	// It stays over Bench now.
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	h.comeForward()
	if h.p.IsHidden(finderID) {
		t.Error("a float rule keeps Finder over Bench")
	}
	if v := h.view(); v.Undo == nil || v.Undo.Name != "Keep over Bench" || v.Undo.Did != "kept Finder windows over Bench" {
		t.Errorf("undo: %+v", v.Undo)
	}

	// Undo takes the rule back, and Finder goes again, as Bench is in front.
	mustOK(t, h.e.Undo(step))
	if got := h.em.lastToast(); got != "Finder windows no longer stay over Bench." {
		t.Errorf("toast: %q", got)
	}
	if cfg := savedConfig(t, h); slices.ContainsFunc(cfg.Rules, func(r config.Rule) bool { return sameApp(r.App, finderID) }) {
		t.Errorf("Undo should take the rule back: %+v", cfg.Rules)
	}
	if !h.p.IsHidden(finderID) {
		t.Error("Finder covers Bench again, so Undo should hide it again")
	}
	if got := h.coveredToasts(); len(got) != 1 {
		t.Errorf("Bench should say so once: %q", got)
	}
}

func TestKeepOverBenchPutsParkedWindowsBack(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	mustOK(t, h.e.KeepOverBench(finderID))
	if h.e.win.IsParked(downloads) || h.frame(downloads) != downloadsFrame {
		t.Errorf("Keep over Bench should put Downloads back: %v", h.frame(downloads))
	}
}

func TestKeepOverBenchNeedsAToast(t *testing.T) {
	h := newHarness(t, setup{})
	mustErr(t, h.e.KeepOverBench(finderID), errCantKeepOver.Error())
	mustErr(t, h.e.KeepOverBench(""), errCantKeepOver.Error())
	if cfg := savedConfig(t, h); slices.ContainsFunc(cfg.Rules, func(r config.Rule) bool { return sameApp(r.App, finderID) }) {
		t.Errorf("config.toml's rules: %+v", cfg.Rules)
	}
}

// With a float rule for the app already, as after Keep over Bench from an
// older toast, Keep over Bench only shows it again.
func TestKeepOverBenchTwice(t *testing.T) {
	h := newHarness(t, setup{})
	h.comeForward()
	mustOK(t, h.e.KeepOverBench(finderID))
	before := len(savedConfig(t, h).Rules)
	mustOK(t, h.e.KeepOverBench(finderID))
	if got := h.em.lastToast(); got != "Finder windows stay over Bench already." {
		t.Errorf("toast: %q", got)
	}
	if after := len(savedConfig(t, h).Rules); after != before {
		t.Errorf("rules: %d, was %d", after, before)
	}
}

// An app with items gets no Keep over Bench: a float rule would keep its
// windows out of them.
func TestCoveringToastWithoutKeepForAnAppWithItems(t *testing.T) {
	cfg := strings.Replace(exampleConfig(t), `items = [{ app = "md.obsidian" }]`,
		`items = [{ app = "md.obsidian" }, { app = "com.apple.finder", title = "Projects" }]`, 1)
	h := newHarness(t, setup{config: cfg})
	if _, free := h.e.unmanaged[h.windowID(finderID, "Downloads")]; !free {
		t.Fatal("Downloads should be out of the tabs")
	}
	h.comeForward()
	if !h.p.IsHidden(finderID) {
		t.Fatal("Finder's window covers Bench, so Finder should be hidden")
	}
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid Finder because it covered Bench."}) {
		t.Errorf("toasts: %q", got)
	}
	if h.em.lastHasAction() {
		t.Errorf("Finder has an item, so the toast should have no button: %+v", h.lastAction())
	}
}

func TestLeaveCoveringWindows(t *testing.T) {
	cfg := strings.Replace(exampleConfig(t), `covering_windows = "hide" `, `covering_windows = "leave"`, 1)
	h := newHarness(t, setup{config: cfg})
	if got := h.view().Settings.CoveringWindows; got != config.CoveringLeave {
		t.Fatalf("the setting: %q", got)
	}
	h.comeForward()
	if h.p.IsHidden(finderID) {
		t.Error("with covering_windows = \"leave\", Finder should stay over Bench")
	}
	if got := h.coveredToasts(); len(got) != 0 {
		t.Errorf("toasts: %q", got)
	}

	// Settings changes it.
	s := h.view().Settings
	s.CoveringWindows = config.CoveringHide
	mustOK(t, h.e.SaveSettings(s))
	if got := savedConfig(t, h).Settings.CoveringWindows; got != config.CoveringHide {
		t.Errorf("config.toml: %q", got)
	}
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	h.comeForward()
	if !h.p.IsHidden(finderID) {
		t.Error("with covering_windows = \"hide\", Finder should be hidden")
	}
	s.CoveringWindows = config.CoveringLeave
	mustOK(t, h.e.SaveSettings(s))
	if got := savedConfig(t, h).Settings.CoveringWindows; got != config.CoveringLeave {
		t.Errorf("config.toml: %q", got)
	}
	s.CoveringWindows = "sometimes"
	if err := h.e.SaveSettings(s); err == nil {
		t.Error("an unknown value should be refused")
	}
}

// The browser preview's Bring Bench forward does what a click on Bench
// does on a Mac.
func TestDemoComeForward(t *testing.T) {
	h := newHarness(t, setup{options: func(o *Options) { o.Demo = true }})
	mustOK(t, h.e.DemoComeForward())
	h.drain()
	if !h.p.IsHidden(finderID) {
		t.Error("Finder's window covers Bench, so Finder should be hidden")
	}
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid Finder because it covered Bench."}) {
		t.Errorf("toasts: %q", got)
	}

	h2 := newHarness(t, setup{})
	mustErr(t, h2.e.DemoComeForward(), "Simulated windows are only for the browser preview.")
}

// A window that Bench parked because it covered Bench, and couldn't put
// back as its app came forward, as when the app didn't answer, stays to put
// back: the next time the app comes forward brings it back.
func TestCoveringWindowComesBackAfterAFailedPutBack(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	if !h.isParked(downloads) {
		t.Fatal("Downloads should be parked")
	}
	// Finder doesn't answer for a moment.
	h.p.SetWindowErr(downloads, "Window", errors.New("Finder didn't answer"))
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if !h.isParked(downloads) {
		t.Fatal("Downloads couldn't be put back, so it should still be parked")
	}
	h.p.SetWindowErr(downloads, "Window", nil)
	// The user goes back to Bench, then to Finder again.
	h.comeForward()
	if !h.isParked(downloads) {
		t.Fatalf("Downloads should still be out of sight: %v", h.frame(downloads))
	}
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if h.e.win.IsParked(downloads) || h.frame(downloads) != downloadsFrame {
		t.Errorf("Downloads should be back where it was the next time Finder came forward: %v", h.frame(downloads))
	}
	if len(h.e.covered) != 0 {
		t.Errorf("covered: %v", h.e.covered)
	}
}

// A parked window on a desktop that isn't shown can't be put back as its
// app comes forward on another: it comes back as the user goes to its
// desktop with the app still in front.
func TestCoveringWindowOnAnotherDesktopComesBack(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	// The user goes to another desktop, and to Finder there.
	mustOK(t, h.p.MoveToOtherDesktop(downloads))
	h.drain()
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if !h.e.win.IsParked(downloads) {
		t.Fatal("Downloads is on another desktop, so it should still be parked")
	}
	// Back to Bench's desktop, with Finder in front.
	mustOK(t, h.p.ReturnToDesktop(downloads))
	h.p.EmitEvent(platform.Event{Kind: platform.DesktopChanged})
	h.drain()
	if h.e.win.IsParked(downloads) || h.frame(downloads) != downloadsFrame {
		t.Errorf("Downloads should be back where it was: %v", h.frame(downloads))
	}
}

// Without Accessibility permission, Bench can't move a parked window back
// as its app comes forward: it does so once the permission is back.
func TestCoveringWindowComesBackOnceAccessibilityIsBack(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	loseAccessibility(t, h)
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if !h.e.win.IsParked(downloads) {
		t.Fatal("Bench can't move Downloads without the permission")
	}
	h.p.SetTrusted(true)
	h.advance(trustEvery + 100*time.Millisecond)
	if h.e.win.IsParked(downloads) || h.frame(downloads) != downloadsFrame {
		t.Errorf("Downloads should be back where it was: %v", h.frame(downloads))
	}
}

// A window in native full screen is one to keep showing: Bench parks only
// the window over it. Going to the full-screen window's desktop brings
// Finder forward, but Downloads, on Bench's desktop, can't be reached from
// there: it comes back the next time Finder comes forward.
func TestCoveringParksBesideAFullScreenWindow(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.AddWindow(finderID, "Documents", platform.WithFullScreen())
	}})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	if h.p.IsHidden(finderID) {
		t.Fatal("Finder has a window in full screen, so it should stay shown")
	}
	if !h.isParked(downloads) {
		t.Fatalf("Downloads covered Bench, so it should be parked: %v", h.frame(downloads))
	}
	h.p.SetWindowErr(downloads, "Window", platform.ErrOtherDesktop)
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if !h.e.win.IsParked(downloads) {
		t.Fatal("Downloads can't be reached, so it should still be parked")
	}
	h.p.SetWindowErr(downloads, "Window", nil)
	h.comeForward()
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if h.e.win.IsParked(downloads) || h.frame(downloads) != downloadsFrame {
		t.Errorf("Downloads should be back where it was: %v", h.frame(downloads))
	}
}

// A window on the same screen as Bench, but clear of it, is one to keep
// showing too.
func TestCoveringParksBesideAWindowClearOfBench(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetContentFrame(layout.Rect{Y: 25, W: 1800, H: 1415})
		p.AddWindow(finderID, "Documents", platform.WithFrame(layout.Rect{X: 1900, Y: 100, W: 500, H: 400}))
	}})
	downloads, documents := h.windowID(finderID, "Downloads"), h.windowID(finderID, "Documents")
	docsFrame := h.frame(documents)
	h.comeForward()
	if h.p.IsHidden(finderID) {
		t.Fatal("Finder has a window beside Bench, so it should stay shown")
	}
	if !h.e.win.IsParked(downloads) {
		t.Errorf("Downloads covered Bench, so it should be parked: %v", h.frame(downloads))
	}
	if h.e.win.IsParked(documents) || h.frame(documents) != docsFrame {
		t.Errorf("Documents should stay where it is: %v", h.frame(documents))
	}
}

// A parked window that comes back over Bench by itself, as when macOS
// moves windows as displays come back, goes again as Bench comes forward,
// although its app hasn't come forward.
func TestCoveringWindowBackOverBenchGoesAgain(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	mustOK(t, h.p.AppMoveWindow(downloads, downloadsFrame))
	h.settle()
	mustOK(t, h.p.ActivateApp(zedID))
	h.drain()
	h.comeForward()
	if !h.isParked(downloads) {
		t.Errorf("Downloads covered Bench again, so it should be parked again: %v", h.frame(downloads))
	}
	if got := h.coveredToasts(); len(got) != 1 {
		t.Errorf("Bench should say so once: %q", got)
	}
}

// As the screens settle after the Mac wakes, a parked window that macOS
// has put back on screen goes again, as other tabs' windows do, while its
// app stays in the background.
func TestCoveringWindowParkedAgainAfterWaking(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	mustOK(t, h.p.AppMoveWindow(downloads, downloadsFrame))
	h.p.EmitEvent(platform.Event{Kind: platform.Woke})
	h.advance(screenSettle + 100*time.Millisecond)
	if !h.isParked(downloads) {
		t.Errorf("Downloads should be parked again: %v", h.frame(downloads))
	}
}

// With covering_windows set to "leave" after Bench parked a window, a
// window that macOS puts back on screen as the screens settle stays where
// it is.
func TestCoveringWindowNotParkedAgainOnceLeft(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	if !h.isParked(downloads) {
		t.Fatalf("Downloads covered Bench, so it should be parked: %v", h.frame(downloads))
	}
	s := h.view().Settings
	s.CoveringWindows = config.CoveringLeave
	mustOK(t, h.e.SaveSettings(s))
	mustOK(t, h.p.AppMoveWindow(downloads, downloadsFrame))
	h.p.EmitEvent(platform.Event{Kind: platform.Woke})
	h.advance(screenSettle + 100*time.Millisecond)
	if h.isParked(downloads) || h.frame(downloads) != downloadsFrame {
		t.Errorf("with covering_windows = \"leave\", Downloads should stay where macOS put it: %v", h.frame(downloads))
	}
}

// Bench's window coming back while Bench is in front puts away what covers
// it, as Bench coming forward does.
func TestCoveringWindowsGoWhenBenchsWindowIsBack(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.comeForward()
	if h.e.win.IsParked(downloads) {
		t.Fatal("Bench's window is hidden, so Downloads should stay")
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if !h.isParked(downloads) {
		t.Errorf("Downloads covers Bench's window, so it should be parked: %v", h.frame(downloads))
	}
	if got := h.coveredToasts(); len(got) != 1 {
		t.Errorf("toasts: %q", got)
	}
}

// A parked window that a tab has taken since is the tab's to place: it
// stays out of sight as Finder comes forward. Here a reloaded config.toml
// gives Research an item that takes it, which doesn't bring Finder
// forward.
func TestCoveringWindowTakenIntoATabIsLeftToIt(t *testing.T) {
	h := newHarness(t, setup{world: twoFinderWindows})
	downloads := h.windowID(finderID, "Downloads")
	h.comeForward()
	parkedAt := h.frame(downloads)
	writeConfig(t, h, strings.Replace(exampleConfig(t), `items = [{ app = "md.obsidian" }]`,
		`items = [{ app = "md.obsidian" }, { app = "com.apple.finder", title = "Downloads" }]`, 1))
	mustOK(t, h.e.ReloadConfig())
	h.settle()
	if h.itemOf(downloads) == "" {
		t.Fatal("Research should have taken Downloads")
	}
	mustOK(t, h.p.ActivateApp(finderID))
	h.drain()
	if !h.e.win.IsParked(downloads) || h.frame(downloads) != parkedAt {
		t.Errorf("Downloads belongs to Research, which isn't shown, so it should stay out of sight: %v", h.frame(downloads))
	}
	if len(h.e.covered) != 0 {
		t.Errorf("covered: %v", h.e.covered)
	}
}

// Keep over Bench changes only config.toml's rules, so its Undo leaves the
// arrangement as it is now: a tab switch and new sizes since stay.
func TestUndoKeepOverBenchKeepsTheArrangement(t *testing.T) {
	h := newHarness(t, setup{})
	h.comeForward()
	mustOK(t, h.e.KeepOverBench(finderID))
	step := h.em.lastUndo(t)
	mustOK(t, h.e.SwitchTab(1))
	h.drain()
	split := h.view().Tabs[1].Root
	if !split.IsSplit() {
		t.Fatalf("Code's root isn't a split: %+v", split)
	}
	mustOK(t, h.e.SetSizes(split.ID, []float64{30, 70}))
	h.drain()
	sizes := h.view().Tabs[1].Root.Sizes
	mustOK(t, h.e.Undo(step))
	h.drain()
	v := h.view()
	if v.Current != 1 {
		t.Errorf("Undo switched tabs: %d", v.Current)
	}
	if got := v.Tabs[1].Root.Sizes; !slices.Equal(got, sizes) {
		t.Errorf("Undo changed the sizes: %v", got)
	}
	if cfg := savedConfig(t, h); slices.ContainsFunc(cfg.Rules, func(r config.Rule) bool { return sameApp(r.App, finderID) }) {
		t.Errorf("Undo should take the rule back: %+v", cfg.Rules)
	}
}

// An app that a rule sends to a tab gets no Keep over Bench: a float rule
// for the app would win over that rule.
func TestNoKeepOverBenchForAnAppWithATabRule(t *testing.T) {
	cfg := exampleConfig(t) + "\n[[rule]]\napp = \"" + finderID + "\"\ntab = \"Research\"\n"
	h := newHarness(t, setup{config: cfg})
	if _, free := h.e.unmanaged[h.windowID(finderID, "Downloads")]; !free {
		t.Fatal("Downloads should be out of the tabs")
	}
	h.comeForward()
	if got := h.coveredToasts(); !slices.Equal(got, []string{"Hid Finder because it covered Bench."}) {
		t.Errorf("toasts: %q", got)
	}
	if h.em.lastHasAction() {
		t.Errorf("a rule sends Finder to a tab, so the toast should have no button: %+v", h.lastAction())
	}
	mustErr(t, h.e.KeepOverBench(finderID), errKeepOverInTabs.Error())
	if rules := savedConfig(t, h).Rules; slices.ContainsFunc(rules, func(r config.Rule) bool { return sameApp(r.App, finderID) && r.Float }) {
		t.Errorf("config.toml's rules: %+v", rules)
	}
	if v := h.view(); slices.Contains(v.KeepOver, finderID) {
		t.Errorf("keepOver: %v", v.KeepOver)
	}
}

// Keep over Bench stays on offer, for the recent messages, until it's
// used: the View lists the apps it can keep over Bench.
// View.KeepOver has lower-case bundle IDs, and the toast's button the app's
// as macOS gives it, which the frontend compares in lower case, and which
// the rule gets.
func TestKeepOverForABundleIDWithCapitals(t *testing.T) {
	const previewID = "com.apple.Preview"
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.AddApp(previewID, "Preview")
		p.AddWindow(previewID, "scan.pdf", platform.WithFrame(layout.Rect{X: 800, Y: 200, W: 500, H: 400}))
	}})
	mustOK(t, h.p.Hide(platform.App{BundleID: finderID}))
	h.drain()
	h.comeForward()
	a := h.lastAction()
	if a.Method != "KeepOverBench" || a.Item != previewID {
		t.Fatalf("the toast's button: %+v", a)
	}
	if v := h.view(); !slices.Contains(v.KeepOver, strings.ToLower(a.Item)) {
		t.Errorf("keepOver %v doesn't list %s in lower case", v.KeepOver, a.Item)
	}
	mustOK(t, h.e.KeepOverBench(a.Item))
	if !slices.ContainsFunc(savedConfig(t, h).Rules, func(r config.Rule) bool { return r.App == previewID && r.Float }) {
		t.Errorf("config.toml should have a float rule for %s: %+v", previewID, savedConfig(t, h).Rules)
	}
}

func TestKeepOverListedUntilUsed(t *testing.T) {
	h := newHarness(t, setup{})
	if v := h.view(); len(v.KeepOver) != 0 {
		t.Fatalf("keepOver: %v", v.KeepOver)
	}
	h.comeForward()
	if v := h.view(); !slices.Equal(v.KeepOver, []string{finderID}) {
		t.Errorf("keepOver: %v", v.KeepOver)
	}
	mustOK(t, h.e.KeepOverBench(finderID))
	step := h.em.lastUndo(t)
	if v := h.view(); len(v.KeepOver) != 0 {
		t.Errorf("Finder floats by a rule now: keepOver %v", v.KeepOver)
	}
	mustOK(t, h.e.Undo(step))
	if v := h.view(); !slices.Equal(v.KeepOver, []string{finderID}) {
		t.Errorf("Undo took the rule back: keepOver %v", v.KeepOver)
	}
}
