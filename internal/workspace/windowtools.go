package workspace

import (
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
)

// This file has the window tools: apps such as Rectangle that move and
// size other apps' windows, often with global shortcuts of their own.
// Bench can't stop one moving a docked window, so a pane says when a window
// kept moving out of it (keptMovingLocked), and names the tools that are
// running. And Bench registers its shortcuts without keeping them to
// itself, as Wails does, so macOS says nothing when a tool's shortcut is
// the same as one of Bench's, and both apps may act on the key. So the
// first time each tool runs while Bench does, the status bar says what it
// can do to Bench, and, for a tool whose shortcuts Bench can read
// reliably, which of Bench's they take (decision 69).

// windowTool is an app that moves and sizes other apps' windows, which
// Bench knows by its bundle ID.
type windowTool struct {
	bundleID string
	// name is the tool's name, as the notice and a pane say it.
	name string
	// unsure is set for a bundle ID that hasn't been checked on a Mac
	// with the tool installed. docs/development.md lists each one, to
	// check (TestWindowToolsTable).
	unsure bool
	// keys reads the tool's shortcuts from its preferences, in canonical
	// form (shortcuts.Parse), and reports whether it could; nil for a tool
	// whose shortcuts Bench can't read reliably, as for one that keeps
	// them in a format of its own, or in an App Store app's container.
	keys func(platform.ToolPreferences) ([]string, bool)
}

// windowTools are the window tools that Bench knows, in the order the
// notices and panes name them.
var windowTools = []windowTool{
	{bundleID: "com.knollsoft.Rectangle", name: "Rectangle", keys: rectangleKeys},
	// Rectangle Pro was Hookshot, and keeps its bundle ID. It keeps its
	// shortcuts in a format of its own.
	{bundleID: "com.knollsoft.Hookshot", name: "Rectangle Pro"},
	{bundleID: "com.crowdcafe.windowmagnet", name: "Magnet"},
	{bundleID: "com.MrKai77.Loop", name: "Loop", unsure: true},
	{bundleID: "com.raycast.macos", name: "Raycast"},
	{bundleID: "com.hegenberg.BetterSnapTool", name: "BetterSnapTool"},
	{bundleID: "com.hegenberg.BetterTouchTool", name: "BetterTouchTool"},
	{bundleID: "com.manytricks.Moom", name: "Moom"},
	// Divvy from the App Store, and from its own website.
	{bundleID: "com.mizage.Divvy", name: "Divvy", unsure: true},
	{bundleID: "com.mizage.direct.Divvy", name: "Divvy", unsure: true},
	{bundleID: "co.highlyopinionated.swish", name: "Swish", unsure: true},
	{bundleID: "bobko.aerospace", name: "AeroSpace", unsure: true},
	{bundleID: "com.amethyst.Amethyst", name: "Amethyst"},
	// yabai is a command-line program that usually runs without an app
	// bundle, so macOS may not list it among the running apps at all.
	{bundleID: "com.koekeishiya.yabai", name: "yabai", unsure: true},
	{bundleID: "com.divisiblebyzero.Spectacle", name: "Spectacle"},
	{bundleID: "org.hammerspoon.Hammerspoon", name: "Hammerspoon"},
	{bundleID: "com.irradiatedsoftware.SizeUp", name: "SizeUp"},
}

// rectangleActions are Rectangle's actions that have a shortcut by default
// (WindowAction.swift): each one's name, which is the key it keeps the
// user's shortcut under, the name it had before Rectangle renamed it, and
// its shortcut in each of Rectangle's two sets, the recommended one and
// Spectacle's, in canonical form. Rectangle keeps a shortcut in its
// preferences only once the user changes it, or sets it to none.
var rectangleActions = []struct {
	name, alias, recommended, spectacle string
}{
	{"leftHalf", "leftSide", "ctrl+alt+left", "alt+cmd+left"},
	{"rightHalf", "rightSide", "ctrl+alt+right", "alt+cmd+right"},
	{"bottomHalf", "bottomSide", "ctrl+alt+down", "alt+cmd+down"},
	{"topHalf", "topSide", "ctrl+alt+up", "alt+cmd+up"},
	{"bottomLeft", "", "ctrl+alt+j", "ctrl+shift+cmd+left"},
	{"bottomRight", "", "ctrl+alt+k", "ctrl+shift+cmd+right"},
	{"topLeft", "", "ctrl+alt+u", "ctrl+cmd+left"},
	{"topRight", "", "ctrl+alt+i", "ctrl+cmd+right"},
	{"maximize", "", "ctrl+alt+return", "alt+cmd+f"},
	{"maximizeHeight", "", "ctrl+alt+shift+up", "ctrl+alt+shift+up"},
	{"previousDisplay", "", "ctrl+alt+cmd+left", "ctrl+alt+cmd+left"},
	{"nextDisplay", "", "ctrl+alt+cmd+right", "ctrl+alt+cmd+right"},
	{"larger", "", "ctrl+alt+=", "ctrl+alt+shift+right"},
	{"smaller", "", "ctrl+alt+-", "ctrl+alt+shift+left"},
	{"center", "", "ctrl+alt+c", "alt+cmd+c"},
	{"restore", "", "ctrl+alt+delete", "ctrl+alt+delete"},
	{"firstThird", "", "ctrl+alt+d", ""},
	{"firstTwoThirds", "", "ctrl+alt+e", ""},
	{"centerThird", "", "ctrl+alt+f", ""},
	{"lastTwoThirds", "", "ctrl+alt+t", ""},
	{"lastThird", "", "ctrl+alt+g", ""},
	// Only in the recommended set of a Rectangle first installed after
	// version 94 (rectangleNewDefaults).
	{"centerTwoThirds", "", "ctrl+alt+r", ""},
}

// rectangleNewDefaults is the last version of Rectangle whose recommended
// set has no shortcut for centerTwoThirds: Rectangle gives it one only
// when its installVersion is later.
const rectangleNewDefaults = 94

// rectangleKeys returns Rectangle's shortcuts, from its preferences: the
// ones the user set, and the defaults of the set they chose as Rectangle
// first opened (alternateDefaultShortcuts) for the others. Without that
// choice, as before Rectangle has first opened, or when Bench can't read
// its preferences, it reports false.
func rectangleKeys(p platform.ToolPreferences) ([]string, bool) {
	recommended, chose := p.Numbers["alternateDefaultShortcuts"]
	if !chose {
		return nil, false
	}
	var keys []string
	add := func(s string) {
		if s != "" && !slices.Contains(keys, s) {
			keys = append(keys, s)
		}
	}
	aliases := map[string]string{}
	for _, a := range rectangleActions {
		if a.alias != "" {
			aliases[a.alias] = a.name
		}
	}
	// The user's own, apart from an old name's when the action has its
	// own as well, which is the one Rectangle uses.
	for _, key := range slices.Sorted(maps.Keys(p.Shortcuts)) {
		if name, old := aliases[key]; old {
			if _, both := p.Shortcuts[name]; both {
				continue
			}
		}
		add(p.Shortcuts[key])
	}
	newer := false
	if v, err := strconv.Atoi(strings.TrimSpace(p.Texts["installVersion"])); err == nil {
		newer = v > rectangleNewDefaults
	}
	for _, a := range rectangleActions {
		_, set := p.Shortcuts[a.name]
		_, setOld := p.Shortcuts[a.alias]
		if set || (a.alias != "" && setOld) {
			continue
		}
		switch {
		case recommended == 0:
			add(a.spectacle)
		case a.name != "centerTwoThirds" || newer:
			add(a.recommended)
		}
	}
	return keys, true
}

// windowToolFor returns the window tool with a bundle ID, whatever its
// case.
func windowToolFor(bundleID string) (windowTool, bool) {
	for _, t := range windowTools {
		if strings.EqualFold(t.bundleID, bundleID) {
			return t, true
		}
	}
	return windowTool{}, false
}

// windowToolNamesLocked names the window tools that are running, apart
// from the app with bundle ID app, each once, in windowTools' order.
func (e *Engine) windowToolNamesLocked(app string) []string {
	var names []string
	for _, t := range windowTools {
		if strings.EqualFold(t.bundleID, app) || slices.Contains(names, t.name) {
			continue
		}
		if _, running := e.running[strings.ToLower(t.bundleID)]; running {
			names = append(names, t.name)
		}
	}
	return names
}

// runningTool is a window tool that's running: the tool, and its
// shortcuts, if Bench could read them (read).
type runningTool struct {
	tool windowTool
	keys []string
	read bool
}

// readKeys reads the tool's shortcuts from its preferences, afresh, and
// reports whether they changed.
func (r *runningTool) readKeys(p platform.Platform) bool {
	if r.tool.keys == nil {
		return false
	}
	keys, read := r.tool.keys(p.ToolPreferences(r.tool.bundleID))
	if read == r.read && slices.Equal(keys, r.keys) {
		return false
	}
	r.keys, r.read = keys, read
	return true
}

// noteWindowToolsLocked looks for the window tools among the running apps,
// which the caller has just loaded (e.running): a tool that has started
// gets its shortcuts read, and, unless the user has dismissed its notice
// before (toolsTold), a notice; one that has quit is forgotten, with its
// notice. With reread, as Bench becomes the active app, after the user may
// have changed a tool's shortcuts in the tool, it reads them again for the
// tools that are running. macOS doesn't announce apps such as these, which
// live in the menu bar, as they open or quit, so Bench looks as it starts,
// as it arranges, and as it becomes the active app. When the shortcuts
// that Bench knows change, the shell checks Bench's against them again
// (WindowToolKeys).
func (e *Engine) noteWindowToolsLocked(reread bool) {
	changed, keys := false, false
	for _, t := range windowTools {
		key := strings.ToLower(t.bundleID)
		_, running := e.running[key]
		on := e.toolsOn[key]
		switch {
		case running && on == nil:
			on = &runningTool{tool: t}
			on.readKeys(e.p)
			e.log.Debug("a window tool is running", "app", t.bundleID, "readKeys", on.read, "keys", len(on.keys), "told", e.toolsTold[key])
			e.toolsOn[key] = on
			changed, keys = true, keys || on.read
		case running && reread:
			if on.readKeys(e.p) {
				e.log.Debug("a window tool's shortcuts changed", "app", t.bundleID, "readKeys", on.read, "keys", len(on.keys))
				changed, keys = true, true
			}
		case !running && on != nil:
			delete(e.toolsOn, key)
			changed, keys = true, keys || on.read
		}
	}
	if keys {
		e.later(e.em.ShortcutsChanged)
	}
	if changed {
		e.touch()
	}
}

// WindowToolKeys returns the shortcuts of the window tools that are
// running, for those whose shortcuts Bench could read, in windowTools'
// order, for the shell to check Bench's against as it registers them, so
// that Settings ▸ Keyboard doesn't say that a shortcut works when a tool
// uses it too, and suggests none that a tool uses (decision 69).
func (e *Engine) WindowToolKeys() []ToolKeys {
	e.lock()
	defer e.unlock()
	var out []ToolKeys
	for _, t := range windowTools {
		if on := e.toolsOn[strings.ToLower(t.bundleID)]; on != nil && on.read && len(on.keys) > 0 {
			out = append(out, ToolKeys{Name: t.name, Keys: slices.Clone(on.keys)})
		}
	}
	return out
}

// windowToolNoticesLocked returns the notices of the window tools that are
// running and whose notice the user hasn't dismissed, in windowTools'
// order, each with Keyboard settings and Dismiss, and a key that stays
// the same as its text follows Bench's shortcuts.
func (e *Engine) windowToolNoticesLocked() []NoticeView {
	var out []NoticeView
	for _, t := range windowTools {
		key := strings.ToLower(t.bundleID)
		on := e.toolsOn[key]
		if on == nil || e.toolsTold[key] {
			continue
		}
		out = append(out, NoticeView{
			Key:  "window-tool:" + key,
			Text: windowToolText(t.name, toolClashes(e.cfg.Shortcuts, on.keys)),
			Actions: []NoticeAction{
				{Label: "Keyboard settings", Command: "keyboard-settings"},
				{Label: "Dismiss", Method: "DismissWindowTool", Arg: t.bundleID},
			},
		})
	}
	return out
}

// toolClashes names Bench's shortcuts that a window tool's keys take, as
// the bindings of cfg make them, by what they do: "Focus a pane (⌃⌥←,
// ⌃⌥→)" for each field with any, in the order Bindings has them.
func toolClashes(cfg shortcuts.Config, keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	bindings, _ := shortcuts.Bindings(cfg)
	var fields []string
	byField := map[string][]string{}
	for _, b := range bindings {
		if !slices.Contains(keys, b.Shortcut) {
			continue
		}
		if _, ok := byField[b.Field]; !ok {
			fields = append(fields, b.Field)
		}
		byField[b.Field] = append(byField[b.Field], b.Display)
	}
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = shortcuts.Label(strings.TrimPrefix(f, "shortcuts.")) + " (" + strings.Join(byField[f], ", ") + ")"
	}
	return out
}

// windowToolText is a window tool's notice: what it can do to Bench, and,
// when Bench could read its shortcuts and some of them are Bench's, which,
// in short sentences that end with what to do. The first two still make
// sense when the status bar cuts the notice short. It doesn't say that
// none clash when it found none: the tool may have shortcuts that Bench
// can't read, and the user may change them.
func windowToolText(name string, clashes []string) string {
	text := name + " is running. It can move docked windows out of their panes. "
	if len(clashes) == 0 {
		return text + "Its shortcuts can use the same keys as Bench's. " +
			"macOS doesn't warn about this, and both apps may act on such a key. " +
			"Check the keys in " + name + " and in Keyboard settings."
	}
	return text + "It uses the same keys as Bench for " + joinNames(clashes) + ". " +
		"macOS doesn't warn about this, and both apps may act on these keys. " +
		"Change the keys in " + name + " or in Keyboard settings."
}

// DismissWindowTool is the Dismiss of a window tool's notice: the notice
// goes, and Bench doesn't show it again, as state.json remembers. Until
// the user dismisses it, the notice shows whenever the tool runs, so that
// a tool that quits before the user has read it gets its notice again.
func (e *Engine) DismissWindowTool(bundleID string) error {
	e.lock()
	defer e.unlock()
	key := strings.ToLower(bundleID)
	if _, known := windowToolFor(key); !known || e.toolsTold[key] {
		return nil
	}
	e.toolsTold[key] = true
	e.changed()
	return nil
}

// demoTools is what the browser preview's platform adds, for simulating a
// window tool: one that starts and stops without an event, as a window
// tool does on a Mac, and moves another app's window.
type demoTools interface {
	StartAgent(bundleID string) error
	StopAgent(bundleID string) error
	AppMoveWindow(id platform.WindowID, frame layout.Rect) error
}

// SetDemoWindowTool starts the browser preview's window tool, Rectangle
// (platform.DemoWindowTool), or stops it, as the demo bar's Rectangle
// does: the first time it runs, the status bar says what it can do to
// Bench, and a window that keeps moving out of its pane names it. It needs
// the preview's platform (platform.NewDemo).
func (e *Engine) SetDemoWindowTool(on bool) error {
	dt, ok := e.p.(demoTools)
	if !ok || !e.opts.Demo {
		return errNoDemoTools
	}
	e.lock()
	defer e.unlock()
	e.loadRunning()
	if _, running := e.running[strings.ToLower(platform.DemoWindowTool)]; running != on {
		start := dt.StopAgent
		if on {
			start = dt.StartAgent
		}
		if err := start(platform.DemoWindowTool); err != nil {
			return err
		}
		e.loadRunning()
	}
	e.noteWindowToolsLocked(false)
	e.touch()
	return nil
}

// demoToolGap is how long the browser preview's window tool waits between
// its moves of a window (DemoToolMoves): well past Bench putting the
// window back (redockDelay), and the time after that in which a move is
// taken as Bench's own (windows.Manager.OwnChange), so that each move is a
// try of its own however late the redock runs, and well within the time
// that Bench counts the tries in (redockMemory).
const demoToolGap = time.Second

// DemoToolMoves has the browser preview's window tool move the window that
// the focused pane shows to the left half of the screen, as Rectangle's
// shortcut would, once more than Bench puts it back (redockLimit),
// demoToolGap apart, so that the pane says that something keeps moving it.
// The platform reports each move, as macOS would. While the screens
// settle, as after the demo bar's screen picker, Bench doesn't count the
// moves and docks the window again afterwards, so the tool waits until they
// have settled. It works whether the demo bar's Rectangle runs or not: an
// app can move its own window too. It needs the preview's platform
// (platform.NewDemo).
func (e *Engine) DemoToolMoves() error {
	dt, ok := e.p.(demoTools)
	if !ok || !e.opts.Demo {
		return errNoDemoTools
	}
	e.lock()
	defer e.unlock()
	g, ok := e.displayLocked().focusedGeom()
	if !ok {
		return errNoToolMove
	}
	it := e.model.Items[g.node.Active]
	if !dockable(it) {
		return errNoToolMove
	}
	id := it.WindowID
	vis := e.p.ScreenOfWindow().Visible
	frame := layout.Rect{X: vis.X, Y: vis.Y, W: math.Floor(vis.W / 2), H: vis.H}
	var move func(left int)
	move = func(left int) {
		if it := e.model.ItemByWindow(id); it == nil || it.Undocked {
			return
		}
		if e.screenTimer != nil {
			e.after(demoToolGap, func() { move(left) })
			return
		}
		if err := dt.AppMoveWindow(id, frame); err != nil {
			e.log.Debug("the demo's window tool couldn't move a window", "window", id, "error", err)
			return
		}
		if left > 1 {
			e.after(demoToolGap, func() { move(left - 1) })
		}
	}
	move(redockLimit + 1)
	return nil
}

// demoToolTimeout is how long DemoToolMoves takes at most once the
// screens have settled, for tests.
const demoToolTimeout = time.Duration(redockLimit+1) * demoToolGap
