package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// mine is a config file of the user's own.
const mine = "# My own layout.\n[[tab]]\nname = \"Mine\"\nlayout = \"single\"\n"

func TestReadDoesntWriteAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFile)
	if _, _, _, err := Read(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read of a missing file = %v, want fs.ErrNotExist", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read wrote the file: %v", err)
	}
}

func TestLoadFileReturnsTheContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFile)
	_, _, data, err := LoadFile(path, nil)
	if err != nil || string(data) != string(DefaultTOML()) {
		t.Errorf("first run: %v\n%s\nwant the default config written", err, data)
	}
	writeFile(t, path, mine)
	cfg, problems, data, err := LoadFile(path, nil)
	if err != nil || len(problems) > 0 || string(data) != mine || cfg.Tabs[0].Name != "Mine" {
		t.Errorf("LoadFile = %+v %v %v\n%s", cfg, problems, err, data)
	}
}

func TestFileSum(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFile)
	if sum, err := FileSum(path); err != nil || sum != (Sum{}) {
		t.Errorf("a missing file's Sum = %x, %v; want the zero Sum", sum, err)
	}
	writeFile(t, path, mine)
	if sum, err := FileSum(path); err != nil || sum != SumOf([]byte(mine)) {
		t.Errorf("FileSum = %x, %v", sum, err)
	}
}

func TestSaveIfRefusesAChangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	writeFile(t, path, mine)
	read := SumOf([]byte(mine))
	const edited = mine + "\n[[rule]]\napp = \"com.apple.mail\"\nfloat = true\n"
	writeFile(t, path, edited)

	if _, err := SaveIf(path, Example(), read); !errors.Is(err, ErrChanged) {
		t.Fatalf("SaveIf over a changed file = %v, want ErrChanged", err)
	}
	if got := readFile(t, path); got != edited {
		t.Errorf("config = %q, want the edit kept", got)
	}
	if _, err := os.Stat(filepath.Join(dir, BackupFile)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("SaveIf made a backup: %v", err)
	}

	data, err := SaveIf(path, Example(), SumOf([]byte(edited)))
	if err != nil || string(data) != string(ExampleTOML()) {
		t.Fatalf("SaveIf = %v\n%s", err, data)
	}
	if got := readFile(t, path); got != string(ExampleTOML()) {
		t.Errorf("config = %q, want the example", got)
	}
	if got := readFile(t, filepath.Join(dir, BackupFile)); got != edited {
		t.Errorf("backup = %q, want the edited file", got)
	}
	checkNoTempFiles(t, dir)
}

func TestSaveIfWritesAMissingFileForTheZeroSum(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFile)
	if _, err := SaveIf(path, Example(), SumOf([]byte(mine))); !errors.Is(err, ErrChanged) {
		t.Errorf("SaveIf where a file was = %v, want ErrChanged", err)
	}
	if _, err := SaveIf(path, Example(), Sum{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != string(ExampleTOML()) {
		t.Errorf("config = %q", got)
	}
}

func TestUseBackupSwapsTheFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFile)
	const broken = "[[tab]]\nnam = \"Mine\"\n"
	writeFile(t, path, broken)
	if BackupLoads(path) {
		t.Error("BackupLoads without a backup")
	}
	if _, _, err := UseBackup(path); err == nil || err.Error() != "there's no config.toml.bak" {
		t.Errorf("UseBackup without one = %v", err)
	}
	writeFile(t, path+".bak", "gapp = 1\n")
	if BackupLoads(path) {
		t.Error("BackupLoads with a backup that has problems")
	}
	if _, _, err := UseBackup(path); err == nil || readFile(t, path) != broken {
		t.Errorf("UseBackup of a backup with problems = %v", err)
	}

	writeFile(t, path+".bak", mine)
	if !BackupLoads(path) {
		t.Error("BackupLoads = false for a good backup")
	}
	cfg, data, err := UseBackup(path)
	if err != nil || cfg.Tabs[0].Name != "Mine" || string(data) != mine {
		t.Fatalf("UseBackup = %+v, %v\n%s", cfg, err, data)
	}
	if readFile(t, path) != mine || readFile(t, path+".bak") != broken {
		t.Errorf("the files weren't swapped: config %q, backup %q", readFile(t, path), readFile(t, path+".bak"))
	}
	checkNoTempFiles(t, dir)
}

func TestKeepGoodAndLoadGood(t *testing.T) {
	path := filepath.Join(t.TempDir(), GoodFile)
	if LoadGood(path) != nil {
		t.Error("LoadGood without a file")
	}
	if err := KeepGood(path, []byte(mine)); err != nil {
		t.Fatal(err)
	}
	if cfg := LoadGood(path); cfg == nil || cfg.Tabs[0].Name != "Mine" {
		t.Errorf("LoadGood = %+v", cfg)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the copy: %v %v, want it private", info, err)
	}
	// An unchanged copy isn't written again.
	before, _ := os.Stat(path)
	if err := KeepGood(path, []byte(mine)); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.Stat(path); !os.SameFile(before, after) {
		t.Error("KeepGood replaced an unchanged copy")
	}
	// A copy with problems, as after an update changes what Bench
	// accepts, isn't used.
	writeFile(t, path, "gapp = 1\n")
	if LoadGood(path) != nil {
		t.Error("LoadGood of a copy with problems")
	}
}

func TestPeekStateSetsNothingAside(t *testing.T) {
	requireLayout(t)
	path := filepath.Join(t.TempDir(), StateFile)
	// Tabs that the config doesn't have don't matter.
	s := goodState()
	s.Tabs[2].Name = "Reading"
	s.Tabs = append(s.Tabs, TabState{Name: "More", Root: pane("p9")})
	writeState(t, path, s)
	if st, aside, err := PeekState(path); st == nil || aside != nil || err != nil || st.Tabs[3].Name != "More" {
		t.Errorf("PeekState = %v, %v, %v; want the state as saved", st, aside, err)
	}
	// A file that can't be used isn't returned, but says why, and stays.
	s = goodState()
	s.Version = 0
	writeState(t, path, s)
	st, aside, err := PeekState(path)
	if st != nil || err != nil || aside == nil || aside.Reason == nil ||
		!strings.Contains(aside.Reason.Error(), "version 0") || !slices.Equal(aside.Tabs, s.TabNames()) {
		t.Errorf("PeekState = %v, %+v, %v; want why, and the tabs", st, aside, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state.json was moved: %v", err)
	}
	if _, err := os.Stat(path + ".bad"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("state.json was set aside: %v", err)
	}

	// Once Bench is about to save over it, it's set aside whole.
	if err := SetAsideWhole(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("state.json is still there: %v", err)
	}
	if _, err := os.Stat(path + ".bad"); err != nil {
		t.Errorf("state.json.bad: %v", err)
	}
}
