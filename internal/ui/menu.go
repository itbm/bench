//go:build darwin || server

package ui

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/workspace"
)

// appName is Bench's name in the app menu and the about box.
const appName = "Bench"

// acknowledgementsLabel is the Acknowledgements item's label in the app menu,
// and its window's title.
const acknowledgementsLabel = "Acknowledgements"

// reloadConfigLabel is the label of the app menu's item that reads
// config.toml again.
const reloadConfigLabel = "Reload config.toml"

// The Help menu's pages: the user guide, and its Troubleshooting section,
// on GitHub, which Bench Help and Troubleshooting open in the default
// browser.
const (
	helpURL            = "https://github.com/itbm/bench/blob/main/docs/user-guide.md"
	troubleshootingURL = helpURL + "#troubleshooting"
)

// menuActions are what Bench's own menu items do. Most call the engine, or
// ask the frontend, through act and command.
type menuActions struct {
	// acknowledgements opens the Acknowledgements window, or brings it
	// forward.
	acknowledgements func()
	// settings opens the Settings dialog, for the settings that apply to
	// every tab.
	settings func()
	// command sends the frontend a bench:command, for what the frontend
	// does: open a dialog, ask before closing, or rename a tab in place.
	command func(c workspace.Command)
	// act calls the engine, and shows its error, if any, as a toast.
	act func(f func(e engine) error)
	// fillScreen zooms Bench's window to fill the screen, unless it fills
	// it already.
	fillScreen func()
	// openLink opens a page in the default browser, for Help.
	openLink func(address string)
	// reportProblem opens a new GitHub issue in the default browser,
	// filled in for the user to check and send.
	reportProblem func()
}

// layoutChoices are the View ▸ Layout menu's items, with the mode each
// sets, in the order of the title bar's Auto, Wide, and Compact.
var layoutChoices = []struct {
	mode  layout.Mode
	label string
}{
	{layout.ModeAuto, "Auto"},
	{layout.ModeWide, "Wide"},
	{layout.ModeCompact, "Compact"},
}

// layoutItems are the View ▸ Layout menu's items, by the mode each sets.
type layoutItems map[string]*application.MenuItem

// Labels of Close Tab…, which reads Close All Windows… while there's only
// one tab, as closing it leaves the tab, empty; and of Close Window, which
// names what the focused pane shows, as its ⋯ menu does: Close Page, Close
// Terminal, or, for a window that's in other tabs too, Remove from This
// Tab, which is what it does then.
const (
	closeTabLabel      = "Close Tab…"
	closeAllLabel      = "Close All Windows…"
	closeItemLabel     = "Close Window"
	closePageLabel     = "Close Page"
	closeTerminalLabel = "Close Terminal"
	removeFromTabLabel = "Remove from This Tab"
)

// closeItemLabelFor returns Close Window's label for the focused pane's
// window, p.
func closeItemLabelFor(p workspace.MenuBarPane) string {
	switch {
	case p.Shared:
		return removeFromTabLabel
	case p.Page:
		return closePageLabel
	case p.Terminal:
		return closeTerminalLabel
	}
	return closeItemLabel
}

// closeTabLabelFor returns Close Tab…'s label with tabs tabs.
func closeTabLabelFor(tabs int) string {
	if tabs == 1 {
		return closeAllLabel
	}
	return closeTabLabel
}

// Labels of the Tab menu's items that move the current tab, and of the one
// that goes back to the tab used before it.
const (
	moveTabLeftLabel  = "Move Tab Left"
	moveTabRightLabel = "Move Tab Right"
	lastTabLabel      = "Last Used Tab"
)

// showAllTabsLabel is the label of View ▸ Show All Tabs, which opens the
// overview of every tab, before its shortcut.
const showAllTabsLabel = "Show All Tabs"

// Labels of the Tab menu's Undo, which names the latest change, as "Undo
// Close Tab", and of Reopen Closed Page or Terminal, which names what it
// opens again: Reopen Closed Page, or Reopen Closed Terminal.
const (
	undoLabel            = "Undo"
	reopenItemLabel      = "Reopen Closed Page or Terminal"
	reopenPageLabel      = "Reopen Closed Page"
	reopenTerminalLabel  = "Reopen Closed Terminal"
	reopenClosedTabLabel = "Reopen Closed Tab"
)

// undoLabelFor returns the Tab menu's Undo label for the latest change's
// name: "Undo Close Tab", or "Undo" when there's none.
func undoLabelFor(name string) string {
	if name == "" {
		return undoLabel
	}
	return undoLabel + " " + name
}

// reopenItemLabelFor returns Reopen Closed Page or Terminal's label for m.
func reopenItemLabelFor(m workspace.MenuBar) string {
	switch {
	case m.ClosedItem == "":
		return reopenItemLabel
	case m.ClosedTerminal:
		return reopenTerminalLabel
	}
	return reopenPageLabel
}

// sideBySideLabel is Split Right's label in the "tabs" view, which hides
// the tab's splits: it changes to the wide layout to show the window side
// by side (ShowSideBySide).
const sideBySideLabel = "Show Side by Side (Wide Layout)"

// Labels of Zoom Pane, which reads Show All Panes while a pane is zoomed.
const (
	zoomPaneLabel     = "Zoom Pane"
	showAllPanesLabel = "Show All Panes"
)

// Labels of the items for the keys that arrange the current tab's panes:
// Pane ▸ Grow Pane, Shrink Pane, and Make Panes Equal, and Tab ▸ Next
// Layout and Previous Layout (decision 59).
const (
	growPaneLabel       = "Grow Pane"
	shrinkPaneLabel     = "Shrink Pane"
	equalPanesLabel     = "Make Panes Equal"
	nextLayoutLabel     = "Next Layout"
	previousLayoutLabel = "Previous Layout"
)

// Labels of Pane ▸ Add Front Window, which adds the window in front, if it's
// out of the tabs, to the focused pane, and of Window ▸ Back to Bench, which
// puts the popped-out web page or terminal that has the keyboard back in
// Bench.
const (
	addFrontWindowLabel = "Add Front Window"
	backToBenchLabel    = "Back to Bench"
)

// Labels of Pane ▸ Page's items that read otherwise while the page's
// microphone is muted, or its camera turned off: Unmute Microphone and Turn
// On Camera.
const (
	muteMicLabel    = "Mute Microphone"
	unmuteMicLabel  = "Unmute Microphone"
	cameraOffLabel  = "Turn Off Camera"
	cameraOnLabel   = "Turn On Camera"
	actualSizeLabel = "Actual Size"
)

// keepRunningLabel is Pane ▸ Page's checkbox for whether the page keeps
// running in the background, as the ⋯ menu names it (pages.js).
const keepRunningLabel = "Keep Running in the Background"

// Labels of Pop Out of Bench, which reads Remove from Bench for a window
// that has closed, as there's nothing to pop out.
const (
	popOutLabel     = "Pop Out of Bench"
	removeItemLabel = "Remove from Bench"
)

// withKeys adds a global shortcut to an item's label, as Bench's tooltips
// show them: "Next Tab (⌃⌥])". The app menu shows Bench's global shortcuts
// this way rather than as its items' key equivalents, which would also run
// while the global shortcuts are released, as Settings ▸ Keyboard records
// one, and would take those keys before a web page could (decision 46).
func withKeys(label, keys string) string {
	if keys == "" {
		return label
	}
	return label + " (" + keys + ")"
}

// menuItems are the app menu's items that change as the engine says what's
// what (appMenu.apply).
type menuItems struct {
	// The View menu: Show All Tabs, Show Drop-down, which reads Hide
	// Drop-down while it shows, and Rearrange, with their shortcuts, and
	// View ▸ Layout's items, ticked for the layout mode.
	showAllTabs *application.MenuItem
	dropdown    *application.MenuItem
	layouts     layoutItems
	rearrange   *application.MenuItem
	// The Tab menu: Undo, first, which names the latest change, and is off
	// while there's none; Next Layout and Previous Layout, which are off
	// where the tab has no other layout to go to; Move Tab Left and Right,
	// which are off at the ends; Next and Previous Tab, off with one tab;
	// Last Used Tab, off until there's one to go back to; Close Tab…,
	// labelled for the number of tabs; Delete Tab (Keep Windows), off with
	// one tab; Reopen Closed Tab, off with none to reopen, and Reopen Closed
	// Page or Terminal, which names what it opens again; and the tabs, the
	// current one ticked.
	undo                       *application.MenuItem
	nextLayout, previousLayout *application.MenuItem
	moveLeft, moveRight        *application.MenuItem
	next, previous             *application.MenuItem
	lastTab                    *application.MenuItem
	closeTab, deleteTab        *application.MenuItem
	reopen, reopenItem         *application.MenuItem
	tabs                       []*application.MenuItem
	// The Pane menu, for the focused pane: each is off where it would do
	// nothing. Zoom Pane reads Show All Panes while a pane is zoomed, Pop
	// Out of Bench reads Remove from Bench for a window that has closed,
	// and Close Window names what the pane shows. Move to Tab lists every
	// tab, the current one off.
	jump, split, splitDown, zoom *application.MenuItem
	grow, shrink, equal          *application.MenuItem
	moveTo                       *application.MenuItem
	moveToTabs                   []*application.MenuItem
	moveToNew                    *application.MenuItem
	popOut, dockAgain            *application.MenuItem
	closeWindow, closePane       *application.MenuItem
	// Pane ▸ Add Front Window, on while the window in front is out of the
	// tabs, and Window ▸ Back to Bench, on while a popped-out web page or
	// terminal has the keyboard.
	addFront, backToBench *application.MenuItem
	// Pane ▸ Page, on for a Web item, and its items: each is off where it
	// would do nothing. Actual Size shows the page's zoom, and the camera
	// and microphone items read Turn On Camera and Unmute Microphone while
	// they're off. Keep Running in the Background is a checkbox, ticked
	// while it's on, and off for VS Code for the Web.
	page                          *application.MenuItem
	reload, back, forward         *application.MenuItem
	zoomIn, zoomOut, actualSize   *application.MenuItem
	find, copyLink                *application.MenuItem
	microphone, camera, stopSound *application.MenuItem
	keepRunning                   *application.MenuItem
}

// appMenu is the app menu: Bench, Edit, View, Tab, Pane, Window, and Help.
// It follows what the engine says (set): its Tab menu lists the tabs, its
// items' labels show the global shortcuts, and its Pane menu acts on the
// focused pane.
//
// Only the standard macOS shortcuts are bound (Hide, Hide Others, Quit, the
// Edit commands, and Minimise). Command shortcuts that pages rely on, such
// as ⌘W, ⌘P, ⌘N, ⌘T, ⌘F, ⌘L, and ⌘, (VS Code's settings), are left for Web
// items, so Bench's own items have none, and show Bench's global shortcuts
// in their labels instead (withKeys).
type appMenu struct {
	root *application.Menu
	do   menuActions

	// mu guards what follows. Updating the menu waits for the main thread
	// once the app runs, so the main thread must never take it.
	mu sync.Mutex
	// bar is what the engine last said, and built what the items were
	// built for: the menu is built again when the number of tabs or their
	// colours change, and otherwise changed in place.
	bar   workspace.MenuBar
	built workspace.MenuBar
	items menuItems
	// started is set once the app runs (start). Until then, the menu
	// isn't changed, as Wails reads it to make the native menu.
	started bool
	// clicks are what the items do, for tests, which can't click them.
	clicks map[*application.MenuItem]func()
	// dots are the colours of the dots of the Tab menu's tabs and Pane ▸
	// Move to Tab's, by item, for tests, which can't see an item's image:
	// "" for a tab with no colour, which has a blank image while another
	// tab has a colour.
	dots map[*application.MenuItem]string

	// latest is what the engine last said, for the items to act on when
	// chosen, without mu, which a rebuild may hold.
	latest atomic.Pointer[workspace.MenuBar]
}

// newAppMenu builds the app menu, before the engine has said what's what.
// It must be called after application.New, because the standard items take
// the app's name from it.
func newAppMenu(do menuActions) *appMenu {
	a := &appMenu{root: application.NewMenu(), do: do}
	a.latest.Store(&workspace.MenuBar{})
	a.mu.Lock()
	defer a.mu.Unlock()
	a.buildLocked(a.bar)
	return a
}

// set brings the menu up to date with what the engine says: once the app
// runs, it builds the menu again if the number of tabs, or their colours,
// have changed, and otherwise changes its items in place. A tab's dot is
// an image, which Wails gives the native item, on the main thread, only as
// it builds the menu: it would change an item's image later from this
// thread.
func (a *appMenu) set(m workspace.MenuBar) {
	a.latest.Store(&m)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bar = m
	if !a.started {
		return
	}
	if len(m.Tabs) != len(a.built.Tabs) || !sameColours(m.Tabs, a.built.Tabs) {
		a.buildLocked(m)
		a.root.Update()
		return
	}
	a.applyLocked(m)
}

// start builds the menu for what the engine has said, once the app runs.
// Call it off the main thread, as it waits for it.
func (a *appMenu) start() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.started = true
	a.buildLocked(a.bar)
	a.root.Update()
}

// retick ticks the item of the layout mode the engine last said again,
// after Wails has ticked a layout item, or a tab, that the engine refused.
func (a *appMenu) retick() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		a.applyLocked(a.bar)
	}
}

// now returns what the engine last said, for an item to act on.
func (a *appMenu) now() workspace.MenuBar { return *a.latest.Load() }

// add adds an item to a menu that runs run when chosen.
func (a *appMenu) add(menu *application.Menu, label string, run func()) *application.MenuItem {
	return a.bind(menu.Add(label), run)
}

// bind has an item run run when chosen.
func (a *appMenu) bind(item *application.MenuItem, run func()) *application.MenuItem {
	a.clicks[item] = run
	return item.OnClick(func(*application.Context) { run() })
}

// command returns what sends the frontend a bench:command.
func (a *appMenu) command(name string) func() {
	return func() { a.do.command(workspace.Command{Name: name}) }
}

// act returns what calls the engine with what it last said.
func (a *appMenu) act(f func(e engine, m workspace.MenuBar) error) func() {
	return func() {
		m := a.now()
		a.do.act(func(e engine) error { return f(e, m) })
	}
}

// buildLocked builds the menu's items for m, in place of those it had,
// which it lets go of.
func (a *appMenu) buildLocked(m workspace.MenuBar) {
	for i := 0; ; i++ {
		item := a.root.ItemAt(i)
		if item == nil {
			break
		}
		item.Destroy()
	}
	a.root.Clear()
	a.clicks = map[*application.MenuItem]func(){}
	a.dots = map[*application.MenuItem]string{}
	a.items = menuItems{}
	a.built = m
	a.addBenchMenu()
	a.addEditMenu()
	a.addViewMenu()
	a.addTabMenu(m)
	a.addPaneMenu(m)
	window := application.NewMenu()
	addRole(window, application.Minimise, "Minimise")
	window.AddSeparator()
	a.items.backToBench = a.add(window, backToBenchLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.BackToBench("") }))
	a.root.Append(application.NewMenuFromItems(
		application.NewSubmenu("Window", window).SetRole(application.WindowMenu)))
	a.addHelpMenu()
	a.applyLocked(m)
}

// addBenchMenu adds the app's own menu: About, Check for Updates…, which
// has the updater look for a new version, or without one opens Bench's
// latest release on GitHub, Acknowledgements, Settings…, Reload
// config.toml, Hide, and Quit.
func (a *appMenu) addBenchMenu() {
	bench := a.root.AddSubmenu(appName)
	bench.AddRole(application.About)
	a.add(bench, "Check for Updates…", a.act(func(e engine, _ workspace.MenuBar) error { return e.CheckForUpdates() }))
	a.add(bench, acknowledgementsLabel, func() { a.do.acknowledgements() })
	bench.AddSeparator()
	a.add(bench, "Settings…", func() { a.do.settings() })
	a.add(bench, reloadConfigLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.ReloadConfig() }))
	bench.AddSeparator()
	bench.AddRole(application.Hide)
	bench.AddRole(application.HideOthers)
	bench.AddRole(application.ShowAll)
	bench.AddSeparator()
	addRole(bench, application.Quit, "Quit "+appName)
}

// addEditMenu adds the Edit menu, with the standard items.
func (a *appMenu) addEditMenu() {
	edit := a.root.AddSubmenu("Edit")
	edit.AddRole(application.Undo)
	edit.AddRole(application.Redo)
	edit.AddSeparator()
	edit.AddRole(application.Cut)
	edit.AddRole(application.Copy)
	edit.AddRole(application.Paste)
	edit.AddRole(application.SelectAll)
}

// addViewMenu adds the View menu: Show All Tabs, which opens the overview
// of every tab; Show Drop-down, which shows the drop-down terminal over
// any app, or hides it (decision 72); Layout ▸ Auto, Wide, and Compact, a
// group of radio items, which Wails ticks one of when it's chosen;
// Rearrange; and Fill the Screen.
func (a *appMenu) addViewMenu() {
	view := a.root.AddSubmenu("View")
	a.items.showAllTabs = a.add(view, showAllTabsLabel, a.command(showAllTabsCommand.Name))
	a.items.dropdown = a.add(view, workspace.MenuBarDropdown{}.Label(), a.act(func(e engine, _ workspace.MenuBar) error { return e.ToggleDropdown() }))
	view.AddSeparator()
	layouts := view.AddSubmenu("Layout")
	a.items.layouts = layoutItems{}
	for _, c := range layoutChoices {
		mode := string(c.mode)
		a.items.layouts[mode] = a.bind(layouts.AddRadio(c.label, false), func() {
			a.do.act(func(e engine) error {
				err := e.SetMode(mode)
				if err != nil {
					a.retick()
				}
				return err
			})
		})
	}
	a.items.rearrange = a.add(view, "Rearrange", a.act(func(e engine, _ workspace.MenuBar) error { return e.Rearrange() }))
	view.AddSeparator()
	a.add(view, "Fill the Screen", func() { a.do.fillScreen() })
}

// addTabMenu adds the Tab menu, for the current tab: undoing the latest
// change, adding a tab, its properties and name, the next and previous
// layouts, moving it, the next and previous tabs and the one used last,
// closing, deleting, or reopening one, reopening a page or terminal, and
// the tabs, with their shortcuts, to switch to.
func (a *appMenu) addTabMenu(m workspace.MenuBar) {
	tab := a.root.AddSubmenu("Tab")
	// Edit ▸ Undo stays the page's or the terminal's, as Command-Z is.
	a.items.undo = a.add(tab, undoLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.UndoLatest() }))
	tab.AddSeparator()
	a.add(tab, "New Tab…", a.command(newTabCommand.Name))
	a.add(tab, "New Tab from Open Windows…", a.command(newTabFromWindowsCommand.Name))
	tab.AddSeparator()
	a.add(tab, "Tab Properties…", a.command(tabPropertiesCommand.Name))
	a.add(tab, "Rename…", a.command(renameTabCommand.Name))
	tab.AddSeparator()
	a.items.nextLayout = a.add(tab, nextLayoutLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.StepLayout(1) }))
	a.items.previousLayout = a.add(tab, previousLayoutLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.StepLayout(-1) }))
	tab.AddSeparator()
	a.items.moveLeft = a.add(tab, moveTabLeftLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.MoveCurrentTab(-1) }))
	a.items.moveRight = a.add(tab, moveTabRightLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.MoveCurrentTab(1) }))
	tab.AddSeparator()
	a.items.next = a.add(tab, "Next Tab", a.act(func(e engine, _ workspace.MenuBar) error { return e.StepTab(1) }))
	a.items.previous = a.add(tab, "Previous Tab", a.act(func(e engine, _ workspace.MenuBar) error { return e.StepTab(-1) }))
	a.items.lastTab = a.add(tab, lastTabLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.LastTab() }))
	tab.AddSeparator()
	a.items.closeTab = a.add(tab, closeTabLabel, a.command(closeTabCommand.Name))
	a.items.deleteTab = a.add(tab, "Delete Tab (Keep Windows)", a.act(func(e engine, m workspace.MenuBar) error {
		return e.DeleteTab(m.Current)
	}))
	a.items.reopen = a.add(tab, reopenClosedTabLabel, a.act(func(e engine, m workspace.MenuBar) error {
		if m.Closed == "" {
			return nil
		}
		return e.ReopenTab(m.Closed)
	}))
	a.items.reopenItem = a.add(tab, reopenItemLabel, a.act(func(e engine, m workspace.MenuBar) error {
		if m.ClosedItem == "" {
			return nil
		}
		return e.ReopenItem(m.ClosedItem)
	}))
	if len(m.Tabs) > 0 {
		tab.AddSeparator()
	}
	// While any tab has a colour, each has a dot of its colour before its
	// name, and a tab with none a blank image as wide, so that the names
	// line up (decision 74).
	coloured := slices.ContainsFunc(m.Tabs, func(t workspace.MenuBarTab) bool { return t.Colour != "" })
	for i := range m.Tabs {
		item := a.bind(tab.AddRadio("", false), func() {
			a.do.act(func(e engine) error {
				err := e.SwitchTab(i)
				if err != nil {
					a.retick()
				}
				return err
			})
		})
		if coloured {
			item.SetBitmap(menuDot(m.Tabs[i].Colour))
			a.dots[item] = m.Tabs[i].Colour
		}
		a.items.tabs = append(a.items.tabs, item)
	}
}

// sameColours reports whether two lists of tabs, as long as each other,
// have the same colours.
func sameColours(a, b []workspace.MenuBarTab) bool {
	return slices.EqualFunc(a, b, func(x, y workspace.MenuBarTab) bool { return x.Colour == y.Colour })
}

// addPaneMenu adds the Pane menu, for the focused pane and the window it
// shows: the jump palette, the add dialog, and adding the window in front;
// putting the window beside the pane, or zooming the pane; growing or
// shrinking the pane, and making the tab's panes equal; moving the window
// to another tab, out of Bench, or back into its pane; and closing the
// window, or the pane.
func (a *appMenu) addPaneMenu(m workspace.MenuBar) {
	pane := a.root.AddSubmenu("Pane")
	it := &a.items
	it.jump = a.add(pane, jumpLabel, a.command(paletteCommand.Name))
	a.add(pane, "Add to Pane…", a.command(addToPaneCommand.Name))
	it.addFront = a.add(pane, addFrontWindowLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.AddFrontWindow() }))
	pane.AddSeparator()
	it.split = a.add(pane, "Split Right", a.act(func(e engine, m workspace.MenuBar) error {
		if m.Pane.SideBySide {
			return e.ShowSideBySide(m.Pane.Item)
		}
		return e.MoveItem(m.Pane.Item, m.Current, m.Pane.ID, "right")
	}))
	it.splitDown = a.add(pane, "Split Down", a.act(func(e engine, m workspace.MenuBar) error {
		return e.MoveItem(m.Pane.Item, m.Current, m.Pane.ID, "bottom")
	}))
	it.zoom = a.add(pane, zoomPaneLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.ToggleZoom() }))
	pane.AddSeparator()
	it.grow = a.add(pane, growPaneLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.GrowPane() }))
	it.shrink = a.add(pane, shrinkPaneLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.ShrinkPane() }))
	it.equal = a.add(pane, equalPanesLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.EqualPanes() }))
	pane.AddSeparator()
	// Move to Tab's tabs have their dots as the Tab menu's do, as the ⋯
	// menu's Move to Tab has them (decision 74).
	moveTo := application.NewMenu()
	coloured := slices.ContainsFunc(m.Tabs, func(t workspace.MenuBarTab) bool { return t.Colour != "" })
	for i := range m.Tabs {
		item := a.add(moveTo, "", a.act(func(e engine, m workspace.MenuBar) error {
			return e.MoveItemToTab(m.Pane.Item, i)
		}))
		if coloured {
			item.SetBitmap(menuDot(m.Tabs[i].Colour))
			a.dots[item] = m.Tabs[i].Colour
		}
		it.moveToTabs = append(it.moveToTabs, item)
	}
	if len(m.Tabs) > 0 {
		moveTo.AddSeparator()
	}
	it.moveToNew = a.add(moveTo, "New Tab", a.act(func(e engine, m workspace.MenuBar) error {
		return e.MoveItemToNewTab(m.Pane.Item, false)
	}))
	it.moveTo = application.NewSubmenu("Move to Tab", moveTo)
	pane.Append(application.NewMenuFromItems(it.moveTo))
	it.popOut = a.add(pane, popOutLabel, a.act(func(e engine, m workspace.MenuBar) error { return e.PopOut(m.Pane.Item) }))
	it.dockAgain = a.add(pane, "Dock Again", a.act(func(e engine, m workspace.MenuBar) error { return e.Redock(m.Pane.Item) }))
	pane.AddSeparator()
	a.addPageMenu(pane)
	pane.AddSeparator()
	it.closeWindow = a.add(pane, closeItemLabel, a.command(closeWindowCommand.Name))
	it.closePane = a.add(pane, "Close Pane", a.act(func(e engine, m workspace.MenuBar) error { return e.ClosePane(m.Pane.ID) }))
}

// addPageMenu adds Pane ▸ Page, for the web page that the focused pane
// shows, as its ⋯ menu's Page submenu does: reloading it, going back or
// forward, zooming it, finding in it, copying its link, what it does with
// the camera, the microphone, and sound, and whether it keeps running in
// the background (decision 76). None has a key equivalent, so that pages
// keep their own ⌘R, ⌘F, ⌘+, and the like (decision 50).
func (a *appMenu) addPageMenu(pane *application.Menu) {
	it := &a.items
	page := application.NewMenu()
	it.reload = a.add(page, "Reload Page", a.act(func(e engine, m workspace.MenuBar) error { return e.ReloadPage(m.Pane.Item) }))
	it.back = a.add(page, "Back", a.act(func(e engine, m workspace.MenuBar) error { return e.GoBack(m.Pane.Item) }))
	it.forward = a.add(page, "Forward", a.act(func(e engine, m workspace.MenuBar) error { return e.GoForward(m.Pane.Item) }))
	page.AddSeparator()
	it.zoomIn = a.add(page, "Zoom In", a.act(func(e engine, m workspace.MenuBar) error { return e.ZoomPage(m.Pane.Item, 1) }))
	it.zoomOut = a.add(page, "Zoom Out", a.act(func(e engine, m workspace.MenuBar) error { return e.ZoomPage(m.Pane.Item, -1) }))
	it.actualSize = a.add(page, actualSizeLabel, a.act(func(e engine, m workspace.MenuBar) error { return e.ZoomPage(m.Pane.Item, 0) }))
	page.AddSeparator()
	it.find = a.add(page, "Find in Page…", a.itemCommand(findInPageCommand.Name))
	it.copyLink = a.add(page, "Copy Page Link", a.itemCommand(copyPageLinkCommand.Name))
	page.AddSeparator()
	it.microphone = a.add(page, muteMicLabel, a.act(func(e engine, m workspace.MenuBar) error {
		return e.MuteCapture(m.Pane.Item, "microphone", m.Pane.Microphone != "muted")
	}))
	it.camera = a.add(page, cameraOffLabel, a.act(func(e engine, m workspace.MenuBar) error {
		return e.MuteCapture(m.Pane.Item, "camera", m.Pane.Camera != "muted")
	}))
	it.stopSound = a.add(page, "Stop Sound and Video", a.act(func(e engine, m workspace.MenuBar) error { return e.StopMedia(m.Pane.Item) }))
	page.AddSeparator()
	// Wails ticks or unticks a checkbox as it's chosen, so the tick goes
	// back to what the engine says if it refuses, as for the layouts.
	it.keepRunning = a.bind(page.AddCheckbox(keepRunningLabel, false), func() {
		m := a.now()
		a.do.act(func(e engine) error {
			err := e.SetKeepRunning(m.Pane.Item, !m.Pane.KeepRunning)
			if err != nil {
				a.retick()
			}
			return err
		})
	})
	it.page = application.NewSubmenu("Page", page)
	pane.Append(application.NewMenuFromItems(it.page))
}

// itemCommand returns what sends the frontend a bench:command for the
// item that the focused pane shows, as the engine last said.
func (a *appMenu) itemCommand(name string) func() {
	return func() { a.do.command(workspace.Command{Name: name, ItemID: a.now().Pane.Item}) }
}

// addHelpMenu adds the Help menu, to which macOS adds its search field,
// which finds the app menu's items by name: the user guide, and its
// troubleshooting, on GitHub; the keyboard shortcuts; the welcome guide;
// the recent messages; Bench's files; and a new GitHub issue, filled in.
func (a *appMenu) addHelpMenu() {
	help := a.root.AddSubmenu("Help")
	a.add(help, "Bench Help", func() { a.do.openLink(helpURL) })
	a.add(help, "Troubleshooting", func() { a.do.openLink(troubleshootingURL) })
	a.add(help, "Keyboard Shortcuts", a.command(shortcutsCommand.Name))
	help.AddSeparator()
	a.add(help, "Show Welcome Guide", a.command(welcomeCommand.Name))
	a.add(help, "Recent Messages", a.command(messagesCommand.Name))
	help.AddSeparator()
	a.add(help, "Show Bench's Files in Finder", a.act(func(e engine, _ workspace.MenuBar) error { return e.ShowConfigInFinder() }))
	a.add(help, "Report a Problem…", func() { a.do.reportProblem() })
}

// applyLocked changes the items in place for m: their ticks, their labels,
// and whether they're on.
func (a *appMenu) applyLocked(m workspace.MenuBar) {
	it := a.items
	setLabel(it.showAllTabs, withKeys(showAllTabsLabel, m.Keys.ShowAllTabs))
	setLabel(it.dropdown, withKeys(m.Dropdown.Label(), m.Keys.Dropdown))
	it.layouts.tick(m.Mode)
	setLabel(it.rearrange, withKeys("Rearrange", m.Keys.Rearrange))

	tabs := len(m.Tabs)
	setLabel(it.undo, withKeys(undoLabelFor(m.Undo), m.Keys.Undo))
	enable(it.undo, m.Undo != "")
	setLabel(it.nextLayout, withKeys(nextLayoutLabel, m.Keys.NextLayout))
	setLabel(it.previousLayout, withKeys(previousLayoutLabel, m.Keys.PreviousLayout))
	enable(it.nextLayout, m.Arrange.NextLayout != "")
	enable(it.previousLayout, m.Arrange.PreviousLayout != "")
	enable(it.moveLeft, m.Current > 0)
	enable(it.moveRight, m.Current < tabs-1)
	setLabel(it.next, withKeys("Next Tab", m.Keys.NextTab))
	setLabel(it.previous, withKeys("Previous Tab", m.Keys.PreviousTab))
	enable(it.next, tabs > 1)
	enable(it.previous, tabs > 1)
	setLabel(it.lastTab, withKeys(lastTabLabel, m.Keys.LastTab))
	enable(it.lastTab, m.LastTab >= 0 && m.LastTab < tabs && m.LastTab != m.Current)
	setLabel(it.closeTab, closeTabLabelFor(tabs))
	enable(it.closeTab, tabs > 0)
	enable(it.deleteTab, tabs > 1)
	enable(it.reopen, m.Closed != "")
	setLabel(it.reopenItem, reopenItemLabelFor(m))
	enable(it.reopenItem, m.ClosedItem != "")
	for i, item := range it.tabs {
		if i < tabs {
			setLabel(item, withKeys(m.Tabs[i].Label(), m.Tabs[i].Keys))
			tick(item, i == m.Current)
		}
	}

	p := m.Pane
	shows := p.Item != ""
	setLabel(it.jump, withKeys(jumpLabel, m.Keys.Jump))
	move := m.Keys.MoveWindow
	// In the "tabs" view, which hides the tab's splits, Split Right shows
	// the window side by side in the wide layout instead, rather than be
	// off, as the move keys' → does there.
	split := "Split Right"
	if p.SideBySide {
		split = sideBySideLabel
	}
	setLabel(it.split, withKeys(split, keysWith(move, "→")))
	setLabel(it.splitDown, withKeys("Split Down", keysWith(move, "↓")))
	enable(it.split, p.CanSplit || p.SideBySide)
	enable(it.splitDown, p.CanSplit)
	zoom := zoomPaneLabel
	if m.Zoomed {
		zoom = showAllPanesLabel
	}
	setLabel(it.zoom, withKeys(zoom, m.Keys.ZoomPane))
	enable(it.zoom, p.CanZoom)
	setLabel(it.grow, withKeys(growPaneLabel, m.Keys.GrowPane))
	setLabel(it.shrink, withKeys(shrinkPaneLabel, m.Keys.ShrinkPane))
	setLabel(it.equal, withKeys(equalPanesLabel, m.Keys.EqualPanes))
	enable(it.grow, m.Arrange.Grow)
	enable(it.shrink, m.Arrange.Shrink)
	enable(it.equal, m.Arrange.Equal)
	enable(it.moveTo, shows)
	for i, item := range it.moveToTabs {
		if i < tabs {
			// The first nine tabs have the send_to_tab_modifiers shortcut
			// with their number, which moves the window there too, unless
			// another shortcut has that key.
			setLabel(item, withKeys(m.Tabs[i].Name, m.Tabs[i].SendKeys))
			enable(item, shows && i != m.Current)
		}
	}
	enable(it.moveToNew, shows)
	pop := popOutLabel
	if p.NoWindow {
		pop = removeItemLabel
	}
	setLabel(it.popOut, pop)
	enable(it.popOut, shows)
	enable(it.dockAgain, shows && p.Undocked)
	// A window that has closed has nothing to close, but one in other tabs
	// too can still leave this tab, as its ⋯ menu's Remove from This Tab
	// does.
	setLabel(it.closeWindow, closeItemLabelFor(p))
	enable(it.closeWindow, shows && (p.Shared || !p.NoWindow))
	enable(it.closePane, p.CanClose)
	setLabel(it.addFront, withKeys(addFrontWindowLabel, m.Keys.AddFrontWindow))
	enable(it.addFront, m.AddFront)
	enable(it.backToBench, m.BackToBench)
	a.applyPageLocked(p)
}

// applyPageLocked changes Pane ▸ Page's items in place for the focused
// pane's window, p.
func (a *appMenu) applyPageLocked(p workspace.MenuBarPane) {
	it := a.items
	page := p.Item != "" && p.Page
	enable(it.page, page)
	enable(it.reload, page)
	enable(it.back, page && p.CanGoBack)
	enable(it.forward, page && p.CanGoForward)
	enable(it.zoomIn, page && p.CanZoomIn)
	enable(it.zoomOut, page && p.CanZoomOut)
	size := actualSizeLabel
	if page && p.Zoom != 0 {
		size = fmt.Sprintf("%s (now %d%%)", actualSizeLabel, p.Zoom)
	}
	setLabel(it.actualSize, size)
	enable(it.actualSize, page && p.Zoom != 0)
	enable(it.find, page)
	enable(it.copyLink, page && p.Link)
	mic, camera := muteMicLabel, cameraOffLabel
	if p.Microphone == "muted" {
		mic = unmuteMicLabel
	}
	if p.Camera == "muted" {
		camera = cameraOnLabel
	}
	setLabel(it.microphone, mic)
	setLabel(it.camera, camera)
	enable(it.microphone, page && p.Microphone != "")
	enable(it.camera, page && p.Camera != "")
	enable(it.stopSound, page)
	enable(it.keepRunning, page && p.Link)
	tick(it.keepRunning, page && p.KeepRunning)
}

// keysWith returns modifiers with a key added, such as "⌃⌥⇧→", or "" if
// the modifiers are off.
func keysWith(modifiers, key string) string {
	if modifiers == "" {
		return ""
	}
	return modifiers + key
}

// setLabel labels a menu item, if it isn't labelled so already.
func setLabel(item *application.MenuItem, label string) {
	if item != nil && item.Label() != label {
		item.SetLabel(label)
	}
}

// enable turns a menu item on or off, if it isn't already.
func enable(item *application.MenuItem, on bool) {
	if item != nil && item.Enabled() != on {
		item.SetEnabled(on)
	}
}

// tick ticks or unticks a menu item, if it isn't already.
func tick(item *application.MenuItem, on bool) {
	if item != nil && item.Checked() != on {
		item.SetChecked(on)
	}
}

// tick ticks the item of a layout mode, and unticks the others.
func (items layoutItems) tick(mode string) {
	for m, item := range items {
		tick(item, m == mode)
	}
}

// addRole adds a standard item to a menu with its own label, which Wails
// otherwise gives in American English ("Minimize") or, away from macOS,
// without the app's name ("Quit").
func addRole(menu *application.Menu, role application.Role, label string) {
	if item := application.NewRole(role); item != nil {
		menu.Append(application.NewMenuFromItems(item.SetLabel(label)))
	}
}

// settingsCommand is the bench:command that Settings… sends.
var settingsCommand = workspace.Command{Name: "settings"}

// paletteCommand is the bench:command that Jump to…, in the Pane menu and
// the menu bar item's menu, sends. Unlike the jump shortcut's, it doesn't
// close the palette if it's open.
var paletteCommand = workspace.Command{Name: "open-palette"}

// showAllTabsCommand is the bench:command that View ▸ Show All Tabs sends.
// Unlike the show_all_tabs shortcut's, it doesn't close the overview if
// it's open.
var showAllTabsCommand = workspace.Command{Name: "show-all-tabs"}

// newTabCommand, newTabFromWindowsCommand, tabPropertiesCommand, and
// renameTabCommand are the bench:commands that New Tab…, New Tab from Open
// Windows…, Tab Properties…, and Rename… send: the first three open Tab
// properties, for a new tab, a new tab of the windows that are open, or
// the current tab, and the last renames the current tab in place, in the
// tab strip.
var (
	newTabCommand            = workspace.Command{Name: "new-tab"}
	newTabFromWindowsCommand = workspace.Command{Name: "new-tab-from-windows"}
	tabPropertiesCommand     = workspace.Command{Name: "edit-tab"}
	renameTabCommand         = workspace.Command{Name: "rename-tab"}
)

// closeTabCommand and closeWindowCommand are the bench:commands that Close
// Tab… and Close Window send. The frontend asks first where it would, as a
// tab's menu does.
var (
	closeTabCommand    = workspace.Command{Name: "close-tab"}
	closeWindowCommand = workspace.Command{Name: "close-window"}
)

// addToPaneCommand is the bench:command that Add to Pane… sends: it opens
// the add dialog for the focused pane.
var addToPaneCommand = workspace.Command{Name: "add-to-pane"}

// findInPageCommand and copyPageLinkCommand are the bench:commands that
// Pane ▸ Page's Find in Page… and Copy Page Link send, with the item that
// the focused pane shows: the frontend opens its find bar, with the
// keyboard in its field, and puts its link on the clipboard, as its ⋯
// menu does.
var (
	findInPageCommand   = workspace.Command{Name: "find-in-page"}
	copyPageLinkCommand = workspace.Command{Name: "copy-page-link"}
)

// shortcutsCommand, welcomeCommand, and messagesCommand are the
// bench:commands of Help's Keyboard Shortcuts, Show Welcome Guide, and
// Recent Messages: they show the shortcuts, the welcome guide that the
// first run shows, and the recent messages.
var (
	shortcutsCommand = workspace.Command{Name: "keyboard-shortcuts"}
	welcomeCommand   = workspace.Command{Name: "first-run"}
	messagesCommand  = workspace.Command{Name: "recent-messages"}
)
