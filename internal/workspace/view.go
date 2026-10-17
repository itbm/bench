package workspace

import "github.com/itbm/bench/internal/layout"

// This file is the contract between Go and the frontend: View is the payload
// of the bench:state event, and the other types are what the frontend sends
// back. docs/architecture.md lists the bound methods and events.

// Item kinds.
const (
	KindWeb     = "web"
	KindApp     = "app"
	KindBuiltin = "builtin"
)

// Item statuses.
const (
	StatusReady       = "ready"
	StatusLaunching   = "launching"   // Its app is opening, or Bench is waiting for its window.
	StatusMissing     = "missing"     // Its app isn't running or its window has gone, and Bench isn't opening it.
	StatusUndocked    = "undocked"    // The user dragged its window out of the pane.
	StatusNeedsRoom   = "needs-room"  // Its window can't shrink to the pane.
	StatusFullScreen  = "full-screen" // Its window is in native full screen.
	StatusUnavailable = "unavailable" // Its window is on another desktop.
)

// View is everything the frontend draws. Go sends a whole View in every
// bench:state event; the frontend keeps only UI-local state (open menus and
// dialogs, the palette's query, a drag in progress).
type View struct {
	Tabs    []TabView `json:"tabs"`
	Current int       `json:"current"`
	// Display is the current tab as shown, after the small-screen setting.
	Display DisplayView `json:"display"`
	// Items holds every item in every tab, and popped-out items, by ID.
	Items map[string]ItemView `json:"items"`
	// Floating lists what's "Not in a tab": popped-out items, then open
	// standard windows Bench isn't managing (with IDs "f<window id>").
	Floating []ItemView `json:"floating"`

	Mode        string  `json:"mode"`      // auto, wide, or compact
	SizeClass   string  `json:"sizeClass"` // wide or compact
	SizeReason  string  `json:"sizeReason"`
	ScreenWidth float64 `json:"screenWidth"`

	Settings    SettingsView    `json:"settings"`
	Permissions PermissionsView `json:"permissions"`
	// Notices are problems to show in the status bar: config problems,
	// shortcuts that couldn't be registered, missing permissions.
	Notices []string `json:"notices"`
	// ConfigWritable is false when config.toml has problems; the layout
	// editor then applies changes without saving them to the file.
	ConfigWritable bool `json:"configWritable"`
	// ConfigPath is where config.toml is, for messages.
	ConfigPath string `json:"configPath"`

	// FirstRun is true until the user dismisses the permissions guide.
	FirstRun    bool `json:"firstRun"`
	OpenAtLogin bool `json:"openAtLogin"`
	// Demo is set in the browser preview (the server build), where window
	// contents are placeholders and the frontend handles shortcuts itself.
	Demo *DemoView `json:"demo,omitempty"`
}

// TabView is a tab in the tab strip, the palette, and the layout editor.
type TabView struct {
	Name string `json:"name"`
	// Shortcut is the display form, such as "⌃⌥1", or "" after the ninth tab.
	Shortcut string `json:"shortcut"`
	// Preset is a preset ID, or "custom" once items have been moved.
	Preset       string `json:"preset"`
	SmallScreens string `json:"smallScreens"`
	// Root is the tab's own tree, before small-screen changes.
	Root  *layout.Node `json:"root"`
	Focus string       `json:"focus"`
	// Apps are the names of the tab's apps, in order, without repeats.
	Apps    []string `json:"apps"`
	Windows int      `json:"windows"` // Number of items.
	Panes   int      `json:"panes"`
}

// DisplayView is the current tab as drawn.
type DisplayView struct {
	// Area is the pane area in window content coordinates, as last reported
	// by the frontend with SetPaneArea.
	Area layout.Rect `json:"area"`
	// Root is the displayed tree. In the "tabs" small-screen view it's a
	// single pane with ID "compact".
	Root *layout.Node `json:"root"`
	// Panes are the displayed panes in reading order, with their frames
	// and bodies in content coordinates.
	Panes    []PaneView           `json:"panes"`
	Dividers []layout.DividerRect `json:"dividers"`
	// Focus is the focused pane's ID ("compact" in the tabs view).
	Focus string `json:"focus"`
	// Compact is true for the tabs view.
	Compact bool `json:"compact"`
	// SplitsDisabled says why split actions are disabled, or is "".
	SplitsDisabled string `json:"splitsDisabled"`
}

// PaneView is a displayed pane.
type PaneView struct {
	ID     string      `json:"id"`
	Rect   layout.Rect `json:"rect"`
	Body   layout.Rect `json:"body"`
	Items  []string    `json:"items"`
	Active string      `json:"active"`
}

// ItemView is an item: a sub-tab and what its pane shows.
type ItemView struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // web, app, or builtin
	// App is the app's name: "Zed", "Slack", "Terminal", "Web page".
	App      string `json:"app"`
	BundleID string `json:"bundleId,omitempty"`
	Mono     string `json:"mono"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	// StatusText explains a status other than ready, for the pane body,
	// such as "Opening Zed…" or "Zed's window can't shrink to fit this pane".
	StatusText string `json:"statusText,omitempty"`
	// Tab is the index of the item's tab, or -1 if it isn't in a tab.
	Tab  int    `json:"tab"`
	Pane string `json:"pane,omitempty"`
	// URL is a Web item's address; Folder is a terminal's folder.
	URL    string `json:"url,omitempty"`
	Folder string `json:"folder,omitempty"`
}

// SettingsView is the [settings] table, for the layout editor.
type SettingsView struct {
	CompactBelow    float64 `json:"compactBelow"`
	Gap             float64 `json:"gap"`
	OpenMissingApps bool    `json:"openMissingApps"`
	HideOtherTabs   bool    `json:"hideOtherTabs"`
	HideOtherApps   bool    `json:"hideOtherApps"`
	NewWindows      string  `json:"newWindows"`
	FollowFocus     string  `json:"followFocus"`
	LivePreviews    bool    `json:"livePreviews"`
	LaunchTimeout   string  `json:"launchTimeout"` // Such as "15s".
}

// PermissionsView says which macOS permissions Bench has.
type PermissionsView struct {
	Accessibility   bool `json:"accessibility"`
	ScreenRecording bool `json:"screenRecording"`
}

// DemoView is the browser preview's simulated screen.
type DemoView struct {
	// Screen is "fit" (the browser's width), "monitor" (2,560 pt), or
	// "laptop" (1,512 pt).
	Screen string `json:"screen"`
}

// TabDraft is what the layout editor saves, with SaveTab.
type TabDraft struct {
	Name string `json:"name"`
	// Preset is a preset ID, or "custom" to keep the tab's current tree.
	Preset       string       `json:"preset"`
	SmallScreens string       `json:"smallScreens"`
	Settings     SettingsView `json:"settings"`
}

// Source is something the add dialog can add to a pane, with AddToPane.
type Source struct {
	// Type is one of:
	//   floating:   ID is a popped-out item or an unmanaged window ("f123").
	//   new-window: a new window of the app with BundleID.
	//   app:        the installed app with BundleID: open it if needed and
	//               add its windows that aren't in a tab, or open a new one.
	//   terminal:   a new terminal in Folder ("~" if empty).
	//   web:        a Web item for URL (https; "https://" is added if
	//               there's no scheme).
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	BundleID string `json:"bundleId,omitempty"`
	Folder   string `json:"folder,omitempty"`
	URL      string `json:"url,omitempty"`
}

// AddOption is one row of the add dialog.
type AddOption struct {
	// Group is "Windows not in a tab", "New window", "Built in", or "Apps".
	Group  string `json:"group"`
	Mono   string `json:"mono"`
	Label  string `json:"label"`
	Hint   string `json:"hint"`
	Source Source `json:"source"`
	// AsksForURL is true for "Web page…": the dialog asks for an address
	// and puts it in Source.URL.
	AsksForURL bool `json:"asksForUrl,omitempty"`
}

// Command is the payload of the bench:command event: something the frontend
// should do because of a global shortcut or a menu, such as opening the
// palette.
type Command struct {
	// Name is "open-palette", "toggle-palette", "new-tab", "edit-tab", or
	// "first-run".
	Name string `json:"name"`
}

// TermData is the payload of the term:data event.
type TermData struct {
	ID string `json:"id"`
	// Data is the output bytes, base64-encoded.
	Data string `json:"data"`
}
