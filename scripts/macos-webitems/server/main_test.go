package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerServesAndLogs(t *testing.T) {
	var buf bytes.Buffer
	l := &requestLog{w: &buf, now: func() time.Time { return time.UnixMilli(1_700_000_000_250) }}
	h := newHandler("item", "https://127.0.0.1:9443", l)

	get := func(path string, header map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "https://localhost:8443"+path, nil)
		for k, v := range header {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// Bench's own load of the item's address, or the browser opening an
	// address that Bench passed it: no page asked.
	w := get("/a/", map[string]string{"Sec-Fetch-Site": "none", "Sec-Fetch-Dest": "document"})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `<iframe src="https://127.0.0.1:9443/a/frame">`) {
		t.Errorf("/a/ = %d %q, want the page with a frame from the other site", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store, so that every reload reaches the log", got)
	}
	// A frame's navigation of the page.
	get("/a/away", map[string]string{"Sec-Fetch-Site": "same-origin", "Referer": "https://127.0.0.1:9443/a/frame"})
	// A browser without Fetch Metadata, with and without a page behind it.
	get("/b/away", nil)
	get("/b/away", map[string]string{"Referer": "https://localhost:8443/b/hop"})
	if w := get("/beacon?e=d-alert&n=3", nil); w.Code != http.StatusNoContent {
		t.Errorf("/beacon = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w := get("/favicon.ico", nil); w.Code != http.StatusNotFound {
		t.Errorf("/favicon.ico = %d, want %d", w.Code, http.StatusNotFound)
	}

	var got []entry
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("a log line isn't JSON: %q: %v", line, err)
		}
		got = append(got, e)
	}
	want := []entry{
		{T: 1_700_000_000.25, Site: "item", Method: "GET", Path: "/a/", FetchSite: "none", FetchDest: "document", Fresh: true},
		{T: 1_700_000_000.25, Site: "item", Method: "GET", Path: "/a/away", FetchSite: "same-origin", Referer: "https://127.0.0.1:9443/a/frame"},
		{T: 1_700_000_000.25, Site: "item", Method: "GET", Path: "/b/away", Fresh: true},
		{T: 1_700_000_000.25, Site: "item", Method: "GET", Path: "/b/away", Referer: "https://localhost:8443/b/hop"},
		{T: 1_700_000_000.25, Site: "item", Method: "GET", Path: "/beacon", Query: "e=d-alert&n=3", Fresh: true},
		{T: 1_700_000_000.25, Site: "item", Method: "GET", Path: "/favicon.ico", Fresh: true},
	}
	if len(got) != len(want) {
		t.Fatalf("logged %d requests, want %d:\n%s", len(got), len(want), buf.String())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("log line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPagesNameTheOtherSite(t *testing.T) {
	var buf bytes.Buffer
	h := newHandler("other", "https://127.0.0.1:9443", &requestLog{w: &buf, now: time.Now})
	for path, want := range map[string]string{
		"/a/frame": `top.location.href = "https://127.0.0.1:9443/a/away"`,
		"/b/hop":   `location.replace("https://127.0.0.1:9443/b/away")`,
		"/c/":      `"\u0301".repeat(2000000)`,
		"/d/":      `location.reload()`,
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://127.0.0.1:9443"+path, nil))
		if body := w.Body.String(); !strings.Contains(body, want) || strings.Contains(body, "OTHER") {
			t.Errorf("%s lacks %q, or still says OTHER:\n%s", path, want, body)
		}
	}
}

func TestServerErrorsAreLogged(t *testing.T) {
	var buf bytes.Buffer
	l := &requestLog{w: &buf, now: time.Now}
	log.New(l.errors("other"), "", 0).Print("http: TLS handshake error from 127.0.0.1:50000: remote error: tls: unknown certificate authority")
	var e entry
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatalf("the line isn't JSON: %q: %v", buf.String(), err)
	}
	if e.Site != "other" || !strings.HasPrefix(e.Error, "http: TLS handshake error") || strings.HasSuffix(e.Error, "\n") {
		t.Errorf("logged %+v, want the other site's error, without its newline", e)
	}
}
