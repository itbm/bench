package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// This file keeps Bench's config and config.toml in step: it notices when
// the file has been changed outside Bench, reloads it, writes Bench's config
// over it, swaps in its backup, and opens it for the user to edit. Settings
// and Tab properties save it (settings.go, editor.go), through saveConfig
// (persist.go), which won't write over changes made outside Bench.

// outsideChange says whether config.toml has changed outside Bench since
// Bench last read or wrote it.
type outsideChange int

const (
	// outsideNone: it hasn't, as far as Bench knows.
	outsideNone outsideChange = iota
	// outsideSeen: it has, as Bench found when it became active. Bench has
	// no changes of its own waiting to be saved.
	outsideSeen
	// outsideKept: a save found that it had, and didn't write over it, so
	// Bench's change is applied but not saved, until the user reloads the
	// file or writes Bench's config over it.
	outsideKept
)

// Why saveConfig didn't save config.toml, for the toast after a change.
const (
	notSavedProblems = "config.toml has problems"
	notSavedChanged  = "config.toml was changed outside Bench"
)

// The config's notices.
const (
	outsideSeenText = "config.toml was changed outside Bench. Reload it to use the changes."
	outsideKeptText = "config.toml was changed outside Bench, so Bench didn't save its own changes over it. " +
		"Reload the file to use it instead, or overwrite it with Bench's changes."
	outsideKeptHeld = " Until then, Bench doesn't save the arrangement either."
)

// The config's notice buttons: each calls a bound method.
var (
	actionOpenConfig = NoticeAction{Label: "Open config.toml", Method: "OpenConfig"}
	actionShowConfig = NoticeAction{Label: "Show in Finder", Method: "ShowConfigInFinder"}
	actionReload     = NoticeAction{Label: "Reload", Method: "ReloadConfig"}
	actionUseBackup  = NoticeAction{Label: "Use backup", Method: "UseConfigBackup"}
	actionOverwrite  = NoticeAction{Label: "Overwrite", Method: "OverwriteConfig"}
)

// What Bench runs with while config.toml has problems, for problemsLead.
const (
	usingGood    = "the last config that worked"
	usingDefault = "the default config"
	usingOwn     = "the config it had"
)

// appliedText is the toast after a change that Bench applied but didn't
// save, saying why (notSavedProblems or notSavedChanged).
func appliedText(reason string) string { return "Applied for now, not saved: " + reason }

// notSavedText adds why a change wasn't saved to a toast that says what the
// change was.
func notSavedText(reason string) string { return "Not saved: " + reason + "." }

// problemsLead is the first of the config's notices while config.toml has
// problems, or Bench can't read it (cantRead), saying what Bench runs with
// instead (usingGood, say), and whether it saves the arrangement meanwhile.
func problemsLead(using string, cantRead, savesState bool) string {
	lead := "config.toml has problems, so Bench is using " + using + ". Until they're fixed, "
	if cantRead {
		lead = "Bench couldn't read config.toml, so it's using " + using + ". Until it can, "
	}
	if savesState {
		return lead + "it doesn't save changes to config.toml, only the arrangement."
	}
	return lead + "it doesn't save changes to config.toml or the arrangement."
}

// cantReadText is the notice that says why Bench couldn't read config.toml.
func cantReadText(err error) string { return "config.toml: " + err.Error() }

// problemTexts returns config.toml's problems as notices.
func problemTexts(problems []config.Problem) []string {
	out := make([]string, len(problems))
	for i, p := range problems {
		out[i] = "config.toml: " + p.Error()
	}
	return out
}

// setProblemsLocked records that config.toml has problems, or can't be
// read (cantRead), while Bench runs with the config that using names: Bench
// stops saving it, and state.json too unless the model follows a config
// that loaded without problems (followsGood). The config's notices say so,
// then list the problems, offering the backup if it loads.
func (e *Engine) setProblemsLocked(using string, cantRead bool, problems []string) {
	e.writable = false
	e.using, e.cantRead = using, cantRead
	e.problems = problems
	e.backupOK = config.BackupLoads(e.cfgPath)
	e.outside = outsideNone
}

// clearProblemsLocked records that config.toml has loaded without problems.
func (e *Engine) clearProblemsLocked() {
	e.writable, e.followsGood = true, false
	e.using, e.cantRead, e.problems, e.backupOK = "", false, nil, false
	e.outside = outsideNone
}

// configNoticesLocked returns the notices about config.toml, with their
// buttons: its problems, or that it was changed outside Bench.
func (e *Engine) configNoticesLocked() []NoticeView {
	var out []NoticeView
	if e.using != "" {
		actions := []NoticeAction{actionOpenConfig, actionShowConfig, actionReload}
		if e.backupOK {
			actions = append(actions, actionUseBackup)
		}
		out = append(out, NoticeView{Text: problemsLead(e.using, e.cantRead, e.persistent()), Actions: actions})
		for _, p := range e.problems {
			out = append(out, NoticeView{Text: p})
		}
	}
	switch e.outside {
	case outsideSeen:
		out = append(out, NoticeView{Text: outsideSeenText, Actions: []NoticeAction{actionReload}})
	case outsideKept:
		text := outsideKeptText
		if e.stateHeld {
			text += outsideKeptHeld
		}
		out = append(out, NoticeView{Text: text, Actions: []NoticeAction{actionReload, actionOverwrite}})
	}
	return out
}

// checkConfigLocked checks, once the lock is released, whether config.toml
// has changed outside Bench, as when the user has edited it and comes back
// to Bench or an app in a tab, or opens a menu or dialog. The notice then
// offers to reload it.
func (e *Engine) checkConfigLocked() {
	if e.opts.Headless || !e.writable || e.outside == outsideKept {
		return
	}
	e.later(e.checkConfigFile)
}

// checkConfigFile compares config.toml with the Sum Bench recorded when it
// last read or wrote it, and notes whether it has changed, or has been put
// back as it was. While a save or a reload holds cfgMu, it leaves the file
// to them: they check it themselves, and waiting for them could hold up the
// event loop, which Close waits for while it holds cfgMu.
func (e *Engine) checkConfigFile() {
	if !e.cfgMu.TryLock() {
		return
	}
	defer e.cfgMu.Unlock()
	sum, err := config.FileSum(e.cfgPath)
	if err != nil {
		e.log.Warn("couldn't check config.toml", "error", err)
		return
	}
	e.lock()
	defer e.unlock()
	if e.closed || !e.writable {
		return
	}
	switch {
	case sum != e.cfgSum && e.outside == outsideNone:
		e.outside = outsideSeen
		e.touch()
	case sum == e.cfgSum && e.outside == outsideSeen:
		e.outside = outsideNone
		e.touch()
	}
}

// ReloadConfig reads config.toml again and applies it, as Bench would at
// startup, but keeping the arrangement: for edits made to the file while
// Bench runs, or once its problems are fixed. Changes Bench couldn't save
// over the file are dropped. If the file has problems, Bench keeps the
// config it has and says so; if it isn't there, Bench writes its config
// there.
func (e *Engine) ReloadConfig() error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	cfg, problems, data, err := config.Read(e.cfgPath)
	if errors.Is(err, fs.ErrNotExist) {
		return e.rewriteConfig()
	}
	e.lock()
	if e.closed {
		e.unlock()
		return errNotRunning
	}
	if err != nil || len(problems) > 0 {
		// The model follows the config Bench keeps, if that loaded without
		// problems, so state.json can go on following it.
		e.followsGood = e.writable || e.followsGood
	}
	if err != nil {
		e.setProblemsLocked(usingOwn, true, []string{cantReadText(err)})
		e.toastLocked("Bench couldn't read config.toml, so it kept the config it had.")
		e.touch()
		e.unlock()
		return nil
	}
	if len(problems) > 0 {
		e.cfgSum = config.SumOf(data)
		e.setProblemsLocked(usingOwn, false, problemTexts(problems))
		where := "The status bar lists them."
		if !e.cfg.Settings.ShowStatusBar {
			where = "The notices in the title bar list them."
		}
		e.toastLocked("config.toml has problems, so Bench kept the config it had. " + where)
		e.touch()
		e.unlock()
		return nil
	}
	e.useConfigLocked(cfg, data)
	e.toastLocked("Reloaded config.toml")
	e.unlock()
	e.keepGood(data)
	return nil
}

// rewriteConfig writes Bench's config to config.toml, which isn't there,
// for ReloadConfig, as Bench writes the default config on first run. The
// caller holds cfgMu.
func (e *Engine) rewriteConfig() error {
	e.lock()
	if e.closed {
		e.unlock()
		return errNotRunning
	}
	saved := e.cfg.Clone()
	e.unlock()

	data, err := config.SaveIf(e.cfgPath, saved, config.Sum{})
	if err != nil {
		return fmt.Errorf("config.toml isn't there, and Bench couldn't save its config there: %w", err)
	}
	e.lock()
	e.cfgSum = config.SumOf(data)
	e.clearProblemsLocked()
	e.stateHeld = false
	e.clearHeldNoticeLocked()
	// As useConfigLocked: Bench is about to save over state.json.
	e.revealSetAsideLocked()
	e.changed()
	e.toastLocked("config.toml wasn't there, so Bench saved the config it's using there.")
	e.unlock()
	e.keepGood(data)
	return nil
}

// OverwriteConfig writes Bench's config over config.toml, which was changed
// outside Bench, once the user has chosen to (see saveConfig). The file's
// version goes to config.toml.bak, as with every save.
func (e *Engine) OverwriteConfig() error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	switch {
	case e.closed:
		e.unlock()
		return errNotRunning
	case !e.writable:
		e.unlock()
		return errConfigProblems
	}
	saved := e.cfg.Clone()
	e.unlock()

	if err := config.Save(e.cfgPath, saved); err != nil {
		return fmt.Errorf("Bench couldn't save config.toml: %w", err)
	}
	data := config.Format(saved)
	e.cfgSum = config.SumOf(data)
	e.keepGood(data)
	e.lock()
	e.outside = outsideNone
	e.touch()
	e.unlock()
	e.saveHeldState()
	e.em.Toast("Saved Bench's changes to config.toml. The version changed outside Bench is in config.toml.bak.")
	return nil
}

// UseConfigBackup puts config.toml.bak in place of config.toml, which has
// problems, if the backup loads without any, and applies it as ReloadConfig
// would. The file with problems becomes config.toml.bak, so the user can
// still fix it and swap it back.
func (e *Engine) UseConfigBackup() error {
	e.cfgMu.Lock()
	defer e.cfgMu.Unlock()
	e.lock()
	switch {
	case e.closed:
		e.unlock()
		return errNotRunning
	case e.writable:
		e.unlock()
		return errNoConfigProblems
	}
	e.unlock()

	cfg, data, err := config.UseBackup(e.cfgPath)
	if err != nil {
		e.lock()
		e.backupOK = config.BackupLoads(e.cfgPath)
		e.touch()
		e.unlock()
		return fmt.Errorf("Bench couldn't use the backup: %v.", err)
	}
	e.lock()
	e.useConfigLocked(cfg, data)
	e.toastLocked("Using config.toml.bak. The file that had problems is config.toml.bak now.")
	e.unlock()
	e.keepGood(data)
	return nil
}

// OpenConfig opens config.toml in the default app for plain text, such as
// TextEdit.
func (e *Engine) OpenConfig() error {
	return e.openConfigWith(e.p.OpenTextFile, "Bench couldn't open config.toml.")
}

// ShowConfigInFinder shows config.toml in the Finder, selected.
func (e *Engine) ShowConfigInFinder() error {
	return e.openConfigWith(e.p.ShowInFinder, "Bench couldn't show config.toml in the Finder.")
}

// openConfigWith hands config.toml to the platform, which the browser
// preview refuses.
func (e *Engine) openConfigWith(open func(path string) error, failed string) error {
	err := open(e.cfgPath)
	switch {
	case errors.Is(err, platform.ErrUnsupported):
		return errCantOpenFiles
	case err != nil:
		e.log.Warn("couldn't open config.toml", "error", err)
		return errors.New(failed)
	}
	return nil
}

// useConfigLocked runs with a config that has loaded without problems from
// config.toml, whose content is data: it applies it to the model and the
// settings, and saves state.json again, now that the model and the file
// agree. The shell registers the shortcuts again if they changed. The
// caller holds cfgMu, and keeps data as the last good config once the lock
// is released.
func (e *Engine) useConfigLocked(cfg *config.Config, data []byte) {
	old := e.cfg
	// The default config's tabs, which Bench may have run with while the
	// file had problems, aren't the user's.
	own := e.writable || e.followsGood
	e.cfg = cfg
	e.cfgSum = config.SumOf(data)
	e.clearProblemsLocked()
	e.applyConfigLocked(old, cfg, own)
	// The model's tabs are the file's now, so state.json can follow them.
	e.stateHeld = false
	e.clearHeldNoticeLocked()
	e.revealSetAsideLocked()
	if cfg.Settings.LivePreviews && !old.Settings.LivePreviews {
		e.screenRec = e.p.ScreenRecordingAllowed(true)
	}
	if cfg.Shortcuts != old.Shortcuts {
		em := e.em
		e.later(em.ShortcutsChanged)
	}
	e.changed()
}

// clearHeldNoticeLocked takes away the notice that state.json is held, once
// it no longer is.
func (e *Engine) clearHeldNoticeLocked() {
	if slices.Equal(e.notices[sourceState], []string{stateHeldText}) {
		e.setNoticesLocked(sourceState, nil)
	}
}

// applyConfigLocked brings the model into line with a config reloaded from
// config.toml, in place of old, keeping the arrangement as startup keeps
// state.json's (see syncEntries):
//
//   - a tab is the same tab if it has the same name, ignoring case, or if
//     it's the only tab whose name changed, with nothing else about the
//     tabs changed (see config.MatchTabs); its entries' edits apply to its
//     items as at startup, and its layout changes only if the file's does;
//   - a tab that's new to the file is built from it, and a tab that's no
//     longer in it is deleted, as Delete tab does; if the tabs are the
//     user's (own), not the default config's, its arrangement is set aside,
//     for the notice and Restore previous arrangement, as startup sets
//     aside the arrangement of tabs it drops;
//   - the tabs take the file's order, and the current tab stays current if
//     it's still there;
//   - a window whose entry is gone, or whose tab is, goes where an entry
//     claims it, as at startup, or else to "Not in a tab";
//   - a page or terminal whose entry is gone, or whose tab is, goes on as
//     the item of a new entry for the same address, or folder and command,
//     if there's one; else a page or terminal whose entry is gone closes,
//     and one whose tab is gone moves to "Not in a tab";
//   - a page whose address or profile changed loads again, but a terminal
//     whose command changed runs it next time it starts;
//   - an app's windows that no tab had go where a new entry for the app
//     claims them.
//
// Settings and rules apply from then on, as they're read when needed.
func (e *Engine) applyConfigLocked(old, cfg *config.Config, own bool) {
	m := e.model
	before := maps.Clone(m.Items)
	wasFloating := slices.Clone(m.Floating)
	pages := map[string]Item{}
	for id, it := range m.Items {
		if it.Kind == KindWeb {
			pages[id] = *it
		}
	}
	current, was := m.CurrentTab(), m.Current
	showOthers := old.Settings.HideOtherTabs && !cfg.Settings.HideOtherTabs

	// Match the file's tabs with the model's by name, or as renamed. The
	// arrangement of those the file no longer has is kept for the notice.
	names := make([]string, len(m.Tabs))
	for ti, t := range m.Tabs {
		names[ti] = t.Name
	}
	from := config.MatchTabs(names, cfg.TabNames())
	dropped := config.Dropped(from, len(m.Tabs))
	var st *config.State
	if len(dropped) > 0 && own {
		st = m.State()
	}

	// Bring each match's items into line with its entries.
	order := make([]*Tab, len(cfg.Tabs))
	given := map[string]Item{}
	tabs := slices.Clone(m.Tabs)
	for ci, ti := range from {
		if ti < 0 {
			continue
		}
		ct, t := cfg.Tabs[ci], m.Tabs[ti]
		order[ci] = t
		var saved []string
		oct, ok := oldTab(old, ti, t.Name)
		if ok {
			saved = entryKeys(oct)
			if ct.Layout != oct.Layout || !slices.Equal(ct.Sizes, oct.Sizes) {
				if err := m.ApplyPreset(ti, ct.Layout, ct.Sizes); err != nil {
					e.log.Warn("couldn't apply a reloaded tab's layout", "tab", ct.Name, "error", err)
				}
			}
		}
		t.Name, t.SmallScreens = ct.Name, smallScreensOf(ct.SmallScreens)
		m.syncEntries(ti, ct, saved, e.appName, given)
	}
	for ci, ct := range cfg.Tabs {
		if order[ci] != nil {
			continue
		}
		t, err := m.addConfigTab(ct, e.appName)
		if err != nil {
			e.log.Warn("couldn't add a reloaded tab", "tab", ct.Name, "error", err)
			continue
		}
		order[ci] = t
	}

	// Delete the tabs the file no longer has, last first, as Delete tab
	// does; the file has at least one tab, now in the model too. Their
	// windows find places below.
	keep := func(it *Item) bool { return it.Kind != KindApp }
	for _, ti := range slices.Backward(dropped) {
		i := m.IndexOf(tabs[ti])
		kept, _, err := m.DeleteTab(i, keep)
		if err != nil {
			e.log.Warn("couldn't delete a tab that config.toml no longer has", "tab", tabs[ti].Name, "error", err)
			continue
		}
		for _, id := range kept {
			e.leaveTabLocked(m.Items[id])
		}
	}
	m.Tabs = slices.DeleteFunc(order, func(t *Tab) bool { return t == nil })
	if i := m.IndexOf(current); i >= 0 {
		m.Current = i
	} else {
		// As when the current tab is deleted: the tab in its place.
		m.Current = max(min(was, len(m.Tabs)-1), 0)
	}

	e.settleReloadedItemsLocked(before, wasFloating, pages)
	if len(dropped) > 0 && own {
		e.noteReloadDroppedLocked(old, st, dropped, wasFloating)
	}
	if m.CurrentTab() != current {
		e.switchLocked(m.Current, switchOpts{focus: true})
		return
	}
	e.arrangeLocked(arrangeOpts{showOtherTabs: showOthers})
}

// oldTab returns the config tab that model tab ti, called name, was built
// from: the config's tab at the same index, which it is unless the two have
// got out of step, or else the one with the same name.
func oldTab(old *config.Config, ti int, name string) (config.Tab, bool) {
	if ti < len(old.Tabs) && strings.EqualFold(old.Tabs[ti].Name, name) {
		return old.Tabs[ti], true
	}
	for _, ct := range old.Tabs {
		if strings.EqualFold(ct.Name, name) {
			return ct, true
		}
	}
	return config.Tab{}, false
}

// settleReloadedItemsLocked deals with the items that applyConfigLocked
// took out of the tabs, as startup would. before holds the model's items
// before the reload, wasFloating those that were floating, and pages the
// pages as they were. A page or terminal goes on as the item of a new entry
// for the same address, or folder and command, if there's one. The
// windows, and those that no tab had of apps that new entries list, go
// where the entries claim them. What's left ends, or stays in Floating, as
// endRemovedItemsLocked says. Pages whose address changed reload.
func (e *Engine) settleReloadedItemsLocked(before map[string]*Item, wasFloating []string, pages map[string]Item) {
	m := e.model
	e.reuseLeftItemsLocked(before, wasFloating)
	for id, was := range pages {
		it := m.Items[id]
		if it != nil && !it.OwnWindow && (it.Home != was.Home || it.Profile != was.Profile || it.VSCode != was.VSCode) {
			e.closeWebViewLocked(it)
		}
	}
	freed := map[platform.WindowID]bool{}
	for _, id := range slices.Sorted(maps.Keys(before)) {
		it := before[id]
		if m.Items[id] != nil || it.Kind != KindApp || it.WindowID == 0 || m.ItemByWindow(it.WindowID) != nil {
			continue
		}
		e.unmanaged[it.WindowID] = platform.Window{
			ID: it.WindowID, PID: it.pid, BundleID: it.BundleID, App: it.App, Title: it.Title,
			Subrole: platform.SubroleStandard, Created: it.Created,
		}
		freed[it.WindowID] = true
	}
	var apps []string
	for id, it := range m.Items {
		if before[id] == nil && it.Kind == KindApp && !slices.ContainsFunc(apps, func(a string) bool { return sameApp(a, it.BundleID) }) {
			apps = append(apps, it.BundleID)
		}
	}
	e.claimUnmanagedLocked(apps, freed)
	e.endRemovedItemsLocked(before)
}

// reuseLeftItemsLocked lets the pages and terminals that a reload took out
// of every tab go on as the items of new entries for the same address, or
// folder and command, in their place: the page stays loaded, and the
// terminal's session goes on, rather than closing and starting again. A
// page goes on only with the same profile. before and wasFloating are as
// for settleReloadedItemsLocked.
func (e *Engine) reuseLeftItemsLocked(before map[string]*Item, wasFloating []string) {
	m := e.model
	left := map[string][]*Item{}
	for _, id := range slices.Sorted(maps.Keys(before)) {
		it := before[id]
		if it.Kind == KindApp || slices.Contains(wasFloating, id) || len(m.TabsOf(id)) > 0 {
			continue
		}
		if keys := itemKeys(it); len(keys) > 0 {
			left[keys[0]] = append(left[keys[0]], it)
		}
	}
	for _, it := range m.orderedItems() {
		keys := itemKeys(it)
		if before[it.ID] != nil || it.Kind == KindApp || len(keys) == 0 {
			continue
		}
		i := slices.IndexFunc(left[keys[0]], func(was *Item) bool { return was.Profile == it.Profile })
		ci, _, ok := configItem(it)
		if i < 0 || !ok {
			continue
		}
		was := left[keys[0]][i]
		left[keys[0]] = slices.Delete(left[keys[0]], i, i+1)
		applyEntry(was, ci, e.appName)
		used := was.Used
		m.Floating = slices.DeleteFunc(m.Floating, func(id string) bool { return id == was.ID })
		m.Replace(it.ID, was)
		was.Used = used
	}
}

// endRemovedItemsLocked ends what Bench ran for the items that the model,
// which held before before, no longer has: a page's web view closes, and a
// terminal's session ends. A window stays, in Floating, put back on screen,
// unless another item has it now.
func (e *Engine) endRemovedItemsLocked(before map[string]*Item) {
	m := e.model
	for _, id := range slices.Sorted(maps.Keys(before)) {
		it := before[id]
		if m.Items[id] != nil {
			continue
		}
		switch it.Kind {
		case KindApp:
			if it.WindowID != 0 && m.ItemByWindow(it.WindowID) == nil {
				delete(e.unmanaged, it.WindowID)
				m.Float(it)
				e.releaseWindowLocked(it)
			}
		case KindWeb:
			e.closeWebViewLocked(it)
		case KindBuiltin:
			e.terms.Close(it.ID)
			delete(e.termOn, it.ID)
		}
	}
}

// claimUnmanagedLocked puts the windows of apps that Bench doesn't manage
// where the config's entries claim them, as the startup scan does: those of
// apps whose entries are new, and those in freed, which left their tabs.
// Windows that a rule floats stay floating.
func (e *Engine) claimUnmanagedLocked(apps []string, freed map[platform.WindowID]bool) {
	var ws []platform.Window
	for _, w := range e.unmanaged {
		if freed[w.ID] || slices.ContainsFunc(apps, func(a string) bool { return sameApp(a, w.BundleID) }) {
			ws = append(ws, w)
		}
	}
	sortByCreated(ws)
	for _, w := range ws {
		if float, _ := matchRule(e.cfg.Rules, w.BundleID, w.Title); float {
			continue
		}
		delete(e.unmanaged, w.ID)
		if !e.placeByClaimLocked(w) {
			e.unmanaged[w.ID] = w
		}
	}
}
