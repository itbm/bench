package workspace

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// This file has launchers, config.toml's [[launcher]] entries: terminals
// that type a command into the user's shell, and an app's window on a
// folder or link, which the add dialog lists first, and Jump to… after the
// tabs and windows, and add each time they're chosen. The add dialog's Save as launcher
// (SaveLauncher) and Settings ▸ Launchers (SaveSettings) make and edit
// them, and ChooseFolder shows the folder picker for their folders.

// Launcher kinds (LauncherView.Kind).
const (
	launcherTerminal = "terminal"
	launcherApp      = "app"
)

// launcherViewsLocked returns the launchers as the lists show them, in
// config order.
func (e *Engine) launcherViewsLocked() []LauncherView {
	out := []LauncherView{}
	for _, l := range e.cfg.Launchers {
		out = append(out, e.launcherViewLocked(l))
	}
	return out
}

// launcherViewLocked returns a launcher as the lists show it: a terminal
// launcher with its own monogram and its command and folder as the hint, and
// an app launcher with its app's, and the app and what it opens.
func (e *Engine) launcherViewLocked(l config.Launcher) LauncherView {
	v := LauncherView{Name: l.Name, Folder: l.Folder}
	if !l.IsApp() {
		folder := l.Folder
		if folder == "" {
			folder = defaultFolder
		}
		what := l.Run
		if what == "" {
			what = defaultShell
		}
		v.Kind, v.Command, v.StartAgain = launcherTerminal, l.Run, l.StartsAgain()
		v.App, v.Mono, v.Hint = terminalApp, apps.Mono(l.Name), what+" · "+folder
		return v
	}
	opens := l.URL
	if opens == "" {
		opens = l.Folder
	}
	v.Kind, v.BundleID, v.URL, v.Title = launcherApp, l.App, l.URL, l.Title
	v.App = e.appName(l.App)
	v.Mono, v.Hint = apps.Mono(v.App), v.App+" · "+opens
	v.Icon = e.appIconLocked(l.App)
	return v
}

// launcherOf returns the launcher that a view of one saves: a terminal
// launcher's folder, command, and whether it starts again, or an app
// launcher's app, folder or link, and title. Spaces around each field are
// dropped.
func launcherOf(v LauncherView) (config.Launcher, error) {
	l := config.Launcher{Name: strings.TrimSpace(v.Name), Folder: strings.TrimSpace(v.Folder)}
	switch v.Kind {
	case launcherTerminal, "":
		l.Run = strings.TrimSpace(v.Command)
		if l.Run != "" {
			l.StartAgain = config.StartAgainOf(v.StartAgain)
		}
	case launcherApp:
		l.App, l.URL, l.Title = strings.TrimSpace(v.BundleID), strings.TrimSpace(v.URL), strings.TrimSpace(v.Title)
	default:
		return config.Launcher{}, errLauncherKind
	}
	return l, nil
}

// launchersOf returns the launchers that Settings ▸ Launchers saves, nil
// for none. A launcher without a name gets one, as SaveLauncher gives one,
// numbered if another launcher has it. It refuses a launcher that
// config.toml couldn't have, saying why in the user's terms.
func launchersOf(vs []LauncherView, appName namer) ([]config.Launcher, error) {
	var out []config.Launcher
	for _, v := range vs {
		l, err := launcherOf(v)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	for i := range out {
		if out[i].Name == "" {
			out[i].Name = uniqueLauncherName(out, defaultLauncherName(out[i], appName))
		}
	}
	for i, l := range out {
		if err := checkLauncher(l, appName); err != nil {
			return nil, err
		}
		if j := (&config.Config{Launchers: out[:i]}).LauncherIndex(l.Name); j >= 0 {
			return nil, errLauncherTaken(out[j].Name)
		}
	}
	return out, nil
}

// checkLauncher returns why config.toml couldn't have a launcher, in the
// words of the forms that make them, or nil. Validate checks it again.
func checkLauncher(l config.Launcher, appName namer) error {
	name := l.Name
	switch {
	case name == "":
		return errors.New("Give the launcher a name, which the add dialog and Jump to… show.")
	case strings.ContainsAny(name, "\r\n"):
		return errors.New("A launcher's name must be one line.")
	}
	if !l.IsApp() {
		if strings.ContainsFunc(l.Run, unicode.IsControl) {
			return fmt.Errorf("The command of %s must be one line, without tabs.", name)
		}
		return nil
	}
	app := appName(l.App)
	switch {
	case l.Folder != "" && l.URL != "":
		return fmt.Errorf("Give %s a folder or a link to open in %s, not both.", name, app)
	case l.Folder == "" && l.URL == "":
		return fmt.Errorf("Give %s a folder or a link to open in %s.", name, app)
	case l.URL != "" && !hasScheme(l.URL):
		return fmt.Errorf("The link of %s needs a scheme, such as https://.", name)
	case l.URL != "" && l.Title == "":
		return fmt.Errorf("Give %s a window title too: Bench docks the %s window whose title has it.", name, app)
	}
	return nil
}

// hasScheme reports whether a link has a scheme, such as https or zed.
func hasScheme(link string) bool {
	u, err := url.Parse(link)
	return err == nil && u.Scheme != ""
}

// defaultLauncherName names a launcher saved without a name: a terminal
// launcher after its command's program, or else its folder, or else
// "Terminal", and an app launcher after its app.
func defaultLauncherName(l config.Launcher, appName namer) string {
	if l.IsApp() {
		return appName(l.App)
	}
	for _, word := range strings.Fields(l.Run) {
		if !strings.Contains(word, "=") { // Not FOO=1, which sets a variable.
			return filepath.Base(word)
		}
	}
	if folder := strings.TrimRight(l.Folder, "/"); folder != "" && folder != defaultFolder {
		return filepath.Base(folder)
	}
	return terminalApp
}

// uniqueLauncherName returns name, numbered if a launcher has it already,
// ignoring case, as tab names are: "claude" becomes "claude (1)".
func uniqueLauncherName(ls []config.Launcher, name string) string {
	taken := func(n string) bool { return (&config.Config{Launchers: ls}).LauncherIndex(n) >= 0 }
	if !taken(name) {
		return name
	}
	base := unnumbered(name)
	for n := 1; ; n++ {
		if next := fmt.Sprintf("%s (%d)", base, n); !taken(next) {
			return next
		}
	}
}

// SaveLauncher adds a launcher after the others, for the add dialog's Save
// as launcher, and saves config.toml when it can. It returns the launcher's
// name, for the terminal that the dialog adds with it. A launcher without a
// name is named after its command's program, or its folder, numbered if
// another launcher has that name. A name that another launcher has
// already, ignoring case, is refused, naming that launcher.
func (e *Engine) SaveLauncher(v LauncherView) (string, error) {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	l, err := launcherOf(v)
	if err == nil && l.Name == "" {
		l.Name = uniqueLauncherName(e.cfg.Launchers, defaultLauncherName(l, e.appName))
	}
	if err == nil {
		err = checkLauncher(l, e.appName)
	}
	if i := e.cfg.LauncherIndex(l.Name); err == nil && i >= 0 {
		err = errLauncherTaken(e.cfg.Launchers[i].Name)
	}
	if err != nil {
		e.unlock()
		return "", err
	}
	cfg := e.cfg.Clone()
	cfg.Launchers = append(cfg.Launchers, l)
	if problems := cfg.Validate(); len(problems) > 0 {
		e.unlock()
		return "", fmt.Errorf("Bench can't save that: %s.", problems[0].Message)
	}
	e.cfg = cfg
	e.touch()
	writable := e.writable
	saved := cfg.Clone()
	e.unlock()

	notSaved, err := e.saveEdit(saved, writable)
	if err != nil {
		return l.Name, err
	}
	if notSaved != "" {
		e.em.Toast(appliedText(notSaved))
		return l.Name, nil
	}
	e.em.Toast("Saved launcher " + l.Name)
	return l.Name, nil
}

// addLauncherLocked adds what a launcher opens to a pane: a terminal that
// types its command into the shell and runs it, or its app's window.
func (e *Engine) addLauncherLocked(t *Tab, p *layout.Node, name string) (*Item, error) {
	i := e.cfg.LauncherIndex(name)
	if i < 0 {
		return nil, errNoLauncher(name)
	}
	l := e.cfg.Launchers[i]
	if l.IsApp() {
		return e.addAppLauncherLocked(t, p, l)
	}
	nt := newTerminal{folder: l.Folder, run: l.Run, name: l.Name, startAgain: l.StartsAgain()}
	return e.addTerminalLocked(t, p, nt), nil
}

// addAppLauncherLocked adds an app launcher's window to a pane. If the app
// has a window whose title has the launcher's title already, it's added as
// the add dialog adds a window from its groups: shown, if it's in the tab
// already, shown here as well, if it's in other tabs, or brought back into
// Bench. Otherwise a new item for the window takes its place in the pane,
// with the launcher's title and link, as an entry in config.toml with a
// url would: a window of the app that's in no tab goes there, or else the
// app opens the folder or link, as Open does for such an entry, and its
// window goes there.
func (e *Engine) addAppLauncherLocked(t *Tab, p *layout.Node, l config.Launcher) (*Item, error) {
	filter, link := launcherTarget(l)
	m := e.model
	for _, it := range m.orderedItems() {
		if !isAppOf(it, l.App) || it.WindowID == 0 || !containsFold(it.Title, filter) {
			continue
		}
		switch {
		case m.IsFloating(it.ID):
			return e.addFloatingLocked(t, p, it.ID)
		case m.PaneIn(m.Current, it.ID) != nil:
			return it, nil
		}
		return e.addFromTabLocked(t, p, it.ID)
	}
	it := &Item{ID: m.NewID(itemPrefix), Kind: KindApp, BundleID: l.App, TitleFilter: filter, OpenURL: link,
		Status: StatusMissing}
	e.nameItemLocked(it)
	it.Title = it.App
	m.Add(it, t, p, -1)
	if w, ok := e.freeWindowLocked(l.App, filter); ok {
		delete(e.unmanaged, w.ID)
		return e.placeLocked(t, p, w, placeQuiet), nil
	}
	if err := e.openItemLocked(it.ID); err != nil {
		m.Remove(it.ID)
		return nil, err
	}
	return nil, nil
}

// launcherTarget returns the title filter of an app launcher's window, and
// the link that opens it in the app: its title and url, or its folder as a
// file link, with its title or else the folder's name, which editors show
// in their windows' titles.
func launcherTarget(l config.Launcher) (filter, link string) {
	if l.URL != "" {
		return l.Title, l.URL
	}
	path := term.FullPath(l.Folder)
	filter = l.Title
	if filter == "" {
		filter = filepath.Base(path)
	}
	return filter, (&url.URL{Scheme: "file", Path: path}).String()
}

// freeWindowLocked returns the first window of an app that's in no tab and
// whose title has filter, ignoring case: one Bench doesn't manage, or a new
// one it hasn't seen yet.
func (e *Engine) freeWindowLocked(bundleID, filter string) (platform.Window, bool) {
	var free []platform.Window
	for _, w := range e.unmanaged {
		if sameApp(w.BundleID, bundleID) {
			free = append(free, w)
		}
	}
	if app, ok := e.runningAppLocked(bundleID); ok {
		free = append(free, e.newWindowsOfLocked(app)...)
	}
	sortByCreated(free)
	for _, w := range free {
		if containsFold(w.Title, filter) {
			return w, true
		}
	}
	return platform.Window{}, false
}

// ChooseFolder shows the folder picker, for the add dialog's New terminal…
// form and Settings ▸ Launchers, starting in a folder written as terminals'
// folders are, such as ~/code. It returns the folder the user chose,
// written the same way, or "" if they cancelled. The picker waits for the
// user, so the lock isn't held meanwhile.
func (e *Engine) ChooseFolder(start string) (string, error) {
	choose := e.opts.ChooseFolder
	if choose == nil {
		return "", errNoFolderPicker
	}
	path, err := choose(term.FullPath(strings.TrimSpace(start)))
	if err != nil {
		e.log.Warn("couldn't show the folder picker", "error", err)
		return "", errors.New("Bench couldn't show the folder picker. Type the folder instead.")
	}
	if path == "" {
		return "", nil
	}
	return term.ShortPath(path), nil
}
