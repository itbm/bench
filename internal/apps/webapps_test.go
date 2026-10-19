package apps

import (
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
	want := []platform.AppInfo{
		{BundleID: "com.apple.Safari.WebApp.6C1F7E36-0B3A-4C5E-9D2B-1A2B3C4D5E6F", Name: "Calendar", Path: path("", "Calendar.app"), WebApp: "Safari"},
		{BundleID: "com.google.Chrome.beta.app.bbbb", Name: "Docs", Path: path("Chrome Apps.localized", "Docs.app"), WebApp: "Chrome"},
		{BundleID: "com.kagi.kagimacOS.WebApp.1234", Name: "Kagi", Path: path(filepath.Join("Orion", "WebApps"), "Kagi.app"), WebApp: "Orion"},
		{BundleID: "com.brave.Browser.app.eeee", Name: "Mastodon", Path: path("Brave Browser Apps.localized", "Mastodon.app"), WebApp: "Brave"},
		{BundleID: "net.imput.helium.app.gggg", Name: "Notes", Path: path("Helium Apps.localized", "Notes.app"), WebApp: "Helium"},
		{BundleID: "com.microsoft.edgemac.app.dddd", Name: "Outlook", Path: path("Edge Apps.localized", "Outlook.app"), WebApp: "Edge"},
		{BundleID: "com.google.Chrome.app.aaaa", Name: "Slack: Work", Path: path("Chrome Apps.localized", "Slack - Work.app"), WebApp: "Chrome"},
		{BundleID: "com.vivaldi.Vivaldi.app.ffff", Name: "Squoosh", Path: path("Vivaldi Apps.localized", "Squoosh.app"), WebApp: "Vivaldi"},
		{BundleID: "com.google.chrome.for.testing.app.cccc", Name: "Tester", Path: path("Chromium Apps.localized", "Tester.app"), WebApp: "Chrome for Testing"},
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
