package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseKeepsDefaultsForMissingSettings(t *testing.T) {
	cfg := mustParse(t, `
[settings]
gap = 12
launch_timeout = "1m30s"

[shortcuts]
jump = "ctrl+alt+j"
`+oneTab)
	wantSettings := DefaultSettings()
	wantSettings.Gap = 12
	wantSettings.LaunchTimeout.Duration = 90 * time.Second
	if cfg.Settings != wantSettings {
		t.Errorf("settings = %+v, want %+v", cfg.Settings, wantSettings)
	}
	wantShortcuts := Default().Shortcuts
	wantShortcuts.Jump = "ctrl+alt+j"
	if cfg.Shortcuts != wantShortcuts {
		t.Errorf("shortcuts = %+v, want %+v", cfg.Shortcuts, wantShortcuts)
	}
	if want := []Tab{{Name: "Code", Layout: "single"}}; !reflect.DeepEqual(cfg.Tabs, want) {
		t.Errorf("tabs = %+v, want %+v", cfg.Tabs, want)
	}
	if cfg.Rules != nil {
		t.Errorf("rules = %+v, want none (the default rule isn't added)", cfg.Rules)
	}
}

// TestParseKeepsDefaultsForSettingsOnByDefault checks that settings that are
// on by default stay on in a file from before they existed, rather than
// taking Go's zero value.
func TestParseKeepsDefaultsForSettingsOnByDefault(t *testing.T) {
	cfg := mustParse(t, `
[settings]
gap = 4
menu_bar = "icon"
`+oneTab)
	s := cfg.Settings
	if !s.FillScreen || !s.ShowOnTabShortcut || s.MenuBar != MenuBarIcon {
		t.Errorf("fill_screen %v, show_on_tab_shortcut %v, menu_bar %q; want true, true, icon", s.FillScreen, s.ShowOnTabShortcut, s.MenuBar)
	}
	if !s.ConfirmQuitBusyTerminals {
		t.Error("confirm_quit_busy_terminals = false, want true")
	}
	if !s.ConfirmClose {
		t.Error("confirm_close = false, want true")
	}
	if !s.ShowStatusBar || s.TabShortcutHints != TabHintsAuto || s.ScrollBars != ScrollBarsAuto {
		t.Errorf("show_status_bar %v, tab_shortcut_hints %q, scroll_bars %q; want true, auto, auto", s.ShowStatusBar, s.TabShortcutHints, s.ScrollBars)
	}
	if s.CheckForUpdates {
		t.Error("check_for_updates = true, want false: Bench checks for updates only if the user asks")
	}
	cfg = mustParse(t, "[settings]\ncheck_for_updates = true\n"+oneTab)
	if !cfg.Settings.CheckForUpdates || !cfg.Settings.FillScreen {
		t.Errorf("check_for_updates %v, fill_screen %v; want true, true", cfg.Settings.CheckForUpdates, cfg.Settings.FillScreen)
	}
	cfg = mustParse(t, "[settings]\nshow_on_tab_shortcut = false\n"+oneTab)
	if cfg.Settings.ShowOnTabShortcut || cfg.Settings.MenuBar != MenuBarName {
		t.Errorf("show_on_tab_shortcut %v, menu_bar %q; want false, name", cfg.Settings.ShowOnTabShortcut, cfg.Settings.MenuBar)
	}
	cfg = mustParse(t, "[settings]\nunread_counts = false\n"+oneTab)
	if cfg.Settings.UnreadCounts || !cfg.Settings.ShowOnTabShortcut {
		t.Errorf("unread_counts %v, show_on_tab_shortcut %v; want false, true", cfg.Settings.UnreadCounts, cfg.Settings.ShowOnTabShortcut)
	}
	if !mustParse(t, oneTab).Settings.UnreadCounts {
		t.Error("unread_counts = false when the file leaves it out, want true")
	}
	cfg = mustParse(t, "[settings]\nconfirm_quit_busy_terminals = false\n"+oneTab)
	if cfg.Settings.ConfirmQuitBusyTerminals || !cfg.Settings.ShowOnTabShortcut {
		t.Errorf("confirm_quit_busy_terminals %v, show_on_tab_shortcut %v; want false, true", cfg.Settings.ConfirmQuitBusyTerminals, cfg.Settings.ShowOnTabShortcut)
	}
	cfg = mustParse(t, "[settings]\nconfirm_close = false\n"+oneTab)
	if cfg.Settings.ConfirmClose || !cfg.Settings.ConfirmQuitBusyTerminals {
		t.Errorf("confirm_close %v, confirm_quit_busy_terminals %v; want false, true", cfg.Settings.ConfirmClose, cfg.Settings.ConfirmQuitBusyTerminals)
	}
	cfg = mustParse(t, "[settings]\nshow_status_bar = false\ntab_shortcut_hints = \"always\"\n"+oneTab)
	if cfg.Settings.ShowStatusBar || cfg.Settings.TabShortcutHints != TabHintsAlways || cfg.Settings.Gap != 4 || cfg.Settings.ScrollBars != ScrollBarsAuto {
		t.Errorf("show_status_bar %v, tab_shortcut_hints %q, gap %v, scroll_bars %q; want false, always, 4, auto",
			cfg.Settings.ShowStatusBar, cfg.Settings.TabShortcutHints, cfg.Settings.Gap, cfg.Settings.ScrollBars)
	}
	cfg = mustParse(t, "[settings]\nscroll_bars = \"never\"\n"+oneTab)
	if cfg.Settings.ScrollBars != ScrollBarsNever || cfg.Settings.TabShortcutHints != TabHintsAuto {
		t.Errorf("scroll_bars %q, tab_shortcut_hints %q; want never, auto", cfg.Settings.ScrollBars, cfg.Settings.TabShortcutHints)
	}
	if cfg.Settings.Density != DensityAuto || cfg.Settings.PaneHeaders != PaneHeadersAlways {
		t.Errorf("density %q, pane_headers %q; want auto, always", cfg.Settings.Density, cfg.Settings.PaneHeaders)
	}
	// A file that leaves the appearance out matches macOS, the default.
	if cfg.Settings.Appearance != AppearanceSystem {
		t.Errorf("appearance %q, want system", cfg.Settings.Appearance)
	}
	cfg = mustParse(t, "[settings]\ndensity = \"compact\"\npane_headers = \"auto\"\n"+oneTab)
	if cfg.Settings.Density != DensityCompact || cfg.Settings.PaneHeaders != PaneHeadersAuto || cfg.Settings.ScrollBars != ScrollBarsAuto {
		t.Errorf("density %q, pane_headers %q, scroll_bars %q; want compact, auto, auto", cfg.Settings.Density, cfg.Settings.PaneHeaders, cfg.Settings.ScrollBars)
	}
	// A file that says dark, as one that Bench wrote while dark was the
	// default does, stays dark.
	cfg = mustParse(t, "[settings]\nappearance = \"dark\"\n"+oneTab)
	if cfg.Settings.Appearance != AppearanceDark || cfg.Settings.Density != DensityAuto {
		t.Errorf("appearance %q, density %q; want dark, auto", cfg.Settings.Appearance, cfg.Settings.Density)
	}
	// A file from before the terminal settings gets their defaults, not
	// a text size of 0, which would be a problem.
	if s := cfg.Settings; s.TerminalFontSize != 12.5 || s.TerminalOptionAsMeta {
		t.Errorf("terminal_font_size %v, terminal_option_as_meta %v; want 12.5, false", s.TerminalFontSize, s.TerminalOptionAsMeta)
	}
	cfg = mustParse(t, "[settings]\nterminal_font_size = 15\nterminal_option_as_meta = true\n"+oneTab)
	if s := cfg.Settings; s.TerminalFontSize != 15 || !s.TerminalOptionAsMeta || s.Density != DensityAuto {
		t.Errorf("terminal_font_size %v, terminal_option_as_meta %v, density %q; want 15, true, auto", s.TerminalFontSize, s.TerminalOptionAsMeta, s.Density)
	}
	// Notifications and kept output are on unless the file turns them off.
	if s := cfg.Settings; !s.NotifyTerminals || !s.KeepTerminalOutput {
		t.Errorf("notify_terminals %v, keep_terminal_output %v; want true, true", s.NotifyTerminals, s.KeepTerminalOutput)
	}
	cfg = mustParse(t, "[settings]\nnotify_terminals = false\nkeep_terminal_output = false\n"+oneTab)
	if s := cfg.Settings; s.NotifyTerminals || s.KeepTerminalOutput || !s.ConfirmClose {
		t.Errorf("notify_terminals %v, keep_terminal_output %v, confirm_close %v; want false, false, true", s.NotifyTerminals, s.KeepTerminalOutput, s.ConfirmClose)
	}
}

func TestParseWithoutSettingsOrShortcuts(t *testing.T) {
	cfg := mustParse(t, oneTab)
	if def := Default(); cfg.Settings != def.Settings || cfg.Shortcuts != def.Shortcuts {
		t.Errorf("got %+v and %+v, want the defaults", cfg.Settings, cfg.Shortcuts)
	}
}

// TestParseOldShortcutsKeepLastTabOff checks that a file written before
// last_tab existed, which gives its default to another field, as Suggest
// offers "ctrl+alt+`" for next_window, gets last_tab off rather than a
// clash. A file that sets last_tab keeps it, clash or not.
func TestParseOldShortcutsKeepLastTabOff(t *testing.T) {
	cfg := mustParse(t, "[shortcuts]\nnext_window = \"ctrl+alt+`\"\n"+oneTab)
	if s := cfg.Shortcuts; s.LastTab != "" || s.NextWindow != "ctrl+alt+`" {
		t.Errorf("last_tab %q, next_window %q; want off, ctrl+alt+`", s.LastTab, s.NextWindow)
	}
	cfg = mustParse(t, "[shortcuts]\njump = \"ctrl+alt+j\"\n"+oneTab)
	if got := cfg.Shortcuts.LastTab; got != "ctrl+alt+`" {
		t.Errorf("without a clash, last_tab = %q, want the default", got)
	}
	cfg = mustParse(t, "[shortcuts]\nnext_window = \"ctrl+alt+`\"\nlast_tab = \"ctrl+alt+`\"\n"+oneTab)
	if got := cfg.Shortcuts.LastTab; got != "ctrl+alt+`" {
		t.Errorf("set in the file, last_tab = %q, want it kept", got)
	}
}

// TestParseOldShortcutsKeepSendToTabOff checks that a file written before
// send_to_tab_modifiers existed, which gives one of its default shortcuts
// to another field, gets it off rather than a clash, even when the field
// is add_front_window, which binds before it. A file that sets it keeps it,
// clash or not.
func TestParseOldShortcutsKeepSendToTabOff(t *testing.T) {
	for _, doc := range []string{
		"[shortcuts]\njump = \"ctrl+alt+shift+1\"\n",
		"[shortcuts]\nadd_front_window = \"ctrl+alt+shift+`\"\n",
		// The previous window, next_window with Shift added.
		"[shortcuts]\nnext_window = \"ctrl+alt+`\"\n",
	} {
		if got := mustParse(t, doc+oneTab).Shortcuts.SendToTabModifiers; got != "" {
			t.Errorf("%q: send_to_tab_modifiers = %q, want off", doc, got)
		}
	}
	cfg := mustParse(t, "[shortcuts]\njump = \"ctrl+alt+j\"\n"+oneTab)
	if got := cfg.Shortcuts.SendToTabModifiers; got != "ctrl+alt+shift" {
		t.Errorf("without a clash, send_to_tab_modifiers = %q, want the default", got)
	}
	cfg = mustParse(t, "[shortcuts]\njump = \"ctrl+alt+shift+1\"\nsend_to_tab_modifiers = \"ctrl+alt+shift\"\n"+oneTab)
	if got := cfg.Shortcuts.SendToTabModifiers; got != "ctrl+alt+shift" {
		t.Errorf("set in the file, send_to_tab_modifiers = %q, want it kept", got)
	}
	// last_tab goes off first, and send_to_tab_modifiers then only if it
	// still clashes.
	cfg = mustParse(t, "[shortcuts]\nnext_tab = \"ctrl+alt+`\"\n"+oneTab)
	if s := cfg.Shortcuts; s.LastTab != "" || s.SendToTabModifiers != "ctrl+alt+shift" {
		t.Errorf("last_tab %q, send_to_tab_modifiers %q; want off, the default", s.LastTab, s.SendToTabModifiers)
	}
}

func TestParseNeedsATab(t *testing.T) {
	for _, doc := range []string{"", "# Nothing here.\n", "[settings]\ngap = 8\n", "tab = []\n"} {
		cfg, problems, err := Parse([]byte(doc))
		if err != nil || cfg != nil {
			t.Errorf("Parse(%q) = %v, %v; want problems only", doc, cfg, err)
		}
		checkWheres(t, problems, []where{{"", "tab"}})
	}
}

func TestParseEmptyListsAreNil(t *testing.T) {
	cfg := mustParse(t, `
[[tab]]
name = "Code"
layout = "side"
sizes = []

  [[tab.pane]]
  items = []

  [[tab.pane]]
`)
	want := []Tab{{Name: "Code", Layout: "side", Panes: []Pane{{}, {}}}}
	if !reflect.DeepEqual(cfg.Tabs, want) {
		t.Errorf("tabs = %#v, want %#v", cfg.Tabs, want)
	}
}

func TestParseItemsAsArrayTables(t *testing.T) {
	cfg := mustParse(t, `
[[tab]]
name = "Code"
layout = "single"

  [[tab.pane]]

    [[tab.pane.items]]
    app = "dev.zed.Zed"

    [[tab.pane.items]]
    terminal = "~/code"
    command = "htop"
`)
	want := []Item{{App: "dev.zed.Zed"}, {Terminal: "~/code", Command: "htop"}}
	if got := cfg.Tabs[0].Panes[0].Items; !reflect.DeepEqual(got, want) {
		t.Errorf("items = %+v, want %+v", got, want)
	}
}

func TestParseLaunchers(t *testing.T) {
	cfg := mustParse(t, `
[[launcher]]
name = "Claude"
folder = "~/code/bench"
run = "claude --continue"

[[launcher]]
name = "Codex"
run = "codex"
start_again = false
`+oneTab+`
  [[tab.pane]]
  items = [{ terminal = "~", run = "htop" }, { terminal = "~", run = "top", start_again = false }]
`)
	no := false
	want := []Launcher{
		{Name: "Claude", Folder: "~/code/bench", Run: "claude --continue"},
		{Name: "Codex", Run: "codex", StartAgain: &no},
	}
	if !reflect.DeepEqual(cfg.Launchers, want) {
		t.Errorf("launchers = %+v, want %+v", cfg.Launchers, want)
	}
	if !cfg.Launchers[0].StartsAgain() || cfg.Launchers[1].StartsAgain() {
		t.Errorf("StartsAgain = %v, %v; want a missing start_again to mean true", cfg.Launchers[0].StartsAgain(), cfg.Launchers[1].StartsAgain())
	}
	items := cfg.Tabs[0].Panes[0].Items
	if !items[0].StartsAgain() || items[1].StartsAgain() {
		t.Errorf("items' StartsAgain = %v, %v; want a missing start_again to mean true", items[0].StartsAgain(), items[1].StartsAgain())
	}
	if cfg.LauncherIndex("codex") != 1 || cfg.LauncherIndex("Nope") != -1 {
		t.Errorf("LauncherIndex = %d, %d; want 1, -1", cfg.LauncherIndex("codex"), cfg.LauncherIndex("Nope"))
	}
}

func TestParseUnknownFields(t *testing.T) {
	doc := `bogus = 1
[settings]
gapp = 9
[shortcuts]
jumpp = "x"
[[tab]]
name = "A"
layout = "side"
colour = "red"
  [[tab.pane]]
  items = [{ app = "x", titel = "y" }, { web = "https://a.example", bar = { baz = 2 } }]
  extra = true
  [[tab.pane]]
  items = [{ app = "x" }, { app = "y", qux = 3 }]
[[tab]]
name = "B"
layout = "single"
[tab.wat]
x = 1
  [[tab.pane]]
    [[tab.pane.items]]
    app = "z"
    nam = "Z"
[[rule]]
app = "a"
float = true
flt = true
[nope]
z = 1
[[launcher]]
name = "Claude"
rnu = "claude"
`
	cfg, problems, err := Parse([]byte(doc))
	if err != nil || cfg != nil {
		t.Fatalf("Parse = %v, %v; want problems only", cfg, err)
	}
	checkWheres(t, problems, []where{
		{"", "bogus"},
		{"", "settings.gapp"},
		{"", "shortcuts.jumpp"},
		{"A", "colour"},
		{"A", "pane[1].items[1].titel"},
		{"A", "pane[1].items[2].bar"},
		{"A", "pane[1].extra"},
		{"A", "pane[2].items[2].qux"},
		{"B", "wat"},
		{"B", "pane[1].items[1].nam"},
		{"", "rule[1].flt"},
		{"", "nope"},
		{"", "launcher[1].rnu"},
	})
	messages := map[string]string{
		"bogus":                  "unknown field (line 1, column 1)",
		"settings.gapp":          `unknown field (line 3, column 1); did you mean "gap"?`,
		"shortcuts.jumpp":        `unknown field (line 5, column 1); did you mean "jump"?`,
		"colour":                 "unknown field (line 9, column 1)",
		"pane[1].items[1].titel": `unknown field (line 11, column 25); did you mean "title"?`,
		"pane[1].items[1].nam":   `unknown field (line 23, column 5); did you mean "name"?`,
		"wat":                    "unknown table (line 18, column 2)",
		"rule[1].flt":            `unknown field (line 27, column 1); did you mean "float"?`,
		"nope":                   "unknown table (line 28, column 2)",
		"launcher[1].rnu":        `unknown field (line 32, column 1); did you mean "run"?`,
	}
	for _, p := range problems {
		if want, ok := messages[p.Field]; ok && p.Message != want {
			t.Errorf("%s: message %q, want %q", p.Field, p.Message, want)
		}
	}
}

func TestParseUnknownFieldsWithOtherProblems(t *testing.T) {
	_, problems, _ := Parse([]byte(`
[settings]
gap = -1

[[tab]]
name = "Code"
layout = "single"
size = [50]

  [[tab.pane]]
  items = [{ app = "dev.zed.Zed", tittle = "Bench", url = "https://zed.dev" }]
`))
	checkWheres(t, problems, []where{
		{"Code", "size"},
		{"Code", "pane[1].items[1].tittle"},
		{"", "settings.gap"},
		{"Code", "pane[1].items[1].url"},
	})
}

func TestParseSyntaxErrors(t *testing.T) {
	tests := []struct {
		name  string
		doc   string
		want  where
		place string // "line L, column C"
	}{
		{"missing value", "[settings]\ngap = \n", where{"", "settings.gap"}, "line 2, column 7"},
		{"missing equals", "[[tab]]\nname = \"Code\"\nlayout \"side\"\n", where{"Code", ""}, "line 3, column 8"},
		{"in an unnamed tab", "[[tab]]\nlayout = \"side\"\n[[tab]]\nlayout = = 1\n", where{"tab 2", "layout"}, "line 4, column 10"},
		{"unclosed string", "[[tab]]\nname = \"Code\n", where{"tab 1", "name"}, "line 2"},
		{"unclosed array", oneTab + "sizes = [1, 2\n", where{"Code", "sizes"}, "line 5"},
		{"error on a later line of a value", oneTab + "sizes = [\n  1,\n  2 3,\n]\n", where{"Code", "sizes"}, "line 7"},
		{"after comments and blank lines", "[settings]\ngap = 1\n\n# A comment.\n  # Another.\nnew_windows = \"float\" x\n" + oneTab, where{"", "settings.new_windows"}, "line 6"},
		{"dotted key", oneTab + "  [[tab.pane]]\n  items . x = \n", where{"Code", "pane[1].items.x"}, "line 6"},
		{"in an inline item", oneTab + "  [[tab.pane]]\n  items = [{ app = \"x\", web = }]\n", where{"Code", "pane[1].items"}, "line 6"},
		{"in a key", "[settings]\nga$p = 1\n" + oneTab, where{"", ""}, "line 2, column 3"},
		{"unclosed table header", "[settings\ngap = 1\n", where{"", ""}, "line 1"},
		{"key defined twice", "[[tab]]\nname = \"Code\"\nname = \"Again\"\n", where{"Code", "name"}, "line 3, column 1"},
		{"table defined twice", "[settings]\ngap = 1\n[settings]\n" + oneTab, where{"", "settings"}, "line 3"},
		{"number too large", "[settings]\ngap = 99999999999999999999\n" + oneTab, where{"", "settings.gap"}, "line 2, column 7"},
		{"size too large", oneTab + "sizes = [50, 1e400]\n", where{"Code", "sizes[2]"}, "line 5"},
		{"after the tabs", oneTab + "[[rule]]\napp = \"x\"\nfloat = maybe\n", where{"", "rule[1].float"}, "line 7, column 9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, problems, err := Parse([]byte(tc.doc))
			if err != nil || cfg != nil {
				t.Fatalf("Parse = %v, %v; want problems only", cfg, err)
			}
			checkWheres(t, problems, []where{tc.want})
			if len(problems) == 1 && !strings.Contains(problems[0].Message, tc.place) {
				t.Errorf("message %q doesn't give %q", problems[0].Message, tc.place)
			}
		})
	}
}

func TestParseWrongTypes(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want where
		msg  string
	}{
		{"text for a number", "[settings]\ngap = \"8\"\n" + oneTab, where{"", "settings.gap"}, "must be a number, not text (line 2, column 7)"},
		{"text for true or false", "[settings]\nopen_missing_apps = \"yes\"\n" + oneTab, where{"", "settings.open_missing_apps"}, "must be true or false, not text"},
		{"number for text", "[settings]\nnew_windows = 3\n" + oneTab, where{"", "settings.new_windows"}, "must be text in quotes, not a number"},
		{"duration that doesn't parse", "[settings]\nlaunch_timeout = \"soon\"\n" + oneTab, where{"", "settings.launch_timeout"}, `"soon" isn't a duration such as "15s" (line 2, column 18)`},
		{"duration without quotes", "[settings]\nlaunch_timeout = 15\n" + oneTab, where{"", "settings.launch_timeout"}, `"15" isn't a duration such as "15s" (line 2, column 1)`},
		{"settings that isn't a table", "settings = 3\n" + oneTab, where{"", "settings"}, "must be the [settings] table, not a number"},
		{"tab name", "[[tab]]\nname = 3\nlayout = \"single\"\n", where{"tab 1", "name"}, "must be text in quotes, not a number (line 2, column 8)"},
		{"size", "[[tab]]\nname = \"Code\"\nlayout = \"side\"\nsizes = [50, \"x\"]\n", where{"Code", "sizes[2]"}, "must be a number, not text"},
		{"sizes", "[[tab]]\nname = \"Code\"\nlayout = \"side\"\nsizes = 50\n", where{"Code", "sizes"}, "must be a list of numbers"},
		{"items", oneTab + "  [[tab.pane]]\n  items = 5\n", where{"Code", "pane[1].items"}, "must be a list of items"},
		{"item", oneTab + "  [[tab.pane]]\n  items = [\"dev.zed.Zed\"]\n", where{"Code", "pane[1].items[1]"}, "must be an item, such as"},
		{"item field", oneTab + "  [[tab.pane]]\n  items = [{ app = \"x\" }, { app = 5 }]\n", where{"Code", "pane[1].items[2].app"}, "must be text in quotes, not a number"},
		{"rule float", oneTab + "[[rule]]\napp = \"x\"\nfloat = \"yes\"\n", where{"", "rule[1].float"}, "must be true or false, not text"},
		{"duration as a date", "[settings]\nlaunch_timeout = 2026-10-17\n" + oneTab, where{"", "settings.launch_timeout"}, `must be a duration in quotes, such as "15s", not a date (line 2, column 18)`},
		{"list in sizes", oneTab + "sizes = [[50, 50]]\n", where{"Code", "sizes[1]"}, "must be a number, not a list (line 5, column 11)"},
		{"empty list in sizes", oneTab + "sizes = [50, []]\n", where{"Code", "sizes[2]"}, "must be a number, not a list (line 5, column 1)"},
		{"list in the second tab's sizes", oneTab + "sizes = [50]\n[[tab]]\nname = \"B\"\nlayout = \"side\"\nsizes = [1, [2]]\n", where{"B", "sizes[2]"}, "(line 9, column 14)"},
		{"list in items", oneTab + "  [[tab.pane]]\n  items = [[{ app = \"x\" }]]\n", where{"Code", "pane[1].items[1]"}, "must be an item, such as { app = \"dev.zed.Zed\" }, not a list (line 6, column 13)"},
		{"list in tab", "tab = [[{ name = \"A\" }]]\n", where{"", "tab[1]"}, "must be a [[tab]] table, not a list (line 1, column 9)"},
		{"list after an unknown field", "bogus = 1\n" + oneTab + "sizes = [[1]]\n", where{"Code", "sizes[1]"}, "(line 6, column 11)"},
		{"launcher start_again", "[[launcher]]\nname = \"Claude\"\nrun = \"claude\"\nstart_again = \"yes\"\n" + oneTab, where{"", "launcher[1].start_again"}, "must be true or false, not text (line 4, column 15)"},
		{"launcher name", "[[launcher]]\nname = 3\n" + oneTab, where{"", "launcher[1].name"}, "must be text in quotes, not a number"},
		{"item start_again", oneTab + "  [[tab.pane]]\n  items = [{ terminal = \"~\", run = \"x\", start_again = 1 }]\n", where{"Code", "pane[1].items[1].start_again"}, "must be true or false, not a number"},
		{"launcher that isn't a table", "launcher = 3\n" + oneTab, where{"", "launcher"}, "must be"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, problems, err := Parse([]byte(tc.doc))
			if err != nil || cfg != nil {
				t.Fatalf("Parse = %v, %v; want problems only", cfg, err)
			}
			checkWheres(t, problems, []where{tc.want})
			if len(problems) == 1 && !strings.Contains(problems[0].Message, tc.msg) {
				t.Errorf("message %q doesn't contain %q", problems[0].Message, tc.msg)
			}
		})
	}
}

func TestParseEmptyItemKinds(t *testing.T) {
	// The Item struct can't tell an empty field from a missing one, but the
	// file can.
	tests := []struct {
		item  string
		field string
		msg   string
	}{
		{`{ app = "" }`, "pane[1].items[1].app", "needs a bundle ID"},
		{`{ web = "", name = "Slack" }`, "pane[1].items[1].web", "needs an https address"},
		{`{ vscode = "" }`, "pane[1].items[1].vscode", "needs a folder"},
		{`{ terminal = "" }`, "pane[1].items[1].terminal", "needs a starting folder"},
		{`{ app = "dev.zed.Zed", web = "" }`, "pane[1].items[1]", "has app and web"},
		{`{ name = "Nothing" }`, "pane[1].items[1]", "needs one of"},
	}
	for _, tc := range tests {
		t.Run(tc.item, func(t *testing.T) {
			_, problems, _ := Parse([]byte(oneTab + "  [[tab.pane]]\n  items = [" + tc.item + "]\n"))
			checkWheres(t, problems, []where{{"Code", tc.field}})
			if len(problems) == 1 && !strings.Contains(problems[0].Message, tc.msg) {
				t.Errorf("message %q doesn't contain %q", problems[0].Message, tc.msg)
			}
		})
	}
}

func TestParseItemKindsIgnoreCase(t *testing.T) {
	// go-toml reads Web into the web field, so the one-kind check must
	// count it too, or the config would pass here and fail every save.
	tests := []struct {
		name, doc string
	}{
		{"a capitalised kind", oneTab + "  [[tab.pane]]\n  items = [{ Web = \"https://app.slack.com\", app = \"com.tinyspeck.slackmacgap\" }]\n"},
		{"an empty capitalised kind", oneTab + "  [[tab.pane]]\n  items = [{ Web = \"\", app = \"com.tinyspeck.slackmacgap\" }]\n"},
		{"capitalised tables", "[[Tab]]\nname = \"Code\"\nlayout = \"single\"\n  [[Tab.Pane]]\n  Items = [{ WEB = \"\", app = \"x\" }]\n"},
		// go-toml lowers keys with strings.ToLower, which turns İ into i,
		// though strings.EqualFold doesn't match them.
		{"a key that lowers to items", oneTab + "  [[tab.pane]]\n  \"İtems\" = [{ Web = \"\", app = \"x\" }]\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, problems, err := Parse([]byte(tc.doc))
			if err != nil {
				t.Fatal(err)
			}
			checkWheres(t, problems, []where{{"Code", "pane[1].items[1]"}})
			if len(problems) == 1 && !strings.Contains(problems[0].Message, "has app and web") {
				t.Errorf("message %q doesn't contain %q", problems[0].Message, "has app and web")
			}
		})
	}
	// A capitalised kind on its own is still that kind.
	cfg := mustParse(t, oneTab+"  [[tab.pane]]\n  items = [{ Terminal = \"~\" }]\n")
	if got := cfg.Tabs[0].Panes[0].Items[0].Terminal; got != "~" {
		t.Errorf("Terminal = %q, want ~", got)
	}
}

func TestParseValidationProblems(t *testing.T) {
	_, problems, err := Parse([]byte(`
[settings]
new_windows = "tab"

[[tab]]
name = "Code"
layout = "columns"

  [[tab.pane]]
  items = [{ web = "http://example.com" }]

[[tab]]
name = "code"
layout = "single"

[[rule]]
app = "com.brave.Browser"
tab = "Reading"
`))
	if err != nil {
		t.Fatal(err)
	}
	checkWheres(t, problems, []where{
		{"", "settings.new_windows"},
		{"Code", "layout"},
		{"Code", "pane[1].items[1].web"},
		{"code", "name"},
		{"", "rule[1].tab"},
	})
}

func TestParseCRLF(t *testing.T) {
	doc := strings.ReplaceAll(string(ExampleTOML()), "\n", "\r\n")
	if got := mustParse(t, doc); !reflect.DeepEqual(got, Example()) {
		t.Errorf("the default file with CRLF line endings parses to %+v", got)
	}
}

func TestParseByteOrderMark(t *testing.T) {
	doc := "\xef\xbb\xbf" + string(ExampleTOML())
	if got := mustParse(t, doc); !reflect.DeepEqual(got, Example()) {
		t.Errorf("the default file with a byte order mark parses to %+v", got)
	}
}

func TestPlainMessage(t *testing.T) {
	tests := []struct{ in, want string }{
		{"toml: cannot decode TOML string into struct field config.Settings.Gap of type float64", "must be a number, not text"},
		{"toml: cannot decode TOML integer into config.Tab", "must be a [[tab]] table, not a number"},
		{"toml: cannot decode TOML string into struct field x.Y of type complex128", "cannot decode TOML string into struct field x.Y of type complex128"},
		{"toml: expected '=' after key", "expected '=' after key"},
	}
	for _, tc := range tests {
		if got := plainMessage(errorString(tc.in)); got != tc.want {
			t.Errorf("plainMessage(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestSuggest(t *testing.T) {
	tests := []struct{ kind, name, want string }{
		{kindSettings, "gapp", "gap"},
		{kindSettings, "Gap", "gap"},
		{kindItem, "titel", "title"},
		{kindItem, "colour", ""},
		{kindTab, "smallscreens", "small_screens"},
		{kindTop, "settigns", "settings"},
		{kindNone, "gapp", ""},
		{kindRule, "x", ""},
	}
	for _, tc := range tests {
		if got := suggest(tc.kind, tc.name); got != tc.want {
			t.Errorf("suggest(%q, %q) = %q, want %q", tc.kind, tc.name, got, tc.want)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add(DefaultTOML())
	f.Add(ExampleTOML())
	f.Add([]byte(oneTab + "  [[tab.pane]]\n  items = [{ app = 5 }]\n"))
	f.Add([]byte("[[tab]]\n[[tab.pane]]\n[[tab.pane.items]]\n[tab.pane.items.x]\n"))
	f.Add([]byte("bogus = 1\n" + oneTab + "sizes = [[1, []], [[2]]]\n"))
	f.Add([]byte("tab = [[{ name = \"A\" }]]\n"))
	f.Add([]byte("[settings]\ngap = \n# x\n" + oneTab))
	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, problems, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse returned an error: %v", err)
		}
		if (cfg == nil) == (len(problems) == 0) {
			t.Fatalf("Parse returned config %v with problems %v", cfg != nil, problems)
		}
		for _, p := range problems {
			if p.Message == "" {
				t.Fatalf("problem %+v has no message", p)
			}
		}
		if cfg != nil {
			again := mustParse(t, string(Format(cfg)))
			if !reflect.DeepEqual(again, cfg) {
				t.Fatalf("Parse(Format(c)) = %+v, want %+v", again, cfg)
			}
		}
	})
}
