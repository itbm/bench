package workspace

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// This file has the add dialog's Browse…, which adds an app that the user
// chooses in macOS's open panel, or fills in the New terminal… form for a
// program or script.

// browseStart is the folder the open panel starts in, until the user has
// chosen something in it.
const browseStart = "/Applications"

// sourceBrowse is Browse…'s row in the add dialog (AddOption.Browses).
const sourceBrowse = "browse"

// shebangLimit is the most of a script's first line that's read for its #!
// line.
const shebangLimit = 512

// BrowseToAdd shows macOS's open panel, for the add dialog's Browse…, for
// the user to choose an app, or a program or script, to add to a pane of
// the current tab, and waits for them, without the lock. The panel is a
// window of its own, which stays in front of Bench's window only while
// Bench is lowered, so it lowers Bench first: the dialog has closed, but
// its call that lowers Bench may come after this one.
//
//   - An app is added to the pane as the dialog's Apps rows add one, even
//     one outside the folders that Bench scans, and Added is its name. It
//     must still be the tab the user chose from.
//   - A program or script is for the dialog's New terminal… form, which
//     the user checks before adding it, under its file name (Name). Command
//     runs it: by its name if the user's shell finds that program by it, as
//     a typed command would, or else by its path, with the program that
//     runs a script that can't run itself. A command by path starts in the
//     program's folder (Folder).
//   - If the user cancels, it returns nothing, and the dialog opens again.
//
// The panel starts in the folder of what the user last chose with it, or
// in /Applications.
func (e *Engine) BrowseToAdd(paneID string) (BrowseResult, error) {
	e.lock()
	if _, _, err := e.targetPaneLocked(paneID); err != nil {
		e.unlock()
		return BrowseResult{}, err
	}
	tab := e.model.Current
	// The panel has the keyboard, so it isn't given back to the window that
	// had it before the jump palette opened (refocusAfterJumpLocked).
	e.jump = jumpNote{}
	if e.overlay {
		e.setOverlayLocked(false)
	}
	start := e.browseDir
	e.unlock()
	if start == "" {
		start = browseStart
	}

	path, err := e.p.ChooseToAdd(start)
	switch {
	case errors.Is(err, platform.ErrUnsupported):
		return BrowseResult{}, errNoBrowse
	case errors.Is(err, platform.ErrBusy):
		return BrowseResult{}, errBrowsing
	case err != nil:
		e.log.Warn("couldn't show the open panel", "error", err)
		return BrowseResult{}, errBrowseFailed
	case path == "":
		return BrowseResult{}, nil
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return BrowseResult{}, errCantBrowse(filepath.Base(path))
	}
	if info.IsDir() && strings.EqualFold(filepath.Ext(path), ".app") {
		return e.addBrowsedApp(paneID, tab, path)
	}
	result, err := e.browsedProgram(path, info)
	if err != nil {
		return BrowseResult{}, err
	}
	e.lock()
	e.browseDir = filepath.Dir(path)
	e.unlock()
	return result, nil
}

// addBrowsedApp adds the app at path, which the user chose with Browse…, to
// the pane, if tab is still the current tab. The app is noted as installed
// (noteBrowsedLocked), wherever it is, so that it has its name.
func (e *Engine) addBrowsedApp(paneID string, tab int, path string) (BrowseResult, error) {
	info, err := apps.ReadBundle(path)
	if err != nil {
		e.log.Warn("couldn't read an app chosen to add", "error", err)
		return BrowseResult{}, errNotAnApp(apps.DisplayName(filepath.Base(path)))
	}
	name := apps.DisplayName(info.Name)
	e.lock()
	defer e.unlock()
	e.browseDir = filepath.Dir(path)
	if e.model.Current != tab {
		return BrowseResult{}, errBrowseTabChanged(name)
	}
	t, p, err := e.targetPaneLocked(paneID)
	if err != nil {
		return BrowseResult{}, err
	}
	e.noteBrowsedLocked(info)
	shown, err := e.addAppLocked(t, p, info.BundleID)
	if err != nil {
		return BrowseResult{}, err
	}
	e.finishAddLocked(shown)
	return BrowseResult{Added: name}, nil
}

// noteBrowsedLocked notes an app that the user chose with Browse… as
// installed, for as long as Bench runs, as a scan may not find it
// (setInstalledLocked).
func (e *Engine) noteBrowsedLocked(info platform.AppInfo) {
	if e.browsed == nil {
		e.browsed = map[string]platform.AppInfo{}
	}
	key := strings.ToLower(info.BundleID)
	e.browsed[key] = info
	if e.installed != nil {
		if _, ok := e.installed[key]; !ok {
			e.installed[key] = info
		}
	}
}

// withBrowsedLocked returns the installed apps from a scan, with the apps
// the user chose with Browse… that it didn't find, in order by name,
// ignoring case, as the scan orders them.
func (e *Engine) withBrowsedLocked(list []platform.AppInfo) []platform.AppInfo {
	seen := map[string]bool{}
	for _, info := range list {
		seen[strings.ToLower(info.BundleID)] = true
	}
	out := list
	for key, info := range e.browsed {
		if !seen[key] {
			out = append(slices.Clip(out), info)
		}
	}
	if len(out) == len(list) {
		return list
	}
	slices.SortStableFunc(out, func(a, b platform.AppInfo) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.Name, b.Name),
			cmp.Compare(a.Path, b.Path),
		)
	})
	return out
}

// browsedProgram returns the command for the New terminal… form that runs
// the file at path, which the user chose with Browse…: a program, or a
// script that a shell, Python, or another program it names runs. It asks
// the user's shell where it finds a program by its name, which may take a
// moment, so it's called without the lock.
func (e *Engine) browsedProgram(path string, info os.FileInfo) (BrowseResult, error) {
	name := filepath.Base(path)
	if !info.Mode().IsRegular() {
		return BrowseResult{}, errNotProgram(name)
	}
	short := term.ShortPath(path)
	folder := term.ShortPath(filepath.Dir(path))
	if info.Mode()&0o111 != 0 {
		if found := e.terms.LookPath(name); found != "" && sameFile(found, path) {
			return BrowseResult{Command: name, Name: name}, nil
		}
		return BrowseResult{Command: shellWord(short), Name: name, Folder: folder}, nil
	}
	runner := interpreterFor(path)
	if runner == "" {
		return BrowseResult{}, errNotProgram(name)
	}
	return BrowseResult{Command: runner + " " + shellWord(short), Name: name, Folder: folder}, nil
}

// sameFile reports whether two paths are the same file, after links.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}

// interpreters run scripts that can't run themselves, by their extensions,
// when they have no #! line.
var interpreters = map[string]string{
	".sh":      "sh",
	".command": "zsh",
	".zsh":     "zsh",
	".bash":    "bash",
	".py":      "python3",
	".rb":      "ruby",
	".pl":      "perl",
	".js":      "node",
	".mjs":     "node",
	".cjs":     "node",
}

// interpreterFor returns what runs the script at path, which can't run
// itself, as a shell word: the program its #! line names, by name when the
// line runs it with env, or else by its path; or else the program for its
// extension (interpreters). It returns "" for a file that isn't a script.
func interpreterFor(path string) string {
	if line, ok := shebang(path); ok {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return ""
		}
		if filepath.Base(fields[0]) != "env" {
			return shellWord(fields[0])
		}
		// env's options, such as -S, and variables come before the
		// program.
		for _, f := range fields[1:] {
			if !strings.HasPrefix(f, "-") && !strings.Contains(f, "=") {
				return shellWord(f)
			}
		}
		return ""
	}
	return interpreters[strings.ToLower(filepath.Ext(path))]
}

// shebang returns what follows #! on the first line of the file at path,
// if it starts with one.
func shebang(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, shebangLimit)
	line, err := r.ReadSlice('\n')
	if err != nil && !errors.Is(err, bufio.ErrBufferFull) && len(line) == 0 {
		return "", false
	}
	rest, ok := strings.CutPrefix(string(line), "#!")
	if !ok {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// plainWord matches a word that shells read as it's written.
var plainWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellWord writes a path, or another word, for a shell to read as it is:
// in single quotes if it has spaces or other characters that shells treat
// specially. A path in the home folder keeps its "~/" outside them, so that
// the shell still reads it as the home folder.
func shellWord(s string) string {
	if rest, ok := strings.CutPrefix(s, "~/"); ok && rest != "" {
		return "~/" + shellWord(rest)
	}
	if plainWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// errCantBrowse says that what the user chose with Browse… can't be read.
func errCantBrowse(name string) error {
	return fmt.Errorf("Bench couldn't open %s. Check that it's still there.", name)
}

// errNotAnApp says that what the user chose with Browse… looks like an app
// but isn't one that macOS can open, as it has no bundle ID.
func errNotAnApp(name string) error {
	return fmt.Errorf("%s isn't an app that Bench can open.", name)
}

// errNotProgram says that what the user chose with Browse… is neither an
// app nor a program or script.
func errNotProgram(name string) error {
	return fmt.Errorf("%s isn't an app, or a program or script that a terminal can run.", name)
}

// errBrowseTabChanged says that Browse… didn't add an app, as the tab
// changed while the user chose it.
func errBrowseTabChanged(name string) error {
	return fmt.Errorf("Bench didn't add %s, as the tab changed while you chose it. Add it again from this tab.", name)
}
