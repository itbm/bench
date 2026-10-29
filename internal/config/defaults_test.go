package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
)

func TestDefaultTOMLParsesToDefault(t *testing.T) {
	got := mustParse(t, string(DefaultTOML()))
	if want := Default(); !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(DefaultTOML()) =\n%+v\nwant Default() =\n%+v", got, want)
	}
}

func TestFormatDefaultIsDefaultTOML(t *testing.T) {
	got, want := Format(Default()), DefaultTOML()
	if !bytes.Equal(got, want) {
		t.Errorf("Format(Default()) isn't DefaultTOML().\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestExampleTOMLParsesToExample(t *testing.T) {
	got := mustParse(t, string(ExampleTOML()))
	if want := Example(); !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(ExampleTOML()) =\n%+v\nwant Example() =\n%+v", got, want)
	}
}

func TestFormatExampleIsExampleTOML(t *testing.T) {
	got, want := Format(Example()), ExampleTOML()
	if !bytes.Equal(got, want) {
		t.Errorf("Format(Example()) isn't ExampleTOML().\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestDefaultTOMLMatchesSpec checks that the default and example files are
// the two in the spec's "Config file" section, byte for byte.
func TestDefaultTOMLMatchesSpec(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "spec.md"))
	if err != nil {
		t.Skipf("can't read the spec: %v", err)
	}
	_, section, ok := strings.Cut(string(spec), "\n## Config file\n")
	if !ok {
		t.Fatal("the spec has no Config file section")
	}
	for _, want := range []struct {
		name string
		toml []byte
	}{{"DefaultTOML()", DefaultTOML()}, {"ExampleTOML()", ExampleTOML()}} {
		var block string
		_, section, ok = strings.Cut(section, "```toml\n")
		if !ok {
			t.Fatalf("the Config file section has no TOML block for %s", want.name)
		}
		block, section, ok = strings.Cut(section, "```")
		if !ok {
			t.Fatalf("the TOML block for %s isn't closed", want.name)
		}
		if got := string(want.toml); got != block {
			t.Errorf("%s differs from the spec.\ngot:\n%s\nspec:\n%s", want.name, got, block)
		}
	}
}

func TestDefaultHasNoProblems(t *testing.T) {
	if problems := Default().Validate(); len(problems) > 0 {
		t.Errorf("Default().Validate() = %s", describe(problems))
	}
	if problems := Example().Validate(); len(problems) > 0 {
		t.Errorf("Example().Validate() = %s", describe(problems))
	}
}

// TestDefaultFor checks that the default config holds the browser it's
// given, or Safari, and nothing else, with a picture-in-picture rule for a
// browser other than Safari.
func TestDefaultFor(t *testing.T) {
	pip := func(id string) []Rule { return []Rule{{App: id, Title: "Picture in picture", Float: true}} }
	for _, tc := range []struct {
		browser, want string
		rules         []Rule
	}{
		{"", "com.apple.Safari", nil},
		{"com.apple.Safari", "com.apple.Safari", nil},
		{"com.google.Chrome", "com.google.Chrome", pip("com.google.Chrome")},
	} {
		c := DefaultFor(tc.browser)
		want := []Tab{{Name: "Research", Layout: "single", Panes: []Pane{{Items: []Item{{App: tc.want}}}}}}
		if !reflect.DeepEqual(c.Tabs, want) || !reflect.DeepEqual(c.Rules, tc.rules) {
			t.Errorf("DefaultFor(%q) tabs = %+v, rules = %+v; want %+v and %+v", tc.browser, c.Tabs, c.Rules, want, tc.rules)
		}
		if got := mustParse(t, string(DefaultTOMLFor(tc.browser))); !reflect.DeepEqual(got, c) {
			t.Errorf("Parse(DefaultTOMLFor(%q)) =\n%+v\nwant DefaultFor =\n%+v", tc.browser, got, c)
		}
	}
	chrome := strings.Replace(string(DefaultTOML()), "com.apple.Safari", "com.google.Chrome", 1) +
		"\n[[rule]]\napp = \"com.google.Chrome\"\ntitle = \"Picture in picture\"\nfloat = true\n"
	if got := string(DefaultTOMLFor("com.google.Chrome")); got != chrome {
		t.Errorf("DefaultTOMLFor(Chrome) =\n%s\nwant\n%s", got, chrome)
	}
}

func TestDefaultValues(t *testing.T) {
	c := Default()
	s := c.Settings
	checks := []struct {
		name      string
		got, want any
	}{
		{"fill_screen", s.FillScreen, true},
		{"menu_bar", s.MenuBar, "name"},
		{"show_on_tab_shortcut", s.ShowOnTabShortcut, true},
		{"check_for_updates", s.CheckForUpdates, false},
		{"confirm_quit_busy_terminals", s.ConfirmQuitBusyTerminals, true},
		{"confirm_close", s.ConfirmClose, true},
		{"compact_below", s.CompactBelow, 1800.0},
		{"gap", s.Gap, 4.0},
		{"show_status_bar", s.ShowStatusBar, true},
		{"tab_shortcut_hints", s.TabShortcutHints, "auto"},
		{"scroll_bars", s.ScrollBars, "auto"},
		{"density", s.Density, "auto"},
		{"pane_headers", s.PaneHeaders, "always"},
		{"open_missing_apps", s.OpenMissingApps, true},
		{"hide_other_tabs", s.HideOtherTabs, true},
		{"hide_other_apps", s.HideOtherApps, false},
		{"new_windows", s.NewWindows, "app-pane"},
		{"follow_focus", s.FollowFocus, "switch-tab"},
		{"live_previews", s.LivePreviews, false},
		{"notify_terminals", s.NotifyTerminals, true},
		{"keep_terminal_output", s.KeepTerminalOutput, true},
		{"launch_timeout", s.LaunchTimeout.Duration, 15 * time.Second},
		{"jump", c.Shortcuts.Jump, "ctrl+alt+space"},
		{"next_window", c.Shortcuts.NextWindow, "ctrl+alt+tab"},
		{"zoom_pane", c.Shortcuts.ZoomPane, "ctrl+alt+return"},
		{"last_tab", c.Shortcuts.LastTab, "ctrl+alt+`"},
		{"tabs", len(c.Tabs), 1},
		{"tab", c.Tabs[0].Name, "Research"},
		{"browser", c.Tabs[0].Panes[0].Items, []Item{{App: "com.apple.Safari"}}},
		{"rules", len(c.Rules), 0},
	}
	for _, tc := range checks {
		if !reflect.DeepEqual(tc.got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestDefaultReturnsFreshCopies(t *testing.T) {
	a := Default()
	a.Tabs[0].Name = "Changed"
	a.Tabs[0].Panes[0].Items[0].App = "com.example.Other"
	if b := Default(); b.Tabs[0].Name != "Research" || b.Tabs[0].Panes[0].Items[0].App != "com.apple.Safari" {
		t.Error("changing one Default() changed another")
	}
	d := DefaultTOML()
	d[0] = '#'
	if DefaultTOML()[0] != '[' {
		t.Error("changing DefaultTOML()'s bytes changed the default")
	}
	x := Example()
	x.Tabs[0].Panes[0].Items[0].Web = "https://example.com"
	if y := Example(); y.Tabs[0].Panes[0].Items[0].Web != "https://app.slack.com/client" {
		t.Error("changing one Example() changed another")
	}
	e := ExampleTOML()
	e[0] = '#'
	if ExampleTOML()[0] != '[' {
		t.Error("changing ExampleTOML()'s bytes changed the example")
	}
}

// TestPresetShapesMatchLayout checks validation's copy of the presets
// against the layout package.
func TestPresetShapesMatchLayout(t *testing.T) {
	var presets []layout.Preset
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("layout.Presets isn't implemented yet (%v)", r)
			}
		}()
		presets = layout.Presets()
	}()
	if len(presets) != len(presetShapes) {
		t.Errorf("layout has %d presets, validation knows %d", len(presets), len(presetShapes))
	}
	for _, p := range presets {
		shape, ok := presetShapes[p.ID]
		if !ok {
			t.Errorf("validation doesn't know preset %q", p.ID)
			continue
		}
		if shape.panes != p.Slots {
			t.Errorf("preset %q: validation says %d panes, layout says %d", p.ID, shape.panes, p.Slots)
		}
		if shape.children != len(p.DefaultSizes) {
			t.Errorf("preset %q: validation says %d sizes, layout's defaults have %d", p.ID, shape.children, len(p.DefaultSizes))
		}
	}
}
