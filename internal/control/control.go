// Package control reads what links and scripts ask the running Bench to do,
// and serves bench list.
//
// There are two ways in, which trust their callers differently
// (docs/architecture.md, decision 77):
//
//   - bench:// links, which any web page, app, or Shortcuts can open. They
//     can only switch tabs and show Jump to… or every tab, so they work
//     without asking (ParseLink).
//   - The bench command's script subcommands, such as bench tab Code, which
//     a Bench started from Terminal forwards to the running one. They can
//     also move the shown window and start launchers, so they work only
//     while the allow_scripts setting is on (ParseCommand). bench list asks
//     over a Unix socket in Bench's support folder that only the user can
//     open (Serve and Query), while the setting is on.
//
// The package has no state of its own: the engine (internal/workspace)
// carries out each Request.
package control

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Scheme is the URL scheme of Bench's links, which Bench.app's Info.plist
// registers with macOS (CFBundleURLTypes).
const Scheme = "bench"

// What a Request asks Bench to do.
const (
	ActionTab       = "tab"         // Switch to the tab that Arg names.
	ActionNext      = "next"        // Switch to the next tab.
	ActionPrevious  = "previous"    // Switch to the previous tab.
	ActionLast      = "last"        // Switch back to the tab Bench was on before.
	ActionJump      = "jump"        // Show Jump to…: links only.
	ActionOverview  = "overview"    // Show every tab (Show All Tabs): links only.
	ActionMoveToTab = "move-to-tab" // Move the shown window to the tab that Arg names: scripts only.
	ActionLaunch    = "launch"      // Start the launcher that Arg names in the focused pane: scripts only.
)

// Request is what a link or a script asks of Bench.
type Request struct {
	// Action is one of the Action constants.
	Action string
	// Arg is a tab's name or number, for ActionTab and ActionMoveToTab, or
	// a launcher's name, for ActionLaunch. It's "" for the others.
	Arg string
}

// linkActions are the actions that a link can ask for, and whether each
// takes an argument. A link can only switch tabs, as a click on a tab
// does, and show Jump to… or every tab, since any web page can open one.
var linkActions = map[string]bool{
	ActionTab:      true,
	ActionNext:     false,
	ActionPrevious: false,
	ActionLast:     false,
	ActionJump:     false,
	ActionOverview: false,
}

// commandActions are the bench command's subcommands that the running Bench
// carries out, and whether each takes an argument.
var commandActions = map[string]bool{
	ActionTab:       true,
	ActionNext:      false,
	ActionPrevious:  false,
	ActionLast:      false,
	ActionMoveToTab: true,
	ActionLaunch:    true,
}

// CommandNames returns the bench command's subcommands that the running
// Bench carries out, in the order that bench help lists them.
func CommandNames() []string {
	return []string{ActionTab, ActionNext, ActionPrevious, ActionLast, ActionMoveToTab, ActionLaunch}
}

// IsLink reports whether s is meant as one of Bench's links: it starts with
// "bench:", in any case. ParseLink says whether Bench can open it.
func IsLink(s string) bool {
	scheme, _, ok := strings.Cut(s, ":")
	return ok && strings.EqualFold(scheme, Scheme)
}

// ParseLink reads a bench:// link: bench://tab/<name or number>, with the
// name percent-encoded as in any URL (bench://tab/Mail%20and%20chat),
// bench://next, bench://previous, bench://last, bench://jump, or
// bench://overview. The action's case doesn't matter, and a slash at the
// end is allowed. Anything else is refused, with an error that says what
// links can do and then names the link, as two sentences for a toast: the
// app's toasts are one line, cut at the end, so what to do comes first.
func ParseLink(link string) (Request, error) {
	refuse := func(why string) (Request, error) {
		return Request{}, fmt.Errorf("%s Bench can't open %s.", why, cut(link, linkLimit))
	}
	const can = "Links can only switch tabs or show Jump to… or every tab."
	const needs = "A tab link needs a name or number, as in bench://tab/Code."
	// A tab's name with #, ?, or % in it must be encoded, or the link
	// means something else: bench://tab/C#, unencoded, would be the tab C.
	const encode = "In a link, write # as %23, ? as %3F, and % as %25."
	tabLink := strings.EqualFold(linkAction(link), ActionTab)
	u, err := url.Parse(link)
	if tabLink && (strings.ContainsAny(link, "#?") || (err != nil && strings.Contains(link, "%"))) {
		return refuse(encode)
	}
	// An empty fragment, as in bench://jump#, leaves no Fragment.
	if err != nil || u.Scheme != Scheme || strings.Contains(link, "#") {
		return refuse(can)
	}
	if u.Opaque != "" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return refuse(can)
	}
	action := strings.ToLower(u.Hostname())
	parts := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	needsArg, ok := linkActions[action]
	switch {
	case !ok:
		return refuse(can)
	case needsArg && len(parts) != 1:
		return refuse(needs)
	case !needsArg && len(parts) != 0:
		return refuse("Nothing can come after bench://" + action + ".")
	}
	req := Request{Action: action}
	if needsArg {
		arg, err := url.PathUnescape(parts[0])
		if err != nil || arg == "" || !utf8.ValidString(arg) {
			return refuse(needs)
		}
		req.Arg = arg
	}
	return req, nil
}

// linkAction returns what a link's action seems to be, before ParseLink
// reads it: what comes after "bench://", up to a slash, "?", or "#".
func linkAction(link string) string {
	scheme, rest, ok := strings.Cut(link, "://")
	if !ok || !strings.EqualFold(scheme, Scheme) {
		return ""
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// IsCommand reports whether args, the arguments after the program's name,
// are one of the script subcommands that the running Bench carries out
// (CommandNames). ParseCommand says whether they're right.
func IsCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	_, ok := commandActions[args[0]]
	return ok
}

// ParseCommand reads a script subcommand's arguments, after the program's
// name: such as ["tab", "Code"] or ["next"]. A "--" after the subcommand
// is dropped, so that ["tab", "--", "-x"] names the tab "-x"; otherwise an
// argument that starts with "-" is refused, as the subcommands have no
// flags. The bench command sends the subcommands to the running Bench in
// this form (CommandArgs), and the running Bench reads them with this.
func ParseCommand(args []string) (Request, error) {
	if !IsCommand(args) {
		if len(args) == 0 {
			return Request{}, errors.New("there's no command")
		}
		return Request{}, fmt.Errorf("there's no command called %s", Shown(args[0]))
	}
	action, rest := args[0], args[1:]
	dashes := len(rest) > 0 && rest[0] == "--"
	if dashes {
		rest = rest[1:]
	}
	if !dashes {
		if i := slices.IndexFunc(rest, func(a string) bool { return strings.HasPrefix(a, "-") && a != "-" }); i >= 0 {
			return Request{}, fmt.Errorf("bench %s has no flag %s", action, Shown(rest[i]))
		}
	}
	if !commandActions[action] {
		if len(rest) > 0 {
			return Request{}, fmt.Errorf("bench %s takes nothing after it", action)
		}
		return Request{Action: action}, nil
	}
	what := "a tab's name or number"
	if action == ActionLaunch {
		what = "a launcher's name"
	}
	switch {
	case len(rest) == 0 || rest[0] == "":
		return Request{}, fmt.Errorf("bench %s needs %s", action, what)
	case len(rest) > 1:
		return Request{}, fmt.Errorf("bench %s takes one argument, %s: put a name with spaces in quotes", action, what)
	case !utf8.ValidString(rest[0]):
		return Request{}, fmt.Errorf("bench %s needs %s in UTF-8", action, what)
	}
	return Request{Action: action, Arg: rest[0]}, nil
}

// CommandArgs returns the arguments that send r to the running Bench, which
// ParseCommand reads back as r: the subcommand, then "--" and the argument
// if it has one, so that an argument starting with "-" stays one.
func CommandArgs(r Request) []string {
	if !commandActions[r.Action] {
		return []string{r.Action}
	}
	return []string{r.Action, "--", r.Arg}
}

// shownLimit is the most characters of a link or argument that a message
// shows, and linkLimit the most of a link that a refused link's toast
// shows, so that a long link doesn't take the whole line.
const (
	shownLimit = 80
	linkLimit  = 40
)

// Shown returns s as a message can show it: with control characters,
// including line breaks, as "?", and cut to shownLimit characters with "…".
// A link comes from anywhere, so a message mustn't show all of it as it is.
func Shown(s string) string { return cut(s, shownLimit) }

// cut is Shown, with at most limit characters.
func cut(s string, limit int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == limit {
			b.WriteString("…")
			break
		}
		if r == utf8.RuneError || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			r = '?'
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// FindTab returns the index of the tab that arg names, among names: the tab
// with that name, matched exactly, then ignoring case, as Bench matches
// tab names everywhere; or else, if arg is a number from 1 to the number of
// tabs, the tab with that number. So a tab named "2" comes before the
// second tab. It returns -1 if there's no such tab.
func FindTab(names []string, arg string) int {
	if i := slices.Index(names, arg); i >= 0 {
		return i
	}
	for i, n := range names {
		if strings.EqualFold(n, arg) {
			return i
		}
	}
	if n, err := strconv.Atoi(arg); err == nil && n >= 1 && n <= len(names) && strconv.Itoa(n) == arg {
		return n - 1
	}
	return -1
}
