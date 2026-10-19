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
	SetNotices(source string, notices []string)
	Shortcut(action, arg string)
	TerminalWindowClosed(itemID string)
}

// bound is the part of workspace.Engine that Service exposes to the
// frontend, one method for each bound method.
type bound interface {
	Ready() (workspace.View, error)
	SetPaneArea(area layout.Rect) error
	SwitchTab(index int) error
	StepTab(step int) error
	SetMode(mode string) error
	FocusPane(paneID string) error
	ShowItem(paneID, itemID string) error
	MoveItem(itemID string, tab int, paneID, zone string) error
	MoveItemToTab(itemID string, tab int) error
	AddItemToTab(itemID string, tab int) error
	RemoveItemFromTab(itemID string, tab int) error
	FocusDirection(dir string) error
	MoveDirection(dir string) error
	CycleSubtab(step int) error
	AddOptions(paneID string) ([]workspace.AddOption, error)
	AddToPane(paneID string, source workspace.Source) error
	PopOut(itemID string) error
	Redock(itemID string) error
	SetSizes(splitID string, sizes []float64) error
	Rearrange() error
	PreviewTree(index int, preset string) (*layout.Node, error)
	SaveTab(index int, draft workspace.TabDraft) error
	DeleteTab(index int) error
	ResetLayout(index int) error
	TermInput(itemID, data string) error
	TermResize(itemID string, cols, rows int) error
	TermSnapshot(itemID string) (workspace.TermSnapshot, error)
	SetOverlay(open bool) error
	ShowPreview(itemID string, anchor layout.Rect) error
	HidePreview() error
	RequestAccessibility() error
	RequestScreenRecording() error
	SetOpenAtLogin(on bool) error
	DismissFirstRun() error
	SetDemoScreen(screen string, fitWidth float64) error
	TermAck(itemID string, offset int64) error
	OpenLink(address string) error
	OpenInBrowser(itemID string) error
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

// SetMode sets the layout mode: auto, wide, or compact.
func (s *Service) SetMode(mode string) error {
	return s.call(func(e bound) error { return e.SetMode(mode) })
}

// FocusPane focuses a pane.
func (s *Service) FocusPane(paneID string) error {
	return s.call(func(e bound) error { return e.FocusPane(paneID) })
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

// MoveItemToTab moves an item into a tab's focused pane.
func (s *Service) MoveItemToTab(itemID string, tab int) error {
	return s.call(func(e bound) error { return e.MoveItemToTab(itemID, tab) })
}

// AddItemToTab shows an item in a tab's focused pane as well as where it is.
func (s *Service) AddItemToTab(itemID string, tab int) error {
	return s.call(func(e bound) error { return e.AddItemToTab(itemID, tab) })
}

// RemoveItemFromTab takes an item that's in several tabs out of one of them.
func (s *Service) RemoveItemFromTab(itemID string, tab int) error {
	return s.call(func(e bound) error { return e.RemoveItemFromTab(itemID, tab) })
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

// PopOut takes an item out of Bench.
func (s *Service) PopOut(itemID string) error {
	return s.call(func(e bound) error { return e.PopOut(itemID) })
}

// Redock docks an undocked window again.
func (s *Service) Redock(itemID string) error {
	return s.call(func(e bound) error { return e.Redock(itemID) })
}

// SetSizes sets a split's sizes, as percentages, when the user drags a
// divider.
func (s *Service) SetSizes(splitID string, sizes []float64) error {
	return s.call(func(e bound) error { return e.SetSizes(splitID, sizes) })
}

// Rearrange arranges the current tab again.
func (s *Service) Rearrange() error {
	return s.call(func(e bound) error { return e.Rearrange() })
}

// PreviewTree returns the tree the layout editor would make for the tab at
// index (or a new tab, -1) with a preset.
func (s *Service) PreviewTree(index int, preset string) (*layout.Node, error) {
	e, err := s.get()
	if err != nil {
		return nil, err
	}
	return e.PreviewTree(index, preset)
}

// SaveTab saves the layout editor; index -1 adds a tab.
func (s *Service) SaveTab(index int, draft workspace.TabDraft) error {
	return s.call(func(e bound) error { return e.SaveTab(index, draft) })
}

// DeleteTab deletes a tab; its windows move to "Not in a tab".
func (s *Service) DeleteTab(index int) error {
	return s.call(func(e bound) error { return e.DeleteTab(index) })
}

// ResetLayout returns a tab to its config layout.
func (s *Service) ResetLayout(index int) error {
	return s.call(func(e bound) error { return e.ResetLayout(index) })
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

// DismissFirstRun closes the permissions guide for good.
func (s *Service) DismissFirstRun() error {
	return s.call(func(e bound) error { return e.DismissFirstRun() })
}

// SetDemoScreen simulates a screen in the browser preview: fit, monitor, or
// laptop.
func (s *Service) SetDemoScreen(screen string, fitWidth float64) error {
	return s.call(func(e bound) error { return e.SetDemoScreen(screen, fitWidth) })
}

// TermAck tells a terminal that a view has drawn its output up to offset.
func (s *Service) TermAck(itemID string, offset int64) error {
	return s.call(func(e bound) error { return e.TermAck(itemID, offset) })
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
