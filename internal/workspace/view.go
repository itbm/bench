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
	StatusMinimised   = "minimised"   // The user minimised its window.
)

// View is everything the frontend draws. Go sends a whole View in every
// bench:state event; the frontend keeps only UI-local state (open menus and
// dialogs, the palette's query, a drag in progress).
type View struct {
	Tabs    []TabView `json:"tabs"`
	Current int       `json:"current"`
	// LastTab is the tab that was current before the current one, which
	// the last_tab shortcut goes back to and the jump palette lists first,
	// or -1 if there's none yet.
	LastTab int `json:"lastTab"`
	// Display is the current tab as shown, after the small-screen setting.
	Display DisplayView `json:"display"`
	// Items holds every item in every tab, and popped-out items, by ID.
	Items map[string]ItemView `json:"items"`
	// Floating lists what's "Not in a tab": popped-out items, then open
	// standard windows Bench isn't managing (with IDs "f<window id>").
	Floating []ItemView `json:"floating"`
	// Front is the window in front, the one the user last had the keyboard
	// in outside Bench's main window, if it isn't docked in a pane: a
	// popped-out item, a window not in a tab ("f<window id>"), or an item
	// whose window was dragged out of its pane. AddFrontWindow adds it to
	// the focused pane. It's "" otherwise.
	Front string `json:"front,omitempty"`

	Mode        string  `json:"mode"`      // auto, wide, or compact
	SizeClass   string  `json:"sizeClass"` // wide or compact
	SizeReason  string  `json:"sizeReason"`
	ScreenWidth float64 `json:"screenWidth"`
	// WindowWidth is the width of Bench's window's content area, which Auto
	// chooses the size class by.
	WindowWidth float64 `json:"windowWidth"`

	Settings SettingsView `json:"settings"`
	// Density is the density in use, standard or compact: the density
	// setting, where auto is compact in the compact size class. It sizes the
	// pane headers, their sub-tabs, and their buttons. TitleBar is the
	// title bar's, which in the app keeps the density it had at launch, as
	// the window's native title bar does (see titleBarLocked).
	Density  string `json:"density"`
	TitleBar string `json:"titleBar"`
	// SystemScrollBars is set while macOS shows scroll bars all the time,
	// rather than only while scrolling (Platform.ScrollBarsShown): the tab
	// and sub-tab strips show theirs too while the scroll_bars setting is
	// auto.
	SystemScrollBars bool `json:"systemScrollBars"`
	// Keys are the configured global shortcuts, for hints in the menus,
	// palette, and status bar.
	Keys KeysView `json:"keys"`
	// Shortcuts are the [shortcuts] table's fields, in config order, each
	// with whether its shortcuts work, for Settings ▸ Keyboard.
	Shortcuts   []ShortcutView  `json:"shortcuts"`
	Permissions PermissionsView `json:"permissions"`
	// Notices are problems to show in the status bar: config problems,
	// shortcuts that couldn't be registered, missing permissions. Some
	// have buttons for what can be done about them.
	Notices []NoticeView `json:"notices"`
	// ConfigWritable is false when config.toml has problems; Settings and
	// Tab properties then apply changes without saving them to the file.
	ConfigWritable bool `json:"configWritable"`
	// ConfigPath is where config.toml is, for messages.
	ConfigPath string `json:"configPath"`

	// FirstRun is true until the user dismisses the welcome guide.
	FirstRun    bool `json:"firstRun"`
	OpenAtLogin bool `json:"openAtLogin"`
	// LoginApproval is set while macOS has Bench's login item but won't
	// open Bench at login until the user allows it in System Settings ▸
	// General ▸ Login Items (OpenLoginItems).
	LoginApproval bool `json:"loginApproval,omitempty"`
	// InstallsUpdates is set when Bench has an updater, Sparkle, which
	// installs new versions (Options.Updater), for Settings to say so.
	InstallsUpdates bool `json:"installsUpdates,omitempty"`
	// Dialogs holds the size that the user gave each dialog they've
	// resized, by kind, such as "settings", for it to open at that size
	// (SetDialogSize).
	Dialogs map[string]DialogSize `json:"dialogs,omitempty"`
	// ClosedTabs names the tabs that Reopen closed tab can bring back
	// (ReopenTab), the latest first.
	ClosedTabs []string `json:"closedTabs,omitempty"`
	// Demo is set in the browser preview (the server build), where window
	// contents are placeholders and the frontend handles shortcuts itself.
	Demo *DemoView `json:"demo,omitempty"`
}

// NoticeView is a notice in the status bar, with the buttons that the list
// of notices shows for it, if any.
type NoticeView struct {
	Text    string         `json:"text"`
	Actions []NoticeAction `json:"actions,omitempty"`
}

// NoticeAction is a notice's button: it calls a bound method, such as
// ReloadConfig, that takes nothing, or with Arg, one that takes a string,
// such as UseSuggestedShortcuts with the fields to change; or, with Command
// instead, it does in the frontend what that bench:command does, such as
// "keyboard-settings".
type NoticeAction struct {
	Label   string `json:"label"`
	Method  string `json:"method,omitempty"`
	Arg     string `json:"arg,omitempty"`
	Command string `json:"command,omitempty"`
}

// Toast is a bench:toast with a button: its text, and what the button
// does. A toast without one is its text alone. Second is a second button,
// after the first, or nil: Go there, after Undo, on the toast that says a
// window moved to another tab.
type Toast struct {
	Text   string       `json:"text"`
	Action ToastAction  `json:"action"`
	Second *ToastAction `json:"second,omitempty"`
}

// ToastAction is a toast's button: it calls a bound method that takes one
// number, Arg, such as Undo with the change to take back, or, with Item set,
// one that takes an item's ID, such as PopOut.
type ToastAction struct {
	Label  string `json:"label"`
	Method string `json:"method"`
	Arg    int    `json:"arg"`
	Item   string `json:"item,omitempty"`
}

// TabView is a tab in the tab strip, the palette, and the Tab properties
// dialog.
type TabView struct {
	Name string `json:"name"`
	// Shortcut is the display form, such as "⌃⌥1", or "" after the ninth tab.
	Shortcut string `json:"shortcut"`
	// SendShortcut is the send_to_tab_modifiers shortcut that sends the
	// focused pane's shown window to this tab, in display form, such as
	// "⌃⌥⇧1", for Move to tab in the ⋯ menu. It's "" after the ninth tab,
	// while the field is off, and when another field has that shortcut,
	// which shortcuts.Bindings then leaves to that field.
	SendShortcut string `json:"sendShortcut"`
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
	// Attention is the number of the tab's items that need the user (see
	// ItemView.Attention), for the mark on the tab.
	Attention int `json:"attention,omitempty"`
	// Camera and Microphone are the numbers of the tab's pages that use
	// them, live or muted (see ItemView.Camera), and CameraLive and
	// MicrophoneLive are set if any of them has that one live, for the mark
	// on the tab.
	Camera         int  `json:"camera,omitempty"`
	Microphone     int  `json:"microphone,omitempty"`
	CameraLive     bool `json:"cameraLive,omitempty"`
	MicrophoneLive bool `json:"microphoneLive,omitempty"`
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
	// Zoomed is the zoomed pane's ID (ZoomPane), which Root is alone, or "".
	Zoomed string `json:"zoomed,omitempty"`
	// Arrange says what the keys that arrange the panes would do now, for
	// the jump palette and the menus.
	Arrange ArrangeView `json:"arrange"`
}

// ArrangeView says what the keys that arrange the current tab's panes
// would do now (decision 59): Grow and Shrink are set when GrowPane and
// ShrinkPane can make the focused pane larger or smaller, and Equal when
// EqualPanes has a split to make equal. NextLayout and PreviousLayout name
// the layouts that StepLayout(1) and StepLayout(-1) would give the tab,
// such as "Main and side", or are "" when it would refuse.
type ArrangeView struct {
	Grow           bool   `json:"grow,omitempty"`
	Shrink         bool   `json:"shrink,omitempty"`
	Equal          bool   `json:"equal,omitempty"`
	NextLayout     string `json:"nextLayout,omitempty"`
	PreviousLayout string `json:"previousLayout,omitempty"`
}

// PaneView is a displayed pane.
type PaneView struct {
	ID   string      `json:"id"`
	Rect layout.Rect `json:"rect"`
	Body layout.Rect `json:"body"`
	// Header is the height of the pane's header, in points, which Body is
	// below (with its 1 pt divider): layout.HeaderHeight, or
	// layout.CompactHeaderHeight in the compact density. It's 0 for a pane
	// with no header: the "tabs" view's pane, whose sub-tabs are in the
	// title bar, and a pane whose one App window's own title bar stands in
	// for it (pane_headers).
	Header float64 `json:"header"`
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
	// Icon is the address of the app's icon, which its tile shows in
	// place of Mono, or "" (see icons.go).
	Icon string `json:"icon,omitempty"`
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
	// Icon is the address of the item's icon, which its tile shows in
	// place of Mono: its app's, or its page's once the page has one. A
	// terminal has none (see icons.go).
	Icon string `json:"icon,omitempty"`
	// Title is what the item's sub-tab shows: Label, if the user has given
	// a Web or Built-in item a name of its own with Rename…, or else the
	// title its window, page, or shell gives it, which AutoTitle then has.
	Title     string `json:"title"`
	Label     string `json:"label,omitempty"`
	AutoTitle string `json:"autoTitle,omitempty"`
	Status    string `json:"status"`
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
	// URL is a Web item's address; Folder is a terminal's folder, as its
	// config entry or the add dialog gave it, and Here the folder its
	// shell is in now, as far as Bench knows, for New terminal in.
	URL    string `json:"url,omitempty"`
	Folder string `json:"folder,omitempty"`
	Here   string `json:"here,omitempty"`
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
	// Zoom is a Web item's page zoom in percent, such as 125, left out at
	// actual size (ZoomPage). CanGoBack and CanGoForward say whether its
	// view can go back or forward, for Back and Forward, and Camera and
	// Microphone how its page uses them, "active" or "muted", for the mark
	// on its sub-tab, as the view last reported them. Finding is set while
	// its find bar is open (SetFindBar), at the top of its pane's body,
	// layout.FindBarHeight tall, above its web view.
	Zoom         int    `json:"zoom,omitempty"`
	CanGoBack    bool   `json:"canGoBack,omitempty"`
	CanGoForward bool   `json:"canGoForward,omitempty"`
	Camera       string `json:"camera,omitempty"`
	Microphone   string `json:"microphone,omitempty"`
	Finding      bool   `json:"finding,omitempty"`
	// Ended is set for a terminal whose command has finished. Its output
	// stays, and Return runs the command again.
	Ended bool `json:"ended,omitempty"`
	// Attention says why an item in a tab needs the user, for the mark on
	// its sub-tab, such as "Rang the bell" or "Notification: Done", until
	// it's shown: a terminal rang the bell, or it or a page posted a
	// notification, or a terminal's command finished, while the user
	// couldn't see it.
	Attention string `json:"attention,omitempty"`
	// Used orders items by when they were last shown or focused, for the
	// jump palette to list the most recent first: the higher, the more
	// recent. It's left out for an item that hasn't been used since Bench
	// opened.
	Used int `json:"used,omitempty"`
}

// SettingsView is the [settings] table, for the Settings dialog, which saves
// it with SaveSettings.
type SettingsView struct {
	FillScreen               bool    `json:"fillScreen"`
	MenuBar                  string  `json:"menuBar"` // name, icon, or hidden.
	ShowOnTabShortcut        bool    `json:"showOnTabShortcut"`
	CheckForUpdates          bool    `json:"checkForUpdates"`
	ConfirmQuitBusyTerminals bool    `json:"confirmQuitBusyTerminals"`
	ConfirmClose             bool    `json:"confirmClose"`
	CompactBelow             float64 `json:"compactBelow"`
	Gap                      float64 `json:"gap"`
	ShowStatusBar            bool    `json:"showStatusBar"`
	TabShortcutHints         string  `json:"tabShortcutHints"` // auto, always, or never.
	ScrollBars               string  `json:"scrollBars"`       // auto, always, or never.
	Appearance               string  `json:"appearance"`       // system, light, or dark.
	Density                  string  `json:"density"`          // auto, standard, or compact.
	PaneHeaders              string  `json:"paneHeaders"`      // always or auto.
	OpenMissingApps          bool    `json:"openMissingApps"`
	HideOtherTabs            bool    `json:"hideOtherTabs"`
	HideOtherApps            bool    `json:"hideOtherApps"`
	NewWindows               string  `json:"newWindows"`
	FollowFocus              string  `json:"followFocus"`
	DragToAdd                bool    `json:"dragToAdd"`
	LivePreviews             bool    `json:"livePreviews"`
	// TerminalFontSize is the terminals' text size, in points. Zero, as
	// when it's missing, keeps it as it is.
	TerminalFontSize     float64 `json:"terminalFontSize"`
	TerminalOptionAsMeta bool    `json:"terminalOptionAsMeta"`
	NotifyTerminals      bool    `json:"notifyTerminals"`
	KeepTerminalOutput   bool    `json:"keepTerminalOutput"`
	LaunchTimeout        string  `json:"launchTimeout"` // Such as "15s".
	// Launchers are the [[launcher]] entries, in order. SaveSettings
	// replaces them with these, unless they're missing (nil), which keeps
	// them as they are.
	Launchers []LauncherView `json:"launchers"`
	// Shortcuts are the [shortcuts] table's fields that Settings ▸
	// Keyboard changes, by their names in config.toml, such as "jump",
	// each a shortcut in config form or "" for off. SaveSettings sets
	// those it has and keeps the rest; the View leaves it out.
	Shortcuts map[string]string `json:"shortcuts,omitempty"`
}

// ShortcutView is a field of the [shortcuts] table, for Settings ▸
// Keyboard: its shortcut, or modifiers, and whether its shortcuts work.
type ShortcutView struct {
	// Field is its name in config.toml, such as "jump", and Label what its
	// shortcuts do, such as "Jump to…".
	Field string `json:"field"`
	Label string `json:"label,omitempty"`
	// Keys names the keys that Bench adds to a field of modifiers, such
	// as "the arrow keys", or is "" for a whole shortcut.
	Keys string `json:"keys,omitempty"`
	// Value is the field as config.toml has it, such as "ctrl+alt+space",
	// or "" for off, and Display the same as macOS shows it, "⌃⌥Space".
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	// Default is the field's default value.
	Default string `json:"default,omitempty"`
	// Problem says why some or all of its shortcuts don't work, or may
	// not, in a sentence or two, or is "" when they work: Bench couldn't
	// register them, the field has a problem, or macOS or VoiceOver uses
	// them.
	Problem string `json:"problem,omitempty"`
	// Suggestion is a value to use instead, in config form, which Bench
	// checked it can register and which clashes with nothing it knows of,
	// and SuggestionDisplay the same as macOS shows it; or "".
	Suggestion        string `json:"suggestion,omitempty"`
	SuggestionDisplay string `json:"suggestionDisplay,omitempty"`
}

// ShortcutReport is what the shell found as it registered the global
// shortcuts (Engine.ReportShortcuts): each field of the [shortcuts] table,
// with whether its shortcuts work, and the notices about those that don't.
type ShortcutReport struct {
	Fields  []ShortcutView
	Notices []NoticeView
}

// LauncherView is a [[launcher]] entry, as the add dialog's Save as launcher
// and Settings ▸ Launchers show and save it.
type LauncherView struct {
	Name string `json:"name"`
	// Kind is "terminal" or "app".
	Kind string `json:"kind"`
	// Folder is a terminal's starting folder, or a folder to open in the
	// app.
	Folder string `json:"folder"`
	// Command is a terminal's command, which it types into the shell, and
	// StartAgain whether it runs it again when Bench next opens.
	Command    string `json:"command"`
	StartAgain bool   `json:"startAgain"`
	// BundleID is an app launcher's app, URL a link to open in it instead
	// of Folder, and Title part of the title of the window to dock.
	BundleID string `json:"bundleId"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	// App and Mono are the app's name and monogram, or the terminal's, and
	// Hint what the launcher opens, in a few words, for the lists. Saving
	// ignores them.
	App  string `json:"app"`
	Mono string `json:"mono"`
	Hint string `json:"hint"`
	// Icon is the address of an app launcher's app's icon, or "".
	Icon string `json:"icon,omitempty"`
}

// KeysView holds the configured shortcuts in display form (shortcuts.Display),
// such as "⌃⌥Space". A shortcut that's turned off is "".
type KeysView struct {
	// Tabs is the modifiers for tabs 1 to 9, such as "⌃⌥".
	Tabs        string `json:"tabs"`
	NextTab     string `json:"nextTab"`
	PreviousTab string `json:"previousTab"`
	LastTab     string `json:"lastTab"`
	Jump        string `json:"jump"`
	Rearrange   string `json:"rearrange"`
	// FocusPane and MoveWindow are modifiers for the arrow keys, such as
	// "⌃⌥" and "⌃⌥⇧".
	FocusPane  string `json:"focusPane"`
	MoveWindow string `json:"moveWindow"`
	// SendToTab is modifiers for 1 to 9 and `, such as "⌃⌥⇧", which send
	// the focused pane's shown window to that tab (SendToTab), or to the
	// last used tab (SendToLastTab).
	SendToTab  string `json:"sendToTab"`
	NextWindow string `json:"nextWindow"`
	ZoomPane   string `json:"zoomPane"`
	// GrowPane, ShrinkPane, EqualPanes, NextLayout, and PreviousLayout
	// arrange the current tab's panes (decision 59). They're off by
	// default.
	GrowPane       string `json:"growPane"`
	ShrinkPane     string `json:"shrinkPane"`
	EqualPanes     string `json:"equalPanes"`
	NextLayout     string `json:"nextLayout"`
	PreviousLayout string `json:"previousLayout"`
	// AddFrontWindow adds the window in front to the focused pane
	// (AddFrontWindow). It's off by default.
	AddFrontWindow string `json:"addFrontWindow"`
}

// PermissionsView says which macOS permissions Bench has.
type PermissionsView struct {
	Accessibility bool `json:"accessibility"`
	// AccessibilityUpdated is set while Bench lacks Accessibility
	// permission that an earlier version of it had: macOS stopped
	// applying it with the update, and the user removes Bench from the
	// list in System Settings and turns it on again.
	AccessibilityUpdated bool `json:"accessibilityUpdated,omitempty"`
	ScreenRecording      bool `json:"screenRecording"`
	// ScreenRecordingReopen is set once Bench has asked for Screen
	// Recording permission, or has got it, since it opened: macOS applies
	// it only as Bench opens, so the guide and Settings offer Quit and
	// Reopen Bench (QuitAndReopen).
	ScreenRecordingReopen bool `json:"screenRecordingReopen,omitempty"`
}

// DialogSize is a dialog's size, in points.
type DialogSize struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
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
	// Place is the tab's place among the tabs, counting from 1, which gives
	// it its shortcut. 0 leaves it where it is, or puts a new tab last.
	Place int `json:"place"`
}

// Source is something the add dialog can add to a pane, with AddToPane.
type Source struct {
	// Type is one of:
	//   floating:   ID is a popped-out item or an unmanaged window ("f123").
	//   item:       ID is an item in other tabs, to show in this pane too.
	//   new-window: a new window of the app with BundleID.
	//   app:        the installed app with BundleID: open it if needed and
	//               add its windows that aren't in a tab, or open a new one.
	//   terminal:   a new terminal in Folder ("~" if empty), which types
	//               Command into the shell and runs it, if there is one,
	//               with Name as its label until the shell sets a title.
	//   launcher:   ID is the name of a [[launcher]] to add.
	//   web:        a Web item for URL (https; "https://" is added if
	//               there's no scheme).
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	BundleID string `json:"bundleId,omitempty"`
	Folder   string `json:"folder,omitempty"`
	URL      string `json:"url,omitempty"`
	Command  string `json:"command,omitempty"`
	Name     string `json:"name,omitempty"`
	// StartAgain is whether a terminal with Command runs it again when
	// Bench next opens, or only types it, for the user to run. Missing
	// means true, as start_again does.
	StartAgain *bool `json:"startAgain,omitempty"`
}

// AddOption is one row of the add dialog.
type AddOption struct {
	// Group is "Launchers", "Windows not in a tab", "Windows in other tabs",
	// "New window", "Built in", "Web apps", or "Apps". The first Built in
	// row is "New terminal in <folder>", while the pane, or else the
	// focused pane, shows a terminal whose shell is in a folder other than
	// the home folder.
	Group string `json:"group"`
	Mono  string `json:"mono"`
	// Icon is the address of the row's app's or page's icon, which its
	// tile shows in place of Mono, or "".
	Icon   string `json:"icon,omitempty"`
	Label  string `json:"label"`
	Hint   string `json:"hint"`
	Source Source `json:"source"`
	// AsksForURL is true for "Web page…": the dialog asks for an address
	// and puts it in Source.URL.
	AsksForURL bool `json:"asksForUrl,omitempty"`
	// AsksForCommand is true for "New terminal…": the dialog asks for a
	// name, a command, and a folder, and puts them in the Source.
	AsksForCommand bool `json:"asksForCommand,omitempty"`
	// WebAppBrowser is set for "Web page…" when AddWebsite asks the default
	// browser to make a web app of a site that has none: it's the browser's
	// name, such as "Safari". It's "" when Bench shows such a site itself,
	// because it doesn't have Accessibility permission, which it needs to
	// dock web apps, or the default browser can't make them.
	WebAppBrowser string `json:"webAppBrowser,omitempty"`
	// Browses is true for "Browse…": the dialog closes, and BrowseToAdd
	// shows macOS's open panel for an app or a program.
	Browses bool `json:"browses,omitempty"`
	// ChoosesFolder is true for "New terminal in…": the dialog shows the
	// folder picker (ChooseFolder), starting in Source.Folder, or the home
	// folder if it's "", and adds a terminal in the folder chosen.
	ChoosesFolder bool `json:"choosesFolder,omitempty"`
}

// BrowseResult is what BrowseToAdd did with what the user chose in the
// open panel: Added is the name of the app it added to the pane, or
// Command, Name, and Folder if it's set, are for the New terminal… form,
// to run the program or script chosen. All are empty if the user
// cancelled.
type BrowseResult struct {
	Added   string `json:"added,omitempty"`
	Command string `json:"command,omitempty"`
	Name    string `json:"name,omitempty"`
	Folder  string `json:"folder,omitempty"`
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
	// Name is "open-palette", "toggle-palette", "new-tab",
	// "new-tab-from-windows" (the New tab dialog, with "Windows that are
	// open" chosen), "edit-tab" (the current tab's properties),
	// "settings", "keyboard-settings" (Settings, at its Keyboard section),
	// "first-run",
	// "focus-terminal" (give the terminal ItemID the keyboard focus, because
	// its pane was focused: the engine has already activated Bench and
	// given its UI the keyboard), "confirm-quit" (ask before quitting, as
	// Terminals are running something), "confirm-close-terminal" (ask,
	// in the terminal ItemID's own window, before closing it, as it's
	// running something), or, from Pane ▸ Page, "find-in-page" and
	// "copy-page-link" (open the Web item ItemID's find bar, or put its
	// page's link on the clipboard).
	Name   string `json:"name"`
	ItemID string `json:"itemId,omitempty"`
	// Terminals are the terminals that confirm-quit and
	// confirm-close-terminal ask about.
	Terminals []BusyTerminal `json:"terminals,omitempty"`
	// Reopen is set on the confirm-quit that QuitAndReopen sends: its
	// Quit and Reopen calls QuitAndReopen again, rather than Quit.
	Reopen bool `json:"reopen,omitempty"`
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
