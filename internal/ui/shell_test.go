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
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/frontend"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/workspace"
)

func TestMainWindowOptions(t *testing.T) {
	o := mainWindowOptions(platform.Screen{})
	if o.Width != 1440 || o.Height != 900 || o.MinWidth != 900 || o.MinHeight != 600 {
		t.Errorf("size %dx%d, minimum %dx%d; want 1440x900, minimum 900x600", o.Width, o.Height, o.MinWidth, o.MinHeight)
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
	if o.Mac.CollectionBehavior&application.MacWindowCollectionBehaviorFullScreenNone == 0 {
		t.Error("the window can go into native full screen")
	}
}

// TestMainWindowOnScreen checks that the main window opens in the middle of
// the visible area of the screen it was last on, and no bigger than it.
func TestMainWindowOnScreen(t *testing.T) {
	tests := []struct {
		name       string
		visible    layout.Rect
		x, y, w, h int
	}{
		// A 2560x1440 display to the right of the built-in one, with the
		// menu bar at the top.
		{"big", layout.Rect{X: 1512, Y: 25, W: 2560, H: 1415}, 2072, 283, 1440, 900},
		// A display above the primary one, so it has negative coordinates.
		{"above", layout.Rect{X: -200, Y: -1080, W: 1920, H: 1055}, 40, -1003, 1440, 900},
		// A display smaller than the window's usual size.
		{"small", layout.Rect{X: 0, Y: 25, W: 1280.5, H: 775}, 0, 25, 1280, 775},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := mainWindowOptions(platform.Screen{ID: "2", Visible: tc.visible})
			if o.InitialPosition != application.WindowXY {
				t.Errorf("initial position = %v, want X and Y", o.InitialPosition)
			}
			if o.X != tc.x || o.Y != tc.y || o.Width != tc.w || o.Height != tc.h {
				t.Errorf("window at %d,%d %dx%d, want %d,%d %dx%d", o.X, o.Y, o.Width, o.Height, tc.x, tc.y, tc.w, tc.h)
			}
			if o.MinWidth != 900 || o.MinHeight != 600 {
				t.Errorf("minimum %dx%d, want 900x600", o.MinWidth, o.MinHeight)
			}
			if o.Screen != nil {
				t.Error("Screen is set, which makes X and Y relative to it")
			}
		})
	}
}

func TestTerminalWindow(t *testing.T) {
	o := terminalWindowOptions("w 4&x", "zsh — ~/code")
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
}

func TestAcknowledgementsWindowOptions(t *testing.T) {
	o := acknowledgementsWindowOptions()
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
	s := newAppShell(nil)
	s.switchTab = func(index int) error {
		switched = append(switched, index)
		return nil
	}
	s.openPalette = func() { opened = append(opened, "palette") }
	s.openSettings = func() { opened = append(opened, "settings") }
	s.bar = workspace.MenuBar{
		Current: 1,
		Tabs: []workspace.MenuBarTab{
			{Name: "Comms", Shortcut: "Ctrl+Option+1"},
			{Name: "Code", Shortcut: "Ctrl+Option+2"},
			{Name: "Research"},
		},
		Jump: "Ctrl+Option+Space",
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
		{"Comms", false, keys("Ctrl+Option+1")}, {"Code", true, keys("Ctrl+Option+2")}, {"Research", false, ""},
		{label: "-"}, {label: "Jump to…", keys: keys("Ctrl+Option+Space")}, {label: "Settings…"},
		{label: "-"}, {label: "Show Bench"}, {label: "Quit Bench"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("menu bar item's menu = %v, want %v", got, want)
	}
	if len(s.trayItems) != 3 {
		t.Errorf("tracked %d tab items, want 3", len(s.trayItems))
	}
	if !s.trayMenu.SameMenu(s.bar) {
		t.Errorf("the menu was built for %+v, want %+v", s.trayMenu, s.bar)
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

func TestTrayMenuWithoutTabs(t *testing.T) {
	s := newAppShell(nil)
	menu := s.trayMenuLocked()
	if first := menu.ItemAt(0); first == nil || first.Label() != "Jump to…" || first.GetAccelerator() != "" {
		t.Errorf("first item = %v, want Jump to…, with no separator before it and no shortcut", first)
	}
}

// TestTrayLabel checks the text beside the menu bar item's icon: the
// current tab's name, cut short with an ellipsis, or nothing with menu_bar
// set to icon.
func TestTrayLabel(t *testing.T) {
	bar := func(show, name string) workspace.MenuBar {
		return workspace.MenuBar{Show: show, Tabs: []workspace.MenuBarTab{{Name: name}}}
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
	s := newAppShell(nil)
	s.startTray()
	s.setMenuBar(workspace.MenuBar{Show: "hidden", Tabs: []workspace.MenuBarTab{{Name: "Code"}}})
	if s.tray != nil {
		t.Error("the menu bar item was added while hidden")
	}
	s = newAppShell(nil)
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
	s := newAppShell(nil)
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
	s := newAppShell(nil)
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
