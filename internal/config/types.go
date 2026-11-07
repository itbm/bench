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
	Settings  Settings   `toml:"settings" json:"settings"`
	Shortcuts Shortcuts  `toml:"shortcuts" json:"shortcuts"`
	Launchers []Launcher `toml:"launcher" json:"launchers"`
	Tabs      []Tab      `toml:"tab" json:"tabs"`
	Rules     []Rule     `toml:"rule" json:"rules"`
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

// Values for Settings.TabShortcutHints.
const (
	TabHintsAuto   = "auto"   // Shown except in the compact size class.
	TabHintsAlways = "always" // Always shown.
	TabHintsNever  = "never"  // Never shown.
)

// Values for Settings.ScrollBars.
const (
	ScrollBarsAuto   = "auto"   // As macOS shows scroll bars: all the time, or only while scrolling.
	ScrollBarsAlways = "always" // Always shown.
	ScrollBarsNever  = "never"  // Never shown.
)

// Values for Settings.Appearance.
const (
	AppearanceSystem = "system" // Light or dark, as macOS is.
	AppearanceLight  = "light"  // Always light.
	AppearanceDark   = "dark"   // Always dark.
)

// Values for Settings.Density.
const (
	DensityAuto     = "auto"     // Compact in the compact size class, standard otherwise.
	DensityStandard = "standard" // Bench's usual sizes.
	DensityCompact  = "compact"  // A shorter title bar, pane headers, tabs, and sub-tabs.
)

// Values for Settings.PaneHeaders.
const (
	PaneHeadersAlways = "always" // Every pane has its header.
	PaneHeadersAuto   = "auto"   // A pane with one App window, and nothing to report, has none.
)

// Settings is the [settings] table.
type Settings struct {
	// Open Bench's window filling its screen.
	FillScreen bool `toml:"fill_screen" json:"fillScreen"`
	// name, icon, or hidden: what Bench's menu bar item shows.
	MenuBar string `toml:"menu_bar" json:"menuBar"`
	// A tab shortcut brings back a hidden Bench, on that tab.
	ShowOnTabShortcut bool `toml:"show_on_tab_shortcut" json:"showOnTabShortcut"`
	// Show unread counts on sub-tabs and tabs: the badges that apps show on
	// their Dock icons, and the counts that pages put at the start of their
	// titles.
	UnreadCounts bool `toml:"unread_counts" json:"unreadCounts"`
	// Look for a newer version of Bench on GitHub, at most once a day.
	CheckForUpdates bool `toml:"check_for_updates" json:"checkForUpdates"`
	// Ask before quitting Bench, or closing a terminal from its sub-tab, its
	// own window, or Close Tab…, while a terminal is running something.
	ConfirmQuitBusyTerminals bool `toml:"confirm_quit_busy_terminals" json:"confirmQuitBusyTerminals"`
	// Ask before Close tab closes two or more things, or a terminal that's
	// running something.
	ConfirmClose bool `toml:"confirm_close" json:"confirmClose"`
	// Bench's windows narrower than this, in points, are small: Auto
	// chooses the compact layout for them (DefaultCompactBelow by default).
	CompactBelow float64 `toml:"compact_below" json:"compactBelow"`
	// Points between panes.
	Gap float64 `toml:"gap" json:"gap"`
	// Show the status bar at the bottom of Bench's window.
	ShowStatusBar bool `toml:"show_status_bar" json:"showStatusBar"`
	// auto, always, or never: when tabs show their shortcuts.
	TabShortcutHints string `toml:"tab_shortcut_hints" json:"tabShortcutHints"`
	// auto, always, or never: when the tab and sub-tab strips show scroll
	// bars.
	ScrollBars string `toml:"scroll_bars" json:"scrollBars"`
	// system, light, or dark: the colours of Bench's own windows. Web
	// pages and apps follow macOS either way.
	Appearance string `toml:"appearance" json:"appearance"`
	// auto, standard, or compact: the size of Bench's title bar, pane
	// headers, tabs, and sub-tabs.
	Density string `toml:"density" json:"density"`
	// always, or auto: whether a pane that shows one App window, with
	// nothing to report, has a header, or lets the app's own title bar
	// stand in for it.
	PaneHeaders string `toml:"pane_headers" json:"paneHeaders"`
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
	// Dragging a window that isn't in a tab, or one of Bench's own
	// popped-out windows, onto a pane and resting there adds it to the
	// pane.
	DragToAdd bool `toml:"drag_to_add" json:"dragToAdd"`
	// Needs Screen Recording permission.
	LivePreviews bool `toml:"live_previews" json:"livePreviews"`
	// The size of the text in Bench's terminals, in points, from
	// MinTerminalFontSize to MaxTerminalFontSize.
	TerminalFontSize float64 `toml:"terminal_font_size" json:"terminalFontSize"`
	// Option sends Meta in Bench's terminals, as Esc and the key, for the
	// shell's word keys, rather than typing the key's Option character.
	TerminalOptionAsMeta bool `toml:"terminal_option_as_meta" json:"terminalOptionAsMeta"`
	// Post a notification when a terminal that isn't in view rings the
	// bell, asks for a notification, or finishes its command. Its sub-tab
	// and tab are marked either way.
	NotifyTerminals bool `toml:"notify_terminals" json:"notifyTerminals"`
	// Keep each terminal's recent output when Bench quits, to show it
	// again when the terminal starts next time, with its programs' colours
	// and the rest in grey.
	KeepTerminalOutput bool `toml:"keep_terminal_output" json:"keepTerminalOutput"`
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
	// Built-in item: a command to type into the shell as it starts, as a
	// launcher's is, rather than run instead of it.
	Run string `toml:"run,omitempty" json:"run,omitempty"`
	// Built-in item with Run: whether Bench types Run and presses Return
	// when it next opens, or only types it, for the user to run. Missing
	// means true; see StartsAgain.
	StartAgain *bool `toml:"start_again,omitempty" json:"startAgain,omitempty"`
	// Web and built-in items: the sub-tab's label before the page or shell
	// sets a title.
	Name string `toml:"name,omitempty" json:"name,omitempty"`
}

// StartsAgain reports whether a terminal item with Run runs it again when
// Bench next opens: start_again, which is true if it's missing.
func (it Item) StartsAgain() bool { return it.StartAgain == nil || *it.StartAgain }

// Launcher is a [[launcher]] entry: a terminal, or an app's window, that the
// add dialog and Jump to… add each time they're chosen, first in their
// lists. A terminal launcher types Run into the user's shell in Folder. An
// app launcher, one with App, opens Folder or URL in the app and docks the
// window whose title has Title.
type Launcher struct {
	// The launcher's name, which the lists show: unique, ignoring case.
	Name string `toml:"name" json:"name"`
	// App launcher: a bundle ID.
	App string `toml:"app,omitempty" json:"app,omitempty"`
	// Terminal launcher: the folder the terminal starts in, "~" if it's
	// empty. App launcher: a folder to open in the app, instead of URL.
	Folder string `toml:"folder,omitempty" json:"folder,omitempty"`
	// App launcher: a link to open in the app, instead of Folder.
	URL string `toml:"url,omitempty" json:"url,omitempty"`
	// App launcher: part of the title of the window to dock, ignoring case.
	// Without it, a launcher that opens a folder looks for the folder's
	// name.
	Title string `toml:"title,omitempty" json:"title,omitempty"`
	// Terminal launcher: a command to type into the shell as it starts.
	// Empty means just the shell.
	Run string `toml:"run,omitempty" json:"run,omitempty"`
	// Terminal launcher: whether the terminal types Run and presses Return
	// again when Bench next opens, or only types it, for the user to run.
	// Missing means true; see StartsAgain.
	StartAgain *bool `toml:"start_again,omitempty" json:"startAgain,omitempty"`
}

// IsApp reports whether a launcher opens an app, rather than a terminal.
func (l Launcher) IsApp() bool { return l.App != "" }

// StartsAgain reports whether a terminal launcher's terminals run Run again
// when Bench next opens: start_again, which is true if it's missing.
func (l Launcher) StartsAgain() bool { return l.StartAgain == nil || *l.StartAgain }

// StartAgainOf returns the start_again field for an item or launcher that
// starts again, or doesn't: nil for true, the default, which Format leaves
// out.
func StartAgainOf(on bool) *bool {
	if on {
		return nil
	}
	return &on
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
	// Dialogs holds the size of each of Bench's dialogs that the user has
	// resized, by kind, such as "settings", for it to open at that size
	// next time. Older files don't have it, and older builds ignore it.
	Dialogs map[string]DialogSize `json:"dialogs,omitempty"`
	// Closed holds the tabs that the user deleted or closed most recently,
	// the latest first, for Reopen Closed Tab. Older files don't have it,
	// and older builds ignore it.
	Closed []ClosedTab `json:"closed,omitempty"`
	// Updates is what Bench knows of its releases, for the
	// check_for_updates setting. Older files don't have it, and older
	// builds ignore it.
	Updates *UpdatesState `json:"updates,omitempty"`
}

// UpdatesState is what Bench knows of its releases.
type UpdatesState struct {
	// Checked is when Bench last asked GitHub for its latest release,
	// whether or not GitHub answered: Bench asks at most once a day.
	Checked time.Time `json:"checked,omitzero"`
	// Latest is the newest version GitHub named, such as "0.3.0", and URL
	// its page, while it's newer than the version Bench was then.
	Latest string `json:"latest,omitempty"`
	URL    string `json:"url,omitempty"`
	// Dismissed is the version whose notice the user dismissed, which
	// Bench doesn't announce again.
	Dismissed string `json:"dismissed,omitempty"`
}

// ClosedTab is a tab that the user deleted or closed: its arrangement and
// items, as Tabs and Items hold a tab's, and what went from config.toml
// with it, for Reopen Closed Tab to put back.
type ClosedTab struct {
	Tab TabState `json:"tab"`
	// Items holds the tab's items, by ID.
	Items map[string]ItemState `json:"items"`
	// Config is the tab's [[tab]] entry, and Rules the [[rule]] entries
	// that named it.
	Config Tab    `json:"config"`
	Rules  []Rule `json:"rules,omitempty"`
	// Index is where the tab was among the tabs, counting from 0.
	Index int `json:"index"`
	// Closed is when the tab was deleted or closed.
	Closed time.Time `json:"closed,omitzero"`
}

// DialogSize is the size of one of Bench's dialogs, in points.
type DialogSize struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// ScreenState names a display, so that Bench can find it again: by its ID,
// or else by its name, or else by where it is.
type ScreenState struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Frame is the whole display, in global coordinates.
	Frame layout.Rect `json:"frame"`
	// Window is where Bench's window was on the display, as its content
	// area in global coordinates, if it didn't fill the display's visible
	// area, as when the user tiled it into half the screen: Bench opens
	// there next time. It's left out when the window filled the screen.
	// Older files don't have it, and older builds ignore it.
	Window layout.Rect `json:"window,omitzero"`
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
	Preset string `json:"preset"`
	// FromPreset is the preset that a custom tab had before it became
	// custom, for Next Layout and Previous Layout to go on from. Older
	// files don't have it.
	FromPreset string       `json:"fromPreset,omitempty"`
	Root       *layout.Node `json:"root"`
	Focus      string       `json:"focus"`
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
	// Label is the user's own name for a Web or built-in item, which its
	// sub-tab shows instead of Title. Older files don't have it.
	Label string `json:"label,omitempty"`
	// URL is a Web item's address.
	URL string `json:"url,omitempty"`
	// Home is the address a Web item was given (in the config, or when it
	// was added), which it loads and the config keeps. Older files don't
	// have it.
	Home    string `json:"home,omitempty"`
	Profile string `json:"profile,omitempty"`
	// VSCode is the folder for a VS Code for the Web item.
	VSCode string `json:"vscode,omitempty"`
	// Folder is a terminal's starting folder, which matches it with its
	// config entry, and LastFolder the folder its shell was last in, as a
	// path with ~ for the home folder, where a shell without a command or
	// a typed one starts again. Older files don't have LastFolder.
	Folder     string `json:"folder,omitempty"`
	LastFolder string `json:"lastFolder,omitempty"`
	Command    string `json:"command,omitempty"`
	// Run is a command typed into a terminal's shell as it starts, and
	// StartAgain whether it's run again when Bench next opens, or only
	// typed. Older files don't have them.
	Run        string `json:"run,omitempty"`
	StartAgain bool   `json:"startAgain,omitempty"`
	// TitleFilter is the config's title filter for an App item, if any.
	TitleFilter string `json:"titleFilter,omitempty"`
	// OpenURL is the config's url for an App item, if any.
	OpenURL string `json:"openUrl,omitempty"`
	// OwnWindow is set for a popped-out Web or built-in item that had a
	// window of its own.
	OwnWindow bool `json:"ownWindow,omitempty"`
	// Zoom is a Web item's page zoom, as a scale such as 1.25 for 125%, or
	// 0 for actual size. Older files don't have it.
	Zoom float64 `json:"zoom,omitempty"`
}
