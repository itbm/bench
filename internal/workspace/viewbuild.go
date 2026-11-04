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
		Current:          m.Current,
		LastTab:          e.lastTabLocked(),
		Display:          e.displayViewLocked(d),
		Items:            map[string]ItemView{},
		Floating:         e.floatingViewsLocked(),
		Front:            e.frontLocked(),
		Mode:             string(m.Mode),
		SizeClass:        string(d.class),
		ScreenWidth:      e.screen.Frame.W,
		WindowWidth:      e.contentWidthLocked(),
		Settings:         settingsView(e),
		Density:          e.densityLocked(),
		TitleBar:         e.titleBarLocked(),
		SystemScrollBars: e.scrollBars,
		Keys:             keys,
		Shortcuts:        e.shortcutViewsLocked(),
		Permissions:      e.permissionsLocked(),
		Notices:          e.noticesLocked(),
		ConfigWritable:   e.writable,
		ConfigPath:       e.cfgPath,
		FirstRun:         e.firstRun,
		OpenAtLogin:      e.openAtLogin,
		LoginApproval:    e.loginApproval,
		InstallsUpdates:  e.opts.Updater != nil,
		ClosedTabs:       e.closedNamesLocked(),
	}
	if d.tab != nil {
		v.SizeReason = sizeReason(d.class, m.Mode, d.tab.SmallScreens, v.WindowWidth)
	}
	for kind, size := range e.dialogs {
		if v.Dialogs == nil {
			v.Dialogs = map[string]DialogSize{}
		}
		v.Dialogs[kind] = DialogSize(size)
	}
	bindings, _ := shortcuts.Bindings(e.cfg.Shortcuts)
	send := byTab(bindings, shortcuts.ActionSendToTab)
	for i, t := range m.Tabs {
		tv := e.tabViewLocked(i, t, keys)
		if i < len(send) {
			tv.SendShortcut = send[i].Display
		}
		v.Tabs = append(v.Tabs, tv)
	}
	for _, it := range m.orderedItems() {
		if ti, p := m.Where(it.ID); p != nil {
			iv := e.itemViewLocked(it, ti, p.ID)
			if tabs := m.TabsOf(it.ID); len(tabs) > 1 {
				iv.Tabs = tabs
			}
			v.Items[it.ID] = iv
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
	tv.Shortcut = tabShortcutText(keys, i)
	for _, id := range t.ItemIDs() {
		it := e.model.Items[id]
		if it == nil || e.hiddenLocked(it) {
			continue
		}
		tv.Windows++
		if !slices.Contains(tv.Apps, it.App) {
			tv.Apps = append(tv.Apps, it.App)
		}
		if it.attention != "" {
			tv.Attention++
		}
		if it.web.Camera != platform.CaptureNone {
			tv.Camera++
		}
		if it.web.Microphone != platform.CaptureNone {
			tv.Microphone++
		}
		if it.web.Camera == platform.CaptureActive {
			tv.CameraLive = true
		}
		if it.web.Microphone == platform.CaptureActive {
			tv.MicrophoneLive = true
		}
	}
	return tv
}

// tabShortcutText returns the shortcut of the tab at index i in display
// form, such as "⌃⌥1": tabs get one by their place, up to the ninth. It's ""
// after the ninth, or if tab_modifiers is off.
func tabShortcutText(keys KeysView, i int) string {
	if i < 9 && keys.Tabs != "" {
		return keys.Tabs + strconv.Itoa(i+1)
	}
	return ""
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
	if d.zoomed {
		dv.Zoomed = d.root.ID
	}
	dv.Arrange = e.arrangeViewLocked(d.tab)
	for _, g := range d.panes {
		dv.Panes = append(dv.Panes, PaneView{
			ID: g.node.ID, Rect: g.frame, Body: g.body, Header: g.header,
			Items: append([]string{}, g.node.Items...), Active: g.node.Active,
			Remembered: e.rememberedLocked(d.tab, g.node.ID),
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
		ID: it.ID, Kind: it.Kind, App: it.App, BundleID: it.BundleID, Mono: it.Mono, Title: it.shownTitle(),
		Status: status, StatusText: text, Tab: tab, Pane: pane,
		NoWindow:  it.Kind == KindApp && it.WindowID == 0,
		Hidden:    tab >= 0 && e.hiddenLocked(it),
		Attention: it.attention,
		Used:      it.Used,
		Icon:      e.itemIconLocked(it),
	}
	iv.CanOpen = iv.NoWindow && !e.notFound[strings.ToLower(it.BundleID)]
	if it.Label != "" {
		iv.Label, iv.AutoTitle = it.Label, it.Title
	}
	switch {
	case it.Kind == KindBuiltin:
		iv.Folder, iv.Here = it.Folder, hereOf(it)
		iv.Ended = it.ended
	case it.VSCode != "":
		iv.Folder = it.VSCode
	case it.Kind == KindWeb:
		iv.URL = it.URL
		iv.Host, iv.Site, iv.OffSite, iv.NotSecure = it.host, it.site, it.offSite, it.notSecure
	}
	if it.Kind == KindWeb {
		iv.Zoom = zoomPercent(it.Zoom)
		iv.CanGoBack, iv.CanGoForward = it.web.CanGoBack, it.web.CanGoForward
		iv.Camera, iv.Microphone = string(it.web.Camera), string(it.web.Microphone)
		iv.Finding = it.finding && !it.OwnWindow
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
		if e.notFound[key] {
			return StatusMissing, notInstalledText(it.App)
		}
		return StatusMissing, notRunningText(it.App)
	}
	if it.gone {
		return StatusMissing, noWindowText(it.App)
	}
	if it.Undocked {
		return StatusUndocked, undockedText
	}
	if it.minimised {
		return StatusMinimised, minimisedText
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
			Icon: e.appIconLocked(w.BundleID), Title: w.Title, Status: StatusReady, Tab: -1,
		})
	}
	return out
}

// noticesLocked returns the status bar's notices: config.toml's and
// state.json's, with their buttons, missing permission, then each source's,
// in the order they first came.
func (e *Engine) noticesLocked() []NoticeView {
	out := append([]NoticeView{}, e.configNoticesLocked()...)
	out = append(out, e.setAsideNoticesLocked()...)
	if !e.trusted {
		out = append(out, e.accessibilityNoticeLocked())
	}
	for _, source := range e.noticeOrder {
		out = append(out, e.notices[source]...)
	}
	return out
}

// settingsView returns [settings] for the Settings dialog.
func settingsView(e *Engine) SettingsView {
	s := e.cfg.Settings
	return SettingsView{
		FillScreen:               s.FillScreen,
		MenuBar:                  s.MenuBar,
		ShowOnTabShortcut:        s.ShowOnTabShortcut,
		CheckForUpdates:          s.CheckForUpdates,
		ConfirmQuitBusyTerminals: s.ConfirmQuitBusyTerminals,
		ConfirmClose:             s.ConfirmClose,
		CompactBelow:             s.CompactBelow,
		Gap:                      s.Gap,
		ShowStatusBar:            s.ShowStatusBar,
		TabShortcutHints:         s.TabShortcutHints,
		ScrollBars:               s.ScrollBars,
		Appearance:               s.Appearance,
		Density:                  s.Density,
		PaneHeaders:              s.PaneHeaders,
		OpenMissingApps:          s.OpenMissingApps,
		HideOtherTabs:            s.HideOtherTabs,
		HideOtherApps:            s.HideOtherApps,
		NewWindows:               s.NewWindows,
		FollowFocus:              s.FollowFocus,
		DragToAdd:                s.DragToAdd,
		LivePreviews:             s.LivePreviews,
		TerminalFontSize:         s.TerminalFontSize,
		TerminalOptionAsMeta:     s.TerminalOptionAsMeta,
		NotifyTerminals:          s.NotifyTerminals,
		KeepTerminalOutput:       s.KeepTerminalOutput,
		LaunchTimeout:            s.LaunchTimeout.String(),
		Launchers:                e.launcherViewsLocked(),
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
		LastTab:     show("last_tab", c.LastTab),
		Jump:        show("jump", c.Jump),
		Rearrange:   show("rearrange", c.Rearrange),
		FocusPane:   show("focus_pane_modifiers", c.FocusPaneModifiers),
		MoveWindow:  show("move_window_modifiers", c.MoveWindowModifiers),
		SendToTab:   show("send_to_tab_modifiers", c.SendToTabModifiers),
		NextWindow:  show("next_window", c.NextWindow),
		ZoomPane:    show("zoom_pane", c.ZoomPane),

		GrowPane:       show("grow_pane", c.GrowPane),
		ShrinkPane:     show("shrink_pane", c.ShrinkPane),
		EqualPanes:     show("equal_panes", c.EqualPanes),
		NextLayout:     show("next_layout", c.NextLayout),
		PreviousLayout: show("previous_layout", c.PreviousLayout),
		AddFrontWindow: show("add_front_window", c.AddFrontWindow),
	}
}
