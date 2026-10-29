// Package config loads, validates, saves, and defaults config.toml, and reads
// and writes state.json. Both live in Bench's support folder,
// ~/Library/Application Support/Bench/ (see Dir).
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/shortcuts"
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

// Values for Settings.MenuBar.
const (
	MenuBarName   = "name"   // Bench's icon and the current tab's name.
	MenuBarIcon   = "icon"   // Bench's icon alone.
	MenuBarHidden = "hidden" // No menu bar item.
)

// Settings is the [settings] table.
type Settings struct {
	// Open Bench's window filling its screen.
	FillScreen bool `toml:"fill_screen" json:"fillScreen"`
	// name, icon, or hidden: what Bench's menu bar item shows.
	MenuBar string `toml:"menu_bar" json:"menuBar"`
	// A tab shortcut brings back a hidden Bench, on that tab.
	ShowOnTabShortcut bool `toml:"show_on_tab_shortcut" json:"showOnTabShortcut"`
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

// Shortcuts is the [shortcuts] table. Its type lives in the shortcuts
// package, which parses it.
type Shortcuts = shortcuts.Config

// Tab is a [[tab]] entry.
type Tab struct {
	Name string `toml:"name" json:"name"`
	// A preset: single, side, main-side, three, top-bottom, or grid.
	Layout string `toml:"layout" json:"layout"`
	// Optional percentages for the preset's top-level split.
	Sizes []float64 `toml:"sizes,omitempty" json:"sizes,omitempty"`
	// stack, tabs, or same. Empty means stack; Format leaves it out.
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
		return &durationError{text: string(b)}
	}
	d.Duration = v
	return nil
}

// durationError is text that isn't a duration. It has its own type so that
// Parse can recognise it when the TOML decoder passes it on without a
// position.
type durationError struct{ text string }

func (e *durationError) Error() string {
	return fmt.Sprintf("%q isn't a duration such as \"15s\"", e.text)
}

// Problem is something wrong in config.toml, located by tab and field.
type Problem struct {
	// Tab is the tab's name, or "" for problems outside [[tab]]. For a tab
	// with no name, it's "tab 2" (counting from 1).
	Tab string `json:"tab,omitempty"`
	// Field is a dotted path such as "layout", "pane[2].items[1].web",
	// "settings.new_windows", or "rule[1].tab", counting from 1. It's "" for
	// a syntax error that can't be placed at a field, such as one in a table
	// header.
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error formats the problem as `tab "Code": pane[1].items[2].web: message`,
// leaving out the tab or the field when there isn't one (as for a syntax
// error, whose message gives the line and column).
func (p Problem) Error() string {
	var b strings.Builder
	if p.Tab != "" {
		fmt.Fprintf(&b, "tab %q: ", p.Tab)
	}
	if p.Field != "" {
		b.WriteString(p.Field)
		b.WriteString(": ")
	}
	b.WriteString(p.Message)
	return b.String()
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
	// Tabs are the tabs when it was saved, which the config's tabs continue
	// by name (see MatchTabs).
	Tabs []TabState `json:"tabs"`
	// Items by ID.
	Items map[string]ItemState `json:"items"`
	// Floating holds items that were popped out of Bench.
	Floating []string `json:"floating"`
	// Apps holds what Bench remembers about the apps that have items, by
	// lower-case bundle ID. Older files don't have it, and older builds
	// ignore it.
	Apps map[string]AppState `json:"apps,omitempty"`
	// Ran is when Bench was last running, as of this save. Bench saves at
	// least once an hour while it runs, so the time since is about how long
	// it was off, which doesn't count towards an app's time unseen.
	Ran time.Time `json:"ran,omitzero"`
	// Screen is the display Bench's window was on, which it opens on next
	// time if that display is still there. Older files don't have it, and
	// older builds ignore it.
	Screen *ScreenState `json:"screen,omitempty"`
}

// ScreenState names a display, so that Bench can find it again: by its ID,
// or else by its name, or else by where it is.
type ScreenState struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Frame is the whole display, in global coordinates.
	Frame layout.Rect `json:"frame"`
}

// AppState is what Bench remembers about an app that has items.
type AppState struct {
	// Quit is set when the user quit the app while Bench was running, other
	// than by logging out or restarting: Bench leaves it closed until it
	// next opens.
	Quit bool `json:"quit,omitempty"`
	// Seen is when Bench last saw the app running, so that the places of an
	// app that hasn't run for a long time can be forgotten.
	Seen time.Time `json:"seen,omitzero"`
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
	// Entries identify the tab's items in the config when this was saved,
	// so that entries added to config.toml by hand since can be added to
	// the tab. Older files don't have them.
	Entries []string `json:"entries"`
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
	URL string `json:"url,omitempty"`
	// Home is the address a Web item was given (in the config, or when it
	// was added), which it loads and the config keeps. Older files don't
	// have it.
	Home    string `json:"home,omitempty"`
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
	// OwnWindow is set for a popped-out Web or built-in item that had a
	// window of its own.
	OwnWindow bool `json:"ownWindow,omitempty"`
}
