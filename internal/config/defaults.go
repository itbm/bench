package config

import (
	_ "embed"
	"strings"
	"time"
)

// FallbackBrowser is the browser the default config holds when macOS names
// no default browser: Safari, which every Mac has.
const FallbackBrowser = "com.apple.Safari"

// defaultTOML is the first config from the spec's "Config file" section.
//
//go:embed default.toml
var defaultTOML []byte

// exampleTOML is the second, fuller one.
//
//go:embed example.toml
var exampleTOML []byte

// DefaultTOML returns the default config as Bench writes it on first run on
// a Mac whose default browser is Safari, with a comment on each setting. It
// parses to Default(), and it's what Format(Default()) writes.
func DefaultTOML() []byte {
	return append([]byte(nil), defaultTOML...)
}

// DefaultTOMLFor returns DefaultTOML with browser, a bundle ID, in place of
// Safari. An empty browser means Safari.
func DefaultTOMLFor(browser string) []byte {
	return Format(DefaultFor(browser))
}

// Default returns the default config: the settings and shortcuts, and one
// tab, Research, holding Safari. It has no other apps, since a Mac may not
// have them.
func Default() *Config {
	return DefaultFor("")
}

// DefaultFor returns the default config with browser, a bundle ID, in place
// of Safari, for the default browser of the Mac that Bench runs on. An empty
// browser means Safari. Another browser also gets a rule that leaves its
// picture-in-picture window floating, as Chromium browsers show it in a
// window of their own; Safari's is the system's.
func DefaultFor(browser string) *Config {
	if browser == "" {
		browser = FallbackBrowser
	}
	c := &Config{
		Settings:  DefaultSettings(),
		Shortcuts: DefaultShortcuts(),
		Tabs: []Tab{
			{
				Name:   "Research",
				Layout: "single",
				Panes:  []Pane{{Items: []Item{{App: browser}}}},
			},
		},
	}
	if !strings.EqualFold(browser, FallbackBrowser) {
		c.Rules = []Rule{{App: browser, Title: "Picture in picture", Float: true}}
	}
	return c
}

// ExampleTOML returns the example config: the second one in the spec's
// "Config file" section, which the browser preview starts with. It parses
// to Example(), and it's what Format(Example()) writes.
func ExampleTOML() []byte {
	return append([]byte(nil), exampleTOML...)
}

// Example returns the example config, with three tabs (Comms, Code,
// Research) and the Brave picture-in-picture rule.
func Example() *Config {
	return &Config{
		Settings:  DefaultSettings(),
		Shortcuts: DefaultShortcuts(),
		Tabs: []Tab{
			{
				Name:         "Comms",
				Layout:       "side",
				Sizes:        []float64{45, 55},
				SmallScreens: "tabs",
				Panes: []Pane{
					{Items: []Item{{Web: "https://app.slack.com/client", Profile: "slack", Name: "Slack"}}},
					{Items: []Item{{Web: "https://outlook.office.com/mail/", Profile: "outlook", Name: "Outlook"}}},
				},
			},
			{
				Name:         "Code",
				Layout:       "main-side",
				Sizes:        []float64{65, 35},
				SmallScreens: "stack",
				Panes: []Pane{
					{Items: []Item{{App: "dev.zed.Zed"}, {App: "com.microsoft.VSCode"}}},
					{Items: []Item{{Terminal: "~/code"}}},
				},
			},
			{
				Name:         "Research",
				Layout:       "side",
				Sizes:        []float64{60, 40},
				SmallScreens: "tabs",
				Panes: []Pane{
					{Items: []Item{{App: "com.brave.Browser"}}},
					{Items: []Item{{App: "md.obsidian"}}},
				},
			},
		},
		Rules: []Rule{
			{App: "com.brave.Browser", Title: "Picture in picture", Float: true},
		},
	}
}

// DefaultShortcuts returns the default [shortcuts] table, which Settings ▸
// Keyboard's Restore defaults puts back.
func DefaultShortcuts() Shortcuts {
	return Shortcuts{
		TabModifiers:        "ctrl+alt",
		NextTab:             "ctrl+alt+]",
		PreviousTab:         "ctrl+alt+[",
		LastTab:             "ctrl+alt+`",
		Jump:                "ctrl+alt+space",
		Rearrange:           "ctrl+alt+0",
		FocusPaneModifiers:  "ctrl+alt",
		MoveWindowModifiers: "ctrl+alt+shift",
		SendToTabModifiers:  "ctrl+alt+shift",
		NextWindow:          "ctrl+alt+tab",
		ZoomPane:            "ctrl+alt+return",
	}
}

// DefaultCompactBelow is compact_below's default: Auto chooses the compact
// layout for Bench's window when it's narrower than this many points. It's
// the only place in Go with the default. A new one needs, with it, a new
// default.toml and example.toml, the TOML blocks in docs/spec.md and
// docs/user-guide.md, the default that the spec and the user guide name
// in words, and the workspace test of the default on a MacBook
// (TestDefaultMacBookIsSideBySide); the tests fail until the files agree.
// At 1,280 pt, a window that fills a 13-inch or 14-inch MacBook's screen
// shows its panes side by side. The default reaches only new config.toml
// files: Bench writes every setting when it makes the file and when
// Settings saves it, so a file that an earlier version wrote keeps that
// version's default, 1,800, as its own value. See decision 58 in
// docs/architecture.md.
const DefaultCompactBelow = 1280

// DefaultSettings returns the default [settings] table: the values a file
// gets for the settings it leaves out.
func DefaultSettings() Settings {
	return Settings{
		FillScreen:               true,
		MenuBar:                  MenuBarName,
		ShowOnTabShortcut:        true,
		UnreadCounts:             true,
		CheckForUpdates:          false,
		ConfirmQuitBusyTerminals: true,
		ConfirmClose:             true,
		CompactBelow:             DefaultCompactBelow,
		Gap:                      4,
		ShowStatusBar:            true,
		TabShortcutHints:         TabHintsAuto,
		ScrollBars:               ScrollBarsAuto,
		Appearance:               AppearanceSystem,
		Density:                  DensityAuto,
		PaneHeaders:              PaneHeadersAlways,
		OpenMissingApps:          true,
		HideOtherTabs:            true,
		HideOtherApps:            false,
		CoveringWindows:          CoveringHide,
		NewWindows:               NewWindowsAppPane,
		FollowFocus:              FollowSwitchTab,
		DragToAdd:                true,
		LivePreviews:             false,
		TerminalFontSize:         12.5,
		TerminalOptionAsMeta:     false,
		NotifyTerminals:          true,
		KeepTerminalOutput:       true,
		LaunchTimeout:            Duration{15 * time.Second},
	}
}
