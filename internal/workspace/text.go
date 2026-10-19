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
	splitsHidden = "Splits are hidden on small screens"
	// accessibilityNotice is the status bar's notice while Bench doesn't
	// have Accessibility permission.
	accessibilityNotice = "Bench needs Accessibility permission to arrange other apps' windows."
)

// User-facing errors.
var (
	errSplitsHidden  = errors.New(splitsHidden + ".")
	errOnlyItem      = errors.New("This pane has only one window, so there's nothing to show beside it. Add another window first, or drag one here.")
	errNothingBeside = errors.New("There's nothing to move it next to.")
	errEmptyPane     = errors.New("There's no window in this pane to move.")
	errGone          = errors.New("That window has closed.")
	errNoPane        = errors.New("There's no such pane.")
	errNoSplit       = errors.New("There's no such split.")
	errNotInPane     = errors.New("That window isn't in that pane.")
	errNotInTab      = errors.New("That window isn't in that tab.")
	errOnlyTab       = errors.New("That window is only in this tab. To take it out of Bench, pop it out.")
	errPoppedOut     = errors.New("That window is out of Bench. Add it to a pane with +.")
	errBadZone       = errors.New("That isn't a place to put a window.")
	errBadDirection  = errors.New("That isn't a direction: use left, right, up, or down.")
	errBadSizes      = errors.New("Those sizes don't fit that split.")
	errBadMode       = errors.New("That isn't a layout: use auto, wide, or compact.")
	errNotHTTPS      = errors.New("That address isn't a secure web address (https).")
	errNoAddress     = errors.New("Enter a web address, such as https://example.com.")
	errLastTab       = errors.New("You can't delete the last tab.")
	errNoName        = errors.New("Give the tab a name.")
	errNoTerminal    = errors.New("There's no such terminal.")
	errTermEnded     = errors.New("That terminal has ended.")
	errNotUndocked   = errors.New("That window is already docked.")
	errNoLoginItem   = errors.New("Open at login isn't available here.")
	errNoDemo        = errors.New("Simulated screens are only for the browser preview.")
	errBadSource     = errors.New("Bench can't add that.")
	errNotRunning    = errors.New("Bench isn't running any more.")
	errNotWebLink    = errors.New("Bench only opens http and https links.")
	errNotWebPage    = errors.New("Bench only opens http and https pages in your browser.")
	errNotPage       = errors.New("Only web pages open in a browser.")
	errNoBrowser     = errors.New("This build of Bench can't open links.")
)

// errNoTab says there's no tab at index i (counting from 0).
func errNoTab(i int) error { return fmt.Errorf("There's no tab %d.", i+1) }

// errNoTabNamed says there's no tab with a name.
func errNoTabNamed(name string) error { return fmt.Errorf("There's no tab called %s.", name) }

// errNoPreset says a layout preset doesn't exist.
func errNoPreset(id string) error { return fmt.Errorf("There's no layout called %s.", id) }

// errTabExists says a tab name is taken.
func errTabExists(name string) error { return fmt.Errorf("There's already a tab called %s.", name) }

// errAlreadyIn says a window is in a tab already.
func errAlreadyIn(name string) error { return fmt.Errorf("That window is already in %s.", name) }

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

// sizeReason is the status bar's size class and why, as in the prototype:
// "Wide layout, chosen for a 2,560-pt screen", or "Compact layout (stack
// panes), set by hand".
func sizeReason(class layout.SizeClass, mode layout.Mode, setting layout.SmallScreens, screenWidth float64) string {
	s := "Wide layout"
	if class == layout.Compact {
		s = "Compact layout (" + smallScreenText(setting) + ")"
	}
	if mode == layout.ModeAuto || mode == "" {
		return s + ", chosen for a " + groupThousands(screenWidth) + "-pt screen"
	}
	return s + ", set by hand"
}

// labelOf names an item the way the prototype does: "Zed — bench".
func labelOf(it *Item) string {
	if it.Title == "" || it.Title == it.App {
		return it.App
	}
	return it.App + " — " + it.Title
}

// Status texts for items that aren't ready.
func openingText(app string) string    { return "Opening " + app + "…" }
func notRunningText(app string) string { return app + " isn't running" }
func noWindowText(app string) string   { return app + " has no window open for this" }
func couldNotOpenText(app string) string {
	return "Bench couldn't open " + app
}

const (
	undockedText    = "Moved out of its pane"
	fullScreenText  = "In full screen"
	unavailableText = "On another desktop"
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
