//go:build darwin || server

package ui

import (
	"net/url"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Bench's main window, in points.
const (
	mainWidth     = 1440
	mainHeight    = 900
	mainMinWidth  = 900
	mainMinHeight = 600
)

// A popped-out terminal's window, in points.
const (
	termWidth     = 760
	termHeight    = 480
	termMinWidth  = 360
	termMinHeight = 200
)

// windowBackground matches the frontend's --bg-window, so a window never
// flashes white while its page loads.
var windowBackground = application.NewRGB(0x12, 0x14, 0x19)

// mainWindowOptions returns the options for Bench's main window: a hidden,
// inset title bar with full-size content, so that the traffic lights sit at
// the left of the frontend's tab strip, which also moves the window (it
// marks itself with --wails-draggable). There's no invisible native title
// bar: it would turn clicks on the tab strip into window drags. The window
// can't go into native full screen; View ▸ Fill the Screen zooms it instead.
func mainWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             "main",
		Title:            appName,
		Width:            mainWidth,
		Height:           mainHeight,
		MinWidth:         mainMinWidth,
		MinHeight:        mainMinHeight,
		URL:              "/",
		BackgroundColour: windowBackground,
		Mac: application.MacWindow{
			TitleBar:           application.MacTitleBarHiddenInset,
			Appearance:         application.NSAppearanceNameDarkAqua,
			CollectionBehavior: application.MacWindowCollectionBehaviorFullScreenNone,
		},
	}
}

// terminalWindowOptions returns the options for a popped-out terminal's
// window, which loads the frontend showing just that terminal.
func terminalWindowOptions(itemID, title string) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             "terminal:" + itemID,
		Title:            title,
		Width:            termWidth,
		Height:           termHeight,
		MinWidth:         termMinWidth,
		MinHeight:        termMinHeight,
		URL:              terminalURL(itemID),
		BackgroundColour: windowBackground,
		Mac: application.MacWindow{
			Appearance:         application.NSAppearanceNameDarkAqua,
			CollectionBehavior: application.MacWindowCollectionBehaviorFullScreenNone,
		},
	}
}

// terminalURL is the page a popped-out terminal's window loads.
func terminalURL(itemID string) string {
	return "/?" + url.Values{"terminal": {itemID}}.Encode()
}

// previewShell is the browser preview's shell, which has no windows of its
// own and no menu bar item.
type previewShell struct{}

func (previewShell) bringForward()                       {}
func (previewShell) popOutTerminal(string, string) error { return nil }
func (previewShell) closeTerminalWindow(string)          {}
func (previewShell) setCurrentTab(string, []string)      {}

// appShell is the macOS app's shell: Bench's main window, popped-out
// terminals' windows, and the menu bar item.
type appShell struct {
	app *application.App
	// main is set before the app runs.
	main *application.WebviewWindow
	// switchTab and toast are the engine's SwitchTab and the emitter's
	// Toast, for the menu bar item's menu. Set before the app runs.
	switchTab func(index int) error
	toast     func(message string)
	// quitting is set when Bench starts quitting, so that closing the main
	// window then really closes it.
	quitting atomic.Bool

	// termMu guards terms. It's never held while waiting for the main
	// thread.
	termMu sync.Mutex
	terms  map[string]*application.WebviewWindow

	// trayMu serialises changes to the menu bar item, which wait for the
	// main thread, so the main thread must never take it.
	trayMu    sync.Mutex
	tray      *application.SystemTray
	trayItems []*application.MenuItem // One for each tab.
	trayTabs  []string                // The tabs in the tray's menu.
	tab       string                  // The current tab.
	tabs      []string
}

// newAppShell returns the shell for app. It opens nothing until openMain.
func newAppShell(app *application.App) *appShell {
	return &appShell{app: app, terms: map[string]*application.WebviewWindow{}}
}

// attachEvents are the main window's events that come once it has an
// NSWindow: whichever comes first attaches it.
var attachEvents = []events.WindowEventType{
	events.Common.WindowShow,
	events.Common.WindowFocus,
	events.Common.WindowRuntimeReady,
}

// openMain creates Bench's main window. Closing it hides it, until Bench
// quits; the menu bar item's Show Bench brings it back. attach, if not nil,
// gets the window once it has an NSWindow, when it's first shown or focused
// or its page has loaded the Wails runtime.
func (s *appShell) openMain(attach func(*application.WebviewWindow)) {
	w := s.app.Window.NewWithOptions(mainWindowOptions())
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.quitting.Load() {
			return
		}
		e.Cancel()
		w.Hide()
	})
	if attach != nil {
		var once sync.Once
		for _, ev := range attachEvents {
			w.OnWindowEvent(ev, func(*application.WindowEvent) {
				if w.NativeWindow() != nil {
					once.Do(func() { attach(w) })
				}
			})
		}
	}
	s.main = w
}

// bringForward shows Bench's main window and activates the app.
func (s *appShell) bringForward() {
	w := s.main
	if w == nil {
		return
	}
	// App.Show calls AppKit directly, so it must run on the main thread;
	// global shortcuts and the engine call this from other goroutines. The
	// window methods below hop to the main thread themselves.
	application.InvokeAsync(s.app.Show)
	if w.IsMinimised() {
		w.UnMinimise()
	}
	w.Show()
	w.Focus()
}

// fillScreen zooms the main window to fill the screen.
func (s *appShell) fillScreen() {
	if s.main != nil {
		s.main.Zoom()
	}
}

// popOutTerminal opens a window showing just a terminal, or brings its
// window forward if it has one.
func (s *appShell) popOutTerminal(itemID, title string) error {
	s.termMu.Lock()
	w := s.terms[itemID]
	s.termMu.Unlock()
	if w != nil {
		w.SetTitle(title)
		w.Show()
		w.Focus()
		return nil
	}
	w = s.app.Window.NewWithOptions(terminalWindowOptions(itemID, title))
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.forgetTerminal(itemID, w)
	})
	s.termMu.Lock()
	old := s.terms[itemID]
	s.terms[itemID] = w
	s.termMu.Unlock()
	if old != nil {
		old.Close()
	}
	return nil
}

// forgetTerminal drops a terminal's window, if it's still the one recorded.
func (s *appShell) forgetTerminal(itemID string, w *application.WebviewWindow) {
	s.termMu.Lock()
	defer s.termMu.Unlock()
	if s.terms[itemID] == w {
		delete(s.terms, itemID)
	}
}

// closeTerminalWindow closes a terminal's window, if it has one.
func (s *appShell) closeTerminalWindow(itemID string) {
	s.termMu.Lock()
	w := s.terms[itemID]
	delete(s.terms, itemID)
	s.termMu.Unlock()
	if w != nil {
		w.Close()
	}
}

// setCurrentTab updates the menu bar item, once it exists.
func (s *appShell) setCurrentTab(name string, tabs []string) {
	s.trayMu.Lock()
	defer s.trayMu.Unlock()
	s.tab, s.tabs = name, slices.Clone(tabs)
	if s.tray != nil {
		s.updateTrayLocked()
	}
}

// startTray adds the menu bar item. Call it once the app is running, off the
// main thread.
func (s *appShell) startTray() {
	s.trayMu.Lock()
	defer s.trayMu.Unlock()
	if s.tray != nil {
		return
	}
	s.tray = s.app.SystemTray.New()
	s.updateTrayLocked()
}

// updateTrayLocked shows the current tab's name in the menu bar item, and
// rebuilds its menu if the tabs have changed.
func (s *appShell) updateTrayLocked() {
	label := s.tab
	if label == "" {
		label = appName
	}
	s.tray.SetLabel(label)
	if s.trayItems == nil || !slices.Equal(s.tabs, s.trayTabs) {
		s.trayTabs = slices.Clone(s.tabs)
		s.tray.SetMenu(s.trayMenuLocked())
		return
	}
	for i, item := range s.trayItems {
		item.SetChecked(s.trayTabs[i] == s.tab)
	}
}

// trayMenuLocked builds the menu bar item's menu: the tabs (choosing one
// switches to it and shows Bench), then Show Bench and Quit Bench.
func (s *appShell) trayMenuLocked() *application.Menu {
	menu := application.NewMenu()
	s.trayItems = make([]*application.MenuItem, 0, len(s.trayTabs))
	for i, name := range s.trayTabs {
		item := menu.AddRadio(name, name == s.tab)
		item.OnClick(func(*application.Context) { s.chooseTab(i) })
		s.trayItems = append(s.trayItems, item)
	}
	if len(s.trayTabs) > 0 {
		menu.AddSeparator()
	}
	menu.Add("Show " + appName).OnClick(func(*application.Context) { s.bringForward() })
	menu.Add("Quit " + appName).OnClick(func(*application.Context) { s.app.Quit() })
	return menu
}

// chooseTab switches to a tab from the menu bar item and shows Bench.
func (s *appShell) chooseTab(index int) {
	if s.switchTab != nil {
		if err := s.switchTab(index); err != nil && s.toast != nil {
			s.toast(err.Error())
		}
	}
	s.bringForward()
}
