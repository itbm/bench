# Developing Bench

This guide is for people working on Bench's code. For using Bench, see the
[user guide](user-guide.md). For how Bench is built and why, see the
[architecture note](architecture.md).

## Contents

- [Set up](#set-up)
- [Packages and builds](#packages-and-builds)
- [Run the tests](#run-the-tests)
- [Continuous integration](#continuous-integration)
- [The browser preview](#the-browser-preview)
- [Build the app](#build-the-app)
- [Signing and the Accessibility permission](#signing-and-the-accessibility-permission)
- [Manual checks on a Mac](#manual-checks-on-a-mac)
- [Debug](#debug)

## Set up

You need:

| Tool | Version | For |
| --- | --- | --- |
| Go | 1.26, as `go.mod` names | Everything. |
| Xcode command-line tools | Any recent | The app on macOS: cgo, `codesign`, `sips`, `iconutil`, and `lipo`. Run `xcode-select --install`. |
| Node | 22 | The browser tests. |
| An Apple Development certificate | — | Signing development builds. See [Signing and the Accessibility permission](#signing-and-the-accessibility-permission). |

On Linux, you can work on everything except `internal/mac` and the native
side of `internal/ui`. You don't need GTK: the browser preview builds with
`CGO_ENABLED=0`. The race detector needs cgo and a C compiler.

Wails v3 is pinned to v3.0.0-beta.27 in `go.mod`. You don't need the
`wails3` tool: the Makefile builds and packages Bench with `go` and the
macOS tools.

Some packages only build with the `server` tag on Linux, such as
`internal/ui`. To have your editor's Go tools see them, set the tag in
gopls, for example with `"buildFlags": ["-tags=server"]`.

Run `make help` to list the Makefile's targets.

## Packages and builds

[Packages](architecture.md#packages) in the architecture note lists what
each package does, and [Builds](architecture.md#builds) explains the three
builds and their build constraints. In short:

- Only `internal/mac` uses cgo. Everything else builds and tests on any OS.
- `go build ./cmd/bench` builds the app on macOS. On other systems it builds
  the commands, and running the app explains how to start the preview.
- The `server` tag builds the browser preview: Wails' server mode, with the
  demo platform in place of `internal/mac`.

## Run the tests

### Go tests

```sh
make test
```

`make test` runs both of these:

| Command | What it tests |
| --- | --- |
| `go test ./...` | Every package. On Linux, that leaves out `internal/ui` and the native code in `internal/mac`; on macOS, it includes them. Add the race detector with `make test RACE=-race`. |
| `CGO_ENABLED=0 go test -tags server ./...` | The same, plus `internal/ui` in server mode, as the browser preview builds it. |

CI also runs the first with the race detector: `go test -race ./...`.

Before you push, run `make vet`. It fails if `gofmt -l .` lists any file,
then runs `go vet` on both builds.

The unit tests drive the workspace engine and the windows manager with
`platform.Fake` and `clock.Fake`, so they need no Mac and no waiting. To add
a case, see the existing tests in `internal/workspace` and
`internal/windows`.

### Browser tests

The browser tests drive the UI in headless Chromium with Playwright and
Node's test runner:

```sh
make e2e
```

`make e2e` installs Playwright and Chromium from `e2e/package-lock.json` the
first time, builds the browser preview as `bin/bench-preview`, then runs
`scripts/e2e.sh`. The script runs `node --test` on each test folder that
exists:

| Folder | What it tests |
| --- | --- |
| `e2e/frontend/` | The UI against `FakeGo`, a mock of Go's bound methods and events. It needs no Go build. |
| `e2e/app/` | The browser preview, end to end. Each test starts its own preview, with a new support folder (`BENCH_HOME`) on a free port, and stops it afterwards. |

The `e2e/app/` tests run the preview binary that `BENCH_PREVIEW` names,
which `make e2e` and CI set to `bin/bench-preview`. Without it, they build
the preview once per run with `go build -tags server` (without cgo), so they
need Go as well as Node.

To set up and run them by hand:

```sh
cd e2e
npm ci
npx playwright install chromium
npm test
```

On a new Linux machine, add `--with-deps` to the `playwright install`
command to install Chromium's system libraries.

To run both folders without the script, run
`node --test e2e/frontend/ e2e/app/` from the repository's root. To run one
file, run `node --test e2e/frontend/drag.test.js`. To run tests
by name, pass a pattern: `sh scripts/e2e.sh --test-name-pattern="palette"`.

The screenshot tests write PNG files to `e2e/frontend/screenshots/`, which
git ignores, so that you can compare the UI with the prototype.

If `e2e/node_modules` is missing, the tests look for a global Playwright
install instead.

## Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request:

| Job | Runner | Steps |
| --- | --- | --- |
| Linux | `ubuntu-latest` | `gofmt` check, `go vet ./...`, `go test -race ./...`, `go vet` and `go test` with `-tags server`, building the browser preview, and the browser tests. |
| macOS | `macos-15` (Apple silicon) | `go vet ./...`, `go test ./...`, `go build ./cmd/bench` (which compiles `internal/mac` and the Wails app), and `make app SIGN_IDENTITY=-`, to check that `Bench.app` assembles and signs. Then it runs `bench version` and `bench layout` from the bundle, to check that the signed binary starts under the hardened runtime. |

The repository is private and macOS minutes cost more, so the macOS job
waits for the Linux job and doesn't repeat its checks. A push to a pull
request's branch starts a run for the push and another for the pull request;
only the push run has a macOS job, unless the pull request comes from a fork.
If a step fails, the Linux job keeps the browser tests' screenshots as an
artifact for a week.

To run the Linux job's checks locally, run `make check`. Like CI, it runs the
Go tests with the race detector.

## The browser preview

```sh
make preview
```

This runs `CGO_ENABLED=0 go run -tags server ./cmd/bench --port 8080` and
serves the UI at <http://localhost:8080>. The preview is the real app,
with the demo platform (`platform.NewDemo`) in place of macOS and pretend
terminals (`term.Echo`) in place of shells.

| Option | Effect |
| --- | --- |
| `--port` | The port. `make preview PORT=9000` sets it. |
| `--dir` | The folder for the preview's `config.toml` and `state.json`. Without it, the preview uses `$BENCH_HOME`, or a `Preview` folder in Bench's support folder (`~/.config/Bench/Preview` on Linux). |

The preview serves on `localhost` and has no authentication, which is why
its terminals never run real shells. Wails' `WAILS_SERVER_HOST` and
`WAILS_SERVER_PORT` environment variables override the address and port, so
leave them unset.

In the preview, Bench's global shortcuts work while the page has focus, and
Control+K also opens the jump palette. The bar above the window fits the
window to the browser or simulates a monitor or a laptop.

## Build the app

On a Mac:

| Command | What you get |
| --- | --- |
| `make build` | `bin/bench`, for your Mac's architecture, not signed with your certificate. Use it for the commands. |
| `make app` | `bin/Bench.app`: a universal binary (arm64 and amd64), `Info.plist`, and `Bench.icns`, signed with the hardened runtime. |
| `make run` | `make app`, then opens it. |

`make app` takes these variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `SIGN_IDENTITY` | `Apple Development` | The certificate to sign with. `-` signs ad hoc; empty skips signing. |
| `ARCHS` | `arm64 amd64` | The architectures to build. Name one, such as `ARCHS=arm64`, for a faster build. |
| `APP_TAGS` | `production` | Build tags. The `production` tag turns off Wails' web inspector; set `APP_TAGS=` to keep it. |
| `VERSION` | From `Info.plist` | The version that `bench version` prints. |

`make app` builds with a macOS 14 deployment target, to match
`LSMinimumSystemVersion`. The files it packages are in `build/darwin/`; see
[its README](../build/darwin/README.md).

If a `bench` process is running (Bench, or a command such as
`bench arrange`), `make run` stops before building: `open` would bring the
running Bench forward rather than start the new build. Quit it, then run
`make run` again.

To change the icon, edit `build/darwin/icon/main.go`, run `make icon`, and
commit the new `build/darwin/icon.png`.

## Signing and the Accessibility permission

macOS ties the Accessibility permission to the app's code signature. An ad
hoc signature changes with every build, so macOS treats each build as a new
app: the permission resets, or the switch in System Settings stays on but no
longer applies. When you sign every build with the same certificate, macOS
recognises the app by its bundle ID and certificate, and the permission
survives rebuilds.

That's why `SIGN_IDENTITY` defaults to `Apple Development`. To get a
certificate:

1. In Xcode, choose **Settings ▸ Accounts** and add your Apple ID. A free
   account works.
2. Select your team, click **Manage Certificates**, click **+**, and choose
   **Apple Development**.
3. Check that it's there:

   ```sh
   security find-identity -v -p codesigning
   ```

If you have more than one Apple Development certificate, `codesign` can't
choose. Name one in full, or by its SHA-1 hash from that list:

```sh
make app SIGN_IDENTITY="Apple Development: Your Name (TEAMID1234)"
```

`make app` signs with:

```sh
codesign --force --options runtime --timestamp=none \
  --entitlements build/darwin/entitlements.plist \
  --sign "$SIGN_IDENTITY" bin/Bench.app
```

`--options runtime` turns on the hardened runtime, and `--timestamp=none`
skips Apple's timestamp server, which development builds don't need.
`make app` then checks the signature with `codesign --verify --strict`.

Things to know:

- **If the permission seems stuck**, open **System Settings ▸ Privacy &
  Security ▸ Accessibility**, select Bench, remove it with the minus button,
  and open Bench again. Or reset it from the command line:
  `tccutil reset Accessibility io.github.itbm.bench`.
- **Running `bench` from a terminal** (`go run ./cmd/bench`, `bin/bench`, or
  the commands) uses the terminal app's permissions, not Bench.app's. To use
  `bench arrange` or `bench restore`, give your terminal app Accessibility
  permission. An unbundled binary also has no bundle ID, so it posts no
  notifications, and Open at login would start that binary (Wails falls
  back to a launch agent) rather than Bench.app. Use `make run` to try the
  real app.
- **The watchdog** (`bench watchdog`) is a child process of Bench.app, so it
  shares Bench's permission.
- **Releases** need a Developer ID certificate and notarisation, which the
  Makefile doesn't do yet.

## Manual checks on a Mac

The macOS layer can only be tested by hand. Work through these before each
release, and after changes to `internal/mac`, `internal/windows`, or
`internal/ui`. They come from the spec's
[Testing](spec.md#testing) and [Milestones](spec.md#milestones) sections.

### Before each release

- [ ] Laptop alone.
- [ ] Laptop with an external monitor.
- [ ] Unplugging and plugging in the monitor while Bench is running: the
      current tab moves to its compact or wide layout within 2 seconds,
      without changing focus.
- [ ] A tab whose apps aren't running.
- [ ] An app that's running with no windows.
- [ ] A minimised window.
- [ ] A window in native full screen: its item shows "In full screen".
- [ ] Three Brave windows in one pane.
- [ ] One of them moved beside another, by dragging its sub-tab to the
      pane's right edge.
- [ ] One of them moved to another tab, by dropping its sub-tab on the tab.
- [ ] Brave quit and reopened: its windows come back to their panes.
- [ ] A link clicked in Slack that opens in a Brave window in another tab:
      Bench switches tabs (with `follow_focus = "switch-tab"`).
- [ ] Dialogs, settings windows, and picture-in-picture stay floating.
- [ ] A docked window dragged out by the user: its pane shows **Dock again**.
- [ ] Bench on a second desktop (Space): items whose windows are on another
      desktop show "On another desktop".

### Milestone checks

- [ ] **Arranging without Bench's window.** With the test config below,
      `bench arrange Code` docks Zed, VS Code, and Terminal correctly on a
      monitor and on a laptop, keeps VS Code stacked behind Zed in the same
      frame, parks a Brave window that belongs to Research, and puts
      everything back when you press Control-C.
- [ ] **Layering.** Clicking Bench's tabs, sub-tabs, and pane headers never
      hides a docked window. Moving or resizing Bench re-docks within one
      frame of releasing the mouse. Menus, dialogs, and drags appear above
      docked windows (raised mode), and the docked windows come back when
      they close.
- [ ] **Persistence.** Quit and reopen Bench: the arrangement is intact.
- [ ] **Web items.** Slack and Outlook stay signed in after quitting and
      reopening Bench, and a Slack message notification appears in
      Notification Centre.
- [ ] **VS Code for the Web.** A `vscode` item opens the folder, and the
      `code serve-web` process stops when Bench quits.
- [ ] **Terminals.** `vim`, `htop`, and `git log` behave as they do in
      Terminal, and sessions survive tab switches.
- [ ] **Shortcuts.** Every shortcut in the
      [user guide](user-guide.md#keyboard-shortcuts) works from another app.
      A shortcut that another app has already registered shows in the status
      bar, with another to try. Also set up two input sources, so that macOS
      uses Control+Option+Space: macOS may let Bench register it but keep the
      keys for itself, so note whether the status bar reports the clash.
- [ ] **Palette and editor.** Every palette action works by keyboard alone.
      Saving the layout editor updates `config.toml` (keeping
      `config.toml.bak`) and the window.
- [ ] **Live previews.** With previews on and Screen Recording allowed,
      hovering an inactive sub-tab shows a preview within half a second.
- [ ] **Packaging.** `make app` builds a signed `Bench.app` that opens, with
      a Dock icon and a menu bar item showing the current tab. The first-run
      guide appears on first run, and **Open Bench at login** works.
- [ ] **Crash safety.** With windows parked (switch away from a tab), end
      Bench with `kill -9`: the watchdog puts the parked windows back. If it
      doesn't, `bench restore` does.

Test config for the arranging check, with Terminal as an App item in place
of the built-in terminal:

```toml
[[tab]]
name = "Code"
layout = "main-side"

  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }, { app = "com.microsoft.VSCode" }]

  [[tab.pane]]
  items = [{ app = "com.apple.Terminal" }]

[[tab]]
name = "Research"
layout = "side"

  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]

  [[tab.pane]]
  items = [{ app = "md.obsidian" }]
```

Save it as `/tmp/bench-arrange/config.toml`, then run:

```sh
bin/bench arrange Code --config /tmp/bench-arrange/config.toml
```

## Debug

### Check the config

```sh
bin/bench check-config
bin/bench check-config --config path/to/config.toml
```

It prints each problem with its tab and field, such as
`tab "Code": layout: "sideways" isn't a preset`, and exits with status 1 if
there are any. It checks the `[shortcuts]` table too.

### See the layout maths

```sh
bin/bench layout --width 2560 --height 1415
bin/bench layout --width 1512 --height 949 --y 33
```

It prints every tab's size class, its tree after the small-screen setting,
and each pane's frame, as JSON. The pane area starts at x 0 and y `--y`
(25 by default). Add `--compact` to force the compact size class. The
frames should match the spec's [layout maths](spec.md#layout-maths).

### Arrange a tab without Bench's window

```sh
bin/bench arrange Code
```

It opens the tab's apps if they aren't running, docks each pane's first
window across the screen's visible area, and parks the windows of other
tabs. Press Control-C to put everything back. It skips Web and terminal
items. Your terminal app needs Accessibility permission.

### Put windows back after a crash

```sh
bin/bench restore
```

It puts back the windows listed in `parked.json` and shows the apps that
Bench hid. Bench does this itself when it next starts, and its watchdog
does it when Bench crashes, so you only need this if both failed. Use
`--dir` for a support folder other than the default.

### Read the log

Bench writes Wails' warnings and errors to standard error. To keep them when
you open the app, send standard error to a file:

```sh
open --stderr /tmp/bench.log bin/Bench.app
tail -f /tmp/bench.log
```

### Inspect the UI

Build with Wails' web inspector, then inspect Bench's UI from Safari's
**Develop** menu (turn it on in Safari's **Advanced** settings):

```sh
make run APP_TAGS=
```

The browser preview is often quicker: open it in any browser and use its
developer tools.

### Start again

To start from your config rather than the saved arrangement, quit Bench and
delete `state.json` from `~/Library/Application Support/Bench/`. To start
from the default config, move `config.toml` aside too; Bench writes a new
one on its next start.

To try changes without touching your own files, point the commands at
another folder with `BENCH_HOME`, for example
`BENCH_HOME=/tmp/bench bin/bench check-config`.
