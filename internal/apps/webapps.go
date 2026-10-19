package apps

import (
	"os"
	"path/filepath"
	"strings"
)

// Web apps are websites that a browser has made into apps of their own, with
// their own icon in the Dock, their own windows, and the browser's own
// protections. Each has a bundle ID of its own and runs in a process of its
// own, which owns its windows, so Bench docks it like any other app.
//
// Safari (macOS 14 and later) keeps them in ~/Applications itself. Chrome,
// Edge, Brave, Vivaldi, and other browsers built on Chromium keep them in a
// folder there named after the browser, such as "Chrome Apps.localized".
// Orion keeps them in ~/Applications/Orion/WebApps. Firefox, Opera, and Arc
// can't make web apps on macOS.

// webAppDirs returns the folders in home's Applications folder that browsers
// keep web apps in: each folder whose name ends in ".localized", which is
// how Chromium browsers name theirs, and Orion's.
func webAppDirs(home string) []string {
	apps := filepath.Join(home, "Applications")
	// ReadDir returns what it read before any error, and a missing folder
	// simply has no entries.
	entries, _ := os.ReadDir(apps)
	var dirs []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(strings.ToLower(name), ".localized") {
			continue
		}
		path := filepath.Join(apps, name)
		// Stat, not the entry's type, so that a link to a folder counts.
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			dirs = append(dirs, path)
		}
	}
	return append(dirs, filepath.Join(apps, "Orion", "WebApps"))
}

// chromiumBrowsers names the browsers built on Chromium by their bundle IDs,
// in lower case. A web app's CrBundleIdentifier is its browser's ID, which
// for another channel adds a suffix, such as com.google.Chrome.beta, so a
// longer ID that begins with one of these is that browser too. Longer IDs
// come first, so that Chrome for Testing isn't taken for Chrome.
var chromiumBrowsers = []struct{ id, name string }{
	{"com.google.chrome.for.testing", "Chrome for Testing"},
	{"com.google.chrome", "Chrome"},
	{"com.microsoft.edgemac", "Edge"},
	{"com.brave.browser", "Brave"},
	{"com.vivaldi.vivaldi", "Vivaldi"},
	{"org.chromium.chromium", "Chromium"},
}

// webAppPrefixes are the bundle ID prefixes, in lower case, of the web apps
// that browsers other than Chromium's make.
var webAppPrefixes = []struct{ prefix, name string }{
	{"com.apple.safari.webapp.", "Safari"},
	{"com.kagi.kagimacos.webapp.", "Orion"},
}

// webAppBrowser returns the name of the browser that made a bundle from a
// website, or "" if the bundle isn't a web app.
func webAppBrowser(info infoPlist) string {
	if browser := strings.TrimSpace(string(info.Browser)); browser != "" {
		lower := strings.ToLower(browser)
		for _, b := range chromiumBrowsers {
			if lower == b.id || strings.HasPrefix(lower, b.id+".") {
				return b.name
			}
		}
		return NameFromBundleID(browser)
	}
	id := strings.ToLower(strings.TrimSpace(string(info.BundleID)))
	for _, w := range webAppPrefixes {
		if strings.HasPrefix(id, w.prefix) && len(id) > len(w.prefix) {
			return w.name
		}
	}
	return ""
}
