package apps

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/itbm/bench/internal/platform"
)

// TestDirsFindsWebAppFolders checks that Dirs adds the folders where
// browsers keep web apps, after ~/Applications, and only folders.
func TestDirsFindsWebAppFolders(t *testing.T) {
	home := t.TempDir()
	apps := filepath.Join(home, "Applications")
	for _, dir := range []string{"Chrome Apps.localized", "Brave Browser Apps.localized", "Edge Apps.localized", ".Hidden.localized", "Games"} {
		if err := os.MkdirAll(filepath.Join(apps, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A file with the suffix isn't a folder of web apps.
	if err := os.WriteFile(filepath.Join(apps, "Notes.localized"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/Applications",
		"/Applications/Utilities",
		apps,
		filepath.Join(apps, "Utilities"),
		filepath.Join(apps, "Brave Browser Apps.localized"),
		filepath.Join(apps, "Chrome Apps.localized"),
		filepath.Join(apps, "Edge Apps.localized"),
		filepath.Join(apps, "Orion", "WebApps"),
		"/System/Applications",
		"/System/Applications/Utilities",
	}
	if got := Dirs(home); !reflect.DeepEqual(got, want) {
		t.Errorf("Dirs = %q, want %q", got, want)
	}
}

// chromiumWebApp returns the Info.plist of a web app that a Chromium browser
// with bundle ID browser made, as Chromium writes it: CFBundleName is the
// file name, and the site's own name is in CrAppModeShortcutName.
func chromiumWebApp(id, browser, fileName, name string) map[string]any {
	return map[string]any{
		"CFBundleIdentifier":    id,
		"CFBundleName":          fileName,
		"CFBundleExecutable":    "app_mode_loader",
		"CrBundleIdentifier":    browser,
		"CrAppModeShortcutID":   "abcdefghijklmnop",
		"CrAppModeShortcutName": name,
		"CrAppModeShortcutURL":  "https://app.example/",
	}
}

// TestScanFindsWebApps checks that Scan finds the web apps of every browser
// that makes them on macOS, in the folders each keeps them in, with the
// site's name and the browser that made each.
func TestScanFindsWebApps(t *testing.T) {
	home := t.TempDir()
	apps := filepath.Join(home, "Applications")
	in := func(dir string, b bundle) {
		t.Helper()
		folder := filepath.Join(apps, dir)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		makeBundle(t, folder, b)
	}
	// Safari's have no executable of their own.
	in("", bundle{name: "Calendar.app", info: map[string]any{
		"CFBundleIdentifier":    "com.apple.Safari.WebApp.6C1F7E36-0B3A-4C5E-9D2B-1A2B3C4D5E6F",
		"CFBundleName":          "Calendar",
		"LSTemplateApplication": true,
		"Manifest":              map[string]any{"start_url": "https://calendar.example/"},
	}})
	in("Chrome Apps.localized", bundle{name: "Slack - Work.app",
		info: chromiumWebApp("com.google.Chrome.app.aaaa", "com.google.Chrome", "Slack - Work", "Slack: Work")})
	in("Chrome Apps.localized", bundle{name: "Docs.app",
		info: chromiumWebApp("com.google.Chrome.beta.app.bbbb", "com.google.Chrome.beta", "Docs", "Docs")})
	in("Chromium Apps.localized", bundle{name: "Tester.app",
		info: chromiumWebApp("com.google.chrome.for.testing.app.cccc", "com.google.chrome.for.testing", "Tester", "Tester")})
	in("Edge Apps.localized", bundle{name: "Outlook.app",
		info: chromiumWebApp("com.microsoft.edgemac.app.dddd", "com.microsoft.edgemac", "Outlook", "Outlook")})
	in("Brave Browser Apps.localized", bundle{name: "Mastodon.app",
		info: chromiumWebApp("com.brave.Browser.app.eeee", "com.brave.Browser", "Mastodon", "Mastodon")})
	in("Vivaldi Apps.localized", bundle{name: "Squoosh.app",
		info: chromiumWebApp("com.vivaldi.Vivaldi.app.ffff", "com.vivaldi.Vivaldi", "Squoosh", "Squoosh")})
	in("Helium Apps.localized", bundle{name: "Notes.app",
		info: chromiumWebApp("net.imput.helium.app.gggg", "net.imput.helium", "Notes", "Notes")})
	in(filepath.Join("Orion", "WebApps"), bundle{name: "Kagi.app",
		info: app("com.kagi.kagimacOS.WebApp.1234", "Kagi")})
	in("", bundle{name: "Zed.app", info: app("dev.zed.Zed", "Zed")})

	path := func(dir, name string) string { return filepath.Join(apps, dir, name) }
	const site = "https://app.example/"
	want := []platform.AppInfo{
		{BundleID: "com.apple.Safari.WebApp.6C1F7E36-0B3A-4C5E-9D2B-1A2B3C4D5E6F", Name: "Calendar", Path: path("", "Calendar.app"), WebApp: "Safari",
			WebURL: "https://calendar.example/"},
		{BundleID: "com.google.Chrome.beta.app.bbbb", Name: "Docs", Path: path("Chrome Apps.localized", "Docs.app"), WebApp: "Chrome", WebURL: site},
		{BundleID: "com.kagi.kagimacOS.WebApp.1234", Name: "Kagi", Path: path(filepath.Join("Orion", "WebApps"), "Kagi.app"), WebApp: "Orion"},
		{BundleID: "com.brave.Browser.app.eeee", Name: "Mastodon", Path: path("Brave Browser Apps.localized", "Mastodon.app"), WebApp: "Brave", WebURL: site},
		{BundleID: "net.imput.helium.app.gggg", Name: "Notes", Path: path("Helium Apps.localized", "Notes.app"), WebApp: "Helium", WebURL: site},
		{BundleID: "com.microsoft.edgemac.app.dddd", Name: "Outlook", Path: path("Edge Apps.localized", "Outlook.app"), WebApp: "Edge", WebURL: site},
		{BundleID: "com.google.Chrome.app.aaaa", Name: "Slack: Work", Path: path("Chrome Apps.localized", "Slack - Work.app"), WebApp: "Chrome", WebURL: site},
		{BundleID: "com.vivaldi.Vivaldi.app.ffff", Name: "Squoosh", Path: path("Vivaldi Apps.localized", "Squoosh.app"), WebApp: "Vivaldi", WebURL: site},
		{BundleID: "com.google.chrome.for.testing.app.cccc", Name: "Tester", Path: path("Chromium Apps.localized", "Tester.app"), WebApp: "Chrome for Testing",
			WebURL: site},
		{BundleID: "dev.zed.Zed", Name: "Zed", Path: path("", "Zed.app")},
	}
	// Only the folders in home count, so that the test doesn't depend on
	// what this computer has installed.
	var dirs []string
	for _, d := range Dirs(home) {
		if filepath.IsAbs(d) && len(d) > len(home) && d[:len(home)] == home {
			dirs = append(dirs, d)
		}
	}
	if got := Scan(dirs); !reflect.DeepEqual(got, want) {
		t.Errorf("Scan =\n%+v\nwant\n%+v", got, want)
	}
}

func TestWebAppBrowser(t *testing.T) {
	tests := []struct {
		info infoPlist
		want string
	}{
		{infoPlist{BundleID: "com.google.Chrome.app.x", Browser: "com.google.Chrome"}, "Chrome"},
		{infoPlist{BundleID: "com.google.Chrome.canary.app.x", Browser: "com.google.Chrome.canary"}, "Chrome"},
		{infoPlist{Browser: "com.google.chrome.for.testing"}, "Chrome for Testing"},
		{infoPlist{Browser: "com.microsoft.edgemac.Beta"}, "Edge"},
		{infoPlist{Browser: "com.brave.Browser.nightly"}, "Brave"},
		{infoPlist{Browser: "com.vivaldi.Vivaldi"}, "Vivaldi"},
		{infoPlist{Browser: "org.chromium.Chromium"}, "Chromium"},
		// An ID that only starts with a known one is another browser.
		{infoPlist{Browser: "com.google.chromecast"}, "Chromecast"},
		{infoPlist{BundleID: "com.apple.Safari.WebApp.ABC"}, "Safari"},
		{infoPlist{BundleID: "COM.APPLE.SAFARI.WEBAPP.ABC"}, "Safari"},
		{infoPlist{BundleID: "com.kagi.kagimacOS.WebApp.123"}, "Orion"},
		// Safari's and Orion's own web app runners aren't web apps.
		{infoPlist{BundleID: "com.apple.Safari.WebApp"}, ""},
		{infoPlist{BundleID: "com.kagi.kagimacOS.WebApp."}, ""},
		{infoPlist{BundleID: "com.apple.Safari"}, ""},
		{infoPlist{BundleID: "dev.zed.Zed"}, ""},
	}
	for _, tt := range tests {
		if got := webAppBrowser(tt.info); got != tt.want {
			t.Errorf("webAppBrowser(%+v) = %q, want %q", tt.info, got, tt.want)
		}
	}
}

// squooshURL is the address that Safari's and Chromium's web apps of
// squoosh.app record, as on a Mac with macOS 15.
const squooshURL = "https://squoosh.app/?utm_medium=PWA&utm_source=launcher"

// safariWebApp returns the Info.plist of a web app that Safari made, with
// extra keys.
func safariWebApp(extra map[string]any) map[string]any {
	info := map[string]any{
		"CFBundleIdentifier":    "com.apple.Safari.WebApp.0B6C4A07-7C1E-4A5B-8F0C-2D3E4F5A6B7C",
		"CFBundleName":          "Squoosh",
		"LSTemplateApplication": true,
	}
	maps.Copy(info, extra)
	return info
}

// chromiumWithURL returns the Info.plist of a Chrome web app whose
// CrAppModeShortcutURL is url, which needn't be a string.
func chromiumWithURL(url any) map[string]any {
	info := chromiumWebApp("com.google.Chrome.app.kkkk", "com.google.Chrome", "Squoosh", "Squoosh")
	info["CrAppModeShortcutURL"] = url
	return info
}

// TestReadBundleWebURL checks that ReadBundle finds the address a web app
// opens where its browser records it, and that keys of the wrong type, or
// addresses that aren't http or https, read as no address, without
// spoiling the rest of the bundle.
func TestReadBundleWebURL(t *testing.T) {
	manifest := map[string]any{"start_url": squooshURL, "name": "Squoosh", "display": "standalone"}
	push := map[string]any{"manifestId": "https://squoosh.app/", "pushPartition": "abc"}
	tests := []struct {
		name   string
		info   map[string]any
		format plistFormat
		want   string
	}{
		{"Safari", safariWebApp(map[string]any{"Manifest": manifest, "WKPushBundleMetadata": push}), xmlPlist, squooshURL},
		{"Safari, binary", safariWebApp(map[string]any{"Manifest": manifest, "WKPushBundleMetadata": push}), binaryPlist, squooshURL},
		{"Safari's manifest ID when its manifest has no start_url",
			safariWebApp(map[string]any{"Manifest": map[string]any{"name": "Squoosh"}, "WKPushBundleMetadata": push}), xmlPlist, "https://squoosh.app/"},
		{"Safari's manifest ID without a manifest", safariWebApp(map[string]any{"WKPushBundleMetadata": push}), binaryPlist, "https://squoosh.app/"},
		{"a manifest that's a string", safariWebApp(map[string]any{"Manifest": squooshURL, "WKPushBundleMetadata": push}), xmlPlist, "https://squoosh.app/"},
		{"a manifest that's an array", safariWebApp(map[string]any{"Manifest": []any{squooshURL}}), binaryPlist, ""},
		{"a start_url that's a number", safariWebApp(map[string]any{"Manifest": map[string]any{"start_url": 42}, "WKPushBundleMetadata": push}),
			xmlPlist, "https://squoosh.app/"},
		{"push settings that are a string", safariWebApp(map[string]any{"Manifest": manifest, "WKPushBundleMetadata": "https://squoosh.app/"}),
			xmlPlist, squooshURL},
		{"push settings that are a number", safariWebApp(map[string]any{"WKPushBundleMetadata": 7}), binaryPlist, ""},
		{"a manifest ID that's an array", safariWebApp(map[string]any{"WKPushBundleMetadata": map[string]any{"manifestId": []any{"x"}}}), xmlPlist, ""},
		{"a start_url that isn't http or https, then the manifest ID",
			safariWebApp(map[string]any{"Manifest": map[string]any{"start_url": "/"}, "WKPushBundleMetadata": push}), xmlPlist, "https://squoosh.app/"},
		{"no address", safariWebApp(nil), xmlPlist, ""},

		{"Chrome", chromiumWithURL(squooshURL), xmlPlist, squooshURL},
		{"Chrome, binary", chromiumWithURL(squooshURL), binaryPlist, squooshURL},
		{"an http address", chromiumWithURL("http://localhost:8080/app"), xmlPlist, "http://localhost:8080/app"},
		{"spaces around it", chromiumWithURL("  HTTPS://Squoosh.app/  "), xmlPlist, "HTTPS://Squoosh.app/"},
		{"an address that's a dictionary", chromiumWithURL(map[string]any{"url": squooshURL}), xmlPlist, ""},
		{"an address that's an array", chromiumWithURL([]any{squooshURL}), binaryPlist, ""},
		{"a file", chromiumWithURL("file:///Users/sam/index.html"), xmlPlist, ""},
		{"a script", chromiumWithURL("javascript:alert(1)"), xmlPlist, ""},
		{"another scheme", chromiumWithURL("ftp://squoosh.app/"), xmlPlist, ""},
		{"no scheme", chromiumWithURL("squoosh.app"), xmlPlist, ""},
		{"a relative address", chromiumWithURL("/index.html"), xmlPlist, ""},
		{"no host", chromiumWithURL("https:///index.html"), xmlPlist, ""},
		{"an address that doesn't parse", chromiumWithURL("https://squoosh app/"), xmlPlist, ""},
		{"an empty address", chromiumWithURL(""), xmlPlist, ""},

		// Orion's keys are unknown, and an app that isn't a web app has no
		// address, whatever its keys.
		{"Orion", map[string]any{"CFBundleIdentifier": "com.kagi.kagimacOS.WebApp.1234", "CFBundleName": "Squoosh",
			"Manifest": manifest, "CrAppModeShortcutURL": squooshURL}, xmlPlist, ""},
		{"not a web app", map[string]any{"CFBundleIdentifier": "app.squoosh.desktop", "CFBundleName": "Squoosh",
			"Manifest": manifest, "CrAppModeShortcutURL": squooshURL}, xmlPlist, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := makeBundle(t, t.TempDir(), bundle{name: "Squoosh.app", info: tt.info, format: tt.format})
			got, err := ReadBundle(path)
			if err != nil {
				t.Fatalf("ReadBundle: %v", err)
			}
			if got.WebURL != tt.want || got.Name != "Squoosh" || got.BundleID != tt.info["CFBundleIdentifier"] {
				t.Errorf("ReadBundle = %+v, want Squoosh with WebURL %q", got, tt.want)
			}
		})
	}
}

func TestWebAppMakerFor(t *testing.T) {
	const window = " If it asks how to open, choose a window, not a tab."
	safari := WebAppMaker{Browser: "Safari", Steps: "Choose File ▸ Add to Dock, then click Add."}
	chrome := WebAppMaker{Browser: "Chrome", Steps: "Open the ⋮ menu and choose Cast, save, and share ▸ Install page as app, " +
		"or, for a site that's made to be installed, Install and the site's name." + window}
	edge := WebAppMaker{Browser: "Edge", Steps: "Open the … menu and choose More tools ▸ Apps ▸ Install this site as an app." + window}
	generic := func(name string) WebAppMaker {
		return WebAppMaker{Browser: name, Steps: "Choose " + name + "'s command to install the page as an app, in its menu or its address bar." + window}
	}
	tests := []struct {
		id   string
		want WebAppMaker // Browser "" for a browser that can't.
	}{
		{"com.apple.Safari", safari},
		{"COM.APPLE.SAFARI", safari},
		{"com.apple.SafariTechnologyPreview", WebAppMaker{Browser: "Safari Technology Preview", Steps: safari.Steps}},
		{"com.google.Chrome", chrome},
		{"com.google.Chrome.beta", chrome},
		{"com.google.Chrome.canary", chrome},
		{"com.google.chrome.for.testing", generic("Chrome for Testing")},
		{"com.microsoft.edgemac", edge},
		{"com.microsoft.edgemac.Dev", edge},
		{"com.brave.Browser", generic("Brave")},
		{"com.brave.Browser.nightly", generic("Brave")},
		{"com.vivaldi.Vivaldi", generic("Vivaldi")},
		{"org.chromium.Chromium", generic("Chromium")},
		{"com.kagi.kagimacOS", WebAppMaker{Browser: "Orion", Steps: "Choose Tools ▸ Install This Site as an App."}},
		{" com.kagi.kagimacOS ", WebAppMaker{Browser: "Orion", Steps: "Choose Tools ▸ Install This Site as an App."}},
		// These can't make web apps on macOS, or aren't known to.
		{"org.mozilla.firefox", WebAppMaker{}},
		{"org.mozilla.firefoxdeveloperedition", WebAppMaker{}},
		{"company.thebrowser.Browser", WebAppMaker{}}, // Arc
		{"com.operasoftware.Opera", WebAppMaker{}},
		{"com.duckduckgo.macos.browser", WebAppMaker{}},
		{"app.zen-browser.zen", WebAppMaker{}},
		{"com.sindresorhus.Velja", WebAppMaker{}}, // A browser picker.
		{"net.imput.helium", WebAppMaker{}},
		{"com.google.chromecast", WebAppMaker{}},
		{"com.apple.Safarix", WebAppMaker{}},
		{"", WebAppMaker{}},
	}
	for _, tt := range tests {
		got, ok := WebAppMakerFor(tt.id)
		if got != tt.want || ok != (tt.want.Browser != "") {
			t.Errorf("WebAppMakerFor(%q) = %+v, %v; want %+v", tt.id, got, ok, tt.want)
		}
	}
}

func TestCantMakeWebApps(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"org.mozilla.firefox", true},
		{"org.mozilla.firefoxdeveloperedition", true},
		{"org.mozilla.nightly", true},
		{"company.thebrowser.Browser", true}, // Arc
		{"com.operasoftware.Opera", true},
		{"com.operasoftware.OperaGX", true},
		{"com.duckduckgo.macos.browser", true},
		{" app.zen-browser.zen ", true},
		// Browsers that make web apps, and ones that Bench doesn't know.
		{"com.apple.Safari", false},
		{"com.google.Chrome", false},
		{"com.kagi.kagimacOS", false},
		{"net.imput.helium", false},           // Built on Chromium.
		{"com.sindresorhus.Velja", false},     // A browser picker.
		{"org.mozilla.firefoxfork", false},    // Not Firefox.
		{"com.operasoftware.Operator", false}, // Not Opera.
		{"", false},
	}
	for _, tt := range tests {
		if got := CantMakeWebApps(tt.id); got != tt.want {
			t.Errorf("CantMakeWebApps(%q) = %v, want %v", tt.id, got, tt.want)
		}
		if _, ok := WebAppMakerFor(tt.id); tt.want && ok {
			t.Errorf("WebAppMakerFor(%q) says it can make web apps", tt.id)
		}
	}
}

func TestBrowserName(t *testing.T) {
	tests := []struct {
		id, want string
		// chromium is set for a browser built on Chromium, whose web apps
		// name their browser by its bundle ID.
		chromium bool
	}{
		{"com.apple.Safari", "Safari", false},
		{"com.kagi.kagimacOS", "Orion", false},
		{"com.google.Chrome.dev", "Chrome", true},
		{"com.microsoft.edgemac", "Edge", true},
		// A browser that Bench doesn't know is named from its bundle ID, as
		// web apps that a Chromium browser Bench doesn't know name it.
		{"net.imput.helium", "Helium", true},
		{"org.mozilla.firefox", "Firefox", false},
		{"", "", false},
	}
	for _, tt := range tests {
		if got := BrowserName(tt.id); got != tt.want {
			t.Errorf("BrowserName(%q) = %q, want %q", tt.id, got, tt.want)
		}
		if got := webAppBrowser(infoPlist{Browser: plistString(tt.id)}); tt.chromium && got != tt.want {
			t.Errorf("a web app that %s made names it %q, want %q", tt.id, got, tt.want)
		}
	}
}
