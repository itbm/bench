package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/platform"
)

// pageOf is what an item's view says about the page it shows.
type pageOf struct {
	URL, Host, Site    string
	OffSite, NotSecure bool
}

func (h *harness) pageOf(id string) pageOf {
	h.t.Helper()
	it := h.item(id)
	return pageOf{URL: it.URL, Host: it.Host, Site: it.Site, OffSite: it.OffSite, NotSecure: it.NotSecure}
}

// showPage reports a page in a Web item's view, as its web view does.
func (h *harness) showPage(id, address string, page platform.Page) {
	h.p.EmitEvent(platform.Event{Kind: platform.WebURL, ItemID: id, Body: address, Page: page})
	h.drain()
}

func TestWebPageInView(t *testing.T) {
	h := newHarness(t, setup{})
	if got, want := h.pageOf("w1"), (pageOf{URL: "https://app.slack.com/client"}); got != want {
		t.Errorf("before its view reports a page, Slack has %+v, want %+v", got, want)
	}
	tests := []struct {
		name    string
		address string
		page    platform.Page
		want    pageOf
	}{
		{"the item's own site", "https://app.slack.com/client/T1/C2",
			platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true},
			pageOf{URL: "https://app.slack.com/client/T1/C2", Host: "app.slack.com", Site: "slack.com"}},
		{"another site", "https://evil.example/slack",
			platform.Page{Host: "evil.example", Site: "evil.example", Secure: true, OffSite: true},
			pageOf{URL: "https://evil.example/slack", Host: "evil.example", Site: "evil.example", OffSite: true}},
		{"not secure", "http://slack.com/",
			platform.Page{Host: "slack.com", Site: "slack.com"},
			pageOf{URL: "http://slack.com/", Host: "slack.com", Site: "slack.com", NotSecure: true}},
		{"another site, not secure", "http://example.com/",
			platform.Page{Host: "example.com", Site: "example.com", OffSite: true},
			pageOf{URL: "http://example.com/", Host: "example.com", Site: "example.com", OffSite: true, NotSecure: true}},
		{"a blob: page on another site", "blob:https://evil.us.auth0.com/6f1c2a9e",
			platform.Page{Host: "evil.us.auth0.com", Site: "auth0.com", Secure: true, OffSite: true},
			pageOf{URL: "blob:https://evil.us.auth0.com/6f1c2a9e", Host: "evil.us.auth0.com", Site: "auth0.com", OffSite: true}},
		{"back to the item's own site", "https://app.slack.com/client",
			platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true},
			pageOf{URL: "https://app.slack.com/client", Host: "app.slack.com", Site: "slack.com"}},
		// A page with no web address mustn't look like one still to load.
		{"no web address", "about:blank", platform.Page{OffSite: true},
			pageOf{URL: "about:blank", OffSite: true, NotSecure: true}},
		{"no web address, with only secure content", "data:text/html,hi", platform.Page{OffSite: true, Secure: true},
			pageOf{URL: "data:text/html,hi", OffSite: true}},
		{"a page the view doesn't describe", "https://app.slack.com/client", platform.Page{},
			pageOf{URL: "https://app.slack.com/client"}},
	}
	for _, tt := range tests {
		h.showPage("w1", tt.address, tt.page)
		if got := h.pageOf("w1"); got != tt.want {
			t.Errorf("%s: Slack has %+v, want %+v", tt.name, got, tt.want)
		}
	}
	// The other Web item's page is its own.
	if got, want := h.pageOf("w2"), (pageOf{URL: "https://outlook.office.com/mail/"}); got != want {
		t.Errorf("Outlook has %+v, want %+v", got, want)
	}
}

func TestWebItemsFirstLoadWithNoAddress(t *testing.T) {
	h := newHarness(t, setup{})
	// The view shows about:blank for an address WebKit can't load: before
	// any page has been in view, that's the item's first load, which shows
	// no host, and no warning.
	h.showPage("w1", "about:blank", platform.Page{OffSite: true})
	if got, want := h.pageOf("w1"), (pageOf{URL: "about:blank"}); got != want {
		t.Errorf("Slack's first page has %+v, want %+v", got, want)
	}
	// After a page, one with no web address is on another site.
	h.showPage("w1", "https://app.slack.com/client",
		platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true})
	h.showPage("w1", "about:blank", platform.Page{OffSite: true})
	if got, want := h.pageOf("w1"), (pageOf{URL: "about:blank", OffSite: true, NotSecure: true}); got != want {
		t.Errorf("after a page, about:blank has %+v, want %+v", got, want)
	}
}

func TestOnlyWebPagesHaveAHost(t *testing.T) {
	h := newHarness(t, setup{})
	page := platform.Page{Host: "evil.example", Site: "evil.example", OffSite: true}
	for _, id := range []string{"w3", "w5"} {
		h.showPage(id, "http://evil.example/", page)
		if it := h.item(id); it.URL != "" || it.Host != "" || it.Site != "" || it.OffSite || it.NotSecure {
			t.Errorf("%s (%s) has a page: %+v", id, it.App, it)
		}
	}

	// VS Code for the Web's page is on the Mac itself, and its address
	// holds the server's connection token.
	h = newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) { return newFakeServer(0), nil }
	}})
	h.advance(100 * time.Millisecond)
	h.showPage("w1", "http://127.0.0.1:9999/?tkn=secret&folder=%2Fcode",
		platform.Page{Host: "127.0.0.1", Site: "127.0.0.1", Secure: true})
	if it := h.item("w1"); it.URL != "" || it.Host != "" || it.Site != "" || it.OffSite || it.NotSecure {
		t.Errorf("VS Code for the Web has a page: %+v", it)
	}
}

func TestWebPageIsntSaved(t *testing.T) {
	h := newHarness(t, setup{})
	h.showPage("w1", "https://evil.example/slack",
		platform.Page{Host: "evil.example", Site: "evil.example", OffSite: true})
	h.restart()
	data, err := os.ReadFile(filepath.Join(h.dir, config.StateFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"host"`, `"site"`, `"offSite"`, `"notSecure"`} {
		if strings.Contains(string(data), key) {
			t.Errorf("state.json has %s:\n%s", key, data)
		}
	}
	// The item keeps its address, but loads the one it was given, so it
	// shows no host until its view reports the page.
	if got, want := h.pageOf("w1"), (pageOf{URL: "https://evil.example/slack"}); got != want {
		t.Errorf("after a restart, Slack has %+v, want %+v", got, want)
	}
	if got := h.p.WebViews()["w1"].View.URL; got != "https://app.slack.com/client" {
		t.Errorf("Slack loads %q after a restart, want the address it was given", got)
	}
}

func TestOpenInBrowser(t *testing.T) {
	var opened []string
	h := newHarness(t, setup{options: func(o *Options) {
		o.OpenLink = func(address string) error { opened = append(opened, address); return nil }
	}})
	// Before its view reports a page, the address it was given opens.
	mustOK(t, h.e.OpenInBrowser("w1"))
	h.showPage("w1", "https://app.slack.com/client/T1/C2",
		platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true})
	mustOK(t, h.e.OpenInBrowser("w1"))
	h.showPage("w1", "http://slack.com/x", platform.Page{Host: "slack.com", Site: "slack.com"})
	mustOK(t, h.e.OpenInBrowser("w1"))
	// A blob: page opens as the site it belongs to, and any other page with
	// no web address as the item's own address.
	for _, address := range []string{
		"blob:https://evil.us.auth0.com/6f1c2a9e", "blob:http://app.slack.com@evil.example:8080/1",
		"about:blank", "data:text/html,hi", "blob:null/1", "blob:data:text/html,hi", "blob:https:///1",
	} {
		h.showPage("w1", address, platform.Page{OffSite: true})
		mustOK(t, h.e.OpenInBrowser("w1"))
	}
	mustErr(t, h.e.OpenInBrowser("w3"), "Only web pages open in a browser.")
	mustErr(t, h.e.OpenInBrowser("w5"), "Only web pages open in a browser.")
	mustErr(t, h.e.OpenInBrowser("w99"), "That window has closed.")
	want := []string{"https://app.slack.com/client", "https://app.slack.com/client/T1/C2", "http://slack.com/x",
		"https://evil.us.auth0.com", "http://evil.example:8080",
		"https://app.slack.com/client", "https://app.slack.com/client", "https://app.slack.com/client",
		"https://app.slack.com/client", "https://app.slack.com/client"}
	if !slices.Equal(opened, want) {
		t.Errorf("opened %q, want %q", opened, want)
	}

	// After a restart, the view loads the address the item was given, not
	// its last page, which state.json keeps: that address opens until the
	// view reports a page.
	opened = nil
	h.showPage("w1", "https://evil.example/slack",
		platform.Page{Host: "evil.example", Site: "evil.example", Secure: true, OffSite: true})
	h.restart()
	h.e.opts.OpenLink = func(address string) error { opened = append(opened, address); return nil }
	if got := h.item("w1").URL; got != "https://evil.example/slack" {
		t.Fatalf("after a restart, Slack's address is %q, want its last page", got)
	}
	mustOK(t, h.e.OpenInBrowser("w1"))
	h.showPage("w1", "https://app.slack.com/client/T1",
		platform.Page{Host: "app.slack.com", Site: "slack.com", Secure: true})
	mustOK(t, h.e.OpenInBrowser("w1"))
	if want := []string{"https://app.slack.com/client", "https://app.slack.com/client/T1"}; !slices.Equal(opened, want) {
		t.Errorf("after a restart, opened %q, want %q", opened, want)
	}

	// The browser preview can't open the default browser.
	h = newHarness(t, setup{})
	mustErr(t, h.e.OpenInBrowser("w1"), "This build of Bench can't open links.")

	h = newHarness(t, setup{options: func(o *Options) {
		o.OpenLink = func(string) error { return errors.New("no browser") }
	}})
	mustErr(t, h.e.OpenInBrowser("w2"), "Bench couldn't open outlook.office.com.")

	// VS Code for the Web's address holds the server's connection token.
	opened = nil
	h = newHarness(t, setup{config: vscodeConfig, options: func(o *Options) {
		o.StartVSCode = func(ctx context.Context) (VSCodeServer, error) { return newFakeServer(0), nil }
		o.OpenLink = func(address string) error { opened = append(opened, address); return nil }
	}})
	h.advance(100 * time.Millisecond)
	mustErr(t, h.e.OpenInBrowser("w1"), "Only web pages open in a browser.")
	if len(opened) != 0 {
		t.Errorf("VS Code for the Web opened %q", opened)
	}
}
