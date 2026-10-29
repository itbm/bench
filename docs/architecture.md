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
| `internal/layout` | Trees, presets, unit rectangles, frames, dividers, size class, small-screen changes, neighbours. Pure Go. | No |
| `internal/config` | `config.toml` and `state.json`: load, validate, format, save, defaults. | No |
| `internal/platform` | The `Platform` interface, `ParkPlace` and `ParkFrame`, `Fake` for tests, `NewDemo` for the browser preview, and `Logged`, which logs calls and events when `BENCH_DEBUG` is set, and gives a `DebugLogger`, such as the macOS platform, the log for decisions of its own. | No |
| `internal/windows` | Docking, stacking, parking, hiding, and restoring other apps' windows, through `Platform`. | No |
| `internal/workspace` | The model (tabs, items, moves, rules) and the engine that runs it: platform events, switching, arranging, persistence, and the `View` sent to the frontend. | No |
| `internal/term` | Terminal sessions in pseudo-terminals (`creack/pty`). A session lasts until the engine removes its item, which closes it. When Bench runs with SIGHUP ignored, as under `nohup`, it starts each shell through its own binary, run as a helper that puts SIGHUP back to its default action and then becomes the shell, so that closing a session still ends its programs. | No |
| `internal/vscode` | `code serve-web` for VS Code for the Web. | No |
| `internal/apps` | Installed apps, including the web apps that browsers make and the sites they open, the browsers that can make them, monograms, and display names. | No |
| `internal/shortcuts` | The `[shortcuts]` config as global shortcut bindings. | No |
| `internal/clock` | A clock that tests can move by hand. | No |
| `internal/ui` | The Wails app: Bench's windows (the main window, popped-out terminals' windows, and Acknowledgements), the bound `Service`, events, global shortcuts, the menu bar item, the app menu. | Through Wails |
| `internal/mac` | macOS only: Objective-C for the Accessibility API, NSWorkspace, screens, window levels, web views, ScreenCaptureKit, and notifications. It implements `platform.Platform`. Its pure-Go helpers build everywhere, among them the Web items' navigation policy (decision 24), and the browser preview uses one, `PageInfo`, for its web pages' hosts. | Yes |
| `frontend` | The UI, from the prototype. Plain HTML, CSS, and ES modules, with no build step. `acknowledgements.html` is the Acknowledgements window's page, which `build/acknowledgements` writes. | — |
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
  `bench:state` per 16 ms.
- Terminal output has a queue and lock of its own, and is sent without the
  engine's lock, at most once per 16 ms for each terminal, so an arrange
  that waits for a slow app doesn't hold it up. Typing doesn't take the
  engine's lock either, except in a terminal whose command has finished,
  where Return runs it again. Only titles and ends of sessions are applied
  under it.
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
- Go never waits for the main thread. It sends AppKit work there with
  `dispatch_async`, and what it needs to read, such as the screens and
  Bench's content frame, is cached as the main thread changes it. Alerts
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
- **Raised mode.** Menus, dialogs, live previews, and drags would be hidden
  behind docked windows and web views, and a drag can't reach a pane body
  that another app's window covers. So while any of them is open, the
  frontend calls `SetOverlay(true)`: Bench raises its window above the docked
  windows (`Platform.SetRaised`) and hides its web views. `SetOverlay(false)`
  puts Bench back below normal level, raises the docked windows again, and
  shows the web views. The docked windows are behind the dialog's scrim
  anyway, so nothing visible is lost. While Bench isn't the active app, its
  window goes back below normal windows even in raised mode, so that it
  doesn't float over other apps.
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
  tab's apps hide with it (decision 16).
- **Other windows.** Bench's other windows are ordinary windows at the
  normal level: popped-out web pages and terminals, web pages' popups, and
  Acknowledgements (decision 27). Raised mode floats only Bench's main
  window, and only while none of them has the keyboard, so that one that
  the user opens or clicks stays in front of it. Bench becoming active
  hides apps over its main window only while that window is shown, since
  the user may have clicked one of these instead.
- Live previews are a native panel (`Platform.ShowPreview`) above everything.

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
| `SetPaneArea(area layout.Rect)` | The pane area (between the title bar and the status bar) in content coordinates, measured by the frontend whenever it changes. Go lays panes out in it. |
| `SwitchTab(index int)` | Switch tabs. |
| `StepTab(step int)` | Next (1) or previous (-1) tab, wrapping. |
| `SetMode(mode string)` | `auto`, `wide`, or `compact`. |
| `FocusPane(paneID string)` | Focus a pane. |
| `ShowItem(paneID, itemID string)` | Show an item in its pane (or in the `compact` pane). Also focuses the pane. |
| `MoveItem(itemID string, tab int, paneID, zone string)` | `zone` is `center`, `left`, `right`, `top`, or `bottom`. |
| `MoveItemToTab(itemID string, tab int)` | Into that tab's focused pane. |
| `AddItemToTab(itemID string, tab int)` and `RemoveItemFromTab(itemID string, tab int)` | Show an item in that tab's focused pane as well as where it is; take an item that's in several tabs out of one (see [The window model](#the-window-model)). |
| `FocusDirection(dir string)` | Focus the pane `left`, `right`, `up`, or `down`. |
| `MoveDirection(dir string)` | Move the focused pane's shown item into the neighbouring pane, or split there. |
| `CycleSubtab(step int)` | Next (1) or previous (-1) sub-tab in the focused pane. |
| `AddOptions(paneID string) ([]AddOption, error)` | The add dialog's rows, in group order. |
| `AddToPane(paneID string, source Source)` | Add a floating item, an item from other tabs (as well as where it is), a new window, an installed app, a terminal, or a web page. |
| `AddWebsite(paneID, address string) (WebsiteResult, error)` | Add a website from the add dialog's **Web page…**: the site's web app if one is installed; else, if Bench can dock web apps and the default browser can make them, nothing yet, as it opens the site in the browser and returns the browser's steps; else a Web item, and why. See decision 21. |
| `AddWebAppWhenReady(paneID, address string) (bool, error)` | Add the site's web app to the pane once the browser has made it, and report whether it did. The add dialog calls it every 1.5 seconds while it waits. |
| `WebAppChoice(itemID string) (WebAppChoice, error)` | What a Web item's menu offers to put a web app of its site in its place: `use` an installed web app, named with the browser that made it; or else, if Bench can dock web apps and the default browser can make them, `make` one in that browser; or else nothing. Worked out afresh each time the menu opens. See decision 22. |
| `UseWebApp(itemID string)` | Put the installed web app of a Web item's site in the item's place, in every tab that shows it, and close the item's web view. If the item came from `config.toml`, its entries there become `app` entries for the web app, and the file is saved, as **Tab properties** saves it. |
| `MakeWebApp(itemID string) (WebsiteResult, error)` | Open a Web item's site in the default browser, for the user to make a web app there, and return the browser's steps, as `AddWebsite` does for a site with none. |
| `UseWebAppWhenReady(itemID string) (bool, error)` | Put the web app that the browser has made in the Web item's place, as `UseWebApp` does, once it's installed, and report whether it did. The dialog that `MakeWebApp` opens calls it every 1.5 seconds while it waits. |
| `PopOut(itemID string)` and `Redock(itemID string)` | Pop out of Bench; dock an undocked window again. An App item with no window has nothing to pop out, so `PopOut` removes it from every tab instead: the frontend calls that **Remove from Bench**, for an item whose view has `noWindow`. |
| `OpenItem(itemID string)` | **Open** on a pane that's waiting for an app: open the app of an App item without a window, or ask the running app for a new window, and put its next window in the item's place. The item shows, saying that Bench is opening its app, until the window comes or Bench gives up. Without Accessibility permission, it opens the app, for an item whose view has `canOpen`, which the item's pane offers then. |
| `RemoveRemembered(paneID, bundleID string)` | **Remove from this pane**: forget the places of an app in a pane that its windows would go back to, the hidden App items without a window. An item that's in other tabs too stays there. |
| `ClosePane(paneID string)` | **Close this pane**: close a pane that shows nothing, forgetting the places it's waiting for, so the rest of the tab closes up. Not the tab's last pane, and not in the `tabs` view. |
| `SetSizes(splitID string, sizes []float64)` | When the user drags a divider. |
| `Rearrange()` | Arrange the current tab again. A tab's menu switches to another tab first. |
| `PreviewTree(index int, preset string) (*layout.Node, error)` | The tree that **Tab properties** would make for tab `index` (or a new tab, -1) with a preset, for its preview. |
| `SaveTab(index int, draft TabDraft)` | Save **Tab properties**: a tab's name, layout, and small-screen setting. `index` -1 adds a tab. A name that another tab has, ignoring case, is numbered (`uniqueTabName`): "Code" becomes "Code (1)", then "Code (2)", replacing a number in brackets that it ends with. Tab properties says so under the name first, working it out the same way. |
| `SaveSettings(settings SettingsView)` | Save **Settings**: the `[settings]` that apply to every tab, such as `compact_below` and `live_previews`. Turning live previews on asks for Screen Recording permission. |
| `DeleteTab(index int)` and `ResetLayout(index int)` | From **Tab properties**. `ResetLayout` is in a tab's menu too, and says what it did in a toast, since the tab may not be the current one. |
| `TermInput(itemID, data string)` and `TermResize(itemID string, cols, rows int)` | Terminal input and size. Return in a terminal whose command has finished runs the command again. |
| `TermSnapshot(itemID string) (TermSnapshot, error)` | Recent output, base64-encoded, for a new terminal view to replay, and the offset where it ends (see [Terminal output](#terminal-output)). |
| `TermAck(itemID string, offset int64)` | A terminal view has drawn the output up to `offset`. |
| `OpenLink(address string)` | Open an `http` or `https` link that a terminal program printed, and the user Command-clicked, in the default browser. The browser preview refuses. |
| `OpenInBrowser(itemID string)` | Open the page that a Web item shows in the default browser: the one its web view last reported, or until it reports one, after a restart too, the address the item was given. For a page that isn't `http` or `https`, it opens a `blob:` page's inner origin if that is one, or else the item's own address. Only `http` and `https` addresses open, and not VS Code for the Web. The browser preview refuses. See decision 23. |
| `SetOverlay(open bool)` | Raised mode on or off (see [The window model](#the-window-model)). Lowering Bench after the jump shortcut opened the palette gives the keyboard back to the App window that had it (decision 15). |
| `ShowPreview(itemID string, anchor layout.Rect)` and `HidePreview()` | Live previews, with the sub-tab's rectangle in content coordinates. |
| `RequestAccessibility()` and `RequestScreenRecording()` | Ask macOS for permission. |
| `SetOpenAtLogin(on bool)` | Open Bench at login. |
| `DismissFirstRun()` | Close the permissions guide for good. |
| `SetDemoScreen(screen string, fitWidth float64)` | Browser preview only: simulate `fit`, `monitor`, or `laptop`. |

### Events

| Event | Payload |
| --- | --- |
| `bench:state` | `workspace.View`. |
| `bench:toast` | A short message, as a string. |
| `bench:command` | `workspace.Command`, such as `{"name": "toggle-palette"}` when the jump shortcut is pressed, `{"name": "settings"}` from **Bench ▸ Settings…**, `{"name": "edit-tab"}` (the current tab's properties) from **View ▸ Tab Properties…**, or `{"name": "focus-terminal", "itemId": "w7"}` to give a terminal the keyboard. For the commands that need Bench's window, such as the palette and Settings, the shell brings Bench forward first; for `focus-terminal`, the engine already has (decision 15). |
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

## Files

All in `~/Library/Application Support/Bench/`, or `$BENCH_HOME`. Bench runs
the commands in `config.toml` and acts on `parked.json` and `vscode.json`, so
`config.CheckDir` refuses a `$BENCH_HOME` or `--dir` folder that another user
owns or can change, or that sits in a folder they can change, and creates a
missing one readable only by the user. The files and folders Bench creates
are private too (0600 and 0700); an existing file keeps its permissions.

| File | What it is |
| --- | --- |
| `config.toml` | Tabs, layouts, settings, shortcuts, and rules. Created on first run, with one tab holding the default browser (decision 28); **Settings** and **Tab properties** update it, and so does `UseWebApp` (or `UseWebAppWhenReady`) for a web page that it lists (decision 22). |
| `config.toml.bak` | The previous `config.toml`, kept by every save. |
| `state.json` | The live arrangement, saved half a second after it changes, and within 3 seconds however often it changes. New titles and addresses alone are saved within 30 seconds. Set aside as `state.json.bad` if it doesn't parse or match the config. Each tab records its config entries' keys (an App entry's app and title, a Web entry's address, a VS Code entry's folder, a terminal's folder and command), so that at startup the entries edited, removed, or added in `config.toml` since can be applied to the arrangement. Its `apps` holds, for each app with items, by lower-case bundle ID, whether the user quit it (`quit`, decision 28) and when Bench last saw it running (`seen`, decision 10), and `ran` when Bench last ran. Older builds ignore them. |
| `parked.json` | Windows that are parked right now, with where they were, and apps that Bench hid, so they can be put back after a crash. Set aside as `parked.json.bad` if it doesn't parse. |
| `vscode.json` | The running `code serve-web`, so that it can be stopped after a crash. |
| `vscode-token-*` | The running `code serve-web`'s connection token, readable only by the user. It's here, not in `$TMPDIR`, which macOS clears of old files, because the server rereads it whenever it starts VS Code. |
| `bench.lock` | The support folder's lock, which every process that acts on `parked.json` and `vscode.json` holds while it does (see [Crash safety](#crash-safety)). |
| `first-run-done` | Present once the permissions guide has been dismissed. |
| `view.json` | Only with `BENCH_DEBUG` set: the latest `View`, as the frontend gets it, for scripts such as the macOS demo. |

Bench saves `state.json` 500 ms after the last change. A save from Tab
properties, which can rename, add, or delete tabs, saves it straight after
`config.toml` instead, because a `state.json` whose tabs differ from
`config.toml`'s is set aside, and the arrangement with it. A web page
switched to a web app saves them in the same way, because `state.json`'s
entry keys must name the entries in the `config.toml` on disk: keys from
the new file with the old one would make the web app's entry look removed,
and the page's look added. Quitting waits for a save from Settings or Tab
properties in progress, so it can't come between the two. If `config.toml`
can't be written, Bench saves no `state.json` until a later save from
Settings or Tab properties writes `config.toml`, so the two files on disk
still match, and says so in the status bar.

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
   only arranging windows waits for the permission.
2. **Wails v3 is pinned to v3.0.0-beta.27.** Its server mode (the `server`
   build tag) gives a browser preview of the real app on any OS, which the
   end-to-end tests use.
3. **The menu bar item and Open at login use Wails' `SystemTray` and
   `Autostart` managers** (`SMAppService` on macOS).
4. **Raised mode** (see [The window model](#the-window-model)) answers the
   milestone 4 question of how menus, dialogs, and drags appear above docked
   windows. It needs checking on a Mac.
5. **The frontend has no build step.** It's plain ES modules, with xterm.js and
   the IBM Plex fonts vendored in `frontend/vendor`, so the app works offline.
6. **Splits have IDs** (`s1`, `s2`, …) as well as panes (`p1`) and items
   (`w1`), so `SetSizes` can name a split.
7. **Config problems don't stop Bench.** If `config.toml` has problems, Bench
   reports them in the status bar and runs with the default config, without
   overwriting the file. Until the file is fixed, Bench saves neither
   `config.toml` nor `state.json`, so that the arrangement made for the
   user's own config isn't overwritten.
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
    of Bench's dividers. It puts back any other move or resize of a docked
    window 300 ms later, but at most three times in ten seconds for one
    window: after that, it leaves the window where its app put it until
    the tab is next arranged, so that Bench and the app don't fight.
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
    Bench. While Bench's window is hidden, focusing waits until
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
    Bench again, and the note waits until that closes. Web items and
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
    that no item stands for, and a `state.json` that's set aside loses the
    switch. So the Web item's entries in `config.toml`, found as
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
    static page, `frontend/acknowledgements.html`, with no scripts and no
    links out of it, since it has no address bar. Wails holds the events
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
    a window of their own. While `config.toml` has problems, Bench runs
    with the same config. The fuller example config (`config.Example`), with Slack,
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
