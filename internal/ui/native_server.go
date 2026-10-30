//go:build server

package ui

import "github.com/wailsapp/wails/v3/pkg/application"

// nativeShell is false in the browser preview (Wails' server mode), which
// serves the UI over HTTP and has no windows, menus, menu bar item, global
// shortcuts (the frontend handles keys itself), or Open at login.
const nativeShell = false

// linkOpener returns nil: the browser preview doesn't open links on the
// machine its server runs on, just as it runs no shells there (decision 8).
func linkOpener(*application.App) func(address string) error {
	return nil
}

// folderChooser returns nil: the browser preview has no folder picker on
// the machine its server runs on, so the forms ask for the folder to be
// typed instead.
func folderChooser(*application.App, func() *application.WebviewWindow) func(start string) (string, error) {
	return nil
}
