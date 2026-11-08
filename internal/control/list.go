package control

import "strconv"

// ListVersion is the version of List's shape. It goes up only if a field
// changes or goes; new fields can come without it.
const ListVersion = 1

// List is what bench list --json prints: Bench's tabs, their panes, and
// what each pane holds, with what needs the user. docs/user-guide.md
// (Control Bench from scripts) documents it for scripts, field by field, so
// a change here needs one there.
type List struct {
	// Version is ListVersion.
	Version int `json:"version"`
	// Current is the number of the tab that Bench shows, from 1.
	Current int `json:"current"`
	// Attention is how many items need the user: a terminal that rang its
	// bell or finished its command, or a page that posted a notification,
	// while the user couldn't see it. An item in more than one tab counts
	// once, and the drop-down counts too.
	Attention int `json:"attention"`
	// Unread is the total of the items' unread counts, each item once.
	Unread Unread `json:"unread"`
	// Tabs are the tabs, in order.
	Tabs []Tab `json:"tabs"`
	// Dropdown is the drop-down terminal, which is in no tab, or nil, which
	// the JSON has as null, if no terminal is the drop-down. Its Shown is
	// true while it shows over the app in front, with the keyboard.
	Dropdown *Item `json:"dropdown"`
}

// Tab is a tab in a List.
type Tab struct {
	// Number is the tab's place, from 1: bench tab 2 and bench://tab/2
	// switch to the second tab.
	Number int `json:"number"`
	// Name is the tab's name.
	Name string `json:"name"`
	// Current is true for the tab that Bench shows.
	Current bool `json:"current"`
	// Colour is the tab's colour, one of config.TabColours, such as
	// "blue", or "" for none.
	Colour string `json:"colour"`
	// Attention is how many of the tab's items need the user.
	Attention int `json:"attention"`
	// Unread is the total of the tab's unread counts.
	Unread Unread `json:"unread"`
	// Panes are the tab's panes, in reading order.
	Panes []Pane `json:"panes"`
}

// Pane is a pane in a Tab.
type Pane struct {
	// ID is the pane's ID, such as "p3". It lasts while Bench runs.
	ID string `json:"id"`
	// Focused is true for the tab's focused pane, which bench launch adds
	// to in the current tab.
	Focused bool `json:"focused"`
	// Items are what the pane holds, in its sub-tabs' order.
	Items []Item `json:"items"`
}

// Item is something that a Pane holds: one of its sub-tabs.
type Item struct {
	// ID is the item's ID, such as "w7". It lasts while Bench runs, and
	// scripts mustn't read anything into it.
	ID string `json:"id"`
	// Kind is "app" (another app's window), "web" (a web page in Bench),
	// "vscode" (VS Code for the Web), or "terminal" (a terminal in Bench).
	Kind string `json:"kind"`
	// App is the app's name, such as "Zed", or "Terminal" for a terminal
	// in Bench.
	App string `json:"app"`
	// Title is what the item's sub-tab says: its window's or page's title,
	// or the name that the user gave it.
	Title string `json:"title"`
	// Shown is true for the item that the pane shows: its selected
	// sub-tab.
	Shown bool `json:"shown"`
	// Attention says why the item needs the user, as its sub-tab's bell
	// does, such as "Rang the bell", or "Notification: " and the
	// notification's title and text, or is "" if it doesn't.
	Attention string `json:"attention"`
	// Unread is the item's unread count.
	Unread Unread `json:"unread"`
	// KeepRunning is true for a web page whose Keep Running in the
	// Background is on, which Bench loads hidden soon after it opens.
	KeepRunning bool `json:"keepRunning"`
	// NotLoaded is true for a web page that Bench hasn't loaded, and won't
	// until the user shows it, as its sub-tab's tooltip says: it has no
	// unread count, and posts no notifications, until then.
	NotLoaded bool `json:"notLoaded"`
}

// Item kinds in a List.
const (
	KindApp      = "app"
	KindWeb      = "web"
	KindVSCode   = "vscode"
	KindTerminal = "terminal"
)

// Unread is an unread count, from an app's Dock badge or the number at the
// start of a page's title. It's all zero while the unread_counts setting is
// off.
type Unread struct {
	// Count is how many, or 0.
	Count int `json:"count"`
	// More is true when there are more than Count, as with a badge that
	// says "99+".
	More bool `json:"more"`
	// Dot is true when there's something unread without a count, as with
	// a badge that shows a dot.
	Dot bool `json:"dot"`
}

// Text returns the count as Bench shows it: "3", "99+", "•" for a dot, or
// "" for none.
func (u Unread) Text() string {
	switch {
	case u.Count > 0 && u.More:
		return strconv.Itoa(u.Count) + "+"
	case u.Count > 0:
		return strconv.Itoa(u.Count)
	case u.Dot:
		return "•"
	}
	return ""
}

// TabNames returns the names of l's tabs, in order, for FindTab.
func (l List) TabNames() []string {
	names := make([]string, len(l.Tabs))
	for i, t := range l.Tabs {
		names[i] = t.Name
	}
	return names
}
