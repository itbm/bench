package platform

import (
	"slices"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
)

// Names of the browser preview's simulated screens, for SetDemoScreen. They
// match the prototype's screen menu.
const (
	DemoFit     = "fit"     // The browser's width, 900 points high.
	DemoMonitor = "monitor" // 2,560 by 1,440 points.
	DemoLaptop  = "laptop"  // 1,512 by 982 points.
)

// DemoLaunchDelay is how long a demo app that isn't running takes to open.
const DemoLaunchDelay = 600 * time.Millisecond

// DemoBrowser is the demo's default browser, Safari.
const DemoBrowser = "com.apple.Safari"

// DemoWindowTool is the demo's window tool, Rectangle, which is installed
// but doesn't run until the demo bar's Rectangle starts it (StartAgent).
const DemoWindowTool = "com.knollsoft.Rectangle"

// demoToolPreferences are the demo's Rectangle's preferences: the
// recommended shortcuts, as a Rectangle first installed at version 95 has
// them, with none that the user changed.
var demoToolPreferences = ToolPreferences{
	Numbers: map[string]int64{"alternateDefaultShortcuts": 1},
	Texts:   map[string]string{"installVersion": "95"},
}

// DemoWebAppDelay is how long the demo's default browser takes to make a web
// app of a site that it's asked to open, standing in for the user following
// the browser's steps.
const DemoWebAppDelay = 3 * time.Second

// demoFitWidth is the "fit" screen's width until SetDemoScreen gives one.
const demoFitWidth = 1440

// demoHistory is how many calls, opened URLs, previews, and notifications
// the demo platform keeps (see Fake.SetHistoryLimit). Nothing in the preview
// reads them, but a few are handy when debugging it.
const demoHistory = 256

// DemoScreens is what the browser preview's platform adds for simulating
// screens. The engine asserts its Platform to it for SetDemoScreen.
type DemoScreens interface {
	// SetDemoScreen switches the simulated screen (see Fake.SetDemoScreen).
	SetDemoScreen(name string, fitWidth float64)
	// DemoScreen returns the simulated screen's name.
	DemoScreen() string
}

var _ DemoScreens = (*Fake)(nil)

// DemoDisplays is what the browser preview's platform adds for unplugging
// its simulated monitor and plugging it in again, as on a laptop with a
// monitor, and for moving Bench's simulated window from one of its screens
// to another, as Bench does when the display that the user chose for it
// comes back (decision 70).
type DemoDisplays interface {
	// SetDemoMonitor unplugs the monitor, or plugs it in again (see
	// Fake.SetDemoMonitor).
	SetDemoMonitor(plugged bool) bool
	// DemoUnplugged reports whether the monitor is unplugged.
	DemoUnplugged() bool
	// MoveDemoBench moves Bench's window to a screen, by ID.
	MoveDemoBench(screenID string)
}

var _ DemoDisplays = (*Fake)(nil)

// demoApp is an installed demo app, and its windows if it's running.
type demoApp struct {
	bundleID string
	name     string // Installed name.
	path     string
	running  string   // Running name, if it's running.
	windows  []string // Window titles, if it's running.
	// webApp and webURL are a web app's browser and address.
	webApp, webURL string
}

// demoApps are the prototype's apps. Windows are created in this order, so
// sub-tabs come out as they do in the prototype.
var demoApps = []demoApp{
	{bundleID: "dev.zed.Zed", name: "Zed", path: "/Applications/Zed.app",
		running: "Zed", windows: []string{"bench — switch.go"}},
	{bundleID: "com.microsoft.VSCode", name: "Visual Studio Code", path: "/Applications/Visual Studio Code.app",
		running: "VS Code", windows: []string{"index.html — bench-ui"}},
	{bundleID: "com.brave.Browser", name: "Brave Browser", path: "/Applications/Brave Browser.app",
		running: "Brave", windows: []string{"Wails v3 docs", "Pull request #42 · bench", "Go packages"}},
	{bundleID: "md.obsidian", name: "Obsidian", path: "/Applications/Obsidian.app",
		running: "Obsidian", windows: []string{"Daily note"}},
	{bundleID: "com.apple.finder", name: "Finder", path: "/System/Library/CoreServices/Finder.app",
		running: "Finder", windows: []string{"Downloads"}},
	{bundleID: "com.apple.iCal", name: "Calendar", path: "/System/Applications/Calendar.app"},
	{bundleID: "com.figma.Desktop", name: "Figma", path: "/Applications/Figma.app"},
	{bundleID: "com.linear", name: "Linear", path: "/Applications/Linear.app"},
	{bundleID: "com.apple.Notes", name: "Notes", path: "/System/Applications/Notes.app"},
	{bundleID: "com.apple.Safari", name: "Safari", path: "/Applications/Safari.app"},
	{bundleID: "com.spotify.client", name: "Spotify", path: "/Applications/Spotify.app"},
	{bundleID: "com.apple.dt.Xcode", name: "Xcode", path: "/Applications/Xcode.app"},
	{bundleID: "com.apple.Terminal", name: "Terminal", path: "/System/Applications/Utilities/Terminal.app"},
	{bundleID: "com.tinyspeck.slackmacgap", name: "Slack", path: "/Applications/Slack.app"},
	{bundleID: "com.apple.mail", name: "Mail", path: "/System/Applications/Mail.app"},
	{bundleID: "com.apple.Preview", name: "Preview", path: "/System/Applications/Preview.app"},
	{bundleID: "com.apple.ActivityMonitor", name: "Activity Monitor", path: "/System/Applications/Utilities/Activity Monitor.app"},
	{bundleID: "com.apple.systempreferences", name: "System Settings", path: "/System/Applications/System Settings.app"},
	{bundleID: "com.brave.Browser.app.fmgjjmmmlfnkbppncabfkddbjimcfncm", name: "Gmail",
		path: "~/Applications/Brave Browser Apps.localized/Gmail.app", webApp: "Brave", webURL: "https://mail.google.com/mail/u/0/"},
	{bundleID: "com.brave.Browser.app.jneocipojkkahfcibhjaiilegofacenn", name: "Slack",
		path: "~/Applications/Brave Browser Apps.localized/Slack.app", webApp: "Brave", webURL: "https://app.slack.com/client"},
}

// demoDockBadges are the badges that demo apps show on their Dock icons
// while they run, by bundle ID: Brave, which runs from the start, a count,
// as of downloads, which Bench shows on the first of its three windows;
// Mail a count; and Slack a dot, as it does for unread messages that don't
// mention the user.
var demoDockBadges = map[string]string{
	"com.brave.Browser":         "4",
	"com.apple.mail":            "8",
	"com.tinyspeck.slackmacgap": "•",
}

// demoWebTitles are the titles of the pages at the example config's
// addresses, which the browser preview starts with: each page's name, after a count of unread messages, as web
// mail and chat sites put one.
var demoWebTitles = map[string]string{
	"https://app.slack.com/client":     "(3) Slack",
	"https://outlook.office.com/mail/": "(12) Outlook",
}

// NewDemo returns a Fake seeded with the prototype's world, for the browser
// preview (the server build):
//
//   - Running, with standard windows: Zed (dev.zed.Zed) with "bench —
//     switch.go"; VS Code (com.microsoft.VSCode) with "index.html —
//     bench-ui"; Brave (com.brave.Browser) with "Wails v3 docs", "Pull
//     request #42 · bench", and "Go packages"; Obsidian (md.obsidian) with
//     "Daily note"; and Finder (com.apple.finder) with "Downloads".
//   - Installed: those five (as Zed, Visual Studio Code, Brave Browser,
//     Obsidian, and Finder) and Calendar, Figma, Linear, Notes, Safari,
//     Spotify, Xcode, Terminal, Slack, Mail, Preview, Activity Monitor,
//     System Settings, and two web apps that Brave made: Gmail, of
//     https://mail.google.com/mail/u/0/, and Slack, of
//     https://app.slack.com/client, the default config's Slack page, whose
//     menu then offers it. Opening one that isn't running
//     launches it after DemoLaunchDelay (by clk) with one window, titled
//     "<name> window".
//   - Safari (DemoBrowser) as the default browser. Asked to open a site, it
//     makes a web app of it DemoWebAppDelay later, as if the user had
//     followed its steps (see AppMakesWebApps).
//   - The "fit" screen, 1,440 points wide until SetDemoScreen says
//     otherwise.
//   - Accessibility and Screen Recording permission granted.
//   - Scroll bars shown only while scrolling (ScrollBarsShown is false), as
//     with a trackpad.
//   - No keys that macOS keeps for itself (SystemKeys): one input source,
//     and VoiceOver off.
//   - Stage Manager off (StageManager is false).
//   - Rectangle (DemoWindowTool), a window tool, installed and not
//     running, with the preferences of one whose user chose its
//     recommended shortcuts (ToolPreferences). StartAgent starts it.
//   - Dock badges for Brave ("4"), Mail ("8"), and Slack ("•"), which
//     DockBadges reports while they run and Bench watches the Dock (see
//     SetDockBadge): Brave's from the start.
//   - Page titles with counts of unread messages, sent as the pages load
//     (see SetWebTitles): "(3) Slack" for https://app.slack.com/client,
//     and "(12) Outlook" for https://outlook.office.com/mail/.
//   - No icons, for apps or pages (AppIcon and PageIcon give ErrNoIcon),
//     so tiles show monograms, and the engine names none.
//   - Web pages that are placeholders: they never go anywhere to go back
//     from, nor use the camera or microphone, and find in page looks in
//     the page's address (see FindInWebView).
//   - Only the newest 256 calls, opened URLs, previews, and notifications
//     kept, since the preview runs for as long as the user likes.
//   - OpenTextFile, ShowInFinder, ChooseToAdd, and OpenSettings refused,
//     with ErrUnsupported: the preview's server has no authentication, so
//     it opens nothing on the computer it runs on, nor shows what's there.
//
// A nil clk means clock.Real().
func NewDemo(clk clock.Clock) *Fake {
	f := NewFake(clk)
	f.SetHistoryLimit(demoHistory)
	f.SetErr("OpenTextFile", ErrUnsupported)
	f.SetErr("ShowInFinder", ErrUnsupported)
	f.SetErr("ChooseToAdd", ErrUnsupported)
	f.SetErr("OpenSettings", ErrUnsupported)
	f.SetTrusted(true)
	f.SetScreenRecordingAllowed(true)
	f.SetDemoScreen(DemoFit, demoFitWidth)
	f.SetDefaultBrowser(DemoBrowser)
	f.DrainEvents()
	for _, d := range demoApps {
		opts := []AppOption{
			AppPath(d.path),
			AppLaunchDelay(DemoLaunchDelay),
			AppLaunchWindows(d.name + " window"),
		}
		if d.webApp != "" {
			opts = append(opts, AppWebApp(d.webApp), AppWebURL(d.webURL))
		}
		if d.bundleID == DemoBrowser {
			opts = append(opts, AppMakesWebApps(DemoWebAppDelay))
		}
		f.InstallApp(d.bundleID, d.name, opts...)
		if d.running == "" {
			continue
		}
		f.AddApp(d.bundleID, d.running)
		for _, title := range d.windows {
			f.AddWindow(d.bundleID, title)
		}
	}
	for bundleID, label := range demoDockBadges {
		f.SetDockBadge(bundleID, label)
	}
	f.SetWebTitles(func(address string) string { return demoWebTitles[address] })
	f.InstallApp(DemoWindowTool, "Rectangle", AppNotRegular())
	f.SetToolPreferences(DemoWindowTool, demoToolPreferences)
	return f
}

// SetDemoScreen switches to one of the browser preview's simulated screens
// and sends ScreensChanged:
//
//   - DemoMonitor: 2,560 by 1,440 points, visible from y 25.
//   - DemoLaptop: 1,512 by 982 points, visible from y 33.
//   - DemoFit: fitWidth by 900 points, all of it visible. A fitWidth of 0
//     or less keeps the last width (1,440 points at first).
//
// It does nothing for any other name. Bench's content frame follows the new
// screen's visible area unless SetContentFrame has set one.
func (f *Fake) SetDemoScreen(name string, fitWidth float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var s Screen
	switch name {
	case DemoMonitor, DemoLaptop:
		s = demoDisplay(name)
	case DemoFit:
		if fitWidth > 0 {
			f.demoFit = fitWidth
		}
		if f.demoFit <= 0 {
			f.demoFit = demoFitWidth
		}
		s = Screen{
			Name:    "Browser window",
			Frame:   layout.Rect{W: f.demoFit, H: 900},
			Visible: layout.Rect{W: f.demoFit, H: 900},
		}
	default:
		return
	}
	s.ID = name
	s.Primary = true
	f.demoScreen = name
	f.demoUnplugged = false
	f.benchScreen = ""
	f.setScreensLocked([]Screen{s})
	f.emitLocked(Event{Kind: ScreensChanged})
}

// demoDisplay returns the simulated monitor or laptop screen, primary, at
// the top left of the displays.
func demoDisplay(name string) Screen {
	if name == DemoMonitor {
		return Screen{ID: DemoMonitor, Name: "Monitor", Primary: true,
			Frame: layout.Rect{W: 2560, H: 1440}, Visible: layout.Rect{Y: 25, W: 2560, H: 1415}}
	}
	return Screen{ID: DemoLaptop, Name: "Laptop", Primary: true,
		Frame: layout.Rect{W: 1512, H: 982}, Visible: layout.Rect{Y: 33, W: 1512, H: 949}}
}

// SetDemoMonitor simulates unplugging the monitor, with plugged false, or
// plugging it in again, as on a laptop with a monitor, and reports whether
// it could:
//
//   - Unplugging needs the monitor among the screens, as after
//     SetDemoScreen(DemoMonitor) or plugging it in. The laptop's screen is
//     then the only one, and Bench's window goes to it, as macOS moves the
//     windows of a display that goes: it sends ScreensChanged, then
//     BenchMoved.
//   - Plugging in needs the monitor unplugged. The monitor comes back to
//     the right of the laptop's screen, which stays primary, and Bench's
//     window stays where it is, as macOS 26 leaves it: it sends
//     ScreensChanged.
//
// The simulated screen (DemoScreen) is the one Bench's window is on.
func (f *Fake) SetDemoMonitor(plugged bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	laptop := demoDisplay(DemoLaptop)
	if !plugged {
		if !slices.ContainsFunc(f.screens, func(s Screen) bool { return s.ID == DemoMonitor }) {
			return false
		}
		f.setScreensLocked([]Screen{laptop})
		f.benchScreen, f.demoScreen, f.demoUnplugged = DemoLaptop, DemoLaptop, true
		f.emitLocked(Event{Kind: ScreensChanged})
		f.emitLocked(Event{Kind: BenchMoved})
		return true
	}
	if !f.demoUnplugged {
		return false
	}
	monitor := demoDisplay(DemoMonitor)
	monitor.Primary = false
	monitor.Frame.X, monitor.Visible.X = laptop.Frame.W, laptop.Frame.W
	f.setScreensLocked([]Screen{laptop, monitor})
	f.demoUnplugged = false
	f.emitLocked(Event{Kind: ScreensChanged})
	return true
}

// DemoUnplugged reports whether SetDemoMonitor has unplugged the monitor,
// and it hasn't been plugged in again since.
func (f *Fake) DemoUnplugged() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.demoUnplugged
}

// MoveDemoBench moves Bench's window to one of the screens, by ID, which
// becomes the simulated screen (DemoScreen), and sends BenchMoved, as the
// shell's move does on a Mac. It does nothing for an ID that isn't one of
// the screens.
func (f *Fake) MoveDemoBench(screenID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.ContainsFunc(f.screens, func(s Screen) bool { return s.ID == screenID }) {
		return
	}
	f.benchScreen, f.demoScreen = screenID, screenID
	f.emitLocked(Event{Kind: BenchMoved})
}

// DemoScreen returns the name of the simulated screen set by SetDemoScreen,
// or "" if it hasn't been called.
func (f *Fake) DemoScreen() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.demoScreen
}
