// Package apps finds installed apps and names things for display.
package apps

import "github.com/itbm/bench/internal/platform"

// This file fixes the package's exported API so that other packages can build
// against it. Delete it once everything is implemented in its own file.

// Dirs returns the folders to scan: /Applications, ~/Applications, and
// /System/Applications, each followed by its Utilities folder.
func Dirs(home string) []string { panic("not implemented") }

// Scan returns the apps (*.app bundles) directly inside dirs, without
// duplicates by bundle ID (the first found wins), sorted by name, ignoring
// case. Bundles without a bundle ID are skipped.
func Scan(dirs []string) []platform.AppInfo { panic("not implemented") }

// ReadBundle reads an app bundle's Contents/Info.plist (XML or binary) for its
// bundle ID and name (CFBundleDisplayName, then CFBundleName, then the bundle's
// file name without .app).
func ReadBundle(path string) (platform.AppInfo, error) { panic("not implemented") }

// Mono returns the two-character monogram for a sub-tab: known apps have
// fixed ones ("Zed" is "Zd", "Finder" is "Fn", "Terminal" is ">_",
// "Web page" is "Wb"); names of more than one word use their initials ("VS
// Code" is "VS"); others use their first two letters ("Slack" is "Sl").
func Mono(name string) string { panic("not implemented") }

// NameFromBundleID guesses an app's name from its bundle ID when it isn't
// installed: the last part, with known IDs mapped to their names
// ("com.microsoft.VSCode" is "VS Code", "com.brave.Browser" is "Brave",
// "md.obsidian" is "Obsidian").
func NameFromBundleID(id string) string { panic("not implemented") }

// WebName names a Web item from its address: the site's main name,
// capitalised ("https://app.slack.com/client" is "Slack",
// "https://outlook.office.com/mail/" is "Outlook").
func WebName(address string) string { panic("not implemented") }
