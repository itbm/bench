//go:build darwin && !server

package ui

// nativeShell is true in the macOS app, which has a main window, an app
// menu, a menu bar item, global shortcuts, and Open at login.
const nativeShell = true
