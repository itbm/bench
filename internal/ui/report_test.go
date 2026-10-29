//go:build darwin || server

package ui

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/workspace"
)

// TestProblemReport checks the text that Help ▸ Report a Problem… fills a
// new GitHub issue with: room for the user's own words, then what Bench
// knows, for them to check.
func TestProblemReport(t *testing.T) {
	r := workspace.ProblemReport{
		Screens: []workspace.ReportScreen{
			{Name: "Built-in Retina Display", W: 1512, H: 982, Primary: true},
			{Name: "DELL U2720Q", W: 2560, H: 1440, Bench: true},
		},
		Mode: "auto", SizeClass: "wide", Density: "standard", Accessibility: true,
		Notices: []string{"config.toml has problems:\n  line 3: bad", strings.Repeat("x", 400)},
	}
	got := problemReport(r, "1.2.0 (abc123)", "macOS 15.1 (arm64)", "~/Library/Logs/Bench/bench.log")
	for _, want := range []string{
		"### What happened?",
		"- Bench: 1.2.0 (abc123)\n",
		"- System: macOS 15.1 (arm64)\n",
		"- Displays: Built-in Retina Display, 1512 × 982 (main); DELL U2720Q, 2560 × 1440 (Bench is on it)\n",
		"- Layout: auto, shown wide, standard density\n",
		"- Accessibility: allowed; Screen Recording: not allowed\n",
		"- config.toml has problems: line 3: bad\n",
		"`~/Library/Logs/Bench/bench.log`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report has no %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, strings.Repeat("x", reportNoticeChars)) {
		t.Errorf("a long notice wasn't cut short:\n%s", got)
	}

	// Many notices are cut short, so that the address stays short enough.
	r.Notices = nil
	for range 12 {
		r.Notices = append(r.Notices, strings.Repeat("n", 280))
	}
	got = problemReport(r, "", "", "")
	if !strings.Contains(got, "- And 4 more.") || !strings.Contains(got, "- Bench: unknown") || strings.Contains(got, "bench.log") {
		t.Errorf("with 12 notices, no version, and no log:\n%s", got)
	}
	address := newIssueURL + "?" + url.Values{"body": {got}}.Encode()
	if len(address) > 8000 {
		t.Errorf("the report's address is %d characters long, which browsers may cut short", len(address))
	}
}

// TestHomeRelative checks that a report writes a path in the user's home
// folder from ~, so that it doesn't name their account.
func TestHomeRelative(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder")
	}
	if got := homeRelative(filepath.Join(home, "Library", "Logs", "Bench", "bench.log")); got != "~/Library/Logs/Bench/bench.log" {
		t.Errorf("homeRelative = %q, want ~/Library/Logs/Bench/bench.log", got)
	}
	if got := homeRelative("/var/log/bench.log"); got != "/var/log/bench.log" {
		t.Errorf("homeRelative of a path outside the home folder = %q", got)
	}
	if got := homeRelative(""); got != "" {
		t.Errorf("homeRelative(\"\") = %q", got)
	}
}
