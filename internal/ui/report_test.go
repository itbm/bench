//go:build darwin || server

package ui

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestProblemReportArranging checks the times that a report gives for the
// latest tab switches, and for the other arrangements (decision 67): one
// line for each reason, with the median, the worst, how many there were,
// and the most windows changed, and no more reasons than reportReasons.
func TestProblemReportArranging(t *testing.T) {
	r := workspace.ProblemReport{
		Mode: "auto", SizeClass: "wide", Density: "standard",
		Switching: []workspace.ArrangeTiming{
			{Reason: "tab", Count: 30, Median: 42*time.Millisecond + 400*time.Microsecond, Worst: 180 * time.Millisecond, Windows: 7},
			{Reason: "tab-shortcut", Count: 2, Median: 61 * time.Millisecond, Worst: 80 * time.Millisecond, Windows: 5},
		},
		Arranging: []workspace.ArrangeTiming{
			{Reason: "bench-moved", Count: 49, Median: 3250 * time.Microsecond, Worst: 9960 * time.Microsecond, Windows: 0},
			{Reason: "rearrange", Count: 1, Median: 12 * time.Millisecond, Worst: 12 * time.Millisecond, Windows: 1},
		},
	}
	got := problemReport(r, "1.2.0", "macOS 15.1", "")
	for _, want := range []string{
		"Time to switch tabs, the last 32 times, from the click, key, or change to the last window placed:\n\n" +
			"- tab: median 42 ms, worst 180 ms (30 times, up to 7 windows changed)\n" +
			"- tab-shortcut: median 61 ms, worst 80 ms (2 times, up to 5 windows changed)\n" +
			"\nTime to arrange windows for other reasons, the last 50 times, from the event or change to the last window placed:\n\n" +
			"- bench-moved: median 3.3 ms, worst 10 ms (49 times, no windows changed)\n" +
			"- rearrange: median 12 ms, worst 12 ms (1 time, up to 1 window changed)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report has no %q:\n%s", want, got)
		}
	}

	// Without times, there's no such part.
	r.Switching = nil
	if got := problemReport(r, "", "", ""); strings.Contains(got, "Time to switch") || !strings.Contains(got, "Time to arrange") {
		t.Errorf("a report without switches' times has a part for them:\n%s", got)
	}
	r.Arranging = nil
	if got := problemReport(r, "", "", ""); strings.Contains(got, "Time to") {
		t.Errorf("a report without times has a part for them:\n%s", got)
	}

	// Many reasons are cut short; with the most notices too, the address
	// stays short enough.
	for i := range 20 {
		r.Arranging = append(r.Arranging, workspace.ArrangeTiming{
			Reason: strings.Repeat("r", 20) + string(rune('a'+i)), Count: 2, Median: time.Second, Worst: 9 * time.Second, Windows: 99,
		})
	}
	for range 12 {
		r.Notices = append(r.Notices, strings.Repeat("n", 280))
	}
	got = problemReport(r, "", "", "~/Library/Logs/Bench/bench.log")
	if !strings.Contains(got, "the last 40 times") || !strings.Contains(got, "- And 8 more reasons.\n") ||
		strings.Contains(got, strings.Repeat("r", 20)+string(rune('a'+reportReasons))) {
		t.Errorf("with 20 reasons:\n%s", got)
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
