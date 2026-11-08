# Bench architecture

This note records how Bench is built and why. It goes with the build spec
(`docs/spec.md`) and the UI prototype (`docs/prototype.html`). Where this note
and the spec differ, this note records a decision made while building; each
one is listed in [Decisions](#decisions).

## Contents

- [Packages](#packages)
- [Builds](#builds)
- [Coordinates](#coordinates)
- [Threads and locking](#threads-and-locking)
- [The window model](#the-window-model)
- [Go and the frontend](#go-and-the-frontend)
- [Files](#files)
- [Crash safety](#crash-safety)
- [Decisions](#decisions)

## Packages

| Package | What it does | cgo |
| --- | --- | --- |
| `cmd/bench` | Entry point: the app, and the `check-config`, `layout`, `arrange`, `restore`, and `watchdog` subcommands. | No |
| `internal/layout` | Trees, presets, unit rectangles, frames, dividers, size class, small-screen changes, neighbours, where a drop on a pane lands (`DropAt`, decision 38), and the divider beside a pane's edge, with the sizes that put it at a place (`DividerBeside` and `SizesAt`, decision 62). Pure Go. | No |
| `internal/config` | `config.toml` and `state.json`: load, validate, format, save, defaults. | No |
| `internal/platform` | The `Platform` interface, `ParkPlace` and `ParkFrame`, `Fake` for tests, `NewDemo` for the browser preview, and `Logged`, which logs calls and events when `BENCH_DEBUG` is set, and gives a `DebugLogger`, such as the macOS platform, the log for decisions of its own. | No |
| `internal/windows` | Docking, stacking, parking, hiding, and restoring other apps' windows, through `Platform`, parking an app's windows that are to stay out of sight before it shows the app (decision 67). | No |
| `internal/workspace` | The model (tabs, items, moves, rules) and the engine that runs it: platform events, switching, arranging, persistence, **Undo** and **Reopen Closed Tab** (decision 37), **Reopen Closed Page or Terminal** (decision 65), how long each arrangement takes, for **Report a Problem…** and the log (`timing.go`, decision 67), the drop-down terminal (`dropdown.go`, decision 72), files, folders, and apps dropped from the Finder (`drops.go`, decision 73), and the `View` sent to the frontend. | No |
| `internal/term` | Terminal sessions in pseudo-terminals (`creack/pty`). A session lasts until the engine removes its item, which closes it. `Busy` says whether a session is running something that ending it would stop, and what (decision 35). A session can type a command into its shell once the shell's line editor is ready, for launchers (`Options.Input`, decision 41), and `Host.LookPath` asks the user's interactive login shell which program a name runs, for **Browse…** (decision 41). A session reports the bell and the notifications its program asks for with OSC 9 and OSC 777 (`Callbacks.Alert`), and the folder its shell moves to (`Callbacks.Folder`), and can start with output to show first (`Options.Preface`), which `Restorable` makes safe to draw again (decisions 43 and 44). When Bench runs with SIGHUP ignored, as under `nohup`, it starts each shell through its own binary, run as a helper that puts SIGHUP back to its default action and then becomes the shell, so that closing a session still ends its programs. | No |
| `internal/vscode` | `code serve-web` for VS Code for the Web. | No |
| `internal/icons` | The small PNGs that tiles show in place of monograms: a web page's icon, chosen from its `<link>` elements or `/favicon.ico`, fetched without cookies, and decoded and scaled in Go (`Fit`, `Candidates`, and `Pages`, decision 51). Pure Go. | No |
| `internal/apps` | Installed apps, from the usual folders and the folders one level inside `/Applications`, including the web apps that browsers make and the sites they open, the browsers that can make them, monograms, and display names. | No |
| `internal/shortcuts` | The `[shortcuts]` config as global shortcut bindings, with what each field does, how macOS shows a shortcut, suggestions for one that's taken, and the clashes macOS doesn't report: VoiceOver's keys and the shortcuts macOS keeps (decision 45). | No |
| `internal/clock` | A clock that tests can move by hand. | No |
| `internal/update` | Bench's version against its latest release on GitHub, for the `check_for_updates` setting in a Bench without Sparkle, and the releases page that **Bench ▸ Check for Updates…** opens there. It only looks: Sparkle downloads and installs new versions (decision 48). | No |
| `internal/ui` | The Wails app: Bench's windows (the main window, popped-out terminals' windows, the drop-down terminal's panel, decision 72, and Acknowledgements), with the appearance setting's look (decision 52), the bound `Service`, events, global shortcuts, the menu bar item, the app menu, with the dots of tabs' colours in its Tab menu and **Pane ▸ Move to Tab** (decision 74), and **Report a Problem…** (decision 46), the log file (decision 47), whether Bench may quit (`ShouldQuit`, decision 35), opening Bench again after it quits, for **Quit and Reopen Bench** (`reopenAfterQuit`, decision 48), and the folder picker (decision 41), and the icons the tiles show, which the asset server serves (`iconHandler`, decision 51), and the drops from the Finder that Bench's windows get (`watchDrops`, decision 73). | Through Wails |
| `internal/mac` | macOS only: Objective-C for the Accessibility API, NSWorkspace, screens, window levels, web views, ScreenCaptureKit, and notifications. It implements `platform.Platform`, including `PressClose`, which closes another app's window as its close button does (decision 36), and `NewWindow`, which presses an app's **New Window** menu item, both finding the item by its title, in English or 17 other languages, and its key equivalent (`menus.go`, decision 75), and `ChooseToAdd`, the open panel of the add dialog's **Browse…** (decision 41), and `SetBadge`, the count on Bench's Dock icon (decision 43), and `SystemKeys`, the input sources' shortcuts and whether VoiceOver is on (decision 45), and `HeldModifiers`, the modifier keys held down now (`keys_darwin.m` and `hotkeys.go`, decision 60), and `OpenSettings` and `LoginItemNeedsApproval`, System Settings' panes for permissions and whether macOS needs the user to allow Bench's login item (decision 48), and `TrackDrags`, `ShowDropGuide`, and `HideDropGuide`, which follow windows dragged by their title bars and show where a drop on a pane goes (`drag_darwin.m`, `guide_darwin.m`, and `guide.go`, decision 13), and `MouseHeld`, whether the left mouse button is held now, for the end of a drag of a docked window's edge (`drag_darwin.m`, decision 62), and a web page's controls, `ReloadWebView`, `NavigateWebView`, `ZoomWebView`, `FindInWebView`, `SetCaptureState`, and `PauseWebViewMedia`, with `WebStateChanged` for what the page can do and how it uses the camera and microphone (`web_darwin.m` and `pagecontrols.go`, decision 50), and `LoadWebView`, which loads a page kept running in the background, hidden (`benchWebLoad`, decision 76), and `KeepWebViewRunning`, which has a hidden page loaded again after its process ends, or its first load fails, without waiting to be shown (`benchWebKeepRunning`, decision 76), and `StageManager`, whether Stage Manager is on, with `StageManagerChanged` (decision 53), and `WatchDockBadges` and `DockBadges`, the badges on apps' Dock icons, which it reads from the Dock's Accessibility items, with `DockBadgesChanged` (`ax_darwin.m` and `badges.go`, decision 61), and the events for the Mac waking, the user coming back to their session, and the user going to Bench's desktop (`Woke`, `ScreensWoke`, `SessionActive`, `ScreenUnlocked`, and `DesktopChanged`, decision 56), and `AppIcon` and `PageIcon`, an app's icon, which NSWorkspace draws, and a Web item's page's, with `PageIconChanged` (`apps_darwin.m`, `web_darwin.m`, and `icons.go`, decision 51), and `ToolPreferences`, a window tool's preferences that say which shortcuts it has, which it reads with `CFPreferencesCopyMultiple` (`keys_darwin.m` and `toolprefs.go`, decision 69). Outside the interface, `Updater` loads Sparkle at run time, if `Bench.app` has it, and implements `workspace.Updater` (`updater_darwin.m`, decision 48), `ShowDropdown` and `HideDropdown` slide the drop-down terminal's panel down from the top of the screen and back up, without making Bench active (`window_darwin.m`, decision 72), `SetWindowAppearance` gives Bench's own windows and panels the appearance setting, `SystemDark` says whether macOS is dark, before the app runs too, and Web items' web views follow macOS's appearance (`appearance.go`, decision 52). Its pure-Go helpers build everywhere, among them the Web items' navigation policy (decision 24), and the browser preview uses one, `PageInfo`, for its web pages' hosts. | Yes |
| `frontend` | The UI, from the prototype. Plain HTML, CSS, and ES modules, with no build step. `styles.css` has the dark colours and `light.css` the light ones, which `js/theme.js` chooses (decision 52). `js/tiles.js` draws apps' and pages' icons in place of monograms (decision 51). `js/drops.js` marks what takes drops from the Finder, and keeps other drops from loading in place of the page (decision 73). `acknowledgements.html` is the Acknowledgements window's page, which `build/acknowledgements` writes. | — |
| `build/acknowledgements` | A command, not part of the app: `make acknowledgements` runs it to write `frontend/acknowledgements.html` from what `Bench.app` links and includes, and its tests check that the page is current (decision 27). | No |

Only `internal/mac` contains cgo, but `internal/ui` needs cgo too on macOS,
through Wails, and builds on other systems only with the `server` tag.
Everything else builds and tests on any OS.

## Builds

| Build | Command | What you get |
| --- | --- | --- |
| The app (macOS) | `go build ./cmd/bench`, or `make app` for a signed `Bench.app` | Bench, using `internal/mac`. |
| Browser preview (any OS) | `CGO_ENABLED=0 go run -tags server ./cmd/bench`, or `make preview` | Wails' server mode: the same app with no native window, serving the UI at <http://localhost:8080>, backed by the demo platform (`platform.NewDemo`) and pretend terminals (`term.Echo`). Window contents are placeholders, as in the prototype. |
| Core tests (any OS) | `go test ./...` | The app's build. On Linux, `internal/ui` is left out, and `internal/mac` runs only its pure-Go tests; on macOS, both are included. |
| Preview tests | `CGO_ENABLED=0 go test -tags server ./...` and `node --test e2e/frontend/ e2e/app/` | The preview's build, with `internal/ui` in server mode; then the UI against a mock of Go (`e2e/frontend`) and the whole preview (`e2e/app`), in headless Chromium. |

Build the preview, and its Go tests, with `CGO_ENABLED=0`, as the Makefile
does: on Linux, with cgo, Wails needs GTK 4 and WebKitGTK.

Build constraints:

- `internal/mac`: `//go:build darwin && cgo && !server`.
- `internal/ui`: `//go:build darwin || server`. On Linux without `server`,
  Wails would need GTK, so the package is left out.
- `cmd/bench`: the subcommands build everywhere; running the app on a
  platform without `internal/ui` prints how to start the preview instead.

## Coordinates

- **Global**: points, origin at the top-left of the primary display, y
  downwards. This is the Accessibility API's convention, and what `Platform`
  uses for screens and windows.
- **Content**: points, origin at the top-left of Bench's window content area,
  y downwards. The frontend, pane rectangles, and web view frames use these.
  A docked window's global frame is its pane body plus
  `Platform.ContentFrame()`'s origin.
- AppKit uses the bottom-left of the primary display with y upwards. Only
  `internal/mac` sees AppKit coordinates. It converts with
  `y_ax = primaryHeight - (y_appkit + height)`.

## Threads and locking

- The workspace engine has one mutex. Bound methods, platform events, and
  timers all take it, so the model changes on one thread at a time.
- The engine never calls the frontend or blocks on it while holding the lock.
  It marks the view dirty, and a separate goroutine sends at most one
  `bench:state` per 16 ms. A tab's shortcut, and a hold after `last_tab`'s
  as its keys are let go of, sends its `switching` command between two
  holds of the lock: after the one that finds the tab, before the one that
  switches, which notes the current tab as it ends, for `switch-refused`
  after it (`switchShowing`, decision 67).
- Timing an arrangement takes no lock of its own and no hold more
  (`timing.go`, decision 67). What takes the lock reads the clock first,
  so that the wait for the lock counts, and notes the time and the reason
  in `Engine.held`; the arrangement notes its end there; and `unlock`
  keeps one time for the hold, in one of two rings of 50 that never grow,
  for the tab switches and for the rest, and logs it with `BENCH_DEBUG`,
  before it lets the lock go.
- Terminal output has a queue and lock of its own, and is sent without the
  engine's lock, at most once per 16 ms for each terminal, so an arrange
  that waits for a slow app doesn't hold it up. Typing doesn't take the
  engine's lock either, except in a terminal whose command has finished,
  where Return runs it again, and for a second Escape in the drop-down
  terminal, which hides it. An atomic tells the drop-down's typing apart
  (`dropKeys`), and an Escape that waits there has a small mutex of its
  own (`escMu`, decision 72). Only titles and ends of sessions are
  applied under it.
- `internal/mac` runs AppKit work on the main thread. Native callbacks into Go
  (AX notifications, web view delegates, NSNotifications) never take Go locks
  or block: they make an `Event` and hand it to a queue. An event goes
  straight onto the `Events` channel, which holds 4,096, when there's room.
  When it's full, events wait, in order, in an overflow list that a
  goroutine feeds into the channel. A waiting move or resize is replaced by
  a newer move or resize of the same window, a waiting `BenchMoved` by a
  newer one, and a Web item's waiting title or page by a newer one for
  the same item, unless something else has happened to that window or item
  since, which it mustn't jump ahead of. A move or resize keeps `MouseDown`
  from the one it replaces. Events are dropped only once 65,536 are
  waiting.
- Wails asks whether Bench may quit (`ShouldQuit`) on the main thread, which
  mustn't wait for the engine's lock, as the engine may hold it while it
  waits for the main thread. So the engine answers there only if it can take
  the lock at once (`AskToQuitNow`, with `TryLock`), and otherwise the shell
  refuses the quit and asks on another goroutine, quitting again if it may
  (decision 35).
- Go never waits for the main thread, except for a native menu (see
  `ShowMenu` below). It sends AppKit work there with
  `dispatch_async`, and what it needs to read, such as the screens,
  Bench's content frame, and whether macOS shows scroll bars all the time,
  is cached as the main thread changes it. Alerts
  that web pages bring up (their own alerts, prompts to open other apps'
  links, and requests for the camera or microphone) run from the main run
  loop, with `CFRunLoopPerformBlock`,
  not from a block on the main queue: the queue is serial, so work sent
  there would wait until the alert closed.
- Accessibility calls run on the calling goroutine, or on a serial dispatch
  queue for observer work, never on the main thread. Each waits up to a
  second for an app that doesn't answer, often with the engine's lock held.
  So once one times out, `internal/mac` asks that app nothing more for the
  same call, and for the next 3 seconds Go's calls about its windows fail
  at once, with an error that says the app didn't respond; the observer's
  work still asks. An item whose window couldn't be read stays bound to it.
- `ChooseToAdd`, for **Browse…**, waits for the user, but not on the main
  thread: it sends the open panel there, and waits on a goroutine for the
  answer, which the panel's completion handler hands over on a channel
  with room for it, so the main thread never waits. Closing the platform
  answers a waiting call with nothing. The engine holds no lock meanwhile.
  `FindInWebView` waits for WebKit's answer the same way, numbering each
  request so that a late answer can't reach the next, for up to 3 seconds.
- `ShowMenu` waits for the user too, without the engine's lock, which it
  takes only to note the menu as open before, and as closed after
  (`openMenuLocked`, `closeMenuLocked`). Wails' `OpenContextMenu` runs the
  native menu on the main thread and waits there until it closes
  (`InvokeSync`): the one call that waits for the main thread. The bound
  method's goroutine holds no lock but the shell's `contextMenu.mu`, which
  keeps one menu open at a time and which the main thread never takes, so
  nothing can wait in a circle. AppKit runs the main run loop while the menu
  is open, so the work that Go sends there, with `dispatch_async` or
  `CFRunLoopPerformBlock`, goes on: the View, the docked windows, and the
  web views keep up meanwhile. A click on an item comes to Go through Wails'
  channel, a moment after the menu closes, so the shell waits up to 250 ms
  for it (`choiceWait`), on a channel of the menu's own, so that a late
  choice can't reach the next menu (decision 71).
- Between processes, a lock on `bench.lock` in the support folder makes
  the app, `bench arrange`, `bench restore`, and the watchdog take turns
  with `parked.json` (see [Crash safety](#crash-safety)).
- `bench arrange` has no Wails app to run the main thread's run loop, and
  macOS updates its list of running apps, and reports their windows, only
  while that runs. So its main goroutine runs the loop
  (`mac.RunMainLoop`), on the main thread, while another goroutine
  arranges.

## The window model

Bench uses approach A from the spec, with one addition.

- Bench's window sits one level below normal windows
  (`NSNormalWindowLevel - 1`). Docked App windows stay at the normal level, so
  they're always above Bench, inside their pane bodies. Clicking Bench's tabs,
  sub-tabs, and pane headers activates Bench without covering them. The click
  also acts, even while another app is active: see decisions 11 and 12. Their
  tooltips show then too, because `internal/mac` sets Bench's window's
  `allowsToolTipsWhenApplicationIsInactive`. But WebKit may not track the
  pointer in a window that isn't the key window, so nothing depends on
  hovering alone: what a tooltip says is also in the control's label, in a
  menu, or in a message that clicking shows.
- Web items are `WKWebView`s added to Bench's window above the UI's web view,
  positioned at their pane bodies. Terminals are drawn by the UI itself.
- **Raised mode.** Dialogs, live previews, drags, and the menus that the
  UI draws in HTML would be hidden behind docked windows and web views, and
  a drag can't reach a pane body that another app's window covers. So while
  any of them is open, the frontend calls `SetOverlay(true)`: Bench raises
  its window above the docked windows (`Platform.SetRaised`) and hides its
  web views. `SetOverlay(false)` puts Bench back below normal level, raises
  the docked windows again, and shows the web views. The docked windows are
  behind the dialog's scrim anyway, so nothing visible is lost. While Bench
  isn't the active app, its window goes back below normal windows even in
  raised mode, so that it doesn't float over other apps.
- **Native menus.** In the app, Bench's own menus (a tab's, a sub-tab's and
  the **⋯** window menu, an empty pane's, a terminal's, and the slim title
  bar's layout menu) are native macOS menus, which AppKit draws above every
  window. They need no raised mode, so the web pages and the docked windows
  stay in view while one is open (`ShowMenu`, decision 71). But a native
  menu closes as soon as another app becomes active, so while one is open
  the engine counts it as open, as it does raised mode
  (`menuOrDialogLocked`): nothing that would focus another app's window
  happens then, and Bench's window doesn't move back to the display that
  the user chose until it closes (decision 70). The browser preview draws
  the same menus in HTML, raised.
  So does the app for the status bar's lists of the windows that need
  attention and of notices and recent messages, and for a popped-out
  terminal's menu, in its own window.
- **Keyboard focus** follows the focused pane: its App window is focused, a
  Web item's view gets the keyboard, or the frontend focuses its terminal
  (decision 15).
- **A window in several tabs.** An item can be in one pane of each of
  several tabs (decision 20). Only the current tab is arranged, so its
  window is docked in that tab's pane; for parking and hiding, a window in
  the current tab counts only as the current tab's.
- **Places of closed windows.** When an app quits, its App items stay,
  without windows, so that its windows go back to them (decision 10). An
  App item without a window is hidden (`hiddenLocked`) unless Bench is
  opening its app for it right now: a tab switch or `OpenItem` waits for
  it while the app launches, or a pending placement that hasn't expired
  names it. When that ends without a window, Bench arranges the tab again
  (`arrangeWhenOpeningEndsLocked`), for the pane's other window to show.
  Without Accessibility permission, Bench can't tell which windows are
  open, so it hides nothing. Hiding changes only what's drawn, never the model: the view's
  trees, sub-tabs, counts, and lists leave hidden items out (`hidden` in
  an item's view marks them), and a pane whose active item is hidden shows
  its most recently used other item (`shownTreeLocked`), so the item comes
  back in its place, active if it was. A pane stays even if all its items
  are hidden, and its view's `remembered` lists the apps it's waiting for,
  one for each app with hidden items in it (the whole tab's, for the
  `tabs` view's pane), with the item that **Open** fills: the pane's
  active item if it's the app's, or else the app's most recently used. An
  item whose window Bench can't find, with no event saying that it closed,
  still shows until Bench checks again, so that another of the pane's
  windows doesn't take its place and give it back.
- **Bench hidden.** When Bench's window is hidden or minimised, the current
  tab's apps hide with it, and a tab shortcut brings Bench back on that tab
  (decision 16).
- **Minimised windows.** A docked window that the user minimises stays in
  the Dock until the user shows it again: its item is marked minimised,
  arranging leaves it out, and its pane shows another of its windows, or
  says it's minimised (decision 55).
- **Other windows.** Bench's other windows are ordinary windows at the
  normal level: popped-out web pages and terminals, web pages' popups, and
  Acknowledgements (decision 27). Raised mode floats only Bench's main
  window, and only while none of them has the keyboard, so that one that
  the user opens or clicks stays in front of it. Bench becoming active
  puts out of sight what covers its main window only while that window is
  shown, since the user may have clicked one of these instead: the app,
  hidden, or only the windows over Bench, parked, if the app has other
  windows to keep showing, unless `covering_windows` is `leave`
  (`covering.go`, decision 68). Windows that a `float = true` rule
  matches, such as picture-in-picture, don't count: they're meant to stay
  over Bench, which fills the screen by default (decision 30). A popped-out web page's or terminal's window has the
  identifier `bench.item:` and its item's ID, for `internal/mac` to report
  it getting and losing the keyboard (`PopOutFocused`, `PopOutUnfocused`)
  and the user dragging it (`WindowDragged`, `WindowDropped`), and has a
  **Back to Bench** control: a title bar button on a web page's window,
  which sends `DockRequested`, and a bar and a menu entry in a terminal's
  page, which call `BackToBench` (decision 13).
- The drop-down terminal is a panel of its own, which floats over every app,
  on every desktop and beside full-screen apps, and doesn't make Bench
  active as it takes the keyboard. It isn't tagged `bench.item:`, so it
  doesn't count as a popped-out window for **Add Front Window** or
  **Window ▸ Back to Bench**. Its level is one above the floating level,
  so it stays over Bench's main window in raised mode too (decision 72).
- Live previews are a native panel (`Platform.ShowPreview`) above everything,
  and so is the drop guide (`Platform.ShowDropGuide`), which lets the mouse
  through to the window being dragged under it.

## Go and the frontend

The frontend imports the Wails runtime from `/wails/runtime.js`, which the
asset server provides, and calls bound methods by name:

```js
import { Call, Events } from '/wails/runtime.js';
const SERVICE = 'github.com/itbm/bench/internal/ui.Service.';
await Call.ByName(SERVICE + 'SwitchTab', 2);
Events.On('bench:state', (e) => render(e.data));
```

`frontend/js/bridge.js` wraps this, and swaps in a mock when a test sets
`window.__BENCH_MOCK__` before the page loads.

### Bound methods

All are methods of `ui.Service`. Each returns an error for a refused action,
with a message to show the user as a toast (for example, a pane can't be split
by its only window). After any change, Go sends `bench:state`.

| Method | Effect |
| --- | --- |
| `Ready() (View, error)` | The current view. The frontend calls it on load. |
| `SetPaneArea(area layout.Rect)` | The pane area (between the title bar and the status bar, or the window's foot while `show_status_bar` is off) in content coordinates, measured by the frontend whenever it changes, as when the status bar is hidden. Go lays panes out in it. |
| `SwitchTab(index int)` | Switch tabs. |
| `StepTab(step int)` | Next (1) or previous (-1) tab, wrapping. |
| `LastTab()` | Back to the tab that was current before this one, as the `last_tab` shortcut and **Tab ▸ Last Used Tab** do, so that it goes back and forth between the two tabs used last. The View's `lastTab` names it. Refused until Bench has switched tabs, and once that tab is deleted (decision 49). While `last_tab`'s modifiers are held, it starts a hold instead, which switches once they're let go of, and during a hold it chooses the next tab (decision 60). A hold starts without a last used tab too, and its quick tap is refused then. |
| `StepHold(step int)` and `CancelHold()` | The overview's keys during a hold after the `last_tab` shortcut (decision 60): `StepHold` chooses the next tab (1) or the previous one (-1), wrapping, for Left and Right, or moves a row of cards, as the overview draws them, for Down (the row's length) and Up (minus it), stopping at the ends; `CancelHold` ends the hold without switching, for Escape or a click outside the overview. Each does nothing without a hold, as it may have ended just before. |
| `SetMode(mode string)` | `auto`, `wide`, or `compact`. |
| `ShowSideBySide(itemID string)` | **Show Side by Side (Wide Layout)**, in the `tabs` view, from the **⋯** menu, the jump palette, **Pane ▸ Show Side by Side (Wide Layout)**, and the pane's split button: change to the wide layout, so the tab shows its panes, and show the item in its pane, focused. A tab of two or more panes keeps its tree. In a tab of one pane, the item gets a pane of its own on the right, as `MoveItem` with `right` gives it. The toast says how to go back, with **Undo**, which puts back the mode and the arrangement. `MoveDirection("right")` does it too in the `tabs` view. Outside the `tabs` view it is `MoveItem` with `right`. Refused for a tab with one window. See decision 58. |
| `FocusPane(paneID string)` | Focus a pane. |
| `FocusUI()` | Give Bench's own UI the keyboard, for the field that renames a tab or sub-tab in place: take it from the window or page that the focused pane shows, which the switch of the click before a double-click on a tab may have given it, or that lowering Bench after a menu gives back to a page, and drop any focus still put off. The frontend calls it as the field opens, once a click's `SwitchTab` has answered (decision 39). |
| `ShowItem(paneID, itemID string)` | Show an item in its pane (or in the `compact` pane). Also focuses the pane. |
| `MoveItem(itemID string, tab int, paneID, zone string)` | `zone` is `center`, `left`, `right`, `top`, or `bottom`: a drop on a pane's body, which splits at its edges (decision 38). |
| `MoveItemAt(itemID string, tab int, paneID string, index int)` | A drop on a pane's header, and a sub-tab's **Move Left** and **Move Right**: put an item in a pane at `index` among its sub-tabs as drawn, without hidden ones, before the sub-tab there, or last for an index past them. It never splits. Along its own pane, only the order changes; from another pane or tab, it joins this one there. In the `tabs` view, `compact` stands for the tab's one strip, in reading order: the item stays in its pane if a sub-tab beside its new place is in it, or else joins the pane of the sub-tab it goes before, or after, leaving its own pane in place, even empty. Nothing changes, and there's no toast, for a place next to where it is. See decision 38. |
| `MoveItemToTab(itemID string, tab int)` | Into that tab's focused pane, without switching tabs. Its toast has **Undo** and **Go there**, which calls `SwitchTab` with the tab. |
| `SendToTab(index int)` and `SendToLastTab()` | Move the focused pane's shown item into the focused pane of tab `index`, or of the tab that `LastTab` goes back to, as `MoveItemToTab` does: the `send_to_tab_modifiers` shortcut with 1 to 9 or &#96; (decision 57). Refused for an empty pane, for the current tab, and, for `SendToLastTab`, until Bench has switched tabs. |
| `AddItemToTab(itemID string, tab int)` and `RemoveItemFromTab(itemID string, tab int)` | Show an item in that tab's focused pane as well as where it is; take an item that's in several tabs out of one (see [The window model](#the-window-model)). |
| `MoveItemToNewTab(itemID string, also bool)` | **Move to Tab ▸ New Tab**, or with `also`, **Also Show In ▸ New Tab**: from a window's menu, the jump palette, and a sub-tab dropped on the tabs' **+**, or just after it (with Option held, `also`). In one step, add a tab with the single layout after the others, named after the item's app, numbered if a tab has that name (`uniqueTabName`), and move the item there, or show it there as well as where it is; then switch to it, and save `config.toml` with the tab and the item in it, as **Tab properties** saves a new tab. Its toast has **Undo**, which takes the tab away again, from `config.toml` too, and puts the item back. A popped-out item can move, but not also show. See decision 39. |
| `RenameItem(itemID, label string)` | A terminal's or web page's **Rename…**: give it a name of its own, one line of up to 200 characters, which its sub-tab, its tooltip, toasts, and the questions before closing show in place of the title that its shell or page sets. `""`, which **Use Automatic Title** sends, takes the name away. `state.json` keeps it; the item's view has it as `label`, with `title` showing it and `autoTitle` the title it hides. Refused for an app's window. See decision 39. |
| `FocusDirection(dir string)` | Focus the pane `left`, `right`, `up`, or `down`. |
| `MoveDirection(dir string)` | Move the focused pane's shown item into the neighbouring pane, or split there. |
| `CycleSubtab(step int)` | Next (1) or previous (-1) sub-tab in the focused pane. |
| `AddOptions(paneID string) ([]AddOption, error)` | The add dialog's rows, in group order: the launchers first, then windows not in a tab, and so on. **New terminal…** has `asksForCommand`, for the form that asks for a name, a command, and a folder. The first **Built in** row is **New terminal in *folder***, a `terminal` source with the folder that the shell of the terminal in the pane, or else the focused pane, is in, unless that's the home folder; **New terminal in…** has `choosesFolder`, for the frontend to ask for a folder (`ChooseFolder`), starting in that one (decision 44). |
| `AddToPane(paneID string, source Source)` | Add a floating item, an item from other tabs (as well as where it is), a new window, an installed app, a terminal, a launcher, or a web page. A `terminal` source with a `command` types it into the user's shell and runs it, with its `name` as the label until the shell sets a title, and `startAgain` (missing means true) says whether it runs again when Bench next opens, or is only typed. A `launcher` source's `id` names a `[[launcher]]`. See decision 41. |
| `AddWindowsToPane(paneID string, windows []string)` | The add dialog's ticked windows, by their IDs in `floating` (popped-out items, and `f` IDs of windows Bench doesn't manage) and `items` (those of other tabs), in the dialog's order: each moves to the pane, or shows there as well as where it is, as `AddToPane` adds a `floating` or `item` source, and the pane shows the first. If some couldn't be added, as they have closed or are in the tab already, a toast says how many were. See decision 54. |
| `AddWebsite(paneID, address string) (WebsiteResult, error)` | Add a website from the add dialog's **Web page…**: the site's web app if one is installed; else, if Bench can dock web apps and the default browser can make them, nothing yet, as it opens the site in the browser and returns the browser's steps; else a Web item, and why. See decision 21. |
| `AddWebAppWhenReady(paneID, address string) (bool, error)` | Add the site's web app to the pane once the browser has made it, and report whether it did. The add dialog calls it every 1.5 seconds while it waits. |
| `WebAppChoice(itemID string) (WebAppChoice, error)` | What a Web item's menu offers to put a web app of its site in its place: `use` an installed web app, named with the browser that made it; or else, if Bench can dock web apps and the default browser can make them, `make` one in that browser; or else nothing. Worked out afresh each time the menu opens. See decision 22. |
| `UseWebApp(itemID string)` | Put the installed web app of a Web item's site in the item's place, in every tab that shows it, and close the item's web view. If the item came from `config.toml`, its entries there become `app` entries for the web app, and the file is saved, as **Tab properties** saves it. |
| `MakeWebApp(itemID string) (WebsiteResult, error)` | Open a Web item's site in the default browser, for the user to make a web app there, and return the browser's steps, as `AddWebsite` does for a site with none. |
| `UseWebAppWhenReady(itemID string) (bool, error)` | Put the web app that the browser has made in the Web item's place, as `UseWebApp` does, once it's installed, and report whether it did. The dialog that `MakeWebApp` opens calls it every 1.5 seconds while it waits. |
| `PopOut(itemID string)` and `Redock(itemID string)` | **Pop Out of Bench**; dock an undocked window again (**Dock Again**), one that the user dragged out or that its app or a window tool kept moving out (status `moving`, decision 69), with its three tries again. An App item with no window has nothing to pop out, so `PopOut` removes it from every tab instead: the frontend calls that **Remove from Bench**, for an item whose view has `noWindow`. |
| `KeepOverBench(bundleID string)` | **Keep over Bench**, on the toast that says Bench hid an app, or a window of it, because it covered Bench, or on its recent message: add a `[[rule]]` with `app` and `float = true` to `config.toml`, which Bench saves, so that the app's windows stay over Bench and its new ones stay out of the tabs, and show again what Bench put out of sight. Its toast has **Undo**, which takes the rule back and puts the windows out of sight again while Bench is in front. Refused for an app that Bench hasn't said that of, and for one with items or a `[[rule]]` that sends its windows to a tab. The recent messages offer it again while the View's `keepOver` lists the app. See decision 68. |
| `AddFrontWindow()` | Add the window in front to the current tab's focused pane, if it's out of the tabs: a popped-out item, a window not in a tab, or one dragged out of its pane, which the View's `front` names. The window in front is the one the user last gave the keyboard to outside Bench's main window, not one that Bench focused itself. It says so with **Pop out**, or with **Undo** for a window dragged out of another pane, and brings back a hidden Bench. The `add_front_window` shortcut, **Add Front Window** in the jump palette, and **Pane ▸ Add Front Window** call it (decision 13). |
| `UseAsDropdown(itemID string)` | Make a terminal the drop-down, from its **⋯** menu: it leaves its pane, or its own window closes, and it shows at the top of the screen. The terminal that was the drop-down goes back into the pane that the new one leaves, or the current tab's focused pane. Its toast says how to show it again. Refused for anything but a terminal (decision 72). |
| `ToggleDropdown()` | Show the drop-down, or hide it while it shows, without bringing Bench forward: the `dropdown` shortcut, **View ▸ Show Drop-down**, the menu bar item's **Show Drop-down**, and the jump palette's. With no drop-down, a new terminal in the home folder becomes it (decision 72). |
| `HideDropdown()` | Hide the drop-down, if it shows: its bar's **Hide** and its menu's **Hide Drop-down**. Its session goes on (decision 72). |
| `BackToBench(itemID string)` | Put a popped-out item back in the current tab's focused pane, and bring Bench forward on it, saying so with **Pop out**: the **Back to Bench** button in a popped-out terminal's window. `""` means the popped-out window that has the keyboard, for **Window ▸ Back to Bench**. An item in a tab already stays where it is (decision 13). |
| `OpenItem(itemID string)` | **Open** on a pane that's waiting for an app: open the app of an App item without a window, or ask the running app for a new window, and put its next window in the item's place. The item shows, saying that Bench is opening its app, until the window comes or Bench gives up. Without Accessibility permission, it opens the app, for an item whose view has `canOpen`, which the item's pane offers then. |
| `RemoveRemembered(paneID, bundleID string)` | **Remove from this pane**: forget the places of an app in a pane that its windows would go back to, the hidden App items without a window. An item that's in other tabs too stays there. |
| `ClosePane(paneID string)` | **Close this pane**: close a pane that shows nothing, forgetting the places it's waiting for, so the rest of the tab closes up. Not the tab's last pane, and not in the `tabs` view. |
| `SetSizes(splitID string, sizes []float64)` | When the user drags a divider. |
| `EvenSizes(splitID string)` | When the user double-clicks a divider: give its split's panes equal sizes, through `SetSizes`, and say so in a toast, with **Undo**. A split whose panes look equal already, to a twentieth of a percent, is left as it is, without a toast. See decision 40. |
| `ZoomPane(paneID string)` and `ShowAllPanes()` | **Zoom Pane**, from a pane's **⋯** menu, the jump palette, **Pane ▸ Zoom Pane**, and the `zoom_pane` shortcut, and **Show All Panes**, from the same places and the pane's **Zoomed** chip: `ZoomPane` makes a pane of the current tab, or the focused pane for `""`, fill the tab's pane area on its own, with its own sub-tabs, and focuses it, while the windows of the tab's other panes are parked, as a pane's are while it shows a page or a terminal. The tab keeps its panes, sizes, and items, and the View's `display.zoomed` names the pane. The zoom ends with `ShowAllPanes`, or as the tab changes, another pane is focused, or the pane goes. Refused for a tab with only one pane, and in the `tabs` view, which shows one pane anyway. Not saved in `state.json`. See decision 40. |
| `GrowPane()`, `ShrinkPane()`, `EqualPanes()`, and `StepLayout(step int)` | The keys that arrange the current tab's panes, from the `grow_pane`, `shrink_pane`, `equal_panes`, `next_layout`, and `previous_layout` shortcuts, the jump palette, **Pane ▸ Grow Pane**, **Shrink Pane**, and **Make Panes Equal**, and **Tab ▸ Next Layout** and **Previous Layout**: `GrowPane` and `ShrinkPane` make the focused pane 5% of its split larger or smaller, along the split that holds it, keeping each pane at 10% or more (`layout.Resize`); `EqualPanes` gives every split of the tab equal sizes; `StepLayout` gives the tab the next (1) or previous (-1) of the six layouts, keeping its windows in order, and skips those without a pane for each of the tab's panes that have windows in them. Each says what it did in a toast, such as "Zed 55% · Terminal 45%" or "Layout: Main and side", with **Undo**. Refused in the `tabs` view, and `GrowPane` and `ShrinkPane` while a pane is zoomed; `EqualPanes` and `StepLayout` show every pane again. See decision 59. |
| `SplitPane(paneID, zone string) (string, error)` and `CancelSplit(paneID string)` | A window menu's **Split Right with an Empty Pane** and **Split Down with an Empty Pane**: `SplitPane` puts an empty pane beside a pane of the current tab, at the edge `zone` names (`left`, `right`, `top`, or `bottom`), focuses it, and returns its ID, for the add dialog that the frontend opens for it straight away. If the dialog closes without adding anything, `CancelSplit` takes the pane away again: the tab goes back to how it was, sizes and all, if nothing else has changed it since, or else the pane closes up as **Close this pane** closes it. A pane that has something in it by then stays, and so does one that a new window is on its way to; `CancelSplit` leaves any other pane alone. Refused in the `tabs` view. See decision 40. |
| `Rearrange()` | Arrange the current tab again. A tab's menu switches to another tab first. |
| `FillScreen()` | The jump palette's **Fill the screen**, as **View ▸ Fill the Screen** does: zoom Bench's window to fill its screen's visible area, unless it fills it already (decision 30). The browser preview refuses, and its palette leaves the row out. |
| `PreviewTree(index int, preset string) (*layout.Node, error)` | The tree that **Tab properties** would make for tab `index` (or a new tab, -1) with a preset, for its preview. |
| `SaveTab(index int, draft TabDraft)` | Save **Tab properties**: a tab's name, layout, small-screen setting, colour (`colour`, one of `config.TabColours`, or "" for none; another is refused, decision 74), and place among the tabs (`place`, from 1, which gives it its shortcut; 0 leaves it where it is, or puts a new tab last), which moves it as `MoveTab` does. `index` -1 adds a tab. A name that another tab has, ignoring case, is numbered (`uniqueTabName`): "Code" becomes "Code (1)", then "Code (2)", replacing a number in brackets that it ends with. Tab properties says so under the name first, working it out the same way. Renaming a tab in place, by double-clicking it or with **Rename…** in its menu, saves through `SaveTab` too, with the name typed, preset `custom`, which keeps the tab's panes and its `config.toml` layout, and the tab's own small-screen setting and colour (decision 39). The toast after a change to a tab that was there has **Undo**, named **Rename Tab** when only the name changed and **Change Tab Properties** otherwise; a new tab puts the changes before it out of Undo's reach (decisions 37 and 74). |
| `PreviewTabFromWindows(preset string, windows []string) (WindowsPreview, error)` | The **New tab** dialog's preview with **Windows that are open** chosen, for the windows ticked, by their IDs in `floating` and `items`: `{suggested, apps, root}`, the layout suggested by the number of apps (one app, `single`; two, `side`; three, `three`; four or more, `grid`), that number, and the tree that `NewTabFromWindows` would make with `preset` ("" for the suggested one), each app's windows in a pane of their own and the apps after the last pane in it. Windows that have gone, and hidden sub-tabs, are left out. See decision 54. |
| `NewTabFromWindows(draft TabDraft, windows []string)` | Make a tab of windows that are open, from the **New tab** dialog with **Windows that are open** chosen (**Tab ▸ New Tab from Open Windows…**, Jump to…'s **New Tab from Open Windows…**, and the welcome guide's **Choose windows…** open it so): the draft's name, or the first window's app's, numbered as `SaveTab` numbers it; the draft's layout, or the suggested one; and its small-screen setting, colour, and place. The windows fill the panes as the preview shows: those not in a tab move there, and those of other tabs show there as well, as **Also Show In** does. Then switch to the tab, and save `config.toml` with it as **Tab properties** saves a new tab. Its toast has **Undo**, which takes the tab away again, from `config.toml` too, and puts each window back where it was. See decision 54. |
| `SaveSettings(settings SettingsView)` | Save **Settings**: the `[settings]` that apply to every tab, such as `compact_below`, `density`, `pane_headers`, `live_previews`, `notify_terminals`, and `keep_terminal_output`, and the launchers, from **Settings ▸ Launchers**, which replace the `[[launcher]]` entries unless `launchers` is missing. One without a name is named as `SaveLauncher` names one, or an app launcher after its app. Turning live previews on asks for Screen Recording permission. Turning `keep_terminal_output` off removes the terminals' saved output (decision 44). Its `shortcuts` are the fields of the `[shortcuts]` table that **Settings ▸ Keyboard** changed, by their names in `config.toml`, such as `jump`, each a shortcut in config form or `""` for off; the others stay as they are. A change that gives the table a problem it didn't have, such as a shortcut that another field has, is refused, saying so. Changed shortcuts are registered again straight away, with no restart (`Emitter.ShortcutsChanged`, decision 45). |
| `PreviewSettings(preview SettingsPreview)` | Show what **Settings** has chosen and not saved yet: its `appearance`, `density`, and `gap`, each left out (`""`, or a gap below 0) to keep the setting. The View's `appearance`, `density`, `titleBar` (where it isn't the native title bar's), and `display` follow it, and the shell gives Bench's windows the appearance, but nothing is saved or arranged, so the docked windows stay where they are behind the raised Bench. A preview with none stops it, as do `SaveSettings`, `SetOverlay(false)`, and `Ready` (decision 66). |
| `CheckShortcuts(changes map[string]string) ([]ShortcutView, error)` | **Settings ▸ Keyboard**, as the user records a shortcut, turns one off, or uses a suggestion: whether each field of the `[shortcuts]` table would work with its changes, by the fields' names, without saving them. The engine asks the shell (`Emitter.CheckShortcuts`), which tries to register each binding that Bench doesn't hold already, releasing it again, and checks it against the shortcuts macOS keeps for itself and VoiceOver's, as it does when it registers them (decision 45). Each row has its `problem`, if any, and a `suggestion` that Bench checked it can register. A name that isn't a field is refused. |
| `PauseShortcuts(paused bool)` | Release Bench's global shortcuts while a field of **Settings ▸ Keyboard** records keys, so that the keys reach the page rather than run Bench's actions, and register them again with `paused` false. `SetOverlay(false)` and `Ready` register them again too, in case the page didn't. |
| `UseSuggestedShortcuts(fields string)` | A shortcut notice's **Use this instead**: set fields of the `[shortcuts]` table, by their names, separated by spaces, to the shortcuts the shell last suggested for them, save `config.toml` as **Settings** does, register them again, and say what changed in a toast. A field whose value has changed since the suggestion stays as it is; if none has one, it's refused. |
| `SaveLauncher(launcher LauncherView) (string, error)` | The **New terminal…** form's **Save as launcher**: add a `[[launcher]]` after the others and save `config.toml`, as **Settings** does, and return its name, for the terminal the form adds with it. One without a name is named after its command's program, or its folder, numbered if that's taken. A name that another launcher has, ignoring case, is refused, naming that launcher. See decision 41. |
| `ChooseFolder(start string) (string, error)` | The **Choose…** button of the **New terminal…** form and **Settings ▸ Launchers**, and the add dialog's **New terminal in…**: show macOS's folder picker as a sheet on Bench's window, starting in `start` (such as `~/code`), and return the folder chosen, with the home folder as `~`, or `""` if the user cancelled. It waits for the user, without the engine's lock. The browser preview refuses, and the forms say to type the folder; **New terminal in…** opens the **New terminal…** form at its folder field instead. |
| `BrowseToAdd(paneID string) (BrowseResult, error)` | The add dialog's **Browse…**, once the dialog has closed: lower Bench, show macOS's open panel as a window of its own (`Platform.ChooseToAdd`), and wait for the user, without the engine's lock. An app chosen is added to the pane, as the **Apps** rows add one (`added`, its name), if the tab is still the one it was chosen from. A program or script comes back for the **New terminal…** form to fill in (`command`, `name`, its file name, and `folder` for one run by its path): by its name when the user's shell finds that program by it (`term.Host.LookPath`), or else by its path, with what runs a script that can't run itself. A cancel returns nothing, and the dialog opens again. Anything else is refused, and so is a second panel while one is open. The browser preview refuses. See decision 41. |
| `DeleteTab(index int)` and `ResetLayout(index int)` | From **Tab properties** (**Delete tab** and **Reset layout**), and a tab's menu (**Delete Tab (Keep Windows)** and **Reset Layout**). Each says what it did in a toast, with **Undo**, since the tab may not be the current one. The last tab can't be deleted. A deleted tab joins the closed tabs that `ReopenTab` brings back. |
| `MoveTab(from, to int)` | Move tab `from` to index `to`, where it ends up: a drag along the tab strip, and **Move Tab Left** and **Move Tab Right** in a tab's menu, and the jump palette's **Move Tab Left** and **Move Tab Right**. (**Tab ▸ Move Tab Left** and **Move Tab Right** move the current tab in Go, without the frontend.) The tab takes the shortcut of its new place. The current tab stays current, each window stays in its tabs, and each tab keeps its arrangement. `config.toml`'s tabs take the same order, saved as **Tab properties** saves them, with `state.json` straight after. It says so in a toast, with **Undo**. Refused past either end. See decision 38. |
| `Undo(step int)` | A toast's **Undo**, after **Delete tab**, **Close Tab…**, **Reset layout**, **Close this pane**, a move by dragging or from a menu (`MoveItem`, `MoveItemAt`, `MoveItemToTab`, `MoveTab`), a move to another tab with the `send_to_tab_modifiers` keys (`SendToTab` and `SendToLastTab`), **Also Show In** (`AddItemToTab`), **Move to Tab ▸ New Tab** and **Also Show In ▸ New Tab** (`MoveItemToNewTab`), a tab made of open windows (`NewTabFromWindows`), a tab made of folders and apps dropped on the tabs' **+** (`Engine.DropFiles`, decision 73), **Remove from This Tab** (`RemoveItemFromTab`), **Show Side by Side (Wide Layout)** (`ShowSideBySide`), equal sizes from a divider (`EvenSizes`), the keys that arrange the panes (`GrowPane`, `ShrinkPane`, `EqualPanes`, and `StepLayout`, a run of which is one change), a drag of a docked window's edge that resizes the panes (decision 62), closing a web page or terminal (`CloseItem`), **Keep over Bench** (`KeepOverBench`, whose rule goes from `config.toml` again, leaving the arrangement as it is, decision 68), and **Save** in **Tab properties** for a tab that was there, or its rename in place (`SaveTab`), or a recent message's **Undo** while its change is the latest: put back the arrangement from before change `step`, and the layout's mode from before `ShowSideBySide`, taking back the changes since too, latest first. A deleted tab comes back with its `config.toml` entry and rules, which Bench saves, and its windows, pages, and terminals that are still open dock in it again; what has closed since stays closed, and what has come since stays where it is. A moved tab goes back to its place, in `config.toml` too, a tab that `SaveTab` changed gets its name, layout, small-screen setting, colour, and place back, in `config.toml` too, and a tab that `MoveItemToNewTab`, `NewTabFromWindows`, or a drop on the + added goes, from `config.toml` too, with the terminals that the drop made; the windows that `NewTabFromWindows` or the drop took from "Not in a tab" go back where they were. Refused once the change is out of reach: Bench keeps the last 10, and forgets them all when the tabs change otherwise, as when a tab is added in **Tab properties**, or `config.toml` is reloaded. See decisions 37 and 74. The toast of Bench's move back to the display that the user chose, and its recent message while the View's `wentBack` names it, call it too, with a number from the same count: that moves only the window back, and makes the display it was on the one chosen, leaving the changes and the arrangement as they are; refused with "Built-in Retina Display isn't connected, so Bench can't move back to it." once that display has gone (decision 70). |
| `UndoLatest()` | Take back the latest change that `Undo` can, as `Undo` does with its step, and say "Undid: *what it did*." (`undoName`): the jump palette's **Undo** row. **Tab ▸ Undo** and the `undo` shortcut call it in Go. Refused with "There's nothing to undo." when there's none. The View's `undo` names it. Bench's move back to the display that the user chose isn't one of these changes (decision 70). See decision 65. |
| `ReopenTab(name string)` | **Reopen Closed Tab**, from a tab's menu and the jump palette: bring back the tab of that name that was deleted or closed most recently, of the last 5, which `state.json` keeps, where it was among the tabs, and switch to it. Its `config.toml` entry and rules come back, which Bench saves, numbered if another tab has its name now (`uniqueTabName`). Its arrangement comes back as `RestoreArrangement` brings one back: its windows that are "Not in a tab" dock in it, its pages and terminals that closed open again, and what another tab has now stays there. The View's `closedTabs` names the tabs it can bring back. |
| `ReopenItem(itemID string)` | **Reopen Closed Page or Terminal**, from a tab's menu and the jump palette: open a web page or terminal that `CloseItem` closed again, with its ID, where it was, in each tab that had it and still does, in a pane of its own again if it had one, and switch to its tab. A terminal's session starts with the output it had, while `keep_terminal_output` is on. The View's `closedItems` lists the last 10, which Bench keeps in memory only. **Tab ▸ Reopen Closed Page or Terminal** calls it in Go. See decision 65. |
| `CloseTab(index int, sure, dontAsk bool) (CloseQuestion, error)` | A tab's **Close Tab…**, its ×, and a middle-click on it: close the tab's App windows, web pages, and terminals, other than those in other tabs too, which stay there, and delete the tab as `DeleteTab` does, with the rules that name it. The only tab stays, empty (**Close All Windows…**). Unless `sure`, it closes nothing, and returns the question instead (`ask`), when `confirm_close` is on and it would close two or more things, or a terminal that's running something, and, with `confirm_close` off, when it would end a terminal that's running something and `confirm_quit_busy_terminals` is on (`busyOnly`). The question has what it would close and what stays, how many of them the tab's `config.toml` entry lists, how many rules go, and the busy terminals; its **Close** calls again, `sure`. With `dontAsk` (**Don't ask again**), it turns `confirm_close` off, or for a `busyOnly` question, `confirm_quit_busy_terminals`, in the same save. See decision 36. |
| `CloseItem(itemID string, sure, dontAsk bool) (CloseQuestion, error)` | A sub-tab's **Close Window**, **Close Page**, or **Close Terminal**, or **Close Everywhere** for one in several tabs, its ×, and a middle-click on it: close the item, from every tab. The frontend calls `RemoveItemFromTab` instead for the × and middle-click on an item that's in other tabs too. Unless `sure`, with `confirm_quit_busy_terminals` on, a terminal that's running something closes nothing, and returns the question (`ask`), as closing its own window would; with `dontAsk`, it turns that setting off. See decision 36. |
| `TermInput(itemID, data string)` and `TermResize(itemID string, cols, rows int)` | Terminal input and size. Return in a terminal whose command has finished runs the command again. |
| `TermSnapshot(itemID string) (TermSnapshot, error)` | Recent output, base64-encoded, for a new terminal view to replay, and the offset where it ends (see [Terminal output](#terminal-output)). |
| `TermAck(itemID string, offset int64)` | A terminal view has drawn the output up to `offset`. |
| `TermClear(itemID string)` | A terminal view's **Clear** (⌘K): forget the session's recent output, so that `TermSnapshot` has none of it, nor does the output Bench keeps as it quits. Offsets carry on. See decision 42. |
| `OpenLink(address string)` | Open an `http` or `https` link that a terminal program printed, and the user Command-clicked, in the default browser. The browser preview refuses. |
| `OpenInBrowser(itemID string)` | Open the page that a Web item shows in the default browser: the one its web view last reported, or until it reports one, after a restart too, the address the item was given. For a page that isn't `http` or `https`, it opens a `blob:` page's inner origin if that is one, or else the item's own address. Only `http` and `https` addresses open, and not VS Code for the Web. The browser preview refuses. See decision 23. |
| `ReloadPage(itemID string)`, `GoBack(itemID string)`, and `GoForward(itemID string)` | A web page's **Reload Page**, first in its **⋯** and sub-tab menus, and **Back** and **Forward**, from their **Page** submenu, the jump palette's **This window**, and **Pane ▸ Page** (decision 50). `ReloadPage` loads the page in view again, or the item's own address if no page has come into view (`Platform.ReloadWebView`). `GoBack` and `GoForward` go back and forward in its history (`Platform.NavigateWebView`), and are refused while its web view says it can't (`WebStateChanged`), which the item's view has as `canGoBack` and `canGoForward`. Each is refused for a window that isn't a Web item, and until its web view has been made, as for VS Code for the Web while its server starts. |
| `ZoomPage(itemID string, step int)` | **Zoom In** (1), **Zoom Out** (-1), and **Actual Size** (0) for a Web item's page: to the next of 50, 67, 75, 80, 90, 100, 110, 125, 150, 175, 200, 250, and 300%, through `WKWebView.pageZoom` (`Platform.ZoomWebView`), saying so in a toast. Refused past either end. The item keeps it, as its view's `zoom` in percent, left out at 100%, and `state.json` saves it, for its web view to take whenever it's made (`WebView.Zoom`). |
| `PageLink(itemID string) (string, error)` | **Copy Page Link**: the address that `OpenInBrowser` would open, which the frontend puts on the clipboard. Refused for VS Code for the Web, whose address holds the server's connection token, and for a page with no web address. |
| `SetFindBar(itemID string, open bool)` and `FindInPage(itemID, text string, backwards bool) (bool, error)` | **Find in page**, from a page's menus, the jump palette, and **Pane ▸ Page ▸ Find in Page…** (the `find-in-page` command): `SetFindBar` opens or closes a Web item's find bar, which the frontend draws at the top of its pane's body while its view has `finding`, `layout.FindBarHeight` (36 pt) tall, as Go moves its web view down by as much, since the web view is drawn over Bench's UI. Opening it gives Bench's own UI the keyboard, for the bar's field; closing it gives the page the keyboard back, if its pane is focused. A page in a window of its own has no bar. `FindInPage` finds text in the page, ignoring case, after the match that's selected, or before it if `backwards`, going round at the end, and WebKit selects the match and scrolls to it (`Platform.FindInWebView`, `findString:withConfiguration:completionHandler:`). It reports whether there's a match, waiting for the page's answer, up to 3 seconds, without the engine's lock. |
| `MuteCapture(itemID, device string, mute bool)` and `StopMedia(itemID string)` | **Mute Microphone** and **Turn Off Camera**, and with `mute` false, **Unmute Microphone** and **Turn On Camera**, for a page that uses them (`device` is `camera` or `microphone`): `Platform.SetCaptureState`, through `setMicrophoneCaptureState:` and `setCameraCaptureState:`, on the item's page and its popups' pages that use it. The page stays in its call, and can't undo it itself. Refused for a device the page isn't using. **Stop Sound and Video** pauses every sound and video the page and its popups play (`Platform.PauseWebViewMedia`, `pauseAllMediaPlaybackWithCompletionHandler:`), which a page can play again; Bench can't tell whether a page plays sound, so it's always offered. Each says what it did in a toast. See decision 50. |
| `SetKeepRunning(itemID string, on bool)` | **Keep Running in the Background**, a checkbox at the end of a web page's page group, after its **Page** submenu, in its **⋯** and sub-tab menus, native or not (decision 71), the jump palette's **This window**, and **Pane ▸ Page**: turn it on or off for the page, as the user's choice, which a notification then leaves alone, and which ends the undo step of the page's first notification. On, a page that has no web view yet loads soon, hidden (`Platform.LoadWebView`), and every page with it on loads that way after Bench opens. Off, the page stays loaded until Bench quits, and loads only when it's shown from the next start. Either way, the platform hears whether its web view keeps running while it's hidden (`Platform.KeepWebViewRunning`). It saves `keep_running` in the page's entries in `config.toml`, and in `state.json`, and says what it did in a toast. Refused for VS Code for the Web, whose page needs its server, for a window that isn't a Web item, and for a page in no tab. See decision 76. |
| `ReloadConfig()` | **Reload config.toml**, from the Bench menu, the jump palette, and the config notices' **Reload**: read `config.toml` again and apply it as startup applies it to `state.json`, keeping the arrangement (decision 7). If the file has problems, or can't be read, Bench keeps the config it runs with and reports them; if it isn't there, Bench writes its config there. Changes that Bench couldn't save over the file give way to it. |
| `OverwriteConfig()` | The notice's **Overwrite**, after a save found `config.toml` changed outside Bench: write Bench's config over it, keeping that version as `config.toml.bak`. Refused while the file has problems. |
| `UseConfigBackup()` | The problems notice's **Use backup**: put `config.toml.bak` in place of a `config.toml` that has problems, if the backup loads without any, keeping the file with problems as `config.toml.bak`, and apply it as `ReloadConfig` does. |
| `OpenConfig()` and `ShowConfigInFinder()` | The problems notice's **Open config.toml**, in the default app for plain text (or TextEdit), and **Show in Finder**, through `Platform.OpenTextFile` and `Platform.ShowInFinder`. The browser preview refuses. |
| `RestoreArrangement()` and `DismissArrangementNotice()` | The arrangement notice's **Restore previous arrangement** and **Dismiss** (see [Files](#files)). `RestoreArrangement` gives each tab whose arrangement Bench couldn't keep, because `config.toml` had no tab of its name at startup or at a reload, that arrangement back, once a tab has its name again: its tree, with new pane and split IDs, and its items, which take the entry edits since, as at startup. A window that the tab's own items have goes back to the arrangement's item; one that another tab has now stays there. A page or terminal that only the tab has, for the same entry as one of the arrangement's, takes that one's place, so that its page or session goes on; the tab's other pages and terminals move to "Not in a tab", and so do its other windows. Items that a reload moved to "Not in a tab" as it deleted the tab come back. Refused when there's nothing to restore, or no tab has the name yet. `DismissArrangementNotice` takes the notices away; `state.json.bad` stays. |
| `SetOverlay(open bool)` | Raised mode on or off (see [The window model](#the-window-model)). Raising it also checks whether `config.toml` has changed outside Bench (decision 7). Lowering Bench after the jump shortcut opened the palette gives the keyboard back to the App window that had it (decision 15). |
| `ShowMenu(menu Menu) (string, error)` | Show one of Bench's own menus as a native macOS menu, without raised mode, so that the web pages and docked windows stay in view (decision 71): a tab's menu, a sub-tab's or the **⋯** window menu, an empty pane's, a terminal's, or the slim title bar's layout menu. `menu` is `{entries, x, y}`: the entries that the frontend's HTML menus draw, as data (`workspace.MenuEntry`, each one of an entry with its `id`, `label`, `keys`, `detail`, `dot` (the colour of a tab's dot, which the item shows as its image, or `none` for a blank one, decision 74), `disabled`, `checked`, and `submenu`; a separator, `sep`; a `heading`; a `note`; or a `status` line), and the point for the menu's top left corner, in the points of the UI's web view, from its top left, as the page's `clientX` and `clientY` count them. It waits until the menu closes, without the engine's lock, and returns the `id` of the entry chosen, or `""` if none was. Meanwhile the engine counts the menu as open, as it does raised mode: nothing focuses another app's window, which would close the menu, a tab switch's put-off focus is dropped, the hold after `last_tab` doesn't start, Bench's move back to the display that the user chose waits (decision 70), and the App window that the jump palette took the keyboard from gets it back only once the menu closes. Only an entry that's on, has no submenu, and isn't in a submenu that's off counts. A menu of no lines shows nothing, and one of more than 1,000 is refused. The View's `nativeMenus` says that Bench has native menus. The browser preview hasn't, and refuses: its frontend draws the menus in HTML. |
| `ShowPreview(itemID string, anchor layout.Rect)` and `HidePreview()` | Live previews, with the sub-tab's rectangle in content coordinates. |
| `RequestAccessibility()` and `RequestScreenRecording()` | Ask macOS for permission, and, while Bench doesn't have it, open System Settings at the permission's list too (`Platform.OpenSettings`), since macOS asks only once (decision 48). Once Bench has asked for Screen Recording, the View's `permissions.screenRecordingReopen` offers **Quit and Reopen Bench**, as macOS applies it only as Bench opens. |
| `SetOpenAtLogin(on bool)` and `OpenLoginItems()` | Open Bench at login. If macOS needs the user to allow Bench's login item (`Platform.LoginItemNeedsApproval`), turning it on opens System Settings ▸ General ▸ Login Items, sets the View's `loginApproval`, and returns an error that says so; `OpenLoginItems` opens Login Items again. While the View has `loginApproval`, Bench reads the login item again each time it comes forward. Refused where there's no Open at login, as in the browser preview. |
| `DismissFirstRun()` | Close the welcome guide for good. |
| `StarterTabs()` and `UseStarterTabs(names []string)` | The welcome guide's **Choose a starting set-up**, the first time Bench opens (decision 48). `StarterTabs` returns the tabs it suggests, `[{name, apps: [{bundleId, name, mono}]}]`: Comms, Code, and Plan, each made only of installed apps that no tab has, one for each pane, with one of Bench's terminals (no `bundleId`) beside an editor. A tab with no installed app, or whose name a tab has, isn't suggested. `UseStarterTabs` adds the tabs it names, as it would suggest them now, after the tabs there are, and saves `config.toml` as **Tab properties** saves a new tab, with a toast; a name it doesn't suggest any more is refused, and nothing is added. **Start with one tab** (or **Don't add tabs**, with more than one tab) adds none. |
| `SetDialogSize(kind string, w, h float64)` | Remember the size the user gave a dialog by resizing it from its corner, for it to open at that size next time: `palette` (Jump to…), `add` (the add dialog), `editor` (Tab properties), `settings`, or `guide` (the welcome guide). The View's `dialogs` has the sizes, and `state.json` saves them. Where the dialog was isn't saved. |
| `Quit(dontAsk bool)` and `CloseTerminal(itemID string, dontAsk bool)` | The **Quit** of the question that Bench asks before quitting while a terminal is running something, and the **Close** of the one it asks, in the terminal's own window, before closing a popped-out terminal's window while the terminal is running something (decision 35). `Quit` quits Bench without asking again. `CloseTerminal` ends the terminal and closes its window, as closing the window does when Bench doesn't ask. With `dontAsk` (**Don't ask again**), each first turns `confirm_quit_busy_terminals` off and saves `config.toml`, as **Settings** does; Bench quits even if it can't save it. The browser preview never asks, and its `Quit` does nothing. |
| `QuitAndReopen(sure, dontAsk bool)` | **Quit and Reopen Bench**, which the welcome guide and **Settings** offer once Bench has asked for Screen Recording (decision 48). It quits as `Quit` does, and the shell opens Bench again once it has quit (`Emitter.Relaunch`). Unless `sure`, it first asks, as quitting does, while a terminal is running something: `confirm-quit` has `reopen` set, and its button, **Quit and Reopen**, calls it again, sure, with `dontAsk` as `Quit` has it. The browser preview refuses. |
| `DismissWindowTool(bundleID string)` | The **Dismiss** of the notice about a window tool that's running, such as Rectangle: the notice goes, and `state.json` keeps the tool, so that its notice doesn't come back when the tool runs again (decision 69). |
| `DismissUpdate()` | The **Dismiss** of the update notice in a Bench without Sparkle: the notice of a newer version goes, and Bench doesn't announce that version again; `state.json` keeps that (decision 48). |
| `InstallUpdate()` | The update notice's **Install…**, in a `Bench.app` with Sparkle: Sparkle's window for the newer version, which offers to install it, opens (`Updater.Check`, decision 48). Refused without an updater, whose notice has **Download** instead. |
| `SetDemoScreen(screen string, fitWidth float64)` | Browser preview only: simulate `fit`, `monitor`, or `laptop`. Choosing a screen chooses it for Bench's window (decision 70). |
| `SetDemoMonitor(plugged bool)` | Browser preview only: the demo bar's **Simulate ▸ Unplug monitor** and **Plug in monitor**. Unplugging needs the monitor, as **Monitor** gives it: the laptop's screen is then the only one, and Bench's window goes to it. Plugging in puts the monitor back to the laptop's right, and once the screens settle, Bench goes back to it, with the toast and its Undo (decision 70). The View's `demo.unplugged` follows. Refused when there's no monitor to unplug or it's plugged in already. |
| `DemoModifiers(held []string)` | Browser preview only: the modifier keys held down in the page, by the names `shortcuts.Modifiers` gives them (`ctrl`, `alt`, `shift`, and `cmd`), which the preview's platform reports as `Platform.HeldModifiers`, so that the hold after the `last_tab` shortcut works there (decision 60). The page sends them before the shortcut's `LastTab`, and again as they change. |
| `DemoMinimise()` | Browser preview only: the demo bar's **Minimise window**. Minimise the App window that the focused pane shows, as Command-M does in the app, for the platform to report as macOS would (decision 55). Refused for a pane that shows no App window. |
| `DemoNotify()` | Browser preview only: the **Post notification** of the demo bar's **Simulate** menu. Have the web page that the focused pane shows, or else the first that the current tab shows, post a notification, as Slack's would for a new message, for the platform to report as WebKit would (`platform.Event` `WebNotification`), so that the first one turns **Keep Running in the Background** on (decision 76). Refused when the tab shows no web page. |
| `DemoComeForward()` | Browser preview only: the demo bar's **Bring Bench forward**. Do what Bench does as it becomes the active app on a Mac, which a page can't make it: hide the apps whose windows cover Bench, or park only those windows, and say so the first time for each app (decision 68). |
| `SetDemoWindowTool(on bool)` and `DemoToolMoves()` | Browser preview only: the **Rectangle** and **Keep moving window** of the demo bar's **Simulate** menu. `SetDemoWindowTool` starts or stops the demo's Rectangle, an app with no Dock icon, as `Platform.RunningApps` would list it on a Mac, so that the View's `windowTools` and the notice about it follow. `DemoToolMoves` moves the window that the focused pane shows to the left half of the screen, as a window tool's shortcut would, once more than Bench puts it back, a second apart, so that its status becomes `moving`; while the screens settle, it waits. Refused for a pane that shows no App window in its pane (decision 69). |
| `SetDemoVoiceOver(on bool)` | Browser preview only: the demo bar's **VoiceOver**. Pretend that VoiceOver is on or off, as `Platform.SystemKeys` would report it on a Mac: the View's `voiceOver` follows, so the terminals are read out, and the shortcuts with ⌃⌥ are checked again (decisions 45 and 64). It works with `BENCH_DEBUG` set too, as `platform.Logged` passes `SetSystemKeys` on to the demo platform. |
| `DemoDropFiles(paths []string, attrs map[string]string)` | Browser preview only: a drop of files from the Finder, which the preview's browser can't give the paths of, so its tests and screenshots call `window._wails.handlePlatformFileDrop` as Wails' native code does, and the page sends the paths and the attributes of the element under the point to this, as Wails' runtime sends them to the shell in the app. It does what the app does with them (`Engine.DropFiles`, decision 73). Refused outside the preview. |

### Events

| Event | Payload |
| --- | --- |
| `bench:state` | `workspace.View`. Each of its `items` has the `title` its sub-tab shows: the `label` the user gave a terminal or web page, if any, or else the title its window, page, or shell gives it, which `autoTitle` then has (decision 39). Its `density` (`standard` or `compact`) sizes the panes' chrome and its `titleBar` the title bar's, and each of `display.panes` has its `header` height, which Go docks the pane's window below (decision 33). Its `appearance`, `system`, `light`, or `dark`, chooses the frontend's colours (`theme.js`, decision 52): it's `settings.appearance`, or while Settings previews another, that one, as are its `density`, `titleBar`, and `display` (decision 66). Its `display.zoomed` names the pane that `ZoomPane` shows alone, which is then `display.root` and the only one of `display.panes`, with no dividers (decision 40). Its `display.arrange` says what the keys that arrange the panes would do now: `grow`, `shrink`, and `equal` are set when `GrowPane`, `ShrinkPane`, and `EqualPanes` would act, and `nextLayout` and `previousLayout` name the layouts that `StepLayout` would give the tab, for the jump palette and the menus (decision 59). Its `settings.launchers` are the `[[launcher]]` entries, each with the `mono` and `hint` the lists show (decision 41). An item's `attention` says why it needs the user, for the bell on its sub-tab, and a tab's `attention` counts its items that have one, for the bell on the tab (decision 43). A terminal's `here` is the folder its shell is in, for **New Terminal in *folder*** in its menu, and ⌘T in it (decisions 44 and 64). Its `voiceOver` is set while VoiceOver is on, as `Platform.SystemKeys` reports it: the terminals are then in screen reader mode, and the bells and finished commands are announced, a terminal's `failed`, set with its `ended`, saying whether its command ended with an error (decision 64). Its `notices` are the status bar's notices, each with its `text`, for some a `key` that stays the same while the text changes, which the recent messages keep one copy for (decision 69), and, for some, `actions`: buttons, each with a `label` and either the `method` it calls, a bound method that takes nothing, such as `ReloadConfig`, or with its `arg`, one string, as **Use this instead** calls `UseSuggestedShortcuts` with the fields to change, or else a `command` that the frontend runs as if Go had sent it as a `bench:command`, as **Keyboard settings** runs `keyboard-settings`. Its `shortcuts` are the fields of the `[shortcuts]` table, in config order, for **Settings ▸ Keyboard**: each with its `field` name, `label`, the `keys` Bench adds to a field of modifiers (such as "the arrow keys"), its `value` in config form and `display` as macOS shows it, its `default`, and, from what the shell found as it registered them, a `problem` if its shortcuts don't work, or may not, and a `suggestion` (with its `suggestionDisplay`) to use instead (decision 45). Its `lastTab` is the tab that `LastTab` goes back to, or -1, and each item's `used` says how lately it was used, for the jump palette's order (decision 49). Its `hold`, while a hold after the `last_tab` shortcut shows the tabs, has the hold's `id`, which counts holds, its `tabs` by index, most recently used first, and the `selected` one, or is `null` (decision 60). Its `front` is the window in front before Bench, if it's out of the tabs, as an item ID or a window not in a tab's `f` ID, for **Add Front Window** (decision 13). A Web item has its `zoom` in percent, left out at 100; `canGoBack` and `canGoForward`; `camera` and `microphone`, `active` or `muted` while its page uses them; and `finding` while its find bar is open, which Go leaves `layout.FindBarHeight` for at the top of its pane's body. A tab's `camera` and `microphone` count its pages that use them, and `cameraLive` and `microphoneLive` are set while any of them has that one live (decision 50). An item's `icon`, and the `icon` of each window not in a tab, remembered app, launcher, and add-dialog row, is the address of the icon its tile shows in place of its `mono`, if it has one (decision 51). An item's `unread` is its unread count, with `count`, `more` for a count with a "+", `dot` for a badge with no number, and `wholeApp` for an app's Dock badge that the app's other items share, and a tab's `unread` is the total of its items'; both are left out when there's nothing unread, and while `settings.unreadCounts` is off (decision 61). Its `keepOver` lists, by bundle ID, the apps that `KeepOverBench` can still keep over Bench, for the recent messages to offer **Keep over Bench** again (decision 68). Its `wentBack`, in the form of `undo`, is Bench's move back to the display that the user chose, while its toast's **Undo** can still take it back, for the recent messages to keep that **Undo** (decision 70). Its `windowTools` names the window tools that Bench knows and that are running, such as Rectangle, for the demo bar's **Rectangle** to show as pressed, and an item's status is `moving` while its app or a window tool kept moving its window out of its pane, with a `statusText` that names the app and those tools (decision 69). Its `nativeMenus` is set in the app, where `ShowMenu` shows Bench's menus as native macOS menus, and left out in the browser preview, which draws them in HTML (decision 71). Its `dropdown`, while a terminal is the drop-down, has the terminal's `id`, `shown` while it shows, and `inPage` in the browser preview, where the page draws it; the terminal is among the `floating` items, and its `attention` marks it while it's hidden. Its `settings.dropdownHeight` is the drop-down's height in percent of the screen's (decision 72). A tab's `colour` is one of `config.TabColours`, or "" for none, for its underline, the band along its pane headers, and the dot beside its name in the overview, the jump palette, and the **Move to Tab** and **Also Show In** menus (decision 74). A web page's `keepRunning` says that **Keep Running in the Background** is on, and its `notLoaded` that it has no web view and won't get one until the user shows it, for the sub-tab's tooltip and VoiceOver name (decision 76). |
| `bench:toast` | A short message, as a string, or, for one with a button, `workspace.Toast`: its `text`, and an `action` with the button's `label`, the bound `method` it calls, and the `arg` it passes, as **Undo** calls `Undo` with its change's number (decision 37), or instead the `item` it passes, as **Pop out** calls `PopOut` with the ID of the item that came into a pane (decision 13). A toast can have a `second` button of the same form after it, as **Go there** calls `SwitchTab` with the tab that a window moved to (decision 57). The frontend shows it over the status bar, which docked windows never cover, or, while the status bar is hidden, in the title bar's empty space (decision 31), and keeps it in the notices' **Recent Messages** (decision 47). |
| `bench:command` | `workspace.Command`, such as `{"name": "toggle-palette"}` when the jump shortcut is pressed, `{"name": "settings"}` from **Bench ▸ Settings…**, `{"name": "keyboard-settings"}` (Settings, at its **Keyboard** section), `{"name": "edit-tab"}` (the current tab's properties) from **Tab ▸ Tab Properties…**, or `{"name": "focus-terminal", "itemId": "w7"}` to give a terminal the keyboard. `{"name": "paste-terminal", "itemId": "w7", "text": "…"}` types `text` into terminal `itemId` as a paste, in the window whose page has its session, for files dropped on it, and `drop-refused`, from the shell, shows `text`, why Go refused a drop on terminal `itemId`'s own window, as that window's toast (decision 73). `confirm-quit` asks before quitting, and `confirm-close-terminal` asks, in the popped-out terminal `itemId`'s own window, before closing it; each has the `terminals` that are running something, each with its `id`, sub-tab `title`, `tab` (left out for one in a window of its own), and what it's `running` (decision 35); a `confirm-quit` from `QuitAndReopen` has `reopen` set (decision 48). **Tab ▸ Close Tab…** and **Pane ▸ Close Window** send `close-tab` and `close-window`, for the current tab and the focused pane's shown item, which the frontend closes as the tab's menu and the sub-tab's × do, asking first where Go says (decision 36). The app menu's other items that the frontend does send `new-tab` (**Tab ▸ New Tab…**), `new-tab-from-windows` (**Tab ▸ New Tab from Open Windows…**, the **New tab** dialog with **Windows that are open** chosen, decision 54), `rename-tab` (**Tab ▸ Rename…**, which renames the current tab in place), `open-palette` (**Pane ▸ Jump to…**), `add-to-pane` (**Pane ▸ Add to Pane…**, the add dialog for the focused pane), `keyboard-shortcuts` (**Help ▸ Keyboard Shortcuts**, which opens the **Keyboard Shortcuts** sheet, decision 48), `first-run` (**Help ▸ Show Welcome Guide**, which opens the welcome guide even if it was dismissed), `recent-messages` (**Help ▸ Recent Messages**, decision 47), `show-all-tabs` (**View ▸ Show All Tabs**, the overview of every tab, decision 60), and, with the focused pane's shown item as `itemId`, `find-in-page` and `copy-page-link` (**Pane ▸ Page ▸ Find in Page…** and **Copy Page Link**, which open the page's find bar and put its link on the clipboard, as its menu's entries do, decision 50). The `show_all_tabs` shortcut sends `toggle-all-tabs`, which closes the overview if it's open, and a hold after the `last_tab` shortcut sends `hold-tabs` once it shows the tabs, and `hold-up` and `hold-down` for the `focus_pane_modifiers` shortcuts with Up and Down while it shows them, which the frontend answers with `StepHold` and a row's length (decision 60). A tab's shortcut, `next_tab`'s, `previous_tab`'s, and `last_tab`'s, or for `last_tab` a hold as its keys are let go of, send `switching`, with the `tab` they switch to (left out for the first tab, 0), before Go arranges it, for the title bar to show that tab as current at once, and `switch-refused` if Go then refuses the switch, or it ends on another tab, to put it back as the View has it (decision 67). For the commands that need Bench's window, such as the palette, Settings, and `confirm-quit`, the shell brings Bench forward first; for `focus-terminal`, the engine already has (decision 15). |
| `term:data` | `workspace.TermData`: an item ID, base64-encoded output of up to 4 KB, and the offset where it starts. |

### Terminal output

Each terminal's output is numbered by byte, from the start of its session:
a `term:data` event's `offset` is where its output starts, and a snapshot's
is where it ends. A new terminal view, such as a popped-out terminal's
window, replays the snapshot, then draws only the output past its end, so
output that reaches it both ways is drawn once. A session that replaces one
that ended carries on its numbering. While it replays a snapshot, a view
sends nothing that xterm.js answers to queries in it: those programs have
long since stopped waiting. Once a session's 256 KB of kept output has
dropped its start, the snapshot begins with the sequences that set the
terminal modes the dropped output set, such as the alternate screen,
application cursor keys, mouse reporting, and bracketed paste.

Only one view draws a terminal: its pane in the main window, or its own
window once it's popped out, when the main window lets its view go. That
view answers the program's queries, and says how far it has drawn with
`TermAck` after every 64 KB. A session reads no more of its program's output
while the views are 256 KB or more behind, so a program that prints faster
than Bench can draw waits for it, as it would for a slow terminal. If no
view says anything for a second, as for a terminal that no view shows, the
session carries on anyway, and doesn't wait again until a view next says
how far it has drawn. Each batch of output goes out in events of up to
4 KB, under the size above which Wails sends an event's payload separately,
one at a time, and drops it if it's waited 30 seconds.

A render that redraws the panes moves each terminal's element out and back,
and a focused terminal loses the focus and gets it back. The frontend holds
back the focus reports xterm.js makes meanwhile, so a program that asked for
them (DECSET 1004), such as vim, hears only of real changes. Links that
programs print with OSC 8 open through `OpenLink`, since Bench's web view
opens no windows of its own. xterm.js activates a link on any click that
starts and ends on it, with or without a modifier key, so the frontend's
link handler opens it only when Command is held, and its `hover` and
`leave` show and hide the link's address in a tooltip inside the
terminal's element. The tooltip shows the address as the URL parser
writes it, in ASCII, with the host in bold and every part cut short, so
neither look-alike letters nor a long user name or path can hide the host.
Plain `http` and `https` addresses in a terminal's text are links too,
through xterm.js's web-links add-on, which the frontend gives the same
handler, so they open only with Command held, and show the same tooltip
(decision 42).

A session that `state.json` brings back starts with the output that Bench
kept when it last quit (`Options.Preface`, decision 44): it's the start of
the session's output, numbered like the rest, so a snapshot and
`term:data` carry it as they carry the program's own. It's made safe with
`term.Restorable`, which keeps only text and the sequences that colour it,
move the cursor, and erase, so no query in it reaches a view that would
answer it into the new shell, and then grey with `term.Greyed`: text in the
terminal's own colour is bright black (SGR 90), the colours the program
chose stay, and faint (SGR 2) goes.

Terminals keep text at a contrast of 4.5:1 at least against its
background, or 7:1 with macOS's Increase contrast on (xterm.js's
`minimumContrastRatio`, which `terminals.js` changes as
`prefers-contrast: more` does). The terminals take their colours again
then too, as the Increase contrast layer changes `--text-muted` and
`--text-faint`, their white and bright black. xterm.js asks only half that
contrast of faint text, which it draws at half strength: on a light
background, faint text in the terminal's own colour would be 3.2:1.
`styles.css` draws that text in `--term-dim` instead, which each set of
colours, and its Increase contrast layer, makes readable. Faint text in a
colour that the program chose keeps xterm.js's half strength, and can be
harder to read: xterm.js checks that colour at full strength against half
the contrast. A rule for each of its 256 colours, in each set of colours
and layer, would be a lot to keep for a little-used look, so the user
guide says that such text can be harder to read instead. Bench's own grey lines, the one after restored
output and the one after a command that has finished, are bright black
rather than faint for the same reason (decision 52).

## Files

All in `~/Library/Application Support/Bench/`, or `$BENCH_HOME`. Bench runs
the commands in `config.toml` and `state.json`, and acts on `parked.json` and
`vscode.json`, so
`config.CheckDir` refuses a `$BENCH_HOME` or `--dir` folder that another user
owns or can change, or that sits in a folder they can change, and creates a
missing one readable only by the user. The files and folders Bench creates
are private too (0600 and 0700); an existing file keeps its permissions.

| File | What it is |
| --- | --- |
| `config.toml` | Tabs, layouts, settings, shortcuts, launchers, and rules. Created on first run, with one tab holding the default browser (decision 28); **Settings** and **Tab properties** update it, and so do **Save as launcher** (`SaveLauncher`, decision 41), and the **Don't ask again** of the questions before quitting and before closing a terminal's window (`confirm_quit_busy_terminals`, decision 35), and before closing a tab (`confirm_close`), and **Close Tab…**, which deletes the tab as **Delete tab** does (decision 36), and **Undo** and **Reopen Closed Tab**, which put a deleted tab's entry back (decision 37), and moving a tab, which moves its entry (`MoveTab`, decision 38), and **Move to Tab ▸ New Tab** and **Also Show In ▸ New Tab**, which add a tab entry (`MoveItemToNewTab`, decision 39), and a tab made of open windows (`NewTabFromWindows`, decision 54), and a shortcut notice's **Use this instead**, which changes `[shortcuts]` (`UseSuggestedShortcuts`, decision 45), and `UseWebApp` (or `UseWebAppWhenReady`) for a web page that it lists (decision 22), and the welcome guide's starting set-up, which adds its tabs (`UseStarterTabs`, decision 48). Bench records a hash of it (`config.Sum`) whenever it reads or writes it, and doesn't save over it once it has changed outside Bench (decision 7). |
| `config.toml.bak` | The previous `config.toml`, kept by every save. **Use backup** swaps it with a `config.toml` that has problems. |
| `config.toml.good` | The last `config.toml` that loaded without problems, or that Bench saved: what Bench runs with while `config.toml` has problems (decision 7). |
| `state.json` | The live arrangement, saved half a second after it changes, and within 3 seconds however often it changes. New titles and addresses, the display Bench is on, and dialog sizes alone are saved within 30 seconds. Its tabs are matched with `config.toml`'s by name at startup (decision 29). Each tab records its config entries' keys (an App entry's app and title, a Web entry's address, a VS Code entry's folder, a terminal's folder and command, and the command it types, if any), so that at startup the entries edited, removed, or added in `config.toml` since can be applied to the arrangement. Its `apps` holds, for each app with items, by lower-case bundle ID, whether the user quit it (`quit`, decision 28) and when Bench last saw it running (`seen`, decision 10), and `ran` when Bench last ran. Its `screen` is the display Bench's window was on, by ID, name, and frame, for Bench to open on next time, with `window`, the window's content frame there, if it didn't fill the display, for Bench to open at (decision 30); its `preferredScreen` is the display that the user last chose for the window, in the same way, which Bench goes back to when it comes back, and opens on when it's connected (decision 70); and its `dialogs` the sizes the user gave dialogs, by kind (`SetDialogSize`, decision 34). An item's `label` is the name the user gave a terminal or web page (`RenameItem`, decision 39). A terminal's `run` is the command Bench types into its shell, and `startAgain` whether it runs it again at the next start (decision 41), and a plain shell's `lastFolder` the folder it was last in, where it starts again; its `folder` stays the config entry's key (decision 44). A Web item's `zoom` is its page's zoom, such as 1.25, left out at actual size; one that isn't among Bench's zooms is taken as the nearest (decision 50). Its `closed` holds the last 5 tabs deleted or closed, latest first, each with its arrangement and items, its `config.toml` entry and rules, where it was among the tabs, and when it went, for **Reopen Closed Tab** (decision 37); one that couldn't come back is dropped at load, not the whole file. Its `updates` has, while `check_for_updates` is or was on, when Bench last asked GitHub for its latest release (`checked`), the newer version it found and its page (`latest` and `url`), and the version whose notice the user dismissed (`dismissed`, decision 48). Its `covering` lists the apps, by lower-case bundle ID, that Bench has said it hid, or moved windows of out of sight, because they covered it, so that it says so once for each (decision 68). `windowTools` lists the window tools, by lower-case bundle ID, whose notice the user dismissed, so that it doesn't come back (decision 69). Its `dropdown` is the item ID of the terminal that is the drop-down, which is among the items out of the tabs (decision 72). A Web item's `keepRunning` is its page's `keep_running`, left out until Bench or the user sets it, which keeps the setting of a page that `config.toml` doesn't list, such as one added in Bench (decision 76). Older builds ignore them. |
| `state.json.bad` | A `state.json` that Bench set aside (decision 29): one it couldn't use, such as one that doesn't parse, or a copy of one with tabs that `config.toml` no longer has, whose arrangement it dropped at startup or at a reload. The two before are `state.json.bad.1` and `state.json.bad.2`; older ones go. |
| `terms/` | Each terminal's output from when Bench last quit, at most 256 KB, as `<item ID>.out` (0600, in a 0700 folder), while `keep_terminal_output` is on. Written as Bench closes, before it ends the sessions, only when it saves `state.json`; removed as the terminal closes, or leaves `config.toml`, and all of it when the setting goes off (decision 44). |
| `parked.json` | Windows that are parked right now, with where they were, and apps that Bench hid, so they can be put back after a crash. Set aside as `parked.json.bad` if it doesn't parse. |
| `vscode.json` | The running `code serve-web`, so that it can be stopped after a crash. |
| `vscode-token-*` | The running `code serve-web`'s connection token, readable only by the user. It's here, not in `$TMPDIR`, which macOS clears of old files, because the server rereads it whenever it starts VS Code. |
| `bench.lock` | The support folder's lock, which every process that acts on `parked.json` and `vscode.json` holds while it does (see [Crash safety](#crash-safety)). |
| `first-run-done` | Present once the welcome guide has been dismissed, with its last page's **Done**. |
| `accessibility-version` | The version of Bench that last had Accessibility permission, for the notice that says an update needs the user to allow Bench again (decision 48). |
| `view.json` | Only with `BENCH_DEBUG` set: the latest `View`, as the frontend gets it, for scripts such as the macOS demo. |

Bench saves `state.json` 500 ms after the last change. A save from Tab
properties, which can rename, add, or delete tabs, saves it straight after
`config.toml` instead, because at the next start, a tab that `state.json`
names differently from `config.toml` keeps its arrangement only if it's the
only change to the tabs (decision 29). A web page
switched to a web app saves them in the same way, because `state.json`'s
entry keys must name the entries in the `config.toml` on disk: keys from
the new file with the old one would make the web app's entry look removed,
and the page's look added. Quitting waits for a save from Settings or Tab
properties in progress, so it can't come between the two. If `config.toml`
can't be written, Bench saves no `state.json` until a later save from
Settings or Tab properties writes `config.toml`, so the two files on disk
still match, and says so in the status bar. The same goes for a change to
the tabs that Bench didn't save because `config.toml` had changed outside
it, until the user reloads the file or overwrites it.

Bench's log is apart from these, where macOS keeps apps' logs:
`~/Library/Logs/Bench/bench.log`, readable only by the user, in a folder
only they can open (0600 and 0700, even if they were made otherwise). It has
a line as Bench starts, with its version, then Bench's warnings and errors,
each notice as it first appears, and, with `BENCH_DEBUG` set, the debug
messages too. Once it would pass 1 MiB, it's moved aside as `bench.log.1`,
in place of the one before, and starts again (decision 47). A second Bench,
which only brings the first forward, adds no start line and never moves the
file aside. The browser preview keeps no log file.

## Crash safety

Bench moves other apps' windows off screen. If it quit without putting them
back, they'd be stranded, so:

1. `windows.Manager` records every window it parks (or minimises, when no
   place to park it keeps clear of other displays), and every app it
   hides, in `parked.json` before it does it, and removes each as it puts
   it back. A failed park or hide keeps its record, unless the window has
   closed or is on another desktop, since an app that answered too slowly
   may still do as asked. If the file goes missing while the manager still
   records something, the next arrangement writes it again. Each write
   goes through a temporary file, flushed to disk before it replaces
   `parked.json`, so a power cut can't leave the file empty.
2. On a clean quit (the app menu, a signal, or Wails' shutdown hook), Bench
   calls `RestoreAll`. It removes `parked.json` once everything is back; a
   window it can't put back yet, such as one on another desktop or in
   native full screen, stays in the file for the watchdog or the next
   start.
3. At startup, Bench first restores anything left in `parked.json` by an
   earlier run (`RestoreCrashed`), and keeps in the file what it still
   can't put back. A window whose app it has just shown gets up to 1.5
   seconds to appear in the app's window list.
4. The app starts `bench watchdog` as a child process, in a process group
   of its own and connected by a pipe. If the pipe closes while
   `parked.json` still lists windows (Bench crashed, even in native code),
   the watchdog restores them. Child processes of an app bundle share its
   Accessibility permission.
5. `code serve-web` runs in a process group of its own, recorded in
   `vscode.json`. The watchdog, or failing that the next start, stops a
   server that a run which crashed left running, if its first process is
   still the one recorded, and removes its token.
6. Each process that acts on a support folder's `parked.json` and
   `vscode.json` holds an exclusive `flock(2)` lock on `bench.lock` there
   while it does, because each rewrites the whole file from its own
   records: the app and `bench arrange` for as long as they run (the app
   takes it after Wails' single-instance check, and lets it go once the
   engine has closed), `bench restore` and the watchdog while they put
   windows back. The watchdog takes it only once its pipe has closed, and
   waits for it, so it never acts while Bench, or a Bench that has started
   again, is running; that Bench puts back what the last run left itself.
   The app waits for a watchdog or `bench restore` that holds it, and
   `bench restore` refuses while anything holds it. `bench arrange` also
   holds Wails' single-instance lock (`<UniqueID>.lock`), so the app can't
   start while it runs, and keeps a temporary support folder, printing a
   `bench restore --dir` hint, while its `parked.json` still lists
   anything or its watchdog hasn't finished.

## Decisions

Choices made while building that the spec left open or that differ from it.
Decisions 11 to 14 and 19 come from running Bench on a real Mac, in the
[macOS demo](development.md#the-macos-demo):

1. **Global shortcuts use Wails' `GlobalShortcut` manager**, not
   `golang.design/x/hotkey`. Wails v3 (beta.27) registers them with Carbon's
   `RegisterEventHotKey` on the main thread, which is the spec's own fallback,
   and reports registration failures, which Bench shows in the status bar.
   Carbon's hot keys don't need Accessibility permission, which the spec
   assumed they would, so Bench registers them whether or not it has it;
   only arranging windows waits for the permission. The manager registers
   and releases them while the app runs too, so when **Reload config.toml**,
   **Settings ▸ Keyboard**, or a notice's **Use this instead** changes
   `[shortcuts]`, the engine tells the shell (`Emitter.ShortcutsChanged`),
   which releases the old ones and registers the new ones, without a
   restart (decision 45).
2. **Wails v3 is pinned to v3.0.0-beta.27.** Its server mode (the `server`
   build tag) gives a browser preview of the real app on any OS, which the
   end-to-end tests use.
3. **The menu bar item and Open at login use Wails' `SystemTray` and
   `Autostart` managers** (`SMAppService` on macOS). The menu bar item shows
   a template icon, which macOS tints to suit the menu bar:
   `internal/ui/menubar.png`, which `build/darwin/menubar/main.go` draws, and
   `make icon` draws again. Wails sizes it to the menu bar's thickness, 22
   points, so it's 44 by 44 pixels, two to a point. Beside it is the current
   tab's name, cut to 16 characters with an ellipsis, so that a long name
   doesn't push other items behind the notch; or nothing, or no item at all,
   as `menu_bar` says. The engine tells the shell what to show
   (`Emitter.MenuBar`): the setting, the current tab, the tabs with their
   shortcuts, the jump shortcut, the layout mode, which ticks
   **View ▸ Layout** (decision 31), the other shortcuts, the tab closed
   last, and the focused pane, which the app menu's **Tab** and **Pane**
   menus follow (decision 46), whenever any of them changes, as when
   a tab is renamed, **Settings** is saved, or `config.toml` is reloaded. The
   shell adds the item only once the app runs and the engine has said what it
   shows, so that an item that's to be hidden never appears; hidden later,
   it's kept, and Wails' `Hide` sets the status item's `visible` to NO.
   macOS keeps that from one launch to the next, under the item's autosave
   name, which Wails leaves the same each time, so the shell shows each new
   item (`Show`) as it adds it: otherwise an item hidden before Bench last
   quit would start hidden. The menu lists the tabs, each with its global
   shortcut as its key equivalent, so that the menu shows it, then
   **Jump to…**, with its shortcut, and **Settings…**, which send the
   `open-palette` and `settings` commands, then **Show Bench** and
   **Quit Bench**. With the item hidden, the jump
   shortcut, the tab shortcuts (decision 16), and the Dock icon bring Bench
   back: clicking Bench in the Dock while its window is closed, minimised, or
   out of sight brings the window forward (`reopen`), since Wails shows a
   closed window itself only when none of Bench's windows, such as a
   popped-out page, is open.
4. **Raised mode** (see [The window model](#the-window-model)) answers the
   milestone 4 question of how menus, dialogs, and drags appear above docked
   windows. It needs checking on a Mac. For menus, decision 71 replaces it
   in the app: Bench's own menus are native macOS menus, which need no
   raised mode. Dialogs, drags, live previews, and the menus drawn in HTML
   keep it.
5. **The frontend has no build step.** It's plain ES modules, with xterm.js,
   its fit, search, and web-links add-ons, and the IBM Plex fonts vendored
   in `frontend/vendor`, so the app works offline.
6. **Splits have IDs** (`s1`, `s2`, …) as well as panes (`p1`) and items
   (`w1`), so `SetSizes` can name a split.
7. **Config problems don't stop Bench, and Bench never writes over the
   user's edits.** If `config.toml` has problems, or can't be read, Bench
   reports them in the status bar and runs with the last config that loaded
   without problems: `config.toml.good`, which it keeps each time
   `config.toml` loads cleanly or Bench saves it. Only if there's none does
   it run with the default config. That goes for an unknown field or a
   single bad entry too. Running with the rest of the file was considered,
   as the UX audit suggested, and not chosen. Saving it isn't the risk, as
   Bench doesn't save `config.toml` while it has problems. The risk is that
   what's left out looks removed: a tab or entry skipped for a typo would
   lose its arrangement, or its item, closing a page or terminal, just as
   if the user had deleted it (`syncEntries`, decision 29). A whole config
   that worked keeps everything until the file is fixed. Until then, Bench doesn't
   save `config.toml`. It saves `state.json` only while the model follows a
   config that loaded cleanly (`followsGood`): the last good config, if
   `state.json` had its tabs at startup, or the config Bench had when a
   reload found problems; not after a change to the tabs (`stateHeld`), and
   never for the default config, whose tabs aren't the user's. Otherwise
   it leaves `state.json` alone, so that the arrangement made for the
   user's own config isn't overwritten: it restores it
   (`config.PeekState`) only if it has the tabs of the config it runs with,
   by name, since that config may be the default one, whose tab only
   happens to share a name with one of the user's, and leaves a file it
   can't use where it is. Once the file is fixed, by a reload, **Use
   backup**, or a reload that writes a missing file, Bench sets `state.json`
   aside before it saves over it, whole if it couldn't use it
   (`config.SetAsideWhole`), or as a copy, and the arrangement notice says
   so, and offers to restore the tabs (decision 29). The problems notice
   says whether Bench saves the arrangement meanwhile. **Settings** and
   **Tab properties** apply their changes and say they weren't saved. The
   problems notice has buttons: **Open config.toml**, **Show in Finder**,
   **Reload**, and, if `config.toml.bak` loads without problems, **Use
   backup**.

   Bench also records a hash of `config.toml` whenever it reads or writes
   it. Before each save it checks that the file still has it (`SaveIf`, under
   the same lock as the write); if the user has changed the file since, as
   they may while Bench runs at login, Bench doesn't write it, keeps its own
   change applied, and a notice offers **Reload** (the file wins) and
   **Overwrite** (Bench's config wins, and the file's version goes to
   `config.toml.bak`). It checks the hash too when Bench becomes active,
   when an app with a window in a tab does (the user may switch straight to
   it after editing the file), and when a menu or dialog opens
   (`SetOverlay`, for an edit made in one of Bench's own terminals), and
   offers **Reload**. **Reload config.toml** (`ReloadConfig`) applies the
   file the way startup applies it to `state.json` (`syncEntries`), keeping
   the arrangement: a tab is the same tab if it has the same name, ignoring
   case, or if it's the only tab renamed, with nothing else about the tabs
   changed (`config.MatchTabs`), and takes its entries' edits, and the file's layout if that
   changed; tabs new to the file are built from it, tabs no longer in it are
   deleted as **Delete tab** does, and the tabs take the file's order. The
   arrangement of a deleted tab is kept for the notice and **Restore
   previous arrangement**, with a copy of `state.json` set aside, as at
   startup (decision 29), unless it's the default config's, which Bench ran
   with while the file had problems. The windows of items whose entry or tab
   has gone go where the file's entries claim them, as the startup scan
   would place them (`claimUnmanagedLocked`), or else to "Not in a tab", so
   that a moved entry or a renamed tab doesn't ask its app for new windows.
   A page or terminal whose entry or tab has gone carries on as the item of
   a new entry for the same address (and profile), or folder and command, so
   that its page stays loaded and its session goes on; otherwise one whose
   entry has gone closes, and one whose tab has gone moves to "Not in a
   tab". A page whose address or profile changed loads again, and a
   terminal whose command changed runs the new one when it next starts.
   Windows that no tab had go where new entries for their app claim them,
   as at startup. Settings and rules apply from then
   on, and changed shortcuts are registered again (decision 1). While Bench
   ran with the last good config, tabs that the reloaded file adds start
   from the file, not from `state.json`.
8. **The browser preview never runs real shells.** Its terminals are
   `term.Echo`, because the preview's server has no authentication.
9. **The add dialog's rows come from Go** (`AddOptions`), so the frontend
   doesn't need the installed-apps list.
10. **New windows that Bench is expecting come first.** Before the spec's
    order for new windows (a rule that floats them, a rule's `tab`, then
    `new_windows`), a new window goes where Bench is waiting for one
    (`adoptLocked`), in this order: the next window that the add dialog's
    **New window**, or **Open** on a running app, asked for; a remembered
    item with the window's exact title (`Model.MatchByTitle`); an item that
    a tab switch or **Open** opened the app for, preferring one whose
    `title` the window's title contains, then the one that had a window
    first; a remembered item by creation order (`Model.MatchByCreation`),
    but only at the scan of the windows already open (at startup, or when
    Accessibility permission arrives) or while the app is opening again
    (see below); or the pane of an add dialog, or the item of an **Open**,
    that launched the app. A remembered item is an App item without a
    window that had one, after its app quit or Bench restarted; the
    config's placeholders, which never had a window, don't count. Only a
    rule that floats the window comes before these. The place that Bench
    opened an app for comes before the app's other places by creation
    order, because those may be in other tabs, out of sight. A window that
    a tab switch's item with a `title` doesn't take yet, because the app
    titles it only once its page has loaded, is placed as usual, but joins
    that item if a new title matches while the switch still waits. The
    spec leaves open where an App item's `title` ranks: with
    `new_windows = "app-pane"`, it counts at the step for the pane the
    config gives the app, after the panes holding the app's most recently
    used window, so a rule's `tab` and a recently used pane both come
    before it. Windows already open when Bench starts go where the config's
    items claim them, title filters first.
    - **Opening again.** Creation order is a guess, and a remembered item
      is out of sight (see [The window model](#the-window-model)), so it
      takes a window by creation order only while its app is opening again
      (`reopen.go`): for at least twice `launch_timeout` after the app
      launches, until four times after the scan (`scanned`), whenever the
      app launched, and for twice after each of its new windows, taking
      `launch_timeout` to be at least 15 seconds (`reopenSpan`), so that a
      short one doesn't cut the login minute short. A period starts only for an app with remembered items, and
      only with Accessibility permission. Afterwards, a new window of the
      app, such as one from Command+N, goes by rules and `new_windows`
      rather than to a place the user can't see.
    - **Forgetting.** When the period ends, `forgetPlacesLocked` forgets
      the app's remembered items that no window filled, in every tab, with
      `Model.DropFrom`, which, unlike `Take`, leaves the pane in place even
      if it's left empty, and doesn't mark the tab custom: only the user's
      own actions change a layout, and the config's claims find a tab's
      panes by their place (`paneForConfig`). It keeps one item without a
      window for each App entry in each config pane (`anchorsLocked`), so
      that a tab switch still opens the app: one in the pane the entry
      maps to, unless a window there stands for the entry, or else, if the
      tab has no window for the entry, the tab's first. It also keeps the
      items that Bench is filling right now, and the config's placeholders.
      It forgets nothing if the app isn't running, and waits another
      `launch_timeout` while Bench is launching the app. If the user has
      hidden the app, or Bench knows none of its windows, such as ones on
      another desktop, which the Accessibility API leaves out, it forgets
      nothing, and the next window Bench sees starts a new period.
      `forgetStaleLocked` forgets the places of an app that hasn't run for
      30 days in the same way, by `state.json`'s `apps` (`seen`); an app
      with no record counts as seen now. Only the time that Bench is
      running counts: `state.json` also says when Bench last ran (`ran`,
      saved at least every hour), and `loadAppStates` moves each `seen`
      on by the time Bench was off. The first check comes four times
      `launch_timeout` after startup, so that the apps macOS reopens at
      login open first, and then every hour.
11. **Bench's web views accept the first click.** Bench sits below the
    docked windows, whose apps are usually the active ones. When Bench
    wasn't active, a click on its tabs, sub-tabs, or web pages only
    activated it, and had to be made again. So `internal/mac` makes
    `acceptsFirstMouse:` answer yes for every `WKWebView` in Bench's
    process: the UI's and the Web items'.
12. **Bench's title bar passes clicks through.** The tab strip and the
    Auto, Wide, and Compact control sit where macOS keeps Bench's
    invisible title bar, which took the first click there to activate the
    window. So Bench's title bar views answer hit tests only over the
    traffic lights, and every other click goes to the UI's web view.
    Moving the window and double-click zoom come from the UI, through
    Wails.
13. **Only the user's drag undocks a window.** The spec undocks a window
    the user drags out of its pane. But apps move and resize their own
    windows too, often just after Bench sizes them (VS Code did on a real
    Mac), and macOS reports both alike. So window events carry
    `MouseDown`, which `internal/mac` sets only while the left mouse button
    is held with the pointer on that window or its edge, for a press that
    didn't begin in Bench's window. The engine undocks a window only on a
    move with `MouseDown`, and not within a second of the user dragging one
    of Bench's dividers. A drag of the window's edges that face other panes
    resizes the panes instead (decision 62). It puts back any other move
    or resize of a docked window 300 ms later, but at most three times in
    ten seconds for one window and one pane body: after that, it leaves the
    window where its app put it, so that Bench and the app don't fight, and
    its pane says so, with **Dock again** (decision 69). A try to put the
    window back into another body, as after a resize of the panes, starts
    the count again, so that an app that nudges its window after each of
    Bench's resizes isn't taken as fighting Bench. While the screens
    settle, as after the Mac wakes, it's
    macOS moving the windows: the settle puts them back, and its moves
    don't count (decision 56).

    The same goes the other way: a window comes into a pane by the user's
    drag only. A window out of the tabs (popped out with Bench, opened
    elsewhere and not in a tab, or dragged out of its pane) docks when the
    user drags it by its title bar onto a pane of the current tab and holds
    it there. `Platform.TrackDrags` is on while `drag_to_add` is, and the
    engine isn't headless. `internal/mac` follows other apps' windows with
    a global monitor of the left mouse button, which needs no permission:
    at the press, it looks up the window under the pointer in the window
    server's list, on a queue of its own, so an app that doesn't answer
    can't hold it up, and then reads that window's frame as the mouse
    drags, and ten times a second while the button is held, for windows
    the window server moves without drag events and for a pointer that
    rests. It reads the frame at most 30 times a second, however fast the
    mouse reports, as a press that selects text drags too. Once the window
    has moved, it reports `WindowDragged`, with the frame at the press
    (`Start`), the frame now, and the pointer, and `WindowDropped` at the
    release. Neither says whether the window is in full screen. It follows
    no drag while Bench's window isn't on the active desktop (Space): a
    window dropped then would go into a pane the user can't see. Accessibility
    notifications are hints only: a drag counts only while the window's
    size is the same as at the press, so an edge resize is never a drop.
    Bench's own popped-out windows send nothing to a global monitor, and
    AppKit doesn't give Bench their mouse-up while the window server moves
    them, so `NSWindowWillMoveNotification` with the button held starts a
    timer that reports their drag until the button is up; the event names
    the item, and the window docks once the drag has ended.

    The engine arms a pane only once the pointer has rested on it, within
    8 points, for half a second, so that a window dragged across Bench on
    its way elsewhere does nothing; then it shows the drop guide, a
    click-through panel over the pane at the pop-up menu level, with the
    targets that `layout.DropAt` finds, as for a dragged sub-tab, and the
    target follows the pointer while it stays on the pane. The guide goes
    when the pointer leaves the pane, after 2 seconds with no drag event,
    when Bench is hidden, when an app other than the dragged window's
    becomes active (pressing a window's title bar activates its app, which
    Bench can hear of after the drag), and when Bench is raised. A timer
    that fires as the pointer moves on does nothing: each checks that it's
    still the drag's. A drop with the guide shown docks the window: the
    middle joins the pane, and an edge splits it if it has a window other
    than the dragged one, which the guide shows too. A window that a
    `float = true` rule matches, a non-standard window, and one in full
    screen or minimised, which the engine asks the platform about as it
    arms a pane and at the drop, are never armed. A window dragged out of its pane
    loses `Undocked` and moves to the target pane in this tab, keeping its
    places in other tabs. **Add Front Window** and **Back to Bench** dock
    in the same way, in the focused pane: the View's `front`
    is the window the user last gave the keyboard outside Bench's main
    window, from `WindowFocused` (not one that Bench focused itself) or a
    popped-out window's `PopOutFocused`, and **Window ▸ Back to Bench**
    acts on the popped-out window that has the keyboard. The toast has
    **Pop out**, a `ToastAction` with the item's ID, or **Undo** for a
    window dragged out of another pane.
14. **Hiding and showing apps falls back to `AXHidden`.** On a real Mac
    with macOS 15, `NSRunningApplication`'s `hide` and `unhide` returned NO
    every time, although the apps hid and showed as asked. So when they
    return NO, Bench also sets the app's `AXHidden` attribute, which
    reports real errors, and reports a failure only if that fails too and
    the app isn't already as asked.
15. **Keyboard focus follows the focused pane.** Focusing a pane, by any
    means, focuses its shown App window as before. For a Web item, the
    engine calls `Platform.FocusWebView`, which activates Bench and gives
    the item's web view the keyboard; while Bench is raised, it does so
    when Bench is lowered, unless another app has become active meanwhile.
    For a terminal, it calls `Platform.FocusUI`, which activates Bench and
    takes the keyboard back from any web view for Bench's UI, then sends
    the `focus-terminal` command with the terminal's `itemId`, and the
    frontend focuses that terminal, waiting up to 2 seconds for a view
    that shows it. If a menu or dialog is open, as when Tab properties
    deletes the current tab, the wait starts when it closes, and the
    terminal takes the focus from the control that opened it. Closing one
    gives the focus back to a terminal that opened it only if that
    terminal is still the focused pane's shown item, so that typing never
    reaches a terminal outside the focused pane. Dragging a divider gives
    the focus back to what had it on release, on the same terms, unless
    something else has taken it meanwhile. Every native focus call is made
    under the engine's lock, so they reach macOS in the order the focus
    changed; a `focus-terminal` command that a newer focus has replaced
    before it's sent is dropped. A click inside a web view, which the UI
    doesn't see, sends `WebFocused`, which focuses its pane. When a tab
    switch can't focus the pane yet, because its window is still opening,
    the engine focuses it once the window arrives, but only while the pane
    still shows the same item, no menu or dialog is open, nothing else has
    been focused, and no other app has been activated, other than by
    Bench. A native menu that opens meanwhile drops it for good (decision
    71). While Bench's window is hidden, focusing waits until
    `BenchShown`. Arranging raises the focused pane's windows last, since
    raising a window of the frontmost app can make it the app's focused
    window. When the jump shortcut brings Bench forward for the palette
    while the focused pane's App window has the keyboard, the engine notes
    the window. Once Bench is lowered, it gives the window the keyboard
    back, after 150 ms, but only if nothing has been focused meanwhile, the
    pane still shows it, and no other app is active. The wait lets a palette row
    that focuses something, such as a tab, come first: the palette lowers
    Bench as it closes, before it runs the row, which reaches Go as a call
    of its own. A dialog that the palette opens, such as Settings, raises
    Bench again, and the note waits until that closes; so does a native
    menu that the palette opens, as **Window Options…** does, which the
    window's app, made active, would close (decision 71). Web items and
    terminals need none of this: Bench's window gives a web view the
    keyboard back when it's lowered, and the frontend a terminal.
16. **Bench hides the current tab's apps with its own window.** When
    Bench's window is hidden (**Bench ▸ Hide Bench**), minimised, or closed,
    which only hides it, `internal/mac` sends `BenchHidden`, and
    `BenchShown` when it's back. On `BenchHidden`, the engine arranges once
    as if no tab were shown, whatever `hide_other_tabs` says: it hides the
    current tab's apps, or parks their windows if the app also has a window
    popped out of Bench, and hides the web views. Until `BenchShown`,
    arranging leaves windows alone, so that an app the user brings forward
    meanwhile stays, and the keyboard focus that a shortcut gives a pane
    waits too. Bench being hidden by another app's **Hide Others**
    doesn't count, because the user wants that app's windows, which may be
    docked in Bench, to stay. Bench tells the two apart by whether it was
    active: hidden while another app is active, it counts only if no other
    app hides within a second of it, since **Hide Others** hides them all
    and **Hide** in Bench's Dock menu hides only Bench. (If every other app
    was hidden already, **Hide Others** counts.)
    - **A tab shortcut brings Bench back.** While Bench's window is out of
      sight, for any of these reasons or because **Hide Others** hid Bench
      (`Platform.OwnAppHidden`), a tab shortcut (a tab's, or the next or previous tab's) switches tabs,
      then asks the shell to bring Bench forward (`Emitter.BringForward`),
      as choosing a tab from the menu bar item does, so that a refusal,
      such as "There's no tab 9.", shows too. Bench coming back sends
      `BenchShown`, which arranges the tab and gives its focused pane the
      keyboard. The pane, window, rearrange, and zoom shortcuts still
      wait. With `show_on_tab_shortcut` off, a tab shortcut waits too: it
      switches tabs, but Bench stays hidden until the user brings it back.
17. **The browser preview checks where calls come from.** Its server has
    no authentication (decision 8), and Wails runs a bound method for any
    request to `/wails/runtime`, even a GET. So the preview serves only on
    `localhost`, ignoring `WAILS_SERVER_HOST` and `WAILS_SERVER_PORT`, and
    its middleware (`previewGuard` in `internal/ui`) refuses requests under
    `/wails/` unless the `Host` is `localhost` or `127.0.0.1` with the
    preview's port, any `Origin` and `Referer` are on such a host, any
    `Sec-Fetch-Site` is `same-origin` or `none`, and a call is a POST of
    `application/json`. That stops other web sites, and pages that rebind
    their host name to `127.0.0.1`, from calling Go. The middleware can't
    reach Wails' event WebSocket, so such a page can still read the
    preview's events: the demo view and what its pretend terminals echo.
18. **A recorded demo runs the macOS layer in CI.** The
    [macOS demo](development.md#the-macos-demo) workflow runs Bench on a
    GitHub macOS runner with real apps, presses its shortcuts, and drags
    with the real mouse, recording the screen. It finds Bench's panes in
    `view.json`, and its log shows every platform call and event, which
    Bench writes when `BENCH_DEBUG` is set (`platform.Logged`).
19. **Sub-tab drags follow the pointer, without HTML drag and drop.** In
    a `WKWebView`, an HTML drag runs as a native macOS drag session. On a
    real Mac, each drop took seconds to arrive, and once the session never
    ended: the page saw the drag start but neither the drop nor its end
    until the next click, and the drag after that didn't start at all. The
    frontend now starts a drag when the pointer moves 4 px with the button
    held on a sub-tab, moves a copy of the sub-tab with the pointer, finds
    the drop target with `elementFromPoint`, and swallows the click that
    follows the drop. Escape, a pointer cancel, or Bench losing the
    keyboard cancels the drag. WebKit on macOS reads `buttons` when it
    handles an event, not when it happened, so the last move of a quick
    release can report no button held just before the `pointerup`; on the
    real Mac, treating that move as a cancel lost every drop. So such a
    move cancels only if no release follows within 250 ms, and a `mouseup`
    counts as the release too, because WebKit sends a release it still
    reads as held on as a `pointermove`; a `mouseup` for another button
    doesn't. The release after a cancelled drag isn't a click either.
    Divider drags already followed the pointer, and end on a `mouseup` in
    the same way; a press on a divider cancels its `mousedown`, not its
    `pointerdown`, which would stop the `mouseup` too. Redraws, which wait
    while the button is held so that the element pressed isn't replaced
    before the click, also wait 250 ms after a move with no button held
    before going ahead.
20. **A window in several tabs is one item.** To show a window in more
    than one tab, its item goes in a pane of each tab, rather than each tab
    getting an item of its own that names the same window. A window can
    only be in one place, so Bench docks it in the current tab's pane, and
    the window's title, status, and binding stay as one: after its app
    restarts, the window matches the one item by title or creation order,
    and comes back to every tab at once. `Model.Where` gives an item's place
    in the current tab if it's there, or else in its first tab; moving the
    item, or removing it from a tab, changes one place, while closing or
    popping out the window takes it out of every tab. `state.json` lists the
    item in each tab's tree; an older Bench sets such a file aside as
    `state.json.bad`.
21. **The add dialog adds websites as web apps first.** A web app runs in
    the browser, with the browser's protections and sign-ins, which a Web
    item doesn't have, so **Web page…** calls `AddWebsite`. It adds the
    site's web app if one is installed: `apps.SameWebHost` compares the
    address's host with each web app's `AppInfo.WebURL`, which
    `apps.ReadBundle` reads from its `Info.plist`. Otherwise, if Bench has
    Accessibility permission and the default browser can make web apps
    (`apps.WebAppMakerFor`), it opens the site there and returns the
    browser's steps. `Platform.DefaultBrowser` names the default browser:
    the app that LaunchServices opens `https` links with
    (`URLForApplicationToOpenURL:`). The dialog then calls
    `AddWebAppWhenReady` every 1.5 seconds, one call at a time, which scans
    the installed apps without the engine's lock, as the scan reads every
    app's `Info.plist`, and adds the web app once it's there. That's the
    web app on the address's host, or else the only web app that wasn't
    installed when `AddWebsite` opened the site, which the engine records
    (`makingWebApp`): a browser records the address the site ended on,
    which a redirect can put on another host, and Orion records none. A
    Chromium browser opens the web app it makes, so Bench may have put its
    window in the tab already, by `new_windows`: it moves to the dialog's
    pane, and a web app that has started without a window yet gets a moment
    to open one, rather than being sent for a second. While the dialog
    waits, focus in the browser's windows in other tabs doesn't switch tabs,
    as `follow_focus` would, which would close the dialog; Bench doesn't
    hide the browser when Bench becomes active, as the user goes between
    the two, nor as it arranges the tab, by `hide_other_apps` or
    `hide_other_tabs` (it leaves the browser's windows in other tabs where
    they are, rather than parking them, since the browser may have opened
    the site in one); and the browser's new windows stay out of the tab,
    rather than joining a pane by `new_windows`. A choice
    to stop waiting waits for the call under way, so that it can't add a
    Web item as well as the web app. If the default browser can't make web
    apps, or Bench doesn't know how (`apps.CantMakeWebApps` tells the two
    apart for the note), and with Option-Return, the dialog adds a Web
    item, as before. In the browser preview, the demo platform's default
    browser, Safari, makes a web app of a site 3 seconds after it's asked
    to open it, standing in for the user.
22. **A web page switches to a web app in its place, with one click.** The
    Web items the user has, such as the example config's Slack and
    Outlook, don't become web apps by themselves: the user chooses, from
    the item's menu, which asks `WebAppChoice` as it opens. That scans the
    installed apps without the engine's lock, as `AddWebsite` does, and
    offers the web app of the item's `Home` address (`webAppFor`), or else,
    with Accessibility permission and a default browser that can make web
    apps, to make one. The scan can take a moment, so the menu opens at
    once, and the entry joins its end when the answer comes, moving nothing
    above it: a web page's menu opens at the menus' greatest width, and
    `fitMenu` places it once, leaving room below for the entry, and keeps
    it there (`ui.menu.top`), scrolling it if it grows past the window's
    bottom. An answer for a menu that has closed is dropped.
    `UseWebApp` makes an App item for the web app, with no name, as App
    items have none in `config.toml`, and `Model.Replace` puts it where the
    Web item was in every tab: the same pane and index, active where the
    Web item was. The Web item's view closes. The web app's window comes as
    `addAppLocked` and `addMadeWebAppLocked` find theirs: a window that's
    popped out or that Bench doesn't manage; the first window of a web app
    that's running without one; a new window; or the web app opened. Until
    then the item is a placeholder, and a pending placement with
    `pendingPlace.item` gives the window to it, without changing what its
    pane shows, where an ordinary placement shows a new item; the web
    app's other windows, such as the ones it restores as it opens, join
    the pane without being shown.
    `MakeWebApp` opens the item's site in the default browser, and
    `UseWebAppWhenReady` waits for the web app as `AddWebAppWhenReady`
    does, through the same `makingWebApp`, which records whether the web
    app joins a pane or replaces an item; for a web app just made, a window
    that `new_windows` put in the tab moves to the item. The frontend
    reuses the add dialog's make step, with Cancel as its only button.
    `state.json` alone keeps the switch across a restart, because a tab's
    saved entry keys stop `syncEntries` adding back an entry whose item has
    gone, but resetting the tab's layout adds back the items of entries
    that no item stands for, and a tab whose arrangement isn't kept from
    `state.json` loses the switch. So the Web item's entries in `config.toml`, found as
    `syncEntries` pairs entries with items, become `app` entries for the
    web app, in the same place, and Bench saves the file and then
    `state.json`, as a save from Tab properties does (`saveEdit`).
23. **A web page's pane header shows its host.** A web view has no
    address bar, and a Web item's page can reach another site without a
    click: a redirect, a script, someone else's tenant of a sign-in site, or
    a plain `http` page. So the pane header shows the host of the page in
    view in place of the **Web** badge, following Chromium's URL display
    guidelines: the site (see decision 24) is emphasised, a long host
    loses its start but never its site (in a narrow pane, the warning's
    words and then the host give way to the sub-tabs and buttons, leaving
    the warning's icon), a page that isn't secure says
    **Not secure**, one on another site than the item's address says **Other
    site**, and there's no padlock, since phishing pages use `https` too.
    The chip changes nothing about where pages may go, which decision 24
    covers. The native side follows the committed page, not whatever
    `WKWebView.URL` names, which can be a page that's still loading. It
    reads the URL in `webView:didCommitNavigation:` (where it's still a load
    Bench asked for if that load hasn't started: then it takes the address
    the committing navigation reached, from its start or its response), and
    when the page changes it itself, with pushState or a link to a part of
    the page: a URL change within the page's origin while no navigation is
    under way, looked at once WebKit has finished with it, since WebKit
    reports a navigation's URL just before it says that the navigation has
    started. The page is secure if
    `hasOnlySecureContent` was true when it came into view, and still is,
    counting changes only at those times, while WebKit's URL is on the
    page's origin. `WebURL` carries the address and what `mac.PageInfo`
    works out from it and the item's own address: the host and site, from
    the parsed address, so that a user name such as `app.slack.com@` in
    front of another host can't pass for it, and whether the page is secure
    and on another site. A `blob:` page, which a page makes of content it
    has, is described by the origin inside its address, as browsers show
    it, so that a page can't clear its warning by moving to a `blob:`
    address of its own making. Any other page with no web address, such as
    `about:blank` after a page, or a `data:` page, which the navigation
    policy keeps in view without a click, and which can hold anything, is
    on another site, with no host: the header says **page with no
    address** after the warning, rather than going back to the badge that
    an item with no page yet shows. A page with no web address is secure
    only if WebKit says that all its content is, and a `blob:` page only if
    its origin is local, or `https` with only secure content in WebKit's
    view; `hasOnlySecureContent` is false for any address that isn't
    `https`, so in practice only a local `blob:` page is. The engine shows
    the badge for a page with no web address only before any page has been
    in view: the native side reports nothing for a new web view's empty
    page, but loads `about:blank`, and reports it, for an address WebKit
    can't take. For a page that isn't `http` or `https`, **Open in
    browser** opens a `blob:` page's origin if that is, or else the item's
    own address, never the page's. The engine keeps these out of
    `state.json`: after a restart, a Web item loads the address it was
    given, and shows its badge until that page is in view, and **Open in
    browser** opens that address, not the last page that `state.json`
    keeps. Its pages are local, so VS Code for the Web shows none. The demo
    platform reports each address its web views are given as their page,
    so the browser preview shows hosts too.
24. **Go decides where a Web item's page goes, by what came before.** A
    web view's navigation delegate is asked about one navigation at a time,
    and much of what decides it lies elsewhere: who started it, whether it
    belongs to a navigation the user clicked, and what the page in view is.
    So the native side only reports: for each navigation of the item's main
    frame, its address, its kind (a link, going back, forward or reloading,
    or anything else), what started it (`WKNavigationAction.sourceFrame`:
    Bench's own load, which it tells by the address WebKit's URL took when
    it asked; no frame; the main frame; a frame inside the page; or a frame
    in another web view, a popup's), the source frame's security origin, and
    the user's last event in the web view; then each navigation's start,
    commit, and failure, by a number it gives each `WKNavigation`. The
    `mac.navigator` keeps each item's state and decides, and
    `navigation_test.go` and `navstate_test.go` follow attacks and sign-in
    flows through it on any OS.
    - A navigation counts as clicked when the user clicked it, when
      anything but the item started it (a frame inside the page, another
      window, or a page on another site that the user clicked through to),
      or when a page on the item's site that a click brought into view
      starts it within 5 seconds, which is how long client-side
      redirectors, such as link trackers' pages, take. A page on another
      site that counts as clicked goes to the browser. WebKit asks about a
      server redirect with the kind and source frame of the navigation it
      belongs to, and never says it's a redirect, so a navigation like that
      while one is under way keeps that one's answer, however long its
      server took. A page on another site that the item went to by itself,
      such as its identity provider's, still counts as the item's own, so
      that a sign-in flow through an organisation's own sign-in server
      (Microsoft to AD FS, say) works as before.
    - A page on another site kept in view by a navigation that counts as
      clicked (only a sign-in page is), or brought back by going back,
      forward, or reloading, is one the user clicked through to. When it
      sends the user on without a click to a page that's neither the item's
      site nor a sign-in page, that page opens in the browser (once for each
      page clicked through to), and the native side loads the item's
      address. Until that load's page is in view, every navigation but
      that load and its redirects is cancelled unless the user clicks it,
      and if the load fails by itself, the page in view stays, going only
      where the user clicks; a navigation that replaces the load has the
      item load it again, up to 3 times. The load's redirects are the
      item's own, wherever they go, as they are when nothing was clicked
      through to, such as Jira's to `id.atlassian.com`. The native side
      tells them by the navigation that WebKit asks about them for, which
      only a private property of `WKNavigationAction`
      (`_mainFrameNavigation`) says. Without it, a redirect to another site
      can't be told from a navigation that the page in view starts, so one
      that isn't to a sign-in page is cancelled.
    - Opening the browser or another app for a navigation, or for a popup
      (`PopupBrowser` and `PopupAsk`), needs a user event, and each event
      allows one (`webOpenLimit`, a `gestureLimiter` like the downloads'):
      the click itself, the click that a redirect or a redirector's page
      follows, or for a navigation that something other than the item
      started, a mouse-up or key-down in the web view within 2 seconds.
    - Sites are registrable domains on the whole Public Suffix List
      (`golang.org/x/net/publicsuffix`). Where the list doesn't give each
      customer of a shared domain their own name, `tenantDomains` does,
      for domains such as `atlassian.net`, `service-now.com`, and the
      identity providers that give anyone a sign-in page, but not for
      domains whose own apps move between names, such as `slack.com`.
      `userContentHosts` and Google Docs' forms and published documents are
      sites of their own, though `relatedSites` groups their apps' sites.
      Sign-in, notifications, downloads, blank popups, and the host chip's
      site and **Other site** all compare sites this way.
    - A blank popup needs a frame in the item's own web view whose origin is
      on the item's site, since the frame that opens one fills it.
25. **Go decides what a Web item's page's prompts say, and which may
    come.** A page's alerts and confirmations, and its requests for the
    camera or microphone, are app-modal alerts, since a sheet on Bench's
    window would sit under the docked windows; for the camera, WebKit's own
    prompt is such a sheet, so both `BenchWebItem` and `BenchPopup`
    implement `webView:requestMediaCapturePermissionForOrigin:...`. An
    app-modal alert blocks all of Bench, so what a page can put in one,
    and how often, is decided in Go (`prompts.go`), and `prompts_test.go`
    and `navstate_test.go` check it on any OS. The native side keeps only
    the waiting and showing prompt of each kind per item, and answers each
    exactly once: when it closes, when it's refused, or when the item, the
    popup, or the page that asked goes.
    - `dialogText` makes a page's text fit: 2,000 characters at most, and 8
      combining marks in a row at most. WebKit passes the text on whole,
      and `rangeOfComposedCharacterSequencesForRange:` widens a cut without
      limit, since a letter can carry any number of marks, so Go cuts
      before the character that the limit falls in, which it finds by
      looking back at most 32 code points, and otherwise at the limit. The
      native side reads at most 32,000 UTF-16 code units of the text, and
      converts it so that half a character on its own can't empty it.
    - `dialogHeading` and `mediaPrompt` name who's asking from the frame's
      security origin and the page in view: only the main frame is named as
      the page, a frame on another site is named as embedded in it, and a
      frame with no host of its own gets its answer at once, as does a
      request for the camera from a frame that isn't on the item's site.
    - `promptGate` keeps what the user stopped, and holds an item's
      prompts of a kind back once 3 of that kind have closed within 10
      seconds: dialogs and camera requests apart, so that confirmations
      can't refuse a call. The navigator tells it about each page that
      comes into view, since only it knows whether Bench loaded the page,
      whether the user clicked it, and the site of the page before. A stop
      lasts until a clicked navigation to another site or Bench's load, so
      that a page can't undo it by reloading itself. Holding back lasts
      until a clicked navigation, Bench's load, or 10 seconds with no
      request of that kind: each request refused meanwhile counts, so a
      page in a loop stays held back, but a single-page app, which changes
      views without a page coming into view, isn't held back for good.
26. **A check on a real Mac runs Web items' pages.** Decisions 24 and 25
    rely on what WebKit reports, such as a navigation's source frame and
    type, its redirects, and when a page's dialogs come, which only a real
    Mac shows. The
    [macOS web items check](development.md#the-macos-web-items-check) runs
    Bench against a test server with two https sites on the Mac itself,
    clicks in the pages with the real mouse, and reads and presses Bench's
    alerts with System Events. It tells what Bench decided from its log:
    when `BENCH_DEBUG` is set, `platform.Logged` gives its log to the
    platform, a `platform.DebugLogger`, and the macOS platform logs each
    decision about a navigation of a Web item's main frame, a new window,
    and a popup's navigation (`mac.logDecision`), with the target's scheme
    and host only: the rest of an address can hold tokens and private
    names.
27. **About Bench is the standard panel, and Acknowledgements has a window
    of its own.** Mac apps use macOS's standard About panel, so
    **Bench ▸ About Bench** stays Wails' `About` role, and `make app` copies
    `build/darwin/Credits.rtf` into `Bench.app` for the panel to show under
    the version. The licences of what Bench includes are long documents,
    read beside other work, so **Bench ▸ Acknowledgements** opens them in an
    ordinary window of its own (see [The window model](#the-window-model)),
    not in a dialog in Bench's window, which would need raised mode, bring
    back a hidden Bench, and close any open dialog. The window shows a
    static page, `frontend/acknowledgements.html`, with no links out of
    it, since it has no address bar, and no scripts but one, which keeps
    a link or file dropped on the page from loading in its place
    (decision 73). Wails holds the events
    that it sends every window until the page's runtime says it's ready,
    and this page has none, so the shell says so on its behalf. Choosing
    the item again brings the window forward, and Command+W, which the app
    menu leaves to Web items, closes it: the window binds the key itself.
    The list must stay right as dependencies change, so
    `build/acknowledgements` generates the page from what the shipped build
    links: `go list -deps` of `./cmd/bench` for darwin with cgo, for each of
    the Makefile's `ARCHS`, with its `APP_TAGS`. Its `components.go` lists
    the components by hand, each with the code that it took from other
    projects as its parts. The texts come from the module cache, `GOROOT`,
    `frontend/vendor`, and a few texts copied by hand into
    `build/acknowledgements/texts`. The generator's tests fail when the
    page is out of date, when the build links a module or `frontend/vendor`
    has a package that `components.go` doesn't list, and when a file that
    the app includes from someone else, the Go standard library's among
    them, carries a copyright notice that its component doesn't account
    for. See
    [Add or update a dependency](development.md#add-or-update-a-dependency).
28. **The first config holds only the default browser.** A config that
    names apps the Mac doesn't have fills tabs, from the start, with
    sub-tabs that only say so. So on first run, the engine writes
    `config.DefaultFor` the app that `Platform.DefaultBrowser` names: the
    settings and shortcuts in full, with their comments, and one tab,
    Research, holding that browser, or Safari, which every Mac has, if
    macOS names none. `DefaultBrowser` names no agent (`LSUIElement` or
    `LSBackgroundOnly`), as a browser picker such as Finicky is: it has no
    window to dock. Another browser than Safari also gets a rule that
    floats its picture-in-picture window, which Chromium browsers show in
    a window of their own. While `config.toml` has problems, and no
    config has loaded without them (decision 7), Bench runs with the same
    config. The fuller example config (`config.Example`), with Slack,
    Outlook, Zed, VS Code, a terminal, Brave, and Obsidian, is what the
    browser preview starts with, since the demo platform has all of them,
    and what the tests build on. The spec shows both files, and a test
    checks each byte for byte. An app that macOS doesn't know, from a
    config the user wrote, isn't asked for again until it launches or the
    user clicks **Open** (`OpenItem`). Like any App item without a window,
    its sub-tab is hidden (see [The window model](#the-window-model)), and
    its pane lists it as not installed, without **Open**
    (`RememberedApp.CanOpen`). Tab switches leave two more kinds of app
    alone in the same way (`openMissingLocked`):
    - **An app the user quit.** The engine records an `AppTerminated` in
      `state.json`'s `apps`, by lower-case bundle ID (`quit`), for an app
      with items, so that the user's quit lasts across a restart of Bench.
      A launch by any means clears it. Quits within 5 minutes after
      `LoggingOut` don't count, so that Bench opens those apps as usual
      after the next login: `internal/mac` sends it on
      `NSWorkspaceWillPowerOffNotification`, for Log Out, Restart, and Shut
      Down, through the same queue as app events, so that it reaches Go
      before the quits. If an app cancels the log-out, nothing says so,
      hence the time limit. An `AppTerminated` whose PID isn't the running
      copy's, as when an updater starts the new version before the old one
      has quit, takes only that copy's windows: the app stays running, and
      nothing is recorded.
    - **An app that gave no window.** When a tab switch, or **Open**,
      waited for a running app's window and none came within
      `launch_timeout`, Bench doesn't ask that app with that `url` again
      (`noWindow`) until it launches, or the user clicks **Open**: asking
      on every switch would only bring it forward. Switching away ends the
      wait before it can find that out, so `askedFor` records when each
      switch asked, and for which items: a switch within `launch_timeout`
      waits for that window without asking, and a later one, with the app
      running and none of the items filled, sets `noWindow`. A launch
      clears the records older than `launch_timeout`, but not newer ones,
      which it may be answering.
29. **Saved tabs match the config's by name.** The user guide suggests
    editing `config.toml` by hand for what the UI doesn't do, so a tab
    added, removed, reordered, or renamed there mustn't cost the other tabs
    their arrangement. At startup, `config.MatchTabs` pairs each of the
    config's tabs with the tab in `state.json` of the same name, ignoring
    case, wherever it is; if the lists differ only in one name, in the same
    place, that tab is taken as renamed. `modelFromState` keeps the
    arrangement of each tab paired, in the config's order and with its
    name, builds the others from the config, with IDs after the state's,
    and drops the saved tabs left over, with the items that no other tab
    has: their windows find places at the startup scan, as other windows
    do. A rename along with another change to the tabs can't be told from
    a new tab, so that tab's arrangement is dropped too. Reload pairs tabs
    the same way (decision 7).

    Nothing is dropped silently. `config.LoadState` sets aside a file that
    `CheckState` rejects, such as one that doesn't parse, as
    `state.json.bad`, and Bench starts from the config. For a file whose
    tabs it drops, it sets aside a copy, since the next save loses them. The
    copies before move along to `state.json.bad.1` and `state.json.bad.2`.
    Either way, a notice in the status bar names the tabs whose arrangement
    Bench couldn't keep, and says why, with **Dismiss**. A file it couldn't
    use can't be restored, and the notice says so. For dropped tabs, the
    notice keeps each tab as Bench loaded it, with the items it held, and
    offers **Restore previous arrangement** (`RestoreArrangement`) once a
    tab has a dropped tab's name again, whether renamed in Tab properties or
    in `config.toml` and reloaded; until then, it says how. A reload that
    deletes tabs does the same: it keeps their arrangement from the model
    as it was, sets aside a copy of `state.json`, and adds them to the
    notice, in place of a tab of the same name dropped before. Restoring a
    tab while Bench runs gives its panes and splits new IDs, so they can't
    repeat one in use, and its items keep theirs, which no item since can
    have taken, as new IDs come after the state's. A page or terminal that
    the tab has for the same entry as one of the arrangement's takes that
    one's place, as the tab built from the config has new items for the
    entries, and ending its session to start the arrangement's would lose
    what's running in it; the tab's other pages and terminals move to "Not
    in a tab", as with **Delete tab**.
30. **Bench fills the screen by zooming, from the start.** Native full
    screen would leave the docked windows behind in another space, so
    Bench's window zooms to its screen's visible area instead: from
    **View ▸ Fill the Screen**, the jump palette's **Fill the screen**
    (`FillScreen`), and at launch, with `fill_screen` on, the default.
    - **Where it opens.** While the displays are settled, each
      `BenchMoved` records the display Bench's window is on
      (`noteBenchPlaceLocked`), and `state.json` saves it as `screen`: its
      ID, and its name and frame too, since macOS can give a display a new
      ID. At launch the shell asks the engine (`WindowStart`), which looks
      first for the display that the user chose, `preferredScreen`, if
      it's connected and isn't the one Bench was last on (decision 70).
      Otherwise it finds the display Bench was last on by ID, then by name
      and frame together, since two displays of the same model have the
      same name, then by name, then by frame, or else takes the primary one
      (`findScreen`). It opens the window in the middle of that display's
      visible area (`mainWindowOptions`, with Wails' `WindowXY`).
    - **Where the user left it.** A window that doesn't fill its screen,
      as when the user tiles it into half the screen, is recorded too, as
      `screen.window`: its content frame, in global coordinates. At launch,
      `WindowStart.Frame` is that frame if the display's frame is as it
      was, the same size at the same place among the displays, and the
      window's frame lies inside its visible area, within 2 points
      (`openFrame`). Then the window opens there, and doesn't fill the
      screen, even with `fill_screen` on, which is for a window that filled
      its screen when Bench quit. Otherwise `Frame` is the usual 1440 × 900
      in the middle of the visible area, which Auto makes compact (decision
      58). A window that fills its screen saves no `window`. Wails makes a
      window 1 pt narrower and shorter than its `Width` and `Height`, and
      its web view, whose frame is the content frame, that size, sticking
      out 1 pt above the window. So `mainPlace` puts the window 1 pt below
      the frame's top, or the window would creep 1 pt up at each launch.
      `Frame` decides the size class at launch too, before the platform
      reports the window (decision 58). **Not** a frame per display: one is
      enough to put back a tiled window, and a display's frame changes as
      the user rearranges the displays.
    - **Filled before it shows.** Wails' `StartState` of `Maximised`
      doesn't do: Wails' window class animates `zoom:`, and positioning the
      window just after stops the animation part-way. Wails shows the
      window from a listener of its `WebViewDidFinishNavigation` event, by
      sending the work to the main thread. An event's hooks run before its
      listeners start, and the main thread runs what it's sent in order, so
      the shell's hook on that event sends `mac.Platform.FillWindow`
      (`ui.Options.Fill`) there first. It calls `NSWindow`'s own `zoom:`,
      skipping Wails' animation, so the window first shows filled, and
      AppKit keeps the frame it opened with, for the green button to give
      back.
    - **At least 640 × 480, zoomed or not.** That is small enough for
      macOS to tile the window into half the screen of a 13-inch MacBook,
      about 735 × 925 points at its default scaling, or into a quarter of
      a large display. 900 × 600 was wider than half a laptop's screen, so
      macOS couldn't tile the window there. In a window that narrow, the
      layout is compact (decision 58), the title bar slims (decision 31),
      and the dialogs move and resize inside the window (decision 34), so
      they fit. Double-clicking the title bar zooms the window through
      Wails' `Maximise`, which drops its minimum size until Wails' own
      `UnMaximise` puts it back; un-zooming with the green button, or by
      resizing, never does. So whenever the window resizes, the shell puts
      the minimum back once the window is no longer zoomed
      (`keepMinimum`).
    - **Filled again after the displays change.** Each `BenchMoved` while
      the displays are settled also records whether Bench's content frame
      fills its screen's visible area, within 2 points, since Wails' web
      view sticks out 1 pt above and to the right (`fills`). Moves while
      they settle after they change, or after the window goes to another
      screen, don't count, since macOS moves the window then
      (`screensMoved`). A wait after the Mac wakes doesn't change the
      screens, so a move then counts, as the user's (decision 56). Once
      they settle, if the window filled its screen before and doesn't fill
      the one it's on now, the engine asks the shell to fill it
      (`Emitter.FillScreen`, `refillLocked`). That moves the window within
      its screen, so it doesn't start another wait. A window the user made
      smaller is left as it is. While the window is hidden or minimised,
      when moves aren't the user's either, filling waits until it's shown,
      since zooming it could bring it back. If the display that the user
      chose has come back, the window goes back to it instead (decision
      70).
    - The menu, the palette, and filling again zoom with Wails' `Zoom`,
      which animates, and only when `IsMaximised` (AppKit's `isZoomed`)
      says the window isn't zoomed already: `zoom:` on a zoomed window
      would shrink it.
31. **More room for apps.** Two things give docked windows more of the
    screen, without taking anything away for good:
    - **The status bar can go** (`show_status_bar`). The frontend hides
      `#statusbar`, and `#panes`, which fills the rest of the window, grows;
      its `ResizeObserver` reports the new area (`SetPaneArea`), so the
      engine lays the panes out in it and docked windows grow too. Docked
      windows cover the pane bodies, so what the status bar showed moves to
      the title bar, which they never cover: the windows that need attention
      and the notices become amber buttons beside **Jump to…**, opening the
      same lists below them, and a toast sits in the middle of the title
      bar's empty space, between **+** and the controls at its right
      (`placeToast` in `state.js`). If that space is under 240 pt, it goes
      over the tabs, and failing that across the title bar, but never over
      the traffic lights, whose clicks macOS takes. It lets clicks through
      (`pointer-events: none`), so the title bar still moves the window
      under it, and shows for at most 4 seconds, since it covers that space.
      A toast's button, such as **Undo** (decision 37), takes clicks, there
      and over the status bar, and a toast with one shows for 10 seconds,
      but for 4 when it goes over the tabs, where its button would take a
      click meant for a tab. The browser preview does the same, so the
      end-to-end tests see it. The default `gap` is 4 pt, and Settings
      offers 8, 4, and 2 pt, never 0, which would leave no divider to grab.
    - **The title bar slims** below 1,200 pt, or in the compact size class
      (`syncSlim` in `render.js` sets `data-slim` on `#titlebar`, as it's
      drawn and as it resizes, and `styles.css` does the rest). Both forms
      of the layout control and of **Jump to…** are drawn, and CSS shows
      one, so a resize needs no redraw; if the layout control with the focus
      is hidden, the focus moves to the one shown. A slim title bar doesn't
      wrap: its strips give their room to the controls at its right. Below
      1,000 pt (`data-short-notes`, from `syncSlim` too), a note of the
      hidden status bar shows its number without its words
      (`.title-note-words`), which its accessible name and tooltip keep.
      So it stays one row at 640 pt with both notes, and the pane area
      doesn't move as a note comes and goes. At 1,000 pt and wider, the
      words fit, so a compact window at the usual size, 1,440 pt, keeps
      them. The slim layout button opens a menu
      (`kind: 'layout'`) of the three modes as `menuitemradio` entries,
      with why the layout is in use as its note. **View ▸ Layout**
      has the same three choices as radio items, without key equivalents,
      which Control+Option shortcuts would clash with; the app menu
      (`appMenu` in `internal/ui/menu.go`) ticks them from `MenuBar.Mode`,
      keeping the items it built under a lock that the main thread never
      takes, since `SetChecked` waits for it (decision 46). Tabs show their
      shortcuts as `tab_shortcut_hints` says: `auto` hides them in the
      compact size class. A tab's name is in its own `.tab-name` span, so a
      later close button can take the hint's place at its right end.
32. **Strips that don't fit scroll.** The tab strip and the panes' sub-tab
    strips scroll sideways rather than squash, so that every tab and
    sub-tab stays readable however many there are. `frontend/js/strips.js`
    does it:
    - **Sizes.** A tab is at most `--tab-max` (200 pt), its `.tab-name`
      cut short with an ellipsis and its shortcut kept; a sub-tab is
      `flex: 0 1 auto` between `--subtab-min` (96 pt), more for the marks
      before its title (decision 61), and
      `clamp(220px, 30cqw, 360px)`, where `cqw` is the pane's width (`.pane`
      is an inline-size container). Both tooltips have the whole name. The
      shown sub-tab has a slot for its × after its title, as a tab has,
      and is 22 pt wider for it, at its widest too. In a strip with
      `data-more` (below), the slot goes, and the × is over the end of the
      title. The slot doesn't change the sub-tabs' least widths, but it
      does change the strip's flex basis: beside a sibling that shrinks
      with it, such as the host in the `tabs` view's title bar, the strip
      is a few points narrower without the slot. So, measured as it is, a
      strip could be full or not at the same width, depending on what it
      was before, and a resize, which doesn't redraw the title bar, would
      keep the old answer. `syncStrips` measures a full strip again with
      `data-more` taken off (`settleFull`), as a fresh redraw does, and
      puts it back, with the scroll position, only if the strip still
      doesn't fit. A strip that's full with the slot is full without it,
      which leaves it no wider, so the answer can't go back and forth.
      Scrolling doesn't change it, so a `scroll` event doesn't measure
      again.
    - **The wheel.** One listener each on `#titlebar` and `#panes`, which
      outlive redraws, takes wheels over a strip. It isn't passive: over a
      strip that doesn't fit, a mainly vertical wheel scrolls it sideways
      (`preventDefault`), with lines and pages turned into points; a
      sideways swipe is left to the web view. Any wheel over a strip hides
      the live preview, whose sub-tab may move away from the pointer.
    - **Keeping what's shown in view.** `syncStrips` runs after every
      render, and with `resized` as `#titlebar` or `#panes` changes size.
      It scrolls each strip by hand (`scrollLeft`, not `scrollIntoView`,
      which would scroll the page's other scrollers too) to keep the
      current tab, or the pane's shown sub-tab, clear of the fades. Redraws
      replace a strip's contents but keep its position (`data-scroll` in
      `setRegion`), so without more, a strip that the user scrolled by hand
      would jump back at the next `bench:state`. So a wheel over a strip, or
      a press on its scroll bar, marks it browsed, with what it showed then:
      redraws leave a browsed strip alone until it shows another tab or
      sub-tab, or has one more or fewer, and a resize clears every mark. A
      strip with the keyboard on another of its tabs or sub-tabs is marked
      too: as the arrow keys move the focus there, `revealFocus` scrolls it
      into view and marks the strip (decision 64), and so does a redraw
      that shows what the last one did. A `focusin` listener can't mark
      it, since `setRegion` puts the focus back as it redraws: a strip
      whose current tab has just changed would be marked before it could
      show it.
    - **Fades.** `data-more="start end"` names the ends with more to show,
      updated as a strip scrolls (a capturing `scroll` listener on the
      document, since strips don't bubble it) and as it's drawn. A
      `mask-image` gradient fades those ends over `--strip-fade` (24 pt).
      Its second layer is opaque over the bottom `--bar-room`, so a shown
      scroll bar doesn't fade.
    - **Dragging.** `edgeScroll`, which `drag.js` calls as a tab or
      sub-tab drag moves, scrolls the strip under the pointer by up to
      14 pt a frame (`requestAnimationFrame`) while the pointer is within
      24 pt of either end, faster nearer it, and `stopEdgeScroll` ends it
      with the drag. The drag listens for scrolls anywhere while it lasts
      and marks its drop target again, because what's under a still
      pointer has changed. A tab dragged along the tab strip, and a
      sub-tab along a strip of sub-tabs (decision 38), can go to a place
      that was out of view.
    - **Scroll bars.** `scroll_bars` chooses: `always`, `never`, or `auto`,
      which follows macOS. `internal/mac` reads
      `NSScroller.preferredScrollerStyle` on the main thread at start and
      at each `NSPreferredScrollerStyleDidChangeNotification` (System
      Settings' Show scroll bars, or plugging in or unplugging a mouse),
      caches whether it's legacy, and sends `ScrollBarsChanged` when that
      changes. The engine keeps the answer and sends it in the View as
      `systemScrollBars`, so the frontend decides (`scrollBarsShown` in
      `util.js`) and sets `body.scroll-bars`. The Fake and the browser
      preview report overlay scroll bars, so `auto` hides them there.
      Hidden, a strip has `scrollbar-width: none` and, for WebKit before
      Safari 18.2 (macOS 14), `::-webkit-scrollbar { display: none }`.
      Shown, the bar is 4 pt (`--strip-bar`), styled with
      `::-webkit-scrollbar`, which both WebKit and Chromium follow only
      while `scrollbar-width` is `auto`. It sits in padding that's there
      anyway: the tab strip is a fixed `--tab-height` plus the bar, with a
      negative bottom margin of the bar into the title bar's padding, and a
      sub-tab strip fills the pane header's height with its sub-tabs held
      at the top, where they'd be centred. So neither the title bar nor a
      pane header changes height, `--pane-header` and Go's `HeaderHeight`
      stay in step, and no docked window moves whether a strip fits or
      not. A denser pane header must stay taller than its sub-tabs by at
      least twice the bar, and a denser title bar must keep at least the
      bar's height of bottom padding. In the app, the title bar moves the
      window (`--wails-draggable: drag`), so a strip that doesn't fit, with
      its bar shown, is `no-drag`, for a press on the bar to scroll it.
33. **Density and pane headers.** A 13- or 14-inch MacBook has little
    room to spare, so Bench's chrome can shrink, and a pane's header can go:
    - **Density.** `density` is `auto`, `standard`, or `compact`; `auto` is
      compact in the compact size class (`densityFor` in
      `internal/workspace/headers.go`). The View carries the density in use
      as `density`, and `render.js` sets `data-density` on the body, which
      `styles.css` turns into smaller sub-tabs and buttons and a 3 pt
      scroll bar. Pane headers are 40 pt (`layout.HeaderHeight`) or 32 pt
      (`layout.CompactHeaderHeight`). Go decides each pane's header, so
      `paneGeom` has a `header`, `layout.Body(frame, header)` places the
      window below it, and the View sends it as each pane's `header`, which
      `renderPane` sets as the pane's `--pane-header`. Nothing in the
      frontend assumes a height, so the CSS can't drift from Go's
      arithmetic. A future drop hit test in Go must take the pane's header
      the same way. A 32 pt header still leaves twice the 3 pt bar below
      26 pt sub-tabs (decision 32).
    - **The title bar at launch.** The traffic lights are centred in the
      native title bar, whose height the window's toolbar style sets:
      unified (about 52 pt) or unified compact (about 38 pt). Changing the
      style of a live window would move the traffic lights under title bar
      views that pass clicks through (decision 12), so `ui.mainWindowOptions`
      chooses it once, from `WindowStart.CompactTitleBar`: the density that
      `density` gives at the width the window opens at (decision 58). The
      engine keeps it as `titleBar`, sent in the View, and `render.js` sets
      `data-title-bar`, which sizes Bench's title bar (38 pt and a 1 pt
      line, or 52 pt and a line) to match until Bench opens again. Settings
      says so in the app, for a density that would change it (`densityHelp`
      compares it with `titleBar`). The browser preview has no native title
      bar, so its `titleBar` follows the density.
    - **The `tabs` view's pane has no header.** Its sub-tabs, **+**, and
      the other header controls are drawn in the title bar after the tabs'
      **+** (`titlePane` in `render.js`), with the same markup, so clicks,
      drags (the drag source is any `.subtab`), right-clicks, markers, and
      the scrolling strip (`strips.js` takes `#titlebar .subtabs`) work as
      in a header. Its frame reaches up to the top of the pane area
      (`layout.ReachTop`, which `bench layout` uses too), over the gap, and
      its header is 0.
    - **`pane_headers = "auto"`** drops the header of a pane that holds one
      App item with a window, while Bench is trusted, that has nothing to
      report (`soleAppLocked` and `nothingToReportLocked`); the app's own
      title bar stands in. A window that needs room is judged against the
      body without a header: `windows.Manager.NeedsRoom` compares it with
      the body that the window last failed to fit, so docking it in the
      taller body is tried once, and if it still doesn't fit, the header
      comes back and stays. `updateHeadersLocked` keeps, per displayed pane,
      whether it's hidden and since when it has wanted the other way: a
      header comes back after a status has lasted 1 s (`headerShowDelay`),
      goes after 2 s of nothing to report (`headerHideDelay`), and a timer
      arranges the tab again then. A second item brings it back at once,
      a pane seen for the first time decides at once, and panes that aren't
      displayed are forgotten. `arrangeLocked` updates the headers before
      each pass and again after applying it, since statuses change only as
      windows are docked; a change after the first pass runs the second,
      and one after the second sets the timer to arrange again at once.
      The jump palette's **Add to Pane…** and its **This window** group
      (decision 49), which ends with **Window Options…**, stand in for the
      missing buttons. The last opens the ⋯ menu in the palette's place
      (`openWindowOptions`), without a render between, so Bench isn't
      lowered and raised again; with no ⋯ button to hang it from, it opens
      inside the pane's top right corner.
34. **Dialogs move and resize inside Bench's window.** Tab properties,
    Settings, the add dialog, Jump to…, and the welcome guide stay HTML
    overlays in the main window, rather than windows of their own, as
    Acknowledgements is (decision 27), or sheets. A window of their own
    would need the drafts, the control to give the focus back to, and the
    keys rebuilt in another page, and would sit among the docked windows,
    or float over other apps; the palette would be slow to open; and a
    sheet can't move. So `frontend/js/dialogs.js` moves a dialog by its
    header (`.dialog-handle`, but not a field or button in it), with
    pointer capture, and resizes it from a grip in its bottom-right corner.
    A press becomes a move or a resize once the pointer has moved 4 pt, as
    for sub-tabs, so a click that wobbles changes nothing. Renders wait
    while the button is held (`installRenderDeferral`), so the dialog
    isn't replaced during a move, which sets its style itself. Where the
    user moved each dialog is kept by kind in `store.ui.dialogPositions`,
    and the size they gave it in `store.ui.dialogSizes`, in content
    coordinates, as they left them. `placeDialog` fits them to the window
    as it is and writes the result into the dialog's style, after every
    render and whenever the window changes size, but never writes it back,
    so a dialog that a smaller window cut short gets its size back when the
    window grows. The HTML leaves the frame out, so moving a dialog never
    redraws it. The frame is fitted to the window: 8 pt from its edges, at
    least 420 by 300 pt (200 pt tall for the palette and the add dialog,
    and 560 pt wide for Settings, decision 66), and in the app, out of the traffic lights' corner, 78 by 52 pt, where
    they take the clicks (decision 12). Moving a dialog doesn't size it:
    one with no size of its own stays as tall as its contents, which
    change as the palette filters or the add dialog changes step, and is
    cut short above the window's bottom edge, down to its smallest height,
    where its middle scrolls. The size the user gives a dialog goes to Go
    (`SetDialogSize`), which keeps it in `state.json`, saving it lazily,
    as it does a new title, and sends it in the View's `dialogs`, rather
    than in `localStorage`, which might not last on Wails' own URL scheme.
    A dialog with a size but no position opens in the middle, and a
    double-click on its header drops its position to put it back there.
    Raised mode stays on while a dialog is open, so moving it never shows
    the docked window behind it.
35. **Bench asks before ending busy terminals, without blocking.**
    Quitting Bench ends its terminals, and a build, a server, or `claude`
    with them, so with `confirm_quit_busy_terminals` on (the default),
    Bench asks first while a terminal is busy. `term.Host.Busy` says
    whether one is, and what it runs: a session is busy while its PTY's
    foreground process group (`TIOCGPGRP`) isn't its shell's, as while a
    command that Bench typed for a launcher runs (decision 41), or, for a
    config `command`, which Bench runs with `$SHELL -l -c` and so becomes
    the session's process, while that runs. The question is Bench's own,
    in its window, rather than an `NSAlert`: Wails' `ShouldQuit` comes on
    the main thread, which a modal alert would hold, and the engine
    mustn't be waited for there (see
    [Threads and locking](#threads-and-locking)). So `ShouldQuit`
    (`appShell.shouldQuit`) answers at once. It lets the quit through
    while the user is logging out, restarting, or shutting down, since
    refusing would cancel the log out: `Platform.LoggingOut` reports it
    from the quit's Apple Event, whose `kAEQuitReason` says so as macOS
    asks Bench to quit, or from `NSWorkspaceWillPowerOffNotification`,
    which `internal/mac` notes as it comes, within `LogOutGrace` (5
    minutes), as the engine does for apps that quit (decision 28). It lets
    it through too once the user has answered (`passQuit`). Otherwise it
    asks the engine (`AskToQuitNow`), which refuses the quit if a terminal
    is busy, and sends `confirm-quit`, naming the terminals by sub-tab
    title and tab, in the tabs' order, then those in windows of their own.
    The shell brings Bench forward for it, and the frontend shows the
    question (`frontend/js/quit.js`), over any dialog, which **Cancel**
    goes back to. Its **Quit** calls `Quit`, which has the shell quit
    again (`Emitter.Quit`), with `passQuit` set; with **Don't ask again**
    ticked, it first saves the setting off, as Settings would. If the
    engine's lock is held, `ShouldQuit` refuses the quit and asks
    `AskToQuit` on a goroutine, quitting again at once if nothing is busy,
    or after 5 seconds without an answer (`quitAskWait`), so a stuck engine
    never stops Bench quitting. A popped-out terminal's window asks the
    same as the user closes it: a Wails `WindowClosing` hook, which runs
    before the listener that closes the window, cancels the close while
    `AskToCloseTerminal` says the terminal is busy, and the window shows
    the question (`confirm-close-terminal`), whose **Close** calls
    `CloseTerminal`. Closes that Bench makes itself, having forgotten the
    window first, and those as Bench quits, never ask. Signals never ask
    either: in the app, Bench handles `SIGHUP`, `SIGINT`, and `SIGTERM`
    itself (`quitOnSignals`, with Wails' own handler turned off), setting
    `passQuit` before it quits, and drops later signals, so that a second
    one doesn't end Bench before it has put every window back. The browser
    preview never asks: its terminals are pretend ones (`term.Echo`), and
    Wails' server mode never asks `ShouldQuit`.

36. **Closing presses the window's close button, after Bench is lowered, and
    checks a moment later.** Bench closes another app's window as the user
    would, so that the app can ask about unsaved changes:
    `Platform.PressClose` presses the window's `AXCloseButton`, or, for a
    window without one, focuses it and presses its app's **Close Window**
    menu item, or failing that **Close** (`closeWindowRank`, in English and
    the languages of decision 75), never **Close Tab** or **Close All**.
    Web pages and terminals close at once, in the engine, without the
    page's `beforeunload`. The engine's `CloseItem` and `CloseTab`
    (`internal/workspace/close.go`) mark the App items they close
    (`Item.closing`) and press later:
    `pressCloseSoonLocked` queues the presses while Bench is raised, as it
    is while the question or menu that asked is open, and
    `setOverlayLocked(false)` presses them once Bench is back below the
    docked windows, so that a save sheet isn't hidden behind it. `CloseTab`
    deletes the tab at once, through `deleteTabLocked`, the same path as
    `DeleteTab`, so that no timer holds a tab's index: its App windows that
    are closing are then not in a tab, which unhides their apps, as for any
    floating window (`planOthersLocked`), before the press. A window that
    Bench had parked stays parked until the check below finds it still open,
    which puts it back on screen (`releaseWindowLocked`). Unhiding briefly
    shows the windows of a hidden app, but a window that's still open then
    has its app's question in view, and the presence check below can tell a
    window that its app hid on close from one that Bench hid. A window that
    has gone already at the press closed before `WindowDestroyed` came: its
    item goes, and Bench arranges and focuses its pane as `WindowDestroyed`
    would. A second Close on a window that Bench has asked already
    (`Item.closing`) does nothing, as the app's close button is off while it
    asks about unsaved changes. `closeGrace` (1.5 seconds, after the 1
    second the macOS layer waits before it reports `WindowDestroyed`) later,
    `checkClosedLocked` asks the platform for each window. `ErrNoWindow`
    means it closed. `ErrOtherDesktop`, for a window that wasn't on another
    desktop at the press, means its app hid it rather than close it, as
    Slack and Music do: the app no longer lists it, and its app isn't
    hidden, so it counts as closed too, and its item goes. It's let go as
    `releaseWindowLocked` lets a window go, not forgotten where it is: one
    that Bench had parked can't be put back while it's hidden, so it stays
    parked, and `RestoreAll` puts it back as Bench quits, rather than the
    app showing it again off screen. Anything else is still open, as its app
    is asking, say: it's released to **Not in a tab** if its tab was closed
    (`releaseWindowLocked`), or shown in its pane, focused, and named in a
    toast. A closing item goes as its window closes, through the usual
    `WindowDestroyed` path, and one whose app quits with its window is
    removed rather than remembered (`onAppTerminated`). A press that fails
    gives the item back at once and says why. The question before closing is
    Go's: `CloseTab` and `CloseItem` return a `CloseQuestion` (`ask`) rather
    than close, unless `sure`, so that the frontend shows only what Go
    counted, and Go doesn't hold a call open waiting for the user.
    `confirm_close` governs a tab's question, for two or more things or a
    busy terminal; a single busy terminal's question, from its sub-tab,
    follows `confirm_quit_busy_terminals`, as closing its own window does
    (decision 35), and so does a tab's with `confirm_close` off, when it
    would end a busy terminal (`busyOnly`), whose **Don't ask again** then
    turns `confirm_quit_busy_terminals` off, so that turning off the tab's
    question alone never lets a running program end without asking. Whether
    the × closes a window that's in several tabs or only takes it out of
    this one (`RemoveItemFromTab`) is the frontend's choice, as it knows
    which tab the × is in; the menu's **Close Everywhere** closes it.
    **Tab ▸ Close Tab…** and **Pane ▸ Close Window** have no shortcut, so
    that Command-W stays with the window that has the keyboard (decision
    46).
37. **Undo puts back a snapshot of the arrangement, and closed tabs are kept
    in `state.json`.** Each change that the user might regret, and that a
    toast reports, keeps the arrangement from before it
    (`internal/workspace/undo.go`): **Delete tab**, **Close Tab…**,
    **Reset layout**, **Close this pane**, a move of a window or a tab by
    dragging or from a menu, **Also Show In**, **Move to Tab ▸ New Tab** and
    **Also Show In ▸ New Tab** (decision 39), a tab made of open windows
    (`NewTabFromWindows`, decision 54), a move to another tab with the
    `send_to_tab_modifiers` keys (`SendToTab` and `SendToLastTab`,
    decision 57), **Show Side by Side (Wide Layout)** in the `tabs` view
    (`ShowSideBySide`, decision 58), **Remove from this tab**, equal
    sizes from a divider (`EvenSizes`, decision 40), the keys that arrange
    the panes (`GrowPane`, `ShrinkPane`, `EqualPanes`, and `StepLayout`,
    decision 59), a run of which shares one step, a drag of a docked
    window's edge that resizes the panes (decision 62), and closing a web
    page or terminal (`CloseItem`, decision 65).
    `beginUndoLocked` notes what `state.json` saves of it, the model's
    tabs, in order, with copies of their trees, sizes, and focus, the
    current tab, and what's "Not in a tab", and `keepUndoLocked` keeps it
    once the change is made, with a
    number for the toast's **Undo** (`undoToast`), and the items the change
    forgot (App items without a window, which only remember a place) and
    those it added (the pages and terminals that **Reset layout** opens for
    config entries). A step of `ShowSideBySide` also keeps the layout's
    mode from before it (`setsMode` and `mode`). Of the steps that an Undo
    takes back, the oldest that has a mode gives the mode back. The engine
    keeps the last 10, in memory only.
    `Undo(step)` takes back that change and every later one, latest first,
    and puts back the oldest one's arrangement, reconciled with what has
    happened since: an item that has gone, such as a window that closed, is
    taken out of its pane, which closes up if that empties it, so Undo after
    **Close Tab…** brings back the tab and what's still open, never what
    closed, which the toast says; an item that has come since stays where
    it is, in its pane if the tab still has it, or else in the tab's
    focused pane, or else "Not in a tab". The same `Tab` values come back,
    so a deleted tab comes back as itself, with its entry keys. A change
    that deleted a tab also keeps `config.toml`'s tabs and rules
    (`beginDeleteLocked`), and so does one that added a tab for a window
    (`MoveItemToNewTab`) or for open windows (`NewTabFromWindows`), or
    that **Tab properties** or a rename in place made to a tab that was
    there (`SaveTab`, decision 74), which Undo puts back and saves, keeping
    the settings saved since. A step of
    `NewTabFromWindows` also notes the windows it took from "Not in a tab"
    as they were (`outsideWindow`), which Undo puts back
    (`putBackOutsideLocked`, decision 54). A window that comes back to a tab from "Not in a
    tab" docks again (`bringBackLocked`), and one that a closing tab had
    queued to close stays open. A change that moved a tab marks its step
    (`reordered`), and Undo puts `config.toml`'s tabs back in the order of
    the step's tabs, by name (`inTabOrder`), and saves them. Any other
    change to the tabs, such as a tab added in **Tab properties**, a
    reloaded `config.toml`, **Restore previous arrangement**, or **Reopen
    Closed Tab**, forgets the history (`forgetUndoLocked`),
    and Undo refuses a step whose tabs don't match
    the config's by name, so it never brings back a tab that `config.toml`
    no longer agrees with. Command-Z belongs to the pages and terminals,
    and a change elsewhere would be undone unseen, so Undo away from its
    toast (decision 65) has a shortcut of its own, off by default.
    A deleted or closed tab is also kept for **Reopen Closed Tab**, the
    last 5, latest first (`closedTabLocked`): its `TabState` and items, as
    `state.json` saves a tab, its `config.toml` entry and rules, and where
    it was. `state.json` saves them (`closed`), so they outlast a restart,
    and `LoadState` drops one that couldn't come back, rather than the
    whole file, and keeps its item IDs in use (`raiseSeq`). `ReopenTab`
    puts the entry and rules back, numbered if another tab has its name
    now, and brings back its arrangement as `RestoreArrangement` does
    (`restoreTabLocked`, decision 29), so that pages and terminals open
    again and places wait for windows, as at startup, while what another
    tab has now stays there. A toast's button is a bound method that takes
    one number, named by Go, so the frontend runs it without knowing what
    it does; it checks only that the name looks like a method's.
38. **A strip of sub-tabs takes a drop between sub-tabs, a pane's body
    splits, and a tab moves with its `config.toml` entry.** Where a dragged
    sub-tab lands follows one rule. Over a strip of sub-tabs, which is a
    pane's header, buttons and all, or in the `tabs` view the title bar's
    part with the pane's sub-tabs (`.title-pane`), it goes between two
    sub-tabs, where a caret shows, and never splits: dropping near a pane's
    top edge used to split it, when what was meant was the strip. Over the
    pane's body, the outer quarter on a side splits the pane there, and the
    middle joins it, measured on the body alone, and the indicator covers
    the body. Over a tab, it moves the window to that tab, as before.
    `drag.js` applies the rule (`targetAt`, `stripPlace`, `zoneFor`), and
    `layout.DropAt` holds it for Go, for a point over a pane's frame: in
    the header, a strip; on the body, an edge in its outer quarter
    (`EdgeShare`), or the middle, ties going left, right, top, then bottom
    in both. Go finds no drop targets itself yet; when it does, as for a
    drag that starts on a docked window's title bar, it is to use
    `DropAt`, with the title bar's strip in the `tabs` view as a place to
    insert too. The place among a strip's sub-tabs depends on how wide the
    frontend draws them, so it stays the frontend's.
    A drop on a strip calls `MoveItemAt` with the place among the strip's
    sub-tabs as drawn, before the one there or after the last. Hidden
    items (App items without a window) have no sub-tab, so the frontend
    counts only what it draws, and Go maps the place to one among all the
    pane's items (`placeAmong`): before the item that sub-tab shows, or
    after the last. A place next to the sub-tab itself changes nothing, so
    the caret doesn't show there and Go does nothing, with no toast. In the
    `tabs` view, `compact` stands for the one strip, which shows the tab's
    windows in reading order; the window stays in its own pane when a
    sub-tab beside its new place is in that pane, so that putting sub-tabs
    in order there leaves the wide layout's panes alone where it can, and
    otherwise joins the pane of the sub-tab it goes before, or after
    (`compactPlaceLocked`). Its own pane stays there even if that leaves
    it empty (`MoveAt` with `keep`), and the toast says so: closing it up
    would change the wide layout, which the user can't see from the
    `tabs` view. A move along one pane keeps the tab's preset, since no
    pane changed shape; one into another pane makes the layout custom, as
    `MoveItem` does. Both are saved to `state.json` as other moves are,
    and have **Undo** (decision 37). A sub-tab's menu has **Move Left**
    and **Move Right** along the strip it's drawn in, which call
    `MoveItemAt` too.
    Tabs move along the title bar the same way. A press on a tab starts a
    drag after 4 px, as a sub-tab's does (decision 19), but never a press
    on its × (`.close-cross`), and never with one tab. The title bar's own
    space moves the window in the app (`--wails-draggable: drag`) and isn't
    a tab, while the tabs and the tab strip are `no-drag`. A drop anywhere
    in the title bar counts, by where the pointer is along the strip;
    elsewhere, the tab stays put. The caret between tabs is thicker, with a
    dot at each end, so it isn't taken for the tint that marks a sub-tab's
    drop on a tab. Both kinds of drag share `ui.drag`, so Bench is raised
    while either lasts and redraws wait for it to end, and both scroll a
    strip near its ends (decision 32).
    `MoveTab` reorders the model's tabs and `config.toml`'s together
    (`moveTabLocked`), under `cfgMu` and the engine's lock, so that no
    reader sees one order in one and another in the other. The current tab
    is followed by identity, not index, so it stays current; a window's
    tabs come with it, as a tab holds its panes; and `state.json` matches
    tabs with `config.toml`'s by name (decision 29), so each keeps its
    arrangement across a restart. Rules name their tabs, so they need no
    change. It saves with `saveEdit`, as **Tab properties** does: a `.bak`
    of the old file, no save over a file edited outside Bench (decision 7),
    and `state.json` held until `config.toml` is saved, so that a restart
    never finds the new order in one file and the old in the other.
    Shortcuts belong to places (`tabShortcutText`), so they follow the
    order with nothing to update. **Move Tab Left** and **Move Tab Right**
    are in a tab's menu, the jump palette (for the current tab), and the Tab
    menu (`MoveCurrentTab`), whose items the shell enables from
    `MenuBar.Current` and the number of tabs, as it labels **Close Tab…**.
    **Tab properties** had a **Shortcut** field whose **Change** button
    only explained that shortcuts follow the order; it's now **Position**,
    a list of the places, each named with its shortcut ("1st (⌃⌥1)"), which
    `SaveTab` applies (`place`) in the same save as the rest of the draft.
    Like any **Tab properties** save, that forgets the undo history.
39. **Tabs and sub-tabs are renamed in place, and a window gets a tab of
    its own in one call.** **Rename…** in a tab's menu used to open
    **Tab properties**, a whole dialog for one field. Now it, and a
    double-click on the tab, turn the tab's name into a field in the title
    bar (`rename.js`, `ui.rename`), as browsers and terminals do. Return
    saves with `SaveTab`, so there's no method of its own: preset `custom`
    keeps the tab's panes and its `config.toml` layout, and the tab's own
    small-screen setting goes with it. A name that another tab has is
    numbered as Tab properties numbers it (`uniqueTabName`), and the note
    after the field shows the number before Return, read out as it
    changes. An unchanged or empty name just closes the field, and Escape,
    or the field losing the focus, to a click elsewhere or another app,
    cancels. A refusal leaves the field, with Go's message, while it keeps
    the focus. The field doesn't raise Bench: the title bar is never
    covered by docked windows, so the note sits in the tab rather than
    below it, where they would cover it, and a terminal asking for the
    keyboard waits until the field closes (`applyTermFocus`), as for a
    menu.
    The title bar's own space moves the window, and Wails zooms the window
    on a double-click there, on whatever its computed `--wails-draggable`
    is `drag`. Tabs are buttons, which are `no-drag`, but the tab being
    renamed isn't one, so `.tab` and `.subtab` in the title bar are
    `no-drag` themselves, and the double-click handler stops the event
    too. Switching focuses the tab's window or page, which takes the
    keyboard from Bench, and Go handles each call on its own goroutine,
    so a click's `SwitchTab` could focus the window after the field has
    opened. The second click of a double-click doesn't call `SwitchTab`
    again (`detail` above 1). On the current tab, where a click only
    gives the keyboard back to the tab's window, a single click waits
    300 ms before it does (`clickTab`), and the double-click cancels it.
    On another tab, the first click must switch, so as the field opens,
    the frontend waits for that `SwitchTab` to answer and then calls
    `FocusUI`, which gives Bench's UI the keyboard back (`newFocusLocked`
    and the platform's `FocusUI`, as for a terminal). **Rename…** in a
    menu calls it too: Bench being lowered as the menu closes gives the
    keyboard back to a page that had it, unless a focus was asked for
    meanwhile, and `FocusUI` is one, whichever comes first.
    A terminal's or web page's sub-tab can have a name of its own:
    **Rename…** in its menu opens the same field in the sub-tab, and
    `RenameItem` sets the item's `Label`. `Title` stays what the shell or
    page last set, and keeps changing; everything that shows an item's
    title asks `shownTitle`, which prefers the label: its sub-tab, the jump
    palette, toasts and the questions before closing (`labelOf`), the add
    dialog's **Windows in other tabs**, and the terminals named before
    quitting. The View sends the label as `label`, and the hidden title as
    `autoTitle`, which the sub-tab's tooltip and **Use Automatic Title**
    show. `state.json` keeps the label, so it outlasts a restart. An App
    item's title comes from its app, and rules match windows by it, so it
    can't be renamed. A label is one line of up to 200 characters, with
    its runs of spaces collapsed, and the field takes no more.
    **Move to Tab ▸ New Tab** and **Also Show In ▸ New Tab**, from a
    window's menu, the jump palette, and a sub-tab dropped on the tabs'
    **+**, are one engine call, `MoveItemToNewTab`, rather than `SaveTab`
    and then `MoveItemToTab`, so there is no moment with an empty tab, or a
    second save, and the item never lands in the tab that was current. Under
    `cfgMu` and the engine's lock, it names the tab after the item's app
    (`uniqueTabName`: "Code (1)", then "Code (2)"), adds it to the config
    with the single layout and validates it, adds it to the model with the
    item in its one pane (`Model.NewTabWith`), taking the item out of the
    tab it's shown in, which becomes custom, unless `also`, and switches
    to it. It saves with `saveEdit`, holding `state.json` as **Tab
    properties** does. Unlike a tab added in **Tab properties**, its
    toast has **Undo**, since a drop on the **+** is easy to make by
    mistake: its step keeps `config.toml`'s tabs and rules from before
    (`cfgTabs`), as Delete tab's does, so Undo takes the new tab out of
    `config.toml` too, and saves, and the history before it stays. A
    popped-out item comes back into Bench in the new tab; it can't also
    stay out.
    In a sub-tab's drag, `drag.js` treats the **+** (`data-drop-new`), and
    the title bar's own space just after it on its row, for as far again
    as the **+** is wide and short of the next control, as a new tab:
    tinted like a tab dropped on, with the ghost's **+** when Option is
    held, and a dashed outline on the **+** throughout the drag. The rest
    of the title bar is the window's, so a drop there makes no tab and
    cancels the drag, as before. That space still moves the window when
    pressed, since a sub-tab's drag follows the pointer without pressing
    there (decision 19). A tab's own drag is unchanged: over the **+** it
    shows the caret at the end of the tabs.
40. **A zoom is only drawn, never saved, and a split with an empty pane
    goes again unless something is added.** **Zoom Pane** shows one pane
    alone for a moment, as for a long diff or log on a laptop, without
    the compact layout, which changes every tab and puts every pane's
    sub-tabs in one strip. The engine notes the zoomed pane and its tab
    (`zoom`, in `internal/workspace/panes.go`), and changes nothing in the
    model: `displayLocked` draws that pane alone, with its own sub-tabs,
    in the whole pane area, with no dividers, and `planLocked` gives the
    windows manager a stack with no window shown for each of the tab's
    other panes (`unshownStacksLocked`), so that their App windows park
    as a pane's do while it shows a page or a terminal, and come back to
    their places when the zoom ends. A zoomed pane always has its header,
    even with `pane_headers = "auto"`, to show its **Zoomed** chip.
    The zoom ends without each method having to end it: `zoomedLocked`
    checks it whenever the display is built, and forgets it once it no
    longer applies, when its tab isn't current, its pane isn't focused,
    the tab has one pane left, or the tab shows the `tabs` view. So a tab
    switch, a focused pane, a moved window that takes the focus with it,
    **Close this pane**, and a change to the compact layout all end it,
    and a tab switched away from and back to shows every pane, as the
    proposal asked. `FocusDirection` and `MoveDirection` look for
    neighbours across the tab's whole layout, not the one pane drawn:
    focusing a neighbour ends the zoom, and a move that Go refuses keeps
    it. The zoom isn't saved in `state.json`, nor in the undo snapshots:
    it's meant for a moment, and a restart that showed one pane alone
    would hide the rest of the tab with no sign of why, so Bench always
    starts with every pane shown. The View names the zoomed pane
    (`display.zoomed`), which the frontend marks with the chip and the
    status bar's **One pane zoomed**, both buttons for `ShowAllPanes`,
    and counts the tab's panes, not the one drawn. The `zoom_pane`
    shortcut and **Pane ▸ Zoom Pane** call `ToggleZoom`; the menu bar
    state carries `Zoomed` (decision 3), so that the shell relabels the
    item **Show All Panes**. Like **Rearrange**, the menu item has no key
    equivalent, since the global shortcut works in Bench too, and shows the
    shortcut in its label instead (decision 46).
    `EvenSizes` gives a split's children equal sizes through the same
    path as `SetSizes` (`setSizesLocked`), with an undo step (decision 37)
    and a toast, so that a double-click on a divider can be taken back. A
    split that looks even already, to a twentieth of a percent, is left
    alone without a toast, so that a double-click there doesn't fill the
    undo history with nothing. The frontend listens for `dblclick` on a
    divider; its two presses don't drag it, since a divider drags only
    once the pointer moves. Return on a divider selected from the
    keyboard does the same, as the arrow keys move it. `EvenSizes` goes
    in the split's lane (`actLatest`), as `SetSizes` does, so a
    `SetSizes` that a click's slight move or an arrow key sent just
    before can't reach Go after it and undo it.
    A pane still can't be split by its only window (`errOnlyItem`), but
    **Split Right with an Empty Pane** and **Split Down with an Empty
    Pane** make room for another: `SplitPane` puts an empty pane on that
    side (`Model.SplitEmpty`), focuses it, and returns its ID, and the
    frontend opens the add dialog for it at once (`splitEmpty` in
    `palette.js`). The engine keeps the tab's tree, preset, and focus from
    before, and the tree from just after (`emptySplit`), so that
    `CancelSplit`, when the dialog closes without adding anything, puts
    the tab back exactly, sizes and all, if nothing else has changed it,
    and otherwise closes the pane up as **Close this pane** does. It
    leaves a pane alone that has something in it, or that a new window is
    on its way to from the dialog's **New window** or an app it opened
    (`awaitedLocked`), and any pane but the last one `SplitPane` made. The
    dialog may close before the View with the new pane arrives, so the
    add dialog's state marks the pane as new (`ui.add.split`) until a
    View shows it, rather than closing as it does for a pane that has
    gone. The dialog closes before it runs the row chosen, and an add is
    a call that takes time, so the frontend asks for `CancelSplit` in a
    microtask, after the calls the dialog made for the pane have answered,
    and Go then finds the item or the window it's waiting for. The window
    menu grew by three entries, so menus may now take 80% of Bench's
    window, not 70%, before they scroll.

41. **Launchers type their command into the user's own shell, and come
    first.** The add dialog's **New terminal…** and `[[launcher]]` entries
    run programs such as `claude` and `codex`, which npm and nvm often put
    on the `PATH` in `~/.zshrc`. A config `command` runs as `$SHELL -l -c`,
    which isn't interactive, so zsh doesn't read `~/.zshrc`, and such a
    program isn't found. So these terminals start the user's shell as
    Terminal does, as an interactive login shell, and type the command
    into it (`term.Options.Input`), as Terminal's "Run command" and
    iTerm2's "Send text at start" do: the command finds the same
    programs, aliases, and functions as in the user's own terminal, and
    the shell carries on at its prompt, in the folder, when the command
    ends. The session types it once the shell's line editor has the
    terminal, which it takes out of canonical mode (`ICANON`, read from
    the PTY's own side with `TIOCGETA` on macOS and `TCGETS` on Linux).
    If that hasn't happened in time, it types it anyway: after 15 seconds
    for zsh, bash, and fish, which always start a line editor, but whose
    start-up files can take seconds, loading nvm or conda, or rebuilding
    completions; and after a second for any other shell, such as dash,
    which may have none. Typed sooner, the shell's start-up files, or a
    question one asks, could read it, and it would show before the
    prompt. Config `command` entries keep `-l -c`, so existing files
    behave as before; the typed command is an item's `run`, a separate
    field, in `config.toml` and `state.json`. Busy (decision 35) needs
    nothing new: the typed program has the PTY's foreground while it runs.
    If the folder has gone, the shell starts in the home folder and says
    so, but doesn't type the command, which was written for its folder,
    as a `command` doesn't run there. The browser preview's pretend
    terminal (`term.Echo`) shows the command typed at its first prompt.
    `start_again`, true unless it's `false`, says whether the command
    runs again when Bench next opens. Terminals added with a command are
    in `state.json`, and start again with their tab, as config terminals
    always have, but a one-off script, such as a deploy, shouldn't run on
    every start. With it off, the command is typed without the Return,
    for the user to see and run. Only the session started as the item is
    added runs it at once (`Item.runNow`, never saved).
    A launcher is a `[[launcher]]` entry of its own, not an item, because
    each use adds a new terminal, where an item in two tabs is one session
    (decision 20). Launchers come first in `AddOptions`, because the
    dialog runs the first row that matches: a Claude desktop window would
    otherwise win when the user types `claude`. In Jump to…, they come
    after the tabs and windows, so that Return with nothing typed, or with
    a tab's name typed, jumps as it always has, and doesn't start a
    command. Typing finds a launcher by its name only, in both, not by the
    command and folder of its hint, which share words such as `code` with
    tabs and windows. An app launcher reuses what App items with a
    `url` do already: it opens its folder, as a `file://` link, or its
    link, in its app, and docks the window whose title has its `title`,
    or the folder's name. A window of the app that matches already is
    added as the add dialog adds that window; otherwise a new App item
    takes the window that comes, and goes again if the app can't open.
    **Save as launcher** calls `SaveLauncher`, which names a launcher
    without a name after its command's program, or its folder, and the
    form then adds the terminal under that name. **Settings ▸ Launchers**
    sends the whole list with `SaveSettings`, but only if it changed, so
    that a launcher added meanwhile, from the add dialog or the file,
    isn't dropped by a dialog opened before; it names a launcher without
    a name in the same way, or an app launcher after its app. A name
    taken, ignoring case, is refused with the name of the launcher that
    has it. Both save through `saveEdit`,
    so a file changed outside Bench isn't written over (decision 7).
    Settings edits an app launcher's fields but not its app, since Bench
    has no app picker yet: the file sets it. Older builds report the new
    table and fields as problems, and run on their last good config until
    they're upgraded (decision 7). **Choose…** shows macOS's folder picker
    with Wails' `OpenFile` dialog, as a sheet on Bench's window
    (`folderChooser` in `internal/ui/native_darwin.go`), rather than a
    free-standing panel: Bench is raised while a dialog is open, and a
    panel that took the keyboard would drop Bench's window, and the form,
    below the docked windows (`benchLevelNow`), while a sheet counts as
    Bench's own window. `PromptForSingleSelection` waits on the calling
    goroutine, so `ChooseFolder` holds no lock, and the shell shows one
    picker at a time. `ResolvesAliases` is set, since Wails turns it off.
    Global shortcuts still work while the sheet is open, and a tab switch
    closes the add dialog, so the frontend keeps Bench raised until the
    picker answers (`ui.choosing`), and drops a folder chosen for a form
    that has gone. The browser preview has no picker: its `ChooseFolder`
    hook is nil, and the engine refuses, saying to type the folder.
    Commands are plain text in `config.toml`, its backup, and
    `state.json`, so the user guide says to keep secrets out of them.
    **Browse…** finds what the dialog doesn't list. It closes the dialog
    first, so raised mode is off, and `BrowseToAdd` lowers Bench itself
    too, as the frontend's `SetOverlay(false)` may come after it. It then
    shows macOS's open panel as a free-standing window
    (`Platform.ChooseToAdd`, `beginWithCompletionHandler`), not a sheet:
    with Bench lowered, a sheet would go below the docked windows with
    Bench's window, and the panel, Bench's own frontmost window, stays in
    front of them. Bench can't end Wails' panel, so it shows its own. A
    second **Browse…** while it's open brings it to the front
    (`platform.ErrBusy`). The engine waits without its lock, and then
    checks the tab again, as the user can switch tabs meanwhile, and
    refuses to add an app to another tab's pane. An app chosen is read
    with `apps.ReadBundle`, registered with LaunchServices
    (`LSRegisterURL`) so that `OpenApp` finds it by bundle ID wherever it
    is, and noted until Bench quits, so that its name and its **Apps** row
    survive the next scan. A program goes to the **New terminal…** form
    rather than straight into a pane, so the user can check it, add
    arguments, and save it as a launcher. Its command is its bare name
    when the user's shell finds that very file by it
    (`term.Host.LookPath`, which runs `$SHELL -l -i -c 'command -v NAME'`
    for a plain name, with a 3-second limit, in a session of its own, so
    that it never takes the terminal Bench was started from, and what it
    starts ends with it), because a path into nvm's
    or Homebrew's folders breaks when they update, and otherwise its path,
    quoted for the shell, with `~` outside the quotes. A script that can't
    run itself runs with what its `#!` line names, or what its extension
    suggests, such as `python3` for `.py`. The browser preview's platform
    refuses (`ErrUnsupported`), and the dialog says so. The scan of
    installed apps also looks one level inside `/Applications`, for
    folders such as Setapp's, which reads a few more folders, not bundles,
    so start-up isn't slower.
42. **Terminal essentials live in the terminal, not the menu bar.** Find
    (`@xterm/addon-search`), plain-address links
    (`@xterm/addon-web-links`), text size, **Clear**, **Select All**, and a
    right-click menu with **Copy** and **Paste** are the frontend's: each
    terminal view, in Bench's window or its own, handles ⌘F, ⌘G, ⇧⌘G, ⌘+,
    ⌘−, ⌘0, ⌘K, and ⌘A itself, with xterm.js's custom key handler, which
    keeps them from the program and from the app menu. They aren't app
    menu items with key equivalents, since those would take the keys
    wherever Bench has the keyboard, from Web items' pages and Bench's own
    fields too. The find bar is the frontend's own element inside the
    terminal's, which a render moves with it, and it marks every match
    with the add-on's decorations (xterm.js's proposed API), which also
    give the count. The text size and the Option key are settings,
    `terminal_font_size` and `terminal_option_as_meta`, sent in the View,
    so every terminal, popped out too, follows **Settings** at once; ⌘+
    and ⌘− change one view's size only until **Settings** changes it, or
    the view goes. A new size changes the terminal's rows and columns, so
    the view fits again and tells Go (`TermResize`). Copy and Paste use
    the Wails runtime's clipboard, as reading through the web view's own
    clipboard API shows macOS's Paste prompt in WKWebView; the browser
    preview uses the browser's. **Clear** clears the view, and has Go
    forget the session's kept output too (`TermClear`), so that the
    output cleared doesn't come back when a view draws the terminal again,
    as when it's popped out, nor after a restart, in `terms/`. The
    session's offsets carry on, and the modes that the output cleared set
    still count, as for output dropped from the start of the 256 KB.
43. **A window that needs the user gets a bell, and it's Go's to say
    when.** A terminal's session reports the bell (BEL) and the
    notifications that programs ask for with OSC 9 and OSC 777
    (`term.Callbacks.Alert`), as Claude Code and Codex, set up to, send
    them when they finish or need an answer; a config `command` that ends
    counts too, and so does a Web item's notification
    (`WebNotification`). The engine marks the item (`Item.attention`,
    saying why) only when the user can't see it (`seenLocked`): Bench is
    hidden, or the item isn't what a displayed pane of the current tab
    shows, or the active app is neither Bench (`benchActive`, from
    `AppActivated`; the browser preview, whose platform doesn't say,
    counts as active) nor an app with a window in the current tab
    (`activeApp`), as when the user types in Zed beside the terminal,
    which is in view. A popped-out item counts as seen only while Bench is
    active. It has no sub-tab to mark, so it's never marked, and one that
    leaves its tabs loses its mark. `clearSeenLocked` takes the mark off
    whatever the user can see, at every arrange and every View sent, and
    as Bench, or an app with a window in the current tab, becomes active,
    so any way of showing an item clears it, without each path having to.
    The View has the mark on the item and a count on its tab, which the
    frontend draws as a blue bell before the sub-tab's title, where its ×
    never covers it, even at the sub-tab's narrowest, which is wider by
    the bell (decision 61), and after the tab's name, with why in the tooltip
    and "needs you" in the accessible name: not another amber marker on
    the monogram, whose shapes say why a window can't be shown, and
    "needs you" rather than the status bar's "need attention", which
    counts those. The Dock icon's
    badge counts the marked items (`Platform.SetBadge`), set only when the
    count changes, after the lock is released. With `notify_terminals` on,
    a terminal also posts a notification (`Platform.Notify`, with its tab
    as the `Subtitle`): at most one every 5 seconds per terminal
    (`notifyGap`), and for the bell only when it newly marks the
    terminal, as programs ring it in bursts. Web items post theirs as
    before, whatever the setting, since the page asked. Clicking any
    notification sends `NotificationClicked`, which shows its item
    wherever it is (`showAnywhereLocked`): on its tab, bringing Bench
    forward if it's hidden, or its own window if it's popped out.
44. **A terminal keeps its folder and its output across a restart, apart
    from its config entry.** A session reports the folder its shell moves
    to (`term.Callbacks.Folder`): the shell process's current folder, or
    else the last one it reported with OSC 7. The engine keeps it as the
    item's `LastFolder`, which `state.json` saves as `lastFolder` for a
    plain shell only, and the View sends as `here`. `Folder` stays the
    config entry's key, so moving around never makes the entry look
    edited or removed at the next start. A plain shell starts again in
    `LastFolder`, with `Folder` as `Options.Fallback` if that has gone,
    and then the home folder; a terminal with a `command` or `run` starts
    in `Folder`, as its command expects. The add dialog's first **Built
    in** row and a terminal's menu open a terminal in `here`, and **New
    terminal in…** asks with the folder picker (`ChooseFolder`). With
    `keep_terminal_output` on, `Close` writes each running terminal's
    output, at most 256 KB from its end, through `term.Restorable`, to
    `terms/<id>.out` (0600, written to a temporary file and renamed, with
    the file's time as when it was written), before `CloseAll` ends the
    sessions, and only when it saves `state.json`, since otherwise the
    items `state.json` has aren't the model's. It keeps the file of a
    terminal that `state.json` brought back and that hasn't started yet,
    and removes the rest. Only an item that `state.json` brought back
    shows a file (`Item.savedOutput`), once, so a new terminal that gets a
    reused ID never shows someone else's output. It starts with it as
    `Options.Preface`: grey, made safe again in case the file changed,
    with "Restored output from 14:02. The program that wrote it has
    ended." after it, so the user can't take it for a live program.
    `Restorable` keeps text and the sequences that colour it, move the
    cursor, and erase, and drops queries, whose answers would reach the
    new shell as if typed, mode changes, titles, the clipboard, the bell,
    and the alternate screen. Closing the terminal, or its entry leaving
    `config.toml`, removes its file, and turning the setting off removes
    the folder at once. Output can hold secrets, which the setting's help
    and the user guide say.
45. **Settings ▸ Keyboard changes the shortcuts while Bench runs, and Bench
    explains the clashes that macOS doesn't report.** Wails reports only
    Carbon's `eventHotKeyExistsErr`: `RegisterEventHotKey` lets Bench take a
    shortcut that switches input sources, one that VoiceOver uses, or one
    that macOS keeps, such as ⌘Space, without saying so, and the key press
    may then never reach Bench. So `internal/mac` (`keys_darwin.m`,
    `hotkeys.go`) reads the input sources' symbolic hot keys, 60 (Select the
    previous input source) and 61 (Select next source in Input menu), from
    `com.apple.symbolichotkeys`, with macOS's defaults, ⌃Space and ⌃⌥Space,
    for one that's missing, and counts them only while the user has two
    input sources or more to switch between. It watches `NSWorkspace`'s
    `voiceOverEnabled` with KVO, and the enabled input sources with their
    distributed notification, and reports `SystemKeysChanged`
    (`Platform.SystemKeys`). The shell (`internal/ui/shortcuts.go`) works
    out each field's state as it registers the shortcuts: a problem with the
    field, a shortcut that another app has, one that switches input sources,
    one with ⌃⌥ while VoiceOver is on (its VO keys, which Caps Lock adds to
    rather than replaces), one that `shortcuts.Reserved` lists, or one that
    every Mac app uses for a command of its own, such as ⌘Z for Undo, or
    ⌘, and ⌘T, which open Settings and New tab in Bench's window too
    (`shortcuts.AppKey`, decision 64), which Bench would take from every
    app, or one that a window tool that's running uses too, such as
    Rectangle (`WindowToolKeys`, decision 69), in that order. For each, it suggests a shortcut that it checked
    it can register, that clashes with nothing it knows of, and that
    leaves the other fields' problems no worse, field by field, so that
    taking them all at once makes no new clash. For VoiceOver, ⌘ takes
    ⌥'s place (`shortcuts.WithoutVoiceOverKeys`), or, where that gives
    shortcuts that clash or that macOS keeps, ⌃'s place
    (`shortcuts.WithoutVoiceOverControl`). It reports them to the engine
    (`ReportShortcuts`), which keeps them for the View's `shortcuts` and
    shows the notices, each with **Use this instead**, which calls
    `UseSuggestedShortcuts` with its fields, and **Keyboard settings**,
    which opens Settings at its **Keyboard** section. **Settings ▸
    Keyboard** checks a change as it's made (`CheckShortcuts`), without
    saving it: the shell tries each binding it doesn't hold already, and
    releases it again. While a field records keys, the shell releases all of
    Bench's shortcuts (`PauseShortcuts`), since a key press that a hot key
    takes never reaches the page, and registers them again when recording
    stops, Settings closes (`SetOverlay(false)`), or the page loads again
    (`Ready`). A command that opens another dialog or the overview in
    Settings' place, as from the menu bar, stops recording first
    (`afterAsking`): the overlay stays open, so `SetOverlay(false)` doesn't
    come, and the draft that knew about the recording goes. **Save**
    writes `[shortcuts]`, and the shell registers them again (decision 1).
    It first refuses a field changed to a shortcut that another field
    makes (`keysClash`), with the field's own line, since Go's refusal
    names whichever of the two comes later in `config.toml`, which may be
    the one the user didn't change. The browser preview runs the same
    code, with a registrar that accepts every shortcut, and `NewDemo`
    reports no system keys, until the demo bar's **VoiceOver** pretends
    that VoiceOver is on (`SetDemoVoiceOver`, decision 64). Which side gets
    a key press that both Bench and VoiceOver, or an input source, want is
    for a Mac to tell, so the notices say Bench "may not" see it.
46. **The app menu has Tab, Pane, and Help menus, and shows Bench's
    shortcuts in its items' titles.** The app menu (`appMenu` in
    `internal/ui/menu.go`) is Bench, Edit, View, Tab, Pane, Window, and
    Help. **View** has **Layout ▸ Auto, Wide, Compact** (decision 31),
    **Rearrange**, and **Fill the Screen**; **Tab** has what acts on the
    current tab, and the tabs to switch to; **Pane** has what acts on the
    focused pane and the window it shows. The engine tells the shell what
    they need in `MenuBar` (decision 3), which it works out from the View it
    has just built (`menuBarLocked`), so that the menu agrees with what the
    frontend draws: the shortcuts in display form (`Keys`), the tab that
    **Reopen Closed Tab** brings back (`Closed`), the page or terminal that
    **Reopen Closed Page or Terminal** opens again (`ClosedItem` and
    `ClosedTerminal`), the latest change that **Undo** names (`Undo`,
    decision 65), the tab that **Last Used
    Tab** goes back to (`LastTab`, decision 49), and the focused pane
    (`Pane`), with the item it shows, whether that has a window and is
    docked, and whether the pane can be split, zoomed, or closed. Each item
    does what the same item does elsewhere: an engine method (`act`, which
    shows its error as a toast, with the `MenuBar` of the moment it was
    chosen), or a `bench:command` for what the frontend does, such as
    `rename-tab`, `add-to-pane`, and `close-window`.
    - **Shortcuts in titles.** Bench's global shortcuts are Control+Option
      ones. As key equivalents, they would also run while Settings ▸
      Keyboard has released the global shortcuts to record one (decision
      45), and would take those keys from web pages, so an item shows its
      shortcut in its title instead, as Bench's tooltips do: "Next Tab
      (⌃⌥])" (`withKeys`). Only the standard macOS items, such as Hide,
      Quit, the Edit menu's, and Minimise, have key equivalents, so that
      Command shortcuts reach the page that has the keyboard, as the spec
      asks.
    - **Changing the menu.** `Menu.Update` builds the whole native menu
      again, and waits for the main thread, so the shell does that only
      when the number of tabs changes, and otherwise changes the items in
      place (`SetLabel`, `SetEnabled`, `SetChecked`), each only if it
      differs. It changes nothing until the app runs (`start`, on
      `ApplicationStarted`), since Wails makes the native menu from it
      then. It holds its own lock meanwhile, which the main thread never
      takes. Wails ticks a radio item as it's chosen, so a layout or tab
      that the engine refuses is ticked again as the engine says
      (`retick`).
    - **Help.** **Bench Help** and **Troubleshooting** open the user guide
      on GitHub in the default browser. **Keyboard Shortcuts** opens the
      **Keyboard Shortcuts** sheet, which lists them (decision 48). **Show
      Welcome Guide** sends `first-run`, **Recent Messages** opens the
      notices' recent messages (decision 47), and **Show Bench's Files in
      Finder** calls `ShowConfigInFinder`. The menu is titled Help, for
      macOS to add its search field, which finds the menus' items; Wails
      doesn't name it as the app's Help menu, so that's for a Mac to
      confirm.
    - **Report a Problem…** opens a new GitHub issue in the default
      browser, with the body filled in (`problemReport` in
      `internal/ui/report.go`): headings for what happened and what was
      expected, then Bench's version, the macOS version, the displays,
      the layout in use, the permissions, and up to 8 notices of up to 300
      characters each, from the engine (`ProblemReport`, which the shell
      calls; it isn't bound), how long the latest tab switches and other
      arrangements took, for up to 12 reasons each (decision 67), and where
      the log is, written from `~`. The body asks the user to check it.
      Bench sends nothing itself: the user reads the issue, changes it, and
      submits it, or doesn't.
47. **Bench keeps a log file, and the frontend keeps the recent
    messages.** Bench opened from the Finder or at login has no terminal, so
    its standard error goes nowhere a user can find. So the app also writes
    its log to `~/Library/Logs/Bench/bench.log` (`logFile` in
    `internal/ui/logfile.go`): `cmd/bench` names the file, and `keepLog`
    tees `logOutput` into it, so Wails' warnings, Bench's own (`warn`, now
    through `slog`, with a time), and the engine's go to both. The engine
    always has a logger now, at the warning level, or the debug level with
    `BENCH_DEBUG`, and the emitter logs each notice as it first appears, so
    that the log says when a problem began. Each launch starts with a line
    that has Bench's version, written in `run`, which a second Bench never
    reaches: Wails ends it in `application.New`, once it has brought the
    first forward. The file takes Wails' messages from before that, but is
    only added to until `run` holds it (`logFile.hold`), so that a second
    Bench neither says it started nor moves the file aside from under the
    first. The file is the user's alone (0600, in a 0700 folder, set again
    at each launch), since it names tabs and apps. It's moved aside as
    `bench.log.1` once it would pass 1 MiB, so the two hold at most 2 MiB; a
    write that fails is dropped, so the log still reaches standard error.
    Console lists it under **Log Reports**. The toasts and notices, though,
    are the frontend's: it keeps the last 20 (`noteMessage` in `state.js`),
    with when each came, counting one that comes again straight after, and
    lists them in the notices menu, under **Recent Messages**, each with
    **Copy**. A notice is listed there once it's gone, as the **Notices**
    above list it while it lasts. They live only as long as the page, which
    is enough to read or copy what went by; the log file is what lasts. The
    status bar shows a button for them while there are no notices, and
    **Help ▸ Recent Messages** and the jump palette open them with the
    status bar hidden.
48. **The welcome guide sets Bench up, and Bench helps with what macOS
    asks of it.** The guide (`guide.js`) has pages: the permissions, with
    Open at login; the first time Bench opens, **Choose a starting
    set-up**; and four tips, with the shortcuts as configured. **Help ▸
    Show Welcome Guide** and the jump palette open it again.
    - **Starting set-up.** The first config holds one tab with the
      browser, since a Mac may not have any other app (decision 28). The
      set-up suggests more, built only from apps that are installed
      (`starterTabs` in `internal/workspace/starter.go`): Comms, Code, and
      Plan, each pane taking the first of a few apps that's installed and
      that no tab has already, and Code a terminal beside its editor. A tab
      with nothing but a terminal, or whose name a tab has, isn't suggested,
      so the set-up is left out once there's nothing to suggest. The user
      removes any they don't want; `UseStarterTabs` works the tabs out again
      and adds the ones named, refusing a name it wouldn't suggest now, and
      saves `config.toml` as Tab properties saves a new tab. The set-up is
      the first run's only: from Help, the guide is the permissions and the
      tips. Once the user has added its tabs, or none, Back and Continue
      skip it, but the step count still counts it (`setupDone`), so that it
      doesn't count back. Its second button is **Start with one tab** with
      the first config's one tab, and **Don't add tabs** with more, as in a
      `config.toml` copied from another Mac, since it removes none; the
      guide notes which as it shows the set-up, so that the tabs it adds
      don't change it. Its first block is a tab of the windows that are
      open, while any aren't in a tab (decision 54): **Choose windows…**
      opens the **New tab** dialog over the guide, and its Cancel and
      **Make tab** come back to the set-up, which names the tabs made and
      asks for its suggestions again. The set-up shows while there are
      windows to make a tab of, even with nothing to suggest.
    - **Taking the user to the switch.** macOS shows its permission
      prompt once; after that, asking does nothing visible. So
      `RequestAccessibility` and `RequestScreenRecording` ask, and while
      Bench doesn't have the permission, open System Settings at its list
      too (`Platform.OpenSettings`, with the `x-apple.systempreferences`
      addresses in `internal/mac/settings.go`). Login Items has no address
      that every version keeps, so the Mac opens it with `SMAppService`.
    - **After an update.** Releases aren't notarised yet, so macOS takes
      each new version for a new app: System Settings still shows Bench
      turned on, but macOS doesn't apply it, and the usual text ("Turn on
      Bench") doesn't help. The engine records the version that last had
      the permission in `accessibility-version`, and while a version that
      isn't older than that starts without it (`updatedLocked`; a build
      that isn't a release counts if its text differs), the notice, the
      banner, and the guide say "Bench was updated, so macOS needs you to
      allow it again: select Bench, click −, then turn it on again." Bench
      doesn't run `tccutil reset` for the user, which would clear every
      app's permission of that kind without asking.
    - **Quit and Reopen Bench.** macOS applies Screen Recording only as
      Bench opens. Once Bench has asked for it, the View's
      `permissions.screenRecordingReopen` offers **Quit and Reopen Bench**
      until Bench quits. `QuitAndReopen` quits as `Quit` does, asking
      first about terminals that are running something (decision 35),
      and the shell (`Emitter.Relaunch`) first starts a small `/bin/sh`
      script, in a process group of its own, that waits for Bench's
      process to end, up to a minute, and then opens the app bundle with
      `open`, or runs the binary again outside one (`reopenAfterQuit` in
      `internal/ui/relaunch.go`). Opening Bench again straight away
      wouldn't work, because Wails' single-instance lock hands the new
      process over to the old one, which is quitting. If the script can't
      start, Bench doesn't quit, and says so.
    - **Login item approval.** Wails' `Autostart` reports a login item
      that macOS needs the user to allow as not enabled, without saying
      why, so the Mac asks `SMAppService` for its status
      (`Platform.LoginItemNeedsApproval`). While macOS needs that,
      `SetOpenAtLogin` opens Login Items and says so, the View's
      `loginApproval` puts a note and **Open Login Items** in Settings and
      the guide, and the engine reads the login item again each time
      Bench comes forward, as the user may have allowed it meanwhile.
    - **New versions.** Bench makes no network requests of its own to
      look for new versions unless the user turns `check_for_updates` on,
      or chooses **Bench ▸ Check for Updates…**. A `Bench.app` built with
      an update key (`make app SPARKLE_PUBLIC_KEY=…`, as releases are)
      has Sparkle 2, which installs new versions. `internal/mac`'s
      `Updater` loads `Sparkle.framework` at run time with `NSBundle`,
      without linking it, so a build without it runs as before. It's the
      engine's `Options.Updater`, and the shell starts it once the app
      has started. The setting turns Sparkle's daily look on and off
      (`Updater.SetAutomatic`); `Info.plist`'s `SUEnableAutomaticChecks`
      is false and the delegate says no, so Sparkle never asks the user
      itself. Sparkle reads the appcast that the release workflow adds to
      each release (`build/appcast`), signed, as each update is, with
      the update key (EdDSA): `SURequireSignedFeed` and
      `SUVerifyUpdateBeforeExtraction` make it refuse anything else. The
      delegate takes Sparkle's gentle reminders: a newer version that a
      daily look finds goes to the engine (`FoundUpdate`), which shows a
      notice with **Install…** (`InstallUpdate`), which shows Sparkle's
      window for it. The notice has no **Dismiss**: Sparkle doesn't look
      again until the user answers in its window, with **Install
      Update**, **Skip This Version**, or **Remind Me Later**, and so
      doesn't hide a version that the user dismissed in a build without
      it. The notice goes once Sparkle is done with the version
      (`UpdateFinished`). `Info.plist`'s `SUAllowsAutomaticUpdates` is
      false, so Sparkle's window doesn't offer to install updates without
      asking, which would leave out the notice. A press of **Check for
      Updates…** or **Install…** while Sparkle is busy, as it is for a
      few seconds after it starts, waits for it. **Check for Updates…** has Sparkle look at
      once and show what it finds (`Updater.Check`). The View's
      `installsUpdates` has Settings say that Bench installs new versions. Sparkle installs as
      Bench quits, through Bench's usual quit, which asks about busy
      terminals first, and opens the new version. Without Sparkle, at
      most once a day, `update.Latest` asks GitHub's API for the latest
      release (`Options.LatestRelease`, which the browser preview leaves
      out), and if it's newer than Bench, a notice says so, with
      **Download**, which opens the release's page, and **Dismiss**
      (`DismissUpdate`). After **Dismiss**, Bench doesn't announce that
      version again. A look that fails counts, and is only logged.
      `state.json`'s `updates` keeps when Bench last looked and what it
      or Sparkle found, so that a restart doesn't ask again. Without
      Sparkle, **Bench ▸ Check for Updates…** opens the releases page in
      the browser whatever the setting, so it needs no network access of
      Bench's own.
    - **Keyboard Shortcuts.** The sheet (`shortcuts.js`) is built from
      the View's `shortcuts`, the list that **Settings ▸ Keyboard** shows,
      in the same words, so the two can't disagree, then the terminal's
      keys, which aren't configured. **Help ▸ Keyboard Shortcuts**, the
      jump palette, and the status bar's key hints, now a button, open
      it, and its **Change Shortcuts…** opens Settings ▸ Keyboard. It
      moves but doesn't resize, as there's nothing in it to grow. An
      empty pane says how to bring a window in, with the move shortcut as
      configured.
49. **Jump to… finds words in any order, lists what was used last first,
    and offers the focused window's menu.** The palette's rows are built
    from the View (`paletteItems` in `frontend/js/palette.js`), and
    `filterItems` keeps the rows that have every typed word, in any order,
    in their label or hint, folded to lower case without accents. A word
    with no letter or digit, such as ".", is left out, unless the query has
    nothing else, as it's found almost anywhere. A window's whole title
    (`labelOf`, with its app's name) counts as its label, so "zed spec"
    finds Zed's spec.md. Each word scores 6 at the start of the label, 5 at
    the start of a word in it, 3 inside it, 2 at the start of a word in the
    hint, and 1 inside the hint; words typed side by side as the label has
    them add 3, so "move tab" finds **Move Tab Left** before **Move to Tab ▸
    Comms**. Groups stay together, the group with the best row first, and
    ties keep the order the rows were built in, which is the order with
    nothing typed. Only when no row has every word does a word also match a
    word that starts with the same letter and has its letters in order
    (`lettersScore`), for typing slips such as "termnal". The add dialog
    keeps its rows whose label or hint has the query as typed, whole
    (`addFilter`), as text that matches nothing there is offered as a
    command to run: word by word, "code ." would find another tab's VS Code
    window with a "." in its title, and Return would add it. Launchers are
    matched by name only (decision 41).
    - **Last used tab.** The engine keeps the tab that was current before
      this one (`Engine.lastTab`), set in `switchLocked` whenever the
      current tab changes, and by an `Undo` that shows another tab, which
      sets the current tab before it switches. It holds the `*Tab`, as
      `Undo` and the closed tabs do, so that moving tabs, which reorders the
      model's tabs, and reloading `config.toml` follow it; once that tab is
      deleted or closed, `IndexOf` doesn't find it and there's none.
      `LastTab()` switches to it, and the switch makes the tab it left the
      last one, so pressing it again comes back. The View's `lastTab` and
      `MenuBar.LastTab` name it, or -1. It isn't saved: after a restart,
      there's no tab to go back to until the user switches. The `last_tab`
      shortcut, ⌃⌥&#96; by default, runs it as the other tab shortcuts do,
      bringing a hidden Bench forward unless `show_on_tab_shortcut` is off
      (decision 16), and so does **Tab ▸ Last Used Tab**, which is disabled
      while there's none. ⌃⌥&#96; is the key left of 1, beside the tabs'
      shortcuts, and macOS doesn't keep it. Its field comes after
      `previous_tab` in the table and Settings, but after the older fields
      in `Bindings`, so that a config written before it existed, whose
      other fields may hold ⌃⌥&#96; (which `Suggest` offers for
      `next_window`), keeps their shortcuts. `Parse` turns `last_tab` off
      in a file that leaves it out when its default clashes so
      (`offClashingNewDefaults`): otherwise a notice would report a clash
      the user didn't make, and the next Settings save would write it.
    - **Order.** With nothing typed, the Tabs group starts with the last
      used tab, marked **last used** and showing the `last_tab` shortcut,
      so that ⌃⌥Space then Return goes back as ⌃⌥&#96; does; the others keep
      the tab strip's order, which their shortcuts number. The Windows
      group is ordered by the items' `used` (`Item.Used`, from
      `Model.Use`), the latest first, which showing an item, focusing its
      pane, and switching to its tab update. It isn't saved either.
    - **This window.** The group is what the ⋯ menu has for the focused
      pane's shown item, built by the same `windowMenuEntries`, so that
      the palette can't offer less or other than the menu. Entries in a
      submenu, or with `under`, are named with it, as "Move to Pane ▸ Pane
      with Slack" and "Move to Tab ▸ Research" (decisions 63 and 71).
      Disabled entries are left out, and so is Zoom Pane (`pane`), which
      acts on the pane and is in Actions, where an empty pane has it too.
      The group ends with **Window Options…**, which opens the menu itself
      (decision 33), as the web app entries join it only once Go has said
      whether the browser can make one. Entries that the menu gains, such
      as a page's controls, join the group the same way. Actions keep what
      isn't the window's: the pane's add dialog (**Add to Pane…**), **Close
      Pane** for an empty pane, as **Pane ▸ Close Pane** has it
      (`panesCanClose`), **Close Tab…**, which asks first as the tab's menu
      does, **Delete Tab (Keep Windows)**, **Tab Properties…** for each
      tab, and the rest.

50. **A web page's controls are in its menus, never on ⌘ keys, and Go
    follows what the page does with the camera and microphone.** A Web
    item has no toolbar, and pages such as Slack's replace WebKit's own
    right-click menu, so the only ways to reload a stuck page or go back
    were a swipe, or **Open in Browser**. Its window menu now has a **Page**
    submenu (`pageEntries` in `pages.js`), which the jump palette's **This
    window** lists as **Page ▸** *entry* and the app menu as **Pane ▸
    Page**, with no key equivalents: ⌘R, ⌘F, ⌘L, ⌘+, and ⌘− stay the
    page's, as decision 46 keeps the app menu off them.
    - **A submenu.** Back, Forward, Zoom In, Zoom Out, Actual Size, which
      says the zoom ("Actual Size (now 125%)"), Find in Page…, Copy Page
      Link, the microphone and camera entries, and Stop Sound and Video
      are the **Page** submenu, with the names, the order, and the
      separators of **Pane ▸ Page**. They were a row of buttons, labelled
      Page, as in a browser's menu, with entries after it: a web page's
      menu already had about 20 entries, and as one entry each the group
      took a third of a 900-point window, pushing the web app entry
      (decision 22) out of sight. A native menu has no rows of buttons,
      and a submenu takes one line, so decision 71 made the group a
      submenu, in the HTML menus too. **Reload Page** is the first of the
      menu's common actions, at its top (decision 63). Menus may take the
      window's height less 16 points before they scroll, not 80% of it,
      and `fitMenu` leaves room for a web app entry of two lines, as its
      names often wrap.
    - **Navigation.** The web view observes its `canGoBack` and
      `canGoForward` with key-value observing and reports them with its
      capture states in one `WebStateChanged` event, which the event queue
      coalesces by item, so a burst of changes costs one. Go keeps it on
      the item (`Item.web`, not saved), and refuses **Back** and
      **Forward** where it says the page can't go; WebKit would do nothing
      anyway. `GoBack`, `GoForward`, and swipes go through the same
      navigation policy (decision 24), as going back or forward is the
      item's own. `ReloadPage` reloads the page in view, or loads the
      item's own address if none has come into view, as when its first
      load failed.
    - **Zoom.** `WKWebView.pageZoom` (macOS 11) scales the page's layout,
      as a browser's zoom does, not the view. Zoom steps through browsers'
      levels, from 50% to 300%, and the item keeps its zoom in `state.json`
      (`zoom`, left out at actual size). `WebView.Zoom` gives a view its
      zoom as it's made, after a restart, a pop-out, or VS Code for the Web
      loading its server again, and `ZoomWebView` changes it at once. A
      file's zoom that isn't one of the levels is taken as the nearest.
    - **Find in page.** WebKit's `findString:withConfiguration:` selects
      the match and scrolls to it, but has no find bar, and Bench's UI is
      under the web view, so the frontend draws the bar at the top of the
      pane's body and Go moves the web view down by `layout.FindBarHeight`
      while it's open (`webFrame`, and `--find-bar` in `styles.css`). A
      popped-out page has no bar. Opening it gives Bench's UI the keyboard
      (`Platform.FocusUI`), and closing it gives it back to the page.
      WebKit finds from the end of the selection, without
      `StartInSelection`, so finding as each letter is typed would skip the
      match the letters before found: the bar finds on Return, Shift-Return,
      and its arrows. `FindInPage` waits for WebKit's answer without the
      engine's lock, numbering each request (`mac.finds`), for up to 3
      seconds, so a page that doesn't answer can't hold Bench up.
    - **Copy Page Link.** `PageLink` gives the address that **Open in
      Browser** would open (decision 23), and the frontend puts it on the
      clipboard through the Wails runtime. VS Code for the Web's address
      holds the server's connection token, so it has none.
    - **Camera and microphone.** `cameraCaptureState` and
      `microphoneCaptureState` (macOS 12) say whether a page uses them,
      live or muted. The sub-tab shows a mark of each before its title,
      where the sub-tab's × never covers it, even at its narrowest, which
      is wider by the marks (decision 61), and apart from the marker of
      the item's status on its monogram, green while live and grey and
      struck through while muted, and the tab one mark after its name, with
      counts and each device's liveness in `TabView`, so that the mark and
      its words name the device that's live. A call can run in a popup,
      as a meeting opened with `window.open` does, so the item reports its
      own web view's state and its popups' together: live if any is, else
      muted if any is. **Mute Microphone** and **Turn Off Camera** set the
      state to muted (`setMicrophoneCaptureState:` and
      `setCameraCaptureState:`) on each of those web views that uses the
      device, which the page can't undo, and **Unmute Microphone** and
      **Turn On Camera** set it back; Go refuses them for a device the
      page isn't using, and the native side checks again. A web view's
      state resets as it closes.
    - **Sound.** WebKit has no public property to observe that a page
      plays sound, and a script on media elements would miss Web Audio and
      pages that build their own players, so there's no speaker mark, and
      **Stop Sound and Video**, which pauses the page's and its popups'
      media (`pauseAllMediaPlaybackWithCompletionHandler:`), is always
      offered.

51. **Tiles show apps' and pages' icons, which Bench serves itself.** Every
    sub-tab, and every row of Jump to…, the add dialog, Settings ▸
    Launchers, an empty pane's list, and the welcome guide's starter tabs,
    named its app with a two-letter monogram, so three Brave windows were
    three "Br"s, and Slack's "# general" and Outlook's "Inbox", both Web
    items, said nothing of what they were.
    - **Addresses, not pictures, in the View.** Go resends the whole View
      on every change, so it holds only an icon's address (`icon` on
      `ItemView`, `RememberedApp`, `LauncherView`, `AddOption`, and
      `StarterApp`; `workspace/icons.go`): `/icons/app/<bundle ID>.png`,
      and `/icons/page/<item ID>.png?v=<n>`, whose number changes with
      each new icon, so the frontend loads each icon once. `ui`'s
      `iconHandler`, before the frontend's files in the asset server, asks
      the platform for the PNG (`Platform.AppIcon` and `PageIcon`), and
      answers 404 for none. A terminal has no icon, nor has an app that
      macOS says isn't installed, nor anything in the browser preview,
      whose platform has none, so that no tile tries in vain.
    - **Apps.** `mac.Platform.AppIcon` asks NSWorkspace for the icon of the
      app's bundle (`iconForFile:`), draws it 40 pixels square, which is
      20 points at 2x, the tile's size, and keeps the PNG by bundle ID.
      NSImage drawing is safe off the main thread, and it's done on a
      serial queue of its own, from the asset server's goroutine, so Go
      never waits for the main thread, and the UI never waits for an icon.
    - **Pages.** When a Web item's page has loaded, `web_darwin.m` runs a
      script in WebKit's client world, which the page's scripts can't
      reach, that lists its `<link>` elements' rel, sizes, type, and
      resolved address, at most 64. `icons.Candidates` chooses at most
      three to try: the smallest icon at least 40 pixels wide, then
      apple-touch-icons, then icons that don't say their size, then smaller
      ones, and `/favicon.ico` of the page's origin last. `icons.Pages`
      fetches them in Go, in the background, without cookies or the page's
      session, at most 256 KB each, in 10 seconds, through at most five
      redirects, each to http or https, and decodes PNG, GIF, JPEG, and
      ICO itself (`icons.Fit`), refusing anything over 1024 pixels before
      it decodes the pixels. What a page names comes from wherever the page
      likes, and Bench's own process isn't sandboxed as WebKit's content
      processes are, so its bytes are never handed to native decoders. An
      item keeps its icon while its pages offer the same candidates, so
      moving about a site fetches nothing again, and the platform reports
      a change with `PageIconChanged`, which the event queue coalesces by
      item. The icon goes with the web view, and the frontend never loads
      anything from the page's site.
    - **The tile.** `tiles.js` draws the icon over the monogram, which
      shows until the icon has loaded, and stays if it doesn't. It keeps the
      addresses that have loaded, so a tile drawn again, as any change to
      the View does, shows its icon at once from WebKit's cache. The icon
      is 20 points in every tile, so a sub-tab is no taller, and a status
      marker stays on its corner.
    - **Not** SVG icons, nor WebKit's own favicon loading: it has no public
      API for one on macOS. **Not** `data:` addresses in the View, nor the
      page's own address in an `<img>`, which would load it in Bench's UI
      with no limits, and tell the site when the user looks at Bench.

52. **Bench matches macOS unless the user chooses light or dark, and web
    pages always match macOS.** Bench's windows were dark only: Wails
    made each with the DarkAqua appearance and a fixed background. Web
    items' web views inherit their window's appearance, and WebKit tells
    pages `prefers-color-scheme` from it, so a site that follows the system
    was dark in Bench while it was light in Safari.
    - **Web pages.** Each Web item's web view, and each of its popups',
      has `NSApp.effectiveAppearance`, which is macOS's own, as Bench sets
      only its windows' appearance, never the app's. `web_darwin.m`
      observes it with key-value observing and gives every web view the
      new one. Pages then look as they do in Safari whatever the setting,
      as the apps in the panes, which are other apps, do anyway.
    - **The setting.** `appearance` is `system`, `light`, or `dark`, and
      `system` by default, as Apple's Human Interface Guidelines ask: users
      may think that an app which ignores the appearance they chose for
      the whole system is broken. Before, it was dark by default, so that
      Bench looked as it had. A `config.toml` that Bench wrote then says
      `dark`, so those users keep the look they had. A new user's file
      says `system`, and a file without the setting follows macOS too.
      Bench wrote such files until 30 October 2026, when the setting came.
      No release of Bench was made before then, so only builds from source
      wrote them, and Bench doesn't treat them as dark. The
      engine tells the shell with the first View and when it
      changes (`Emitter.Appearance`), and the shell gives each of Bench's
      windows, the main window, popped-out terminals, and Acknowledgements,
      its appearance (`Options.Appearance`, which is
      `mac.Platform.SetWindowAppearance`): none for `system`, so that the
      window follows macOS, and Aqua or DarkAqua for the others. It does so
      as each window gets its NSWindow and whenever the setting changes.
      Wails gives a window a fixed background as it makes it, so the native
      side replaces it with a dynamic `NSColor`, `--bg-window` of the
      appearance the window has then. Windows open with the setting's
      appearance and background already (`windowLook` in `shell.go`), so a
      light Bench doesn't open dark and change. For `system`, the shell
      asks `Options.SystemDark` (`mac.Platform.SystemDark`, which reads
      `AppleInterfaceStyle` from the user's defaults) whether macOS is
      dark: the main window is made before the Wails app runs, and until
      then Wails says light. Bench's panels, the drop guide and the live
      preview's card, take the setting too, as they next show: they draw
      with the light or dark tokens of the appearance they have. So does
      the add dialog's **Browse…** panel, a window of its own rather than a
      sheet on Bench's window, as it opens.
    - **Colours.** `styles.css` keeps the dark tokens in `:root`, and
      `light.css` has the light ones. Every colour that a rule wrote out is
      a token now, the terminals' ANSI colours and find's marks among them
      (`terminals.js` reads them). The light set was checked for contrast as
      the dark one was, and `appearance.test.js` checks both. A page links
      `light.css` with a media query, which is `(prefers-color-scheme:
      light)` as the page loads: WebKit takes that from the window's
      appearance, so a page starts in the right colours before any script
      runs. `theme.js` then follows the View's setting itself, through the
      style sheet's own media list, which takes effect at once, where a
      change to the link's `media` attribute waits for the browser to read
      the link again. The browser preview has no window for Go to set, and
      in the app a new View can come before the window's new appearance.
      It sets `data-theme` on `<html>`, and fires `bench:theme` when the
      colours change, which the terminals answer by taking the new tokens.
      Acknowledgements runs no scripts (decision 27), so its page keeps the
      media query, and follows its window.
    - **Increase contrast.** macOS's Increase contrast reaches pages as
      `prefers-contrast: more`, in every appearance. `styles.css` and
      `light.css` each have a small `@media (prefers-contrast: more)`
      layer, after their tokens: stronger lines, quiet text
      (`--text-muted`, `--text-faint`) at 7:1 at least on every surface it
      sits on, faint terminal text (`--term-dim`) just above 7:1 on the
      pane, so that it stays at most half as readable as plain text, and a
      focused pane with a ring of the accent outside its border, which
      keeps the pane's inside where Go docks its window. The focused pane
      sits above the others, so that a neighbour doesn't cover the ring
      when `gap` is 0. With no gap, the pane area's edges still clip the
      ring where the pane meets them, and the pane keeps only its 1 pt
      accent border there. The colours meet WCAG AA without it, so the
      layer stays small. `appearance.test.js` checks both sets with and
      without it, and terminals ask xterm.js for 7:1 rather than 4.5:1,
      and take the new tokens, as it changes (see Terminal output).
    - **Not** one style sheet with `light-dark()`, which WebKit has had
      only since Safari 17.5, while Bench runs on macOS 14.0. **Not** a copy
      of the light tokens under an attribute as well as under a media query:
      two copies would drift apart.

53. **Bench says when Stage Manager is on, rather than fighting it.** Stage
    Manager moves windows into its stages, and its strip at the side of the
    screen, whenever an app comes forward, so a docked window leaves its
    pane, and Bench's moves fight macOS's. There's no API to keep it away
    from some windows, or to ask whether it's on, so `internal/mac` reads
    `GloballyEnabled` from the `com.apple.WindowManager` preferences
    (`CFPreferencesAppSynchronize` first, as another process writes them)
    on the main thread, as Bench starts and each time it becomes active:
    the user turns Stage Manager on and off in Control Centre, which says
    nothing. It caches the answer (`Platform.StageManager`) and sends
    `StageManagerChanged` when it changes. While it's on, the engine shows
    a notice, with no button, as only the user can turn it off: "Stage
    Manager is on, so Bench can't keep windows in their panes. Turn it off
    in Control Centre." macOS 15's tiling needs no notice, as it moves a
    window only when the user asks: the user guide says to tile Bench's own
    window, whose panes follow it, and not a docked one.
54. **A tab can be made of the windows that are open, and its Undo puts
    them back where they were.** People open Bench with their windows open
    already, and making a tab of them one at a time, with + or by
    dragging, is slow. The **New tab** dialog has **Start with: An empty
    layout / Windows that are open** rather than a second dialog, because
    the two share the name, place, layout, and small-screen setting, and
    the title bar's + opens **New tab** with no choices of its own: an
    empty layout stays the default there. **Tab ▸ New Tab from Open
    Windows…** (`new-tab-from-windows`), Jump to…'s **New tab from open
    windows…**, and the first block of the welcome guide's set-up open the
    dialog at **Windows that are open**. The frontend lists the windows
    from the View (`fromwindows.js`): those not in a tab (`floating`),
    ticked, grouped by app, and, in a group that starts closed, unticked,
    the items of the tabs, each labelled with its tabs. It groups them as
    Go does (`appKey`: the bundle ID, or a page's or terminal's kind and
    name), and sends their IDs in its order. Go groups them again
    (`windowGroupsLocked`) and fills the panes in that order (`fillPanes`):
    each app's windows share a pane, as sub-tabs, and the apps after the
    last pane go there too, so nothing is left out, whatever the layout.
    The layout suggested follows only the number of apps (`suggestPreset`:
    one, two, three, or four or more panes), not the windows' frames:
    guessing a layout from where windows are would be wrong as often as
    right, and the preview (`PreviewTabFromWindows`) shows the result
    before anything moves. The name is the first app's, numbered as any
    new tab's is. An item of another tab shows in the new tab as well
    (`addFromTabLocked`), never moving from its tab, so making a tab can't
    empty another. `NewTabFromWindows` is one change, with one **Undo**:
    its step keeps `config.toml`'s tabs, as `MoveItemToNewTab`'s does,
    and, for each window that wasn't in a tab, how it was
    (`outsideWindow`): a window Bench didn't manage, with its frame, or a
    popped-out item, with its frame or whether it had a window of its
    own. Undo takes the tab away as it takes back any new tab, which
    leaves the windows Bench docked as popped-out items, and then puts
    each back (`putBackOutsideLocked`): a window Bench didn't manage
    stops being an item, so it's listed with its own `f` ID again, a
    page or terminal gets its window of its own again, and an App window
    goes back to its frame (`windows.Manager.MoveBack`, which moves it
    only if that frame is on a screen and the window isn't minimised or
    in full screen). A switch to another tab can hide the app of a
    window that Bench didn't manage, while the window is in the new tab.
    After Undo, if the app has no items with windows left, the next
    arrangement shows the app again (`Engine.putBack`), and the window is
    in sight where it was. The add dialog lets the user tick the windows'
    rows too, to add several at once (`AddWindowsToPane`), as people
    often want an app's windows together. With none ticked, a click on
    a row still adds just that row. While some are ticked, a click on a
    window's row ticks or unticks it, as in a checklist, so that a click
    never drops the ticks. Every row keeps a slot for the tick box, so
    that the tiles line up.

55. **A window that the user minimises stays in the Dock.** macOS reports
    Command-M, the yellow button, and **Window ▸ Minimize** as
    `WindowMinimised`, and bringing the window back from the Dock as
    `WindowRestored`. Arranging used to un-minimise every docked window
    that it moved, so a window came back as soon as the user switched tabs.
    Now the engine marks the window's item `minimised` (`minimise.go`), if
    it's an App item docked in a tab, and `dockable` leaves it out of the
    stacks, so the windows manager never moves or un-minimises it.
    `windows.Manager` minimises a window itself when it has nowhere to park
    it clear of other displays, and macOS reports that in the same way, so
    the engine asks the manager (`Manager.Minimised`), and ignores it.
    - **Hearing of it.** The events don't come in a fixed order: macOS
      gives the keyboard to the app's next window as the minimise starts,
      and reports the minimise once the window is in the Dock. So the
      engine marks the item at whichever of three comes first.
      `WindowMinimised`, for a window that's still minimised when the
      engine reads it again (the event can come after Bench has restored
      one it minimised itself). `WindowFocused` for another of the app's
      windows, when a pane of the current tab shows one of its windows
      that's now minimised (`noticeMinimisedLocked`, which reads only the
      panes' shown windows). Or arranging: `Manager.Apply` un-minimises a
      window in a stack only if the manager minimised it, or the stack's
      `Restore` names it, and reports any other as `StatusMinimised`,
      leaving it in the Dock. An app that opens a window already
      minimised, as apps restoring their windows after a login can, has
      its item marked as Bench adopts it.
    - **What the pane shows.** Marking the item shows the most recently
      used other window of its pane in its place, in each tab it's in, as
      macOS brings forward the window behind it. A pane with no other
      window keeps it active: its status is `minimised`, "Minimised", with
      a grey ⓘ marker, for information, as for full screen, and the pane's
      message has **Show again** (`ShowItem`) and **Remove from this tab**
      (`RemoveItemFromTab`, or `PopOut` for a window in no other tab,
      which leaves it in the Dock, as its toast says).
    - **Bringing it back.** What the user does to see the window clears
      the mark and sets the item's `restore`, for the next arrangement to
      put in its stack's `Restore` (`showAgain`): `ShowItem` (clicking the
      sub-tab, **Show again**, and Jump to…), **Dock again**, **Show side
      by side** and the other splits that move it into a pane of its own,
      adding it back with **+** after **Remove from this tab**, and
      making a tab of it in **New tab**'s **Windows that are open**, where
      it's the window that its pane shows (`fillTabLocked`).
      `WindowRestored` clears the mark too, and shows the item in its
      pane: in the current tab, it arranges, and in another, it shows it
      there, and macOS's focus is then followed as `follow_focus` says.
      `WindowFocused` for a marked window that's no longer minimised
      counts as its restore, as the focus can come first, and one for a
      marked window that's still minimised, which can only be from before
      the minimise, is ignored. Neither cycling through the sub-tabs nor
      closing a sub-tab's menu brings it back by chance: cycling shows the
      pane's message, and closing the menu without choosing focuses the
      pane but doesn't show the window.
    - **Focus.** The keyboard follows the focused pane. After a minimise,
      macOS gives it to the app's next window. One in the current tab is
      shown, as any focus is followed. One of another tab's, parked out of
      sight, isn't the user choosing it: for a second the engine doesn't
      follow that focus to the other tab (`justMinimisedLocked`), and
      gives the keyboard back to what the focused pane shows now. When
      Bench hears of the minimise from `WindowMinimised` or from
      arranging, and the window was what the focused pane showed, it
      gives the pane the keyboard back at once. A pane that shows the
      message gives it to Bench (`FocusUI`), so that **Show again** can be
      reached.
    - **Nothing saved.** The mark isn't in `state.json`. As Bench quits,
      it puts back only what it minimised itself, so a window that the
      user minimised is still minimised when Bench next opens, and the
      scan marks it again (`markMinimisedWindowsLocked`).
    - **The browser preview.** The demo bar's **Minimise window**
      (`DemoMinimise`) minimises the focused pane's App window on the demo
      platform, which reports it as macOS would. `platform.Logged`, used
      when `BENCH_DEBUG` is set, passes the call on.
    - **Not** un-minimising the window at the next arrangement, as before:
      it undid what the user asked for. **Not** waiting for
      `WindowMinimised` alone: an arrangement or a focus event before it
      would bring the window back, or follow macOS's focus to another tab.
      **Not** treating the minimise as closing the window, which would
      lose its place in its pane.

56. **Bench puts windows back after sleep, a lock, a switch of user, or a
    desktop change, without a word.** macOS can move windows as the Mac
    wakes, as the screen unlocks, and as the user switches back to their
    session, often many times as displays come back. Each move of a docked
    window reaches the engine, but it used to put the window back at once,
    so a burst at wake could use up the window's three tries in ten
    seconds (decision 13), and leave it out of its pane.
    - **Events.** `internal/mac` observes `NSWorkspaceDidWakeNotification`,
      `NSWorkspaceScreensDidWakeNotification`, and
      `NSWorkspaceSessionDidBecomeActiveNotification`, and sends `Woke`,
      `ScreensWoke`, and `SessionActive`. It also observes the
      `com.apple.screenIsUnlocked` distributed notification, as
      `ScreenUnlocked`, but Apple doesn't document it, so nothing depends
      on it: the other events come at the same times. AppKit holds
      distributed notifications back while Bench isn't the active app, and
      delivers them when it next becomes active, so this observer has a
      selector (`BenchUnlockObserver`), not a block, to ask for
      `NSNotificationSuspensionBehaviorDeliverImmediately`: an unlock that
      came late would start a wait at a time that has nothing to do with
      it. For `NSWorkspaceActiveSpaceDidChangeNotification`, it sends
      `DesktopChanged` only while Bench's window is visible and on a
      desktop that's shown (`isOnActiveSpace`), as the drag tracker checks
      (decision 13). With **Displays have separate Spaces** on, as it is by
      default, that includes each change on another display. Each goes
      through the Accessibility queue, after the window events before it,
      as `LoggingOut` does.
    - **The settle.** Each wake event starts the wait for the screens to
      settle that `ScreensChanged` starts (`scheduleScreenSettleLocked`,
      1.5 seconds after the last of them). While it waits, a docked window
      that's moved isn't put back, and a put-back already due waits too
      (`redockSoonLocked`): the settle arranges once it's over. Then the
      engine reads Bench's screen again, fills it again if Bench filled it
      before (decision 30), forgets every window's tries (`redocks`),
      docks again the windows that kept moving out of their panes
      (decision 69), looks again for the current tab's windows that it
      said were on another desktop (`recheckUnavailableLocked`), and
      arranges the tab,
      without moving the focus. A wake alone doesn't change the screens,
      so a move of Bench's window while it waits is the user's, as when
      they tile Bench just after switching back to their session: the
      engine records it, and doesn't fill the screen again over it. Only
      once `ScreensChanged` comes too, or the window goes to another
      screen, are its moves macOS's until the wait is over
      (`screensMoved`).
    - **Bench's desktop.** The Accessibility API lists only the windows on
      the desktop the user is on. So an arrangement while the user is on
      another desktop, such as a tab switch by its shortcut, finds the
      current tab's windows away, and their panes say "On another
      desktop", until a tab switch, **Rearrange**, or a move of the window
      looked again. Now `DesktopChanged` looks again at once, and arranges
      if a window is back. If one is still away, it looks once more a
      second later (`desktopRecheck`), in case macOS lists it late. It
      doesn't start the settle: macOS doesn't move windows as the desktop
      changes, and with separate Spaces each change on any display sends
      it. A settle would hold back put-backs, give apps their tries back,
      and take a resize of Bench's window just after for macOS's, filling
      the screen again over it. While Bench's window is on a desktop that
      isn't shown, nothing is sent, since an arrangement from there would
      only find the windows away again.
    - **Losing Accessibility.** macOS can take the permission away while
      Bench runs, as when the user turns Bench off in System Settings.
      Accessibility calls then fail with `kAXErrorAPIDisabled`, which
      `internal/mac` reports as `ErrNotTrusted`. When an arrangement fails
      with it (`noteApplyErrorsLocked`), or as Bench becomes active
      (`onAppActivated`), since the user usually comes back to Bench from
      System Settings, and `Platform.Trusted` says that Bench doesn't have
      the permission, the engine stops arranging (`lostTrustLocked`): the
      status bar's notice and the banner come back, as at the start, and the
      2-second check for the permission starts again. Showing an app doesn't
      need the permission (`NSRunningApplication`'s `unhide`), so the
      windows manager shows the apps it hid (`Manager.ShowHidden`), and the
      user can reach their windows. Moving windows does, so the windows it
      parked stay parked, and recorded in `parked.json`: an arrangement once
      the permission is back, or the next start with it, puts them back.
      Once it's back, the engine scans the windows and switches to the
      current tab, as when it first gets the permission
      (`becameTrustedLocked`). `internal/mac` doesn't try again to watch an
      app whose registration failed with `kAXErrorAPIDisabled`, such as one
      that opened meanwhile, so each `benchTrusted` that finds no permission
      clears `gObservingAll`, and the next that finds it watches every app
      that isn't watched yet (`benchObserveAllApps`). Whether macOS keeps
      the observers of the apps watched before is a release check. A failure
      while `Platform.Trusted` still says yes changes nothing, so that the
      check doesn't take the permission for back at once, and the engine
      doesn't hide and show the apps every 2 seconds.
    - **Logs.** With `BENCH_DEBUG` set, the engine logs each of these
      events, a move it holds back, and the settle's end. Losing the
      permission is a warning in the log.
    - **Not** a toast that says how many windows went back, with **Undo**:
      undoing it would only take windows out of their panes. Bench speaks
      only when something fails, as the banner does. **Not** registering
      the global shortcuts again after a wake: Carbon's hot keys survive
      sleep. **Not** treating macOS's moves of docked windows at wake as
      the user's: only a move with the mouse held undocks a window
      (decision 13), at any time.
57. **One key sends the shown window to another tab, and the user stays
    where they are.** `send_to_tab_modifiers`, ⌃⌥⇧ by default, with 1 to 9
    calls `SendToTab`, and with &#96; calls `SendToLastTab`, for the tab
    that `LastTab` goes back to. Both take the focused pane's shown item
    into that tab's focused pane through `moveToTabLocked`, as **Move to
    tab** in the **⋯** menu and **Pane ▸ Move to Tab** do, so a window
    that's in other tabs too follows the same rules (decision 20). Bench
    doesn't switch tabs. The toast says where the window went, with
    **Undo** and a second button, **Go there** (`Toast.Second`), which
    calls `SwitchTab` with that tab. The **⋯** menu's and **Pane ▸ Move to
    Tab**'s moves send the same toast, and their entries for the first
    nine tabs show the shortcut.
    - **The default.** ⌃⌥⇧ with a digit or &#96; is none of Bench's
      other shortcuts (⌃⌥⇧ goes with the arrows for
      `move_window_modifiers`, and with Tab for the previous window), and
      none that `shortcuts.Reserved` lists. Bench registers key codes, so
      the keyboard layout doesn't change which keys they are; &#96; is
      the same key as in `last_tab`. With VoiceOver on, swapping ⌥ for ⌘,
      as for the others, gives ⌃⇧⌘, but ⌃⇧⌘3 and ⌃⇧⌘4 are macOS's
      screenshots to the clipboard, so `Reserved` lists them now. The
      suggestion swaps ⌃ for ⌘ instead, which gives ⌥⇧⌘, so that **Use
      this instead** on the VoiceOver notice still clears it. Whether
      another app on a real Mac takes ⌃⌥⇧ with a digit is a release
      check.
    - **Old configs.** `Bindings` binds the field last, after
      `add_front_window`, `undo` (decision 65), and `dropdown` (decision
      72), so a clash is its own.
      As with `last_tab` (decision 49), `config.Parse` turns it off in a
      `config.toml` that leaves it out and gives one of its shortcuts to
      another field (`offClashingNewDefaults`), such as
      ``next_window = "ctrl+alt+`"``, whose previous window is ⌃⌥⇧&#96;:
      otherwise an update would greet the user with a notice about a
      shortcut they never set.
    - **While Bench is hidden**, the shortcut moves the window in the
      model at once, as the other window shortcuts do, and the
      arrangement waits for Bench to come back (decision 16).
    - **The frontend.** `toast` takes up to two buttons. Each runs once,
      and the toast goes when either is clicked. A toast with buttons
      shows for as long as one with **Undo** alone does.
    - **Not** following the window to its tab: `follow_focus` is about
      another tab's window coming forward, not about a move, and **Go
      there** is one click. **Not** a variant with Option that does **Also
      show in**: the default modifiers have Option already, and the jump
      palette has **Also Show In**. **Not** a message in the middle of the
      screen while Bench is hidden: the toast is kept in **Recent
      messages**.
58. **Auto chooses the layout by the width of Bench's window, and the
    compact one-pane view shows windows side by side in one step.** A
    window tiled into half a large display has as little room as a laptop's
    screen, so Auto compares `compact_below` with the width of Bench's
    content area (`contentWidthLocked`), not with the screen's.
    - **When it chooses.** Each arrangement starts by choosing
      (`chooseSizeClassLocked`), so the class follows each `BenchMoved` as
      the user resizes or tiles the window, `SetPaneArea`, and a new
      `compact_below`. The width is the platform's `ContentFrame`. Wails'
      web view is 1 pt wider than the window, so a width up to `fillSlack`,
      2 pt, wider than the screen's visible area is taken as the visible
      area's. A wider one is taken as it is: the engine's screen can be
      stale, as at launch, when it is the primary display until the window
      has opened on another, or the narrower of two displays that the
      window straddles. Until the window has opened, the width is the one
      that `WindowStart` opens it at, which also chooses the title bar's
      density (decision 33). Without a window of its own, as for `bench
      arrange`, it is the visible area's.
    - **Hysteresis.** A window that is compact stays compact until it is
      `layout.WideAgainSlack`, 40 pt, wider than `compact_below`
      (`layout.AutoSizeClass`), so that a drag of the window's edge near it
      doesn't change the layout to and fro, each change moving every
      docked window. The engine keeps the class that Auto chose last
      (`autoClass`), with the `compact_below` it chose for: a new one
      chooses afresh. While the displays settle after they changed, or
      after the window went to another screen (`screensMoved`, decision
      56), macOS moves and resizes the window. So the class stays as it
      is, as before, and is chosen once they have settled. A wait after a
      wake alone doesn't hold the class: a move of the window then is the
      user's, and the class follows it at once.
    - **One default, for new files.** `config.DefaultCompactBelow` is the
      only place in Go that has the default, and the frontend never assumes
      it: it shows the number that the View's `settings` carries. A new
      default needs, with it, a new `default.toml` and `example.toml`
      (`TestFormatDefaultIsDefaultTOML` and `TestFormatExampleIsExampleTOML`),
      the TOML blocks in `spec.md` and `user-guide.md` that show them
      (`TestDefaultTOMLMatchesSpec` and `TestDefaultTOMLMatchesUserGuide`),
      the default that the spec's **Size class** and the user guide's
      **Auto** row and config reference name (`TestCompactBelowInTheDocs`),
      and the engine's test of the default on a MacBook
      (`TestDefaultMacBookIsSideBySide`). Each of those tests fails until
      its file agrees. But a new default reaches only new `config.toml`
      files. Bench writes every setting when it makes the file at first
      run, and when **Settings** saves it, so each file that an earlier
      version wrote has that version's default, `compact_below = 1800`, as
      its own value, and keeps it. For a new default to reach those files,
      `Format` would have to leave `compact_below` out, or write it as a
      comment, while it equals the default. Then a user who chose 1,800
      would lose it, as the file can't show who chose it. The owner chose
      to keep those files as they are. The user guide tells their users
      how to get the new default.
    - **The default is 1,280 pt.** The owner chose it, so that a window
      that fills the screen of a 13-inch or 14-inch MacBook, 1,470 or
      1,512 pt at the default scaling, shows the panes side by side. A
      window at the usual size, `UsualWidth`, 1,440 pt, is wide too, on
      any display: with `fill_screen` off, after a window that filled its
      screen, and whenever Bench can't put the window back where the user
      left it. A window tiled into half of a MacBook's screen, about
      735 pt, is compact. The Automatic density follows the size class
      (decision 33), so a MacBook that fills its screen gets the standard
      density, and **Compact** in Settings gives the smaller one. The
      engine's tests keep 1,800 in the example config that they use
      (`testCompactBelow` in `harness_test.go`), so that the 1,512-pt
      laptop screen of those tests stays compact for the tests of the
      compact layout.
    - **Show side by side.** In the `tabs` view, the four splits of the
      **⋯** menu were off, with a note, and the split button refused with a
      toast: a dead end. Now the menu has one item, **Show side by side
      (wide layout)**, the jump palette lists it, **Pane ▸ Split Right**
      reads **Show Side by Side (Wide Layout)** (`MenuBarPane.SideBySide`),
      and the split button does it too. All call `ShowSideBySide`, which
      sets the mode to Wide, as the title bar's control would, and shows
      and focuses the item in its pane. A tab of two or more panes keeps
      its tree: the wide layout shows the item beside the tab's other
      panes, and choosing Auto again gives back the view from before, as
      the toast says: "To go back, choose Auto at the right of the title
      bar." (or Compact). Only a tab of one pane, which the wide layout
      shows as one pane too, is split: the item gets a pane of its own on
      the right, which the tab keeps after Auto. So that toast says that
      **Undo** removes the pane instead. The **Undo** step
      (`undoStep.setsMode`) puts back the mode with the arrangement. The
      status bar then reads **Wide**, set by hand. The split button's name
      is "Show this window side by side, in the wide layout", for VoiceOver
      too. In the `tabs` view, the move-window keys' → is
      `ShowSideBySide` as well, and the menus show it as its keys; the
      other arrows refuse, saying "Choose Wide to show them."
    - **The welcome guide** says which layout the Mac gets: its tips say
      that a tab's windows are side by side only in the wide layout, and,
      in the compact one, why (the window's width, or the user's choice)
      and how to change it, with the layout control's name as the title
      bar shows it. The tip compares the View's `windowWidth` with
      `compact_below`. For a window that is not narrower, and stays compact
      by the hysteresis, it gives the width at which the wide layout comes
      back (`WIDE_AGAIN_SLACK` in `guide.js`, as `layout.WideAgainSlack`).
    - **Not** changing the tab's `small_screens` to `stack` or `same`:
      that would change `config.toml` behind the user's back, for every
      later visit to a small screen, and be harder to undo. **Not** a
      class per display: the window's width is what decides the room.
      **Not** a change to the files that Bench has written (see **One
      default, for new files**).
59. **Keys arrange the current tab's panes: grow, shrink, equal, and the
    six layouts in turn, and a run of them is one Undo.**
    `internal/workspace/panekeys.go` has `GrowPane` and `ShrinkPane`, which
    change the focused pane's size, `EqualPanes`, which evens every split
    of the tab, and `StepLayout(step)`, which gives the tab the next (1) or
    previous (-1) layout. Each has a `[shortcuts]` field, off by default
    (`grow_pane`, `shrink_pane`, `equal_panes`, `next_layout`, and
    `previous_layout`), a row in the jump palette's **Actions**, and an
    item in the menu bar: **Pane ▸ Grow Pane**, **Shrink Pane**, and **Make
    Panes Equal**, and **Tab ▸ Next Layout** and **Previous Layout**, as a
    layout is the tab's, not the pane's. Each says what it did in a toast,
    with **Undo**.
    - **Growing and shrinking.** `layout.Resize` changes one child of a
      split by `growStep`, 5 percentage points, in the split that holds the
      focused pane (`layout.Find`'s parent), so in **Grid of four** a pane
      grows along its row. When the child grows, each other child gives
      up room in proportion to its room above `layout.MinSize`, 10%, the
      smallest that a divider drags to (`MIN_SIZE` in `drag.js`), so no
      child goes below it and a small one isn't squeezed first. When it
      shrinks, the others share the room in proportion to their sizes. A
      pane near the limit gets what room there is; only one that can't
      change at all is refused, by the way its split is drawn: "This pane
      is as wide as it can be." (narrow, tall, or short; the `stack` view
      draws the top-level row as a column). The toast says how the split's
      children share it, in whole percentages that add up to 100
      (`percents`, by largest remainder), each named by its pane's shown
      item, its label or else its app, or a nested split by its panes:
      "Zed 70% · Terminal 30%". A window title would be too long.
    - **Equal.** `EqualPanes` gives each split whose children don't look
      equal (`nearSize`, as `EvenSizes`) equal sizes, all at once, and is
      refused when there's none.
    - **Layouts.** `StepLayout` goes through `layout.Presets()`, which is
      Tab properties' order, wrapping, and applies the preset as Tab
      properties does (`layout.ApplyPreset`, with the preset's own sizes),
      which keeps the items in order and reuses the panes' IDs in reading
      order. It goes on from the tab's layout (`layoutOf`). A tab is
      marked custom whenever a window moves into it, out of it, or between
      its panes, or a pane is split, which is most tabs in use, but the
      tree often keeps its layout's shape, and going on from the first or
      last layout would step back, or give the layout it has. So a custom
      tab remembers the preset it had (`Tab.FromPreset`, set by
      `markCustom`, and `fromPreset` in `state.json`), which counts while
      the tree has its shape (`layout.HasShape`). Otherwise, as for a tab
      from an older `state.json`, `layout.PresetOf` names the preset with
      the tree's shape, where only the sizes tell Side by side from Main
      and side: the nearer of 50/50 and 65/35. From a tree with no
      preset's shape, it goes to the first layout with room, or for -1 the
      last. First it drops the empty panes (`withoutEmptyLocked`), so
      that going through **Three columns** and on doesn't leave empty
      panes behind. It keeps one that a new window is on its way to
      (`awaitedLocked`), and the one that the add dialog is open for after
      `SplitPane` (`Engine.split`), as the dialog adds to that pane's ID.
      If the dialog then closes without adding anything, `CancelSplit`
      closes the pane up and marks the tab custom, as the tree no longer
      has its layout's shape. Then it skips a layout with fewer panes than are left (`keptPanesLocked`):
      `ApplyPreset` would put the extra panes' items into its last pane,
      which going on to the next layout couldn't take apart again. A tab
      that only its own layout has room for says so (`errOnlyLayout`), and
      one with more panes than any layout has (`errNoLayoutFits`).
    - **Not in `config.toml`.** The new preset and tree are the live
      arrangement, which `state.json` keeps, as after a dragged divider.
      A key pressed several times in a row would otherwise write the file,
      and its `.bak`, each time. Tab properties opens with the live layout,
      so its **Save** keeps it in the file, and its **Reset layout** gives
      back the file's.
    - **One Undo for a run.** `beginArrangeLocked` reuses the undo step of
      the last of these changes (`Engine.arranged`) if it was to the same
      tab less than `arrangeRunFor`, 10 seconds, ago, as long as a toast
      with its **Undo** shows (`ACTION_TOAST_MS`), unless the pointer or
      the keyboard holds the toast there (`holdToast`), which doesn't make
      the run longer; no change has kept a step since; and the tab's tree
      and preset are as that change left them. Otherwise it begins a new
      step. So Grow Pane three times, then **Undo** in any of the three
      toasts, puts the tab back as it was before the first. The step's own
      toast says "Put back the layout." if any change in the run changed
      the layout, or else "Put back the sizes of the panes.", and the run's
      name, for Undo away from its toast, says what the whole run did
      (decision 65).
    - **Refusals.** All five are refused in the `tabs` view, which has no
      panes to arrange: "In the compact layout, this tab shows its windows
      as one pane. Choose Wide to arrange its panes." It names the layout,
      not the screen, as the user may have chosen it. While a pane is
      zoomed, Grow Pane and Shrink Pane are refused, as the pane fills the
      tab and nothing would seem to change; Make Panes Equal and the
      layouts end the zoom first, to show what changed, and give the
      keyboard to the focused pane's item. A tab with one pane has nothing
      to resize or make equal. The View's `display.arrange`
      (`ArrangeView`, also `MenuBar.Arrange`) comes from the same checks
      (`resizeLocked`, `unevenLocked`, and `nextLayoutLocked`), so the
      palette leaves out, and the menus disable, exactly what the keys
      would refuse. The palette's layout rows name the layout each would
      give.
    - **The shortcuts.** The five fields follow `zoom_pane` in the table,
      in Settings ▸ Keyboard, and in `Bindings`, before `last_tab`, so that
      the rule in the user guide still holds: the first in the table keeps
      a shortcut, except `last_tab`, `add_front_window`, `undo` (decision
      65), `dropdown` (decision 72), and `send_to_tab_modifiers`, which
      give way to the others. They're off
      by default: window tools such as Rectangle use ⌃⌥- and ⌃⌥= to resize
      windows, and the owner asked for them off. Being off, they need no
      `offClashingNewDefaults` entry for old configs. In the browser
      preview, `keyboard.js` runs them.
    - **Not** keys that move a divider in a direction (four keys, or a
      modifier with the arrows): the owner asked for grow and shrink of
      the focused pane, which work the same in a row and a column. **Not**
      a run that spans tabs: Undo would then change a tab out of sight.
60. **An overview shows every tab at once, and holding the `last_tab`
    shortcut's modifiers opens it to choose a tab, as Command-Tab does for
    apps.**
    `frontend/js/overview.js` draws a card for each tab, from the View
    alone: its name and shortcut, **Current** on the current tab's, the
    bell, ⚠, and camera and microphone marks of its tab and sub-tabs, and
    its panes to scale (`renderMini`, as Tab properties' preview, in the
    pane area's shape), each with the shown item's tile, marker, and title,
    then the tiles of its other items, four at most and then a count.
    - **Opening it.** **View ▸ Show All Tabs** sends `show-all-tabs`, and
      the `show_all_tabs` shortcut sends `toggle-all-tabs`, which closes
      it too; both bring Bench forward first (`windowCommands`). The jump
      palette's **Show All Tabs** opens it in the palette's place. The
      shortcut is off by default, as the owner asked of new shortcuts. Its
      field follows `jump` in the table, in Settings ▸ Keyboard, and in
      `Bindings`, so that the user guide's rule holds: the first in the
      table keeps a shortcut. Being off, it needs no
      `offClashingNewDefaults` entry. Like the jump shortcut, it notes the
      App window that has the keyboard (`noteJumpLocked`), which gets it
      back when the overview closes (decision 15).
    - **Choosing.** It's an overlay like the palette: Bench is raised while
      it's open (`SetOverlay`). The cards are in the tab strip's order,
      with the last used tab chosen, as the palette's first row. Typing
      filters them by every word, in the tab's name or an item's label
      (`fold`, as the palette). The Left and Right arrows choose the card
      before or after, and Up and Down the card a row above or below
      (`rowLength`, the cards on the first row as drawn), stopping at the
      ends. With nothing typed, 1 to 9 choose that tab; Return and a click
      switch to the chosen one (`SwitchTab`), and Escape and a click on the
      backdrop close it. A Control-click does what a click does
      (`overviewContextMenu`): macOS sends it as a `contextmenu` event,
      not a click, and during a hold the user is holding Control. The
      cards draw each tab's tree as the wide layout has it, in the pane
      area's shape (`display.area`), whatever the tab's small-screen
      view. The search field is a combobox whose `aria-activedescendant`
      is the chosen card, an option in a listbox, as the palette's rows
      are. During a hold there's no field: the listbox has the keyboard
      (`tabindex="0"`) and the `aria-activedescendant`. The dialogs keep
      Tab inside them (`trapTab`), and count such an element as a stop,
      so that Tab goes from the list to the esc button and back, and never
      leaves the dialog for the page behind it. Return chooses the tab
      chosen, but on the esc button it presses the button, as on any
      button. A card's label is its visible name and
      shortcut first, then "current tab" or "last used", its windows, what
      its panes show, and its marks; the drawing is `aria-hidden`.
    - **No pictures.** Live pictures of the windows would need Screen
      Recording and a capture of each window, which takes time and is out
      of date as soon as it's taken; the owner asked for none. Live
      previews stay on the sub-tabs (`live_previews`).
    - **The hold.** `LastTab` starts a hold (`hold.go`) if `last_tab`'s
      modifiers are still held as its shortcut arrives
      (`Platform.HeldModifiers`). A Carbon hot key reports its press only,
      and not the modifiers being let go of, so the engine asks every
      `holdPoll`, 20 ms, with its timer (`pollHold`, which switches as
      decision 67 says). The macOS
      platform asks `+[NSEvent modifierFlags]` (`benchHeldModifiers` in
      `keys_darwin.m`), which needs no Input Monitoring, unlike an event
      tap. Let go before `holdDelay`, 300 ms, the hold switches to the
      last used tab, as the shortcut always did, at most 20 ms later. Held
      past it, the hold shows the tabs: the View's `hold` (`HoldView`) has
      them, and `hold-tabs` brings Bench forward, and the frontend opens
      the overview for it (`followHold`). Its tabs are in the order they
      were used (`recentTabsLocked`): the current tab, the last used tab,
      which is chosen, the others in the order the user left them
      (`Engine.recent`, which `noteLeftTabLocked` keeps, from
      `switchLocked` and `Undo`), and last the tabs not used since Bench
      opened, in the tab strip's order. Letting go switches to the chosen
      tab (`endHoldLocked`), which gives that tab's focused pane the
      keyboard, so the jump note is dropped: the window that had it before
      doesn't get it back. The hold has no View until it shows, so a quick
      tap draws nothing. Before Bench has switched tabs there's no last
      used tab, but a hold starts anyway, with the first other tab in the
      tab strip chosen, as Command-Tab works from the first press; its
      quick tap is refused as `LastTab` is (`holdState.none`).
    - **Go keeps the choice.** While Bench's window has the keyboard, the
      `last_tab` shortcut still reaches Go as a global hot key, not the
      page, so Go, not the frontend, moves the choice: each `LastTab`
      during a hold chooses the next tab, wrapping, passing over tabs
      deleted since it began. The other global shortcuts share ⌃⌥ by
      default, and the user is holding it, so while the tabs show,
      `holdShortcutLocked` has `next_tab`, `previous_tab`, and
      `focus_pane_modifiers` with Left and Right move the choice, as the
      arrow keys do in the overview (`StepHold`); `send_to_last_tab`, by
      default the shortcut with Shift added, move it back, as
      Command-Shift-Tab does; and `zoom_pane`, whose key is Return, switch
      at once, as Return does. Only the frontend knows how many cards a
      row has, so `focus_pane_modifiers` with Up and Down send `hold-up`
      and `hold-down`, and the frontend asks `StepHold` to move a row, as
      its own Up and Down do: a step longer than one moves along the tabs
      still there and stops at the ends, where one step wraps. A tab's
      shortcut switches to that tab, which ends the hold (`switchLocked`),
      and `jump` and `show_all_tabs` end it; the rest do nothing until it
      ends, so that no pane or window moves under the overview. Escape and
      a click on the backdrop call `CancelHold`, which ends it without
      switching. `HoldView.ID` counts the holds, so that the overview
      doesn't open again for a hold the user closed (`ui.closedHold`) in a
      View sent before Go ended it.
    - **Before the tabs show.** Any shortcut but `last_tab`'s ends a hold
      that doesn't show the tabs yet with its switch (`endHoldLocked`),
      before it runs. Before there was a hold, the tap switched at once,
      so a quick ⌃⌥&#96; then ⌃⌥] went to the tab after the last used
      one, and ⌃⌥&#96; then ⌃⌥⇧&#96; sent that tab's window back: waiting
      for the hold would have them act on the tab being left.
    - **While Bench is hidden.** With `show_on_tab_shortcut` on, a tab
      shortcut brings Bench back on the tab it switches to (decision 16).
      `last_tab`'s shortcut that starts a hold hasn't switched yet, so
      `Shortcut` doesn't bring Bench forward then: Bench would come back
      on the tab being left, and `onBenchShown` would arrange that tab and
      unhide its apps, only for the switch to hide them again a moment
      later. `endHoldLocked` brings Bench forward instead, after its
      switch, or after its toast when there's no last used tab; a hold that
      shows the tabs brings it forward with `hold-tabs` already.
    - **When there's no hold.** The shortcut switches at once, as before,
      while a menu or dialog is open (`Engine.overlay`, or a native menu,
      `Engine.menus`), as the overview can't open over it and the keys go to
      it; when `last_tab` has no modifiers, such as F6; and while Bench is
      hidden and `show_on_tab_shortcut` is off (decision 16), as Bench won't
      come forward to show the tabs. A dialog that opens during a hold
      before it shows the tabs keeps them hidden, and letting go still
      switches. A hold ends without switching after `holdLimit`, a minute,
      as Bench may have missed the modifiers being let go of, and when the
      engine stops (`stopTimersLocked`). It ends without switching once the
      Mac has slept, too: the keys were let go of meanwhile, and the first
      poll after waking would otherwise see that and switch, behind the lock
      screen. The monotonic clock that timers and `Time.Sub` use stops while
      the Mac sleeps, so `holdLimit` alone wouldn't catch it; the poll
      compares the wall clock, which doesn't stop, with it (`holdSlept`),
      and `Woke`, `ScreensWoke`, `SessionActive`, and `ScreenUnlocked` end
      it too. When Go ends a hold without switching, the frontend gives the
      control or terminal that had the keyboard its focus back
      (`followHold`), as Escape does, and Go gives an App window its own
      (`refocusAfterJumpLocked`).
    - **The browser preview.** The demo platform can't read the keyboard,
      so `keyboard.js` tells Go the modifiers the page holds
      (`DemoModifiers`, refused outside the preview), in one chain of calls
      that sends them before the shortcut's `LastTab`, and again as they
      change, until none are held, or the page loses the keyboard. In the
      overview, the preview runs `last_tab`, `next_tab`, `previous_tab`,
      `send_to_last_tab`, the `focus_pane_modifiers` arrows, and the tab
      shortcuts as Go would during a hold. It runs the other shortcuts'
      methods itself, not through `Engine.Shortcut`, so a shortcut pressed
      before the tabs show doesn't land the quick tap's switch first
      there. Tests set the fake platform's modifiers with
      `SetHeldModifiers`.
    - **Not** switching tabs on each press while the modifiers are held:
      each switch would arrange the tab's windows on the way, and the
      shortcut couldn't then tell a tap from a hold. **Not** a shortcut of
      its own for the hold: a quick tap of `last_tab` keeps decision 49's
      back and forth, and holding it adds the rest. **Not** a key-up hot
      key: Carbon has none for modifiers, and an event tap would need
      Input Monitoring.

61. **Sub-tabs show unread counts, from the badges on apps' Dock icons and
    the counts at the start of pages' titles, and tabs show their totals.**
    The owner asked for one setting, `unread_counts`, on by default, in
    Settings ▸ General after **Bring back a hidden Bench with tab
    shortcuts**. Off, Bench doesn't read the Dock, and pages' titles keep
    their counts. The counts are the apps' own: looking at a tab or a
    sub-tab changes none of them, so the current tab shows its total too,
    apart from in the "tabs" view (see **Drawing**).
    - **Reading the Dock.** macOS has no API for another app's badge: an
      app sets its own with `NSDockTile`, and tells no one. The Dock shows
      the badges, and offers them through Accessibility, as VoiceOver reads
      them: the Dock app's `AXList` children are its items, an item with
      the subrole `AXApplicationDockItem` stands for an app, its
      `AXStatusLabel` is the badge's text, and its `AXURL` the app's
      bundle. `benchCopyDockBadges` (`ax_darwin.m`) reads them: the badge
      first, as most items have none, and the subrole and URL only for
      those that have one. It finds the bundle ID by the URL among the
      running apps' bundle URLs, with symbolic links resolved and in lower
      case, as the Dock and `NSRunningApplication` can give one app's path
      in different forms, or else from the bundle at that path. It needs
      Accessibility permission, which Bench has for docking anyway, and
      waits for the Dock at most the global messaging timeout, 1 second,
      for each item.
      `kAXErrorCannotComplete`, as while the Dock restarts, fails the
      reading. An app that isn't running, or has no Dock icon, has no
      badge.
    - **When.** Apps change their badges without telling anyone, so Bench
      has to look. `badgeWatcher` (`badges.go`, pure Go, tested on any OS
      with the fake clock) reads at once when it's turned on, every
      `dockBadgeEvery`, 5 seconds, and when an app comes to the front
      (`poke`, from `AppActivated`), as the user is going to read what its
      badge counts; that reading waits until `dockBadgeGap`, 1 second,
      after the last one, so that switching quickly between apps doesn't
      ask the Dock about each. One reading runs at a time, on the clock's
      timer goroutine, never under the engine's lock, as it can wait for
      the Dock; a count of the times watching was turned on drops a reading
      that began before it was turned off and on again. A reading that
      fails keeps the badges that Bench had, since the Dock may be
      restarting or busy, but `ErrNotTrusted` leaves none. The watcher
      sends `DockBadgesChanged` only when the badges differ from the last
      reading's, and the engine then asks `DockBadges`, which answers at
      once, and keeps them by lower-case bundle ID.
    - **Whether.** The engine watches (`WatchDockBadges`) while it runs with
      the setting on and Accessibility permission, and not in `bench
      arrange` (`Headless`). `flushState` decides it before it builds the
      View (`dockBadgeWatchChangedLocked`), so that the badges go in the
      same View as the permission or the setting, and tells the platform
      after unlocking, as it does `SetBadge`.
    - **Which item.** A badge is the whole app's, not a window's, so it
      shows on one item, the app's first App item in tab order
      (`orderedItems`) that's drawn as a sub-tab (not `hiddenLocked`), and
      `wholeApp` says when the app has others, for the tooltip's
      "(whole app)" and the spoken "in the whole app". On every item, a
      tab's total would count it once for each window. A tab's total adds
      its items' counts; the menu bar item's adds each item once, though it
      may be in several tabs.
    - **Pages' titles.** Web mail and chat sites put the count at the start
      of the page's title: `titleCount` takes digits in round or square
      brackets, grouped in threes with commas, full stops, or spaces, with
      a "+" for more, such as "(3) Inbox", "[12] Chat", "(1,234) Mail", and
      "(99+) Inbox". It counts only once the page's web view has reported
      its title (`WebTitle`, which sets `Item.titled` until the view
      closes), as the title in `state.json` may be out of date, and a page
      that fails to load reports none. The title loses its count before
      that too, while the setting is on. Bench makes a page's web view the
      first time it shows the page, so a page in a tab that hasn't been
      shown since Bench started, or that isn't its pane's shown sub-tab,
      has no count until then, unless it keeps running in the background
      (decision 76): the user guide says so, and so does its sub-tab.
      **Not** loading every tab's pages at launch: that would load pages,
      and use memory and the network, that the user hasn't asked for.
      Decision 76 loads only the pages kept running. The count is taken
      out wherever Bench shows the title: the View (`unreadTitleLocked`),
      and the add dialog and the toasts (`shownTitleLocked` and
      `labelOfLocked`). A title that's the count alone gives the app's
      name, and the user's own name for the item stays as it is.
      `Item.Title` keeps the page's whole title, so turning the setting off
      gives it back. A popped-out page's window has no pill, so its title
      keeps the count (`wholeLabelOf`), as the page's own title does when
      the web view puts it there as it changes.
    - **Badges' text.** `parseBadge` reads digits, grouped as above, with a
      "+" for more, as a count, and anything else, such as "•" or "!", as a
      dot. Empty and "0" are none. The platform keeps 16 runes of a badge
      (`maxBadge`), and Go counts to 999,999 (`maxUnread`). The pill shows
      at most "999+" (`UNREAD_MAX` in `util.js`), and its tooltip has the
      whole count, as "1,234 unread".
    - **Drawing.** `unreadMark` draws a grey pill, `aria-hidden`: a sub-tab's
      before its title, after the bell, and a tab's right after its name.
      A screen reader reads the count instead from the sub-tab's
      `aria-label` notes, as "(3 unread)", and from visually hidden text
      after a tab's name, ", 15 unread", so that it says the name first.
      The pill is in the flow, so a sub-tab's ×, which covers the end of
      the title, never covers it, and on a tab it comes before the
      shortcut's place, where the × goes. The marks before a sub-tab's
      title don't shrink, so in a full strip, where each sub-tab is as
      narrow as it goes, they once took the title's room: with a bell and
      "999+", the title was 0 pt wide and the × lay on the pill, which a
      click then closed. A sub-tab's narrowest is therefore
      `--subtab-min` plus the room of its marks, each with its gap: the
      capture marks, the bell, and the pill, which is 7 pt wider for each
      character (`data-chars`), or the dot (`:has` rules in
      `styles.css`). Its title keeps the 54 pt that it has without marks,
      of which the × and its fade cover the end, as on any sub-tab. In
      the smallest window, the title bar's strip of sub-tabs can be
      narrower than one of them, and scrolls. In the "tabs" view, the current
      tab has no pill: its sub-tabs, with theirs, are right beside it in
      the title bar, where its strip has no room to spare, and in a window
      as narrow as 640 pt the pill would push its × out of the strip. Its
      tooltip and the hidden text still have the total. Its colours are
      tokens
      (`--unread-bg`, `--unread-text`, and `--unread-dot`) in `styles.css`
      and `light.css`, with Increase contrast versions, grey so that the
      blue bell and the amber ⚠ stand out, and the appearance tests check
      their contrast: 4.5:1, or 7:1 with Increase contrast. The overview
      shows each tab's total and the count of each pane's shown item.
    - **Menus.** The menu bar item shows the total after the current tab's
      name, as "Code · 15", or "Code •" for a dot, while `menu_bar` shows
      text, and its text is VoiceOver's name for it. Wails gives a status
      item no tooltip and no other name on macOS, so that "15" alone could
      pass for the current tab's count: while anything is unread, its menu
      starts with a line that can't be chosen, "15 Unread in All Tabs"
      (`MenuBar.UnreadLabel`). Its menu and the Tab menu list each tab as
      "Comms — 15 Unread" (`MenuBarTab.Label`), in title case, as menu
      items are (decision 63). The menu bar item's menu changes its labels
      in place (`SameMenu` leaves the counts out, but not whether anything
      is unread), so that a count that changes doesn't close a menu that's
      open.
    - **Tests and the preview.** The fake platform's `SetDockBadge` sets an
      app's badge, which it reports while it watches, the app runs, and
      Bench has Accessibility permission, and `SetWebTitles` gives pages
      their titles. The demo's Brave, which runs from the start with three
      windows, has a badge of 4, its Mail a badge of 8, and Slack's app a
      dot, and the example config's Slack and Outlook pages have "(3)
      Slack" and "(12) Outlook" as their titles.
    - **Not** LaunchServices' private application information, which has
      the badge's text too: a private interface can change in any release
      of macOS, and the Dock's items are what VoiceOver reads. **Not** the
      apps' notifications, which Bench can't read, and which don't say
      what's unread. **Not** a count of Bench's own bells: those say a
      window needs the user (decision 43), and keep their blue.

62. **Dragging a docked window's edge that faces another pane resizes the
    panes, once the user lets go.** The owner asked for it: grabbing a
    window's edge is the natural way to make its pane larger, and the drag
    took the window out of its pane (decision 13). `edges.go` decides it
    all from frames, in the engine, so the fake platform tests it.
    - **Which drags.** A move of a docked window with `MouseDown`, past
      `dividerGrace`, compares the window's frame with where the windows
      manager docked it (`Manager.DockedFrame`: unlike `Docked`'s body,
      it's where a window that sizes in steps ended up). An edge more than
      `edgeSlack`, 2 pt, from there has moved, as `OwnChange` lets a window
      be 2 pt from a frame that the manager set. The left and right edges
      both, or the top and bottom both, are a move of the window. Each
      moved edge must face a divider: `layout.DividerBeside` walks from
      the pane up to the root, for the nearest split that runs that way (a
      row for left and right, a column for up and down) and has a child on
      that side, so the left edge of the first pane of a nested row faces
      the outer row's divider. The displayed tree counts, so the `stack`
      view's column, whose splits are the tab's own, works too. Anything
      else undocks the window as before: an edge along the pane area's
      edge, and every edge in the "tabs" view and of a zoomed pane, whose
      displayed tree is one pane. So does a drag that moves the window
      after its edge, before the user lets go.
    - **While the user drags.** An `edgeDrag` holds the window:
      `planLocked` leaves out the stack of the pane that shows it
      (`edgeHeldLocked`), so that arranging, for whatever reason, never
      pulls the window from under the pointer, and the pane's other
      windows, out of sight, stay there. Each move of the held window
      follows the drag, with `MouseDown` or not, as the pointer can run
      ahead of an edge that the app's minimum size stops. Neighbouring
      windows don't move until the user lets go. There's one mouse, so a
      move with `MouseDown` of another docked window meanwhile is its
      app's, even with the pointer on it, as it is when the held edge goes
      over it (`otherEdgeLocked`): while `Platform.MouseHeld` says the
      button is held, the window goes back 300 ms later, as an app's move
      does, and the hold carries on. Once the button is up, the user has
      let go before the next poll: the drag ends then, as the poll would
      end it. Only the tracker's `WindowDragged`, which follows a press on
      the window itself, moves the hold to another window.
    - **Letting go.** Accessibility reports moves, never the release. The
      drag tracker reports `WindowDropped`, but only while `drag_to_add`
      is on, and only for a press inside the window's frame, which a
      press on a resize edge, just outside the frame, can miss. So the
      engine also asks `Platform.MouseHeld` every `edgePoll`, 50 ms, with
      its timer, as the hold to switch tabs asks `HeldModifiers` (decision
      60). The macOS platform reads `CGEventSourceButtonState`
      (`benchMouseHeld` in `drag_darwin.m`), the button's state from the
      window server, which `benchUserDragging` reads already: it needs no
      permission and no event monitor. The first to come ends the drag:
      the drop, with the frame that it reports, or the poll, with the
      frame that the platform reports then. The tracker's `WindowDragged`
      for a docked window whose size has changed holds it too
      (`dragEdgeLocked`), within `dividerGrace` too, as the tracker
      follows only a press that went to the window itself. Past
      `edgeLimit`, a minute, with the button still held, as if Bench
      missed the release, the window undocks as before.
    - **The sizes.** For each moved edge, the divider's middle moves as
      far as the window's edge is from the edge of the pane's body, so
      that the body takes the window's edge. `layout.SizesAt` turns that
      place into sizes as dragging the divider does (`sizesAt` in
      `drag.js`): through `Frame`'s formula, so that the gap is centred
      on it, changing only the divider's two children, each at `MinSize`
      or more, rounded to hundredths. One undo step and one toast,
      "Resized the panes." with **Undo** ("Put back the sizes of the
      panes."), cover a corner's two dividers. A divider at its limit says
      "This pane is as wide as it can be.", as **Grow Pane** does
      (decision 59), with no **Undo**, and the window goes back. The tab's
      windows then dock again, and `sizedAt`, with `edgeSized`, gives
      them a narrower grace than a dragged divider's (see **A second
      drag at once**).
    - **Open risk: a press in the gap.** With the default gap of 4 pt, each
      docked window's edge is 2 pt from the middle of the gap, and macOS's
      resize area reaches a few points outside a window's frame, over
      Bench's divider. Which of the two takes a press in the gap is
      unknown until it's checked on a Mac (the first check in
      [development.md](development.md#resize-panes-by-a-windows-edge)).
      If Bench's divider takes it, nothing changes: the divider drags as
      before. If the window's edge takes it, this decision handles it,
      provided that the pointer stays within `benchUserDragging`'s 3 pt
      of the window's frame, so that the moves have `MouseDown`. A press
      further out, as in a gap of 8 pt, would be put back as an app's
      move, unless the tracker follows it. The result goes here once it's
      checked. Not checked yet.
    - **A second drag at once.** Users often drag an edge again at once,
      to adjust it. Bench resizes the panes only once it has seen the
      user let go, so a move with `MouseDown` after it is a new press,
      never the one that ended. Within `dividerGrace`, a second after the
      resize, `inGraceLocked` therefore takes a new drag of inner edges
      (or a change of 2 pt or less) as the user's, and holds the window,
      as at any other time. It puts back only a move that would undock a
      window, as after a divider drag, as the tab's windows have only
      just docked again: `OwnChange` catches Bench's own moves of them,
      but an app can settle its window in its new pane late, as the user
      presses near it. The Mac checks try a second drag and a move by the
      title bar. After a drag of one of Bench's dividers, `edgeSized` is
      false, and every move with `MouseDown` within the second is put
      back, as before: the mouse may still be held on the divider, beside
      the windows it resizes. Only a divider notes that (`SetSizes`, and
      `EvenSizes` for a double-click on one, through
      `dividerSizedLocked`). **Grow Pane**, **Shrink Pane**, and **Make
      Panes Equal** (decision 59) leave `sizedAt` as it was: they come from
      the keyboard or a menu, so a mouse held down just after them is the
      user's, and a drag of an edge then resizes the panes, rather than
      being put back and lost.
    - **An app's own resize.** An app that resizes its docked window by
      itself while the user holds the mouse on it, as in a text selection,
      looks the same as a drag of the window's edge. Before, that undocked
      the window; now, if only edges that face other panes moved, it
      resizes the panes when the user lets go, with **Undo**. A change of
      2 pt or less, in a whole drag, puts the window back.
    - **Native code.** `benchMouseHeld` alone, beside the drag tracker,
      reading what `benchUserDragging` reads. The tracker and its
      `benchWindowUnder` don't change, and the browser preview's platform
      reports the button as never held.
    - **Not** live: resizing the panes, and the other windows, as the edge
      moves would resize other apps' windows many times a second, which
      many apps draw slowly, and would fight the app whose window is being
      resized. On release, each window changes size once. **Not** tied to
      `drag_to_add`, which is about adding windows: the edge drag works
      without the tracker. **Not** a setting: the gesture changes only what
      used to undock a window, whose **Dock again** the user then needed,
      and **Undo** takes it back.
    - **Tests.** The fake platform's `SetMouseHeld` holds the button, and
      `DragWindow` and `DropWindow` hold it and let go.
      `engine_edges_test.go` drags inner, outer, and corner edges, in
      nested splits, the `stack` and "tabs" views, and a zoomed pane, with
      and without the tracker. It also drags an edge again within the
      grace, moves a window out within it, after an edge and after a
      divider, and moves another window during a hold, with the button
      held and let go. `beside_test.go` tests `DividerBeside` and
      `SizesAt`.
63. **Menus have submenus, common actions first, and one name per
    command; questions and address errors are short and say what to do.**
    The walkthrough of the menus and messages found a window menu that grew
    by two entries for each tab, names that changed from the menu bar to
    the **⋯** menu to the jump palette, a close question of four sentences
    with **Close tab** as its default, and a refused address that showed
    only in a toast, away from the field.
    - **Submenus.** **Move to Tab** and **Also Show In** are submenus of
      the **⋯** and sub-tab menus, as **Pane ▸ Move to Tab** is in the
      menu bar. Each lists the tabs, with those the window can't go to
      off, such as the current one, and **Move to Tab** with the
      `send_to_tab_modifiers` shortcuts (decision 57); then **New Tab**.
      An entry with a submenu has `submenu` and an `id`; the menu keeps
      the open one in `ui.menu.sub`, so a View that arrives while it is
      open draws it again, open. There is one submenu, `#menu-sub`, beside
      the menu in `#menu-root`, not inside it, so that the menu's scroll
      doesn't clip it. `fitMenu` places it after each draw
      (`placeSubmenu`): to the right of the menu, 4 pt over its edge, or
      to the left if there's no room, and level with its entry, inside the
      window less 8 pt. Until it's placed, it is hidden, so it never shows
      in the wrong place.
    - **Keys and roles.** The entry is a `menuitem` with
      `aria-haspopup="menu"`, `aria-expanded`, and `aria-controls`, and the
      submenu is a `menu` labelled by it. Right Arrow, Return, or Space on
      the entry opens it with the focus on its first entry; Left Arrow or
      Escape closes it, with the focus back on the entry. Up and Down
      Arrow stay in the submenu, and moving in the menu closes it. Escape
      in the menu closes the menu, as before, with a submenu that the
      pointer opened: Escape closes the menu that has the focus.
    - **Hover.** The pointer opens a submenu after it rests on its entry
      for 200 ms (`HOVER_MS`), without taking the focus, as macOS does,
      and another entry closes it after the same time. While the pointer
      moves towards the open submenu, inside the triangle from where it
      was to the submenu's near corners (`aimsAt`), the entries it crosses
      don't close it, for up to 400 ms (`AIM_MS`) after it stops, so that
      a diagonal move reaches the submenu. A click on the entry opens it
      at once. A pointer that leaves the menus before the 200 ms are up
      cancels the open, so a submenu it only grazed doesn't open later.
    - **Groups.** The **⋯** menu opens with the common actions: **Reload
      Page** for a web page (it was in the Page group), **Rename…** and
      **Use Automatic Title**, then **Close Window**, **Close Page**,
      **Close Terminal**, or, for a window in other tabs too, **Remove
      from This Tab** and **Close Everywhere**. Then splitting and Zoom
      Pane, Move Left and Move Right, **Move to Pane**, the two submenus,
      **Closed Windows**, the **Page** submenu, and the rest. The focus doesn't
      start on the first entry any more, which for an App window is
      **Close Window**: a second Return from habit would close the window
      at once, as Bench doesn't ask first and **Undo** can't bring back
      what closed. The menu itself (`#window-menu`, `tabindex="-1"`) has
      the focus, with no entry chosen, as a macOS context menu opens, and
      Down Arrow goes to the first entry. Screen readers read the menu's
      name and its status line, which describes it. The other menus keep
      the focus on their first entry, which closes nothing. The web app
      entry still joins at the end, so nothing moves when it arrives
      (decision 22).
    - **One name per command, in title case.** Every menu item, in the menu
      bar and the HTML menus, and every command in the jump palette, has
      title case, as macOS's menus do: **Pop Out of Bench**, **Split
      Right**, **Zoom Pane**, **Tab Properties…**, **Move Tab Left**. A
      command has the menu bar's name everywhere: the palette's **Zoom
      this pane** and **Add a window or app to this pane…** are **Zoom
      Pane** and **Add to Pane…**, and the **⋯** menu's **Show side by side
      (split right)** is **Split Right**. **Pane ▸ Close Window** takes the
      **⋯** menu's names for what it closes (`closeItemLabelFor`, from the
      `MenuBarPane`'s `Page`, `Terminal`, and `Shared`). Buttons, tooltips,
      headings, toasts, and body text keep sentence case, as on macOS, so
      the pane's **Show side by side** button, Tab properties' **Reset
      layout** button, and the rows of Settings ▸ Keyboard keep theirs.
      The e2e tests check the labels exactly, and `internal/ui`'s tests
      check the menu bar's.
    - **The close question** for a tab is one sentence, its title, that
      names what closes: "Close Plan and its 2 windows?", or "Close the 2
      windows in Code?" for the only tab (`closeTabTitle`). The terminals
      that are running something stay in the text, as they need a
      decision. The rest (what stays in other tabs, that apps can still
      ask about unsaved changes, `config.toml`, and what **Undo** can't
      bring back) is behind **Details**, a disclosure button with
      `aria-expanded`. When other apps' windows close, the question has no
      default button: the dialog has the focus, Return does nothing, and
      **Close tab** has the danger style, so that a habit can't close
      another app's work. Tab moves to the buttons. A question that closes
      only Bench's pages and terminals keeps **Close tab** as its default.
    - **Address errors show under the field.** While the add dialog asks
      for an address, Go's refusal, or an empty address, shows under the
      field (`#palette-url-error`, `role="alert"`), which has
      `aria-invalid` and `aria-describedby` that names it, and keeps the
      focus. Typing clears it. A refusal that comes when the dialog has
      gone to another step is a toast, as before. The texts say what to
      do: "That isn't a web address. Try example.com.", "Type a web
      address, such as example.com.", and "Bench shows http pages only
      from this Mac, such as localhost. Try https://example.com."
    - **http on this Mac.** A Web item's address can be http when its host
      is this Mac (`config.LocalHost`): `localhost` or a loopback address,
      such as `127.0.0.1` or `[::1]`. Browsers count these as secure, and a
      development server seldom has a certificate. `normaliseAddress` adds
      `http://` for them where the user types no scheme, as browsers do,
      `config.WebAddressOK` accepts them in `config.toml` and in
      `secureAddress`, and `AddWebsite` adds them as a Web item at once,
      with no web app step, as a web app would outlive the server; for the
      same reason, `webPageLocked` refuses them, so the **⋯** menu offers
      no web app and `UseWebApp` and `MakeWebApp` refuse. Every other
      address stays https only. The web view's **Not secure** rule
      (`PageInfo` in `navigation.go`) uses `config.LocalHost` too, in place
      of its own copy, so the two can't drift apart. The app's
      `Info.plist` already allows local networking, so the native code
      doesn't change.
    - **Not** http on the local network, such as `192.168.1.10`: its
      traffic leaves the Mac, unencrypted. **Not** names under
      `.localhost`, such as `app.localhost`: browsers keep them on the Mac
      themselves, but the web view asks macOS to look them up, which
      doesn't promise to, so a hostile network could answer for them.
      **Not** a submenu for **Move to Pane**: it lists only the tab's
      other panes, few, under a heading. (Decision 71 makes it a submenu:
      a native menu has no headings.)
      **Not** title case for buttons and toasts: macOS keeps sentence case
      there, and the owner asked for it.
64. **Bench's window works from the keyboard and with VoiceOver: strips
    and choices are one stop each, the terminals are read out, and ⌘,,
    ⌘T, and ⌘W work as in other Mac apps.** A pass with VoiceOver and the
    keyboard alone found tabs and sub-tabs read as toggle buttons
    (`aria-pressed`), a Tab key that stopped at every tab and sub-tab, no
    key to close one or open its menu, names that didn't start with what
    the control shows, terminals that VoiceOver couldn't read, and no ⌘,
    for Settings.
    - **Roles.** The title bar's tabs are a `tablist` of `tab`s with
      `aria-selected`; the current one has `aria-controls="panes"`, the
      `main` landmark named after the tab, which stays a landmark. Each
      strip of sub-tabs is a `tablist` too, and the shown sub-tab controls
      its pane's body, a `tabpanel` labelled by it (`pane-body-<pane>`,
      `subtab-<pane>-<item>`; `panelOf` in `render.js`). A pane with no
      strip drawn (its header is 0) has no panel. A tab or sub-tab being
      renamed is `presentation`, around its field. The segmented choices
      (the layout control, the screen picker, Tab properties' **Start
      with**, layouts, and small screens, and Settings' choices) are
      `radiogroup`s of `radio`s with `aria-checked` (`radioState` in
      `util.js`): one of them is chosen, which a toggle button doesn't
      say. The demo bar's **VoiceOver** is a toggle, and keeps
      `aria-pressed`, as Settings ▸ Keyboard's fields do. The jump
      palette's groups are `group`s, each labelled by its heading
      (`aria-labelledby`), so that VoiceOver says which group a row is in.
    - **Names start with what shows** (WCAG 2.5.3), so that Voice Control
      finds a control by the words on it: a tab is its name, then its
      unread total and its other notes, in visually hidden text after the
      name ("Comms, 15 unread"; the pill is `aria-hidden`, decision 61), a
      sub-tab is its title, then its app if that differs, then its notes
      ("zsh — ~/code/bench, Terminal (needs you)", "general, Slack (3
      unread)"), the layout button "Auto layout", Jump to… "Jump to…
      ⌃⌥Space", the title bar's attention note "1 needs attention", and
      the dialogs' esc buttons "Esc to close", the overview's included,
      or "Esc to go back to the list" in the add dialog's forms. One
      exception: the status
      bar's key hints are a button named **Keyboard shortcuts**, after the
      sheet it opens, as the owner asked, with the hints, which are key
      symbols, as its description (`aria-describedby`).
    - **One stop each, and the arrow keys** (`roving.js`). Each `tablist`
      and `radiogroup` has one member with `tabindex="0"`: the HTML gives it
      to the selected or chosen one, or the first, and `syncRoving`, after
      each render and on `focusin`, to the one with the focus while the
      group has it. In a strip, Left and Right Arrow move the focus,
      wrapping, Home and End go to the ends, and `revealFocus` scrolls the
      strip to keep it in view, which marks the strip browsed (decision 32).
      Return and Space are the button's own click: they show the tab or
      sub-tab, as before. Down Arrow on a tab goes to the stop of the
      focused pane's strip (the `tabs` view's, in the title bar), noting the
      tab's name (`downFrom`), and Up Arrow on a sub-tab goes back to that
      tab while it's there, or else to the tabs' stop: the tabs' stop is the
      current tab again once the focus has left them. Delete and Backspace
      close as the × does (`closeTab`, `quickClose`), asking where it asks;
      every tab and sub-tab has `aria-keyshortcuts="Delete Shift+F10"`,
      since its × isn't a control a screen reader finds. Shift+F10,
      Control+Return, and the menu key open the tab's menu, or the sub-tab's
      window menu, below it, with the sub-tab as the menu's `opener`, so
      that Escape gives it the focus back; they're `preventDefault`ed, so
      the browser's own context menu event doesn't follow. In a radio group,
      the arrow keys, Home, and End move and choose (a click), as macOS's
      radio buttons and the ARIA pattern do: Tab properties' and Settings'
      choices change a draft. The layout control and the screen picker have
      `data-activate="manual"`: their choice arranges every window, and
      choosing Compact can slim the title bar, which hides the control, so
      the keys only move the focus, and Return or Space, the button's own
      click, chooses, as an `NSSegmentedControl` does with Full Keyboard
      Access. Modifier keys pass, so Bench's shortcuts in the browser
      preview, and the menus' and dialogs' keys, are as before. The keys
      come before the dividers' and the dialogs' in `onKey`, after an open
      menu's. The overview's cards (decision 60) are `option`s in a
      `listbox`, not tabs, so `rovingKeys` leaves them to `overviewKeys`,
      and the overview's opener, a tab that had the keyboard, gets the
      focus back as it closes, still in its strip.
    - **Where the focus goes when its sub-tab closes.** `setRegion` puts
      the focus back by `data-key`, which a closed sub-tab no longer has,
      so the focus would fall to the page. `roving.js` notes the strip and
      place of a tab or sub-tab that has the keyboard's focus
      (`:focus-visible`, so a click doesn't count), and `keepPlace`, after
      a render that has no dialog or menu open, moves a focus that's on
      the page to the one in its place, or the strip's last, or to the
      current tab if the strip has gone, as a pane left with one app's
      window has no header. The note waits across the question before
      closing, and goes once the focus or the pointer moves anywhere
      outside the dialogs and menus. It's matched by `data-key`, not by
      element: the web view takes the focus off a removed element with a
      `focusout` while it's still in the page, which looks like the user
      leaving it. A closed tab needs nothing: `followTabKey` gives the
      focus to the current tab.
    - **VoiceOver and the terminals.** The engine keeps
      `Platform.SystemKeys().VoiceOver` (decision 45) and sends it as the
      View's `voiceOver`, again when `SystemKeysChanged` changes it; the
      Fake platform's `SetSystemKeys`, and the browser preview's
      `SetDemoVoiceOver`, set it. While it's on, every xterm.js terminal, in
      the main window and a terminal's own window, has `screenReaderMode`,
      which keeps the shown lines in an element that VoiceOver reads and
      reads out new output; it's off otherwise, as it costs a second copy of
      the screen. `terminals.js` announces through the toast, which is a
      polite live region (`role="status"`), so no second live region
      competes with it: xterm.js's `onBell` in a terminal that this window
      draws, unless it's the snapshot being drawn again; and, from each View
      (`announceChanges`, in `prune`), a terminal whose `ended` turns on,
      saying whether with an error from its `failed`, which Go sets whether
      or not the user can see the terminal (`attention` says so only for one
      the user can't, and never for a popped-out one), and one whose
      `attention` turns to "Rang the bell", which covers terminals in other
      tabs that aren't drawn. Each window speaks for the terminals it draws:
      the main window for those in tabs, a terminal's own window for its
      own. The text is the sub-tab's title, the tab if it isn't the current
      one, and what happened: "go test — ~/code/bench in Code: its command
      has finished.". The same text for a terminal isn't announced again
      within 5 seconds, as Go's `notifyGap`, so the bell and Go's mark for
      it are one announcement. While another toast shows, `announce` keeps
      the text until that toast has shown for its reading time (`readTime`,
      how long a toast without buttons shows), so that a message from Go
      isn't replaced unread, and the same text waits once. Then it replaces
      a toast without buttons; a toast with buttons, such as **Undo**, keeps
      them and its text, and gets the announcement after its text as a span
      of its own, which the live region reads alone, and stays until that
      has been read too. Waiting until such a toast goes, 10 seconds, left
      an announcement out of date. The recent messages keep the
      announcements, as they keep every toast.
    - **⌘ keys in Bench's window.** The app menu's items have no key
      equivalents (decision 46), so WKWebView passes ⌘ keys that nothing
      else takes to the page. `commandKeys` in `keyboard.js`, in the capture
      phase, after Settings ▸ Keyboard's recording and before a focused
      terminal: ⌘, opens Settings and ⌘T **New tab**, while no dialog is
      open, the overview of every tab included (decision 60); in a terminal in a pane, ⌘T is **New Terminal in *folder***
      (`AddToPane` with its `here`), and ⌘W closes it as its × does
      (`quickClose`), which asks first through `CloseItem` while it's
      running something. A terminal's own window binds ⌘W natively
      (`KeyBindings`, as the Acknowledgements window does), and its `Close`
      goes through `keepTerminalOpen`, which asks first. Every other ⌘ key
      is left alone: ⌘W outside a terminal doesn't close Bench's window, and
      a web page or an app's window keeps its own ⌘ keys, as they never
      reach Bench's page. The Keyboard Shortcuts sheet lists them, with the
      strips' keys, under **In Bench's window**. In the field that renames a
      tab or sub-tab, ⌘, and ⌘T first save the name typed (`commitRename`),
      as a Mac's text field ends editing for a menu command, and the
      dialog's `opener` is the tab or sub-tab; a save that answers while a
      dialog or menu is open leaves the focus there.
    - **Not** `aria-pressed` for tabs and choices: a toggle says on or off,
      not which one of a set is shown. **Not** a Tab stop for each tab and
      sub-tab: with 20 tabs, reaching the panes took 20 presses, and the
      ARIA tabs pattern has one. **Not** automatic activation as the focus
      moves along the tabs, or along the layout control: switching a tab or
      a layout moves and raises other apps' windows, which is slow, so
      Return or Space does it, as the ARIA pattern allows; exploring the
      layout control with the arrow keys rearranged every window, and
      Compact then hid the control. **Not** a live region of Bench's own for
      the announcements: two would talk over each other. **Not** ⌘W to close
      Bench's window: it holds every tab, and other apps' windows, and
      macOS's **Window ▸ Close** isn't in its menu. **Not** ⌘T in a
      terminal's own window: it has no pane for the new terminal.
65. **Undo can be reached after its toast has gone, and a closed page or
    terminal can be opened again.** A toast's **Undo** was the only way to
    take a change back: it went after 10 seconds, even while the user was
    reaching for it, and a toast that came while the list of notices was
    open covered the list. Closing a web page or a terminal couldn't be
    undone at all.
    - **The toast waits.** `holdToast` in `state.js` stops the time of a
      toast with buttons while the pointer is over it or the keyboard is in
      it, and starts it again with what it had left, or 3 seconds
      (`HELD_TOAST_MS`) if that's longer. The toast lets clicks through
      (`pointer-events: none`, but for its buttons), so its own pointer
      events don't come: `watchToast` hit-tests the pointer's place on the
      document's `pointermove`, and a `mouseout` with no `relatedTarget`,
      as the pointer leaves the window, lets go. `focusin` and `focusout`
      on the toast follow the keyboard. A toast without buttons goes in its
      own time: there's nothing in it to reach for.
    - **Not over the open list.** While the list of notices and recent
      messages is open (`ui.menu.kind === 'notices'`), `keepToastOffList`,
      which each render calls, gives the toast `under-list`: `opacity: 0`,
      but still the polite live region that VoiceOver reads, with its
      buttons out of reach (`pointer-events: none`, `tabindex="-1"`). The
      list shows the message at its top, as it shows every toast, with
      **Undo**. The toast shows again if the list closes while it has time
      left. Moving the toast elsewhere would still cover something.
    - **The latest change, named.** Each undo step has an `undoName`: the
      command's name in title case, for "Undo Close Tab", and what it did,
      for "Undid: closed Code." `keepUndoLocked` takes it with the step, so
      that every change that offers **Undo** names itself. A run of the keys
      that arrange the panes shares one step (decision 59), so
      `arrangeToastLocked` names the run: with the command's name while
      every change in it has the same name, as **Grow Pane** pressed again
      for the same pane has, and otherwise, as Undo takes them all back, with
      **Change Layout** once one of them changed the layout, or **Resize
      Panes** (`resizePanesName`), which also names a drag of a docked
      window's edge (decision 62). The View's `undo`
      is the latest step's number, name, and what it did, and `MenuBar.Undo`
      its name. `UndoLatest` is `Undo` of that step, with the "Undid:" toast,
      as a shortcut's or menu's Undo happens away from what changed, and
      the toast says what came back. It's reached from **Tab ▸ Undo**, which
      reads **Undo** *name* and is disabled with none (**Edit ▸ Undo** keeps
      its role, for text fields, pages, and terminals); the jump palette's
      first row, under **Last change**, which is found by "undo", and by
      the change's name only as a hint is (`findAs`), so that the name of
      a command typed again finds the command before its Undo; a recent
      message whose toast had **Undo**, while `undo.step` is still that
      message's step (`noteMessage` keeps the step); and the `undo`
      shortcut. The shortcut is off by default, as `add_front_window` is:
      it binds after `add_front_window`, and a config that leaves it out
      doesn't need its shortcut taken from another field. From another
      app, it brings a hidden Bench forward, as **Add Front Window** does,
      so that what Undo put back and its toast can be seen, and with
      nothing to undo too, so that the press isn't lost without a word:
      macOS beeps for a command it can't run, but a beep needs a native
      call, and the toast says why. The clash checker refuses ⌘Z for it,
      and the other shortcuts that every Mac app uses
      (`shortcuts.AppKey`, decision 45), which would take them from every
      app, Bench's pages and terminals too. There's no
      Redo: a change the user didn't mean to undo can be made again, and
      Redo would need a second history that every change forgets.
    - **The palette's Return.** The **Undo** row is first, but with nothing
      typed, `renderRows` selects the first row that isn't `passOver`, the
      last used tab, so that Return goes back there, as decision 49 has it;
      Up Arrow reaches **Undo**. A palette opened or typed in has no
      selection until its rows are drawn (`sel: null`).
    - **Closed pages and terminals.** `CloseItem` on a Web or Built-in item
      in a tab keeps it (`closedItemLocked`, `closeditems.go`): its
      `ItemState`, as `state.json` saves one, its title, its places (each
      tab that had it, by pointer and by name, its pane, its index among the
      pane's items, and whether it was shown), and, while
      `keep_terminal_output` is on, a terminal's output, made safe to show
      again (`term.Restorable`), and when it closed. The step's `reopen`
      holds it, and `keepUndoLocked` notes it for **Reopen Closed Page or
      Terminal**, the last 10, latest first. They're in memory only: a
      closed terminal's output never reaches the disk for this, and turning
      `keep_terminal_output` off forgets it (`forgetClosedOutputLocked`),
      with the output of reopened terminals whose sessions haven't started.
      `reopenedItemLocked` makes the item again with its own ID
      (`itemFromState`), and the terminal's session starts with the output
      and the line that says when it's from (`restoredPreface`), through
      `Item.preface`, a string so that items stay comparable, which
      `savedOutputLocked` takes once, before a file's.
    - **Undo, or Reopen.** Undo after the close puts the item back in the
      arrangement from before it: `takeBack` puts the reopened item in
      `kept`, so `restoreLocked` keeps it where the arrangement had it, and
      the item is no longer kept to reopen. Undo of an earlier change that
      takes the close back too does the same only if that change's
      arrangement had the item (`hadItem`); otherwise there's no place for
      it, and it stays for Reopen. `ReopenItem` puts it back in each tab
      that had it and still does, matched by the `Tab` itself, or else by
      name, in its pane at its index (`putBack`). A pane that held only the
      item closed up with it, so `closedItemLocked` also notes, for such a
      pane, the pane next to it in its split, the nearest leaf of its
      neighbour, and the side it was on (`besideOf`): Reopen splits that
      pane again on that side, as a move to a pane's edge does, so that a
      page that had a pane of its own gets one again, rather than going over
      another pane's windows, and at the size it had (`resizeTo`): the pane
      beside it takes back the half that `layout.Split` gave the new pane,
      and then the split's panes give up its old share in proportion, which
      puts back the sizes of the split it was in. If that pane has gone too,
      it goes in the tab's focused pane; with no such tab, in the current
      tab's focused pane. It switches to the first, and drops the undo step
      that closed it, which would now do nothing. Undo of the close needs
      none of this, as it puts the whole tree back. The changes made while
      the item was closed are kept, but their arrangements from before them
      have no place for it, so that their Undo would put it in the focused
      pane, as an item that has come since, and a page that has a pane of
      its own again would go over another pane's windows, hidden. So
      `ReopenItem` also puts the item back, with `putBack`, in the saved
      trees of each change after the close (`closedItem.step`, the close's
      number, which `keepUndoLocked` notes, so that this holds once Undo
      no longer has the close too), for each tab it went back in; their
      roots are copies, so this changes no live tree. A closed item can
      come back another way, with its ID, as Reopen Closed Tab and Restore
      previous arrangement bring back a tab's pages and terminals:
      `restoreTabLocked` then forgets it (`dropReopenedLocked`), the View
      leaves out any that's open, a second close of the same ID replaces
      the first, and `ReopenItem` of one that's open shows it where it is.
      Each is named as Bench's messages name it (`labelOfLocked`, such as
      "Slack — # general"), so that two pages with the same title can be
      told apart. A tab's menu
      offers the latest, the menu bar's Tab menu names it as **Reopen Closed
      Page** or **Reopen Closed Terminal** (`ClosedItem`, `ClosedTerminal`),
      and the jump palette's Actions list them all, with their tab.
    - **Not** for an App window: its app closed it, and may have asked
      about saving, so Bench can't open the same window again. **Not** for
      a page or terminal closed while popped out of Bench: it has no place
      in a tab to go back to. **Not** in `state.json`: unlike a closed
      tab, a page or terminal is quick to add again, and keeping a
      terminal's output after it closed would put it on disk where the user
      expects it gone. **Not** **Undo** in a tab's right-click menu: the
      menu bar and the palette have it, and the tab's menu acts on that
      tab.
66. **Settings shows one section at a time, asks before it throws changes
    away, and previews the colours, density, and space between panes.**
    Settings had grown to eight groups in one long dialog: at 640 by 480
    points most of it was out of view. Escape, **Cancel**, or a click
    outside threw away every change without a word, as they did in Tab
    properties, **New tab**, and the add dialog's forms. And the colours,
    density, and space between panes showed only after **Save**.
    - **Sections.** `SECTIONS` in `settings.js` lists them, and the draft's
      `section` is the one shown. The list down the side is a vertical
      `tablist` with `data-activate="auto"`, so that `roving.js` shows
      each section as the focus moves to it, with the Up and Down arrows,
      Home, and End. The keys of the title bar's strips (Left and Right,
      the menu keys, and Delete) stay with `.tabs` and `.subtabs`. The
      section is a `tabpanel`, labelled by its tab. Settings is 760 points
      tall, or as tall as the overlay has room for (`min(760px, 100%)`),
      for every section, so that it doesn't change size, and move its
      buttons, as the user goes down the list. A section that scrolls has
      a shadow at each edge with more beyond it, as macOS hides scroll
      bars until they're used: two `scroll` gradients, under two `local`
      covers of the dialog's colour that scroll with the section and hide
      each shadow at its end, with no script. Under 560 points wide (a
      container query on `.settings-sheet`, which measures inside its
      border), a `select` beside the title takes the list's place.
      `MIN_SIZES` keeps a Settings that the user resizes 560 points wide
      at least, so that only a window too narrow for that hides the list.
      That also gives the list to a size remembered from before the
      sections, when Settings opened 560 points wide, which would
      otherwise have kept the pop-up menu on every screen. The section shown last is kept in
      `localStorage`, read and written in `try`, and in a variable for a
      page that can't use its storage. The draft is still one for the whole
      dialog, so that **Save** saves the changes in every section
      together. `changedSections` compares the draft with what Settings
      opened with, by `FIELD_SECTIONS`, for the dots in the list and the
      question's text. The number fields don't redraw the dialog as
      they're typed in, so `markChanged` puts the dots, and the pop-up
      menu's "(changed)", right in place.
    - **The question.** `discard.js`: `unsaved` says what the open dialog
      would lose, against a `base` that each dialog keeps as it opens
      (`editorChanged`, `terminalChanged`, `changedSections`), so that a
      change put back by hand isn't a change. `askDiscard` keeps the
      dialog in `ui.discard` (`back`, `what`, the command to go on with,
      `next`, and the focus) and opens the `discard` overlay, which draws
      the question over the dialog it covers. The covered dialog is drawn
      as before, inside an `inert` wrapper, so it keeps its place, its
      draft, and its scroll, and takes no click, focus, or drag
      (`coveredUI` gives the fill hooks its kind, and `placeDialog` places
      it as its kind and the question over its middle). **Save** calls the
      covered dialog's own save (`SAVES`), so that the same checks,
      refusals, and toasts apply, and `next` runs only if the dialog then
      closed. Commands that open a dialog in the open one's place go
      through `afterAsking`; while the question is open, they do nothing:
      a second question would hide the first. The question before quitting
      takes its place, and gives back the covered dialog (`openQuit`);
      while that question is open, `afterAsking` does nothing either, since
      `unsaved` sees only the open overlay, not the dialog the question
      covers. **Close Tab…** and **Close Window** ask Go first, and go
      through `afterAsking` only when Go answers with its question, which
      would take the dialog's place: a close that needs no question leaves
      the dialog as it is. **View ▸ Show All Tabs** and the
      `show_all_tabs` shortcut go through `afterAsking` too, as the
      overview of every tab (decision 60) takes the dialog's place: it and
      Settings both raise Bench, but there's one overlay, so they're never
      open together, and Discard ends the preview as the overview opens.
      A hold after the `last_tab` shortcut doesn't show the overview over
      a dialog (`startHoldLocked`, `followHold`). The add dialog's Escape
      and **Back** (`backToList`) ask with `stays`: **Discard** leaves the
      dialog open and goes back to its list, and **Save** (**Add**) closes
      it as the form's own **Add** does, so it doesn't go back.
    - **The preview.** The engine keeps `SettingsPreview` in `e.preview`
      (`settingspreview.go`). The View's `appearance`, `density`,
      `titleBar`, and `display` take it (`lookLocked`,
      `shownDensityLocked`, and `displayWithLocked`, with the previewed gap
      and header height), and the shell gives Bench's windows the
      appearance (`Emitter.Appearance`). Arranging still uses the saved
      settings, so a window event meanwhile docks to the bodies saved. So
      do the keys that arrange the panes (decision 59): they change only
      the splits' sizes, and the View draws the new sizes with the
      previewed gap. So does a drag of a docked window's edge (decision
      62): `edgeChangeLocked` measures it against the body that the window
      was docked in (`displayLocked`), not the one that the View draws. The
      frontend's `syncPreview`, which each render calls, sends
      `PreviewSettings` with `actLatest` when what the draft would preview
      changes: each of the three that differs from what Settings opened
      with. `SaveSettings`, `SetOverlay(false)`, as when the last dialog
      closes, and `Ready` end it in Go, so that a preview can't outlive
      the dialog, and `ready` and `catchUp` send it again (`resendPreview`)
      for a dialog that is still open.
    - **Not** a preview that moves the docked windows: Bench is raised over
      them while Settings is open, and moving every window at each click,
      and back again on **Discard**, would be slow, and would show other
      apps' windows jumping behind Bench. They move once, on **Save**.
      **Not** saving each change as it's made, as System Settings does:
      each save writes `config.toml` and its backup, and arranges the
      windows, and **Cancel** would mean nothing. **Not** a dialog that
      changes size for each section, as some Mac settings windows do: it
      would jump as the arrow keys go down the list. **Not** keeping the
      add dialog's typed address or form when Escape or **Back** goes back
      to its list, to come back to: a draft that came back unasked, when
      the row is chosen again, would surprise more than the question does.
      **Not** the jump palette: its search is only a search. **Not** commands queued while the question is open: the user
      answers it first.
67. **Bench times each arrangement, parks before it shows an app, and shows
    the new tab at once.** A switch felt slow, and nothing said how slow:
    Bench didn't measure it. A switch to a tab with a window of an app that
    Bench hid showed the app's windows of other tabs over the tab for a
    moment (the flash): `Apply` showed the app, then parked them. And the
    title bar showed the new tab only once Go had arranged it.
    - **The times.** `timing.go`. An arrangement is timed from what asked
      for it, as the engine got it, to the last frame set: the end of
      `arrangeLocked` that applied a plan, after the web views. Each bound
      method, event, and timer that takes the lock reads the clock first,
      with `lockFor` or `lockAt`, so that the wait for the lock counts, and
      gives its reason: `tab` for `SwitchTab`, `StepTab`, and `LastTab` (a
      click on a tab, a palette row, an overview card, a key in the browser
      preview, the Tab menu, the menu bar item); `tab-shortcut` for a tab's
      shortcut, `next_tab`'s, `previous_tab`'s, and `last_tab`'s, and the
      end of a hold after `last_tab`'s, timed from the poll that saw its
      keys let go of (decision 60); the event's kind for an event, such as
      `bench-moved`; `pane-area` for a new pane area; `rearrange` for
      **Rearrange**; `settle` for the end of the wait after a wake or a
      change of displays (decision 56); `apps-opened` for a switch's wait
      for the apps it opened; `timer` for another wait; `switch` for
      another switch, such as one that **New Tab…** or **Undo** makes; and
      `other` for the rest, such as a change in a menu. The bound methods
      keep their arguments: the frontend's calls don't say where they came
      from. The clock is the engine's: `clock.Real` is `time.Now`, whose
      times carry the monotonic clock, which `Time.Sub` uses, so a change
      of the wall clock doesn't count. A hold of the lock that arranges
      more than once keeps one time, to its last end, with the most windows
      that one of its plans asked the platform to change
      (`windows.Result.Touched`: moved, parked, minimised, restored, or
      raised, each window once). `unlock` keeps it in an `arrangeTimes`, a
      ring of the latest 50, in memory only: `Engine.switches` for the tab
      switches (`tab`, `tab-shortcut`, and `switch`), and `Engine.arranges`
      for the rest, so that dragging Bench's window, which arranges once a
      frame, doesn't push the switches out. It logs it at debug level as
      one line: `msg=arranged reason=tab ms=41.2 windows=6`. The cost is a
      clock read as the lock is taken, another as an arrangement ends, and
      a copy of a small struct as the lock is let go, with no allocation;
      the log line is built only with `BENCH_DEBUG`. `ProblemReport` sums
      each ring up by reason (`ArrangeTiming`: how many, the median, the
      worst, and the most windows that one arrangement changed), as
      `Switching` and `Arranging`, and **Report a Problem…** writes a part
      for each, with each reason's line, the most frequent first, up to 12.
      The macOS demo workflow sums up the log's lines in
      `arrange-times.txt` and its steps log, and never fails on a slow
      time: a runner is slower than a Mac, and `BENCH_DEBUG` logs every
      platform call too.
    - **Parking first.** `parkBeforeShowing`, the first pass of `Apply`:
      for each app in `Plan.Unhide` that is hidden now, or that the manager
      hid, it parks the app's windows that are to go out of sight (those in
      `Plan.Park`, and those in a pane that shows a Web or Built-in item),
      reading each first and leaving out one in full screen or minimised,
      as `park` does, and one with no place to park clear of other
      displays, which `park` would minimise. It's quiet: what it doesn't
      park, the passes after showing the app read again and park as before,
      with the errors that they report. A window that has closed, or is on
      another desktop (`ErrNoWindow`, `ErrOtherDesktop`), is passed over;
      any other error, which may be a slow app's, ends the pass for that
      app, so as not to wait for it again. So an app whose hidden windows
      don't move, which macOS may not allow for some apps, still has them
      parked, as before, with the flash. `parked.json` lists a window
      before it moves, as for any park (see [Crash safety](#crash-safety)).
      The fake platform's `FixHiddenWindows` stands for such an app in the
      tests.
    - **The title bar at once.** The frontend's `switchTab` (`state.js`)
      shows the tab as current (`ui.switching`, which `renderTitlebar`
      draws with `shownCurrent`) and then calls `SwitchTab`, or `StepTab`,
      with a target counted from the tab shown. The View that has the tab
      current ends it (`settleSwitch`, in `setView`); a View that doesn't,
      sent before Go switched, keeps it. A refusal puts the title bar back
      at once (`dropSwitch`), and so does the end of a wait for the View: a
      second after Go answers, or 5 seconds in all. A click on the tab that
      Go has current, while the title bar shows another, is a click on
      another tab: `clickTab` compares with `shownCurrent`, so it switches
      back at once, with no wait for a double-click. In the app, Go handles
      the tab shortcuts, even in Bench's window, so `switchShowing` sends
      the `switching` command, with the tab, before it switches, and
      `switch-refused` after it if the switch was refused or ended on
      another tab, as when a click comes in between its holds of the lock.
      It sends none for the current tab, a tab that isn't there, or
      `last_tab`'s shortcut that starts a hold: the hold sends it as its
      keys are let go of, before it switches, from `pollHold`, which takes
      the lock again for the switch. A switch that a hold makes as another
      shortcut ends it (decision 60) shows with the View only. Only the
      title bar changes at once: the panes, their sub-tabs, and the windows
      are the View's, and change as Go arranges them.
    - **Not** a speed in the README or the user guide: a Mac with many
      windows, or a slow app, takes longer, and a number would read as a
      promise. **Not** times kept on disk, or sent anywhere: the report's
      text has them for the user to read and send. **Not** a reason
      argument for `SwitchTab`: its signature is the frontend's and the
      tests', and the reasons from the frontend are all `tab`. **Not**
      minimising, or moving, a hidden app's windows another way first:
      parking is what the windows get anyway, and minimising animates, and
      may show the app. **Not** one ring for every reason: a drag of
      Bench's window would fill it in under a second. **Not** a change to
      how Bench follows its own window as it's dragged or resized live:
      those arrangements are timed, as `bench-moved` and `pane-area`, but
      done as before. **Not** showing the new tab's panes before Go has
      arranged them: Go's View is what the windows follow, and drawn panes
      that the windows didn't match yet would look broken.
68. **Bench hides only what covers it, and says why.** Bench's window sits
    below normal windows (approach A), so a window that isn't in a tab can
    cover its tabs, panes, and status bar, and a click on Bench can't bring
    Bench in front of it. As Bench became active, it hid every app with
    such a window over its content area. That also hid the app's other
    windows, such as one on another display, and nothing said why, so the
    windows seemed to go away.
    - **The test.** As before: a standard window that isn't in a tab and
      isn't minimised, whose frame overlaps `ContentFrame`, while Bench's
      main window shows (`onAppActivated` and `onBenchShown`). An app with
      a window docked in the current tab or popped out is left alone, as
      is the browser that the add dialog waits for a web app from, and a
      window that a `float = true` rule matches (decision 30). Bench
      coming forward for a drop from the Finder, to give a new terminal
      the keyboard, isn't the user going to Bench, so it puts nothing
      away (`droppedForwardLocked`, decision 73).
    - **Hide or park.** `hideCoveringLocked` (`covering.go`) reads each
      such app's windows (`coveringLocked`). If the app has another window
      to keep showing (a standard window on screen and clear of the
      content area, one in native full screen, or one that a float rule
      keeps over Bench), Bench parks only the windows over Bench, with the
      new `windows.Manager.Park`, which parks one window outside a plan as
      `Apply` parks another tab's: below the bottom of Bench's screen
      (`parkScreenLocked`), or minimised if no place there is clear of
      the other displays, recorded in `parked.json` first. Otherwise it
      hides the app with `Manager.Hide`, as before. Minimised windows,
      and windows that the manager has parked, such as other tabs', while
      they're clear of the content area, aren't on screen, so they don't
      count as others to keep. A parked window that something else has
      put back over the content area, as macOS can as displays come back,
      covers Bench again, and `Park` parks it again.
    - **Coming back.** macOS shows a hidden app when the user brings it
      forward. The engine keeps the windows it parked, with their apps,
      in `covered`, in memory, and `Unpark`s them when their app becomes
      active (`unparkCoveredLocked`), however the user brought it
      forward: its Dock icon, Command-Tab, or a click on its window on
      another display. Quitting puts them back with `RestoreAll`, and a
      crash with `RestoreCrashed`, the watchdog, or `bench restore`, as
      for any parked window. One that closes, or whose app quits, is
      forgotten, with `Manager.Forget`; one that a tab has taken since is
      the tab's, as docking drops the manager's record of it. A hidden app
      always shows when it comes forward, but `Unpark` can fail: for a
      window on a desktop that isn't shown, in native full screen, of an
      app that doesn't answer, or without Accessibility permission. The
      manager then keeps its record, and so does `covered`, which drops a
      window only once the manager no longer has it parked. Bench tries
      again the next time the app becomes active, and, while the app is
      in front, as the desktop changes (`onDesktopChanged`), the screens
      settle, and the permission comes back (`becameTrustedLocked`), all
      through `retryCoveredLocked`. As the screens settle, it also parks
      again, with `Park`, the windows in `covered` whose apps aren't in
      front (`reparkCoveredLocked`), as the arrangement parks other tabs'
      windows again: `Park` leaves alone one that's still where it parked
      it.
    - **Saying so.** The first time Bench puts an app's windows out of
      sight this way, a toast says so: "Hid Calculator because it covered
      Bench.", "Hid the Finder window “Downloads” because it covered
      Bench.", or "Hid 2 Finder windows because they covered Bench.". The
      engine keeps the apps it has told about, by lower-case bundle ID,
      in `told`, which `state.json` saves as `covering`, so that the toast
      comes once for each app, even after a restart. Only one app's toast
      comes at a time: when two apps are new at once, the second app is
      told about the next time, with its own button.
    - **Keep over Bench.** For an app with no items, and no rule that
      sends its windows to a tab (`canFloatLocked`), the toast's button
      calls `KeepOverBench(bundleID)`. It adds `[[rule]]` with `app` and
      `float = true` to `config.toml`, through `saveEdit` as Settings
      saves, then shows the app with the new `Manager.Unhide`, or
      unparks its windows. Its toast has **Undo**: the step keeps
      `config.toml`'s rules and `covering`, which marks a change to the
      rules alone. `takeBack` restores the rules, leaves `config.toml`'s
      tabs as they are, and puts back the arrangement from before the
      first step in its range that isn't such a change: with none, the
      arrangement stays as it is, so that tab switches and divider drags
      since stay. Then, while Bench is active and shown, it puts what
      covers Bench out of sight again, for the tab now shown. It's
      refused for an app that Bench hasn't told about, and for one with
      items or a tab rule (`errKeepOverInTabs`), and with an app-wide
      float rule for the app already, it only shows the windows again.
      The toast shows for a few seconds, and another can replace it, so
      the recent messages keep its button, as they keep **Undo**: the
      View's `keepOver` lists the apps that Bench has told about, that no
      app-wide float rule floats, and that can float, and a message whose
      toast had **Keep over Bench** for one of them shows it again.
    - **The setting.** `covering_windows`, `hide` by default, or `leave`,
      which turns all of this off. Settings ▸ Windows has it as **When
      Bench comes forward**: **Hide windows that cover Bench** or **Leave
      them**, whose line says that windows left in place sit over Bench's
      own tabs, panes, and buttons.
    - **The browser preview.** A page can't make Bench the active app, so
      the demo bar's **Bring Bench forward** calls `DemoComeForward`,
      which does what becoming active does. The demo's Finder window,
      Downloads, is in no tab and covers Bench.
    - **Not** raising Bench to the normal level and raising the docked
      windows again, the spike's other way: it would show Bench over its
      own docked windows for a moment each time, and race with apps that
      raise their own windows. **Not** parking the windows of an app that
      has no others: hiding is one call, the app's Dock icon says it's
      hidden, and it's what Bench did before. **Not** a toast every time:
      once the user knows why, it's noise. **Not** the button for an app
      with items or a tab rule: a float rule keeps the app's new windows,
      and windows that open again, out of those items, and wins over
      every tab rule (`matchRule`). **Not** a rule for the one
      window, by title: the app's other windows would still go, and
      titles change. **Not** showing parked windows again when the
      setting changes to `leave`: they come back as a hidden app does,
      when the app comes forward. **Not** `told` in `config.toml`: it
      records what Bench said, which isn't a setting.

69. **A pane says what keeps moving its window, and the status bar says
    what a window tool can do to Bench.** Bench puts back a docked
    window that its app moves, three times in ten seconds (decision 13),
    then left it where it was, with no word, until the tab was next
    arranged: the window seemed to come loose for no reason. Window tools,
    such as Rectangle, Magnet, and Moom, move docked windows too, with
    their shortcuts and automatic rules, and their shortcuts can use
    Bench's keys with no warning. Their snapping acts only on the user's
    own drag, which undocks the window first ("Moved out of its pane").
    - **The mark.** When a window uses up its tries
      (`redockSoonLocked`), `keptMovingLocked` (`events.go`) marks its
      item as undocked, with `keptMoving` set, and the status `moving`, as
      if the user had dragged it out: it isn't docked, a tab switch parks
      it on other tabs, and **Dock again** and a drop onto a pane dock it.
      The manager forgets it (`Manager.Forget`), as for a window the user
      drags out. Its pane says "*App* or another tool keeps moving this
      window", or, with window tools that Bench knows running, names them
      instead of "another tool", as in "Zed or Rectangle keeps moving this
      window" (`keepsMovingText`), and its sub-tab has the warning marker.
      The status bar counts it among the windows that need attention.
      The mark stays across tab switches and arrangements, so that it
      doesn't come and go. It goes with **Dock again** (`Redock`) and
      **Rearrange**, which give the window its tries again; with a drop
      onto a pane; with the settle after the screens change, as macOS may
      have made the moves (decision 56); and with the user's own drag of
      the window (`MouseDown`), after which it's plainly undocked:
      "Moved out of its pane". `keptMoving` isn't saved: after a restart,
      Bench docks the window again, with its tries. The tries count only
      while Bench puts the window back into the same pane body
      (`redockTries`): an app that nudges its window after each resize of
      the panes, as **Grow Pane** pressed again and again gives, isn't
      fighting Bench, and a try at a new body starts the count again.
    - **The tools.** `windowtools.go` keeps one table of the window tools
      that Bench knows, by bundle ID, with their names. A bundle ID that
      nobody has checked on a Mac with the tool installed is marked
      `unsure`, and `TestWindowToolsTable` checks that
      [development.md](development.md) lists each one, to check. A tool is
      running when `Platform.RunningApps` lists it, which includes apps
      with no Dock icon, as most window tools are. macOS doesn't post
      `NSWorkspaceDidLaunchApplicationNotification` for those, so Bench
      looks at start, at each arrangement, as it becomes active, and as
      apps open and quit (`noteWindowToolsLocked`), and keeps those
      running in `toolsOn`. A pane names the tools running, other than the
      window's own app.
    - **Why macOS gives no error.** Wails registers Bench's global
      shortcuts with `RegisterEventHotKey` and no options, so not
      exclusively, and the only clash it names is `eventHotKeyExistsErr`
      (-9878), which macOS gives when another app holds the shortcut
      exclusively. Rectangle, and the other tools that use MASShortcut,
      register theirs without options too, so both registrations work:
      Bench gets no error, so the registration alone shows no problem
      (decision 45), and both apps may act on the key.
    - **The notice.** While a tool runs, the status bar has a notice
      about it, in short sentences that end with what to do, so that the
      first two still make sense when the status bar cuts it short:
      "Rectangle is running. It can move docked windows out of their
      panes. It uses the same keys as Bench for Focus a pane (⌃⌥←, ⌃⌥→,
      ⌃⌥↑, ⌃⌥↓), Move a window (⌃⌥⇧↑), and Zoom pane (⌃⌥↩). macOS doesn't
      warn about this, and both apps may act on these keys. Change the
      keys in Rectangle or in Keyboard settings." It has **Keyboard
      settings**, which runs `keyboard-settings`, and **Dismiss**
      (`DismissWindowTool`). It goes when the tool quits, and comes back
      when the tool runs again, so that a tool that quits before the user
      reads the notice gets it again. **Dismiss** adds the tool, by
      lower-case bundle ID, to `toolsTold`, which `state.json` saves as
      `windowTools`, and then the notice never comes back. The keys the
      tool shares with Bench are read as it starts, and again each time
      Bench becomes the active app, as the user may have changed them in
      the tool, and matched with Bench's shortcuts as each View is built,
      so the notice follows changes in both apps: a field with no shared
      key isn't named, and with none at all, the notice says only that the
      tool's shortcuts "can use the same keys as Bench's", never that
      there's no clash. The notice has a `key`, `window-tool:` and the
      lower-case bundle ID, which stays the same as its text changes, so
      that the recent messages keep one copy of it, the latest
      (`noteNotices` in `state.js`).
    - **Settings ▸ Keyboard.** `WindowToolKeys` gives the shell the keys of
      the tools running whose shortcuts Bench could read, and when they
      change, the engine emits `ShortcutsChanged`, so that the shell
      checks Bench's shortcuts again (`assess` in `internal/ui`). A field
      that has some of them has the problem `problemTool`, "Rectangle uses
      ⌃⌥↩ too. Both apps may act on this key.", in place of "Works.", and
      a suggestion that avoids them, as it avoids the other apps' keys
      (decision 45). It comes after the shell's other problems, as macOS
      doesn't refuse the key, and adds no notice of its own: the tool's
      notice says it already, until it's dismissed, and the field says it
      after.
    - **Reading Rectangle's shortcuts.** Only Rectangle's can be read
      reliably. `Platform.ToolPreferences` reads a tool's preferences
      domain with `CFPreferencesCopyMultiple`, after
      `CFPreferencesAppSynchronize`, for the current user and any host
      (`benchCopyToolPreferences` in `keys_darwin.m`): numbers, strings,
      and MASShortcut's dictionaries, `keyCode` and `modifierFlags` (Cocoa
      modifier flags), each read with `integerValue`, as MASShortcut reads
      them, or any other dictionary, such as an empty one, for a shortcut
      set to none. A value of another kind, such as data, comes as
      `BENCH_PREF_OTHER`, and `toolprefs.go` records it as set, with no
      shortcut, so that Bench doesn't take the action's default for it.
      `toolprefs.go` turns them into canonical shortcuts, as
      `SystemKeys`'s symbolic hot keys (decision 45). Rectangle keeps a
      shortcut only once the user changes it. For the others, it uses one
      of two sets, which the user chooses as it first opens, and records
      in `alternateDefaultShortcuts`: 1 for its recommended set, with
      ⌃⌥ and the arrow keys, 0 for Spectacle's. `rectangleActions` has
      both sets, from Rectangle's `WindowAction.swift`, with the names
      that some actions had before, which Rectangle still reads, and
      `centerTwoThirds`'s ⌃⌥R, which only a Rectangle first installed
      after version 94 has (`installVersion`). Without
      `alternateDefaultShortcuts`, as before Rectangle first opened, or
      when Bench can't read the domain, Bench doesn't guess, and says only
      that the shortcuts can use Bench's keys. Other tools keep their
      shortcuts in formats of their own, or in an App Store app's
      container, so Bench names no keys for them.
    - **The browser preview.** The demo has Rectangle installed, as an app
      with no Dock icon, with the recommended set chosen, not running. The
      demo bar's **Simulate** menu holds the buttons that act as macOS or
      another app would (`simulate.js`), so that the bar's hint still fits
      beside them. Its **Rectangle** starts and stops the demo's Rectangle
      (`SetDemoWindowTool`, with `StartAgent` and `StopAgent` on the fake
      platform), and is pressed while the View's `windowTools` has it.
      **Keep moving window** (`DemoToolMoves`) moves the focused pane's
      window to the left half of the screen, once more than Bench puts it
      back, a second apart (`demoToolGap`), well past the redock and the
      time in which a move is taken as Bench's own (`ownRecent`), and
      waits while the screens settle, as their settle docks the window
      again. `platform.Logged` passes `AppMoveWindow`, `StartAgent`, and
      `StopAgent` on to the demo platform, so both work with `BENCH_DEBUG`
      set.
    - **Not** registering Bench's shortcuts exclusively: Wails doesn't
      offer it, and it would depend on which app started first, so that
      Bench would either fail to register or silently take the keys from
      the tool. **Not** checking the tools' snapping, edge tiling, or
      Spaces settings, or macOS's own tiling: what they do changes between
      versions, and the mark says when it matters. **Not** counting tries
      across pane bodies: a resize of the panes isn't a fight. **Not** a set of keys
      in the welcome guide that avoids every tool's. **Not** guessing the
      shortcuts of tools whose preferences Bench can't read, nor saying
      "no clash" when it finds none. **Not** recording the tool as told
      when its notice first shows: a tool that quits at once would take
      its notice with it, unread. **Not** a notice of its own for
      `problemTool`: it would say again what the tool's notice says, and
      come back after **Dismiss**. **Not** `windowTools` in `config.toml`:
      it records what the user dismissed, which isn't a setting.
70. **Bench goes back to the display that the user chose when it comes
    back.** When a display goes away, because the user unplugs it or macOS
    turns it off while it sleeps, macOS moves its windows to another
    display. Bench filled that display (decision 30), and recorded it as
    the display it was on after each settle, so when the monitor came back,
    Bench stayed on the laptop's screen.
    - **The display chosen.** The engine keeps a second display apart
      from `lastScreen`: `preferred`, which `state.json` saves as
      `preferredScreen`, with the same fields. It's the display that the
      user last chose for the window: the one Bench's window is on when the
      user moves or resizes it within a display, or fills the screen on it,
      while the screens are settled (`onBenchMoved`), and the one the user
      moved it to from another display, at the end of the settle that the
      move starts (`screensSettledLocked`). That holds while the display
      chosen before is away too: a user who closes the lid and works on the
      monitor, moving Bench there, keeps it there when the lid opens. A
      settle that the displays changed in (`displaysChanged`, set by
      `ScreensChanged`) doesn't choose, because macOS moved the window
      then; neither does a settle after Bench's own move (`ownMove`), nor
      the zoom with which Bench fills the screen again after the displays
      change (`refillLocked`): its moves animate after the settle has
      ended, so until the window fills the screen, or for 2 seconds at
      most (`ownFill`), a move within the screen is the zoom's
      (`noteUserMoveLocked`). Until the user chooses, the display Bench
      first opens on is the one chosen (`WindowStart`), and a `state.json`
      from before this decision takes its `screen`. Choosing a screen in
      the browser preview's screen picker chooses it.
    - **Away and back.** At each `ScreensChanged`, if the display chosen
      isn't connected, it's away. It's connected when a display has its
      ID, or else its name and frame together (`connectedScreen`), as
      `findScreen` finds them: macOS can give a display a new ID, and two
      displays of the same model have the same name, so the name alone
      doesn't do, nor the frame alone, which another display can have. At
      the end of a settle that the displays changed in, the display chosen
      is back (`returnDue`) if it's connected and was away, or if the
      window was on it before the settle and isn't now. The second is for
      a display that goes and comes back before the engine reads the
      displays without it, so that it never sees it away: as when both
      `ScreensChanged` events wait while the engine is busy, or macOS
      sends one for both, as it can across a wake.
    - **Going back.** `refillLocked` then calls `goBackLocked` before it
      fills the screen again. If the window is on that display already,
      as when macOS has put it back, as macOS 27 can, there's nothing more
      to do, and no toast. Otherwise the engine asks the shell to move the
      window (`Emitter.MoveToScreen`): to where it was on that display, if
      it didn't fill it and that's still inside the display's visible area
      (`openFrame`), or else to the usual size in the middle, then filling
      the display if it filled it. It records the move as
      `noteBenchPlaceLocked` would, and the display's ID, which may be new,
      and waits for the screens to settle again (`moveBenchLocked`), so
      that the moves macOS makes meanwhile aren't the user's, and Auto
      chooses the layout again for the display (decision 58). The toast
      says "Back on DELL U2723QE.", with the display's name, as macOS
      gives it, and **Undo**.
    - **The shell's move.** `appShell.moveToScreen` gives the main
      window's frame, from `mainPlace`, as at launch, to
      `mac.Platform.PlaceWindow` (`Options.Place`), which sets it on the
      main thread in one step, without animation, then, to fill, zooms the
      window at once with NSWindow's own `zoom:`, as `FillWindow` does at
      launch. So the window jumps to the other display once. Wails'
      `SetSize` would change its size first on the display it's leaving,
      then `SetPosition` would move it, and Wails' `Zoom` would animate.
      Without `PlaceWindow`, the shell still uses those, size first, since
      `SetSize` keeps the window's bottom-left corner where it is.
      `moveToScreen` and `fillScreen` run one at a time, in the order the
      engine asks for them (`appShell.moves`): each waits for the main
      thread more than once, and an Undo straight after the move back
      would otherwise put its frame in between the move's frame and its
      zoom, and `zoom:` goes back and forth.
    - **What waits.** While a hold after `last_tab`'s shortcut (decision
      60) or a tab switch that waits for an app to open is running, or one
      of Bench's menus or dialogs is open (`menuOrDialogLocked`), the move
      waits, and looks again every 250 ms (`returnRetry`), so that the
      tabs don't move from under the user, nor a dialog, nor a native
      menu, which stays where it opened (decision 71). While the window is hidden
      or minimised, it waits until it's shown (`refillOnShow`), as filling
      does. A settle that starts meanwhile takes over.
    - **Undo.** The toast's **Undo** takes the move back
      (`undoWentBackLocked`): the window goes back to the display and place
      it was on, filling it if it filled it, the toast says "Back on
      Built-in Retina Display.", and that display is the one chosen, as
      the user has chosen it by undoing. While the window is hidden or
      minimised, the move waits until it's shown (`placeOnShow`), as the
      move back does. The move isn't one of the changes in Undo's history
      (decision 37): the engine keeps it apart (`wentBack`), with a number
      from the same count, so Tab ▸ Undo, the jump palette, and the `undo`
      shortcut (`UndoLatest`, decision 65) don't take it back, and the
      View's `undo` stays the user's latest change. The user didn't make
      the move, and taking it back by mistake would move the window and
      change the display chosen. Undo of an earlier change leaves the
      window where it is. The View's `wentBack` names the move, **Move to
      Display**, "moved Bench back to DELL U2723QE", for the recent
      messages to keep its **Undo** (decision 65). It goes once the user
      chooses another display, or the display chosen goes again. If the
      display the window came from has gone, as when the lid has closed,
      Undo says "Built-in Retina Display isn't connected, so Bench can't
      move back to it.", and forgets the move.
    - **The browser preview.** The demo bar's **Simulate ▸ Unplug
      monitor** (`SetDemoMonitor`) needs the monitor, as **Monitor**
      gives it. It takes it away: the laptop's screen is the only one, and
      Bench's window goes to it. It then reads **Plug in monitor**, which
      puts the monitor back to the right of the laptop's screen. Once the
      screens settle, Bench goes back to it (`MoveDemoBench`), with the
      toast and its **Undo**. The View's `demo.unplugged` says which the
      button shows. The fake platform has `UnplugScreen` and `PlugScreen`
      for the engine's tests, and `platform.Logged` passes the demo's
      methods on.
    - **Not** a menu item, palette row, or setting: going back is what
      the user expects, and the toast's Undo takes it back. **Not** an
      Undo in Tab ▸ Undo or the jump palette: see **Undo**. **Not**
      ignoring the user's moves while the display chosen is away: a move
      the user makes is a choice, whichever displays are connected; only
      macOS's moves and Bench's own aren't. **Not** recording the
      display chosen on every settle, which was the problem. **Not**
      going back to a display that's only like the one chosen, with the
      same name somewhere else: it may be another of the same model.
      **Not** keeping a place for each display: one display chosen, with
      the place on it, is enough to go back. **Not** telling the user's drag
      from macOS's move while the screens settle after the displays
      change: Bench can't, so a drag to another display then counts as
      macOS's, and Bench still goes back if the display chosen is back;
      Undo puts it right.
71. **Bench's own menus are native macOS menus in the app, so web pages
    and docked windows stay in view.** Each menu that Bench drew in HTML
    raised Bench over the docked windows and hid every web view (decision
    4), so a right-click on a web page's sub-tab blanked the page, and the
    docked windows went behind Bench, until the menu closed. Menus open
    often, and that was the most visible cost of raised mode. AppKit draws
    a native menu above every window, so it needs neither. This decision
    replaces the part of decision 4 that is about menus.
    - **Which menus.** A tab's menu, a sub-tab's and the **⋯** window
      menu, an empty pane's menu, a terminal's right-click menu, and the
      slim title bar's layout menu. The status bar's lists of the windows
      that need attention and of notices and recent messages have buttons
      and text that wraps, and are lists more than menus: they stay in
      HTML, raised. So does a popped-out terminal's menu: the shell opens
      native menus in the main window, and the terminal's own window has
      no web views or docked windows to hide. Dialogs, drags, and live
      previews keep raised mode.
    - **One model of entries.** `menu.js` builds each menu's entries as
      data, the same for the HTML menus (`renderEntries`), the jump
      palette's **This window** (`windowRows`), and the native menus
      (`plainEntries`), from one function for each menu (`menuEntriesOf`),
      so that their names and submenus can't differ. An entry has an `id`,
      a `label`, and can have `keys`, a `detail`, a `dot` (a tab's colour,
      decision 74), `disabled`, `checked` (one of a choice, ticked if
      true), and a `submenu`; or it's a
      separator (`sep`), a `heading`, a `note`, or a `status` line. The
      frontend keeps what each entry does (`run`) to itself. Each `id` is
      unique in its menu: a fixed name, such as `split-right` or
      `page-find`, or a name with what the entry acts on, such as
      `move-to-tab:2`, `move-to-pane:p4`, or `layout:wide`.
    - **Showing a menu.** `ShowMenu` takes the entries without their
      `run`, and the point for the menu's top left corner, in the points
      of the UI's web view. The engine refuses where it has no
      `Options.ShowMenu`, as in the browser preview, shows nothing for a
      menu of no lines, refuses one of more than 1,000, and calls the
      hook without its lock, with the menu counted as open (see **Focus
      while a menu is open**). It returns the `id` that comes back only for
      an entry that's on, has no submenu, and isn't in a submenu that's
      off. The shell (`internal/ui/contextmenu.go`) makes the entries into
      a native menu's items (`nativeItems`). An item's label has the
      entry's detail in brackets, as "Reopen Closed Tab (Research)", and
      its keys after it, as the app menu shows Bench's global shortcuts
      (decision 46): no item has a key equivalent, so a page's ⌘ keys stay
      its own. The frontend sends only Bench's global shortcuts: keys that
      work only where the menu is, such as a terminal's ⌘C (`localKeys`),
      are left out, as Mac context menus show no keys, and the HTML menus
      show them. A long label is cut at 60 characters, and a detail at 40,
      with an ellipsis, as the HTML menus cut them with CSS. A native menu
      has no headings, notes, or status lines, so they are items that are
      off, which VoiceOver reads as dimmed; a note or status line longer
      than 60 characters is on more than one item, broken between words,
      as the HTML menus wrap it. Separators go only between items. A
      submenu none of whose items is on is off. `buildNativeMenu` makes
      the items into Wails' menu items, each of which reports its `id`
      when it's clicked, and has the image of its entry's `dot`, if it has
      one.
    - **Wails' context menus, not a bridge in `internal/mac`.** Wails v3
      beta.27's context menus can show a menu built as the user opens it,
      at a point, and report the item chosen. `application.NewContextMenu`
      registers one menu, `bench-menu`, whose items the shell builds again
      for each menu, and `WebviewWindow.OpenContextMenu` pops it up in the
      main window's web view (`popUpMenuPositioningItem:atLocation:inView:`),
      whose coordinates start at its top left, as the page's do, and
      returns when it has closed. AppKit keeps the menu on the screen near
      its edges, and opens a submenu on the side that has room. The items
      of the menu shown before are destroyed (`Menu.Destroy`) as the next
      one opens, long after that menu's choice has come. A bridge would
      need its own menu delegate, item lifetime, and way back to Go, for
      nothing more.
    - **The choice.** AppKit calls the item's action before the menu's
      call returns, but Wails gives the click to Go through a channel and
      two goroutines, so it can arrive a moment after `OpenContextMenu`
      returns. `contextMenu.show` waits up to 250 ms for it
      (`choiceWait`): a menu closed with nothing chosen answers after that
      wait. Each menu has a channel of its own, so a late choice can't
      reach the next menu, and one menu shows at a time
      (`contextMenu.mu`). See [Threads and locking](#threads-and-locking).
    - **Focus while a menu is open.** A native menu doesn't raise Bench,
      but AppKit closes it as soon as another app becomes active, and the
      engine read raised mode (`Engine.overlay`) as "a menu or dialog is
      open" to hold back what would focus another app's window. So the
      engine counts the native menus open (`Engine.menus`, from
      `openMenuLocked` to `closeMenuLocked`), and `menuOrDialogLocked`
      reads either. While one is open, a tab switch's focus that waits
      for a window to open isn't made (`waitFocusDueLocked`), and opening
      the menu drops it for good, as the user has turned to the menu. The
      App window that the jump palette took the keyboard from gets it back
      (`refocusAfterJumpLocked`) only once the menu closes: closing it
      arms the 150 ms wait again, as lowering Bench does, so that
      **Window Options…** from the palette, over a docked app, stays open,
      and Escape then gives the app the keyboard back. The hold after
      `last_tab` doesn't start, and the jump shortcut keeps its note.
    - **The frontend.** The View's `nativeMenus` says that the shell has
      native menus. Each menu's opener goes through `openMenu`, which
      shows the menu natively (`showNative`) or draws it in HTML. A
      right-click opens a native menu at the pointer; a click, or the
      keyboard (Shift-F10, Control-Return, or the menu key), opens it
      below its control, at the control's left, as a pop-up button's menu
      opens. `ui.nativeMenu` holds the menu while it's open: it isn't
      drawn, and `overlayOpen` leaves it out, so the frontend doesn't send
      `SetOverlay(true)`, the web views stay shown, and Bench stays below
      the docked windows. The control that opened it is marked as
      expanded meanwhile (`syncExpanded`). When it closes, the frontend
      builds the menu's entries again, from the View as it is then, and
      runs the entry with the `id` that came back, if the menu's subject
      is still there (`staleMenu`, which follows a tab's menu to its tab)
      and the entry has the label that it had when the menu opened. The
      focus goes back to the control that opened the menu, and with
      nothing chosen, the click that opened it does what it would have
      done in its pane, as `closeMenu` does for an HTML menu. One native
      menu shows at a time (`ui.nativeMenu`), and a web page's can wait a
      moment before it opens: a click that would open another meanwhile
      does nothing, in its pane too. So each opener hands `openMenu` what
      its click does in its pane (`later`), which a native menu keeps with
      itself (`m.later`), not in `ui.paneLater`, where a refused click
      could replace it. If `ShowMenu` fails, the menu opens in HTML
      instead.
    - **The web app entry.** A web page's HTML menu takes its web app
      entry when Go answers `WebAppChoice` (decision 22). A native menu
      can't take an entry once it's open. Go scans the installed apps for
      each answer, which can take longer than a menu may wait to open. So
      once Go has answered for the page, the menu opens at once with that
      answer (`webAppAnswers`), and asks again, keeping the new answer
      for the next time. Before Go has answered once, as with the first
      menu after Bench opens, the menu waits for the answer, for 300 ms
      at most (`WEB_APP_WAIT_MS`), and then opens without the entry; the
      user guide says to open the menu again. An answer kept from before
      can be out of date, as when the web app was made since: the entry
      then does what Go does with it now. Keeping a web page's menu in
      HTML was considered and not chosen: it's the menu where hiding the
      web views matters most.
    - **The Page submenu.** The row of buttons of a web page's Page group
      (decision 50), **Back**, **Forward**, and the zooms, has no native
      form, so it's a **Page** submenu, in the HTML menus too, with
      **Pane ▸ Page**'s names and order. **Find in Page…**, **Copy Page
      Link**, and the microphone, camera, and sound entries stay in the
      menu itself, after it, as before: muting a call can't wait for a
      submenu. Decision 76 puts **Keep Running in the Background** last
      among them, an entry with `checked`, which the native item shows as a
      checkmark. The jump palette names them all **Page ▸** *entry*, as the
      menu bar's **Pane ▸ Page** has them (`under`).
    - **Move to Pane is a submenu.** Decision 63 kept **Move to Pane** a
      heading over the panes, named **Pane with** *window*. A native menu
      has no headings: the heading became an item that's off, which read
      as a command that can't be used, and the panes' items, which have
      no verb, as going to a pane. So **Move to Pane** is a submenu, as
      **Move to Tab** is, in the HTML menus too, and the jump palette
      names its items as before ("Move to Pane ▸ Pane with Slack").
    - **Accessibility.** macOS's menus work with the keyboard (the arrow
      keys, typing an item's first letters, Return, and Escape) and with
      VoiceOver, which reads each item, says which are dimmed, and says
      which have a submenu. They follow light and dark, Increase contrast,
      and Reduce transparency, as every Mac menu does. In the smallest
      window, 640 by 480 points, a native menu can go past the window's
      edges, onto the screen, where the HTML menu has to scroll.
    - **With decisions 67 to 70.** A native menu's choice runs the entry
      of the same model as an HTML menu's, so what it does is the same: a
      tab's **Rearrange** for a tab that isn't current switches with
      `switchTab`, which shows the tab as current at once (decision 67),
      and its arrangement is timed with the same reasons, `tab` and then
      `rearrange`, as `ShowMenu` arranges nothing and the bound methods
      don't say where a call came from. **Dock Again** for a window that
      keeps moving out of its pane (decision 69) is an entry of the model,
      `dock-again`, in the native menu and the palette too, after the
      menu's status line, which says why. Bench puts away the windows that
      cover it as it becomes active (decision 68), not as it's raised,
      and a native menu does neither: opening one puts nothing away, and a
      click on Bench, or a dialog that the shell brings Bench forward
      for, still does. The move back to the display that the user chose
      (decision 70) waits while a menu or dialog is open
      (`menuOrDialogLocked`), as for a hold, so that Bench's window
      doesn't move from under a native menu, which stays where it opened.
      The status bar's lists, with the window tools' notice and **Keep
      over Bench** in the recent messages, stay in HTML, raised; the
      browser preview's **Simulate** list on the demo bar isn't one of
      Bench's menus, and is as it was.
    - **Raised mode now** is for what Bench draws over the docked windows
      and web views itself: dialogs, the jump palette, drags, live
      previews, and the menus and lists drawn in HTML.
    - **Not** native menus in the browser preview: a browser page has
      none, so the preview draws the same entries in HTML. **Not** key
      equivalents on the items: the label shows them, as in the app menu,
      and the page's ⌘ keys stay its own. **Not** changing an open native
      menu as the View changes: the entries stay as they were when it
      opened, as an HTML menu's don't move under the pointer (decision
      22), and the choice is checked against the View once the menu has
      closed.

72. **One terminal can be the drop-down, which slides down from the top
    of the screen over any app, in a panel that doesn't make Bench
    active.** A terminal for a quick command is most useful where the
    user is, without a switch to Bench or to the terminal's tab. So one
    terminal at a time can leave its pane and become the drop-down: a
    shortcut, a menu item, or the jump palette shows it over the app in
    front, on any desktop and over a full-screen app, and the same key,
    ⌘⎋, Escape at its prompt, or a click elsewhere hides it again and
    gives the keyboard back.
    - **Terminals only, not web pages.** A web page's view is a
      `WKWebView` that `web_darwin.m` docks in Bench's main window, or in
      the ordinary `NSWindow` that `popOutWebView` makes for it, with
      Bench's own title bar button. A panel for it would need a second
      native window path, with its own focus, first responder, and
      title bar handling, which can't be built or tried without a Mac.
      A terminal's own window is a Wails window with the popped-out
      terminal's page (`terminalURL`, `terminal-window.js`), so the
      drop-down reuses it, with `dropdown=1` in its address, and needs
      only a panel's options and two native calls. **Use as Drop-down**
      is in a terminal's menus only, and `UseAsDropdown` refuses anything
      else: "Only a terminal can be the drop-down."
    - **One at a time, out of the tabs.** The drop-down is a Built-in
      item in `Model.Floating`, with a window of its own (`OwnWindow`),
      that `Model.Dropdown` names. It counts only while it's still a
      terminal in `Floating` (`dropdownLocked`): `Take` forgets it as it
      leaves `Floating`, and `leaveDropdownLocked` does in
      `bringBackLocked` and `endItemLocked`, with the note that its
      terminal shows (`dropKeys`), so that Escape in it no longer stops
      to look for the drop-down. So every way
      back into a pane, **Move into This Pane** (`AddToPane` with the
      drop-down as a floating source), **Back to Bench**, and the others
      that bring a floating item in, ends it, with no code of its own.
      **Use as Drop-down** on another terminal puts the drop-down that
      was into the pane that the new one leaves, before it leaves, so
      that the pane stays, or into the current tab's focused pane. A
      pane that the new one leaves empty closes, as after **Pop Out of
      Bench**. Once the drop-down is back in a pane, it isn't the
      drop-down: the next show makes a new terminal in the home folder
      the drop-down, and **Use as Drop-down** makes it the drop-down
      again.
    - **Not a window that waits for a tab.** The drop-down is in
      `Floating`, but **New Tab from Open Windows** (`windowChoices`,
      and `windowAppLocked`, which refuses it) and the jump palette's
      **Not in a tab** (`outsideTabs`) leave it out, so that a new tab
      doesn't take it without a word, and choosing a row doesn't end it.
      The add dialog's **Windows not in a tab** still has it, as one
      item the user chooses for one pane, as **Move into This Pane**
      does.
    - **The panel.** `dropdownWindowOptions` makes a Wails window of
      class `MacWindowClassPanel`, with `FloatingPanel` and
      `NonActivating`, frameless, not resizable, and hidden at first, on
      every Space and beside full-screen apps (`CanJoinAllSpaces |
      FullScreenAuxiliary`), and out of the windows that ⌘&#96; goes
      through (`IgnoresCycle`). A non-activating panel takes the
      keyboard without making Bench the active app, so the app in front
      keeps its menu bar and stays in front, and gets the keyboard back
      when the panel orders out. `benchShowDropdown` puts the panel at
      `NSFloatingWindowLevel + 1`, above Bench's main window in raised
      mode and below the menu bar, sets `canHide` to `NO`, so that it
      shows while Bench is hidden, and gives the page's web view the
      keyboard. It places the panel across the top of the visible frame
      of Bench's screen, the one its window is on, or was on while it's
      hidden (`benchDropScreen`), as wide as that screen: the drop-down
      belongs with Bench's tabs, and comes in the same place each time.
      Without Bench's window, it's the screen with the keyboard
      (`NSScreen.mainScreen`). The panel isn't tagged `bench.item:`, so
      it isn't a popped-out window for **Add Front Window** and **Window
      ▸ Back to Bench**; its bar has **Back to Bench** itself. The
      page's body is named "Drop-down terminal" for VoiceOver, and its
      bar names the terminal ("Drop-down: *name*") and has **Back to
      Bench** and **Hide**.
    - **Hiding from the page.** The page hides the drop-down when its
      window loses the keyboard (`blur`), as a click in another window
      or app takes it: the panel, at the floating level, would go on
      covering the top of that app, without the keyboard, which Escape
      and a shortcut used only in the panel would then not reach. While
      the close question shows, it stays, as the question waits for an
      answer. The page also hides it for ⌘⎋ (`DROPDOWN_HIDE_KEYS`), in a
      capture listener, before the terminal sees the key, so that a key
      in the panel hides it while a program has Escape, and while the
      `dropdown` shortcut is off. **Hide**'s tooltip and the right-click
      menu's **Hide Drop-down** show ⌘⎋, which works in the panel only,
      not the shortcut, which they can't keep up to date. The browser
      preview hides its drop-down for ⌘⎋ in it and for a press outside
      it, except while a dialog or menu is open over it.
    - **The slide.** The panel slides down from above the top of the
      screen in 0.18 s (`NSAnimationContext`), and up again as it hides,
      ordering out once it's out of sight, unless it has shown again
      meanwhile (`gDropSerial`). Where another screen is arranged above
      Bench's, so that the frame above the top is on that screen
      (`benchDropCanSlide`), it fades in and out over the same time
      instead (`alphaValue`): a slide would show it on that screen
      first. With **Reduce motion** on
      (`accessibilityDisplayShouldReduceMotion`), it's put in place, or
      ordered out, at once. The frame and the alpha are set before the
      panel is ordered front, so that it never shows for a moment in the
      wrong place. The browser preview's slide is a CSS animation, which
      `prefers-reduced-motion` turns off.
    - **The first show.** Wails gives a hidden window its `NSWindow`
      only once its page has loaded the runtime, so the shell keeps the
      height that the first show asked for (`dropdownWindow.want`) and
      slides the panel down when it has one (`onceNative`,
      `dropdownReady`). Hiding
      orders the panel out and keeps it: the next show slides the same
      panel down, with the terminal's screen as it was. The session is
      Go's, so it runs whether the panel shows or not.
    - **Height: a setting, not a dragged edge.** `dropdown_height`, in
      **Settings ▸ Terminals**, is the panel's height in percent of the
      screen's, from 20 to 90, 40 by default, kept in `config.toml`. A
      height kept from a dragged edge would need a resize handle in a
      frameless panel, a delegate to report the size, a `state.json`
      field, and a height for each screen; a percentage fits every
      screen as it is. The owner allowed either, and the setting is the
      simpler. A new height applies the next time the drop-down shows.
      Settings refuses one outside 20 to 90: "The drop-down's height
      must be from 20 to 90 percent of the screen."
    - **Escape.** Go takes it in `TermInput` (`escapeDropdown`): an
      Escape alone (`\x1b` as one piece of input), while the drop-down
      shows and its terminal isn't busy (`Host.Busy`, decision 35),
      isn't typed at once. Go holds it for 0.4 s (`escapeWait`). If no
      other key comes in that time, the drop-down hides, and the Escape
      is never typed. A key in that time goes to the shell after the
      Escape, so that the Escape prefix that shells read as Meta, such
      as Escape B for a word back with Option as Meta off, still works,
      and the drop-down stays. A second Escape hides it at once. Any
      other show or hide forgets a held Escape. The hold has a mutex of
      its own (`escMu`), which may be taken with the engine's lock held,
      never the other way round, so that the first Escape and the key
      after it don't wait for an arrange. While a program runs, such as
      vim, less, or a coding agent, Escape is the program's. Keys that
      send Escape with more, such as the arrows and Option as Meta, come
      as one piece, so they're typed. The page joins what's typed while
      a `TermInput` is on its way into the next one, but never an Escape
      alone, which goes on its own (`send` in `terminals.js`), so that a
      quick Escape after typing still hides the drop-down. An atomic
      (`dropKeys`) says which terminal is the drop-down that shows, so
      that Escape in another terminal takes no lock. In the page, Escape
      with the focus on the bar, not in the terminal, hides it too. A
      shell in vi mode at its prompt gets Escape only with a key that
      follows it in the hold: Escape alone hides the drop-down.
    - **The shortcut.** `dropdown` in `[shortcuts]` is off by default,
      as Option+&#96;, the obvious key, is a dead key on many keyboard
      layouts. It's bound after `undo` and before
      `send_to_tab_modifiers` (decision 65), and checked for clashes as
      every shortcut is (decision 45). It calls `ToggleDropdown`, which,
      unlike the jump shortcut, never brings Bench forward. **View ▸ Show
      Drop-down**, the menu bar item's **Show Drop-down**, and the jump
      palette's call it too, and read **Hide Drop-down** while it shows.
    - **Needing the user while hidden.** `seenLocked` counts the
      drop-down as seen while it shows, over any app, even with Bench
      hidden, and not while it's hidden, so its bell, a notification
      that its program asks for, or its command finishing marks it as
      a terminal out of sight is marked (decision 43), and
      `clearSeenLocked` keeps that mark though the drop-down is out of
      the tabs. The menu bar item has no sub-tab to mark, so a bell
      follows its text (`dropdownMark`), and its menu's item, and **View
      ▸ Show Drop-down**, read **Show Drop-down — Needs Attention**. The
      notification's subtitle is "Drop-down", where a terminal in a tab
      has its tab's name, and clicking it shows the drop-down
      (`showAnywhereLocked`). Showing it takes the mark off.
    - **Between launches.** `state.json` keeps the drop-down's item ID
      (`dropdown`), with the item among the floating ones. As Bench
      opens, `reopenOwnWindowsLocked` starts the drop-down's session,
      out of sight (`dropdownFromStateLocked`), as it was when Bench
      quit, and its panel is made the first time it shows.
    - **Closing it.** ⌘W in the panel closes it, which ends the
      terminal, asking first while it's busy, as a popped-out terminal's
      window does (decision 35).
    - **The browser preview** has no panel, and its shell's
      `showDropdown` does nothing. The View's `dropdown.inPage` is set,
      and the page draws the drop-down over the top of Bench's window,
      `dropdown_height` percent of its height tall, with the same bar,
      and gives the keyboard back to what had it as it hides. The
      shortcut works only while the page has the keyboard, as every
      shortcut does there.
    - **With the decisions before it.** The panel is one of Bench's own
      windows, of Bench's own process (`ownPID`), so Bench never adopts
      it, puts it out of sight as a window that covers Bench as Bench
      comes forward (decision 68), or parks it before it shows an app
      (decision 67). The drop-down terminal is in no pane, so it has no
      frame to be docked back into, and Bench never redocks it or marks
      it as kept moving (decision 69); a window tool that moves the
      panel moves it until it next shows, when `benchShowDropdown` puts
      it in its place again. Showing and hiding it, from the `dropdown`
      shortcut too, arrange nothing, so they keep no time; **Use as
      Drop-down**, which takes the terminal out of its pane, is timed as
      `other` (decision 67). The `dropdown` shortcut is checked against
      a running window tool's keys as the others are (decision 69): ⌃⌥D,
      which the Mac checks give it, is Rectangle's **First Third**, so
      Settings ▸ Keyboard says so, and the tool's notice names it last,
      as it binds after Bench's other shortcuts. The panel goes to the
      display that Bench's window is on each time it shows, so after
      Bench goes back to the display that the user chose (decision 70),
      the next show is there. A drop-down that shows while Bench goes
      back stays where it is, so that it doesn't move while the user
      types in it. If the display that it shows on goes away, macOS
      moves the panel onto another display at the size it had, so once
      the screens settle, Bench shows it again, which puts it across the
      top of Bench's display (`dropdownScreensLocked`, with
      `dropScreen`, the display that Bench's window was on as it last
      showed).
    - **Not** web pages, as above. **Not** a height from a dragged edge,
      as above. **Not** more than one drop-down. **Not** **Undo** for
      **Use as Drop-down**: its toast says how to show it again, and
      **Move into This Pane** puts it back. So Undo of another change
      leaves the drop-down as it is (`restoreLocked`): the arrangement
      before that change can have the drop-down in a pane, or not have
      it at all, and bringing that back would end the drop-down without
      a word, or close its terminal as something the change added. The
      drop-down stays out of the tabs, and a terminal that was the
      drop-down before that change stays where it has gone since, as
      what has come since does. **Not** a default shortcut.
73. **Files, folders, and apps dropped from the Finder go where they're
    dropped: a terminal types their paths, and a pane or the tabs' +
    opens them.** Dragging a file to a terminal to type its path is what
    Terminal and iTerm2 users do many times a day, and dragging a folder
    or an app to a place in Bench is the quickest way to open it there.
    Before this, Bench's windows didn't take drops of files (Wails'
    `EnableFileDrop` was off, so its runtime cancelled them), and a link
    or text dragged from another app could load in place of Bench's page.
    - **Wails' file drop, not HTML's.** WebKit gives a page the names of
      dropped files, never their paths. Wails v3 beta.27 puts a view
      over the web view (`WebviewDrag` in its `webview_window_darwin.m`)
      that takes drags of files from the Finder, in a window with
      `EnableFileDrop`: the main window, popped-out terminals' windows,
      and the drop-down's panel (`mainWindowOptions`,
      `terminalWindowOptions`, and `dropdownWindowOptions`), and not
      Acknowledgements, whose page cancels every drag over it instead
      (see **Other drops**). While a drag is over it, Wails' native code
      calls `window._wails.handleDragOver(x, y)`, and its runtime gives
      the element under the point that has a `data-file-drop-target`
      attribute, the nearest, the class `file-drop-target-active`, which
      `styles.css` draws. At the drop, it calls
      `window._wails.handlePlatformFileDrop(paths, x, y)`, and the runtime
      sends the paths to Go with that element's attributes. The shell's
      `watchDrops` gets them in `WindowFilesDropped`
      (`DroppedFiles` and `DropTargetDetails`) and calls
      `Engine.DropFiles` with what `DropTargetOf` reads from the
      attributes. A refusal shows in the window that got the drop, where
      the user is looking (`dropped`): a toast in Bench's window, or, in
      a popped-out terminal's window or the drop-down's, the
      `drop-refused` command, with the text, which that window's page
      shows as its own toast. Each refusal is short enough for the app's
      toast, which is one line, in Bench's smallest window, even in the
      title bar while the status bar is hidden, and starts with what to
      do: `TestDropMessagesFit` keeps them to 64 characters.
    - **What takes a drop.** The page marks the elements with
      `dropAttrs` and `setDropTarget` (`drops.js`):

      | Element | `data-file-drop-target` | Drop |
      | --- | --- | --- |
      | A terminal (`.term-host`, and a terminal's pane body) | `terminal`, with `data-drop-item` | Types the paths into the terminal. |
      | A pane's header, an empty pane's body, a pane whose item isn't ready, and the `tabs` view's pane in the title bar | `pane`, with `data-drop-pane` | Adds the folders and apps to the pane. |
      | The tabs' **+** (`.new-tab`) | `new-tab` | Makes a new tab of the folders and apps. |
      | The body of Bench's window, anywhere else | `window` | Says where to drop. |
      | The body of a popped-out terminal's window or the drop-down's | `terminal`, with `data-drop-item` | Types the paths into its terminal, wherever they're dropped. |

      A docked app's pane body has no attribute, as the app's window is
      over it, and the app gets the drop, as it does outside Bench. A Web
      item's page is a `WKWebView` over Bench's web view and its drag
      view, so the page gets the drop, as in a browser. Neither reaches
      Bench, which needs no native code for drops.
    - **Paths typed as Terminal types them** (`shellPath`). Each path
      gets a backslash before every ASCII character that isn't a letter,
      a digit, or one of `-_./+:@%,`, as Terminal does, so that a space,
      a quote, a bracket, `$`, `*`, `!`, and `~` are read as they are,
      and a space after it, so that the next word or path can follow.
      Letters from other alphabets stay as they are, as zsh and bash read
      them so. A path with a control character, such as a new line, or
      bytes that aren't UTF-8, which a backslash can't keep, is written as
      `$'…'`, with `\n`, `\t`, `\r`, and `\x` escapes, and `!` as `\x21`
      so that bash's history doesn't read it; zsh, bash, and ksh read
      that. `TestShellPathInAShell` checks each with bash. Bench never
      presses Return: the user can check the line, and add to it.
    - **Typed as a paste, by the page.** Go sends the text as
      `paste-terminal` (`Command.Text`), and the page whose terminal has
      the session open types it with xterm.js's `paste`
      (`terminals.pasteText`), which marks it as a paste
      (`ESC [200~` … `ESC [201~`) while the program has asked for that
      (bracketed paste), as zsh, vim, and Claude Code do: a program that
      reads keys one at a time, such as an agent's prompt, then doesn't
      take a new line in a path for Return. Only xterm.js knows whether
      the program asked, so Go doesn't write to the PTY itself. The
      command goes to every page; the main window has sessions only for
      the terminals in the tab and its drawn drop-down, and a popped-out
      terminal's window only for its own (`terminal-window.js`), so the
      one page that shows the terminal types it. Bench doesn't come
      forward, and the keyboard stays where it was. A terminal whose
      command has finished is refused
      (`errDropEnded`), as typing there would do nothing.
    - **Folders and apps on a pane** (`readDropped`, `dropOnPane`).
      Each path is read on disk: a folder whose name ends in `.app`, with
      a bundle ID (`apps.ReadBundle`), is an app, wherever it is; another
      folder is a folder; anything else is a file. A folder adds a new
      terminal in it (`addTerminalLocked`), and an app is added as the add
      dialog's **Apps** rows add one (`addAppLocked`), after
      `noteBrowsedLocked` lists it as installed, as **Browse…** does. A
      package, a folder that the Finder shows as one item, such as an
      Xcode project or a Photos library, is a file, as it is to the
      Finder: `packageExts` lists the common ones by extension, as only
      macOS knows them all. The pane is found as `AddToPane` finds it
      (`targetPaneLocked`), and becomes the focused pane, as the user
      chose it, and the first item added shows there and gets the
      keyboard (`finishAddLocked`). The add dialog's **+** focuses its
      pane as it's clicked; a drop has no such click, and without this,
      an app still to open would leave the keyboard to another pane's
      app, which would come in front of the Finder. Like the add dialog,
      a drop on a pane has no **Undo**: each item closes as usual. Files
      are left out, with a toast that says how many, and a drop of files
      only is refused (`errDropFiles`), as Bench has no way to open a
      file in a pane. Paths that have gone, and `.app` folders without a
      bundle ID, are left out too, and the toast says why; a second copy
      of an app is left out without a word. A drop of which nothing can
      be added, as when no app opens, is refused with the first reason:
      `addAppLocked` fails only before it adds a window or opens the app,
      so a thing with an error added nothing. More than 8 folders and
      apps (`dropLimit`) is refused, and none opens, so that a drop of a
      whole folder's contents doesn't open dozens of terminals.
    - **A new tab from the +** (`dropOnNewTab`), as the **New tab** dialog
      makes a tab of windows that are open (decision 54): named after the
      first folder or app (`uniqueTabName`), with `suggestPreset`'s
      layout for their number, and each in a pane of its own, in order,
      those after the last pane in the last one. Bench switches to it and
      saves it in `config.toml`, and its toast's **Undo** deletes the
      tab, the terminals it made with it, and puts back the windows it
      took from **Not in a tab**, by the same snapshot and
      `undoStep.outside` as decision 54's. If nothing went in the tab, as
      when no app dropped opens, the tab goes again before the config or
      Undo has it, and the drop is refused with the first reason: an empty
      tab named after an app that never opened would stay in the user's
      `config.toml`.
    - **What covers Bench, a window that keeps moving, and the new tab.**
      A drop on a pane or the + whose new terminal gets the keyboard
      brings Bench forward (`FocusUI`) if another app, such as the
      Finder, was in front. That isn't the user going to Bench, so it
      doesn't put away the windows over Bench (decision 68): the Finder
      window that the drag came from would go, with the next folder to
      drop, and the toast that says so would push out the new tab's
      **Undo**. `noteDropForwardLocked` notes such a drop, and Bench's
      next activation within `dropForwardGrace`, 2 seconds, passes
      without `hideCoveringLocked` (`droppedForwardLocked`); after that,
      or after a drop that gave no terminal the keyboard, coming forward
      is the user's doing again. A pane whose window its app or a window
      tool kept moving (decision 69) is a pane whose item isn't ready, so
      a drop on its message, or its **Dock again**, adds to the pane, as
      on its header. The window keeps its mark, and stays where its app
      put it: a drop from the Finder isn't a drag of the window onto a
      pane, which `TrackDrags` follows (decision 13) and which docks it
      again, and it never reaches `Redock`. The new tab from the + comes
      in the View that has it current, once Go has arranged it, so the
      title bar shows it as current from the moment it shows it at all;
      the `switching` command (decision 67) is only for tabs that the
      View has already, and the switch's time is kept as a `switch`'s.
    - **Other drops** (`guard` in `drops.js`). Anything else dragged
      onto the page, such as a link or text from a browser, comes as HTML
      drag events, which Wails' runtime lets through, as they aren't
      files, and which WebKit would end by loading the link in place of
      Bench's page. So the page cancels every `dragenter`, `dragover`,
      and `drop` that nothing in it has taken, and at a drop from another
      app says what Bench takes (`NOT_FILES`). A field that takes text,
      such as the jump palette's, still takes a drop of text. A drag that
      started in the page, such as of selected text, shows that it can't
      drop (`dropEffect` `none`), so it ends without a drop. Bench's own
      drags of tabs and sub-tabs aren't HTML drags (decision 19), and
      `drag.js` stops an HTML drag of their text as it starts, so the
      guard counts a drag as the page's only once a `dragstart` that
      nothing stopped reaches the window. A drag ends with a `dragend`
      at its source, which a new view may have taken out of the page by
      then, so the guard listens on the source too; and a press of the
      pointer, which can't come during a drag, ends one whose end went
      unheard. Otherwise the guard would take every later drag from
      another app for the page's own, and say nothing. In the app, a drag
      of files that comes as HTML drag events can't be from the Finder,
      as Wails' native view takes those first: it's of files that have
      no paths yet, such as an image from a browser, a photo from Photos,
      or an attachment from Mail, which WebKit offers the page as files.
      Wails' runtime would light up what it's over, and cancel the drop
      without a word, so the guard takes it first, as it reaches the
      window (`notFinder`), lights nothing up, and at the drop says to
      save it as a file and drag that (`NOT_FINDER`). The browser
      preview has no `enableFileDrop`, so there it does nothing, as
      before. The Acknowledgements window's page has a script of one
      loop for the same reason (`dropGuard` in `build/acknowledgements`),
      its only one: it cancels every drag over the page, and says that it
      can't drop.
    - **The browser preview** gets no paths from its browser. Its tests
      and screenshots call `window._wails.handlePlatformFileDrop` as
      Wails' native code does, and the page's `benchDrop` finds the
      element as Wails' runtime does, and sends the paths and its
      attributes to `DemoDropFiles`, as Wails' runtime would send them as
      a window event, which the preview's windows don't have. In the app,
      where Wails sets `flags.enableFileDrop` in the windows that take
      drops, `benchDrop` passes them to the runtime. A page without Wails'
      runtime, as in the frontend tests, gets hover handlers like Wails'.
      A file dragged onto the preview from the computer's own file
      manager does nothing.
    - **Not** links dropped to add a web page: that's for later, with
      **Web page…**'s choice of a web app or a Web item. **Not**
      spring-loaded tabs: a drag held over a tab doesn't switch to it.
      **Not** files opened in a pane, as Bench has no item for a file.

74. **A tab can have one of eight colours, for its underline, a band along
    its pane headers, and a dot beside its name in lists.** With several
    tabs of the same width and grey text, finding a tab meant reading
    every name. A colour can be seen at a glance, but it must not be the
    only way to tell tabs apart, and it must not hide the current tab or
    the focused pane, which already use the accent.
    - **Config.** `colour` on `[[tab]]` (`config.Tab.Colour`, with
      `omitempty`, so that `Format` writes no line for a tab with none, and
      the `json` that keeps closed tabs in `state.json` has none either).
      `config.TabColours` lists the eight names, in the order Tab
      properties shows them, and `IsTabColour` checks one. `Validate`
      refuses any other value, with the list of names, and says what a
      name in capitals should be, as `"Blue" needs to be in lower case:
      "blue"`. For a name that's nearly right, such as the American
      `gray`, `nearestColour` asks `did you mean "grey"?` in place of the
      list, as `suggest` does for a field: for the American `color`, it
      asks "did you mean "colour"?", as for any other field it doesn't
      know. `Example` gives Comms blue and Code green, and the welcome
      guide's suggested tabs have colours of their own (`starterTabs`). No
      config from before has a colour, and Bench never adds one to a tab
      it reads.
    - **Engine.** `Tab.Colour` is in the model, so the snapshots that
      **Undo** takes (`undoStep.saved`) and **Reopen Closed Tab**'s closed
      tabs keep it, as they keep a tab's config entry. `state.json`
      doesn't repeat it: the model takes it from the config, when Bench
      starts and when `config.toml` reloads (`reload` sets it with the name
      and small-screen setting). `SaveTab` and `NewTabFromWindows` take it
      from `TabDraft.Colour`; `draftConfigLocked` refuses a colour that
      `IsTabColour` doesn't know, before anything changes. Rename in place
      sends the tab's colour back as Go sent it (`rename.js`), as it sends
      its small-screen setting, and Tab properties starts from it, not
      from the frontend's list, so that a save never drops a colour that
      the page doesn't know. `TabView.Colour` and `MenuBarTab.Colour` carry
      it to the frontend and the app menu.
    - **Frontend.** `TAB_COLOURS` in `util.js`, and `colourAttr`, which
      gives an element `data-colour`. `TestTabColoursMatchFrontend` checks
      that `util.js`, the rules and tokens in `styles.css` and
      `light.css`, and `appearance.test.js` have the colours of
      `config.TabColours`, in its order. One rule for each colour sets
      `--tab-colour` from the theme's `--tab-<name>` token on whatever has
      the attribute: a tab, a pane (`renderPane` gives each pane the
      current tab's), a dot (`tabDot`), and a swatch. A coloured tab has a
      2 pt `::after` line, 12 pt in from each side; the current one hides
      it and has its `inset` underline in `--tab-colour` in place of the
      accent's, so it stays the one with the raised background and the
      full underline. A pane header's band is a background image along
      its top, 2 pt tall, 1 pt below the pane's border and 12 pt in from
      each end, as the tab's short underline is: it takes no room from the
      docked window, and as it never touches the pane's border, it can't
      be taken for part of the focused pane's accent border. Dots
      show only while a tab has a colour (`anyColoured`), with a hidden
      dot for a tab with none, so that the names line up; they are
      `aria-hidden`, and no name or label has the colour, as the name is
      always there. Tab properties' **Colour** is a `radiogroup` of
      buttons with `radioState`, so `roving.js` gives it the keys of every
      other radio group; each swatch's `aria-label` and `title` name it,
      and the name of the one chosen shows beside them (`colourText`), for
      users who can't tell the colours apart. It is the first row of the
      part that scrolls, so it stays in view in the smallest window.
    - **Contrast.** Each colour has a dark value in `styles.css` and a
      light one in `light.css`, chosen at 3:1 at least, as WCAG asks of
      marks that aren't text, on every surface the colour sits on:
      `--bg-chrome`, `--bg-tab-active`, `--bg-hover`, `--bg-pressed`,
      `--bg-pane-header`, `--bg-raised`, `--bg-window`, and
      `--bg-control-on`, and the accent's wash over the title bar, on a tab
      that a sub-tab is dragged over, and over the pane, on a pane header
      that files from the Finder are over (decision 73). The lowest is
      light yellow on `--bg-pressed`, at 3.16:1. Each is also 20 apart
      from the accent at least, as CIEDE2000, so that a tab's colour can't
      be taken for the focus: the lowest is light red, at 21.4. Yellow is
      a greener yellow than the accent's orange, at 25.7 in light and 30.5
      in dark. A mustard yellow, at 19, looks like the accent: a yellow
      tab's underline looks like the accent's, and a band against the
      pane's border makes an unfocused pane look focused. Each is as far
      from `--warning`, the amber of the warning sign that a window that
      keeps moving shows on its sub-tab (decision 69), so that a yellow
      band can't be taken for it: the lowest is yellow, at 20.2 in light
      and 20.5 in dark. Increase contrast changes lines and text, not those
      surfaces or the accent, so the colours need no third set. The test
      is a rule in `appearance.test.js`'s `READABLE`, with the washes and
      the distances from the accent and `--warning` beside it, which runs
      in both appearances, with and without Increase contrast.
    - **App menu.** Wails' `MenuItem.SetBitmap` takes PNG data, so the Tab
      menu's tab items, and those of **Pane ▸ Move to Tab**, get a dot
      with no Objective-C: `tabdots.go` draws
      each colour once (`menuDot`), 24 pixels square with a 9-pixel
      radius, smoothed at the edge, and adds a `pHYs` chunk of 144 pixels
      to the inch, so that `NSImage` takes it as 12 points, sharp on a
      Retina screen: a 9 pt dot in a 12 pt image. The dots in Bench's own
      lists are 8 pt. A tab with no colour gets a blank image as large,
      while another has one, so that the names line up. The menu follows
      macOS's appearance, not Bench's, and an image can't change with it,
      so each dot has one value, between the light and dark ones, at 3:1
      on a light menu and a dark one (`TestMenuDotContrast`). `SetBitmap`
      on an item that's already built doesn't run on the main thread, so
      `set` rebuilds the menu, as it does for a new tab, when a tab's
      colour changes (`sameColours`), and the bitmaps are set before
      `Update`, which applies them on the main thread.
    - **Menus.** **Move to Tab** and **Also Show In**, in a pane's ⋯ menu
      and its sub-tabs' menus, have the dots too, while any tab has a
      colour: `tabEntries` gives each tab's entry a `dot`, its colour or
      `none` for a blank one as wide, in the menus' one model (decision 71).
      Bench's own menus show it with `tabDot`. A native menu gets it in
      `MenuEntry.Dot`, and `buildNativeMenu` gives its item the image of
      `menuDot` before Wails builds the menu, as the app menu does. **New
      Tab** has none. The jump palette's This window rows, such as **Move to
      Tab ▸ Comms**, have none either: the tab's name is part of a longer
      label there, and the palette's Tabs rows have the dots.
    - **With the decisions before it.** A switch shows the tab that it's
      going to as current at once (`shownCurrent`, decision 67), and the
      rule for the full underline follows `aria-selected`, so a coloured
      tab's underline goes at once with it. The pane headers keep the band
      of the tab that the View has current until the switch is done, as they
      show its panes. The warning sign on a sub-tab whose window keeps
      moving (decision 69) starts 5 pt above the sub-tab's tile, which in
      the compact density is where the band is, so in a coloured tab's pane
      header it comes down to 2 pt above, clear of the band. While files
      from the Finder are over a pane header (decision 73), the band moves
      down by the width of the frame, to 3 pt, or 4 pt with Increase
      contrast, so that both show. The drop-down terminal (decision 72) is
      in no tab, so its panel has no colour. **Keep over Bench** (decision
      68) changes `config.toml`'s rules after a **Save**: its Undo leaves
      the saved colour, and the Undo of the **Save** takes the rule back
      too, as it does any change after it. Toasts have no colour.
    - **Undo.** A change of colour goes through Tab properties' **Save**,
      and so through its **Undo**: `SaveTab` keeps an undo step for a tab
      that was there, with `config.toml`'s tabs and rules (decision 37),
      so the toast after **Save**, or after a rename in place, has
      **Undo**, which puts back the tab's name, layout, small-screen
      setting, colour, and place, and saves them. It's **Rename Tab** when
      only the name changed, and **Change Tab Properties** otherwise
      (`savedTabUndo`). As the step keeps `config.toml`'s tabs from before
      the change, the changes before it stay within Undo's reach: Undo of
      one of them takes this one back too, and the tabs' names match
      again. A tab added by **New tab** still puts them out of reach.
      **Undo** of a deleted or closed tab, and **Reopen Closed Tab**,
      bring its colour back.
    - **Not** a symbol or icon field for a tab: an emoji in the name shows
      everywhere the name does, and the user guide says so. **Not**
      orange, which is the accent's, for the current tab's underline
      before this and the focused pane's border, nor more than eight, which
      would be harder to tell apart, nor any colour value in `config.toml`.
      **Not** colouring tabs that a config already has, or the focused
      pane's border. **Not** the colour in the menu bar item's menu, whose
      items are plain text. **Not** a **Save** in Tab properties that
      Undo can't take back, as it was before this, which also put the
      changes before it out of reach.

75. **Bench finds an app's New Window and Close Window menu items in the
    Mac's language, and uses their key equivalents only to choose between
    titles that match.** Bench presses another app's **New Window** to open
    a window of an app that's running (the add dialog's **New window**, an
    app whose windows are all in tabs added from its **Apps** or dropped
    from the Finder on a pane or the tabs' **+**, which add it the same way
    (`addAppLocked`, decision 73), and a pane's **Open** *app*; a tab switch
    only opens the app again, in `openMissingLocked`), and its **Close
    Window**, or **Close**, to close a window that has no close button
    (decision 36). It found them by their English titles, and the File menu
    by the word File, so on a Mac set to another language, New Window
    opened the app again instead, and a window without a close button
    stayed open.
    - **Titles.** `menuLanguages` (`internal/mac/menus.go`) has, for 17
      languages, the titles that Apple's own apps give **New Window**
      (SwiftUI's standard menu, `MainMenu.loctable`, and Safari), **Close
      Window** (AppKit's standard menu, `MenuCommands.loctable`), and
      **Close** (AppKit's `Document.loctable`, and Preview), with the
      words for new and for window in New Window, and Safari's word for
      private in New Private Window. They come from macOS 27's resources,
      as the Apple Localization Terms Glossary (applelocalization.com)
      lists them: Danish, Dutch, Finnish, French, German, Italian,
      Japanese, Korean, Norwegian, Polish, Portuguese, Russian, Simplified
      and Traditional Chinese, Spanish, Swedish, and Turkish. Portuguese is
      the same in Brazil and Portugal but for case, and Chinese is the
      same in Hong Kong as in Taiwan. Titles compare in lower case, without
      an ellipsis (`menuTitle`), and otherwise as the app gives them, with
      no Unicode normalisation. Mail titles its New Viewer Window as plain
      New Window in French, German, Italian, Portuguese, and Spanish, so it
      matches there too.
    - **Key equivalents.** The native side reads each item's
      `AXMenuItemCmdChar` and `AXMenuItemCmdModifiers`, which are public
      (`benchCopyMenuKey`), and passes them to Go with the title; an
      unknown modifiers value is -1. Command-N, or Command-W, with no other
      modifier key (modifiers 0) adds one to the rank of an item whose
      title matches: New Window with Command-N (`newWindowExactKey`) is
      above New Window without it (`newWindowExact`), which is above New
      Finder Window with Command-N (`newWindowOtherKey`). So a key breaks
      ties between titles of the same rank, and never lifts a title above a
      better one. It never counts on its own: Command-N is New Message in
      Mail, New Note in Notes, and New File in many editors, and Command-W
      is Close Tab in Safari and Chrome. Pressing those would make an empty
      message or close a tab, which is worse than opening the app again.
      The proposal had the key first, with the English title as a guard,
      but on a Mac in another language that guard can't run.
    - **Other New … Window items.** In English, a "New … Window" item that
      isn't private or incognito still counts (`newWindowOther`), as
      before. In other languages the words come in any order and in
      compounds ("Nouvelle fenêtre Finder", "Nyt Findervindue",
      "新規Finderウインドウ"), so an item that starts with the word for new
      and contains the word for window counts only with Command-N, and
      never with the word for private. That keeps Finder's New Finder
      Window, and leaves out private and incognito windows, which don't
      have Command-N alone (Safari and Chrome give them Shift-Command-N,
      Firefox Shift-Command-P), without a list of each browser's own words.
      Mail's viewer window in Swedish or Japanese, with Option-Command-N,
      doesn't count, so Bench opens Mail again.
    - **The File menu** is the menu bar item after the app's own menu
      (index 2, as 0 is the Apple menu), whatever its title (Ablage,
      Fichier, ファイル), or, wherever it is, an item titled File in
      English, which wins as a cross-check (`fileMenuRank`).
      `benchMenusFileFirst` moves it to the front, for both finders, which
      each had their own copy of that loop before. An app without a File
      menu, such as System Settings, has another menu in that place, which
      does no harm: Bench looks in every menu, and the order only decides
      between items of the same rank.
    - **Native code** stays small and does no ranking: for each item, it
      reads the title, skips an item without one, and asks Go
      (`benchNewWindowRank`, `benchCloseWindowRank`, and
      `benchFileMenuRank`, in `exports_darwin.go`). It keeps the first item
      with the highest rank. A key equivalent only ever raises a title's
      rank, and Command-N, or Command-W, raises it most. So the native side
      first ranks the title with that key, which costs no Accessibility
      call, and reads the item's own key only if the title could then beat
      the best so far. Most items' titles can't, so they cost one
      Accessibility call, the title, as before; the few that can cost one
      more, or two with a key equivalent. The search stops at once at
      `BENCH_NEW_WINDOW_BEST` or `BENCH_CLOSE_WINDOW_BEST`, and after the
      menu in which it finds an exact title (`BENCH_NEW_WINDOW_EXACT` or
      `BENCH_CLOSE_WINDOW_EXACT`), so a key breaks ties only within that
      menu. Before, the search stopped at the exact title itself, so it now
      reads the rest of that menu's titles too: about 20 more calls in a
      long File menu. The search runs with the engine's lock held (in
      `addNewWindowLocked`, `addAppLocked`, `decideLaunchLocked`,
      `openItemLocked`, and `pressCloseLocked`). A hold of the lock is
      timed from what asked for it (decision 67), so the search counts in
      the time of the arrangement after it in the same hold, which **Report
      a Problem…** and the `arranged` log lines give: under the reason
      other for the add dialog and a drop on a pane, and under switch for a
      drop on the tabs' **+**, which switches to its new tab. A click on a
      tab and a tab's shortcut never search, so their times (tab and
      tab-shortcut) don't change. `menus_test.go` covers the ranking: each
      language's titles, Finder's New Finder Window in each, the traps
      (Safari's New Private Window, New Tab, and Close Tab, Mail's New
      Message, Notes' New Note, and Close All, with the keys they have), and
      the item that wins in whole menus, and what the search reads to find
      it (`searchMenus`, which searches as the native side does).
      `TestMenuKeyRaises` checks that a key never lowers a rank, nor raises
      it more than Command-N or Command-W does, which the native side counts
      on. `TestHeaderConstants` checks the `_EXACT` and `_BEST` values.
    - **The fallbacks stay.** With no item that ranks, `NewWindow` opens
      the app again (`platform_darwin.go`), and `PressClose` returns
      `errNoCloseButton`, whose message says to close the window in the
      app. The user guide's troubleshooting says which languages Bench
      knows and what to do in others.
    - **Not** the key equivalent alone, nor first. **Not** a key read for
      every item, nor a search on through every menu after New Window
      without Command-N, as Terminal's and Visual Studio Code's are, in case
      a later one has it: with the engine's lock held, that would cost
      hundreds of Accessibility calls in an app with many menus, for a tie
      that hardly ever happens. **Not** every language macOS has: Arabic,
      Catalan, Croatian, Czech, Greek, Hebrew, Hindi, Hungarian, Indonesian,
      Malay, Romanian, Slovak, Slovenian, Thai, Ukrainian, and Vietnamese
      aren't in the table yet; each is a row and a test, from the same
      source. **Not** other makers' own translations, which may differ from
      Apple's: their apps match only where they use Apple's words. In
      Japanese, for example, Chrome and Visual Studio Code write ウィンドウ, with
      a small ィ, where Apple writes ウインドウ, so neither their New Window nor
      their Close Window matches. **Not** a localised **New** (Neu, Nouveau,
      新規), which makes a document in TextEdit and Pages, as English **New**
      doesn't count either.

76. **A web page can keep running in the background, so that its
    notifications and its unread count arrive before it's shown, and its
    first notification turns that on.** Bench makes a page's web view the
    first time it shows the page (decision 61), so Slack or Outlook in a tab
    not shown since Bench opened posted no notifications, and had no count,
    and the user missed messages. The owner asked for the first step only:
    a setting for each page, which the first notification turns on, and a
    note on pages that aren't loaded. Bench doesn't put pages to sleep or
    unload them yet (see **Not**).
    - **The setting.** `keep_running`, a boolean in a `web` item's entry in
      `config.toml` (`config.Item.KeepRunning`). It's a `*bool`, missing
      until Bench or the user sets it, so it's off by default, and a
      missing one is the only one a notification changes. `Validate`
      refuses it on other items, and `inlineItem` writes it last. The
      entry's key (`entryKey`) leaves it out, so that changing it keeps
      the item's place in `state.json`. A page with an entry takes it from
      `config.toml` at startup and at a reload (`applyEntry`), so that
      taking it out of the file is as if Bench had never set it. A page
      added in Bench has no entry, and `state.json` keeps it
      (`ItemState.KeepRunning`). The engine has it as `Item.KeepRunning`,
      with `Item.KeepChosen` set once it's set either way. VS Code for the
      Web has none (`keepable`), as its page needs its own server.
    - **Loading.** `Platform.LoadWebView` makes a web view, hidden, the size
      of the pane area, and loads its address. It does nothing for a web
      view that exists. On the Mac, `benchWebLoad` (`web_darwin.m`) makes a
      `BenchWebItem` as `benchWebShow` does, with the page's profile, user
      agent, name, and zoom, and adds its `WKWebView` to Bench's window,
      hidden, as `benchWebHide` leaves one. Its page runs: it can post
      notifications and change its title. `benchWebShow` later moves it to
      its pane and shows it. The engine (`keeprunning.go`) loads the pages
      with the setting on that have no web view, one at a time:
      `preloadDelay`, 3 seconds, after the first arrangement with a pane
      area (`arrangeWebLocked` calls `preloadSoonLocked`), so that the tab
      the user sees comes first, then `preloadGap`, 2 seconds, after each
      one, the current tab's first, then in tab order
      (`nextPreloadLocked`). A page turned on while Bench runs loads the
      same way. Bench tries each page once (`preloadTried`) until its web
      view closes; one that fails is logged, and loads when it's shown.
      Once loaded, a page is in `webMade`, as one that has been shown is,
      so its title's count (decision 61) and its notifications arrive. Not
      in `bench arrange` (`Headless`), and not for a page in a window of
      its own, which has its web view already.
    - **Running while hidden.** A hidden page used to need no process: one
      whose process ended reloaded only once it was shown
      (`reloadAfterCrash`). A page kept running has to go on running, or
      its notifications and count stop without a word. So the platform
      hears which web views keep running (`Platform.KeepWebViewRunning`,
      `benchWebKeepRunning`, and `LoadWebView`, which implies it):
      `keepWebViewsLocked` tells it, for each web view, whenever that
      changes, after each arrangement and each change to the setting
      (`webKept`). On the Mac, such a `BenchWebItem` (`keepRunning`)
      reloads after a crash while hidden, with the crash's backoff, up to a
      minute apart. And a page loaded hidden as Bench opens at login can
      fail for want of the network, which nothing would retry, as its
      `requestedURL` is set. So a first load that fails for want of the
      network before any page has come into view (`benchOffline`: no
      connection, no host, a timeout, or a TLS failure, as at a captive
      portal; not a cancel, or a navigation the policy refused, which
      would loop) sets `needsLoad`: `benchWebShow` loads it again, and a
      page kept running tries again by itself after 15 seconds, doubling
      to 5 minutes (`retryLoadSoon`). The engine doesn't hear of these
      failures, so such a page isn't said to be not loaded.
    - **The first notification.** When a page in a tab posts a notification
      (`WebNotification`) and its `keep_running` isn't set,
      `keepNotifierLocked` turns it on and says so once, as "Slack will
      keep running in the background, so it can notify you.", with
      **Undo**. Undo turns it off as the user's choice. Once it's set, by
      Bench, the user, or `config.toml`, a notification leaves it as it
      is, so Undo stays. Any page that posts a notification counts, so
      there's no list of hosts: the user's own chat, mail, and calendar
      sites count, and a site that never posts one never gets it. The
      event loop mustn't wait for `cfgMu`, which `Close` holds while it
      waits for the loop, so saving `config.toml`, and the toast, wait for
      the clock's goroutine (`AfterFunc(0)`). `state.json` is saved at
      once.
    - **Undo.** A first notification most often comes while the user is in
      another app, and the toast is often the only chance to undo it, so
      the change is an undo step (decision 65), not a toast of its own:
      **Undo Keep Running in the Background** in **Tab ▸ Undo**, the jump
      palette, the recent messages, and the `undo` shortcut, "Undid: kept
      Slack running in the background." Its step (`undoStep.keepOn`) has
      no arrangement: `takeBack` puts back the arrangement of the first
      other change it takes back, if there's one, so that undoing it alone
      doesn't move the user back to the tab they were on. The toast waits
      for Bench to be the active app (`keepToasts`, sent from
      `onAppActivated`), as the user would miss it; its Undo goes if the
      step has gone meanwhile. **Keep Running in the Background** in a
      menu ends the step, so that Undo can't take back the user's own
      choice. And a step from before the setting changed puts back
      `config.toml`'s tabs as they were then (`cfgTabs`), which would take
      the field away, so `takeBack` puts each page's `keep_running`, as the
      model has it, back in the entries it saves
      (`keepRunningInConfigLocked`): it's the page's setting, not part of
      the arrangement.
    - **Saving.** `keepRunningConfigLocked` puts the field in the page's
      entry in each tab whose entries include it (`sameConfigTab`,
      `entryPlace`, and `nthEntry`, which a web app's link uses too), and
      `saveEdit` saves `config.toml` as **Settings** does. If it can't,
      the toast says why. **Keep Running in the Background** in the menus
      (`SetKeepRunning`) saves it in the same way, and says what it did.
      Turned off, a page stays loaded until Bench quits, as Bench never
      unloads one, so the toast says "Slack will load only when you show
      it, from the next time Bench opens."
    - **Not loaded yet.** A web page with no web view that won't get one
      until it's shown (`notLoadedLocked`: it isn't kept running, or its
      load failed) has `notLoaded` in its view. Its sub-tab's tooltip has
      the line "Not loaded yet: no notifications or counts until you show
      it", and its VoiceOver name the same note, in lower case, after its
      status, so that no count doesn't pass for nothing unread. A page in a
      tab not opened since Bench started has no sub-tab in sight, so the
      overview of every tab (decision 60), where the user compares the
      tabs' counts, says so too: the page's tooltip has the line, and its
      tab's card's VoiceOver name says "1 page not loaded yet". A page
      waiting to load soon, VS Code for the Web, a page in a window of its
      own, and `bench arrange` don't have it.
    - **Menus.** **Keep Running in the Background**, in title case
      (decision 63), is one entry of the menus' shared model (decision
      71), `page-keep-running` (`pageEntries`), with `checkbox` and
      `checked`. It's last in a web page's page group in its **⋯** and
      sub-tab menus, after **Stop Sound and Video**, in the menu itself,
      not in the **Page** submenu with **Back**, **Forward**, and the
      zooms: it's a setting that the user reads at a glance, and a tick in
      a submenu is out of sight. **Pane ▸ Page** has it last, after a
      separator. The HTML menus draw it as a checkbox (`menuitemcheckbox`)
      with its tick before the label, where macOS puts it; the native
      menus (`plainEntries` and `nativeItems`) make it an item with a
      checkmark, as the layouts have theirs; and the jump palette's **This
      window** (`windowRows`) lists it as **Page ▸ Keep Running in the
      Background (now on)** or **(now off)**, after **Actual Size (now
      125%)**, so that the row doesn't read as what Return does. As
      `NSMenu` does, an HTML menu with a checkbox keeps the tick's column
      before every entry (`renderEntries`' `ticks`), and its headings,
      notes, and submenus' entries line up with the labels; other menus,
      the **Page** submenu among them, have no column. A native menu's
      choice runs only if the entry has the tick it had as the menu opened
      (`chosenEntry`): a notification that turns it on while the menu is
      open would otherwise make the user's "on" an "off". Wails ticks a
      checkbox as it's chosen, so **Pane ▸ Page**'s goes back (`retick`)
      if the engine refuses.
    - **With the other items.** Loading a page in the background arranges
      nothing (`loadWebLocked` sets no frame), so the times that **Report
      a Problem…** keeps (decision 67) don't count it. A hidden web view is
      Bench's own, in its window, not another app's window, so Bench
      coming forward never puts one away as covering it (decision 68). If a
      page's first notification came while the user was away, and Bench
      hides an app that covers it as it comes forward, both toasts go out,
      the notification's first: the one about the app, which has just
      changed what's on the screen, stays in view, and the recent messages
      keep both, with their buttons. Only a terminal can be the drop-down
      (decision 72), so a page's menus don't offer **Use as Drop-down**.
      `config.toml` keeps a tab's `colour` (decision 74) and its pages'
      `keep_running` side by side, and `state.json` keeps
      `ItemState.KeepRunning` with the fields that decisions 68, 69, 70,
      and 72 added (`covering`, `windowTools`, `preferredScreen`, and
      `dropdown`).
    - **Tests and the preview.** The fake platform records each web view it
      makes, in order (`WebViewsMade`), and its `LoadWebView` makes one
      hidden, loaded, and kept running, which `KeepWebViewRunning` changes
      (`FakeWebView.KeepRunning`). `engine_keeprunning_test.go` checks that
      pages with the setting load after the first arrangement, one at a
      time with the gap, the current tab's first; that a page without it
      loads only when it's shown; that a load that fails isn't tried again;
      that the first notification turns it on once, saves it, and says so,
      once Bench is active, and that Undo, from the toast or **Tab ▸ Undo**,
      turns it off for good, leaving the tab shown; that Undo of a rename
      from before keeps `keep_running`, across a restart; that the menu's
      choice ends the notification's step; which web views keep running;
      the menus' `SetKeepRunning`; VS Code for the Web; a page added in
      Bench, across a restart; `bench arrange`; and that loading pages in
      the background is neither timed nor moves a window, and leaves them
      hidden as Bench comes forward. The fake can't fail a page's load or
      end its process, so the Mac checks cover those. The demo bar's
      **Simulate ▸ Post notification** (`DemoNotify`) has the focused
      pane's page post one, for the browser tests
      (`e2e/app/keeprunning.test.js`) and the screenshots.
      `e2e/frontend/native-menus.test.js` checks the native menus'
      checkmark, and that a choice whose tick changed doesn't run. With
      `keep_running` on, the example config's Outlook, whose title is
      "(12) Outlook" in the preview, shows its count from a tab that hasn't
      been shown.
    - **Not** putting pages to sleep or unloading them, to save memory: that
      needs a measurement first of what pages kept running cost, with four
      chat and mail pages kept running on a Mac (the manual checks in
      `development.md`). Until then, a page that's loaded stays loaded.
      **Not** a list of chat and mail hosts: the notification is the sign
      that a page wants to reach the user, whatever its host. **Not** on
      for every page by default: that would load pages, and use memory and
      the network, that the user hasn't asked for (decision 61). **Not** all
      at once at startup: one at a time, after the shown tab, keeps the
      first seconds for what the user sees.
