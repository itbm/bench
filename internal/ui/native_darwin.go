//go:build darwin && !server

package ui

import (
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// nativeShell is true in the macOS app, which has a main window, an app
// menu, a menu bar item, global shortcuts, and Open at login.
const nativeShell = true

// linkOpener returns what opens the links terminal programs print: the
// default browser.
func linkOpener(app *application.App) func(address string) error {
	return app.Browser.OpenURL
}

// folderChooser returns what shows the folder picker for the add dialog's
// New terminal… form and Settings ▸ Launchers: macOS's open panel, for
// folders only, as a sheet on window, Bench's main window, once it has one.
// A sheet stays with Bench's window, which is raised while a dialog is
// open, rather than going behind the windows docked in front of it. It
// waits for the user, off the main thread, and returns "" if they cancel.
// One shows at a time: asking again while one is open returns "".
func folderChooser(app *application.App, window func() *application.WebviewWindow) func(start string) (string, error) {
	var open atomic.Bool
	return func(start string) (string, error) {
		if !open.CompareAndSwap(false, true) {
			return "", nil
		}
		defer open.Store(false)
		d := app.Dialog.OpenFile().
			CanChooseFiles(false).
			CanChooseDirectories(true).
			CanCreateDirectories(true).
			ResolvesAliases(true).
			SetMessage("Choose a folder.").
			SetButtonText("Choose").
			SetDirectory(start)
		if w := window(); w != nil {
			d.AttachToWindow(w)
		}
		return d.PromptForSingleSelection()
	}
}
