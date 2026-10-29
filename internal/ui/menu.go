//go:build darwin || server

package ui

import (
	"github.com/wailsapp/wails/v3/pkg/application"

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
}

// appMenu builds the app menu: Bench, Edit, View, and Window.
//
// Only the standard macOS shortcuts are bound (Hide, Hide Others, Quit, the
// Edit commands, and Minimise). Command shortcuts that pages rely on, such as
// ⌘W, ⌘P, ⌘N, ⌘T, ⌘F, ⌘L, and ⌘, (VS Code's settings), are left for Web
// items, so Acknowledgements, Settings…, Reload config.toml, Tab
// Properties…, Fill the Screen, and Rearrange have none.
//
// It must be called after application.New, because the standard items take
// the app's name from it.
func appMenu(do menuActions) *application.Menu {
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
	view.Add("Fill the Screen").OnClick(func(*application.Context) { do.fillScreen() })
	view.Add("Rearrange").OnClick(func(*application.Context) { do.rearrange() })

	window := application.NewMenu()
	addRole(window, application.Minimise, "Minimise")
	menu.Append(application.NewMenuFromItems(
		application.NewSubmenu("Window", window).SetRole(application.WindowMenu)))

	return menu
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
