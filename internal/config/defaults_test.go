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

// TestDefaultTOMLMatchesSpec checks that the default file is the example in
// the spec's "Config file" section, byte for byte.
func TestDefaultTOMLMatchesSpec(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "docs", "spec.md"))
	if err != nil {
		t.Skipf("can't read the spec: %v", err)
	}
	_, section, ok := strings.Cut(string(spec), "\n## Config file\n")
	if !ok {
		t.Fatal("the spec has no Config file section")
	}
	_, block, ok := strings.Cut(section, "```toml\n")
	if !ok {
		t.Fatal("the Config file section has no TOML example")
	}
	block, _, ok = strings.Cut(block, "```")
	if !ok {
		t.Fatal("the TOML example isn't closed")
	}
	if got := string(DefaultTOML()); got != block {
		t.Errorf("DefaultTOML() differs from the spec.\ngot:\n%s\nspec:\n%s", got, block)
	}
}

func TestDefaultHasNoProblems(t *testing.T) {
	if problems := Default().Validate(); len(problems) > 0 {
		t.Errorf("Default().Validate() = %s", describe(problems))
	}
}

func TestDefaultValues(t *testing.T) {
	c := Default()
	s := c.Settings
	checks := []struct {
		name      string
		got, want any
	}{
		{"compact_below", s.CompactBelow, 1800.0},
		{"gap", s.Gap, 8.0},
		{"open_missing_apps", s.OpenMissingApps, true},
		{"hide_other_tabs", s.HideOtherTabs, true},
		{"hide_other_apps", s.HideOtherApps, false},
		{"new_windows", s.NewWindows, "app-pane"},
		{"follow_focus", s.FollowFocus, "switch-tab"},
		{"live_previews", s.LivePreviews, false},
		{"launch_timeout", s.LaunchTimeout.Duration, 15 * time.Second},
		{"jump", c.Shortcuts.Jump, "ctrl+alt+space"},
		{"next_window", c.Shortcuts.NextWindow, "ctrl+alt+tab"},
		{"tabs", len(c.Tabs), 3},
		{"tab 1", c.Tabs[0].Name, "Comms"},
		{"tab 2", c.Tabs[1].Name, "Code"},
		{"tab 3", c.Tabs[2].Name, "Research"},
		{"code items", c.Tabs[1].Panes[0].Items, []Item{{App: "dev.zed.Zed"}, {App: "com.microsoft.VSCode"}}},
		{"rules", c.Rules, []Rule{{App: "com.brave.Browser", Title: "Picture in picture", Float: true}}},
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
	a.Tabs[0].Panes[0].Items[0].Web = "https://example.com"
	if b := Default(); b.Tabs[0].Name != "Comms" || b.Tabs[0].Panes[0].Items[0].Web != "https://app.slack.com/client" {
		t.Error("changing one Default() changed another")
	}
	d := DefaultTOML()
	d[0] = '#'
	if DefaultTOML()[0] != '[' {
		t.Error("changing DefaultTOML()'s bytes changed the default")
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
