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

	Settings SettingsView `json:"settings"`
	// Keys are the configured global shortcuts, for hints in the menus,
	// palette, and status bar.
	Keys        KeysView        `json:"keys"`
	Permissions PermissionsView `json:"permissions"`
	// Notices are problems to show in the status bar: config problems,
	// shortcuts that couldn't be registered, missing permissions.
	Notices []string `json:"notices"`
	// ConfigWritable is false when config.toml has problems; Settings and
	// Tab properties then apply changes without saving them to the file.
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

// TabView is a tab in the tab strip, the palette, and the Tab properties
// dialog.
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
	// Apps are the names of the tab's apps, in order, without repeats, and
	// Windows the number of its items, leaving out hidden ones. Panes is
	// the number of panes in its own tree (Root), before small-screen
	// changes: the "tabs" view shows them as one, so the status bar counts
	// the Display's.
	Apps    []string `json:"apps"`
	Windows int      `json:"windows"`
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
	ID   string      `json:"id"`
	Rect layout.Rect `json:"rect"`
	Body layout.Rect `json:"body"`
	// Items are the pane's sub-tabs, without hidden items (see ItemView).
	Items  []string `json:"items"`
	Active string   `json:"active"`
	// Remembered lists the apps whose windows the pane is waiting for: one
	// for each app with hidden items in it (in the whole tab, for the
	// "tabs" view's pane).
	Remembered []RememberedApp `json:"remembered"`
}

// RememberedApp is an app whose windows a pane keeps places for, out of
// sight, until they come back.
type RememberedApp struct {
	// ItemID is the place that Open fills (OpenItem).
	ItemID   string `json:"itemId"`
	BundleID string `json:"bundleId"`
	App      string `json:"app"`
	Mono     string `json:"mono"`
	// Text says why there's no window, such as "Brave isn't running" or
	// "Zed isn't installed".
	Text string `json:"text"`
	// CanOpen is false for an app that macOS says isn't installed.
	CanOpen bool `json:"canOpen"`
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
	// NoWindow is set for an App item that has no window: its app isn't
	// running or installed, or has no window for it. Removing it from
	// Bench (PopOut) forgets it.
	NoWindow bool `json:"noWindow,omitempty"`
	// CanOpen is set for an App item without a window whose app macOS
	// hasn't said isn't installed: OpenItem can open it.
	CanOpen bool `json:"canOpen,omitempty"`
	// Hidden is set for an item that isn't drawn as a sub-tab: an App item
	// without a window, unless Bench is opening its app for it. Its pane
	// lists its app in Remembered.
	Hidden bool `json:"hidden,omitempty"`
	// Tab is the index of the item's tab, or -1 if it isn't in a tab. For
	// an item in several tabs, it's the current tab if the item is there,
	// or else the first tab holding it. Pane is its pane in that tab.
	Tab  int    `json:"tab"`
	Pane string `json:"pane,omitempty"`
	// Tabs lists every tab holding an item that's in more than one, in
	// order; it's left out for an item in one tab or none.
	Tabs []int `json:"tabs,omitempty"`
	// URL is a Web item's address; Folder is a terminal's folder.
	URL    string `json:"url,omitempty"`
	Folder string `json:"folder,omitempty"`
	// Host is the host of the page a Web item shows, such as app.slack.com,
	// and Site the end of it that names its site, such as slack.com. They're
	// left out until the item's web view reports a page with a host, and
	// for VS Code for the Web, whose page is on the Mac itself. OffSite is
	// set when the page is on another site than the item's own address, or
	// has no web address, such as a data: page, which has no host; and
	// NotSecure when it isn't secure: an http page, or an https one that has
	// loaded content that isn't.
	Host      string `json:"host,omitempty"`
	Site      string `json:"site,omitempty"`
	OffSite   bool   `json:"offSite,omitempty"`
	NotSecure bool   `json:"notSecure,omitempty"`
	// Ended is set for a terminal whose command has finished. Its output
	// stays, and Return runs the command again.
	Ended bool `json:"ended,omitempty"`
}

// SettingsView is the [settings] table, for the Settings dialog, which saves
// it with SaveSettings.
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

// KeysView holds the configured shortcuts in display form (shortcuts.Display),
// such as "⌃⌥Space". A shortcut that's turned off is "".
type KeysView struct {
	// Tabs is the modifiers for tabs 1 to 9, such as "⌃⌥".
	Tabs        string `json:"tabs"`
	NextTab     string `json:"nextTab"`
	PreviousTab string `json:"previousTab"`
	Jump        string `json:"jump"`
	Rearrange   string `json:"rearrange"`
	// FocusPane and MoveWindow are modifiers for the arrow keys, such as
	// "⌃⌥" and "⌃⌥⇧".
	FocusPane  string `json:"focusPane"`
	MoveWindow string `json:"moveWindow"`
	NextWindow string `json:"nextWindow"`
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

// TabDraft is what the Tab properties dialog saves, with SaveTab. It holds
// only the tab's own properties: the settings that apply to every tab are
// the Settings dialog's (SaveSettings).
type TabDraft struct {
	Name string `json:"name"`
	// Preset is a preset ID, or "custom" to keep the tab's current tree.
	Preset       string `json:"preset"`
	SmallScreens string `json:"smallScreens"`
}

// Source is something the add dialog can add to a pane, with AddToPane.
type Source struct {
	// Type is one of:
	//   floating:   ID is a popped-out item or an unmanaged window ("f123").
	//   item:       ID is an item in other tabs, to show in this pane too.
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
	// Group is "Windows not in a tab", "Windows in other tabs", "New window",
	// "Built in", "Web apps", or "Apps".
	Group  string `json:"group"`
	Mono   string `json:"mono"`
	Label  string `json:"label"`
	Hint   string `json:"hint"`
	Source Source `json:"source"`
	// AsksForURL is true for "Web page…": the dialog asks for an address
	// and puts it in Source.URL.
	AsksForURL bool `json:"asksForUrl,omitempty"`
	// WebAppBrowser is set for "Web page…" when AddWebsite asks the default
	// browser to make a web app of a site that has none: it's the browser's
	// name, such as "Safari". It's "" when Bench shows such a site itself,
	// because it doesn't have Accessibility permission, which it needs to
	// dock web apps, or the default browser can't make them.
	WebAppBrowser string `json:"webAppBrowser,omitempty"`
}

// WebAppChoice is what a Web item's menu offers to put a web app of its
// site in its place, from WebAppChoice.
type WebAppChoice struct {
	// Offer is one of:
	//   use:  the web app of the item's site, App, which Browser made, is
	//         installed (UseWebApp).
	//   make: there's none, and the default browser, Browser, can make one
	//         (MakeWebApp).
	//   "":   neither, or the item isn't a web page that can have a web
	//         app, such as VS Code for the Web.
	Offer   string `json:"offer"`
	App     string `json:"app,omitempty"`
	Browser string `json:"browser,omitempty"`
}

// WebsiteResult is what AddWebsite did with an address, or what MakeWebApp
// asked the default browser to do.
type WebsiteResult struct {
	// Added is what it added to the pane:
	//   web-app: the site's web app, App, which Browser made.
	//   web:     a Web item, which shows the page in Bench, for the reason
	//            in Note.
	//   "":      nothing yet. It opened the site in the default browser,
	//            Browser, which makes a web app of it with Steps, and
	//            AddWebAppWhenReady adds the web app once it's made.
	Added   string `json:"added"`
	App     string `json:"app,omitempty"`
	Browser string `json:"browser,omitempty"`
	Steps   string `json:"steps,omitempty"`
	// Note tells the user what it added and why, as a sentence for a
	// toast, when it added something.
	Note string `json:"note,omitempty"`
}

// Command is the payload of the bench:command event: something the frontend
// should do because of a global shortcut or a menu, such as opening the
// palette.
type Command struct {
	// Name is "open-palette", "toggle-palette", "new-tab", "edit-tab" (the
	// current tab's properties), "settings", "first-run", or
	// "focus-terminal" (give the terminal ItemID the keyboard focus, because
	// its pane was focused: the engine has already activated Bench and
	// given its UI the keyboard).
	Name   string `json:"name"`
	ItemID string `json:"itemId,omitempty"`
}

// TermData is the payload of the term:data event.
type TermData struct {
	ID string `json:"id"`
	// Data is the output bytes, base64-encoded.
	Data string `json:"data"`
	// Offset is where Data starts in the terminal's output: the number of
	// bytes output before it.
	Offset int64 `json:"offset"`
}

// TermSnapshot is a terminal's recent output, for a new view to replay.
type TermSnapshot struct {
	// Data is the output bytes, base64-encoded.
	Data string `json:"data"`
	// Offset is where Data ends in the terminal's output. term:data events
	// whose output ends there or before it are already in Data.
	Offset int64 `json:"offset"`
}
