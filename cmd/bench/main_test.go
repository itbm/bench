package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
	"github.com/itbm/bench/internal/windows"
	"github.com/itbm/bench/internal/workspace"
)

// runBench runs bench with args and returns its exit status and output.
func runBench(t *testing.T, args ...string) (status int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status = run(args, &out, &errOut)
	return status, out.String(), errOut.String()
}

// writeConfig writes a config file called name in a new folder.
func writeConfig(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const badConfig = `
[settings]
new_windows = "somewhere"

[shortcuts]
jump = "hyper+j"

[[tab]]
name = "Comms"
layout = "sideways"

[[tab]]
name = "Code"
layout = "single"

  [[tab.pane]]
  items = [{ web = "http://example.com" }]
`

func TestCheckConfig(t *testing.T) {
	good := writeConfig(t, "config.toml", string(config.DefaultTOML()))
	bad := writeConfig(t, "config.toml", badConfig)
	syntax := writeConfig(t, "config.toml", "[settings]\ngap = \n")
	missing := filepath.Join(t.TempDir(), "config.toml")
	tests := []struct {
		name       string
		args       []string
		status     int
		stdout     []string // Every line, in order.
		stderrHas  string
		stdoutHas  string
		stdoutNone bool
	}{
		{name: "fine", args: []string{"check-config", "--config", good}, status: 0,
			stdout: []string{good + " is fine."}},
		{name: "problems", args: []string{"check-config", "--config", bad}, status: 1,
			stdout: []string{
				`settings.new_windows: must be app-pane, focused-pane, or float, not "somewhere"`,
				`tab "Comms": layout: "sideways" isn't a preset; use single, side, main-side, three, top-bottom, or grid`,
				`tab "Code": pane[1].items[1].web: "http://example.com" isn't an https address`,
				`shortcuts.jump: "hyper" isn't a modifier; use ctrl, alt, shift, or cmd`,
			},
			stderrHas: "has 4 problems."},
		{name: "syntax error", args: []string{"check-config", "--config", syntax}, status: 1,
			stdoutHas: "settings.gap", stderrHas: "has 1 problem."},
		{name: "missing", args: []string{"check-config", "--config", missing}, status: 1,
			stderrHas: "there's no config file at " + missing, stdoutNone: true},
		{name: "argument", args: []string{"check-config", "extra"}, status: 2,
			stderrHas: `unexpected argument "extra"`, stdoutNone: true},
		{name: "unknown flag", args: []string{"check-config", "--nope"}, status: 2,
			stderrHas: "flag provided but not defined: -nope", stdoutNone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, stdout, stderr := runBench(t, tt.args...)
			if status != tt.status {
				t.Errorf("status = %d, want %d\nstdout: %s\nstderr: %s", status, tt.status, stdout, stderr)
			}
			if tt.stdout != nil {
				if got := lines(stdout); !slices.Equal(got, tt.stdout) {
					t.Errorf("stdout:\n%s\nwant lines like:\n%s", stdout, strings.Join(tt.stdout, "\n"))
				}
			}
			if tt.stdoutHas != "" && !strings.Contains(stdout, tt.stdoutHas) {
				t.Errorf("stdout = %q, want it to mention %q", stdout, tt.stdoutHas)
			}
			if tt.stdoutNone && stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if tt.stderrHas != "" && !strings.Contains(stderr, tt.stderrHas) {
				t.Errorf("stderr = %q, want it to mention %q", stderr, tt.stderrHas)
			}
		})
	}
}

func TestCheckConfigUsesTheSupportFolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BENCH_HOME", dir)
	path := filepath.Join(dir, config.ConfigFile)
	if err := os.WriteFile(path, config.DefaultTOML(), 0o644); err != nil {
		t.Fatal(err)
	}
	status, stdout, _ := runBench(t, "check-config")
	if status != 0 || strings.TrimSpace(stdout) != path+" is fine." {
		t.Errorf("check-config = %d %q, want 0 and %q", status, stdout, path+" is fine.")
	}
	if _, err := os.Stat(filepath.Join(dir, config.StateFile)); err == nil {
		t.Error("check-config wrote state.json")
	}
}

func TestCheckConfigWritesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BENCH_HOME", dir)
	if status, _, _ := runBench(t, "check-config"); status != 1 {
		t.Errorf("check-config with no config.toml = %d, want 1", status)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("check-config wrote %v", entries)
	}
}

func TestLayout(t *testing.T) {
	path := writeConfig(t, "config.toml", string(config.DefaultTOML()))
	monitor := []string{"layout", "--width", "2560", "--height", "1415", "--config", path}
	laptop := []string{"layout", "--width", "1512", "--height", "949", "--y", "33", "--config", path}
	tests := []struct {
		name   string
		args   []string
		class  layout.SizeClass
		frames map[string][]layout.Rect
		panes  map[string][][]string // Each displayed pane's items.
	}{
		{
			name:  "monitor",
			args:  monitor,
			class: layout.Wide,
			frames: map[string][]layout.Rect{
				"Comms":    {{X: 8, Y: 33, W: 1140, H: 1399}, {X: 1156, Y: 33, W: 1396, H: 1399}},
				"Code":     {{X: 8, Y: 33, W: 1651, H: 1399}, {X: 1667, Y: 33, W: 885, H: 1399}},
				"Research": {{X: 8, Y: 33, W: 1523, H: 1399}, {X: 1539, Y: 33, W: 1013, H: 1399}},
			},
			panes: map[string][][]string{
				"Comms": {{"w1"}, {"w2"}},
				"Code":  {{"w3", "w4"}, {"w5"}},
			},
		},
		{
			name:  "laptop",
			args:  laptop,
			class: layout.Compact,
			frames: map[string][]layout.Rect{
				"Comms":    {{X: 8, Y: 41, W: 1496, H: 933}},
				"Code":     {{X: 8, Y: 41, W: 1496, H: 604}, {X: 8, Y: 653, W: 1496, H: 321}},
				"Research": {{X: 8, Y: 41, W: 1496, H: 933}},
			},
			panes: map[string][][]string{
				"Comms":    {{"w1", "w2"}},
				"Research": {{"w6", "w7"}},
			},
		},
		{
			name:  "forced compact",
			args:  append(slices.Clone(monitor), "--compact"),
			class: layout.Compact,
			frames: map[string][]layout.Rect{
				"Code": {{X: 8, Y: 33, W: 2544, H: 907}, {X: 8, Y: 948, W: 2544, H: 484}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := layoutOutput(t, tt.args...)
			if report.SizeClass != tt.class {
				t.Errorf("size class = %q, want %q", report.SizeClass, tt.class)
			}
			if report.Gap != 8 || report.Config != path {
				t.Errorf("gap %v, config %q; want 8 and %q", report.Gap, report.Config, path)
			}
			tabs := map[string]tabLayout{}
			for _, tab := range report.Tabs {
				tabs[tab.Name] = tab
				if tab.SizeClass != tt.class {
					t.Errorf("%s: size class = %q, want %q", tab.Name, tab.SizeClass, tt.class)
				}
			}
			for name, want := range tt.frames {
				var got []layout.Rect
				for _, f := range tabs[name].Frames {
					got = append(got, f.Rect)
				}
				if !slices.Equal(got, want) {
					t.Errorf("%s frames = %v, want %v", name, got, want)
				}
			}
			for name, want := range tt.panes {
				var got [][]string
				for _, p := range layout.Leaves(tabs[name].Root) {
					got = append(got, p.Items)
				}
				if !slices.EqualFunc(got, want, slices.Equal) {
					t.Errorf("%s panes = %v, want %v", name, got, want)
				}
			}
		})
	}
}

func TestLayoutDescribesItemsAndIDs(t *testing.T) {
	path := writeConfig(t, "config.toml", string(config.DefaultTOML()))
	report := layoutOutput(t, "layout", "--width", "2560", "--height", "1415", "--config", path)
	code := report.Tabs[1]
	if code.Name != "Code" || code.SmallScreens != "stack" {
		t.Fatalf("second tab = %q (%q), want Code (stack)", code.Name, code.SmallScreens)
	}
	want := map[string]string{"w3": "dev.zed.Zed", "w4": "com.microsoft.VSCode", "w5": "Terminal: ~/code"}
	for id, what := range want {
		if code.Items[id] != what {
			t.Errorf("Code item %s = %q, want %q", id, code.Items[id], what)
		}
	}
	if got := report.Tabs[0].Items["w1"]; got != "Slack — https://app.slack.com/client" {
		t.Errorf("Comms item w1 = %q, want Slack and its address", got)
	}
	// Items are numbered first (w1 to w7), then each tab's panes and split.
	if code.Root.ID != "s13" || code.Frames[0].ID != "p11" || code.Frames[1].ID != "p12" {
		t.Errorf("Code tree %s with panes %v; want split s13 and panes p11 and p12", code.Root.ID, code.Frames)
	}
}

// TestLayoutUsesTheAppsIDs checks that bench layout names splits, panes,
// and items as the app does when it starts from the same config.
func TestLayoutUsesTheAppsIDs(t *testing.T) {
	dir := t.TempDir()
	cfgText := string(config.DefaultTOML()) + `
[[tab]]
name = "Docs"
layout = "grid"

  [[tab.pane]]
  items = [{ app = "md.obsidian", title = "Notes" }]
`
	path := filepath.Join(dir, config.ConfigFile)
	if err := os.WriteFile(path, []byte(cfgText), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := workspace.New(workspace.Options{
		Platform: platform.NewFake(clock.NewFake(time.Time{})),
		NewTerms: func(cb term.Callbacks) term.Host { return term.NewEcho(cb) },
		Dir:      dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	view := e.View()

	report := layoutOutput(t, "layout", "--width", "2560", "--height", "1415", "--config", path)
	if len(report.Tabs) != len(view.Tabs) {
		t.Fatalf("layout has %d tabs, the app %d", len(report.Tabs), len(view.Tabs))
	}
	for i, tab := range report.Tabs {
		got, _ := json.Marshal(tab.Root)
		want, _ := json.Marshal(view.Tabs[i].Root)
		if !bytes.Equal(got, want) {
			t.Errorf("%s tree:\n got %s\nwant %s (the app's)", tab.Name, got, want)
		}
	}
}

func TestLayoutWithoutAConfig(t *testing.T) {
	t.Setenv("BENCH_HOME", t.TempDir())
	status, stdout, stderr := runBench(t, "layout", "--width", "2560", "--height", "1415")
	if status != 0 || !strings.Contains(stderr, "default config") {
		t.Fatalf("layout = %d, stderr %q; want 0 and a note about the default config", status, stderr)
	}
	var report layoutReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || len(report.Tabs) != 3 {
		t.Errorf("layout printed %d tabs (%v), want the default config's 3", len(report.Tabs), err)
	}
}

func TestLayoutWithAMissingConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "typo.toml")
	status, stdout, stderr := runBench(t, "layout", "--width", "2560", "--height", "1415", "--config", missing)
	if status != 1 || stdout != "" || !strings.Contains(stderr, "there's no config file at "+missing) {
		t.Errorf("layout --config %s = %d %q %q; want 1, no output, and that the file is missing", missing, status, stdout, stderr)
	}
}

func TestLayoutRefusesABadConfig(t *testing.T) {
	path := writeConfig(t, "config.toml", badConfig)
	status, stdout, stderr := runBench(t, "layout", "--width", "2560", "--height", "1415", "--config", path)
	if status != 1 || stdout != "" || !strings.Contains(stderr, "run bench check-config") {
		t.Errorf("layout = %d %q %q; want 1, no output, and a pointer to check-config", status, stdout, stderr)
	}
}

// shortcutProblem is the default config with a [shortcuts] problem, which
// the app reports in its status bar and runs with.
func shortcutProblem(t *testing.T) string {
	t.Helper()
	text := strings.Replace(string(config.DefaultTOML()), `jump = "ctrl+alt+space"`, `jump = "ctrl+alt+spacey"`, 1)
	if !strings.Contains(text, "spacey") {
		t.Fatal("the default config has no jump shortcut to change")
	}
	return writeConfig(t, "config.toml", text)
}

func TestLayoutWarnsAboutShortcuts(t *testing.T) {
	path := shortcutProblem(t)
	status, stdout, stderr := runBench(t, "layout", "--width", "2560", "--height", "1415", "--config", path)
	if status != 0 {
		t.Fatalf("layout = %d %q; want 0, as the app runs with this config", status, stderr)
	}
	if !strings.Contains(stderr, "shortcuts.jump") || !strings.Contains(stderr, "which bench layout doesn't use") {
		t.Errorf("stderr = %q, want the shortcut's problem as a warning", stderr)
	}
	var report layoutReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil || len(report.Tabs) != 3 {
		t.Errorf("layout printed %d tabs (%v), want the config's 3", len(report.Tabs), err)
	}
	// check-config still reports it.
	if status, stdout, _ := runBench(t, "check-config", "--config", path); status != 1 || !strings.Contains(stdout, "shortcuts.jump") {
		t.Errorf("check-config = %d %q, want 1 and the shortcut's problem", status, stdout)
	}
}

func layoutOutput(t *testing.T, args ...string) layoutReport {
	t.Helper()
	status, stdout, stderr := runBench(t, args...)
	if status != 0 {
		t.Fatalf("%v = %d: %s", args, status, stderr)
	}
	var report layoutReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("output isn't JSON: %v\n%s", err, stdout)
	}
	return report
}

func TestArgumentErrors(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		status    int
		stdoutHas string
		stderrHas string
	}{
		{"unknown command", []string{"frobnicate"}, 2, "", `there's no command "frobnicate"`},
		{"layout without a size", []string{"layout"}, 2, "", "give a positive --width and --height"},
		{"layout with a bad width", []string{"layout", "--width", "wide", "--height", "9"}, 2, "", "invalid value"},
		{"layout with an argument", []string{"layout", "--width", "9", "--height", "9", "Code"}, 2, "", `unexpected argument "Code"`},
		{"arrange without a tab", []string{"arrange"}, 2, "", "name one tab"},
		{"arrange with two tabs", []string{"arrange", "Code", "Comms"}, 2, "", "name one tab"},
		{"watchdog without a folder", []string{"watchdog"}, 2, "", "give the support folder with --dir"},
		{"restore with an argument", []string{"restore", "now"}, 2, "", `unexpected argument "now"`},
		{"version with an argument", []string{"version", "now"}, 2, "", `unexpected argument "now"`},
		{"help for two commands", []string{"help", "layout", "arrange"}, 2, "", "name one command"},
		{"help for nothing", []string{"help", "nope"}, 2, "", `there's no command "nope"`},
		{"help", []string{"help"}, 0, "bench layout --width w --height h", ""},
		{"help for a command", []string{"help", "layout"}, 0, "-width", ""},
		{"help for the app", []string{"help", "bench"}, 0, "Usage: bench [flags]", ""},
		{"-h", []string{"check-config", "-h"}, 0, "Usage: bench check-config [--config path]", ""},
		{"--help", []string{"arrange", "--help"}, 0, "Usage: bench arrange <tab> [--config path]", ""},
		{"version", []string{"version"}, 0, "bench ", ""},
		{"top-level -h", []string{"-h"}, 0, "bench check-config [--config path]", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, stdout, stderr := runBench(t, tt.args...)
			if status != tt.status {
				t.Errorf("status = %d, want %d\nstdout: %s\nstderr: %s", status, tt.status, stdout, stderr)
			}
			if tt.stdoutHas != "" && !strings.Contains(stdout, tt.stdoutHas) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tt.stdoutHas)
			}
			if tt.stderrHas != "" && !strings.Contains(stderr, tt.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.stderrHas)
			}
			if tt.status == exitUsage && tt.name != "unknown command" && !strings.Contains(stderr, "Usage") &&
				!strings.HasPrefix(tt.name, "help") {
				t.Errorf("stderr = %q, want the command's usage after a usage error", stderr)
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args   []string
		rest   []string
		config string
	}{
		{[]string{"Code", "--config", "x.toml"}, []string{"Code"}, "x.toml"},
		{[]string{"--config", "x.toml", "Code"}, []string{"Code"}, "x.toml"},
		{[]string{"Code", "Comms"}, []string{"Code", "Comms"}, ""},
		{[]string{"--", "--config"}, []string{"--config"}, ""},
		{[]string{"Code", "--", "-x"}, []string{"Code", "-x"}, ""},
		{nil, nil, ""},
	}
	for _, tt := range tests {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		cfg := fs.String("config", "", "")
		rest, err := parseArgs(fs, tt.args)
		if err != nil || !slices.Equal(rest, tt.rest) || *cfg != tt.config {
			t.Errorf("parseArgs(%q) = %q, config %q, %v; want %q, config %q", tt.args, rest, *cfg, err, tt.rest, tt.config)
		}
	}
}

func TestAppArgs(t *testing.T) {
	got := appArgs([]string{"-psn_0_12345", "--port", "9000"})
	if want := []string{"--port", "9000"}; !slices.Equal(got, want) {
		t.Errorf("appArgs = %q, want %q", got, want)
	}
}

func TestPrepareArrange(t *testing.T) {
	c := &cli{stdout: io.Discard, stderr: io.Discard}
	named := writeConfig(t, "config.toml", string(config.DefaultTOML()))
	other := writeConfig(t, "test.toml", string(config.DefaultTOML()))

	setup, err := prepareArrange(c, "code", named)
	if err != nil || setup.Dir != filepath.Dir(named) || setup.Temporary {
		t.Errorf("prepareArrange(config.toml) = %+v, %v; want its own folder", setup, err)
	}

	setup, err = prepareArrange(c, "Code", other)
	if err != nil || !setup.Temporary {
		t.Fatalf("prepareArrange(test.toml) = %+v, %v; want a temporary copy", setup, err)
	}
	defer os.RemoveAll(setup.Dir)
	copied, err := os.ReadFile(filepath.Join(setup.Dir, config.ConfigFile))
	if err != nil || !bytes.Equal(copied, config.DefaultTOML()) {
		t.Errorf("the temporary config.toml = %v; want a copy of test.toml", err)
	}

	if _, err := prepareArrange(c, "Mail", named); err == nil || !strings.Contains(err.Error(), "Comms, Code, Research") {
		t.Errorf("prepareArrange(Mail) = %v, want an error listing the tabs", err)
	}
	bad := writeConfig(t, "config.toml", badConfig)
	if _, err := prepareArrange(c, "Code", bad); err == nil || !strings.Contains(err.Error(), "check-config") {
		t.Errorf("prepareArrange(bad config) = %v, want an error pointing to check-config", err)
	}

	var stderr bytes.Buffer
	warned := &cli{stdout: io.Discard, stderr: &stderr}
	shortcuts := shortcutProblem(t)
	if setup, err := prepareArrange(warned, "Code", shortcuts); err != nil || setup.Dir != filepath.Dir(shortcuts) {
		t.Errorf("prepareArrange(a [shortcuts] problem) = %+v, %v; want to go ahead, as the app does", setup, err)
	}
	if !strings.Contains(stderr.String(), "shortcuts.jump") {
		t.Errorf("stderr = %q, want the shortcut's problem as a warning", stderr.String())
	}
}

func TestRestoreParked(t *testing.T) {
	parked := windows.ParkedFile{
		Windows: []windows.ParkedWindow{{ID: 12, BundleID: "com.brave.Browser", Title: "Go packages"}},
		Hidden:  []windows.HiddenApp{{PID: 40, BundleID: "md.obsidian"}},
	}
	tests := []struct {
		name    string
		file    *windows.ParkedFile // Nil means no file.
		putErr  error
		wantPut bool
		wantErr bool
	}{
		{name: "no file"},
		{name: "empty file", file: &windows.ParkedFile{}},
		{name: "parked windows", file: &parked, wantPut: true},
		{name: "failure", file: &parked, putErr: errors.New("window not found"), wantPut: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, config.ParkedFile)
			if tt.file != nil {
				if err := os.WriteFile(path, mustJSON(t, *tt.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var put []string
			err := waitAndRestore(strings.NewReader("anything"), dir, func(p string) error {
				put = append(put, p)
				return tt.putErr
			})
			if (len(put) > 0) != tt.wantPut || (tt.wantPut && put[0] != path) {
				t.Errorf("put back %q, want it called (%v) with %s", put, tt.wantPut, path)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, want an error: %v", err, tt.wantErr)
			}
		})
	}
}

// TestWatchdogStopsVSCode checks that the watchdog deals with the VS Code
// for the Web server's record, which a Bench that crashed leaves (the
// vscode package's tests check that stopping from a record works).
func TestWatchdogStopsVSCode(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, config.VSCodeFile)
	if err := os.WriteFile(record, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := waitAndRestore(strings.NewReader(""), dir, func(string) error { return nil })
	if err == nil {
		t.Error("waitAndRestore() = nil with a damaged vscode.json, want an error")
	}
	if _, err := os.Stat(record); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("vscode.json is still there after the watchdog: %v", err)
	}
}

// TestVSCodeOptions checks that the app keeps the VS Code for the Web
// server's token in the support folder, not in the temporary folder, which
// macOS clears of files that haven't been used for a few days.
func TestVSCodeOptions(t *testing.T) {
	dir := t.TempDir()
	opts := vscodeOptions(dir)
	if opts.TokenDir != dir {
		t.Errorf("TokenDir = %q, want the support folder, %q", opts.TokenDir, dir)
	}
	if want := filepath.Join(dir, config.VSCodeFile); opts.Record != want {
		t.Errorf("Record = %q, want %q", opts.Record, want)
	}
}

func TestWhileLooping(t *testing.T) {
	var looping atomic.Bool
	worked := false
	whileLooping(func(stop <-chan struct{}) {
		looping.Store(true)
		<-stop
		looping.Store(false)
	}, func() {
		deadline := time.Now().Add(5 * time.Second)
		for !looping.Load() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		worked = looping.Load()
	})
	if !worked {
		t.Error("the work didn't run while the loop was running")
	}

	// A loop that can't run (off the main thread) returns at once; the
	// work still finishes first.
	finished := false
	whileLooping(func(<-chan struct{}) {}, func() {
		time.Sleep(20 * time.Millisecond)
		finished = true
	})
	if !finished {
		t.Error("whileLooping returned before the work finished")
	}
}

func TestParkedSummary(t *testing.T) {
	tests := []struct {
		f    windows.ParkedFile
		want string
	}{
		{windows.ParkedFile{}, "nothing"},
		{windows.ParkedFile{Windows: make([]windows.ParkedWindow, 1)}, "1 window"},
		{windows.ParkedFile{Windows: make([]windows.ParkedWindow, 3), Hidden: make([]windows.HiddenApp, 1)}, "3 windows and 1 app"},
		{windows.ParkedFile{Hidden: make([]windows.HiddenApp, 2)}, "2 apps"},
	}
	for _, tt := range tests {
		if got := parkedSummary(tt.f); got != tt.want {
			t.Errorf("parkedSummary(%d windows, %d apps) = %q, want %q", len(tt.f.Windows), len(tt.f.Hidden), got, tt.want)
		}
	}
}

func TestPreviewDir(t *testing.T) {
	t.Setenv("BENCH_HOME", "")
	base, err := config.Dir()
	if err != nil {
		t.Skip(err)
	}
	if got, _ := previewDir(""); got != filepath.Join(base, previewFolder) {
		t.Errorf("previewDir() = %q, want the Preview folder in %s", got, base)
	}
	t.Setenv("BENCH_HOME", "/tmp/bench-home")
	if got, _ := previewDir(""); got != "/tmp/bench-home" {
		t.Errorf("previewDir() with BENCH_HOME = %q, want BENCH_HOME", got)
	}
	if got, _ := previewDir("/elsewhere"); got != "/elsewhere" {
		t.Errorf("previewDir(/elsewhere) = %q", got)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func lines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
