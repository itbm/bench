package workspace

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// These tests check that the sub-tabs of windows that have closed are out
// of sight (phase 3): an App item without a window is hidden, unless Bench
// is opening its app for it, but it keeps its place in its pane, which
// stays, and the pane lists the apps it's waiting for.

// hidingPane returns a displayed pane, failing the test if it isn't drawn.
func hidingPane(h *harness, id string) PaneView {
	h.t.Helper()
	for _, p := range h.view().Display.Panes {
		if p.ID == id {
			return p
		}
	}
	h.t.Fatalf("pane %s isn't displayed", id)
	return PaneView{}
}

// hidingModelPane returns a pane of a tab's own tree, as the model holds
// it, hidden items and all, failing the test if there's no such pane.
func hidingModelPane(h *harness, tab, id string) *layout.Node {
	h.t.Helper()
	n, _, _ := layout.Find(h.tabNamed(tab).Root, id)
	if n == nil || !n.IsPane() {
		h.t.Fatalf("%s has no pane %s", tab, id)
	}
	return n
}

// hidingDisplayedItems returns every item the display tree draws, in
// reading order.
func hidingDisplayedItems(v View) []string {
	var out []string
	for _, p := range layout.Leaves(v.Display.Root) {
		out = append(out, p.Items...)
	}
	return out
}

// hidingRememberedApps returns the bundle IDs a displayed pane is waiting
// for, in order, with the item Open would fill for each.
func hidingRememberedApps(p PaneView) []string {
	var out []string
	for _, r := range p.Remembered {
		out = append(out, r.BundleID+"="+r.ItemID)
	}
	return out
}

// Quitting an app hides its sub-tabs in every tab that holds them, without
// changing the model: each keeps its place in its pane, and the panes stay,
// even one left with nothing to show. The tab strip counts only what shows.
func TestHidingQuitHidesSubtabsInEveryTab(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	// "Go packages" is in Code too.
	mustOK(t, h.e.AddItemToTab("w18", 1))
	if tv := h.tabNamed("Code"); tv.Windows != 4 || !slices.Contains(tv.Apps, "Brave") {
		t.Fatalf("Code before the quit: %d windows, apps %v", tv.Windows, tv.Apps)
	}
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()

	v := h.view()
	for _, id := range []string{"w6", "w17", "w18"} {
		it := v.Items[id]
		if !it.Hidden || !it.NoWindow {
			t.Errorf("%s: hidden %v, no window %v; want both", id, it.Hidden, it.NoWindow)
		}
		if slices.Contains(hidingDisplayedItems(v), id) {
			t.Errorf("%s is still drawn", id)
		}
	}
	if it := v.Items["w7"]; it.Hidden {
		t.Errorf("Obsidian's window is open, so w7 shouldn't be hidden")
	}
	// The model is as it was: Research's Brave pane still holds its items,
	// with the same one active, and Code still holds w18.
	if p := hidingModelPane(h, "Research", "p14"); !slices.Equal(p.Items, []string{"w6", "w17", "w18"}) || p.Active != "w6" {
		t.Errorf("Research's p14 in the model: %v active %s", p.Items, p.Active)
	}
	if p := hidingModelPane(h, "Code", "p11"); !slices.Contains(p.Items, "w18") {
		t.Errorf("Code's p11 in the model lost w18: %v", p.Items)
	}
	// Both of Research's panes are drawn, the Brave one with nothing in it,
	// waiting for Brave.
	if got := leafIDs(TabView{Root: v.Display.Root}); !slices.Equal(got, []string{"p14", "p15"}) {
		t.Errorf("displayed panes: %v, want p14 and p15", got)
	}
	if p := hidingPane(h, "p14"); len(p.Items) != 0 || p.Active != "" || len(p.Remembered) != 1 {
		t.Errorf("p14: items %v active %q remembered %+v", p.Items, p.Active, p.Remembered)
	}
	if tv := h.tabNamed("Research"); tv.Windows != 1 || !slices.Equal(tv.Apps, []string{"Obsidian"}) || tv.Panes != 2 {
		t.Errorf("Research: %d windows, apps %v, %d panes; want 1, [Obsidian], 2", tv.Windows, tv.Apps, tv.Panes)
	}
	if tv := h.tabNamed("Code"); tv.Windows != 3 || slices.Contains(tv.Apps, "Brave") {
		t.Errorf("Code: %d windows, apps %v; want 3 without Brave", tv.Windows, tv.Apps)
	}
	// In Code too, w18 is out of sight and its pane waits for Brave.
	mustOK(t, h.e.SwitchTab(1))
	p := hidingPane(h, "p11")
	if slices.Contains(p.Items, "w18") || !slices.Equal(hidingRememberedApps(p), []string{braveID + "=w18"}) {
		t.Errorf("Code's p11: items %v remembered %v", p.Items, hidingRememberedApps(p))
	}
}

// A pane whose active item is hidden shows the most recently used of its
// others, without making it active in the model, so that when the window
// comes back it goes to its old place and shows again.
func TestHidingPaneShowsRecentOtherUntilWindowReturns(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(obsidianID, "Obsidian", platform.AppLaunchWindows("Daily note"))
	}})
	mustOK(t, h.e.SwitchTab(2))
	// Obsidian's pane gets three more windows: [w7, w17, w18, w3].
	mustOK(t, h.e.MoveItem("w17", 2, "p15", "center"))
	mustOK(t, h.e.MoveItem("w18", 2, "p15", "center"))
	mustOK(t, h.e.MoveItem("w3", 2, "p15", "center"))
	if got := hidingModelPane(h, "Research", "p15").Items; !slices.Equal(got, []string{"w7", "w17", "w18", "w3"}) {
		t.Fatalf("p15: %v", got)
	}
	// Of the others, w18 is used last: it's neither next to w7 nor first
	// among them.
	for _, id := range []string{"w17", "w3", "w18", "w7"} {
		mustOK(t, h.e.ShowItem("p15", id))
	}
	h.settle()
	mustOK(t, h.p.QuitApp(obsidianID))
	h.drain()
	h.settle()

	p := hidingPane(h, "p15")
	if !slices.Equal(p.Items, []string{"w17", "w18", "w3"}) || p.Active != "w18" {
		t.Errorf("p15 shows %v, active %s; want [w17 w18 w3] with w18", p.Items, p.Active)
	}
	if m := hidingModelPane(h, "Research", "p15"); !slices.Equal(m.Items, []string{"w7", "w17", "w18", "w3"}) || m.Active != "w7" {
		t.Errorf("the model's p15 changed: %v active %s, want w7 still active in its place", m.Items, m.Active)
	}
	if it := h.item("w7"); !it.Hidden || it.Pane != "p15" || it.Tab != 2 {
		t.Errorf("w7: hidden %v, tab %d pane %s", it.Hidden, it.Tab, it.Pane)
	}
	// Obsidian opens again, restoring its window, which goes back to w7.
	mustOK(t, h.p.LaunchApp(obsidianID))
	h.drain()
	h.settle()
	note := h.windowID(obsidianID, "Daily note")
	if got := h.itemOf(note); got != "w7" {
		t.Fatalf("Obsidian's window went to %q, want w7", got)
	}
	p = hidingPane(h, "p15")
	if !slices.Equal(p.Items, []string{"w7", "w17", "w18", "w3"}) || p.Active != "w7" {
		t.Errorf("p15 shows %v, active %s; want w7 back in its place, active", p.Items, p.Active)
	}
	if it := h.item("w7"); it.Hidden || it.Status != StatusReady {
		t.Errorf("w7: hidden %v, status %s", it.Hidden, it.Status)
	}
	if h.frame(note) != h.bodyOf("p15") {
		t.Errorf("Obsidian's window should be docked in p15: %v, want %v", h.frame(note), h.bodyOf("p15"))
	}
}

// A pane lists the apps it's waiting for in reading order, one entry per
// app, each naming the item Open would fill: the pane's active item if it's
// the app's, or else the app's most recently used there. Its text is that
// item's status text.
func TestHidingRememberedListsAppsInReadingOrder(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	// Zed joins Brave's pane, after Brave's windows: [w6, w17, w18, w3].
	mustOK(t, h.e.MoveItem("w3", 2, "p14", "center"))
	mustOK(t, h.e.ShowItem("p14", "w18"))
	mustOK(t, h.e.ShowItem("p14", "w3"))
	// Zed quits first, and was used last, but Brave comes first on the
	// page.
	mustOK(t, h.p.QuitApp(zedID))
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()

	p := hidingPane(h, "p14")
	if len(p.Items) != 0 || p.Active != "" {
		t.Errorf("p14 should show nothing: %v active %q", p.Items, p.Active)
	}
	if len(p.Remembered) != 2 {
		t.Fatalf("p14 remembers %+v, want Brave and Zed", p.Remembered)
	}
	brave, zed := h.item("w18"), h.item("w3")
	want := []RememberedApp{
		{ItemID: "w18", BundleID: braveID, App: "Brave", Mono: brave.Mono, Icon: appIcon(braveID), Text: brave.StatusText, CanOpen: true},
		{ItemID: "w3", BundleID: zedID, App: "Zed", Mono: zed.Mono, Icon: appIcon(zedID), Text: zed.StatusText, CanOpen: true},
	}
	if !slices.Equal(p.Remembered, want) {
		t.Errorf("remembered:\n got %+v\nwant %+v", p.Remembered, want)
	}
	if brave.StatusText != "Brave isn't running" || brave.Mono == "" || zed.Mono == "" {
		t.Errorf("status text %q, monos %q %q", brave.StatusText, brave.Mono, zed.Mono)
	}
	if r := hidingPane(h, "p15").Remembered; r == nil || len(r) != 0 {
		t.Errorf("p15 waits for nothing, so its list should be empty, not %#v", r)
	}
}

// The item Open fills is the pane's active item if it's the app's, even
// when another of the app's items was used more recently, in another tab.
func TestHidingRememberedPrefersPanesActiveItem(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.AddItemToTab("w18", 1))
	mustOK(t, h.e.ShowItem("p14", "w17"))
	// w18 is used last, in Code.
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.ShowItem("p11", "w18"))
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	if got := hidingRememberedApps(hidingPane(h, "p14")); !slices.Equal(got, []string{braveID + "=w17"}) {
		t.Errorf("Research's p14 remembers %v, want Brave filling its active w17", got)
	}
}

// In the compact "tabs" view, the one pane lists what the whole tab is
// waiting for, across its panes, in reading order.
func TestHidingRememberedInTabsViewCoversWholeTab(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(2))
	if v := h.view(); !v.Display.Compact {
		t.Fatal("Research should be in the tabs view on the laptop")
	}
	mustOK(t, h.e.ShowItem(layout.CompactPaneID, "w17"))
	mustOK(t, h.e.ShowItem(layout.CompactPaneID, "w7"))
	// Obsidian quits first, but Brave's pane comes first.
	mustOK(t, h.p.QuitApp(obsidianID))
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	v := h.view()
	if len(v.Display.Panes) != 1 {
		t.Fatalf("displayed panes: %+v", v.Display.Panes)
	}
	p := v.Display.Panes[0]
	if p.ID != layout.CompactPaneID || len(p.Items) != 0 {
		t.Errorf("the compact pane %s shows %v, want nothing", p.ID, p.Items)
	}
	if got := hidingRememberedApps(p); !slices.Equal(got, []string{braveID + "=w17", obsidianID + "=w7"}) {
		t.Errorf("remembered %v, want Brave (w17, its most recently used) then Obsidian", got)
	}
	// Panes stay in the model, though nothing in them shows.
	if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got, []string{"p14", "p15"}) {
		t.Errorf("Research's panes: %v", got)
	}
}

// In the "tabs" view, the one pane holds the whole tab's sub-tabs. When
// the one it shows is hidden, it shows the most recently used of the
// others, wherever they are in the tab, as any pane does: the user doesn't
// see the tab's panes there.
func TestHidingTabsViewShowsMostRecentOtherItem(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(2))
	// Zed joins Obsidian's pane: p14 [w6, w17, w18], p15 [w7, w3].
	mustOK(t, h.e.MoveItem("w3", 2, "p15", "center"))
	// Zed was used long ago, Brave's w17 just before Obsidian.
	for _, id := range []string{"w3", "w17", "w7"} {
		mustOK(t, h.e.ShowItem(layout.CompactPaneID, id))
	}
	h.settle()
	if p := hidingPane(h, layout.CompactPaneID); p.Active != "w7" {
		t.Fatalf("the compact pane shows %s, want w7", p.Active)
	}
	mustOK(t, h.p.QuitApp(obsidianID))
	h.drain()
	h.settle()
	p := hidingPane(h, layout.CompactPaneID)
	if !slices.Equal(p.Items, []string{"w6", "w17", "w18", "w3"}) {
		t.Errorf("the compact pane holds %v", p.Items)
	}
	if p.Active != "w17" {
		t.Errorf("the compact pane shows %s, want w17, the most recently used of the others", p.Active)
	}
}

// In the "tabs" view, the item that Open fills for an app is the one the
// pane showed, if it's the app's, as in any other pane, even if another of
// the app's items was used more recently, in another tab.
func TestHidingTabsViewRememberedPrefersShownItem(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.AddItemToTab("w18", 1))
	mustOK(t, h.e.ShowItem(layout.CompactPaneID, "w17"))
	// w18 is used last, in Code.
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.ShowItem("p11", "w18"))
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	if p := hidingPane(h, layout.CompactPaneID); p.Active != "w17" {
		t.Fatalf("the compact pane shows %s, want w17", p.Active)
	}
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	if got := hidingRememberedApps(hidingPane(h, layout.CompactPaneID)); !slices.Equal(got, []string{braveID + "=w17"}) {
		t.Errorf("the compact pane remembers %v, want Brave filling w17, which it showed", got)
	}
}

// An app that macOS says isn't installed can't be opened from its pane, and
// its entry says why.
func TestHidingRememberedCantOpenUninstalledApp(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		// Obsidian runs, but its app has gone from the disk.
		p.AddApp(obsidianID, "Obsidian", platform.AppNotInstalled())
	}})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.p.QuitApp(obsidianID))
	h.drain()
	if got := hidingPane(h, "p15").Remembered; len(got) != 1 || !got[0].CanOpen {
		t.Fatalf("before Bench tries, Obsidian should look openable: %+v", got)
	}
	mustErr(t, h.e.OpenItem("w7"), "Bench couldn't open Obsidian, because it isn't installed.")
	got := hidingPane(h, "p15").Remembered
	if len(got) != 1 || got[0].ItemID != "w7" || got[0].CanOpen || got[0].Text != "Obsidian isn't installed" {
		t.Errorf("remembered %+v, want w7 that can't open, as Obsidian isn't installed", got)
	}
	if it := h.item("w7"); !it.Hidden {
		t.Errorf("w7 should stay hidden: Bench isn't opening Obsidian")
	}
}

// While a tab switch waits for an app it opened, the item shows, saying
// the app is opening. Once the wait gives up, the item is hidden, and its
// pane waits for the app.
func TestHidingSwitchShowsItemWhileAppOpens(t *testing.T) {
	h := newHarness(t, setup{config: launchConfig, world: func(p *platform.Fake) {
		p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(time.Second), platform.AppLaunchWindows("Calendar"))
		// Safari opens, but never makes a window for its item.
		p.InstallApp(safariID, "Safari")
	}})
	// On Home, Plan's items have no windows: they're out of sight.
	if tv := h.tabNamed("Plan"); tv.Windows != 0 || len(tv.Apps) != 0 {
		t.Errorf("Plan before the switch: %d windows, apps %v", tv.Windows, tv.Apps)
	}
	if !h.item("w2").Hidden || !h.item("w3").Hidden {
		t.Errorf("Plan's items should be hidden before the switch")
	}

	mustOK(t, h.e.SwitchTab(1))
	for _, id := range []string{"w2", "w3"} {
		it := h.item(id)
		if it.Hidden || it.Status != StatusLaunching {
			t.Errorf("%s while its app opens: hidden %v, status %s %q", id, it.Hidden, it.Status, it.StatusText)
		}
		p := hidingPane(h, it.Pane)
		if !slices.Equal(p.Items, []string{id}) || p.Active != id || len(p.Remembered) != 0 {
			t.Errorf("%s's pane: items %v active %q remembered %+v", id, p.Items, p.Active, p.Remembered)
		}
	}
	if tv := h.tabNamed("Plan"); tv.Windows != 2 {
		t.Errorf("Plan while opening: %d windows, want 2", tv.Windows)
	}

	// Calendar's window arrives; Safari's never does.
	h.advance(1200 * time.Millisecond)
	if it := h.item("w2"); it.Hidden || it.Status != StatusReady {
		t.Errorf("w2 with its window: hidden %v, status %s", it.Hidden, it.Status)
	}
	if it := h.item("w3"); it.Hidden {
		t.Errorf("w3 should show until the wait gives up")
	}
	h.advance(2500 * time.Millisecond)
	it := h.item("w3")
	if !it.Hidden {
		t.Fatalf("after the launch timeout, w3 should be hidden: %s %q", it.Status, it.StatusText)
	}
	p := hidingPane(h, it.Pane)
	if len(p.Items) != 0 || len(p.Remembered) != 1 || p.Remembered[0].ItemID != "w3" ||
		p.Remembered[0].Text != "Safari has no window open for this" {
		t.Errorf("w3's pane: items %v remembered %+v", p.Items, p.Remembered)
	}
	if tv := h.tabNamed("Plan"); tv.Windows != 1 || !slices.Equal(tv.Apps, []string{"Calendar"}) {
		t.Errorf("Plan afterwards: %d windows, apps %v", tv.Windows, tv.Apps)
	}
}

// Open shows the item it opens the app for while the app opens, and the
// app's restored window fills it; the app's other places stay hidden.
func TestHidingOpenShowsItemWhileAppOpens(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(braveID, "Brave", platform.AppLaunchDelay(2*time.Second), platform.AppLaunchWindows("Go packages"))
	}})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	mustOK(t, h.e.OpenItem("w18"))
	if it := h.item("w18"); it.Hidden || it.Status != StatusLaunching || it.StatusText != "Opening Brave…" {
		t.Errorf("w18 while Brave opens: hidden %v, %s %q", it.Hidden, it.Status, it.StatusText)
	}
	p := hidingPane(h, "p14")
	if !slices.Equal(p.Items, []string{"w18"}) || p.Active != "w18" {
		t.Errorf("p14 while Brave opens: %v active %q, want w18 alone", p.Items, p.Active)
	}
	if !h.item("w6").Hidden || !h.item("w17").Hidden {
		t.Errorf("Brave's other places should stay hidden")
	}
	h.advance(2500 * time.Millisecond)
	if got := h.itemOf(h.windowID(braveID, "Go packages")); got != "w18" {
		t.Errorf("Brave's window went to %q, want w18", got)
	}
	if p := hidingPane(h, "p14"); !slices.Equal(p.Items, []string{"w18"}) || p.Active != "w18" {
		t.Errorf("p14 afterwards: %v active %q", p.Items, p.Active)
	}
}

// Remove from this pane forgets an app's hidden items in the pane. One
// that's in other tabs too only leaves this one, and a pane left empty
// closes up; a pane with other items stays.
func TestHidingRemoveRemembered(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustOK(t, h.e.AddItemToTab("w18", 1))
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()

	mustOK(t, h.e.RemoveRemembered("p14", braveID))
	if got := h.em.lastToast(); got != "Removed Brave from this pane." {
		t.Errorf("toast %q", got)
	}
	v := h.view()
	for _, id := range []string{"w6", "w17"} {
		if _, ok := v.Items[id]; ok {
			t.Errorf("%s should be forgotten", id)
		}
	}
	if it, ok := v.Items["w18"]; !ok || it.Tab != 1 || it.Pane != "p11" || len(it.Tabs) != 0 {
		t.Errorf("w18 should stay in Code alone: %+v", it)
	}
	if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got, []string{"p15"}) {
		t.Errorf("Research's panes: %v, want p14 closed up", got)
	}
	if got := leafIDs(TabView{Root: v.Display.Root}); !slices.Equal(got, []string{"p15"}) {
		t.Errorf("displayed panes: %v", got)
	}

	// In Code, w18 is the last of Brave's places, in a pane that shows Zed
	// and VS Code, which stays.
	mustOK(t, h.e.SwitchTab(1))
	mustOK(t, h.e.RemoveRemembered("p11", braveID))
	if _, ok := h.view().Items["w18"]; ok {
		t.Errorf("w18 should be forgotten")
	}
	if got := leafIDs(h.tabNamed("Code")); !slices.Equal(got, []string{"p11", "p12"}) {
		t.Errorf("Code's panes: %v", got)
	}
	if p := hidingPane(h, "p11"); !slices.Equal(p.Items, []string{"w3", "w4"}) || len(p.Remembered) != 0 {
		t.Errorf("p11: %v remembered %+v", p.Items, p.Remembered)
	}
}

// Remove from this pane refuses a pane that isn't waiting for the app, as
// when the app's windows there are open, and a pane that isn't in the
// current tab, forgetting nothing.
func TestHidingRemoveRememberedRefusals(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustErr(t, h.e.RemoveRemembered("p14", braveID), "This pane isn't waiting for Brave.")
	mustErr(t, h.e.RemoveRemembered("p15", zedID), "This pane isn't waiting for Zed.")
	mustErr(t, h.e.RemoveRemembered("p99", braveID), "There's no such pane.")
	mustErr(t, h.e.RemoveRemembered("p11", zedID), "There's no such pane.")
	mustErr(t, h.e.RemoveRemembered(layout.CompactPaneID, braveID), "There's no such pane.")
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	// Brave's places are in p14, not p15.
	mustErr(t, h.e.RemoveRemembered("p15", braveID), "This pane isn't waiting for Brave.")
	for _, id := range []string{"w6", "w17", "w18", "w7"} {
		if _, ok := h.view().Items[id]; !ok {
			t.Errorf("%s was forgotten by a refused removal", id)
		}
	}
	if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got, []string{"p14", "p15"}) {
		t.Errorf("Research's panes: %v", got)
	}
}

// In the "tabs" view, Remove from this pane works on the whole tab, as the
// one pane shows it: the app's hidden items leave every pane of the tab.
func TestHidingRemoveRememberedInTabsView(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(2))
	// Brave has places in both of Research's panes, and w17 is in Code too.
	mustOK(t, h.e.MoveItem("w18", 2, "p15", "center"))
	mustOK(t, h.e.AddItemToTab("w17", 1))
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	if got := hidingRememberedApps(hidingPane(h, layout.CompactPaneID)); len(got) != 1 || !strings.HasPrefix(got[0], braveID+"=") {
		t.Fatalf("the compact pane remembers %v, want Brave", got)
	}

	mustOK(t, h.e.RemoveRemembered(layout.CompactPaneID, braveID))
	if got := h.em.lastToast(); got != "Removed Brave from this pane." {
		t.Errorf("toast %q", got)
	}
	v := h.view()
	for _, id := range []string{"w6", "w18"} {
		if _, ok := v.Items[id]; ok {
			t.Errorf("%s should be forgotten", id)
		}
	}
	if it, ok := v.Items["w17"]; !ok || it.Tab != 1 {
		t.Errorf("w17 should stay in Code: %+v", it)
	}
	if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got, []string{"p15"}) {
		t.Errorf("Research's panes: %v, want p14 closed up", got)
	}
	if p := hidingModelPane(h, "Research", "p15"); !slices.Equal(p.Items, []string{"w7"}) {
		t.Errorf("p15 in the model: %v, want only w7", p.Items)
	}
	if p := hidingPane(h, layout.CompactPaneID); !slices.Equal(p.Items, []string{"w7"}) || len(p.Remembered) != 0 {
		t.Errorf("the compact pane: %v remembered %+v", p.Items, p.Remembered)
	}
	mustErr(t, h.e.RemoveRemembered(layout.CompactPaneID, braveID), "This pane isn't waiting for Brave.")
}

// Close this pane closes a pane that shows nothing, forgetting the places
// it's waiting for in this tab (an item in other tabs too stays there), and
// the rest of the tab closes up. The tab's layout is then custom.
func TestHidingClosePane(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	// Code's Zed window is shown in Obsidian's pane too, which doesn't
	// change Research's layout: p15 holds [w7, w3].
	mustOK(t, h.e.FocusPane("p15"))
	mustOK(t, h.e.AddItemToTab("w3", 2))
	if tv := h.tabNamed("Research"); tv.Preset != "side" {
		t.Fatalf("Research's preset: %s", tv.Preset)
	}
	if got := hidingModelPane(h, "Research", "p15").Items; !slices.Equal(got, []string{"w7", "w3"}) {
		t.Fatalf("p15: %v", got)
	}
	before := hidingPane(h, "p14").Rect
	mustOK(t, h.p.QuitApp(obsidianID))
	mustOK(t, h.p.QuitApp(zedID))
	h.drain()

	mustOK(t, h.e.ClosePane("p15"))
	v := h.view()
	if _, ok := v.Items["w7"]; ok {
		t.Errorf("w7 should be forgotten with its pane")
	}
	if it, ok := v.Items["w3"]; !ok || it.Tab != 1 || it.Pane != "p11" || len(it.Tabs) != 0 || !it.Hidden {
		t.Errorf("w3 should stay in Code alone, hidden: %+v", it)
	}
	tv := h.tabNamed("Research")
	if got := leafIDs(tv); !slices.Equal(got, []string{"p14"}) || tv.Preset != layout.PresetCustom {
		t.Errorf("Research: panes %v, preset %s; want [p14], custom", got, tv.Preset)
	}
	if len(v.Display.Panes) != 1 {
		t.Fatalf("displayed panes: %+v", v.Display.Panes)
	}
	if after := v.Display.Panes[0].Rect; after.W <= before.W {
		t.Errorf("p14 should close up over p15's room: %v, was %v", after, before)
	}
	brave := h.windowID(braveID, "Wails v3 docs")
	if h.frame(brave) != h.bodyOf("p14") {
		t.Errorf("Brave's window should be arranged in the wider pane: %v, want %v", h.frame(brave), h.bodyOf("p14"))
	}
	// The tab's last pane stays, even when it shows nothing.
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	mustErr(t, h.e.ClosePane("p14"), "This is the tab's only pane.")
}

// Close this pane refuses a pane that isn't there, a pane that shows
// something, even if it also waits for an app, and any pane in the "tabs"
// view.
func TestHidingClosePaneRefusals(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	mustErr(t, h.e.ClosePane("p99"), "There's no such pane.")
	mustErr(t, h.e.ClosePane("p11"), "There's no such pane.")
	mustErr(t, h.e.ClosePane("p14"), "This pane has windows in it. Move them to another pane first.")
	// p15 holds a Brave window as well as Obsidian's, which quits.
	mustOK(t, h.e.MoveItem("w18", 2, "p15", "center"))
	mustOK(t, h.p.QuitApp(obsidianID))
	h.drain()
	if p := hidingPane(h, "p15"); len(p.Remembered) != 1 || !slices.Equal(p.Items, []string{"w18"}) {
		t.Fatalf("p15: %v remembered %+v", p.Items, p.Remembered)
	}
	mustErr(t, h.e.ClosePane("p15"), "This pane has windows in it. Move them to another pane first.")
	if got := leafIDs(h.tabNamed("Research")); !slices.Equal(got, []string{"p14", "p15"}) {
		t.Errorf("Research's panes: %v", got)
	}

	// The "tabs" view shows one pane, so none closes there.
	l := newHarness(t, setup{screen: laptop})
	mustOK(t, l.e.SwitchTab(2))
	mustOK(t, l.p.QuitApp(obsidianID))
	l.drain()
	mustErr(t, l.e.ClosePane("p15"), "Splits are hidden on small screens.")
	if got := leafIDs(l.tabNamed("Research")); !slices.Equal(got, []string{"p14", "p15"}) {
		t.Errorf("Research's panes on the laptop: %v", got)
	}
	if _, ok := l.view().Items["w7"]; !ok {
		t.Errorf("w7 was forgotten by a refused close")
	}
}

// Cycling through a pane's sub-tabs skips the hidden ones, as it goes by
// what's drawn.
func TestHidingCycleSubtabSkipsHidden(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// p11 holds [w3 Zed, w4 VS Code, w18 Brave]; VS Code quits.
	mustOK(t, h.e.AddItemToTab("w18", 1))
	mustOK(t, h.e.ShowItem("p11", "w3"))
	if got := hidingModelPane(h, "Code", "p11").Items; !slices.Equal(got, []string{"w3", "w4", "w18"}) {
		t.Fatalf("p11: %v", got)
	}
	mustOK(t, h.p.QuitApp(vscodeID))
	h.drain()
	// Forward from w3 skips w4 to w18, and wraps to w3; back from w3 wraps
	// to w18.
	for i, c := range []struct {
		step int
		want string
	}{{1, "w18"}, {1, "w3"}, {-1, "w18"}} {
		mustOK(t, h.e.CycleSubtab(c.step))
		if p := hidingPane(h, "p11"); p.Active != c.want {
			t.Errorf("cycle %d (%+d): p11 shows %s, want %s", i, c.step, p.Active, c.want)
		}
		if m := hidingModelPane(h, "Code", "p11"); m.Active == "w4" {
			t.Errorf("cycle %d made the hidden w4 active", i)
		}
	}
}

// Moves go by what a pane shows. A pane's hidden items don't count as
// something to leave behind when it would split, by keyboard or by drag:
// they'd be left out of sight. A keyboard move takes the item the pane
// shows, not a hidden active one, and a pane whose items are all hidden
// stays, with nothing to move.
func TestHidingMovesGoByShownItems(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// VS Code is p11's active item when it quits; p11 shows Zed instead.
	mustOK(t, h.e.ShowItem("p11", "w4"))
	mustOK(t, h.p.QuitApp(vscodeID))
	h.drain()
	if p := hidingPane(h, "p11"); !slices.Equal(p.Items, []string{"w3"}) || p.Active != "w3" {
		t.Fatalf("p11: %v active %s", p.Items, p.Active)
	}
	// Nothing is to the left, and splitting would leave only VS Code's
	// hidden place behind.
	mustErr(t, h.e.MoveDirection("left"), "There's nothing to move it next to.")
	mustErr(t, h.e.MoveItem("w3", 1, "p11", "left"),
		"This pane has only one window, so there's nothing to show beside it. Add another window first, or drag one here.")
	if got := leafIDs(h.tabNamed("Code")); !slices.Equal(got, []string{"p11", "p12"}) {
		t.Errorf("Code's panes: %v", got)
	}
	if m := hidingModelPane(h, "Code", "p11"); !slices.Equal(m.Items, []string{"w3", "w4"}) {
		t.Errorf("p11 in the model: %v", m.Items)
	}
	mustOK(t, h.e.MoveDirection("right"))
	if it := h.item("w3"); it.Pane != "p12" {
		t.Errorf("Zed's window should move to p12, not %s", it.Pane)
	}
	if it := h.item("w4"); it.Pane != "p11" || !it.Hidden {
		t.Errorf("VS Code's hidden place should stay in p11: pane %s hidden %v", it.Pane, it.Hidden)
	}
	p := hidingPane(h, "p11")
	if len(p.Items) != 0 || !slices.Equal(hidingRememberedApps(p), []string{vscodeID + "=w4"}) {
		t.Errorf("p11 should stay, waiting for VS Code: %v remembered %v", p.Items, hidingRememberedApps(p))
	}
	mustOK(t, h.e.FocusPane("p11"))
	mustErr(t, h.e.MoveDirection("right"), "There's no window in this pane to move.")
	if it := h.item("w4"); it.Pane != "p11" {
		t.Errorf("w4 moved to %s", it.Pane)
	}
	// With two windows showing in p11 beside VS Code's place, one of them
	// can split it, leaving the other showing.
	mustOK(t, h.e.AddItemToTab("w18", 1))
	mustOK(t, h.e.MoveItem("w3", 1, "p11", "center"))
	mustOK(t, h.e.MoveItem("w3", 1, "p11", "bottom"))
	if got := leafIDs(h.tabNamed("Code")); len(got) != 3 {
		t.Errorf("Code's panes after the split: %v", got)
	}
	if m := hidingModelPane(h, "Code", "p11"); !slices.Equal(m.Items, []string{"w4", "w18"}) {
		t.Errorf("p11 should keep VS Code's hidden place and w18: %v", m.Items)
	}
}

// The add dialog doesn't offer hidden items as windows in other tabs, nor
// a new window of an app whose only items in the tab are hidden.
func TestHidingAddOptionsLeaveOutHidden(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	rows := func() (others []string, fresh []string, apps []string) {
		opts, err := h.e.AddOptions("p12")
		mustOK(t, err)
		for _, o := range opts {
			switch o.Group {
			case "Windows in other tabs":
				others = append(others, o.Source.ID)
			case "New window":
				fresh = append(fresh, o.Source.BundleID)
			case "Apps":
				apps = append(apps, o.Source.BundleID)
			}
		}
		return others, fresh, apps
	}
	others, fresh, _ := rows()
	if !slices.Contains(others, "w17") || !slices.Contains(fresh, vscodeID) {
		t.Fatalf("before the quits: others %v, new windows %v", others, fresh)
	}
	mustOK(t, h.p.QuitApp(braveID))
	mustOK(t, h.p.QuitApp(vscodeID))
	h.drain()
	others, fresh, apps := rows()
	for _, id := range []string{"w6", "w17", "w18"} {
		if slices.Contains(others, id) {
			t.Errorf("hidden %s is offered from another tab: %v", id, others)
		}
	}
	if !slices.Contains(others, "w7") {
		t.Errorf("Obsidian's open window should still be offered: %v", others)
	}
	if !slices.Equal(fresh, []string{zedID}) {
		t.Errorf("new windows %v, want Zed's only", fresh)
	}
	// VS Code can still be added from Apps.
	if !slices.Contains(apps, vscodeID) {
		t.Errorf("Apps should still offer VS Code: %v", apps)
	}
}

// A window that disappears without an event, as one does while its app is
// quitting, leaves its item showing until Bench looks again: hiding it at
// once would show another window in its place, and then switch back.
func TestHidingGoneItemShowsUntilChecked(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(2))
	h.settle()
	docs := h.windowID(braveID, "Wails v3 docs")
	h.p.SetWindowErr(docs, "Window", platform.ErrNoWindow)
	mustOK(t, h.e.Rearrange())
	h.drain()
	it := h.item("w6")
	if it.Hidden || it.Status != StatusMissing {
		t.Fatalf("w6 should show, missing, while Bench waits to look again: hidden %v, %s %q", it.Hidden, it.Status, it.StatusText)
	}
	if p := hidingPane(h, "p14"); !slices.Equal(p.Items, []string{"w6", "w17", "w18"}) || p.Active != "w6" || len(p.Remembered) != 0 {
		t.Errorf("p14: %v active %s remembered %+v", p.Items, p.Active, p.Remembered)
	}
	if tv := h.tabNamed("Research"); tv.Windows != 4 {
		t.Errorf("Research counts %d windows, want 4", tv.Windows)
	}
	// The window was there after all.
	h.p.SetWindowErr(docs, "Window", nil)
	h.advance(2 * time.Second)
	if it := h.item("w6"); it.Hidden || it.Status != StatusReady {
		t.Errorf("w6 after the check: hidden %v, %s", it.Hidden, it.Status)
	}
	if p := hidingPane(h, "p14"); p.Active != "w6" {
		t.Errorf("p14 shows %s, want w6", p.Active)
	}
}

// hidingItemOf returns the first item of an app in the current tab, in
// reading order, failing the test if there's none.
func hidingItemOf(h *harness, bundleID string) string {
	h.t.Helper()
	v := h.view()
	for _, p := range layout.Leaves(v.Tabs[v.Current].Root) {
		for _, id := range p.Items {
			if v.Items[id].BundleID == bundleID {
				return id
			}
		}
	}
	h.t.Fatalf("the current tab has no item of %s", bundleID)
	return ""
}

// hidingPlanConfig has a tab whose first pane waits for Calendar, ahead of
// Zed, and whose second has Brave.
const hidingPlanConfig = `
[settings]
launch_timeout = "3s"

[[tab]]
name = "Home"
layout = "single"
  [[tab.pane]]
  items = [{ terminal = "~" }]

[[tab]]
name = "Plan"
layout = "side"
  [[tab.pane]]
  items = [{ app = "com.apple.iCal" }, { app = "dev.zed.Zed" }]
  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]
`

// When Bench stops opening an app for an item that's still without a
// window, the item is hidden, and the tab is arranged again: its pane shows
// another of its windows, docked, not a window left parked. Open during a
// switch's wait keeps the switch's items opening until the wait ends.
func TestHidingArrangesWhenOpeningEnds(t *testing.T) {
	h := newHarness(t, setup{config: hidingPlanConfig, world: func(p *platform.Fake) {
		standardWorld(p)
		// Calendar opens late, and with no window.
		p.InstallApp(calendarID, "Calendar", platform.AppLaunchDelay(10*time.Second))
		p.InstallApp(braveID, "Brave", platform.AppLaunchDelay(10*time.Second))
	}})
	mustOK(t, h.e.SwitchTab(1))
	cal, zed, brave := hidingItemOf(h, calendarID), hidingItemOf(h, zedID), hidingItemOf(h, braveID)
	pane := h.paneOf(zed)
	if it := h.item(cal); it.Hidden || it.Status != StatusLaunching {
		t.Fatalf("Calendar's item while it opens: hidden %v, %s", it.Hidden, it.Status)
	}
	// A second into the wait, the user quits Brave and clicks Open for it.
	h.advance(time.Second)
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	mustOK(t, h.e.OpenItem(brave))
	// Calendar's item still shows that it's opening when the switch's own
	// timeout would have passed.
	h.advance(2500 * time.Millisecond)
	if it := h.item(cal); it.Hidden || it.Status != StatusLaunching {
		t.Errorf("Calendar's item during the longer wait: hidden %v, %s", it.Hidden, it.Status)
	}
	// Once the wait ends, Calendar's item is hidden, and Zed's window shows
	// in its place.
	h.advance(1500 * time.Millisecond)
	if it := h.item(cal); !it.Hidden {
		t.Errorf("Calendar's item after the wait should be hidden: %s %q", it.Status, it.StatusText)
	}
	if p := hidingPane(h, pane); p.Active != zed {
		t.Errorf("the pane shows %q, want Zed's %s", p.Active, zed)
	}
	if w := h.windowID(zedID, "bench — switch.go"); h.frame(w) != h.bodyOf(pane) {
		t.Errorf("Zed's window should be docked in %s: %v, want %v", pane, h.frame(w), h.bodyOf(pane))
	}
}

// Forgetting a hidden item that was its pane's active one, when its app
// has opened again without filling it, keeps the pane showing what it
// showed: the most recently used of its other items, not the item next to
// the one forgotten.
func TestHidingForgettingKeepsWhatThePaneShows(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.InstallApp(braveID, "Brave", platform.AppLaunchWindows("Wails v3 docs"))
	}})
	mustOK(t, h.e.SwitchTab(1))
	// Code's p11 gets two Brave windows: [w3 Zed, w4 VS Code, w6, w17].
	mustOK(t, h.e.MoveItem("w6", 1, "p11", "center"))
	mustOK(t, h.e.MoveItem("w17", 1, "p11", "center"))
	for _, id := range []string{"w4", "w3", "w17"} {
		mustOK(t, h.e.ShowItem("p11", id))
	}
	h.settle()
	mustOK(t, h.p.QuitApp(braveID))
	h.drain()
	h.settle()
	if p := hidingPane(h, "p11"); p.Active != "w3" {
		t.Fatalf("with Brave closed, p11 shows %q, want w3, the most recently used", p.Active)
	}
	// Brave opens again with one window, which goes back to w6. A while
	// later, a minute after Bench started, w17's place is forgotten.
	mustOK(t, h.p.LaunchApp(braveID))
	h.drain()
	h.advance(65 * time.Second)
	if got := h.itemOf(h.windowID(braveID, "Wails v3 docs")); got != "w6" {
		t.Fatalf("Brave's window went to %q, want w6", got)
	}
	if _, ok := h.view().Items["w17"]; ok {
		t.Fatalf("w17's place should be forgotten by now")
	}
	if p := hidingPane(h, "p11"); p.Active != "w3" {
		t.Errorf("after w17 is forgotten, p11 shows %q, want w3 still", p.Active)
	}
	if m := hidingModelPane(h, "Code", "p11"); m.Active != "w3" {
		t.Errorf("the model's p11 has %q active, want w3", m.Active)
	}
	if w := h.windowID(zedID, "bench — switch.go"); h.frame(w) != h.bodyOf("p11") {
		t.Errorf("Zed's window should still be docked in p11: %v", h.frame(w))
	}
}

// In the "tabs" view, forgetting the places of the app whose window the
// pane showed keeps the pane showing what it showed in their place, the
// most recently used of the others, and the tab remembers that.
func TestHidingTabsViewRemoveRememberedKeepsWhatItShows(t *testing.T) {
	h := newHarness(t, setup{screen: laptop})
	mustOK(t, h.e.SwitchTab(2))
	// w6 is used last before Obsidian's w7.
	for _, id := range []string{"w6", "w17", "w6", "w7"} {
		mustOK(t, h.e.ShowItem(layout.CompactPaneID, id))
	}
	h.settle()
	mustOK(t, h.p.QuitApp(obsidianID))
	h.drain()
	h.settle()
	if p := hidingPane(h, layout.CompactPaneID); p.Active != "w6" {
		t.Fatalf("with Obsidian closed, the compact pane shows %q, want w6", p.Active)
	}
	mustOK(t, h.e.RemoveRemembered(layout.CompactPaneID, obsidianID))
	if p := hidingPane(h, layout.CompactPaneID); p.Active != "w6" {
		t.Errorf("after Remove, the compact pane shows %q, want w6 still", p.Active)
	}
	if got := h.e.model.Tabs[2].CompactShown; got != "w6" {
		t.Errorf("Research's tabs view shows %q in the model, want w6", got)
	}
}

// A pane whose shown item leaves it shows the item next to it that isn't
// hidden, not a hidden one, which would leave it showing nothing.
func TestHidingNeighbourOfAMovedItemIsShown(t *testing.T) {
	h := newHarness(t, setup{})
	mustOK(t, h.e.SwitchTab(1))
	// p11 [w3 Zed, w4 VS Code, w18 Brave], showing w18; VS Code quits.
	mustOK(t, h.e.AddItemToTab("w18", 1))
	mustOK(t, h.e.MoveItem("w18", 1, "p11", "center"))
	mustOK(t, h.e.ShowItem("p11", "w18"))
	if m := hidingModelPane(h, "Code", "p11"); !slices.Equal(m.Items, []string{"w3", "w4", "w18"}) {
		t.Fatalf("p11 in the model: %v", m.Items)
	}
	mustOK(t, h.p.QuitApp(vscodeID))
	h.drain()
	mustOK(t, h.e.MoveItem("w18", 1, "p12", "center"))
	if m := hidingModelPane(h, "Code", "p11"); m.Active != "w3" {
		t.Errorf("p11's active item in the model is %q, want w3, not VS Code's hidden place", m.Active)
	}
	if p := hidingPane(h, "p11"); p.Active != "w3" {
		t.Errorf("p11 shows %q, want w3", p.Active)
	}
}

// Without Accessibility permission, Bench can't tell which windows are
// open, so it hides nothing: the panes show what it remembers.
func TestHidingNothingWithoutAccessibility(t *testing.T) {
	h := newHarness(t, setup{untrusted: true})
	v := h.view()
	if it := v.Items["w3"]; it.Hidden || it.Status != StatusMissing || !it.CanOpen {
		t.Errorf("w3: hidden %v, status %s, can open %v", it.Hidden, it.Status, it.CanOpen)
	}
	if code := v.Tabs[1]; code.Windows != 3 || !slices.Contains(code.Apps, "Zed") {
		t.Errorf("Code counts %d windows, apps %v; want 3, with Zed", code.Windows, code.Apps)
	}
	for _, p := range v.Display.Panes {
		if len(p.Remembered) != 0 {
			t.Errorf("pane %s says it's waiting for %v", p.ID, hidingRememberedApps(p))
		}
	}
}
