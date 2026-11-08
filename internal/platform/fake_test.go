package platform

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
)

var epoch = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func newTestFake(t *testing.T) (*Fake, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(epoch)
	return NewFake(clk), clk
}

func kinds(evs []Event) []EventKind {
	out := make([]EventKind, len(evs))
	for i, ev := range evs {
		out[i] = ev.Kind
	}
	return out
}

func titles(ws []Window) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Title
	}
	return out
}

func methods(calls []Call) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.Method
	}
	return out
}

func mustWindow(t *testing.T, f *Fake, id WindowID) Window {
	t.Helper()
	w, err := f.Window(id)
	if err != nil {
		t.Fatalf("Window(%d): %v", id, err)
	}
	return w
}

func TestNewFakeDefaults(t *testing.T) {
	f, _ := newTestFake(t)
	screens := f.Screens()
	want := Screen{
		ID:      "1",
		Name:    "Built-in Retina Display",
		Frame:   layout.Rect{W: 1512, H: 982},
		Visible: layout.Rect{Y: 25, W: 1512, H: 957},
		Primary: true,
	}
	if len(screens) != 1 || screens[0] != want {
		t.Fatalf("Screens() = %+v, want [%+v]", screens, want)
	}
	if got := f.ScreenOfWindow(); got != want {
		t.Errorf("ScreenOfWindow() = %+v, want %+v", got, want)
	}
	if got := f.ContentFrame(); got != want.Visible {
		t.Errorf("ContentFrame() = %+v, want the visible area %+v", got, want.Visible)
	}
	if !f.Trusted(false) {
		t.Error("Trusted() = false, want true by default")
	}
	if f.ScreenRecordingAllowed(false) {
		t.Error("ScreenRecordingAllowed() = true, want false by default")
	}
	if got := f.DefaultBrowser(); got != "" {
		t.Errorf("DefaultBrowser() = %q, want none by default", got)
	}
	if f.ScrollBarsShown() {
		t.Error("ScrollBarsShown() = true, want false by default, as with a trackpad")
	}
	if f.StageManager() {
		t.Error("StageManager() = true, want false by default")
	}
	if m := f.HeldModifiers(); m != 0 {
		t.Errorf("HeldModifiers() = %v, want none by default", m)
	}
	if f.MouseHeld() {
		t.Error("MouseHeld() = true, want false by default")
	}
	if apps := f.RunningApps(); len(apps) != 0 {
		t.Errorf("RunningApps() = %+v, want none", apps)
	}
	if cap(f.Events()) != FakeEventBuffer {
		t.Errorf("Events() capacity = %d, want %d", cap(f.Events()), FakeEventBuffer)
	}
	if NewFake(nil) == nil {
		t.Error("NewFake(nil) = nil")
	}
}

func TestScreens(t *testing.T) {
	laptop := Screen{ID: "laptop", Frame: layout.Rect{W: 1512, H: 982}, Visible: layout.Rect{Y: 33, W: 1512, H: 949}}
	monitor := Screen{ID: "monitor", Frame: layout.Rect{X: 1512, W: 2560, H: 1440}, Visible: layout.Rect{X: 1512, Y: 25, W: 2560, H: 1415}}

	tests := []struct {
		name        string
		benchScreen string
		content     *layout.Rect
		wantScreen  string
		wantContent layout.Rect
	}{
		{name: "primary by default", wantScreen: "laptop", wantContent: laptop.Visible},
		{name: "set by ID", benchScreen: "monitor", wantScreen: "monitor", wantContent: monitor.Visible},
		{name: "unknown ID falls back to primary", benchScreen: "nope", wantScreen: "laptop", wantContent: laptop.Visible},
		{
			name:        "the screen holding most of the content frame",
			content:     &layout.Rect{X: 1400, Y: 100, W: 1000, H: 800},
			wantScreen:  "monitor",
			wantContent: layout.Rect{X: 1400, Y: 100, W: 1000, H: 800},
		},
		{
			name:        "no window",
			content:     &layout.Rect{},
			wantScreen:  "laptop",
			wantContent: layout.Rect{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := newTestFake(t)
			f.SetScreens(laptop, monitor)
			f.SetBenchScreen(tt.benchScreen)
			if tt.content != nil {
				f.SetContentFrame(*tt.content)
			}
			if got := f.ScreenOfWindow().ID; got != tt.wantScreen {
				t.Errorf("ScreenOfWindow() = %q, want %q", got, tt.wantScreen)
			}
			if got := f.ContentFrame(); got != tt.wantContent {
				t.Errorf("ContentFrame() = %+v, want %+v", got, tt.wantContent)
			}
		})
	}

	t.Run("first screen becomes primary", func(t *testing.T) {
		f, _ := newTestFake(t)
		f.SetScreens(laptop, monitor)
		got := f.Screens()
		if !got[0].Primary || got[1].Primary {
			t.Errorf("Screens() = %+v, want only the first primary", got)
		}
		if evs := f.DrainEvents(); len(evs) != 0 {
			t.Errorf("SetScreens sent %v, want no events", kinds(evs))
		}
	})
}

func TestAppSetUp(t *testing.T) {
	tests := []struct {
		name          string
		setUp         func(f *Fake)
		wantRunning   []App
		wantInstalled []AppInfo
	}{
		{
			name:          "running and installed by default",
			setUp:         func(f *Fake) { f.AddApp("dev.zed.Zed", "Zed") },
			wantRunning:   []App{{PID: 1000, BundleID: "dev.zed.Zed", Name: "Zed", Regular: true}},
			wantInstalled: []AppInfo{{BundleID: "dev.zed.Zed", Name: "Zed", Path: "/Applications/Zed.app"}},
		},
		{
			name: "not running",
			setUp: func(f *Fake) {
				f.AddApp("dev.zed.Zed", "Zed", AppNotRunning())
			},
			wantInstalled: []AppInfo{{BundleID: "dev.zed.Zed", Name: "Zed", Path: "/Applications/Zed.app"}},
		},
		{
			name: "not installed, not regular, hidden",
			setUp: func(f *Fake) {
				f.AddApp("com.example.helper", "Helper", AppNotInstalled(), AppNotRegular(), AppHidden())
			},
			wantRunning: []App{{PID: 1000, BundleID: "com.example.helper", Name: "Helper", Hidden: true}},
		},
		{
			name: "installed only, with a path, sorted by name ignoring case",
			setUp: func(f *Fake) {
				f.InstallApp("com.apple.Terminal", "Terminal", AppPath("/System/Applications/Utilities/Terminal.app"))
				f.InstallApp("com.apple.iCal", "calendar")
			},
			wantInstalled: []AppInfo{
				{BundleID: "com.apple.iCal", Name: "calendar", Path: "/Applications/calendar.app"},
				{BundleID: "com.apple.Terminal", Name: "Terminal", Path: "/System/Applications/Utilities/Terminal.app"},
			},
		},
		{
			name: "a web app, with its browser and address",
			setUp: func(f *Fake) {
				f.InstallApp("com.google.Chrome.app.abc", "Squoosh", AppWebApp("Chrome"), AppWebURL("https://squoosh.app/"))
			},
			wantInstalled: []AppInfo{{BundleID: "com.google.Chrome.app.abc", Name: "Squoosh", Path: "/Applications/Squoosh.app",
				WebApp: "Chrome", WebURL: "https://squoosh.app/"}},
		},
		{
			name: "running name differs from installed name",
			setUp: func(f *Fake) {
				f.InstallApp("com.microsoft.VSCode", "Visual Studio Code")
				f.AddApp("com.microsoft.VSCode", "VS Code")
			},
			wantRunning:   []App{{PID: 1000, BundleID: "com.microsoft.VSCode", Name: "VS Code", Regular: true}},
			wantInstalled: []AppInfo{{BundleID: "com.microsoft.VSCode", Name: "Visual Studio Code", Path: "/Applications/Visual Studio Code.app"}},
		},
		{
			name: "in launch order",
			setUp: func(f *Fake) {
				f.AddApp("b.second", "B", AppNotRunning())
				f.AddApp("a.first", "A")
				f.AddApp("b.second", "B")
			},
			wantRunning: []App{
				{PID: 1000, BundleID: "a.first", Name: "A", Regular: true},
				{PID: 1001, BundleID: "b.second", Name: "B", Regular: true},
			},
			wantInstalled: []AppInfo{
				{BundleID: "a.first", Name: "A", Path: "/Applications/A.app"},
				{BundleID: "b.second", Name: "B", Path: "/Applications/B.app"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := newTestFake(t)
			tt.setUp(f)
			if got := f.RunningApps(); !slices.Equal(got, tt.wantRunning) {
				t.Errorf("RunningApps() = %+v, want %+v", got, tt.wantRunning)
			}
			if got := f.InstalledApps(); !slices.Equal(got, tt.wantInstalled) {
				t.Errorf("InstalledApps() = %+v, want %+v", got, tt.wantInstalled)
			}
			if evs := f.DrainEvents(); len(evs) != 0 {
				t.Errorf("set-up sent %v, want no events", kinds(evs))
			}
		})
	}
}

func TestOpenAppLaunchesAfterDelay(t *testing.T) {
	f, clk := newTestFake(t)
	f.InstallApp("md.obsidian", "Obsidian", AppLaunchDelay(2*time.Second), AppLaunchWindows("Daily note", "Ideas"))

	if err := f.OpenApp("md.obsidian", ""); err != nil {
		t.Fatalf("OpenApp: %v", err)
	}
	if err := f.OpenApp("md.obsidian", ""); err != nil {
		t.Fatalf("OpenApp again: %v", err)
	}
	if f.IsRunning("md.obsidian") || !f.IsLaunching("md.obsidian") {
		t.Fatal("app should be launching, not running, before its delay")
	}
	clk.Advance(1999 * time.Millisecond)
	if f.IsRunning("md.obsidian") {
		t.Fatal("app is running before its delay is up")
	}
	clk.Advance(time.Millisecond)
	if !f.IsRunning("md.obsidian") || f.IsLaunching("md.obsidian") {
		t.Fatal("app isn't running after its delay")
	}
	if got := clk.Pending(); got != 0 {
		t.Errorf("%d timers pending, want 0 (one launch only)", got)
	}

	evs := f.DrainEvents()
	if want := []EventKind{AppLaunched, WindowCreated, WindowCreated, AppActivated}; !slices.Equal(kinds(evs), want) {
		t.Fatalf("events = %v, want %v", kinds(evs), want)
	}
	if f.ActiveApp() != "md.obsidian" {
		t.Errorf("ActiveApp() = %q, want the launched app", f.ActiveApp())
	}
	if evs[0].App.BundleID != "md.obsidian" || evs[0].App.PID == 0 {
		t.Errorf("AppLaunched app = %+v", evs[0].App)
	}
	if evs[1].Window.Title != "Daily note" || evs[1].App.BundleID != "md.obsidian" {
		t.Errorf("first WindowCreated = %+v", evs[1])
	}
	app, _ := f.App("md.obsidian")
	ws, err := f.Windows(app)
	if err != nil {
		t.Fatalf("Windows: %v", err)
	}
	if got, want := titles(ws), []string{"Daily note", "Ideas"}; !slices.Equal(got, want) {
		t.Errorf("windows = %q, want %q", got, want)
	}
	for _, w := range ws {
		if w.PID != app.PID || !w.Standard() {
			t.Errorf("window %+v: want PID %d and a standard window", w, app.PID)
		}
	}
	// The last window opened is in front, so it's the focused one.
	if ws[0].Focused || !ws[1].Focused {
		t.Errorf("focused = %v, %v; want only the frontmost window, %q", ws[0].Focused, ws[1].Focused, ws[1].Title)
	}
}

func TestOpenApp(t *testing.T) {
	t.Run("without a delay, launches before returning", func(t *testing.T) {
		f, _ := newTestFake(t)
		f.InstallApp("com.figma.Desktop", "Figma", AppLaunchWindows("Untitled"))
		if err := f.OpenApp("com.figma.Desktop", ""); err != nil {
			t.Fatal(err)
		}
		if !f.IsRunning("com.figma.Desktop") {
			t.Fatal("app isn't running")
		}
		if got, want := kinds(f.DrainEvents()), []EventKind{AppLaunched, WindowCreated, AppActivated}; !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
	})

	t.Run("a running app with no windows opens one", func(t *testing.T) {
		f, _ := newTestFake(t)
		f.AddApp("com.brave.Browser", "Brave", AppNewWindowTitle("New tab"))
		f.AddWindow("com.brave.Browser", "elsewhere", WithOtherDesktop())
		if err := f.OpenApp("com.brave.Browser", ""); err != nil {
			t.Fatal(err)
		}
		evs := f.DrainEvents()
		if got, want := kinds(evs), []EventKind{AppActivated, WindowCreated}; !slices.Equal(got, want) {
			t.Fatalf("events = %v, want %v", got, want)
		}
		if evs[1].Window.Title != "New tab" {
			t.Errorf("new window = %q, want %q", evs[1].Window.Title, "New tab")
		}
		if err := f.OpenApp("com.brave.Browser", ""); err != nil {
			t.Fatal(err)
		}
		if evs := f.DrainEvents(); len(evs) != 0 {
			t.Errorf("opening it again with a window sent %v, want nothing", kinds(evs))
		}
	})

	t.Run("a running app is activated and unhidden", func(t *testing.T) {
		f, _ := newTestFake(t)
		f.AddApp("com.brave.Browser", "Brave", AppHidden())
		f.AddWindow("com.brave.Browser", "docs")
		if err := f.OpenApp("com.brave.Browser", ""); err != nil {
			t.Fatal(err)
		}
		if f.IsHidden("com.brave.Browser") || f.ActiveApp() != "com.brave.Browser" {
			t.Errorf("hidden = %v, active = %q; want shown and active", f.IsHidden("com.brave.Browser"), f.ActiveApp())
		}
		if got, want := kinds(f.DrainEvents()), []EventKind{AppActivated}; !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
		if err := f.OpenApp("com.brave.Browser", ""); err != nil {
			t.Fatal(err)
		}
		if evs := f.DrainEvents(); len(evs) != 0 {
			t.Errorf("activating the frontmost app again sent %v", kinds(evs))
		}
	})

	t.Run("URLs are recorded and can open windows", func(t *testing.T) {
		f, clk := newTestFake(t)
		f.InstallApp("md.obsidian", "Obsidian", AppLaunchDelay(time.Second),
			AppURLWindow("obsidian://open?vault=work", "Work"))
		f.AddApp("com.brave.Browser", "Brave", AppURLWindow("https://go.dev", "Go"))
		f.AddWindow("com.brave.Browser", "docs")

		if err := f.OpenApp("com.brave.Browser", "https://go.dev"); err != nil {
			t.Fatal(err)
		}
		if err := f.OpenApp("md.obsidian", "obsidian://open?vault=work"); err != nil {
			t.Fatal(err)
		}
		if err := f.OpenApp("md.obsidian", "obsidian://unknown"); err != nil {
			t.Fatal(err)
		}
		clk.Advance(time.Second)

		want := []OpenedURL{
			{BundleID: "com.brave.Browser", URL: "https://go.dev"},
			{BundleID: "md.obsidian", URL: "obsidian://open?vault=work"},
			{BundleID: "md.obsidian", URL: "obsidian://unknown"},
		}
		if got := f.OpenedURLs(); !slices.Equal(got, want) {
			t.Errorf("OpenedURLs() = %+v, want %+v", got, want)
		}
		var created []string
		for _, ev := range f.DrainEvents() {
			if ev.Kind == WindowCreated {
				created = append(created, ev.Window.Title)
			}
		}
		if want := []string{"Go", "Work"}; !slices.Equal(created, want) {
			t.Errorf("windows created = %q, want %q", created, want)
		}
	})

	t.Run("an unknown app is an error", func(t *testing.T) {
		f, _ := newTestFake(t)
		f.AddApp("com.example.helper", "Helper", AppNotInstalled(), AppNotRunning())
		for _, id := range []string{"com.example.nope", "com.example.helper"} {
			if err := f.OpenApp(id, ""); err == nil {
				t.Errorf("OpenApp(%q) = nil, want an error", id)
			}
		}
		if got := f.CallsTo("OpenApp"); len(got) != 2 {
			t.Errorf("OpenApp calls = %v, want both logged", got)
		}
	})
}

func TestScrollBarsShown(t *testing.T) {
	f, _ := newTestFake(t)
	f.SetScrollBarsShown(true)
	if !f.ScrollBarsShown() {
		t.Error("ScrollBarsShown() = false after SetScrollBarsShown(true)")
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("SetScrollBarsShown sent %+v, want no events", evs)
	}
	f.SetScrollBarsShown(false)
	if f.ScrollBarsShown() {
		t.Error("ScrollBarsShown() = true after SetScrollBarsShown(false)")
	}
}

func TestIcons(t *testing.T) {
	f, _ := newTestFake(t)
	if _, err := f.AppIcon("dev.zed.Zed"); !errors.Is(err, ErrNoIcon) {
		t.Errorf("AppIcon with none set = %v, want ErrNoIcon", err)
	}
	if _, err := f.PageIcon("w1"); !errors.Is(err, ErrNoIcon) {
		t.Errorf("PageIcon with none set = %v, want ErrNoIcon", err)
	}
	f.SetAppIcon("dev.zed.Zed", []byte("zed"))
	f.SetPageIcon("w1", []byte("page"))
	// Bundle IDs are matched ignoring case, as macOS does.
	if png, err := f.AppIcon("dev.zed.zed"); err != nil || string(png) != "zed" {
		t.Errorf("AppIcon = %q, %v; want zed", png, err)
	}
	if png, err := f.PageIcon("w1"); err != nil || string(png) != "page" {
		t.Errorf("PageIcon = %q, %v; want page", png, err)
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("setting icons sent %+v, want no events", evs)
	}
	f.SetAppIcon("dev.zed.Zed", nil)
	f.SetPageIcon("w1", nil)
	if _, err := f.AppIcon("dev.zed.Zed"); !errors.Is(err, ErrNoIcon) {
		t.Errorf("AppIcon after taking it away = %v, want ErrNoIcon", err)
	}
	if _, err := f.PageIcon("w1"); !errors.Is(err, ErrNoIcon) {
		t.Errorf("PageIcon after taking it away = %v, want ErrNoIcon", err)
	}
	if got := f.CallsTo("AppIcon"); len(got) != 3 {
		t.Errorf("AppIcon calls %v, want 3", got)
	}
	// Closing a page's view forgets its icon, as on the Mac.
	f.SetPageIcon("w2", []byte("page"))
	f.CloseWebView("w2")
	if _, err := f.PageIcon("w2"); !errors.Is(err, ErrNoIcon) {
		t.Errorf("PageIcon after CloseWebView = %v, want ErrNoIcon", err)
	}
}

func TestStageManager(t *testing.T) {
	f, _ := newTestFake(t)
	f.SetStageManager(true)
	if !f.StageManager() {
		t.Error("StageManager() = false after SetStageManager(true)")
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("SetStageManager sent %+v, want no events", evs)
	}
	f.SetStageManager(false)
	if f.StageManager() {
		t.Error("StageManager() = true after SetStageManager(false)")
	}
}

func TestToolPreferences(t *testing.T) {
	f, _ := newTestFake(t)
	if got := f.ToolPreferences("com.knollsoft.Rectangle"); got.Shortcuts != nil || got.Numbers != nil || got.Texts != nil {
		t.Errorf("ToolPreferences() = %+v, want none by default", got)
	}
	shortcuts := map[string]string{"leftHalf": "ctrl+alt+left", "maximize": ""}
	f.SetToolPreferences("com.knollsoft.Rectangle", ToolPreferences{Shortcuts: shortcuts, Numbers: map[string]int64{"alternateDefaultShortcuts": 1}})
	shortcuts["leftHalf"] = "changed"
	got := f.ToolPreferences("COM.KNOLLSOFT.RECTANGLE")
	if got.Shortcuts["leftHalf"] != "ctrl+alt+left" || got.Shortcuts["maximize"] != "" || got.Numbers["alternateDefaultShortcuts"] != 1 {
		t.Errorf("ToolPreferences() = %+v, want what SetToolPreferences set, whatever the case", got)
	}
	got.Numbers["alternateDefaultShortcuts"] = 0
	if f.ToolPreferences("com.knollsoft.Rectangle").Numbers["alternateDefaultShortcuts"] != 1 {
		t.Error("changing what ToolPreferences returned changed the Fake's")
	}
	if calls := f.CallsTo("ToolPreferences"); len(calls) != 3 || calls[0].BundleID != "com.knollsoft.Rectangle" {
		t.Errorf("calls = %+v", calls)
	}
}

// A window tool that lives in the menu bar starts and stops without an
// event, as on a Mac, which doesn't announce such apps.
func TestAgents(t *testing.T) {
	f, _ := newTestFake(t)
	f.InstallApp("com.knollsoft.Rectangle", "Rectangle", AppNotRegular())
	f.InstallApp("dev.zed.Zed", "Zed")
	running := func() bool {
		return slices.ContainsFunc(f.RunningApps(), func(a App) bool { return a.BundleID == "com.knollsoft.Rectangle" })
	}
	if err := f.StartAgent("com.knollsoft.Rectangle"); err != nil {
		t.Fatal(err)
	}
	if !running() {
		t.Error("RunningApps doesn't list the app that StartAgent started")
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("StartAgent sent %+v, want no events", evs)
	}
	if err := f.StartAgent("com.knollsoft.Rectangle"); err == nil {
		t.Error("StartAgent of a running app succeeded")
	}
	if err := f.StartAgent("dev.zed.Zed"); err == nil {
		t.Error("StartAgent of a regular app succeeded")
	}
	if err := f.StartAgent("com.example.Missing"); err == nil {
		t.Error("StartAgent of an app that isn't installed succeeded")
	}
	if err := f.StopAgent("com.knollsoft.Rectangle"); err != nil {
		t.Fatal(err)
	}
	if running() {
		t.Error("RunningApps lists the app that StopAgent stopped")
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("StopAgent sent %+v, want no events", evs)
	}
	if err := f.StopAgent("com.knollsoft.Rectangle"); err == nil {
		t.Error("StopAgent of an app that isn't running succeeded")
	}
}

func TestDockBadges(t *testing.T) {
	f, _ := newTestFake(t)
	f.AddApp("com.apple.mail", "Mail")
	f.InstallApp("com.tinyspeck.slackmacgap", "Slack", AppLaunchWindows("Slack"))
	f.SetDockBadge("com.apple.mail", "8")
	f.SetDockBadge("com.tinyspeck.slackmacgap", "•")
	if got := f.DockBadges(); len(got) != 0 {
		t.Errorf("DockBadges() before WatchDockBadges = %v, want none", got)
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("SetDockBadge while not watching sent %v, want nothing", kinds(evs))
	}

	f.WatchDockBadges(true)
	if !f.WatchingDockBadges() {
		t.Error("WatchingDockBadges() = false after WatchDockBadges(true)")
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{DockBadgesChanged}; !slices.Equal(got, want) {
		t.Errorf("WatchDockBadges(true) sent %v, want %v", got, want)
	}
	// Slack isn't running, so its badge doesn't show.
	if got := f.DockBadges(); len(got) != 1 || got["com.apple.mail"] != "8" {
		t.Errorf("DockBadges() = %v, want Mail's 8 alone", got)
	}

	f.SetDockBadge("com.apple.mail", "8")
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("setting the same badge sent %v, want nothing", kinds(evs))
	}
	f.SetDockBadge("com.apple.mail", "9")
	if got, want := kinds(f.DrainEvents()), []EventKind{DockBadgesChanged}; !slices.Equal(got, want) {
		t.Errorf("a new badge sent %v, want %v", got, want)
	}

	// Slack shows its badge as it opens, after AppLaunched.
	if err := f.LaunchApp("com.tinyspeck.slackmacgap"); err != nil {
		t.Fatal(err)
	}
	if got := kinds(f.DrainEvents()); len(got) < 2 || got[0] != AppLaunched || !slices.Contains(got, DockBadgesChanged) {
		t.Errorf("launching an app with a badge sent %v, want AppLaunched, then DockBadgesChanged", got)
	}
	if got := f.DockBadges(); got["com.tinyspeck.slackmacgap"] != "•" {
		t.Errorf("DockBadges() = %v, want Slack's •", got)
	}
	// Mail's badge goes as it quits, after AppTerminated.
	if err := f.QuitApp("com.apple.mail"); err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{AppTerminated, DockBadgesChanged}; !slices.Equal(got, want) {
		t.Errorf("quitting an app with a badge sent %v, want %v", got, want)
	}
	if got := f.DockBadges(); len(got) != 1 {
		t.Errorf("DockBadges() after Mail quit = %v, want Slack's alone", got)
	}

	// Without Accessibility permission, the Dock can't be read.
	f.SetTrusted(false)
	if got, want := kinds(f.DrainEvents()), []EventKind{DockBadgesChanged}; !slices.Equal(got, want) {
		t.Errorf("losing Accessibility sent %v, want %v", got, want)
	}
	if got := f.DockBadges(); len(got) != 0 {
		t.Errorf("DockBadges() without Accessibility = %v, want none", got)
	}
	f.SetTrusted(true)
	f.DrainEvents()

	f.WatchDockBadges(false)
	if got, want := kinds(f.DrainEvents()), []EventKind{DockBadgesChanged}; !slices.Equal(got, want) {
		t.Errorf("WatchDockBadges(false) sent %v, want %v", got, want)
	}
	if got := f.DockBadges(); len(got) != 0 {
		t.Errorf("DockBadges() after WatchDockBadges(false) = %v, want none", got)
	}
	if got := methods(f.Calls()); !slices.Contains(got, "WatchDockBadges") || !slices.Contains(got, "DockBadges") {
		t.Errorf("calls %v lack WatchDockBadges and DockBadges", got)
	}
}

func TestWebTitles(t *testing.T) {
	f, _ := newTestFake(t)
	f.SetWebTitles(func(address string) string {
		if address == "https://app.slack.com/client" {
			return "(3) general - Slack"
		}
		return ""
	})
	if err := f.ShowWebView("w1", WebView{URL: "https://app.slack.com/client"}, layout.Rect{W: 10, H: 10}); err != nil {
		t.Fatal(err)
	}
	evs := f.DrainEvents()
	if len(evs) != 1 || evs[0].Kind != WebTitle || evs[0].ItemID != "w1" || evs[0].Title != "(3) general - Slack" {
		t.Errorf("ShowWebView sent %+v, want a WebTitle for w1", evs)
	}
	// The same address again, or one without a title, sends nothing.
	if err := f.ShowWebView("w1", WebView{URL: "https://app.slack.com/client"}, layout.Rect{W: 20, H: 10}); err != nil {
		t.Fatal(err)
	}
	if err := f.ShowWebView("w2", WebView{URL: "https://example.com/"}, layout.Rect{W: 10, H: 10}); err != nil {
		t.Fatal(err)
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("ShowWebView sent %v, want nothing", kinds(evs))
	}
}

func TestHeldModifiers(t *testing.T) {
	f, _ := newTestFake(t)
	f.SetHeldModifiers(ModControl | ModOption)
	if m := f.HeldModifiers(); m != ModControl|ModOption {
		t.Errorf("HeldModifiers() = %v, want Control and Option", m)
	}
	f.SetHeldModifiers(0)
	if m := f.HeldModifiers(); m != 0 {
		t.Errorf("HeldModifiers() = %v after SetHeldModifiers(0), want none", m)
	}
}

func TestMouseHeld(t *testing.T) {
	f, _ := newTestFake(t)
	f.SetMouseHeld(true)
	if !f.MouseHeld() {
		t.Error("MouseHeld() = false after SetMouseHeld(true)")
	}
	f.SetMouseHeld(false)
	if f.MouseHeld() {
		t.Error("MouseHeld() = true after SetMouseHeld(false)")
	}
}

func TestModifiersNamed(t *testing.T) {
	cases := []struct {
		names []string
		want  Modifiers
	}{
		{nil, 0},
		{[]string{"ctrl", "alt"}, ModControl | ModOption},
		{[]string{"shift", "cmd", "hyper"}, ModShift | ModCommand},
	}
	for _, c := range cases {
		if got := ModifiersNamed(c.names); got != c.want {
			t.Errorf("ModifiersNamed(%q) = %v, want %v", c.names, got, c.want)
		}
	}
}

func TestSystemKeys(t *testing.T) {
	f, _ := newTestFake(t)
	if k := f.SystemKeys(); k.VoiceOver || len(k.InputSources) != 0 {
		t.Errorf("SystemKeys() = %+v, want none by default", k)
	}
	keys := []string{"ctrl+space", "ctrl+alt+space"}
	f.SetSystemKeys(SystemKeys{InputSources: keys, VoiceOver: true})
	keys[0] = "changed"
	k := f.SystemKeys()
	if !k.VoiceOver || len(k.InputSources) != 2 || k.InputSources[0] != "ctrl+space" {
		t.Errorf("SystemKeys() = %+v, want what SetSystemKeys set, kept apart from the caller's slice", k)
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("SetSystemKeys sent %+v, want no events", evs)
	}
}

func TestLoggingOut(t *testing.T) {
	f, _ := newTestFake(t)
	if f.LoggingOut() {
		t.Error("LoggingOut() = true, want false by default")
	}
	f.SetLoggingOut(true)
	if !f.LoggingOut() {
		t.Error("LoggingOut() = false after SetLoggingOut(true)")
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("SetLoggingOut sent %+v, want no events", evs)
	}
}

func TestSystemSettings(t *testing.T) {
	f, _ := newTestFake(t)
	if f.LoginItemNeedsApproval() {
		t.Error("LoginItemNeedsApproval() = true, want false by default")
	}
	f.SetLoginItemNeedsApproval(true)
	if !f.LoginItemNeedsApproval() {
		t.Error("LoginItemNeedsApproval() = false after SetLoginItemNeedsApproval(true)")
	}
	for _, pane := range []SettingsPane{SettingsAccessibility, SettingsScreenRecording} {
		if err := f.OpenSettings(pane); err != nil {
			t.Fatalf("OpenSettings(%s): %v", pane, err)
		}
	}
	if got := f.SettingsOpened(); !slices.Equal(got, []SettingsPane{SettingsAccessibility, SettingsScreenRecording}) {
		t.Errorf("SettingsOpened() = %v, want accessibility, then screen-recording", got)
	}
	f.SetErr("OpenSettings", ErrUnsupported)
	if err := f.OpenSettings(SettingsLoginItems); !errors.Is(err, ErrUnsupported) {
		t.Errorf("OpenSettings = %v, want the error set", err)
	}
}

func TestDefaultBrowser(t *testing.T) {
	f, _ := newTestFake(t)
	f.SetDefaultBrowser("com.apple.Safari")
	if got := f.DefaultBrowser(); got != "com.apple.Safari" {
		t.Errorf("DefaultBrowser() = %q, want com.apple.Safari", got)
	}
	f.SetDefaultBrowser("")
	if got := f.DefaultBrowser(); got != "" {
		t.Errorf("DefaultBrowser() = %q after setting none, want none", got)
	}
	if got := f.CallsTo("DefaultBrowser"); len(got) != 2 {
		t.Errorf("DefaultBrowser calls = %v, want both logged", got)
	}
}

func TestBrowserMakesWebApps(t *testing.T) {
	f, clk := newTestFake(t)
	f.InstallApp("com.apple.Safari", "Safari", AppLaunchDelay(time.Second), AppMakesWebApps(3*time.Second))
	f.InstallApp("com.brave.Browser", "Brave Browser")
	webApps := func() []AppInfo {
		var out []AppInfo
		for _, a := range f.InstalledApps() {
			if a.WebApp != "" {
				out = append(out, a)
			}
		}
		return out
	}
	for _, url := range []string{"https://www.squoosh.app/?utm_source=bench", "about:blank", "obsidian://open", "https:///nohost"} {
		if err := f.OpenApp("com.apple.Safari", url); err != nil {
			t.Fatal(err)
		}
	}
	// A browser without the option makes none.
	if err := f.OpenApp("com.brave.Browser", "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(2999 * time.Millisecond)
	if got := webApps(); len(got) != 0 {
		t.Fatalf("web apps after 2.999s = %+v, want none yet", got)
	}
	clk.Advance(time.Millisecond)
	want := []AppInfo{{BundleID: "com.apple.Safari.WebApp.1", Name: "Squoosh", Path: "~/Applications/Squoosh.app",
		WebApp: "Safari", WebURL: "https://www.squoosh.app/?utm_source=bench"}}
	if got := webApps(); !slices.Equal(got, want) {
		t.Fatalf("web apps = %+v, want %+v", got, want)
	}
	if f.IsRunning("com.apple.Safari.WebApp.1") {
		t.Error("the new web app is running, want it installed only")
	}
	// Asked again for the same host, the browser makes no other.
	if err := f.OpenApp("com.apple.Safari", "https://squoosh.app/editor"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(3 * time.Second)
	if got := webApps(); !slices.Equal(got, want) {
		t.Errorf("web apps after asking again = %+v, want %+v", got, want)
	}
	// It opens a window titled with its name when it launches, after the
	// browser's launch delay.
	f.DrainEvents()
	if err := f.OpenApp("com.apple.Safari.WebApp.1", ""); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Second)
	var created []string
	for _, ev := range f.DrainEvents() {
		if ev.Kind == WindowCreated {
			created = append(created, ev.Window.Title)
		}
	}
	if !slices.Equal(created, []string{"Squoosh"}) {
		t.Errorf("windows created = %q, want [Squoosh]", created)
	}
}

func TestLaunchAndQuitApp(t *testing.T) {
	f, _ := newTestFake(t)
	f.InstallApp("com.brave.Browser", "Brave", AppLaunchDelay(time.Hour), AppLaunchWindows("New tab"))
	if err := f.LaunchApp("com.brave.Browser"); err != nil {
		t.Fatalf("LaunchApp: %v", err)
	}
	if err := f.LaunchApp("com.brave.Browser"); err == nil {
		t.Error("LaunchApp of a running app = nil, want an error")
	}
	if err := f.LaunchApp("com.example.nope"); err == nil {
		t.Error("LaunchApp of an unknown app = nil, want an error")
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{AppLaunched, WindowCreated, AppActivated}; !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	first, _ := f.App("com.brave.Browser")
	w := f.ZOrder()[0]
	if err := f.FocusWindow(w); err != nil {
		t.Fatal(err)
	}
	f.DrainEvents()

	if err := f.QuitApp("com.brave.Browser"); err != nil {
		t.Fatalf("QuitApp: %v", err)
	}
	evs := f.DrainEvents()
	if want := []EventKind{AppTerminated}; !slices.Equal(kinds(evs), want) {
		t.Fatalf("events = %v, want %v (and no WindowDestroyed)", kinds(evs), want)
	}
	if evs[0].App.PID != first.PID || evs[0].App.Active {
		t.Errorf("AppTerminated app = %+v, want PID %d and not active", evs[0].App, first.PID)
	}
	if _, err := f.Window(w); !errors.Is(err, ErrNoWindow) {
		t.Errorf("Window after quit: %v, want ErrNoWindow", err)
	}
	if f.ActiveApp() != "" || f.IsRunning("com.brave.Browser") || len(f.ZOrder()) != 0 {
		t.Error("quit app is still active, running, or has windows")
	}
	if err := f.QuitApp("com.brave.Browser"); err == nil {
		t.Error("QuitApp of a stopped app = nil, want an error")
	}

	if err := f.LaunchApp("com.brave.Browser"); err != nil {
		t.Fatal(err)
	}
	second, _ := f.App("com.brave.Browser")
	if second.PID == first.PID {
		t.Errorf("relaunched app kept PID %d, want a new one", first.PID)
	}
}

func TestNewWindow(t *testing.T) {
	f, _ := newTestFake(t)
	zed := f.AddApp("dev.zed.Zed", "Zed")
	brave := f.AddApp("com.brave.Browser", "Brave", AppNewWindowTitle("New tab"))
	stopped := f.AddApp("md.obsidian", "Obsidian", AppNotRunning())

	for _, app := range []App{zed, brave} {
		if err := f.NewWindow(app); err != nil {
			t.Fatalf("NewWindow(%s): %v", app.BundleID, err)
		}
	}
	if err := f.NewWindow(stopped); err == nil {
		t.Error("NewWindow of an app that isn't running = nil, want an error")
	}
	evs := f.DrainEvents()
	if got, want := kinds(evs), []EventKind{WindowCreated, WindowCreated}; !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	if evs[0].Window.Title != DefaultNewWindowTitle || evs[1].Window.Title != "New tab" {
		t.Errorf("titles = %q, %q", evs[0].Window.Title, evs[1].Window.Title)
	}
	if got := f.ZOrder()[0]; got != evs[1].Window.ID {
		t.Errorf("front window = %d, want the newest, %d", got, evs[1].Window.ID)
	}
}

func TestWindowsAndWindow(t *testing.T) {
	f, _ := newTestFake(t)
	brave := f.AddApp("com.brave.Browser", "Brave")
	a := f.AddWindow("com.brave.Browser", "Wails v3 docs")
	b := f.AddWindow("com.brave.Browser", "Picture in picture", WithSubrole(SubroleFloating))
	c := f.AddWindow("com.brave.Browser", "Elsewhere", WithOtherDesktop())
	d := f.AddWindow("com.brave.Browser", "Minimised", WithMinimised())
	other := f.AddWindow("dev.zed.Zed", "bench")

	ws, err := f.Windows(brave)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := titles(ws), []string{"Wails v3 docs", "Picture in picture", "Minimised"}; !slices.Equal(got, want) {
		t.Errorf("Windows() = %q, want %q", got, want)
	}
	if ws[1].Standard() || !ws[0].Standard() || !ws[2].Minimised {
		t.Errorf("subroles or minimised wrong: %+v", ws)
	}
	if ws, err := f.Windows(App{PID: brave.PID}); err != nil || len(ws) != 3 {
		t.Errorf("Windows by PID = %d windows, %v; want 3", len(ws), err)
	}
	if _, err := f.Windows(App{BundleID: "com.example.nope"}); err == nil {
		t.Error("Windows of an app that isn't running = nil error")
	}

	if got := mustWindow(t, f, other.ID); got.App != "dev.zed.Zed" || got.PID == brave.PID {
		t.Errorf("window of an app added by AddWindow = %+v", got)
	}
	if created := []int{a.Created, b.Created, c.Created, d.Created, other.Created}; !slices.Equal(created, []int{1, 2, 3, 4, 5}) {
		t.Errorf("Created = %v, want 1 to 5", created)
	}
	if _, err := f.Window(c.ID); !errors.Is(err, ErrOtherDesktop) {
		t.Errorf("Window on another desktop: %v, want ErrOtherDesktop", err)
	}
	if err := f.CloseWindow(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Window(a.ID); !errors.Is(err, ErrNoWindow) {
		t.Errorf("Window after closing: %v, want ErrNoWindow", err)
	}
	if _, err := f.Window(999); !errors.Is(err, ErrNoWindow) {
		t.Errorf("Window(999): %v, want ErrNoWindow", err)
	}
	for _, call := range []func() error{
		func() error { return f.SetFrame(c, layout.Rect{W: 10, H: 10}) },
		func() error { return f.Raise(c) },
		func() error { return f.Park(c, f.ScreenOfWindow()) },
		func() error { return f.Focus(c) },
		func() error { return f.Unminimise(c) },
		func() error { return f.Minimise(c) },
	} {
		if err := call(); !errors.Is(err, ErrOtherDesktop) {
			t.Errorf("acting on a window on another desktop: %v, want ErrOtherDesktop", err)
		}
	}
}

func TestUnplugAndPlugScreen(t *testing.T) {
	laptop := Screen{ID: "laptop", Primary: true, Frame: layout.Rect{W: 1512, H: 982}, Visible: layout.Rect{Y: 33, W: 1512, H: 949}}
	monitor := Screen{ID: "monitor", Frame: layout.Rect{X: 1512, W: 2560, H: 1440}, Visible: layout.Rect{X: 1512, Y: 25, W: 2560, H: 1415}}
	f, _ := newTestFake(t)
	f.SetScreens(laptop, monitor)
	f.SetContentFrame(monitor.Visible)
	if err := f.UnplugScreen("projector"); err == nil {
		t.Error("unplugged a display that isn't connected")
	}

	// macOS moves Bench's window onto the laptop, no larger than its
	// visible area.
	if err := f.UnplugScreen("monitor"); err != nil {
		t.Fatal(err)
	}
	if got := f.Screens(); !slices.Equal(got, []Screen{laptop}) {
		t.Errorf("unplugged: Screens() = %+v, want the laptop's alone", got)
	}
	if got := f.ContentFrame(); got != laptop.Visible || f.ScreenOfWindow().ID != "laptop" {
		t.Errorf("unplugged: ContentFrame() = %+v, want the laptop's visible area", got)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{ScreensChanged, BenchMoved}; !slices.Equal(got, want) {
		t.Errorf("unplugged: events = %v, want %v", got, want)
	}

	// Plugged in again, the monitor is back, and Bench's window stays.
	f.PlugScreen(monitor)
	if got := f.Screens(); !slices.Equal(got, []Screen{laptop, monitor}) {
		t.Errorf("plugged in: Screens() = %+v, want both", got)
	}
	if got := f.ScreenOfWindow().ID; got != "laptop" {
		t.Errorf("plugged in: Bench is on %q, want still the laptop", got)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{ScreensChanged}; !slices.Equal(got, want) {
		t.Errorf("plugged in: events = %v, want %v", got, want)
	}

	// A display that Bench's window isn't on goes without moving it; one
	// plugged in again with the same ID takes its place.
	if err := f.UnplugScreen("monitor"); err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{ScreensChanged}; !slices.Equal(got, want) {
		t.Errorf("unplugged without Bench: events = %v, want %v", got, want)
	}
	f.PlugScreen(monitor)
	moved := monitor
	moved.Frame.X, moved.Visible.X = -2560, -2560
	f.PlugScreen(moved)
	if got := f.Screens(); !slices.Equal(got, []Screen{laptop, moved}) {
		t.Errorf("plugged in twice: Screens() = %+v, want the laptop's and the monitor once", got)
	}

	// Unplugging the primary display makes the first one left primary.
	f.SetBenchScreen("laptop")
	f.DrainEvents()
	if err := f.UnplugScreen("laptop"); err != nil {
		t.Fatal(err)
	}
	if got := f.Screens(); len(got) != 1 || got[0].ID != "monitor" || !got[0].Primary {
		t.Errorf("unplugged the primary display: Screens() = %+v, want the monitor, primary", got)
	}
	if got := f.ScreenOfWindow().ID; got != "monitor" {
		t.Errorf("unplugged the primary display: Bench is on %q, want the monitor", got)
	}
}

func TestDefaultFramesAreOnScreen(t *testing.T) {
	f, _ := newTestFake(t)
	visible := f.ScreenOfWindow().Visible
	for i := range 12 {
		w := f.AddWindow("dev.zed.Zed", fmt.Sprint(i))
		if in := Intersect(w.Frame, visible); in != w.Frame {
			t.Errorf("window %d at %+v isn't inside the visible area %+v", i, w.Frame, visible)
		}
	}
}

func TestSetFrame(t *testing.T) {
	start := layout.Rect{X: 100, Y: 100, W: 800, H: 600}
	tests := []struct {
		name       string
		opts       []WindowOption
		ownMovesOn bool
		to         layout.Rect
		want       layout.Rect
		wantEvents []EventKind
	}{
		{
			name:       "move and resize",
			ownMovesOn: true,
			to:         layout.Rect{X: 9, Y: 75, W: 1000, H: 700},
			want:       layout.Rect{X: 9, Y: 75, W: 1000, H: 700},
			wantEvents: []EventKind{WindowResized, WindowMoved},
		},
		{
			name:       "move only",
			ownMovesOn: true,
			to:         layout.Rect{X: 9, Y: 75, W: 800, H: 600},
			want:       layout.Rect{X: 9, Y: 75, W: 800, H: 600},
			wantEvents: []EventKind{WindowMoved},
		},
		{
			name:       "resize only",
			ownMovesOn: true,
			to:         layout.Rect{X: 100, Y: 100, W: 400, H: 300},
			want:       layout.Rect{X: 100, Y: 100, W: 400, H: 300},
			wantEvents: []EventKind{WindowResized},
		},
		{
			name:       "no change",
			ownMovesOn: true,
			to:         start,
			want:       start,
		},
		{
			name:       "clamped up to the minimum size",
			opts:       []WindowOption{WithMinSize(700, 500)},
			ownMovesOn: true,
			to:         layout.Rect{X: 9, Y: 75, W: 600, H: 600},
			want:       layout.Rect{X: 9, Y: 75, W: 700, H: 600},
			wantEvents: []EventKind{WindowResized, WindowMoved},
		},
		{
			name:       "sized in whole steps, like a terminal",
			opts:       []WindowOption{WithSizeStep(7, 15)},
			ownMovesOn: true,
			to:         layout.Rect{X: 9, Y: 75, W: 1000, H: 700},
			want:       layout.Rect{X: 9, Y: 75, W: 994, H: 690},
			wantEvents: []EventKind{WindowResized, WindowMoved},
		},
		{
			name:       "stepped, then clamped up to the minimum size",
			opts:       []WindowOption{WithSizeStep(10, 0), WithMinSize(705, 0)},
			ownMovesOn: true,
			to:         layout.Rect{X: 100, Y: 100, W: 600, H: 600},
			want:       layout.Rect{X: 100, Y: 100, W: 705, H: 600},
			wantEvents: []EventKind{WindowResized},
		},
		{
			name: "events switched off",
			to:   layout.Rect{X: 9, Y: 75, W: 1000, H: 700},
			want: layout.Rect{X: 9, Y: 75, W: 1000, H: 700},
		},
		{
			name:       "minimised windows move quietly",
			opts:       []WindowOption{WithMinimised()},
			ownMovesOn: true,
			to:         layout.Rect{X: 9, Y: 75, W: 1000, H: 700},
			want:       layout.Rect{X: 9, Y: 75, W: 1000, H: 700},
		},
		{
			name:       "full-screen windows are left alone",
			opts:       []WindowOption{WithFullScreen()},
			ownMovesOn: true,
			to:         layout.Rect{X: 9, Y: 75, W: 1000, H: 700},
			want:       layout.Rect{W: 1512, H: 982},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := newTestFake(t)
			f.SetEmitOwnMoves(tt.ownMovesOn)
			w := f.AddWindow("dev.zed.Zed", "bench", append([]WindowOption{WithFrame(start)}, tt.opts...)...)
			if err := f.SetFrame(w, tt.to); err != nil {
				t.Fatalf("SetFrame: %v", err)
			}
			if got := mustWindow(t, f, w.ID).Frame; got != tt.want {
				t.Errorf("frame = %+v, want %+v", got, tt.want)
			}
			evs := f.DrainEvents()
			if got := kinds(evs); !slices.Equal(got, tt.wantEvents) {
				t.Errorf("events = %v, want %v", got, tt.wantEvents)
			}
			for _, ev := range evs {
				if ev.Window.ID != w.ID || ev.Window.Frame != tt.want || ev.App.BundleID != "dev.zed.Zed" {
					t.Errorf("event %+v doesn't carry the window's new state", ev)
				}
			}
		})
	}
}

func TestRaiseFocusAndZOrder(t *testing.T) {
	f, _ := newTestFake(t)
	a := f.AddWindow("dev.zed.Zed", "a")
	b := f.AddWindow("com.microsoft.VSCode", "b")
	c := f.AddWindow("com.microsoft.VSCode", "c")
	if got, want := f.ZOrder(), []WindowID{c.ID, b.ID, a.ID}; !slices.Equal(got, want) {
		t.Fatalf("ZOrder() = %v, want newest first %v", got, want)
	}

	if err := f.Raise(a); err != nil {
		t.Fatal(err)
	}
	if got, want := f.ZOrder(), []WindowID{a.ID, c.ID, b.ID}; !slices.Equal(got, want) {
		t.Errorf("after Raise(a), ZOrder() = %v, want %v", got, want)
	}
	if f.ActiveApp() != "" || len(f.DrainEvents()) != 0 {
		t.Error("Raise activated the app or sent events")
	}

	f.MinimiseWindow(b.ID)
	f.DrainEvents()
	if err := f.Focus(b); err != nil {
		t.Fatal(err)
	}
	if got, want := f.ZOrder(), []WindowID{b.ID, a.ID, c.ID}; !slices.Equal(got, want) {
		t.Errorf("after Focus(b), ZOrder() = %v, want %v", got, want)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{AppActivated, WindowRestored, WindowFocused}; !slices.Equal(got, want) {
		t.Errorf("Focus events = %v, want %v", got, want)
	}
	if got := mustWindow(t, f, b.ID); !got.Focused || got.Minimised {
		t.Errorf("focused window = %+v, want focused and not minimised", got)
	}
	if mustWindow(t, f, c.ID).Focused {
		t.Error("another window of the app is focused too")
	}

	if err := f.FocusWindow(c.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{WindowFocused}; !slices.Equal(got, want) {
		t.Errorf("focusing another window of the frontmost app sent %v, want %v", got, want)
	}
	if err := f.FocusWindow(a.ID); err != nil {
		t.Fatal(err)
	}
	evs := f.DrainEvents()
	if got, want := kinds(evs), []EventKind{AppActivated, WindowFocused}; !slices.Equal(got, want) {
		t.Errorf("FocusWindow events = %v, want %v", got, want)
	}
	if !evs[1].Window.Focused || mustWindow(t, f, c.ID).Focused {
		t.Error("focus didn't move to the other app's window")
	}
	if len(f.CallsTo("Focus")) != 1 {
		t.Error("FocusWindow was logged as a Platform call")
	}
}

func TestPark(t *testing.T) {
	f, _ := newTestFake(t)
	right := Screen{ID: "2", Frame: layout.Rect{X: 1512, W: 1512, H: 982}, Visible: layout.Rect{X: 1512, Y: 25, W: 1512, H: 957}}
	f.SetScreens(append(f.Screens(), right)...)
	w := f.AddWindow("dev.zed.Zed", "bench", WithFrame(layout.Rect{X: 10, Y: 40, W: 800, H: 600}))
	full := f.AddWindow("dev.zed.Zed", "full", WithFullScreen())
	f.ResetCalls()

	screen := f.Screens()[0]
	if err := f.Park(w, screen); err != nil {
		t.Fatal(err)
	}
	want := ParkFrame(f.Screens(), screen, layout.Rect{W: 800, H: 600})
	if want.X != -799 {
		t.Fatalf("test set-up: expected the bottom-left corner, got %+v", want)
	}
	if got, _ := f.Frame(w.ID); got != want {
		t.Errorf("parked at %+v, want %+v", got, want)
	}
	if got, wantKinds := kinds(f.DrainEvents()), []EventKind{WindowMoved}; !slices.Equal(got, wantKinds) {
		t.Errorf("events = %v, want %v", got, wantKinds)
	}
	calls := f.CallsTo("Park", "SetFrame")
	if len(calls) != 1 || calls[0].Rect != want || calls[0].Arg != "1" {
		t.Errorf("calls = %v, want one Park at %+v on screen 1", calls, want)
	}

	before, _ := f.Frame(full.ID)
	if err := f.Park(full, screen); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.Frame(full.ID); got != before {
		t.Errorf("full-screen window moved to %+v", got)
	}

	// A screen with no frame means the one Bench is on.
	f.SetBenchScreen("2")
	f.ResetCalls()
	if err := f.Park(w, Screen{}); err != nil {
		t.Fatal(err)
	}
	want = ParkFrame(f.Screens(), right, layout.Rect{W: 800, H: 600})
	if got, _ := f.Frame(w.ID); got != want {
		t.Errorf("parked on no screen at %+v, want Bench's screen's corner %+v", got, want)
	}
	if calls := f.CallsTo("Park"); len(calls) != 1 || calls[0].Arg != "2" {
		t.Errorf("calls = %v, want one Park on screen 2", calls)
	}

	if err := f.Park(Window{ID: 999}, screen); !errors.Is(err, ErrNoWindow) {
		t.Errorf("Park of a window that doesn't exist: %v, want ErrNoWindow", err)
	}
}

func TestHideAndUnhide(t *testing.T) {
	f, _ := newTestFake(t)
	app := f.AddApp("com.brave.Browser", "Brave")
	w := f.AddWindow("com.brave.Browser", "docs")
	f.FocusWindow(w.ID)
	f.DrainEvents()

	if err := f.Hide(app); err != nil {
		t.Fatal(err)
	}
	if !f.IsHidden("com.brave.Browser") || f.ActiveApp() != "" {
		t.Error("app isn't hidden, or is still frontmost")
	}
	if got, _ := f.App("com.brave.Browser"); !got.Hidden {
		t.Errorf("App() = %+v, want Hidden", got)
	}
	if ws, _ := f.Windows(app); len(ws) != 1 {
		t.Errorf("hidden app has %d windows, want 1", len(ws))
	}
	if err := f.Unhide(app); err != nil {
		t.Fatal(err)
	}
	if f.IsHidden("com.brave.Browser") || f.ActiveApp() != "" {
		t.Error("Unhide didn't show the app, or activated it")
	}
	if err := f.Hide(App{BundleID: "com.example.nope"}); err == nil {
		t.Error("Hide of an app that isn't running = nil")
	}
	if err := f.Unhide(App{PID: 4242}); err == nil {
		t.Error("Unhide of an unknown PID = nil")
	}
	if got, want := methods(f.CallsTo("Hide", "Unhide")), []string{"Hide", "Unhide", "Hide", "Unhide"}; !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

// TestOwnApp checks that, as on a Mac, RunningApps leaves Bench out, and
// OwnAppHidden reports whether Bench is hidden.
func TestOwnApp(t *testing.T) {
	f, _ := newTestFake(t)
	f.AddApp("com.brave.Browser", "Brave")
	bench := f.AddApp("com.itbm.bench", "Bench")
	if f.OwnAppHidden() {
		t.Error("OwnAppHidden before SetOwnApp = true")
	}
	if got := f.RunningApps(); len(got) != 2 {
		t.Errorf("before SetOwnApp, RunningApps has %d apps, want 2", len(got))
	}
	f.SetOwnApp("com.itbm.bench")
	if got := f.RunningApps(); len(got) != 1 || got[0].BundleID != "com.brave.Browser" {
		t.Errorf("RunningApps = %+v, want only Brave", got)
	}
	if f.OwnAppHidden() {
		t.Error("OwnAppHidden of a shown Bench = true")
	}
	if err := f.Hide(bench); err != nil {
		t.Fatal(err)
	}
	if !f.OwnAppHidden() {
		t.Error("OwnAppHidden of a hidden Bench = false")
	}
}

func TestUnminimise(t *testing.T) {
	f, _ := newTestFake(t)
	w := f.AddWindow("dev.zed.Zed", "bench", WithMinimised())
	if err := f.Unminimise(w); err != nil {
		t.Fatal(err)
	}
	if mustWindow(t, f, w.ID).Minimised {
		t.Error("window is still minimised")
	}
	if err := f.Unminimise(w); err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{WindowRestored}; !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestMinimise(t *testing.T) {
	f, _ := newTestFake(t)
	w := f.AddWindow("dev.zed.Zed", "bench")
	if err := f.Minimise(w); err != nil {
		t.Fatal(err)
	}
	if !mustWindow(t, f, w.ID).Minimised {
		t.Error("window isn't minimised")
	}
	if err := f.Minimise(w); err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{WindowMinimised}; !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	if got := len(f.CallsTo("Minimise")); got != 2 {
		t.Errorf("logged %d Minimise calls, want 2", got)
	}
}

func TestPressClose(t *testing.T) {
	f, _ := newTestFake(t)
	f.AddApp("dev.zed.Zed", "Zed")
	w := f.AddWindow("dev.zed.Zed", "bench")
	asks := f.AddWindow("dev.zed.Zed", "notes", WithCloseAsks())
	hides := f.AddWindow("com.tinyspeck.slackmacgap", "Slack", WithCloseHides())
	away := f.AddWindow("dev.zed.Zed", "away", WithOtherDesktop())

	if err := f.PressClose(w); err != nil {
		t.Fatal(err)
	}
	evs := f.DrainEvents()
	if got, want := kinds(evs), []EventKind{WindowDestroyed}; !slices.Equal(got, want) || evs[0].Window.ID != w.ID {
		t.Errorf("events = %v, want WindowDestroyed for the window", got)
	}
	if _, err := f.Window(w.ID); !errors.Is(err, ErrNoWindow) {
		t.Errorf("Window after PressClose: %v, want ErrNoWindow", err)
	}
	if err := f.PressClose(w); !errors.Is(err, ErrNoWindow) {
		t.Errorf("PressClose of a closed window: %v, want ErrNoWindow", err)
	}

	// One that asks about unsaved changes stays, until the user answers.
	if err := f.PressClose(asks); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Window(asks.ID); err != nil {
		t.Errorf("a window that asks closed: %v", err)
	}
	// One that its app hides goes off screen, saying nothing.
	if err := f.PressClose(hides); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Window(hides.ID); !errors.Is(err, ErrOtherDesktop) {
		t.Errorf("Window of a window its app hid: %v, want ErrOtherDesktop", err)
	}
	if ws, _ := f.Windows(App{BundleID: "com.tinyspeck.slackmacgap"}); len(ws) != 0 {
		t.Errorf("Windows lists %q, want the hidden window left out", titles(ws))
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("events = %v, want none", kinds(evs))
	}
	if err := f.PressClose(away); !errors.Is(err, ErrOtherDesktop) {
		t.Errorf("PressClose on another desktop: %v, want ErrOtherDesktop", err)
	}
	if got := len(f.CallsTo("PressClose")); got != 5 {
		t.Errorf("logged %d PressClose calls, want 5", got)
	}

	// An app that quits with its last window says only that it quit.
	f.AddApp("com.apple.calculator", "Calculator", AppQuitsWithLastWindow())
	one := f.AddWindow("com.apple.calculator", "Calculator")
	two := f.AddWindow("com.apple.calculator", "Paper Tape")
	if err := f.PressClose(one); err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{WindowDestroyed}; !slices.Equal(got, want) {
		t.Errorf("closing one of two windows: events = %v, want %v", got, want)
	}
	if err := f.PressClose(two); err != nil {
		t.Fatal(err)
	}
	if got, want := kinds(f.DrainEvents()), []EventKind{AppTerminated}; !slices.Equal(got, want) {
		t.Errorf("closing the last window: events = %v, want %v", got, want)
	}
	if f.IsRunning("com.apple.calculator") {
		t.Error("the app still runs after its last window closed")
	}
}

func TestScriptedActions(t *testing.T) {
	frame := layout.Rect{X: 100, Y: 100, W: 800, H: 600}
	tests := []struct {
		name       string
		act        func(f *Fake, w Window) error
		wantEvents []EventKind
		check      func(t *testing.T, f *Fake, w Window, evs []Event)
	}{
		{
			name:       "close",
			act:        func(f *Fake, w Window) error { return f.CloseWindow(w.ID) },
			wantEvents: []EventKind{WindowDestroyed},
			check: func(t *testing.T, f *Fake, w Window, evs []Event) {
				if evs[0].Window.Title != "bench" {
					t.Errorf("WindowDestroyed carries %+v, want the window's last state", evs[0].Window)
				}
				if _, ok := f.Frame(w.ID); ok || len(f.ZOrder()) != 0 {
					t.Error("closed window still exists")
				}
			},
		},
		{
			name:       "retitle",
			act:        func(f *Fake, w Window) error { return f.RetitleWindow(w.ID, "bench — main.go") },
			wantEvents: []EventKind{WindowRetitled},
			check: func(t *testing.T, f *Fake, w Window, evs []Event) {
				if evs[0].Window.Title != "bench — main.go" || mustWindow(t, f, w.ID).Title != "bench — main.go" {
					t.Error("title didn't change")
				}
			},
		},
		{
			name: "user drags the window",
			act: func(f *Fake, w Window) error {
				return f.UserMoveWindow(w.ID, layout.Rect{X: 300, Y: 200, W: 800, H: 600})
			},
			wantEvents: []EventKind{WindowMoved},
		},
		{
			name: "user drags even when own moves are quiet",
			act: func(f *Fake, w Window) error {
				f.SetEmitOwnMoves(false)
				return f.UserMoveWindow(w.ID, layout.Rect{X: 300, Y: 200, W: 500, H: 600})
			},
			wantEvents: []EventKind{WindowResized, WindowMoved},
		},
		{
			name:       "move to another desktop",
			act:        func(f *Fake, w Window) error { return f.MoveToOtherDesktop(w.ID) },
			wantEvents: []EventKind{WindowMoved},
			check: func(t *testing.T, f *Fake, w Window, evs []Event) {
				if _, err := f.Window(w.ID); !errors.Is(err, ErrOtherDesktop) {
					t.Errorf("Window: %v, want ErrOtherDesktop", err)
				}
				if err := f.ReturnToDesktop(w.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := f.Window(w.ID); err != nil {
					t.Errorf("Window after returning: %v", err)
				}
			},
		},
		{
			name:       "minimise",
			act:        func(f *Fake, w Window) error { return f.MinimiseWindow(w.ID) },
			wantEvents: []EventKind{WindowMinimised},
			check: func(t *testing.T, f *Fake, w Window, evs []Event) {
				if !evs[0].Window.Minimised {
					t.Error("WindowMinimised window isn't minimised")
				}
			},
		},
		{
			name: "full screen and back",
			act: func(f *Fake, w Window) error {
				if err := f.SetFullScreen(w.ID, true); err != nil {
					return err
				}
				if err := f.SetFullScreen(w.ID, true); err != nil {
					return err
				}
				return f.SetFullScreen(w.ID, false)
			},
			wantEvents: []EventKind{WindowResized, WindowResized},
			check: func(t *testing.T, f *Fake, w Window, evs []Event) {
				if !evs[0].Window.FullScreen || evs[0].Window.Frame != (layout.Rect{W: 1512, H: 982}) {
					t.Errorf("in full screen: %+v", evs[0].Window)
				}
				if got := mustWindow(t, f, w.ID); got.FullScreen || got.Frame != frame {
					t.Errorf("after full screen: %+v, want back at %+v", got, frame)
				}
				if err := f.SetFullScreen(w.ID, true); err != nil {
					t.Fatal(err)
				}
				if err := f.UserMoveWindow(w.ID, frame); err == nil {
					t.Error("moving a full-screen window = nil error")
				}
			},
		},
		{
			name: "open a window",
			act: func(f *Fake, w Window) error {
				_, err := f.OpenWindow("dev.zed.Zed", "settings", WithSubrole(SubroleDialog))
				return err
			},
			wantEvents: []EventKind{WindowCreated},
			check: func(t *testing.T, f *Fake, w Window, evs []Event) {
				got := evs[0].Window
				if got.Title != "settings" || got.Subrole != SubroleDialog || got.Created != w.Created+1 || got.Focused {
					t.Errorf("new window = %+v", got)
				}
				if _, err := f.OpenWindow("com.example.nope", "x"); err == nil {
					t.Error("OpenWindow for an app that isn't running = nil")
				}
			},
		},
		{
			name: "send any event",
			act: func(f *Fake, w Window) error {
				f.EmitEvent(Event{Kind: WebTitle, ItemID: "w1", Title: "# general"})
				f.EmitEvent(Event{Kind: BenchMoved})
				return nil
			},
			wantEvents: []EventKind{WebTitle, BenchMoved},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := newTestFake(t)
			f.AddApp("dev.zed.Zed", "Zed")
			w := f.AddWindow("dev.zed.Zed", "bench", WithFrame(frame))
			if err := tt.act(f, w); err != nil {
				t.Fatalf("action: %v", err)
			}
			evs := f.DrainEvents()
			if got := kinds(evs); !slices.Equal(got, tt.wantEvents) {
				t.Fatalf("events = %v, want %v", got, tt.wantEvents)
			}
			if tt.check != nil {
				tt.check(t, f, w, evs)
			}
			if calls := f.Calls(); len(calls) > 0 && tt.check == nil {
				t.Errorf("scripted actions logged calls: %v", calls)
			}
		})
	}

	t.Run("unknown windows are ErrNoWindow", func(t *testing.T) {
		f, _ := newTestFake(t)
		for name, act := range map[string]func() error{
			"CloseWindow":        func() error { return f.CloseWindow(7) },
			"RetitleWindow":      func() error { return f.RetitleWindow(7, "x") },
			"FocusWindow":        func() error { return f.FocusWindow(7) },
			"UserMoveWindow":     func() error { return f.UserMoveWindow(7, frame) },
			"MoveToOtherDesktop": func() error { return f.MoveToOtherDesktop(7) },
			"MinimiseWindow":     func() error { return f.MinimiseWindow(7) },
			"SetFullScreen":      func() error { return f.SetFullScreen(7, true) },
		} {
			if err := act(); !errors.Is(err, ErrNoWindow) {
				t.Errorf("%s: %v, want ErrNoWindow", name, err)
			}
		}
	})
}

func TestPermissions(t *testing.T) {
	f, _ := newTestFake(t)
	f.SetTrusted(false)
	f.Trusted(false)
	f.Trusted(true)
	f.ScreenRecordingAllowed(true)
	f.ScreenRecordingAllowed(true)
	if got := f.TrustPrompts(); got != 1 {
		t.Errorf("TrustPrompts() = %d, want 1", got)
	}
	if got := f.ScreenRecordingRequests(); got != 2 {
		t.Errorf("ScreenRecordingRequests() = %d, want 2", got)
	}
	f.SetTrusted(true)
	f.SetScreenRecordingAllowed(true)
	if !f.Trusted(true) || !f.ScreenRecordingAllowed(true) {
		t.Error("permissions weren't granted")
	}
	if f.TrustPrompts() != 1 || f.ScreenRecordingRequests() != 2 {
		t.Error("asking with permission granted counted as a prompt")
	}
	if got := f.CallsTo("Trusted"); len(got) != 3 || got[1].Arg != "true" {
		t.Errorf("Trusted calls = %v", got)
	}
}

func TestAccessibilityNeedsTrust(t *testing.T) {
	f, _ := newTestFake(t)
	app := f.AddApp("dev.zed.Zed", "Zed", AppHidden())
	w := f.AddWindow("dev.zed.Zed", "bench", WithFrame(layout.Rect{X: 100, Y: 100, W: 800, H: 600}))
	f.SetTrusted(false)

	for name, call := range map[string]func() error{
		"Windows":    func() error { _, err := f.Windows(app); return err },
		"Window":     func() error { _, err := f.Window(w.ID); return err },
		"SetFrame":   func() error { return f.SetFrame(w, layout.Rect{W: 10, H: 10}) },
		"Raise":      func() error { return f.Raise(w) },
		"Park":       func() error { return f.Park(w, f.Screens()[0]) },
		"Unminimise": func() error { return f.Unminimise(w) },
		"Focus":      func() error { return f.Focus(w) },
		"NewWindow":  func() error { return f.NewWindow(app) },
		"Minimise":   func() error { return f.Minimise(w) },
		"PressClose": func() error { return f.PressClose(w) },
	} {
		if err := call(); !errors.Is(err, ErrNotTrusted) {
			t.Errorf("%s without permission: %v, want ErrNotTrusted", name, err)
		}
	}
	if got, _ := f.Frame(w.ID); got != (layout.Rect{X: 100, Y: 100, W: 800, H: 600}) {
		t.Errorf("window moved to %+v without permission", got)
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("events = %v, want none", kinds(evs))
	}

	// Launching, hiding, and showing apps use NSWorkspace, which doesn't
	// need it.
	if err := f.Unhide(app); err != nil || f.IsHidden("dev.zed.Zed") {
		t.Errorf("Unhide without permission: %v", err)
	}
	if err := f.Hide(app); err != nil {
		t.Errorf("Hide without permission: %v", err)
	}
	if err := f.OpenApp("dev.zed.Zed", ""); err != nil {
		t.Errorf("OpenApp without permission: %v", err)
	}

	f.SetTrusted(true)
	if _, err := f.Window(w.ID); err != nil {
		t.Errorf("Window with permission: %v", err)
	}
}

func TestWebViews(t *testing.T) {
	f, _ := newTestFake(t)
	slack := WebView{URL: "https://app.slack.com/client", Profile: "slack"}
	frame := layout.Rect{X: 9, Y: 90, W: 1000, H: 700}

	if err := f.ShowWebView("w1", slack, frame); err != nil {
		t.Fatal(err)
	}
	moved := layout.Rect{X: 9, Y: 90, W: 600, H: 700}
	if err := f.ShowWebView("w1", slack, moved); err != nil {
		t.Fatal(err)
	}
	want := FakeWebView{ID: "w1", View: slack, Frame: moved, Visible: true, Shows: 2}
	if got := f.WebViews()["w1"]; got != want {
		t.Errorf("after showing, %+v, want %+v", got, want)
	}

	f.HideWebView("w1")
	f.HideWebView("nope")
	if f.WebViews()["w1"].Visible {
		t.Error("hidden web view is visible")
	}

	if err := f.PopOutWebView("w1", "Slack"); err != nil {
		t.Fatal(err)
	}
	if got := f.WebViews()["w1"]; !got.PoppedOut || !got.Visible || got.Title != "Slack" {
		t.Errorf("popped out: %+v", got)
	}
	if err := f.DockWebView("w1"); err != nil {
		t.Fatal(err)
	}
	if got := f.WebViews()["w1"]; got.PoppedOut || got.Visible {
		t.Errorf("docked: %+v, want back in Bench and hidden", got)
	}
	if err := f.PopOutWebView("nope", "x"); err == nil {
		t.Error("PopOutWebView of an unknown view = nil")
	}
	if err := f.DockWebView("nope"); err == nil {
		t.Error("DockWebView of an unknown view = nil")
	}

	f.CloseWebView("w1")
	if len(f.WebViews()) != 0 {
		t.Errorf("WebViews() = %v after closing, want none", f.WebViews())
	}
	got := methods(f.Calls())
	want2 := []string{"ShowWebView", "ShowWebView", "HideWebView", "HideWebView", "PopOutWebView", "DockWebView", "PopOutWebView", "DockWebView", "CloseWebView"}
	if !slices.Equal(got, want2) {
		t.Errorf("calls = %v, want %v", got, want2)
	}
}

// TestLoadWebView checks that LoadWebView makes a web view hidden, with
// its page's events, that it leaves one that exists as it is, and that
// WebViewsMade lists each web view made, in order.
func TestLoadWebView(t *testing.T) {
	f, _ := newTestFake(t)
	f.SetWebPages(func(address string) Page { return Page{Host: "host of " + address, Secure: true} })
	f.SetWebTitles(func(address string) string { return "(3) " + address })
	slack := WebView{URL: "https://app.slack.com/client", Profile: "slack", Zoom: 1.25}
	frame := layout.Rect{X: 0, Y: 40, W: 1000, H: 700}
	if err := f.LoadWebView("w1", slack, frame); err != nil {
		t.Fatal(err)
	}
	want := FakeWebView{ID: "w1", View: slack, Frame: frame, Zoom: 1.25, Loaded: true, KeepRunning: true}
	if got := f.WebViews()["w1"]; got != want {
		t.Errorf("after loading, %+v, want %+v, hidden and kept running", got, want)
	}
	wantEvents := []Event{
		{Kind: WebURL, ItemID: "w1", Body: slack.URL, Page: Page{Host: "host of " + slack.URL, Secure: true}},
		{Kind: WebTitle, ItemID: "w1", Title: "(3) " + slack.URL},
	}
	if got := f.DrainEvents(); !slices.Equal(got, wantEvents) {
		t.Errorf("events = %+v, want %+v", got, wantEvents)
	}

	// Showing it later sends nothing more, as its page has loaded.
	if err := f.ShowWebView("w2", WebView{URL: "https://example.com/"}, frame); err != nil {
		t.Fatal(err)
	}
	if err := f.ShowWebView("w1", slack, frame); err != nil {
		t.Fatal(err)
	}
	if got := f.WebViews()["w1"]; !got.Visible || got.Shows != 1 || !got.Loaded {
		t.Errorf("after showing, %+v, want it visible, shown once, and loaded before", got)
	}
	if err := f.LoadWebView("w2", WebView{URL: "https://other.example/"}, layout.Rect{}); err != nil {
		t.Fatal(err)
	}
	if got := f.WebViews()["w2"]; got.Loaded || got.KeepRunning || !got.Visible || got.View.URL != "https://example.com/" {
		t.Errorf("LoadWebView changed a web view that exists: %+v", got)
	}
	f.DrainEvents()

	// KeepWebViewRunning says whether one keeps running while it's hidden.
	f.KeepWebViewRunning("w2", true)
	f.KeepWebViewRunning("w1", false)
	f.KeepWebViewRunning("nope", true)
	if w1, w2 := f.WebViews()["w1"], f.WebViews()["w2"]; w1.KeepRunning || !w2.KeepRunning {
		t.Errorf("after KeepWebViewRunning, w1 %+v, w2 %+v", w1, w2)
	}
	if got, want := f.CallsTo("KeepWebViewRunning"), []Call{
		{Method: "KeepWebViewRunning", Arg: "w2 true"}, {Method: "KeepWebViewRunning", Arg: "w1 false"},
		{Method: "KeepWebViewRunning", Arg: "nope true"},
	}; !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}

	f.CloseWebView("w1")
	if err := f.LoadWebView("w1", slack, frame); err != nil {
		t.Fatal(err)
	}
	if got, want := f.WebViewsMade(), []string{"w1", "w2", "w1"}; !slices.Equal(got, want) {
		t.Errorf("WebViewsMade() = %v, want %v", got, want)
	}
	if got, want := methods(f.CallsTo("LoadWebView")), []string{"LoadWebView", "LoadWebView", "LoadWebView"}; !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	f.SetErr("LoadWebView", errors.New("no"))
	if err := f.LoadWebView("w3", slack, frame); err == nil || f.WebViews()["w3"] != (FakeWebView{}) {
		t.Errorf("LoadWebView with an error set = %v, and made %+v", err, f.WebViews()["w3"])
	}
}

func TestWebPages(t *testing.T) {
	f, _ := newTestFake(t)
	slack := WebView{URL: "https://app.slack.com/client", Profile: "slack"}
	if err := f.ShowWebView("w1", slack, layout.Rect{W: 10, H: 10}); err != nil {
		t.Fatal(err)
	}
	if evs := f.DrainEvents(); len(evs) != 0 {
		t.Errorf("without SetWebPages, ShowWebView sent %+v", evs)
	}

	f.SetWebPages(func(address string) Page { return Page{Host: "host of " + address, Secure: true} })
	if err := f.ShowWebView("w2", WebView{URL: "https://example.com/"}, layout.Rect{W: 10, H: 10}); err != nil {
		t.Fatal(err)
	}
	if err := f.ShowWebView("w2", WebView{URL: "https://example.com/"}, layout.Rect{W: 20, H: 20}); err != nil {
		t.Fatal(err)
	}
	if err := f.ShowWebView("w1", WebView{URL: "https://app.slack.com/client/T1", Profile: "slack"}, layout.Rect{W: 10, H: 10}); err != nil {
		t.Fatal(err)
	}
	want := []Event{
		{Kind: WebURL, ItemID: "w2", Body: "https://example.com/", Page: Page{Host: "host of https://example.com/", Secure: true}},
		{Kind: WebURL, ItemID: "w1", Body: "https://app.slack.com/client/T1", Page: Page{Host: "host of https://app.slack.com/client/T1", Secure: true}},
	}
	if got := f.DrainEvents(); !slices.Equal(got, want) {
		t.Errorf("events = %+v\nwant a new view's address and a new address only: %+v", got, want)
	}
}

func TestWebPageControls(t *testing.T) {
	f, _ := newTestFake(t)
	if err := f.ShowWebView("w1", WebView{URL: "https://app.slack.com/client", Zoom: 1.5}, layout.Rect{W: 10, H: 10}); err != nil {
		t.Fatal(err)
	}
	if got := f.WebViews()["w1"].Zoom; got != 1.5 {
		t.Errorf("a new view's zoom = %v, want the one it was shown with", got)
	}
	if err := f.ZoomWebView("w1", 0.8); err != nil {
		t.Fatal(err)
	}
	if got := f.WebViews()["w1"].Zoom; got != 0.8 {
		t.Errorf("after ZoomWebView, zoom = %v, want 0.8", got)
	}

	// Back and forward do nothing until the page says it can go there.
	if err := f.NavigateWebView("w1", -1); err != nil {
		t.Fatal(err)
	}
	if err := f.SetWebState("w1", WebState{CanGoBack: true, Camera: CaptureActive, Microphone: CaptureActive}); err != nil {
		t.Fatal(err)
	}
	if err := f.NavigateWebView("w1", -1); err != nil {
		t.Fatal(err)
	}
	if err := f.NavigateWebView("w1", 1); err != nil {
		t.Fatal(err)
	}
	if err := f.ReloadWebView("w1"); err != nil {
		t.Fatal(err)
	}
	if got := f.WebViews()["w1"]; got.Back != 1 || got.Forward != 0 || got.Reloads != 1 {
		t.Errorf("back %d, forward %d, reloads %d, want 1, 0, and 1", got.Back, got.Forward, got.Reloads)
	}

	// Muting a device the page uses changes it; one it doesn't use stays
	// unused.
	if err := f.SetCaptureState("w1", Microphone, CaptureMuted); err != nil {
		t.Fatal(err)
	}
	if err := f.SetWebState("w1", WebState{Microphone: CaptureMuted}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCaptureState("w1", Camera, CaptureActive); err != nil {
		t.Fatal(err)
	}
	if err := f.PauseWebViewMedia("w1"); err != nil {
		t.Fatal(err)
	}
	want := []Event{
		{Kind: WebStateChanged, ItemID: "w1", Web: WebState{CanGoBack: true, Camera: CaptureActive, Microphone: CaptureActive}},
		{Kind: WebStateChanged, ItemID: "w1", Web: WebState{CanGoBack: true, Camera: CaptureActive, Microphone: CaptureMuted}},
		{Kind: WebStateChanged, ItemID: "w1", Web: WebState{Microphone: CaptureMuted}},
	}
	if got := f.DrainEvents(); !slices.Equal(got, want) {
		t.Errorf("events = %+v\nwant %+v", got, want)
	}
	if got := f.WebViews()["w1"].Paused; got != 1 {
		t.Errorf("paused %d times, want 1", got)
	}

	// Find looks in the page's address, or its text once that's set.
	for _, tt := range []struct {
		text  string
		found bool
	}{{"SLACK", true}, {"payroll", false}, {"", false}} {
		if found, err := f.FindInWebView("w1", tt.text, false); err != nil || found != tt.found {
			t.Errorf("FindInWebView(%q) = %v, %v, want %v", tt.text, found, err, tt.found)
		}
	}
	if err := f.SetPageText("w1", "Payroll for March"); err != nil {
		t.Fatal(err)
	}
	if found, _ := f.FindInWebView("w1", "payroll", true); !found {
		t.Error("FindInWebView didn't find text in the page's text")
	}
	if got := f.WebViews()["w1"]; got.Found != "payroll" || !got.Backwards {
		t.Errorf("the find was recorded as %q, backwards %v", got.Found, got.Backwards)
	}

	for name, err := range map[string]error{
		"ReloadWebView":     f.ReloadWebView("nope"),
		"NavigateWebView":   f.NavigateWebView("nope", 1),
		"ZoomWebView":       f.ZoomWebView("nope", 1),
		"SetCaptureState":   f.SetCaptureState("nope", Camera, CaptureMuted),
		"PauseWebViewMedia": f.PauseWebViewMedia("nope"),
		"SetWebState":       f.SetWebState("nope", WebState{}),
		"SetPageText":       f.SetPageText("nope", ""),
	} {
		if err == nil {
			t.Errorf("%s of an unknown view = nil", name)
		}
	}
	if _, err := f.FindInWebView("nope", "x", false); err == nil {
		t.Error("FindInWebView of an unknown view = nil")
	}
	f.SetErr("ReloadWebView", ErrUnsupported)
	if err := f.ReloadWebView("w1"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ReloadWebView = %v, want the error set", err)
	}
}

func TestPreviewsAndNotifications(t *testing.T) {
	f, _ := newTestFake(t)
	w := f.AddWindow("dev.zed.Zed", "bench")
	f.ShowWebView("w1", WebView{URL: "https://app.slack.com/client"}, layout.Rect{W: 10, H: 10})
	anchor := layout.Rect{X: 20, Y: 48, W: 160, H: 30}

	if err := f.ShowPreview(PreviewTarget{Window: w.ID, Title: "bench"}, anchor); err != nil {
		t.Fatal(err)
	}
	if got, ok := f.PreviewShown(); !ok || got.Target.Window != w.ID || got.Anchor != anchor {
		t.Errorf("PreviewShown() = %+v, %v", got, ok)
	}
	if err := f.ShowPreview(PreviewTarget{WebViewID: "w1"}, anchor); err != nil {
		t.Fatal(err)
	}
	if err := f.ShowPreview(PreviewTarget{Window: 99}, anchor); !errors.Is(err, ErrNoWindow) {
		t.Errorf("preview of an unknown window: %v, want ErrNoWindow", err)
	}
	if err := f.ShowPreview(PreviewTarget{WebViewID: "nope"}, anchor); err == nil {
		t.Error("preview of an unknown web view = nil")
	}
	f.HidePreview()
	if _, ok := f.PreviewShown(); ok {
		t.Error("preview still shown after HidePreview")
	}
	if got := f.Previews(); len(got) != 2 {
		t.Errorf("Previews() = %+v, want the two that worked", got)
	}

	n := Notification{ItemID: "w1", Title: "Ana", Body: "Lunch?"}
	if err := f.Notify(n); err != nil {
		t.Fatal(err)
	}
	if got := f.Notifications(); !slices.Equal(got, []Notification{n}) {
		t.Errorf("Notifications() = %+v", got)
	}
	f.SetBadge(2)
	if f.Badge() != 2 || !slices.Contains(f.CallsTo("SetBadge"), Call{Method: "SetBadge", Arg: "2"}) {
		t.Errorf("Badge() = %d, calls %+v; want 2, recorded", f.Badge(), f.CallsTo("SetBadge"))
	}

	f.SetRaised(true)
	if !f.Raised() {
		t.Error("Raised() = false after SetRaised(true)")
	}
	f.SetRaised(false)
	if f.Raised() {
		t.Error("Raised() = true after SetRaised(false)")
	}
}

func TestCalls(t *testing.T) {
	f, _ := newTestFake(t)
	w := f.AddWindow("dev.zed.Zed", "bench")
	r := layout.Rect{X: 9, Y: 75, W: 800, H: 600}
	f.SetFrame(w, r)
	f.Raise(w)
	f.OpenApp("dev.zed.Zed", "zed://file")

	calls := f.Calls()
	if got, want := methods(calls), []string{"SetFrame", "Raise", "OpenApp"}; !slices.Equal(got, want) {
		t.Fatalf("Calls() = %v, want %v", got, want)
	}
	want := Call{Method: "SetFrame", Window: w.ID, BundleID: "dev.zed.Zed", Rect: r}
	if calls[0] != want {
		t.Errorf("first call = %+v, want %+v", calls[0], want)
	}
	if got, want := calls[0].String(), "SetFrame(w1 dev.zed.Zed {9 75 800 600})"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := calls[2].String(), `OpenApp(dev.zed.Zed "zed://file")`; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got := f.CallsTo("Raise", "OpenApp"); len(got) != 2 {
		t.Errorf("CallsTo() = %v", got)
	}
	if got := f.CallsTo("Hide"); len(got) != 0 {
		t.Errorf("CallsTo(Hide) = %v, want none", got)
	}
	f.ResetCalls()
	if got := f.Calls(); len(got) != 0 {
		t.Errorf("Calls() after ResetCalls = %v", got)
	}
}

func TestEventsNeverBlock(t *testing.T) {
	f, _ := newTestFake(t)
	for range FakeEventBuffer + 10 {
		f.EmitEvent(Event{Kind: BenchMoved})
	}
	w := f.AddWindow("dev.zed.Zed", "bench")
	if err := f.CloseWindow(w.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.Dropped(); got != 11 {
		t.Errorf("Dropped() = %d, want 11", got)
	}
	if got := len(f.DrainEvents()); got != FakeEventBuffer {
		t.Errorf("drained %d events, want %d", got, FakeEventBuffer)
	}
}

func TestInjectedErrors(t *testing.T) {
	f, _ := newTestFake(t)
	a := f.AddWindow("dev.zed.Zed", "a")
	b := f.AddWindow("dev.zed.Zed", "b")
	boom := errors.New("boom")

	f.SetErr("Raise", boom)
	if err := f.Raise(a); !errors.Is(err, boom) {
		t.Errorf("Raise: %v, want boom", err)
	}
	f.SetErr("Raise", nil)
	if err := f.Raise(a); err != nil {
		t.Errorf("Raise after clearing: %v", err)
	}

	f.SetWindowErr(b.ID, "SetFrame", boom)
	if err := f.SetFrame(a, layout.Rect{W: 10, H: 10}); err != nil {
		t.Errorf("SetFrame(a): %v", err)
	}
	if err := f.SetFrame(b, layout.Rect{W: 10, H: 10}); !errors.Is(err, boom) {
		t.Errorf("SetFrame(b): %v, want boom", err)
	}
	if got, _ := f.Frame(b.ID); got == (layout.Rect{W: 10, H: 10}) {
		t.Error("a failed SetFrame moved the window")
	}
	f.SetWindowErr(b.ID, "SetFrame", nil)
	if err := f.SetFrame(b, layout.Rect{W: 10, H: 10}); err != nil {
		t.Errorf("SetFrame(b) after clearing: %v", err)
	}
	if got := len(f.CallsTo("Raise", "SetFrame")); got != 5 {
		t.Errorf("logged %d calls, want 5 including the failures", got)
	}
}

// TestConcurrentUse runs Platform methods, scripted actions, and the clock
// together, for the race detector.
func TestConcurrentUse(t *testing.T) {
	f, clk := newTestFake(t)
	f.InstallApp("md.obsidian", "Obsidian", AppLaunchDelay(time.Second), AppLaunchWindows("Daily note"))
	zed := f.AddApp("dev.zed.Zed", "Zed")
	w := f.AddWindow("dev.zed.Zed", "bench")

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 50 {
				r := layout.Rect{X: float64(i), Y: float64(j), W: 400, H: 300}
				f.SetFrame(w, r)
				f.Raise(w)
				f.Windows(zed)
				f.RunningApps()
				f.OpenApp("md.obsidian", "")
				f.RetitleWindow(w.ID, fmt.Sprint(j))
				f.Calls()
				f.ZOrder()
				f.Hide(zed)
				f.Unhide(zed)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			clk.Advance(100 * time.Millisecond)
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 400 {
			select {
			case <-f.Events():
			default:
			}
		}
	}()
	wg.Wait()
	clk.Advance(time.Second)
	if !f.IsRunning("md.obsidian") {
		t.Error("Obsidian didn't launch")
	}
}

func TestDragsAndDropGuide(t *testing.T) {
	f, _ := newTestFake(t)
	f.AddApp("dev.zed.Zed", "Zed")
	w := f.AddWindow("dev.zed.Zed", "main.go", WithFrame(layout.Rect{X: 100, Y: 100, W: 800, H: 600}))
	at := Point{X: 500, Y: 310}

	// Until Bench turns tracking on, a drag is only a move.
	if err := f.DragWindow(w.ID, layout.Rect{X: 120, Y: 110, W: 800, H: 600}, at); err != nil {
		t.Fatal(err)
	}
	if got := kinds(f.DrainEvents()); !slices.Equal(got, []EventKind{WindowMoved}) {
		t.Errorf("events without tracking = %v, want a move only", got)
	}
	if err := f.DropWindow(w.ID, at); err != nil {
		t.Fatal(err)
	}
	if got := f.DrainEvents(); len(got) != 0 {
		t.Errorf("events of a drop without tracking = %+v, want none", got)
	}
	if err := f.DropWindow(w.ID, at); err == nil {
		t.Error("dropping a window that isn't being dragged = nil")
	}

	f.TrackDrags(true)
	if !f.TrackingDrags() {
		t.Fatal("TrackingDrags() = false after TrackDrags(true)")
	}
	start := layout.Rect{X: 120, Y: 110, W: 800, H: 600}
	moved := layout.Rect{X: 200, Y: 50, W: 800, H: 600}
	if err := f.DragWindow(w.ID, layout.Rect{X: 150, Y: 80, W: 800, H: 600}, at); err != nil {
		t.Fatal(err)
	}
	if err := f.DragWindow(w.ID, moved, at); err != nil {
		t.Fatal(err)
	}
	if !f.MouseHeld() {
		t.Error("MouseHeld() = false during a drag")
	}
	if err := f.DropWindow(w.ID, Point{X: 580, Y: 250}); err != nil {
		t.Fatal(err)
	}
	if f.MouseHeld() {
		t.Error("MouseHeld() = true after the drop")
	}
	evs := f.DrainEvents()
	if got := kinds(evs); !slices.Equal(got, []EventKind{WindowMoved, WindowDragged, WindowMoved, WindowDragged, WindowDropped}) {
		t.Fatalf("events = %v", got)
	}
	for _, ev := range evs[1:] {
		if ev.Kind == WindowMoved {
			if !ev.MouseDown {
				t.Errorf("a drag's move lacks MouseDown: %+v", ev)
			}
			continue
		}
		if ev.Start != start || ev.Window.ID != w.ID || ev.Window.PID != w.PID || ev.Window.BundleID != "" {
			t.Errorf("%s = %+v, want the window, its process, and the frame it started from, and no more", ev.Kind, ev)
		}
	}
	if drop := evs[4]; drop.Window.Frame != moved || drop.Pointer != (Point{X: 580, Y: 250}) {
		t.Errorf("drop = %+v, want the window where it was let go, and the pointer", drop)
	}

	// Bench's own popped-out windows are reported by their items.
	from := layout.Rect{X: 40, Y: 40, W: 600, H: 400}
	to := layout.Rect{X: 300, Y: 90, W: 600, H: 400}
	f.DragPopOut("w1", from, to, at)
	f.DropPopOut("w1", to, at)
	evs = f.DrainEvents()
	if got := kinds(evs); !slices.Equal(got, []EventKind{WindowDragged, WindowDropped}) {
		t.Fatalf("pop-out events = %v", got)
	}
	for _, ev := range evs {
		if ev.ItemID != "w1" || ev.Start != from || ev.Window.Frame != to || ev.Window.ID != 0 {
			t.Errorf("%s = %+v, want the item, where it started, and where it is", ev.Kind, ev)
		}
	}
	f.FocusPopOut("w1", true)
	f.FocusPopOut("w1", false)
	f.PressBackToBench("w1")
	if got := kinds(f.DrainEvents()); !slices.Equal(got, []EventKind{PopOutFocused, PopOutUnfocused, DockRequested}) {
		t.Errorf("events = %v, want focus, unfocus, and Back to Bench", got)
	}

	g := DropGuide{Pane: layout.Rect{X: 0, Y: 50, W: 700, H: 900}, Target: layout.Rect{X: 0, Y: 91, W: 350, H: 859}, Zone: "left", Splits: true, Label: "New pane on the left"}
	f.ShowDropGuide(g)
	if got, ok := f.DropGuideShown(); !ok || got != g {
		t.Errorf("DropGuideShown() = %+v, %v; want %+v", got, ok, g)
	}
	f.HideDropGuide()
	if _, ok := f.DropGuideShown(); ok {
		t.Error("the drop guide still shows after HideDropGuide")
	}
	f.TrackDrags(false)
	if f.TrackingDrags() {
		t.Error("TrackingDrags() = true after TrackDrags(false)")
	}
}
