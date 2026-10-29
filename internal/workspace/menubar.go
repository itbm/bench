package workspace

import (
	"slices"

	"github.com/itbm/bench/internal/shortcuts"
)

// This file has what the engine tells the shell about Bench's menu bar item
// and the app menu, and the tab shortcuts that bring back a hidden Bench, as
// choosing a tab from that item does.

// MenuBar is what Bench's menu bar item shows: Bench's icon, with the
// current tab's name unless the menu_bar setting says otherwise, and in its
// menu, the tabs with their shortcuts. It also has the layout mode, which
// the app menu's View menu ticks.
type MenuBar struct {
	// Show is the menu_bar setting: "name", "icon", or "hidden" (the
	// config.MenuBar constants).
	Show string
	// Current is the current tab's index in Tabs.
	Current int
	Tabs    []MenuBarTab
	// Jump is the jump shortcut as a Wails accelerator, such as
	// "Ctrl+Option+Space", or "" if it's off.
	Jump string
	// Mode is the layout mode: auto, wide, or compact.
	Mode string
}

// MenuBarTab is a tab in the menu bar item's menu.
type MenuBarTab struct {
	Name string
	// Shortcut is the tab's shortcut as a Wails accelerator, such as
	// "Ctrl+Option+1", or "" if it has none: tabs after the ninth, or all of
	// them if tab_modifiers is off.
	Shortcut string
}

// TabName returns the current tab's name, or "" if there's no current tab.
func (m MenuBar) TabName() string {
	if m.Current < 0 || m.Current >= len(m.Tabs) {
		return ""
	}
	return m.Tabs[m.Current].Name
}

// SameMenu reports whether the menu bar item's menu for m has the same items
// as for o, ticks aside.
func (m MenuBar) SameMenu(o MenuBar) bool {
	return m.Jump == o.Jump && slices.Equal(m.Tabs, o.Tabs)
}

// same reports whether the menu bar item, and the View menu's layout mode,
// show the same for m as for o.
func (m MenuBar) same(o MenuBar) bool {
	return m.Show == o.Show && m.Current == o.Current && m.Mode == o.Mode && m.SameMenu(o)
}

// menuBarLocked returns what the menu bar item and the View menu show now.
func (e *Engine) menuBarLocked() MenuBar {
	bindings, _ := shortcuts.Bindings(e.cfg.Shortcuts)
	var tabKeys [9]string
	jump := ""
	for _, b := range bindings {
		switch b.Action {
		case shortcuts.ActionTab:
			if n, err := tabNumber(b.Arg); err == nil {
				tabKeys[n-1] = b.Accelerator
			}
		case shortcuts.ActionJump:
			jump = b.Accelerator
		}
	}
	m := MenuBar{
		Show: e.cfg.Settings.MenuBar, Current: e.model.Current, Tabs: make([]MenuBarTab, len(e.model.Tabs)), Jump: jump,
		Mode: string(e.model.Mode),
	}
	for i, t := range e.model.Tabs {
		m.Tabs[i].Name = t.Name
		if i < len(tabKeys) {
			m.Tabs[i].Shortcut = tabKeys[i]
		}
	}
	return m
}

// menuBarChangedLocked returns what the menu bar item shows, and whether
// that has changed since it was last sent to the shell.
func (e *Engine) menuBarChangedLocked() (MenuBar, bool) {
	m := e.menuBarLocked()
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
