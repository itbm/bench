package workspace

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/itbm/bench/internal/platform"
)

// This file gives items, apps, and launchers the addresses of their icons,
// which the tiles of sub-tabs and lists draw in place of the monogram. Bench
// serves them itself (ui's icon handler): an app's from its bundle
// (Platform.AppIcon), a Web item's from its page (Platform.PageIcon). The
// view holds only the address, which stays the same while the icon does, so
// that the frontend loads each one once, however often the view is sent.
// The tile shows the monogram if an icon doesn't load.

// The paths that icons are served at: AppIconPath, then an app's bundle ID,
// escaped, then ".png"; PageIconPath, then a Web item's ID, then ".png" and
// "?v=" and a number that changes with the icon.
const (
	AppIconPath  = "/icons/app/"
	PageIconPath = "/icons/page/"
)

// iconsShownLocked reports whether the view gives icons' addresses: not in
// the browser preview, whose platform has no icons, so that each tile
// doesn't try to load one in vain, nor for `bench arrange`.
func (e *Engine) iconsShownLocked() bool {
	return !e.opts.Demo && !e.opts.Headless
}

// appIconLocked returns the address of an app's icon, or "" for none: an app
// that macOS says isn't installed has none.
func (e *Engine) appIconLocked(bundleID string) string {
	if bundleID == "" || !e.iconsShownLocked() || e.notFound[strings.ToLower(bundleID)] {
		return ""
	}
	return AppIconPath + url.PathEscape(bundleID) + ".png"
}

// itemIconLocked returns the address of an item's icon: its app's, or its
// page's once the page has one. A terminal has none, nor has a Web item
// before its page's icon is fetched.
func (e *Engine) itemIconLocked(it *Item) string {
	switch {
	case it.Kind == KindApp:
		return e.appIconLocked(it.BundleID)
	case it.Kind == KindWeb && it.pageIcon != 0 && e.iconsShownLocked():
		return PageIconPath + url.PathEscape(it.ID) + ".png?v=" + strconv.Itoa(it.pageIcon)
	}
	return ""
}

// onPageIconChanged notes that a Web item's page has a new icon, or none
// now (PageIconChanged). Each new icon gets a new number, so that its
// address changes and the frontend loads it.
func (e *Engine) onPageIconChanged(it *Item, ev platform.Event) {
	if !ev.HasIcon {
		if it.pageIcon != 0 {
			it.pageIcon = 0
			e.touch()
		}
		return
	}
	e.pageIcons++
	it.pageIcon = e.pageIcons
	e.touch()
}
