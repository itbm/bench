//go:build darwin || server

package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreviewGuard(t *testing.T) {
	const port = 8089
	const page = "http://localhost:8089"
	type request struct {
		method, path, host           string
		origin, referer, site, ctype string
	}
	call := request{method: "POST", path: "/wails/runtime", host: "localhost:8089",
		origin: page, referer: page + "/", site: "same-origin", ctype: "application/json"}
	with := func(change func(*request)) request {
		r := call
		change(&r)
		return r
	}
	tests := []struct {
		name  string
		req   request
		allow bool
	}{
		{"a bound method from the page", call, true},
		{"from the page at 127.0.0.1", with(func(r *request) {
			r.host, r.origin, r.referer = "127.0.0.1:8089", "http://127.0.0.1:8089", "http://127.0.0.1:8089/"
		}), true},
		{"with a charset", with(func(r *request) { r.ctype = "application/json; charset=utf-8" }), true},
		{"from a program, without Origin or Referer", with(func(r *request) { r.origin, r.referer, r.site = "", "", "" }), true},
		{"the runtime script", request{method: "GET", path: "/wails/runtime.js", host: "localhost:8089", referer: page + "/"}, true},
		{"the page itself, for any host", request{method: "GET", path: "/", host: "evil.example:8089"}, true},

		{"a GET with the call in its query (an <img>)", with(func(r *request) {
			r.method, r.origin, r.ctype, r.site, r.referer = "GET", "", "", "cross-site", "https://evil.example/"
		}), false},
		{"a GET from a browser that hides where it's from", with(func(r *request) {
			r.method, r.origin, r.referer, r.site, r.ctype = "GET", "", "", "", ""
		}), false},
		{"a form post", with(func(r *request) {
			r.origin, r.referer, r.site, r.ctype = "", "", "", "application/x-www-form-urlencoded"
		}), false},
		{"text", with(func(r *request) { r.ctype = "text/plain" }), false},
		{"another site", with(func(r *request) { r.origin = "https://evil.example" }), false},
		{"another local site", with(func(r *request) { r.origin = "http://localhost:3000" }), false},
		{"an opaque origin", with(func(r *request) { r.origin = "null" }), false},
		{"another site's page", with(func(r *request) { r.origin, r.referer = "", "https://evil.example/page" }), false},
		{"cross-site", with(func(r *request) { r.site = "cross-site" }), false},
		{"same-site", with(func(r *request) { r.site = "same-site" }), false},
		{"DNS rebinding", with(func(r *request) {
			r.host, r.origin, r.referer = "evil.example:8089", "http://evil.example:8089", "http://evil.example:8089/"
		}), false},
		{"DNS rebinding the runtime script", request{method: "GET", path: "/wails/runtime.js", host: "evil.example:8089"}, false},
		{"another port", with(func(r *request) { r.host = "localhost:9000" }), false},
		{"no port", with(func(r *request) { r.host = "localhost" }), false},
		{"an escaped path", with(func(r *request) {
			r.path, r.method, r.ctype, r.origin, r.referer = "/%77ails/runtime", "GET", "", "", ""
		}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			h := previewGuard(port)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
			req := httptest.NewRequest(tt.req.method, "http://localhost:8089"+tt.req.path, nil)
			req.Host = tt.req.host
			for name, value := range map[string]string{
				"Origin": tt.req.origin, "Referer": tt.req.referer,
				"Sec-Fetch-Site": tt.req.site, "Content-Type": tt.req.ctype,
			} {
				if value != "" {
					req.Header.Set(name, value)
				}
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if reached != tt.allow {
				t.Errorf("reached Wails = %v, want %v (status %d, %s)", reached, tt.allow, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
			if !tt.allow && rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}
}

func TestIsPreviewHost(t *testing.T) {
	tests := []struct {
		host string
		port int
		want bool
	}{
		{"localhost:8080", 8080, true},
		{"LOCALHOST:8080", 8080, true},
		{"127.0.0.1:8080", 8080, true},
		{"localhost", 80, true},
		{"localhost", 8080, false},
		{"[::1]:8080", 8080, false},
		{"0.0.0.0:8080", 8080, false},
		{"localhost.evil.example:8080", 8080, false},
		{"", 8080, false},
	}
	for _, tt := range tests {
		if got := isPreviewHost(tt.host, tt.port); got != tt.want {
			t.Errorf("isPreviewHost(%q, %d) = %v, want %v", tt.host, tt.port, got, tt.want)
		}
	}
}
