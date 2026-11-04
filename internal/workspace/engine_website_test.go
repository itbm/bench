package workspace

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// The browsers and web apps in these tests.
const (
	chromeID      = "com.google.Chrome"
	firefoxID     = "org.mozilla.firefox"
	orionID       = "com.kagi.kagimacOS"
	heliumID      = "net.imput.helium"
	chromeSquoosh = "com.google.Chrome.app.squoosh"
	safariSquoosh = "com.apple.Safari.WebApp.squoosh"
	// squooshURL is the address that squoosh.app's web apps record.
	squooshURL  = "https://squoosh.app/?utm_medium=PWA&utm_source=launcher"
	safariSteps = "Choose File ▸ Add to Dock, then click Add."
)

// appItems returns the items of an app's windows, by ID.
func (h *harness) appItems(bundleID string) []ItemView {
	var out []ItemView
	for _, it := range h.view().Items {
		if it.Kind == KindApp && it.BundleID == bundleID {
			out = append(out, it)
		}
	}
	slices.SortFunc(out, func(a, b ItemView) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// webItems returns the Web items for an address, by ID.
func (h *harness) webItems(address string) []ItemView {
	var out []ItemView
	for _, it := range h.view().Items {
		if it.Kind == KindWeb && it.URL == address {
			out = append(out, it)
		}
	}
	slices.SortFunc(out, func(a, b ItemView) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// webPageRow returns the add dialog's Web page… row for a pane.
func (h *harness) webPageRow(paneID string) AddOption {
	h.t.Helper()
	opts, err := h.e.AddOptions(paneID)
	mustOK(h.t, err)
	for _, o := range opts {
		if o.AsksForURL {
			return o
		}
	}
	h.t.Fatal("the add dialog has no Web page… row")
	return AddOption{}
}

// shownIn reports whether an item is a pane's active item.
func (h *harness) shownIn(id, paneID string) bool {
	for _, p := range h.view().Display.Panes {
		if p.ID == paneID {
			return p.Active == id
		}
	}
	return false
}

func TestAddWebsiteAddsTheSitesWebApp(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetDefaultBrowser(safariID)
		p.InstallApp(chromeSquoosh, "Squoosh", platform.AppWebApp("Chrome"), platform.AppWebURL(squooshURL),
			platform.AppLaunchWindows("Squoosh"))
		// Another host of the same domain is another site.
		p.InstallApp("com.google.Chrome.app.docs", "Docs", platform.AppWebApp("Chrome"),
			platform.AppWebURL("https://docs.squoosh.app/"), platform.AppLaunchWindows("Docs"))
	}})
	mustOK(t, h.e.SwitchTab(1))
	h.p.ResetCalls()
	res, err := h.e.AddWebsite("p12", "www.Squoosh.app/editor")
	mustOK(t, err)
	want := WebsiteResult{Added: "web-app", App: "Squoosh", Browser: "Chrome", Note: "Added the Squoosh web app from Chrome."}
	if res != want {
		t.Errorf("AddWebsite = %+v, want %+v", res, want)
	}
	h.drain()
	items := h.appItems(chromeSquoosh)
	if len(items) != 1 || items[0].Pane != "p12" || !h.shownIn(items[0].ID, "p12") {
		t.Fatalf("the web app's items = %+v, want one shown in p12", items)
	}
	// Bench opened the web app, not the browser, and added no Web item.
	for _, c := range h.calls("OpenApp") {
		if c.BundleID != chromeSquoosh || c.Arg != "" {
			t.Errorf("Bench called %v, want only the web app opened", c)
		}
	}
	if got := h.webItems("https://www.Squoosh.app/editor"); len(got) != 0 {
		t.Errorf("Web items = %+v, want none", got)
	}
	if got := h.appItems("com.google.Chrome.app.docs"); len(got) != 0 {
		t.Errorf("the other site's web app was added too: %+v", got)
	}
}

func TestWebAppFor(t *testing.T) {
	// As InstalledApps lists them, by name.
	installed := []platform.AppInfo{
		{BundleID: "com.brave.Browser.app.1", Name: "A Squoosh", WebApp: "Brave", WebURL: "https://squoosh.app/"},
		{BundleID: "com.google.Chrome.app.1", Name: "B Squoosh", WebApp: "Chrome", WebURL: "https://www.squoosh.app/editor"},
		{BundleID: "com.apple.Safari.WebApp.1", Name: "C Squoosh", WebApp: "Safari", WebURL: squooshURL},
		{BundleID: "com.google.Chrome.app.2", Name: "Gmail", WebApp: "Chrome", WebURL: "https://mail.google.com/mail/u/0/"},
		// No address, and not a web app.
		{BundleID: "com.kagi.kagimacOS.WebApp.1", Name: "Orion Squoosh", WebApp: "Orion"},
		{BundleID: "app.squoosh.desktop", Name: "Squoosh Desktop", WebURL: "https://squoosh.app/"},
	}
	withoutSafari := slices.DeleteFunc(slices.Clone(installed), func(a platform.AppInfo) bool { return a.WebApp == "Safari" })
	tests := []struct {
		name      string
		installed []platform.AppInfo
		address   string
		browser   string
		want      string // "" for none.
	}{
		{"the default browser's", installed, "https://squoosh.app", "com.google.Chrome.beta", "com.google.Chrome.app.1"},
		{"the default browser's, which is Safari", installed, "https://squoosh.app", safariID, "com.apple.Safari.WebApp.1"},
		{"else Safari's", installed, "https://squoosh.app", firefoxID, "com.apple.Safari.WebApp.1"},
		{"else Safari's, with no default browser", installed, "https://squoosh.app", "", "com.apple.Safari.WebApp.1"},
		{"else the first by name", withoutSafari, "https://squoosh.app", firefoxID, "com.brave.Browser.app.1"},
		{"by host", installed, "https://mail.google.com/", chromeID, "com.google.Chrome.app.2"},
		{"not another host of the domain", installed, "https://docs.google.com/", chromeID, ""},
		{"none for the site", installed, "https://example.com/", safariID, ""},
		{"none installed", nil, "https://squoosh.app", safariID, ""},
	}
	for _, tt := range tests {
		got, ok := webAppFor(tt.installed, tt.address, tt.browser)
		if ok != (tt.want != "") || got.BundleID != tt.want {
			t.Errorf("%s: webAppFor = %q, %v; want %q", tt.name, got.BundleID, ok, tt.want)
		}
	}
}

func TestAddWebsiteWithoutAccessibility(t *testing.T) {
	h := newHarness(t, setup{untrusted: true, world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetDefaultBrowser(safariID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	if o := h.webPageRow("p12"); o.WebAppBrowser != "" || o.Hint != "Opens in a Bench web view" {
		t.Errorf("Web page… = %+v, want Bench to show pages itself", o)
	}
	h.p.ResetCalls()
	res, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	want := WebsiteResult{Added: "web", Note: "Bench shows this page itself, because it needs Accessibility permission to dock web apps."}
	if res != want {
		t.Errorf("AddWebsite = %+v, want %+v", res, want)
	}
	items := h.webItems("https://squoosh.app")
	if len(items) != 1 || items[0].Pane != "p12" || items[0].App != "Squoosh" {
		t.Errorf("Web items = %+v, want one in p12", items)
	}
	if got := h.calls("OpenApp"); len(got) != 0 {
		t.Errorf("Bench called %v, want no app opened", got)
	}
}

func TestAddWebsiteWithABrowserThatCantMakeWebApps(t *testing.T) {
	tests := []struct {
		name, browser, note string
	}{
		{"Firefox", firefoxID, "Firefox can't make web apps, so Bench shows the page itself."},
		{"Arc", "company.thebrowser.Browser", "Arc can't make web apps, so Bench shows the page itself."},
		// A browser built on Chromium that Bench doesn't know may make web
		// apps, but Bench doesn't know its steps.
		{"a browser Bench doesn't know", heliumID, "Bench doesn't know how Helium makes web apps, so it shows the page itself."},
		{"no default browser", "", "Bench couldn't find a browser to make a web app, so it shows the page itself."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, setup{world: func(p *platform.Fake) {
				standardWorld(p)
				p.InstallApp(firefoxID, "Firefox")
				p.InstallApp("company.thebrowser.Browser", "Arc")
				p.InstallApp(heliumID, "Helium")
				p.SetDefaultBrowser(tt.browser)
			}})
			mustOK(t, h.e.SwitchTab(1))
			if o := h.webPageRow("p12"); o.WebAppBrowser != "" || o.Hint != "Opens in a Bench web view" {
				t.Errorf("Web page… = %+v, want Bench to show pages itself", o)
			}
			h.p.ResetCalls()
			res, err := h.e.AddWebsite("p12", "squoosh.app")
			mustOK(t, err)
			if want := (WebsiteResult{Added: "web", Note: tt.note}); res != want {
				t.Errorf("AddWebsite = %+v, want %+v", res, want)
			}
			items := h.webItems("https://squoosh.app")
			if len(items) != 1 || items[0].Pane != "p12" || !h.shownIn(items[0].ID, "p12") {
				t.Errorf("Web items = %+v, want one shown in p12", items)
			}
			if wv := h.p.WebViews()[items[0].ID]; !wv.Visible || wv.View.URL != "https://squoosh.app" {
				t.Errorf("web view: %+v", wv)
			}
			if got := h.calls("OpenApp"); len(got) != 0 {
				t.Errorf("Bench called %v, want no app opened", got)
			}
		})
	}
}

func TestAddWebsiteOpensTheSiteInTheDefaultBrowser(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetDefaultBrowser(safariID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	if o := h.webPageRow("p12"); o.WebAppBrowser != "Safari" || o.Hint != "As a web app, or in Bench" {
		t.Errorf("Web page… = %+v, want Safari to make web apps", o)
	}
	before := slices.Clone(h.e.model.Tabs[1].Pane("p12").Items)
	h.p.ResetCalls()
	res, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	if want := (WebsiteResult{Browser: "Safari", Steps: safariSteps}); res != want {
		t.Errorf("AddWebsite = %+v, want %+v", res, want)
	}
	if got, want := h.p.OpenedURLs(), []platform.OpenedURL{{BundleID: safariID, URL: "https://squoosh.app"}}; !slices.Equal(got, want) {
		t.Errorf("OpenedURLs = %+v, want %+v", got, want)
	}
	h.drain()
	if got := h.e.model.Tabs[1].Pane("p12").Items; !slices.Equal(got, before) {
		t.Errorf("p12 = %v, want nothing added yet (%v)", got, before)
	}
	if got := h.webItems("https://squoosh.app"); len(got) != 0 {
		t.Errorf("Web items = %+v, want none", got)
	}

	// Until the user has made the web app, there's nothing to add.
	ready, err := h.e.AddWebAppWhenReady("p12", "squoosh.app")
	mustOK(t, err)
	if ready {
		t.Fatal("AddWebAppWhenReady = true before the web app was made")
	}
	// Safari adds the web app to the Dock, without opening it.
	h.p.InstallApp(safariSquoosh, "Squoosh", platform.AppWebApp("Safari"), platform.AppWebURL(squooshURL),
		platform.AppLaunchWindows("Squoosh"))
	ready, err = h.e.AddWebAppWhenReady("p12", "squoosh.app")
	mustOK(t, err)
	if !ready {
		t.Fatal("AddWebAppWhenReady = false once the web app was made")
	}
	h.drain()
	items := h.appItems(safariSquoosh)
	if len(items) != 1 || items[0].Pane != "p12" || !h.shownIn(items[0].ID, "p12") {
		t.Errorf("the web app's items = %+v, want one shown in p12", items)
	}
}

func TestAddWebAppWhenReadyWaitsForTheBrowser(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(safariID, "Safari", platform.AppMakesWebApps(3*time.Second))
		p.SetDefaultBrowser(safariID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	res, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	if res.Added != "" || res.Browser != "Safari" {
		t.Fatalf("AddWebsite = %+v, want Safari to make the web app", res)
	}
	// The dialog asks every 1.5 seconds, as the browser takes 3.
	polls := 0
	for {
		polls++
		ready, err := h.e.AddWebAppWhenReady("p12", "squoosh.app")
		mustOK(t, err)
		if ready {
			break
		}
		if polls == 4 {
			t.Fatal("no web app after 4.5 seconds")
		}
		h.advance(1500 * time.Millisecond)
	}
	if polls != 3 {
		t.Errorf("the web app was added at poll %d, want 3", polls)
	}
	h.drain()
	items := h.appItems("com.apple.Safari.WebApp.1")
	if len(items) != 1 || items[0].Pane != "p12" || items[0].Title != "Squoosh" {
		t.Errorf("the web app's items = %+v, want one in p12", items)
	}
}

func TestAddWebsiteKeepsTheTabWhileTheUserIsInTheBrowser(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetDefaultBrowser(braveID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	res, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	if res.Browser != "Brave" || !strings.HasPrefix(res.Steps, "Choose Brave's command") {
		t.Fatalf("AddWebsite = %+v, want Brave to make the web app", res)
	}
	h.drain()
	// Brave's windows are in Research, and follow_focus is switch-tab, but
	// while the add dialog waits, the user's clicks in Brave don't switch
	// tabs, which would close the dialog.
	focus := func(title string) {
		t.Helper()
		mustOK(t, h.p.FocusWindow(h.windowID(braveID, title)))
		h.drain()
	}
	h.advance(2 * time.Second)
	focus("Wails v3 docs")
	if got := h.view().Current; got != 1 {
		t.Fatalf("focusing Brave as the dialog waited switched to tab %d", got)
	}
	for range 4 {
		ready, err := h.e.AddWebAppWhenReady("p12", "squoosh.app")
		mustOK(t, err)
		if ready {
			t.Fatal("AddWebAppWhenReady = true, want no web app yet")
		}
		h.advance(1500 * time.Millisecond)
	}
	focus("Pull request #42 · bench")
	if got := h.view().Current; got != 1 {
		t.Fatalf("focusing Brave as the dialog waited switched to tab %d", got)
	}
	// Once the dialog stops asking, focus follows Brave's windows again.
	h.advance(6 * time.Second)
	focus("Go packages")
	if got := h.view().Current; got != 2 {
		t.Errorf("focusing Brave after the dialog stopped left tab %d, want Research", got)
	}
}

func TestAddWebAppWhenReadyMovesTheWindowBenchDocked(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(chromeID, "Google Chrome")
		p.SetDefaultBrowser(chromeID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	if _, err := h.e.AddWebsite("p12", "squoosh.app"); err != nil {
		t.Fatal(err)
	}
	h.drain()
	// Chrome makes the web app and opens it at once. Bench puts its window
	// in the focused pane, as new_windows says, before the dialog asks.
	h.p.InstallApp(chromeSquoosh, "Squoosh", platform.AppWebApp("Chrome"), platform.AppWebURL(squooshURL),
		platform.AppLaunchWindows("Squoosh"))
	mustOK(t, h.p.LaunchApp(chromeSquoosh))
	h.drain()
	w := h.windowID(chromeSquoosh, "Squoosh")
	id := h.itemOf(w)
	if id == "" || h.paneOf(id) == "p12" {
		t.Fatalf("the web app's window should be in another pane first: item %q", id)
	}
	h.p.ResetCalls()
	ready, err := h.e.AddWebAppWhenReady("p12", "squoosh.app")
	mustOK(t, err)
	if !ready {
		t.Fatal("AddWebAppWhenReady = false, want the web app added")
	}
	h.settle()
	items := h.appItems(chromeSquoosh)
	if len(items) != 1 || items[0].ID != id || items[0].Pane != "p12" || !h.shownIn(id, "p12") {
		t.Errorf("the web app's items = %+v, want %s moved to p12 and shown", items, id)
	}
	if got := h.calls("NewWindow", "OpenApp"); len(got) != 0 {
		t.Errorf("Bench called %v, want no other window", got)
	}
}

func TestAddWebAppWhenReadyWaitsForAStartingWebAppsWindow(t *testing.T) {
	for _, opens := range []bool{true, false} {
		name := "it opens a window"
		if !opens {
			name = "it opens none"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, setup{world: func(p *platform.Fake) {
				standardWorld(p)
				p.InstallApp(chromeID, "Google Chrome")
				p.SetDefaultBrowser(chromeID)
			}})
			mustOK(t, h.e.SwitchTab(1))
			if _, err := h.e.AddWebsite("p12", "squoosh.app"); err != nil {
				t.Fatal(err)
			}
			h.drain()
			// The dialog asks just as Chrome starts the web app, before its
			// window opens.
			h.p.InstallApp(chromeSquoosh, "Squoosh", platform.AppWebApp("Chrome"), platform.AppWebURL(squooshURL))
			mustOK(t, h.p.LaunchApp(chromeSquoosh))
			h.drain()
			h.p.ResetCalls()
			ready, err := h.e.AddWebAppWhenReady("p12", "squoosh.app")
			mustOK(t, err)
			if !ready {
				t.Fatal("AddWebAppWhenReady = false, want the web app added")
			}
			if got := h.calls("NewWindow"); len(got) != 0 {
				t.Fatalf("Bench opened a new window at once: %v", got)
			}
			if opens {
				h.advance(500 * time.Millisecond)
				_, err := h.p.OpenWindow(chromeSquoosh, "Squoosh")
				mustOK(t, err)
			}
			h.settle()
			items := h.appItems(chromeSquoosh)
			if len(items) != 1 || items[0].Pane != "p12" {
				t.Errorf("the web app's items = %+v, want one in p12", items)
			}
			if got, want := len(h.calls("NewWindow")), map[bool]int{true: 0, false: 1}[opens]; got != want {
				t.Errorf("Bench opened %d new windows, want %d", got, want)
			}
		})
	}
}

func TestAddWebsiteRefusals(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetDefaultBrowser(safariID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	for _, call := range []func(pane, address string) error{
		func(pane, address string) error { _, err := h.e.AddWebsite(pane, address); return err },
		func(pane, address string) error { _, err := h.e.AddWebAppWhenReady(pane, address); return err },
	} {
		mustErr(t, call("p12", "http://squoosh.app"), "That address isn't a secure web address (https).")
		mustErr(t, call("p12", "  "), "Enter a web address, such as https://example.com.")
		mustErr(t, call("p99", "squoosh.app"), "There's no such pane.")
		// The add dialog's pane is in another tab now.
		mustOK(t, h.e.SwitchTab(0))
		mustErr(t, call("p12", "squoosh.app"), "There's no such pane.")
		mustOK(t, h.e.SwitchTab(1))
	}
	h.p.SetErr("OpenApp", errors.New("LaunchServices said no"))
	_, err := h.e.AddWebsite("p12", "squoosh.app")
	mustErr(t, err, "Bench couldn't open squoosh.app in Safari.")
	if got := h.webItems("https://squoosh.app"); len(got) != 0 {
		t.Errorf("Web items = %+v, want none", got)
	}
}

func TestAddWebAppWhenReadyAddsTheNewWebAppOnAnotherHost(t *testing.T) {
	tests := []struct {
		name, browser, address string
		id, app                string
		opts                   []platform.AppOption
	}{
		// slack.com sends a signed-in user to app.slack.com, and Chrome
		// records the address it ended on.
		{"a site that redirects", chromeID, "slack.com", "com.google.Chrome.app.slack", "Slack",
			[]platform.AppOption{platform.AppWebApp("Chrome"), platform.AppWebURL("https://app.slack.com/client/T01/C01")}},
		// Bench doesn't know where Orion records a web app's address.
		{"Orion, which records none", orionID, "squoosh.app", "com.kagi.kagimacOS.WebApp.1234", "Squoosh",
			[]platform.AppOption{platform.AppWebApp("Orion")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, setup{world: func(p *platform.Fake) {
				standardWorld(p)
				p.InstallApp(chromeID, "Google Chrome")
				p.InstallApp(orionID, "Orion")
				// A web app that the user had already, of another site.
				p.InstallApp("com.google.Chrome.app.gmail", "Gmail", platform.AppWebApp("Chrome"),
					platform.AppWebURL("https://mail.google.com/mail/u/0/"))
				p.SetDefaultBrowser(tt.browser)
			}})
			mustOK(t, h.e.SwitchTab(1))
			res, err := h.e.AddWebsite("p12", tt.address)
			mustOK(t, err)
			if res.Added != "" || res.Steps == "" {
				t.Fatalf("AddWebsite = %+v, want the browser to make the web app", res)
			}
			ready, err := h.e.AddWebAppWhenReady("p12", tt.address)
			mustOK(t, err)
			if ready {
				t.Fatal("AddWebAppWhenReady = true before the web app was made")
			}
			h.p.InstallApp(tt.id, tt.app, append(tt.opts, platform.AppLaunchWindows(tt.app))...)
			// It's the site's only for the pane and address that AddWebsite
			// opened the site for.
			for _, other := range [][2]string{{"p11", tt.address}, {"p12", "example.com"}} {
				ready, err := h.e.AddWebAppWhenReady(other[0], other[1])
				mustOK(t, err)
				if ready {
					t.Fatalf("AddWebAppWhenReady%q = true, want the web app left for the address AddWebsite opened", other)
				}
			}
			ready, err = h.e.AddWebAppWhenReady("p12", tt.address)
			mustOK(t, err)
			if !ready {
				t.Fatal("AddWebAppWhenReady = false once the web app was made")
			}
			h.drain()
			items := h.appItems(tt.id)
			if len(items) != 1 || items[0].Pane != "p12" || !h.shownIn(items[0].ID, "p12") {
				t.Errorf("the web app's items = %+v, want one shown in p12", items)
			}
			if got := h.appItems("com.google.Chrome.app.gmail"); len(got) != 0 {
				t.Errorf("the other site's web app was added too: %+v", got)
			}
		})
	}
}

func TestAddWebAppWhenReadyKeepsWaitingAfterAFailure(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(orionID, "Orion")
		p.SetDefaultBrowser(orionID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	_, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	h.drain()
	// Orion records no address, so the web app is the site's only as the one
	// made since Bench opened the site.
	const id = "com.kagi.kagimacOS.WebApp.1234"
	h.p.InstallApp(id, "Squoosh", platform.AppWebApp("Orion"), platform.AppLaunchWindows("Squoosh"))
	h.p.SetErr("OpenApp", errors.New("LaunchServices said no"))
	ready, err := h.e.AddWebAppWhenReady("p12", "squoosh.app")
	mustErr(t, err, "Bench couldn't open Squoosh.")
	if ready {
		t.Fatal("AddWebAppWhenReady = true, want the failure")
	}
	// Bench still waits for the web app, and leaves the browser alone.
	h.e.lock()
	browser := h.e.makingBrowserLocked()
	h.e.unlock()
	if browser != orionID {
		t.Errorf("Bench waits for a web app from %q, want Orion", browser)
	}
	h.p.SetErr("OpenApp", nil)
	ready, err = h.e.AddWebAppWhenReady("p12", "squoosh.app")
	mustOK(t, err)
	if !ready {
		t.Fatal("AddWebAppWhenReady = false once the web app could open")
	}
	h.settle()
	if items := h.appItems(id); len(items) != 1 || items[0].Pane != "p12" {
		t.Errorf("the web app's items = %+v, want one in p12", items)
	}
}

func TestAddWebAppWhenReadyDoesntGuessBetweenNewWebApps(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(chromeID, "Google Chrome")
		p.SetDefaultBrowser(chromeID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	_, err := h.e.AddWebsite("p12", "slack.com")
	mustOK(t, err)
	h.p.InstallApp("com.google.Chrome.app.a", "Slack", platform.AppWebApp("Chrome"), platform.AppWebURL("https://app.slack.com/client"))
	h.p.InstallApp("com.google.Chrome.app.b", "Docs", platform.AppWebApp("Chrome"), platform.AppWebURL("https://docs.google.com/"))
	ready, err := h.e.AddWebAppWhenReady("p12", "slack.com")
	mustOK(t, err)
	if ready {
		t.Fatalf("AddWebAppWhenReady = true with two new web apps on other hosts: %+v %+v",
			h.appItems("com.google.Chrome.app.a"), h.appItems("com.google.Chrome.app.b"))
	}
	// One on the site's host is the site's.
	h.p.InstallApp("com.google.Chrome.app.c", "Slack", platform.AppWebApp("Chrome"), platform.AppWebURL("https://slack.com/"),
		platform.AppLaunchWindows("Slack"))
	ready, err = h.e.AddWebAppWhenReady("p12", "slack.com")
	mustOK(t, err)
	if !ready {
		t.Fatal("AddWebAppWhenReady = false with a new web app on the site's host")
	}
	h.drain()
	if items := h.appItems("com.google.Chrome.app.c"); len(items) != 1 || items[0].Pane != "p12" {
		t.Errorf("the web app's items = %+v, want one in p12", items)
	}
}

func TestAddWebsiteLeavesTheBrowserShownWhileTheDialogWaits(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		// Safari is open, with a window that Bench doesn't manage, over Bench.
		p.AddApp(safariID, "Safari")
		p.AddWindow(safariID, "Apple", platform.WithFrame(layout.Rect{X: 200, Y: 100, W: 900, H: 700}))
		p.SetDefaultBrowser(safariID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	_, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	h.drain()
	// The user goes back to Bench to read the steps, and to Safari to
	// follow them, as the dialog asks for the web app.
	for range 3 {
		h.advance(1500 * time.Millisecond)
		ready, err := h.e.AddWebAppWhenReady("p12", "squoosh.app")
		mustOK(t, err)
		if ready {
			t.Fatal("AddWebAppWhenReady = true, want no web app yet")
		}
		mustOK(t, h.p.ActivateApp(benchID))
		h.drain()
		if h.p.IsHidden(safariID) {
			t.Fatal("Bench hid Safari as the dialog waited for its web app")
		}
		mustOK(t, h.p.ActivateApp(safariID))
		h.drain()
	}
	// Once the dialog stops asking, Bench hides it as any other app over it.
	h.advance(6 * time.Second)
	mustOK(t, h.p.ActivateApp(benchID))
	h.drain()
	if !h.p.IsHidden(safariID) {
		t.Error("Safari's window over Bench wasn't hidden once the dialog stopped waiting")
	}
}

func TestHideOtherAppsLeavesTheBrowserShownWhileTheDialogWaits(t *testing.T) {
	cfg := strings.Replace(exampleConfig(t), "hide_other_apps = false", "hide_other_apps = true", 1)
	h := newHarness(t, setup{config: cfg, world: func(p *platform.Fake) {
		standardWorld(p)
		p.AddApp(safariID, "Safari")
		p.AddWindow(safariID, "Apple")
		p.SetDefaultBrowser(safariID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	if !h.p.IsHidden(safariID) {
		t.Fatal("Safari has no items and should be hidden")
	}
	_, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	h.drain()
	// Switching to the current tab, as its shortcut does, and Rearrange hide
	// every app without items, but not the browser the user is making the
	// web app in.
	for _, arrange := range []func() error{func() error { return h.e.SwitchTab(1) }, h.e.Rearrange} {
		h.advance(time.Second)
		finder, _ := h.p.App(finderID)
		mustOK(t, h.p.Unhide(finder))
		mustOK(t, arrange())
		h.drain()
		if h.p.IsHidden(safariID) {
			t.Fatal("Bench hid Safari as the dialog waited for its web app")
		}
		if !h.p.IsHidden(finderID) {
			t.Error("Finder has no items and should be hidden")
		}
	}
	// Once the dialog stops asking, Bench hides it as any other app.
	h.advance(6 * time.Second)
	mustOK(t, h.e.Rearrange())
	h.drain()
	if !h.p.IsHidden(safariID) {
		t.Error("Safari wasn't hidden once the dialog stopped waiting")
	}
}

func TestHideOtherTabsLeavesTheBrowserShownWhileTheDialogWaits(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetDefaultBrowser(braveID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	// All of Brave's windows are in Research.
	if !h.p.IsHidden(braveID) {
		t.Fatal("Brave's windows are all in another tab, and it should be hidden")
	}
	_, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	h.drain()
	h.advance(time.Second)
	mustOK(t, h.e.Rearrange())
	h.drain()
	if h.p.IsHidden(braveID) {
		t.Fatal("Bench hid Brave as the dialog waited for its web app")
	}
	// Its windows in other tabs stay where they are: Brave opened the site
	// in one of them.
	brave, _ := h.p.App(braveID)
	ws, err := h.p.Windows(brave)
	mustOK(t, err)
	if len(ws) != 3 {
		t.Fatalf("Brave has %d windows, want its 3 in Research", len(ws))
	}
	for _, w := range ws {
		if h.e.win.IsParked(w.ID) {
			t.Errorf("Bench parked Brave's window %q as the dialog waited", w.Title)
		}
	}
	h.advance(6 * time.Second)
	mustOK(t, h.e.Rearrange())
	h.drain()
	if !h.p.IsHidden(braveID) {
		t.Error("Brave wasn't hidden once the dialog stopped waiting")
	}
}

func TestAddWebsiteKeepsTheBrowsersNewWindowOutOfTheTab(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.SetDefaultBrowser(safariID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	_, err := h.e.AddWebsite("p12", "squoosh.app")
	mustOK(t, err)
	h.settle()
	// Safari wasn't running, and opens with a window, which new_windows
	// would put in the focused pane: the user is only to make the web app
	// in it, so it stays out of the tab, where the add dialog lists it.
	w := h.windowID(safariID, "Apple")
	if id := h.itemOf(w); id != "" {
		t.Fatalf("Safari's window became item %s in %s", id, h.paneOf(id))
	}
	if !slices.ContainsFunc(h.view().Floating, func(it ItemView) bool { return it.BundleID == safariID && it.Title == "Apple" }) {
		t.Errorf("Floating = %+v, want Safari's window", h.view().Floating)
	}
	h.p.InstallApp(safariSquoosh, "Squoosh", platform.AppWebApp("Safari"), platform.AppWebURL(squooshURL),
		platform.AppLaunchWindows("Squoosh"))
	ready, err := h.e.AddWebAppWhenReady("p12", "squoosh.app")
	mustOK(t, err)
	if !ready {
		t.Fatal("AddWebAppWhenReady = false once the web app was made")
	}
	h.settle()
	if items := h.appItems(safariSquoosh); len(items) != 1 || items[0].Pane != "p12" {
		t.Errorf("the web app's items = %+v, want one in p12", items)
	}
	if got := h.appItems(safariID); len(got) != 0 {
		t.Errorf("Safari's items = %+v, want none", got)
	}
	// Once the dialog has stopped waiting, Safari's new windows join the
	// tab as new_windows says.
	later, err := h.p.OpenWindow(safariID, "Later")
	mustOK(t, err)
	h.drain()
	if id := h.itemOf(later.ID); id == "" || h.item(id).Tab != 1 {
		t.Errorf("Safari's later window: item %q, want one in this tab", id)
	}
}
