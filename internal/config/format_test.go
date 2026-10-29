package config

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

// roundTripConfigs are valid configs that must survive Format and Parse.
func roundTripConfigs() map[string]*Config {
	configs := map[string]*Config{"default": Default(), "example": Example()}

	c := Example()
	c.Settings = Settings{
		FillScreen:               false,
		MenuBar:                  MenuBarHidden,
		ShowOnTabShortcut:        false,
		ConfirmQuitBusyTerminals: false,
		ConfirmClose:             false,
		CompactBelow:             1440.5,
		Gap:                      0,
		ShowStatusBar:            false,
		TabShortcutHints:         TabHintsNever,
		ScrollBars:               ScrollBarsAlways,
		Density:                  DensityCompact,
		PaneHeaders:              PaneHeadersAuto,
		OpenMissingApps:          false,
		HideOtherTabs:            false,
		HideOtherApps:            true,
		NewWindows:               NewWindowsFloat,
		FollowFocus:              FollowOff,
		LivePreviews:             true,
		LaunchTimeout:            Duration{90*time.Second + 500*time.Millisecond},
	}
	c.Shortcuts = Shortcuts{
		TabModifiers:        "cmd+alt",
		NextTab:             "ctrl+alt+shift+cmd+right",
		PreviousTab:         "ctrl+alt+left",
		Jump:                "ctrl+alt+j",
		Rearrange:           "ctrl+alt+r",
		FocusPaneModifiers:  "ctrl+shift",
		MoveWindowModifiers: "ctrl+alt+shift+cmd",
		NextWindow:          "alt+tab",
	}
	configs["other settings"] = c

	configs["every preset and item kind"] = &Config{
		Settings:  DefaultSettings(),
		Shortcuts: Example().Shortcuts,
		Tabs: []Tab{
			{Name: "One", Layout: "single", SmallScreens: "same", Panes: []Pane{
				{Items: []Item{{App: "com.apple.Terminal"}}},
			}},
			{Name: "Grid", Layout: "grid", Sizes: []float64{30, 70}, Panes: []Pane{
				{Items: []Item{{App: "com.brave.Browser", Title: "Docs", URL: "https://docs.example.com/a?b=c&d=e"}}},
				{},
				{Items: []Item{{VSCode: "~/code/bench", Name: "Bench"}}},
				{Items: []Item{{Terminal: "~/code", Command: "htop -d 10", Name: "Top"}}},
			}},
			{Name: "Thirds", Layout: "three", Sizes: []float64{33.333333333333336, 33.33, 0.0001}, SmallScreens: "tabs", Panes: []Pane{
				{Items: []Item{{Web: "https://example.com/path#frag", Profile: "work"}}},
			}},
			{Name: "Stacked", Layout: "top-bottom", SmallScreens: "stack"},
			{Name: "Wide", Layout: "main-side", Sizes: []float64{1e20, 2.5e-7}},
		},
		Rules: []Rule{
			{App: "com.brave.Browser", Title: "Picture in picture", Float: true},
			{App: "com.brave.Browser", Tab: "Grid"},
			{App: "md.obsidian", Tab: "thirds"},
		},
	}

	configs["awkward text"] = &Config{
		Settings:  DefaultSettings(),
		Shortcuts: Example().Shortcuts,
		Tabs: []Tab{
			{Name: `Quotes "and" \backslashes\`, Layout: "side", Panes: []Pane{
				{Items: []Item{{Terminal: "~/My Folder/ünïcödé ☕", Command: "printf 'a\tb\n'\r", Name: "Tab\there"}}},
				{Items: []Item{{App: "com.example.App", Title: "# not a comment", URL: "x-app://open?q=\"1\""}}},
			}},
			{Name: "Control \x01\x1f\x7f characters", Layout: "single"},
			{Name: "日本語", Layout: "single"},
		},
	}

	var many []Item
	for _, id := range []string{"dev.zed.Zed", "com.microsoft.VSCode", "com.apple.Terminal", "com.brave.Browser", "md.obsidian", "com.apple.finder"} {
		many = append(many, Item{App: id})
	}
	configs["long items line"] = &Config{
		Settings:  DefaultSettings(),
		Shortcuts: Example().Shortcuts,
		Tabs:      []Tab{{Name: "Busy", Layout: "single", Panes: []Pane{{Items: many}}}},
	}
	return configs
}

func TestFormatRoundTrip(t *testing.T) {
	for name, c := range roundTripConfigs() {
		t.Run(name, func(t *testing.T) {
			if problems := c.Validate(); len(problems) > 0 {
				t.Fatalf("the test config has problems:\n%s", describe(problems))
			}
			text := Format(c)
			got := mustParse(t, string(text))
			if !reflect.DeepEqual(got, c) {
				t.Errorf("Parse(Format(c)) =\n%+v\nwant\n%+v\nfrom:\n%s", got, c, text)
			}
			if again := Format(got); !bytes.Equal(again, text) {
				t.Errorf("formatting again gives\n%s\nwant\n%s", again, text)
			}
		})
	}
}

func TestFormatEmptyListsComeBackNil(t *testing.T) {
	c := &Config{
		Settings:  DefaultSettings(),
		Shortcuts: Example().Shortcuts,
		Tabs:      []Tab{{Name: "Code", Layout: "side", Sizes: []float64{}, Panes: []Pane{{Items: []Item{}}}}},
		Rules:     []Rule{},
	}
	got := mustParse(t, string(Format(c)))
	want := &Config{
		Settings:  c.Settings,
		Shortcuts: c.Shortcuts,
		Tabs:      []Tab{{Name: "Code", Layout: "side", Panes: []Pane{{}}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFormatLayout(t *testing.T) {
	c := roundTripConfigs()["every preset and item kind"]
	c.Tabs = c.Tabs[1:2]
	c.Rules = c.Rules[:2]
	c.Tabs[0].SmallScreens = ""
	got := string(Format(c))
	_, got, _ = strings.Cut(got, "[[tab]]") // The settings are checked against the default file.
	want := `
name = "Grid"
layout = "grid"
sizes = [30, 70]

  [[tab.pane]]
  items = [{ app = "com.brave.Browser", title = "Docs", url = "https://docs.example.com/a?b=c&d=e" }]

  [[tab.pane]]
  items = []

  [[tab.pane]]
  items = [{ vscode = "~/code/bench", name = "Bench" }]

  [[tab.pane]]
  items = [{ terminal = "~/code", command = "htop -d 10", name = "Top" }]

[[rule]]
app = "com.brave.Browser"
title = "Picture in picture"
float = true

[[rule]]
app = "com.brave.Browser"
tab = "Grid"
`
	if got != want {
		t.Errorf("Format wrote:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatLongItemsLine(t *testing.T) {
	got := string(Format(roundTripConfigs()["long items line"]))
	want := `  [[tab.pane]]
  items = [
    { app = "dev.zed.Zed" },
    { app = "com.microsoft.VSCode" },
    { app = "com.apple.Terminal" },
    { app = "com.brave.Browser" },
    { app = "md.obsidian" },
    { app = "com.apple.finder" },
  ]
`
	if !strings.HasSuffix(got, want) {
		t.Errorf("Format wrote:\n%s\nwant it to end with:\n%s", got, want)
	}
}

func TestFormatCommentsStayAligned(t *testing.T) {
	c := Example()
	c.Settings.NewWindows = "focused-pane"
	c.Shortcuts.TabModifiers = "ctrl+alt+shift+cmd+fn"
	got := string(Format(c))
	for _, line := range []string{
		`new_windows = "focused-pane"  # app-pane, focused-pane, or float`,
		`tab_modifiers = "ctrl+alt+shift+cmd+fn"  # Plus 1 to 9.`,
	} {
		if !strings.Contains(got, line+"\n") {
			t.Errorf("Format didn't write %q:\n%s", line, got)
		}
	}
}

func TestFormatNil(t *testing.T) {
	if got := Format(nil); got != nil {
		t.Errorf("Format(nil) = %q", got)
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{1800, "1800"},
		{0, "0"},
		{-8, "-8"},
		{45.5, "45.5"},
		{33.333333333333336, "33.333333333333336"},
		{1e15, "1e+15"},
		{1e-7, "1e-07"},
		{math.Inf(1), "inf"},
		{math.Inf(-1), "-inf"},
		{math.NaN(), "nan"},
	}
	for _, tc := range tests {
		if got := formatFloat(tc.in); got != tc.want {
			t.Errorf("formatFloat(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"plain", `"plain"`},
		{`a "b" \c`, `"a \"b\" \\c"`},
		{"tab\tnew\nline\rret\bbs\fff", `"tab\tnew\nline\rret\bbs\fff"`},
		{"\x00\x1f\x7f", `"\u0000\u001F\u007F"`},
		{"café ☕", `"café ☕"`},
		{"bad \xff byte", "\"bad \uFFFD byte\""},
	}
	for _, tc := range tests {
		if got := quote(tc.in); got != tc.want {
			t.Errorf("quote(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
