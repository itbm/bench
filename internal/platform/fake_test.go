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
