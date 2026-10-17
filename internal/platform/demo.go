package platform

import (
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

// demoFitWidth is the "fit" screen's width until SetDemoScreen gives one.
const demoFitWidth = 1440

// DemoScreens is what the browser preview's platform adds for simulating
// screens. The engine asserts its Platform to it for SetDemoScreen.
type DemoScreens interface {
	// SetDemoScreen switches the simulated screen (see Fake.SetDemoScreen).
	SetDemoScreen(name string, fitWidth float64)
	// DemoScreen returns the simulated screen's name.
	DemoScreen() string
}

var _ DemoScreens = (*Fake)(nil)

// demoApp is an installed demo app, and its windows if it's running.
type demoApp struct {
	bundleID string
	name     string // Installed name.
	path     string
	running  string   // Running name, if it's running.
	windows  []string // Window titles, if it's running.
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
//     Spotify, Xcode, Terminal, Slack, Mail, Preview, Activity Monitor, and
//     System Settings. Opening one that isn't running launches it after
//     DemoLaunchDelay (by clk) with one window, titled "<name> window".
//   - The "fit" screen, 1,440 points wide until SetDemoScreen says
//     otherwise.
//   - Accessibility and Screen Recording permission granted.
//
// A nil clk means clock.Real().
func NewDemo(clk clock.Clock) *Fake {
	f := NewFake(clk)
	f.SetTrusted(true)
	f.SetScreenRecordingAllowed(true)
	f.SetDemoScreen(DemoFit, demoFitWidth)
	f.DrainEvents()
	for _, d := range demoApps {
		opts := []AppOption{
			AppPath(d.path),
			AppLaunchDelay(DemoLaunchDelay),
			AppLaunchWindows(d.name + " window"),
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
	case DemoMonitor:
		s = Screen{
			Name:    "Monitor",
			Frame:   layout.Rect{W: 2560, H: 1440},
			Visible: layout.Rect{Y: 25, W: 2560, H: 1415},
		}
	case DemoLaptop:
		s = Screen{
			Name:    "Laptop",
			Frame:   layout.Rect{W: 1512, H: 982},
			Visible: layout.Rect{Y: 33, W: 1512, H: 949},
		}
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
	f.setScreensLocked([]Screen{s})
	f.emitLocked(Event{Kind: ScreensChanged})
}

// DemoScreen returns the name of the simulated screen set by SetDemoScreen,
// or "" if it hasn't been called.
func (f *Fake) DemoScreen() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.demoScreen
}
