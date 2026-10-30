package workspace

import (
	"testing"

	"github.com/itbm/bench/internal/platform"
)

// appIcon returns the address of an app's icon, as the view gives it.
func appIcon(bundleID string) string { return AppIconPath + bundleID + ".png" }

// TestIconsInTheView checks which items have icons: an App item has its
// app's, a terminal none, and a Web item none until its page's icon
// comes. Windows not in a tab have their apps' too.
func TestIconsInTheView(t *testing.T) {
	h := newHarness(t, setup{})
	for id, want := range map[string]string{
		"w3": appIcon(zedID),
		"w4": appIcon(vscodeID),
		"w5": "",
		"w1": "",
	} {
		if got := h.item(id).Icon; got != want {
			t.Errorf("%s's icon = %q, want %q", id, got, want)
		}
	}
	finder := floatingID(h.windowID(finderID, "Downloads"))
	found := false
	for _, f := range h.view().Floating {
		if f.ID == finder {
			found = true
			if f.Icon != appIcon(finderID) {
				t.Errorf("Finder's window's icon = %q, want Finder's", f.Icon)
			}
		}
	}
	if !found {
		t.Fatalf("Finder's window isn't among the windows not in a tab")
	}
	mustOK(t, h.e.SwitchTab(1))
	opts, err := h.e.AddOptions("p12")
	mustOK(t, err)
	rows := map[string]int{}
	for _, o := range opts {
		rows[o.Group]++
		switch {
		case o.Source.Type == sourceApp && o.Icon != appIcon(o.Source.BundleID):
			t.Errorf("the add dialog's %q row has icon %q, want its app's", o.Label, o.Icon)
		case o.Source.Type == sourceNewWindow && o.Icon != appIcon(o.Source.BundleID):
			t.Errorf("the add dialog's %q row has icon %q, want its app's", o.Label, o.Icon)
		case o.Group == groupBuiltin && o.Icon != "":
			t.Errorf("the add dialog's %q row has icon %q, want none", o.Label, o.Icon)
		}
	}
	if rows[groupApps] == 0 || rows[groupNew] == 0 || rows[groupBuiltin] == 0 {
		t.Errorf("the add dialog's rows by group: %v, want apps, new windows, and built-in items", rows)
	}
}

// TestPageIcons checks that a Web item gets its page's icon when its web
// view reports one, at an address that changes with each new icon, and
// loses it when the page has none, or its view closes.
func TestPageIcons(t *testing.T) {
	h := newHarness(t, setup{})
	icon := func(has bool) {
		h.p.EmitEvent(platform.Event{Kind: platform.PageIconChanged, ItemID: "w1", HasIcon: has})
		h.drain()
	}
	icon(true)
	if got, want := h.item("w1").Icon, PageIconPath+"w1.png?v=1"; got != want {
		t.Errorf("Slack's icon = %q, want %q", got, want)
	}
	icon(true)
	if got, want := h.item("w1").Icon, PageIconPath+"w1.png?v=2"; got != want {
		t.Errorf("after a new icon, Slack's = %q, want %q", got, want)
	}
	icon(false)
	if got := h.item("w1").Icon; got != "" {
		t.Errorf("Slack's page has no icon now, but its item's is %q", got)
	}
	// A new icon never takes an old one's address, which the frontend may
	// have kept.
	icon(true)
	if got, want := h.item("w1").Icon, PageIconPath+"w1.png?v=3"; got != want {
		t.Errorf("Slack's icon = %q, want %q", got, want)
	}
	// The platform forgets a page's icon as its view closes.
	h.e.lock()
	h.e.closeWebViewLocked(h.e.model.Items["w1"])
	h.e.unlock()
	if got := h.item("w1").Icon; got != "" {
		t.Errorf("after its web view closed, Slack's icon is %q", got)
	}
	// Only Web items have pages.
	h.p.EmitEvent(platform.Event{Kind: platform.PageIconChanged, ItemID: "w3", HasIcon: true})
	h.drain()
	if got := h.item("w3").Icon; got != appIcon(zedID) {
		t.Errorf("Zed's icon = %q, want its app's", got)
	}
}

// TestNoIconForAnUninstalledApp checks that an app that macOS says isn't
// installed has no icon to show: its tile keeps the monogram.
func TestNoIconForAnUninstalledApp(t *testing.T) {
	h := newHarness(t, setup{world: func(p *platform.Fake) {
		standardWorld(p)
		p.AddApp(obsidianID, "Obsidian", platform.AppNotInstalled())
	}})
	mustOK(t, h.e.SwitchTab(2))
	if got := h.item("w7").Icon; got != appIcon(obsidianID) {
		t.Errorf("before Bench knows, Obsidian's icon = %q, want its app's", got)
	}
	mustOK(t, h.p.QuitApp(obsidianID))
	h.drain()
	mustErr(t, h.e.OpenItem("w7"), "Bench couldn't open Obsidian, because it isn't installed.")
	if got := h.item("w7").Icon; got != "" {
		t.Errorf("Obsidian isn't installed, but its icon is %q", got)
	}
}

// TestNoIconsInThePreview checks that the browser preview, whose platform
// has no icons, and `bench arrange` give no icons' addresses.
func TestNoIconsInThePreview(t *testing.T) {
	for name, set := range map[string]func(o *Options){
		"the preview":   func(o *Options) { o.Demo = true },
		"bench arrange": func(o *Options) { o.Headless = true },
	} {
		h := newHarness(t, setup{options: set})
		h.p.EmitEvent(platform.Event{Kind: platform.PageIconChanged, ItemID: "w1", HasIcon: true})
		h.drain()
		for _, id := range []string{"w1", "w3"} {
			if got := h.item(id).Icon; got != "" {
				t.Errorf("in %s, %s's icon is %q", name, id, got)
			}
		}
	}
}
