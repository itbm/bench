//go:build darwin || server

package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/internal/workspace"
)

// TestNativeItems checks how a menu's entries become a native menu's
// items: labels with their detail and keys, items that are off where the
// entry is off or has no ID, headings, notes, and status lines as items
// that are off, a long status line on two, submenus, ticks, and
// separators only between items.
func TestNativeItems(t *testing.T) {
	on, off := true, false
	entries := []workspace.MenuEntry{
		{Separator: true},
		{Status: "Waiting for the app to open its window, which can take a minute or two."},
		{Separator: true},
		{Separator: true},
		{ID: "reload-page", Label: "Reload Page"},
		{ID: "close", Label: "Close Window", Disabled: true},
		{Note: "Closing an app's window needs Accessibility permission."},
		{Separator: true},
		{ID: "split-right", Label: "Split Right", Keys: "⌃⌥⇧→"},
		{ID: "reopen-tab", Label: "Reopen Closed Tab", Detail: "Research"},
		{Label: "No ID"},
		{ID: "no-label"},
		{Heading: "Closed Windows"},
		{ID: "open-closed:0", Label: "Open Calendar"},
		{ID: "move-to-pane", Label: "Move to Pane", Submenu: []workspace.MenuEntry{
			{ID: "move-to-pane:p2", Label: "Pane with Outlook"},
		}},
		{ID: "move-to-tab", Label: "Move to Tab", Submenu: []workspace.MenuEntry{
			{ID: "move-to-tab:0", Label: "Comms", Keys: "⌃⌥1", Disabled: true},
			{ID: "move-to-tab:1", Label: "Research", Keys: "⌃⌥2"},
			{Separator: true},
			{ID: "move-to-tab:new", Label: "New Tab"},
		}},
		{ID: "also-show-in", Label: "Also Show In", Submenu: []workspace.MenuEntry{
			{ID: "also-show-in:0", Label: "Comms", Disabled: true},
		}},
		{ID: "page", Label: "Page", Disabled: true, Submenu: []workspace.MenuEntry{
			{ID: "page-back", Label: "Back"},
		}},
		{Separator: true},
		{ID: "layout:auto", Label: "Auto", Checked: &on},
		{ID: "layout:wide", Label: "Wide", Checked: &off},
		{Separator: true},
	}
	want := []nativeItem{
		{Label: "Waiting for the app to open its window, which can take a"},
		{Label: "minute or two."},
		{Separator: true},
		{ID: "reload-page", Label: "Reload Page", Enabled: true},
		{Label: "Close Window"},
		{Label: "Closing an app's window needs Accessibility permission."},
		{Separator: true},
		{ID: "split-right", Label: "Split Right (⌃⌥⇧→)", Enabled: true},
		{ID: "reopen-tab", Label: "Reopen Closed Tab (Research)", Enabled: true},
		{Label: "No ID"},
		{Label: "Closed Windows"},
		{ID: "open-closed:0", Label: "Open Calendar", Enabled: true},
		{Label: "Move to Pane", Enabled: true, Items: []nativeItem{
			{ID: "move-to-pane:p2", Label: "Pane with Outlook", Enabled: true},
		}},
		{Label: "Move to Tab", Enabled: true, Items: []nativeItem{
			{Label: "Comms (⌃⌥1)"},
			{ID: "move-to-tab:1", Label: "Research (⌃⌥2)", Enabled: true},
			{Separator: true},
			{ID: "move-to-tab:new", Label: "New Tab", Enabled: true},
		}},
		{Label: "Also Show In", Items: []nativeItem{{Label: "Comms"}}},
		{Label: "Page", Items: []nativeItem{{ID: "page-back", Label: "Back", Enabled: true}}},
		{Separator: true},
		{ID: "layout:auto", Label: "Auto", Enabled: true, Choice: true, Checked: true},
		{ID: "layout:wide", Label: "Wide", Enabled: true, Choice: true},
	}
	got := nativeItems(entries)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nativeItems:\n got %+v\nwant %+v", got, want)
	}
	if got := nativeItems(nil); len(got) != 0 {
		t.Errorf("no entries: got %+v, want no items", got)
	}
}

// TestNativeItemsCutLongText checks that a long label or detail is cut,
// by characters, not bytes, with an ellipsis, and that a long word in a
// note is cut too.
func TestNativeItemsCutLongText(t *testing.T) {
	long := strings.Repeat("é", maxLabel+5)
	detail := "  " + strings.Repeat("ü", maxDetail+1) + "  "
	word := strings.Repeat("x", maxNote+10)
	got := nativeItems([]workspace.MenuEntry{
		{ID: "a", Label: long},
		{ID: "b", Label: "Use Automatic Title", Detail: detail},
		{Note: "See " + word},
		{Heading: long},
	})
	want := []string{
		strings.Repeat("é", maxLabel-1) + "…",
		"Use Automatic Title (" + strings.Repeat("ü", maxDetail-1) + "…)",
		"See",
		strings.Repeat("x", maxNote-1) + "…",
		strings.Repeat("é", maxLabel-1) + "…",
	}
	var labels []string
	for _, it := range got {
		labels = append(labels, it.Label)
	}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("labels:\n got %q\nwant %q", labels, want)
	}
	if cut("  Comms  ", 10) != "Comms" || cut("Research tab", 9) != "Research…" {
		t.Errorf("cut: %q, %q", cut("  Comms  ", 10), cut("Research tab", 9))
	}
}

// TestBuildNativeMenu checks the Wails menu that buildNativeMenu builds:
// its items' labels, what's off, separators, submenus, and ticks, and that
// only the items that can be chosen report an ID.
func TestBuildNativeMenu(t *testing.T) {
	items := []nativeItem{
		{Label: "Loading"},
		{Separator: true},
		{ID: "reload-page", Label: "Reload Page", Enabled: true},
		{Label: "Page", Enabled: true, Items: []nativeItem{
			{Label: "Back"},
			{ID: "page-zoom-in", Label: "Zoom In", Enabled: true},
		}},
		{ID: "layout:auto", Label: "Auto", Enabled: true, Choice: true, Checked: true},
		{ID: "layout:wide", Label: "Wide", Enabled: true, Choice: true},
	}
	menu, ids := buildNativeMenu(items, func(string) {})
	type row struct {
		label                string
		enabled, sep, ticked bool
		box                  bool
		sub                  int
	}
	describe := func(m *application.Menu) []row {
		var rows []row
		for i := 0; m.ItemAt(i) != nil; i++ {
			it := m.ItemAt(i)
			r := row{label: it.Label(), enabled: it.Enabled(), sep: it.IsSeparator(), box: it.IsCheckbox(), ticked: it.Checked(), sub: -1}
			if it.IsSubmenu() {
				r.sub = len(itemsOf(it.GetSubmenu()))
			}
			rows = append(rows, r)
		}
		return rows
	}
	want := []row{
		{label: "Loading", sub: -1},
		{sep: true, enabled: true, sub: -1},
		{label: "Reload Page", enabled: true, sub: -1},
		{label: "Page", enabled: true, sub: 2},
		{label: "Auto", enabled: true, box: true, ticked: true, sub: -1},
		{label: "Wide", enabled: true, box: true, sub: -1},
	}
	got := describe(menu)
	for i := range got {
		// A separator's label and enabled state mean nothing.
		if got[i].sep {
			got[i].label, got[i].enabled = "", true
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("menu:\n got %+v\nwant %+v", got, want)
	}
	page := menu.ItemAt(3).GetSubmenu()
	if back, zoom := page.ItemAt(0), page.ItemAt(1); back.Label() != "Back" || back.Enabled() || zoom.Label() != "Zoom In" || !zoom.Enabled() {
		t.Errorf("the Page submenu: %q %v, %q %v", back.Label(), back.Enabled(), zoom.Label(), zoom.Enabled())
	}
	reported := map[string]string{}
	for item, id := range ids {
		reported[item.Label()] = id
	}
	wantIDs := map[string]string{"Reload Page": "reload-page", "Zoom In": "page-zoom-in", "Auto": "layout:auto", "Wide": "layout:wide"}
	if !reflect.DeepEqual(reported, wantIDs) {
		t.Errorf("IDs: got %v, want %v", reported, wantIDs)
	}
}

// itemsOf returns a menu's items.
func itemsOf(m *application.Menu) []*application.MenuItem {
	var items []*application.MenuItem
	for i := 0; m != nil && m.ItemAt(i) != nil; i++ {
		items = append(items, m.ItemAt(i))
	}
	return items
}

// TestContextMenuShow checks that contextMenu shows a menu at its point,
// rounded, in place of the menu shown before, and reports the item
// chosen, even when the choice comes a moment after the menu has closed;
// that a menu closed with nothing chosen reports none, as does a choice
// that comes too late, which can't reach the next menu either; and that a
// menu that can't be shown says why.
func TestContextMenuShow(t *testing.T) {
	type opened struct{ x, y int }
	var c *contextMenu
	var at []opened
	choice, late := "", time.Duration(0)
	var fail error
	c = &contextMenu{menu: application.NewMenu(), wait: 100 * time.Millisecond, open: func(x, y int) error {
		at = append(at, opened{x, y})
		if fail != nil {
			return fail
		}
		ch, id := c.chosen, choice
		switch {
		case id == "":
		case late == 0:
			ch <- id
		default:
			go func(d time.Duration) {
				time.Sleep(d)
				select {
				case ch <- id:
				default:
				}
			}(late)
		}
		return nil
	}}
	page := workspace.Menu{X: 812.4, Y: 63.6, Entries: []workspace.MenuEntry{
		{ID: "rename", Label: "Rename…"},
		{ID: "close", Label: "Close Page"},
	}}

	choice = "rename"
	if got, err := c.show(page); err != nil || got != "rename" {
		t.Errorf("chose Rename…: got %q, %v", got, err)
	}
	if len(at) != 1 || at[0] != (opened{812, 64}) {
		t.Errorf("opened at %+v, want once at 812, 64", at)
	}
	if labels := shownLabels(c.menu); !reflect.DeepEqual(labels, []string{"Rename…", "Close Page"}) {
		t.Errorf("the menu shown has %q", labels)
	}

	// A choice that comes after the menu has closed, within the wait.
	choice, late = "close", 20*time.Millisecond
	tab := workspace.Menu{X: 10, Y: 20, Entries: []workspace.MenuEntry{{ID: "close", Label: "Close Tab…"}}}
	if got, err := c.show(tab); err != nil || got != "close" {
		t.Errorf("a late choice: got %q, %v", got, err)
	}
	if labels := shownLabels(c.menu); !reflect.DeepEqual(labels, []string{"Close Tab…"}) {
		t.Errorf("the next menu has %q, want the last menu's items gone", labels)
	}

	// Closed with nothing chosen, and a choice that comes too late.
	choice, late = "", 0
	start := time.Now()
	if got, err := c.show(tab); err != nil || got != "" {
		t.Errorf("closed: got %q, %v", got, err)
	}
	if waited := time.Since(start); waited < c.wait {
		t.Errorf("closed: answered after %v, want the wait, %v", waited, c.wait)
	}
	choice, late = "close", 300*time.Millisecond
	if got, _ := c.show(tab); got != "" {
		t.Errorf("a choice too late: got %q", got)
	}
	choice, late = "", 0
	time.Sleep(300 * time.Millisecond)
	if got, _ := c.show(tab); got != "" {
		t.Errorf("the next menu got the last one's late choice, %q", got)
	}

	// The menu couldn't be shown.
	fail = errNoMainWindow
	if _, err := c.show(tab); !errors.Is(err, errNoMainWindow) {
		t.Errorf("no window: got %v", err)
	}
}

// TestContextMenuOneAtATime checks that a menu asked for while one is open
// waits until it has closed.
func TestContextMenuOneAtATime(t *testing.T) {
	release := make(chan struct{})
	opens := make(chan int, 2)
	n := 0
	c := &contextMenu{menu: application.NewMenu(), wait: time.Millisecond, open: func(x, y int) error {
		n++
		opens <- n
		if n == 1 {
			<-release
		}
		return nil
	}}
	menu := workspace.Menu{Entries: []workspace.MenuEntry{{ID: "a", Label: "A"}}}
	done := make(chan struct{}, 2)
	go func() { c.show(menu); done <- struct{}{} }()
	if <-opens != 1 {
		t.Fatal("the first menu didn't open")
	}
	go func() { c.show(menu); done <- struct{}{} }()
	select {
	case <-opens:
		t.Error("a second menu opened while the first was open")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-done
	<-done
	if n != 2 {
		t.Errorf("opened %d menus, want 2", n)
	}
}

// shownLabels returns the labels of a menu's items.
func shownLabels(m *application.Menu) []string {
	var labels []string
	for _, it := range itemsOf(m) {
		labels = append(labels, it.Label())
	}
	return labels
}
