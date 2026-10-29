package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
)

func TestSaveSettings(t *testing.T) {
	h := newHarness(t, setup{})
	before := h.view()
	s := before.Settings
	s.Gap = 12
	s.FollowFocus = "off"
	s.OpenMissingApps = false
	s.CompactBelow = 1600
	mustOK(t, h.e.SaveSettings(s))
	v := h.view()
	if v.Settings != s {
		t.Errorf("settings: %+v, want %+v", v.Settings, s)
	}
	if got := h.em.lastToast(); got != "Saved settings" {
		t.Errorf("toast: %q", got)
	}
	cfg := savedConfig(t, h)
	if cfg.Settings.Gap != 12 || cfg.Settings.FollowFocus != "off" || cfg.Settings.OpenMissingApps ||
		cfg.Settings.CompactBelow != 1600 {
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
	if got := h.view().Settings; got != v.Settings {
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
	if got := h.view().Settings.Gap; got != 8 {
		t.Errorf("gap after the refused save = %v, want 8", got)
	}
	if got := savedConfig(t, h).Settings.Gap; got != 8 {
		t.Errorf("config.toml's gap = %v, want 8", got)
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
