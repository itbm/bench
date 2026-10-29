//go:build darwin || server

package ui

import (
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

// menuActions are what Bench's own menu items do.
type menuActions struct {
	// acknowledgements opens the Acknowledgements window, or brings it
	// forward.
	acknowledgements func()
	// settings opens the Settings dialog, for the settings that apply to
	// every tab.
	settings func()
	// reloadConfig reads config.toml again, for changes made to it outside
	// Bench.
	reloadConfig func()
	// tabProperties opens the Tab properties dialog for the current tab.
	tabProperties func()
	// fillScreen zooms Bench's window to fill the screen, unless it fills
	// it already.
	fillScreen func()
	// rearrange arranges the current tab again.
	rearrange func()
	// closeTab asks to close the current tab, with its windows, pages, and
	// terminals, and closeWindow to close the focused pane's window, page,
	// or terminal.
	closeTab    func()
	closeWindow func()
	// setMode sets the layout mode: auto, wide, or compact.
	setMode func(mode string)
}

// layoutChoices are the View menu's layout items, with the mode each sets,
// in the order of the title bar's Auto, Wide, and Compact.
var layoutChoices = []struct {
	mode  layout.Mode
	label string
}{
	{layout.ModeAuto, "Auto Layout"},
	{layout.ModeWide, "Wide Layout"},
	{layout.ModeCompact, "Compact Layout"},
}

// layoutItems are the View menu's layout items, by the mode each sets.
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

// viewItems are the View menu's items that the shell changes as the engine
// says what's what: the layout items, ticked for the layout mode, and Close
// Tab…, labelled for the number of tabs.
type viewItems struct {
	layouts  layoutItems
	closeTab *application.MenuItem
}

// addCloseItems adds Close Tab… and Close Window to a menu, with no
// shortcuts, and returns Close Tab…. They're in the View menu, for now: a
// Tab menu could take them.
func addCloseItems(menu *application.Menu, do menuActions) *application.MenuItem {
	closeTab := menu.Add(closeTabLabel).OnClick(func(*application.Context) { do.closeTab() })
	menu.Add(closeItemLabel).OnClick(func(*application.Context) { do.closeWindow() })
	return closeTab
}

// appMenu builds the app menu: Bench, Edit, View, and Window. It returns
// the View menu's layout items too, which the shell ticks as the layout
// mode changes (setMenuBar), however it's chosen, and Close Tab…, which the
// shell labels for the number of tabs.
//
// Only the standard macOS shortcuts are bound (Hide, Hide Others, Quit, the
// Edit commands, and Minimise). Command shortcuts that pages rely on, such as
// ⌘W, ⌘P, ⌘N, ⌘T, ⌘F, ⌘L, and ⌘, (VS Code's settings), are left for Web
// items, so Acknowledgements, Settings…, Reload config.toml, Tab
// Properties…, Close Tab…, Close Window, the layouts, Fill the Screen, and
// Rearrange have none.
//
// It must be called after application.New, because the standard items take
// the app's name from it.
func appMenu(do menuActions) (*application.Menu, viewItems) {
	menu := application.NewMenu()

	bench := menu.AddSubmenu(appName)
	bench.AddRole(application.About)
	bench.Add(acknowledgementsLabel).OnClick(func(*application.Context) { do.acknowledgements() })
	bench.AddSeparator()
	bench.Add("Settings…").OnClick(func(*application.Context) { do.settings() })
	bench.Add(reloadConfigLabel).OnClick(func(*application.Context) { do.reloadConfig() })
	bench.AddSeparator()
	bench.AddRole(application.Hide)
	bench.AddRole(application.HideOthers)
	bench.AddRole(application.ShowAll)
	bench.AddSeparator()
	addRole(bench, application.Quit, "Quit "+appName)

	edit := menu.AddSubmenu("Edit")
	edit.AddRole(application.Undo)
	edit.AddRole(application.Redo)
	edit.AddSeparator()
	edit.AddRole(application.Cut)
	edit.AddRole(application.Copy)
	edit.AddRole(application.Paste)
	edit.AddRole(application.SelectAll)

	view := menu.AddSubmenu("View")
	view.Add("Tab Properties…").OnClick(func(*application.Context) { do.tabProperties() })
	view.AddSeparator()
	closeTab := addCloseItems(view, do)
	view.AddSeparator()
	// A group of radio items, which Wails ticks one of when it's chosen.
	// None is ticked until the engine says which mode is set.
	layouts := layoutItems{}
	for _, c := range layoutChoices {
		mode := string(c.mode)
		layouts[mode] = view.AddRadio(c.label, false).OnClick(func(*application.Context) { do.setMode(mode) })
	}
	view.AddSeparator()
	view.Add("Fill the Screen").OnClick(func(*application.Context) { do.fillScreen() })
	view.Add("Rearrange").OnClick(func(*application.Context) { do.rearrange() })

	window := application.NewMenu()
	addRole(window, application.Minimise, "Minimise")
	menu.Append(application.NewMenuFromItems(
		application.NewSubmenu("Window", window).SetRole(application.WindowMenu)))

	return menu, viewItems{layouts: layouts, closeTab: closeTab}
}

// label labels Close Tab… for the number of tabs.
func (v viewItems) label(tabs int) {
	if v.closeTab == nil || tabs <= 0 {
		return
	}
	if label := closeTabLabelFor(tabs); v.closeTab.Label() != label {
		v.closeTab.SetLabel(label)
	}
}

// tick ticks the item of a layout mode, and unticks the others.
func (items layoutItems) tick(mode string) {
	for m, item := range items {
		if item.Checked() != (m == mode) {
			item.SetChecked(m == mode)
		}
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

// paletteCommand is the bench:command that the menu bar item's Jump to…
// sends. Unlike the jump shortcut's, it doesn't close the palette if it's
// open.
var paletteCommand = workspace.Command{Name: "open-palette"}

// tabPropertiesCommand is the bench:command that Tab Properties… sends.
var tabPropertiesCommand = workspace.Command{Name: "edit-tab"}

// closeTabCommand and closeWindowCommand are the bench:commands that Close
// Tab… and Close Window send. The frontend asks first where it would, as a
// tab's menu does.
var (
	closeTabCommand    = workspace.Command{Name: "close-tab"}
	closeWindowCommand = workspace.Command{Name: "close-window"}
)
