package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// Web apps of the default config's Slack and Outlook, and their pages.
const (
	braveSlack    = "com.brave.Browser.app.slack"
	safariSlack   = "com.apple.Safari.WebApp.slack"
	chromeOutlook = "com.google.Chrome.app.outlook"
	// madeBySafari is the first web app that the Fake's Safari makes.
	madeBySafari = "com.apple.Safari.WebApp.1"
	slackHome    = "https://app.slack.com/client"
	outlookHome  = "https://outlook.office.com/mail/"
	// slackWebURL is where Slack's web apps open: a browser records the
	// address the site ended on.
	slackWebURL = "https://app.slack.com/client/T01/C01"
)

// installSlack installs a web app of Slack that browser (by name) made. It
// isn't running, and opens with a window called "Slack".
func installSlack(p *platform.Fake, bundleID, browser string) {
	p.InstallApp(bundleID, "Slack", platform.AppWebApp(browser), platform.AppWebURL(slackWebURL),
		platform.AppLaunchWindows("Slack"))
}

// slackWorld is the standard world with Brave's web app of Slack.
func slackWorld(p *platform.Fake) {
	standardWorld(p)
	installSlack(p, braveSlack, "Brave")
}

// paneIn returns a pane of tab ti, as the view has it.
func (h *harness) paneIn(ti int, paneID string) *layout.Node {
	h.t.Helper()
	n, _, _ := layout.Find(h.view().Tabs[ti].Root, paneID)
	if n == nil || !n.IsPane() {
		h.t.Fatalf("tab %d has no pane %s", ti, paneID)
	}
	return n
}

// savedConfig reads config.toml as Bench saved it.
func (h *harness) savedConfig() *config.Config {
	h.t.Helper()
	cfg, problems, err := config.Load(filepath.Join(h.dir, config.ConfigFile))
	if err != nil || len(problems) > 0 {
		h.t.Fatalf("config.toml: %v %v", err, problems)
	}
	return cfg
}

func TestWebAppChoice(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		world     func(p *platform.Fake)
		untrusted bool
		prep      func(h *harness)
		item      string
		want      WebAppChoice
	}{
		{name: "an installed web app of the site", world: slackWorld, item: "w1",
			want: WebAppChoice{Offer: "use", App: "Slack", Browser: "Brave"}},
		{name: "the default browser's web app first", world: func(p *platform.Fake) {
			slackWorld(p)
			installSlack(p, safariSlack, "Safari")
			p.SetDefaultBrowser(braveID)
		}, item: "w1", want: WebAppChoice{Offer: "use", App: "Slack", Browser: "Brave"}},
		{name: "else Safari's", world: func(p *platform.Fake) {
			slackWorld(p)
			installSlack(p, safariSlack, "Safari")
			p.SetDefaultBrowser(firefoxID)
		}, item: "w1", want: WebAppChoice{Offer: "use", App: "Slack", Browser: "Safari"}},
		{name: "a web app without Accessibility permission", world: slackWorld, untrusted: true, item: "w1",
			want: WebAppChoice{Offer: "use", App: "Slack", Browser: "Brave"}},
		{name: "none, but the default browser can make one", world: func(p *platform.Fake) {
			slackWorld(p)
			p.SetDefaultBrowser(safariID)
		}, item: "w2", want: WebAppChoice{Offer: "make", Browser: "Safari"}},
		{name: "none, without Accessibility permission", world: func(p *platform.Fake) {
			standardWorld(p)
			p.SetDefaultBrowser(safariID)
		}, untrusted: true, item: "w2"},
		{name: "none, with a browser that can't make web apps", world: func(p *platform.Fake) {
			standardWorld(p)
			p.InstallApp(firefoxID, "Firefox")
			p.SetDefaultBrowser(firefoxID)
		}, item: "w2"},
		{name: "none, with no default browser", item: "w2"},
		{name: "VS Code for the Web", config: vscodeConfig, world: func(p *platform.Fake) {
			standardWorld(p)
			p.SetDefaultBrowser(safariID)
		}, item: "w1"},
		{name: "an App item", world: func(p *platform.Fake) {
			standardWorld(p)
			p.SetDefaultBrowser(safariID)
		}, item: "w3"},
		{name: "a terminal", world: func(p *platform.Fake) {
			standardWorld(p)
			p.SetDefaultBrowser(safariID)
		}, item: "w5"},
		{name: "a web page popped out of Bench", world: func(p *platform.Fake) {
			slackWorld(p)
			p.SetDefaultBrowser(safariID)
		}, prep: func(h *harness) { mustOK(h.t, h.e.PopOut("w1")) }, item: "w1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, setup{config: tt.config, world: tt.world, untrusted: tt.untrusted})
			if tt.prep != nil {
				tt.prep(h)
			}
			got, err := h.e.WebAppChoice(tt.item)
			mustOK(t, err)
			if got != tt.want {
				t.Errorf("WebAppChoice(%s) = %+v, want %+v", tt.item, got, tt.want)
			}
		})
	}
	h := newHarness(t, setup{world: slackWorld})
	_, err := h.e.WebAppChoice("w99")
	mustErr(t, err, "That window has closed.")
}

func TestUseWebAppPutsTheWebAppInThePagesPlace(t *testing.T) {
	for _, shown := range []bool{true, false} {
		name := "the page is shown"
		if !shown {
			name = "another sub-tab is shown"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, setup{world: slackWorld})
			// Slack's pane holds a page added in Bench after it.
			mustOK(t, h.e.AddToPane("p8", Source{Type: "web", URL: "example.com"}))
			other := h.paneIn(0, "p8").Items[1]
			if shown {
				mustOK(t, h.e.ShowItem("p8", "w1"))
			}
			h.drain()
			if _, ok := h.p.WebViews()["w1"]; !ok {
				t.Fatal("Slack's web view should be open")
			}
			h.p.ResetCalls()
			mustOK(t, h.e.UseWebApp("w1"))
			p := h.paneIn(0, "p8")
			if len(p.Items) != 2 || p.Items[0] == "w1" || p.Items[1] != other {
				t.Fatalf("p8 = %v, want the web app in w1's place, then %s", p.Items, other)
			}
			id := p.Items[0]
			if (p.Active == id) != shown {
				t.Errorf("p8 shows %s; the web app is shown %v, want %v", p.Active, p.Active == id, shown)
			}
			it := h.item(id)
			if it.Kind != KindApp || it.BundleID != braveSlack || it.App != "Slack" || it.Tab != 0 {
				t.Errorf("the new item = %+v, want an App item of Brave's Slack in Comms", it)
			}
			if it.Status != StatusLaunching || it.StatusText != "Opening Slack…" {
				t.Errorf("before its window comes, the new item is %s %q", it.Status, it.StatusText)
			}
			if _, ok := h.view().Items["w1"]; ok {
				t.Error("w1 is still an item")
			}
			if _, ok := h.p.WebViews()["w1"]; ok {
				t.Error("w1's web view is still open")
			}
			if got := h.calls("OpenApp"); len(got) != 1 || got[0].BundleID != braveSlack || got[0].Arg != "" {
				t.Errorf("Bench called %v, want the web app opened", got)
			}

			h.settle()
			if got := h.itemOf(h.windowID(braveSlack, "Slack")); got != id {
				t.Errorf("the web app's window is item %q, want %s", got, id)
			}
			if it := h.item(id); it.Status != StatusReady || it.Title != "Slack" {
				t.Errorf("once its window came, the new item = %+v", it)
			}
			if p := h.paneIn(0, "p8"); (p.Active == id) != shown || len(p.Items) != 2 {
				t.Errorf("once the window came, p8 = %v showing %s", p.Items, p.Active)
			}
			if got := h.appItems(braveSlack); len(got) != 1 {
				t.Errorf("the web app's items = %+v, want one", got)
			}
		})
	}
}

func TestUseWebAppInSeveralTabs(t *testing.T) {
	h := newHarness(t, setup{world: slackWorld})
	mustOK(t, h.e.AddItemToTab("w1", 1))
	codePane := layout.PaneOf(h.view().Tabs[1].Root, "w1")
	at := slices.Index(codePane.Items, "w1")
	mustOK(t, h.e.UseWebApp("w1"))
	h.settle()
	id := h.paneIn(0, "p8").Items[0]
	if got := h.paneIn(1, codePane.ID).Items; len(got) != len(codePane.Items) || got[at] != id {
		t.Errorf("Code's %s = %v, want %s in w1's place (%v)", codePane.ID, got, id, codePane.Items)
	}
	if got := h.paneIn(1, codePane.ID).Active == id; got != (codePane.Active == "w1") {
		t.Errorf("Code shows the web app %v, want %v", got, codePane.Active == "w1")
	}
	if it := h.item(id); !slices.Equal(it.Tabs, []int{0, 1}) || it.Status != StatusReady {
		t.Errorf("the new item = %+v, want it ready in Comms and Code", it)
	}
	// Only Comms lists Slack in config.toml.
	cfg := h.savedConfig()
	if got := cfg.Tabs[0].Panes[0].Items; !slices.Equal(got, []config.Item{{App: braveSlack}}) {
		t.Errorf("Comms's first pane in config.toml = %+v, want the web app", got)
	}
	if got := cfg.Tabs[1]; !slices.Equal(got.Panes[0].Items, config.Example().Tabs[1].Panes[0].Items) {
		t.Errorf("Code in config.toml = %+v, want it as it was", got)
	}
}

func TestUseWebAppTakesTheWebAppsWindow(t *testing.T) {
	// running starts the web app, with one window, "Slack — general", that
	// Bench doesn't manage.
	running := func(p *platform.Fake) {
		slackWorld(p)
		p.AddApp(braveSlack, "Slack")
		p.AddWindow(braveSlack, "Slack — general")
	}
	tests := []struct {
		name  string
		world func(p *platform.Fake)
		prep  func(h *harness)
		// calls are the calls that open the web app or a window, at once.
		calls []string
		// window is the title of the window the new item gets.
		window string
		// opens is set for a web app that opens its first window late,
		// and wait for one that opens none.
		opens, wait bool
		// taken is set when the window was a popped-out item's.
		taken bool
	}{
		{name: "a window Bench doesn't manage", world: running, window: "Slack — general"},
		{name: "a window popped out of Bench", world: running, prep: func(h *harness) {
			w := h.windowID(braveSlack, "Slack — general")
			mustOK(h.t, h.e.AddToPane("p9", Source{Type: "floating", ID: floatingID(w)}))
			mustOK(h.t, h.e.PopOut(h.itemOf(w)))
		}, window: "Slack — general", taken: true},
		{name: "every window in a tab", world: running, prep: func(h *harness) {
			mustOK(h.t, h.e.SwitchTab(1))
			w := h.windowID(braveSlack, "Slack — general")
			mustOK(h.t, h.e.AddToPane("p12", Source{Type: "floating", ID: floatingID(w)}))
			mustOK(h.t, h.e.SwitchTab(0))
		}, calls: []string{"NewWindow"}, window: platform.DefaultNewWindowTitle},
		{name: "running without a window, which opens one", world: func(p *platform.Fake) {
			slackWorld(p)
			p.AddApp(braveSlack, "Slack")
		}, opens: true, window: "Slack — late"},
		{name: "running without a window, which opens none", world: func(p *platform.Fake) {
			slackWorld(p)
			p.AddApp(braveSlack, "Slack")
		}, wait: true, window: platform.DefaultNewWindowTitle},
		{name: "not running", world: slackWorld, calls: []string{"OpenApp"}, window: "Slack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, setup{world: tt.world})
			if tt.prep != nil {
				tt.prep(h)
				h.drain()
			}
			items := len(h.view().Items)
			h.p.ResetCalls()
			mustOK(t, h.e.UseWebApp("w1"))
			id := h.paneIn(0, "p8").Items[0]
			var got []string
			for _, c := range h.calls("OpenApp", "NewWindow") {
				got = append(got, c.Method)
			}
			if !slices.Equal(got, tt.calls) {
				t.Errorf("Bench called %v at once, want %v", got, tt.calls)
			}
			if tt.opens {
				h.advance(500 * time.Millisecond)
				_, err := h.p.OpenWindow(braveSlack, "Slack — late")
				mustOK(t, err)
			}
			if tt.wait {
				h.advance(launchSettle - 10*time.Millisecond)
				if got := h.calls("NewWindow"); len(got) != 0 {
					t.Fatalf("Bench opened a new window before waiting for one: %v", got)
				}
			}
			h.settle()
			if got := h.itemOf(h.windowID(braveSlack, tt.window)); got != id {
				t.Errorf("the window %q is item %q, want %s", tt.window, got, id)
			}
			if it := h.item(id); it.Status != StatusReady || it.Title != tt.window || it.Tab != 0 {
				t.Errorf("the new item = %+v, want it ready in Comms", it)
			}
			// The web page went, and its web app came, with a popped-out
			// item's window if it took one.
			want := items
			if tt.taken {
				want--
			}
			if got := len(h.view().Items); got != want {
				t.Errorf("there are %d items, want %d", got, want)
			}
			if slices.ContainsFunc(h.view().Floating, func(it ItemView) bool { return it.BundleID == braveSlack }) {
				t.Errorf("Floating = %+v, want none of the web app's windows", h.view().Floating)
			}
		})
	}
}

func TestUseWebAppLeavesWhatThePaneShows(t *testing.T) {
	// The web app opens two windows, as a Chromium web app does when it
	// restores its windows.
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(braveSlack, "Slack", platform.AppWebApp("Brave"), platform.AppWebURL(slackWebURL),
			platform.AppLaunchWindows("Slack A", "Slack B"))
	}})
	// Slack's pane shows a page added in Bench after it.
	mustOK(t, h.e.AddToPane("p8", Source{Type: "web", URL: "example.com"}))
	other := h.paneIn(0, "p8").Items[1]
	if got := h.paneIn(0, "p8").Active; got != other {
		t.Fatalf("p8 shows %s, want %s", got, other)
	}
	mustOK(t, h.e.UseWebApp("w1"))
	id := h.paneIn(0, "p8").Items[0]
	h.settle()
	a, b := h.itemOf(h.windowID(braveSlack, "Slack A")), h.itemOf(h.windowID(braveSlack, "Slack B"))
	if a != id {
		t.Errorf("the first window is item %q, want %s", a, id)
	}
	// The other window joins the pane, without being shown.
	p := h.paneIn(0, "p8")
	if want := []string{id, other, b}; !slices.Equal(p.Items, want) || b == "" {
		t.Errorf("p8 = %v, want %v", p.Items, want)
	}
	if p.Active != other {
		t.Errorf("p8 shows %s, want %s, as before", p.Active, other)
	}
}

func TestUseWebAppRefusals(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(safariSlack, "Slack", platform.AppWebApp("Safari"), platform.AppWebURL(slackWebURL))
	}})
	mustErr(t, h.e.UseWebApp("w99"), "That window has closed.")
	mustErr(t, h.e.UseWebApp("w3"), "Only a web page can become a web app.")
	mustErr(t, h.e.UseWebApp("w2"), "Bench can't find a web app of outlook.office.com.")
	h.p.SetErr("OpenApp", errors.New("LaunchServices said no"))
	mustErr(t, h.e.UseWebApp("w1"), "Bench couldn't open Slack.")
	if p := h.paneIn(0, "p8"); !slices.Equal(p.Items, []string{"w1"}) {
		t.Errorf("p8 = %v, want Slack's page still there", p.Items)
	}
	if _, ok := h.p.WebViews()["w1"]; !ok {
		t.Error("Slack's web view closed")
	}
	if _, err := os.Stat(filepath.Join(h.dir, config.ConfigFile+".bak")); err == nil {
		t.Error("config.toml was saved")
	}
	h.p.SetErr("OpenApp", nil)
	mustOK(t, h.e.PopOut("w1"))
	mustErr(t, h.e.UseWebApp("w1"), errPoppedOutDrag.Error())

	// A web app running with its windows in tabs, which won't open another.
	h = newHarness(t, setup{world: func(p *platform.Fake) {
		slackWorld(p)
		p.AddApp(braveSlack, "Slack")
		p.AddWindow(braveSlack, "Slack — general")
	}})
	w := h.windowID(braveSlack, "Slack — general")
	mustOK(t, h.e.AddToPane("p9", Source{Type: "floating", ID: floatingID(w)}))
	h.p.SetErr("NewWindow", errors.New("no"))
	mustErr(t, h.e.UseWebApp("w1"), "Bench couldn't open a new Slack window.")
	if p := h.paneIn(0, "p8"); !slices.Equal(p.Items, []string{"w1"}) {
		t.Errorf("p8 = %v, want Slack's page still there", p.Items)
	}
}

func TestUseWebAppSurvivesRestarting(t *testing.T) {
	h := newHarness(t, setup{world: slackWorld})
	mustOK(t, h.e.UseWebApp("w1"))
	h.settle()
	id := h.paneIn(0, "p8").Items[0]
	w := h.windowID(braveSlack, "Slack")

	// config.toml lists the web app in the page's place, and the file it
	// had before is kept.
	cfg := h.savedConfig()
	if got := cfg.Tabs[0].Panes[0].Items; !slices.Equal(got, []config.Item{{App: braveSlack}}) {
		t.Errorf("Comms's first pane in config.toml = %+v, want the web app", got)
	}
	if got := cfg.Tabs[0].Panes[1].Items; len(got) != 1 || got[0].Web != outlookHome {
		t.Errorf("Comms's second pane in config.toml = %+v, want Outlook", got)
	}
	if old, err := os.ReadFile(filepath.Join(h.dir, config.ConfigFile+".bak")); err != nil || !strings.Contains(string(old), slackHome) {
		t.Errorf("config.toml.bak = %q, %v; want the file from before", old, err)
	}

	check := func(when string, wantID string) {
		t.Helper()
		if got := h.webItems(slackHome); len(got) != 0 {
			t.Errorf("%s, Slack's page came back: %+v", when, got)
		}
		items := h.appItems(braveSlack)
		if len(items) != 1 || (wantID != "" && items[0].ID != wantID) || h.itemOf(w) != items[0].ID {
			t.Fatalf("%s, the web app's items = %+v, want %s with its window", when, items, wantID)
		}
		if got := layout.PaneOf(h.view().Tabs[0].Root, items[0].ID); got == nil || got.Items[0] != items[0].ID {
			t.Errorf("%s, the web app isn't first in Comms's first pane: %+v", when, got)
		}
		if got := h.webItems(outlookHome); len(got) != 1 {
			t.Errorf("%s, Outlook's pages = %+v, want one", when, got)
		}
	}
	h.restart()
	check("after a restart", id)
	mustOK(t, h.e.ResetLayout(0))
	check("after resetting Comms's layout", id)

	// Without state.json, Comms is built from config.toml.
	h.drain()
	mustOK(t, h.e.Close())
	mustOK(t, os.Remove(filepath.Join(h.dir, config.StateFile)))
	h.restart()
	check("after a restart without state.json", "")
}

func TestUseWebAppForAPageAddedInBench(t *testing.T) {
	h := newHarness(t, setup{world: slackWorld})
	mustOK(t, h.e.AddToPane("p9", Source{Type: "web", URL: "app.slack.com/client/T02"}))
	page := h.paneIn(0, "p9").Items[1]
	mustOK(t, h.e.UseWebApp(page))
	h.settle()
	id := h.paneIn(0, "p9").Items[1]
	if id == page || h.item(id).BundleID != braveSlack {
		t.Fatalf("p9 = %v, want the web app in %s's place", h.paneIn(0, "p9").Items, page)
	}
	// config.toml doesn't list the page, so it's left as it is.
	if _, err := os.Stat(filepath.Join(h.dir, config.ConfigFile+".bak")); err == nil {
		t.Error("config.toml was saved")
	}
	h.restart()
	if got := h.webItems("https://app.slack.com/client/T02"); len(got) != 0 {
		t.Errorf("the page came back: %+v", got)
	}
	if got := h.paneIn(0, "p9").Items; !slices.Equal(got, []string{"w2", id}) {
		t.Errorf("p9 = %v, want Outlook and %s", got, id)
	}
	if got := h.paneIn(0, "p8").Items; !slices.Equal(got, []string{"w1"}) {
		t.Errorf("p8 = %v, want Slack's page from config.toml", got)
	}
}

// makerWorld is the standard world with a default browser that makes web
// apps 3 seconds after it opens a site.
func makerWorld(p *platform.Fake) {
	standardWorld(p)
	p.InstallApp(safariID, "Safari", platform.AppLaunchWindows("Apple"), platform.AppMakesWebApps(3*time.Second))
	p.SetDefaultBrowser(safariID)
}

func TestMakeWebAppPutsTheWebAppInThePagesPlace(t *testing.T) {
	h := newHarness(t, setup{world: makerWorld})
	if got, err := h.e.WebAppChoice("w2"); err != nil || got != (WebAppChoice{Offer: "make", Browser: "Safari"}) {
		t.Fatalf("WebAppChoice = %+v, %v; want Safari to make one", got, err)
	}
	h.p.ResetCalls()
	res, err := h.e.MakeWebApp("w2")
	mustOK(t, err)
	if want := (WebsiteResult{Browser: "Safari", Steps: safariSteps}); res != want {
		t.Errorf("MakeWebApp = %+v, want %+v", res, want)
	}
	if got, want := h.p.OpenedURLs(), []platform.OpenedURL{{BundleID: safariID, URL: outlookHome}}; !slices.Equal(got, want) {
		t.Errorf("OpenedURLs = %+v, want %+v", got, want)
	}
	// The dialog asks every 1.5 seconds, as the browser takes 3.
	polls := 0
	for {
		polls++
		ready, err := h.e.UseWebAppWhenReady("w2")
		mustOK(t, err)
		if ready {
			break
		}
		if polls == 4 {
			t.Fatal("no web app after 4.5 seconds")
		}
		if p := h.paneIn(0, "p9"); !slices.Equal(p.Items, []string{"w2"}) {
			t.Fatalf("p9 = %v while the dialog waits, want Outlook's page", p.Items)
		}
		h.advance(1500 * time.Millisecond)
	}
	if polls != 3 {
		t.Errorf("the web app came at poll %d, want 3", polls)
	}
	h.settle()
	p := h.paneIn(0, "p9")
	if len(p.Items) != 1 || p.Active != p.Items[0] {
		t.Fatalf("p9 = %v showing %s, want the web app", p.Items, p.Active)
	}
	it := h.item(p.Items[0])
	if it.BundleID != madeBySafari || it.App != "Outlook" || it.Status != StatusReady {
		t.Errorf("the new item = %+v, want Safari's Outlook, ready", it)
	}
	if _, ok := h.p.WebViews()["w2"]; ok {
		t.Error("Outlook's web view is still open")
	}
	// Safari's own window, which it opened for the site, stayed out of the
	// tab.
	if got := h.appItems(safariID); len(got) != 0 {
		t.Errorf("Safari's items = %+v, want none", got)
	}
	if got := h.savedConfig().Tabs[0].Panes[1].Items; !slices.Equal(got, []config.Item{{App: madeBySafari}}) {
		t.Errorf("Comms's second pane in config.toml = %+v, want the web app", got)
	}
}

func TestUseWebAppWhenReadyFindsTheOnlyNewWebApp(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(chromeID, "Google Chrome")
		// A web app that the user had already, of another site.
		p.InstallApp("com.google.Chrome.app.gmail", "Gmail", platform.AppWebApp("Chrome"),
			platform.AppWebURL("https://mail.google.com/mail/u/0/"))
		p.SetDefaultBrowser(chromeID)
	}})
	// slack.com sends a signed-in user to app.slack.com, and Chrome
	// records the address it ended on.
	mustOK(t, h.e.AddToPane("p8", Source{Type: "web", URL: "slack.com"}))
	page := h.paneIn(0, "p8").Items[1]
	res, err := h.e.MakeWebApp(page)
	mustOK(t, err)
	if res.Browser != "Chrome" || res.Steps == "" {
		t.Fatalf("MakeWebApp = %+v, want Chrome's steps", res)
	}
	ready, err := h.e.UseWebAppWhenReady(page)
	mustOK(t, err)
	if ready {
		t.Fatal("UseWebAppWhenReady = true before the web app was made")
	}
	h.p.InstallApp("com.google.Chrome.app.slack", "Slack", platform.AppWebApp("Chrome"), platform.AppWebURL(slackWebURL),
		platform.AppLaunchWindows("Slack"))
	// It's the site's only for the page that MakeWebApp opened the site for.
	ready, err = h.e.UseWebAppWhenReady("w2")
	mustOK(t, err)
	if ready {
		t.Fatalf("UseWebAppWhenReady(w2) = true, want the web app left for %s", page)
	}
	ready, err = h.e.UseWebAppWhenReady(page)
	mustOK(t, err)
	if !ready {
		t.Fatal("UseWebAppWhenReady = false once the web app was made")
	}
	h.settle()
	p := h.paneIn(0, "p8")
	if len(p.Items) != 2 || p.Items[0] != "w1" || h.item(p.Items[1]).BundleID != "com.google.Chrome.app.slack" {
		t.Errorf("p8 = %v, want Slack's page, then the web app in %s's place", p.Items, page)
	}
	if got := h.appItems("com.google.Chrome.app.gmail"); len(got) != 0 {
		t.Errorf("the other site's web app was used: %+v", got)
	}
}

func TestUseWebAppWhenReadyMovesTheWindowBenchDocked(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(chromeID, "Google Chrome")
		p.SetDefaultBrowser(chromeID)
	}})
	_, err := h.e.MakeWebApp("w2")
	mustOK(t, err)
	h.drain()
	// Chrome makes the web app and opens it at once. Bench puts its window
	// in the focused pane, as new_windows says, before the dialog asks.
	h.p.InstallApp(chromeOutlook, "Outlook", platform.AppWebApp("Chrome"), platform.AppWebURL(outlookHome),
		platform.AppLaunchWindows("Outlook"))
	mustOK(t, h.p.LaunchApp(chromeOutlook))
	h.drain()
	w := h.windowID(chromeOutlook, "Outlook")
	if id := h.itemOf(w); id == "" || h.paneOf(id) != "p8" {
		t.Fatalf("the web app's window should be in p8 first: item %q", id)
	}
	h.p.ResetCalls()
	ready, err := h.e.UseWebAppWhenReady("w2")
	mustOK(t, err)
	if !ready {
		t.Fatal("UseWebAppWhenReady = false, want the web app used")
	}
	h.settle()
	p := h.paneIn(0, "p9")
	if len(p.Items) != 1 || h.itemOf(w) != p.Items[0] || p.Active != p.Items[0] {
		t.Errorf("p9 = %v showing %s, want the web app's window, item %s", p.Items, p.Active, h.itemOf(w))
	}
	if p := h.paneIn(0, "p8"); !slices.Equal(p.Items, []string{"w1"}) || p.Active != "w1" {
		t.Errorf("p8 = %v showing %s, want only Slack's page", p.Items, p.Active)
	}
	if got := h.appItems(chromeOutlook); len(got) != 1 {
		t.Errorf("the web app's items = %+v, want one", got)
	}
	if got := h.calls("NewWindow", "OpenApp"); len(got) != 0 {
		t.Errorf("Bench called %v, want no other window", got)
	}
}

func TestUseWebAppWhenReadyAfterThePageHasGone(t *testing.T) {
	h := newHarness(t, setup{world: makerWorld})
	_, err := h.e.MakeWebApp("w2")
	mustOK(t, err)
	h.drain()
	// The page closes while the browser makes the web app.
	mustOK(t, h.e.PopOut("w2"))
	h.p.EmitEvent(platform.Event{Kind: platform.WebViewClosed, ItemID: "w2"})
	h.advance(3 * time.Second)
	ready, err := h.e.UseWebAppWhenReady("w2")
	if ready {
		t.Error("UseWebAppWhenReady = true for a page that has gone")
	}
	mustErr(t, err, "That window has closed.")
	if got := h.appItems(madeBySafari); len(got) != 0 {
		t.Errorf("the web app's items = %+v, want none", got)
	}
	// Bench has stopped waiting: Safari's new windows join the tab again.
	h.e.lock()
	browser := h.e.makingBrowserLocked()
	h.e.unlock()
	if browser != "" {
		t.Errorf("Bench still waits for a web app from %s", browser)
	}
}

func TestMakeWebAppRefusals(t *testing.T) {
	tests := []struct {
		name      string
		world     func(p *platform.Fake)
		untrusted bool
		item      string
		want      string
	}{
		{"without Accessibility permission", makerWorld, true, "w2",
			"Bench needs Accessibility permission to dock web apps."},
		{"a browser that can't make web apps", func(p *platform.Fake) {
			standardWorld(p)
			p.InstallApp(firefoxID, "Firefox")
			p.SetDefaultBrowser(firefoxID)
		}, false, "w2", "Firefox can't make web apps."},
		{"a browser Bench doesn't know", func(p *platform.Fake) {
			standardWorld(p)
			p.InstallApp(heliumID, "Helium")
			p.SetDefaultBrowser(heliumID)
		}, false, "w2", "Bench doesn't know how Helium makes web apps."},
		{"no default browser", nil, false, "w2", "Bench couldn't find a browser to make a web app."},
		{"an App item", makerWorld, false, "w3", "Only a web page can become a web app."},
		{"an item that has gone", makerWorld, false, "w99", "That window has closed."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, setup{world: tt.world, untrusted: tt.untrusted})
			h.p.ResetCalls()
			_, err := h.e.MakeWebApp(tt.item)
			mustErr(t, err, tt.want)
			if got := h.calls("OpenApp"); len(got) != 0 {
				t.Errorf("Bench called %v", got)
			}
		})
	}
	h := newHarness(t, setup{world: makerWorld})
	h.p.SetErr("OpenApp", errors.New("LaunchServices said no"))
	_, err := h.e.MakeWebApp("w2")
	mustErr(t, err, "Bench couldn't open outlook.office.com in Safari.")
	h.e.lock()
	browser := h.e.makingBrowserLocked()
	h.e.unlock()
	if browser != "" {
		t.Errorf("Bench waits for a web app from %s, which it couldn't open the site in", browser)
	}
	_, err = h.e.UseWebAppWhenReady("w3")
	mustErr(t, err, "Only a web page can become a web app.")
}

// TestNoWebAppForAPageOnThisMac checks that a web page on this Mac, such as
// a development server, is offered no web app, even with a default browser
// that can make one, and that Use and Make refuse it, as AddWebsite adds it
// without one: a web app of it would outlive the server.
func TestNoWebAppForAPageOnThisMac(t *testing.T) {
	for _, address := range []string{"http://localhost:3000", "http://[::1]:8080", "https://localhost:8443"} {
		t.Run(address, func(t *testing.T) {
			h := newHarness(t, setup{world: makerWorld})
			mustOK(t, h.e.AddToPane("p8", Source{Type: "web", URL: address}))
			items := h.webItems(address)
			if len(items) != 1 {
				t.Fatalf("Web items for %s = %+v, want one", address, items)
			}
			id := items[0].ID
			got, err := h.e.WebAppChoice(id)
			mustOK(t, err)
			if got != (WebAppChoice{}) {
				t.Errorf("WebAppChoice = %+v, want nothing", got)
			}
			h.p.ResetCalls()
			_, err = h.e.MakeWebApp(id)
			mustErr(t, err, "Only a web page can become a web app.")
			if got := h.calls("OpenApp"); len(got) != 0 {
				t.Errorf("MakeWebApp called %v", got)
			}
			mustErr(t, h.e.UseWebApp(id), "Only a web page can become a web app.")
		})
	}
}
