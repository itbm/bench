package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// These tests cover defects found by a later review: where the keyboard
// focus lands after arranging, following focus, and focus that Bench puts
// off; saving state.json; and parking as the screens change.

// raiseCalls returns the windows Bench raised, in order.
func (h *harness) raiseCalls() []platform.WindowID {
	var out []platform.WindowID
	for _, c := range h.calls("Raise") {
		out = append(out, c.Window)
	}
	return out
}

func TestArrangingRaisesTheFocusedPanesWindowsLast(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	h.advance(time.Second)
	brave := h.windowID(braveID, "Wails v3 docs")
	obsidian := h.windowID(obsidianID, "Daily note")
	for _, tt := range []struct {
		pane string
		last platform.WindowID
	}{
		{"p14", brave},
		{"p15", obsidian},
	} {
		mustOK(t, h.e.FocusPane(tt.pane))
		h.advance(time.Second)
		h.p.ResetCalls()
		// Raising a window of the frontmost app can make it the app's
		// focused window, so the focused pane's windows go last.
		mustOK(t, h.e.Rearrange())
		raised := h.raiseCalls()
		if len(raised) == 0 || raised[len(raised)-1] != tt.last {
			t.Errorf("focus in %s: raised %v, want %d last", tt.pane, raised, tt.last)
		}
	}
}

func TestNewWindowSeenFirstByItsFocusIsFollowed(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.p.QuitApp(zedID))
	h.drain()
	// The user launches Zed from the Dock, from Comms. macOS reports its
	// window's focus before Bench hears of the window any other way.
	mustOK(t, h.p.LaunchApp(zedID))
	w, err := h.p.OpenWindow(zedID, "bench — switch.go")
	mustOK(t, err)
	h.p.DrainEvents()
	zed := w.ID
	h.e.handle(h.focusEvent(zed))
	h.drain()
	if got := h.view().Current; got != 1 {
		t.Fatalf("current: %d, want Code, where Zed's window went", got)
	}
	if h.p.IsHidden(zedID) || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed should be shown, docked: hidden %v, frame %v", h.p.IsHidden(zedID), h.frame(zed))
	}
}

func TestNewWindowSeenFirstByItsFocusWithFollowFocusOff(t *testing.T) {
	h := newHarness(t, setup{config: followConfig(t, "off")})
	mustOK(t, h.p.QuitApp(zedID))
	h.drain()
	mustOK(t, h.p.LaunchApp(zedID))
	w, err := h.p.OpenWindow(zedID, "bench — switch.go")
	mustOK(t, err)
	h.p.DrainEvents()
	zed := w.ID
	h.e.handle(h.focusEvent(zed))
	h.drain()
	if got := h.view().Current; got != 0 || h.itemOf(zed) == "" {
		t.Fatalf("current: %d, item %q: the window should join Code, staying on Comms", got, h.itemOf(zed))
	}
	if !h.p.IsHidden(zedID) {
		t.Error("Zed belongs to another tab, so it should be hidden")
	}
}

func TestFocusInTheTabsViewShowsTheWindow(t *testing.T) {
	h := newHarness(t, setup{screen: laptop, config: `
[settings]
compact_below = 1800 # The laptop is compact, as in the example config.

[[tab]]
name = "Comms"
layout = "side"
small_screens = "tabs"
  [[tab.pane]]
  items = [{ web = "https://app.slack.com/client", name = "Slack" }]
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]
`})
	obsidian := h.windowID(obsidianID, "Daily note")
	if v := h.view(); v.Display.Panes[0].Active != "w1" || !h.p.WebViews()["w1"].Visible {
		t.Fatalf("the one pane should show Slack: %+v", v.Display.Panes)
	}
	h.advance(time.Second)
	// The user switches to Obsidian with Command+Tab. It's its pane's
	// active item already, but the one pane shows Slack.
	h.p.FocusWindow(obsidian)
	h.drain()
	if got := h.view().Display.Panes[0].Active; got != h.itemOf(obsidian) {
		t.Fatalf("the one pane shows %s, want Obsidian's item", got)
	}
	if h.frame(obsidian) != h.bodyOf(layout.CompactPaneID) || h.p.WebViews()["w1"].Visible {
		t.Errorf("Obsidian should be docked in place of Slack: frame %v, Slack shown %v", h.frame(obsidian), h.p.WebViews()["w1"].Visible)
	}
}

func TestTitleChangesDontPutOffSavingState(t *testing.T) {
	h := newHarness(t, setup{})
	path := filepath.Join(h.dir, config.StateFile)
	os.Remove(path)
	mustOK(t, h.e.SwitchTab(1))
	// A page retitles itself every 300 ms, and an app its window.
	zed := h.windowID(zedID, "bench — switch.go")
	for i := range 12 {
		h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w1", Title: "(" + strings.Repeat("!", i+1) + ") Slack"})
		h.p.RetitleWindow(zed, strings.Repeat("*", i+1)+" switch.go")
		h.advance(300 * time.Millisecond)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("state.json should be saved within %v of the switch: %v", saveMaxDelay, err)
	}
	var st config.State
	mustOK(t, json.Unmarshal(data, &st))
	if st.Current != 1 {
		t.Errorf("state: current %d, want Code", st.Current)
	}

	// Nor do other changes, however often they come.
	os.Remove(path)
	for i := range 12 {
		mustOK(t, h.e.FocusPane([]string{"p11", "p12"}[i%2]))
		h.advance(300 * time.Millisecond)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state.json should be saved within %v of the first change: %v", saveMaxDelay, err)
	}

	// Titles alone are saved too, but not soon.
	h.advance(time.Second)
	os.Remove(path)
	for i := range 20 {
		h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w1", Title: strings.Repeat("?", i+1)})
		h.advance(300 * time.Millisecond)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("titles alone shouldn't be saved within %v", 20*300*time.Millisecond)
	}
	h.advance(retitleSaveDelay)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("titles should be saved in the end: %v", err)
	}
}

func TestTabShortcutWhileBenchIsHiddenWaitsUntilItsShown(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.p.ResetCalls()
	// Control+Option+3, for Research, with Bench hidden.
	h.e.Shortcut("tab", "3")
	h.drain()
	if got := h.view().Current; got != 2 {
		t.Fatalf("current: %d, want Research", got)
	}
	if len(h.focusCalls()) != 0 || !h.p.IsHidden(braveID) {
		t.Errorf("nothing should be focused or shown while Bench is hidden: focus %v, Brave hidden %v", h.focusCalls(), h.p.IsHidden(braveID))
	}
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	brave := h.windowID(braveID, "Wails v3 docs")
	if got := h.focusCalls(); !slices.Equal(got, []platform.WindowID{brave}) || h.frame(brave) != h.bodyOf("p14") {
		t.Errorf("once Bench is shown, Research's focused window should be docked and focused: focus %v, frame %v", got, h.frame(brave))
	}
}

func TestOpeningARunningAppForAPlaceholderParksItsOtherWindows(t *testing.T) {
	cfg := strings.Replace(exampleConfig(t), `items = [{ terminal = "~/code" }]`,
		`items = [{ terminal = "~/code" }, { app = "com.brave.Browser", title = "localhost" }]`, 1)
	h := newHarness(t, setup{config: cfg})
	mustOK(t, h.e.SwitchTab(2))
	h.advance(time.Second)
	mustOK(t, h.e.SwitchTab(1))
	h.drain()
	if !h.called("OpenApp", func(c platform.Call) bool { return c.BundleID == braveID }) {
		t.Fatal("Brave should be opened for the localhost placeholder")
	}
	// Opening Brave activates it, which shows it if it's hidden: so its
	// windows of other tabs are parked instead.
	h.p.ActivateApp(braveID)
	h.drain()
	for _, title := range []string{"Wails v3 docs", "Pull request #42 · bench", "Go packages"} {
		if id := h.windowID(braveID, title); !h.e.win.IsParked(id) {
			t.Errorf("Research's %q should be parked while Brave opens", title)
		}
	}
	// Once Bench stops waiting, Brave is hidden again.
	h.advance(16 * time.Second)
	if !h.p.IsHidden(braveID) {
		t.Errorf("Brave should be hidden once the wait is over")
	}
}

// slowCalendar is a world where Calendar takes 3 seconds to open.
func slowCalendar(p *platform.Fake) {
	standardWorld(p)
	p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(3*time.Second), platform.AppLaunchWindows("Calendar"))
}

// planConfig has a tab whose focused pane waits for Calendar, with a
// terminal in the same pane.
const planConfig = `
[[tab]]
name = "Home"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }]

[[tab]]
name = "Plans"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.apple.iCal" }, { terminal = "~/plans" }]
`

func TestSwitchsPutOffFocusIsMadeWhenTheWindowArrives(t *testing.T) {
	h := newHarness(t, setup{config: planConfig, world: slowCalendar})
	h.p.ResetCalls()
	mustOK(t, h.e.SwitchTab(1))
	h.advance(4 * time.Second)
	cal := h.windowID(calendarID, "Calendar")
	if !slices.Contains(h.focusCalls(), cal) {
		t.Errorf("Calendar's window should be focused when it arrives: %v", h.focusCalls())
	}
}

func TestSwitchWhileBenchIsHiddenFocusesTheWindowWhenItArrives(t *testing.T) {
	h := newHarness(t, setup{config: planConfig, world: slowCalendar})
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	h.p.ResetCalls()
	// A shortcut switches to Plans while Bench is hidden, and the user
	// brings Bench back while Calendar is still opening.
	h.e.Shortcut("tab", "2")
	h.advance(time.Second)
	h.p.EmitEvent(platform.Event{Kind: platform.BenchShown})
	h.drain()
	if got := h.focusCalls(); len(got) != 0 {
		t.Fatalf("nothing is ready to focus yet: %v", got)
	}
	h.advance(3 * time.Second)
	cal := h.windowID(calendarID, "Calendar")
	if !slices.Contains(h.focusCalls(), cal) {
		t.Errorf("Calendar's window should be focused when it arrives: %v", h.focusCalls())
	}
}

func TestSwitchsPutOffFocusIsDroppedWhenTheUserTurnsAway(t *testing.T) {
	for _, tt := range []struct {
		name string
		away func(h *harness)
	}{
		{"another app", func(h *harness) { h.p.ActivateApp(finderID) }},
		{"a menu", func(h *harness) { mustOK(h.t, h.e.SetOverlay(true)) }},
		{"another item", func(h *harness) {
			// Something other than a focus shows the terminal in the pane.
			h.e.lock()
			h.e.model.Show("w3")
			h.e.unlock()
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, setup{config: planConfig, world: slowCalendar})
			mustOK(t, h.e.SwitchTab(1))
			h.advance(1500 * time.Millisecond)
			h.takeCommands()
			h.p.ResetCalls()
			tt.away(h)
			h.advance(3 * time.Second)
			if !h.p.IsRunning(calendarID) {
				t.Fatal("Calendar should have opened")
			}
			if got := h.focusCalls(); len(got) != 0 {
				t.Errorf("the put-off focus took the keyboard: %v", got)
			}
			if got := h.takeCommands(); len(got) != 0 {
				t.Errorf("the put-off focus took the keyboard: %v", got)
			}
		})
	}
}

func TestNotificationOfAPoppedOutWebPageBringsItsWindowForward(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.PopOut("w2"))
	h.p.ResetCalls()
	h.p.EmitEvent(platform.Event{Kind: platform.NotificationClicked, ItemID: "w2"})
	h.drain()
	if calls := h.calls("FocusWebView"); len(calls) != 1 || calls[0].Arg != "w2" {
		t.Errorf("FocusWebView calls: %v, want Outlook's window", calls)
	}
}

func TestRedockHistoryGoesWithItsWindow(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	code := h.windowID(vscodeID, "index.html — bench-ui")
	h.advance(3 * time.Second)
	redocked := func(id platform.WindowID) bool {
		h.e.lock()
		defer h.e.unlock()
		_, ok := h.e.redocks[id]
		return ok
	}
	mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: 250, Y: 170, W: 878, H: 758}))
	h.advance(time.Second)
	mustOK(t, h.e.ShowItem("p11", "w4"))
	h.advance(time.Second)
	mustOK(t, h.p.AppMoveWindow(code, layout.Rect{X: 250, Y: 170, W: 878, H: 758}))
	h.advance(time.Second)
	if !redocked(zed) || !redocked(code) {
		t.Fatalf("Bench should remember putting both windows back")
	}
	mustOK(t, h.p.CloseWindow(zed))
	h.drain()
	if redocked(zed) {
		t.Error("a closed window's history should go")
	}
	mustOK(t, h.p.QuitApp(vscodeID))
	h.drain()
	if redocked(code) {
		t.Error("the history of a quit app's windows should go")
	}
}

func TestWebFocusIsDroppedWhenAnotherAppTakesTheKeyboard(t *testing.T) {
	for _, heard := range []bool{true, false} {
		h := newHarness(t, setup{})
		// A menu is open and a shortcut focuses Slack's pane: its view gets
		// the keyboard once the menu closes.
		mustOK(t, h.e.SetOverlay(true))
		mustOK(t, h.e.FocusPane("p9"))
		mustOK(t, h.e.FocusPane("p8"))
		h.p.ResetCalls()
		// The user clicks another app, which closes the menu. Bench may hear
		// of that app's activation first, or only later.
		h.p.ActivateApp(finderID)
		if heard {
			h.drain()
		}
		mustOK(t, h.e.SetOverlay(false))
		if calls := h.calls("FocusWebView"); len(calls) != 0 {
			t.Errorf("activation heard first %v: FocusWebView calls %v would bring Bench back in front", heard, calls)
		}
	}
}

func TestParkingUsesTheScreenAsItIsNow(t *testing.T) {
	h := newHarness(t, setup{})
	pkgs := h.windowID(braveID, "Go packages")
	mustOK(t, h.e.MoveItemToTab(h.itemOf(pkgs), 1))
	mustOK(t, h.e.SwitchTab(1))
	h.advance(time.Second)
	docs := h.windowID(braveID, "Wails v3 docs")
	if !h.e.win.IsParked(docs) {
		t.Fatal("Research's Brave windows should be parked in Code")
	}
	// The user picks a "More Space" resolution: the display grows, and the
	// old parking corner is on screen.
	bigger := monitor
	bigger.Frame = layout.Rect{W: 3008, H: 1692}
	bigger.Visible = layout.Rect{Y: 25, W: 3008, H: 1667}
	h.p.SetScreens(bigger)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.p.EmitEvent(platform.Event{Kind: platform.BenchMoved})
	h.drain()
	want := platform.ParkFrame([]platform.Screen{bigger}, bigger, h.frame(docs))
	if got := h.frame(docs); got != want {
		t.Errorf("parked at %v, want %v, before the screens settle", got, want)
	}
}

func TestTerminalFocusIsMadeInOrder(t *testing.T) {
	h := newHarness(t, setup{config: `
[[tab]]
name = "Work"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }, { web = "https://example.com", name = "Docs" }]
`})
	h.takeCommands()
	h.p.ResetCalls()
	// next_window, pressed twice quickly: the first press's command is
	// still waiting to be sent when the second press focuses the page.
	h.e.lock()
	h.e.model.Show("w1")
	h.e.focusShownLocked()
	queued := h.e.out
	h.e.out = nil
	h.e.unlock()
	mustOK(t, h.e.CycleSubtab(1))
	for _, f := range queued {
		f()
	}
	var got []string
	for _, c := range h.calls("FocusUI", "FocusWebView") {
		got = append(got, c.Method)
	}
	if want := []string{"FocusUI", "FocusWebView"}; !slices.Equal(got, want) {
		t.Errorf("native focus calls: %v, want %v", got, want)
	}
	if cmds := h.takeCommands(); len(cmds) != 0 {
		t.Errorf("commands: %v, want none: the page has the keyboard now", cmds)
	}
	if got := h.p.FocusedWebView(); got != "w2" {
		t.Errorf("focused web view: %q, want the page", got)
	}
	// Back to the terminal: the UI takes the keyboard, and the frontend is
	// told to focus the terminal.
	h.p.ResetCalls()
	mustOK(t, h.e.CycleSubtab(1))
	if len(h.calls("FocusUI")) != 1 || h.p.FocusedWebView() != "" {
		t.Errorf("FocusUI calls: %v, focused web view %q", h.calls("FocusUI"), h.p.FocusedWebView())
	}
	if cmds := h.takeCommands(); !reflect.DeepEqual(cmds, []Command{{Name: "focus-terminal", ItemID: "w1"}}) {
		t.Errorf("commands: %v, want focus-terminal for w1", cmds)
	}
}
