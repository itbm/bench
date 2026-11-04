//go:build darwin || server

package ui

import (
	"fmt"
	"sync"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/workspace"
)

// call is a method call a fake recorded.
type call struct {
	Method string
	Args   []any
}

func (c call) String() string { return fmt.Sprintf("%s%v", c.Method, c.Args) }

// fakeEngine records every call and returns err from every method that
// returns an error. The method called panicOn panics.
type fakeEngine struct {
	mu      sync.Mutex
	calls   []call
	err     error
	panicOn string
	view    workspace.View
	cfg     workspace.ConfigInfo
	// reports holds what ReportShortcuts was told, in turn.
	reports []workspace.ShortcutReport
	start   workspace.WindowStart
	started bool
	closed  int
	// mayQuit is what AskToQuit, AskToQuitNow, and AskToCloseTerminal
	// answer, and busyEngine has AskToQuitNow not answer, as an engine
	// whose lock is held doesn't.
	mayQuit    bool
	busyEngine bool
}

var _ engine = (*fakeEngine)(nil)

func newFakeEngine() *fakeEngine {
	return &fakeEngine{}
}

func (f *fakeEngine) record(method string, args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{Method: method, Args: args})
	if method == f.panicOn {
		panic("the fake engine panicked in " + method)
	}
	return f.err
}

// Calls returns the calls so far.
func (f *fakeEngine) Calls() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

func (f *fakeEngine) Ready() (workspace.View, error) {
	if err := f.record("Ready"); err != nil {
		return workspace.View{}, err
	}
	return f.view, nil
}

func (f *fakeEngine) Start() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = true
}

func (f *fakeEngine) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return nil
}

func (f *fakeEngine) ConfigCopy() workspace.ConfigInfo { return f.cfg }

func (f *fakeEngine) ReportShortcuts(r workspace.ShortcutReport) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports = append(f.reports, r)
}

// lastReport returns what ReportShortcuts was last told, and whether it
// was told anything.
func (f *fakeEngine) lastReport() (workspace.ShortcutReport, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reports) == 0 {
		return workspace.ShortcutReport{}, false
	}
	return f.reports[len(f.reports)-1], true
}

func (f *fakeEngine) Shortcut(action, arg string) { _ = f.record("Shortcut", action, arg) }

func (f *fakeEngine) MoveCurrentTab(step int) error { return f.record("MoveCurrentTab", step) }
func (f *fakeEngine) ToggleZoom() error             { return f.record("ToggleZoom") }

func (f *fakeEngine) ProblemReport() workspace.ProblemReport {
	_ = f.record("ProblemReport")
	return workspace.ProblemReport{Mode: "auto"}
}

func (f *fakeEngine) WindowStart() workspace.WindowStart {
	_ = f.record("WindowStart")
	return f.start
}

func (f *fakeEngine) TerminalWindowClosed(itemID string) {
	_ = f.record("TerminalWindowClosed", itemID)
}

func (f *fakeEngine) AskToQuit() bool {
	_ = f.record("AskToQuit")
	return f.mayQuit
}

func (f *fakeEngine) AskToQuitNow() (quit, answered bool) {
	_ = f.record("AskToQuitNow")
	return f.mayQuit, !f.busyEngine
}

func (f *fakeEngine) AskToCloseTerminal(itemID string) bool {
	_ = f.record("AskToCloseTerminal", itemID)
	return f.mayQuit
}

func (f *fakeEngine) Quit(dontAsk bool) error { return f.record("Quit", dontAsk) }
func (f *fakeEngine) QuitAndReopen(sure, dontAsk bool) error {
	return f.record("QuitAndReopen", sure, dontAsk)
}
func (f *fakeEngine) CloseTerminal(itemID string, dontAsk bool) error {
	return f.record("CloseTerminal", itemID, dontAsk)
}

func (f *fakeEngine) SetPaneArea(area layout.Rect) error { return f.record("SetPaneArea", area) }
func (f *fakeEngine) SwitchTab(index int) error          { return f.record("SwitchTab", index) }
func (f *fakeEngine) StepTab(step int) error             { return f.record("StepTab", step) }
func (f *fakeEngine) LastTab() error                     { return f.record("LastTab") }
func (f *fakeEngine) StepHold(step int) error            { return f.record("StepHold", step) }
func (f *fakeEngine) CancelHold() error                  { return f.record("CancelHold") }
func (f *fakeEngine) SetMode(mode string) error          { return f.record("SetMode", mode) }
func (f *fakeEngine) FocusPane(paneID string) error      { return f.record("FocusPane", paneID) }
func (f *fakeEngine) FocusUI() error                     { return f.record("FocusUI") }
func (f *fakeEngine) ShowItem(paneID, itemID string) error {
	return f.record("ShowItem", paneID, itemID)
}
func (f *fakeEngine) MoveItem(itemID string, tab int, paneID, zone string) error {
	return f.record("MoveItem", itemID, tab, paneID, zone)
}
func (f *fakeEngine) MoveItemAt(itemID string, tab int, paneID string, index int) error {
	return f.record("MoveItemAt", itemID, tab, paneID, index)
}
func (f *fakeEngine) ShowSideBySide(itemID string) error {
	return f.record("ShowSideBySide", itemID)
}
func (f *fakeEngine) MoveItemToTab(itemID string, tab int) error {
	return f.record("MoveItemToTab", itemID, tab)
}
func (f *fakeEngine) SendToTab(index int) error { return f.record("SendToTab", index) }
func (f *fakeEngine) SendToLastTab() error      { return f.record("SendToLastTab") }
func (f *fakeEngine) AddItemToTab(itemID string, tab int) error {
	return f.record("AddItemToTab", itemID, tab)
}
func (f *fakeEngine) RemoveItemFromTab(itemID string, tab int) error {
	return f.record("RemoveItemFromTab", itemID, tab)
}
func (f *fakeEngine) MoveItemToNewTab(itemID string, also bool) error {
	return f.record("MoveItemToNewTab", itemID, also)
}
func (f *fakeEngine) RenameItem(itemID, label string) error {
	return f.record("RenameItem", itemID, label)
}
func (f *fakeEngine) FocusDirection(dir string) error { return f.record("FocusDirection", dir) }
func (f *fakeEngine) MoveDirection(dir string) error  { return f.record("MoveDirection", dir) }
func (f *fakeEngine) CycleSubtab(step int) error      { return f.record("CycleSubtab", step) }
func (f *fakeEngine) AddOptions(paneID string) ([]workspace.AddOption, error) {
	err := f.record("AddOptions", paneID)
	if err != nil {
		return nil, err
	}
	return []workspace.AddOption{{Group: "Built in", Label: "New terminal", Source: workspace.Source{Type: "terminal"}}}, nil
}
func (f *fakeEngine) AddToPane(paneID string, source workspace.Source) error {
	return f.record("AddToPane", paneID, source)
}
func (f *fakeEngine) AddWindowsToPane(paneID string, windows []string) error {
	return f.record("AddWindowsToPane", paneID, windows)
}
func (f *fakeEngine) AddWebsite(paneID, address string) (workspace.WebsiteResult, error) {
	err := f.record("AddWebsite", paneID, address)
	if err != nil {
		return workspace.WebsiteResult{}, err
	}
	return workspace.WebsiteResult{Browser: "Safari", Steps: "Choose File ▸ Add to Dock, then click Add."}, nil
}
func (f *fakeEngine) AddWebAppWhenReady(paneID, address string) (bool, error) {
	err := f.record("AddWebAppWhenReady", paneID, address)
	return err == nil, err
}
func (f *fakeEngine) WebAppChoice(itemID string) (workspace.WebAppChoice, error) {
	err := f.record("WebAppChoice", itemID)
	if err != nil {
		return workspace.WebAppChoice{}, err
	}
	return workspace.WebAppChoice{Offer: "use", App: "Slack", Browser: "Brave"}, nil
}
func (f *fakeEngine) UseWebApp(itemID string) error { return f.record("UseWebApp", itemID) }
func (f *fakeEngine) MakeWebApp(itemID string) (workspace.WebsiteResult, error) {
	err := f.record("MakeWebApp", itemID)
	if err != nil {
		return workspace.WebsiteResult{}, err
	}
	return workspace.WebsiteResult{Browser: "Safari", Steps: "Choose File ▸ Add to Dock, then click Add."}, nil
}
func (f *fakeEngine) UseWebAppWhenReady(itemID string) (bool, error) {
	err := f.record("UseWebAppWhenReady", itemID)
	return err == nil, err
}
func (f *fakeEngine) PopOut(itemID string) error { return f.record("PopOut", itemID) }
func (f *fakeEngine) Redock(itemID string) error { return f.record("Redock", itemID) }
func (f *fakeEngine) AddFrontWindow() error      { return f.record("AddFrontWindow") }
func (f *fakeEngine) BackToBench(itemID string) error {
	return f.record("BackToBench", itemID)
}
func (f *fakeEngine) OpenItem(itemID string) error { return f.record("OpenItem", itemID) }
func (f *fakeEngine) RemoveRemembered(paneID, bundleID string) error {
	return f.record("RemoveRemembered", paneID, bundleID)
}
func (f *fakeEngine) ClosePane(paneID string) error { return f.record("ClosePane", paneID) }
func (f *fakeEngine) SetSizes(splitID string, sizes []float64) error {
	return f.record("SetSizes", splitID, sizes)
}
func (f *fakeEngine) EvenSizes(splitID string) error  { return f.record("EvenSizes", splitID) }
func (f *fakeEngine) ZoomPane(paneID string) error    { return f.record("ZoomPane", paneID) }
func (f *fakeEngine) ShowAllPanes() error             { return f.record("ShowAllPanes") }
func (f *fakeEngine) GrowPane() error                 { return f.record("GrowPane") }
func (f *fakeEngine) ShrinkPane() error               { return f.record("ShrinkPane") }
func (f *fakeEngine) EqualPanes() error               { return f.record("EqualPanes") }
func (f *fakeEngine) StepLayout(step int) error       { return f.record("StepLayout", step) }
func (f *fakeEngine) CancelSplit(paneID string) error { return f.record("CancelSplit", paneID) }
func (f *fakeEngine) SplitPane(paneID, zone string) (string, error) {
	if err := f.record("SplitPane", paneID, zone); err != nil {
		return "", err
	}
	return "p9", nil
}
func (f *fakeEngine) Rearrange() error  { return f.record("Rearrange") }
func (f *fakeEngine) FillScreen() error { return f.record("FillScreen") }
func (f *fakeEngine) PreviewTree(index int, preset string) (*layout.Node, error) {
	err := f.record("PreviewTree", index, preset)
	if err != nil {
		return nil, err
	}
	return layout.NewPane("p1", "w1"), nil
}
func (f *fakeEngine) SaveTab(index int, draft workspace.TabDraft) error {
	return f.record("SaveTab", index, draft)
}
func (f *fakeEngine) PreviewTabFromWindows(preset string, windows []string) (workspace.WindowsPreview, error) {
	if err := f.record("PreviewTabFromWindows", preset, windows); err != nil {
		return workspace.WindowsPreview{}, err
	}
	return workspace.WindowsPreview{Suggested: "side", Apps: 2, Root: layout.NewPane("p1", "f7")}, nil
}
func (f *fakeEngine) NewTabFromWindows(draft workspace.TabDraft, windows []string) error {
	return f.record("NewTabFromWindows", draft, windows)
}
func (f *fakeEngine) SaveSettings(settings workspace.SettingsView) error {
	return f.record("SaveSettings", settings)
}
func (f *fakeEngine) CheckShortcuts(changes map[string]string) ([]workspace.ShortcutView, error) {
	if err := f.record("CheckShortcuts", changes); err != nil {
		return nil, err
	}
	return []workspace.ShortcutView{{Field: "jump", Value: changes["jump"]}}, nil
}
func (f *fakeEngine) PauseShortcuts(paused bool) error { return f.record("PauseShortcuts", paused) }
func (f *fakeEngine) UseSuggestedShortcuts(fields string) error {
	return f.record("UseSuggestedShortcuts", fields)
}
func (f *fakeEngine) SaveLauncher(launcher workspace.LauncherView) (string, error) {
	err := f.record("SaveLauncher", launcher)
	if err != nil {
		return "", err
	}
	return "claude", nil
}
func (f *fakeEngine) ChooseFolder(start string) (string, error) {
	err := f.record("ChooseFolder", start)
	if err != nil {
		return "", err
	}
	return "~/code/bench", nil
}
func (f *fakeEngine) BrowseToAdd(paneID string) (workspace.BrowseResult, error) {
	err := f.record("BrowseToAdd", paneID)
	if err != nil {
		return workspace.BrowseResult{}, err
	}
	return workspace.BrowseResult{Command: "claude"}, nil
}
func (f *fakeEngine) DeleteTab(index int) error  { return f.record("DeleteTab", index) }
func (f *fakeEngine) MoveTab(from, to int) error { return f.record("MoveTab", from, to) }
func (f *fakeEngine) CloseTab(index int, sure, dontAsk bool) (workspace.CloseQuestion, error) {
	if err := f.record("CloseTab", index, sure, dontAsk); err != nil {
		return workspace.CloseQuestion{}, err
	}
	return workspace.CloseQuestion{Ask: !sure, Tab: "Code", Windows: 2}, nil
}
func (f *fakeEngine) CloseItem(itemID string, sure, dontAsk bool) (workspace.CloseQuestion, error) {
	if err := f.record("CloseItem", itemID, sure, dontAsk); err != nil {
		return workspace.CloseQuestion{}, err
	}
	return workspace.CloseQuestion{Terminals: 1}, nil
}
func (f *fakeEngine) ResetLayout(index int) error { return f.record("ResetLayout", index) }
func (f *fakeEngine) Undo(step int) error         { return f.record("Undo", step) }
func (f *fakeEngine) ReopenTab(name string) error { return f.record("ReopenTab", name) }
func (f *fakeEngine) TermInput(itemID, data string) error {
	return f.record("TermInput", itemID, data)
}
func (f *fakeEngine) TermResize(itemID string, cols, rows int) error {
	return f.record("TermResize", itemID, cols, rows)
}
func (f *fakeEngine) TermSnapshot(itemID string) (workspace.TermSnapshot, error) {
	err := f.record("TermSnapshot", itemID)
	if err != nil {
		return workspace.TermSnapshot{}, err
	}
	return workspace.TermSnapshot{Data: "aGk=", Offset: 2}, nil
}
func (f *fakeEngine) SetOverlay(open bool) error { return f.record("SetOverlay", open) }
func (f *fakeEngine) ShowPreview(itemID string, anchor layout.Rect) error {
	return f.record("ShowPreview", itemID, anchor)
}
func (f *fakeEngine) HidePreview() error            { return f.record("HidePreview") }
func (f *fakeEngine) RequestAccessibility() error   { return f.record("RequestAccessibility") }
func (f *fakeEngine) RequestScreenRecording() error { return f.record("RequestScreenRecording") }
func (f *fakeEngine) SetOpenAtLogin(on bool) error  { return f.record("SetOpenAtLogin", on) }
func (f *fakeEngine) OpenLoginItems() error         { return f.record("OpenLoginItems") }
func (f *fakeEngine) DismissFirstRun() error        { return f.record("DismissFirstRun") }
func (f *fakeEngine) StarterTabs() ([]workspace.StarterTab, error) {
	err := f.record("StarterTabs")
	if err != nil {
		return nil, err
	}
	return []workspace.StarterTab{{Name: "Comms", Apps: []workspace.StarterApp{{BundleID: "com.apple.mail", Name: "Mail", Mono: "Ma"}}}}, nil
}
func (f *fakeEngine) UseStarterTabs(names []string) error {
	return f.record("UseStarterTabs", names)
}
func (f *fakeEngine) SetDialogSize(kind string, w, h float64) error {
	return f.record("SetDialogSize", kind, w, h)
}
func (f *fakeEngine) SetDemoScreen(screen string, fitWidth float64) error {
	return f.record("SetDemoScreen", screen, fitWidth)
}

func (f *fakeEngine) DemoMinimise() error {
	return f.record("DemoMinimise")
}
func (f *fakeEngine) DemoModifiers(held []string) error {
	return f.record("DemoModifiers", held)
}

func (f *fakeEngine) SetDemoVoiceOver(on bool) error {
	return f.record("SetDemoVoiceOver", on)
}
func (f *fakeEngine) TermAck(itemID string, offset int64) error {
	return f.record("TermAck", itemID, offset)
}

func (f *fakeEngine) TermClear(itemID string) error {
	return f.record("TermClear", itemID)
}
func (f *fakeEngine) OpenLink(address string) error { return f.record("OpenLink", address) }
func (f *fakeEngine) OpenInBrowser(itemID string) error {
	return f.record("OpenInBrowser", itemID)
}
func (f *fakeEngine) ReloadPage(itemID string) error { return f.record("ReloadPage", itemID) }
func (f *fakeEngine) GoBack(itemID string) error     { return f.record("GoBack", itemID) }
func (f *fakeEngine) GoForward(itemID string) error  { return f.record("GoForward", itemID) }
func (f *fakeEngine) ZoomPage(itemID string, step int) error {
	return f.record("ZoomPage", itemID, step)
}
func (f *fakeEngine) PageLink(itemID string) (string, error) {
	if err := f.record("PageLink", itemID); err != nil {
		return "", err
	}
	return "https://app.slack.com/client", nil
}
func (f *fakeEngine) SetFindBar(itemID string, open bool) error {
	return f.record("SetFindBar", itemID, open)
}
func (f *fakeEngine) FindInPage(itemID, text string, backwards bool) (bool, error) {
	if err := f.record("FindInPage", itemID, text, backwards); err != nil {
		return false, err
	}
	return true, nil
}
func (f *fakeEngine) MuteCapture(itemID, device string, mute bool) error {
	return f.record("MuteCapture", itemID, device, mute)
}
func (f *fakeEngine) StopMedia(itemID string) error { return f.record("StopMedia", itemID) }
func (f *fakeEngine) ReloadConfig() error           { return f.record("ReloadConfig") }
func (f *fakeEngine) OverwriteConfig() error        { return f.record("OverwriteConfig") }
func (f *fakeEngine) UseConfigBackup() error        { return f.record("UseConfigBackup") }
func (f *fakeEngine) OpenConfig() error             { return f.record("OpenConfig") }
func (f *fakeEngine) ShowConfigInFinder() error     { return f.record("ShowConfigInFinder") }
func (f *fakeEngine) RestoreArrangement() error     { return f.record("RestoreArrangement") }
func (f *fakeEngine) DismissArrangementNotice() error {
	return f.record("DismissArrangementNotice")
}
func (f *fakeEngine) DismissUpdate() error   { return f.record("DismissUpdate") }
func (f *fakeEngine) InstallUpdate() error   { return f.record("InstallUpdate") }
func (f *fakeEngine) CheckForUpdates() error { return f.record("CheckForUpdates") }
func (f *fakeEngine) FoundUpdate(version string) {
	f.record("FoundUpdate", version)
}
func (f *fakeEngine) UpdateFinished() { f.record("UpdateFinished") }
