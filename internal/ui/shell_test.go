//go:build darwin || server

package ui

import (
	"bytes"
	"image/color"
	"image/png"
	"io/fs"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/frontend"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/workspace"
)

func TestMainWindowOptions(t *testing.T) {
	o := mainWindowOptions(workspace.WindowStart{Appearance: config.AppearanceDark}, false)
	if o.Width != 1440 || o.Height != 900 || o.MinWidth != 640 || o.MinHeight != 480 {
		t.Errorf("size %dx%d, minimum %dx%d; want 1440x900, minimum 640x480", o.Width, o.Height, o.MinWidth, o.MinHeight)
	}
	if o.InitialPosition != application.WindowCentered {
		t.Errorf("with no screen, initial position = %v, want centred", o.InitialPosition)
	}
	if o.Mac.TitleBar != application.MacTitleBarHiddenInset {
		t.Errorf("title bar = %+v, want hidden inset", o.Mac.TitleBar)
	}
	if !o.Mac.TitleBar.FullSizeContent {
		t.Error("the content isn't full size, so the traffic lights won't sit in the tab strip")
	}
	if o.BackgroundColour != application.NewRGB(0x12, 0x14, 0x19) {
		t.Errorf("background = %+v, want --bg-window #121419", o.BackgroundColour)
	}
	if o.Mac.Appearance != application.NSAppearanceNameDarkAqua {
		t.Errorf("appearance = %q, want dark", o.Mac.Appearance)
	}
	if o.Mac.CollectionBehavior&application.MacWindowCollectionBehaviorFullScreenNone == 0 {
		t.Error("the window can go into native full screen")
	}
}

// TestWindowLook checks each appearance setting's look for Bench's windows:
// their own appearance, which their pages follow, and their background
// while their pages load, which matches the page's --bg-window.
func TestWindowLook(t *testing.T) {
	dark, light := application.NewRGB(0x12, 0x14, 0x19), application.NewRGB(0xF5, 0xF6, 0xF8)
	tests := []struct {
		appearance string
		systemDark bool
		mac        application.MacAppearanceType
		background application.RGBA
	}{
		{config.AppearanceDark, false, application.NSAppearanceNameDarkAqua, dark},
		{config.AppearanceLight, true, application.NSAppearanceNameAqua, light},
		// System leaves the window's appearance to macOS.
		{config.AppearanceSystem, true, application.DefaultAppearance, dark},
		{config.AppearanceSystem, false, application.DefaultAppearance, light},
		// Before the engine has said, or for a setting Bench doesn't know,
		// dark, as Bench always was.
		{"", false, application.NSAppearanceNameDarkAqua, dark},
	}
	for _, tc := range tests {
		look := windowLook{appearance: tc.appearance, systemDark: tc.systemDark}
		if got := look.macAppearance(); got != tc.mac {
			t.Errorf("%q, macOS dark %v: appearance %q, want %q", tc.appearance, tc.systemDark, got, tc.mac)
		}
		if got := look.background(); got != tc.background {
			t.Errorf("%q, macOS dark %v: background %+v, want %+v", tc.appearance, tc.systemDark, got, tc.background)
		}
		start := workspace.WindowStart{Appearance: tc.appearance}
		for _, o := range []application.WebviewWindowOptions{
			mainWindowOptions(start, tc.systemDark), terminalWindowOptions("w4", "zsh", look), acknowledgementsWindowOptions(look),
		} {
			if o.Mac.Appearance != tc.mac || o.BackgroundColour != tc.background {
				t.Errorf("%q: the %s window opens with %q and %+v, want %q and %+v", tc.appearance, o.Name, o.Mac.Appearance, o.BackgroundColour, tc.mac, tc.background)
			}
		}
	}
}

// TestSystemDarkBeforeTheAppRuns checks that the main window, which is made
// before the app runs, when Wails says macOS is light whatever it is, opens
// with the dark background for "system" on a dark Mac: the shell asks the
// platform (Options.SystemDark).
func TestSystemDarkBeforeTheAppRuns(t *testing.T) {
	s := newAppShell(nil, func() bool { return true })
	start := workspace.WindowStart{Appearance: config.AppearanceSystem}
	if o := mainWindowOptions(start, s.systemDark()); o.BackgroundColour != darkBackground {
		t.Errorf("the main window opens with %+v, want the dark background", o.BackgroundColour)
	}
	s.setAppearance(config.AppearanceSystem)
	if look := s.look(); !look.systemDark || look.background() != darkBackground {
		t.Errorf("look = %+v, want macOS dark", look)
	}
	// Without a platform to ask, Wails is asked, which says light with no
	// app.
	if newAppShell(nil, nil).systemDark() {
		t.Error("macOS is dark with no app to ask")
	}
}

// TestSetAppearance checks that the shell keeps the appearance the engine
// gives, for the windows it opens next, and gives it to the windows it has
// only when it changes.
func TestSetAppearance(t *testing.T) {
	s := newAppShell(nil, nil)
	var applied []string
	s.applyAppearance = func(_ unsafe.Pointer, appearance string) { applied = append(applied, appearance) }
	s.setAppearance(config.AppearanceLight)
	if got := s.look(); got.appearance != config.AppearanceLight || got.systemDark {
		t.Errorf("look = %+v, want light, with macOS light, as with no app", got)
	}
	s.setAppearance(config.AppearanceLight)
	// No window has an NSWindow here, so nothing is given one yet: each
	// gets it as it does (keepLook).
	if len(applied) != 0 {
		t.Errorf("applied %v to windows that have no NSWindow", applied)
	}
	s.keepLook(unsafe.Pointer(&applied))
	if !slices.Equal(applied, []string{config.AppearanceLight}) {
		t.Errorf("keepLook applied %v, want light", applied)
	}
	s.keepLook(nil)
	if len(applied) != 1 {
		t.Errorf("keepLook applied %v to no window", applied)
	}
}

// TestMainWindowCompactTitleBar checks that the compact density's title bar
// is the unified compact toolbar's shorter band, which keeps the traffic
// lights centred in it, and is otherwise the hidden inset one.
func TestMainWindowCompactTitleBar(t *testing.T) {
	o := mainWindowOptions(workspace.WindowStart{CompactTitleBar: true}, false)
	want := application.MacTitleBarHiddenInset
	want.ToolbarStyle = application.MacToolbarStyleUnifiedCompact
	if o.Mac.TitleBar != want {
		t.Errorf("compact title bar = %+v, want %+v", o.Mac.TitleBar, want)
	}
	if application.MacTitleBarHiddenInset.ToolbarStyle != application.MacToolbarStyleAutomatic {
		t.Error("changed Wails' own hidden inset title bar")
	}
}

// TestMainWindowOnScreen checks that the main window opens with its content
// area where the engine says (WindowStart.Frame), which macOS can tile into
// half a laptop's screen: Wails' web view, which is the content area, sticks
// out 1 pt above the window, so the window is 1 pt lower (mainPlace).
func TestMainWindowOnScreen(t *testing.T) {
	big := platform.Screen{ID: "2", Visible: layout.Rect{X: 1512, Y: 25, W: 2560, H: 1415}}
	tests := []struct {
		name       string
		screen     platform.Screen
		frame      layout.Rect
		x, y, w, h int
	}{
		// In the middle of a 2560x1440 display to the right of the
		// built-in one, with the menu bar at the top.
		{"big", big, layout.Rect{X: 2072, Y: 282.5, W: 1440, H: 900}, 2072, 284, 1440, 900},
		// A display above the primary one, so it has negative coordinates.
		{"above", platform.Screen{ID: "3", Visible: layout.Rect{X: -200, Y: -1080, W: 1920, H: 1055}},
			layout.Rect{X: 40, Y: -1004, W: 1440, H: 900}, 40, -1003, 1440, 900},
		// Tiled into the left half of a 13-inch MacBook's screen.
		{"half", platform.Screen{ID: "1", Visible: layout.Rect{Y: 33, W: 1470, H: 923}},
			layout.Rect{Y: 33, W: 735, H: 923}, 0, 34, 735, 923},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := mainWindowOptions(workspace.WindowStart{Screen: tc.screen, Frame: tc.frame}, false)
			if o.InitialPosition != application.WindowXY {
				t.Errorf("initial position = %v, want X and Y", o.InitialPosition)
			}
			if o.X != tc.x || o.Y != tc.y || o.Width != tc.w || o.Height != tc.h {
				t.Errorf("window at %d,%d %dx%d, want %d,%d %dx%d", o.X, o.Y, o.Width, o.Height, tc.x, tc.y, tc.w, tc.h)
			}
			if o.MinWidth != 640 || o.MinHeight != 480 || o.MinWidth > tc.w {
				t.Errorf("minimum %dx%d, want 640x480, no wider than the window", o.MinWidth, o.MinHeight)
			}
			if o.Screen != nil {
				t.Error("Screen is set, which makes X and Y relative to it")
			}
		})
	}
	// Without a frame, as with no screens at all, it opens in the middle
	// of the primary screen, at the usual size.
	if o := mainWindowOptions(workspace.WindowStart{Screen: big}, false); o.InitialPosition != application.WindowCentered || o.Width != 1440 {
		t.Errorf("with no frame: initial position %v, width %d; want centred, 1440", o.InitialPosition, o.Width)
	}
}

// TestMovesRunInOrder checks that the main window's moves and zooms
// (appShell.moves) run one at a time, in the order the engine asks for
// them, as a move back to a display and its Undo straight after can come,
// and that asking doesn't wait for the moves before to end.
func TestMovesRunInOrder(t *testing.T) {
	var q serial
	var mu sync.Mutex
	var got []int
	var running atomic.Int32
	release, done := make(chan struct{}), make(chan struct{})
	move := func(i int) func() {
		return func() {
			if running.Add(1) > 1 {
				t.Errorf("move %d ran while another was running", i)
			}
			if i == 0 {
				<-release
			}
			time.Sleep(time.Millisecond)
			mu.Lock()
			got = append(got, i)
			mu.Unlock()
			running.Add(-1)
		}
	}
	for i := range 5 {
		q.run(move(i))
	}
	// The first move is still waiting, as for the main thread, and asking
	// for the others didn't wait for it.
	close(release)
	q.run(func() { close(done) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the moves didn't all run")
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(got, []int{0, 1, 2, 3, 4}) {
		t.Errorf("the moves ran as %v, want in order", got)
	}

	// Once it has nothing to run, it starts again for the next.
	again := make(chan struct{})
	q.run(func() { close(again) })
	select {
	case <-again:
	case <-time.After(5 * time.Second):
		t.Fatal("a move after the others didn't run")
	}
}

func TestTerminalWindow(t *testing.T) {
	o := terminalWindowOptions("w 4&x", "zsh — ~/code", windowLook{})
	if o.Title != "zsh — ~/code" {
		t.Errorf("title = %q, want the item's title", o.Title)
	}
	u, err := url.Parse(o.URL)
	if err != nil || u.Path != "/" || u.Query().Get("terminal") != "w 4&x" {
		t.Errorf("URL = %q (%v), want / with terminal=<item ID>", o.URL, err)
	}
	if got := terminalURL("w4"); got != "/?terminal=w4" {
		t.Errorf("terminalURL(w4) = %q, want /?terminal=w4", got)
	}
	// ⌘W closes the window, which asks first while the terminal is busy,
	// as the close button does.
	keys := slices.Sorted(maps.Keys(o.KeyBindings))
	if !slices.Equal(keys, []string{"CmdOrCtrl+W"}) {
		t.Fatalf("key bindings %q, want just CmdOrCtrl+W", keys)
	}
	w := &closeRecorder{}
	o.KeyBindings["CmdOrCtrl+W"](w)
	if w.closed != 1 {
		t.Errorf("⌘W closed the window %d times, want once", w.closed)
	}
}

func TestAcknowledgementsWindowOptions(t *testing.T) {
	o := acknowledgementsWindowOptions(windowLook{appearance: config.AppearanceDark})
	if o.Name != "acknowledgements" || o.Title != acknowledgementsLabel {
		t.Errorf("name %q, title %q; want acknowledgements, titled as the menu item", o.Name, o.Title)
	}
	if o.URL != "/acknowledgements.html" {
		t.Errorf("URL = %q, want /acknowledgements.html", o.URL)
	}
	if _, err := fs.Stat(frontend.FS, strings.TrimPrefix(o.URL, "/")); err != nil {
		t.Errorf("the window's page isn't embedded: %v", err)
	}
	if o.Width != 640 || o.Height != 720 || o.MinWidth != 420 || o.MinHeight != 320 {
		t.Errorf("size %dx%d, minimum %dx%d; want 640x720, minimum 420x320", o.Width, o.Height, o.MinWidth, o.MinHeight)
	}
	if o.BackgroundColour != application.NewRGB(0x12, 0x14, 0x19) {
		t.Errorf("background = %+v, want --bg-window #121419", o.BackgroundColour)
	}
	if o.Mac.Appearance != application.NSAppearanceNameDarkAqua {
		t.Errorf("appearance = %q, want dark", o.Mac.Appearance)
	}
	if o.Mac.CollectionBehavior&application.MacWindowCollectionBehaviorFullScreenNone == 0 {
		t.Error("the window can go into native full screen")
	}

	keys := slices.Sorted(maps.Keys(o.KeyBindings))
	if !slices.Equal(keys, []string{"CmdOrCtrl+W"}) {
		t.Fatalf("key bindings %q, want just CmdOrCtrl+W", keys)
	}
	w := &closeRecorder{}
	o.KeyBindings["CmdOrCtrl+W"](w)
	if w.closed != 1 {
		t.Errorf("⌘W closed the window %d times, want once", w.closed)
	}
}

// closeRecorder is a window that counts how often it's closed. Its other
// methods are the embedded Window's, which is nil, so calling one panics.
type closeRecorder struct {
	application.Window
	closed int
}

func (w *closeRecorder) Close() { w.closed++ }

func TestTrayMenu(t *testing.T) {
	var switched []int
	var opened []string
	s := newAppShell(nil, nil)
	s.switchTab = func(index int) error {
		switched = append(switched, index)
		return nil
	}
	s.openPalette = func() { opened = append(opened, "palette") }
	s.openSettings = func() { opened = append(opened, "settings") }
	s.bar = workspace.MenuBar{
		Current: 1,
		Tabs: []workspace.MenuBarTab{
			{Name: "Comms", Shortcut: "Ctrl+Option+1", Unread: workspace.UnreadView{Count: 15}},
			{Name: "Code", Shortcut: "Ctrl+Option+2"},
			{Name: "Research", Unread: workspace.UnreadView{Dot: true}},
		},
		Unread: workspace.UnreadView{Count: 15},
		Jump:   "Ctrl+Option+Space",
	}
	menu := s.trayMenuLocked()

	type row struct {
		label   string
		checked bool
		keys    string
	}
	var got []row
	for i := 0; ; i++ {
		item := menu.ItemAt(i)
		if item == nil {
			break
		}
		if item.IsSeparator() {
			got = append(got, row{label: "-"})
			continue
		}
		got = append(got, row{item.Label(), item.Checked(), item.GetAccelerator()})
	}
	// Wails writes accelerators with the names of this system's keys.
	keys := func(accelerator string) string {
		return application.NewMenuItem("").SetAccelerator(accelerator).GetAccelerator()
	}
	want := []row{
		{label: "15 Unread in All Tabs"}, {label: "-"},
		{"Comms — 15 Unread", false, keys("Ctrl+Option+1")}, {"Code", true, keys("Ctrl+Option+2")}, {"Research — Unread", false, ""},
		{label: "-"}, {label: "Jump to…", keys: keys("Ctrl+Option+Space")}, {label: "Settings…"},
		{label: "-"}, {label: "Show Bench"}, {label: "Quit Bench"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("menu bar item's menu = %v, want %v", got, want)
	}
	if len(s.trayItems) != 3 {
		t.Errorf("tracked %d tab items, want 3", len(s.trayItems))
	}
	if s.trayTotal != menu.ItemAt(0) || s.trayTotal.Enabled() {
		t.Errorf("the unread total's line = %+v, want the first item, which can't be chosen", s.trayTotal)
	}
	if !s.trayMenu.SameMenu(s.bar) {
		t.Errorf("the menu was built for %+v, want %+v", s.trayMenu, s.bar)
	}
	// Unread counts change the items' labels in place, so the menu isn't
	// built again for them.
	later := s.bar
	later.Tabs = slices.Clone(s.bar.Tabs)
	later.Tabs[0].Unread = workspace.UnreadView{Count: 16}
	if !later.SameMenu(s.trayMenu) {
		t.Error("a new unread count should keep the same menu")
	}
	later.Tabs[0].Name = "Mail"
	if later.SameMenu(s.trayMenu) {
		t.Error("a renamed tab should need a new menu")
	}
	later = s.bar
	later.Unread = workspace.UnreadView{}
	if later.SameMenu(s.trayMenu) {
		t.Error("with nothing unread, the menu should lose the unread total's line")
	}

	s.chooseTab(2)
	if !slices.Equal(switched, []int{2}) {
		t.Errorf("choosing Research switched to %v, want [2]", switched)
	}
	s.run(s.openPalette)
	s.run(s.openSettings)
	if !slices.Equal(opened, []string{"palette", "settings"}) {
		t.Errorf("opened %v, want the palette, then Settings", opened)
	}
}

// TestTrayCountsChangeInPlace checks that a change to the unread counts
// relabels the menu bar item's menu items in place, rather than build the
// menu again, which would close it while it's open: it changes every few
// seconds.
func TestTrayCountsChangeInPlace(t *testing.T) {
	s := newAppShell(nil, nil)
	// A menu bar item that Wails hasn't added keeps its label and menu.
	s.tray = &application.SystemTray{}
	s.trayStarted, s.barSet = true, true
	s.bar = workspace.MenuBar{
		Show:    config.MenuBarName,
		Current: 1,
		Tabs: []workspace.MenuBarTab{
			{Name: "Comms", Unread: workspace.UnreadView{Count: 15}},
			{Name: "Code"},
		},
		Unread: workspace.UnreadView{Count: 15},
	}
	s.updateTrayLocked()
	items, total := slices.Clone(s.trayItems), s.trayTotal
	if len(items) != 2 || total == nil || s.tray.Label() != "Code · 15" {
		t.Fatalf("menu bar item %q, with %d tab items and the total %v; want Code · 15, 2, and a total", s.tray.Label(), len(items), total)
	}

	s.bar.Tabs = []workspace.MenuBarTab{
		{Name: "Comms", Unread: workspace.UnreadView{Count: 16}},
		{Name: "Code", Unread: workspace.UnreadView{Dot: true}},
	}
	s.bar.Unread = workspace.UnreadView{Count: 16}
	s.updateTrayLocked()
	if !slices.Equal(s.trayItems, items) || s.trayTotal != total {
		t.Fatal("new unread counts built the menu again")
	}
	if got := []string{s.tray.Label(), total.Label(), items[0].Label(), items[1].Label()}; !slices.Equal(got,
		[]string{"Code · 16", "16 Unread in All Tabs", "Comms — 16 Unread", "Code — Unread"}) {
		t.Errorf("labels = %q, want the new counts", got)
	}

	// With nothing unread, the menu loses the total's line.
	s.bar.Tabs = []workspace.MenuBarTab{{Name: "Comms"}, {Name: "Code"}}
	s.bar.Unread = workspace.UnreadView{}
	s.updateTrayLocked()
	if s.trayTotal != nil || s.tray.Label() != "Code" || s.trayItems[0].Label() != "Comms" {
		t.Errorf("with nothing unread: menu bar item %q, total %v, Comms %q; want Code, no total, and Comms", s.tray.Label(), s.trayTotal, s.trayItems[0].Label())
	}
}

func TestTrayMenuWithoutTabs(t *testing.T) {
	s := newAppShell(nil, nil)
	menu := s.trayMenuLocked()
	if first := menu.ItemAt(0); first == nil || first.Label() != "Jump to…" || first.GetAccelerator() != "" {
		t.Errorf("first item = %v, want Jump to…, with no separator before it and no shortcut", first)
	}
}

// TestTrayLabel checks the text beside the menu bar item's icon: the
// current tab's name, cut short with an ellipsis, and the unread total, or
// nothing with menu_bar set to icon.
func TestTrayLabel(t *testing.T) {
	bar := func(show, name string) workspace.MenuBar {
		return workspace.MenuBar{Show: show, Tabs: []workspace.MenuBarTab{{Name: name}}}
	}
	unread := func(m workspace.MenuBar, u workspace.UnreadView) workspace.MenuBar {
		m.Unread = u
		return m
	}
	tests := []struct {
		name string
		bar  workspace.MenuBar
		want string
	}{
		{"short name", bar("name", "Code"), "Code"},
		{"sixteen characters", bar("name", "Sixteen chars ok"), "Sixteen chars ok"},
		{"long name", bar("name", "Research and writing"), "Research and wr…"},
		{"space before the cut", bar("name", "Research papers to read"), "Research papers…"},
		{"accents count as one", bar("name", "Café réunion déjà vue"), "Café réunion dé…"},
		{"icon only", bar("icon", "Code"), ""},
		{"before the setting is known", bar("", "Code"), "Code"},
		{"no tab", workspace.MenuBar{Show: "name", Current: 2}, ""},
		{"unread", unread(bar("name", "Code"), workspace.UnreadView{Count: 15}), "Code · 15"},
		{"more unread", unread(bar("name", "Code"), workspace.UnreadView{Count: 99, More: true}), "Code · 99+"},
		{"unread without a count", unread(bar("name", "Code"), workspace.UnreadView{Dot: true}), "Code •"},
		{"long name, unread", unread(bar("name", "Research and writing"), workspace.UnreadView{Count: 3}), "Research and wr… · 3"},
		{"icon only, unread", unread(bar("icon", "Code"), workspace.UnreadView{Count: 3}), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := trayLabel(tc.bar); got != tc.want {
				t.Errorf("trayLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTrayWaits checks that the menu bar item isn't added until the app
// runs and the engine has said what it shows, and never while the menu_bar
// setting hides it. Adding it would need the app, which the shell doesn't
// have here.
func TestTrayWaits(t *testing.T) {
	s := newAppShell(nil, nil)
	s.startTray()
	s.setMenuBar(workspace.MenuBar{Show: "hidden", Tabs: []workspace.MenuBarTab{{Name: "Code"}}})
	if s.tray != nil {
		t.Error("the menu bar item was added while hidden")
	}
	s = newAppShell(nil, nil)
	s.setMenuBar(workspace.MenuBar{Show: "name"})
	if s.tray != nil || s.trayStarted {
		t.Error("the menu bar item was added before the app ran")
	}
}

// TestMenuBarIcon checks the menu bar item's icon, which
// build/darwin/menubar/main.go draws: a template image at two pixels a
// point for the menu bar's 22 points, black where it's drawn.
func TestMenuBarIcon(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(menuBarIcon))
	if err != nil {
		t.Fatalf("decoding menubar.png: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 44 || b.Dy() != 44 {
		t.Fatalf("menubar.png is %d by %d pixels, want 44 by 44", b.Dx(), b.Dy())
	}
	// Points well inside each shape, away from anti-aliased edges.
	tests := []struct {
		name   string
		x, y   int
		opaque bool
	}{
		{"outside the panes", 1, 1, false},
		{"the main pane's outline", 6, 21, true},
		{"inside the main pane", 13, 21, false},
		{"the top side pane", 32, 13, true},
		{"between the side panes", 32, 21, false},
		{"the bottom side pane", 32, 30, true},
	}
	for _, tt := range tests {
		c := color.NRGBAModel.Convert(img.At(tt.x, tt.y)).(color.NRGBA)
		want := color.NRGBA{}
		if tt.opaque {
			want = color.NRGBA{A: 0xFF}
		}
		if c != want {
			t.Errorf("%s at (%d, %d) = %v, want %v", tt.name, tt.x, tt.y, c, want)
		}
	}
}

func TestTerminalWindowClosing(t *testing.T) {
	var reported []string
	s := newAppShell(nil, nil)
	s.terminalClosed = func(itemID string) { reported = append(reported, itemID) }
	w4, w5, w6 := &application.WebviewWindow{}, &application.WebviewWindow{}, &application.WebviewWindow{}
	s.terms["w4"], s.terms["w6"] = w4, w6

	// The user closes w4's window.
	s.terminalClosing("w4", w4)
	// Bench closed w5's window itself, having forgotten it.
	s.terminalClosing("w5", w5)
	// w6's window closes as Bench quits.
	s.quitting.Store(true)
	s.terminalClosing("w6", w6)

	if !slices.Equal(reported, []string{"w4"}) {
		t.Errorf("closes reported to the engine: %v, want only the user's, w4", reported)
	}
	if len(s.terms) != 0 {
		t.Errorf("windows still recorded: %v", s.terms)
	}
}

func TestAcknowledgementsWindowClosing(t *testing.T) {
	s := newAppShell(nil, nil)
	old, open := &application.WebviewWindow{}, &application.WebviewWindow{}
	s.ack = open

	// A window that Bench closed itself, having recorded another since.
	s.forgetAcknowledgements(old)
	if s.ack != open {
		t.Error("closing a window that's no longer recorded forgot the one that is")
	}
	// The user closes the window that's recorded.
	s.forgetAcknowledgements(open)
	if s.ack != nil {
		t.Error("the window is still recorded after closing")
	}
}

// TestFrontendIsEmbedded checks that every file of the frontend folder,
// except Go files and the dot and underscore files that go:embed leaves
// out, is in frontend.FS, so that a file added to the folder isn't missing
// from the app.
func TestFrontendIsEmbedded(t *testing.T) {
	root := filepath.Join("..", "..", "frontend")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || strings.HasSuffix(name, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, err := fs.Stat(frontend.FS, filepath.ToSlash(rel)); err != nil {
			t.Errorf("%s isn't embedded: add it to the go:embed line in frontend/embed.go", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(frontend.FS, "embed.go"); err == nil {
		t.Error("frontend.FS serves embed.go")
	}
}
