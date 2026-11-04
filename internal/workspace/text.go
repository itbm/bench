package workspace

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/itbm/bench/internal/layout"
)

// Messages for the user. Bound methods return errors with these as their
// whole text, because the frontend shows an error's text as a toast.
const (
	// splitsHidden is View.Display.SplitsDisabled in the "tabs" view.
	splitsHidden = "Splits are hidden in the compact layout"
	// accessibilityNotice is the status bar's notice while Bench doesn't
	// have Accessibility permission.
	accessibilityNotice = "Bench needs Accessibility permission to arrange other apps' windows."
	// accessibilityUpdatedNotice is the notice in its place while an
	// earlier version of Bench had the permission, which macOS has
	// stopped applying since the update (see permissions.go).
	accessibilityUpdatedNotice = "Bench was updated, so macOS needs you to allow it again: select Bench, click −, then turn it on again."
	// loginApprovalText says that macOS needs the user to allow Bench's
	// login item, as SetOpenAtLogin turns it on.
	loginApprovalText = "macOS needs you to allow Bench in System Settings ▸ General ▸ Login Items. Bench has opened it: turn on Bench there."
	// noAccessibilityForWebApps and noBrowserForWebApps say why AddWebsite
	// added a Web item rather than a web app (see also cantMakeWebApps and
	// unknownWebAppMaker).
	noAccessibilityForWebApps = "Bench shows this page itself, because it needs Accessibility permission to dock web apps."
	noBrowserForWebApps       = "Bench couldn't find a browser to make a web app, so it shows the page itself."
)

// cantMakeWebApps says that the default browser, called browser, can't make
// web apps, so AddWebsite added a Web item.
func cantMakeWebApps(browser string) string {
	return browser + " can't make web apps, so Bench shows the page itself."
}

// unknownWebAppMaker says that Bench doesn't know how the default browser,
// called browser, makes web apps, if it can, so AddWebsite added a Web item.
func unknownWebAppMaker(browser string) string {
	return "Bench doesn't know how " + browser + " makes web apps, so it shows the page itself."
}

// User-facing errors.
var (
	errSplitsHidden  = errors.New(splitsHidden + ". Choose Wide to show them.")
	errOnlyItem      = errors.New("This pane has only one window, so there's nothing to show beside it. Add another window first, or drag one here.")
	errNothingBeside = errors.New("There's nothing to move it next to.")
	errEmptyPane     = errors.New("There's no window in this pane to move.")
	errGone          = errors.New("That window has closed.")
	errWindowsGone   = errors.New("Those windows have closed.")
	errNoWindows     = errors.New("Tick at least one window.")
	errNoPane        = errors.New("There's no such pane.")
	errNoSplit       = errors.New("There's no such split.")
	errNotInPane     = errors.New("That window isn't in that pane.")
	errNotInTab      = errors.New("That window isn't in that tab.")
	errOnlyTab       = errors.New("That window is only in this tab. To take it out of Bench, pop it out.")
	errPoppedOut     = errors.New("That window is out of Bench. Add it to a pane with +.")
	errPoppedOutDrag = errors.New("That window is out of Bench. To add it back, drag it onto a pane and wait a moment, or use +.")
	errBadZone       = errors.New("That isn't a place to put a window.")
	errBadDirection  = errors.New("That isn't a direction: use left, right, up, or down.")
	errBadSizes      = errors.New("Those sizes don't fit that split.")
	errBadMode       = errors.New("That isn't a layout: use auto, wide, or compact.")
	errNotWebAddress = errors.New("That isn't a web address. Try example.com.")
	errInsecure      = errors.New("Bench shows http pages only from this Mac, such as localhost.")
	errNoAddress     = errors.New("Type a web address, such as example.com.")
	errLastTab       = errors.New("You can't delete the last tab.")
	errNoLastTab     = errors.New("There's no tab to go back to yet. Switch tabs first.")
	errNoName        = errors.New("Give the tab a name.")
	errNoTerminal    = errors.New("There's no such terminal.")
	errTermEnded     = errors.New("That terminal has ended.")
	errNotUndocked   = errors.New("That window is already docked.")
	errAppTitle      = errors.New("An app's window takes its title from the app. Only terminals and web pages can be renamed.")
	errNoLoginItem   = errors.New("Open at login isn't available here.")
	errNoDemo        = errors.New("Simulated screens are only for the browser preview.")
	errNoDemoWindows = errors.New("Simulated windows are only for the browser preview.")
	errNoDemoKeys    = errors.New("Simulated keys are only for the browser preview.")
	errNoMinimise    = errors.New("The focused pane doesn't show an app's window to minimise.")
	errBadSource     = errors.New("Bench can't add that.")
	errNotRunning    = errors.New("Bench isn't running any more.")
	errNotWebPage    = errors.New("Only a web page can become a web app.")
	errNeedsTrust    = errors.New("Bench needs Accessibility permission to dock web apps.")
	errNoBrowser     = errors.New("Bench couldn't find a browser to make a web app.")
	errNotWebLink    = errors.New("Bench only opens http and https links.")
	errNotHTTPPage   = errors.New("Bench only opens http and https pages in your browser.")
	errNotPage       = errors.New("Only web pages open in a browser.")
	errCantOpenLinks = errors.New("This build of Bench can't open links.")
	errCantOpenFiles = errors.New("This build of Bench can't open files.")
	// errNoLastTabToSend refuses SendToLastTab until Bench has switched
	// tabs, as errNoLastTab refuses LastTab.
	errNoLastTabToSend = errors.New("There's no last used tab to send the window to yet. Switch tabs first.")
	// errConfigProblems and errNoConfigProblems refuse OverwriteConfig and
	// UseConfigBackup.
	errConfigProblems   = errors.New("config.toml has problems, so Bench doesn't save over it. Fix them and reload it, or use the backup.")
	errNoConfigProblems = errors.New("config.toml has no problems, so Bench kept it.")
	errLastPane         = errors.New("This is the tab's only pane.")
	// errZoomTabsView and errZoomOnePane refuse ZoomPane.
	errZoomTabsView = errors.New("On this screen, the tab already shows its windows as one pane.")
	errZoomOnePane  = errors.New("This tab has only one pane, so it fills the tab already.")
	errPaneNotEmpty = errors.New("This pane has windows in it. Move them to another pane first.")
	// errNothingToRestore and errNoTabToRestore refuse RestoreArrangement.
	errNothingToRestore = errors.New("There's no previous arrangement to restore.")
	errNoTabToRestore   = errors.New("No tab has the name of a tab whose arrangement Bench couldn't keep. Name one so first.")
	// errNoDialog and errDialogSize refuse SetDialogSize.
	errNoDialog   = errors.New("There's no such dialog.")
	errDialogSize = errors.New("That isn't a size for a dialog.")
	// errCantOpenSettings refuses OpenLoginItems where Bench can't open
	// System Settings, as in the browser preview.
	errCantOpenSettings = errors.New("Bench couldn't open System Settings. Open it, then General ▸ Login Items.")
	// errCantReopen refuses QuitAndReopen where Bench can't open itself
	// again, as in the browser preview.
	errCantReopen = errors.New("This build of Bench can't quit and open itself again. Quit Bench, then open it again.")
	// errNoFolderPicker refuses ChooseFolder where there's no picker, as in
	// the browser preview.
	errNoFolderPicker = errors.New("This build of Bench can't show a folder picker. Type the folder instead, such as ~/code/bench.")
	errLauncherKind   = errors.New("A launcher opens a terminal or an app.")
	// errNoBrowse, errBrowsing, and errBrowseFailed refuse BrowseToAdd:
	// where there's no open panel, as in the browser preview, while it's
	// open already, and when it can't be shown.
	errNoBrowse     = errors.New("The browser preview can't look for apps or programs on this computer. In the Mac app, Browse… finds one on your Mac.")
	errBrowsing     = errors.New("The panel for choosing an app or program is open already. Choose one there, or click Cancel.")
	errBrowseFailed = errors.New("Bench couldn't show the panel for choosing an app or program.")
	// errPageOnly, errPageNotOpen, and errPageFailed refuse a web page's
	// page controls (see pages.go): for another kind of window, before its
	// page has opened, and when the page couldn't do it.
	errPageOnly    = errors.New("Only web pages have page controls.")
	errPageNotOpen = errors.New("The page hasn't opened yet.")
	errPageFailed  = errors.New("Bench couldn't do that to the page.")
	errNoBack      = errors.New("There's no page to go back to.")
	errNoForward   = errors.New("There's no page to go forward to.")
	errZoomedIn    = errors.New("The page is as large as Bench makes it.")
	errZoomedOut   = errors.New("The page is as small as Bench makes it.")
	// errFindOwnWindow refuses find in page for a page in a window of its
	// own, where Bench has no find bar.
	errFindOwnWindow = errors.New("Find in page works in Bench's panes. Dock the page first.")
	errFindFailed    = errors.New("Bench couldn't search the page.")
	// errVSCodeLink and errNoPageLink refuse Copy Page Link.
	errVSCodeLink = errors.New("VS Code for the Web's address only works in Bench, so there's no link to copy.")
	errNoPageLink = errors.New("This page has no web address to copy.")
	errBadDevice  = errors.New("That isn't the camera or the microphone.")
)

// errNotCapturing says that a page isn't using a device: "camera" or
// "microphone".
func errNotCapturing(device string) error {
	return fmt.Errorf("The page isn't using the %s.", device)
}

// errNoLauncher says there's no launcher with a name.
func errNoLauncher(name string) error {
	return fmt.Errorf("There's no launcher called %s. Settings ▸ Launchers lists them.", name)
}

// errLauncherTaken says a launcher, by its own name, has the name given
// already, ignoring case.
func errLauncherTaken(name string) error {
	return fmt.Errorf("There's a launcher called %s already. Give this one another name.", name)
}

// errNoWebApp says that no web app of a site, by its host, is installed.
func errNoWebApp(host string) error { return fmt.Errorf("Bench can't find a web app of %s.", host) }

// insecureAddress says that an http address on another host than this Mac
// can't be a web page, and offers its https address: host is the host, with
// its port, if any.
func insecureAddress(host string) error {
	return fmt.Errorf("%w Try https://%s.", errInsecure, host)
}

// errNoTab says there's no tab at index i (counting from 0).
func errNoTab(i int) error { return fmt.Errorf("There's no tab %d.", i+1) }

// errNoTabNamed says there's no tab with a name.
func errNoTabNamed(name string) error { return fmt.Errorf("There's no tab called %s.", name) }

// errNoPreset says a layout preset doesn't exist.
func errNoPreset(id string) error { return fmt.Errorf("There's no layout called %s.", id) }

// errAlreadyIn says a window is in a tab already.
func errAlreadyIn(name string) error { return fmt.Errorf("That window is already in %s.", name) }

// poppedOut returns err, but errPoppedOut, which the model returns, says
// how to add the window back as the pop-out toast does: by dragging it onto
// a pane, too, while drag_to_add is on.
func poppedOut(err error, dragToAdd bool) error {
	if dragToAdd && errors.Is(err, errPoppedOut) {
		return errPoppedOutDrag
	}
	return err
}

// joinNames joins names into a list: "Code", "Code and Notes", or "Code,
// Notes, and Research".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
}

// groupThousands formats a number of points the en-GB way, rounded to a
// whole number, with commas between groups of three digits: 2560 is
// "2,560".
func groupThousands(v float64) string {
	n := int64(math.Round(v))
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	digits := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return sign + b.String()
}

// ordinal returns a place among the tabs, counting from 1, as English
// writes it: 1st, 2nd, 3rd, 4th, 11th, 21st, and so on.
func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return strconv.Itoa(n) + suffix
}

// errTabAtEnd refuses to move a tab past the first or last place.
func errTabAtEnd(name string, first bool) error {
	if first {
		return fmt.Errorf("%s is the first tab already.", name)
	}
	return fmt.Errorf("%s is the last tab already.", name)
}

// smallScreenText describes a small-screen setting, as the status bar does.
func smallScreenText(s layout.SmallScreens) string {
	switch s {
	case layout.Tabs:
		return "one pane, every window as a sub-tab"
	case layout.Same:
		return "keep the layout"
	}
	return "stack panes"
}

// sizeReason is the status bar's size class and why: "Wide layout, chosen
// for a window 2,560 pt wide", the width of Bench's window, or "Compact
// layout (stack panes), set by hand".
func sizeReason(class layout.SizeClass, mode layout.Mode, setting layout.SmallScreens, windowWidth float64) string {
	s := "Wide layout"
	if class == layout.Compact {
		s = "Compact layout (" + smallScreenText(setting) + ")"
	}
	if mode == layout.ModeAuto || mode == "" {
		return s + ", chosen for a window " + groupThousands(windowWidth) + " pt wide"
	}
	return s + ", set by hand"
}

// layoutName is a layout mode's name, as the title bar's layout control
// has it.
func layoutName(mode layout.Mode) string {
	switch mode {
	case layout.ModeWide:
		return "Wide"
	case layout.ModeCompact:
		return "Compact"
	}
	return "Auto"
}

// sideBySideText is ShowSideBySide's toast, for an item called label, with
// the mode it changed from, was. Choosing that mode again, with the title
// bar's layout control, goes back, unless the item got a pane of its own
// (split): then Undo, which removes the pane too.
func sideBySideText(label string, was layout.Mode, split bool) string {
	if split {
		return "Changed to the wide layout, and gave " + label + " a pane of its own, to show it side by side. Undo removes the pane and goes back to " + layoutName(was) + "."
	}
	return "Changed to the wide layout, to show " + label + " side by side. To go back, choose " + layoutName(was) + " at the right of the title bar."
}

// backToLayoutText is the toast after Undo takes back ShowSideBySide,
// which changed from mode was.
func backToLayoutText(was layout.Mode) string {
	if was == layout.ModeCompact {
		return "Back to the compact layout."
	}
	return "Back to Auto, which chooses the layout by the window's width."
}

// labelOfLocked names an item the way the prototype does: "Zed — bench",
// with the title its sub-tab shows (shownTitleLocked).
func (e *Engine) labelOfLocked(it *Item) string {
	title := e.shownTitleLocked(it)
	if title == "" || title == it.App {
		return it.App
	}
	return it.App + " — " + title
}

// wholeLabelOf names an item as labelOfLocked does, but with its whole
// title, an unread count at its start too: for a popped-out page's window,
// which shows no pill, and whose title the page's own replaces as it
// changes.
func wholeLabelOf(it *Item) string {
	title := it.Label
	if title == "" {
		title = it.Title
	}
	if title == "" || title == it.App {
		return it.App
	}
	return it.App + " — " + title
}

// shownTitleLocked returns the title an item's sub-tab shows: the user's
// own name for it (Label), if they've given it one, or else its title,
// without an unread count at its start while the sub-tab shows the count
// (unreadTitleLocked).
func (e *Engine) shownTitleLocked(it *Item) string {
	if it.Label != "" {
		return it.Label
	}
	return e.unreadTitleLocked(it)
}

// Status texts for items that aren't ready.
func openingText(app string) string      { return "Opening " + app + "…" }
func notRunningText(app string) string   { return app + " isn't running" }
func notInstalledText(app string) string { return app + " isn't installed" }
func noWindowText(app string) string     { return app + " has no window open for this" }
func couldNotOpenText(app string) string {
	return "Bench couldn't open " + app
}

const (
	undockedText    = "Moved out of its pane"
	fullScreenText  = "In full screen"
	unavailableText = "On another desktop"
	minimisedText   = "Minimised"
	vscodeStarting  = "Starting VS Code for the Web…"
	vscodeMissing   = "VS Code for the Web isn't available in this build of Bench"
	vscodeStopped   = "VS Code for the Web stopped. Show this item again to restart it."
)

// needsRoomText explains a window that can't shrink to its pane, suggesting
// the compact layout unless it's in use.
func needsRoomText(app string, compact bool) string {
	s := app + "'s window can't shrink to fit this pane"
	if compact {
		return s
	}
	return s + ". Try the compact layout"
}

// containsFold reports whether s contains sub, ignoring case.
func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
