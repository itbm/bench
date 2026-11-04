//go:build darwin || server

package ui

import (
	"errors"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/workspace"
)

// engine is what the shell needs from workspace.Engine. Tests use a fake.
type engine interface {
	bound

	Start()
	Close() error
	ConfigCopy() workspace.ConfigInfo
	ReportShortcuts(r workspace.ShortcutReport)
	Shortcut(action, arg string)
	MoveCurrentTab(step int) error
	ToggleZoom() error
	TerminalWindowClosed(itemID string)
	WindowStart() workspace.WindowStart
	AskToQuit() bool
	AskToQuitNow() (quit, answered bool)
	AskToCloseTerminal(itemID string) bool
	ProblemReport() workspace.ProblemReport
	CheckForUpdates() error
	FoundUpdate(version string)
	UpdateFinished()
}

// bound is the part of workspace.Engine that Service exposes to the
// frontend, one method for each bound method.
type bound interface {
	Ready() (workspace.View, error)
	SetPaneArea(area layout.Rect) error
	SwitchTab(index int) error
	StepTab(step int) error
	LastTab() error
	StepHold(step int) error
	CancelHold() error
	SetMode(mode string) error
	FocusPane(paneID string) error
	FocusUI() error
	ShowItem(paneID, itemID string) error
	MoveItem(itemID string, tab int, paneID, zone string) error
	MoveItemAt(itemID string, tab int, paneID string, index int) error
	ShowSideBySide(itemID string) error
	MoveItemToTab(itemID string, tab int) error
	SendToTab(index int) error
	SendToLastTab() error
	AddItemToTab(itemID string, tab int) error
	RemoveItemFromTab(itemID string, tab int) error
	MoveItemToNewTab(itemID string, also bool) error
	RenameItem(itemID, label string) error
	FocusDirection(dir string) error
	MoveDirection(dir string) error
	CycleSubtab(step int) error
	AddOptions(paneID string) ([]workspace.AddOption, error)
	AddToPane(paneID string, source workspace.Source) error
	AddWindowsToPane(paneID string, windows []string) error
	AddWebsite(paneID, address string) (workspace.WebsiteResult, error)
	AddWebAppWhenReady(paneID, address string) (bool, error)
	WebAppChoice(itemID string) (workspace.WebAppChoice, error)
	UseWebApp(itemID string) error
	MakeWebApp(itemID string) (workspace.WebsiteResult, error)
	UseWebAppWhenReady(itemID string) (bool, error)
	PopOut(itemID string) error
	Redock(itemID string) error
	AddFrontWindow() error
	BackToBench(itemID string) error
	OpenItem(itemID string) error
	RemoveRemembered(paneID, bundleID string) error
	ClosePane(paneID string) error
	SetSizes(splitID string, sizes []float64) error
	EvenSizes(splitID string) error
	ZoomPane(paneID string) error
	ShowAllPanes() error
	GrowPane() error
	ShrinkPane() error
	EqualPanes() error
	StepLayout(step int) error
	SplitPane(paneID, zone string) (string, error)
	CancelSplit(paneID string) error
	Rearrange() error
	FillScreen() error
	PreviewTree(index int, preset string) (*layout.Node, error)
	SaveTab(index int, draft workspace.TabDraft) error
	PreviewTabFromWindows(preset string, windows []string) (workspace.WindowsPreview, error)
	NewTabFromWindows(draft workspace.TabDraft, windows []string) error
	SaveSettings(settings workspace.SettingsView) error
	CheckShortcuts(changes map[string]string) ([]workspace.ShortcutView, error)
	PauseShortcuts(paused bool) error
	UseSuggestedShortcuts(fields string) error
	SaveLauncher(launcher workspace.LauncherView) (string, error)
	ChooseFolder(start string) (string, error)
	BrowseToAdd(paneID string) (workspace.BrowseResult, error)
	DeleteTab(index int) error
	MoveTab(from, to int) error
	CloseTab(index int, sure, dontAsk bool) (workspace.CloseQuestion, error)
	CloseItem(itemID string, sure, dontAsk bool) (workspace.CloseQuestion, error)
	ResetLayout(index int) error
	Undo(step int) error
	ReopenTab(name string) error
	TermInput(itemID, data string) error
	TermResize(itemID string, cols, rows int) error
	TermSnapshot(itemID string) (workspace.TermSnapshot, error)
	SetOverlay(open bool) error
	ShowPreview(itemID string, anchor layout.Rect) error
	HidePreview() error
	RequestAccessibility() error
	RequestScreenRecording() error
	SetOpenAtLogin(on bool) error
	OpenLoginItems() error
	DismissFirstRun() error
	StarterTabs() ([]workspace.StarterTab, error)
	UseStarterTabs(names []string) error
	SetDialogSize(kind string, w, h float64) error
	SetDemoScreen(screen string, fitWidth float64) error
	DemoMinimise() error
	DemoModifiers(held []string) error
	SetDemoVoiceOver(on bool) error
	TermAck(itemID string, offset int64) error
	TermClear(itemID string) error
	OpenLink(address string) error
	OpenInBrowser(itemID string) error
	ReloadPage(itemID string) error
	GoBack(itemID string) error
	GoForward(itemID string) error
	ZoomPage(itemID string, step int) error
	PageLink(itemID string) (string, error)
	SetFindBar(itemID string, open bool) error
	FindInPage(itemID, text string, backwards bool) (bool, error)
	MuteCapture(itemID, device string, mute bool) error
	StopMedia(itemID string) error
	ReloadConfig() error
	OverwriteConfig() error
	UseConfigBackup() error
	OpenConfig() error
	ShowConfigInFinder() error
	RestoreArrangement() error
	DismissArrangementNotice() error
	DismissUpdate() error
	InstallUpdate() error
	Quit(dontAsk bool) error
	QuitAndReopen(sure, dontAsk bool) error
	CloseTerminal(itemID string, dontAsk bool) error
}

var _ engine = (*workspace.Engine)(nil)

// errNotReady is returned by the bound methods of a Service with no engine,
// which Run never makes.
var errNotReady = errors.New("Bench is still starting; try again in a moment")

// Service is bound to the frontend: its exported methods are the bound
// methods in docs/architecture.md, which the frontend calls by name as
// "github.com/itbm/bench/internal/ui.Service.<Method>". Each delegates to the
// engine. A refused action returns an error whose message the frontend shows
// as a toast. Every exported method is bound, so Service has no others.
type Service struct {
	e bound
}

// newService returns a Service that delegates to e.
func newService(e bound) *Service { return &Service{e: e} }

// get returns the engine, or errNotReady.
func (s *Service) get() (bound, error) {
	if s == nil || s.e == nil {
		return nil, errNotReady
	}
	return s.e, nil
}

// call runs f with the engine.
func (s *Service) call(f func(e bound) error) error {
	e, err := s.get()
	if err != nil {
		return err
	}
	return f(e)
}

// Ready returns the current view. The frontend calls it on load, so the
// engine also lowers Bench's window, in case the page was reloaded while a
// menu or dialog had raised it.
func (s *Service) Ready() (workspace.View, error) {
	e, err := s.get()
	if err != nil {
		return workspace.View{}, err
	}
	return e.Ready()
}

// SetPaneArea sets the pane area, in content coordinates, as the frontend
// measures it.
func (s *Service) SetPaneArea(area layout.Rect) error {
	return s.call(func(e bound) error { return e.SetPaneArea(area) })
}

// SwitchTab switches to the tab at index.
func (s *Service) SwitchTab(index int) error {
	return s.call(func(e bound) error { return e.SwitchTab(index) })
}

// StepTab switches to the next (1) or previous (-1) tab, wrapping.
func (s *Service) StepTab(step int) error {
	return s.call(func(e bound) error { return e.StepTab(step) })
}

// LastTab switches back to the tab used before the current one, as the
// last_tab shortcut does. While the shortcut's modifiers are held, it
// starts or steps the hold that shows every tab (decision 60).
func (s *Service) LastTab() error {
	return s.call(func(e bound) error { return e.LastTab() })
}

// StepHold chooses the next (1) or previous (-1) tab of the hold after
// last_tab's shortcut, for the arrow keys in the overview of every tab.
func (s *Service) StepHold(step int) error {
	return s.call(func(e bound) error { return e.StepHold(step) })
}

// CancelHold ends the hold after last_tab's shortcut without switching
// tabs, for Escape in the overview.
func (s *Service) CancelHold() error {
	return s.call(func(e bound) error { return e.CancelHold() })
}

// SetMode sets the layout mode: auto, wide, or compact.
func (s *Service) SetMode(mode string) error {
	return s.call(func(e bound) error { return e.SetMode(mode) })
}

// FocusPane focuses a pane.
func (s *Service) FocusPane(paneID string) error {
	return s.call(func(e bound) error { return e.FocusPane(paneID) })
}

// FocusUI gives Bench's own UI the keyboard, for the field that renames a
// tab or sub-tab in place.
func (s *Service) FocusUI() error {
	return s.call(func(e bound) error { return e.FocusUI() })
}

// ShowItem shows an item in its pane and focuses the pane.
func (s *Service) ShowItem(paneID, itemID string) error {
	return s.call(func(e bound) error { return e.ShowItem(paneID, itemID) })
}

// MoveItem moves an item to a zone of a pane in a tab: center, left, right,
// top, or bottom.
func (s *Service) MoveItem(itemID string, tab int, paneID, zone string) error {
	return s.call(func(e bound) error { return e.MoveItem(itemID, tab, paneID, zone) })
}

// ShowSideBySide is Show side by side in the "tabs" view, which shows the
// tab as one pane: it changes to the wide layout, and shows the item
// beside the tab's other windows.
func (s *Service) ShowSideBySide(itemID string) error {
	return s.call(func(e bound) error { return e.ShowSideBySide(itemID) })
}

// MoveItemAt moves an item into a pane of a tab, at an index among the
// pane's sub-tabs as drawn: a drop on a pane's header, or Move left and Move
// right in a sub-tab's menu.
func (s *Service) MoveItemAt(itemID string, tab int, paneID string, index int) error {
	return s.call(func(e bound) error { return e.MoveItemAt(itemID, tab, paneID, index) })
}

// MoveItemToTab moves an item into a tab's focused pane.
func (s *Service) MoveItemToTab(itemID string, tab int) error {
	return s.call(func(e bound) error { return e.MoveItemToTab(itemID, tab) })
}

// SendToTab moves the focused pane's shown window into a tab's focused
// pane, as the send_to_tab_modifiers shortcut with 1 to 9 does.
func (s *Service) SendToTab(index int) error {
	return s.call(func(e bound) error { return e.SendToTab(index) })
}

// SendToLastTab moves the focused pane's shown window into the focused
// pane of the tab used before this one, as send_to_tab_modifiers with `
// does.
func (s *Service) SendToLastTab() error {
	return s.call(func(e bound) error { return e.SendToLastTab() })
}

// AddItemToTab shows an item in a tab's focused pane as well as where it is.
func (s *Service) AddItemToTab(itemID string, tab int) error {
	return s.call(func(e bound) error { return e.AddItemToTab(itemID, tab) })
}

// RemoveItemFromTab takes an item that's in several tabs out of one of them.
func (s *Service) RemoveItemFromTab(itemID string, tab int) error {
	return s.call(func(e bound) error { return e.RemoveItemFromTab(itemID, tab) })
}

// MoveItemToNewTab makes a tab, named after an item's app, and moves the
// item there, or with also, shows it there as well as where it is, then
// switches to the tab and saves config.toml.
func (s *Service) MoveItemToNewTab(itemID string, also bool) error {
	return s.call(func(e bound) error { return e.MoveItemToNewTab(itemID, also) })
}

// RenameItem gives a terminal or web page a name of its own, which its
// sub-tab shows; "" takes it away again.
func (s *Service) RenameItem(itemID, label string) error {
	return s.call(func(e bound) error { return e.RenameItem(itemID, label) })
}

// FocusDirection focuses the pane left, right, up, or down.
func (s *Service) FocusDirection(dir string) error {
	return s.call(func(e bound) error { return e.FocusDirection(dir) })
}

// MoveDirection moves the focused pane's shown item into the neighbouring
// pane in a direction, or splits there.
func (s *Service) MoveDirection(dir string) error {
	return s.call(func(e bound) error { return e.MoveDirection(dir) })
}

// CycleSubtab shows the next (1) or previous (-1) sub-tab in the focused
// pane.
func (s *Service) CycleSubtab(step int) error {
	return s.call(func(e bound) error { return e.CycleSubtab(step) })
}

// AddOptions returns the add dialog's rows for a pane, in group order.
func (s *Service) AddOptions(paneID string) ([]workspace.AddOption, error) {
	e, err := s.get()
	if err != nil {
		return nil, err
	}
	return e.AddOptions(paneID)
}

// AddToPane adds something from the add dialog to a pane.
func (s *Service) AddToPane(paneID string, source workspace.Source) error {
	return s.call(func(e bound) error { return e.AddToPane(paneID, source) })
}

// AddWindowsToPane adds the windows ticked in the add dialog to a pane.
func (s *Service) AddWindowsToPane(paneID string, windows []string) error {
	return s.call(func(e bound) error { return e.AddWindowsToPane(paneID, windows) })
}

// AddWebsite adds a website to a pane: the site's web app, or a Web item,
// or nothing yet, having opened the site in the default browser to make a
// web app of it (see AddWebAppWhenReady).
func (s *Service) AddWebsite(paneID, address string) (workspace.WebsiteResult, error) {
	e, err := s.get()
	if err != nil {
		return workspace.WebsiteResult{}, err
	}
	return e.AddWebsite(paneID, address)
}

// AddWebAppWhenReady adds the web app of an address's site to a pane, if
// it's installed now, and reports whether it did.
func (s *Service) AddWebAppWhenReady(paneID, address string) (bool, error) {
	e, err := s.get()
	if err != nil {
		return false, err
	}
	return e.AddWebAppWhenReady(paneID, address)
}

// WebAppChoice says what a Web item's menu offers to put a web app of its
// site in its place: the web app, or to have the default browser make one,
// or nothing.
func (s *Service) WebAppChoice(itemID string) (workspace.WebAppChoice, error) {
	e, err := s.get()
	if err != nil {
		return workspace.WebAppChoice{}, err
	}
	return e.WebAppChoice(itemID)
}

// UseWebApp puts the installed web app of a Web item's site in its place.
func (s *Service) UseWebApp(itemID string) error {
	return s.call(func(e bound) error { return e.UseWebApp(itemID) })
}

// MakeWebApp opens a Web item's site in the default browser to make a web
// app of it, and returns the browser's steps (see UseWebAppWhenReady).
func (s *Service) MakeWebApp(itemID string) (workspace.WebsiteResult, error) {
	e, err := s.get()
	if err != nil {
		return workspace.WebsiteResult{}, err
	}
	return e.MakeWebApp(itemID)
}

// UseWebAppWhenReady puts the web app of a Web item's site in its place, if
// it's installed now, and reports whether it did.
func (s *Service) UseWebAppWhenReady(itemID string) (bool, error) {
	e, err := s.get()
	if err != nil {
		return false, err
	}
	return e.UseWebAppWhenReady(itemID)
}

// PopOut takes an item out of Bench.
func (s *Service) PopOut(itemID string) error {
	return s.call(func(e bound) error { return e.PopOut(itemID) })
}

// Redock docks an undocked window again.
func (s *Service) Redock(itemID string) error {
	return s.call(func(e bound) error { return e.Redock(itemID) })
}

// AddFrontWindow adds the window in front, if it's out of the tabs, to the
// current tab's focused pane.
func (s *Service) AddFrontWindow() error {
	return s.call(func(e bound) error { return e.AddFrontWindow() })
}

// BackToBench puts a popped-out item back in the current tab's focused
// pane, and brings Bench forward on it; "" means the popped-out window that
// has the keyboard.
func (s *Service) BackToBench(itemID string) error {
	return s.call(func(e bound) error { return e.BackToBench(itemID) })
}

// OpenItem opens the app of a remembered place, for its window to go there.
func (s *Service) OpenItem(itemID string) error {
	return s.call(func(e bound) error { return e.OpenItem(itemID) })
}

// RemoveRemembered forgets the places of an app in a pane that its windows
// would go back to.
func (s *Service) RemoveRemembered(paneID, bundleID string) error {
	return s.call(func(e bound) error { return e.RemoveRemembered(paneID, bundleID) })
}

// ClosePane closes a pane that shows nothing.
func (s *Service) ClosePane(paneID string) error {
	return s.call(func(e bound) error { return e.ClosePane(paneID) })
}

// SetSizes sets a split's sizes, as percentages, when the user drags a
// divider.
func (s *Service) SetSizes(splitID string, sizes []float64) error {
	return s.call(func(e bound) error { return e.SetSizes(splitID, sizes) })
}

// EvenSizes gives a split's panes equal sizes, when the user double-clicks
// one of its dividers.
func (s *Service) EvenSizes(splitID string) error {
	return s.call(func(e bound) error { return e.EvenSizes(splitID) })
}

// ZoomPane zooms a pane of the current tab, or the focused pane for "": it
// fills the tab's pane area until ShowAllPanes, or until the tab changes.
func (s *Service) ZoomPane(paneID string) error {
	return s.call(func(e bound) error { return e.ZoomPane(paneID) })
}

// ShowAllPanes ends ZoomPane's zoom.
func (s *Service) ShowAllPanes() error {
	return s.call(func(e bound) error { return e.ShowAllPanes() })
}

// GrowPane makes the current tab's focused pane 5% larger along the split
// that holds it, with Undo.
func (s *Service) GrowPane() error {
	return s.call(func(e bound) error { return e.GrowPane() })
}

// ShrinkPane makes the current tab's focused pane 5% smaller along the
// split that holds it, with Undo.
func (s *Service) ShrinkPane() error {
	return s.call(func(e bound) error { return e.ShrinkPane() })
}

// EqualPanes gives every split of the current tab equal sizes, with Undo.
func (s *Service) EqualPanes() error {
	return s.call(func(e bound) error { return e.EqualPanes() })
}

// StepLayout gives the current tab the next (1) or previous (-1) of the
// six layouts, keeping its windows in order, with Undo.
func (s *Service) StepLayout(step int) error {
	return s.call(func(e bound) error { return e.StepLayout(step) })
}

// SplitPane puts an empty pane beside a pane, at the edge zone gives, for
// the add dialog to fill, and returns its ID.
func (s *Service) SplitPane(paneID, zone string) (string, error) {
	e, err := s.get()
	if err != nil {
		return "", err
	}
	return e.SplitPane(paneID, zone)
}

// CancelSplit takes away SplitPane's empty pane, as its add dialog closes
// without adding anything to it.
func (s *Service) CancelSplit(paneID string) error {
	return s.call(func(e bound) error { return e.CancelSplit(paneID) })
}

// Rearrange arranges the current tab again.
func (s *Service) Rearrange() error {
	return s.call(func(e bound) error { return e.Rearrange() })
}

// FillScreen fills the screen with Bench's window, unless it fills it
// already.
func (s *Service) FillScreen() error {
	return s.call(func(e bound) error { return e.FillScreen() })
}

// PreviewTree returns the tree Tab properties would make for the tab at
// index (or a new tab, -1) with a preset.
func (s *Service) PreviewTree(index int, preset string) (*layout.Node, error) {
	e, err := s.get()
	if err != nil {
		return nil, err
	}
	return e.PreviewTree(index, preset)
}

// SaveTab saves Tab properties; index -1 adds a tab.
func (s *Service) SaveTab(index int, draft workspace.TabDraft) error {
	return s.call(func(e bound) error { return e.SaveTab(index, draft) })
}

// PreviewTabFromWindows returns the New tab dialog's preview of a tab made
// of windows that are open, with a preset ("" for the one suggested).
func (s *Service) PreviewTabFromWindows(preset string, windows []string) (workspace.WindowsPreview, error) {
	e, err := s.get()
	if err != nil {
		return workspace.WindowsPreview{}, err
	}
	return e.PreviewTabFromWindows(preset, windows)
}

// NewTabFromWindows makes a tab of windows that are open, and switches to
// it.
func (s *Service) NewTabFromWindows(draft workspace.TabDraft, windows []string) error {
	return s.call(func(e bound) error { return e.NewTabFromWindows(draft, windows) })
}

// SaveSettings saves the Settings dialog: the settings for every tab.
func (s *Service) SaveSettings(settings workspace.SettingsView) error {
	return s.call(func(e bound) error { return e.SaveSettings(settings) })
}

// CheckShortcuts says whether each field of the [shortcuts] table would
// work with Settings ▸ Keyboard's changes, by the fields' names in
// config.toml, without saving them, as the user records shortcuts.
func (s *Service) CheckShortcuts(changes map[string]string) ([]workspace.ShortcutView, error) {
	e, err := s.get()
	if err != nil {
		return nil, err
	}
	return e.CheckShortcuts(changes)
}

// PauseShortcuts releases Bench's global shortcuts while Settings ▸
// Keyboard records one, so that the keys reach it, and registers them again
// with paused false.
func (s *Service) PauseShortcuts(paused bool) error {
	return s.call(func(e bound) error { return e.PauseShortcuts(paused) })
}

// UseSuggestedShortcuts sets fields of the [shortcuts] table, by their
// names in config.toml, separated by spaces, to the shortcuts suggested for
// them, as a notice's Use this instead does, and saves config.toml.
func (s *Service) UseSuggestedShortcuts(fields string) error {
	return s.call(func(e bound) error { return e.UseSuggestedShortcuts(fields) })
}

// SaveLauncher adds a launcher, for the add dialog's Save as launcher, and
// saves config.toml. It returns the launcher's name, which Bench gives one
// saved without a name.
func (s *Service) SaveLauncher(launcher workspace.LauncherView) (string, error) {
	e, err := s.get()
	if err != nil {
		return "", err
	}
	return e.SaveLauncher(launcher)
}

// ChooseFolder shows the folder picker, starting in a folder such as
// ~/code, and returns the folder the user chose, or "" if they cancelled.
func (s *Service) ChooseFolder(start string) (string, error) {
	e, err := s.get()
	if err != nil {
		return "", err
	}
	return e.ChooseFolder(start)
}

// BrowseToAdd shows macOS's open panel, for the add dialog's Browse…, and
// adds the app the user chooses to a pane of the current tab, or returns
// the program or script they choose, for the New terminal… form. It
// returns nothing if they cancel.
func (s *Service) BrowseToAdd(paneID string) (workspace.BrowseResult, error) {
	e, err := s.get()
	if err != nil {
		return workspace.BrowseResult{}, err
	}
	return e.BrowseToAdd(paneID)
}

// DeleteTab deletes a tab; its windows move to "Not in a tab".
func (s *Service) DeleteTab(index int) error {
	return s.call(func(e bound) error { return e.DeleteTab(index) })
}

// MoveTab moves a tab to another place among the tabs, which gives it that
// place's shortcut.
func (s *Service) MoveTab(from, to int) error {
	return s.call(func(e bound) error { return e.MoveTab(from, to) })
}

// CloseTab closes a tab's windows, pages, and terminals, other than those in
// other tabs too, and deletes the tab; the only tab stays, empty. Unless
// sure, it may ask first instead (Ask), closing nothing. With dontAsk
// (Don't ask again), it also turns confirm_close off.
func (s *Service) CloseTab(index int, sure, dontAsk bool) (workspace.CloseQuestion, error) {
	e, err := s.get()
	if err != nil {
		return workspace.CloseQuestion{}, err
	}
	return e.CloseTab(index, sure, dontAsk)
}

// CloseItem closes a sub-tab's window, page, or terminal, from every tab.
// Unless sure, a terminal that's running something asks first instead
// (Ask), closing nothing. With dontAsk (Don't ask again), it also turns
// confirm_quit_busy_terminals off.
func (s *Service) CloseItem(itemID string, sure, dontAsk bool) (workspace.CloseQuestion, error) {
	e, err := s.get()
	if err != nil {
		return workspace.CloseQuestion{}, err
	}
	return e.CloseItem(itemID, sure, dontAsk)
}

// ResetLayout returns a tab to its config layout.
func (s *Service) ResetLayout(index int) error {
	return s.call(func(e bound) error { return e.ResetLayout(index) })
}

// Undo takes back a change to the arrangement, which a toast's Undo names,
// and those made after it.
func (s *Service) Undo(step int) error {
	return s.call(func(e bound) error { return e.Undo(step) })
}

// ReopenTab brings back the tab of that name that was deleted or closed
// most recently.
func (s *Service) ReopenTab(name string) error {
	return s.call(func(e bound) error { return e.ReopenTab(name) })
}

// TermInput sends what's typed to a terminal.
func (s *Service) TermInput(itemID, data string) error {
	return s.call(func(e bound) error { return e.TermInput(itemID, data) })
}

// TermResize sets a terminal's size in character cells.
func (s *Service) TermResize(itemID string, cols, rows int) error {
	return s.call(func(e bound) error { return e.TermResize(itemID, cols, rows) })
}

// TermSnapshot returns a terminal's recent output, base64-encoded, for a new
// view to replay, and where it ends.
func (s *Service) TermSnapshot(itemID string) (workspace.TermSnapshot, error) {
	e, err := s.get()
	if err != nil {
		return workspace.TermSnapshot{}, err
	}
	return e.TermSnapshot(itemID)
}

// SetOverlay turns raised mode on while a menu, dialog, preview, or drag is
// open, and off again.
func (s *Service) SetOverlay(open bool) error {
	return s.call(func(e bound) error { return e.SetOverlay(open) })
}

// ShowPreview shows a live preview of an item under anchor, a sub-tab's
// rectangle in content coordinates.
func (s *Service) ShowPreview(itemID string, anchor layout.Rect) error {
	return s.call(func(e bound) error { return e.ShowPreview(itemID, anchor) })
}

// HidePreview hides the live preview.
func (s *Service) HidePreview() error {
	return s.call(func(e bound) error { return e.HidePreview() })
}

// RequestAccessibility asks macOS for Accessibility permission.
func (s *Service) RequestAccessibility() error {
	return s.call(func(e bound) error { return e.RequestAccessibility() })
}

// RequestScreenRecording asks macOS for Screen Recording permission.
func (s *Service) RequestScreenRecording() error {
	return s.call(func(e bound) error { return e.RequestScreenRecording() })
}

// SetOpenAtLogin turns Open at login on or off.
func (s *Service) SetOpenAtLogin(on bool) error {
	return s.call(func(e bound) error { return e.SetOpenAtLogin(on) })
}

// OpenLoginItems opens System Settings ▸ General ▸ Login Items, where the
// user allows Bench to open at login once macOS needs them to.
func (s *Service) OpenLoginItems() error {
	return s.call(func(e bound) error { return e.OpenLoginItems() })
}

// DismissFirstRun closes the welcome guide for good.
func (s *Service) DismissFirstRun() error {
	return s.call(func(e bound) error { return e.DismissFirstRun() })
}

// StarterTabs returns the tabs that the welcome guide suggests for a
// starting set-up, made of installed apps.
func (s *Service) StarterTabs() ([]workspace.StarterTab, error) {
	e, err := s.get()
	if err != nil {
		return nil, err
	}
	return e.StarterTabs()
}

// UseStarterTabs adds the suggested tabs that the user kept, by name, after
// the tabs there are, and saves config.toml.
func (s *Service) UseStarterTabs(names []string) error {
	return s.call(func(e bound) error { return e.UseStarterTabs(names) })
}

// SetDialogSize remembers the size that the user gave a dialog.
func (s *Service) SetDialogSize(kind string, w, h float64) error {
	return s.call(func(e bound) error { return e.SetDialogSize(kind, w, h) })
}

// SetDemoScreen simulates a screen in the browser preview: fit, monitor, or
// laptop.
func (s *Service) SetDemoScreen(screen string, fitWidth float64) error {
	return s.call(func(e bound) error { return e.SetDemoScreen(screen, fitWidth) })
}

// DemoMinimise minimises the window that the focused pane shows, as
// Command-M does, in the browser preview.
func (s *Service) DemoMinimise() error {
	return s.call(func(e bound) error { return e.DemoMinimise() })
}

// DemoModifiers says which modifier keys are held down in the browser
// preview, by name ("ctrl", "alt", "shift", or "cmd"), for the hold after
// last_tab's shortcut.
func (s *Service) DemoModifiers(held []string) error {
	return s.call(func(e bound) error { return e.DemoModifiers(held) })
}

// SetDemoVoiceOver simulates VoiceOver being turned on or off, in the
// browser preview.
func (s *Service) SetDemoVoiceOver(on bool) error {
	return s.call(func(e bound) error { return e.SetDemoVoiceOver(on) })
}

// TermAck tells a terminal that a view has drawn its output up to offset.
func (s *Service) TermAck(itemID string, offset int64) error {
	return s.call(func(e bound) error { return e.TermAck(itemID, offset) })
}

// TermClear forgets a terminal's recent output, as its view clears it.
func (s *Service) TermClear(itemID string) error {
	return s.call(func(e bound) error { return e.TermClear(itemID) })
}

// OpenLink opens an http or https link that a terminal program printed, in
// the default browser.
func (s *Service) OpenLink(address string) error {
	return s.call(func(e bound) error { return e.OpenLink(address) })
}

// OpenInBrowser opens the page a Web item shows in the default browser.
func (s *Service) OpenInBrowser(itemID string) error {
	return s.call(func(e bound) error { return e.OpenInBrowser(itemID) })
}

// ReloadPage loads a Web item's page again.
func (s *Service) ReloadPage(itemID string) error {
	return s.call(func(e bound) error { return e.ReloadPage(itemID) })
}

// GoBack goes back to the page a Web item showed before.
func (s *Service) GoBack(itemID string) error {
	return s.call(func(e bound) error { return e.GoBack(itemID) })
}

// GoForward goes forward again in a Web item's pages, after GoBack.
func (s *Service) GoForward(itemID string) error {
	return s.call(func(e bound) error { return e.GoForward(itemID) })
}

// ZoomPage zooms a Web item's page in (1), out (-1), or back to actual size
// (0), which the item keeps.
func (s *Service) ZoomPage(itemID string, step int) error {
	return s.call(func(e bound) error { return e.ZoomPage(itemID, step) })
}

// PageLink returns the address of the page a Web item shows, for Copy page
// link, which puts it on the clipboard.
func (s *Service) PageLink(itemID string) (string, error) {
	e, err := s.get()
	if err != nil {
		return "", err
	}
	return e.PageLink(itemID)
}

// SetFindBar opens or closes a Web item's find bar, which moves its page
// down to make room.
func (s *Service) SetFindBar(itemID string, open bool) error {
	return s.call(func(e bound) error { return e.SetFindBar(itemID, open) })
}

// FindInPage finds text in a Web item's page, the next match or the one
// before it, and reports whether there's one.
func (s *Service) FindInPage(itemID, text string, backwards bool) (bool, error) {
	e, err := s.get()
	if err != nil {
		return false, err
	}
	return e.FindInPage(itemID, text, backwards)
}

// MuteCapture mutes a Web item's page's camera or microphone (device), or
// turns it back on, without mute.
func (s *Service) MuteCapture(itemID, device string, mute bool) error {
	return s.call(func(e bound) error { return e.MuteCapture(itemID, device, mute) })
}

// StopMedia pauses every sound and video that a Web item's page plays.
func (s *Service) StopMedia(itemID string) error {
	return s.call(func(e bound) error { return e.StopMedia(itemID) })
}

// ReloadConfig reads config.toml again and applies it, keeping the
// arrangement.
func (s *Service) ReloadConfig() error {
	return s.call(func(e bound) error { return e.ReloadConfig() })
}

// OverwriteConfig writes Bench's config over a config.toml that was changed
// outside Bench.
func (s *Service) OverwriteConfig() error {
	return s.call(func(e bound) error { return e.OverwriteConfig() })
}

// UseConfigBackup puts config.toml.bak in place of a config.toml that has
// problems.
func (s *Service) UseConfigBackup() error {
	return s.call(func(e bound) error { return e.UseConfigBackup() })
}

// OpenConfig opens config.toml in the default app for plain text.
func (s *Service) OpenConfig() error {
	return s.call(func(e bound) error { return e.OpenConfig() })
}

// ShowConfigInFinder shows config.toml in the Finder.
func (s *Service) ShowConfigInFinder() error {
	return s.call(func(e bound) error { return e.ShowConfigInFinder() })
}

// RestoreArrangement puts back the arrangement saved in state.json of the
// tabs that Bench couldn't keep at startup, once tabs have their names.
func (s *Service) RestoreArrangement() error {
	return s.call(func(e bound) error { return e.RestoreArrangement() })
}

// DismissArrangementNotice takes away the notice that Bench couldn't keep
// the arrangement saved in state.json.
func (s *Service) DismissArrangementNotice() error {
	return s.call(func(e bound) error { return e.DismissArrangementNotice() })
}

// DismissUpdate takes away the notice of a newer version of Bench, which
// Bench doesn't announce again.
func (s *Service) DismissUpdate() error {
	return s.call(func(e bound) error { return e.DismissUpdate() })
}

// InstallUpdate shows the updater's window for the newer version that the
// notice names, which offers to install it.
func (s *Service) InstallUpdate() error {
	return s.call(func(e bound) error { return e.InstallUpdate() })
}

// Quit quits Bench without asking again, as the question before quitting
// says to. With dontAsk (Don't ask again), it first turns
// confirm_quit_busy_terminals off.
func (s *Service) Quit(dontAsk bool) error {
	return s.call(func(e bound) error { return e.Quit(dontAsk) })
}

// QuitAndReopen quits Bench and opens it again, for macOS to apply Screen
// Recording permission. Unless sure, it first asks, as quitting does, while
// a terminal is running something. With dontAsk (Don't ask again), it
// first turns confirm_quit_busy_terminals off.
func (s *Service) QuitAndReopen(sure, dontAsk bool) error {
	return s.call(func(e bound) error { return e.QuitAndReopen(sure, dontAsk) })
}

// CloseTerminal ends a terminal and closes its own window, as the question
// before closing a popped-out terminal's window says to. With dontAsk
// (Don't ask again), it also turns confirm_quit_busy_terminals off.
func (s *Service) CloseTerminal(itemID string, dontAsk bool) error {
	return s.call(func(e bound) error { return e.CloseTerminal(itemID, dontAsk) })
}
