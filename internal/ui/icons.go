//go:build darwin || server

package ui

import (
	"net/http"
	"strings"

	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/workspace"
)

// iconHandler serves the icons whose addresses the view gives the tiles of
// sub-tabs and lists (see workspace/icons.go): an app's, at
// workspace.AppIconPath, which the platform draws from its bundle, and a Web
// item's page's, at workspace.PageIconPath, which the platform fetched once
// the page had loaded. Every other request goes on to next, the frontend's
// files. An icon there's none of is not found, and the tile keeps its
// monogram.
//
// The asset server runs each request on a goroutine of its own, never the
// main thread, so drawing an app's icon the first time doesn't hold up the
// UI. A page's icon is served from Bench's own address, as a PNG that Go
// made, so the frontend never loads anything from a page's site.
func iconHandler(p platform.Platform, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var get func(name string) ([]byte, error)
		var name string
		var ok bool
		switch path := req.URL.Path; {
		case strings.HasPrefix(path, workspace.AppIconPath):
			name, ok = iconName(path, workspace.AppIconPath)
			get = p.AppIcon
		case strings.HasPrefix(path, workspace.PageIconPath):
			name, ok = iconName(path, workspace.PageIconPath)
			get = p.PageIcon
		default:
			next.ServeHTTP(w, req)
			return
		}
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !ok {
			http.NotFound(w, req)
			return
		}
		png, err := get(name)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		h := w.Header()
		h.Set("Content-Type", "image/png")
		h.Set("X-Content-Type-Options", "nosniff")
		// An app's icon stays the same while Bench runs, and a page's
		// address changes with its icon.
		h.Set("Cache-Control", "private, max-age=86400")
		if req.Method == http.MethodGet {
			_, _ = w.Write(png)
		}
	})
}

// iconName returns the name in an icon's path, between prefix and ".png": a
// bundle ID or an item ID, which has no slash.
func iconName(path, prefix string) (string, bool) {
	name, ok := strings.CutSuffix(strings.TrimPrefix(path, prefix), ".png")
	if !ok || name == "" || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}
