//go:build darwin || server

package ui

import (
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/update"
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
// one tab, as closing it leaves the tab, empty.
const (
	closeTabLabel  = "Close Tab…"
	closeAllLabel  = "Close All Windows…"
	closeItemLabel = "Close Window"
)

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

// Labels of Zoom Pane, which reads Show All Panes while a pane is zoomed.
const (
	zoomPaneLabel     = "Zoom Pane"
	showAllPanesLabel = "Show All Panes"
)

// Labels of Pop Out, which reads Remove from Bench for a window that has
// closed, as there's nothing to pop out.
const (
	popOutLabel     = "Pop Out"
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
	// The View menu: View ▸ Layout's items, ticked for the layout mode,
	// and Rearrange, with its shortcut.
	layouts   layoutItems
	rearrange *application.MenuItem
	// The Tab menu: Move Tab Left and Right, which are off at the ends;
	// Next and Previous Tab, off with one tab; Last Used Tab, off until
	// there's one to go back to; Close Tab…, labelled for the number of
	// tabs; Delete Tab (Keep Windows), off with one tab; Reopen Closed Tab,
	// off with none to reopen; and the tabs, the current one ticked.
	moveLeft, moveRight *application.MenuItem
	next, previous      *application.MenuItem
	lastTab             *application.MenuItem
	closeTab, deleteTab *application.MenuItem
	reopen              *application.MenuItem
	tabs                []*application.MenuItem
	// The Pane menu, for the focused pane: each is off where it would do
	// nothing. Zoom Pane reads Show All Panes while a pane is zoomed, and
	// Pop Out reads Remove from Bench for a window that has closed. Move
	// to Tab lists every tab, the current one off.
	jump, split, splitDown, zoom *application.MenuItem
	moveTo                       *application.MenuItem
	moveToTabs                   []*application.MenuItem
	moveToNew                    *application.MenuItem
	popOut, dockAgain            *application.MenuItem
	closeWindow, closePane       *application.MenuItem
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
	// built for: the menu is built again when the number of tabs changes,
	// and otherwise changed in place.
	bar   workspace.MenuBar
	built workspace.MenuBar
	items menuItems
	// started is set once the app runs (start). Until then, the menu
	// isn't changed, as Wails reads it to make the native menu.
	started bool
	// clicks are what the items do, for tests, which can't click them.
	clicks map[*application.MenuItem]func()

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
// runs, it builds the menu again if the number of tabs has changed, and
// otherwise changes its items in place.
func (a *appMenu) set(m workspace.MenuBar) {
	a.latest.Store(&m)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bar = m
	if !a.started {
		return
	}
	if len(m.Tabs) != len(a.built.Tabs) {
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
	a.items = menuItems{}
	a.built = m
	a.addBenchMenu()
	a.addEditMenu()
	a.addViewMenu()
	a.addTabMenu(m)
	a.addPaneMenu(m)
	window := application.NewMenu()
	addRole(window, application.Minimise, "Minimise")
	a.root.Append(application.NewMenuFromItems(
		application.NewSubmenu("Window", window).SetRole(application.WindowMenu)))
	a.addHelpMenu()
	a.applyLocked(m)
}

// addBenchMenu adds the app's own menu: About, Check for Updates…, which
// opens Bench's releases on GitHub, Acknowledgements, Settings…, Reload
// config.toml, Hide, and Quit.
func (a *appMenu) addBenchMenu() {
	bench := a.root.AddSubmenu(appName)
	bench.AddRole(application.About)
	a.add(bench, "Check for Updates…", func() { a.do.openLink(update.ReleasesURL) })
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

// addViewMenu adds the View menu: Layout ▸ Auto, Wide, and Compact, a group
// of radio items, which Wails ticks one of when it's chosen; Rearrange; and
// Fill the Screen.
func (a *appMenu) addViewMenu() {
	view := a.root.AddSubmenu("View")
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

// addTabMenu adds the Tab menu, for the current tab: adding a tab, its
// properties and name, moving it, the next and previous tabs and the one
// used last, closing, deleting, or reopening one, and the tabs, with their
// shortcuts, to switch to.
func (a *appMenu) addTabMenu(m workspace.MenuBar) {
	tab := a.root.AddSubmenu("Tab")
	a.add(tab, "New Tab…", a.command(newTabCommand.Name))
	tab.AddSeparator()
	a.add(tab, "Tab Properties…", a.command(tabPropertiesCommand.Name))
	a.add(tab, "Rename…", a.command(renameTabCommand.Name))
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
	a.items.reopen = a.add(tab, "Reopen Closed Tab", a.act(func(e engine, m workspace.MenuBar) error {
		if m.Closed == "" {
			return nil
		}
		return e.ReopenTab(m.Closed)
	}))
	if len(m.Tabs) > 0 {
		tab.AddSeparator()
	}
	for i := range m.Tabs {
		a.items.tabs = append(a.items.tabs, a.bind(tab.AddRadio("", false), func() {
			a.do.act(func(e engine) error {
				err := e.SwitchTab(i)
				if err != nil {
					a.retick()
				}
				return err
			})
		}))
	}
}

// addPaneMenu adds the Pane menu, for the focused pane and the window it
// shows: the jump palette and the add dialog; putting the window beside
// the pane, or zooming the pane; moving the window to another tab, out of
// Bench, or back into its pane; and closing the window, or the pane.
func (a *appMenu) addPaneMenu(m workspace.MenuBar) {
	pane := a.root.AddSubmenu("Pane")
	it := &a.items
	it.jump = a.add(pane, jumpLabel, a.command(paletteCommand.Name))
	a.add(pane, "Add to Pane…", a.command(addToPaneCommand.Name))
	pane.AddSeparator()
	it.split = a.add(pane, "Split Right", a.act(func(e engine, m workspace.MenuBar) error {
		return e.MoveItem(m.Pane.Item, m.Current, m.Pane.ID, "right")
	}))
	it.splitDown = a.add(pane, "Split Down", a.act(func(e engine, m workspace.MenuBar) error {
		return e.MoveItem(m.Pane.Item, m.Current, m.Pane.ID, "bottom")
	}))
	it.zoom = a.add(pane, zoomPaneLabel, a.act(func(e engine, _ workspace.MenuBar) error { return e.ToggleZoom() }))
	pane.AddSeparator()
	moveTo := application.NewMenu()
	for i := range m.Tabs {
		it.moveToTabs = append(it.moveToTabs, a.add(moveTo, "", a.act(func(e engine, m workspace.MenuBar) error {
			return e.MoveItemToTab(m.Pane.Item, i)
		})))
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
	it.closeWindow = a.add(pane, closeItemLabel, a.command(closeWindowCommand.Name))
	it.closePane = a.add(pane, "Close Pane", a.act(func(e engine, m workspace.MenuBar) error { return e.ClosePane(m.Pane.ID) }))
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
	it.layouts.tick(m.Mode)
	setLabel(it.rearrange, withKeys("Rearrange", m.Keys.Rearrange))

	tabs := len(m.Tabs)
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
	for i, item := range it.tabs {
		if i < tabs {
			setLabel(item, withKeys(m.Tabs[i].Name, m.Tabs[i].Keys))
			tick(item, i == m.Current)
		}
	}

	p := m.Pane
	shows := p.Item != ""
	setLabel(it.jump, withKeys(jumpLabel, m.Keys.Jump))
	move := m.Keys.MoveWindow
	setLabel(it.split, withKeys("Split Right", keysWith(move, "→")))
	setLabel(it.splitDown, withKeys("Split Down", keysWith(move, "↓")))
	enable(it.split, p.CanSplit)
	enable(it.splitDown, p.CanSplit)
	zoom := zoomPaneLabel
	if m.Zoomed {
		zoom = showAllPanesLabel
	}
	setLabel(it.zoom, withKeys(zoom, m.Keys.ZoomPane))
	enable(it.zoom, p.CanZoom)
	enable(it.moveTo, shows)
	for i, item := range it.moveToTabs {
		if i < tabs {
			setLabel(item, m.Tabs[i].Name)
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
	enable(it.closeWindow, shows && !p.NoWindow)
	enable(it.closePane, p.CanClose)
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

// newTabCommand, tabPropertiesCommand, and renameTabCommand are the
// bench:commands that New Tab…, Tab Properties…, and Rename… send: the
// first two open Tab properties, for a new tab or the current one, and the
// last renames the current tab in place, in the tab strip.
var (
	newTabCommand        = workspace.Command{Name: "new-tab"}
	tabPropertiesCommand = workspace.Command{Name: "edit-tab"}
	renameTabCommand     = workspace.Command{Name: "rename-tab"}
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

// shortcutsCommand, welcomeCommand, and messagesCommand are the
// bench:commands of Help's Keyboard Shortcuts, Show Welcome Guide, and
// Recent Messages: they show the shortcuts, the welcome guide that the
// first run shows, and the recent messages.
var (
	shortcutsCommand = workspace.Command{Name: "keyboard-shortcuts"}
	welcomeCommand   = workspace.Command{Name: "first-run"}
	messagesCommand  = workspace.Command{Name: "recent-messages"}
)
