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
	notices map[string][]string
	started bool
	closed  int
}

var _ engine = (*fakeEngine)(nil)

func newFakeEngine() *fakeEngine {
	return &fakeEngine{notices: map[string][]string{}}
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

func (f *fakeEngine) SetNotices(source string, notices []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notices[source] = notices
}

func (f *fakeEngine) Shortcut(action, arg string) { _ = f.record("Shortcut", action, arg) }

func (f *fakeEngine) TerminalWindowClosed(itemID string) {
	_ = f.record("TerminalWindowClosed", itemID)
}

func (f *fakeEngine) SetPaneArea(area layout.Rect) error { return f.record("SetPaneArea", area) }
func (f *fakeEngine) SwitchTab(index int) error          { return f.record("SwitchTab", index) }
func (f *fakeEngine) StepTab(step int) error             { return f.record("StepTab", step) }
func (f *fakeEngine) SetMode(mode string) error          { return f.record("SetMode", mode) }
func (f *fakeEngine) FocusPane(paneID string) error      { return f.record("FocusPane", paneID) }
func (f *fakeEngine) ShowItem(paneID, itemID string) error {
	return f.record("ShowItem", paneID, itemID)
}
func (f *fakeEngine) MoveItem(itemID string, tab int, paneID, zone string) error {
	return f.record("MoveItem", itemID, tab, paneID, zone)
}
func (f *fakeEngine) MoveItemToTab(itemID string, tab int) error {
	return f.record("MoveItemToTab", itemID, tab)
}
func (f *fakeEngine) AddItemToTab(itemID string, tab int) error {
	return f.record("AddItemToTab", itemID, tab)
}
func (f *fakeEngine) RemoveItemFromTab(itemID string, tab int) error {
	return f.record("RemoveItemFromTab", itemID, tab)
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
func (f *fakeEngine) SetSizes(splitID string, sizes []float64) error {
	return f.record("SetSizes", splitID, sizes)
}
func (f *fakeEngine) Rearrange() error { return f.record("Rearrange") }
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
func (f *fakeEngine) DeleteTab(index int) error   { return f.record("DeleteTab", index) }
func (f *fakeEngine) ResetLayout(index int) error { return f.record("ResetLayout", index) }
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
func (f *fakeEngine) DismissFirstRun() error        { return f.record("DismissFirstRun") }
func (f *fakeEngine) SetDemoScreen(screen string, fitWidth float64) error {
	return f.record("SetDemoScreen", screen, fitWidth)
}
func (f *fakeEngine) TermAck(itemID string, offset int64) error {
	return f.record("TermAck", itemID, offset)
}
func (f *fakeEngine) OpenLink(address string) error { return f.record("OpenLink", address) }
