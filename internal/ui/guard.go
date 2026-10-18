//go:build darwin || server

package ui

import (
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// previewPort is the browser preview's port when Options.Port is 0, as in
// Wails' server mode.
const previewPort = 8080

// previewGuard is the browser preview's asset-server middleware, which runs
// before Wails' own handlers. The preview's server has no authentication
// (docs/architecture.md, decision 8), and Wails' HTTP transport runs a bound
// method for any request to /wails/runtime, even a GET with the call in its
// query. Without this, any web page the user visits could call bound
// methods, with an <img> or a form, and so could a page whose own host name
// it rebinds to 127.0.0.1, which could also read what they return.
//
// So it refuses a request for /wails/ unless all of these hold:
//
//   - Its Host is localhost or 127.0.0.1 with the preview's port, which
//     defeats DNS rebinding.
//   - Its Origin and Referer, where it has them, are on such a host.
//   - Its Sec-Fetch-Site, where it has one, is same-origin or none.
//   - For /wails/runtime, it's a POST of application/json, as the Wails
//     runtime sends. A page on another site can only send that with an
//     Origin header, after a CORS preflight that the preview doesn't
//     answer, so it's refused even by a browser that sends no
//     Sec-Fetch-Site.
//
// It can't reach /wails/events, Wails' event WebSocket, or
// /wails/stream/ws, which Wails serves outside the asset server. Wails
// accepts those only from a page on the request's own host, which stops
// other sites, but not a page that rebinds its host name: such a page can
// listen to events (the view, and what the pretend terminals echo), though
// it still can't call bound methods.
func previewGuard(port int) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/wails/") {
				if why := refuseRequest(r, port); why != "" {
					http.Error(w, "Bench's preview refused this request: "+why+".", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// refuseRequest says why previewGuard refuses a request, or returns "".
func refuseRequest(r *http.Request, port int) string {
	if !isPreviewHost(r.Host, port) {
		return "it isn't for localhost:" + strconv.Itoa(port)
	}
	if origin := r.Header.Get("Origin"); origin != "" && !isPreviewPage(origin, port) {
		return "it comes from another site"
	}
	if referer := r.Header.Get("Referer"); referer != "" && !isPreviewPage(referer, port) {
		return "it comes from another site"
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
	default:
		return "it comes from another site"
	}
	if r.URL.Path == "/wails/runtime" {
		if r.Method != http.MethodPost {
			return "bound methods need a POST"
		}
		if media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || media != "application/json" {
			return "bound methods need JSON"
		}
	}
	return ""
}

// isPreviewHost reports whether a Host header names the preview: localhost
// or 127.0.0.1, with its port.
func isPreviewHost(hostport string, port int) bool {
	host, p, err := net.SplitHostPort(hostport)
	if err != nil {
		host, p = hostport, "80" // No port: HTTP's own.
	}
	if p != strconv.Itoa(port) {
		return false
	}
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1":
		return true
	}
	return false
}

// isPreviewPage reports whether an Origin or a Referer is one of the
// preview's own pages.
func isPreviewPage(address string, port int) bool {
	u, err := url.Parse(address)
	return err == nil && u.Scheme == "http" && u.User == nil && isPreviewHost(u.Host, port)
}
