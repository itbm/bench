// Package mac is Bench's bridge to macOS. Its Platform implements
// platform.Platform with cgo and Objective-C: the Accessibility API for other
// apps' windows and the badges on the Dock's icons, NSWorkspace for
// launching, hiding, and activating apps, NSScreen for displays, WKWebView
// for Web items, ScreenCaptureKit for live previews, UserNotifications for
// web notifications, Carbon's Text Input Sources for the input sources that
// the user can switch between, and ServiceManagement for whether macOS
// needs the user to allow Bench's login item.
//
// The native code builds only on macOS with cgo and without the server tag
// (//go:build darwin && cgo && !server). Everywhere else the package holds
// just its pure-Go helpers, which are unit-tested on any OS: coordinate
// conversion, the name-based UUID for a web profile, the web navigation and
// popup policies (and the navigator, which decides by what came before,
// and the debug log of its decisions, see logDecision), what a Web item's
// page is (its host and site, and whether it's secure, see PageInfo), what
// its pages' alerts and requests for the camera say and which may come (see
// promptGate), the live preview's geometry, the input sources' shortcuts
// as Bench's shortcuts (see systemKeysFrom), the addresses of System
// Settings' panes (see settingsURL), the answers to finding text in a page
// (see finds), which bundle IDs may name an app's icon and the <link>
// elements a page's script reports for its icon (see iconLinks), when to
// read the badges on the Dock's icons (see badgeWatcher), and the event
// queue. The browser preview uses PageInfo too, for its placeholder
// pages. Page icons are fetched and decoded in Go, by package icons, never
// by native code.
//
// # Threads
//
// AppKit and WebKit run on the main thread. Go never waits for the main
// thread: work that needs no answer is sent there with dispatch_async, and
// what Go needs to read (screens, Bench's content frame) is cached as the
// main thread changes it. Accessibility calls run on the calling goroutine,
// or on a serial dispatch queue for observer work, never on the main thread.
// Apps' icons are drawn on a serial dispatch queue of their own, from the
// asset server's goroutine.
// An app that doesn't answer one of them in time is asked nothing more for
// that call, and for a few seconds the Platform's calls about its windows
// fail at once.
//
// Native callbacks into Go never block and never take Bench's locks. They
// build a platform.Event and hand it to the event queue, which sends it on
// the buffered Events channel without waiting. When the channel is full,
// moves and resizes, and Web items' titles, addresses, states, and icons,
// are coalesced (only the latest frame per window, and text, state, or icon
// per item, is kept, unless other events about it came between) and other events
// wait, in order, in a bounded overflow list.
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
// removed. Nor is it sent if the window turns out to be open still, because
// its app replaced the window's Accessibility element but kept its window
// ID. Window and the actions on the window return errors wrapping
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
// over other apps' windows. It goes back too while another of Bench's
// windows, such as Acknowledgements or a popped-out page, has the keyboard,
// so that it doesn't float over that window.
//
// Windows and Window include windows that the Accessibility API leaves out
// of an app's list but that are still on this desktop: those in native full
// screen, minimised ones, and those of a hidden app.
//
// # Web items
//
// WebURL is sent when a Web item's page comes into view (WebKit commits
// it), when the page changes its own address, with pushState or a link to
// a part of the page, and when it loads content that isn't secure. It isn't
// sent for a page that's still loading: such a page may never come into
// view, and the item's pane header shows the host of the page in its view.
// WebKit's URL and hasOnlySecureContent describe pages that are still
// loading too, so Bench reads them at each commit, and between commits
// only while no navigation is under way or about to start, and a change of
// address then must stay on the page's origin. At a commit, WebKit's URL is
// still that of a load Bench asked for that hasn't started, if there is
// one: Bench then takes the address the committing navigation reached.
// PageInfo works out the rest of the event from the address.
//
// WebStateChanged is sent when a Web item's web view can or can't go back
// or forward any more, and when its page starts or stops using the camera
// or microphone, or mutes it, as WebKit's key-value observable canGoBack,
// canGoForward, cameraCaptureState, and microphoneCaptureState say. Each
// carries the whole state, so one may repeat the last. A page's sound
// alone isn't reported: WebKit has nothing to observe for it.
//
// # Coordinates
//
// The Platform's coordinates are global: points, with the origin at the
// top-left of the primary display and y increasing downwards, as the
// Accessibility API uses. AppKit's coordinates start at the bottom-left of
// the primary display with y increasing upwards; only this package sees
// them, and it converts with y_global = primaryHeight - (y_appkit + height).
package mac
