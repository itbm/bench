// Package config loads, validates, saves, and defaults config.toml, and reads
// and writes state.json. Both live in Bench's support folder,
// ~/Library/Application Support/Bench/ (see Dir).
package config

import (
	"fmt"
	"time"

	"github.com/itbm/bench/internal/layout"
)

// Config is config.toml.
type Config struct {
	Settings  Settings  `toml:"settings" json:"settings"`
	Shortcuts Shortcuts `toml:"shortcuts" json:"shortcuts"`
	Tabs      []Tab     `toml:"tab" json:"tabs"`
	Rules     []Rule    `toml:"rule" json:"rules"`
}

// Values for Settings.NewWindows.
const (
	NewWindowsAppPane     = "app-pane"
	NewWindowsFocusedPane = "focused-pane"
	NewWindowsFloat       = "float"
)

// Values for Settings.FollowFocus.
const (
	FollowSwitchTab = "switch-tab"
	FollowBringHere = "bring-here"
	FollowOff       = "off"
)

// Settings is the [settings] table.
type Settings struct {
	// Screens narrower than this, in points, are small.
	CompactBelow float64 `toml:"compact_below" json:"compactBelow"`
	// Points between panes.
	Gap float64 `toml:"gap" json:"gap"`
	// Open a tab's apps if they aren't running.
	OpenMissingApps bool `toml:"open_missing_apps" json:"openMissingApps"`
	// Hide windows that belong to other tabs.
	HideOtherTabs bool `toml:"hide_other_tabs" json:"hideOtherTabs"`
	// Also hide every app Bench doesn't manage.
	HideOtherApps bool `toml:"hide_other_apps" json:"hideOtherApps"`
	// app-pane, focused-pane, or float.
	NewWindows string `toml:"new_windows" json:"newWindows"`
	// switch-tab, bring-here, or off.
	FollowFocus string `toml:"follow_focus" json:"followFocus"`
	// Needs Screen Recording permission.
	LivePreviews bool `toml:"live_previews" json:"livePreviews"`
	// How long to wait for a tab's apps to open.
	LaunchTimeout Duration `toml:"launch_timeout" json:"launchTimeout"`
}

// Shortcuts is the [shortcuts] table. Each is a shortcut such as
// "ctrl+alt+space", or modifiers such as "ctrl+alt" that combine with keys
// (1 to 9, or the arrow keys).
type Shortcuts struct {
	TabModifiers        string `toml:"tab_modifiers" json:"tabModifiers"`
	NextTab             string `toml:"next_tab" json:"nextTab"`
	PreviousTab         string `toml:"previous_tab" json:"previousTab"`
	Jump                string `toml:"jump" json:"jump"`
	Rearrange           string `toml:"rearrange" json:"rearrange"`
	FocusPaneModifiers  string `toml:"focus_pane_modifiers" json:"focusPaneModifiers"`
	MoveWindowModifiers string `toml:"move_window_modifiers" json:"moveWindowModifiers"`
	NextWindow          string `toml:"next_window" json:"nextWindow"`
}

// Tab is a [[tab]] entry.
type Tab struct {
	Name string `toml:"name" json:"name"`
	// A preset: single, side, main-side, three, top-bottom, or grid.
	Layout string `toml:"layout" json:"layout"`
	// Optional percentages for the preset's top-level split.
	Sizes []float64 `toml:"sizes,omitempty" json:"sizes,omitempty"`
	// stack, tabs, or same. Empty means stack.
	SmallScreens string `toml:"small_screens,omitempty" json:"smallScreens,omitempty"`
	// One per pane in the preset, in reading order.
	Panes []Pane `toml:"pane" json:"panes"`
}

// Pane is a [[tab.pane]] entry.
type Pane struct {
	Items []Item `toml:"items" json:"items"`
}

// Item is one entry in a pane's items. It needs exactly one of App, Web,
// VSCode, or Terminal.
type Item struct {
	// App item: a bundle ID. Every standard window of the app in this tab
	// starts in this pane, unless another pane claims it with Title.
	App string `toml:"app,omitempty" json:"app,omitempty"`
	// App item: only windows whose title contains this, ignoring case.
	Title string `toml:"title,omitempty" json:"title,omitempty"`
	// App item with Title: a link to open in the app when no matching window
	// exists.
	URL string `toml:"url,omitempty" json:"url,omitempty"`
	// Web item: the address to load (https).
	Web string `toml:"web,omitempty" json:"web,omitempty"`
	// Web item: data store name; items with the same profile share logins.
	Profile string `toml:"profile,omitempty" json:"profile,omitempty"`
	// Web item: a folder to open in VS Code for the Web.
	VSCode string `toml:"vscode,omitempty" json:"vscode,omitempty"`
	// Built-in item: a terminal's starting folder.
	Terminal string `toml:"terminal,omitempty" json:"terminal,omitempty"`
	// Built-in item: a command to run instead of the shell.
	Command string `toml:"command,omitempty" json:"command,omitempty"`
	// Web and built-in items: the sub-tab's label before the page or shell
	// sets a title.
	Name string `toml:"name,omitempty" json:"name,omitempty"`
}

// Rule is a [[rule]] entry: where new windows of an app go.
type Rule struct {
	App   string `toml:"app" json:"app"`
	Title string `toml:"title,omitempty" json:"title,omitempty"`
	Float bool   `toml:"float,omitempty" json:"float,omitempty"`
	Tab   string `toml:"tab,omitempty" json:"tab,omitempty"`
}

// Duration is a time.Duration written as text, such as "15s".
type Duration struct{ time.Duration }

// MarshalText writes the duration as Go duration text.
func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText reads Go duration text, such as "15s" or "1m30s".
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("%q isn't a duration such as \"15s\"", string(b))
	}
	d.Duration = v
	return nil
}

// Problem is something wrong in config.toml, located by tab and field.
type Problem struct {
	// Tab is the tab's name, or "" for problems outside [[tab]]. For a tab
	// with no name, it's "tab 2" (counting from 1).
	Tab string `json:"tab,omitempty"`
	// Field is a dotted path such as "layout", "pane[2].items[1].web",
	// "settings.new_windows", or "rule[1].tab".
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (p Problem) Error() string {
	if p.Tab != "" {
		return fmt.Sprintf("tab %q: %s: %s", p.Tab, p.Field, p.Message)
	}
	return fmt.Sprintf("%s: %s", p.Field, p.Message)
}

// State is state.json: the live arrangement after dragging and splitting.
// Bench writes it whenever the arrangement changes.
type State struct {
	Version int `json:"version"` // StateVersion.
	// Current is the index of the current tab.
	Current int `json:"current"`
	// Mode is auto, wide, or compact.
	Mode string `json:"mode"`
	// Seq is the last number used in a generated item, pane, or split ID.
	Seq int `json:"seq"`
	// Tabs must match the config's tabs, by name and in order.
	Tabs []TabState `json:"tabs"`
	// Items by ID.
	Items map[string]ItemState `json:"items"`
	// Floating holds items that were popped out of Bench.
	Floating []string `json:"floating"`
}

// StateVersion is the current state.json format.
const StateVersion = 1

// TabState is one tab's live tree.
type TabState struct {
	Name string `json:"name"`
	// Preset is a preset ID, or "custom" once items have been moved.
	Preset string       `json:"preset"`
	Root   *layout.Node `json:"root"`
	Focus  string       `json:"focus"`
	// CompactShown is the item shown by the "tabs" small-screen view.
	CompactShown string `json:"compactShown,omitempty"`
}

// ItemState is what Bench remembers about an item.
type ItemState struct {
	Kind     string `json:"kind"` // web, app, or builtin.
	BundleID string `json:"bundleId,omitempty"`
	WindowID uint32 `json:"windowId,omitempty"`
	Title    string `json:"title"`
	// Created is the item's creation order, for matching windows to items
	// after an app restarts.
	Created int `json:"created"`
	// Name is the label from the config, for Web and built-in items.
	Name string `json:"name,omitempty"`
	// URL is a Web item's address.
	URL     string `json:"url,omitempty"`
	Profile string `json:"profile,omitempty"`
	// VSCode is the folder for a VS Code for the Web item.
	VSCode string `json:"vscode,omitempty"`
	// Folder is a terminal's starting folder.
	Folder  string `json:"folder,omitempty"`
	Command string `json:"command,omitempty"`
	// TitleFilter is the config's title filter for an App item, if any.
	TitleFilter string `json:"titleFilter,omitempty"`
	// OpenURL is the config's url for an App item, if any.
	OpenURL string `json:"openUrl,omitempty"`
}
