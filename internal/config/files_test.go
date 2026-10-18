package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
)

func TestDir(t *testing.T) {
	t.Run("BENCH_HOME", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "bench-home")
		t.Setenv("BENCH_HOME", home)
		got, err := Dir()
		if err != nil || got != home {
			t.Errorf("Dir() = %q, %v; want %s", got, err, home)
		}
	})
	t.Run("BENCH_HOME that others can change", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("no Unix permissions on Windows")
		}
		home := t.TempDir()
		if err := os.Chmod(home, 0o777); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BENCH_HOME", home)
		if got, err := Dir(); err == nil {
			t.Errorf("Dir() = %q, want an error for a folder anyone can change", got)
		}
	})
	t.Run("user config folder", func(t *testing.T) {
		t.Setenv("BENCH_HOME", "")
		base, err := os.UserConfigDir()
		if err != nil {
			t.Skipf("no user config folder here: %v", err)
		}
		got, err := Dir()
		if want := filepath.Join(base, "Bench"); err != nil || got != want {
			t.Errorf("Dir() = %q, %v; want %q", got, err, want)
		}
	})
}

func TestLoadWritesTheDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "folder", ConfigFile)
	cfg, problems, err := Load(path)
	if err != nil || len(problems) > 0 {
		t.Fatalf("Load = %v, %v", problems, err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("Load returned %+v, want Default()", cfg)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, DefaultTOML()) {
		t.Errorf("Load wrote:\n%s\nwant DefaultTOML()", data)
	}
	checkNoTempFiles(t, filepath.Dir(path))
}

func TestLoadReadsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFile)
	writeFile(t, path, "[settings]\ngap = 12\n"+oneTab)
	cfg, problems, err := Load(path)
	if err != nil || len(problems) > 0 {
		t.Fatalf("Load = %v, %v", problems, err)
	}
	if cfg.Settings.Gap != 12 || len(cfg.Tabs) != 1 {
		t.Errorf("Load returned %+v", cfg)
	}
}

func TestLoadReportsProblemsWithoutChangingTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFile)
	const doc = "[[tab]]\nname = \"Code\"\nlayout = \"nope\"\n"
	writeFile(t, path, doc)
	cfg, problems, err := Load(path)
	if err != nil || cfg != nil {
		t.Fatalf("Load = %v, %v; want problems only", cfg, err)
	}
	checkWheres(t, problems, []where{{"Code", "layout"}})
	if got := readFile(t, path); got != doc {
		t.Errorf("Load changed the file to %q", got)
	}
}

func TestLoadError(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Load(dir); err == nil {
		t.Error("Load of a folder didn't fail")
	}
	blocked := filepath.Join(dir, "file")
	writeFile(t, blocked, "")
	if _, _, err := Load(filepath.Join(blocked, ConfigFile)); err == nil {
		t.Error("Load under a file didn't fail")
	}
}

// TestLoadNamesTabAndFieldForEveryProblem is the first half of milestone
// 1's done-when: an invalid config reports the tab and field for each
// problem.
func TestLoadNamesTabAndFieldForEveryProblem(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFile)
	writeFile(t, path, `[settings]
gap = -8
follow_focus = "follow"
colour = "red"

[[tab]]
name = "Comms"
layout = "side"
sizes = [45, 0]
small_screens = "wrap"

  [[tab.pane]]
  items = [{ web = "http://app.slack.com", profile = "slack", title = "Slack" }]

  [[tab.pane]]
  items = [{ app = "com.microsoft.Outlook", web = "https://outlook.office.com" }]

  [[tab.pane]]
  items = [{ terminal = "~", profil = "x" }]

[[tab]]
name = "comms"
layout = "columns"

[[rule]]
app = "com.brave.Browser"
tab = "Reading"
`)
	cfg, problems, err := Load(path)
	if err != nil || cfg != nil {
		t.Fatalf("Load = %v, %v; want problems only", cfg, err)
	}
	checkWheres(t, problems, []where{
		{"", "settings.colour"},
		{"Comms", "pane[3].items[1].profil"},
		{"", "settings.gap"},
		{"", "settings.follow_focus"},
		{"Comms", "small_screens"},
		{"Comms", "sizes[2]"},
		{"Comms", "pane[3]"},
		{"Comms", "pane[1].items[1].web"},
		{"Comms", "pane[1].items[1].title"},
		{"Comms", "pane[2].items[1]"},
		{"comms", "name"},
		{"comms", "layout"},
		{"", "rule[1].tab"},
	})
}

// TestDefaultsRoundTrip is milestone 1's "the defaults round-trip through
// save and load unchanged".
func TestDefaultsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	first, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, first); err != nil {
		t.Fatalf("Save: %v", err)
	}
	second, problems, err := Load(path)
	if err != nil || len(problems) > 0 {
		t.Fatalf("Load = %v, %v", problems, err)
	}
	if !reflect.DeepEqual(second, Default()) {
		t.Errorf("after save and load: %+v, want Default()", second)
	}
	if got := readFile(t, path); got != string(DefaultTOML()) {
		t.Errorf("saving the defaults wrote:\n%s\nwant DefaultTOML()", got)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	for name, c := range roundTripConfigs() {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ConfigFile)
			if err := Save(path, c); err != nil {
				t.Fatal(err)
			}
			got, problems, err := Load(path)
			if err != nil || len(problems) > 0 {
				t.Fatalf("Load = %v, %v", problems, err)
			}
			if !reflect.DeepEqual(got, c) {
				t.Errorf("Load after Save = %+v, want %+v", got, c)
			}
		})
	}
}

func TestSaveKeepsABackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	const before = "# My own layout.\n[[tab]]\nname = \"Mine\"\nlayout = \"single\"\n"
	writeFile(t, path, before)

	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, BackupFile)); got != before {
		t.Errorf("backup = %q, want the previous file %q", got, before)
	}
	if got := readFile(t, path); got != string(DefaultTOML()) {
		t.Errorf("config = %q, want the default", got)
	}

	c := Default()
	c.Settings.Gap = 4
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, BackupFile)); got != string(DefaultTOML()) {
		t.Errorf("second backup = %q, want the version before", got)
	}
	checkNoTempFiles(t, dir)
}

func TestSaveWithoutAPreviousFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", ConfigFile)
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Save made a backup of nothing: %v", err)
	}
	if got := readFile(t, path); got != string(DefaultTOML()) {
		t.Errorf("config = %q", got)
	}
}

func TestSaveRefusesProblems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	const before = "[[tab]]\nname = \"Mine\"\nlayout = \"single\"\n"
	writeFile(t, path, before)

	c := Default()
	c.Tabs[1].Layout = "columns"
	c.Rules[0].Tab = "Nowhere"
	err := Save(path, c)
	var problems Problems
	if !errors.As(err, &problems) {
		t.Fatalf("Save = %v, want a Problems error", err)
	}
	checkWheres(t, problems, []where{{"Code", "layout"}, {"", "rule[1]"}})
	if got := readFile(t, path); got != before {
		t.Errorf("Save changed the file to %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, BackupFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Save made a backup when it refused: %v", err)
	}
	if err := Save(path, nil); err == nil {
		t.Error("Save(nil) didn't fail")
	}
	checkNoTempFiles(t, dir)
}

func TestSaveFollowsALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "bench.toml")
	writeFile(t, real, "# Old.\n"+oneTab)
	link := filepath.Join(dir, ConfigFile)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := Save(link, Default()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v, %v", info, err)
	}
	if got := readFile(t, real); got != string(DefaultTOML()) {
		t.Errorf("the linked file = %q, want the default", got)
	}
	if got := readFile(t, link+".bak"); got != "# Old.\n"+oneTab {
		t.Errorf("backup = %q", got)
	}
}

func TestSaveKeepsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix permissions on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	writeFile(t, path, oneTab)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Default()); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, path + ".bak"} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != 0o640 {
			t.Errorf("%s: mode %v, %v; want 0640", filepath.Base(p), info.Mode().Perm(), err)
		}
	}
	// New files and folders are private.
	other := filepath.Join(dir, "new", "other.toml")
	if err := Save(other, Default()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(other); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("a new file has mode %v, %v; want 0600", info.Mode().Perm(), err)
	}
	if info, err := os.Stat(filepath.Dir(other)); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("a new folder has mode %v, %v; want 0700", info.Mode().Perm(), err)
	}
}

func TestLoadLeavesADanglingLinkAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "Dropbox", "bench.toml")
	link := filepath.Join(dir, ConfigFile)
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	cfg, problems, err := Load(link)
	if err == nil || cfg != nil || len(problems) > 0 {
		t.Errorf("Load = %v, %v, %v; want an error", cfg, problems, err)
	}
	if err := Save(link, Default()); err == nil {
		t.Error("Save through a dangling link didn't fail")
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v, %v", info, err)
	}
	if _, err := os.Stat(real); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the linked file was created: %v", err)
	}
	checkNoTempFiles(t, dir)
}

func TestSaveFailsCleanly(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "file")
	writeFile(t, blocked, "")
	if err := Save(filepath.Join(blocked, ConfigFile), Default()); err == nil {
		t.Error("Save under a file didn't fail")
	}
	// A folder where the file should be can't be replaced.
	path := filepath.Join(dir, ConfigFile)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, Default()); err == nil {
		t.Error("Save over a folder didn't fail")
	}
	checkNoTempFiles(t, dir)
}

// TestSaveConcurrently checks that saves from several goroutines take turns:
// the file and its backup are each one whole version, and nothing is left
// behind.
func TestSaveConcurrently(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	const n = 8
	gaps := map[float64]bool{}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		c := Default()
		c.Settings.Gap = float64(i + 1)
		gaps[c.Settings.Gap] = true
		wg.Go(func() { errs <- Save(path, c) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{path, filepath.Join(dir, BackupFile)} {
		cfg, problems, err := Load(p)
		if err != nil || len(problems) > 0 {
			t.Fatalf("%s: %v, %v", filepath.Base(p), problems, err)
		}
		if !gaps[cfg.Settings.Gap] {
			t.Errorf("%s has gap %v, which no save wrote", filepath.Base(p), cfg.Settings.Gap)
		}
	}
	checkNoTempFiles(t, dir)
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// checkNoTempFiles fails if an atomic write left a temporary file behind.
func checkNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Errorf("temporary files left behind: %v", matches)
	}
}
