package icons

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCandidates(t *testing.T) {
	for _, c := range []struct {
		name  string
		page  string
		links []Link
		want  []string
	}{
		{"no links: /favicon.ico", "https://app.slack.com/client/T1/C2", nil,
			[]string{"https://app.slack.com/favicon.ico"}},
		{"an icon, then /favicon.ico", "https://example.com/a", []Link{{Href: "https://example.com/i.png", Rel: "icon"}},
			[]string{"https://example.com/i.png", "https://example.com/favicon.ico"}},
		{"the smallest at least 40 first, then touch icons, then no size, then smaller",
			"https://example.com/", []Link{
				{Href: "https://example.com/16.png", Rel: "icon", Sizes: "16x16"},
				{Href: "https://example.com/any.ico", Rel: "shortcut icon"},
				{Href: "https://example.com/touch.png", Rel: "apple-touch-icon"},
				{Href: "https://example.com/192.png", Rel: "icon", Sizes: "192x192"},
				{Href: "https://example.com/48.png", Rel: "icon", Sizes: "32x32 48x48"},
			},
			[]string{"https://example.com/48.png", "https://example.com/192.png", "https://example.com/favicon.ico"}},
		{"touch icons before icons of no size", "https://example.com/", []Link{
			{Href: "https://example.com/any.ico", Rel: "icon"},
			{Href: "https://example.com/touch.png", Rel: "apple-touch-icon-precomposed"},
		}, []string{"https://example.com/touch.png", "https://example.com/any.ico", "https://example.com/favicon.ico"}},
		{"SVGs, masks, and other links left out", "https://example.com/", []Link{
			{Href: "https://example.com/i.svg", Rel: "icon"},
			{Href: "https://example.com/i", Rel: "icon", Type: "image/svg+xml"},
			{Href: "https://example.com/pin.png", Rel: "mask-icon"},
			{Href: "https://example.com/s.css", Rel: "stylesheet"},
			{Href: "https://example.com/m.json", Rel: "manifest"},
		}, []string{"https://example.com/favicon.ico"}},
		{"only http, https, and image data addresses", "https://example.com/", []Link{
			{Href: "javascript:alert(1)", Rel: "icon"},
			{Href: "file:///etc/passwd", Rel: "icon"},
			{Href: "data:text/html,<b>", Rel: "icon"},
			{Href: "data:image/svg+xml,<svg/>", Rel: "icon"},
			{Href: "data:image/png;base64,AAAA", Rel: "icon"},
		}, []string{"data:image/png;base64,AAAA", "https://example.com/favicon.ico"}},
		{"/favicon.ico once", "https://example.com/", []Link{{Href: "https://example.com/favicon.ico#x", Rel: "icon"}},
			[]string{"https://example.com/favicon.ico"}},
		{"a page with no web address has only its own icons", "about:blank", nil, nil},
		{"VS Code for the Web's page, on the Mac", "http://127.0.0.1:8123/?tkn=x", nil,
			[]string{"http://127.0.0.1:8123/favicon.ico"}},
	} {
		if got := Candidates(c.page, c.links); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\ngot  %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestFetch(t *testing.T) {
	icon := encodePNG(t, square(16, 16, red))
	var cookies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookies = append(cookies, r.Header.Get("Cookie"))
		switch r.URL.Path {
		case "/favicon.ico":
			_, _ = w.Write(icon)
		case "/moved.ico":
			http.Redirect(w, r, "/favicon.ico", http.StatusFound)
		case "/huge.ico":
			_, _ = w.Write(make([]byte, MaxBytes+1))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	client := NewClient()
	ctx := context.Background()
	for _, path := range []string{"/favicon.ico", "/moved.ico"} {
		if got, err := Fetch(ctx, client, srv.URL+path); err != nil || string(got) != string(icon) {
			t.Errorf("Fetch(%s) = %d bytes, %v; want the icon", path, len(got), err)
		}
	}
	if _, err := Fetch(ctx, client, srv.URL+"/missing.ico"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("Fetch of a missing icon = %v, want a 404", err)
	}
	if _, err := Fetch(ctx, client, srv.URL+"/huge.ico"); !errors.Is(err, ErrTooBig) {
		t.Errorf("Fetch of a huge icon = %v, want ErrTooBig", err)
	}
	for _, c := range cookies {
		if c != "" {
			t.Errorf("a fetch sent cookies: %q", c)
		}
	}
	if _, err := Fetch(ctx, client, "file:///etc/passwd"); err == nil {
		t.Error("Fetch read a file")
	}
}

func TestFetchData(t *testing.T) {
	icon := encodePNG(t, square(16, 16, red))
	ctx := context.Background()
	if got, err := Fetch(ctx, nil, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(icon)); err != nil || string(got) != string(icon) {
		t.Errorf("base64 data = %d bytes, %v; want the icon", len(got), err)
	}
	if got, err := Fetch(ctx, nil, "data:image/x-icon,%00%00%01%00"); err != nil || string(got) != "\x00\x00\x01\x00" {
		t.Errorf("percent-encoded data = %q, %v", got, err)
	}
	if _, err := Fetch(ctx, nil, "data:text/html;base64,PGI+"); !errors.Is(err, ErrNotImage) {
		t.Errorf("HTML data = %v, want ErrNotImage", err)
	}
	huge := "data:image/png;base64," + base64.StdEncoding.EncodeToString(make([]byte, MaxBytes+10))
	if _, err := Fetch(ctx, nil, huge); !errors.Is(err, ErrTooBig) {
		t.Errorf("huge data = %v, want ErrTooBig", err)
	}
}

// fakeFetches answers Pages' fetches from a map of addresses to bytes,
// counting them.
type fakeFetches struct {
	mu      sync.Mutex
	files   map[string][]byte
	fetched []string
}

func (f *fakeFetches) fetch(_ context.Context, _ *http.Client, address string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetched = append(f.fetched, address)
	if data, ok := f.files[address]; ok {
		return data, nil
	}
	return nil, errors.New("404 Not Found")
}

func (f *fakeFetches) set(address string, data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[address] = data
}

func (f *fakeFetches) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.fetched)
}

// changes records Pages' changed calls.
type changes struct {
	mu   sync.Mutex
	seen []string
	ch   chan struct{}
}

func (c *changes) changed(itemID string, has bool) {
	c.mu.Lock()
	what := itemID + "-"
	if has {
		what = itemID + "+"
	}
	c.seen = append(c.seen, what)
	c.mu.Unlock()
	c.ch <- struct{}{}
}

func (c *changes) wait(t *testing.T) {
	t.Helper()
	select {
	case <-c.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("no change came")
	}
}

func (c *changes) list() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.seen...)
}

func TestPages(t *testing.T) {
	ch := &changes{ch: make(chan struct{}, 10)}
	pages := NewPages(ch.changed)
	files := &fakeFetches{files: map[string][]byte{}}
	pages.fetch = files.fetch
	files.set("https://slack.com/favicon.ico", encodePNG(t, square(32, 32, red)))
	links := []Link{{Href: "https://slack.com/missing.png", Rel: "icon"}}

	// The first that can be read.
	pages.Found("w1", "https://slack.com/client", links)
	ch.wait(t)
	icon, ok := pages.Icon("w1")
	if !ok {
		t.Fatal("no icon after it was fetched")
	}
	if got := fitted(t, mustFit(t, icon)).NRGBAAt(20, 20); got != red {
		t.Errorf("the icon's middle is %v, want red", got)
	}
	if got := files.count(); got != 2 {
		t.Errorf("%d fetches, want 2: the link, then /favicon.ico", got)
	}

	// Another page of the site offers the same icons: nothing is fetched.
	pages.Found("w1", "https://slack.com/client/other", links)
	time.Sleep(20 * time.Millisecond)
	if got := files.count(); got != 2 {
		t.Errorf("%d fetches after the same icons, want 2", got)
	}

	// A page of another site with none that can be read: the item has no
	// icon.
	pages.Found("w1", "https://other.example/", nil)
	ch.wait(t)
	if _, ok := pages.Icon("w1"); ok {
		t.Error("the old site's icon stayed")
	}
	if got, want := ch.list(), []string{"w1+", "w1-"}; !reflect.DeepEqual(got, want) {
		t.Errorf("changes %q, want %q", got, want)
	}

	// Forgotten as its web view closes.
	files.set("https://other.example/favicon.ico", encodePNG(t, square(16, 16, blue)))
	pages.Found("w2", "https://other.example/", nil)
	ch.wait(t)
	pages.Forget("w2")
	if _, ok := pages.Icon("w2"); ok {
		t.Error("a forgotten item kept its icon")
	}
	pages.Close()
	pages.Found("w3", "https://other.example/", nil)
	time.Sleep(20 * time.Millisecond)
	if _, ok := pages.Icon("w3"); ok {
		t.Error("an icon was fetched after Close")
	}
}

// mustFit checks that Fit's own output fits again unchanged in size.
func mustFit(t *testing.T, data []byte) []byte {
	t.Helper()
	if _, err := Fit(data, Pixels); err != nil {
		t.Fatalf("an icon from Pages doesn't decode: %v", err)
	}
	return data
}
