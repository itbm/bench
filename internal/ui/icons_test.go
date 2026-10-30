//go:build darwin || server

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/platform"
)

func TestIconHandler(t *testing.T) {
	p := platform.NewFake(nil)
	p.SetAppIcon("dev.zed.Zed", []byte("zed icon"))
	p.SetPageIcon("w1", []byte("slack icon"))
	h := iconHandler(p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("frontend " + r.URL.Path))
	}))
	tests := []struct {
		method, path string
		status       int
		body         string
	}{
		{"GET", "/icons/app/dev.zed.Zed.png", http.StatusOK, "zed icon"},
		{"GET", "/icons/app/dev.zed.zed.png", http.StatusOK, "zed icon"},
		{"GET", "/icons/page/w1.png?v=3", http.StatusOK, "slack icon"},
		{"HEAD", "/icons/page/w1.png", http.StatusOK, ""},
		// None, so the tile keeps its monogram.
		{"GET", "/icons/app/com.example.None.png", http.StatusNotFound, ""},
		{"GET", "/icons/page/w2.png", http.StatusNotFound, ""},
		// Not an icon's address.
		{"GET", "/icons/app/dev.zed.Zed", http.StatusNotFound, ""},
		{"GET", "/icons/app/.png", http.StatusNotFound, ""},
		{"GET", "/icons/app/a/b.png", http.StatusNotFound, ""},
		{"GET", "/icons/page/w1/../w1.png", http.StatusNotFound, ""},
		{"POST", "/icons/app/dev.zed.Zed.png", http.StatusMethodNotAllowed, ""},
		// Everything else is the frontend's.
		{"GET", "/index.html", http.StatusOK, "frontend /index.html"},
		{"GET", "/icons.png", http.StatusOK, "frontend /icons.png"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.status {
			t.Errorf("%s %s: status %d, want %d", tt.method, tt.path, rec.Code, tt.status)
			continue
		}
		if tt.method == "HEAD" && rec.Body.Len() != 0 {
			t.Errorf("HEAD %s wrote a body of %d bytes", tt.path, rec.Body.Len())
		}
		if tt.body != "" && rec.Body.String() != tt.body {
			t.Errorf("%s %s: body %q, want %q", tt.method, tt.path, rec.Body.String(), tt.body)
		}
		if tt.status == http.StatusOK && !strings.HasPrefix(tt.body, "frontend") {
			if got := rec.Header().Get("Content-Type"); got != "image/png" {
				t.Errorf("%s %s: Content-Type %q, want image/png", tt.method, tt.path, got)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("%s %s: X-Content-Type-Options %q, want nosniff", tt.method, tt.path, got)
			}
		}
	}
}
