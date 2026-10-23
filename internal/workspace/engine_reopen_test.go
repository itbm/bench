package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// These tests follow apps as they open again (reopen.go). A remembered place
// (an App item without a window that had one) takes a window of its app by
// creation order only while the app is opening again: for 4 launch
// timeouts after Bench's scan, 2 after the app launches, and 2 after each
// of its new windows. Then the places it didn't fill are forgotten, apart
// from one for each App entry of the config in each pane, so that later
// windows go where new_windows says. With the example config, the launch
// timeout is 15 s: a minute after the scan, and half a minute after a
// launch or a window.

// reopenTimeout is the example config's launch_timeout.
const reopenTimeout = 15 * time.Second

// reopenStaleAfter is how long an app can go without running before its
// places are forgotten.
const reopenStaleAfter = 30 * 24 * time.Hour

// reopenConfig has a tab whose second pane the config leaves empty, for
// the user to fill, and a second tab with an entry for Brave too. Bench
// doesn't open apps for a tab, so that only the apps themselves fill
// places.
const reopenConfig = `
[settings]
launch_timeout = "15s"
open_missing_apps = false

[[tab]]
name = "Web"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
  [[tab.pane]]
  items = []

[[tab]]
name = "Docs"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
`

// reopenBraveWorld is a world with only Brave running, with windows titled
// titles.
func reopenBraveWorld(titles ...string) func(p *platform.Fake) {
	return func(p *platform.Fake) {
		p.AddApp(braveID, "Brave")
		for _, title := range titles {
			p.AddWindow(braveID, title)
		}
	}
}

// reopenBraveWith is the standard world after Brave quit and opened again
// while Bench was closed, with new windows titled titles.
func reopenBraveWith(titles ...string) func(p *platform.Fake) {
	return func(p *platform.Fake) {
		standardWorld(p)
		p.QuitApp(braveID)
		p.AddApp(braveID, "Brave")
		for _, title := range titles {
			p.AddWindow(braveID, title)
		}
	}
}

// reopenRestart closes Bench, has between change the world while it's
// closed (nobody sees the events that sends), and starts Bench again on the
// same Mac, clock, and support folder, as after a login. It returns when
// Bench started, and scanned the windows if it may.
func reopenRestart(h *harness, between func()) time.Time {
	h.t.Helper()
	h.drain()
	mustOK(h.t, h.e.Close())
	if between != nil {
		between()
	}
	h.p.DrainEvents()
	e, err := New(Options{
		Platform: h.p, Clock: h.clk, Emitter: h.em, Dir: h.dir,
		NewTerms: func(cb term.Callbacks) term.Host { h.terms.cb = cb; return h.terms },
	})
	if err != nil {
		h.t.Fatalf("New: %v", err)
	}
	e.manual = true
	e.ownPID = h.bench.PID
	h.e = e
	h.t.Cleanup(func() { _ = e.Close() })
	area := paneArea
	content := h.p.ContentFrame()
	area.W, area.H = content.W, content.H-75
	mustOK(h.t, e.SetPaneArea(area))
	started := h.clk.Now()
	h.start()
	return started
}

// reopenAdvanceTo moves the clock on to at, in small steps, feeding events
// as it goes.
func reopenAdvanceTo(h *harness, at time.Time) {
	if d := at.Sub(h.clk.Now()); d > 0 {
		h.advance(d)
	}
}

// reopenSkip moves the clock on by d in one go, running the timers that
// fall due, and then feeds the events: for days, which small steps would
// take too long to cover.
func reopenSkip(h *harness, d time.Duration) {
	h.drain()
	h.clk.Advance(d)
	h.drain()
}

// reopenLeft returns which of the items Bench still has, shown or hidden.
func reopenLeft(h *harness, ids ...string) []string {
	v := h.view()
	var out []string
	for _, id := range ids {
		if _, ok := v.Items[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// reopenReadState reads state.json.
func reopenReadState(t *testing.T, dir string) config.State {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, config.StateFile))
	if err != nil {
		t.Fatalf("state.json: %v", err)
	}
	var st config.State
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("state.json: %v", err)
	}
	return st
}

// reopenDropApps takes what Bench remembers about apps out of state.json,
// as an older Bench would have written it.
func reopenDropApps(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, config.StateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("state.json: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("state.json: %v", err)
	}
	if _, ok := raw["apps"]; !ok {
		t.Fatalf("state.json has no apps to take out:\n%s", data)
	}
	delete(raw, "apps")
	if data, err = json.Marshal(raw); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestReopenForgetsPlacesTheAppDidNotFill checks that after Bench restarts,
// a running app has a minute (4 launch timeouts from the scan) to fill its
// remembered places, and that the places it didn't fill are then
// forgotten: out of sight, they'd claim the app's later windows. The pane
// and the tab's layout stay as they were, here and in state.json.
func TestReopenForgetsPlacesTheAppDidNotFill(t *testing.T) {
	data := savedState(t, nil)
	// Brave restarted while Bench was closed, with one of its three windows.
	h := newHarness(t, setup{state: writeState(data), world: reopenBraveWith("Go packages"), noStart: true})
	scan := h.clk.Now()
	h.start()
	if got := h.itemOf(h.windowID(braveID, "Go packages")); got != "w18" {
		t.Fatalf("Go packages: item %q, want w18 by its title", got)
	}
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout-time.Second))
	for _, id := range []string{"w6", "w17"} {
		if it, ok := h.view().Items[id]; !ok || !it.Hidden || it.Pane != "p14" {
			t.Errorf("%s just before the minute is up: %+v (there %v), want a hidden place in p14", id, it, ok)
		}
	}
	h.advance(2 * time.Second)
	if left := reopenLeft(h, "w6", "w17"); len(left) != 0 {
		t.Errorf("%v should be forgotten once Brave has had its minute", left)
	}
	research := h.tabNamed("Research")
	if research.Preset != layout.PresetSide || !slices.Equal(leafIDs(research), []string{"p14", "p15"}) {
		t.Errorf("Research: preset %s, panes %v; want side with p14 and p15", research.Preset, leafIDs(research))
	}
	if got := layout.Leaves(research.Root)[0].Items; !slices.Equal(got, []string{"w18"}) {
		t.Errorf("p14: %v, want w18 alone", got)
	}
	mustOK(t, h.e.Close())
	st := reopenReadState(t, h.dir)
	for _, id := range []string{"w6", "w17"} {
		if _, ok := st.Items[id]; ok {
			t.Errorf("state.json still has %s", id)
		}
	}
	if st.Tabs[2].Preset != layout.PresetSide {
		t.Errorf("state.json: Research's preset %s, want side", st.Tabs[2].Preset)
	}
}

// TestReopenKeepsEmptyPanesAndTheLayout checks that forgetting the places
// an app didn't fill leaves their panes in place, even one left empty, and
// doesn't mark the tab custom: only the user's own changes do that, and the
// config's claims follow the panes' places.
func TestReopenKeepsEmptyPanesAndTheLayout(t *testing.T) {
	h := newHarness(t, setup{config: reopenConfig, world: reopenBraveWorld("News", "Mail")})
	panes := leafIDs(h.tabNamed("Web"))
	if len(panes) != 2 {
		t.Fatalf("Web's panes: %v", panes)
	}
	news := h.itemOf(h.windowID(braveID, "News"))
	mail := h.itemOf(h.windowID(braveID, "Mail"))
	// The user asks Brave for a window in the pane the config left empty.
	mustOK(t, h.e.AddToPane(panes[1], Source{Type: sourceNewWindow, BundleID: braveID}))
	h.drain()
	added := h.itemOf(h.windowID(braveID, platform.DefaultNewWindowTitle))
	if added == "" || h.paneOf(added) != panes[1] {
		t.Fatalf("the new window went to %q, want an item in %s", added, panes[1])
	}
	// Brave quits while Bench is closed, and opens again with only News.
	scan := reopenRestart(h, func() {
		h.p.QuitApp(braveID)
		h.p.AddApp(braveID, "Brave")
		h.p.AddWindow(braveID, "News")
	})
	if got := h.itemOf(h.windowID(braveID, "News")); got != news {
		t.Fatalf("News: item %q, want %s", got, news)
	}
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout-time.Second))
	if left := reopenLeft(h, mail, added); len(left) != 2 {
		t.Errorf("before the minute is up, only %v of %s and %s are left", left, mail, added)
	}
	h.advance(2 * time.Second)
	if left := reopenLeft(h, mail, added); len(left) != 0 {
		t.Errorf("%v should be forgotten", left)
	}
	web := h.tabNamed("Web")
	if web.Preset != layout.PresetSide || !slices.Equal(leafIDs(web), panes) {
		t.Errorf("Web: preset %s, panes %v; want side with %v", web.Preset, leafIDs(web), panes)
	}
	if got := layout.Leaves(web.Root)[1].Items; len(got) != 0 {
		t.Errorf("%s: %v, want it empty", panes[1], got)
	}
	shown := slices.ContainsFunc(h.view().Display.Panes, func(p PaneView) bool { return p.ID == panes[1] && len(p.Items) == 0 })
	if !shown {
		t.Errorf("the empty pane %s should still be displayed", panes[1])
	}
}

// TestReopenNeverForgetsConfigPlaceholders checks that an App item that
// never had a window, which stands for a config entry, isn't forgotten
// with the places the app didn't fill: here the Docs tab's entry for
// Brave, in a pane where a Brave window stands for the entry as well, so
// that it isn't kept as the entry's anchor either.
func TestReopenNeverForgetsConfigPlaceholders(t *testing.T) {
	h := newHarness(t, setup{config: reopenConfig, world: reopenBraveWorld("News", "Mail")})
	docs := layout.Items(h.tabNamed("Docs").Root)
	if len(docs) != 1 || !h.item(docs[0]).NoWindow {
		t.Fatalf("Docs: %v, want Brave's entry without a window", docs)
	}
	placeholder := docs[0]
	news := h.itemOf(h.windowID(braveID, "News"))
	mail := h.itemOf(h.windowID(braveID, "Mail"))
	// The user shows the Mail window in Docs too.
	mustOK(t, h.e.AddItemToTab(mail, 1))
	scan := reopenRestart(h, func() {
		h.p.QuitApp(braveID)
		h.p.AddApp(braveID, "Brave")
		h.p.AddWindow(braveID, "Mail")
	})
	if got := h.itemOf(h.windowID(braveID, "Mail")); got != mail {
		t.Fatalf("Mail: item %q, want %s", got, mail)
	}
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
	if left := reopenLeft(h, news); len(left) != 0 {
		t.Errorf("News's place should be forgotten")
	}
	reopenSkip(h, 10*time.Minute)
	if it, ok := h.view().Items[placeholder]; !ok || !it.Hidden || it.Tab != 1 {
		t.Errorf("Docs's entry %s: %+v (there %v), want it kept, out of sight", placeholder, it, ok)
	}
	if got := layout.Items(h.tabNamed("Docs").Root); !slices.Equal(got, []string{placeholder, mail}) {
		t.Errorf("Docs: %v, want %s and %s", got, placeholder, mail)
	}
}

// reopenAnchorRestart restarts Bench after the user added a Brave window
// to Code's p12, and Brave quit and opened again with only that window:
// Brave's three places in Research's p14, which the config's Brave entry
// maps to, wait for windows. It returns the harness, when Bench scanned,
// and the item in Code.
func reopenAnchorRestart(t *testing.T) (*harness, time.Time, string) {
	t.Helper()
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.AddToPane("p12", Source{Type: sourceNewWindow, BundleID: braveID}))
	h.drain()
	code := h.itemOf(h.windowID(braveID, platform.DefaultNewWindowTitle))
	if code == "" || h.paneOf(code) != "p12" {
		t.Fatalf("the new window went to %q, want an item in p12", code)
	}
	scan := reopenRestart(h, func() {
		h.p.QuitApp(braveID)
		h.p.AddApp(braveID, "Brave")
		h.p.AddWindow(braveID, platform.DefaultNewWindowTitle)
	})
	if got := h.itemOf(h.windowID(braveID, platform.DefaultNewWindowTitle)); got != code {
		t.Fatalf("Brave's window went to %q, want %s", got, code)
	}
	return h, scan, code
}

// TestReopenKeepsOneAnchorPerConfigEntry checks that when Brave doesn't
// fill its places, one of them stays in the pane its config entry maps to,
// as the entry's place, so that Bench can open Brave with the tab; the
// rest are forgotten.
func TestReopenKeepsOneAnchorPerConfigEntry(t *testing.T) {
	h, scan, _ := reopenAnchorRestart(t)
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
	left := reopenLeft(h, "w6", "w17", "w18")
	if len(left) != 1 {
		t.Fatalf("left %v, want one place for Research's Brave entry", left)
	}
	if it := h.item(left[0]); it.Pane != "p14" || !it.Hidden {
		t.Errorf("the entry's place %s: pane %s, hidden %v; want it out of sight in p14", left[0], it.Pane, it.Hidden)
	}
	research := h.tabNamed("Research")
	if research.Preset != layout.PresetSide || !slices.Equal(leafIDs(research), []string{"p14", "p15"}) {
		t.Errorf("Research: preset %s, panes %v; want side with p14 and p15", research.Preset, leafIDs(research))
	}
	// It stays: nothing else is forgotten later.
	reopenSkip(h, 10*time.Minute)
	if got := reopenLeft(h, "w6", "w17", "w18"); !slices.Equal(got, left) {
		t.Errorf("ten minutes on, left %v, want %v", got, left)
	}
}

// TestReopenAnchorFallsBackToTheTabsFirstPlace checks that when the pane a
// config entry maps to has none of the entry's items, because the user
// moved them, the tab's first one keeps the entry's place, wherever it is;
// unless a window in the tab stands for the entry already.
func TestReopenAnchorFallsBackToTheTabsFirstPlace(t *testing.T) {
	for _, tc := range []struct {
		name string
		// The tab and pane where the user adds the Brave window that Bench
		// knows after the restart, and the tab's name.
		tab          int
		pane, window string
		want         []string
	}{
		{"no Brave window in Research", 1, "p12", "Code", []string{"w6"}},
		{"a Brave window elsewhere in Research", 2, "p15", "Research", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			mustOK(t, h.e.SwitchTab(2))
			// p14, where Research's Brave entry maps to, gets a terminal, and
			// Brave's windows move to p15, after Obsidian's.
			mustOK(t, h.e.AddToPane("p14", Source{Type: sourceTerminal}))
			for _, id := range []string{"w6", "w17", "w18"} {
				mustOK(t, h.e.MoveItem(id, 2, "p15", "center"))
			}
			if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got, []string{"p14", "p15"}) {
				t.Fatalf("Research's panes: %v", got)
			}
			mustOK(t, h.e.SwitchTab(tc.tab))
			mustOK(t, h.e.AddToPane(tc.pane, Source{Type: sourceNewWindow, BundleID: braveID}))
			h.drain()
			if it := h.itemOf(h.windowID(braveID, platform.DefaultNewWindowTitle)); it == "" || h.paneOf(it) != tc.pane {
				t.Fatalf("the new window went to %q, want an item in %s", it, tc.pane)
			}
			scan := reopenRestart(h, func() {
				h.p.QuitApp(braveID)
				h.p.AddApp(braveID, "Brave")
				h.p.AddWindow(braveID, platform.DefaultNewWindowTitle)
			})
			reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
			if got := reopenLeft(h, "w6", "w17", "w18"); !slices.Equal(got, tc.want) {
				t.Errorf("with the window in %s, left %v, want %v", tc.window, got, tc.want)
			}
			for _, id := range tc.want {
				if p := h.paneOf(id); p != "p15" {
					t.Errorf("%s is in %s, want p15, where the user put it", id, p)
				}
			}
		})
	}
}

// TestReopenNewWindowsAfterThePeriod checks that once Brave's period is
// over, a new window (as from Cmd+N) goes where new_windows says, not to an
// old place by creation order, while a window with a place's exact title
// still fills it, however late.
func TestReopenNewWindowsAfterThePeriod(t *testing.T) {
	h, scan, code := reopenAnchorRestart(t)
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
	left := reopenLeft(h, "w6", "w17", "w18")
	if len(left) != 1 {
		t.Fatalf("left %v, want the Brave entry's place", left)
	}
	anchor := left[0]
	reopenSkip(h, 5*time.Minute)
	w, err := h.p.OpenWindow(braveID, "Release notes")
	mustOK(t, err)
	h.drain()
	// new_windows is app-pane: the pane of Brave's most recently used
	// window, in Code.
	if got := h.itemOf(w.ID); got == "" || got == anchor || h.paneOf(got) != h.paneOf(code) {
		t.Errorf("Cmd+N's window went to %q, want a new item in %s", got, h.paneOf(code))
	}
	if !h.item(anchor).NoWindow {
		t.Errorf("%s took Cmd+N's window by creation order", anchor)
	}
	title := h.item(anchor).Title
	w, err = h.p.OpenWindow(braveID, title)
	mustOK(t, err)
	h.drain()
	if got := h.itemOf(w.ID); got != anchor {
		t.Errorf("a window titled %q went to %q, want %s, whose title it has", title, got, anchor)
	}
}

// TestReopenForgetsNothingWhileTheAppIsClosed checks that an app that quits
// before its period ends keeps all its places, however long it stays
// closed: they wait for it to open again. That holds too when macOS hasn't
// reported the quit yet, and Bench still takes a window of the app to be
// there.
func TestReopenForgetsNothingWhileTheAppIsClosed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reported bool
	}{
		{"its quit reported", true},
		{"its quit not reported yet", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := savedState(t, nil)
			h := newHarness(t, setup{state: writeState(data), world: reopenBraveWith("Go packages"), noStart: true})
			scan := h.clk.Now()
			h.start()
			reopenAdvanceTo(h, scan.Add(2*reopenTimeout))
			mustOK(t, h.p.QuitApp(braveID))
			if tc.reported {
				h.drain()
			} else {
				h.p.DrainEvents()
			}
			reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
			if got := reopenLeft(h, "w6", "w17"); len(got) != 2 {
				t.Errorf("as the minute ends, left %v, want w6 and w17 while Brave is closed", got)
			}
			reopenSkip(h, 10*time.Minute)
			if got := reopenLeft(h, "w6", "w17"); len(got) != 2 {
				t.Errorf("ten minutes on, left %v, want w6 and w17 while Brave is closed", got)
			}
		})
	}
}

// TestReopenAfterALaunchLastsTwoLaunchTimeouts checks that when an app the
// user quit opens again, Bench gives it 2 launch timeouts from the launch
// to fill its places, its windows filling them by creation order, and then
// forgets the places it didn't fill. A window that a rule floats doesn't
// fill a place, nor keep the period going, but it's one Bench knows.
func TestReopenAfterALaunchLastsTwoLaunchTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		title string // Brave's window as it opens again.
		fills string // The place it fills, if any.
	}{
		// Once the period is over, w6 is left in p14, with the window.
		{"a restored window", "New Tab", "w6"},
		// Once the period is over, the Brave entry's place is left in p14.
		{"only a floating window", "Picture in picture", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, setup{})
			mustOK(t, h.p.QuitApp(braveID))
			h.drain()
			reopenSkip(h, time.Hour)
			if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
				t.Fatalf("left %v while Brave is closed, want all three", got)
			}
			h.p.InstallApp(braveID, "Brave", platform.AppLaunchWindows(tc.title))
			launch := h.clk.Now()
			mustOK(t, h.p.LaunchApp(braveID))
			h.drain()
			if got := h.itemOf(h.windowID(braveID, tc.title)); got != tc.fills {
				t.Errorf("%s went to %q, want %q", tc.title, got, tc.fills)
			}
			reopenAdvanceTo(h, launch.Add(2*reopenTimeout-time.Second))
			if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
				t.Errorf("before 2 launch timeouts, left %v, want all three", got)
			}
			h.advance(2 * time.Second)
			got := reopenLeft(h, "w6", "w17", "w18")
			if len(got) != 1 || h.paneOf(got[0]) != "p14" || (tc.fills != "" && got[0] != tc.fills) {
				t.Fatalf("after 2 launch timeouts, left %v, want one item in p14", got)
			}
			if tc.fills != "" && h.item(tc.fills).NoWindow {
				t.Errorf("%s lost its window", tc.fills)
			}
		})
	}
}

// TestReopenScanMinuteCoversAppsThatOpenLater checks that an app that
// isn't running when Bench starts, and opens soon after, as at a login,
// has until the end of the minute after the scan to fill its places, even
// if its own 2 launch timeouts end sooner.
func TestReopenScanMinuteCoversAppsThatOpenLater(t *testing.T) {
	data := savedState(t, nil)
	h := newHarness(t, setup{state: writeState(data), noStart: true, world: func(p *platform.Fake) {
		standardWorld(p)
		p.QuitApp(braveID)
		p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("New Tab"))
	}})
	scan := h.clk.Now()
	h.start()
	reopenAdvanceTo(h, scan.Add(10*time.Second))
	mustOK(t, h.p.LaunchApp(braveID))
	h.drain()
	// Brave's own period would end 2 launch timeouts after its launch.
	reopenAdvanceTo(h, scan.Add(10*time.Second+2*reopenTimeout+5*time.Second))
	if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
		t.Errorf("within the minute after the scan, left %v, want all three", got)
	}
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
	if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 1 {
		t.Errorf("after the minute, left %v, want only the place that New Tab filled", got)
	}
}

// TestReopenPeriodsHaveAMinimum checks that with a short launch_timeout,
// the periods last at least as long as with one of 15 seconds: an app
// that opens again has half a minute to fill its places.
func TestReopenPeriodsHaveAMinimum(t *testing.T) {
	cfg := strings.Replace(string(config.ExampleTOML()), `launch_timeout = "15s"`, `launch_timeout = "2s"`, 1)
	if !strings.Contains(cfg, `launch_timeout = "2s"`) {
		t.Fatal("the example config has no launch_timeout to change")
	}
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	reopenSkip(h, time.Hour)
	h.p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("New Tab"))
	launch := h.clk.Now()
	mustOK(t, h.p.LaunchApp(braveID))
	h.drain()
	reopenAdvanceTo(h, launch.Add(20*time.Second))
	if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
		t.Errorf("20 seconds after the launch, left %v, want all three", got)
	}
	reopenAdvanceTo(h, launch.Add(2*reopenTimeout+time.Second))
	if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 1 {
		t.Errorf("after half a minute, left %v, want one", got)
	}
}

// TestReopenWaitsForAccessibility checks that nothing is forgotten while
// Bench can't see windows, and that the period starts when the permission
// arrives, with the scan that follows it.
func TestReopenWaitsForAccessibility(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	// Bench restarts without permission, and Brave opens again meanwhile
	// with one of its three windows.
	reopenRestart(h, func() {
		h.p.SetTrusted(false)
		h.p.AddApp(braveID, "Brave")
		h.p.AddWindow(braveID, "Go packages")
	})
	reopenSkip(h, 10*time.Minute)
	if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
		t.Fatalf("without permission, left %v, want all three places", got)
	}
	h.p.SetTrusted(true)
	h.advance(trustEvery + 100*time.Millisecond)
	granted := h.clk.Now()
	if got := h.itemOf(h.windowID(braveID, "Go packages")); got != "w18" {
		t.Fatalf("Go packages: item %q, want w18 by its title once Bench can see it", got)
	}
	// The scan came within the last trustEvery.
	reopenAdvanceTo(h, granted.Add(4*reopenTimeout-trustEvery-time.Second))
	if got := reopenLeft(h, "w6", "w17"); len(got) != 2 {
		t.Errorf("within a minute of the permission, left %v, want w6 and w17", got)
	}
	reopenAdvanceTo(h, granted.Add(4*reopenTimeout+time.Second))
	if got := reopenLeft(h, "w6", "w17"); len(got) != 0 {
		t.Errorf("a minute after the permission, %v should be forgotten", got)
	}
}

// TestReopenForgetsNothingWhileTheUserHidesTheApp checks that nothing is
// forgotten when the period ends while the user has hidden the app, whose
// windows the Accessibility API can leave out, and that the next window
// Bench sees starts the period again.
func TestReopenForgetsNothingWhileTheUserHidesTheApp(t *testing.T) {
	// Without hide_other_tabs, Bench leaves Brave as the user left it.
	cfg := strings.Replace(string(config.ExampleTOML()), "hide_other_tabs = true", "hide_other_tabs = false", 1)
	data := savedState(t, nil)
	h := newHarness(t, setup{config: cfg, state: writeState(data), noStart: true, world: func(p *platform.Fake) {
		reopenBraveWith("Go packages")(p)
		p.AddApp(braveID, "Brave", platform.AppHidden()) // The user hid it with Cmd+H.
	}})
	scan := h.clk.Now()
	h.start()
	if !h.p.IsHidden(braveID) || h.itemOf(h.windowID(braveID, "Go packages")) != "w18" {
		t.Fatalf("Brave should be hidden, with Go packages in w18")
	}
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
	reopenSkip(h, 10*time.Minute)
	if got := reopenLeft(h, "w6", "w17"); len(got) != 2 {
		t.Fatalf("while Brave is hidden, left %v, want w6 and w17", got)
	}
	// The user shows Brave and opens a window.
	h.p.AddApp(braveID, "Brave")
	w, err := h.p.OpenWindow(braveID, "Later")
	mustOK(t, err)
	h.drain()
	opened := h.clk.Now()
	if got := h.itemOf(w.ID); got != "w6" {
		t.Errorf("the next window went to %q, want w6 by creation order", got)
	}
	reopenAdvanceTo(h, opened.Add(2*reopenTimeout-time.Second))
	if got := reopenLeft(h, "w17"); len(got) != 1 {
		t.Errorf("w17 should wait 2 launch timeouts from the new window")
	}
	h.advance(2 * time.Second)
	if got := reopenLeft(h, "w17"); len(got) != 0 {
		t.Errorf("w17 should be forgotten once the new period is over")
	}
}

// TestReopenLateFirstWindowStartsThePeriodAgain checks that nothing is
// forgotten when the period ends before Bench has seen any of the app's
// windows, and that the first one, however late, fills a place by
// creation order and starts the period again.
func TestReopenLateFirstWindowStartsThePeriodAgain(t *testing.T) {
	h := newHarness(t, setup{})
	// Brave quits while Bench is closed, and opens again with no window
	// yet.
	scan := reopenRestart(h, func() {
		h.p.QuitApp(braveID)
		h.p.AddApp(braveID, "Brave")
	})
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
	reopenSkip(h, 10*time.Minute)
	if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
		t.Fatalf("before Brave has a window, left %v, want all three places", got)
	}
	w, err := h.p.OpenWindow(braveID, "Restored")
	mustOK(t, err)
	h.drain()
	late := h.clk.Now()
	if got := h.itemOf(w.ID); got != "w6" {
		t.Errorf("the late window went to %q, want w6 by creation order", got)
	}
	reopenAdvanceTo(h, late.Add(2*reopenTimeout-time.Second))
	if got := reopenLeft(h, "w17", "w18"); len(got) != 2 {
		t.Errorf("before 2 launch timeouts from the window, left %v, want w17 and w18", got)
	}
	h.advance(2 * time.Second)
	if got := reopenLeft(h, "w17", "w18"); len(got) != 0 {
		t.Errorf("%v should be forgotten 2 launch timeouts after the window", got)
	}
}

// TestReopenEachNewWindowExtendsThePeriod checks that a new window of an
// app that's opening again fills its first place by creation order, in
// whichever pane, rather than going where new_windows says, and keeps the
// period going for 2 launch timeouts from it, past the minute after the
// scan.
func TestReopenEachNewWindowExtendsThePeriod(t *testing.T) {
	// The user moved w6, Brave's first window, to Obsidian's pane: by
	// new_windows (app-pane), Brave's next window would go to p14, where
	// its most recent one is.
	data := savedState(t, func(h *harness) { mustOK(t, h.e.MoveItem("w6", 2, "p15", "center")) })
	h := newHarness(t, setup{state: writeState(data), world: reopenBraveWith("Go packages"), noStart: true})
	scan := h.clk.Now()
	h.start()
	if got := h.itemOf(h.windowID(braveID, "Go packages")); got != "w18" || h.paneOf("w6") != "p15" {
		t.Fatalf("Go packages went to %q, and w6 is in %s; want w18, and p15", got, h.paneOf("w6"))
	}
	reopenAdvanceTo(h, scan.Add(50*time.Second))
	w, err := h.p.OpenWindow(braveID, "Restored late")
	mustOK(t, err)
	h.drain()
	if got := h.itemOf(w.ID); got != "w6" {
		t.Errorf("the window at 50 s went to %q, want w6 by creation order", got)
	}
	reopenAdvanceTo(h, scan.Add(4*reopenTimeout+time.Second))
	if got := reopenLeft(h, "w17"); len(got) != 1 {
		t.Errorf("w17 was forgotten a minute after the scan, despite the window at 50 s")
	}
	reopenAdvanceTo(h, scan.Add(50*time.Second+2*reopenTimeout-time.Second))
	if got := reopenLeft(h, "w17"); len(got) != 1 {
		t.Errorf("w17 was forgotten before 2 launch timeouts after the last window")
	}
	h.advance(2 * time.Second)
	if got := reopenLeft(h, "w17"); len(got) != 0 {
		t.Errorf("w17 should be forgotten 2 launch timeouts after the last window")
	}
}

// TestReopenForgetsStalePlaces checks that the places of an app that
// hasn't run for 30 days while Bench was running are forgotten, as if it had
// opened again (the config entry's place stays), and not before: by the time
// Bench last saw it, which state.json keeps, not counting the time Bench was
// off; or, for an app it has no record of, as with an older Bench's
// state.json, from when Bench starts.
func TestReopenForgetsStalePlaces(t *testing.T) {
	t.Run("seen in state.json", func(t *testing.T) {
		h := newHarness(t, setup{})
		mustOK(t, h.p.QuitApp(braveID))
		h.drain()
		quit := h.clk.Now()
		// Bench runs for 10 days without Brave, then is off for 50 days.
		reopenSkip(h, 10*24*time.Hour)
		start := reopenRestart(h, func() {
			if a := reopenReadState(t, h.dir).Apps[strings.ToLower(braveID)]; !a.Seen.Equal(quit) {
				t.Errorf("state.json: Brave last seen %v, want %v, when it quit", a.Seen, quit)
			}
			h.clk.Advance(50 * 24 * time.Hour)
		})
		if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
			t.Fatalf("after 60 days, 10 of them with Bench running, left %v, want all three places", got)
		}
		// Brave has 20 more days of Bench running.
		reopenSkip(h, start.Add(20*24*time.Hour-time.Minute).Sub(h.clk.Now()))
		if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
			t.Errorf("just before 30 days with Bench running, left %v, want all three places", got)
		}
		reopenSkip(h, time.Hour+2*time.Minute)
		got := reopenLeft(h, "w6", "w17", "w18")
		if len(got) != 1 || h.paneOf(got[0]) != "p14" {
			t.Errorf("after 30 days with Bench running, left %v, want just the Brave entry's place in p14", got)
		}
	})
	t.Run("no record", func(t *testing.T) {
		h := newHarness(t, setup{})
		mustOK(t, h.p.QuitApp(braveID))
		h.drain()
		// Bench restarts 60 days later, from an older Bench's state.json.
		start := reopenRestart(h, func() {
			reopenDropApps(t, h.dir)
			h.clk.Advance(60 * 24 * time.Hour)
		})
		if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
			t.Fatalf("at the start, left %v, want all three places: Brave counts as seen now", got)
		}
		reopenSkip(h, start.Add(reopenStaleAfter-time.Minute).Sub(h.clk.Now()))
		if got := reopenLeft(h, "w6", "w17", "w18"); len(got) != 3 {
			t.Errorf("just before 30 days from the start, left %v, want all three places", got)
		}
		reopenSkip(h, time.Hour+2*time.Minute)
		got := reopenLeft(h, "w6", "w17", "w18")
		if len(got) != 1 || h.paneOf(got[0]) != "p14" {
			t.Errorf("30 days after the start, left %v, want just the Brave entry's place in p14", got)
		}
	})
}

// TestDropFromKeepsThePaneAndTheLayout checks Model.DropFrom, which Bench
// uses for what it tidies away by itself: unlike Take, it leaves the pane
// in place even when it's left empty, and doesn't mark the tab custom; and
// it forgets the item only once nothing holds it, in a tab or floating.
func TestDropFromKeepsThePaneAndTheLayout(t *testing.T) {
	m := defaultModel(t)
	research := m.Tabs[2]
	brave := addAppItem(m, research, research.Pane("p15"), braveID, win(42, braveID, "Go packages", 1))
	research.CompactShown = "w7"
	m.DropFrom("w7", 2)
	m.DropFrom(brave.ID, 2)
	if got := paneItems(research); !equalPanes(got, [][]string{{"w6"}, {}}) {
		t.Errorf("Research's panes: %v, want w6 and an empty p15", got)
	}
	if research.Pane("p15") == nil || research.Preset != layout.PresetSide {
		t.Errorf("Research: p15 there %v, preset %s; want p15 kept and side", research.Pane("p15") != nil, research.Preset)
	}
	if research.CompactShown != "" {
		t.Errorf("the tabs view still shows %q", research.CompactShown)
	}
	if m.Items["w7"] != nil || m.Items[brave.ID] != nil || m.ItemByWindow(42) != nil {
		t.Errorf("items in no tab should be forgotten, with their windows")
	}
	mustValid(t, m)
	// An item in another tab too only leaves this one.
	if added, err := m.AddToTab("w6", 0, "p8"); err != nil || !added {
		t.Fatalf("AddToTab: %v %v", added, err)
	}
	m.DropFrom("w6", 2)
	if m.Items["w6"] == nil || !slices.Equal(m.TabsOf("w6"), []int{0}) {
		t.Errorf("w6 should stay, in Comms only: tabs %v", m.TabsOf("w6"))
	}
	if research.Pane("p14") == nil || len(research.Pane("p14").Items) != 0 || research.Preset != layout.PresetSide {
		t.Errorf("Research: %v, preset %s; want both panes kept, empty", paneItems(research), research.Preset)
	}
	// An item that's floating as well isn't forgotten either.
	m.Floating = append(m.Floating, "w4")
	m.DropFrom("w4", 1)
	if m.Items["w4"] == nil || m.Tabs[1].Preset != layout.PresetMainSide {
		t.Errorf("a floating item was forgotten, or Code's preset is %s", m.Tabs[1].Preset)
	}
	// Dropping an item from a tab that doesn't hold it changes nothing.
	m.DropFrom("w1", 1)
	m.DropFrom("w1", 9)
	if !slices.Equal(m.TabsOf("w1"), []int{0}) || m.Items["w1"] == nil {
		t.Errorf("w1 should be untouched: tabs %v", m.TabsOf("w1"))
	}
	mustValid(t, m)
}
