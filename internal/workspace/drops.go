package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// This file handles files, folders, and apps that the user drops on
// Bench's window from Finder (decision 73). Wails gives the paths, and the
// element of Bench's page that they were dropped on, which says what the
// drop is for (DropTarget):
//
//   - On a terminal, the paths are typed into it, quoted for the shell and
//     with a space between each, as Terminal and iTerm2 type them. The
//     frontend types them (pasteCommand), as a paste, so that a program
//     that asks for pastes to be marked (bracketed paste) gets them marked.
//   - On a pane's header, an empty pane, or a pane whose item isn't ready,
//     a folder adds a new terminal in it to the pane, and an app adds the
//     app, as the add dialog's Apps rows do.
//   - On the + after the tabs, they go to a new tab, one pane for each, as
//     the New tab dialog makes a tab of windows that are open.
//   - Anywhere else, Bench says where to drop.
//
// Drops reach only Bench's own views: a docked app's window is the app's,
// and a Web item's page is the page's, so a drop there goes to the app or
// the page.
//
// A drop on a pane or the + that gives a new terminal the keyboard brings
// Bench forward, if another app, such as the Finder, was in front. That
// isn't the user going to Bench, so it leaves the windows over Bench where
// they are, the Finder's that the drag came from among them, rather than
// putting them out of sight as Bench does when the user brings it forward
// (droppedForwardLocked, decision 68).

// What a drop is on (DropTarget.Kind): the values of the element's
// data-file-drop-target attribute in the frontend.
const (
	DropTerminal  = "terminal"
	DropPane      = "pane"
	DropNewTab    = "new-tab"
	DropElsewhere = "window"
)

// DropTarget is what files were dropped on: its kind, and the terminal's
// item ID, for DropTerminal, or the pane's ID, in the current tab, for
// DropPane.
type DropTarget struct {
	Kind string
	Item string
	Pane string
}

// The attributes of the elements of Bench's page that take drops:
// data-file-drop-target says what the element is (DropTarget.Kind), and
// for a terminal, data-drop-item names its item, and for a pane,
// data-drop-pane names it. Wails' runtime finds the nearest such element
// under a drop and gives its attributes with the paths.
const (
	dropKindAttr = "data-file-drop-target"
	dropItemAttr = "data-drop-item"
	dropPaneAttr = "data-drop-pane"
)

// DropTargetOf returns what a drop is on, from the attributes of the
// element of the page it's on. With none, as when the drop missed every
// element that takes one, it's on the window, and Bench says where to
// drop.
func DropTargetOf(attrs map[string]string) DropTarget {
	kind := attrs[dropKindAttr]
	switch kind {
	case DropTerminal, DropPane, DropNewTab:
	default:
		kind = DropElsewhere
	}
	return DropTarget{Kind: kind, Item: attrs[dropItemAttr], Pane: attrs[dropPaneAttr]}
}

// pasteCommand asks the window that shows terminal ItemID to type Text
// into it, as a paste (Command.Text).
const pasteCommand = "paste-terminal"

// dropLimit is the most folders and apps that one drop adds, so that a
// drop of a whole folder's contents doesn't open dozens of terminals.
const dropLimit = 8

// What a dropped path is.
const (
	droppedFolder = "folder"
	droppedApp    = "app"
	droppedFile   = "file"
)

// dropped is a path that the user dropped, with what it is, its name for
// messages and a new tab, and for an app, its bundle.
type dropped struct {
	path, kind, name string
	app              platform.AppInfo
}

// The refusals of a drop, each short enough for the app's toast, which
// is one line, in Bench's smallest window, and with the step to take
// first.
var (
	errDropElsewhere = errors.New("Drop on a terminal, a pane's header, or the + after the tabs.")
	errDropNothing   = errors.New("Bench got no files from that drop.")
	errDropEnded     = errors.New("The command has finished. Press Return, then drop again.")
	errDropTooMany   = fmt.Errorf("Drop at most %d folders and apps at once. Bench opened none.", dropLimit)
	// errDropFiles says that a drop on a pane or the + held only files,
	// which Bench doesn't open there.
	errDropFiles  = errors.New("Drop folders and apps here. Drop files on a terminal.")
	errNoDemoDrop = errors.New("Pretend drops are only for the browser preview.")
)

// leftOutText says how many files a drop on a pane or the + left out.
func leftOutText(n int) string {
	files := "1 file"
	if n != 1 {
		files = strconv.Itoa(n) + " files"
	}
	return "Bench left out " + files + ". Drop files on a terminal."
}

// DropFiles handles paths that the user dropped on Bench's window from
// Finder, on target (see this file's comment). Paths that aren't absolute
// are left out.
func (e *Engine) DropFiles(target DropTarget, paths []string) error {
	paths = cleanDropped(paths)
	if len(paths) == 0 {
		return errDropNothing
	}
	switch target.Kind {
	case DropTerminal:
		return e.dropOnTerminal(target.Item, paths)
	case DropPane, DropNewTab:
		things, note, err := readDropped(paths)
		if err != nil {
			return err
		}
		if target.Kind == DropPane {
			return e.dropOnPane(target.Pane, things, note)
		}
		return e.dropOnNewTab(things, note)
	default:
		return errDropElsewhere
	}
}

// DemoDropFiles is DropFiles for the browser preview, whose browser can't
// give the paths of what's dropped on it: the preview's frontend pretends
// to be Wails' native drop, with the paths and the attributes of the
// element they're dropped on (DropTargetOf).
func (e *Engine) DemoDropFiles(paths []string, attrs map[string]string) error {
	if !e.opts.Demo {
		return errNoDemoDrop
	}
	return e.DropFiles(DropTargetOf(attrs), paths)
}

// cleanDropped returns the absolute paths in paths, cleaned, without
// repeats, in order.
func cleanDropped(paths []string) []string {
	var out []string
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			continue
		}
		p = filepath.Clean(p)
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// dropOnTerminal types paths into terminal itemID, as Terminal does: each
// quoted for the shell (shellPath), with a space after each.
func (e *Engine) dropOnTerminal(itemID string, paths []string) error {
	e.lock()
	defer e.unlock()
	it := e.model.Items[itemID]
	switch {
	case it == nil || it.Kind != KindBuiltin:
		return errNoTerminal
	case it.ended:
		return errDropEnded
	}
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(shellPath(p))
		b.WriteByte(' ')
	}
	em, c := e.em, Command{Name: pasteCommand, ItemID: itemID, Text: b.String()}
	e.later(func() { em.Command(c) })
	return nil
}

// readDropped reads what each path is, for a drop on a pane or the +: the
// folders and apps, in order, without an app twice, and a note for the
// toast about what's left out. Files are left out, as are apps that macOS
// can't open, and paths that have gone. With no folders or apps, it
// returns why; with too many, it refuses them all.
func readDropped(paths []string) ([]dropped, string, error) {
	var things []dropped
	files := 0
	var problems []error
	for _, p := range paths {
		d, err := readOneDropped(p)
		switch {
		case err != nil:
			problems = append(problems, err)
		case d.kind == droppedFile:
			files++
		case d.kind == droppedApp && slices.ContainsFunc(things, func(x dropped) bool { return sameApp(x.app.BundleID, d.app.BundleID) }):
		default:
			things = append(things, d)
		}
	}
	switch {
	case len(things) == 0 && len(problems) > 0:
		return nil, "", problems[0]
	case len(things) == 0:
		return nil, "", errDropFiles
	case len(things) > dropLimit:
		return nil, "", errDropTooMany
	}
	var note []string
	if files > 0 {
		note = append(note, leftOutText(files))
	}
	for _, err := range problems {
		note = append(note, err.Error())
	}
	return things, strings.Join(note, " "), nil
}

// packageExts are the extensions, in lower case, of the common packages:
// folders that the Finder shows as one document, such as an Xcode project
// or a Photos library. Bench takes them for files, as the Finder does, and
// so types their paths on a terminal, and leaves them out on a pane or the
// +. An app is a package too, which Bench opens as an app.
var packageExts = []string{
	".appex", ".band", ".bundle", ".component", ".docset", ".dsym", ".fcpbundle", ".framework",
	".imovielibrary", ".kext", ".key", ".logicx", ".mdimporter", ".musiclibrary", ".numbers",
	".pages", ".photoslibrary", ".playground", ".plugin", ".prefpane", ".qlgenerator", ".rtfd",
	".saver", ".scptd", ".sparsebundle", ".tvlibrary", ".vst", ".vst3", ".wdgt", ".xcarchive",
	".xcodeproj", ".xcworkspace", ".xpc",
}

// isPackage reports whether a folder is one of the common packages, by its
// name (packageExts).
func isPackage(path string) bool {
	return slices.Contains(packageExts, strings.ToLower(filepath.Ext(path)))
}

// readOneDropped reads what a dropped path is: an app, if it's a folder
// whose name ends in .app, which must have a bundle ID; a file, if it's a
// file or another package (isPackage); or else a folder.
func readOneDropped(path string) (dropped, error) {
	name := filepath.Base(path)
	info, err := os.Stat(path)
	if err != nil {
		return dropped{}, errCantBrowse(name)
	}
	switch {
	case info.IsDir() && strings.EqualFold(filepath.Ext(path), ".app"):
		app, err := apps.ReadBundle(path)
		if err != nil {
			return dropped{}, errNotAnApp(apps.DisplayName(name))
		}
		return dropped{path: path, kind: droppedApp, name: apps.DisplayName(app.Name), app: app}, nil
	case info.IsDir() && !isPackage(path):
		return dropped{path: path, kind: droppedFolder, name: name}, nil
	default:
		return dropped{path: path, kind: droppedFile, name: name}, nil
	}
}

// dropOnPane adds folders, as new terminals in them, and apps to pane
// paneID of the current tab, as the add dialog adds them, and shows the
// first. The pane is focused, as the user chose it, and what it shows gets
// the keyboard: the first new terminal, or what the pane showed already
// while an app opens. A note says what was left out. If nothing could be
// added, as when no app opened, the drop is refused with the first reason.
func (e *Engine) dropOnPane(paneID string, things []dropped, note string) error {
	e.lock()
	defer e.unlock()
	t, p, err := e.targetPaneLocked(paneID)
	if err != nil {
		return err
	}
	shown, errs := e.addDroppedLocked(t, p, things, nil)
	if len(errs) == len(things) {
		return errs[0]
	}
	for _, err := range errs {
		note = strings.TrimSpace(note + " " + err.Error())
	}
	e.model.FocusPane(t, p)
	e.finishAddLocked(shown)
	e.noteDropForwardLocked()
	if note != "" {
		e.toastLocked(note)
	}
	return nil
}

// addDroppedLocked adds things to pane p of tab t, and returns the first
// item it added, if one is there yet: an app that isn't running brings its
// windows once it has opened. It returns why each thing that it couldn't
// add wasn't added, having added the rest. A thing with an error added
// nothing, and left nothing to come: addAppLocked fails only before it
// adds a window or opens the app. With step, it notes the windows it took
// from "Not in a tab", for Undo.
func (e *Engine) addDroppedLocked(t *Tab, p *layout.Node, things []dropped, step *undoStep) (*Item, []error) {
	var first *Item
	var errs []error
	for _, d := range things {
		var it *Item
		var err error
		switch d.kind {
		case droppedFolder:
			it = e.addTerminalLocked(t, p, newTerminal{folder: term.ShortPath(d.path)})
		case droppedApp:
			e.noteBrowsedLocked(d.app)
			var outside []outsideWindow
			if step != nil {
				outside = e.appOutsideLocked(d.app.BundleID)
			}
			it, err = e.addAppLocked(t, p, d.app.BundleID)
			if step != nil {
				step.outside = append(step.outside, e.outsideItemsLocked(outside)...)
			}
		}
		if err != nil {
			errs = append(errs, err)
		}
		if first == nil {
			first = it
		}
	}
	return first, errs
}

// appOutsideLocked returns the app's windows that aren't in a tab, as they
// are now, for Undo to put back once addAppLocked has taken them into a
// tab: its popped-out items' windows, and its windows that Bench doesn't
// manage.
func (e *Engine) appOutsideLocked(bundleID string) []outsideWindow {
	m := e.model
	var out []outsideWindow
	for _, id := range m.Floating {
		if it := m.Items[id]; isAppOf(it, bundleID) && it.WindowID != 0 {
			if was, ok := e.outsideLocked(id); ok {
				out = append(out, was)
			}
		}
	}
	for _, w := range e.unmanaged {
		if sameApp(w.BundleID, bundleID) {
			if was, ok := e.outsideLocked(floatingID(w.ID)); ok {
				out = append(out, was)
			}
		}
	}
	return out
}

// outsideItemsLocked returns the windows of outside that a change took
// into a tab, each with the item it's in now: a window that Bench didn't
// manage is in the item made for it.
func (e *Engine) outsideItemsLocked(outside []outsideWindow) []outsideWindow {
	m := e.model
	var out []outsideWindow
	for _, was := range outside {
		if was.window.ID != 0 {
			it := m.ItemByWindow(was.window.ID)
			if it == nil {
				continue
			}
			was.item = it.ID
		}
		if !m.IsFloating(was.item) && m.Items[was.item] != nil {
			out = append(out, was)
		}
	}
	return out
}

// dropOnNewTab makes a new tab for things, after the other tabs, as the
// New tab dialog makes a tab of windows that are open (NewTabFromWindows):
// named after the first thing, numbered if another tab has that name; with
// the layout suggested for their number (suggestPreset); and each in a
// pane of its own, in order, those after the last pane in the last pane.
// Bench switches to it, and the config gets it. The toast's Undo takes it
// away again, with the terminals it made, and puts the windows it took
// from "Not in a tab" back.
func (e *Engine) dropOnNewTab(things []dropped, note string) error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	m := e.model
	name := uniqueTabName(m.Tabs, things[0].name, -1)
	preset := suggestPreset(len(things))
	cfg, err := e.draftConfigLocked(-1, name, TabDraft{Preset: preset})
	if err != nil {
		e.unlock()
		return err
	}
	step := e.beginUndoLocked()
	was := e.cfg.Clone()
	step.cfgTabs, step.cfgRules = was.Tabs, was.Rules
	t, err := m.AddTab(name, preset, smallScreensOf(""))
	if err != nil {
		e.unlock()
		return err
	}
	panes := t.Leaves()
	var problems []error
	for i, d := range things {
		p := panes[min(i, len(panes)-1)]
		_, errs := e.addDroppedLocked(t, p, []dropped{d}, step)
		problems = append(problems, errs...)
	}
	// Nothing went in the tab, and nothing will: the tab goes again, before
	// the config or Undo has it, and the drop is refused.
	if len(problems) == len(things) {
		_, _, err := m.DeleteTab(m.IndexOf(t), func(*Item) bool { return false })
		e.unlock()
		if err != nil {
			return err
		}
		return problems[0]
	}
	for _, p := range panes {
		if len(p.Items) > 0 {
			p.Active = p.Items[0]
			showAgain(m.Items[p.Active])
		}
	}
	t.Focus = panes[0].ID
	e.cfg = cfg
	ti := m.IndexOf(t)
	e.syncConfigTab(cfg, ti)
	named := undoName{"New Tab from Drop", "made a new tab, " + name + ", from a drop"}
	undone := "Deleted the new tab, " + name + "."
	if len(step.outside) > 0 {
		undone = "Deleted the new tab, " + name + ", and put its windows back."
	}
	undo := e.keepUndoLocked(step, named, undone)
	e.switchLocked(ti, switchOpts{focus: true})
	e.noteDropForwardLocked()
	writable := e.writable
	saved := cfg.Clone()
	e.holdStateLocked()
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return err
	}
	parts := []string{"Made a new tab, " + name + ".", note}
	for _, err := range problems {
		parts = append(parts, err.Error())
	}
	msg := strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), " ")
	if notSaved != "" {
		msg += " " + notSavedText(notSaved)
	}
	e.em.ActionToast(undoToast(msg, undo))
	return nil
}

// dropForwardGrace is how long after a drop that gave a terminal the
// keyboard Bench coming forward is taken for the drop's doing
// (droppedForwardLocked): macOS says so at once.
const dropForwardGrace = 2 * time.Second

// noteDropForwardLocked notes, after a drop on a pane or the +, whether it
// gave a terminal in Bench's window the keyboard while another app was
// active (focusNowLocked): that brings Bench forward, for the drop, and
// the windows over Bench stay as they are (droppedForwardLocked).
func (e *Engine) noteDropForwardLocked() {
	e.finderDropAt = time.Time{}
	if e.benchActive || e.benchHidden {
		return
	}
	if it := e.model.Items[e.focusedShownLocked()]; it != nil && it.Kind == KindBuiltin && !it.OwnWindow {
		e.finderDropAt = e.clk.Now()
	}
}

// droppedForwardLocked reports, as Bench becomes active, whether a drop
// brought it forward (noteDropForwardLocked), and forgets the drop: Bench
// coming forward after that is the user's doing.
func (e *Engine) droppedForwardLocked() bool {
	at := e.finderDropAt
	e.finderDropAt = time.Time{}
	return !at.IsZero() && e.clk.Now().Sub(at) < dropForwardGrace
}

// shellPath writes a path for a shell to read as it is, as Terminal types a
// dropped file's path: a character that shells treat specially, such as a
// space, a quote, or a bracket, gets a backslash before it, and letters
// from other alphabets stay as they are. A path with a character that a
// backslash can't keep, such as a new line, or that isn't UTF-8, is
// written as $'...', with escapes for those, which zsh, bash, and ksh
// read.
func shellPath(path string) string {
	if needsDollarQuotes(path) {
		return dollarQuoted(path)
	}
	var b strings.Builder
	for _, r := range path {
		if r < utf8.RuneSelf && !plainPathByte(byte(r)) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// plainPathByte reports whether an ASCII character is read as it is by
// shells, anywhere in a word.
func plainPathByte(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-_./+:@%,", c) >= 0
}

// needsDollarQuotes reports whether a path has a control character, or
// isn't UTF-8.
func needsDollarQuotes(path string) bool {
	if !utf8.ValidString(path) {
		return true
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// dollarQuoted writes s as $'...': a backslash and a quote get a backslash,
// common control characters their usual escapes, and other control
// characters, bytes that aren't UTF-8, and ! (which bash's history could
// read), \x escapes.
func dollarQuoted(s string) string {
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size <= 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\\' || r == '\'':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f || r == '!':
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	b.WriteByte('\'')
	return b.String()
}
