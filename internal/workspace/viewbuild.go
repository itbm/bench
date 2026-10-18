package workspace

import (
	"slices"
	"strconv"
	"strings"

	"github.com/itbm/bench/internal/apps"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// This file builds the View that the frontend draws.

// buildViewLocked returns the current view.
func (e *Engine) buildViewLocked() View {
	m := e.model
	d := e.displayLocked()
	keys := keysOf(e.cfg.Shortcuts)
	v := View{
		Current:        m.Current,
		Display:        e.displayViewLocked(d),
		Items:          map[string]ItemView{},
		Floating:       e.floatingViewsLocked(),
		Mode:           string(m.Mode),
		SizeClass:      string(d.class),
		ScreenWidth:    e.screen.Frame.W,
		Settings:       settingsView(e),
		Keys:           keys,
		Permissions:    PermissionsView{Accessibility: e.trusted, ScreenRecording: e.screenRec},
		Notices:        e.noticesLocked(),
		ConfigWritable: e.writable,
		ConfigPath:     e.cfgPath,
		FirstRun:       e.firstRun,
		OpenAtLogin:    e.openAtLogin,
	}
	if d.tab != nil {
		v.SizeReason = sizeReason(d.class, m.Mode, d.tab.SmallScreens, e.screen.Frame.W)
	}
	for i, t := range m.Tabs {
		v.Tabs = append(v.Tabs, e.tabViewLocked(i, t, keys))
		for _, p := range t.Leaves() {
			for _, id := range p.Items {
				if it := m.Items[id]; it != nil {
					v.Items[id] = e.itemViewLocked(it, i, p.ID)
				}
			}
		}
	}
	for _, id := range m.Floating {
		if it := m.Items[id]; it != nil {
			v.Items[id] = e.itemViewLocked(it, -1, "")
		}
	}
	if e.opts.Demo {
		v.Demo = &DemoView{Screen: platform.DemoFit}
		if ds, ok := e.p.(platform.DemoScreens); ok && ds.DemoScreen() != "" {
			v.Demo.Screen = ds.DemoScreen()
		}
	}
	return v
}

// tabViewLocked returns a tab for the tab strip.
func (e *Engine) tabViewLocked(i int, t *Tab, keys KeysView) TabView {
	tv := TabView{
		Name:         t.Name,
		Preset:       t.Preset,
		SmallScreens: string(t.SmallScreens),
		Root:         layout.Clone(t.Root),
		Focus:        t.Focus,
		Apps:         []string{},
		Panes:        len(t.Leaves()),
	}
	if i < 9 && keys.Tabs != "" {
		tv.Shortcut = keys.Tabs + strconv.Itoa(i+1)
	}
	for _, id := range t.ItemIDs() {
		it := e.model.Items[id]
		if it == nil {
			continue
		}
		tv.Windows++
		if !slices.Contains(tv.Apps, it.App) {
			tv.Apps = append(tv.Apps, it.App)
		}
	}
	return tv
}

// displayViewLocked returns the current tab as drawn.
func (e *Engine) displayViewLocked(d display) DisplayView {
	dv := DisplayView{Area: e.area, Root: d.root, Focus: d.focus, Compact: d.compact, Panes: []PaneView{}}
	if d.root == nil {
		return dv
	}
	if d.compact {
		dv.SplitsDisabled = splitsHidden
	}
	for _, g := range d.panes {
		dv.Panes = append(dv.Panes, PaneView{
			ID: g.node.ID, Rect: g.frame, Body: g.body,
			Items: append([]string{}, g.node.Items...), Active: g.node.Active,
		})
	}
	dv.Dividers = layout.Dividers(d.root, d.area, d.gap)
	if dv.Dividers == nil {
		dv.Dividers = []layout.DividerRect{}
	}
	return dv
}

// itemViewLocked returns an item as the frontend shows it.
func (e *Engine) itemViewLocked(it *Item, tab int, pane string) ItemView {
	status, text := e.statusOf(it)
	iv := ItemView{
		ID: it.ID, Kind: it.Kind, App: it.App, BundleID: it.BundleID, Mono: it.Mono, Title: it.Title,
		Status: status, StatusText: text, Tab: tab, Pane: pane,
	}
	switch {
	case it.Kind == KindBuiltin:
		iv.Folder = it.Folder
	case it.VSCode != "":
		iv.Folder = it.VSCode
	case it.Kind == KindWeb:
		iv.URL = it.URL
	}
	return iv
}

// statusOf works out an item's status and its explanation.
func (e *Engine) statusOf(it *Item) (string, string) {
	if it.Kind == KindWeb && it.VSCode != "" && !it.OwnWindow {
		if s, text := e.vscodeStatus(); s != StatusReady {
			return s, text
		}
	}
	if it.Kind != KindApp {
		if it.Status == "" {
			return StatusReady, ""
		}
		return it.Status, it.StatusText
	}
	if it.WindowID == 0 {
		key := strings.ToLower(it.BundleID)
		switch {
		case e.launchingNow(it.BundleID):
			return StatusLaunching, openingText(it.App)
		case e.openFail[key]:
			return StatusMissing, couldNotOpenText(it.App)
		}
		if _, running := e.running[key]; running {
			return StatusMissing, noWindowText(it.App)
		}
		return StatusMissing, notRunningText(it.App)
	}
	if it.gone {
		return StatusMissing, noWindowText(it.App)
	}
	if it.Undocked {
		return StatusUndocked, undockedText
	}
	if it.Status == "" || it.Status == StatusMissing || it.Status == StatusLaunching {
		return StatusReady, ""
	}
	return it.Status, it.StatusText
}

// floatingViewsLocked lists what's not in a tab: popped-out items, then the
// standard windows Bench isn't managing, in the order they were created.
func (e *Engine) floatingViewsLocked() []ItemView {
	out := []ItemView{}
	for _, id := range e.model.Floating {
		if it := e.model.Items[id]; it != nil {
			out = append(out, e.itemViewLocked(it, -1, ""))
		}
	}
	ws := make([]platform.Window, 0, len(e.unmanaged))
	for _, w := range e.unmanaged {
		ws = append(ws, w)
	}
	sortByCreated(ws)
	for _, w := range ws {
		name := w.App
		if name == "" {
			name = e.appName(w.BundleID)
		}
		name = apps.DisplayName(name)
		out = append(out, ItemView{
			ID: floatingID(w.ID), Kind: KindApp, App: name, BundleID: w.BundleID, Mono: apps.Mono(name),
			Title: w.Title, Status: StatusReady, Tab: -1,
		})
	}
	return out
}

// noticesLocked returns the status bar's notices: the config's problems,
// missing permission, then each source's, in the order they first came.
func (e *Engine) noticesLocked() []string {
	out := append([]string{}, e.problems...)
	if !e.trusted {
		out = append(out, accessibilityNotice)
	}
	for _, source := range e.noticeOrder {
		out = append(out, e.notices[source]...)
	}
	return out
}

// settingsView returns [settings] for the layout editor.
func settingsView(e *Engine) SettingsView {
	s := e.cfg.Settings
	return SettingsView{
		CompactBelow:    s.CompactBelow,
		Gap:             s.Gap,
		OpenMissingApps: s.OpenMissingApps,
		HideOtherTabs:   s.HideOtherTabs,
		HideOtherApps:   s.HideOtherApps,
		NewWindows:      s.NewWindows,
		FollowFocus:     s.FollowFocus,
		LivePreviews:    s.LivePreviews,
		LaunchTimeout:   s.LaunchTimeout.String(),
	}
}

// keysOf returns the configured shortcuts in display form. A shortcut that's
// turned off, or that has a problem, is "".
func keysOf(c shortcuts.Config) KeysView {
	bindings, _ := shortcuts.Bindings(c)
	bound := map[string]bool{}
	for _, b := range bindings {
		bound[b.Field] = true
	}
	show := func(field, value string) string {
		if !bound["shortcuts."+field] {
			return ""
		}
		return shortcuts.Display(value)
	}
	return KeysView{
		Tabs:        show("tab_modifiers", c.TabModifiers),
		NextTab:     show("next_tab", c.NextTab),
		PreviousTab: show("previous_tab", c.PreviousTab),
		Jump:        show("jump", c.Jump),
		Rearrange:   show("rearrange", c.Rearrange),
		FocusPane:   show("focus_pane_modifiers", c.FocusPaneModifiers),
		MoveWindow:  show("move_window_modifiers", c.MoveWindowModifiers),
		NextWindow:  show("next_window", c.NextWindow),
	}
}
