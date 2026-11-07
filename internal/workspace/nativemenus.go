package workspace

import "errors"

// This file has ShowMenu, which shows Bench's own menus, such as a tab's or
// a window's ⋯ menu, as native macOS menus in the app, without raised mode
// (decision 71). The frontend builds each menu's entries (menu.js), the
// same ones that its HTML menus and the jump palette's This window show,
// and sends them here as data, with an ID for each entry; the shell shows
// them (Options.ShowMenu), and the frontend runs the entry whose ID comes
// back.

// Menu is one of Bench's menus, for ShowMenu: its entries, and where its
// top-left corner goes, in the points of the UI's web view, from its
// top-left, as the page's clientX and clientY count them: at the pointer
// for a right-click, or under the control that opened it.
type Menu struct {
	Entries []MenuEntry `json:"entries"`
	X       float64     `json:"x"`
	Y       float64     `json:"y"`
}

// MenuEntry is one line of a Menu, one of:
//   - an entry that the user can choose, with its ID, unless it's Disabled.
//     Keys is the global shortcut that does the same, which its label shows
//     after it, as the app menu's items show theirs ("Split Right
//     (⌃⌥⇧→)"); Detail says more, such as the tab that Reopen Closed Tab
//     opens again; Checked, when set, makes it one of a choice, ticked if
//     true, as View ▸ Layout's items are; and Submenu, when it has one, holds
//     the entries it opens, such as Move to Tab's tabs.
//   - a separator (Separator).
//   - a heading (Heading), such as "Closed Windows", over the entries after
//     it.
//   - a note (Note), which says why the entries before it are off, such as
//     "Closing an app's window needs Accessibility permission."
//   - a status line (Status), which says why a window isn't ready.
//
// A native menu has no headings, notes, or status lines of its own, so they
// show as entries that are off.
type MenuEntry struct {
	ID        string      `json:"id,omitempty"`
	Label     string      `json:"label,omitempty"`
	Keys      string      `json:"keys,omitempty"`
	Detail    string      `json:"detail,omitempty"`
	Disabled  bool        `json:"disabled,omitempty"`
	Checked   *bool       `json:"checked,omitempty"`
	Separator bool        `json:"sep,omitempty"`
	Heading   string      `json:"heading,omitempty"`
	Note      string      `json:"note,omitempty"`
	Status    string      `json:"status,omitempty"`
	Submenu   []MenuEntry `json:"submenu,omitempty"`
}

// maxMenuEntries is the most lines, submenus' included, that ShowMenu
// shows. Bench's largest menu, a web page's ⋯ menu with many tabs, has a
// few dozen; more is a mistake.
const maxMenuEntries = 1000

var (
	// errNoNativeMenus refuses ShowMenu where Bench has no native menus,
	// as in the browser preview, whose frontend draws its menus itself.
	errNoNativeMenus = errors.New("This build of Bench draws its menus itself.")
	// errMenuTooLong refuses a menu with more than maxMenuEntries lines.
	errMenuTooLong = errors.New("That menu is too long to show.")
	// errCantShowMenu says that the shell couldn't show a menu.
	errCantShowMenu = errors.New("Bench couldn't show the menu. Try again.")
)

// ShowMenu shows one of Bench's menus as a native macOS menu, where the menu
// says, without raising Bench, so that docked windows and web pages stay
// in view (decision 71). It waits until the user closes the menu, and
// returns the ID of the entry they chose, or "" if they chose none. Only
// an entry that's on and has an ID counts. The lock isn't held meanwhile,
// but the menu counts as open (openMenuLocked), as a raised Bench does for
// an HTML menu. Where there are no native menus (Options.ShowMenu), as in
// the browser preview, it refuses: the frontend draws its menus there.
func (e *Engine) ShowMenu(menu Menu) (string, error) {
	show := e.opts.ShowMenu
	if show == nil {
		return "", errNoNativeMenus
	}
	n := countEntries(menu.Entries)
	if n == 0 {
		return "", nil
	}
	if n > maxMenuEntries {
		return "", errMenuTooLong
	}
	e.lock()
	e.openMenuLocked()
	e.unlock()
	id, err := show(menu)
	e.lock()
	e.closeMenuLocked()
	e.unlock()
	if err != nil {
		e.log.Warn("couldn't show a menu", "error", err)
		return "", errCantShowMenu
	}
	if id == "" || !offers(menu.Entries, id) {
		return "", nil
	}
	e.log.Debug("menu: chosen", "entry", id)
	return id, nil
}

// openMenuLocked notes that a native menu is open. A native menu doesn't
// raise Bench, but it closes as soon as another app becomes active, so
// nothing may focus another app's window meanwhile, as nothing may while
// Bench is raised (menuOrDialogLocked). A switch's put-off focus is
// dropped, as other focus drops it (newFocusLocked): the user has turned
// to the menu. The App window that the jump palette took the keyboard
// from gets it back only once the menu closes (closeMenuLocked).
func (e *Engine) openMenuLocked() {
	e.menus++
	if e.wait != nil {
		e.wait.focus = false
	}
	if e.jumpTimer != nil {
		e.jumpTimer.Stop()
		e.jumpTimer = nil
	}
}

// closeMenuLocked notes that a native menu has closed. Once none is open,
// and Bench isn't raised, the App window that had the keyboard when the jump
// shortcut opened the palette gets it back, if nothing has taken the focus
// since, as when Bench is lowered (setOverlayLocked): the menu may have
// come from the palette's Window Options….
func (e *Engine) closeMenuLocked() {
	e.menus--
	if e.menuOrDialogLocked() || e.jump.itemID == "" {
		return
	}
	if e.focusSeq.Load() != e.jump.seq {
		e.jump = jumpNote{}
		return
	}
	e.refocusLaterLocked()
}

// menuOrDialogLocked reports whether one of Bench's menus or dialogs is
// open: Bench is raised for one (overlay), or a native menu is open
// (menus). Nothing that would make another app active, such as focusing
// its window, may happen then.
func (e *Engine) menuOrDialogLocked() bool {
	return e.overlay || e.menus > 0
}

// countEntries counts a menu's lines, with its submenus'.
func countEntries(entries []MenuEntry) int {
	n := len(entries)
	for _, m := range entries {
		n += countEntries(m.Submenu)
	}
	return n
}

// offers reports whether a menu has an entry with an ID that the user could
// choose: one that's on, has no submenu, and is in no submenu that's off.
func offers(entries []MenuEntry, id string) bool {
	for _, m := range entries {
		if m.Disabled || m.Separator || m.Heading != "" || m.Note != "" || m.Status != "" {
			continue
		}
		if len(m.Submenu) > 0 {
			if offers(m.Submenu, id) {
				return true
			}
			continue
		}
		if m.ID == id {
			return true
		}
	}
	return false
}
