package workspace

import (
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

const (
	rectangleID = "com.knollsoft.Rectangle"
	magnetID    = "com.crowdcafe.windowmagnet"
)

// recommended are the preferences of a Rectangle whose user chose its
// recommended shortcuts, and changed none.
var recommended = platform.ToolPreferences{Numbers: map[string]int64{"alternateDefaultShortcuts": 1}}

// withRectangle is the standard world with Rectangle, a window tool,
// running, with prefs as its preferences.
func withRectangle(prefs platform.ToolPreferences) func(p *platform.Fake) {
	return func(p *platform.Fake) {
		standardWorld(p)
		p.AddApp(rectangleID, "Rectangle", platform.AppNotRegular())
		p.SetToolPreferences(rectangleID, prefs)
	}
}

// keepMoving has Zed move its docked window out of its pane once more
// than Bench puts it back, a second apart, so that Bench stops: the window
// is then where Zed put it.
func keepMoving(t *testing.T, h *harness, zed platform.WindowID) layout.Rect {
	t.Helper()
	where := layout.Rect{X: 250, Y: 170, W: 878, H: 758}
	for range redockLimit + 1 {
		mustOK(t, h.p.AppMoveWindow(zed, where))
		h.drain()
		h.advance(redockDelay)
		h.clk.Advance(time.Second)
	}
	if h.frame(zed) != where {
		t.Fatalf("Bench should have stopped putting the window back: %v", h.frame(zed))
	}
	return where
}

// backToBench makes another app active, then Bench again, as the user
// does after changing a window tool's settings.
func backToBench(h *harness) {
	h.t.Helper()
	mustOK(h.t, h.p.ActivateApp(zedID))
	h.drain()
	h.comeForward()
}

// toolNotices returns the notices about window tools.
func toolNotices(h *harness) []NoticeView {
	var out []NoticeView
	for _, n := range h.view().Notices {
		if strings.Contains(n.Text, "can move docked windows") {
			out = append(out, n)
		}
	}
	return out
}

// A window that its app keeps moving out of its pane stays where the app
// put it, and its pane says so, with Dock again: arranging the tab, as
// after a resize of the panes, leaves it there, as it does a window the
// user dragged away, until the user docks it again, and switching tabs
// keeps the mark.
func TestKeptMovingWindowIsMarked(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	where := keepMoving(t, h, zed)
	it := h.item("w3")
	if it.Status != StatusMoving || it.StatusText != "Zed or another tool keeps moving this window" {
		t.Fatalf("w3: %s %q", it.Status, it.StatusText)
	}
	if !h.menuBarNow().Pane.Undocked {
		t.Error("the Pane menu's Dock Again should be on for a window that kept moving")
	}
	mustOK(t, h.e.GrowPane())
	h.settle()
	if h.frame(zed) != where {
		t.Errorf("arranging the tab put the window back: %v", h.frame(zed))
	}
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if it := h.item("w3"); it.Status != StatusMoving {
		t.Errorf("after switching tabs, w3 is %s, want still marked", it.Status)
	}
	// Moves that Zed makes now are left alone.
	before := len(h.calls("SetFrame"))
	mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: 300, Y: 200, W: 800, H: 700}))
	h.advance(time.Second)
	if after := len(h.calls("SetFrame")); after != before {
		t.Errorf("Bench moved the window %d times after it stopped", after-before)
	}
}

// Dock again puts the window back, and gives Bench its full number of
// tries again.
func TestDockAgainAfterKeptMoving(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	keepMoving(t, h, zed)
	mustOK(t, h.e.Redock("w3"))
	h.settle()
	body := h.bodyOf("p11")
	if it := h.item("w3"); it.Status != StatusReady || h.frame(zed) != body {
		t.Fatalf("after Dock again, w3 is %s at %v, want ready in its pane", it.Status, h.frame(zed))
	}
	where := layout.Rect{X: 250, Y: 170, W: 878, H: 758}
	for i := range redockLimit {
		mustOK(t, h.p.AppMoveWindow(zed, where))
		h.drain()
		h.advance(redockDelay)
		if h.frame(zed) != body {
			t.Fatalf("try %d after Dock again: Bench should put the window back: %v", i+1, h.frame(zed))
		}
		h.clk.Advance(time.Second)
	}
	mustErr(t, h.e.Redock("w3"), "That window is already docked.")
}

// Rearrange docks again a window that kept moving, with all its tries, but
// not one that the user dragged out.
func TestRearrangeDocksKeptMovingWindow(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	keepMoving(t, h, zed)
	mustOK(t, h.e.Rearrange())
	h.settle()
	body := h.bodyOf("p11")
	if it := h.item("w3"); it.Status != StatusReady || h.frame(zed) != body {
		t.Fatalf("after Rearrange, w3 is %s at %v, want ready in its pane", it.Status, h.frame(zed))
	}
	mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: 250, Y: 170, W: 878, H: 758}))
	h.drain()
	h.advance(redockDelay)
	if h.frame(zed) != body {
		t.Fatalf("after Rearrange, Bench should put the window back: %v", h.frame(zed))
	}

	h.clk.Advance(time.Second)
	mustOK(t, h.p.UserMoveWindow(zed, layout.Rect{X: 400, Y: 300, W: 878, H: 758}))
	h.drain()
	mustOK(t, h.e.Rearrange())
	h.settle()
	if it := h.item("w3"); it.Status != StatusUndocked {
		t.Errorf("Rearrange docked a window that the user dragged out: %s", it.Status)
	}
}

// Once the user drags a window that kept moving, it's just out of its pane.
func TestUserMovesKeptMovingWindow(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	keepMoving(t, h, zed)
	mustOK(t, h.p.UserMoveWindow(zed, layout.Rect{X: 400, Y: 300, W: 878, H: 758}))
	h.drain()
	if it := h.item("w3"); it.Status != StatusUndocked || it.StatusText != "Moved out of its pane" {
		t.Errorf("w3: %s %q, want just out of its pane", it.Status, it.StatusText)
	}
	mustOK(t, h.e.Redock("w3"))
	h.settle()
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Dock again didn't put the window back: %v", h.frame(zed))
	}
}

// After the screens change, the window is docked again: the moves may have
// been macOS's (TestSettleGivesRedocksBack).
func TestSettleDocksKeptMovingWindow(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	keepMoving(t, h, zed)
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensWoke})
	h.advance(screenSettle + 100*time.Millisecond)
	if it := h.item("w3"); it.Status != StatusReady || h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("after the settle, w3 is %s at %v, want ready in its pane", it.Status, h.frame(zed))
	}
}

// A pane names the window tools that are running, as they may be what
// moves the window, but not the app itself.
func TestKeptMovingNamesWindowTools(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		withRectangle(recommended)(p)
		p.AddApp(magnetID, "Magnet", platform.AppNotRegular())
	}})
	zed := codeTab(t, h)
	keepMoving(t, h, zed)
	if got := h.item("w3").StatusText; got != "Zed, Rectangle, or Magnet keeps moving this window" {
		t.Errorf("w3's text = %q", got)
	}
	if got := h.view().WindowTools; !slices.Equal(got, []string{"Rectangle", "Magnet"}) {
		t.Errorf("WindowTools = %q", got)
	}
	if got := keepsMovingText("Rectangle", nil); got != "Rectangle or another tool keeps moving this window" {
		t.Errorf("keepsMovingText = %q", got)
	}
}

// When a window tool runs as Bench starts, the status bar says what it can
// do, and which of Bench's shortcuts its own take, until the user
// dismisses it: Dismiss takes the notice away, and state.json remembers.
func TestWindowToolNoticeAtStart(t *testing.T) {
	h := newHarness(t, setup{world: withRectangle(recommended)})
	notices := toolNotices(h)
	if len(notices) != 1 {
		t.Fatalf("notices: %+v", h.view().Notices)
	}
	want := "Rectangle is running. It can move docked windows out of their panes. It uses the same keys as Bench for " +
		"Focus a pane (⌃⌥←, ⌃⌥→, ⌃⌥↑, ⌃⌥↓), Move a window (⌃⌥⇧↑), and Zoom pane (⌃⌥↩). " +
		"macOS doesn't warn about this, and both apps may act on these keys. Change the keys in Rectangle or in Keyboard settings."
	if notices[0].Text != want {
		t.Errorf("notice:\n%s\nwant:\n%s", notices[0].Text, want)
	}
	if notices[0].Key != "window-tool:com.knollsoft.rectangle" {
		t.Errorf("notice's key = %q", notices[0].Key)
	}
	wantActions := []NoticeAction{
		{Label: "Keyboard settings", Command: "keyboard-settings"},
		{Label: "Dismiss", Method: "DismissWindowTool", Arg: rectangleID},
	}
	if !slices.Equal(notices[0].Actions, wantActions) {
		t.Errorf("actions: %+v", notices[0].Actions)
	}
	if calls := h.calls("ToolPreferences"); len(calls) != 1 || calls[0].BundleID != rectangleID {
		t.Errorf("ToolPreferences calls: %+v, want one, for Rectangle", calls)
	}
	if got := h.e.WindowToolKeys(); len(got) != 1 || got[0].Name != "Rectangle" || !slices.Contains(got[0].Keys, "ctrl+alt+left") {
		t.Errorf("WindowToolKeys = %+v", got)
	}

	// The notice follows Bench's shortcuts, and keeps its key.
	h.e.lock()
	h.e.cfg.Shortcuts.ZoomPane = ""
	h.e.unlock()
	if got := toolNotices(h)[0]; strings.Contains(got.Text, "Zoom pane") || !strings.Contains(got.Text, "and Move a window (⌃⌥⇧↑). macOS") ||
		got.Key != notices[0].Key {
		t.Errorf("with zoom_pane off, the notice is %+v", got)
	}

	// Until the user dismisses it, it comes back after a restart.
	h.restart()
	if got := toolNotices(h); len(got) != 1 {
		t.Fatalf("after a restart before Dismiss: %+v", h.view().Notices)
	}
	mustOK(t, h.e.DismissWindowTool("COM.KNOLLSOFT.RECTANGLE"))
	if got := toolNotices(h); len(got) != 0 {
		t.Errorf("after Dismiss: %+v", got)
	}
	mustOK(t, h.e.DismissWindowTool(rectangleID))
	mustOK(t, h.e.DismissWindowTool("com.example.notatool"))
	h.settle()
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	mustOK(t, err)
	if !strings.Contains(string(data), `"windowTools": [`) || !strings.Contains(string(data), `"com.knollsoft.rectangle"`) ||
		strings.Contains(string(data), "notatool") {
		t.Errorf("state.json should list Rectangle alone: %s", data)
	}
	h.restart()
	if got := toolNotices(h); len(got) != 0 {
		t.Errorf("after a restart, the notice came back: %+v", got)
	}
	// Bench still knows Rectangle's keys, for Settings ▸ Keyboard.
	if got := h.e.WindowToolKeys(); len(got) != 1 {
		t.Errorf("after Dismiss, WindowToolKeys = %+v", got)
	}
}

// macOS doesn't announce a window tool that lives in the menu bar as it
// opens or quits, so Bench looks as it arranges and as it becomes the
// active app: the notice comes once the tool runs, and goes when it quits.
// A tool that quits before the user dismissed its notice gets it again
// when it runs again, but not once the user has dismissed it. Each time,
// the shell checks Bench's shortcuts against the tool's again.
func TestWindowToolNoticeWhenToolStartsLater(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(rectangleID, "Rectangle", platform.AppNotRegular())
		p.SetToolPreferences(rectangleID, recommended)
	}})
	if got := toolNotices(h); len(got) != 0 {
		t.Fatalf("a notice before Rectangle runs: %+v", got)
	}
	changed := h.em.shortcutsChanged()
	mustOK(t, h.p.StartAgent(rectangleID))
	h.comeForward()
	if got := toolNotices(h); len(got) != 1 {
		t.Fatalf("after Rectangle started and Bench came forward: %+v", h.view().Notices)
	}
	if got := h.em.shortcutsChanged(); got != changed+1 {
		t.Errorf("ShortcutsChanged %d times as Rectangle started, want once", got-changed)
	}
	mustOK(t, h.p.StopAgent(rectangleID))
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if got := toolNotices(h); len(got) != 0 {
		t.Errorf("after Rectangle quit and Bench arranged: %+v", got)
	}
	if got := h.view().WindowTools; len(got) != 0 {
		t.Errorf("WindowTools = %q after Rectangle quit", got)
	}
	if got := h.e.WindowToolKeys(); len(got) != 0 {
		t.Errorf("WindowToolKeys = %+v after Rectangle quit", got)
	}
	if got := h.em.shortcutsChanged(); got != changed+2 {
		t.Errorf("ShortcutsChanged %d times as Rectangle started and quit, want twice", got-changed)
	}
	mustOK(t, h.p.StartAgent(rectangleID))
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	if got := toolNotices(h); len(got) != 1 {
		t.Errorf("Rectangle started again before the user dismissed its notice: %+v", got)
	}
	mustOK(t, h.e.DismissWindowTool(rectangleID))
	mustOK(t, h.p.StopAgent(rectangleID))
	backToBench(h)
	mustOK(t, h.p.StartAgent(rectangleID))
	backToBench(h)
	if got := toolNotices(h); len(got) != 0 {
		t.Errorf("the notice came back after Dismiss as Rectangle started again: %+v", got)
	}
	if got := h.view().WindowTools; !slices.Equal(got, []string{"Rectangle"}) {
		t.Errorf("WindowTools = %q", got)
	}
}

// As Bench becomes the active app, after the user may have changed the
// tool's shortcuts in the tool, Bench reads them again: the notice no
// longer names a clash that the user took away, and the shell checks
// Bench's shortcuts again, but only when they changed.
func TestWindowToolKeysReadAgain(t *testing.T) {
	h := newHarness(t, setup{world: withRectangle(recommended)})
	if got := toolNotices(h)[0].Text; !strings.Contains(got, "Focus a pane") {
		t.Fatalf("notice: %q", got)
	}
	changed := h.em.shortcutsChanged()
	backToBench(h)
	if got := h.em.shortcutsChanged(); got != changed {
		t.Errorf("ShortcutsChanged %d times with Rectangle's shortcuts as they were", got-changed)
	}
	h.p.SetToolPreferences(rectangleID, platform.ToolPreferences{
		Numbers:   map[string]int64{"alternateDefaultShortcuts": 1},
		Shortcuts: map[string]string{"leftHalf": "", "rightHalf": "", "topHalf": "", "bottomHalf": ""},
	})
	backToBench(h)
	if got := toolNotices(h)[0].Text; strings.Contains(got, "Focus a pane") || !strings.Contains(got, "Move a window (⌃⌥⇧↑)") {
		t.Errorf("after the user cleared Rectangle's halves: %q", got)
	}
	if got := h.em.shortcutsChanged(); got != changed+1 {
		t.Errorf("ShortcutsChanged %d times after Rectangle's shortcuts changed, want once", got-changed)
	}
	if got := h.e.WindowToolKeys(); len(got) != 1 || slices.Contains(got[0].Keys, "ctrl+alt+left") {
		t.Errorf("WindowToolKeys = %+v", got)
	}
}

// A window tool that macOS does announce, as one that's a regular app,
// gets its notice as it opens.
func TestWindowToolNoticeOnLaunch(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(magnetID, "Magnet")
	}})
	mustOK(t, h.p.LaunchApp(magnetID))
	h.drain()
	if got := toolNotices(h); len(got) != 1 || !strings.HasPrefix(got[0].Text, "Magnet is running.") {
		t.Fatalf("after Magnet opened: %+v", h.view().Notices)
	}
	mustOK(t, h.p.QuitApp(magnetID))
	h.drain()
	if got := toolNotices(h); len(got) != 0 {
		t.Errorf("after Magnet quit: %+v", got)
	}
}

// Without shortcuts that Bench can read, the notice doesn't guess: it says
// that the tool's shortcuts can use Bench's keys. So it does when it read
// them and none are Bench's, as the tool may have others it can't read.
func TestWindowToolNoticeWithoutKnownKeys(t *testing.T) {
	general := func(name string) string {
		return name + " is running. It can move docked windows out of their panes. Its shortcuts can use the same keys as Bench's. " +
			"macOS doesn't warn about this, and both apps may act on such a key. Check the keys in " + name + " and in Keyboard settings."
	}
	for _, c := range []struct {
		name  string
		world func(p *platform.Fake)
		want  string
	}{
		{"Magnet, whose shortcuts Bench can't read", func(p *platform.Fake) {
			standardWorld(p)
			p.AddApp(magnetID, "Magnet", platform.AppNotRegular())
		}, general("Magnet")},
		{"Rectangle before it first opened", withRectangle(platform.ToolPreferences{}), general("Rectangle")},
		{"Rectangle with none of Bench's", withRectangle(platform.ToolPreferences{
			Numbers: map[string]int64{"alternateDefaultShortcuts": 1},
			Shortcuts: map[string]string{"leftHalf": "", "rightHalf": "", "topHalf": "", "bottomHalf": "",
				"maximize": "ctrl+alt+shift+cmd+return", "maximizeHeight": ""},
		}), general("Rectangle")},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, setup{world: c.world})
			if got := toolNotices(h); len(got) != 1 || got[0].Text != c.want {
				t.Errorf("notices: %+v\nwant: %s", got, c.want)
			}
		})
	}
}

// Rectangle's shortcuts are the user's own, and the defaults of the set
// they chose for the rest.
func TestRectangleKeys(t *testing.T) {
	if _, ok := rectangleKeys(platform.ToolPreferences{}); ok {
		t.Error("without alternateDefaultShortcuts, Bench can't know Rectangle's set")
	}
	keys, ok := rectangleKeys(recommended)
	if !ok || !slices.Contains(keys, "ctrl+alt+left") || !slices.Contains(keys, "ctrl+alt+return") || slices.Contains(keys, "alt+cmd+left") {
		t.Errorf("recommended: %q, %v", keys, ok)
	}
	if slices.Contains(keys, "ctrl+alt+r") {
		t.Error("centerTwoThirds has a shortcut only after version 94")
	}
	keys, _ = rectangleKeys(platform.ToolPreferences{
		Numbers: map[string]int64{"alternateDefaultShortcuts": 1},
		Texts:   map[string]string{"installVersion": "95"},
	})
	if !slices.Contains(keys, "ctrl+alt+r") {
		t.Errorf("a Rectangle installed at 95 has ctrl+alt+r: %q", keys)
	}
	keys, _ = rectangleKeys(platform.ToolPreferences{Numbers: map[string]int64{"alternateDefaultShortcuts": 0}})
	if !slices.Contains(keys, "alt+cmd+left") || !slices.Contains(keys, "ctrl+alt+shift+right") || slices.Contains(keys, "ctrl+alt+left") {
		t.Errorf("Spectacle's: %q", keys)
	}
	// The user's own, under the action's name or its old one, take the
	// default's place; one set to none has none.
	keys, _ = rectangleKeys(platform.ToolPreferences{
		Numbers: map[string]int64{"alternateDefaultShortcuts": 1},
		Shortcuts: map[string]string{
			"leftSide":  "ctrl+alt+shift+cmd+h",
			"rightHalf": "",
			"topSide":   "ctrl+alt+shift+cmd+k", // Rectangle uses topHalf's.
			"topHalf":   "ctrl+alt+shift+cmd+i",
			"todo":      "ctrl+alt+shift+cmd+t",
		},
	})
	for _, want := range []string{"ctrl+alt+shift+cmd+h", "ctrl+alt+shift+cmd+i", "ctrl+alt+shift+cmd+t", "ctrl+alt+down"} {
		if !slices.Contains(keys, want) {
			t.Errorf("keys lack %s: %q", want, keys)
		}
	}
	for _, not := range []string{"ctrl+alt+left", "ctrl+alt+right", "ctrl+alt+up", "ctrl+alt+shift+cmd+k", ""} {
		if slices.Contains(keys, not) {
			t.Errorf("keys have %q: %q", not, keys)
		}
	}
	// A value that Bench couldn't read as a shortcut, which Rectangle
	// reads as none, is "", and takes the default's place as well.
	keys, _ = rectangleKeys(platform.ToolPreferences{
		Numbers:   map[string]int64{"alternateDefaultShortcuts": 1},
		Shortcuts: map[string]string{"maximize": ""},
	})
	if slices.Contains(keys, "ctrl+alt+return") || !slices.Contains(keys, "ctrl+alt+left") {
		t.Errorf("with maximize unreadable: %q", keys)
	}
	// Every default is a shortcut in canonical form.
	for _, a := range rectangleActions {
		for _, s := range []string{a.recommended, a.spectacle} {
			if s == "" {
				continue
			}
			if got, err := shortcuts.Parse(s, false); err != nil || got != s {
				t.Errorf("%s: %q isn't canonical: %q, %v", a.name, s, got, err)
			}
		}
	}
}

// The table of window tools has each bundle ID once, ignoring case, and a
// name for each. docs/development.md lists each bundle ID that hasn't been
// checked on a Mac, for its Mac checks.
func TestWindowToolsTable(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range windowTools {
		key := strings.ToLower(tool.bundleID)
		if seen[key] {
			t.Errorf("%s is in the table twice", tool.bundleID)
		}
		seen[key] = true
		if tool.name == "" || tool.bundleID == "" {
			t.Errorf("%+v has no name or bundle ID", tool)
		}
		if got, ok := windowToolFor(strings.ToUpper(tool.bundleID)); !ok || got.bundleID != tool.bundleID {
			t.Errorf("windowToolFor(%s) = %+v, %v", tool.bundleID, got, ok)
		}
	}
	for _, name := range []string{"Rectangle", "Rectangle Pro", "Magnet", "Loop", "Raycast", "BetterSnapTool", "Moom",
		"Divvy", "Swish", "AeroSpace", "Amethyst", "yabai"} {
		if !slices.ContainsFunc(windowTools, func(tool windowTool) bool { return tool.name == name }) {
			t.Errorf("the table lacks %s", name)
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "development.md"))
	mustOK(t, err)
	for _, tool := range windowTools {
		if tool.unsure && !strings.Contains(string(data), "`"+tool.bundleID+"`") {
			t.Errorf("docs/development.md doesn't list %s's bundle ID, %s, to check on a Mac", tool.name, tool.bundleID)
		}
	}
}

// The browser preview's demo bar starts Rectangle, which shows its notice,
// and has it keep moving the shown window, which its pane then says.
func TestDemoWindowTool(t *testing.T) {
	h := newHarness(t, setup{
		world: func(p *platform.Fake) {
			standardWorld(p)
			p.InstallApp(platform.DemoWindowTool, "Rectangle", platform.AppNotRegular())
			p.SetToolPreferences(platform.DemoWindowTool, recommended)
		},
		options: func(o *Options) { o.Demo = true },
	})
	mustOK(t, h.e.SetDemoWindowTool(true))
	mustOK(t, h.e.SetDemoWindowTool(true))
	if got := toolNotices(h); len(got) != 1 || !slices.Equal(h.view().WindowTools, []string{"Rectangle"}) {
		t.Fatalf("after the demo's Rectangle started: %+v, %q", got, h.view().WindowTools)
	}
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	mustOK(t, h.e.FocusPane("p11"))
	mustOK(t, h.e.DemoToolMoves())
	h.advance(demoToolTimeout + time.Second)
	if it := h.item("w3"); it.Status != StatusMoving || it.StatusText != "Zed or Rectangle keeps moving this window" {
		t.Errorf("w3: %s %q", it.Status, it.StatusText)
	}
	// The window is out of its pane now.
	mustErr(t, h.e.DemoToolMoves(), "The focused pane doesn't show an app's window in the pane to move.")
	mustOK(t, h.e.SetDemoWindowTool(false))
	if got := h.view().WindowTools; len(got) != 0 {
		t.Errorf("after the demo's Rectangle stopped: %q", got)
	}

	// Right after the screens change, as after the demo bar's screen
	// picker, the tool waits for them to settle, as Bench doesn't count
	// the moves meanwhile.
	mustOK(t, h.e.Redock("w3"))
	h.settle()
	h.p.EmitEvent(platform.Event{Kind: platform.ScreensChanged})
	h.advance(100 * time.Millisecond)
	mustOK(t, h.e.DemoToolMoves())
	h.advance(screenSettle + demoToolTimeout + time.Second)
	if it := h.item("w3"); it.Status != StatusMoving {
		t.Errorf("w3 after a screen change: %s %q", it.Status, it.StatusText)
	}

	h2 := newHarness(t, setup{})
	mustErr(t, h2.e.SetDemoWindowTool(true), "Simulated window tools are only for the browser preview.")
	mustErr(t, h2.e.DemoToolMoves(), "Simulated window tools are only for the browser preview.")

	// With BENCH_DEBUG set, the preview's platform is logged, and still
	// simulates it.
	h3 := newHarness(t, setup{
		world: func(p *platform.Fake) {
			standardWorld(p)
			p.InstallApp(platform.DemoWindowTool, "Rectangle", platform.AppNotRegular())
		},
		options: func(o *Options) {
			o.Demo = true
			o.Platform = platform.Logged(o.Platform, slog.New(slog.DiscardHandler))
		},
	})
	mustOK(t, h3.e.SetDemoWindowTool(true))
	if got := h3.view().WindowTools; !slices.Equal(got, []string{"Rectangle"}) {
		t.Errorf("with BENCH_DEBUG set: %q", got)
	}
}

// An app that nudges its window just after each of Bench's resizes, as
// Electron apps do, isn't fighting Bench: each resize docks the window at
// a new size, which gives it its tries again, so that resizing the panes
// with the keys, again and again, never marks it. Only the moves away from
// the same place count.
func TestNudgesAfterResizesDontMarkWindow(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	mustOK(t, h.e.FocusPane("p11"))
	h.settle()
	for i := range redockLimit + 2 {
		if i%2 == 0 {
			mustOK(t, h.e.GrowPane())
		} else {
			mustOK(t, h.e.ShrinkPane())
		}
		h.drain()
		h.advance(400 * time.Millisecond)
		f := h.frame(zed)
		mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: f.X, Y: f.Y, W: f.W - 40, H: f.H - 30}))
		h.drain()
		h.advance(redockDelay)
		h.clk.Advance(time.Second)
	}
	if it := h.item("w3"); it.Status != StatusReady {
		t.Fatalf("after resizes with nudges, w3 is %s %q, want ready", it.Status, it.StatusText)
	}
	mustOK(t, h.e.GrowPane())
	h.settle()
	if h.frame(zed) != h.bodyOf("p11") {
		t.Errorf("Zed isn't in its pane: %v, want %v", h.frame(zed), h.bodyOf("p11"))
	}
}

// The mouse that's held just after the user dragged one of Bench's
// dividers, or while they drag another docked window's edge, isn't on a
// window that kept moving, so its moves then aren't the user's own: the
// mark stays. Once that's over, the user's drag of the window makes it
// just out of its pane.
func TestKeptMovingStaysThroughOtherDrags(t *testing.T) {
	t.Run("divider", func(t *testing.T) {
		h := newHarness(t, setup{})
		zed := codeTab(t, h)
		keepMoving(t, h, zed)
		mustOK(t, h.e.SetSizes(h.tabNamed("Code").Root.ID, []float64{60, 40}))
		h.advance(400 * time.Millisecond)
		mustOK(t, h.p.UserMoveWindow(zed, layout.Rect{X: 300, Y: 200, W: 878, H: 758}))
		h.drain()
		if it := h.item("w3"); it.Status != StatusMoving {
			t.Errorf("a move with the mouse held just after a divider's drag made w3 %s", it.Status)
		}
		h.advance(dividerGrace)
		mustOK(t, h.p.UserMoveWindow(zed, layout.Rect{X: 320, Y: 220, W: 878, H: 758}))
		h.drain()
		if it := h.item("w3"); it.Status != StatusUndocked {
			t.Errorf("after the grace, the user's drag left w3 %s", it.Status)
		}
	})
	t.Run("another window's edge", func(t *testing.T) {
		h := newHarness(t, setup{})
		brave, _ := researchPanes(t, h)
		h.settle()
		obsidian := h.windowID(obsidianID, "Daily note")
		notes := h.itemOf(obsidian)
		keepMoving(t, h, obsidian)
		if it := h.item(notes); it.Status != StatusMoving {
			t.Fatalf("Obsidian's item is %s", it.Status)
		}
		win := h.shownWindow(brave)
		f := h.frame(win)
		f.W += 100
		h.holdEdge(win, f)
		mustOK(t, h.p.UserMoveWindow(obsidian, layout.Rect{X: 300, Y: 200, W: 878, H: 758}))
		h.drain()
		if it := h.item(notes); it.Status != StatusMoving {
			t.Errorf("a move with the mouse on Brave's edge made Obsidian's item %s", it.Status)
		}
		h.releaseEdge()
		h.advance(dividerGrace)
		mustOK(t, h.p.UserMoveWindow(obsidian, layout.Rect{X: 320, Y: 220, W: 878, H: 758}))
		h.drain()
		if it := h.item(notes); it.Status != StatusUndocked {
			t.Errorf("after the edge's drag, the user's drag left Obsidian's item %s", it.Status)
		}
	})
}

// Dropping a window that kept moving onto a pane docks it there, with all
// its tries.
func TestDropDocksKeptMovingWindow(t *testing.T) {
	h := newHarness(t, setup{})
	zed := codeTab(t, h)
	keepMoving(t, h, zed)
	at := middleOf(h.bodyOf("p12"))
	h.dragTo(zed, at)
	h.advance(dropRest)
	h.drop(zed, at)
	body := h.bodyOf("p12")
	if it := h.item("w3"); it.Status != StatusReady || it.Pane != "p12" || h.frame(zed) != body {
		t.Fatalf("after the drop, w3 is %s in %s at %v, want ready in p12", it.Status, it.Pane, h.frame(zed))
	}
	h.e.lock()
	kept := h.e.model.Items["w3"].keptMoving
	h.e.unlock()
	if kept {
		t.Error("w3 still kept moving after the drop")
	}
	h.settle()
	for i := range redockLimit {
		mustOK(t, h.p.AppMoveWindow(zed, layout.Rect{X: 250, Y: 170, W: 878, H: 758}))
		h.drain()
		h.advance(redockDelay)
		if h.frame(zed) != body {
			t.Fatalf("try %d after the drop: Bench should put the window back: %v", i+1, h.frame(zed))
		}
		h.clk.Advance(time.Second)
	}
}

// A window in two tabs that kept moving in one is marked in the other
// too, and Dock again there docks it there.
func TestKeptMovingInTwoTabs(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	research := h.view().Display.Focus
	mustOK(t, h.e.AddToPane(research, Source{Type: "item", ID: "w3"}))
	zed := codeTab(t, h)
	mustOK(t, h.e.ShowItem("p11", "w3"))
	h.settle()
	keepMoving(t, h, zed)
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	if it := h.item("w3"); it.Status != StatusMoving || it.StatusText != "Zed or another tool keeps moving this window" {
		t.Fatalf("in Research, w3 is %s %q", it.Status, it.StatusText)
	}
	mustOK(t, h.e.ShowItem(research, "w3"))
	mustOK(t, h.e.Redock("w3"))
	h.settle()
	if it := h.item("w3"); it.Status != StatusReady || h.frame(zed) != h.bodyOf(research) {
		t.Errorf("after Dock again in Research, w3 is %s at %v, want ready at %v", it.Status, h.frame(zed), h.bodyOf(research))
	}
}
