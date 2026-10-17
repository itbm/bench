package config

import (
	_ "embed"
	"time"
)

// defaultTOML is the example config from the spec's "Config file" section.
//
//go:embed default.toml
var defaultTOML []byte

// DefaultTOML returns the default config as Bench writes it on first run,
// with a comment on each setting. It parses to Default(), and it's what
// Format(Default()) writes.
func DefaultTOML() []byte {
	return append([]byte(nil), defaultTOML...)
}

// Default returns the default config: the one in the spec, with three tabs
// (Comms, Code, Research) and the Brave picture-in-picture rule.
func Default() *Config {
	return &Config{
		Settings: DefaultSettings(),
		Shortcuts: Shortcuts{
			TabModifiers:        "ctrl+alt",
			NextTab:             "ctrl+alt+]",
			PreviousTab:         "ctrl+alt+[",
			Jump:                "ctrl+alt+space",
			Rearrange:           "ctrl+alt+0",
			FocusPaneModifiers:  "ctrl+alt",
			MoveWindowModifiers: "ctrl+alt+shift",
			NextWindow:          "ctrl+alt+tab",
		},
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

// DefaultSettings returns the default [settings] table: the values a file
// gets for the settings it leaves out.
func DefaultSettings() Settings {
	return Settings{
		CompactBelow:    1800,
		Gap:             8,
		OpenMissingApps: true,
		HideOtherTabs:   true,
		HideOtherApps:   false,
		NewWindows:      NewWindowsAppPane,
		FollowFocus:     FollowSwitchTab,
		LivePreviews:    false,
		LaunchTimeout:   Duration{15 * time.Second},
	}
}
