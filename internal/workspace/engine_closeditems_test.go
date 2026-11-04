package workspace

import (
	"bytes"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/shortcuts"
	"github.com/itbm/bench/internal/term"
)

// TestUndoCloseItem checks that Undo after closing a web page puts it back
// in its place, at the same address, and that Bench no longer offers to
// reopen it once it's back.
func TestUndoCloseItem(t *testing.T) {
	h, ids := closeHarness(t, "")
	docs := ids["Docs"]
	pane := h.paneOf(docs)
	mustOK(t, h.e.ShowItem(pane, docs))
	before := layout.Clone(h.tabNamed("Code").Root)
	url := h.item(docs).URL

	_, err := h.e.CloseItem(docs, false, false)
	mustOK(t, err)
	if got := h.em.lastToast(); got != "Closed Docs." {
		t.Errorf("toast %q", got)
	}
	step := h.em.lastUndo(t)
	v := h.view()
	if got := v.ClosedItems; len(got) != 1 || got[0] != (ClosedItemView{ID: docs, Kind: KindWeb, Label: "Docs", Tab: "Code"}) {
		t.Errorf("ClosedItems = %+v, want Docs in Code", got)
	}
	if v.Undo == nil || *v.Undo != (UndoView{Step: step, Name: "Close Page", Did: "closed Docs"}) {
		t.Errorf("Undo = %+v, want Close Page", v.Undo)
	}
	if m := h.menuBarNow(); m.ClosedItem != docs || m.ClosedTerminal || m.Undo != "Close Page" {
		t.Errorf("menu bar: closed item %q, terminal %v, undo %q", m.ClosedItem, m.ClosedTerminal, m.Undo)
	}

	mustOK(t, h.e.Undo(step))
	h.drain()
	sameTree(t, h, "Code", before)
	if it := h.item(docs); it.Kind != KindWeb || it.URL != url || it.Title != "Docs" {
		t.Errorf("the page came back as %+v", it)
	}
	if !h.called("ShowWebView", func(c platform.Call) bool { return c.Arg == docs }) {
		t.Error("the page's view wasn't made again")
	}
	if got := h.em.lastToast(); got != "Reopened Docs." {
		t.Errorf("toast %q", got)
	}
	v = h.view()
	if len(v.ClosedItems) != 0 || v.Undo != nil {
		t.Errorf("after Undo: ClosedItems %+v, Undo %+v; want neither", v.ClosedItems, v.Undo)
	}
	mustErr(t, h.e.ReopenItem(docs), errNoClosedItem.Error())
}

// TestUndoLatest checks Tab ▸ Undo, the jump palette's Undo row, and the
// undo shortcut: they take back the latest change, and say what it did.
func TestUndoLatest(t *testing.T) {
	h, ids := closeHarness(t, "[shortcuts]\nundo = \"ctrl+alt+u\"\n")
	mustErr(t, h.e.UndoLatest(), errNothingToUndo.Error())
	if h.view().Undo != nil || h.menuBarNow().Undo != "" {
		t.Error("with nothing to undo, the View and the menu bar should offer nothing")
	}
	if got := h.view().Keys.Undo; got != "⌃⌥U" {
		t.Errorf("Keys.Undo = %q, want ⌃⌥U", got)
	}

	// Two changes: the latest is taken back first, and the other after it.
	mustOK(t, h.e.MoveTab(1, 0))
	_, err := h.e.CloseItem(ids["Docs"], false, false)
	mustOK(t, err)
	if u := h.view().Undo; u == nil || u.Name != "Close Page" {
		t.Fatalf("Undo = %+v, want Close Page", u)
	}
	mustOK(t, h.e.UndoLatest())
	h.drain()
	if got := h.em.lastToast(); got != "Undid: closed Docs." {
		t.Errorf("toast %q", got)
	}
	if _, ok := h.view().Items[ids["Docs"]]; !ok {
		t.Error("the page isn't back")
	}
	if u := h.view().Undo; u == nil || u.Name != "Move Tab" || u.Did != "moved Notes" {
		t.Fatalf("Undo = %+v, want Move Tab", u)
	}

	// The shortcut does the same, from another app, and a hidden Bench
	// comes back to show it.
	h.p.EmitEvent(platform.Event{Kind: platform.BenchHidden})
	h.drain()
	forwards := h.em.broughtForward()
	h.e.Shortcut(shortcuts.ActionUndo, "")
	h.drain()
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Code", "Notes"}) {
		t.Errorf("tabs %v, want Notes moved back", got)
	}
	if got := h.em.lastToast(); got != "Undid: moved Notes." {
		t.Errorf("toast %q", got)
	}
	if h.em.broughtForward() != forwards+1 {
		t.Error("the undo shortcut should bring a hidden Bench forward")
	}
	// With nothing to undo, a hidden Bench comes back to say so.
	h.e.Shortcut(shortcuts.ActionUndo, "")
	h.drain()
	if got := h.em.lastToast(); got != "There's nothing to undo." {
		t.Errorf("toast %q", got)
	}
	if h.em.broughtForward() != forwards+2 {
		t.Error("the undo shortcut should bring a hidden Bench forward to say there's nothing to undo")
	}
}

// TestReopenItem checks Reopen Closed Page or Terminal: a terminal comes
// back in its place, in its folder, with the output it had, and switches
// to its tab; and a page whose tab has gone comes back in the current
// tab's focused pane.
func TestReopenItem(t *testing.T) {
	h, ids := closeHarness(t, "")
	claude := ids["claude"]
	pane := h.paneOf(claude)
	index := slices.Index(h.e.model.Tabs[0].Pane(pane).Items, claude)
	h.terms.snaps[claude] = term.Snapshot{Data: []byte("$ claude\r\nDone.\r\n")}
	_, err := h.e.CloseItem(claude, true, false)
	mustOK(t, err)
	if _, ok := h.terms.started[claude]; ok {
		t.Fatal("the terminal's session didn't end")
	}
	if got := h.view().ClosedItems; len(got) != 1 || got[0].Kind != KindBuiltin || got[0].Label != "Terminal — claude" {
		t.Errorf("ClosedItems = %+v", got)
	}
	if m := h.menuBarNow(); m.ClosedItem != claude || !m.ClosedTerminal {
		t.Errorf("menu bar: closed item %q, terminal %v", m.ClosedItem, m.ClosedTerminal)
	}
	mustOK(t, h.e.SwitchTab(1))

	mustOK(t, h.e.ReopenItem(claude))
	h.drain()
	v := h.view()
	if v.Current != 0 {
		t.Errorf("current tab %d, want Code, where the terminal was", v.Current)
	}
	if got := h.paneOf(claude); got != pane || slices.Index(h.e.model.Tabs[0].Pane(pane).Items, claude) != index {
		t.Errorf("the terminal came back in %s, want %s at %d", got, pane, index)
	}
	opts, ok := h.terms.started[claude]
	if !ok || opts.Dir != "~/code" {
		t.Fatalf("the terminal started %v, with %+v; want in ~/code", ok, opts)
	}
	if !bytes.Contains(opts.Preface, []byte("Done.")) || !strings.Contains(string(opts.Preface), "Restored output from") {
		t.Errorf("preface %q, want the output it had", opts.Preface)
	}
	if got := h.em.lastToast(); got != "Reopened Terminal — claude." {
		t.Errorf("toast %q", got)
	}
	if len(v.ClosedItems) != 0 || v.Undo != nil {
		t.Errorf("ClosedItems = %+v, Undo %+v; want neither: the terminal is back", v.ClosedItems, v.Undo)
	}
	mustErr(t, h.e.ReopenItem(claude), errNoClosedItem.Error())

	// A page whose tab has gone comes back in the current tab.
	_, err = h.e.CloseItem(ids["Docs"], false, false)
	mustOK(t, err)
	mustOK(t, h.e.DeleteTab(0))
	h.drain()
	mustOK(t, h.e.ReopenItem(ids["Docs"]))
	h.drain()
	v = h.view()
	focused := h.e.model.CurrentTab().FocusedPane()
	if focused == nil || !slices.Contains(focused.Items, ids["Docs"]) || v.Tabs[v.Current].Name != "Notes" {
		t.Errorf("the page came back in %s, want Notes's focused pane", h.paneOf(ids["Docs"]))
	}
}

// TestReopenItemWithoutOutput checks that a terminal's output isn't kept
// with keep_terminal_output off, and is forgotten when it's turned off
// after the terminal closed.
func TestReopenItemWithoutOutput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings string
		turnOff  bool
	}{
		{"setting off", "[settings]\nkeep_terminal_output = false\n", false},
		{"turned off after", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, ids := closeHarness(t, tc.settings)
			claude := ids["claude"]
			h.terms.snaps[claude] = term.Snapshot{Data: []byte("secret\r\n")}
			_, err := h.e.CloseItem(claude, true, false)
			mustOK(t, err)
			if tc.turnOff {
				s := h.view().Settings
				s.KeepTerminalOutput = false
				mustOK(t, h.e.SaveSettings(s))
			}
			mustOK(t, h.e.ReopenItem(claude))
			h.drain()
			opts, ok := h.terms.started[claude]
			if !ok || opts.Preface != nil {
				t.Errorf("the terminal started %v, with preface %q; want none", ok, opts.Preface)
			}
		})
	}
}

// TestClosedItemsKeepTen checks that Bench keeps the last ten closed pages
// and terminals, the latest first.
func TestClosedItemsKeepTen(t *testing.T) {
	h := newHarness(t, setup{})
	h.e.lock()
	for i := range closedItemsKept + 2 {
		h.e.noteClosedItemLocked(&closedItem{id: "x" + string(rune('a'+i)), label: "Page"})
	}
	h.e.unlock()
	v := h.view()
	if len(v.ClosedItems) != closedItemsKept || v.ClosedItems[0].ID != "xl" || v.ClosedItems[closedItemsKept-1].ID != "xc" {
		t.Errorf("ClosedItems = %+v, want the last %d, the latest first", v.ClosedItems, closedItemsKept)
	}
}

// TestUndoCloseTerminal checks that Undo after closing a terminal starts it
// again in its place, in its folder, with the output it had.
func TestUndoCloseTerminal(t *testing.T) {
	h, ids := closeHarness(t, "")
	claude := ids["claude"]
	pane := h.paneOf(claude)
	mustOK(t, h.e.ShowItem(pane, claude))
	before := layout.Clone(h.tabNamed("Code").Root)
	h.terms.snaps[claude] = term.Snapshot{Data: []byte("$ claude\r\nDone.\r\n")}
	_, err := h.e.CloseItem(claude, true, false)
	mustOK(t, err)
	if _, ok := h.terms.started[claude]; ok {
		t.Fatal("the terminal's session didn't end")
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	h.drain()
	sameTree(t, h, "Code", before)
	opts, ok := h.terms.started[claude]
	if !ok || opts.Dir != "~/code" {
		t.Fatalf("the terminal started %v, with %+v; want in ~/code", ok, opts)
	}
	if !bytes.Contains(opts.Preface, []byte("Done.")) || !strings.Contains(string(opts.Preface), "Restored output from") {
		t.Errorf("preface %q, want the output it had", opts.Preface)
	}
	if got := h.em.lastToast(); got != "Reopened Terminal — claude." {
		t.Errorf("toast %q", got)
	}
	if v := h.view(); len(v.ClosedItems) != 0 {
		t.Errorf("ClosedItems = %+v, want none: the terminal is back", v.ClosedItems)
	}
}

// TestUndoKeepsLaterPageForReopen checks that Undo of a change that came
// before a page was added and then closed takes the close back too, but
// leaves the page closed, as the arrangement it puts back had no place for
// it, and keeps it for Reopen Closed Page or Terminal.
func TestUndoKeepsLaterPageForReopen(t *testing.T) {
	h, ids := closeHarness(t, "")
	mustOK(t, h.e.MoveTab(1, 0))
	first := h.view().Undo.Step
	pane := h.paneOf(ids["Docs"])
	mustOK(t, h.e.AddToPane(pane, Source{Type: sourceWeb, URL: "https://example.com/new"}))
	added := ""
	for id, it := range h.view().Items {
		if it.Kind == KindWeb && id != ids["Docs"] {
			added = id
		}
	}
	if added == "" {
		t.Fatal("no new page")
	}
	_, err := h.e.CloseItem(added, false, false)
	mustOK(t, err)

	mustOK(t, h.e.Undo(first))
	h.drain()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Code", "Notes"}) {
		t.Errorf("tabs %v, want Notes moved back", got)
	}
	if _, ok := v.Items[added]; ok {
		t.Error("the new page came back, though the arrangement before the move had no place for it")
	}
	if len(v.ClosedItems) != 1 || v.ClosedItems[0].ID != added || v.Undo != nil {
		t.Fatalf("ClosedItems = %+v, Undo %+v; want the page, and nothing to undo", v.ClosedItems, v.Undo)
	}
	mustOK(t, h.e.ReopenItem(added))
	h.drain()
	if got := h.paneOf(added); got != pane {
		t.Errorf("the page came back in %s, want %s", got, pane)
	}
}

// TestReopenItemClosedEverywhere checks that a page closed from every tab
// it was in comes back in each of them.
func TestReopenItemClosedEverywhere(t *testing.T) {
	h, ids := closeHarness(t, "")
	docs := ids["Docs"]
	mustOK(t, h.e.AddItemToTab(docs, 1))
	h.drain()
	h.e.lock()
	want := h.e.model.TabsOf(docs)
	h.e.unlock()
	if len(want) != 2 {
		t.Fatalf("Docs is in tabs %v, want Code and Notes", want)
	}
	_, err := h.e.CloseItem(docs, false, false)
	mustOK(t, err)
	if c := h.view().ClosedItems; len(c) != 1 || c[0].Tab != "Code" {
		t.Fatalf("ClosedItems = %+v, want Docs, from Code first", c)
	}
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.ReopenItem(docs))
	h.drain()
	h.e.lock()
	got := h.e.model.TabsOf(docs)
	h.e.unlock()
	if !slices.Equal(got, want) {
		t.Errorf("Docs came back in tabs %v, want %v", got, want)
	}
	if v := h.view(); v.Current != 0 {
		t.Errorf("current tab %d, want Code, the first that had it", v.Current)
	}
}

// TestReopenItemDropsItsStep checks that Reopen drops the change that
// closed the page from the middle of what Undo can take back, and that
// UndoLatest then takes back the changes before and after it, leaving the
// page where it is.
func TestReopenItemDropsItsStep(t *testing.T) {
	h, ids := closeHarness(t, "")
	docs := ids["Docs"]
	before := layout.Clone(h.tabNamed("Code").Root)
	mustOK(t, h.e.MoveTab(1, 0))
	_, err := h.e.CloseItem(docs, false, false)
	mustOK(t, err)
	mustOK(t, h.e.MoveTab(1, 0))
	if u := h.view().Undo; u == nil || u.Did != "moved Code" {
		t.Fatalf("Undo = %+v, want Move Tab, of Code", u)
	}

	mustOK(t, h.e.ReopenItem(docs))
	h.drain()
	if u := h.view().Undo; u == nil || u.Did != "moved Code" {
		t.Fatalf("after Reopen, Undo = %+v, want Move Tab still", u)
	}
	mustOK(t, h.e.UndoLatest())
	h.drain()
	if got := h.em.lastToast(); got != "Undid: moved Code." {
		t.Errorf("toast %q", got)
	}
	if u := h.view().Undo; u == nil || u.Did != "moved Notes" {
		t.Fatalf("Undo = %+v, want the first Move Tab, of Notes", u)
	}
	mustOK(t, h.e.UndoLatest())
	h.drain()
	v := h.view()
	if got := tabNames(v); !slices.Equal(got, []string{"Code", "Notes"}) {
		t.Errorf("tabs %v, want them as they were", got)
	}
	sameTree(t, h, "Code", before)
	if v.Undo != nil || len(v.ClosedItems) != 0 {
		t.Errorf("Undo %+v, ClosedItems %+v; want neither", v.Undo, v.ClosedItems)
	}
	mustErr(t, h.e.UndoLatest(), errNothingToUndo.Error())
}

// TestReopenItemInItsOwnPane checks that a page that had a pane of its own,
// which closed up with it, comes back in a pane of its own, on the side it
// was on, not over another pane's windows.
func TestReopenItemInItsOwnPane(t *testing.T) {
	h, ids := closeHarness(t, "")
	_, err := h.e.CloseItem(ids["claude"], true, false)
	mustOK(t, err)
	docs := ids["Docs"]
	_, err = h.e.CloseItem(docs, false, false)
	mustOK(t, err)
	if got := h.tabNamed("Code").Panes; got != 1 {
		t.Fatalf("Code has %d panes, want 1 once Docs's pane closed up", got)
	}
	mustOK(t, h.e.ReopenItem(docs))
	h.drain()
	root := h.tabNamed("Code").Root
	leaves := layout.Leaves(root)
	if !root.IsSplit() || root.Dir != layout.Row || len(leaves) != 2 ||
		!slices.Equal(leaves[0].Items, []string{ids["Zed"], ids["TextEdit"]}) || !slices.Equal(leaves[1].Items, []string{docs}) {
		t.Errorf("Code's tree %s, want Zed and TextEdit, then Docs on its own on the right", treeText(root))
	}
	if math.Abs(root.Sizes[0]-50) > 0.01 {
		t.Errorf("sizes %v, want 50, 50, as before", root.Sizes)
	}
	if v := h.view(); v.Items[docs].Pane != leaves[1].ID || v.Tabs[0].Focus != leaves[1].ID {
		t.Errorf("Docs is in %s, the focus in %s; want both %s", v.Items[docs].Pane, v.Tabs[0].Focus, leaves[1].ID)
	}
}

// TestReopenItemInItsOwnPaneKeepsSizes checks that a page that had the
// middle pane of three comes back there, at the size it had, and the other
// panes go back to theirs.
func TestReopenItemInItsOwnPaneKeepsSizes(t *testing.T) {
	h := newHarness(t, setup{world: closeWorld, config: `
[[tab]]
name = "Code"
layout = "three"
sizes = [40, 30, 30]
  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }]
  [[tab.pane]]
  items = [{ web = "https://example.com/docs", name = "Docs" }]
  [[tab.pane]]
  items = [{ terminal = "~/code", name = "claude" }]
`})
	var docs string
	for id, it := range h.view().Items {
		if it.Kind == KindWeb {
			docs = id
		}
	}
	before := layout.Clone(h.tabNamed("Code").Root)
	_, err := h.e.CloseItem(docs, false, false)
	mustOK(t, err)
	if got := h.tabNamed("Code").Panes; got != 2 {
		t.Fatalf("Code has %d panes, want 2 once Docs's pane closed up", got)
	}
	mustOK(t, h.e.ReopenItem(docs))
	h.drain()
	root := h.tabNamed("Code").Root
	got, want := layout.Leaves(root), layout.Leaves(before)
	if len(got) != 3 || !slices.Equal(got[1].Items, []string{docs}) || !slices.Equal(got[0].Items, want[0].Items) || !slices.Equal(got[2].Items, want[2].Items) {
		t.Fatalf("Code's tree %s, want Docs back in the middle", treeText(root))
	}
	for i, size := range []float64{40, 30, 30} {
		if math.Abs(root.Sizes[i]-size) > 0.01 {
			t.Errorf("sizes %v, want 40, 30, 30", root.Sizes)
			break
		}
	}
}

// TestBesideOf checks where a pane that closes up goes back: beside the
// nearest pane of its neighbour in its split, on its side.
func TestBesideOf(t *testing.T) {
	p := layout.NewPane
	row := func(children ...*layout.Node) *layout.Node {
		return layout.NewSplit("row", layout.Row, nil, children...)
	}
	col := func(children ...*layout.Node) *layout.Node {
		return layout.NewSplit("col", layout.Column, nil, children...)
	}
	for _, tc := range []struct {
		root   *layout.Node
		pane   string
		beside string
		side   layout.Zone
	}{
		{p("a"), "a", "", ""},
		{row(p("a"), p("b")), "a", "b", layout.Left},
		{row(p("a"), p("b")), "b", "a", layout.Right},
		{col(p("a"), row(p("b"), p("c"))), "a", "b", layout.Top},
		{row(col(p("a"), p("b")), p("c")), "c", "b", layout.Right},
		{col(p("a"), p("b"), p("c")), "c", "b", layout.Bottom},
	} {
		beside, side, _ := besideOf(tc.root, tc.pane)
		if beside != tc.beside || side != tc.side {
			t.Errorf("besideOf(%s, %s) = %q, %q; want %q, %q", treeText(tc.root), tc.pane, beside, side, tc.beside, tc.side)
		}
	}
}

// TestClosedItemBackWithItsTab checks that a page that Reopen Closed Tab
// brings back with its tab isn't offered to reopen any more, that closing
// it again keeps it once, and that Reopen of one that's open shows it.
func TestClosedItemBackWithItsTab(t *testing.T) {
	h, ids := closeHarness(t, "")
	docs := ids["Docs"]
	mustOK(t, h.e.AddItemToTab(docs, 1))
	_, err := h.e.CloseTab(0, true, false)
	mustOK(t, err)
	h.drain()
	if got := tabNames(h.view()); !slices.Equal(got, []string{"Notes"}) {
		t.Fatalf("tabs %v, want Notes alone", got)
	}
	_, err = h.e.CloseItem(docs, false, false)
	mustOK(t, err)
	if c := h.view().ClosedItems; len(c) != 1 || c[0].ID != docs {
		t.Fatalf("ClosedItems = %+v, want Docs", c)
	}

	mustOK(t, h.e.ReopenTab("Code"))
	h.drain()
	if _, ok := h.view().Items[docs]; !ok {
		t.Fatal("Reopen Closed Tab didn't bring Docs back with Code")
	}
	if c := h.view().ClosedItems; len(c) != 0 {
		t.Errorf("ClosedItems = %+v, want none: Docs is open again", c)
	}
	if m := h.menuBarNow(); m.ClosedItem != "" {
		t.Errorf("the menu bar offers to reopen %q", m.ClosedItem)
	}

	// Closed again, it's kept once.
	_, err = h.e.CloseItem(docs, false, false)
	mustOK(t, err)
	if c := h.view().ClosedItems; len(c) != 1 || c[0].ID != docs {
		t.Errorf("ClosedItems = %+v, want Docs once", c)
	}
	mustOK(t, h.e.ReopenItem(docs))
	h.drain()

	// Reopen of one that's open already, which a View from before it came
	// back can still name, shows it where it is.
	mustOK(t, h.e.SwitchTab(slices.IndexFunc(h.view().Tabs, func(tv TabView) bool { return tv.Name == "Notes" })))
	h.e.lock()
	h.e.closedItems = []*closedItem{{id: docs, label: "Docs"}}
	h.e.unlock()
	mustOK(t, h.e.ReopenItem(docs))
	h.drain()
	v := h.view()
	if v.Tabs[v.Current].Name != "Code" || h.activeIn(docs) != docs {
		t.Errorf("current tab %s, showing %q; want Docs shown in Code", v.Tabs[v.Current].Name, h.activeIn(docs))
	}
	if len(v.ClosedItems) != 0 {
		t.Errorf("ClosedItems = %+v, want none", v.ClosedItems)
	}
}
