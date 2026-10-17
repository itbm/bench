// Package apps finds installed apps and names things for display: apps by
// bundle ID, monograms for sub-tabs, and Web items by address.
//
// Every function is safe for concurrent use: the package holds no state
// apart from read-only tables.
package apps

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"howett.net/plist"

	"github.com/itbm/bench/internal/platform"
)

// ErrNoBundleID is returned, wrapped, by ReadBundle for a bundle whose
// Info.plist has no CFBundleIdentifier.
var ErrNoBundleID = errors.New("no bundle ID in Info.plist")

// Dirs returns the folders to scan: /Applications, ~/Applications, and
// /System/Applications, each followed by its Utilities folder. With an empty
// home, the ~/Applications folders are left out.
func Dirs(home string) []string {
	dirs := []string{"/Applications", "/Applications/Utilities"}
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, "Applications"),
			filepath.Join(home, "Applications", "Utilities"))
	}
	return append(dirs, "/System/Applications", "/System/Applications/Utilities")
}

// Scan returns the apps (*.app bundles) directly inside dirs, without
// duplicates by bundle ID (the first found wins, comparing IDs without
// regard to case, as macOS does), sorted by name, ignoring case. Bundles
// without a bundle ID, or whose Info.plist can't be read, are skipped, as
// are hidden ones (whose names start with "."), and folders that don't
// exist. Symbolic links to bundles are followed.
func Scan(dirs []string) []platform.AppInfo {
	found := []platform.AppInfo{}
	seen := make(map[string]bool)
	for _, dir := range dirs {
		for _, path := range bundlesIn(dir) {
			info, err := ReadBundle(path)
			if err != nil {
				continue
			}
			id := strings.ToLower(info.BundleID)
			if seen[id] {
				continue
			}
			seen[id] = true
			found = append(found, info)
		}
	}
	slices.SortStableFunc(found, compareApps)
	return found
}

// compareApps orders apps by name ignoring case, then by name, then by path,
// so the order doesn't depend on the order of dirs.
func compareApps(a, b platform.AppInfo) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(a.Path, b.Path),
	)
}

// bundlesIn returns the paths of the app bundles directly inside dir, in
// file name order. If dir can't be read to the end, it returns the bundles
// read before the problem.
func bundlesIn(dir string) []string {
	// ReadDir returns what it read before any error, so the error itself
	// doesn't matter: a missing or unreadable folder simply has no entries.
	entries, _ := os.ReadDir(dir)
	var paths []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !hasAppSuffix(name) {
			continue
		}
		path := filepath.Join(dir, name)
		// Stat, not the entry's type, so that links to bundles count.
		if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}

// hasAppSuffix reports whether name ends with ".app", ignoring case.
func hasAppSuffix(name string) bool {
	const suffix = ".app"
	return len(name) > len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix)
}

// infoPlists are where a bundle's Info.plist may be, relative to the bundle:
// a Mac app's, then an iPhone or iPad app's, which macOS wraps in a bundle
// of its own.
var infoPlists = []string{
	filepath.Join("Contents", "Info.plist"),
	filepath.Join("WrappedBundle", "Info.plist"),
}

// ReadBundle reads an app bundle's Contents/Info.plist (XML or binary) for its
// bundle ID and name (CFBundleDisplayName, then CFBundleName, then the bundle's
// file name without .app). A name that ends in ".app" loses it, and a name
// that's an unexpanded build setting, such as "$(PRODUCT_NAME)", is skipped.
// For an iPhone or iPad app installed on a Mac, it reads
// WrappedBundle/Info.plist instead.
func ReadBundle(path string) (platform.AppInfo, error) {
	data, plistPath, err := readInfoPlist(path)
	if err != nil {
		return platform.AppInfo{}, err
	}
	var info map[string]any
	if _, err := plist.Unmarshal(data, &info); err != nil {
		return platform.AppInfo{}, fmt.Errorf("reading %s: %w", plistPath, err)
	}
	id := strings.TrimSpace(stringValue(info, "CFBundleIdentifier"))
	if id == "" {
		return platform.AppInfo{}, fmt.Errorf("reading %s: %w", plistPath, ErrNoBundleID)
	}
	return platform.AppInfo{BundleID: id, Name: bundleName(info, path), Path: path}, nil
}

// readInfoPlist returns the contents and path of a bundle's Info.plist.
func readInfoPlist(bundle string) ([]byte, string, error) {
	var firstErr error
	for _, rel := range infoPlists {
		path := filepath.Join(bundle, rel)
		data, err := os.ReadFile(path)
		if err == nil {
			return data, path, nil
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("reading %s: %w", path, err)
		}
	}
	return nil, "", firstErr
}

// stringValue returns a string value from a property list dictionary, or ""
// if it's missing or not a string.
func stringValue(info map[string]any, key string) string {
	s, _ := info[key].(string)
	return s
}

// bundleName chooses a bundle's display name.
func bundleName(info map[string]any, path string) string {
	for _, key := range []string{"CFBundleDisplayName", "CFBundleName"} {
		name := DisplayName(stringValue(info, key))
		if name != "" && !strings.Contains(name, "$(") && !strings.Contains(name, "${") {
			return name
		}
	}
	return DisplayName(filepath.Base(path))
}

// DisplayName tidies an app's name for display: it trims spaces and removes
// a final ".app", ignoring case ("Zed.app" is "Zed").
func DisplayName(name string) string {
	name = strings.TrimSpace(name)
	if hasAppSuffix(name) {
		name = strings.TrimSpace(name[:len(name)-len(".app")])
	}
	return name
}
