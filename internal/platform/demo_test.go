package platform

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
)

func TestNewDemoWorld(t *testing.T) {
	d := NewDemo(clock.NewFake(epoch))

	type app struct {
		bundleID, name string
		windows        []string
	}
	want := []app{
		{"dev.zed.Zed", "Zed", []string{"bench — switch.go"}},
		{"com.microsoft.VSCode", "VS Code", []string{"index.html — bench-ui"}},
		{"com.brave.Browser", "Brave", []string{"Wails v3 docs", "Pull request #42 · bench", "Go packages"}},
		{"md.obsidian", "Obsidian", []string{"Daily note"}},
		{"com.apple.finder", "Finder", []string{"Downloads"}},
	}
	running := d.RunningApps()
	if len(running) != len(want) {
		t.Fatalf("RunningApps() = %+v, want %d apps", running, len(want))
	}
	created := 0
	for i, w := range want {
		got := running[i]
		if got.BundleID != w.bundleID || got.Name != w.name || !got.Regular || got.Hidden {
			t.Errorf("app %d = %+v, want %s (%s), regular and shown", i, got, w.name, w.bundleID)
		}
		ws, err := d.Windows(got)
		if err != nil {
			t.Fatal(err)
		}
		if titles := titles(ws); !slices.Equal(titles, w.windows) {
			t.Errorf("%s windows = %q, want %q", w.name, titles, w.windows)
		}
		for _, win := range ws {
			created++
			if win.Created != created || !win.Standard() || win.App != w.name {
				t.Errorf("window %+v: want Created %d, standard, app %q", win, created, w.name)
			}
		}
	}

	installed := d.InstalledApps()
	names := make([]string, len(installed))
	for i, a := range installed {
		names[i] = a.Name
		if a.BundleID == "" || !strings.HasSuffix(a.Path, "/"+a.Name+".app") {
			t.Errorf("installed app %+v has no bundle ID or an unlikely path", a)
		}
	}
	for _, name := range []string{"Calendar", "Figma", "Finder", "Linear", "Notes", "Safari", "Spotify", "Xcode",
		"Zed", "Visual Studio Code", "Brave Browser", "Obsidian", "Terminal", "Gmail"} {
		if !slices.Contains(names, name) {
			t.Errorf("InstalledApps() is missing %s: %q", name, names)
		}
	}
	if !slices.IsSortedFunc(names, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }) {
		t.Errorf("InstalledApps() isn't sorted by name: %q", names)
	}

	var webApps []AppInfo
	for _, a := range installed {
		if a.WebApp != "" {
			webApps = append(webApps, a)
		}
	}
	if len(webApps) != 2 || webApps[0].Name != "Gmail" || webApps[0].WebApp != "Brave" ||
		webApps[0].WebURL != "https://mail.google.com/mail/u/0/" ||
		webApps[1].Name != "Slack" || webApps[1].WebApp != "Brave" || webApps[1].WebURL != "https://app.slack.com/client" {
		t.Errorf("the demo's web apps = %+v, want Gmail and Slack from Brave", webApps)
	}
	if got := d.DefaultBrowser(); got != DemoBrowser || DemoBrowser != "com.apple.Safari" {
		t.Errorf("DefaultBrowser() = %q, want Safari", got)
	}

	if !d.Trusted(false) || !d.ScreenRecordingAllowed(false) {
		t.Error("demo permissions aren't granted")
	}
	if d.ScrollBarsShown() {
		t.Error("the demo shows scroll bars all the time, want only while scrolling, as with a trackpad")
	}
	if k := d.SystemKeys(); k.VoiceOver || len(k.InputSources) != 0 {
		t.Errorf("the demo's SystemKeys = %+v, want none", k)
	}
	// The preview opens nothing on the computer it runs on.
	if err := d.OpenTextFile("/tmp/config.toml"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("OpenTextFile = %v, want ErrUnsupported", err)
	}
	if err := d.ShowInFinder("/tmp/config.toml"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ShowInFinder = %v, want ErrUnsupported", err)
	}
	if path, err := d.ChooseToAdd("/Applications"); path != "" || !errors.Is(err, ErrUnsupported) {
		t.Errorf("ChooseToAdd = %q, %v, want ErrUnsupported", path, err)
	}
	if err := d.OpenSettings(SettingsAccessibility); !errors.Is(err, ErrUnsupported) {
		t.Errorf("OpenSettings = %v, want ErrUnsupported", err)
	}
	if d.LoginItemNeedsApproval() {
		t.Error("the demo's login item needs approval, want not")
	}
	if got := d.DemoScreen(); got != DemoFit {
		t.Errorf("DemoScreen() = %q, want %q", got, DemoFit)
	}
	if evs := d.DrainEvents(); len(evs) != 0 {
		t.Errorf("a new demo has events waiting: %v", kinds(evs))
	}
}

func TestDemoOpensApps(t *testing.T) {
	clk := clock.NewFake(epoch)
	d := NewDemo(clk)
	if err := d.OpenApp("com.figma.Desktop", ""); err != nil {
		t.Fatal(err)
	}
	clk.Advance(DemoLaunchDelay - time.Millisecond)
	if d.IsRunning("com.figma.Desktop") {
		t.Fatal("Figma opened before the launch delay")
	}
	clk.Advance(time.Millisecond)
	figma, ok := d.App("com.figma.Desktop")
	if !ok || figma.PID == 0 {
		t.Fatal("Figma isn't running after the launch delay")
	}
	ws, _ := d.Windows(figma)
	if got := titles(ws); !slices.Equal(got, []string{"Figma window"}) {
		t.Errorf("Figma's windows = %q, want one", got)
	}
	if got, want := kinds(d.DrainEvents()), []EventKind{AppLaunched, WindowCreated, AppActivated}; !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	if d.ActiveApp() != "com.figma.Desktop" || !ws[0].Focused {
		t.Errorf("after launching, active app = %q and window focused = %v; want Figma and true", d.ActiveApp(), ws[0].Focused)
	}

	zed, _ := d.App("dev.zed.Zed")
	if err := d.NewWindow(zed); err != nil {
		t.Fatal(err)
	}
	ws, _ = d.Windows(zed)
	if got := titles(ws); !slices.Equal(got, []string{"bench — switch.go", DefaultNewWindowTitle}) {
		t.Errorf("Zed's windows = %q", got)
	}
}

func TestDemoUnreadCounts(t *testing.T) {
	clk := clock.NewFake(epoch)
	d := NewDemo(clk)
	d.WatchDockBadges(true)
	if got, want := d.DockBadges(), map[string]string{"com.brave.Browser": "4"}; !maps.Equal(got, want) {
		t.Errorf("DockBadges() = %v, want %v: Brave's alone, while Mail and Slack aren't running", got, want)
	}
	for _, bundleID := range []string{"com.apple.mail", "com.tinyspeck.slackmacgap"} {
		if err := d.OpenApp(bundleID, ""); err != nil {
			t.Fatal(err)
		}
	}
	clk.Advance(DemoLaunchDelay)
	want := map[string]string{"com.brave.Browser": "4", "com.apple.mail": "8", "com.tinyspeck.slackmacgap": "•"}
	if got := d.DockBadges(); !maps.Equal(got, want) {
		t.Errorf("DockBadges() = %v, want %v", got, want)
	}
	d.DrainEvents()

	for address, title := range map[string]string{
		"https://app.slack.com/client":     "(3) Slack",
		"https://outlook.office.com/mail/": "(12) Outlook",
	} {
		if err := d.ShowWebView("w1", WebView{URL: address}, layout.Rect{W: 100, H: 100}); err != nil {
			t.Fatal(err)
		}
		evs := d.DrainEvents()
		if len(evs) != 1 || evs[0].Kind != WebTitle || evs[0].Title != title {
			t.Errorf("showing %s sent %+v, want the title %q", address, evs, title)
		}
	}
}

func TestDemoBrowserMakesWebApps(t *testing.T) {
	clk := clock.NewFake(epoch)
	d := NewDemo(clk)
	if err := d.OpenApp(DemoBrowser, "https://squoosh.app/"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(DemoWebAppDelay)
	var made []AppInfo
	for _, a := range d.InstalledApps() {
		if a.WebApp == "Safari" {
			made = append(made, a)
		}
	}
	if len(made) != 1 || made[0].Name != "Squoosh" || made[0].WebURL != "https://squoosh.app/" {
		t.Fatalf("Safari's web apps = %+v, want Squoosh", made)
	}
	if err := d.OpenApp(made[0].BundleID, ""); err != nil {
		t.Fatal(err)
	}
	clk.Advance(DemoLaunchDelay)
	if !d.IsRunning(made[0].BundleID) {
		t.Error("Squoosh isn't running after the launch delay")
	}
}

func TestSetDemoScreen(t *testing.T) {
	tests := []struct {
		name     string
		screen   string
		fitWidth float64
		want     Screen
	}{
		{
			name:   "monitor",
			screen: DemoMonitor,
			want: Screen{ID: "monitor", Name: "Monitor", Primary: true,
				Frame: layout.Rect{W: 2560, H: 1440}, Visible: layout.Rect{Y: 25, W: 2560, H: 1415}},
		},
		{
			name:   "laptop",
			screen: DemoLaptop,
			want: Screen{ID: "laptop", Name: "Laptop", Primary: true,
				Frame: layout.Rect{W: 1512, H: 982}, Visible: layout.Rect{Y: 33, W: 1512, H: 949}},
		},
		{
			name:     "fit",
			screen:   DemoFit,
			fitWidth: 1280,
			want: Screen{ID: "fit", Name: "Browser window", Primary: true,
				Frame: layout.Rect{W: 1280, H: 900}, Visible: layout.Rect{W: 1280, H: 900}},
		},
		{
			name:   "fit without a width keeps the last one",
			screen: DemoFit,
			want: Screen{ID: "fit", Name: "Browser window", Primary: true,
				Frame: layout.Rect{W: 1440, H: 900}, Visible: layout.Rect{W: 1440, H: 900}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDemo(clock.NewFake(epoch))
			d.SetDemoScreen(tt.screen, tt.fitWidth)
			if got := d.Screens(); len(got) != 1 || got[0] != tt.want {
				t.Errorf("Screens() = %+v, want [%+v]", got, tt.want)
			}
			if got := d.ContentFrame(); got != tt.want.Visible {
				t.Errorf("ContentFrame() = %+v, want %+v", got, tt.want.Visible)
			}
			if got := d.DemoScreen(); got != tt.screen {
				t.Errorf("DemoScreen() = %q, want %q", got, tt.screen)
			}
			if got, want := kinds(d.DrainEvents()), []EventKind{ScreensChanged}; !slices.Equal(got, want) {
				t.Errorf("events = %v, want %v", got, want)
			}
		})
	}

	t.Run("unknown names do nothing", func(t *testing.T) {
		d := NewDemo(clock.NewFake(epoch))
		before := d.Screens()
		d.SetDemoScreen("projector", 800)
		if got := d.Screens(); !slices.Equal(got, before) || d.DemoScreen() != DemoFit {
			t.Errorf("Screens() = %+v, want unchanged", got)
		}
		if evs := d.DrainEvents(); len(evs) != 0 {
			t.Errorf("events = %v, want none", kinds(evs))
		}
	})

	t.Run("the engine can find it through DemoScreens", func(t *testing.T) {
		var p Platform = NewDemo(nil)
		ds, ok := p.(DemoScreens)
		if !ok {
			t.Fatal("the demo platform doesn't implement DemoScreens")
		}
		ds.SetDemoScreen(DemoLaptop, 0)
		if got := ds.DemoScreen(); got != DemoLaptop {
			t.Errorf("DemoScreen() = %q", got)
		}
	})
}

func TestNewDemoDoesntKeepGrowing(t *testing.T) {
	d := NewDemo(clock.NewFake(epoch))
	app, _ := d.App("dev.zed.Zed")
	for i := range 2000 {
		d.Hide(app)
		d.Unhide(app)
		d.OpenApp("com.brave.Browser", "https://example.com/"+strconv.Itoa(i))
		d.ShowPreview(PreviewTarget{}, layout.Rect{})
		d.Notify(Notification{ItemID: "w1"})
	}
	calls := d.Calls()
	if len(calls) != demoHistory {
		t.Errorf("the demo kept %d calls, want %d", len(calls), demoHistory)
	}
	if last := calls[len(calls)-1]; last.Method != "Notify" {
		t.Errorf("the newest call kept is %v, want Notify", last)
	}
	urls := d.OpenedURLs()
	if len(urls) != demoHistory || urls[len(urls)-1].URL != "https://example.com/1999" {
		t.Errorf("the demo kept %d URLs, ending %+v; want %d, ending with the newest", len(urls), urls[len(urls)-1], demoHistory)
	}
	if n, m := len(d.Previews()), len(d.Notifications()); n != demoHistory || m != demoHistory {
		t.Errorf("the demo kept %d previews and %d notifications, want %d of each", n, m, demoHistory)
	}
}
