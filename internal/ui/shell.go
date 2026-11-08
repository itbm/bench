//go:build darwin || server

package ui

import (
	_ "embed"
	"errors"
	"math"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/workspace"
)

// Bench's main window's least size, in points: small enough for macOS to
// tile it into half the screen of a 13-inch MacBook, which is about 735 by
// 925 points at its default scaling, or into a quarter of a larger one.
// Where it opens, and at what size, is the engine's (WindowStart).
const (
	mainMinWidth  = 640
	mainMinHeight = 480
)

// A popped-out terminal's window, in points.
const (
	termWidth     = 760
	termHeight    = 480
	termMinWidth  = 360
	termMinHeight = 200
)

// The Acknowledgements window, in points.
const (
	ackWidth     = 640
	ackHeight    = 720
	ackMinWidth  = 420
	ackMinHeight = 320
)

// acknowledgementsPage is the page the Acknowledgements window loads:
// frontend/acknowledgements.html, which make acknowledgements writes from the
// licences of what Bench.app includes.
const acknowledgementsPage = "/acknowledgements.html"

// The frontend's --bg-window, in the dark and light appearances.
var (
	darkBackground  = application.NewRGB(0x12, 0x14, 0x19)
	lightBackground = application.NewRGB(0xF5, 0xF6, 0xF8)
)

// windowLook is how one of Bench's windows looks as it opens: dark, light,
// or as macOS is, by the appearance setting, so systemDark, whether macOS
// is dark now, matters only for "system".
type windowLook struct {
	appearance string
	systemDark bool
}

// macAppearance is the window's own appearance, which its page's colours
// follow (prefers-color-scheme): none for "system", so that it follows
// macOS. An unknown setting is dark, as Bench always was.
func (l windowLook) macAppearance() application.MacAppearanceType {
	switch l.appearance {
	case config.AppearanceSystem:
		return application.DefaultAppearance
	case config.AppearanceLight:
		return application.NSAppearanceNameAqua
	default:
		return application.NSAppearanceNameDarkAqua
	}
}

// background is the window's colour while its page loads: its page's
// --bg-window, so that the window never flashes white, nor the other
// appearance's colour. Once the window has an NSWindow, the shell gives it
// one that follows its appearance (keepLook).
func (l windowLook) background() application.RGBA {
	light := l.appearance == config.AppearanceLight || l.appearance == config.AppearanceSystem && !l.systemDark
	if light {
		return lightBackground
	}
	return darkBackground
}

// menuBarIcon is the menu bar item's icon: a template image, which macOS
// tints to suit the menu bar. build/darwin/menubar/main.go draws it.
//
//go:embed menubar.png
var menuBarIcon []byte

// menuBarNameLength is the most characters of the current tab's name that
// the menu bar item shows, its ellipsis included, so that a long name
// doesn't push other items aside, or behind the notch.
const menuBarNameLength = 16

// jumpLabel is the label of the menu bar item's menu's item that opens the
// jump palette, as Bench's title bar names it.
const jumpLabel = "Jump to…"

// mainWindowOptions returns the options for Bench's main window: a hidden,
// inset title bar with full-size content, so that the traffic lights sit at
// the left of the frontend's tab strip, which also moves the window (it
// marks itself with --wails-draggable). There's no invisible native title
// bar: it would turn clicks on the tab strip into window drags. The window
// can't go into native full screen; View ▸ Fill the Screen zooms it instead.
//
// The window opens on start's screen, with its content area at start's
// Frame, or, without a screen, at the usual size in the middle of the
// primary screen. openMain then fills the screen with it if start says to.
//
// The title bar is the toolbar's band, which macOS centres the traffic
// lights in: about 52 pt tall, or about 38 pt with the unified compact
// toolbar style, for the compact density, which the frontend's title bar
// matches (the View's TitleBar). AppKit can change the style of a window
// that's open, but the title bar's views, which pass clicks through
// (decision 12), may not survive it, so the style is set as the window is
// made, and a new density reaches the title bar when Bench next opens.
//
// It opens with the appearance setting's look (windowLook), as macOS is
// for "system", which systemDark says.
func mainWindowOptions(start workspace.WindowStart, systemDark bool) application.WebviewWindowOptions {
	titleBar := application.MacTitleBarHiddenInset
	if start.CompactTitleBar {
		titleBar.ToolbarStyle = application.MacToolbarStyleUnifiedCompact
	}
	screen := start.Screen
	look := windowLook{appearance: start.Appearance, systemDark: systemDark}
	o := application.WebviewWindowOptions{
		Name:             "main",
		Title:            appName,
		Width:            workspace.UsualWidth,
		Height:           workspace.UsualHeight,
		MinWidth:         mainMinWidth,
		MinHeight:        mainMinHeight,
		URL:              "/",
		BackgroundColour: look.background(),
		Mac: application.MacWindow{
			TitleBar:           titleBar,
			Appearance:         look.macAppearance(),
			CollectionBehavior: application.MacWindowCollectionBehaviorFullScreenNone,
		},
	}
	if f := start.Frame; screen.Visible.W > 0 && screen.Visible.H > 0 && f.W > 0 && f.H > 0 {
		o.InitialPosition = application.WindowXY
		o.X, o.Y, o.Width, o.Height = mainPlace(f)
	}
	return o
}

// mainPlace returns the position and size that Wails opens the main window
// at, for its content area to be at content, in global coordinates. Wails
// places the window's top-left corner at X and Y, from the top-left of the
// primary screen, as global coordinates are. It makes the window 1 pt
// narrower and shorter than Width and Height, and its web view, whose frame
// is Bench's content area (see benchReferenceView in internal/mac), that
// size, sticking out 1 pt above the window and to its right. So the window
// goes 1 pt below the content area's top, for a content area saved as
// Bench quit to come back where it was, not 1 pt higher at each start.
func mainPlace(content layout.Rect) (x, y, width, height int) {
	return int(math.Round(content.X)), int(math.Round(content.Y)) + 1, int(math.Round(content.W)), int(math.Round(content.H))
}

// terminalWindowOptions returns the options for a popped-out terminal's
// window, which loads the frontend showing just that terminal, with a look.
// The app menu leaves ⌘W to Web items, so the window binds it itself, as
// Acknowledgements does: closing it asks first while the terminal is
// running something, as the close button does (keepTerminalOpen).
func terminalWindowOptions(itemID, title string, look windowLook) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             "terminal:" + itemID,
		Title:            title,
		Width:            termWidth,
		Height:           termHeight,
		MinWidth:         termMinWidth,
		MinHeight:        termMinHeight,
		URL:              terminalURL(itemID),
		BackgroundColour: look.background(),
		Mac: application.MacWindow{
			Appearance:         look.macAppearance(),
			CollectionBehavior: application.MacWindowCollectionBehaviorFullScreenNone,
		},
		KeyBindings: map[string]func(application.Window){
			"CmdOrCtrl+W": func(w application.Window) { w.Close() },
		},
	}
}

// terminalURL is the page a popped-out terminal's window loads.
func terminalURL(itemID string) string {
	return "/?" + url.Values{"terminal": {itemID}}.Encode()
}

// dropdownWindowOptions returns the options for the drop-down terminal's
// window (decision 72): the popped-out terminal's page, in its drop-down
// form, in a panel with no title bar, which floats over other windows and
// doesn't make Bench active as it takes the keyboard, so that the app in
// front stays in front. It's on every Space, over full-screen apps too,
// and out of the windows that ⌘` goes through. It starts hidden: the
// platform puts it at the top of the screen and slides it into sight
// (Options.SlideDown), at the height the setting gives, so it can't be
// resized. ⌘W closes it, as a popped-out terminal's window.
func dropdownWindowOptions(itemID, title string, look windowLook) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             "dropdown:" + itemID,
		Title:            title,
		Width:            termWidth,
		Height:           termHeight,
		URL:              dropdownURL(itemID),
		Hidden:           true,
		Frameless:        true,
		DisableResize:    true,
		BackgroundColour: look.background(),
		Mac: application.MacWindow{
			Appearance:  look.macAppearance(),
			WindowClass: application.MacWindowClassPanel,
			PanelPreferences: application.MacPanelPreferences{
				FloatingPanel: true,
				NonActivating: true,
			},
			CornerType: application.MacWindowCornerTypeSquare,
			CollectionBehavior: application.MacWindowCollectionBehaviorCanJoinAllSpaces |
				application.MacWindowCollectionBehaviorFullScreenAuxiliary |
				application.MacWindowCollectionBehaviorIgnoresCycle,
		},
		KeyBindings: map[string]func(application.Window){
			"CmdOrCtrl+W": func(w application.Window) { w.Close() },
		},
	}
}

// dropdownURL is the page the drop-down terminal's window loads: the
// popped-out terminal's, in its drop-down form.
func dropdownURL(itemID string) string {
	return "/?" + url.Values{"terminal": {itemID}, "dropdown": {"1"}}.Encode()
}

// acknowledgementsWindowOptions returns the options for the Acknowledgements
// window, which shows the licences of the software that Bench includes,
// with a look. The app menu leaves ⌘W to Web items, so the window binds it
// itself, to close. Wails matches a binding by the key's place on a US
// keyboard, so on a layout that moves W, such as French AZERTY, it's the
// key in W's place.
func acknowledgementsWindowOptions(look windowLook) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             "acknowledgements",
		Title:            acknowledgementsLabel,
		Width:            ackWidth,
		Height:           ackHeight,
		MinWidth:         ackMinWidth,
		MinHeight:        ackMinHeight,
		URL:              acknowledgementsPage,
		BackgroundColour: look.background(),
		Mac: application.MacWindow{
			Appearance:         look.macAppearance(),
			CollectionBehavior: application.MacWindowCollectionBehaviorFullScreenNone,
		},
		KeyBindings: map[string]func(application.Window){
			"CmdOrCtrl+W": func(w application.Window) { w.Close() },
		},
	}
}

// previewShell is the browser preview's shell, which has no windows of its
// own and no menu bar item.
type previewShell struct {
	// registerShortcuts checks the global shortcuts again (see
	// shortcutsChanged). Set before the app runs.
	registerShortcuts func()
}

func (*previewShell) bringForward()                          {}
func (*previewShell) popOutTerminal(string, string) error    { return nil }
func (*previewShell) closeTerminalWindow(string)             {}
func (*previewShell) showDropdown(string, string, int) error { return nil }
func (*previewShell) hideDropdown(string)                    {}
func (*previewShell) setMenuBar(workspace.MenuBar)           {}
func (*previewShell) fillScreen()                            {}
func (*previewShell) moveToScreen(layout.Rect, bool)         {}
func (*previewShell) setAppearance(string)                   {}

// shortcutsChanged checks the global shortcuts again, on a goroutine of its
// own, as the app registers them: the preview's run nothing, as the
// frontend handles keys itself (previewRegistrar), but checking them gives
// Settings ▸ Keyboard and the notices what's wrong with the [shortcuts]
// table.
func (s *previewShell) shortcutsChanged() {
	if f := s.registerShortcuts; f != nil {
		go f()
	}
}

// quit does nothing: the preview never asks before quitting, so there's
// nothing to answer. It quits when its server is stopped.
func (*previewShell) quit() {}

// relaunch does nothing: the engine refuses Quit and Reopen Bench in the
// preview, which has no Screen Recording permission to apply.
func (*previewShell) relaunch() {}

// appShell is the macOS app's shell: Bench's main window, popped-out
// terminals' windows, the Acknowledgements window, and the menu bar item.
type appShell struct {
	app *application.App
	// main is set before the app runs.
	main *application.WebviewWindow
	// switchTab and toast are the engine's SwitchTab and the emitter's
	// Toast, and openPalette and openSettings open the jump palette and
	// Settings, for the menu bar item's menu. Set before the app runs.
	switchTab    func(index int) error
	toast        func(message string)
	openPalette  func()
	openSettings func()
	// quitting is set when Bench starts quitting, so that closing the main
	// window then really closes it, and closing terminals' windows isn't
	// taken for the user closing them.
	quitting atomic.Bool
	// passQuit is set once Bench is to quit without asking about its
	// terminals (see quit), so that ShouldQuit lets the quit through.
	passQuit atomic.Bool
	// zoomed is whether the main window filled its screen when it last
	// resized (see keepMinimum).
	zoomed atomic.Bool

	// termMu guards terms. It's never held while waiting for the main
	// thread.
	termMu sync.Mutex
	terms  map[string]*application.WebviewWindow
	// terminalClosed is the engine's TerminalWindowClosed, which the shell
	// calls when the user closes a terminal's window. Set before the app
	// runs.
	terminalClosed func(itemID string)
	// askToCloseTerminal is the engine's AskToCloseTerminal, which the
	// shell calls as the user closes a terminal's window, before it closes.
	// Set before the app runs.
	askToCloseTerminal func(itemID string) bool
	// tagPopOut, if not nil, is given each terminal's window's NSWindow,
	// with the terminal's item ID, once it has one (Options.TagPopOut).
	// Set before the app runs.
	tagPopOut func(nsWindow unsafe.Pointer, itemID string)
	// slideDown and slideUp show and hide the drop-down terminal's window
	// (Options.SlideDown and SlideUp), and toggleDropdown is the engine's
	// ToggleDropdown, for the menu bar item's menu. Set before the app
	// runs.
	slideDown      func(nsWindow unsafe.Pointer, fraction float64)
	slideUp        func(nsWindow unsafe.Pointer)
	toggleDropdown func() error
	// drop is the drop-down terminal's window, which terms holds too, or
	// nil. Guarded by termMu.
	drop *dropdownWindow
	// askToQuit and askToQuitNow are the engine's AskToQuit and
	// AskToQuitNow, and loggingOut the platform's LoggingOut, which
	// shouldQuit asks. Set before the app runs.
	askToQuit    func() bool
	askToQuitNow func() (quit, answered bool)
	loggingOut   func() bool
	// quitApp quits the app: App.Quit, which waits for the main thread.
	quitApp func()
	// openAgain starts what opens Bench again once it has quit
	// (reopenAfterQuit), for relaunch.
	openAgain func() error
	// registerShortcuts registers the global shortcuts again, in place of
	// the ones registered before. It waits for the main thread. Set before
	// the app runs.
	registerShortcuts func()

	// ackMu guards ack, the Acknowledgements window while it's open. It's
	// never held while waiting for the main thread.
	ackMu sync.Mutex
	ack   *application.WebviewWindow

	// trayMu serialises changes to the menu bar item, which wait for the
	// main thread, so the main thread must never take it.
	trayMu sync.Mutex
	// tray is the menu bar item, once it's been shown: none is added while
	// the menu_bar setting hides it.
	tray       *application.SystemTray
	trayItems  []*application.MenuItem // One for each tab.
	trayTotal  *application.MenuItem   // The unread total, or nil.
	trayDrop   *application.MenuItem   // Show Drop-down.
	trayMenu   workspace.MenuBar       // What the tray's menu was built for.
	trayHidden bool                    // Hidden since it was added.
	// trayStarted is set once the app runs, so the menu bar item can be
	// added; bar is what it shows, once barSet, when the engine has said.
	trayStarted bool
	bar         workspace.MenuBar
	barSet      bool

	// menu is the app menu, which follows what the engine says, as the
	// menu bar item does. Set before the app runs.
	menu *appMenu

	// lookMu guards appearance, the appearance setting that Bench's
	// windows have: the one the main window opened with, until the engine
	// gives another.
	lookMu     sync.Mutex
	appearance string
	// systemDark reports whether macOS is dark now, for windows that open
	// with the system appearance (Options.SystemDark).
	systemDark func() bool
	// applyAppearance gives a window's NSWindow an appearance, and a
	// background that follows the appearance in use (Options.Appearance).
	// It doesn't wait for the main thread. Set before the app runs, or nil.
	applyAppearance func(nsWindow unsafe.Pointer, appearance string)
	// place moves the main window's NSWindow to another display in one
	// step, without animation, then fills that display with fill
	// (Options.Place). It doesn't wait for the main thread. Set before the
	// app runs, or nil, for Wails' window API to move it.
	place func(nsWindow unsafe.Pointer, x, y, width, height int, fill bool)

	// moves runs the main window's moves and zooms (fillScreen and
	// moveToScreen) one at a time, in the order the engine asks for them.
	moves serial
}

// serial runs functions one at a time, on a goroutine of its own, in the
// order they come, without making the caller wait: the main window's
// moves and zooms each wait for the main thread several times, and one
// that started before another ended would leave the window where neither
// put it, or zoom it and then back, as AppKit's zoom: goes back and forth.
// The zero value is ready to use.
type serial struct {
	mu      sync.Mutex
	queue   []func()
	running bool
}

// run runs f once the functions before it have run.
func (q *serial) run(f func()) {
	q.mu.Lock()
	q.queue = append(q.queue, f)
	start := !q.running
	q.running = true
	q.mu.Unlock()
	if start {
		go q.drain()
	}
}

// drain runs the queued functions until there are none.
func (q *serial) drain() {
	for {
		q.mu.Lock()
		if len(q.queue) == 0 {
			q.running = false
			q.mu.Unlock()
			return
		}
		f := q.queue[0]
		q.queue = q.queue[1:]
		q.mu.Unlock()
		f()
	}
}

// newAppShell returns the shell for app. It opens nothing until openMain.
// systemDark says whether macOS is dark now; nil asks Wails, which says
// light until the app runs, after the main window is made.
func newAppShell(app *application.App, systemDark func() bool) *appShell {
	if systemDark == nil {
		systemDark = func() bool { return app != nil && app.Env.IsDarkMode() }
	}
	return &appShell{
		app:        app,
		terms:      map[string]*application.WebviewWindow{},
		quitApp:    func() { app.Quit() },
		openAgain:  reopenAfterQuit,
		systemDark: systemDark,
	}
}

// attachEvents are a window's events that come once it has an NSWindow:
// whichever comes first attaches it (onceNative).
var attachEvents = []events.WindowEventType{
	events.Common.WindowShow,
	events.Common.WindowFocus,
	events.Common.WindowRuntimeReady,
}

// onceNative calls f once a window has an NSWindow: when it's first shown
// or focused, or its page has loaded the Wails runtime, whichever comes
// first. Wails calls it on a goroutine of its own.
func onceNative(w *application.WebviewWindow, f func(nsWindow unsafe.Pointer)) {
	var once sync.Once
	for _, ev := range attachEvents {
		w.OnWindowEvent(ev, func(*application.WindowEvent) {
			if ns := w.NativeWindow(); ns != nil {
				once.Do(func() { f(ns) })
			}
		})
	}
}

// openMain creates Bench's main window, on the screen that start names.
// Closing it hides it, until Bench quits; the menu bar item's Show Bench,
// the jump shortcut, and clicking Bench in the Dock (reopen) bring it back.
// attach, if not nil, gets the window once it has an
// NSWindow, when it's first shown or focused or its page has loaded the
// Wails runtime. If start says to fill the screen, fill, if not nil, gets
// the window on the main thread before it's first shown, to fill it.
func (s *appShell) openMain(start workspace.WindowStart, attach, fill func(*application.WebviewWindow)) {
	s.lookMu.Lock()
	s.appearance = start.Appearance
	s.lookMu.Unlock()
	w := s.app.Window.NewWithOptions(mainWindowOptions(start, s.systemDark()))
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.quitting.Load() {
			return
		}
		e.Cancel()
		w.Hide()
	})
	onceNative(w, func(ns unsafe.Pointer) {
		if attach != nil {
			attach(w)
		}
		s.keepLook(ns)
	})
	if start.Fill && fill != nil {
		// Wails shows the window once its page has loaded, from a listener
		// of this event, which sends the showing to the main thread. An
		// event's hooks run before its listeners start, and the main thread
		// runs what it's sent in order, so the window fills before it's
		// first shown. Waiting for the main thread here would hold up
		// Wails' window events.
		var once sync.Once
		w.RegisterHook(events.Mac.WebViewDidFinishNavigation, func(*application.WindowEvent) {
			once.Do(func() { application.InvokeAsync(func() { fill(w) }) })
		})
	}
	w.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		s.keepMinimum(w)
	})
	s.main = w
}

// keepMinimum puts the main window's minimum size back once it no longer
// fills its screen. Double-clicking the tab row zooms the window through
// Wails' Maximise, which drops the minimum until Wails' own UnMaximise puts
// it back; un-zooming with the green button, or resizing the window,
// doesn't.
func (s *appShell) keepMinimum(w *application.WebviewWindow) {
	zoomed := w.IsMaximised()
	if s.zoomed.Swap(zoomed) && !zoomed && !s.quitting.Load() {
		w.SetMinSize(mainMinWidth, mainMinHeight)
	}
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

// reopen brings the main window back when the user clicks Bench in the Dock
// while the window is closed, minimised, or out of sight. Wails shows a
// closed window itself only when none of Bench's windows is open, and a
// popped-out page or terminal may be. Wails calls it on a goroutine of its
// own, so it can wait for the main thread.
func (s *appShell) reopen() {
	w := s.main
	if w == nil || s.quitting.Load() {
		return
	}
	if !w.IsVisible() || w.IsMinimised() {
		s.bringForward()
	}
}

// fillScreen zooms the main window to fill its screen, unless it fills it
// already: Wails' Zoom is AppKit's zoom:, which would shrink it back. It
// does so on a goroutine of its own, since zooming waits for the main
// thread, after the window's moves and zooms before it (moves).
func (s *appShell) fillScreen() {
	w := s.main
	if w == nil || s.quitting.Load() {
		return
	}
	s.moves.run(func() {
		if !w.IsMaximised() {
			w.Zoom()
		}
	})
}

// moveToScreen moves the main window to another display, for its content
// area to be at content, in global coordinates, as mainPlace places it,
// then, with fill, zooms it to fill that display's visible area, unless it
// fills it already, as fillScreen does. With place (Options.Place), the
// window goes there in one step, which doesn't animate, and fills the
// display at once, so that it doesn't change size on the display it's
// leaving first. Otherwise Wails' SetSize, which keeps the window's
// bottom-left corner where it is, comes before SetPosition, which places
// its top-left corner, and Wails' Zoom animates. It does so on a goroutine
// of its own, since each step waits for the main thread, after the
// window's moves and zooms before it (moves).
func (s *appShell) moveToScreen(content layout.Rect, fill bool) {
	w := s.main
	if w == nil || s.quitting.Load() || content.W <= 0 || content.H <= 0 {
		return
	}
	s.moves.run(func() {
		x, y, width, height := mainPlace(content)
		if s.place != nil {
			if ns := w.NativeWindow(); ns != nil {
				s.place(ns, x, y, width, height, fill)
				return
			}
		}
		w.SetSize(width, height)
		w.SetPosition(x, y)
		if fill && !w.IsMaximised() {
			w.Zoom()
		}
	})
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
	w = s.app.Window.NewWithOptions(terminalWindowOptions(itemID, title, s.look()))
	// Wails runs an event's hooks, on a goroutine of its own, before its
	// listeners, and cancelling the event stops them, its own listener that
	// closes the window among them.
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.keepTerminalOpen(itemID, w) {
			e.Cancel()
		}
	})
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.terminalClosing(itemID, w)
	})
	// As for the main window (openMain), whichever event comes first once
	// the window has an NSWindow tags it.
	onceNative(w, func(ns unsafe.Pointer) {
		if tag := s.tagPopOut; tag != nil {
			tag(ns, itemID)
		}
		s.keepLook(ns)
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

// terminalClosing handles a terminal's window closing. If the user closed
// it, the engine is told, so that the terminal goes and its session ends,
// as a popped-out web page goes when its window closes. Bench closes the
// window itself when the terminal comes back into a tab or ends, having
// forgotten it first, and closes every window as it quits.
func (s *appShell) terminalClosing(itemID string, w *application.WebviewWindow) {
	if !s.forgetTerminal(itemID, w) || s.quitting.Load() || s.terminalClosed == nil {
		return
	}
	s.terminalClosed(itemID)
}

// keepTerminalOpen reports whether a terminal's window is to stay open as
// the user closes it, because the terminal is running something and Bench
// asks first, in the window (the engine's AskToCloseTerminal). Bench never
// asks as it closes the window itself, having forgotten it first, nor as it
// quits.
func (s *appShell) keepTerminalOpen(itemID string, w *application.WebviewWindow) bool {
	if s.quitting.Load() || s.askToCloseTerminal == nil {
		return false
	}
	s.termMu.Lock()
	mine := s.terms[itemID] == w
	s.termMu.Unlock()
	return mine && !s.askToCloseTerminal(itemID)
}

// forgetTerminal drops a terminal's window, if it's still the one recorded,
// and reports whether it was.
func (s *appShell) forgetTerminal(itemID string, w *application.WebviewWindow) bool {
	s.termMu.Lock()
	defer s.termMu.Unlock()
	if s.terms[itemID] != w {
		return false
	}
	delete(s.terms, itemID)
	s.forgetDropdownLocked(w)
	return true
}

// closeTerminalWindow closes a terminal's window, if it has one: its
// popped-out window, or the drop-down's.
func (s *appShell) closeTerminalWindow(itemID string) {
	s.termMu.Lock()
	w := s.terms[itemID]
	delete(s.terms, itemID)
	s.forgetDropdownLocked(w)
	s.termMu.Unlock()
	if w != nil {
		w.Close()
	}
}

// dropdownWindow is the drop-down terminal's window: w, and its NSWindow,
// once it has one. want is the height, as a fraction of the screen's, that
// it's to show at once it has one, or 0 to stay hidden. Its fields are
// guarded by appShell.termMu.
type dropdownWindow struct {
	w    *application.WebviewWindow
	ns   unsafe.Pointer
	want float64
}

// forgetDropdownLocked forgets the drop-down's window if it's w. termMu
// is held.
func (s *appShell) forgetDropdownLocked(w *application.WebviewWindow) {
	if s.drop != nil && s.drop.w == w {
		s.drop = nil
	}
}

// showDropdown shows the drop-down terminal's window, sliding it down from
// the top of the screen, height percent of the screen's height tall, with
// the keyboard, while the app in front stays in front (decision 72). The
// first time, it makes the window (dropdownWindowOptions), which shows
// once it has an NSWindow. A terminal's popped-out window, if it had one,
// closes.
func (s *appShell) showDropdown(itemID, title string, height int) error {
	if s.slideDown == nil {
		return errors.New("this build of Bench has no drop-down window")
	}
	fraction := float64(height) / 100
	s.termMu.Lock()
	if d := s.drop; d != nil && s.terms[itemID] == d.w {
		ns := d.ns
		if ns == nil {
			d.want = fraction
		}
		s.termMu.Unlock()
		d.w.SetTitle(title)
		if ns != nil {
			s.slideDown(ns, fraction)
		}
		return nil
	}
	s.termMu.Unlock()

	w := s.app.Window.NewWithOptions(dropdownWindowOptions(itemID, title, s.look()))
	d := &dropdownWindow{w: w, want: fraction}
	// As for a popped-out terminal's window (popOutTerminal): ⌘W asks
	// first while the terminal runs something, and closing it closes the
	// terminal.
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if s.keepTerminalOpen(itemID, w) {
			e.Cancel()
		}
	})
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.terminalClosing(itemID, w)
	})
	// The window is hidden, so its page loading the Wails runtime is what
	// gives it its NSWindow.
	onceNative(w, func(ns unsafe.Pointer) { s.dropdownReady(d, ns) })
	s.termMu.Lock()
	old := s.terms[itemID]
	s.terms[itemID] = w
	s.drop = d
	ns, want := d.ns, d.want
	if ns != nil {
		d.want = 0
	}
	s.termMu.Unlock()
	if old != nil {
		old.Close()
	}
	if ns != nil && want > 0 {
		s.slideDown(ns, want)
	}
	return nil
}

// dropdownReady notes the drop-down's NSWindow, once Wails has made it,
// and slides it down if a show has asked for that meanwhile (want), unless
// a hide has come since, or another window has taken its place.
func (s *appShell) dropdownReady(d *dropdownWindow, ns unsafe.Pointer) {
	s.keepLook(ns)
	s.termMu.Lock()
	d.ns = ns
	want := 0.0
	if s.drop == d {
		want, d.want = d.want, 0
	}
	s.termMu.Unlock()
	if want > 0 {
		s.slideDown(ns, want)
	}
}

// hideDropdown slides the drop-down terminal's window up out of sight, and
// the keyboard goes back to the app that had it. A window made just now
// that hasn't shown yet doesn't show.
func (s *appShell) hideDropdown(itemID string) {
	s.termMu.Lock()
	d := s.drop
	if d == nil || s.terms[itemID] != d.w {
		s.termMu.Unlock()
		return
	}
	d.want = 0
	ns := d.ns
	s.termMu.Unlock()
	if ns != nil && s.slideUp != nil {
		s.slideUp(ns)
	}
}

// quitAskWait is how long Bench waits for the engine to say whether it may
// quit (AskToQuit) before it quits without asking, so that an engine that's
// stuck never stops Bench quitting. Tests shorten it.
var quitAskWait = 5 * time.Second

// shouldQuit is Wails' ShouldQuit, which macOS asks on the main thread as
// Bench is about to quit, however that came about: the Bench menu's Quit,
// the menu bar item's, the Dock's, or an Apple Event's. It lets the quit
// through while the user is logging out, restarting, or shutting down,
// which it mustn't stop, and once Bench is to quit without asking
// (passQuit). Otherwise it asks the engine, which stops the quit if a
// terminal is running something, and asks the user, in Bench's window,
// whether to quit all the same. The main thread mustn't wait for the
// engine, so if the engine is busy, it stops the quit and asks again on
// another goroutine (askThenQuit).
func (s *appShell) shouldQuit() bool {
	if s.quitting.Load() || s.passQuit.Load() || s.askToQuit == nil {
		return true
	}
	if s.loggingOut != nil && s.loggingOut() {
		return true
	}
	if s.askToQuitNow != nil {
		if quit, answered := s.askToQuitNow(); answered {
			return quit
		}
	}
	go s.askThenQuit()
	return false
}

// askThenQuit asks the engine whether Bench may quit, and quits, without
// asking again, if it may; if not, the engine has asked the user, and the
// question's Quit quits. If the engine doesn't answer within quitAskWait,
// Bench quits without asking.
func (s *appShell) askThenQuit() {
	answer := make(chan bool, 1)
	go func() { answer <- s.askToQuit() }()
	select {
	case ok := <-answer:
		if !ok {
			return
		}
	case <-time.After(quitAskWait):
		warn(errors.New("quitting without asking about terminals: Bench didn't answer in time"))
	}
	s.quit()
}

// quit quits Bench without asking about its terminals: ShouldQuit lets the
// quit through once passQuit is set. Quitting waits for the main thread, so
// it does so on a goroutine of its own.
func (s *appShell) quit() {
	s.passQuit.Store(true)
	go s.quitApp()
}

// relaunch quits Bench as quit does, once it has started what opens Bench
// again after it has quit (openAgain). If it can't start that, Bench keeps
// running, and a toast says so.
func (s *appShell) relaunch() {
	if err := s.openAgain(); err != nil {
		warn(err)
		if s.toast != nil {
			s.toast(cantReopenText)
		}
		return
	}
	s.quit()
}

// cantReopenText is the toast when Bench can't arrange to open again.
const cantReopenText = "Bench couldn't arrange to open again, so it hasn't quit. Quit Bench, then open it again."

// openAcknowledgements opens the Acknowledgements window, or brings it
// forward if it's open (un-minimising it), so that there's only ever one. It
// never brings back the main window if it's hidden.
func (s *appShell) openAcknowledgements() {
	s.ackMu.Lock()
	w := s.ack
	s.ackMu.Unlock()
	if w != nil {
		if w.IsMinimised() {
			w.UnMinimise()
		}
		w.Show()
		w.Focus()
		return
	}
	w = s.app.Window.NewWithOptions(acknowledgementsWindowOptions(s.look()))
	onceNative(w, s.keepLook)
	// Wails sends Bench's events, terminals' output among them, to every
	// window, and keeps each window's until the Wails runtime in its page
	// says it's ready. This page has no runtime, so they would pile up for
	// as long as the window is open. Sending the runtime's ready message on
	// the page's behalf makes Wails run each event's script at once instead,
	// which in this page does nothing.
	w.HandleMessage("wails:runtime:ready")
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.forgetAcknowledgements(w)
	})
	// Wails runs each menu click on a goroutine of its own, so another
	// click may have opened a window meanwhile.
	s.ackMu.Lock()
	old := s.ack
	s.ack = w
	s.ackMu.Unlock()
	if old != nil {
		old.Close()
	}
}

// forgetAcknowledgements drops the Acknowledgements window as it closes, if
// it's still the one recorded.
func (s *appShell) forgetAcknowledgements(w *application.WebviewWindow) {
	s.ackMu.Lock()
	defer s.ackMu.Unlock()
	if s.ack == w {
		s.ack = nil
	}
}

// shortcutsChanged registers the global shortcuts again, on a goroutine of
// its own, since registering waits for the main thread.
func (s *appShell) shortcutsChanged() {
	if f := s.registerShortcuts; f != nil && !s.quitting.Load() {
		go f()
	}
}

// setMenuBar updates the menu bar item, adding it if it's to be shown and
// the app runs, and the app menu: its tabs, its layout mode, and what its
// items can do now.
func (s *appShell) setMenuBar(m workspace.MenuBar) {
	if s.menu != nil {
		s.menu.set(m)
	}
	s.trayMu.Lock()
	defer s.trayMu.Unlock()
	m.Tabs = slices.Clone(m.Tabs)
	s.bar, s.barSet = m, true
	s.updateTrayLocked()
}

// startTray adds the menu bar item, unless the menu_bar setting hides it.
// Call it once the app is running, off the main thread. Until the engine has
// said what the item shows, it waits, so that an item that's to be hidden
// never appears.
func (s *appShell) startTray() {
	s.trayMu.Lock()
	defer s.trayMu.Unlock()
	s.trayStarted = true
	s.updateTrayLocked()
}

// updateTrayLocked brings the menu bar item up to date with bar, once the
// app runs and the engine has said what it shows. It adds the item the
// first time it's to be shown, with its icon, and hides it while the
// menu_bar setting says to, which keeps the item for when it's shown again.
// It shows the current tab's name and the unread total beside the icon,
// and rebuilds the menu if its items have changed; ticks and unread counts
// change in place, so that a menu that's open stays open while a count
// changes.
func (s *appShell) updateTrayLocked() {
	if !s.trayStarted || !s.barSet {
		return
	}
	if s.bar.Show == config.MenuBarHidden {
		if s.tray != nil && !s.trayHidden {
			s.tray.Hide()
			s.trayHidden = true
		}
		return
	}
	if s.tray == nil {
		s.tray = s.app.SystemTray.New()
		s.tray.SetTemplateIcon(menuBarIcon)
		// macOS keeps a status item's visibility from one launch to the
		// next, and Wails gives every launch's item the same autosave name,
		// so an item hidden before Bench last quit would start hidden.
		s.tray.Show()
	} else if s.trayHidden {
		s.tray.Show()
		s.trayHidden = false
	}
	s.tray.SetLabel(trayLabel(s.bar))
	if s.trayItems == nil || !s.bar.SameMenu(s.trayMenu) {
		s.tray.SetMenu(s.trayMenuLocked())
		return
	}
	if label := s.bar.UnreadLabel(); s.trayTotal != nil && s.trayTotal.Label() != label {
		s.trayTotal.SetLabel(label)
	}
	if label := s.bar.Dropdown.Label(); s.trayDrop != nil && s.trayDrop.Label() != label {
		s.trayDrop.SetLabel(label)
	}
	for i, item := range s.trayItems {
		if label := s.bar.Tabs[i].Label(); item.Label() != label {
			item.SetLabel(label)
		}
		item.SetChecked(i == s.bar.Current)
	}
}

// trayLabel returns the text beside the menu bar item's icon: the current
// tab's name, cut to menuBarNameLength characters, and the unread total
// over every tab, as in "Code · 15" or "Code •", or nothing when the
// menu_bar setting shows the icon alone. While the drop-down terminal
// needs the user, out of sight, a bell follows, which shows with the icon
// alone too (dropdownMark). The text is the item's name for VoiceOver too.
// Wails can't give the item a tooltip or another name on macOS, so the
// menu's first line says what the number is (UnreadLabel).
func trayLabel(m workspace.MenuBar) string {
	mark := ""
	if m.Dropdown.Attention != "" && !m.Dropdown.Shown {
		mark = dropdownMark
	}
	if m.Show == config.MenuBarIcon {
		return strings.TrimSpace(mark)
	}
	name := shorten(m.TabName(), menuBarNameLength)
	switch {
	case m.Unread.Count > 0:
		return name + " · " + m.Unread.Text() + mark
	case m.Unread.Dot:
		return name + " •" + mark
	}
	return name + mark
}

// dropdownMark follows the menu bar item's text while the drop-down
// terminal needs the user: a bell, which VoiceOver reads as "bell", as its
// terminal's bell or a finished command marks a sub-tab.
const dropdownMark = " 🔔"

// shorten cuts text to at most n characters, ending it with an ellipsis
// where it's cut.
func shorten(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return strings.TrimRight(string(runes[:n-1]), " ") + "…"
}

// trayMenuLocked builds the menu bar item's menu for bar: while anything
// is unread, a line that can't be chosen with the unread total over every
// tab, which says what the number beside the icon is; the tabs, with their
// unread counts and shortcuts (choosing one switches to it and shows
// Bench), then Jump to…, which shows Bench too, Show Drop-down, which
// doesn't, and Settings…, then Show Bench and Quit Bench. The shortcuts
// are the global shortcuts, which work whether or not the menu is open.
func (s *appShell) trayMenuLocked() *application.Menu {
	s.trayMenu = s.bar
	menu := application.NewMenu()
	s.trayTotal = nil
	if label := s.bar.UnreadLabel(); label != "" {
		s.trayTotal = menu.Add(label).SetEnabled(false)
		menu.AddSeparator()
	}
	s.trayItems = make([]*application.MenuItem, 0, len(s.bar.Tabs))
	for i, tab := range s.bar.Tabs {
		item := menu.AddRadio(tab.Label(), i == s.bar.Current)
		if tab.Shortcut != "" {
			item.SetAccelerator(tab.Shortcut)
		}
		item.OnClick(func(*application.Context) { s.chooseTab(i) })
		s.trayItems = append(s.trayItems, item)
	}
	if len(s.bar.Tabs) > 0 {
		menu.AddSeparator()
	}
	jump := menu.Add(jumpLabel).OnClick(func(*application.Context) { s.run(s.openPalette) })
	if s.bar.Jump != "" {
		jump.SetAccelerator(s.bar.Jump)
	}
	s.trayDrop = menu.Add(s.bar.Dropdown.Label()).OnClick(func(*application.Context) { s.chooseDropdown() })
	if s.bar.Dropdown.Shortcut != "" {
		s.trayDrop.SetAccelerator(s.bar.Dropdown.Shortcut)
	}
	menu.Add("Settings…").OnClick(func(*application.Context) { s.run(s.openSettings) })
	menu.AddSeparator()
	menu.Add("Show " + appName).OnClick(func(*application.Context) { s.bringForward() })
	menu.Add("Quit " + appName).OnClick(func(*application.Context) { s.app.Quit() })
	return menu
}

// run calls f, if it's set.
func (s *appShell) run(f func()) {
	if f != nil {
		f()
	}
}

// chooseDropdown shows or hides the drop-down terminal from the menu bar
// item, without showing Bench.
func (s *appShell) chooseDropdown() {
	if s.toggleDropdown == nil {
		return
	}
	if err := s.toggleDropdown(); err != nil && s.toast != nil {
		s.toast(err.Error())
	}
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

// look returns how a window of Bench's opens now: with the appearance
// setting that the engine last gave, as macOS is for "system".
func (s *appShell) look() windowLook {
	s.lookMu.Lock()
	appearance := s.appearance
	s.lookMu.Unlock()
	return windowLook{appearance: appearance, systemDark: s.systemDark()}
}

// keepLook gives a window's NSWindow the appearance that Bench's windows
// have now, and a background that follows the appearance in use, which
// for "system" changes as macOS's does: Wails gave the window a fixed one
// as it made it.
func (s *appShell) keepLook(nsWindow unsafe.Pointer) {
	if s.applyAppearance == nil || nsWindow == nil {
		return
	}
	s.applyAppearance(nsWindow, s.look().appearance)
}

// setAppearance gives Bench's windows the appearance setting: the main
// window, popped-out terminals' windows, and Acknowledgements, each of
// whose pages follows its window's appearance. A window that has no
// NSWindow yet gets it once it has one (keepLook). It doesn't wait for the
// main thread.
func (s *appShell) setAppearance(appearance string) {
	s.lookMu.Lock()
	same := s.appearance == appearance
	s.appearance = appearance
	s.lookMu.Unlock()
	if same || s.quitting.Load() {
		return
	}
	windows := []*application.WebviewWindow{s.main}
	s.termMu.Lock()
	for _, w := range s.terms {
		windows = append(windows, w)
	}
	s.termMu.Unlock()
	s.ackMu.Lock()
	windows = append(windows, s.ack)
	s.ackMu.Unlock()
	for _, w := range windows {
		if w != nil {
			s.keepLook(w.NativeWindow())
		}
	}
}
