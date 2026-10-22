//go:build darwin || server

package ui

import (
	"io/fs"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/frontend"
)

func TestMainWindowOptions(t *testing.T) {
	o := mainWindowOptions()
	if o.Width != 1440 || o.Height != 900 || o.MinWidth != 900 || o.MinHeight != 600 {
		t.Errorf("size %dx%d, minimum %dx%d; want 1440x900, minimum 900x600", o.Width, o.Height, o.MinWidth, o.MinHeight)
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
	s := newAppShell(nil)
	s.switchTab = func(index int) error {
		switched = append(switched, index)
		return nil
	}
	s.tab, s.trayTabs = "Code", []string{"Comms", "Code", "Research"}
	menu := s.trayMenuLocked()

	type row struct {
		label   string
		checked bool
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
		got = append(got, row{item.Label(), item.Checked()})
	}
	want := []row{
		{"Comms", false}, {"Code", true}, {"Research", false},
		{label: "-"}, {label: "Show Bench"}, {label: "Quit Bench"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("menu bar item's menu = %v, want %v", got, want)
	}
	if len(s.trayItems) != 3 {
		t.Errorf("tracked %d tab items, want 3", len(s.trayItems))
	}

	s.chooseTab(2)
	if !slices.Equal(switched, []int{2}) {
		t.Errorf("choosing Research switched to %v, want [2]", switched)
	}
}

func TestTrayMenuWithoutTabs(t *testing.T) {
	s := newAppShell(nil)
	menu := s.trayMenuLocked()
	if first := menu.ItemAt(0); first == nil || first.Label() != "Show Bench" {
		t.Errorf("first item = %v, want Show Bench with no separator before it", first)
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
