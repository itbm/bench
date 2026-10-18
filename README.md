# Bench

Bench is a macOS app, written in Go, that gathers any set of apps into one
window with tabs. Each tab holds a layout of panes, and each pane holds
windows as sub-tabs. You can move any window to another pane, beside another
window, or to another tab. Layouts adapt to the screen: wide on a monitor,
compact on a laptop.

Bench is a working name. To see how it looks and behaves, open
[the clickable prototype](docs/prototype.html) in a browser, or run the
[browser preview](#try-the-browser-preview) of the real app.

## Contents

- [Status](#status)
- [Requirements](#requirements)
- [Build and run the app](#build-and-run-the-app)
- [Try the browser preview](#try-the-browser-preview)
- [Permissions](#permissions)
- [Files](#files)
- [Configuration](#configuration)
- [Command line](#command-line)
- [Documentation](#documentation)

## Status

Bench has a working, tested core, and its macOS layer runs on a real Mac in
a recorded demo, but much of that layer hasn't been checked by hand.

- **Tested on any OS.** The config and layout maths, the workspace engine
  (tabs, panes, moving windows, rules, switching, saving the arrangement),
  window docking and parking, terminals, and VS Code for the Web are tested
  with Go unit tests against a fake platform. The UI is tested in headless
  Chromium against a mock of Go, and the browser preview runs the real app
  with a demo platform.
- **Run on a real Mac.** The
  [macOS demo](docs/development.md#the-macos-demo) runs Bench on a GitHub
  macOS runner with real apps (iTerm2, VS Code, Zed, TextEdit, and Safari),
  and records it. It docks windows, switches tabs with the global
  shortcuts, drags sub-tabs with the real mouse, splits panes, uses the jump
  palette and Compact, moves and resizes Bench's window, and quits. Its
  first runs found bugs that the tests couldn't, which are now fixed: a
  crash when a global shortcut brought Bench forward, clicks lost while
  Bench wasn't the active app, VS Code undocked when it moved its own
  window, and hiding apps that reported failure although it worked. The
  macOS CI job compiles `internal/mac` and the app, and assembles and signs
  `Bench.app` ad hoc.
- **Not yet checked on a Mac.** `Bench.app` itself (the demo runs the
  binary on its own, with permissions granted in advance), the first-run
  guide and asking for permission, signing with a certificate, Developer
  ID signing and notarisation (which the Makefile doesn't do yet), Open at
  login, sign-ins and notifications in Web items, VS Code for the Web, live
  previews, `bench arrange`, crash recovery, multiple displays, desktops
  (Spaces), macOS 14, Intel Macs (the runner has macOS 15 on Apple
  silicon), and any native code that the demo's tour doesn't reach. Expect
  bugs there until someone works through the
  [manual checks](docs/development.md#manual-checks-on-a-mac).

| Milestone | What exists | Still to do |
| --- | --- | --- |
| 1. Skeleton and config | `internal/config`, `bench check-config`, and CI on Linux and macOS. | Nothing. |
| 2. Layout trees | `internal/layout`, with the spec's layout cases as table tests, and `bench layout`. | Nothing. |
| 3. Windows without a Bench window | `internal/windows` (tested against the fake platform), the `internal/mac` bridge, `bench arrange`, `bench restore`, and `bench watchdog`. | Run `bench arrange Code` on a monitor and a laptop. |
| 4. Shell window and layering | The Wails window with the prototype's UI, panes at Go's rectangles, and approach A plus raised mode (see [the window model](docs/architecture.md#the-window-model)). The demo shows docked windows following Bench's window. | Check by hand that clicking Bench never hides a docked window, and that menus and dialogs appear above docked windows. |
| 5. Sub-tabs and moving windows | Dragging, the split button, the **⋯** menu, keyboard moves, pop out, new-window rules, focus following, and `state.json`, in the engine and the UI. The demo drags real windows' sub-tabs to another tab and to a pane's edge. | Check with several Brave windows, and with links that bring another tab's window forward. |
| 6. Web items | Web views with a data store per profile, link handling, the notification bridge, and VS Code for the Web (`internal/vscode`). | Check sign-ins persist and notifications appear. |
| 7. Terminal items | Pseudo-terminal sessions (`internal/term`) drawn with xterm.js. | Check `vim`, `htop`, and `git log` on a Mac. |
| 8. Switching, shortcuts, and screens | The launch-and-wait loop, hiding and parking, Auto, Wide, and Compact, screen changes, and global shortcuts through Wails. The demo presses the global shortcuts. | Check unplugging a monitor, and shortcuts that clash with macOS or other apps. |
| 9. Palette, editor, add dialog, and previews | The jump palette, layout editor, add dialog, and draggable dividers, tested in headless Chromium; live previews with ScreenCaptureKit. | Check live previews on a Mac. |
| 10. Packaging | `make app` (a universal, signed `Bench.app`), the icon, the menu bar item, Open at login, and the first-run guide. CI assembles `Bench.app` and signs it ad hoc. | Sign with a certificate and open `Bench.app`; check the first-run guide and Open at login. |

## Requirements

- macOS 14 (Sonoma) or later, on Apple silicon or Intel.
- Go 1.26.
- The Xcode command-line tools, for cgo, `codesign`, `sips`, and
  `iconutil`. To install them, run `xcode-select --install`.
- An Apple Development certificate, to sign development builds. Xcode
  creates one for your Apple ID in **Settings ▸ Accounts**. See
  [Signing and the Accessibility permission](docs/development.md#signing-and-the-accessibility-permission)
  for why it matters.

The browser preview and the Go tests run on any OS with Go 1.26. The browser
tests also need Node 22.

## Build and run the app

On a Mac, from the repository root:

```sh
make run
```

`make run` builds a universal binary, assembles and signs `bin/Bench.app`,
and opens it. Quit Bench first: if it's running, `make run` stops, because
`open` would bring the running Bench forward instead of the new build. It
also stops while `bench arrange` or the browser preview (`make preview`)
runs, since they're called `bench` too. To build without opening, run
`make app`.

If you don't have an Apple Development certificate, sign ad hoc instead:

```sh
make run SIGN_IDENTITY=-
```

With ad hoc signing, macOS forgets Bench's Accessibility permission each time
you rebuild, and asks again.

Run `make help` to list the targets.

## Try the browser preview

The browser preview is the real app in Wails' server mode: the same UI and
workspace engine, with no native window, backed by a demo of the prototype's
apps. It runs on any OS:

```sh
make preview
```

Then open <http://localhost:8080>. To use another port, run
`make preview PORT=9000`.

In the preview, window contents are placeholders, as in the prototype, and
terminals only echo what you type. The bar above the window fits the window
to your browser, or simulates a monitor (2,560 points) or a laptop (1,512
points). Bench's shortcuts work while the page has focus, and Control+K also
opens the jump palette. The preview keeps its own files in `$BENCH_HOME`, or
a `Preview` folder in Bench's support folder, so it never touches the app's
`config.toml`.

`make preview` runs `CGO_ENABLED=0 go run -tags server ./cmd/bench --port 8080`.
cgo is off because, on Linux, Wails would otherwise need GTK 4.

## Permissions

| Permission | When Bench asks | Why |
| --- | --- | --- |
| Accessibility | On first run. The first-run guide explains it before macOS asks. | Required. Bench moves and sizes other apps' windows into its panes, and hides the ones that belong to other tabs. Its global shortcuts work without it. |
| Screen Recording | When you turn on live previews in the layout editor. | Optional. Only for live previews of windows when you hover a sub-tab. |
| Notifications | When a Web item first shows a notification. | Optional. Passes notifications from web pages, such as Slack, to Notification Centre. |

You can change these in **System Settings ▸ Privacy & Security**. Bench
isn't sandboxed, because the Accessibility API doesn't work in the App
Sandbox.

## Files

Bench keeps its files in `~/Library/Application Support/Bench/`, or in the
folder that `$BENCH_HOME` names:

| File | What it is |
| --- | --- |
| `config.toml` | Tabs, layouts, settings, shortcuts, and rules. Bench creates it on first run, and the layout editor updates it. |
| `config.toml.bak` | The previous `config.toml`, kept by every save. |
| `state.json` | The live arrangement after dragging and splitting. Bench sets it aside as `state.json.bad` if it doesn't parse or match the config. |
| `parked.json` | The windows that Bench has moved off screen, and the apps it has hidden, right now, so that they can be put back after a crash. |
| `vscode.json` | The VS Code for the Web server that Bench is running, so that it can be stopped after a crash. |
| `first-run-done` | Present once you've closed the permissions guide. |
| `view.json` | Only with `BENCH_DEBUG` set: what Bench's window shows, for debugging. See [Read the log](docs/development.md#read-the-log). |

## Configuration

You don't need to edit the config file: the layout editor covers tabs,
layouts, and the main settings. For shortcuts, rules, and items such as web
pages with their own profiles, edit `config.toml`. The
[user guide](docs/user-guide.md#config-reference) describes every setting,
with the default file.

To check your changes, run `bench check-config`. It reports each problem
with its tab and field. If `config.toml` has problems when Bench starts,
Bench lists them in the status bar and runs with the default config, without
overwriting your file. Until you fix it, Bench doesn't save the layout
editor's changes or the arrangement.

## Command line

With no command, `bench` runs the app. Its commands are for development and
recovery:

| Command | What it does |
| --- | --- |
| `bench check-config [--config path]` | Checks `config.toml` and reports each problem with its tab and field. |
| `bench layout --width w --height h [--y points] [--compact] [--config path]` | Prints every tab's pane frames for a screen size, as JSON. |
| `bench arrange <tab> [--config path]` | Arranges one tab across the screen without Bench's window, until you press Control-C. It refuses while Bench is running (macOS). |
| `bench restore [--dir path]` | Puts back windows that a crashed Bench left off screen (macOS). |
| `bench watchdog --dir path` | Waits for Bench to quit, then puts back what it left off screen. Bench starts it itself (macOS). |
| `bench version` | Prints the version. |
| `bench help [command]` | Explains a command and its flags. |

To run them, use the binary inside the app, such as
`bin/Bench.app/Contents/MacOS/bench check-config`, or run
`go run ./cmd/bench check-config` from the repository. `check-config`,
`layout`, `version`, and `help` work on any OS.

## Documentation

- [User guide](docs/user-guide.md): using Bench, keyboard shortcuts, the
  config reference, and troubleshooting.
- [Development](docs/development.md): setting up, running the tests,
  CI and [the recorded macOS demo](docs/development.md#the-macos-demo),
  signing, manual checks on a Mac, and debugging, including `BENCH_DEBUG`.
- [Architecture](docs/architecture.md): packages, builds, threads,
  coordinates, the window model, the Go-to-UI protocol, and decisions.
- [Build spec](docs/spec.md): the brief Bench is built from.
- [UI prototype](docs/prototype.html): the reference for behaviour and
  wording. Open it in a browser.
