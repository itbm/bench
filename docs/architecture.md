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
| `internal/platform` | The `Platform` interface, `ParkFrame`, `Fake` for tests, and `NewDemo` for the browser preview. | No |
| `internal/windows` | Docking, stacking, parking, hiding, and restoring other apps' windows, through `Platform`. | No |
| `internal/workspace` | The model (tabs, items, moves, rules) and the engine that runs it: platform events, switching, arranging, persistence, and the `View` sent to the frontend. | No |
| `internal/term` | Terminal sessions in pseudo-terminals (`creack/pty`). | No |
| `internal/vscode` | `code serve-web` for VS Code for the Web. | No |
| `internal/apps` | Installed apps, monograms, and display names. | No |
| `internal/shortcuts` | The `[shortcuts]` config as global shortcut bindings. | No |
| `internal/clock` | A clock that tests can move by hand. | No |
| `internal/ui` | The Wails app: the window, the bound `Service`, events, global shortcuts, the menu bar item, the app menu. | Through Wails |
| `internal/mac` | macOS only: Objective-C for the Accessibility API, NSWorkspace, screens, window levels, web views, ScreenCaptureKit, and notifications. It implements `platform.Platform`. | Yes |
| `frontend` | The UI, from the prototype. Plain HTML, CSS, and ES modules, with no build step. | — |

Only `internal/mac` contains cgo. Everything else builds and tests on any OS.

## Builds

| Build | Command | What you get |
| --- | --- | --- |
| The app (macOS) | `go build ./cmd/bench`, or `make app` for a signed `Bench.app` | Bench, using `internal/mac`. |
| Browser preview (any OS) | `go run -tags server ./cmd/bench` | Wails' server mode: the same app with no native window, serving the UI at <http://localhost:8080>, backed by the demo platform (`platform.NewDemo`) and pretend terminals (`term.Echo`). Window contents are placeholders, as in the prototype. |
| Core tests (any OS) | `go test ./...` | Every package except `internal/ui` and `internal/mac`. |
| Preview tests | `go test -tags server ./...` and `node --test e2e/` | Adds `internal/ui`, and drives the preview in headless Chromium. |

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
- `internal/mac` runs AppKit work on the main thread. Native callbacks into Go
  (AX notifications, web view delegates, NSNotifications) never take Go locks
  or block: they make an `Event` and send it on a buffered channel without
  waiting, dropping coalescible events (moves, resizes) if it's full.
- Go calls into AppKit with `dispatch_async` when no answer is needed. Queries
  that need an answer use `dispatch_sync` to the main queue, which is safe
  because the main thread never waits for Go.
- Accessibility calls run on the calling goroutine; they don't need the main
  thread.

## The window model

Bench uses approach A from the spec, with one addition.

- Bench's window sits one level below normal windows
  (`NSNormalWindowLevel - 1`). Docked App windows stay at the normal level, so
  they're always above Bench, inside their pane bodies. Clicking Bench's tabs,
  sub-tabs, and pane headers activates Bench without covering them.
- Web items are `WKWebView`s added to Bench's window above the UI's web view,
  positioned at their pane bodies. Terminals are drawn by the UI itself.
- **Raised mode.** Menus, dialogs, live previews, and drags would be hidden
  behind docked windows and web views, and a drag can't reach a pane body
  that another app's window covers. So while any of them is open, the
  frontend calls `SetOverlay(true)`: Bench raises its window above the docked
  windows (`Platform.SetRaised`) and hides its web views. `SetOverlay(false)`
  puts Bench back below normal level, raises the docked windows again, and
  shows the web views. The docked windows are behind the dialog's scrim
  anyway, so nothing visible is lost.
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
| `FocusDirection(dir string)` | Focus the pane `left`, `right`, `up`, or `down`. |
| `MoveDirection(dir string)` | Move the focused pane's shown item into the neighbouring pane, or split there. |
| `CycleSubtab(step int)` | Next (1) or previous (-1) sub-tab in the focused pane. |
| `AddOptions(paneID string) ([]AddOption, error)` | The add dialog's rows, in group order. |
| `AddToPane(paneID string, source Source)` | Add a floating item, a new window, an installed app, a terminal, or a web page. |
| `PopOut(itemID string)` and `Redock(itemID string)` | Pop out of Bench; dock an undocked window again. |
| `SetSizes(splitID string, sizes []float64)` | When the user drags a divider. |
| `Rearrange()` | Arrange the current tab again. |
| `PreviewTree(index int, preset string) (*layout.Node, error)` | The tree the layout editor would make for tab `index` (or a new tab, -1) with a preset, for its preview. |
| `SaveTab(index int, draft TabDraft)` | Save the layout editor; `index` -1 adds a tab. |
| `DeleteTab(index int)` and `ResetLayout(index int)` | From the layout editor. |
| `TermInput(itemID, data string)` and `TermResize(itemID string, cols, rows int)` | Terminal input and size. |
| `TermSnapshot(itemID string) (string, error)` | Recent output, base64-encoded, for a new terminal view to replay. |
| `SetOverlay(open bool)` | Raised mode on or off (see [The window model](#the-window-model)). |
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
| `bench:command` | `workspace.Command`, such as `{"name": "toggle-palette"}` when the jump shortcut is pressed. |
| `term:data` | `workspace.TermData`: an item ID and base64-encoded output. |

## Files

All in `~/Library/Application Support/Bench/`, or `$BENCH_HOME`:

| File | What it is |
| --- | --- |
| `config.toml` | Tabs, layouts, settings, shortcuts, and rules. Created on first run; the layout editor updates it. |
| `config.toml.bak` | The previous `config.toml`, kept by every save. |
| `state.json` | The live arrangement. Set aside as `state.json.bad` if it doesn't parse or match the config. |
| `parked.json` | Windows that are parked right now, with where they were, so they can be put back after a crash. |
| `first-run-done` | Present once the permissions guide has been dismissed. |

## Crash safety

Bench moves other apps' windows off screen. If it quit without putting them
back, they'd be stranded, so:

1. `windows.Manager` records every parked window in `parked.json` as it parks
   it, and removes it as it unparks it.
2. On a clean quit (the app menu, a signal, or Wails' shutdown hook), Bench
   calls `RestoreAll`, then deletes `parked.json`.
3. At startup, Bench first restores anything left in `parked.json` by a run
   that crashed.
4. The app starts `bench watchdog` as a child process, connected by a pipe.
   If the pipe closes while `parked.json` still lists windows (Bench crashed,
   even in native code), the watchdog restores them. Child processes of an
   app bundle share its Accessibility permission.

## Decisions

Choices made while building that the spec left open or that differ from it:

1. **Global shortcuts use Wails' `GlobalShortcut` manager**, not
   `golang.design/x/hotkey`. Wails v3 (beta.27) registers them with Carbon's
   `RegisterEventHotKey` on the main thread, which is the spec's own fallback,
   and reports registration failures, which Bench shows in the status bar.
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
   overwriting the file.
8. **The browser preview never runs real shells.** Its terminals are
   `term.Echo`, because the preview's server has no authentication.
9. **The add dialog's rows come from Go** (`AddOptions`), so the frontend
   doesn't need the installed-apps list.
