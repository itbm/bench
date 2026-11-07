//go:build darwin || server

package ui

import (
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/internal/workspace"
)

// This file shows Bench's own menus, such as a tab's or a window's ⋯ menu,
// as native macOS menus, with Wails' context menus (decision 71). The
// frontend sends a menu's entries as data (workspace.Menu); nativeItems
// makes them into a native menu's items, buildNativeMenu into Wails'
// menu, and contextMenu shows it and reports the entry chosen.

// contextMenuName is the name of the one context menu that Bench registers
// with Wails, whose items change each time it opens.
const contextMenuName = "bench-menu"

// Most characters of a native menu item's label and of an entry's detail
// in it: longer ones are cut, with an ellipsis, so that a long tab name or
// window title can't make the menu as wide as the screen, as the HTML
// menus cut them with CSS. A note or status line longer than maxNote goes
// on more than one line, as the HTML menus wrap it: each line is an item.
const (
	maxLabel  = 60
	maxDetail = 40
	maxNote   = 60
)

// choiceWait is how long contextMenu waits for the entry chosen once the
// menu has closed. AppKit sends the item's action before the menu's call
// returns, but Wails hands it to Go through a channel and two goroutines,
// so it can arrive a moment after; a menu closed with nothing chosen
// answers after this wait.
const choiceWait = 250 * time.Millisecond

// nativeItem is an item of a native menu, as nativeItems makes it from a
// workspace.MenuEntry.
type nativeItem struct {
	// ID is what choosing the item reports, or "" for one that can't be
	// chosen, such as a heading, or a submenu's own item.
	ID string
	// Label is what the item says, with the entry's detail and keys.
	Label string
	// Enabled is set when the item can be chosen, or its submenu opened.
	Enabled bool
	// Choice is set for one of a choice, ticked when Checked is.
	Choice, Checked bool
	// Separator is set for a separator.
	Separator bool
	// Items are the items of the item's submenu, if it has one.
	Items []nativeItem
}

// nativeItems makes a menu's entries into a native menu's items. An entry
// becomes an item labelled with its detail, in brackets, and its keys, as
// the app menu shows Bench's global shortcuts (withKeys), and is off where
// the entry is off or has no ID. An entry with a submenu becomes a submenu,
// off where the entry is, or none of its items is on. A native menu has no
// headings, notes, or status lines, so they become items that are off,
// which VoiceOver reads as dimmed, a long note or status line on more than
// one (lines). Separators go only between items: none first, last, or two
// together.
func nativeItems(entries []workspace.MenuEntry) []nativeItem {
	out := []nativeItem{}
	sep := false
	add := func(it nativeItem) {
		if sep && len(out) > 0 {
			out = append(out, nativeItem{Separator: true})
		}
		sep = false
		out = append(out, it)
	}
	for _, e := range entries {
		switch {
		case e.Separator:
			sep = true
		case e.Status != "":
			for _, line := range lines(e.Status, maxNote) {
				add(nativeItem{Label: line})
			}
		case e.Note != "":
			for _, line := range lines(e.Note, maxNote) {
				add(nativeItem{Label: line})
			}
		case e.Heading != "":
			add(nativeItem{Label: cut(e.Heading, maxLabel)})
		case e.Label == "":
		case len(e.Submenu) > 0:
			items := nativeItems(e.Submenu)
			add(nativeItem{Label: nativeLabel(e), Enabled: !e.Disabled && anyEnabled(items), Items: items})
		default:
			it := nativeItem{Label: nativeLabel(e), Enabled: !e.Disabled && e.ID != ""}
			if it.Enabled {
				it.ID = e.ID
			}
			if e.Checked != nil {
				it.Choice, it.Checked = true, *e.Checked
			}
			add(it)
		}
	}
	return out
}

// nativeLabel returns a native item's label for an entry: its label, its
// detail in brackets, as "Reopen Closed Tab (Research)", and its keys, as
// "Split Right (⌃⌥⇧→)".
func nativeLabel(e workspace.MenuEntry) string {
	label := cut(e.Label, maxLabel)
	if e.Detail != "" {
		label += " (" + cut(e.Detail, maxDetail) + ")"
	}
	return withKeys(label, e.Keys)
}

// anyEnabled reports whether any of a menu's items is on.
func anyEnabled(items []nativeItem) bool {
	for _, it := range items {
		if it.Enabled {
			return true
		}
	}
	return false
}

// cut returns s cut to at most n characters, with an ellipsis in place of
// what's cut, and its spaces trimmed.
func cut(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// lines breaks s into lines of at most n characters, between words, for
// a note or status line. A word longer than n is cut (cut).
func lines(s string, n int) []string {
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		w = cut(w, n)
		switch {
		case line == "":
			line = w
		case len([]rune(line))+1+len([]rune(w)) <= n:
			line += " " + w
		default:
			out = append(out, line)
			line = w
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// buildNativeMenu builds Wails' menu of items, whose items, when chosen,
// call choose with their IDs. It returns too what each item reports, for
// the tests, which can't choose them.
func buildNativeMenu(items []nativeItem, choose func(id string)) (*application.Menu, map[*application.MenuItem]string) {
	ids := map[*application.MenuItem]string{}
	return addNativeItems(application.NewMenu(), items, choose, ids), ids
}

// addNativeItems adds items to menu, as buildNativeMenu does, and returns it.
func addNativeItems(menu *application.Menu, items []nativeItem, choose func(id string), ids map[*application.MenuItem]string) *application.Menu {
	for _, it := range items {
		var item *application.MenuItem
		switch {
		case it.Separator:
			menu.AddSeparator()
			continue
		case it.Items != nil:
			item = application.NewSubmenu(it.Label, addNativeItems(application.NewMenu(), it.Items, choose, ids))
			menu.Append(application.NewMenuFromItems(item))
		case it.Choice:
			item = menu.AddCheckbox(it.Label, it.Checked)
		default:
			item = menu.Add(it.Label)
		}
		item.SetEnabled(it.Enabled)
		if it.ID != "" {
			id := it.ID
			ids[item] = id
			item.OnClick(func(*application.Context) { choose(id) })
		}
	}
	return menu
}

// errNoMainWindow refuses a menu while Bench has no main window.
var errNoMainWindow = errors.New("Bench has no window to show the menu in")

// contextMenu shows Bench's menus as native menus: one at a time, in the
// context menu it registered with Wails (contextMenuName), whose items it
// builds again for each menu.
type contextMenu struct {
	// mu makes the menus wait their turn. It's held while one is open,
	// which waits for the main thread, so the main thread must never take
	// it.
	mu sync.Mutex
	// menu is the registered context menu. Its items are those of the
	// menu shown last, which it lets go of as the next one opens, by
	// when that menu's choice has long been made.
	menu *application.Menu
	// open shows the registered context menu at a point in the main
	// window's web view and returns once it has closed: in the app,
	// WebviewWindow.OpenContextMenu, which waits for the main thread
	// while AppKit runs the menu.
	open func(x, y int) error
	// wait is how long to wait for the choice once the menu has closed
	// (choiceWait; less in tests).
	wait time.Duration
	// chosen is where the open menu's choice goes, with room for one.
	chosen chan string
}

// show shows m and returns the ID of the entry the user chose, or "" if
// they closed it.
func (c *contextMenu) show(m workspace.Menu) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.menu.Destroy()
	chosen := make(chan string, 1)
	c.chosen = chosen
	built, _ := buildNativeMenu(nativeItems(m.Entries), func(id string) {
		select {
		case chosen <- id:
		default:
		}
	})
	c.menu.Append(built)
	if err := c.open(int(math.Round(m.X)), int(math.Round(m.Y))); err != nil {
		return "", err
	}
	select {
	case id := <-chosen:
		return id, nil
	case <-time.After(c.wait):
		return "", nil
	}
}
