package workspace

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// Bundle IDs of the apps that the closing tests add.
const (
	textEditID = "com.apple.TextEdit"
	slackID    = "com.tinyspeck.slackmacgap"
	musicID    = "com.apple.Music"
)

// closeConfig has two tabs. Code has Zed and TextEdit in one pane, and a
// terminal and a web page in the other, and a rule names it; Notes has
// Obsidian and Slack.
const closeConfig = `
[[tab]]
name = "Code"
layout = "side"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }, { app = "com.apple.TextEdit" }]
  [[tab.pane]]
  items = [{ terminal = "~/code", name = "claude" }, { web = "https://example.com/docs", name = "Docs" }]

[[tab]]
name = "Notes"
layout = "side"
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]
  [[tab.pane]]
  items = [{ app = "com.tinyspeck.slackmacgap" }]

[[rule]]
app = "com.apple.TextEdit"
title = "Untitled"
tab = "Code"
`

// closeWorld runs the apps of closeConfig, each with a window. TextEdit
// asks about unsaved changes as its window is asked to close, and Slack
// hides its window rather than close it.
func closeWorld(p *platform.Fake) {
	p.AddApp(benchID, "Bench", platform.AppNotInstalled())
	p.AddApp(zedID, "Zed")
	p.AddWindow(zedID, "main.go")
	p.AddApp(textEditID, "TextEdit")
	p.AddWindow(textEditID, "Notes", platform.WithCloseAsks())
	p.AddApp(obsidianID, "Obsidian")
	p.AddWindow(obsidianID, "Daily note")
	p.AddApp(slackID, "Slack")
	p.AddWindow(slackID, "Slack", platform.WithCloseHides())
}

// closeHarness starts an engine with closeConfig, after settings, and
// closeWorld, on the Code tab. It returns the items' IDs by app or
// sub-tab title: Zed, TextEdit, Obsidian, Slack, claude, and Docs.
func closeHarness(t *testing.T, settings string) (*harness, map[string]string) {
	t.Helper()
	h := newHarness(t, setup{config: settings + closeConfig, world: closeWorld})
	ids := map[string]string{
		"Zed":      h.itemOf(h.windowID(zedID, "main.go")),
		"TextEdit": h.itemOf(h.windowID(textEditID, "Notes")),
		"Obsidian": h.itemOf(h.windowID(obsidianID, "Daily note")),
		"Slack":    h.itemOf(h.windowID(slackID, "Slack")),
	}
	for _, it := range h.view().Items {
		if it.Kind != KindApp {
			ids[it.Title] = it.ID
		}
	}
	for _, name := range []string{"Zed", "TextEdit", "Obsidian", "Slack", "claude", "Docs"} {
		if ids[name] == "" {
			t.Fatalf("no item for %s: %v", name, ids)
		}
	}
	if h.view().Current != 0 {
		t.Fatalf("current tab %d, want Code", h.view().Current)
	}
	return h, ids
}

// pressed returns the windows that Bench asked to close, in order.
func (h *harness) pressed() []platform.WindowID {
	var out []platform.WindowID
	for _, c := range h.calls("PressClose") {
		out = append(out, c.Window)
	}
	return out
}

// activeIn returns the item that shows in an item's pane in the current
// tab, or "" for an item that isn't there.
func (h *harness) activeIn(itemID string) string {
	h.e.lock()
	defer h.e.unlock()
	if p := h.e.model.PaneIn(h.e.model.Current, itemID); p != nil {
		return p.Active
	}
	return ""
}

// TestCloseTabAsksFirst checks that Close tab asks before it closes two or
// more things, saying what closes and what stays, and closes nothing.
func TestCloseTabAsksFirst(t *testing.T) {
	h, ids := closeHarness(t, "")
	// Obsidian is in Code too: it stays in Notes.
	mustOK(t, h.e.AddItemToTab(ids["Obsidian"], 0))
	q, err := h.e.CloseTab(0, false, false)
	mustOK(t, err)
	want := CloseQuestion{Ask: true, Tab: "Code", Windows: 2, Pages: 1, Terminals: 1, Stay: 1, Listed: 4, Rules: 1}
	if !reflect.DeepEqual(q, want) {
		t.Errorf("CloseTab(0) = %+v, want %+v", q, want)
	}
	h.settle()
	if v := h.view(); len(v.Tabs) != 2 || len(h.pressed()) != 0 {
		t.Errorf("asking closed something: %d tabs, pressed %v", len(v.Tabs), h.pressed())
	}
	if _, ok := h.terms.started[ids["claude"]]; !ok {
		t.Error("asking ended the terminal")
	}
	mustErr(t, func() error { _, err := h.e.CloseTab(5, false, false); return err }(), "There's no tab 6.")
}

// TestCloseTab checks that Close tab closes the tab's pages and terminals,
// asks its App windows to close, deletes the tab and the rules that name
// it, and leaves a window that's in another tab too there.
func TestCloseTab(t *testing.T) {
	h, ids := closeHarness(t, "")
	mustOK(t, h.e.AddItemToTab(ids["Obsidian"], 0))
	zed, textEdit := h.windowID(zedID, "main.go"), h.windowID(textEditID, "Notes")
	q, err := h.e.CloseTab(0, true, false)
	mustOK(t, err)
	if q.Ask {
		t.Fatal("CloseTab(sure) asked")
	}
	h.drain()
	v := h.view()
	if len(v.Tabs) != 1 || v.Tabs[0].Name != "Notes" || v.Current != 0 {
		t.Fatalf("tabs %+v, current %d; want only Notes", v.Tabs, v.Current)
	}
	if got := h.pressed(); !slices.Equal(got, []platform.WindowID{zed, textEdit}) {
		t.Errorf("pressed %v, want Zed's and TextEdit's windows %v", got, []platform.WindowID{zed, textEdit})
	}
	for _, name := range []string{"Zed", "claude", "Docs"} {
		if _, ok := v.Items[ids[name]]; ok {
			t.Errorf("%s is still there", name)
		}
	}
	if _, ok := h.terms.started[ids["claude"]]; ok {
		t.Error("the terminal's session wasn't ended")
	}
	if it := h.item(ids["Obsidian"]); it.Tab != 0 || len(it.Tabs) != 0 {
		t.Errorf("Obsidian is in tab %d, tabs %v; want Notes alone", it.Tab, it.Tabs)
	}
	// TextEdit asks about unsaved changes: its window waits, "Not in a
	// tab", untouched.
	if !slices.Equal(floatingIDs(v), []string{ids["TextEdit"]}) {
		t.Errorf("Not in a tab: %v, want TextEdit", floatingIDs(v))
	}
	if got := h.em.lastToast(); got != "Closed Code. What's also in other tabs is still there."+
		" Undo brings back the tab, but not the windows, the page, and the terminal that closed." {
		t.Errorf("toast %q", got)
	}
	h.em.lastUndo(t)
	cfg := savedConfig(t, h)
	if len(cfg.Tabs) != 1 || len(cfg.Rules) != 0 {
		t.Errorf("config.toml has %d tabs and rules %+v; want Notes, no rules", len(cfg.Tabs), cfg.Rules)
	}
	if !savedSettings(t, h).ConfirmClose {
		t.Error("CloseTab(dontAsk false) turned confirm_close off")
	}
	if slices.Contains(h.focusCalls(), textEdit) {
		t.Error("TextEdit's window was focused before Bench looked for it again")
	}

	// After the grace, Bench shows the window that's still open.
	h.advance(closeGrace)
	if !slices.Contains(h.focusCalls(), textEdit) {
		t.Errorf("TextEdit's window wasn't focused: %v", h.focusCalls())
	}
	if got := h.em.lastToast(); got != "TextEdit may be asking about unsaved changes. "+notInTabText {
		t.Errorf("toast %q", got)
	}
	if !slices.Equal(floatingIDs(h.view()), []string{ids["TextEdit"]}) {
		t.Errorf("Not in a tab: %v, want TextEdit", floatingIDs(h.view()))
	}
	// Closed now, it goes.
	mustOK(t, h.p.CloseWindow(textEdit))
	h.settle()
	if _, ok := h.view().Items[ids["TextEdit"]]; ok {
		t.Error("TextEdit's item is still there after its window closed")
	}
}

// TestCloseTabHiddenApps checks that closing a tab that isn't the current
// one shows its hidden apps before Bench asks their windows to close, and
// that a window that's still open after the grace gets the keyboard.
func TestCloseTabHiddenApps(t *testing.T) {
	h, ids := closeHarness(t, "")
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if !h.p.IsHidden(textEditID) || !h.p.IsHidden(zedID) {
		t.Fatal("Code's apps aren't hidden on another tab")
	}
	textEdit := h.windowID(textEditID, "Notes")
	_, err := h.e.CloseTab(0, true, false)
	mustOK(t, err)
	h.drain()
	if v := h.view(); v.Current != 0 || v.Tabs[0].Name != "Notes" {
		t.Errorf("current %d, want Notes", v.Current)
	}
	if h.p.IsHidden(textEditID) || h.p.IsHidden(zedID) {
		t.Error("Bench asked windows of hidden apps to close")
	}
	var unhid, pressed int
	for i, c := range h.calls("Unhide", "PressClose") {
		switch {
		case c.Method == "Unhide" && c.BundleID == textEditID:
			unhid = i + 1
		case c.Method == "PressClose" && c.Window == textEdit:
			pressed = i + 1
		}
	}
	if unhid == 0 || pressed < unhid {
		t.Errorf("TextEdit shown at call %d, its window asked at %d; want shown first", unhid, pressed)
	}
	if slices.Contains(h.focusCalls(), textEdit) {
		t.Error("TextEdit's window was focused before Bench looked for it again")
	}
	h.advance(closeGrace)
	if !slices.Contains(h.focusCalls(), textEdit) {
		t.Error("TextEdit's window wasn't focused")
	}
	if _, ok := h.view().Items[ids["Zed"]]; ok {
		t.Error("Zed's item is still there")
	}
}

// TestCloseTabDontAskAgain checks that Don't ask again turns confirm_close
// off, in the same save, and that Close tab then closes without asking.
func TestCloseTabDontAskAgain(t *testing.T) {
	h, _ := closeHarness(t, "")
	_, err := h.e.CloseTab(1, true, true)
	mustOK(t, err)
	if savedSettings(t, h).ConfirmClose {
		t.Error("CloseTab(dontAsk) didn't save confirm_close off")
	}
	if h.view().Settings.ConfirmClose {
		t.Error("the view still says confirm_close is on")
	}
	if got := h.em.lastToast(); got != "Closed Notes. "+closeAskingText+" Undo brings back the tab, but not the windows that closed." {
		t.Errorf("toast %q", got)
	}
	// The only tab now, Code closes everything without asking.
	q, err := h.e.CloseTab(0, false, false)
	mustOK(t, err)
	if q.Ask || !q.Only {
		t.Errorf("CloseTab(0) = %+v, want the only tab closed without asking", q)
	}
}

// TestCloseOnlyTab checks that closing the only tab closes everything in
// it, and leaves it, with one empty pane.
func TestCloseOnlyTab(t *testing.T) {
	config := `
[[tab]]
name = "Code"
layout = "side"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
  [[tab.pane]]
  items = [{ terminal = "~/code", name = "claude" }, { web = "https://example.com/docs", name = "Docs" }]
`
	h := newHarness(t, setup{config: config, world: closeWorld})
	q, err := h.e.CloseTab(0, false, false)
	mustOK(t, err)
	want := CloseQuestion{Ask: true, Tab: "Code", Only: true, Windows: 1, Pages: 1, Terminals: 1, Listed: 3}
	if !reflect.DeepEqual(q, want) {
		t.Errorf("CloseTab(0) = %+v, want %+v", q, want)
	}
	_, err = h.e.CloseTab(0, true, false)
	mustOK(t, err)
	h.settle()
	v := h.view()
	if len(v.Tabs) != 1 || v.Tabs[0].Name != "Code" {
		t.Fatalf("tabs %+v, want Code", v.Tabs)
	}
	leaves := layout.Leaves(v.Tabs[0].Root)
	if len(leaves) != 1 || len(leaves[0].Items) != 0 {
		t.Errorf("Code has panes %+v, want one empty pane", leaves)
	}
	if got := h.em.lastToast(); got != "Closed everything in Code." {
		t.Errorf("toast %q", got)
	}
	if cfg := savedConfig(t, h); len(cfg.Tabs) != 1 || len(cfg.Tabs[0].Panes) != 2 {
		t.Errorf("config.toml changed: %+v", cfg.Tabs)
	}
}

// busyCloseConfig has a tab with only a terminal, and another with Zed.
const busyCloseConfig = `
[[tab]]
name = "Shell"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~/code", name = "claude" }]

[[tab]]
name = "Code"
layout = "single"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
`

// TestCloseTabBusyTerminal checks that Close tab asks before it ends a
// terminal that's running something, even alone, unless both confirm_close
// and confirm_quit_busy_terminals are off.
func TestCloseTabBusyTerminal(t *testing.T) {
	for _, tt := range []struct {
		settings string
		ask      bool
	}{
		{"", true},
		{"[settings]\nconfirm_close = false\n", true},
		{"[settings]\nconfirm_quit_busy_terminals = false\n", true},
		{"[settings]\nconfirm_close = false\nconfirm_quit_busy_terminals = false\n", false},
	} {
		h := newHarness(t, setup{config: tt.settings + busyCloseConfig, world: closeWorld})
		var id string
		for _, it := range h.view().Items {
			if it.Kind == KindBuiltin {
				id = it.ID
			}
		}
		// Not busy, one terminal closes without asking.
		h.e.lock()
		c := h.e.tabCloseLocked(0)
		h.e.unlock()
		if c.q.closes() != 1 || len(c.q.Busy) != 0 {
			t.Fatalf("Shell closes %d, busy %v; want the terminal, not busy", c.q.closes(), c.q.Busy)
		}
		h.terms.setBusy(id, "claude")
		q, err := h.e.CloseTab(0, false, false)
		mustOK(t, err)
		if q.Ask != tt.ask {
			t.Errorf("%q: CloseTab(0) asked %v, want %v", tt.settings, q.Ask, tt.ask)
		}
		if tt.ask && (len(q.Busy) != 1 || q.Busy[0].Running != "claude" || q.Busy[0].Tab != "Shell") {
			t.Errorf("busy terminals %+v, want claude in Shell", q.Busy)
		}
		if got := len(h.view().Tabs); got != map[bool]int{true: 2, false: 1}[tt.ask] {
			t.Errorf("%q: %d tabs after CloseTab(0)", tt.settings, got)
		}
	}
}

// TestCloseItem checks Close page and Close terminal, the question before
// ending a terminal that's running something, and Close on a place
// without a window.
func TestCloseItem(t *testing.T) {
	h, ids := closeHarness(t, "")
	pane := h.paneOf(ids["Docs"])
	mustOK(t, h.e.ShowItem(pane, ids["Docs"]))
	q, err := h.e.CloseItem(ids["Docs"], false, false)
	mustOK(t, err)
	if q.Ask || q.Pages != 1 {
		t.Errorf("CloseItem(page) = %+v, want it closed", q)
	}
	if _, ok := h.view().Items[ids["Docs"]]; ok {
		t.Error("the page is still there")
	}
	if !h.called("CloseWebView", func(c platform.Call) bool { return c.Arg == ids["Docs"] }) {
		t.Error("the page's view wasn't closed")
	}

	h.terms.setBusy(ids["claude"], "npm")
	q, err = h.e.CloseItem(ids["claude"], false, false)
	mustOK(t, err)
	if !q.Ask || len(q.Busy) != 1 || q.Busy[0].Running != "npm" {
		t.Errorf("CloseItem(busy terminal) = %+v, want the question", q)
	}
	if _, ok := h.terms.started[ids["claude"]]; !ok {
		t.Error("asking ended the terminal")
	}
	q, err = h.e.CloseItem(ids["claude"], true, true)
	mustOK(t, err)
	if q.Ask {
		t.Error("CloseItem(sure) asked")
	}
	if _, ok := h.terms.started[ids["claude"]]; ok {
		t.Error("the terminal's session wasn't ended")
	}
	if savedSettings(t, h).ConfirmQuitBusyTerminals {
		t.Error("Don't ask again didn't turn confirm_quit_busy_terminals off")
	}
	if got := h.em.lastToast(); got != stopAskingText {
		t.Errorf("toast %q", got)
	}

	// Obsidian has quit: its place goes.
	h.p.QuitApp(obsidianID)
	h.settle()
	if !h.item(ids["Obsidian"]).NoWindow {
		t.Fatal("Obsidian's item still has a window")
	}
	_, err = h.e.CloseItem(ids["Obsidian"], false, false)
	mustOK(t, err)
	if _, ok := h.view().Items[ids["Obsidian"]]; ok {
		t.Error("Obsidian's place is still there")
	}
	_, err = h.e.CloseItem("nope", false, false)
	mustErr(t, err, errGone.Error())
}

// TestCloseItemStillOpen checks that a window that's still open after the
// grace shows in its pane, with the keyboard, and that one that closes
// goes, with no toast.
func TestCloseItemStillOpen(t *testing.T) {
	h, ids := closeHarness(t, "")
	if h.activeIn(ids["TextEdit"]) != ids["Zed"] {
		t.Fatalf("the pane shows %s, want Zed", h.activeIn(ids["TextEdit"]))
	}
	_, err := h.e.CloseItem(ids["TextEdit"], false, false)
	mustOK(t, err)
	h.drain()
	if h.activeIn(ids["TextEdit"]) != ids["Zed"] {
		t.Error("the pane changed before Bench looked for the window again")
	}
	h.advance(closeGrace)
	if h.activeIn(ids["TextEdit"]) != ids["TextEdit"] {
		t.Errorf("the pane shows %s, want TextEdit", h.activeIn(ids["TextEdit"]))
	}
	textEdit := h.windowID(textEditID, "Notes")
	if fc := h.focusCalls(); len(fc) == 0 || fc[len(fc)-1] != textEdit {
		t.Errorf("focus calls %v, want TextEdit's window last", fc)
	}
	if got := h.em.lastToast(); got != "TextEdit may be asking about unsaved changes." {
		t.Errorf("toast %q", got)
	}

	before := h.em.lastToast()
	_, err = h.e.CloseItem(ids["Zed"], false, false)
	mustOK(t, err)
	h.advance(closeGrace)
	if _, ok := h.view().Items[ids["Zed"]]; ok {
		t.Error("Zed's item is still there")
	}
	if got := h.em.lastToast(); got != before {
		t.Errorf("toast %q after a window closed", got)
	}
}

// TestCloseWaitsForBenchToBeLowered checks that Bench asks a window to close
// only once its own window is lowered, so that a save sheet isn't under it.
func TestCloseWaitsForBenchToBeLowered(t *testing.T) {
	h, ids := closeHarness(t, "")
	zed := h.windowID(zedID, "main.go")
	mustOK(t, h.e.SetOverlay(true))
	_, err := h.e.CloseItem(ids["Zed"], false, false)
	mustOK(t, err)
	h.drain()
	if len(h.pressed()) != 0 {
		t.Fatalf("pressed %v while Bench was raised", h.pressed())
	}
	mustOK(t, h.e.SetOverlay(false))
	h.drain()
	if got := h.pressed(); !slices.Equal(got, []platform.WindowID{zed}) {
		t.Errorf("pressed %v, want Zed's window", got)
	}
	h.drain()
	if _, ok := h.view().Items[ids["Zed"]]; ok {
		t.Error("Zed's item is still there")
	}
}

// TestCloseWindowsThatHideOrQuit checks that a window that its app hides
// rather than close counts as closed, and that one whose app quits as it
// closes goes, rather than wait for the app to open again.
func TestCloseWindowsThatHideOrQuit(t *testing.T) {
	h, ids := closeHarness(t, "")
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	slack := h.windowID(slackID, "Slack")
	_, err := h.e.CloseItem(ids["Slack"], false, false)
	mustOK(t, err)
	h.advance(closeGrace)
	if _, ok := h.view().Items[ids["Slack"]]; ok {
		t.Error("Slack's item is still there after Slack hid its window")
	}
	if slices.Contains(h.focusCalls(), slack) || strings.Contains(h.em.lastToast(), "unsaved") {
		t.Error("Bench showed Slack's hidden window")
	}

	// Music quits as its last window closes.
	config := `
[[tab]]
name = "Music"
layout = "single"
  [[tab.pane]]
  items = [{ app = "com.apple.Music" }]

[[tab]]
name = "Code"
layout = "single"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
`
	h = newHarness(t, setup{config: config, world: func(p *platform.Fake) {
		closeWorld(p)
		p.AddApp(musicID, "Music", platform.AppQuitsWithLastWindow())
		p.AddWindow(musicID, "Music")
	}})
	music := h.itemOf(h.windowID(musicID, "Music"))
	if music == "" {
		t.Fatal("Music's window has no item")
	}
	_, err = h.e.CloseItem(music, false, false)
	mustOK(t, err)
	h.advance(closeGrace)
	if _, ok := h.view().Items[music]; ok {
		t.Error("Music's item waits for Music to open again, after Bench closed its window")
	}
}

// TestCloseCantAsk checks that a window that Bench can't ask to close is
// left open, and shown, with a toast that says why.
func TestCloseCantAsk(t *testing.T) {
	h, ids := closeHarness(t, "")
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	h.p.SetErr("PressClose", fmt.Errorf("closing: %w", platform.ErrOtherDesktop))
	_, err := h.e.CloseTab(0, true, false)
	mustOK(t, err)
	h.drain()
	v := h.view()
	if !slices.Contains(floatingIDs(v), ids["Zed"]) || !slices.Contains(floatingIDs(v), ids["TextEdit"]) {
		t.Errorf("Not in a tab: %v, want Zed and TextEdit", floatingIDs(v))
	}
	if h.p.IsHidden(zedID) {
		t.Error("Zed is still hidden, with its window open")
	}
	want := "Zed — main.go is on another desktop, so Bench couldn't close it."
	toasts := h.em.toastList()
	if !slices.ContainsFunc(toasts, func(s string) bool {
		return strings.HasPrefix(s, want) && strings.HasSuffix(s, notInTabText)
	}) {
		t.Errorf("toasts %q, want %q… %q", toasts, want, notInTabText)
	}
}

// toastList returns the toasts sent so far.
func (r *recEmitter) toastList() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.toasts)
}

// TestCloseTabOnlyBusyTerminalAsks checks that with confirm_close off, Close
// tab still asks before it ends a terminal that's running something, as
// confirm_quit_busy_terminals has it, and that the question's Don't ask
// again turns that setting off, not confirm_close.
func TestCloseTabOnlyBusyTerminalAsks(t *testing.T) {
	h := newHarness(t, setup{config: "[settings]\nconfirm_close = false\n" + busyCloseConfig, world: closeWorld})
	var id string
	for _, it := range h.view().Items {
		if it.Kind == KindBuiltin {
			id = it.ID
		}
	}
	h.terms.setBusy(id, "claude")
	q, err := h.e.CloseTab(0, false, false)
	mustOK(t, err)
	if !q.Ask || !q.BusyOnly || len(q.Busy) != 1 {
		t.Fatalf("CloseTab(0) = %+v, want the question, asked for the busy terminal", q)
	}
	if len(h.view().Tabs) != 2 {
		t.Error("asking closed the tab")
	}
	_, err = h.e.CloseTab(0, true, true)
	mustOK(t, err)
	if s := savedSettings(t, h); s.ConfirmQuitBusyTerminals || s.ConfirmClose {
		t.Errorf("saved confirm_quit_busy_terminals %v, confirm_close %v; want both off", s.ConfirmQuitBusyTerminals, s.ConfirmClose)
	}
	if got := h.em.lastToast(); got != "Closed Shell. "+stopAskingText+" Undo brings back the tab, but not the terminal that closed." {
		t.Errorf("toast %q", got)
	}
}

// TestCloseOnlyTabPlaces checks that closing the only tab when it holds
// only the places of apps that have quit forgets them, says so, and has
// Undo, which brings them back.
func TestCloseOnlyTabPlaces(t *testing.T) {
	config := `
[[tab]]
name = "Code"
layout = "side"
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
  [[tab.pane]]
  items = [{ app = "md.obsidian" }]
`
	h := newHarness(t, setup{config: config, world: closeWorld})
	h.p.QuitApp(zedID)
	h.p.QuitApp(obsidianID)
	h.settle()
	if n := len(h.view().Items); n != 2 {
		t.Fatalf("%d items, want the places of Zed and Obsidian", n)
	}
	q, err := h.e.CloseTab(0, false, false)
	mustOK(t, err)
	if q.Ask || q.closes() != 0 {
		t.Errorf("CloseTab(0) = %+v, want nothing closed, without asking", q)
	}
	if n := len(h.view().Items); n != 0 {
		t.Errorf("%d items, want none", n)
	}
	if got := h.em.lastToast(); got != "Closed everything in Code." {
		t.Errorf("toast %q", got)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	h.settle()
	if n := len(h.view().Items); n != 2 {
		t.Errorf("%d items after Undo, want the places back", n)
	}

	// With nothing in it, there's nothing to close, and nothing to undo.
	_, err = h.e.CloseTab(0, true, false)
	mustOK(t, err)
	_, err = h.e.CloseTab(0, false, false)
	mustOK(t, err)
	if got := h.em.lastToast(); got != "There's nothing to close in Code." {
		t.Errorf("toast %q", got)
	}
}

// TestCloseItemOnce checks that closing a window that Bench has asked to
// close already, as a second click on its × does, doesn't ask again, and
// that once Bench has found it still open, Close asks again.
func TestCloseItemOnce(t *testing.T) {
	h, ids := closeHarness(t, "")
	textEdit := h.windowID(textEditID, "Notes")
	for range 2 {
		_, err := h.e.CloseItem(ids["TextEdit"], false, false)
		mustOK(t, err)
	}
	h.drain()
	if got := h.pressed(); !slices.Equal(got, []platform.WindowID{textEdit}) {
		t.Errorf("pressed %v, want TextEdit's window once", got)
	}
	h.advance(closeGrace)
	_, err := h.e.CloseItem(ids["TextEdit"], false, false)
	mustOK(t, err)
	if got := h.pressed(); !slices.Equal(got, []platform.WindowID{textEdit, textEdit}) {
		t.Errorf("pressed %v, want TextEdit's window again once it was found open", got)
	}
}

// TestCloseItemGoneAlready checks that closing a window that has closed
// before WindowDestroyed says so removes its item, and that its pane shows,
// docks, and focuses what's next, as WindowDestroyed would.
func TestCloseItemGoneAlready(t *testing.T) {
	h, ids := closeHarness(t, "")
	zed, textEdit := h.windowID(zedID, "main.go"), h.windowID(textEditID, "Notes")
	pane := h.paneOf(ids["Zed"])
	mustOK(t, h.e.FocusPane(pane))
	h.settle()
	if h.activeIn(ids["Zed"]) != ids["Zed"] {
		t.Fatalf("the pane shows %s, want Zed", h.activeIn(ids["Zed"]))
	}
	// Zed's window closes, and Bench hasn't heard yet.
	mustOK(t, h.p.CloseWindow(zed))
	h.p.ResetCalls()
	_, err := h.e.CloseItem(ids["Zed"], false, false)
	mustOK(t, err)
	if _, ok := h.view().Items[ids["Zed"]]; ok {
		t.Error("Zed's item is still there")
	}
	if h.activeIn(ids["TextEdit"]) != ids["TextEdit"] {
		t.Errorf("the pane shows %s, want TextEdit", h.activeIn(ids["TextEdit"]))
	}
	if !h.called("Raise", func(c platform.Call) bool { return c.Window == textEdit }) {
		t.Errorf("TextEdit's window wasn't raised: %v", h.calls("Raise", "SetFrame"))
	}
	if !slices.Contains(h.focusCalls(), textEdit) {
		t.Errorf("TextEdit's window wasn't focused: %v", h.focusCalls())
	}
	h.drain()
	if h.frame(textEdit) != h.bodyOf(pane) {
		t.Errorf("TextEdit's window at %v, want the pane's body %v", h.frame(textEdit), h.bodyOf(pane))
	}
}

// TestCloseTabHiddenWindowStaysParked checks that a window that Bench had
// parked, and that its app hides rather than close, stays parked once its
// item goes, so that Bench puts it back on screen as it quits, rather than
// leave it off screen for its app to show there again.
func TestCloseTabHiddenWindowStaysParked(t *testing.T) {
	h := newHarness(t, setup{config: closeConfig, world: func(p *platform.Fake) {
		closeWorld(p)
		p.AddWindow(slackID, "Huddle")
	}})
	slack, huddle := h.windowID(slackID, "Slack"), h.windowID(slackID, "Huddle")
	// Slack's other window is in Code, which shows Slack, so its window in
	// Notes is parked rather than hidden with Slack.
	mustOK(t, h.e.MoveItemToTab(h.itemOf(huddle), 0))
	h.settle()
	if !h.e.win.IsParked(slack) || h.p.IsHidden(slackID) {
		t.Fatalf("Slack's window in Notes: parked %v, Slack hidden %v; want parked, shown", h.e.win.IsParked(slack), h.p.IsHidden(slackID))
	}
	item := h.itemOf(slack)
	_, err := h.e.CloseTab(1, true, false)
	mustOK(t, err)
	h.advance(closeGrace)
	if _, ok := h.view().Items[item]; ok {
		t.Fatal("Slack's item is still there after Slack hid its window")
	}
	if !h.e.win.IsParked(slack) {
		t.Error("the hidden window was forgotten where Bench had parked it")
	}
	// Slack shows it again; quitting puts it back on screen.
	mustOK(t, h.p.ReturnToDesktop(slack))
	h.drain()
	mustOK(t, h.e.win.RestoreAll())
	if got := h.frame(slack); !OnScreenForTest(got) {
		t.Errorf("Slack's window at %v after RestoreAll, want on screen", got)
	}
}

// TestCloseUndoText checks that the toast after Close tab names what Undo
// can't bring back, as the question's Details do, and says nothing about
// it when nothing closed, as for a tab whose windows are all in other tabs.
func TestCloseUndoText(t *testing.T) {
	for _, tc := range []struct {
		q    CloseQuestion
		want string
	}{
		{CloseQuestion{}, ""},
		{CloseQuestion{Stay: 2}, ""},
		{CloseQuestion{Windows: 1}, "Undo brings back the tab, but not the window that closed."},
		{CloseQuestion{Pages: 2, Terminals: 1}, "Undo brings back the tab, but not the pages and the terminal that closed."},
		{CloseQuestion{Windows: 3, Pages: 1, Terminals: 2}, "Undo brings back the tab, but not the windows, the page, and the terminals that closed."},
	} {
		if got := closeUndoText(tc.q); got != tc.want {
			t.Errorf("closeUndoText(%+v) = %q, want %q", tc.q, got, tc.want)
		}
	}
}
