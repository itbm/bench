package workspace

import (
	"slices"

	"github.com/itbm/bench/internal/shortcuts"
)

// This file has what the engine tells the shell about Bench's menu bar item
// and the app menu, and the tab shortcuts that bring back a hidden Bench, as
// choosing a tab from that item does.

// MenuBar is what Bench's menu bar item shows: Bench's icon, with the
// current tab's name and the unread total unless the menu_bar setting says
// otherwise, and in its menu, the tabs with their shortcuts and unread
// counts. The app menu shows it too: its Tab menu lists the tabs, its View
// menu ticks the layout mode, and its Pane menu acts on the focused pane
// (Pane).
type MenuBar struct {
	// Show is the menu_bar setting: "name", "icon", or "hidden" (the
	// config.MenuBar constants).
	Show string
	// Current is the current tab's index in Tabs, and LastTab the index of
	// the tab that was current before it, which the Tab menu's Last Used
	// Tab goes back to (View.LastTab), or -1.
	Current int
	LastTab int
	Tabs    []MenuBarTab
	// Unread is the total of the unread counts of the items in every tab,
	// each item counted once (ItemView.Unread), while the unread_counts
	// setting is on.
	Unread UnreadView
	// Jump is the jump shortcut as a Wails accelerator, such as
	// "Ctrl+Option+Space", or "" if it's off.
	Jump string
	// Mode is the layout mode: auto, wide, or compact.
	Mode string
	// Zoomed is set while a pane of the current tab is zoomed (ZoomPane),
	// when the Pane menu's Zoom Pane reads Show All Panes.
	Zoomed bool
	// Keys are the configured shortcuts in display form, which the app
	// menu shows in its items' titles.
	Keys KeysView
	// Closed names the tab that the Tab menu's Reopen Closed Tab brings
	// back: the latest that Bench keeps (View.ClosedTabs), or "".
	Closed string
	// Pane is the focused pane, for the Pane menu.
	Pane MenuBarPane
	// Arrange says what the keys that arrange the current tab's panes
	// would do now (View.Display.Arrange), for the Pane menu's Grow Pane,
	// Shrink Pane, and Make Panes Equal, and the Tab menu's Next Layout and
	// Previous Layout.
	Arrange ArrangeView
	// AddFront is set while the window in front is out of the tabs
	// (View.Front), for Pane ▸ Add Front Window, and BackToBench while one
	// of Bench's popped-out windows has the keyboard, for Window ▸ Back to
	// Bench.
	AddFront    bool
	BackToBench bool
}

// MenuBarTab is a tab in the menu bar item's menu.
type MenuBarTab struct {
	Name string
	// Shortcut is the tab's shortcut as a Wails accelerator, such as
	// "Ctrl+Option+1", or "" if it has none: tabs after the ninth, or all of
	// them if tab_modifiers is off. Keys is the same in display form, such
	// as "⌃⌥1".
	Shortcut string
	Keys     string
	// SendKeys is the send_to_tab_modifiers shortcut that sends the focused
	// pane's shown window to the tab, in display form, such as "⌃⌥⇧1", for
	// Pane ▸ Move to Tab, or "" if it has none (TabView.SendShortcut).
	SendKeys string
	// Unread is the tab's unread total (TabView.Unread).
	Unread UnreadView
}

// Label returns the tab's name as the menus list it, with its unread
// count, in title case as the menus' items are: "Comms — 15 Unread", or
// "Comms — Unread" for a dot.
func (t MenuBarTab) Label() string {
	switch {
	case t.Unread.Count > 0:
		return t.Name + " — " + t.Unread.Text() + " Unread"
	case t.Unread.Dot:
		return t.Name + " — Unread"
	}
	return t.Name
}

// MenuBarPane is what the app menu's Pane menu needs to know about the
// focused pane and the window it shows, read from the View as the frontend
// reads it.
type MenuBarPane struct {
	// ID is the focused pane as drawn: "compact" in the "tabs" view.
	ID string
	// Item is the item the pane shows, or "" if it shows none.
	Item string
	// NoWindow is set when Item has no window: Pop Out then forgets it,
	// and reads Remove from Bench.
	NoWindow bool
	// Undocked is set when Item's window has been dragged out of the pane,
	// for Dock Again.
	Undocked bool
	// Terminal is set when Item is a terminal, and Shared when it's in
	// other tabs too, for Close Window's label, which names what it does,
	// as the window's ⋯ menu does: Close Terminal, or Remove from This Tab.
	Terminal, Shared bool
	// CanSplit is set when Item can be put beside the pane: the tab shows
	// its panes (not the "tabs" view). SideBySide is set in the "tabs"
	// view instead, where Split Right is Show Side by Side, which changes
	// to the wide layout (ShowSideBySide).
	CanSplit   bool
	SideBySide bool
	// CanZoom is set when Zoom Pane can zoom the pane, or show every pane
	// again: a pane is zoomed, or the tab shows more than one.
	CanZoom bool
	// CanClose is set when the pane can close: it's empty, and not the
	// only pane of a tab that shows its panes.
	CanClose bool
	// Page is set when Item is a Web item, for Pane ▸ Page, with what its
	// view last said (ItemView): whether it can go back or forward; its
	// zoom in percent, or 0 at actual size, and whether it can zoom in or
	// out further; and how its page uses the camera and microphone,
	// "active", "muted", or "". Link is set for a web page other than VS
	// Code for the Web, which has a link to copy.
	Page                  bool
	CanGoBack             bool
	CanGoForward          bool
	Zoom                  int
	CanZoomIn, CanZoomOut bool
	Camera                string
	Microphone            string
	Link                  bool
}

// TabName returns the current tab's name, or "" if there's no current tab.
func (m MenuBar) TabName() string {
	if m.Current < 0 || m.Current >= len(m.Tabs) {
		return ""
	}
	return m.Tabs[m.Current].Name
}

// UnreadLabel says the unread total, for the line at the top of the menu
// bar item's menu that tells what the number beside the icon is, in title
// case as the menu's items are: "15 Unread in All Tabs", or "Unread in Some
// Tabs" for a dot. It returns "" while there's nothing unread, and the menu
// has no such line.
func (m MenuBar) UnreadLabel() string {
	switch {
	case m.Unread.Count > 0:
		return m.Unread.Text() + " Unread in All Tabs"
	case m.Unread.Dot:
		return "Unread in Some Tabs"
	}
	return ""
}

// SameMenu reports whether the menu bar item's menu for m has the same items
// as for o, ticks and unread counts aside, which change in place. A menu
// gains or loses the line with the unread total (UnreadLabel), though.
func (m MenuBar) SameMenu(o MenuBar) bool {
	return m.Jump == o.Jump && m.Unread.none() == o.Unread.none() && slices.EqualFunc(m.Tabs, o.Tabs, func(a, b MenuBarTab) bool {
		a.Unread, b.Unread = UnreadView{}, UnreadView{}
		return a == b
	})
}

// same reports whether the menu bar item and the app menu show the same
// for m as for o.
func (m MenuBar) same(o MenuBar) bool {
	return m.Show == o.Show && m.Current == o.Current && m.LastTab == o.LastTab && m.Mode == o.Mode && m.Zoomed == o.Zoomed &&
		m.Jump == o.Jump && slices.Equal(m.Tabs, o.Tabs) && m.Unread == o.Unread && m.Keys == o.Keys && m.Closed == o.Closed &&
		m.Pane == o.Pane && m.Arrange == o.Arrange && m.AddFront == o.AddFront && m.BackToBench == o.BackToBench
}

// menuBarLocked returns what the menu bar item and the app menu show now,
// with v, the view as built now, for the Pane menu.
func (e *Engine) menuBarLocked(v View) MenuBar {
	bindings, _ := shortcuts.Bindings(e.cfg.Shortcuts)
	tabKeys := byTab(bindings, shortcuts.ActionTab)
	sendKeys := byTab(bindings, shortcuts.ActionSendToTab)
	jump := ""
	for _, b := range bindings {
		if b.Action == shortcuts.ActionJump {
			jump = b.Accelerator
		}
	}
	m := MenuBar{
		Show: e.cfg.Settings.MenuBar, Current: e.model.Current, LastTab: v.LastTab, Tabs: make([]MenuBarTab, len(e.model.Tabs)),
		Jump: jump, Mode: string(e.model.Mode), Zoomed: e.zoomedLocked() != nil, Keys: v.Keys, Pane: menuPane(v), Arrange: v.Display.Arrange,
		AddFront: v.Front != "", BackToBench: e.keyPopOut != "" && e.model.IsFloating(e.keyPopOut),
	}
	if len(v.ClosedTabs) > 0 {
		m.Closed = v.ClosedTabs[0]
	}
	for _, it := range v.Items {
		m.Unread = m.Unread.add(it.Unread)
	}
	for i, t := range e.model.Tabs {
		m.Tabs[i].Name = t.Name
		if i < len(v.Tabs) {
			m.Tabs[i].Unread = v.Tabs[i].Unread
		}
		if i < len(tabKeys) {
			m.Tabs[i].Shortcut, m.Tabs[i].Keys = tabKeys[i].Accelerator, tabKeys[i].Display
			m.Tabs[i].SendKeys = sendKeys[i].Display
		}
	}
	return m
}

// byTab returns the bindings of an action that takes a tab number,
// ActionTab or ActionSendToTab, by the tab's index. A tab whose shortcut
// another field has, which Bindings leaves out, gets the zero Binding: its
// menu items show no shortcut, rather than one that runs something else.
func byTab(bindings []shortcuts.Binding, action string) [9]shortcuts.Binding {
	var out [9]shortcuts.Binding
	for _, b := range bindings {
		if b.Action != action {
			continue
		}
		if n, err := tabNumber(b.Arg); err == nil {
			out[n-1] = b
		}
	}
	return out
}

// menuPane returns what the Pane menu needs to know about the focused pane
// of a view, as the frontend reads it: the item its display shows there,
// and what can be done with the pane.
func menuPane(v View) MenuBarPane {
	d := v.Display
	p := MenuBarPane{ID: d.Focus}
	for _, pv := range d.Panes {
		if pv.ID == d.Focus {
			p.Item = shownIn(pv)
		}
	}
	if it, ok := v.Items[p.Item]; ok && p.Item != "" {
		p.NoWindow = it.NoWindow
		p.Undocked = it.Status == StatusUndocked
		p.Terminal = it.Kind == KindBuiltin
		p.Shared = len(it.Tabs) > 1
		if it.Kind == KindWeb {
			p.Page, p.Link = true, it.Folder == ""
			p.CanGoBack, p.CanGoForward = it.CanGoBack, it.CanGoForward
			p.Zoom, p.Camera, p.Microphone = it.Zoom, it.Camera, it.Microphone
			_, p.CanZoomIn = zoomStep(float64(it.Zoom)/100, 1)
			_, p.CanZoomOut = zoomStep(float64(it.Zoom)/100, -1)
		}
	}
	// While a pane is zoomed, the tab's other panes are out of sight, but
	// still there.
	panes := len(d.Panes)
	if d.Zoomed != "" && v.Current >= 0 && v.Current < len(v.Tabs) {
		panes = v.Tabs[v.Current].Panes
	}
	p.CanSplit = p.Item != "" && d.SplitsDisabled == ""
	p.SideBySide = p.Item != "" && d.SplitsDisabled != ""
	p.CanZoom = d.Zoomed != "" || (!d.Compact && panes > 1)
	p.CanClose = d.Focus != "" && p.Item == "" && !d.Compact && panes > 1
	return p
}

// shownIn returns the item a displayed pane shows: its active item, or its
// first if it has none.
func shownIn(p PaneView) string {
	if p.Active != "" && slices.Contains(p.Items, p.Active) {
		return p.Active
	}
	if len(p.Items) > 0 {
		return p.Items[0]
	}
	return ""
}

// menuBarChangedLocked returns what the menu bar item and the app menu
// show, with v, the view as built now, and whether that has changed since
// it was last sent to the shell.
func (e *Engine) menuBarChangedLocked(v View) (MenuBar, bool) {
	m := e.menuBarLocked(v)
	if e.menuBarSent && m.same(e.lastMenuBar) {
		return m, false
	}
	e.lastMenuBar, e.menuBarSent = m, true
	return m, true
}

// showOnTabShortcutLocked reports whether a tab shortcut should bring Bench
// forward, on the tab it chooses: when show_on_tab_shortcut is on and
// Bench's window is out of sight. That's when it's hidden, minimised, or
// closed (decision 16), or when Bench is hidden by another app's Hide
// Others, which decision 16 doesn't count.
func (e *Engine) showOnTabShortcutLocked() bool {
	return e.cfg.Settings.ShowOnTabShortcut && (e.benchHidden || e.p.OwnAppHidden())
}
