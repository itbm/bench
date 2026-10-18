// Package mac is Bench's bridge to macOS. Its Platform implements
// platform.Platform with cgo and Objective-C: the Accessibility API for other
// apps' windows, NSWorkspace for launching, hiding, and activating apps,
// NSScreen for displays, WKWebView for Web items, ScreenCaptureKit for live
// previews, and UserNotifications for web notifications.
//
// The native code builds only on macOS with cgo and without the server tag
// (//go:build darwin && cgo && !server). Everywhere else the package holds
// just its pure-Go helpers, which are unit-tested on any OS: coordinate
// conversion, the name-based UUID for a web profile, the web navigation and
// popup policies, the live preview's geometry, and the event queue.
//
// # Threads
//
// AppKit and WebKit run on the main thread. Go never waits for the main
// thread: work that needs no answer is sent there with dispatch_async, and
// what Go needs to read (screens, Bench's content frame) is cached as the
// main thread changes it. Accessibility calls run on the calling goroutine,
// or on a serial dispatch queue for observer work, never on the main thread.
//
// Native callbacks into Go never block and never take Bench's locks. They
// build a platform.Event and hand it to the event queue, which sends it on
// the buffered Events channel without waiting. When the channel is full,
// moves and resizes are coalesced (only the latest per window is kept) and
// other events wait, in order, in a bounded overflow list.
//
// # Window events
//
// WindowCreated is sent at most once per window. Windows that are already
// open when Bench starts aren't announced; Windows lists them. For an app
// that launches while Bench runs, the windows found once it can be watched
// are announced too, so WindowCreated can name a window that Windows has
// already returned.
//
// WindowDestroyed is sent a second after a window closes, and not at all if
// its app has quit by then: many apps close their windows as they quit, and
// an app quitting is reported by AppTerminated alone, as the platform's Fake
// does, so that its items wait for it to open again rather than being
// removed. Window and the actions on the window return errors wrapping
// platform.ErrNoWindow as soon as the window has gone, before
// WindowDestroyed is sent.
//
// # Bench's window
//
// BenchHidden is sent when Bench's window is hidden (Bench > Hide),
// minimised, or closed (which Wails turns into hiding it), and BenchShown
// when it's back. Bench being hidden while another app is active, as Hide
// Others in that app does, doesn't count: the user wants that app's windows,
// some of which may be docked in Bench, to stay. WebFocused is sent when the
// user clicks inside a Web item's web view in Bench's window, which Bench's
// UI doesn't see. While Bench is raised (SetRaised) but isn't the active
// app, its window goes back below normal windows, so that it doesn't float
// over other apps' windows.
//
// Windows and Window include windows that the Accessibility API leaves out
// of an app's list but that are still on this desktop: those in native full
// screen, minimised ones, and those of a hidden app.
//
// # Coordinates
//
// The Platform's coordinates are global: points, with the origin at the
// top-left of the primary display and y increasing downwards, as the
// Accessibility API uses. AppKit's coordinates start at the bottom-left of
// the primary display with y increasing upwards; only this package sees
// them, and it converts with y_global = primaryHeight - (y_appkit + height).
package mac
