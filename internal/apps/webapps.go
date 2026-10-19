package apps

import (
	"net/url"
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
			if isBrowser(lower, b.id) {
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

// isBrowser reports whether a bundle ID, in lower case, is a browser's: the
// browser's own ID, also in lower case, or another channel of it, which
// adds a suffix after a dot.
func isBrowser(id, browser string) bool {
	return id == browser || strings.HasPrefix(id, browser+".")
}

// webAppURL returns the address that a web app made by browser opens, where
// that browser records it: a Chromium browser's in CrAppModeShortcutURL, and
// Safari's in its Manifest's start_url, or else in its WKPushBundleMetadata's
// manifestId. Orion's is unknown. It returns "" for an address that isn't an
// absolute http or https address with a host.
func webAppURL(info infoPlist, browser string) string {
	var found []plistString
	switch {
	case strings.TrimSpace(string(info.Browser)) != "":
		found = []plistString{info.ShortcutURL}
	case browser == "Safari":
		found = []plistString{info.Manifest.Value.StartURL, info.PushMetadata.Value.ManifestID}
	}
	for _, s := range found {
		address := strings.TrimSpace(string(s))
		u, err := url.Parse(address)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if scheme := strings.ToLower(u.Scheme); scheme == "http" || scheme == "https" {
			return address
		}
	}
	return ""
}

// WebAppMaker is a browser that can make web apps, as the default browser
// that Bench opens a site in so that the user can make a web app of it.
type WebAppMaker struct {
	// Browser is the browser's name, such as "Safari".
	Browser string
	// Steps say how to make a web app of the page the browser shows, in
	// sentences.
	Steps string
}

// Steps to make a web app.
const (
	safariSteps = "Choose File ▸ Add to Dock, then click Add."
	orionSteps  = "Choose Tools ▸ Install This Site as an App."
	// chromiumWindow follows every Chromium browser's steps, since a web
	// app that opens in a browser tab is part of the browser's window.
	chromiumWindow = "If it asks how to open, choose a window, not a tab."
)

// chromiumSteps are the steps of the Chromium browsers whose commands
// Bench knows, by name (see chromiumBrowsers). The others' are
// chromiumGenericSteps.
var chromiumSteps = map[string]string{
	"Chrome": "Open the ⋮ menu and choose Cast, save, and share ▸ Install page as app, or, for a site that's made to be installed, Install and the site's name.",
	"Edge":   "Open the … menu and choose More tools ▸ Apps ▸ Install this site as an app.",
}

// chromiumGenericSteps are the steps of a Chromium browser called name.
func chromiumGenericSteps(name string) string {
	return "Choose " + name + "'s command to install the page as an app, in its menu or its address bar."
}

// otherMakers are the browsers other than Chromium's that can make web
// apps, by bundle ID in lower case, as in chromiumBrowsers.
var otherMakers = []struct{ id, name, steps string }{
	{"com.apple.safaritechnologypreview", "Safari Technology Preview", safariSteps},
	{"com.apple.safari", "Safari", safariSteps},
	{"com.kagi.kagimacos", "Orion", orionSteps},
}

// WebAppMakerFor returns the browser with a bundle ID, such as the default
// browser's, and how to make a web app in it, or false if it can't make web
// apps: Safari, Orion, and the Chromium browsers in chromiumBrowsers can, in
// any channel. Any other app can't, as far as Bench knows: Firefox, Arc,
// Opera, DuckDuckGo, and Zen can't make web apps on macOS (see
// CantMakeWebApps), an app that asks which browser to open each link in
// makes none itself, and Bench doesn't know the steps of other browsers.
func WebAppMakerFor(bundleID string) (WebAppMaker, bool) {
	id := strings.ToLower(strings.TrimSpace(bundleID))
	if id == "" {
		return WebAppMaker{}, false
	}
	for _, b := range chromiumBrowsers {
		if isBrowser(id, b.id) {
			steps, ok := chromiumSteps[b.name]
			if !ok {
				steps = chromiumGenericSteps(b.name)
			}
			return WebAppMaker{Browser: b.name, Steps: steps + " " + chromiumWindow}, true
		}
	}
	for _, b := range otherMakers {
		if isBrowser(id, b.id) {
			return WebAppMaker{Browser: b.name, Steps: b.steps}, true
		}
	}
	return WebAppMaker{}, false
}

// nonMakers are the browsers that can't make web apps on macOS, by bundle ID
// in lower case, as in chromiumBrowsers. Firefox's and Opera's other
// channels have IDs of their own.
var nonMakers = []string{
	"org.mozilla.firefox",
	"org.mozilla.firefoxdeveloperedition",
	"org.mozilla.nightly",
	"company.thebrowser.browser", // Arc
	"com.operasoftware.opera",
	"com.operasoftware.operagx",
	"com.operasoftware.operadeveloper",
	"com.duckduckgo.macos.browser",
	"app.zen-browser.zen",
}

// CantMakeWebApps reports whether the browser with a bundle ID is one that
// Bench knows can't make web apps on macOS: Firefox, Arc, Opera, DuckDuckGo,
// or Zen. It's false for a browser that Bench doesn't know, which may make
// them, as other browsers built on Chromium do.
func CantMakeWebApps(bundleID string) bool {
	id := strings.ToLower(strings.TrimSpace(bundleID))
	for _, b := range nonMakers {
		if id != "" && isBrowser(id, b) {
			return true
		}
	}
	return false
}

// BrowserName names the browser with a bundle ID as the web apps it makes
// name it (see platform.AppInfo.WebApp): "Chrome" for com.google.Chrome.beta,
// "Safari" for com.apple.Safari, or else a name made from the bundle ID.
func BrowserName(bundleID string) string {
	if maker, ok := WebAppMakerFor(bundleID); ok {
		return maker.Browser
	}
	return NameFromBundleID(strings.TrimSpace(bundleID))
}
