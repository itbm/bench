//go:build darwin && !server

package ui

import "github.com/wailsapp/wails/v3/pkg/application"

// nativeShell is true in the macOS app, which has a main window, an app
// menu, a menu bar item, global shortcuts, and Open at login.
const nativeShell = true

// linkOpener returns what opens the links terminal programs print: the
// default browser.
func linkOpener(app *application.App) func(address string) error {
	return app.Browser.OpenURL
}
