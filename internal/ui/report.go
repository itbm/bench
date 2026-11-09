//go:build darwin || server

package ui

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/itbm/bench/internal/workspace"
)

// newIssueURL is where Help ▸ Report a Problem… opens a new issue.
const newIssueURL = "https://github.com/itbm/bench/issues/new"

// Limits on the notices and the reasons for arranging that a report
// includes (for each of its parts with times), so that its address stays
// short enough for the browser and GitHub.
const (
	reportNotices     = 8
	reportNoticeChars = 300
	reportReasons     = 12
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
// versions, the displays, the layout in use, the permissions, the notices,
// how long the latest tab switches and other arrangements took (decision
// 67), and where the log is, which they can attach.
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
	arrangeTimes(&b, "Time to switch tabs, the last %s, from the click, key, or change to the last window placed", p.Switching)
	arrangeTimes(&b, "Time to arrange windows for other reasons, the last %s, from the event or change to the last window placed", p.Arranging)
	if logFile != "" {
		fmt.Fprintf(&b, "\nBench keeps a log of its warnings and errors in `%s`. "+
			"You can attach it: read it first, as it names your tabs and apps.\n", logFile)
	}
	return b.String()
}

// arrangeTimes writes a part of the report with the times of the latest
// arrangements, by reason, if there are any, under heading, which has a %s
// for how many: one line for each reason, up to reportReasons.
func arrangeTimes(b *strings.Builder, heading string, times []workspace.ArrangeTiming) {
	if len(times) == 0 {
		return
	}
	n := 0
	for _, t := range times {
		n += t.Count
	}
	fmt.Fprintf(b, "\n"+heading+":\n\n", count(n, "time", "times"))
	for i, t := range times {
		if i == reportReasons {
			fmt.Fprintf(b, "- And %d more reasons.\n", len(times)-i)
			break
		}
		changed := "no windows changed"
		if t.Windows > 0 {
			changed = "up to " + count(t.Windows, "window", "windows") + " changed"
		}
		fmt.Fprintf(b, "- %s: median %s, worst %s (%s, %s)\n",
			t.Reason, millis(t.Median), millis(t.Worst), count(t.Count, "time", "times"), changed)
	}
}

// millis writes a duration in milliseconds: whole ones from 10 ms, and to
// a tenth below that.
func millis(d time.Duration) string {
	ms := math.Round(float64(d)/float64(time.Millisecond)*10) / 10
	if ms >= 10 {
		return fmt.Sprintf("%.0f ms", ms)
	}
	return fmt.Sprintf("%.1f ms", ms)
}

// count writes n with the word for one or for more.
func count(n int, one, more string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, more)
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
