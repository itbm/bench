package mac

import "strings"

// How well a menu item's title names the New Window item.
const (
	// newWindowExact is "New Window" itself, in any case.
	newWindowExact = 2
	// newWindowOther is another "New … Window" item, such as Finder's New
	// Finder Window.
	newWindowOther = 1
)

// newWindowRank ranks a menu item's title as the New Window item:
// newWindowExact, newWindowOther, or 0 for anything else. Private and
// incognito windows don't count, and nor do items that open something other
// than a window, such as New Tab or New File: in many apps those, not New
// Window, have Command-N. Menus in other languages don't match, so for
// those apps Bench opens the app again instead.
func newWindowRank(title string) int {
	t := strings.ToLower(strings.TrimSpace(title))
	t = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(t, "…"), "..."))
	if t == "new window" {
		return newWindowExact
	}
	if !strings.HasPrefix(t, "new ") || !strings.HasSuffix(t, " window") {
		return 0
	}
	if strings.Contains(t, "private") || strings.Contains(t, "incognito") {
		return 0
	}
	return newWindowOther
}

// How well a menu item's title names the item that closes the app's main
// window.
const (
	// closeWindowExact is "Close Window" itself, in any case.
	closeWindowExact = 2
	// closeWindowPlain is "Close", which closes the main window, or the
	// document it shows, in most apps that have no Close Window.
	closeWindowPlain = 1
)

// closeWindowRank ranks a menu item's title as the item that closes the
// app's main window: closeWindowExact, closeWindowPlain, or 0 for anything
// else, such as Close Tab, Close All, or Close Others, which close more or
// less than the window. Menus in other languages don't match, so a window of
// such an app that has no close button stays open, and Bench says so.
func closeWindowRank(title string) int {
	t := strings.ToLower(strings.TrimSpace(title))
	t = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(t, "…"), "..."))
	switch t {
	case "close window":
		return closeWindowExact
	case "close":
		return closeWindowPlain
	}
	return 0
}
