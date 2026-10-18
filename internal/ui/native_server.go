//go:build server

package ui

// nativeShell is false in the browser preview (Wails' server mode), which
// serves the UI over HTTP and has no windows, menus, menu bar item, global
// shortcuts (the frontend handles keys itself), or Open at login.
const nativeShell = false
