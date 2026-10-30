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
| `internal/layout` | Trees, presets, unit rectangles, frames, dividers, size class, small-screen changes, neighbours, and where a drop on a pane lands (`DropAt`, decision 38). Pure Go. | No |
| `internal/config` | `config.toml` and `state.json`: load, validate, format, save, defaults. | No |
| `internal/platform` | The `Platform` interface, `ParkPlace` and `ParkFrame`, `Fake` for tests, `NewDemo` for the browser preview, and `Logged`, which logs calls and events when `BENCH_DEBUG` is set, and gives a `DebugLogger`, such as the macOS platform, the log for decisions of its own. | No |
| `internal/windows` | Docking, stacking, parking, hiding, and restoring other apps' windows, through `Platform`. | No |
| `internal/workspace` | The model (tabs, items, moves, rules) and the engine that runs it: platform events, switching, arranging, persistence, **Undo** and **Reopen closed tab** (decision 37), and the `View` sent to the frontend. | No |
| `internal/term` | Terminal sessions in pseudo-terminals (`creack/pty`). A session lasts until the engine removes its item, which closes it. `Busy` says whether a session is running something that ending it would stop, and what (decision 35). A session can type a command into its shell once the shell's line editor is ready, for launchers (`Options.Input`, decision 41), and `Host.LookPath` asks the user's interactive login shell which program a name runs, for **Browse…** (decision 41). A session reports the bell and the notifications its program asks for with OSC 9 and OSC 777 (`Callbacks.Alert`), and the folder its shell moves to (`Callbacks.Folder`), and can start with output to show first (`Options.Preface`), which `Restorable` makes safe to draw again (decisions 43 and 44). When Bench runs with SIGHUP ignored, as under `nohup`, it starts each shell through its own binary, run as a helper that puts SIGHUP back to its default action and then becomes the shell, so that closing a session still ends its programs. | No |
| `internal/vscode` | `code serve-web` for VS Code for the Web. | No |
| `internal/icons` | The small PNGs that tiles show in place of monograms: a web page's icon, chosen from its `<link>` elements or `/favicon.ico`, fetched without cookies, and decoded and scaled in Go (`Fit`, `Candidates`, and `Pages`, decision 51). Pure Go. | No |
| `internal/apps` | Installed apps, from the usual folders and the folders one level inside `/Applications`, including the web apps that browsers make and the sites they open, the browsers that can make them, monograms, and display names. | No |
| `internal/shortcuts` | The `[shortcuts]` config as global shortcut bindings, with what each field does, how macOS shows a shortcut, suggestions for one that's taken, and the clashes macOS doesn't report: VoiceOver's keys and the shortcuts macOS keeps (decision 45). | No |
| `internal/clock` | A clock that tests can move by hand. | No |
| `internal/update` | Bench's version against its latest release on GitHub, for the `check_for_updates` setting in a Bench without Sparkle, and the releases page that **Bench ▸ Check for Updates…** opens there. It only looks: Sparkle downloads and installs new versions (decision 48). | No |
| `internal/ui` | The Wails app: Bench's windows (the main window, popped-out terminals' windows, and Acknowledgements), with the appearance setting's look (decision 52), the bound `Service`, events, global shortcuts, the menu bar item, the app menu and **Report a Problem…** (decision 46), the log file (decision 47), whether Bench may quit (`ShouldQuit`, decision 35), opening Bench again after it quits, for **Quit and Reopen Bench** (`reopenAfterQuit`, decision 48), and the folder picker (decision 41), and the icons the tiles show, which the asset server serves (`iconHandler`, decision 51). | Through Wails |
| `internal/mac` | macOS only: Objective-C for the Accessibility API, NSWorkspace, screens, window levels, web views, ScreenCaptureKit, and notifications. It implements `platform.Platform`, including `PressClose`, which closes another app's window as its close button does (decision 36), and `ChooseToAdd`, the open panel of the add dialog's **Browse…** (decision 41), and `SetBadge`, the count on Bench's Dock icon (decision 43), and `SystemKeys`, the input sources' shortcuts and whether VoiceOver is on (decision 45), and `OpenSettings` and `LoginItemNeedsApproval`, System Settings' panes for permissions and whether macOS needs the user to allow Bench's login item (decision 48), and `TrackDrags`, `ShowDropGuide`, and `HideDropGuide`, which follow windows dragged by their title bars and show where a drop on a pane goes (`drag_darwin.m`, `guide_darwin.m`, and `guide.go`, decision 13), and a web page's controls, `ReloadWebView`, `NavigateWebView`, `ZoomWebView`, `FindInWebView`, `SetCaptureState`, and `PauseWebViewMedia`, with `WebStateChanged` for what the page can do and how it uses the camera and microphone (`web_darwin.m` and `pagecontrols.go`, decision 50), and `StageManager`, whether Stage Manager is on, with `StageManagerChanged` (decision 53), and `AppIcon` and `PageIcon`, an app's icon, which NSWorkspace draws, and a Web item's page's, with `PageIconChanged` (`apps_darwin.m`, `web_darwin.m`, and `icons.go`, decision 51). Outside the interface, `Updater` loads Sparkle at run time, if `Bench.app` has it, and implements `workspace.Updater` (`updater_darwin.m`, decision 48), `SetWindowAppearance` gives Bench's own windows and panels the appearance setting, `SystemDark` says whether macOS is dark, before the app runs too, and Web items' web views follow macOS's appearance (`appearance.go`, decision 52). Its pure-Go helpers build everywhere, among them the Web items' navigation policy (decision 24), and the browser preview uses one, `PageInfo`, for its web pages' hosts. | Yes |
| `frontend` | The UI, from the prototype. Plain HTML, CSS, and ES modules, with no build step. `styles.css` has the dark colours and `light.css` the light ones, which `js/theme.js` chooses (decision 52). `js/tiles.js` draws apps' and pages' icons in place of monograms (decision 51). `acknowledgements.html` is the Acknowledgements window's page, which `build/acknowledgements` writes. | — |
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
- Wails asks whether Bench may quit (`ShouldQuit`) on the main thread, which
  mustn't wait for the engine's lock, as the engine may hold it while it
  waits for the main thread. So the engine answers there only if it can take
  the lock at once (`AskToQuitNow`, with `TryLock`), and otherwise the shell
  refuses the quit and asks on another goroutine, quitting again if it may
  (decision 35).
- Go never waits for the main thread. It sends AppKit work there with
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
  tab's apps hide with it, and a tab shortcut brings Bench back on that tab
  (decision 16).
- **Other windows.** Bench's other windows are ordinary windows at the
  normal level: popped-out web pages and terminals, web pages' popups, and
  Acknowledgements (decision 27). Raised mode floats only Bench's main
  window, and only while none of them has the keyboard, so that one that
  the user opens or clicks stays in front of it. Bench becoming active
  hides apps over its main window only while that window is shown, since
  the user may have clicked one of these instead. Windows that a
  `float = true` rule matches, such as picture-in-picture, don't count:
  they're meant to stay over Bench, which fills the screen by default
  (decision 30). A popped-out web page's or terminal's window has the
  identifier `bench.item:` and its item's ID, for `internal/mac` to report
  it getting and losing the keyboard (`PopOutFocused`, `PopOutUnfocused`)
  and the user dragging it (`WindowDragged`, `WindowDropped`), and has a
  **Back to Bench** control: a title bar button on a web page's window,
  which sends `DockRequested`, and a bar and a menu entry in a terminal's
  page, which call `BackToBench` (decision 13).
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
| `LastTab()` | Back to the tab that was current before this one, as the `last_tab` shortcut and **Tab ▸ Last Used Tab** do, so that it goes back and forth between the two tabs used last. The View's `lastTab` names it. Refused until Bench has switched tabs, and once that tab is deleted (decision 49). |
| `SetMode(mode string)` | `auto`, `wide`, or `compact`. |
| `FocusPane(paneID string)` | Focus a pane. |
| `FocusUI()` | Give Bench's own UI the keyboard, for the field that renames a tab or sub-tab in place: take it from the window or page that the focused pane shows, which the switch of the click before a double-click on a tab may have given it, or that lowering Bench after a menu gives back to a page, and drop any focus still put off. The frontend calls it as the field opens, once a click's `SwitchTab` has answered (decision 39). |
| `ShowItem(paneID, itemID string)` | Show an item in its pane (or in the `compact` pane). Also focuses the pane. |
| `MoveItem(itemID string, tab int, paneID, zone string)` | `zone` is `center`, `left`, `right`, `top`, or `bottom`: a drop on a pane's body, which splits at its edges (decision 38). |
| `MoveItemAt(itemID string, tab int, paneID string, index int)` | A drop on a pane's header, and a sub-tab's **Move left** and **Move right**: put an item in a pane at `index` among its sub-tabs as drawn, without hidden ones, before the sub-tab there, or last for an index past them. It never splits. Along its own pane, only the order changes; from another pane or tab, it joins this one there. In the `tabs` view, `compact` stands for the tab's one strip, in reading order: the item stays in its pane if a sub-tab beside its new place is in it, or else joins the pane of the sub-tab it goes before, or after, leaving its own pane in place, even empty. Nothing changes, and there's no toast, for a place next to where it is. See decision 38. |
| `MoveItemToTab(itemID string, tab int)` | Into that tab's focused pane. |
| `AddItemToTab(itemID string, tab int)` and `RemoveItemFromTab(itemID string, tab int)` | Show an item in that tab's focused pane as well as where it is; take an item that's in several tabs out of one (see [The window model](#the-window-model)). |
| `MoveItemToNewTab(itemID string, also bool)` | **Move to new tab**, or with `also`, **Also show in new tab**: from a window's menu, the jump palette, and a sub-tab dropped on the tabs' **+**, or just after it (with Option held, `also`). In one step, add a tab with the single layout after the others, named after the item's app, numbered if a tab has that name (`uniqueTabName`), and move the item there, or show it there as well as where it is; then switch to it, and save `config.toml` with the tab and the item in it, as **Tab properties** saves a new tab. Its toast has **Undo**, which takes the tab away again, from `config.toml` too, and puts the item back. A popped-out item can move, but not also show. See decision 39. |
| `RenameItem(itemID, label string)` | A terminal's or web page's **Rename…**: give it a name of its own, one line of up to 200 characters, which its sub-tab, its tooltip, toasts, and the questions before closing show in place of the title that its shell or page sets. `""`, which **Use automatic title** sends, takes the name away. `state.json` keeps it; the item's view has it as `label`, with `title` showing it and `autoTitle` the title it hides. Refused for an app's window. See decision 39. |
| `FocusDirection(dir string)` | Focus the pane `left`, `right`, `up`, or `down`. |
| `MoveDirection(dir string)` | Move the focused pane's shown item into the neighbouring pane, or split there. |
| `CycleSubtab(step int)` | Next (1) or previous (-1) sub-tab in the focused pane. |
| `AddOptions(paneID string) ([]AddOption, error)` | The add dialog's rows, in group order: the launchers first, then windows not in a tab, and so on. **New terminal…** has `asksForCommand`, for the form that asks for a name, a command, and a folder. The first **Built in** row is **New terminal in *folder***, a `terminal` source with the folder that the shell of the terminal in the pane, or else the focused pane, is in, unless that's the home folder; **New terminal in…** has `choosesFolder`, for the frontend to ask for a folder (`ChooseFolder`), starting in that one (decision 44). |
| `AddToPane(paneID string, source Source)` | Add a floating item, an item from other tabs (as well as where it is), a new window, an installed app, a terminal, a launcher, or a web page. A `terminal` source with a `command` types it into the user's shell and runs it, with its `name` as the label until the shell sets a title, and `startAgain` (missing means true) says whether it runs again when Bench next opens, or is only typed. A `launcher` source's `id` names a `[[launcher]]`. See decision 41. |
| `AddWebsite(paneID, address string) (WebsiteResult, error)` | Add a website from the add dialog's **Web page…**: the site's web app if one is installed; else, if Bench can dock web apps and the default browser can make them, nothing yet, as it opens the site in the browser and returns the browser's steps; else a Web item, and why. See decision 21. |
| `AddWebAppWhenReady(paneID, address string) (bool, error)` | Add the site's web app to the pane once the browser has made it, and report whether it did. The add dialog calls it every 1.5 seconds while it waits. |
| `WebAppChoice(itemID string) (WebAppChoice, error)` | What a Web item's menu offers to put a web app of its site in its place: `use` an installed web app, named with the browser that made it; or else, if Bench can dock web apps and the default browser can make them, `make` one in that browser; or else nothing. Worked out afresh each time the menu opens. See decision 22. |
| `UseWebApp(itemID string)` | Put the installed web app of a Web item's site in the item's place, in every tab that shows it, and close the item's web view. If the item came from `config.toml`, its entries there become `app` entries for the web app, and the file is saved, as **Tab properties** saves it. |
| `MakeWebApp(itemID string) (WebsiteResult, error)` | Open a Web item's site in the default browser, for the user to make a web app there, and return the browser's steps, as `AddWebsite` does for a site with none. |
| `UseWebAppWhenReady(itemID string) (bool, error)` | Put the web app that the browser has made in the Web item's place, as `UseWebApp` does, once it's installed, and report whether it did. The dialog that `MakeWebApp` opens calls it every 1.5 seconds while it waits. |
| `PopOut(itemID string)` and `Redock(itemID string)` | Pop out of Bench; dock an undocked window again. An App item with no window has nothing to pop out, so `PopOut` removes it from every tab instead: the frontend calls that **Remove from Bench**, for an item whose view has `noWindow`. |
| `AddFrontWindow()` | Add the window in front to the current tab's focused pane, if it's out of the tabs: a popped-out item, a window not in a tab, or one dragged out of its pane, which the View's `front` names. The window in front is the one the user last gave the keyboard to outside Bench's main window, not one that Bench focused itself. It says so with **Pop out**, or with **Undo** for a window dragged out of another pane, and brings back a hidden Bench. The `add_front_window` shortcut, **Add front window to this pane** in the jump palette, and **Pane ▸ Add Front Window** call it (decision 13). |
| `BackToBench(itemID string)` | Put a popped-out item back in the current tab's focused pane, and bring Bench forward on it, saying so with **Pop out**: the **Back to Bench** button in a popped-out terminal's window. `""` means the popped-out window that has the keyboard, for **Window ▸ Back to Bench**. An item in a tab already stays where it is (decision 13). |
| `OpenItem(itemID string)` | **Open** on a pane that's waiting for an app: open the app of an App item without a window, or ask the running app for a new window, and put its next window in the item's place. The item shows, saying that Bench is opening its app, until the window comes or Bench gives up. Without Accessibility permission, it opens the app, for an item whose view has `canOpen`, which the item's pane offers then. |
| `RemoveRemembered(paneID, bundleID string)` | **Remove from this pane**: forget the places of an app in a pane that its windows would go back to, the hidden App items without a window. An item that's in other tabs too stays there. |
| `ClosePane(paneID string)` | **Close this pane**: close a pane that shows nothing, forgetting the places it's waiting for, so the rest of the tab closes up. Not the tab's last pane, and not in the `tabs` view. |
| `SetSizes(splitID string, sizes []float64)` | When the user drags a divider. |
| `EvenSizes(splitID string)` | When the user double-clicks a divider: give its split's panes equal sizes, through `SetSizes`, and say so in a toast, with **Undo**. A split whose panes look equal already, to a twentieth of a percent, is left as it is, without a toast. See decision 40. |
| `ZoomPane(paneID string)` and `ShowAllPanes()` | **Zoom pane**, from a pane's **⋯** menu, the jump palette, **Pane ▸ Zoom Pane**, and the `zoom_pane` shortcut, and **Show all panes**, from the same places and the pane's **Zoomed** chip: `ZoomPane` makes a pane of the current tab, or the focused pane for `""`, fill the tab's pane area on its own, with its own sub-tabs, and focuses it, while the windows of the tab's other panes are parked, as a pane's are while it shows a page or a terminal. The tab keeps its panes, sizes, and items, and the View's `display.zoomed` names the pane. The zoom ends with `ShowAllPanes`, or as the tab changes, another pane is focused, or the pane goes. Refused for a tab with only one pane, and in the `tabs` view, which shows one pane anyway. Not saved in `state.json`. See decision 40. |
| `SplitPane(paneID, zone string) (string, error)` and `CancelSplit(paneID string)` | A window menu's **Split right with an empty pane** and **Split down with an empty pane**: `SplitPane` puts an empty pane beside a pane of the current tab, at the edge `zone` names (`left`, `right`, `top`, or `bottom`), focuses it, and returns its ID, for the add dialog that the frontend opens for it straight away. If the dialog closes without adding anything, `CancelSplit` takes the pane away again: the tab goes back to how it was, sizes and all, if nothing else has changed it since, or else the pane closes up as **Close this pane** closes it. A pane that has something in it by then stays, and so does one that a new window is on its way to; `CancelSplit` leaves any other pane alone. Refused in the `tabs` view. See decision 40. |
| `Rearrange()` | Arrange the current tab again. A tab's menu switches to another tab first. |
| `FillScreen()` | The jump palette's **Fill the screen**, as **View ▸ Fill the Screen** does: zoom Bench's window to fill its screen's visible area, unless it fills it already (decision 30). The browser preview refuses, and its palette leaves the row out. |
| `PreviewTree(index int, preset string) (*layout.Node, error)` | The tree that **Tab properties** would make for tab `index` (or a new tab, -1) with a preset, for its preview. |
| `SaveTab(index int, draft TabDraft)` | Save **Tab properties**: a tab's name, layout, small-screen setting, and place among the tabs (`place`, from 1, which gives it its shortcut; 0 leaves it where it is, or puts a new tab last), which moves it as `MoveTab` does. `index` -1 adds a tab. A name that another tab has, ignoring case, is numbered (`uniqueTabName`): "Code" becomes "Code (1)", then "Code (2)", replacing a number in brackets that it ends with. Tab properties says so under the name first, working it out the same way. Renaming a tab in place, by double-clicking it or with **Rename…** in its menu, saves through `SaveTab` too, with the name typed, preset `custom`, which keeps the tab's panes and its `config.toml` layout, and the tab's own small-screen setting (decision 39). |
| `SaveSettings(settings SettingsView)` | Save **Settings**: the `[settings]` that apply to every tab, such as `compact_below`, `density`, `pane_headers`, `live_previews`, `notify_terminals`, and `keep_terminal_output`, and the launchers, from **Settings ▸ Launchers**, which replace the `[[launcher]]` entries unless `launchers` is missing. One without a name is named as `SaveLauncher` names one, or an app launcher after its app. Turning live previews on asks for Screen Recording permission. Turning `keep_terminal_output` off removes the terminals' saved output (decision 44). Its `shortcuts` are the fields of the `[shortcuts]` table that **Settings ▸ Keyboard** changed, by their names in `config.toml`, such as `jump`, each a shortcut in config form or `""` for off; the others stay as they are. A change that gives the table a problem it didn't have, such as a shortcut that another field has, is refused, saying so. Changed shortcuts are registered again straight away, with no restart (`Emitter.ShortcutsChanged`, decision 45). |
| `CheckShortcuts(changes map[string]string) ([]ShortcutView, error)` | **Settings ▸ Keyboard**, as the user records a shortcut, turns one off, or uses a suggestion: whether each field of the `[shortcuts]` table would work with its changes, by the fields' names, without saving them. The engine asks the shell (`Emitter.CheckShortcuts`), which tries to register each binding that Bench doesn't hold already, releasing it again, and checks it against the shortcuts macOS keeps for itself and VoiceOver's, as it does when it registers them (decision 45). Each row has its `problem`, if any, and a `suggestion` that Bench checked it can register. A name that isn't a field is refused. |
| `PauseShortcuts(paused bool)` | Release Bench's global shortcuts while a field of **Settings ▸ Keyboard** records keys, so that the keys reach the page rather than run Bench's actions, and register them again with `paused` false. `SetOverlay(false)` and `Ready` register them again too, in case the page didn't. |
| `UseSuggestedShortcuts(fields string)` | A shortcut notice's **Use this instead**: set fields of the `[shortcuts]` table, by their names, separated by spaces, to the shortcuts the shell last suggested for them, save `config.toml` as **Settings** does, register them again, and say what changed in a toast. A field whose value has changed since the suggestion stays as it is; if none has one, it's refused. |
| `SaveLauncher(launcher LauncherView) (string, error)` | The **New terminal…** form's **Save as launcher**: add a `[[launcher]]` after the others and save `config.toml`, as **Settings** does, and return its name, for the terminal the form adds with it. One without a name is named after its command's program, or its folder, numbered if that's taken. A name that another launcher has, ignoring case, is refused, naming that launcher. See decision 41. |
| `ChooseFolder(start string) (string, error)` | The **Choose…** button of the **New terminal…** form and **Settings ▸ Launchers**, and the add dialog's **New terminal in…**: show macOS's folder picker as a sheet on Bench's window, starting in `start` (such as `~/code`), and return the folder chosen, with the home folder as `~`, or `""` if the user cancelled. It waits for the user, without the engine's lock. The browser preview refuses, and the forms say to type the folder; **New terminal in…** opens the **New terminal…** form at its folder field instead. |
| `BrowseToAdd(paneID string) (BrowseResult, error)` | The add dialog's **Browse…**, once the dialog has closed: lower Bench, show macOS's open panel as a window of its own (`Platform.ChooseToAdd`), and wait for the user, without the engine's lock. An app chosen is added to the pane, as the **Apps** rows add one (`added`, its name), if the tab is still the one it was chosen from. A program or script comes back for the **New terminal…** form to fill in (`command`, `name`, its file name, and `folder` for one run by its path): by its name when the user's shell finds that program by it (`term.Host.LookPath`), or else by its path, with what runs a script that can't run itself. A cancel returns nothing, and the dialog opens again. Anything else is refused, and so is a second panel while one is open. The browser preview refuses. See decision 41. |
| `DeleteTab(index int)` and `ResetLayout(index int)` | From **Tab properties**, and a tab's menu: **Delete tab (keep windows)** and **Reset layout**. Each says what it did in a toast, with **Undo**, since the tab may not be the current one. The last tab can't be deleted. A deleted tab joins the closed tabs that `ReopenTab` brings back. |
| `MoveTab(from, to int)` | Move tab `from` to index `to`, where it ends up: a drag along the tab strip, and **Move left** and **Move right** in a tab's menu, and the jump palette's **Move tab left** and **Move tab right**. (**Tab ▸ Move Tab Left** and **Move Tab Right** move the current tab in Go, without the frontend.) The tab takes the shortcut of its new place. The current tab stays current, each window stays in its tabs, and each tab keeps its arrangement. `config.toml`'s tabs take the same order, saved as **Tab properties** saves them, with `state.json` straight after. It says so in a toast, with **Undo**. Refused past either end. See decision 38. |
| `Undo(step int)` | A toast's **Undo**, after **Delete tab**, **Close tab…**, **Reset layout**, **Close this pane**, a move by dragging or from a menu (`MoveItem`, `MoveItemAt`, `MoveItemToTab`, `MoveTab`), **Also show in** (`AddItemToTab`), **Move to new tab** and **Also show in new tab** (`MoveItemToNewTab`), **Remove from this tab** (`RemoveItemFromTab`), and equal sizes from a divider (`EvenSizes`): put back the arrangement from before change `step`, taking back the changes since too, latest first. A deleted tab comes back with its `config.toml` entry and rules, which Bench saves, and its windows, pages, and terminals that are still open dock in it again; what has closed since stays closed, and what has come since stays where it is. A moved tab goes back to its place, in `config.toml` too, and a tab that `MoveItemToNewTab` added goes, from `config.toml` too. Refused once the change is out of reach: Bench keeps the last 10, and forgets them all when the tabs change otherwise, as when a tab is added in **Tab properties** or renamed, or `config.toml` is reloaded. See decision 37. |
| `ReopenTab(name string)` | **Reopen closed tab**, from a tab's menu and the jump palette: bring back the tab of that name that was deleted or closed most recently, of the last 5, which `state.json` keeps, where it was among the tabs, and switch to it. Its `config.toml` entry and rules come back, which Bench saves, numbered if another tab has its name now (`uniqueTabName`). Its arrangement comes back as `RestoreArrangement` brings one back: its windows that are "Not in a tab" dock in it, its pages and terminals that closed open again, and what another tab has now stays there. The View's `closedTabs` names the tabs it can bring back. |
| `CloseTab(index int, sure, dontAsk bool) (CloseQuestion, error)` | A tab's **Close tab…**, its ×, and a middle-click on it: close the tab's App windows, web pages, and terminals, other than those in other tabs too, which stay there, and delete the tab as `DeleteTab` does, with the rules that name it. The only tab stays, empty (**Close all windows…**). Unless `sure`, it closes nothing, and returns the question instead (`ask`), when `confirm_close` is on and it would close two or more things, or a terminal that's running something, and, with `confirm_close` off, when it would end a terminal that's running something and `confirm_quit_busy_terminals` is on (`busyOnly`). The question has what it would close and what stays, how many of them the tab's `config.toml` entry lists, how many rules go, and the busy terminals; its **Close** calls again, `sure`. With `dontAsk` (**Don't ask again**), it turns `confirm_close` off, or for a `busyOnly` question, `confirm_quit_busy_terminals`, in the same save. See decision 36. |
| `CloseItem(itemID string, sure, dontAsk bool) (CloseQuestion, error)` | A sub-tab's **Close window**, **Close page**, or **Close terminal**, or **Close everywhere** for one in several tabs, its ×, and a middle-click on it: close the item, from every tab. The frontend calls `RemoveItemFromTab` instead for the × and middle-click on an item that's in other tabs too. Unless `sure`, with `confirm_quit_busy_terminals` on, a terminal that's running something closes nothing, and returns the question (`ask`), as closing its own window would; with `dontAsk`, it turns that setting off. See decision 36. |
| `TermInput(itemID, data string)` and `TermResize(itemID string, cols, rows int)` | Terminal input and size. Return in a terminal whose command has finished runs the command again. |
| `TermSnapshot(itemID string) (TermSnapshot, error)` | Recent output, base64-encoded, for a new terminal view to replay, and the offset where it ends (see [Terminal output](#terminal-output)). |
| `TermAck(itemID string, offset int64)` | A terminal view has drawn the output up to `offset`. |
| `TermClear(itemID string)` | A terminal view's **Clear** (⌘K): forget the session's recent output, so that `TermSnapshot` has none of it, nor does the output Bench keeps as it quits. Offsets carry on. See decision 42. |
| `OpenLink(address string)` | Open an `http` or `https` link that a terminal program printed, and the user Command-clicked, in the default browser. The browser preview refuses. |
| `OpenInBrowser(itemID string)` | Open the page that a Web item shows in the default browser: the one its web view last reported, or until it reports one, after a restart too, the address the item was given. For a page that isn't `http` or `https`, it opens a `blob:` page's inner origin if that is one, or else the item's own address. Only `http` and `https` addresses open, and not VS Code for the Web. The browser preview refuses. See decision 23. |
| `ReloadPage(itemID string)`, `GoBack(itemID string)`, and `GoForward(itemID string)` | A web page's **Reload**, **Back**, and **Forward**, from the **Page** group of its **⋯** and sub-tab menus, the jump palette's **This window**, and **Pane ▸ Page** (decision 50). `ReloadPage` loads the page in view again, or the item's own address if no page has come into view (`Platform.ReloadWebView`). `GoBack` and `GoForward` go back and forward in its history (`Platform.NavigateWebView`), and are refused while its web view says it can't (`WebStateChanged`), which the item's view has as `canGoBack` and `canGoForward`. Each is refused for a window that isn't a Web item, and until its web view has been made, as for VS Code for the Web while its server starts. |
| `ZoomPage(itemID string, step int)` | **Zoom in** (1), **Zoom out** (-1), and **Actual size** (0) for a Web item's page: to the next of 50, 67, 75, 80, 90, 100, 110, 125, 150, 175, 200, 250, and 300%, through `WKWebView.pageZoom` (`Platform.ZoomWebView`), saying so in a toast. Refused past either end. The item keeps it, as its view's `zoom` in percent, left out at 100%, and `state.json` saves it, for its web view to take whenever it's made (`WebView.Zoom`). |
| `PageLink(itemID string) (string, error)` | **Copy page link**: the address that `OpenInBrowser` would open, which the frontend puts on the clipboard. Refused for VS Code for the Web, whose address holds the server's connection token, and for a page with no web address. |
| `SetFindBar(itemID string, open bool)` and `FindInPage(itemID, text string, backwards bool) (bool, error)` | **Find in page**, from a page's menus, the jump palette, and **Pane ▸ Page ▸ Find in Page…** (the `find-in-page` command): `SetFindBar` opens or closes a Web item's find bar, which the frontend draws at the top of its pane's body while its view has `finding`, `layout.FindBarHeight` (36 pt) tall, as Go moves its web view down by as much, since the web view is drawn over Bench's UI. Opening it gives Bench's own UI the keyboard, for the bar's field; closing it gives the page the keyboard back, if its pane is focused. A page in a window of its own has no bar. `FindInPage` finds text in the page, ignoring case, after the match that's selected, or before it if `backwards`, going round at the end, and WebKit selects the match and scrolls to it (`Platform.FindInWebView`, `findString:withConfiguration:completionHandler:`). It reports whether there's a match, waiting for the page's answer, up to 3 seconds, without the engine's lock. |
| `MuteCapture(itemID, device string, mute bool)` and `StopMedia(itemID string)` | **Mute microphone** and **Turn off camera**, and with `mute` false, **Unmute microphone** and **Turn on camera**, for a page that uses them (`device` is `camera` or `microphone`): `Platform.SetCaptureState`, through `setMicrophoneCaptureState:` and `setCameraCaptureState:`, on the item's page and its popups' pages that use it. The page stays in its call, and can't undo it itself. Refused for a device the page isn't using. **Stop sound and video** pauses every sound and video the page and its popups play (`Platform.PauseWebViewMedia`, `pauseAllMediaPlaybackWithCompletionHandler:`), which a page can play again; Bench can't tell whether a page plays sound, so it's always offered. Each says what it did in a toast. See decision 50. |
| `ReloadConfig()` | **Reload config.toml**, from the Bench menu, the jump palette, and the config notices' **Reload**: read `config.toml` again and apply it as startup applies it to `state.json`, keeping the arrangement (decision 7). If the file has problems, or can't be read, Bench keeps the config it runs with and reports them; if it isn't there, Bench writes its config there. Changes that Bench couldn't save over the file give way to it. |
| `OverwriteConfig()` | The notice's **Overwrite**, after a save found `config.toml` changed outside Bench: write Bench's config over it, keeping that version as `config.toml.bak`. Refused while the file has problems. |
| `UseConfigBackup()` | The problems notice's **Use backup**: put `config.toml.bak` in place of a `config.toml` that has problems, if the backup loads without any, keeping the file with problems as `config.toml.bak`, and apply it as `ReloadConfig` does. |
| `OpenConfig()` and `ShowConfigInFinder()` | The problems notice's **Open config.toml**, in the default app for plain text (or TextEdit), and **Show in Finder**, through `Platform.OpenTextFile` and `Platform.ShowInFinder`. The browser preview refuses. |
| `RestoreArrangement()` and `DismissArrangementNotice()` | The arrangement notice's **Restore previous arrangement** and **Dismiss** (see [Files](#files)). `RestoreArrangement` gives each tab whose arrangement Bench couldn't keep, because `config.toml` had no tab of its name at startup or at a reload, that arrangement back, once a tab has its name again: its tree, with new pane and split IDs, and its items, which take the entry edits since, as at startup. A window that the tab's own items have goes back to the arrangement's item; one that another tab has now stays there. A page or terminal that only the tab has, for the same entry as one of the arrangement's, takes that one's place, so that its page or session goes on; the tab's other pages and terminals move to "Not in a tab", and so do its other windows. Items that a reload moved to "Not in a tab" as it deleted the tab come back. Refused when there's nothing to restore, or no tab has the name yet. `DismissArrangementNotice` takes the notices away; `state.json.bad` stays. |
| `SetOverlay(open bool)` | Raised mode on or off (see [The window model](#the-window-model)). Raising it also checks whether `config.toml` has changed outside Bench (decision 7). Lowering Bench after the jump shortcut opened the palette gives the keyboard back to the App window that had it (decision 15). |
| `ShowPreview(itemID string, anchor layout.Rect)` and `HidePreview()` | Live previews, with the sub-tab's rectangle in content coordinates. |
| `RequestAccessibility()` and `RequestScreenRecording()` | Ask macOS for permission, and, while Bench doesn't have it, open System Settings at the permission's list too (`Platform.OpenSettings`), since macOS asks only once (decision 48). Once Bench has asked for Screen Recording, the View's `permissions.screenRecordingReopen` offers **Quit and Reopen Bench**, as macOS applies it only as Bench opens. |
| `SetOpenAtLogin(on bool)` and `OpenLoginItems()` | Open Bench at login. If macOS needs the user to allow Bench's login item (`Platform.LoginItemNeedsApproval`), turning it on opens System Settings ▸ General ▸ Login Items, sets the View's `loginApproval`, and returns an error that says so; `OpenLoginItems` opens Login Items again. While the View has `loginApproval`, Bench reads the login item again each time it comes forward. Refused where there's no Open at login, as in the browser preview. |
| `DismissFirstRun()` | Close the welcome guide for good. |
| `StarterTabs()` and `UseStarterTabs(names []string)` | The welcome guide's **Choose a starting set-up**, the first time Bench opens (decision 48). `StarterTabs` returns the tabs it suggests, `[{name, apps: [{bundleId, name, mono}]}]`: Comms, Code, and Plan, each made only of installed apps that no tab has, one for each pane, with one of Bench's terminals (no `bundleId`) beside an editor. A tab with no installed app, or whose name a tab has, isn't suggested. `UseStarterTabs` adds the tabs it names, as it would suggest them now, after the tabs there are, and saves `config.toml` as **Tab properties** saves a new tab, with a toast; a name it doesn't suggest any more is refused, and nothing is added. **Start with one tab** (or **Don't add tabs**, with more than one tab) adds none. |
| `SetDialogSize(kind string, w, h float64)` | Remember the size the user gave a dialog by resizing it from its corner, for it to open at that size next time: `palette` (Jump to…), `add` (the add dialog), `editor` (Tab properties), `settings`, or `guide` (the welcome guide). The View's `dialogs` has the sizes, and `state.json` saves them. Where the dialog was isn't saved. |
| `Quit(dontAsk bool)` and `CloseTerminal(itemID string, dontAsk bool)` | The **Quit** of the question that Bench asks before quitting while a terminal is running something, and the **Close** of the one it asks, in the terminal's own window, before closing a popped-out terminal's window while the terminal is running something (decision 35). `Quit` quits Bench without asking again. `CloseTerminal` ends the terminal and closes its window, as closing the window does when Bench doesn't ask. With `dontAsk` (**Don't ask again**), each first turns `confirm_quit_busy_terminals` off and saves `config.toml`, as **Settings** does; Bench quits even if it can't save it. The browser preview never asks, and its `Quit` does nothing. |
| `QuitAndReopen(sure, dontAsk bool)` | **Quit and Reopen Bench**, which the welcome guide and **Settings** offer once Bench has asked for Screen Recording (decision 48). It quits as `Quit` does, and the shell opens Bench again once it has quit (`Emitter.Relaunch`). Unless `sure`, it first asks, as quitting does, while a terminal is running something: `confirm-quit` has `reopen` set, and its button, **Quit and Reopen**, calls it again, sure, with `dontAsk` as `Quit` has it. The browser preview refuses. |
| `DismissUpdate()` | The update notice's **Dismiss**: the notice of a newer version goes, and Bench doesn't announce that version again; `state.json` keeps that (decision 48). |
| `InstallUpdate()` | The update notice's **Install…**, in a `Bench.app` with Sparkle: Sparkle's window for the newer version, which offers to install it, opens (`Updater.Check`, decision 48). Refused without an updater, whose notice has **Download** instead. |
| `SetDemoScreen(screen string, fitWidth float64)` | Browser preview only: simulate `fit`, `monitor`, or `laptop`. |

### Events

| Event | Payload |
| --- | --- |
| `bench:state` | `workspace.View`. Each of its `items` has the `title` its sub-tab shows: the `label` the user gave a terminal or web page, if any, or else the title its window, page, or shell gives it, which `autoTitle` then has (decision 39). Its `density` (`standard` or `compact`) sizes the panes' chrome and its `titleBar` the title bar's, and each of `display.panes` has its `header` height, which Go docks the pane's window below (decision 33). Its `settings.appearance`, `system`, `light`, or `dark`, chooses the frontend's colours (`theme.js`, decision 52). Its `display.zoomed` names the pane that `ZoomPane` shows alone, which is then `display.root` and the only one of `display.panes`, with no dividers (decision 40). Its `settings.launchers` are the `[[launcher]]` entries, each with the `mono` and `hint` the lists show (decision 41). An item's `attention` says why it needs the user, for the bell on its sub-tab, and a tab's `attention` counts its items that have one, for the bell on the tab (decision 43). A terminal's `here` is the folder its shell is in, for **New terminal in *folder*** in its menu (decision 44). Its `notices` are the status bar's notices, each with its `text` and, for some, `actions`: buttons, each with a `label` and either the `method` it calls, a bound method that takes nothing, such as `ReloadConfig`, or with its `arg`, one string, as **Use this instead** calls `UseSuggestedShortcuts` with the fields to change, or else a `command` that the frontend runs as if Go had sent it as a `bench:command`, as **Keyboard settings** runs `keyboard-settings`. Its `shortcuts` are the fields of the `[shortcuts]` table, in config order, for **Settings ▸ Keyboard**: each with its `field` name, `label`, the `keys` Bench adds to a field of modifiers (such as "the arrow keys"), its `value` in config form and `display` as macOS shows it, its `default`, and, from what the shell found as it registered them, a `problem` if its shortcuts don't work, or may not, and a `suggestion` (with its `suggestionDisplay`) to use instead (decision 45). Its `lastTab` is the tab that `LastTab` goes back to, or -1, and each item's `used` says how lately it was used, for the jump palette's order (decision 49). Its `front` is the window in front before Bench, if it's out of the tabs, as an item ID or a window not in a tab's `f` ID, for **Add front window to this pane** (decision 13). A Web item has its `zoom` in percent, left out at 100; `canGoBack` and `canGoForward`; `camera` and `microphone`, `active` or `muted` while its page uses them; and `finding` while its find bar is open, which Go leaves `layout.FindBarHeight` for at the top of its pane's body. A tab's `camera` and `microphone` count its pages that use them, and `cameraLive` and `microphoneLive` are set while any of them has that one live (decision 50). An item's `icon`, and the `icon` of each window not in a tab, remembered app, launcher, and add-dialog row, is the address of the icon its tile shows in place of its `mono`, if it has one (decision 51). |
| `bench:toast` | A short message, as a string, or, for one with a button, `workspace.Toast`: its `text`, and an `action` with the button's `label`, the bound `method` it calls, and the `arg` it passes, as **Undo** calls `Undo` with its change's number (decision 37), or instead the `item` it passes, as **Pop out** calls `PopOut` with the ID of the item that came into a pane (decision 13). The frontend shows it over the status bar, which docked windows never cover, or, while the status bar is hidden, in the title bar's empty space (decision 31), and keeps it in the notices' **Recent messages** (decision 47). |
| `bench:command` | `workspace.Command`, such as `{"name": "toggle-palette"}` when the jump shortcut is pressed, `{"name": "settings"}` from **Bench ▸ Settings…**, `{"name": "keyboard-settings"}` (Settings, at its **Keyboard** section), `{"name": "edit-tab"}` (the current tab's properties) from **Tab ▸ Tab Properties…**, or `{"name": "focus-terminal", "itemId": "w7"}` to give a terminal the keyboard. `confirm-quit` asks before quitting, and `confirm-close-terminal` asks, in the popped-out terminal `itemId`'s own window, before closing it; each has the `terminals` that are running something, each with its `id`, sub-tab `title`, `tab` (left out for one in a window of its own), and what it's `running` (decision 35); a `confirm-quit` from `QuitAndReopen` has `reopen` set (decision 48). **Tab ▸ Close Tab…** and **Pane ▸ Close Window** send `close-tab` and `close-window`, for the current tab and the focused pane's shown item, which the frontend closes as the tab's menu and the sub-tab's × do, asking first where Go says (decision 36). The app menu's other items that the frontend does send `new-tab` (**Tab ▸ New Tab…**), `rename-tab` (**Tab ▸ Rename…**, which renames the current tab in place), `open-palette` (**Pane ▸ Jump to…**), `add-to-pane` (**Pane ▸ Add to Pane…**, the add dialog for the focused pane), `keyboard-shortcuts` (**Help ▸ Keyboard Shortcuts**, which opens the **Keyboard Shortcuts** sheet, decision 48), `first-run` (**Help ▸ Show Welcome Guide**, which opens the welcome guide even if it was dismissed), `recent-messages` (**Help ▸ Recent Messages**, decision 47), and, with the focused pane's shown item as `itemId`, `find-in-page` and `copy-page-link` (**Pane ▸ Page ▸ Find in Page…** and **Copy Page Link**, which open the page's find bar and put its link on the clipboard, as its menu's entries do, decision 50). For the commands that need Bench's window, such as the palette, Settings, and `confirm-quit`, the shell brings Bench forward first; for `focus-terminal`, the engine already has (decision 15). |
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
`term:data` carry it as they carry the program's own. It's faint, and made
safe with `term.Restorable`, which keeps only text and the sequences that
colour it, move the cursor, and erase, so no query in it reaches a view
that would answer it into the new shell.

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
| `config.toml` | Tabs, layouts, settings, shortcuts, launchers, and rules. Created on first run, with one tab holding the default browser (decision 28); **Settings** and **Tab properties** update it, and so do **Save as launcher** (`SaveLauncher`, decision 41), and the **Don't ask again** of the questions before quitting and before closing a terminal's window (`confirm_quit_busy_terminals`, decision 35), and before closing a tab (`confirm_close`), and **Close tab…**, which deletes the tab as **Delete tab** does (decision 36), and **Undo** and **Reopen closed tab**, which put a deleted tab's entry back (decision 37), and moving a tab, which moves its entry (`MoveTab`, decision 38), and **Move to new tab** and **Also show in new tab**, which add a tab entry (`MoveItemToNewTab`, decision 39), and a shortcut notice's **Use this instead**, which changes `[shortcuts]` (`UseSuggestedShortcuts`, decision 45), and `UseWebApp` (or `UseWebAppWhenReady`) for a web page that it lists (decision 22), and the welcome guide's starting set-up, which adds its tabs (`UseStarterTabs`, decision 48). Bench records a hash of it (`config.Sum`) whenever it reads or writes it, and doesn't save over it once it has changed outside Bench (decision 7). |
| `config.toml.bak` | The previous `config.toml`, kept by every save. **Use backup** swaps it with a `config.toml` that has problems. |
| `config.toml.good` | The last `config.toml` that loaded without problems, or that Bench saved: what Bench runs with while `config.toml` has problems (decision 7). |
| `state.json` | The live arrangement, saved half a second after it changes, and within 3 seconds however often it changes. New titles and addresses, the display Bench is on, and dialog sizes alone are saved within 30 seconds. Its tabs are matched with `config.toml`'s by name at startup (decision 29). Each tab records its config entries' keys (an App entry's app and title, a Web entry's address, a VS Code entry's folder, a terminal's folder and command, and the command it types, if any), so that at startup the entries edited, removed, or added in `config.toml` since can be applied to the arrangement. Its `apps` holds, for each app with items, by lower-case bundle ID, whether the user quit it (`quit`, decision 28) and when Bench last saw it running (`seen`, decision 10), and `ran` when Bench last ran. Its `screen` is the display Bench's window was on, by ID, name, and frame, for Bench to open on next time (decision 30), and its `dialogs` the sizes the user gave dialogs, by kind (`SetDialogSize`, decision 34). An item's `label` is the name the user gave a terminal or web page (`RenameItem`, decision 39). A terminal's `run` is the command Bench types into its shell, and `startAgain` whether it runs it again at the next start (decision 41), and a plain shell's `lastFolder` the folder it was last in, where it starts again; its `folder` stays the config entry's key (decision 44). A Web item's `zoom` is its page's zoom, such as 1.25, left out at actual size; one that isn't among Bench's zooms is taken as the nearest (decision 50). Its `closed` holds the last 5 tabs deleted or closed, latest first, each with its arrangement and items, its `config.toml` entry and rules, where it was among the tabs, and when it went, for **Reopen closed tab** (decision 37); one that couldn't come back is dropped at load, not the whole file. Its `updates` has, while `check_for_updates` is or was on, when Bench last asked GitHub for its latest release (`checked`), the newer version it found and its page (`latest` and `url`), and the version whose notice the user dismissed (`dismissed`, decision 48). Older builds ignore them. |
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
   windows. It needs checking on a Mac.
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
    of Bench's dividers. It puts back any other move or resize of a docked
    window 300 ms later, but at most three times in ten seconds for one
    window: after that, it leaves the window where its app put it until
    the tab is next arranged, so that Bench and the app don't fight.

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
    places in other tabs. **Add front window to this pane** and **Back to
    Bench** dock in the same way, in the focused pane: the View's `front`
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
      ID. At launch the shell asks the engine (`WindowStart`), which finds
      that display by ID, then by name and frame together, since two
      displays of the same model have the same name, then by name, then by
      frame, or else takes the primary one (`findScreen`), and opens the
      window in the middle of its visible area (`mainWindowOptions`, with
      Wails' `WindowXY`).
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
    - **At least 900 × 600, zoomed or not.** Double-clicking the title bar
      zooms the window through Wails' `Maximise`, which drops its minimum
      size until Wails' own `UnMaximise` puts it back; un-zooming with the
      green button, or by resizing, never does. So whenever the window
      resizes, the shell puts the minimum back once the window is no longer
      zoomed (`keepMinimum`).
    - **Filled again after the displays change.** Each `BenchMoved` while
      the displays are settled also records whether Bench's content frame
      fills its screen's visible area, within 2 points, since Wails' web
      view sticks out 1 pt above and to the right (`fills`). Moves while
      they settle don't count, since macOS moves the window then. Once they
      settle, if the window filled its screen before and doesn't fill the
      one it's on now, the engine asks the shell to fill it
      (`Emitter.FillScreen`, `refillLocked`). That moves the window within
      its screen, so it doesn't start another wait. A window the user made
      smaller is left as it is. While the window is hidden or minimised,
      when moves aren't the user's either, filling waits until it's shown,
      since zooming it could bring it back.
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
      is hidden, the focus moves to the one shown. The slim layout button
      opens a menu (`kind: 'layout'`) of the three modes as `menuitemradio`
      entries, with why the layout is in use as its note. **View ▸ Layout**
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
      `flex: 0 1 auto` between `--subtab-min` (96 pt) and
      `clamp(220px, 30cqw, 360px)`, where `cqw` is the pane's width (`.pane`
      is an inline-size container). Both tooltips have the whole name.
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
      strip with the keyboard on another of its tabs or sub-tabs, which the
      web view scrolled into view, is marked too, by a redraw that shows
      what the last one did. A `focusin` listener can't mark it, since
      `setRegion` puts the focus back as it redraws: a strip whose current
      tab has just changed would be marked before it could show it.
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
      `density` gives on the screen the window opens on. The engine keeps
      it as `titleBar`, sent in the View, and `render.js` sets
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
      The jump palette's **Add a window or app to this pane…** and its
      **This window** group (decision 49), which ends with **Window
      options…**, stand in for the missing buttons. The last opens the ⋯
      menu in the palette's place (`openWindowOptions`), without a render
      between, so Bench isn't lowered and raised again; with no ⋯ button
      to hang it from, it opens inside the pane's top right corner.
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
    least 420 by 300 pt (200 pt tall for the palette and the add dialog),
    and in the app, out of the traffic lights' corner, 78 by 52 pt, where
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
    menu item, or failing that **Close** (`closeWindowRank`, in English
    only), never **Close Tab** or **Close All**. Web pages and terminals
    close at once, in the engine, without the page's `beforeunload`. The
    engine's `CloseItem` and `CloseTab` (`internal/workspace/close.go`) mark
    the App items they close (`Item.closing`) and press later:
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
    which tab the × is in; the menu's **Close everywhere** closes it.
    **Tab ▸ Close Tab…** and **Pane ▸ Close Window** have no shortcut, so
    that Command-W stays with the window that has the keyboard (decision
    46).
37. **Undo puts back a snapshot of the arrangement, and closed tabs are kept
    in `state.json`.** Each change that the user might regret, and that a
    toast reports, keeps the arrangement from before it
    (`internal/workspace/undo.go`): **Delete tab**, **Close tab…**,
    **Reset layout**, **Close this pane**, a move of a window or a tab by
    dragging or from a menu, **Also show in**, **Move to new tab** and
    **Also show in new tab** (decision 39), **Remove from this tab**, and
    equal sizes from a divider (`EvenSizes`, decision 40).
    `beginUndoLocked` notes what `state.json` saves of it, the model's
    tabs, in order, with copies of their trees, sizes, and focus, the
    current tab, and what's "Not in a tab", and `keepUndoLocked` keeps it
    once the change is made, with a
    number for the toast's **Undo** (`undoToast`), and the items the change
    forgot (App items without a window, which only remember a place) and
    those it added (the pages and terminals that **Reset layout** opens for
    config entries). The engine keeps the last 10, in memory only.
    `Undo(step)` takes back that change and every later one, latest first,
    and puts back the oldest one's arrangement, reconciled with what has
    happened since: an item that has gone, such as a window that closed, is
    taken out of its pane, which closes up if that empties it, so Undo after
    **Close tab…** brings back the tab and what's still open, never what
    closed, which the toast says; an item that has come since stays where
    it is, in its pane if the tab still has it, or else in the tab's
    focused pane, or else "Not in a tab". The same `Tab` values come back,
    so a deleted tab comes back as itself, with its entry keys. A change
    that deleted a tab also keeps `config.toml`'s tabs and rules
    (`beginDeleteLocked`), and so does one that added a tab for a window
    (`MoveItemToNewTab`), which Undo puts back and saves, keeping the
    settings saved since. A window that comes back to a tab from "Not in a
    tab" docks again (`bringBackLocked`), and one that a closing tab had
    queued to close stays open. A change that moved a tab marks its step
    (`reordered`), and Undo puts `config.toml`'s tabs back in the order of
    the step's tabs, by name (`inTabOrder`), and saves them. Any other
    change to the tabs, such as a tab added in **Tab properties** or
    renamed, a reloaded `config.toml`, **Restore previous arrangement**,
    or **Reopen closed tab**, forgets the history (`forgetUndoLocked`),
    and Undo refuses a step whose tabs don't match
    the config's by name, so it never brings back a tab that `config.toml`
    no longer agrees with. There's no shortcut: Command-Z belongs to the
    pages and terminals, and a change elsewhere would be undone unseen.
    A deleted or closed tab is also kept for **Reopen closed tab**, the
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
    and have **Undo** (decision 37). A sub-tab's menu has **Move left**
    and **Move right** along the strip it's drawn in, which call
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
    order with nothing to update. **Move left** and **Move right** are in
    a tab's menu, the jump palette (for the current tab), and the Tab
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
    `autoTitle`, which the sub-tab's tooltip and **Use automatic title**
    show. `state.json` keeps the label, so it outlasts a restart. An App
    item's title comes from its app, and rules match windows by it, so it
    can't be renamed. A label is one line of up to 200 characters, with
    its runs of spaces collapsed, and the field takes no more.
    **Move to new tab** and **Also show in new tab**, from a window's menu,
    the jump palette, and a sub-tab dropped on the tabs' **+**, are one
    engine call, `MoveItemToNewTab`, rather than `SaveTab` and then
    `MoveItemToTab`, so there is no moment with an empty tab, or a second
    save, and the item never lands in the tab that was current. Under
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
    goes again unless something is added.** **Zoom pane** shows one pane
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
    **Split right with an empty pane** and **Split down with an empty
    pane** make room for another: `SplitPane` puts an empty pane on that
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
    (`@xterm/addon-web-links`), text size, **Clear**, **Select all**, and a
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
    never covers it, and after the tab's name, with why in the tooltip
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
    `Options.Preface`: faint, made safe again in case the file changed,
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
    rather than replaces), or one that `shortcuts.Reserved` lists, in that
    order. For each, it suggests a shortcut that it checked it can register,
    that clashes with nothing it knows of, and that leaves the other fields'
    problems no worse, field by field, so that taking them all at once makes
    no new clash. For VoiceOver, ⌘ takes ⌥'s place
    (`shortcuts.WithoutVoiceOverKeys`). It reports them to the engine
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
    (`Ready`). **Save** writes `[shortcuts]`, and the shell registers them
    again (decision 1). It first refuses a field changed to a shortcut that
    another field makes (`keysClash`), with the field's own line, since Go's
    refusal names whichever of the two comes later in `config.toml`, which
    may be the one the user didn't change. The browser preview runs the same
    code, with a registrar that accepts every shortcut, and `NewDemo`
    reports no system keys. Which side gets a key press that both Bench and
    VoiceOver, or an input source, want is for a Mac to tell, so the notices
    say Bench "may not" see it.
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
    **Reopen Closed Tab** brings back (`Closed`), the tab that **Last Used
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
      calls; it isn't bound), and where the log is, written from `~`. The
      body asks the user to check it. Bench sends nothing itself: the
      user reads the issue, changes it, and submits it, or doesn't.
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
    lists them in the notices menu, under **Recent messages**, each with
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
      don't change it.
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
      window for it, and **Dismiss**. The notice goes once Sparkle is
      done with the version (`UpdateFinished`): the user installed,
      skipped, or put it off. **Check for Updates…** has Sparkle look at
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
    them add 3, so "move tab" finds **Move tab left** before **Move to tab ▸
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
      `previous_tab` in the table and Settings, but last in `Bindings`, so
      that a config written before it existed, whose other fields may hold
      ⌃⌥&#96; (which `Suggest` offers for `next_window`), keeps their
      shortcuts. `Parse` turns `last_tab` off in a file that leaves it out
      when its default clashes so (`offClashingLastTab`): otherwise a notice
      would report a clash the user didn't make, and the next Settings save
      would write it.
    - **Order.** With nothing typed, the Tabs group starts with the last
      used tab, marked **last used** and showing the `last_tab` shortcut,
      so that ⌃⌥Space then Return goes back as ⌃⌥&#96; does; the others keep
      the tab strip's order, which their shortcuts number. The Windows
      group is ordered by the items' `used` (`Item.Used`, from
      `Model.Use`), the latest first, which showing an item, focusing its
      pane, and switching to its tab update. It isn't saved either.
    - **This window.** The group is what the ⋯ menu has for the focused
      pane's shown item, built by the same `windowMenuEntries`, so that
      the palette can't offer less or other than the menu. Entries under a
      heading (`under`) are named with it, as "Move to pane ▸ With Slack".
      Disabled entries are left out, and so is Zoom pane (`pane`), which
      acts on the pane and is in Actions, where an empty pane has it too.
      The group ends with **Window options…**, which opens the menu itself
      (decision 33), as the web app entries join it only once Go has said
      whether the browser can make one. Entries that the menu gains, such
      as a page's controls, join the group the same way. Actions keep what
      isn't the window's: the pane's add dialog (**Add a window or app to
      this pane…**), **Close this pane** for an empty pane, as **Pane ▸
      Close Pane** has it (`panesCanClose`), **Close tab…**, which asks
      first as the tab's menu does, **Delete tab (keep windows)**, **Tab
      properties of** each tab, and the rest.

50. **A web page's controls are in its menus, never on ⌘ keys, and Go
    follows what the page does with the camera and microphone.** A Web
    item has no toolbar, and pages such as Slack's replace WebKit's own
    right-click menu, so the only ways to reload a stuck page or go back
    were a swipe, or **Open in browser**. Its window menu now has a **Page**
    group (`pageEntries` in `pages.js`), which the jump palette's **This
    window** lists as **Page ▸** *entry* and the app menu as **Pane ▸
    Page**, with no key equivalents: ⌘R, ⌘F, ⌘L, ⌘+, and ⌘− stay the
    page's, as decision 46 keeps the app menu off them.
    - **One row.** Back, Forward, Reload, Zoom out, Actual size, which
      shows the zoom, and Zoom in are buttons in one row, labelled Page,
      as in a browser's menu, and Find in page…, Copy page link, the
      camera and microphone entries, and Stop sound and video are entries
      after it. A web page's menu already had about 20 entries, and as one
      entry each the group took a third of a 900-point window, pushing the
      web app entry (decision 22) out of sight. For the same reason, menus
      may now take the window's height less 16 points before they scroll,
      not 80% of it, and `fitMenu` leaves room for a web app entry of two
      lines, as its names often wrap. The row's buttons are menu items like
      the others: the up and down arrows go through them, and left and
      right move along the row.
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
    - **Copy page link.** `PageLink` gives the address that **Open in
      browser** would open (decision 23), and the frontend puts it on the
      clipboard through the Wails runtime. VS Code for the Web's address
      holds the server's connection token, so it has none.
    - **Camera and microphone.** `cameraCaptureState` and
      `microphoneCaptureState` (macOS 12) say whether a page uses them,
      live or muted. The sub-tab shows a mark of each before its title,
      where the sub-tab's × never covers it and apart from the marker of
      the item's status on its monogram, green while live and grey and
      struck through while muted, and the tab one mark after its name, with
      counts and each device's liveness in `TabView`, so that the mark and
      its words name the device that's live. A call can run in a popup,
      as a meeting opened with `window.open` does, so the item reports its
      own web view's state and its popups' together: live if any is, else
      muted if any is. **Mute microphone** and **Turn off camera** set the
      state to muted (`setMicrophoneCaptureState:` and
      `setCameraCaptureState:`) on each of those web views that uses the
      device, which the page can't undo, and **Unmute microphone** and
      **Turn on camera** set it back; Go refuses them for a device the
      page isn't using, and the native side checks again. A web view's
      state resets as it closes.
    - **Sound.** WebKit has no public property to observe that a page
      plays sound, and a script on media elements would miss Web Audio and
      pages that build their own players, so there's no speaker mark, and
      **Stop sound and video**, which pauses the page's and its popups'
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

52. **Bench is dark unless the user chooses light, or to match macOS, and
    web pages always match macOS.** Bench's windows were dark only: Wails
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
      dark by default, so that Bench looks as it did until the user
      chooses. The engine tells the shell with the first View and when it
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
