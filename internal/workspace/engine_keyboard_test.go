package workspace

import (
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// shortcutView returns Settings ▸ Keyboard's row for a field of the
// [shortcuts] table, failing the test if there isn't one.
func (h *harness) shortcutView(field string) ShortcutView {
	h.t.Helper()
	for _, s := range h.view().Shortcuts {
		if s.Field == field {
			return s
		}
	}
	h.t.Fatalf("no shortcut row for %s", field)
	return ShortcutView{}
}

// TestShortcutViews checks Settings ▸ Keyboard's rows before the shell
// reports anything: every field of the [shortcuts] table, in config order,
// with what it does, its keys, its default, and how macOS shows it.
func TestShortcutViews(t *testing.T) {
	h := newHarness(t, setup{})
	v := h.view()
	var fields []string
	for _, s := range v.Shortcuts {
		fields = append(fields, s.Field)
	}
	want := []string{"tab_modifiers", "next_tab", "previous_tab", "last_tab", "jump", "show_all_tabs", "rearrange",
		"focus_pane_modifiers", "move_window_modifiers", "send_to_tab_modifiers", "next_window", "zoom_pane",
		"grow_pane", "shrink_pane", "equal_panes", "next_layout", "previous_layout", "add_front_window"}
	if !slices.Equal(fields, want) {
		t.Fatalf("fields %v, want %v", fields, want)
	}
	defaults := config.DefaultShortcuts().Fields()
	for _, s := range v.Shortcuts {
		if s.Label == "" || s.Default != defaults[s.Field] || s.Problem != "" || s.Suggestion != "" {
			t.Errorf("%s: %+v", s.Field, s)
		}
	}
	jump := h.shortcutView("jump")
	if jump.Label != "Jump to…" || jump.Value != "ctrl+alt+space" || jump.Display != "⌃⌥Space" || jump.Keys != "" {
		t.Errorf("jump: %+v", jump)
	}
	tabs := h.shortcutView("tab_modifiers")
	if tabs.Label != "Tabs 1 to 9" || tabs.Keys != "1 to 9" || tabs.Value != "ctrl+alt" || tabs.Display != "⌃⌥" {
		t.Errorf("tab_modifiers: %+v", tabs)
	}
	if focus := h.shortcutView("focus_pane_modifiers"); focus.Keys != "the arrow keys" {
		t.Errorf("focus_pane_modifiers: %+v", focus)
	}
	send := h.shortcutView("send_to_tab_modifiers")
	if send.Label != "Send a window to a tab" || send.Keys != "1 to 9, and ` for the last used tab" || send.Value != "ctrl+alt+shift" || send.Display != "⌃⌥⇧" {
		t.Errorf("send_to_tab_modifiers: %+v", send)
	}
	if got := v.Keys.SendToTab; got != "⌃⌥⇧" {
		t.Errorf("the View's keys.sendToTab = %q, want ⌃⌥⇧", got)
	}
}

// TestReportShortcuts checks that what the shell reports shows in Settings
// ▸ Keyboard and as notices, and that a row whose field changes is taken to
// work until the shell reports again.
func TestReportShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	h.e.SetNotices("other", []string{"Another."})
	notice := NoticeView{
		Text: "Couldn't register ⌃⌥Space (another app is using it). Try ⌃⌥J.",
		Actions: []NoticeAction{
			{Label: "Use this instead", Method: "UseSuggestedShortcuts", Arg: "jump"},
			{Label: "Keyboard settings…", Command: "keyboard-settings"},
		},
	}
	h.e.ReportShortcuts(ShortcutReport{
		Fields: []ShortcutView{{
			Field: "jump", Value: "ctrl+alt+space", Display: "⌃⌥Space",
			Problem: "Another app is using ⌃⌥Space.", Suggestion: "ctrl+alt+j", SuggestionDisplay: "⌃⌥J",
		}},
		Notices: []NoticeView{notice},
	})
	v := h.view()
	// The notices come in the order their sources first gave them.
	if got := texts(v.Notices); !slices.Equal(got, []string{"Another.", notice.Text}) {
		t.Errorf("notices %v", got)
	}
	if n, ok := noticeWith(v.Notices, "Couldn't register"); !ok || !slices.Equal(n.Actions, notice.Actions) {
		t.Errorf("the notice's actions: %+v", n.Actions)
	}
	jump := h.shortcutView("jump")
	if jump.Problem == "" || jump.Suggestion != "ctrl+alt+j" || jump.SuggestionDisplay != "⌃⌥J" ||
		jump.Label != "Jump to…" || jump.Default != "ctrl+alt+space" {
		t.Errorf("jump after the report: %+v", jump)
	}
	if next := h.shortcutView("next_tab"); next.Problem != "" || next.Display != "⌃⌥]" {
		t.Errorf("next_tab: %+v", next)
	}

	// Changing the field drops what the shell said about its old value.
	s := h.view().Settings
	s.Shortcuts = map[string]string{"jump": "ctrl+alt+k"}
	mustOK(t, h.e.SaveSettings(s))
	if jump := h.shortcutView("jump"); jump.Problem != "" || jump.Suggestion != "" || jump.Display != "⌃⌥K" {
		t.Errorf("jump after changing it: %+v", jump)
	}

	// A report without notices clears the shortcuts' notices only.
	h.e.ReportShortcuts(ShortcutReport{})
	if got := texts(h.view().Notices); !slices.Equal(got, []string{"Another."}) {
		t.Errorf("notices after a clean report %v", got)
	}
}

// TestSaveSettingsShortcuts checks that Settings ▸ Keyboard's changes are
// saved, that the shell is told to register the shortcuts again only when
// they change, and that a change that would give the table a problem is
// refused.
func TestSaveSettingsShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	if s.Shortcuts != nil {
		t.Errorf("the view's settings carry shortcuts: %v", s.Shortcuts)
	}
	mustOK(t, h.e.SaveSettings(s))
	h.drain()
	if got := h.em.shortcutsChanged(); got != 0 {
		t.Errorf("ShortcutsChanged %d times without a change", got)
	}

	s.Shortcuts = map[string]string{"jump": "ctrl+alt+j", "zoom_pane": "", "focus_pane_modifiers": "ctrl+cmd"}
	mustOK(t, h.e.SaveSettings(s))
	h.drain()
	if got := h.em.shortcutsChanged(); got != 1 {
		t.Errorf("ShortcutsChanged %d times, want once", got)
	}
	cfg := savedConfig(t, h)
	if cfg.Shortcuts.Jump != "ctrl+alt+j" || cfg.Shortcuts.ZoomPane != "" || cfg.Shortcuts.FocusPaneModifiers != "ctrl+cmd" ||
		cfg.Shortcuts.NextTab != "ctrl+alt+]" {
		t.Errorf("config.toml's shortcuts: %+v", cfg.Shortcuts)
	}
	v := h.view()
	if !strings.Contains(v.Keys.Jump, "J") {
		t.Errorf("the view's jump key %q", v.Keys.Jump)
	}
	if zoom := h.shortcutView("zoom_pane"); zoom.Value != "" || zoom.Display != "" || zoom.Default == "" {
		t.Errorf("zoom_pane turned off: %+v", zoom)
	}
	if got := h.em.lastToast(); got != "Saved settings" {
		t.Errorf("toast %q", got)
	}

	for _, c := range []struct {
		changes map[string]string
		want    string
	}{
		{map[string]string{"jump": "ctrl+alt+]"}, "Bench can't save that shortcut: ⌃⌥] is already the shortcut for Next tab."},
		{map[string]string{"jump": "ctrl+alt+hyper"}, "Bench can't save that shortcut: "},
		{map[string]string{"open_sesame": "ctrl+o"}, "Bench has no shortcut called open_sesame."},
	} {
		bad := h.view().Settings
		bad.Shortcuts = c.changes
		err := h.e.SaveSettings(bad)
		if err == nil || !strings.HasPrefix(err.Error(), c.want) {
			t.Errorf("SaveSettings(%v) = %v, want %q", c.changes, err, c.want)
		}
	}
	if got := savedConfig(t, h).Shortcuts.Jump; got != "ctrl+alt+j" {
		t.Errorf("jump after the refusals %q", got)
	}
	if got := h.em.shortcutsChanged(); got != 1 {
		t.Errorf("ShortcutsChanged %d times after the refusals, want once", got)
	}
}

// TestSaveSettingsKeepsAProblemItHad checks that a [shortcuts] table with a
// problem already, from editing config.toml, can still be changed in
// Settings, so long as the change adds no problem.
func TestSaveSettingsKeepsAProblemItHad(t *testing.T) {
	h := newHarness(t, setup{})
	editConfig(t, h, `jump = "ctrl+alt+space"`, `jump = "ctrl+alt+]"`)
	mustOK(t, h.e.ReloadConfig())
	h.drain()
	s := h.view().Settings
	s.Shortcuts = map[string]string{"zoom_pane": "ctrl+alt+z"}
	mustOK(t, h.e.SaveSettings(s))
	// Fixing the problem is a change too.
	s.Shortcuts = map[string]string{"jump": "ctrl+alt+j"}
	mustOK(t, h.e.SaveSettings(s))
	if cfg := savedConfig(t, h); cfg.Shortcuts.ZoomPane != "ctrl+alt+z" || cfg.Shortcuts.Jump != "ctrl+alt+j" {
		t.Errorf("config.toml's shortcuts: %+v", cfg.Shortcuts)
	}
}

// TestUseSuggestedShortcuts checks a notice's Use this instead: it takes
// the shell's suggestions for the fields as they are now, saves them, and
// says what changed.
func TestUseSuggestedShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	mustErr(t, h.e.UseSuggestedShortcuts("jump"), "Bench has no other shortcut to suggest now. Choose one in Settings ▸ Keyboard.")

	h.e.ReportShortcuts(ShortcutReport{Fields: []ShortcutView{
		{Field: "jump", Value: "ctrl+alt+space", Problem: "Taken.", Suggestion: "ctrl+alt+j"},
		{Field: "tab_modifiers", Value: "ctrl+alt", Problem: "VoiceOver.", Suggestion: "ctrl+cmd"},
		{Field: "focus_pane_modifiers", Value: "ctrl+alt", Problem: "VoiceOver.", Suggestion: "ctrl+shift+cmd"},
	}})
	mustOK(t, h.e.UseSuggestedShortcuts("jump"))
	h.drain()
	if got := h.em.lastToast(); got != "Changed Jump to… to ⌃⌥J" {
		t.Errorf("toast %q", got)
	}
	if got := h.em.shortcutsChanged(); got != 1 {
		t.Errorf("ShortcutsChanged %d times, want once", got)
	}
	if got := savedConfig(t, h).Shortcuts.Jump; got != "ctrl+alt+j" {
		t.Errorf("jump %q", got)
	}
	// The suggestion was for the old value.
	mustErr(t, h.e.UseSuggestedShortcuts("jump"), "Bench has no other shortcut to suggest now. Choose one in Settings ▸ Keyboard.")

	mustOK(t, h.e.UseSuggestedShortcuts("tab_modifiers focus_pane_modifiers"))
	if got := h.em.lastToast(); got != "Changed the shortcuts" {
		t.Errorf("toast for several %q", got)
	}
	if cfg := savedConfig(t, h); cfg.Shortcuts.TabModifiers != "ctrl+cmd" || cfg.Shortcuts.FocusPaneModifiers != "ctrl+shift+cmd" {
		t.Errorf("config.toml's shortcuts: %+v", cfg.Shortcuts)
	}
}

// TestShortcutsToast checks what Use this instead says it did.
func TestShortcutsToast(t *testing.T) {
	for _, c := range []struct {
		fields map[string]string
		want   string
	}{
		{map[string]string{"jump": "ctrl+alt+j"}, "Changed Jump to… to ⌃⌥J"},
		{map[string]string{"tab_modifiers": "ctrl+cmd"}, "Changed Tabs 1 to 9 to ⌃⌘ with 1 to 9"},
		{map[string]string{"move_window_modifiers": "ctrl+shift+cmd"}, "Changed Move a window to ⌃⇧⌘ with the arrow keys"},
		{map[string]string{"zoom_pane": ""}, "Turned off the shortcut for Zoom pane"},
		{map[string]string{"jump": "ctrl+alt+j", "zoom_pane": ""}, "Changed the shortcuts"},
	} {
		if got := shortcutsToast(c.fields); got != c.want {
			t.Errorf("shortcutsToast(%v) = %q, want %q", c.fields, got, c.want)
		}
	}
}

// TestSystemKeysChangedRegistersAgain checks that the shell is told to
// register the shortcuts again, and so look for clashes again, when the
// keys that macOS keeps for itself may have changed.
func TestSystemKeysChangedRegistersAgain(t *testing.T) {
	h := newHarness(t, setup{})
	h.drain()
	before := h.em.shortcutsChanged()
	h.p.SetSystemKeys(platform.SystemKeys{VoiceOver: true})
	h.p.EmitEvent(platform.Event{Kind: platform.SystemKeysChanged})
	h.settle()
	if got := h.em.shortcutsChanged(); got != before+1 {
		t.Errorf("ShortcutsChanged %d times, want %d", got, before+1)
	}
}

// TestSaveSettingsWhereWindowsGo checks Settings' choices of where new
// windows go, what happens when another tab's window comes forward, hiding
// the apps that aren't in any tab, and how long to wait for apps to open.
func TestSaveSettingsWhereWindowsGo(t *testing.T) {
	h := newHarness(t, setup{})
	s := h.view().Settings
	if s.NewWindows != config.NewWindowsAppPane || s.FollowFocus != config.FollowSwitchTab || s.HideOtherApps || s.LaunchTimeout != "15s" {
		t.Errorf("defaults: %q %q %v %q", s.NewWindows, s.FollowFocus, s.HideOtherApps, s.LaunchTimeout)
	}
	s.NewWindows = config.NewWindowsFocusedPane
	s.FollowFocus = config.FollowBringHere
	s.HideOtherApps = true
	s.LaunchTimeout = "30s"
	mustOK(t, h.e.SaveSettings(s))
	v := h.view().Settings
	if v.NewWindows != config.NewWindowsFocusedPane || v.FollowFocus != config.FollowBringHere || !v.HideOtherApps || v.LaunchTimeout != "30s" {
		t.Errorf("view: %q %q %v %q", v.NewWindows, v.FollowFocus, v.HideOtherApps, v.LaunchTimeout)
	}
	cfg := savedConfig(t, h)
	if cfg.Settings.NewWindows != config.NewWindowsFocusedPane || cfg.Settings.FollowFocus != config.FollowBringHere ||
		!cfg.Settings.HideOtherApps || cfg.Settings.LaunchTimeout.String() != "30s" {
		t.Errorf("config.toml: %+v", cfg.Settings)
	}
	s.NewWindows = config.NewWindowsFloat
	s.FollowFocus = config.FollowOff
	s.LaunchTimeout = "0s"
	mustErr(t, h.e.SaveSettings(s), "The launch timeout must be a length of time, such as 15s.")
	s.LaunchTimeout = "1m"
	mustOK(t, h.e.SaveSettings(s))
	if cfg := savedConfig(t, h); cfg.Settings.NewWindows != config.NewWindowsFloat || cfg.Settings.FollowFocus != config.FollowOff ||
		cfg.Settings.LaunchTimeout.String() != "1m0s" {
		t.Errorf("config.toml: %+v", cfg.Settings)
	}
}

// TestCheckShortcuts checks that Settings ▸ Keyboard's draft is checked
// against the whole table, with its changes, by the shell, and that each
// row comes back filled in, whatever the shell says.
func TestCheckShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	h.em.check = func(fields map[string]string) []ShortcutView {
		return []ShortcutView{
			{Field: "jump", Value: fields["jump"], Problem: "Taken.", Suggestion: "ctrl+alt+k", SuggestionDisplay: "⌃⌥K"},
			// An answer about another value is dropped.
			{Field: "next_tab", Value: "ctrl+alt+x", Problem: "Stale."},
		}
	}
	rows, err := h.e.CheckShortcuts(map[string]string{"jump": " ctrl+alt+j ", "zoom_pane": ""})
	mustOK(t, err)
	if len(h.em.checked) != 1 {
		t.Fatalf("CheckShortcuts asked the shell %d times", len(h.em.checked))
	}
	asked := h.em.checked[0]
	if asked["jump"] != "ctrl+alt+j" || asked["zoom_pane"] != "" || asked["next_tab"] != "ctrl+alt+]" || len(asked) != 18 {
		t.Errorf("the shell was asked about %v", asked)
	}
	if len(rows) != 18 {
		t.Fatalf("%d rows", len(rows))
	}
	byField := map[string]ShortcutView{}
	for _, r := range rows {
		byField[r.Field] = r
	}
	if j := byField["jump"]; j.Problem != "Taken." || j.Suggestion != "ctrl+alt+k" || j.Display != "⌃⌥J" || j.Label != "Jump to…" ||
		j.Default != "ctrl+alt+space" {
		t.Errorf("jump: %+v", j)
	}
	if n := byField["next_tab"]; n.Problem != "" || n.Display != "⌃⌥]" {
		t.Errorf("next_tab: %+v", n)
	}
	if z := byField["zoom_pane"]; z.Value != "" || z.Display != "" {
		t.Errorf("zoom_pane: %+v", z)
	}
	if got := savedConfig(t, h).Shortcuts.Jump; got != "ctrl+alt+space" {
		t.Errorf("CheckShortcuts saved jump %q", got)
	}
	if _, err := h.e.CheckShortcuts(map[string]string{"open_sesame": "ctrl+o"}); err == nil ||
		err.Error() != "Bench has no shortcut called open_sesame." {
		t.Errorf("an unknown field: %v", err)
	}
}

// TestPauseShortcuts checks that Settings ▸ Keyboard can release the
// global shortcuts while it records one, and that they're registered again
// when it says so, or once no menu or dialog is open, or the page loads
// again, in case it didn't.
func TestPauseShortcuts(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SetOverlay(true))
	mustOK(t, h.e.PauseShortcuts(true))
	mustOK(t, h.e.PauseShortcuts(true))
	mustOK(t, h.e.PauseShortcuts(false))
	if got := h.em.paused(); !slices.Equal(got, []bool{true, false}) {
		t.Errorf("pauses %v, want true then false", got)
	}
	mustOK(t, h.e.SetOverlay(false))
	if got := h.em.paused(); len(got) != 2 {
		t.Errorf("closing the dialog paused again: %v", got)
	}

	mustOK(t, h.e.SetOverlay(true))
	mustOK(t, h.e.PauseShortcuts(true))
	mustOK(t, h.e.SetOverlay(false))
	if got := h.em.paused(); !slices.Equal(got, []bool{true, false, true, false}) {
		t.Errorf("pauses after closing the dialog %v", got)
	}
	mustOK(t, h.e.PauseShortcuts(true))
	_, err := h.e.Ready()
	mustOK(t, err)
	if got := h.em.paused(); !slices.Equal(got, []bool{true, false, true, false, true, false}) {
		t.Errorf("pauses after loading the page again %v", got)
	}
}
