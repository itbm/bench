package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

func TestSaveSettings(t *testing.T) {
	h := newHarness(t, setup{})
	before := h.view()
	s := before.Settings
	s.Gap = 12
	s.FollowFocus = "off"
	s.OpenMissingApps = false
	s.CompactBelow = 1600
	s.FillScreen = false
	s.ShowStatusBar = false
	s.TabShortcutHints = config.TabHintsNever
	s.ScrollBars = config.ScrollBarsAlways
	s.Density = config.DensityCompact
	s.PaneHeaders = config.PaneHeadersAuto
	mustOK(t, h.e.SaveSettings(s))
	v := h.view()
	if !reflect.DeepEqual(v.Settings, s) {
		t.Errorf("settings: %+v, want %+v", v.Settings, s)
	}
	if got := h.em.lastToast(); got != "Saved settings" {
		t.Errorf("toast: %q", got)
	}
	cfg := savedConfig(t, h)
	if cfg.Settings.Gap != 12 || cfg.Settings.FollowFocus != "off" || cfg.Settings.OpenMissingApps ||
		cfg.Settings.CompactBelow != 1600 || cfg.Settings.FillScreen || cfg.Settings.ShowStatusBar ||
		cfg.Settings.TabShortcutHints != config.TabHintsNever || cfg.Settings.ScrollBars != config.ScrollBarsAlways ||
		cfg.Settings.Density != config.DensityCompact || cfg.Settings.PaneHeaders != config.PaneHeadersAuto {
		t.Errorf("config settings: %+v", cfg.Settings)
	}
	if _, err := os.Stat(filepath.Join(h.dir, config.BackupFile)); err != nil {
		t.Errorf("config.toml.bak: %v", err)
	}
	// The tabs are Tab properties', which SaveSettings leaves alone.
	if len(v.Tabs) != len(before.Tabs) || v.Current != before.Current {
		t.Fatalf("tabs: %d current %d, want %d current %d", len(v.Tabs), v.Current, len(before.Tabs), before.Current)
	}
	for i, tab := range v.Tabs {
		was := before.Tabs[i]
		if tab.Name != was.Name || tab.Preset != was.Preset || tab.SmallScreens != was.SmallScreens {
			t.Errorf("tab %d: %+v, want %+v", i, tab, was)
		}
	}
	// The gap changes the frames.
	if p := v.Display.Panes[0]; p.Rect.X != 12 {
		t.Errorf("gap: %v", p.Rect)
	}
}

func TestSaveSettingsRefusals(t *testing.T) {
	h := newHarness(t, setup{})
	v := h.view()
	bad := v.Settings
	bad.LaunchTimeout = "soon"
	mustErr(t, h.e.SaveSettings(bad), "The launch timeout must be a length of time, such as 15s.")
	bad = v.Settings
	bad.NewWindows = "sideways"
	if err := h.e.SaveSettings(bad); err == nil || !strings.HasPrefix(err.Error(), "Bench can't save that") {
		t.Errorf("a bad setting: %v", err)
	}
	bad = v.Settings
	bad.TabShortcutHints = "sometimes"
	if err := h.e.SaveSettings(bad); err == nil || !strings.Contains(err.Error(), "auto, always, or never") {
		t.Errorf("a bad tab_shortcut_hints: %v", err)
	}
	bad = v.Settings
	bad.ScrollBars = "legacy"
	if err := h.e.SaveSettings(bad); err == nil || !strings.Contains(err.Error(), "auto, always, or never") {
		t.Errorf("a bad scroll_bars: %v", err)
	}
	bad = v.Settings
	bad.Density = "dense"
	if err := h.e.SaveSettings(bad); err == nil || !strings.Contains(err.Error(), "auto, standard, or compact") {
		t.Errorf("a bad density: %v", err)
	}
	bad = v.Settings
	bad.PaneHeaders = "never"
	if err := h.e.SaveSettings(bad); err == nil || !strings.Contains(err.Error(), "always or auto") {
		t.Errorf("a bad pane_headers: %v", err)
	}
	if got := h.view().Settings; !reflect.DeepEqual(got, v.Settings) {
		t.Errorf("settings after the refused saves: %+v, want %+v", got, v.Settings)
	}
}

func TestSaveSettingsRefusesAGapWiderThanTheLimit(t *testing.T) {
	// A gap wider than the pane area would put the panes, and the windows
	// docked in them, off screen.
	h := newHarness(t, setup{})
	s := h.view().Settings
	s.Gap = 8000
	if err := h.e.SaveSettings(s); err == nil || !strings.Contains(err.Error(), "100 points") {
		t.Fatalf("SaveSettings with a gap of 8000 points = %v, want an error about the gap", err)
	}
	if got := h.view().Settings.Gap; got != 4 {
		t.Errorf("gap after the refused save = %v, want 4", got)
	}
	if got := savedConfig(t, h).Settings.Gap; got != 4 {
		t.Errorf("config.toml's gap = %v, want 4", got)
	}
}

// TestSaveSettingsTerminals checks the terminals' text size and Option as
// Meta: they're saved, and sent to the frontend in the View, which applies
// them to every terminal. A size outside 9 to 24 points is refused, and a
// SettingsView without one, as from an older frontend, keeps it.
func TestSaveSettingsTerminals(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	if s.TerminalFontSize != 12.5 || s.TerminalOptionAsMeta {
		t.Fatalf("default terminal settings: %v, %v; want 12.5, false", s.TerminalFontSize, s.TerminalOptionAsMeta)
	}
	s.TerminalFontSize = 15
	s.TerminalOptionAsMeta = true
	mustOK(t, h.e.SaveSettings(s))
	if got := h.view().Settings; got.TerminalFontSize != 15 || !got.TerminalOptionAsMeta {
		t.Errorf("view: %v, %v; want 15, true", got.TerminalFontSize, got.TerminalOptionAsMeta)
	}
	if cfg := savedConfig(t, h); cfg.Settings.TerminalFontSize != 15 || !cfg.Settings.TerminalOptionAsMeta {
		t.Errorf("config.toml: %v, %v; want 15, true", cfg.Settings.TerminalFontSize, cfg.Settings.TerminalOptionAsMeta)
	}
	for _, size := range []float64{8, 24.5, -12} {
		bad := s
		bad.TerminalFontSize = size
		mustErr(t, h.e.SaveSettings(bad), "The terminals' text size must be from 9 to 24 points.")
	}
	s.TerminalFontSize = 0
	s.TerminalOptionAsMeta = false
	mustOK(t, h.e.SaveSettings(s))
	if got := h.view().Settings; got.TerminalFontSize != 15 || got.TerminalOptionAsMeta {
		t.Errorf("after a save without a size: %v, %v; want 15 kept, false", got.TerminalFontSize, got.TerminalOptionAsMeta)
	}
}

// TestSaveSettingsKeepsTheChoicesItDoesntShow checks that a SettingsView
// without tab_shortcut_hints, scroll_bars, density, or pane_headers, as from
// an older frontend, keeps the settings.
func TestSaveSettingsKeepsTheChoicesItDoesntShow(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	s.TabShortcutHints = config.TabHintsAlways
	s.ScrollBars = config.ScrollBarsNever
	s.Density = config.DensityStandard
	s.PaneHeaders = config.PaneHeadersAuto
	mustOK(t, h.e.SaveSettings(s))
	s.TabShortcutHints = ""
	s.ScrollBars = ""
	s.Density = ""
	s.PaneHeaders = ""
	mustOK(t, h.e.SaveSettings(s))
	if got := h.view().Settings; got.TabShortcutHints != config.TabHintsAlways || got.ScrollBars != config.ScrollBarsNever ||
		got.Density != config.DensityStandard || got.PaneHeaders != config.PaneHeadersAuto {
		t.Errorf("tab_shortcut_hints = %q, scroll_bars = %q, density = %q, pane_headers = %q; want always, never, standard, and auto kept",
			got.TabShortcutHints, got.ScrollBars, got.Density, got.PaneHeaders)
	}
}

// TestScrollBarsKeepThePanes checks that showing or hiding the strips'
// scroll bars, by the setting or by macOS, leaves the pane area, the
// panes' headers and bodies, and the docked windows as they were: the
// frontend draws the bars inside the strips' own padding.
func TestScrollBarsKeepThePanes(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	before := h.view().Display
	frame := h.frame(zed)
	h.p.ResetCalls()
	check := func(when string) {
		t.Helper()
		d := h.view().Display
		if d.Area != before.Area || len(d.Panes) != len(before.Panes) {
			t.Fatalf("%s: area %v with %d panes, want %v with %d", when, d.Area, len(d.Panes), before.Area, len(before.Panes))
		}
		for i, p := range d.Panes {
			if was := before.Panes[i]; p.Rect != was.Rect || p.Body != was.Body {
				t.Errorf("%s: pane %s at %v, body %v; want %v, body %v", when, p.ID, p.Rect, p.Body, was.Rect, was.Body)
			}
		}
		if got := h.frame(zed); got != frame {
			t.Errorf("%s: Zed %v, want %v", when, got, frame)
		}
		if moves := h.calls("SetFrame"); len(moves) > 0 {
			t.Errorf("%s: Bench moved windows: %v", when, moves)
		}
	}
	for _, choice := range []string{config.ScrollBarsAlways, config.ScrollBarsNever, config.ScrollBarsAuto} {
		s := h.view().Settings
		s.ScrollBars = choice
		mustOK(t, h.e.SaveSettings(s))
		h.settle()
		if got := h.view().Settings.ScrollBars; got != choice {
			t.Fatalf("scroll_bars = %q, want %q", got, choice)
		}
		check("scroll_bars " + choice)
	}
	h.p.SetScrollBarsShown(true)
	h.p.EmitEvent(platform.Event{Kind: platform.ScrollBarsChanged})
	h.settle()
	check("macOS showing scroll bars")
}

// TestScrollBarsFollowMacOS checks that the view says whether macOS shows
// scroll bars all the time, from the start and as it changes.
func TestScrollBarsFollowMacOS(t *testing.T) {
	h := newHarness(t, setup{})
	if h.view().SystemScrollBars {
		t.Fatal("the view says macOS shows scroll bars, but the platform says it doesn't")
	}
	sent := h.em.stateCount()
	h.p.SetScrollBarsShown(true)
	h.p.EmitEvent(platform.Event{Kind: platform.ScrollBarsChanged})
	h.settle()
	if !h.view().SystemScrollBars {
		t.Error("the view doesn't follow macOS showing scroll bars")
	}
	if h.em.stateCount() == sent {
		t.Error("no new view was sent")
	}
	h.p.SetScrollBarsShown(false)
	h.p.EmitEvent(platform.Event{Kind: platform.ScrollBarsChanged})
	h.settle()
	if h.view().SystemScrollBars {
		t.Error("the view doesn't follow macOS hiding scroll bars")
	}

	// A Bench that starts while macOS shows scroll bars says so at once.
	h.p.SetScrollBarsShown(true)
	h.restart()
	if !h.view().SystemScrollBars {
		t.Error("after a restart, the view doesn't say that macOS shows scroll bars")
	}
}

// TestHidingTheStatusBarGivesThePanesItsRoom checks that, with the status
// bar hidden, the windows docked in the panes take its height, once the
// frontend reports the taller pane area.
func TestHidingTheStatusBarGivesThePanesItsRoom(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	zed := h.windowID(zedID, "bench — switch.go")
	before := h.frame(zed)
	s := h.view().Settings
	s.ShowStatusBar = false
	mustOK(t, h.e.SaveSettings(s))
	if v := h.view(); v.Settings.ShowStatusBar || v.Display.Area != paneArea {
		t.Fatalf("status bar shown %v, area %v; want hidden, and the area as the frontend last reported it", v.Settings.ShowStatusBar, v.Display.Area)
	}
	// The frontend draws the panes down to the bottom of the window, and
	// reports the taller pane area.
	area := paneArea
	area.H += 30
	mustOK(t, h.e.SetPaneArea(area))
	after := h.frame(zed)
	if after.Y != before.Y || after.H != before.H+30 || after != h.bodyOf("p11") {
		t.Errorf("Zed %v, was %v; want 30 points taller, in its pane's body %v", after, before, h.bodyOf("p11"))
	}
}

func TestSaveSettingsAsksForScreenRecording(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	s.LivePreviews = true
	mustOK(t, h.e.SaveSettings(s))
	if h.p.ScreenRecordingRequests() != 1 {
		t.Errorf("turning on live previews should ask for Screen Recording")
	}
	// Saving with them on already doesn't ask again.
	mustOK(t, h.e.SaveSettings(s))
	if got := h.p.ScreenRecordingRequests(); got != 1 {
		t.Errorf("Screen Recording requests = %d, want 1", got)
	}
}

func TestSaveSettingsChangesTheSizeClass(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	if got := h.view().SizeClass; got != "compact" {
		t.Fatalf("size class on a laptop: %s", got)
	}
	s := h.view().Settings
	s.CompactBelow = 1400
	mustOK(t, h.e.SaveSettings(s))
	if got := h.view().SizeClass; got != "wide" {
		t.Errorf("size class with compact_below under the screen's width: %s", got)
	}
}

func TestSaveSettingsWithConfigProblems(t *testing.T) {
	bad := "[[tab]]\nname = \"Mine\"\nlayout = \"nope\"\n"
	h := newHarness(t, setup{config: bad})
	s := h.view().Settings
	s.Gap = 12
	mustOK(t, h.e.SaveSettings(s))
	if got := h.view().Settings.Gap; got != 12 {
		t.Errorf("the change should apply: gap %v", got)
	}
	data, _ := os.ReadFile(filepath.Join(h.dir, config.ConfigFile))
	if string(data) != bad {
		t.Errorf("config.toml was written while it has problems")
	}
}

func TestSaveSettingsDoesntHoldTheArrangement(t *testing.T) {
	h := newHarness(t, setup{})
	// A folder in the way of config.toml.bak stops config.toml being saved.
	bak := filepath.Join(h.dir, config.BackupFile)
	mustOK(t, os.RemoveAll(bak))
	mustOK(t, os.MkdirAll(filepath.Join(bak, "in-the-way"), 0o755))
	state := filepath.Join(h.dir, config.StateFile)
	mustOK(t, os.Remove(state))
	s := h.view().Settings
	s.Gap = 12
	if err := h.e.SaveSettings(s); err == nil {
		t.Fatal("SaveSettings should report that config.toml couldn't be saved")
	}
	// The settings don't change the tabs, so state.json still matches the
	// old config.toml, and goes on being saved.
	if hasPrefix(texts(h.view().Notices), "Bench couldn't save config.toml, so it isn't saving the arrangement") {
		t.Errorf("notices: %v", texts(h.view().Notices))
	}
	h.settle()
	if _, err := os.Stat(state); err != nil {
		t.Errorf("state.json should be saved: %v", err)
	}
}

func TestSaveSettingsSavesTheHeldArrangement(t *testing.T) {
	h := newHarness(t, setup{})
	// A folder in the way of config.toml.bak stops config.toml being saved,
	// so state.json waits for it after a change to the tabs.
	bak := filepath.Join(h.dir, config.BackupFile)
	mustOK(t, os.RemoveAll(bak))
	mustOK(t, os.MkdirAll(filepath.Join(bak, "in-the-way"), 0o755))
	if err := h.e.SaveTab(1, draftOf(h.view(), "Build", "main-side", "stack")); err == nil {
		t.Fatal("SaveTab should report that config.toml couldn't be saved")
	}
	s := h.view().Settings
	s.Gap = 12
	if err := h.e.SaveSettings(s); err == nil {
		t.Fatal("SaveSettings should report that config.toml couldn't be saved")
	}
	if !hasPrefix(texts(h.view().Notices), "Bench couldn't save config.toml, so it isn't saving the arrangement") {
		t.Errorf("notices: %v", texts(h.view().Notices))
	}
	// Once config.toml can be saved again, saving Settings saves it with the
	// tab's change too, and state.json follows.
	mustOK(t, os.RemoveAll(bak))
	mustOK(t, h.e.SaveSettings(s))
	cfg := savedConfig(t, h)
	if cfg.Tabs[1].Name != "Build" || cfg.Settings.Gap != 12 {
		t.Errorf("config.toml: tab %q, gap %v", cfg.Tabs[1].Name, cfg.Settings.Gap)
	}
	if st, err := stateOnDisk(t, h); err != nil || st.Tabs[1].Name != "Build" {
		t.Errorf("after config.toml was saved again: %v", err)
	}
	if hasPrefix(texts(h.view().Notices), "Bench couldn't save config.toml") {
		t.Errorf("the notice should go: %v", texts(h.view().Notices))
	}
}
