package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// These tests follow phase 2: Bench remembers the apps the user quit, and
// the ones that gave no window, so that tab switches leave them alone, and
// it opens an app for one remembered place when the user clicks Open
// (OpenItem). In the standard world, Brave's places are w6 ("Wails v3
// docs", whose window was made first), w17 ("Pull request #42 · bench"),
// and w18 ("Go packages"), all in Research's pane p14.

// quitBraveKey is Brave's key in state.json's apps: its bundle ID in lower
// case.
const quitBraveKey = "com.brave.browser"

// quitRecord is an app's entry in state.json's apps, read by its JSON
// names, as another build would read it.
type quitRecord struct {
	Quit bool      `json:"quit"`
	Seen time.Time `json:"seen"`
}

// quitAppsOf returns the apps of a state.json, by key.
func quitAppsOf(t *testing.T, data []byte) map[string]quitRecord {
	t.Helper()
	var st struct {
		Apps map[string]quitRecord `json:"apps"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("state.json doesn't parse: %v", err)
	}
	return st.Apps
}

// quitSavedApps lets a waiting save of state.json fall due, and returns its
// apps.
func quitSavedApps(h *harness) map[string]quitRecord {
	h.t.Helper()
	h.advance(saveMaxDelay)
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	if err != nil {
		h.t.Fatalf("state.json: %v", err)
	}
	return quitAppsOf(h.t, data)
}

// quitAsked returns the calls since the last reset that asked an app to
// open, or for a new window.
func quitAsked(h *harness, bundleID string) []platform.Call {
	var out []platform.Call
	for _, c := range h.calls("OpenApp", "NewWindow") {
		if c.BundleID == bundleID {
			out = append(out, c)
		}
	}
	return out
}

// quitBrave quits Brave as the user would, and lets Bench see it.
func quitBrave(h *harness) {
	h.t.Helper()
	mustOK(h.t, h.p.QuitApp(braveID))
	h.drain()
}

// quitPane returns a displayed pane, failing the test if it isn't
// displayed.
func quitPane(h *harness, id string) PaneView {
	h.t.Helper()
	for _, p := range h.view().Display.Panes {
		if p.ID == id {
			return p
		}
	}
	h.t.Fatalf("pane %s isn't displayed", id)
	return PaneView{}
}

// TestQuitAppStaysClosedAcrossSwitchesAndRestarts checks that an app the
// user quit isn't opened again by tab switches, though open_missing_apps is
// on, and that state.json keeps the quit, and when the app was last seen
// running, so that a restarted Bench leaves it closed too. Its places wait
// for it, out of sight.
func TestQuitAppStaysClosedAcrossSwitchesAndRestarts(t *testing.T) {
	h := newHarness(t, setup{})
	quitAt := h.clk.Now()
	quitBrave(h)
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	if asked := quitAsked(h, braveID); len(asked) != 0 || h.p.IsRunning(braveID) {
		t.Errorf("switches opened Brave, which the user quit: %v", asked)
	}
	for _, id := range []string{"w6", "w17", "w18"} {
		if it := h.item(id); !it.Hidden || it.Pane != "p14" || it.StatusText != "Brave isn't running" {
			t.Errorf("%s: hidden %v in %s, %q; want a hidden place in p14 saying Brave isn't running",
				id, it.Hidden, it.Pane, it.StatusText)
		}
	}
	apps := quitSavedApps(h)
	if a := apps[quitBraveKey]; !a.Quit || !a.Seen.Equal(quitAt) {
		t.Errorf("state.json's Brave: %+v, want quit, last seen at %v", a, quitAt)
	}
	if a, ok := apps["md.obsidian"]; !ok || a.Quit || a.Seen.IsZero() {
		t.Errorf("state.json's Obsidian: %+v (listed %v), want seen running and not quit", a, ok)
	}

	// Bench restarts on Research: neither its start nor later switches open
	// Brave.
	h.p.ResetCalls()
	h.restart()
	if h.view().Current != 2 {
		t.Fatalf("Bench should restart on Research, not %d", h.view().Current)
	}
	mustOK(t, h.e.SwitchTab(0))
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	if asked := quitAsked(h, braveID); len(asked) != 0 {
		t.Errorf("after a restart, Bench opened Brave, which the user quit: %v", asked)
	}
	if !h.item("w6").Hidden {
		t.Errorf("after a restart, Brave's place should still be hidden")
	}
	if a := quitSavedApps(h)[quitBraveKey]; !a.Quit || !a.Seen.Equal(quitAt) {
		t.Errorf("state.json's Brave after a restart: %+v, want still quit, last seen at %v", a, quitAt)
	}
}

// TestQuitClearsWhenTheAppOpensByItself checks that the quit lasts only
// until the app opens again, whoever opens it: once the user opens Brave
// from the Dock, the record goes from state.json, and the next switch to
// Research asks Brave for windows for its places again.
func TestQuitClearsWhenTheAppOpensByItself(t *testing.T) {
	h := newHarness(t, setup{})
	quitBrave(h)
	if !quitSavedApps(h)[quitBraveKey].Quit {
		t.Fatal("Brave's quit wasn't recorded")
	}
	// Brave opens again, restoring none of its windows. state.json must
	// stop recording the quit, or a Bench that doesn't get to quit cleanly
	// would leave Brave closed at its next start.
	launchedAt := h.clk.Now()
	mustOK(t, h.p.LaunchApp(braveID))
	h.drain()
	if a := quitSavedApps(h)[quitBraveKey]; a.Quit || a.Seen.Before(launchedAt) {
		t.Errorf("state.json's Brave after it opened: %+v, want not quit, seen since %v: "+
			"the quit was forgotten, but no save followed", a, launchedAt)
	}
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(2))
	h.drain()
	if asked := quitAsked(h, braveID); len(asked) != 1 || asked[0].Method != "OpenApp" {
		t.Fatalf("the switch should ask Brave for its windows: %v", asked)
	}
	// Brave answers with a window, which goes to one of its places.
	w := h.windowID(braveID, platform.DefaultNewWindowTitle)
	if got := h.itemOf(w); !slices.Contains([]string{"w6", "w17", "w18"}, got) {
		t.Errorf("Brave's window went to %q, want one of its places in Research", got)
	}
}

// TestQuitForgottenForAnAppRunningAtStart checks that a quit in state.json
// is forgotten if Bench starts to find the app running, as when the user
// opened it while Bench wasn't running: the switch to Research asks it for
// windows for its places, and state.json no longer records the quit.
func TestQuitForgottenForAnAppRunningAtStart(t *testing.T) {
	data := savedState(t, func(h *harness) { quitBrave(h) })
	if !quitAppsOf(t, data)[quitBraveKey].Quit {
		t.Fatal("the saved state should record Brave's quit")
	}
	h := newHarness(t, setup{state: writeState(data), world: func(p *platform.Fake) {
		standardWorld(p)
		// Brave was opened again, and has no windows open.
		p.AddApp(braveID, "Brave", platform.AppNotRunning())
		p.AddApp(braveID, "Brave")
	}})
	if a := quitSavedApps(h)[quitBraveKey]; a.Quit {
		t.Errorf("state.json still records the quit of Brave, which is running: %+v", a)
	}
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(2))
	h.drain()
	if asked := quitAsked(h, braveID); len(asked) != 1 {
		t.Errorf("the switch should ask the running Brave for its windows: %v", asked)
	}
}

// TestQuitsWhileLoggingOutAreNotRecorded checks that apps that quit within
// five minutes of the user choosing Log Out, Restart, or Shut Down aren't
// taken for the user's quits, so that Bench opens them as it would have,
// and that a quit after those five minutes counts again.
func TestQuitsWhileLoggingOutAreNotRecorded(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		// Brave restores its windows when it opens.
		p.InstallApp(braveID, "Brave",
			platform.AppLaunchWindows("Wails v3 docs", "Pull request #42 · bench", "Go packages"))
	}})
	loggingOut := h.clk.Now()
	h.p.EmitEvent(platform.Event{Kind: platform.LoggingOut})
	h.drain()
	// Brave asks about a download, and quits nearly five minutes later.
	h.advance(4*time.Minute + 50*time.Second)
	quitBrave(h)
	if a := quitSavedApps(h)[quitBraveKey]; a.Quit {
		t.Errorf("a quit while logging out was recorded: %+v", a)
	}
	// The user cancelled the log-out: Research's switch opens Brave, whose
	// windows go back to their places.
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(2))
	h.drain()
	if asked := quitAsked(h, braveID); len(asked) != 1 {
		t.Fatalf("the switch should open Brave: %v", asked)
	}
	if got := h.itemOf(h.windowID(braveID, "Go packages")); got != "w18" {
		t.Errorf("Brave's \"Go packages\" went to %q, want w18", got)
	}

	// More than five minutes after the log-out began, the user quits Brave.
	h.advance(loggingOut.Add(5*time.Minute + 10*time.Second).Sub(h.clk.Now()))
	quitBrave(h)
	if a := quitSavedApps(h)[quitBraveKey]; !a.Quit {
		t.Errorf("a quit after the log-out's five minutes wasn't recorded: %+v", a)
	}
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	if asked := quitAsked(h, braveID); len(asked) != 0 {
		t.Errorf("a switch opened Brave, which the user quit: %v", asked)
	}
}

// TestUpdaterRelaunchIsNotAQuit checks that when an updater starts an app's
// new copy before its old copy has quit, the old copy's termination isn't
// taken for the user quitting it: Bench still sees the app running, records
// no quit, and asks the app for windows on the next switch.
func TestUpdaterRelaunchIsNotAQuit(t *testing.T) {
	h := newHarness(t, setup{})
	old, _ := h.p.App(braveID)
	mustOK(t, h.p.QuitApp(braveID))
	terminated := h.p.DrainEvents()
	mustOK(t, h.p.LaunchApp(braveID))
	launched := h.p.DrainEvents()
	if now, _ := h.p.App(braveID); now.PID == old.PID {
		t.Fatalf("the new copy should have a new process ID: %d", now.PID)
	}
	// Bench hears of the new copy's launch first, then of the old copy's
	// end.
	for _, ev := range append(launched, terminated...) {
		h.e.handle(ev)
	}
	h.drain()
	for _, id := range []string{"w6", "w17", "w18"} {
		if it := h.item(id); it.StatusText != "Brave has no window open for this" {
			t.Errorf("%s: %q, want Brave running without a window for it", id, it.StatusText)
		}
	}
	if a := quitSavedApps(h)[quitBraveKey]; a.Quit {
		t.Errorf("an update was recorded as a quit: %+v", a)
	}
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(2))
	h.drain()
	if asked := quitAsked(h, braveID); len(asked) != 1 {
		t.Errorf("the switch should ask Brave for its windows: %v", asked)
	}
}

// quitNoWindowConfig has Calendar in Plan, and Safari for two pages: one in
// Plan, one in Read.
const quitNoWindowConfig = `
[settings]
launch_timeout = "3s"

[[tab]]
name = "Home"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }]

[[tab]]
name = "Plan"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.apple.iCal" }]
  [[tab.pane]]
  items = [{ app = "com.apple.Safari", title = "Apple", url = "https://apple.com" }]

[[tab]]
name = "Read"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.apple.Safari", title = "Example", url = "https://example.com" }]
`

// quitNoWindowWorld has Calendar, which opens without a window, and Safari,
// which opens a window for example.com but none for apple.com. The items
// are w2 (Calendar) and w3 (Safari, apple.com) in Plan, and w4 (Safari,
// example.com) in Read.
func quitNoWindowWorld(p *platform.Fake) {
	p.InstallApp(calendarID, "Calendar")
	p.InstallApp(safariID, "Safari", platform.AppURLWindow("https://example.com", "Example Domain"))
}

// TestAppThatGaveNoWindowIsNotAskedAgain checks that when a switch waited
// for a running app's window and none came within launch_timeout, later
// switches don't ask that app for that page again, which would only bring
// it forward, until it launches again; and that the app is still asked for
// its other pages.
func TestAppThatGaveNoWindowIsNotAskedAgain(t *testing.T) {
	h := newHarness(t, setup{config: quitNoWindowConfig, world: quitNoWindowWorld})
	mustOK(t, h.e.SwitchTab(1))
	if len(quitAsked(h, calendarID)) != 1 || len(quitAsked(h, safariID)) != 1 {
		t.Fatalf("Plan's switch should open Calendar and Safari: %v", h.calls("OpenApp"))
	}
	h.advance(3500 * time.Millisecond)
	if it := h.item("w2"); it.StatusText != "Calendar has no window open for this" {
		t.Errorf("w2 after the launch timeout: %q", it.StatusText)
	}
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(0))
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if asked := append(quitAsked(h, calendarID), quitAsked(h, safariID)...); len(asked) != 0 {
		t.Errorf("Calendar and Safari gave no window, but were asked again: %v", asked)
	}

	// Safari is still asked for another page, and its window goes there.
	mustOK(t, h.e.SwitchTab(2))
	h.drain()
	if asked := quitAsked(h, safariID); len(asked) != 1 || asked[0].Arg != "https://example.com" {
		t.Errorf("Read's switch should ask Safari for example.com: %v", asked)
	}
	if got := h.itemOf(h.windowID(safariID, "Example Domain")); got != "w4" {
		t.Errorf("Safari's window went to %q, want w4", got)
	}

	// Safari quits and opens again: it's asked for apple.com once more,
	// while Calendar, which hasn't opened again, still isn't asked.
	mustOK(t, h.p.QuitApp(safariID))
	h.drain()
	mustOK(t, h.p.LaunchApp(safariID))
	h.drain()
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(1))
	h.drain()
	if asked := quitAsked(h, safariID); len(asked) != 1 || asked[0].Arg != "https://apple.com" {
		t.Errorf("after Safari opened again, the switch should ask it for apple.com: %v", asked)
	}
	if asked := quitAsked(h, calendarID); len(asked) != 0 {
		t.Errorf("Calendar hasn't opened again, but was asked: %v", asked)
	}
}

// TestOpenItemAsksAnAppThatGaveNoWindow checks that Open asks a running app
// that gave no window, as switches no longer do: for a new window, for an
// item without a url, or to open the item's url, and that the window goes
// to the item.
func TestOpenItemAsksAnAppThatGaveNoWindow(t *testing.T) {
	h := newHarness(t, setup{config: quitNoWindowConfig, world: quitNoWindowWorld})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(3500 * time.Millisecond)
	if !h.item("w2").Hidden || !h.item("w3").Hidden {
		t.Fatalf("Plan's places should be hidden once the switch has given up")
	}
	h.p.ResetCalls()
	mustOK(t, h.e.OpenItem("w2"))
	h.drain()
	if asked := quitAsked(h, calendarID); len(asked) != 1 || asked[0].Method != "NewWindow" {
		t.Errorf("Open should ask the running Calendar for a new window: %v", asked)
	}
	if got := h.itemOf(h.windowID(calendarID, platform.DefaultNewWindowTitle)); got != "w2" {
		t.Errorf("Calendar's new window went to %q, want w2", got)
	}
	if it := h.item("w2"); it.Hidden || it.Status != StatusReady {
		t.Errorf("w2 should show its window: hidden %v, %s %q", it.Hidden, it.Status, it.StatusText)
	}

	// Safari now opens a window for apple.com.
	h.p.InstallApp(safariID, "Safari", platform.AppURLWindow("https://apple.com", "Apple"))
	mustOK(t, h.e.OpenItem("w3"))
	h.drain()
	if asked := quitAsked(h, safariID); len(asked) != 1 || asked[0].Method != "OpenApp" || asked[0].Arg != "https://apple.com" {
		t.Errorf("Open should ask the running Safari to open w3's url: %v", asked)
	}
	if got := h.itemOf(h.windowID(safariID, "Apple")); got != "w3" {
		t.Errorf("Safari's window went to %q, want w3", got)
	}
}

// TestOpenItemLaunchesAppIntoClickedPlace checks Open for an app the user
// quit: Bench opens it all the same, forgets the quit, shows the clicked
// place saying the app is opening, and puts the app's new window there,
// rather than in the place that comes first by the order Brave's windows
// were made in.
func TestOpenItemLaunchesAppIntoClickedPlace(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(braveID, "Brave", platform.AppLaunchDelay(5*time.Second), platform.AppLaunchWindows("New Tab"))
	}})
	quitBrave(h)
	mustOK(t, h.e.SwitchTab(2))
	h.p.ResetCalls()
	mustOK(t, h.e.OpenItem("w18"))
	if asked := quitAsked(h, braveID); len(asked) != 1 || asked[0].Method != "OpenApp" || asked[0].Arg != "" {
		t.Fatalf("Open should open Brave: %v", asked)
	}
	// While Brave opens, the clicked place shows, saying so.
	if it := h.item("w18"); it.Hidden || it.StatusText != "Opening Brave…" {
		t.Errorf("w18 while Brave opens: hidden %v, %q", it.Hidden, it.StatusText)
	}
	if p := quitPane(h, "p14"); p.Active != "w18" || !slices.Equal(p.Items, []string{"w18"}) {
		t.Errorf("p14 while Brave opens: items %v, active %q; want only w18", p.Items, p.Active)
	}
	if a := quitSavedApps(h)[quitBraveKey]; a.Quit {
		t.Errorf("Open should forget Brave's quit: %+v", a)
	}
	if h.p.IsRunning(braveID) {
		t.Fatal("Brave opened sooner than the test expects")
	}
	h.advance(2500 * time.Millisecond)
	tab := h.windowID(braveID, "New Tab")
	if got := h.itemOf(tab); got != "w18" {
		t.Errorf("Brave's window went to %q, want w18, the place the user opened it for", got)
	}
	for _, id := range []string{"w6", "w17"} {
		if !h.item(id).Hidden {
			t.Errorf("%s should still wait for its window, hidden", id)
		}
	}
	if p := quitPane(h, "p14"); p.Active != "w18" || h.frame(tab) != h.bodyOf("p14") {
		t.Errorf("p14 should show Brave's window: active %q, frame %v, body %v", p.Active, h.frame(tab), h.bodyOf("p14"))
	}
}

// TestOpenItemWithFewerWindowsThanPlaces checks Open for an app that, as
// it opens, restores fewer windows than it has places: a restored window
// goes back to the place with its title, the clicked place still gets a
// window, which Bench asks the app for, and the place nothing came for
// stays out of sight.
func TestOpenItemWithFewerWindowsThanPlaces(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("Wails v3 docs"))
	}})
	quitBrave(h)
	mustOK(t, h.e.SwitchTab(2))
	h.p.ResetCalls()
	mustOK(t, h.e.OpenItem("w18"))
	h.drain()
	if got := h.itemOf(h.windowID(braveID, "Wails v3 docs")); got != "w6" {
		t.Errorf("the restored \"Wails v3 docs\" went to %q, want w6, which had that title", got)
	}
	if it := h.item("w18"); it.Hidden || it.Status != StatusLaunching {
		t.Errorf("w18 should show, waiting for Brave: hidden %v, %s %q", it.Hidden, it.Status, it.StatusText)
	}
	h.advance(5 * time.Second)
	asked := quitAsked(h, braveID)
	if len(asked) != 2 || asked[0].Method != "OpenApp" || asked[1].Method != "NewWindow" {
		t.Errorf("Bench should open Brave, then ask it for a new window for w18: %v", asked)
	}
	if got := h.itemOf(h.windowID(braveID, platform.DefaultNewWindowTitle)); got != "w18" {
		t.Errorf("Brave's new window went to %q, want w18", got)
	}
	if !h.item("w17").Hidden {
		t.Errorf("w17 got no window, so it should stay hidden")
	}
	if p := quitPane(h, "p14"); p.Active != "w18" {
		t.Errorf("p14 shows %q, want w18, which the user opened", p.Active)
	}
}

// TestOpenItemAsksARunningAppForANewWindow checks Open for a place of an
// app that's running with fewer windows than places: Bench asks it for a
// new window, which goes to the clicked place, not the place that comes
// first by the order the app's windows were made in, and shows it there.
func TestOpenItemAsksARunningAppForANewWindow(t *testing.T) {
	h := newHarness(t, setup{})
	quitBrave(h)
	mustOK(t, h.e.SwitchTab(2))
	// The user opens Brave, which restores one of its three windows.
	h.p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("Go packages"))
	mustOK(t, h.p.LaunchApp(braveID))
	h.drain()
	if got := h.itemOf(h.windowID(braveID, "Go packages")); got != "w18" {
		t.Fatalf("the restored \"Go packages\" went to %q, want w18", got)
	}
	h.p.ResetCalls()
	mustOK(t, h.e.OpenItem("w17"))
	h.drain()
	if asked := quitAsked(h, braveID); len(asked) != 1 || asked[0].Method != "NewWindow" {
		t.Errorf("Open should ask the running Brave for a new window: %v", asked)
	}
	w := h.windowID(braveID, platform.DefaultNewWindowTitle)
	if got := h.itemOf(w); got != "w17" {
		t.Errorf("Brave's new window went to %q, want w17", got)
	}
	if !h.item("w6").Hidden {
		t.Errorf("w6 should still wait for its window, hidden")
	}
	if p := quitPane(h, "p14"); p.Active != "w17" || h.frame(w) != h.bodyOf("p14") {
		t.Errorf("p14 should show the new window: active %q, frame %v", p.Active, h.frame(w))
	}
}

// TestOpenItemWithPlacesInTwoTabs checks Open for an app with places in two
// tabs: the window goes to the clicked place in the current tab, not to the
// app's earlier place in another tab, out of sight; and Open refuses that
// other place while its tab isn't the current one.
func TestOpenItemWithPlacesInTwoTabs(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("New Tab"))
	}})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.MoveItemToTab("w6", 1))
	quitBrave(h)
	h.p.ResetCalls()
	if err := h.e.OpenItem("w6"); err == nil {
		t.Errorf("Open should refuse w6, which is in Code, not Research")
	}
	if asked := quitAsked(h, braveID); len(asked) != 0 {
		t.Fatalf("a refused Open asked Brave: %v", asked)
	}
	mustOK(t, h.e.OpenItem("w17"))
	h.drain()
	if got := h.itemOf(h.windowID(braveID, "New Tab")); got != "w17" {
		t.Errorf("Brave's window went to %q, want w17, not w6 in Code", got)
	}
	if it := h.item("w6"); it.Tab != 1 || !it.Hidden {
		t.Errorf("w6 should still wait in Code, hidden: tab %d, hidden %v", it.Tab, it.Hidden)
	}
}

// TestOpenItemRefusesItemsOutsideTheCurrentTab checks that Open refuses an
// item that doesn't exist, or that isn't in a pane of the current tab,
// without opening anything or forgetting that the user quit the app.
func TestOpenItemRefusesItemsOutsideTheCurrentTab(t *testing.T) {
	h := newHarness(t, setup{})
	quitBrave(h)
	h.p.ResetCalls()
	if err := h.e.OpenItem("w99"); err == nil {
		t.Errorf("Open should refuse an item that doesn't exist")
	}
	// Brave's places are in Research, but Comms is the current tab.
	if err := h.e.OpenItem("w6"); err == nil {
		t.Errorf("Open should refuse a place in another tab")
	}
	if asked := quitAsked(h, braveID); len(asked) != 0 {
		t.Errorf("a refused Open asked Brave: %v", asked)
	}
	if v := h.view(); v.Current != 0 || !v.Items["w6"].Hidden {
		t.Errorf("a refused Open changed the view: current %d, w6 hidden %v", v.Current, v.Items["w6"].Hidden)
	}
	if a := quitSavedApps(h)[quitBraveKey]; !a.Quit {
		t.Errorf("a refused Open forgot Brave's quit: %+v", a)
	}
}

// TestOpenItemWithAWindowShowsIt checks that Open for an item that has a
// window only shows it in its pane, docked and focused, without asking its
// app for anything.
func TestOpenItemWithAWindowShowsIt(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.p.ResetCalls()
	mustOK(t, h.e.OpenItem("w4"))
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	if p := quitPane(h, "p11"); p.Active != "w4" {
		t.Errorf("p11 shows %q, want w4", p.Active)
	}
	if z := h.p.ZOrder(); h.frame(code) != h.bodyOf("p11") || slices.Index(z, code) > slices.Index(z, zed) {
		t.Errorf("VS Code should be docked in front of Zed: frame %v, z-order %v", h.frame(code), z)
	}
	if !slices.Equal(h.focusCalls(), []platform.WindowID{code}) {
		t.Errorf("focus: %v, want VS Code's window", h.focusCalls())
	}
	if asked := h.calls("OpenApp", "NewWindow"); len(asked) != 0 {
		t.Errorf("Open asked for windows that are there already: %v", asked)
	}
}

// TestOpenItemSaysWhyItCouldNotOpen checks Open's refusals when the app
// doesn't open: one that macOS says isn't installed is marked so, and its
// pane says it can't be opened; any other failure says Bench couldn't open
// it, and the pane offers Open again.
func TestOpenItemSaysWhyItCouldNotOpen(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(h *harness)
		want    string // OpenItem's refusal.
		text    string // The place's status text.
		canOpen bool
	}{
		{
			name: "not installed",
			prepare: func(h *harness) {
				h.p.AddApp(braveID, "Brave", platform.AppNotRunning(), platform.AppNotInstalled())
			},
			want: "Bench couldn't open Brave, because it isn't installed.",
			text: "Brave isn't installed",
		},
		{
			name:    "failed",
			prepare: func(h *harness) { h.p.SetErr("OpenApp", errors.New("the app is damaged")) },
			want:    "Bench couldn't open Brave.",
			text:    "Bench couldn't open Brave",
			canOpen: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			quitBrave(h)
			mustOK(t, h.e.SwitchTab(2))
			tc.prepare(h)
			mustErr(t, h.e.OpenItem("w6"), tc.want)
			h.drain()
			if it := h.item("w6"); !it.Hidden || it.StatusText != tc.text {
				t.Errorf("w6: hidden %v, %q; want hidden, %q", it.Hidden, it.StatusText, tc.text)
			}
			rem := quitPane(h, "p14").Remembered
			if len(rem) != 1 || rem[0].BundleID != braveID || rem[0].Text != tc.text || rem[0].CanOpen != tc.canOpen {
				t.Errorf("p14's remembered apps: %+v, want Brave saying %q, can open %v", rem, tc.text, tc.canOpen)
			}
		})
	}
}

// TestOpenItemWithoutAccessibilityJustOpensTheApp checks Open while Bench
// lacks Accessibility permission: it can't place the app's windows, but it
// opens the app, and forgets that the user quit it.
func TestOpenItemWithoutAccessibilityJustOpensTheApp(t *testing.T) {
	data := savedState(t, func(h *harness) {
		mustOK(t, h.e.SwitchTab(2))
		quitBrave(h)
	})
	if !quitAppsOf(t, data)[quitBraveKey].Quit {
		t.Fatal("the saved state should record Brave's quit")
	}
	h := newHarness(t, setup{untrusted: true, state: writeState(data), world: func(p *platform.Fake) {
		standardWorld(p)
		// Brave isn't running, and takes a while to open.
		p.AddApp(braveID, "Brave", platform.AppNotRunning())
		p.InstallApp(braveID, "Brave", platform.AppLaunchDelay(10*time.Second))
	}})
	h.p.ResetCalls()
	mustOK(t, h.e.OpenItem("w6"))
	if asked := quitAsked(h, braveID); len(asked) != 1 || asked[0].Method != "OpenApp" || asked[0].Arg != "" {
		t.Errorf("Open should only open Brave: %v", asked)
	}
	if a := quitSavedApps(h)[quitBraveKey]; a.Quit {
		t.Errorf("Open should forget Brave's quit: %+v", a)
	}
	if h.p.IsRunning(braveID) {
		t.Errorf("Brave opened sooner than the test expects: its launch, not Open, may have forgotten the quit")
	}
}

// TestOpenItemWithoutAccessibilitySaysWhyItCouldNotOpen checks that Open,
// while Bench lacks Accessibility permission, still says when macOS says
// the app isn't installed, and remembers it.
func TestOpenItemWithoutAccessibilitySaysWhyItCouldNotOpen(t *testing.T) {
	const config = `
[[tab]]
name = "Notes"
layout = "single"
  [[tab.pane]]
  items = [{ app = "notion.id" }]
`
	h := newHarness(t, setup{untrusted: true, config: config})
	id := h.view().Display.Panes[0].Items[0]
	mustErr(t, h.e.OpenItem(id), "Bench couldn't open Notion, because it isn't installed.")
	if it := h.item(id); it.StatusText != "Notion isn't installed" {
		t.Errorf("Notion's item says %q, want that it isn't installed", it.StatusText)
	}
}
