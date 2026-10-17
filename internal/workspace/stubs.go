// Package workspace holds Bench's model (tabs, items, moves, rules) and the
// engine that runs it: it reacts to platform events, switches tabs, arranges
// windows through the windows package, keeps state.json up to date, and sends
// the View to the frontend.
package workspace

import (
	"context"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// This file fixes the engine's exported API so that internal/ui and cmd/bench
// can be built against it. Delete it once everything is implemented in its own
// file.

// Emitter sends things to the frontend and the shell. The engine calls it
// without holding its lock; implementations must not block for long.
type Emitter interface {
	// State sends a bench:state event.
	State(v View)
	// Toast sends a bench:toast event.
	Toast(message string)
	// Command sends a bench:command event, and brings Bench's window
	// forward if the command needs it (such as opening the palette).
	Command(c Command)
	// TermData sends a term:data event.
	TermData(d TermData)
	// PopOutTerminal gives a terminal item a window of its own, and
	// CloseTerminalWindow closes it. In the browser preview these do
	// nothing and return nil.
	PopOutTerminal(itemID, title string) error
	CloseTerminalWindow(itemID string)
	// CurrentTab tells the shell the current tab's name, for the menu bar
	// item.
	CurrentTab(name string, tabs []string)
}

// VSCodeServer is a running `code serve-web` (vscode.Server).
type VSCodeServer interface {
	URL(folder string) string
	Stop() error
}

// LoginItem turns Open at login on and off (Wails' Autostart manager).
type LoginItem interface {
	IsEnabled() (bool, error)
	Enable() error
	Disable() error
}

// Options configure an Engine.
type Options struct {
	Platform platform.Platform
	Clock    clock.Clock // Nil means clock.Real().
	// NewTerms makes the terminal host, given the engine's callbacks.
	// Nil means term.NewManager.
	NewTerms func(cb term.Callbacks) term.Host
	Emitter  Emitter
	// Dir is the support folder holding config.toml, state.json, and
	// parked.json. It's created if needed.
	Dir string
	// StartVSCode starts `code serve-web` the first time a VS Code for the
	// Web item is shown. Nil means VS Code items show an explanation.
	StartVSCode func(ctx context.Context) (VSCodeServer, error)
	// LoginItem is nil where Open at login isn't available.
	LoginItem LoginItem
	// Demo is true for the browser preview.
	Demo bool
	// Headless is true for `bench arrange`: there's no Bench window, so the
	// pane area is the screen's visible area in global coordinates, Web and
	// Built-in items are skipped, and nothing is persisted.
	Headless bool
}

// Engine runs Bench. Its methods are safe for concurrent use.
type Engine struct{}

// New loads the config and state from opts.Dir (writing the default config
// on first run), restores windows left parked by a crashed run, and builds
// the model. It doesn't touch any window until Start.
func New(opts Options) (*Engine, error) { panic("not implemented") }

// Start begins handling platform events and arranges the current tab.
func (e *Engine) Start() { panic("not implemented") }

// Close puts every parked window back, ends terminal sessions, stops the VS
// Code server, and saves state.json. It's safe to call more than once.
func (e *Engine) Close() error { panic("not implemented") }

// View returns the current view.
func (e *Engine) View() View { panic("not implemented") }

// Problems returns the config's problems, if it had any when loaded.
func (e *Engine) Problems() []string { panic("not implemented") }

// ConfigCopy returns what the shell needs from the config.
func (e *Engine) ConfigCopy() ConfigInfo { panic("not implemented") }

// ConfigInfo is what the shell needs from the config.
type ConfigInfo struct {
	Path          string
	Shortcuts     map[string]string // Field name to shortcut, from [shortcuts].
	LaunchTimeout time.Duration
}

// SetNotices replaces the notices from one source (such as "shortcuts"),
// shown in the status bar.
func (e *Engine) SetNotices(source string, notices []string) { panic("not implemented") }

// Shortcut runs a global shortcut's action (see the shortcuts package's
// Action constants) with its argument.
func (e *Engine) Shortcut(action, arg string) { panic("not implemented") }

// The rest mirror ui.Service's bound methods (docs/architecture.md).

func (e *Engine) SetPaneArea(area layout.Rect) error   { panic("not implemented") }
func (e *Engine) SwitchTab(index int) error            { panic("not implemented") }
func (e *Engine) StepTab(step int) error               { panic("not implemented") }
func (e *Engine) SetMode(mode string) error            { panic("not implemented") }
func (e *Engine) FocusPane(paneID string) error        { panic("not implemented") }
func (e *Engine) ShowItem(paneID, itemID string) error { panic("not implemented") }
func (e *Engine) MoveItem(itemID string, tab int, paneID, zone string) error {
	panic("not implemented")
}
func (e *Engine) MoveItemToTab(itemID string, tab int) error     { panic("not implemented") }
func (e *Engine) FocusDirection(dir string) error                { panic("not implemented") }
func (e *Engine) MoveDirection(dir string) error                 { panic("not implemented") }
func (e *Engine) CycleSubtab(step int) error                     { panic("not implemented") }
func (e *Engine) AddOptions(paneID string) ([]AddOption, error)  { panic("not implemented") }
func (e *Engine) AddToPane(paneID string, source Source) error   { panic("not implemented") }
func (e *Engine) PopOut(itemID string) error                     { panic("not implemented") }
func (e *Engine) Redock(itemID string) error                     { panic("not implemented") }
func (e *Engine) SetSizes(splitID string, sizes []float64) error { panic("not implemented") }
func (e *Engine) Rearrange() error                               { panic("not implemented") }
func (e *Engine) PreviewTree(index int, preset string) (*layout.Node, error) {
	panic("not implemented")
}
func (e *Engine) SaveTab(index int, draft TabDraft) error             { panic("not implemented") }
func (e *Engine) DeleteTab(index int) error                           { panic("not implemented") }
func (e *Engine) ResetLayout(index int) error                         { panic("not implemented") }
func (e *Engine) TermInput(itemID, data string) error                 { panic("not implemented") }
func (e *Engine) TermResize(itemID string, cols, rows int) error      { panic("not implemented") }
func (e *Engine) TermSnapshot(itemID string) (string, error)          { panic("not implemented") }
func (e *Engine) SetOverlay(open bool) error                          { panic("not implemented") }
func (e *Engine) ShowPreview(itemID string, anchor layout.Rect) error { panic("not implemented") }
func (e *Engine) HidePreview() error                                  { panic("not implemented") }
func (e *Engine) RequestAccessibility() error                         { panic("not implemented") }
func (e *Engine) RequestScreenRecording() error                       { panic("not implemented") }
func (e *Engine) SetOpenAtLogin(on bool) error                        { panic("not implemented") }
func (e *Engine) DismissFirstRun() error                              { panic("not implemented") }
func (e *Engine) SetDemoScreen(screen string, fitWidth float64) error { panic("not implemented") }

// ArrangeOnce is for `bench arrange <tab>`: it switches to the named tab,
// opens missing apps and waits for their windows (up to the launch timeout),
// and arranges the tab across the screen's visible area, showing each pane's
// first window. It returns when the arrangement is done.
func (e *Engine) ArrangeOnce(ctx context.Context, tabName string) error { panic("not implemented") }
