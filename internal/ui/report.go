//go:build darwin || server

package ui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/itbm/bench/internal/workspace"
)

// newIssueURL is where Help ▸ Report a Problem… opens a new issue.
const newIssueURL = "https://github.com/itbm/bench/issues/new"

// Limits on the notices that a report includes, so that its address stays
// short enough for the browser and GitHub.
const (
	reportNotices     = 8
	reportNoticeChars = 300
)

// openLink opens a page in the default browser, or says why it couldn't.
func (r *runner) openLink(address string) {
	if err := r.app.Browser.OpenURL(address); err != nil {
		r.emitter.Toast("Bench couldn't open " + address + " in your browser.")
	}
}

// reportProblem is Help ▸ Report a Problem…: it opens a new GitHub issue
// in the default browser, filled in with what may help (problemReport), for
// the user to check and send. Bench sends nothing itself.
func (r *runner) reportProblem() {
	info := r.app.Env.Info()
	system := info.OS
	if info.OSInfo != nil && info.OSInfo.Version != "" {
		system = info.OSInfo.Version
		if info.OS == "darwin" {
			system = "macOS " + system
		}
	}
	system += " (" + info.Arch + ")"
	body := problemReport(r.engine.ProblemReport(), r.opts.Version, system, homeRelative(r.opts.LogFile))
	r.openLink(newIssueURL + "?" + url.Values{"body": {body}}.Encode())
}

// problemReport returns the text of a new GitHub issue about a problem: a
// place for the user to say what happened, then, for them to check, the
// versions, the displays, the layout in use, the permissions, and the
// notices, and where the log is, which they can attach.
func problemReport(p workspace.ProblemReport, version, system, logFile string) string {
	var b strings.Builder
	b.WriteString("### What happened?\n\n\n\n### What did you expect to happen?\n\n\n\n")
	b.WriteString("### About this Mac\n\n")
	b.WriteString("<!-- Bench filled this in. Check it before you submit, and remove anything you'd rather not share. -->\n\n")
	fmt.Fprintf(&b, "- Bench: %s\n", orUnknown(version))
	fmt.Fprintf(&b, "- System: %s\n", orUnknown(system))
	var displays []string
	for _, s := range p.Screens {
		d := fmt.Sprintf("%g × %g", s.W, s.H)
		if s.Name != "" {
			d = s.Name + ", " + d
		}
		var notes []string
		if s.Primary {
			notes = append(notes, "main")
		}
		if s.Bench {
			notes = append(notes, "Bench is on it")
		}
		if len(notes) > 0 {
			d += " (" + strings.Join(notes, ", ") + ")"
		}
		displays = append(displays, d)
	}
	fmt.Fprintf(&b, "- Displays: %s\n", orUnknown(strings.Join(displays, "; ")))
	fmt.Fprintf(&b, "- Layout: %s, shown %s, %s density\n", p.Mode, p.SizeClass, p.Density)
	fmt.Fprintf(&b, "- Accessibility: %s; Screen Recording: %s\n", allowed(p.Accessibility), allowed(p.ScreenRecording))
	if len(p.Notices) > 0 {
		b.WriteString("\nNotices:\n\n")
		for i, n := range p.Notices {
			if i == reportNotices {
				fmt.Fprintf(&b, "- And %d more.\n", len(p.Notices)-i)
				break
			}
			fmt.Fprintf(&b, "- %s\n", shorten(strings.Join(strings.Fields(n), " "), reportNoticeChars))
		}
	}
	if logFile != "" {
		fmt.Fprintf(&b, "\nBench keeps a log of its warnings and errors in `%s`. "+
			"You can attach it: read it first, as it names your tabs and apps.\n", logFile)
	}
	return b.String()
}

// orUnknown returns s, or "unknown" if it's empty.
func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// allowed says whether a permission is allowed.
func allowed(on bool) string {
	if on {
		return "allowed"
	}
	return "not allowed"
}

// homeRelative writes a path in the user's home folder from ~, so that a
// report doesn't name the user's account.
func homeRelative(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}
