package workspace

import (
	"slices"
	"testing"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

func TestParseBadge(t *testing.T) {
	tests := []struct {
		label string
		want  UnreadView
	}{
		{"3", UnreadView{Count: 3}},
		{" 12 ", UnreadView{Count: 12}},
		{"99+", UnreadView{Count: 99, More: true}},
		{"1,234", UnreadView{Count: 1234}},
		{"1.234", UnreadView{Count: 1234}},
		{"1 234 567", UnreadView{Count: 999999, More: true}},
		{"12345678901", UnreadView{Count: 999999, More: true}},
		{"•", UnreadView{Dot: true}},
		{"!", UnreadView{Dot: true}},
		{"New", UnreadView{Dot: true}},
		{"3 new", UnreadView{Dot: true}},
		{"0", UnreadView{}},
		{"", UnreadView{}},
		{"  ", UnreadView{}},
	}
	for _, tc := range tests {
		if got := parseBadge(tc.label); got != tc.want {
			t.Errorf("parseBadge(%q) = %+v, want %+v", tc.label, got, tc.want)
		}
	}
}

func TestTitleCount(t *testing.T) {
	tests := []struct {
		title string
		want  UnreadView
		rest  string
		ok    bool
	}{
		{"(3) Inbox - Outlook", UnreadView{Count: 3}, "Inbox - Outlook", true},
		{"[12] general - Slack", UnreadView{Count: 12}, "general - Slack", true},
		{"(99+) Inbox", UnreadView{Count: 99, More: true}, "Inbox", true},
		{"(1,024) Inbox", UnreadView{Count: 1024}, "Inbox", true},
		{" (3)  Inbox", UnreadView{Count: 3}, "Inbox", true},
		{"(0) Inbox", UnreadView{}, "Inbox", true},
		{"(3)", UnreadView{Count: 3}, "", true},
		{"Inbox (3)", UnreadView{}, "Inbox (3)", false},
		{"(3 new) Inbox", UnreadView{}, "(3 new) Inbox", false},
		{"(3] Inbox", UnreadView{}, "(3] Inbox", false},
		{"() Inbox", UnreadView{}, "() Inbox", false},
		{"(beta) Inbox", UnreadView{}, "(beta) Inbox", false},
		{"Inbox", UnreadView{}, "Inbox", false},
	}
	for _, tc := range tests {
		got, rest, ok := titleCount(tc.title)
		if got != tc.want || rest != tc.rest || ok != tc.ok {
			t.Errorf("titleCount(%q) = %+v, %q, %v; want %+v, %q, %v", tc.title, got, rest, ok, tc.want, tc.rest, tc.ok)
		}
	}
}

func TestUnreadTextAndTotals(t *testing.T) {
	for _, tc := range []struct {
		u    UnreadView
		want string
	}{
		{UnreadView{Count: 3}, "3"},
		{UnreadView{Count: 99, More: true}, "99+"},
		{UnreadView{Dot: true}, "•"},
		{UnreadView{}, ""},
	} {
		if got := tc.u.Text(); got != tc.want {
			t.Errorf("%+v.Text() = %q, want %q", tc.u, got, tc.want)
		}
	}
	total := unreadTotal(map[string]UnreadView{
		"a": {Count: 3}, "b": {Dot: true}, "c": {Count: 99, More: true},
	}, []string{"a", "b", "c", "missing"})
	if want := (UnreadView{Count: 102, More: true}); total != want {
		t.Errorf("total = %+v, want %+v: counts added, and a dot adds nothing to them", total, want)
	}
	if got := unreadTotal(map[string]UnreadView{"b": {Dot: true}}, []string{"a", "b"}); got != (UnreadView{Dot: true}) {
		t.Errorf("total of a dot alone = %+v, want a dot", got)
	}
	if got := unreadTotal(map[string]UnreadView{"a": {Count: maxUnread}, "b": {Count: 1}}, []string{"a", "b"}); got != (UnreadView{Count: maxUnread, More: true}) {
		t.Errorf("total past the largest count = %+v, want %d and more", got, maxUnread)
	}
}

// TestUnreadCountsFromPageTitles checks the counts at the start of pages'
// titles: on the sub-tab, without the count in the title, and added up
// on the tab and the menu bar item, for pages that have loaded.
func TestUnreadCountsFromPageTitles(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w1", Title: "(3) general - Slack"})
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w2", Title: "[12] Inbox - Outlook"})
	h.advance(50 * time.Millisecond)
	v := h.view()
	if got := v.Items["w1"].Unread; got != (UnreadView{Count: 3}) {
		t.Errorf("Slack's unread = %+v, want 3", got)
	}
	if got := v.Items["w2"].Unread; got != (UnreadView{Count: 12}) {
		t.Errorf("Outlook's unread = %+v, want 12", got)
	}
	if got := v.Items["w1"].Title; got != "general - Slack" {
		t.Errorf("Slack's title = %q, want the page's title without its count", got)
	}
	// The add dialog's rows and the toasts name the page without it too.
	mustOK(t, h.e.SwitchTab(1))
	opts, err := h.e.AddOptions("p12")
	mustOK(t, err)
	if i := slices.IndexFunc(opts, func(o AddOption) bool { return o.Source.ID == "w1" }); i < 0 || opts[i].Label != "general - Slack" {
		t.Errorf("the add dialog's row for Slack's page: %+v", opts)
	}
	mustOK(t, h.e.AddItemToTab("w1", 1))
	if got := h.em.lastToast(); got != "Showing Slack — general - Slack in Code too." {
		t.Errorf("toast: %q", got)
	}
	mustOK(t, h.e.Undo(h.em.lastUndo(t)))
	mustOK(t, h.e.SwitchTab(0))
	// A name of the user's own stays as it is, and the page's title loses
	// its count.
	mustOK(t, h.e.RenameItem("w2", "(1) Mail"))
	if it := h.item("w2"); it.Title != "(1) Mail" || it.AutoTitle != "Inbox - Outlook" || it.Unread != (UnreadView{Count: 12}) {
		t.Errorf("renamed Outlook: title %q, page title %q, unread %+v; want (1) Mail, Inbox - Outlook, and 12", it.Title, it.AutoTitle, it.Unread)
	}
	mustOK(t, h.e.RenameItem("w2", ""))
	v = h.view()
	if it := v.Items["w2"]; it.Title != "Inbox - Outlook" || it.Label != "" || it.Unread != (UnreadView{Count: 12}) {
		t.Errorf("Outlook named by its page again: title %q, label %q, unread %+v; want Inbox - Outlook, none, and 12", it.Title, it.Label, it.Unread)
	}
	if got := v.Tabs[0].Unread; got != (UnreadView{Count: 15}) {
		t.Errorf("Comms' unread = %+v, want 15", got)
	}
	if got := v.Tabs[1].Unread; got != (UnreadView{}) {
		t.Errorf("Code's unread = %+v, want none", got)
	}
	bar := h.lastMenuBar()
	if bar.Unread != (UnreadView{Count: 15}) || bar.Tabs[0].Unread != (UnreadView{Count: 15}) || bar.Tabs[0].Label() != "Comms — 15 Unread" {
		t.Errorf("menu bar item: unread %+v, Comms %+v (%q); want 15", bar.Unread, bar.Tabs[0].Unread, bar.Tabs[0].Label())
	}

	// Read, the page drops its count.
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w1", Title: "general - Slack"})
	h.advance(50 * time.Millisecond)
	if got := h.view().Tabs[0].Unread; got != (UnreadView{Count: 12}) {
		t.Errorf("Comms' unread = %+v, want 12 once Slack has none", got)
	}
	if got := h.lastMenuBar().Unread; got != (UnreadView{Count: 12}) {
		t.Errorf("the menu bar item's unread = %+v, want 12", got)
	}

	// Turned off, the counts go, and the titles keep them.
	h.saveSetting(func(s *SettingsView) { s.UnreadCounts = false })
	h.advance(50 * time.Millisecond)
	v = h.view()
	if v.Items["w2"].Unread != (UnreadView{}) || v.Tabs[0].Unread != (UnreadView{}) || h.lastMenuBar().Unread != (UnreadView{}) {
		t.Errorf("with unread_counts off: Outlook %+v, Comms %+v, menu bar %+v; want none", v.Items["w2"].Unread, v.Tabs[0].Unread, h.lastMenuBar().Unread)
	}
	if got := v.Items["w2"].Title; got != "[12] Inbox - Outlook" {
		t.Errorf("with unread_counts off, Outlook's page title = %q, want it whole", got)
	}
	if savedConfig(t, h).Settings.UnreadCounts {
		t.Error("config.toml should have unread_counts = false")
	}
}

// TestUnreadCountsFromPagesOnThisMac checks that a page on this Mac, which
// may be http (decision 63), counts as an https page does, once it has
// loaded, and that a page of a site with a web app keeps its web app as it
// counts.
func TestUnreadCountsFromPagesOnThisMac(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		slackWorld(p)
		p.SetDefaultBrowser(safariID)
	}})
	mustOK(t, h.e.SwitchTab(1))
	_, err := h.e.AddWebsite("p12", "localhost:3000")
	mustOK(t, err)
	h.settle()
	items := h.webItems("http://localhost:3000")
	if len(items) != 1 {
		t.Fatalf("Web items for http://localhost:3000 = %+v, want one", items)
	}
	id := items[0].ID
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: id, Title: "(2) Dev server"})
	h.advance(50 * time.Millisecond)
	v := h.view()
	if it := v.Items[id]; it.Unread != (UnreadView{Count: 2}) || it.Title != "Dev server" {
		t.Errorf("the page on this Mac: unread %+v, title %q; want 2, and the title without its count", it.Unread, it.Title)
	}
	if got := v.Tabs[1].Unread; got != (UnreadView{Count: 2}) {
		t.Errorf("Code's unread = %+v, want 2", got)
	}
	if got, err := h.e.WebAppChoice(id); err != nil || got != (WebAppChoice{}) {
		t.Errorf("WebAppChoice for the page on this Mac = %+v, %v; want none", got, err)
	}

	// Slack's page, of an https site, counts too, and still offers the web
	// app that Brave made of it.
	mustOK(t, h.e.SwitchTab(0))
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w1", Title: "(3) general - Slack"})
	h.advance(50 * time.Millisecond)
	if got := h.view().Items["w1"].Unread; got != (UnreadView{Count: 3}) {
		t.Errorf("Slack's unread = %+v, want 3", got)
	}
	got, err := h.e.WebAppChoice("w1")
	mustOK(t, err)
	if want := (WebAppChoice{Offer: "use", App: "Slack", Browser: "Brave"}); got != want {
		t.Errorf("WebAppChoice for Slack's page = %+v, want %+v", got, want)
	}
}

// TestUnreadCountsOnlyFromLoadedPages checks that a page that hasn't
// loaded since Bench started has no count, though its title from last
// time had one, which it shows without: not while its tab isn't shown, nor
// once its web view is made, until the page reports its title. Closing
// the view forgets it again.
func TestUnreadCountsOnlyFromLoadedPages(t *testing.T) {
	data := savedState(t, func(h *harness) {
		h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w2", Title: "(12) Inbox - Outlook"})
		h.drain()
		mustOK(t, h.e.SwitchTab(1))
		h.settle()
	})
	h := newHarness(t, setup{state: writeState(data)})
	v := h.view()
	if v.Current != 1 {
		t.Fatalf("started on tab %d, want Code", v.Current)
	}
	if w2 := v.Items["w2"]; w2.Unread != (UnreadView{}) || w2.Title != "Inbox - Outlook" {
		t.Errorf("Outlook before its page loads: unread %+v, title %q; want none, and the title without its count", w2.Unread, w2.Title)
	}
	if got := v.Tabs[0].Unread; got != (UnreadView{}) {
		t.Errorf("Comms' unread = %+v before its pages load, want none", got)
	}
	mustOK(t, h.e.SwitchTab(0))
	h.settle()
	if _, made := h.p.WebViews()["w2"]; !made {
		t.Fatal("Outlook's web view should be made as Comms shows")
	}
	if got := h.view().Items["w2"].Unread; got != (UnreadView{}) {
		t.Errorf("Outlook while its page loads: unread %+v, want none until it reports its title", got)
	}
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w2", Title: "(4) Inbox - Outlook"})
	h.advance(50 * time.Millisecond)
	v = h.view()
	if got := v.Items["w2"].Unread; got != (UnreadView{Count: 4}) {
		t.Errorf("Outlook once its page has loaded: unread %+v, want 4", got)
	}
	if got := v.Tabs[0].Unread; got != (UnreadView{Count: 4}) {
		t.Errorf("Comms' unread = %+v once Outlook's page has loaded, want 4", got)
	}

	// A view that closes, as when the config file gives the page another
	// address, forgets that the title is the page's own: the next page's
	// count shows once it has loaded.
	h.e.lock()
	h.e.closeWebViewLocked(h.e.model.Items["w2"])
	h.e.changed()
	h.e.unlock()
	h.advance(50 * time.Millisecond)
	if got := h.view().Items["w2"].Unread; got != (UnreadView{}) {
		t.Errorf("Outlook once its view has closed: unread %+v, want none until a page loads again", got)
	}
}

// TestPoppedOutPageKeepsItsCount checks that a page moved out of Bench
// into a window of its own, where it has no pill, keeps the count at the
// start of its title, which the page goes on changing there.
func TestPoppedOutPageKeepsItsCount(t *testing.T) {
	h := newHarness(t, setup{})
	h.p.EmitEvent(platform.Event{Kind: platform.WebTitle, ItemID: "w2", Title: "(12) Inbox - Outlook"})
	h.advance(50 * time.Millisecond)
	mustOK(t, h.e.PopOut("w2"))
	if wv := h.p.WebViews()["w2"]; !wv.PoppedOut || wv.Title != "Outlook — (12) Inbox - Outlook" {
		t.Errorf("popped-out Outlook: %+v, want the title Outlook — (12) Inbox - Outlook", wv)
	}
}

// TestUnreadCountsFromDockBadges checks the apps' Dock badges: on the
// first of an app's items in tab order, said to be the whole app's when
// it has others, and added up on its tab.
func TestUnreadCountsFromDockBadges(t *testing.T) {
	h := newHarness(t, setup{})
	if !h.p.WatchingDockBadges() {
		t.Fatal("Bench should read the Dock's badges while unread_counts is on")
	}
	h.p.SetDockBadge(braveID, "4")
	h.p.SetDockBadge(zedID, "•")
	h.p.SetDockBadge(obsidianID, "0")
	h.advance(50 * time.Millisecond)

	brave := h.appItems(braveID)
	if len(brave) != 3 {
		t.Fatalf("Brave has %d items, want 3", len(brave))
	}
	first := layout.Items(h.view().Tabs[2].Root)[0]
	for _, it := range brave {
		want := UnreadView{}
		if it.ID == first {
			want = UnreadView{Count: 4, WholeApp: true}
		}
		if it.Unread != want {
			t.Errorf("Brave's %s (%s): unread %+v, want %+v", it.ID, it.Title, it.Unread, want)
		}
	}
	zed := h.appItems(zedID)
	if len(zed) != 1 || zed[0].Unread != (UnreadView{Dot: true}) {
		t.Errorf("Zed's items %+v, want one with a dot", zed)
	}
	if got := h.appItems(obsidianID)[0].Unread; got != (UnreadView{}) {
		t.Errorf("Obsidian's badge of 0: unread %+v, want none", got)
	}
	v := h.view()
	if got := v.Tabs[2].Unread; got != (UnreadView{Count: 4}) {
		t.Errorf("Research's unread = %+v, want 4", got)
	}
	if got := v.Tabs[1].Unread; got != (UnreadView{Dot: true}) {
		t.Errorf("Code's unread = %+v, want a dot", got)
	}
	bar := h.lastMenuBar()
	if bar.Unread != (UnreadView{Count: 4}) || bar.Tabs[1].Label() != "Code — Unread" {
		t.Errorf("menu bar item: unread %+v, Code %q; want 4, and Code with something unread", bar.Unread, bar.Tabs[1].Label())
	}

	// Without Accessibility permission, the Dock can't be read. Bench finds
	// it has lost the permission as it arranges the next tab.
	h.p.SetTrusted(false)
	mustOK(t, h.e.SwitchTab(1))
	h.settle()
	if h.p.WatchingDockBadges() {
		t.Error("Bench should stop reading the Dock's badges without Accessibility")
	}
	if got := h.view().Tabs[2].Unread; got != (UnreadView{}) {
		t.Errorf("without Accessibility, Research's unread = %+v, want none", got)
	}

	// Nor can it with unread_counts off.
	h.p.SetTrusted(true)
	h.advance(2 * trustEvery)
	if !h.p.WatchingDockBadges() || h.view().Tabs[2].Unread != (UnreadView{Count: 4}) {
		t.Errorf("with Accessibility again: watching %v, Research's unread %+v; want 4", h.p.WatchingDockBadges(), h.view().Tabs[2].Unread)
	}
	h.saveSetting(func(s *SettingsView) { s.UnreadCounts = false })
	h.settle()
	if h.p.WatchingDockBadges() {
		t.Error("Bench should stop reading the Dock's badges with unread_counts off")
	}
	if got := h.view().Tabs[2].Unread; got != (UnreadView{}) {
		t.Errorf("with unread_counts off, Research's unread = %+v, want none", got)
	}
}

// TestNoDockBadgesWithoutAccessibility checks that Bench doesn't ask to
// read the Dock while it doesn't have Accessibility permission.
func TestNoDockBadgesWithoutAccessibility(t *testing.T) {
	h := newHarness(t, setup{untrusted: true})
	if h.p.WatchingDockBadges() {
		t.Error("Bench reads the Dock's badges without Accessibility permission")
	}
}
