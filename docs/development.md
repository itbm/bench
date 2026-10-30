# Developing Bench

This guide is for people working on Bench's code. For using Bench, see the
[user guide](user-guide.md). For how Bench is built and why, see the
[architecture note](architecture.md).

## Contents

- [Project status](#project-status)
- [Set up](#set-up)
- [Packages and builds](#packages-and-builds)
- [Run the tests](#run-the-tests)
- [Continuous integration](#continuous-integration)
- [The browser preview](#the-browser-preview)
- [Build the app](#build-the-app)
- [Signing and the Accessibility permission](#signing-and-the-accessibility-permission)
- [Add or update a dependency](#add-or-update-a-dependency)
- [Release a new version](#release-a-new-version)
- [Manual checks on a Mac](#manual-checks-on-a-mac)
- [Debug](#debug)

## Project status

Bench has a working, tested core, and its macOS layer runs on a real Mac in
a recorded demo, but much of that layer hasn't been checked by hand.

- **Tested on any OS.** The config and layout maths, the workspace engine
  (tabs, panes, moving windows, rules, switching, saving the arrangement),
  window docking and parking, terminals, and VS Code for the Web are tested
  with Go unit tests against a fake platform. The UI is tested in headless
  Chromium against a mock of Go, and the browser preview runs the real app
  with a demo platform.
- **Run on a real Mac.** The [macOS demo](#the-macos-demo) runs Bench on a
  GitHub macOS runner with real apps (iTerm2, VS Code, Zed, TextEdit, and
  Safari), and records it. It docks windows, switches tabs with the global
  shortcuts, drags sub-tabs with the real mouse, splits panes, uses the jump
  palette and Compact, moves and resizes Bench's window, and quits. Its
  first runs found bugs that the tests couldn't, which are now fixed: a
  crash when a global shortcut brought Bench forward, clicks lost while
  Bench wasn't the active app, VS Code undocked when it moved its own
  window, and hiding apps that reported failure although it worked. The
  macOS CI job compiles `internal/mac` and the app, and assembles and signs
  `Bench.app` ad hoc. The [macOS web items check](#the-macos-web-items-check)
  checks there that a Web item's pages can't put another site in place of
  the item's page, or keep Bench behind their alerts.
- **Not yet checked on a Mac.** `Bench.app` itself (the demo runs the
  binary on its own, with permissions granted in advance), the welcome guide
  and asking for permission, with System Settings' deep links, **Quit and
  Reopen Bench**, and the login item's approval, signing with a certificate,
  Developer ID signing and notarisation (which the
  [release workflow](#release-a-new-version) can do, but hasn't yet), Open
  at login, the check for a new version against GitHub itself, the About
  panel's credits, the Acknowledgements window, sign-ins and notifications
  in Web items, VS Code for the Web, live previews, `bench arrange`, crash
  recovery, multiple displays, desktops (Spaces), macOS 14, Intel Macs (the
  runner has macOS 15 on Apple silicon), and any native code that the demo's
  tour doesn't reach. Expect bugs there until someone works through the
  [manual checks](#manual-checks-on-a-mac).

| Milestone | What exists | Still to do |
| --- | --- | --- |
| 1. Skeleton and config | `internal/config`, `bench check-config`, and CI on Linux and macOS. | Nothing. |
| 2. Layout trees | `internal/layout`, with the spec's layout cases as table tests, and `bench layout`. | Nothing. |
| 3. Windows without a Bench window | `internal/windows` (tested against the fake platform), the `internal/mac` bridge, `bench arrange`, `bench restore`, and `bench watchdog`. | Run `bench arrange Code` on a monitor and a laptop. |
| 4. Shell window and layering | The Wails window with the prototype's UI, panes at Go's rectangles, and approach A plus raised mode (see [the window model](architecture.md#the-window-model)). The demo shows docked windows following Bench's window. | Check by hand that clicking Bench never hides a docked window, and that menus and dialogs appear above docked windows. |
| 5. Sub-tabs and moving windows | Dragging, the split button, the **⋯** menu, keyboard moves, pop out, new-window rules, focus following, and `state.json`, in the engine and the UI. The demo drags real windows' sub-tabs to another tab and to a pane's edge. | Check with several Brave windows, and with links that bring another tab's window forward. |
| 6. Web items | Web views with a data store per profile, link handling, the notification bridge, and VS Code for the Web (`internal/vscode`). | Check sign-ins persist and notifications appear. |
| 7. Terminal items | Pseudo-terminal sessions (`internal/term`) drawn with xterm.js. | Check `vim`, `htop`, and `git log` on a Mac. |
| 8. Switching, shortcuts, and screens | The launch-and-wait loop, hiding and parking, Auto, Wide, and Compact, screen changes, and global shortcuts through Wails. The demo presses the global shortcuts. | Check unplugging a monitor, and shortcuts that clash with macOS or other apps. |
| 9. Palette, Tab properties, Settings, add dialog, and previews | The jump palette, a tab's menu, Tab properties, Settings, the add dialog, and draggable dividers, tested in headless Chromium; live previews with ScreenCaptureKit. | Check live previews on a Mac. |
| 10. Packaging | `make app` (a universal, signed `Bench.app`), `make dist` (`Bench.dmg` and `Bench.zip`), the release workflow, the icon, the About panel's credits, the Acknowledgements window and its page (`make acknowledgements`), the menu bar item, Open at login, and the first-run guide. CI assembles `Bench.app` and signs it ad hoc. | Sign with a certificate and open `Bench.app`; check the first-run guide, Open at login, About Bench, and Acknowledgements; notarise a release. |

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

On Linux, `internal/ui` only builds with the `server` tag and without cgo,
as the browser preview builds it. To have your editor's Go tools see it, set
both in gopls:

```json
"gopls": {
  "buildFlags": ["-tags=server"],
  "env": { "CGO_ENABLED": "0" }
}
```

With the tag but with cgo on, gopls still can't load `internal/ui`: Wails'
Linux build wants GTK 4 and WebKitGTK. On a Mac, set neither: the `server`
tag leaves out `internal/mac`'s native code.

Run `make help` to list the Makefile's targets.

## Packages and builds

[Packages](architecture.md#packages) in the architecture note lists what
each package does, and [Builds](architecture.md#builds) explains the three
builds and their build constraints. In short:

- Only `internal/mac` contains cgo, but `internal/ui` needs cgo too on
  macOS, through Wails, and builds on other systems only with the `server`
  tag. Everything else builds and tests on any OS.
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
| `go test ./...` | The app's build. On Linux, that leaves out `internal/ui`, and `internal/mac` runs only its pure-Go tests; on macOS, it includes `internal/ui` and `internal/mac`'s native code. Add the race detector with `make test RACE=-race`. |
| `CGO_ENABLED=0 go test -tags server ./...` | The browser preview's build. On Linux, it adds `internal/ui` in server mode. On macOS, it tests `internal/ui` in server mode instead of the app's, and leaves out `internal/mac`'s native code, as the preview does. |

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
first time (and again after an install that failed, or a change to the
lock file), builds the browser preview as `bin/bench-preview`, then runs
`scripts/e2e.sh`. The script runs `node --test` on each test folder that
exists:

| Folder | What it tests |
| --- | --- |
| `e2e/frontend/` | The UI against `FakeGo`, a mock of Go's bound methods and events, and the Acknowledgements window's page on its own. It needs no Go build. |
| `e2e/app/` | The browser preview, end to end. Each test starts its own preview, with a new support folder (`--dir`, and `BENCH_HOME` too) on a free port, and stops it afterwards. |

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
git ignores, so that you can compare the UI with the prototype, and see
the Acknowledgements window's page.

If `e2e/node_modules` is missing, the tests look for a global Playwright
install instead.

## Continuous integration

`.github/workflows/release.yml` builds releases; see
[Release a new version](#release-a-new-version).
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

### The macOS demo

`.github/workflows/macos-demo.yml` runs Bench on a GitHub macOS runner
(`macos-15`) with real apps, and records it. Other than your own Mac, it,
the [macOS web apps check](#the-macos-web-apps-check), and the
[macOS web items check](#the-macos-web-items-check) are the only places
where Bench's window runs and arranges other apps' windows or shows web
pages. The demo's
first runs found bugs that the tests couldn't, such as an AppKit call off
the main thread, first clicks lost while Bench wasn't the active app,
VS Code undocked when it moved its own window, and a sub-tab drag that
never ended.

macOS runner minutes cost more than Linux ones, so the demo doesn't run on
every push. To start it, do one of these:

- On GitHub, open **Actions ▸ macOS demo**, click **Run workflow**, and
  choose the branch.
- Run `gh workflow run macos-demo.yml --ref <branch>`.
- Push a commit whose message contains `[macos-demo]`. Only the last commit
  of the push (its head commit) counts: the tag in an earlier commit of the
  same push does nothing. If the message also contains `[skip ci]`, GitHub
  runs no workflow for the push.

GitHub only runs a workflow by hand once its file is on the default branch
(`main`). Until `macos-demo.yml` is there, start it with a commit message.

The job builds `bin/bench` with `go build`: the app's binary on its own,
not `Bench.app`. It installs iTerm2, VS Code, and Zed with Homebrew, with
`cliclick` and `ffmpeg`; TextEdit and Safari come with macOS. Then
`scripts/macos-demo/run.sh`:

1. Sets `BENCH_HOME` to a new folder holding
   `scripts/macos-demo/config.toml` (tabs Code, Notes, and Browse), and
   skips the first-run guide.
2. Makes the display as large as it can, up to 1,920 by 1,080, and sets
   the Mac up to look like one in use, since the README shows the
   recording: dark mode, the Dock hidden, no trust, update, or AI prompts
   in Zed and VS Code, quiet shells, and a small Go project with a short
   history. Then it opens the apps and starts recording the screen.
3. Starts Bench with `BENCH_DEBUG=1`, logs whether its window opened
   filling the screen, and takes a tour: it presses Bench's
   global shortcuts through System Events, clicks and drags sub-tabs with
   the real mouse through `cliclick`, uses the jump palette, moves and
   resizes Bench's window, and opens **Bench ▸ Acknowledgements** over the
   palette, taking a screenshot after each step. It finds
   Bench's panes and sub-tabs in `view.json` (see
   [Read the log](#read-the-log)).
4. Quits Bench, and logs whether the apps that Bench hid are visible
   again.

The runner's image gives `/bin/bash` and the runner's own processes
Accessibility, Screen Recording, and synthetic input permission, and Bench,
started from a job step, inherits them. So the demo runs Bench with real
permissions, but without the first-run guide, a bundle ID, or a signature.

The script doesn't stop when a step doesn't work, so a run that succeeds
doesn't mean every step did: watch the video, or read `steps.log`. A run
takes about 5 minutes, and the job stops after 40. Whether the script
finished or not, the job uploads the `macos-demo` artifact, which GitHub
keeps for 7 days:

| File | What it is |
| --- | --- |
| `tour.mp4` | The screen recording. |
| `shots/` | A screenshot after each step. |
| `steps.log` | Each step, and what the script checked, such as which tab a window ended up in. |
| `captions.txt`, `recording-started` | When each step and the recording started, in Unix seconds, to match steps to the video. |
| `bench.log` | Bench's standard error, with every platform call and event, and the engine's decisions. |
| `home/` | Bench's support folder at the end: `config.toml`, `state.json`, and `view.json`. |
| `diagnostics.txt` | The macOS version, the screen, the installed apps, and ffmpeg's capture devices. |
| `displays.txt`, `ffmpeg.log` | The display modes and the one the script chose, and ffmpeg's messages. |

To download it, find the run's ID with
`gh run list --workflow macos-demo.yml`, then run:

```sh
gh run download <run-id> -n macos-demo
```

To change the tour, edit `scripts/macos-demo/run.sh`, and to change its
tabs, `scripts/macos-demo/config.toml`.

#### Refresh the README's screenshots

The README's screenshots and animated tour (`docs/images/`) come from this
recording. To make them again, for example after changing the UI, run the
demo, download its artifact, and run:

```sh
scripts/readme-media.sh <folder>
```

The folder is the unzipped artifact. The script needs ffmpeg. It picks
screenshots by name, and cuts the tour's clips by the captions in
`captions.txt`, so if you rename a step or a screenshot in `run.sh`, change
the script to match. Look at `tour.gif` before you commit it: a step that
didn't work shows there. The script also makes `social-preview.png`, the
image for **Settings ▸ General ▸ Social preview** on GitHub, which link
previews show.

### The macOS web apps check

`.github/workflows/macos-webapps.yml` checks, on a GitHub macOS runner, that
Bench finds and docks the web apps that browsers make (see
[Web apps](user-guide.md#web-apps) in the user guide). Start it like the
demo: from **Actions ▸ macOS web apps**, with
`gh workflow run macos-webapps.yml --ref <branch>`, or with a head commit
whose message contains `[macos-webapps]`.

The job builds `bin/bench`, and `bin/scanapps`, which prints Bench's scan of
installed apps as JSON. It installs Brave and Vivaldi with Homebrew, and
Chrome and Edge if the image doesn't have them. If an install fails, it
updates Homebrew and tries again. A browser that still won't install is
left out with a warning: the job doesn't fail, and `results.md` lists the
browser as not tested, so check for that line before you trust a green run.
Then `scripts/macos-webapps/run.sh` makes a web app of the same site in
each browser that's installed:

- In Chrome, Edge, Brave, and Vivaldi, with the `WebAppInstallForceList`
  policy, which it writes to `/Library/Managed Preferences` before the
  browsers start.
- In Safari, by choosing **File ▸ Add to Dock** with System Events.

For each web app, it checks that:

1. Bench's scan finds it, names the browser that made it, and gives it the
   address it opens (`webUrl` in `scan.json`), on `squoosh.app`, so that the
   add dialog's **Web page…** finds it by its site. `results.md` shows the
   address.
2. A process with the web app's own bundle ID owns its windows, so that Bench
   can tell them from the browser's.
3. Bench, given a `config.toml` that lists each web app by bundle ID in a
   tab of its own, docks it: when the script switches to the tab with
   Bench's shortcut, the item is ready, and its window sits in its pane.

The job fails if any check fails. It uploads the `macos-webapps` artifact:
`results.md` (a table of the checks, also in the job's summary),
`steps.log`, `bench.log`, `scan.json`, `bundles.txt` (each web app's
`Info.plist`), `config.toml`, Bench's support folder in `home/`, a
screenshot of each step in `shots/`, and `jxa.err`, any errors from the
scripts that ask macOS about the web apps' processes and windows.

### The macOS web items check

`.github/workflows/macos-webitems.yml` checks, on a GitHub macOS runner, how
Web items deal with pages that try to put another site in place of the
item's page, or to keep Bench behind their dialogs (see
[New windows and links](user-guide.md#new-windows-and-links) and
[Alerts and camera requests](user-guide.md#alerts-and-camera-requests) in
the user guide). Bench's rules for these depend on what WebKit tells it
about each navigation and dialog, such as which frame started a
navigation, which only a real Mac shows. Start it like the demo: from
**Actions ▸ macOS web items**, with
`gh workflow run macos-webitems.yml --ref <branch>`, or with a head commit
whose message contains `[macos-webitems]`.

The job builds `bin/bench`, and `bin/webitems-server`, a small Go server
(`scripts/macos-webitems/server`) that serves the test pages over https on
two sites: the items' own, `https://localhost:8443`, and another,
`https://127.0.0.1:9443`. Bench counts each local host and port as a site
of its own, and WebKit counts the two hosts as two sites. The job installs
`cliclick` with Homebrew. Then `scripts/macos-webitems/run.sh`:

1. Makes a throwaway certificate authority, and a certificate for both
   hosts, with `openssl`, and trusts the authority system-wide with
   `security add-trusted-cert`, since a Web item's address must be https.
   So that this doesn't wait for an administrator's password, it first
   lets anyone change the Mac's trust settings, which is fine on a runner
   that's thrown away after the job. It checks that macOS trusts the
   certificate, that the server answers, and that the default browser can
   open the server's pages.
2. Starts Bench with `BENCH_DEBUG=1` and a `config.toml` that gives each
   check a Web item in a tab of its own, then switches to each tab with
   Bench's shortcut.
3. Runs the checks:
   - **Frame.** A frame from the other site, once clicked with `cliclick`,
     sets `top.location` to the other site.
   - **Redirector.** A clicked link goes to a page on the item's site that
     sends the user on to the other site with `location.replace`.
   - **Long alert.** A page's alert has 1,999 characters, then one more
     with 2,000,000 combining marks.
   - **Alert loop.** A page brings up an alert, then reloads itself, again
     and again. The script ticks the second alert's **Don't allow more
     dialogs from this page** with System Events. If no alert offers it,
     or ticking it doesn't take, the script keeps pressing **OK** until
     Bench holds the page's alerts back.

For Frame and Redirector, the item must stay on its site, and the other
site's page must open in the default browser, which three records must
agree on: the item's `url`, `host`, and `offSite` in Bench's `view.json`;
Bench's log of its decision (`msg="web navigation"`, with
`decision=in-browser` and the other site's `host`); and the server's log,
where a request that no page asked for (`"fresh": true`, from
`Sec-Fetch-Site: none`) is the browser opening the page, and any other
request for it would be the web view's. For Long alert, Bench must answer
System Events within 5 seconds while the alert shows, and the alert's text
must be at most 2,100 UTF-16 code units. For Alert loop, the second alert
must offer the checkbox, ticking it must take, and no alert may show for 10
seconds after the stop, while the page's own reports show that it kept
reloading and calling `alert()`, and that each call returned at once. When
the stop was never made, those checks fail, even though Bench's hold keeps
the alerts away, and the same 10 seconds check the hold instead, as **Bench
holds back a burst of alerts**.

The job fails if any check fails. It uploads the `macos-webitems` artifact:
`results.md` (a table of the checks, also in the job's summary),
`steps.log`, `bench.log`, `decisions.txt` (each item's navigation
decisions from `bench.log`), `server.log`, `requests.jsonl` (the server's
log of each request, and the pages' reports of what they did),
`diagnostics.txt` (the macOS version, the default browser, and the output
of `openssl` and `security`), the certificates without their keys in
`certs/`, Bench's support folder in `home/`, a screenshot of each step in
`shots/`, and `jxa.err`, any errors from the scripts that read Bench's
dialogs.

To change a check's page, edit the server's `pages`; its tests, which run
with the other Go tests, check the parts that `run.sh` relies on. The check
hasn't run on a runner yet, so expect to fix something in `run.sh` on its
first run.

## The browser preview

```sh
make preview
```

This runs `CGO_ENABLED=0 go run -tags server ./cmd/bench --port 8080` and
serves the UI at <http://localhost:8080>. The preview is the real app,
with the demo platform (`platform.NewDemo`) in place of macOS and pretend
terminals (`term.Echo`) in place of shells. The demo's default browser is
Safari, which makes a web app of a site 3 seconds after the add dialog's
**Web page…** opens the site in it, as if you had followed Safari's steps.
It also has Gmail and Slack web apps, which Brave made. Slack's is of the
example config's Slack page, so that page's **⋯** menu offers it.

| Option | Effect |
| --- | --- |
| `--port` | The port. `make preview PORT=9000` sets it. |
| `--dir` | The folder for the preview's `config.toml` and `state.json`. Without it, the preview uses a `Preview` folder in Bench's support folder (`~/.config/Bench/Preview` on Linux), or in `$BENCH_HOME` if that's set. It never uses the app's folder itself, where it would put back, and so drop, the app's `parked.json` records, since the demo has none of their windows. |

The preview serves on `localhost` and has no authentication, which is why
its terminals never run real shells. It ignores Wails' `WAILS_SERVER_HOST`
and `WAILS_SERVER_PORT` environment variables, which would change the
address and port, and it refuses calls to Go that come from other web
sites, or through a host name other than `localhost` or `127.0.0.1`.

In the preview, Bench's global shortcuts work while the page has focus, and
Control+K also opens the jump palette. The bar above the window fits the
window to the browser or simulates a monitor or a laptop.

## Build the app

On a Mac:

| Command | What you get |
| --- | --- |
| `make build` | `bin/bench`, for your Mac's architecture, not signed with your certificate. Use it for the commands. |
| `make app` | `bin/Bench.app`: a universal binary (arm64 and amd64), `Info.plist`, `Bench.icns`, and `Credits.rtf`, signed with the hardened runtime. |
| `make run` | `make app`, then opens it. |
| `make dist` | `dist/Bench.dmg` and `dist/Bench.zip`, the files a release offers, from the `bin/Bench.app` that `make app` built. See [Release a new version](#release-a-new-version). |

`make app` takes these variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `SIGN_IDENTITY` | `Apple Development` | The certificate to sign with. `-` signs ad hoc; empty skips signing. |
| `SIGN_TIMESTAMP` | `--timestamp=none` | `codesign`'s timestamp flag. Notarisation needs `--timestamp`, which the release workflow sets. |
| `ARCHS` | `arm64 amd64` | The architectures to build. Name one, such as `ARCHS=arm64`, for a faster build. |
| `APP_TAGS` | `production` | Build tags. The `production` tag turns off Wails' web inspector; set `APP_TAGS=` to keep it. |
| `VERSION` | From `Info.plist` | The version that `bench version` prints. |

`make app` builds with a macOS 14 deployment target, to match
`LSMinimumSystemVersion`. The files it packages are in `build/darwin/`; see
[its README](../build/darwin/README.md).

If a process of yours called `bench` is running, `make run` stops before
building: `open` would bring the running Bench forward rather than start the
new build. That's Bench, a command such as `bench arrange`, or the browser
preview that `make preview` started, because `go run` names its program
`bench` too. Quit it, then run `make run` again. Other users' processes,
such as a Bench left open behind fast user switching, don't stop it.

To change the icon, edit `build/darwin/icon/main.go`, run `make icon`, and
commit the new `build/darwin/icon.png`. The menu bar item's icon works the
same way, from `build/darwin/menubar/main.go` to `internal/ui/menubar.png`.

After changing a dependency, run `make acknowledgements`, and commit the new
`frontend/acknowledgements.html`, the Acknowledgements window's page. See
[Add or update a dependency](#add-or-update-a-dependency).

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

## Add or update a dependency

**Bench ▸ Acknowledgements** lists the open source software that
`Bench.app` includes, with each one's copyright notices and licence. Its
page, `frontend/acknowledgements.html`, is written by
`build/acknowledgements`, a command that isn't part of the app. It lists
what the app's build links, and reads the texts from the module cache, the
Go toolchain, `frontend/vendor`, and `build/acknowledgements/texts`. The
components themselves, and the code that each took from other projects,
are listed by hand in `build/acknowledgements/components.go`.

When you add, update, or remove a Go module that the app links, change the
Go toolchain, or add, update, or remove a package in `frontend/vendor`
(see [its README](../frontend/vendor/README.md)):

1. Check the dependency's licence, and look for code that it took from
   other projects, which carries notices of its own.
2. Change `components.go` to match, as below.
3. Run `make acknowledgements`, and commit the new
   `frontend/acknowledgements.html` with the change.
4. Run `go test ./build/acknowledgements`.

`make acknowledgements` runs
`go run ./build/acknowledgements -o frontend/acknowledgements.html`. It
writes the same page every time from the same sources, and stops if
`components.go` doesn't match the modules that the build links and the
packages that `frontend/vendor/README.md` lists.

The tests run `go list` for the macOS build (`CGO_ENABLED=1 GOOS=darwin`,
for each of the Makefile's `ARCHS`, with its `APP_TAGS`) on any OS, so they
need that build's modules in your module cache, or the network to download
them. `go mod download` fetches them in advance.

### What the tests check

The tests in `build/acknowledgements` run with the other Go tests. Most
failures say what to do.

| Test | Fails when |
| --- | --- |
| `TestPageIsCurrent` | The page isn't what `make acknowledgements` would write, for example after a module's version changes. It shows the first line that differs. Run `make acknowledgements`. |
| `TestModulesAreListed` | The app links a third-party module that `components.go` doesn't list, or `components.go` lists a module that the app no longer links. It also fails if the Makefile's `ARCHS` or `APP_TAGS` default changes: check the list against the new build, then change the test. |
| `TestVendoredPackagesAreListed` | `frontend/vendor/README.md`'s table and `components.go` don't agree about a package or its version, or a licence file in `frontend/vendor` isn't one of a component's texts. |
| `TestNoticesAreListed` | A file that the app includes from someone else, the Go standard library's among them, has a copyright notice that the component it belongs to doesn't account for, with its own holders or its parts'. It names the file, the component, and the notice. It also fails when a notice in `reviewed` has gone from its file. |
| `TestTargetsMatchTheBuild` | The Makefile's `app-bin` recipe no longer builds `./cmd/bench` for darwin with cgo, for each of `ARCHS`, with `APP_TAGS`, as the generator assumes. |
| `TestFixedTextsAreUsed` | A file in `build/acknowledgements/texts` isn't any component's text, or `MPL-2.0.txt` there isn't the Mozilla Public License 2.0. |
| `TestPageIsStatic` | The page has a script, uses an id twice, or links to anything but its own sections and its style sheets. Its window has no address bar, so a link would replace the page. |

The other tests check the generator's own code.

When `TestNoticesAreListed` reports a notice, find the code that it
covers:

- Code that the dependency took from another project needs a part, with
  that project's notice and licence.
- A new holder of the component's own code, such as a new author, goes in
  the component's `Holders`.
- A notice that needs no entry, such as one that only names another
  project, goes in `reviewed`, with its file and the reason.

The tests search the Go standard library too, so a new Go toolchain can
bring a notice that the Go component's parts need to account for.

### Add a component or a part

Add a component to `components` in `components.go`, in any order: the page
sorts them by name. The comments on `component` and `text` explain each
field. In short:

- Every component has a `Name`, its `Licence`'s name, and its `Texts`.
  `Holders` are regular expressions for the copyright holders that its
  notices name, which `TestNoticesAreListed` matches notices against.
- A Go module has `Module`, its path. The page shows the version that
  `go list` reports. Its texts are usually `{From: module, File: "LICENSE"}`.
- A vendored package has `Package` and `Version`, as
  `frontend/vendor/README.md`'s table names them, an `Address`, and texts
  from `frontend/vendor` (`From: repo`).
- Code that a component took from another project, under that project's
  notice, is a part, in the component's `Parts`, with `About` saying what
  it is. Its texts are often a block of comments in the file that holds the
  code, which `First` and `Last` pick out.
- If a part's texts don't come from the component's files, set `Evidence`
  to one of the component's files and a string in it, such as the notice,
  so that the generator stops once the component no longer includes the
  part.

A text that isn't in the module cache, the Go toolchain, or
`frontend/vendor`, such as a licence that a part's notice only points to,
goes in `build/acknowledgements/texts`. Copy it unchanged, or with only
its comment markers taken out, use it with
`From: fixed`, and say where it came from, with its address, in a comment
above it in `components.go`.

## Release a new version

`.github/workflows/release.yml` builds and publishes a release when you push
a version tag. To release version 0.2.0:

1. Set `CFBundleShortVersionString` and `CFBundleVersion` in
   `build/darwin/Info.plist` to `0.2.0`, and merge that to `main`.
2. Work through the [manual checks](#before-each-release).
3. Tag the commit on `main` and push the tag:

   ```sh
   git tag v0.2.0
   git push origin v0.2.0
   ```

The workflow builds the universal `Bench.app` on a macOS runner, packages it
as `Bench.dmg` and `Bench.zip` with `make dist`, and checks both as a
download would see them: the signature, both architectures, and that
`bench version` prints the right version. Then it creates a GitHub release
with the two files, `SHA256SUMS.txt`, install steps, and notes generated
from the merged pull requests. It takes a minute or two, and a few minutes
more when it notarises.

- If the tag doesn't match `Info.plist`, the workflow stops before
  building.
- A tag with a suffix, such as `v0.2.0-beta.1`, makes a pre-release.
  GitHub's "latest" link, and so the README's download button, skips
  pre-releases.
- If the release already exists, for example because you ran the workflow
  again, it replaces the release's files.
- The README's download button links to
  `releases/latest/download/Bench.dmg`, so keep the file names.

A push that changes the workflow, the Makefile, `build/darwin/`, or
`scripts/notarise.sh`, or a run you start from **Actions ▸ Release ▸ Run
workflow**, builds and checks the same files and keeps them as the run's
`Bench-<version>` artifact, without a release. To build them on your Mac,
run `make app`, then `make dist`.

### Signing and notarising releases

Without any secrets, the workflow signs `Bench.app` ad hoc. macOS then
won't open a downloaded Bench until the person clicks **Open Anyway** in
**System Settings ▸ Privacy & Security**, and it forgets Bench's
Accessibility permission with each update (see
[Signing and the Accessibility permission](#signing-and-the-accessibility-permission)).
The release notes and the README explain both.

To sign with a Developer ID certificate and have Apple notarise each
release, you need to be a member of the Apple Developer Program. Add these
repository secrets in **Settings ▸ Secrets and variables ▸ Actions**:

| Secret | What it is |
| --- | --- |
| `MACOS_CERTIFICATE` | Your **Developer ID Application** certificate and its private key, exported from Keychain Access as a `.p12` file, then encoded with `base64 -i certificate.p12 \| pbcopy`. |
| `MACOS_CERTIFICATE_PASSWORD` | The password you chose when you exported the `.p12` file. |
| `APPLE_ID` | The email address of an Apple Account in your developer team. |
| `APPLE_TEAM_ID` | Your team's 10-character ID, from **Membership details** at [developer.apple.com/account](https://developer.apple.com/account). |
| `APPLE_APP_PASSWORD` | An [app-specific password](https://support.apple.com/en-gb/102654) for that Apple Account. |

With `MACOS_CERTIFICATE`, the workflow signs `Bench.app` with the
certificate and a secure timestamp (`make app SIGN_TIMESTAMP=--timestamp`).
With the three Apple Account secrets as well, it notarises `Bench.app` and
`Bench.dmg` with `scripts/notarise.sh`, and staples Apple's tickets to them,
so they open without a warning, even offline. If Apple rejects a file, the
job prints Apple's log, which says why. After the first notarised release,
update the README's install steps, which describe Open Anyway.

The repository has no certificate yet, so these steps haven't run. Expect to
fix something on their first run: start the workflow by hand first, which
builds without publishing.

## Manual checks on a Mac

The [macOS demo](#the-macos-demo), the
[macOS web apps check](#the-macos-web-apps-check), and the
[macOS web items check](#the-macos-web-items-check) run part of the macOS
layer on a real Mac, but only what their scripts do, on one display, with
permissions granted in advance. The rest can only be tested by hand. Work
through these before each release, and after changes to `internal/mac`,
`internal/windows`, or `internal/ui`. They come from the spec's
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
- [ ] Sub-tab markers: a window in native full screen has the grey i on its
      icon, one whose app is opening has the grey ring, and one dragged
      out of its pane has the amber warning sign. Right-clicking each sub-tab
      shows why as its menu's first line. The status bar says how many
      windows need attention, counting only the one with the warning sign;
      clicking it lists them, and choosing one shows it.
- [ ] Tooltips while another app is active: type in a docked app, then,
      without clicking Bench, hold the pointer over a tab, a sub-tab, a
      pane's **+** and **⋯**, and the gear. Each shows its tooltip (Bench's
      window sets `allowsToolTipsWhenApplicationIsInactive`). Try it with a
      trackpad and with a mouse, and note whether the hover backgrounds and
      live previews show too before Bench is clicked: if they don't, WebKit
      tracks the pointer only in the key window, and an always-active
      tracking area on the UI's web view is the next thing to try. Nothing
      may depend on hovering alone. If the tooltips do show, the user
      guide's Panes section can say so plainly, instead of "Tooltips can
      also show".
- [ ] The title bar's free space, between the tabs' **+** and the layout
      control, moves the window when dragged and zooms it when
      double-clicked, with three tabs and with more tabs than fit. With
      more than fit, **+** stays in view at the end of the tab strip.
- [ ] Opening filled: with **Open Bench filling the screen** on, quit and
      open Bench. The window first shows already covering everything
      below the menu bar and beside the Dock: no smaller window first, no
      zoom animation, and no flash of white. The docked windows fill their
      panes. The green button then gives back a 1,440 by 900 window in the
      middle of the screen. With the setting off, Bench opens at that size
      and place; if it opens somewhere else, Wails' `WindowXY` doesn't place
      it as [decision 30](architecture.md#decisions) says.
- [ ] The display Bench was last on, with a laptop and an external monitor:
      move Bench to the monitor, quit, and open it: it opens there,
      filled. Quit, unplug the monitor, and open it: it opens on the
      laptop's screen. Try it with the monitor arranged above and to the
      left of the laptop, where coordinates are negative, and with the
      monitor as the main display. With two monitors of the same model,
      move Bench to the second, quit, plug them into each other's ports,
      and open Bench: it opens on the same monitor as before.
- [ ] Filling again: with Bench filling the monitor, unplug it. Once
      macOS has moved Bench to the laptop's screen, Bench fills that
      screen, once, within about 2 seconds, and the tab is arranged
      again. Plug the monitor back in: Bench stays filling the laptop's
      screen. Make the window smaller, then unplug and plug in again: it
      stays smaller. With Bench filling the screen, change the resolution
      in **System Settings ▸ Displays**, and show and hide the Dock
      (Option-Command-D): Bench fills the new visible area each time, and
      doesn't keep zooming.
- [ ] The minimum size: double-click the title bar's free space to zoom,
      double-click again, then drag the window's corner inwards: it stops
      at 900 by 600 points. The same after the green button, and after
      **View ▸ Fill the Screen**, when the window is un-zoomed.
- [ ] **View ▸ Fill the Screen** and the jump palette's **Fill the
      screen** zoom a smaller window, and leave a window that fills the
      screen as it is, rather than shrinking it.
- [ ] macOS 15's tiling: **Window ▸ Fill**, **Window ▸ Move & Resize ▸
      Left**, and dragging Bench to a screen's edge. The docked windows
      follow each time. Unplugging a display afterwards fills the screen
      again after **Fill**, which counts as filling, but not after tiling
      to half the screen. In **System Settings ▸ Desktop & Dock**, set
      **Double-click a window's title bar to** each of **Fill**, **Zoom**,
      **Minimise**, and **Do Nothing**, and double-click the title bar's
      free space: note what each does, since Wails knows only Zoom,
      Minimise, and Do Nothing, and may do nothing for **Fill**.
- [ ] Stage Manager on: open Bench, choose **View ▸ Fill the Screen**, and
      unplug a display. Note whether Bench fills the space beside the strip
      of recent apps, and whether it counts as filling the screen.
- [ ] Open at login with **Open Bench filling the screen** on: log out and
      in. Bench opens filled, on the display it was last on, with no
      smaller window first.
- [ ] Three Brave windows in one pane.
- [ ] One of them moved beside another, by dragging its sub-tab to the
      pane's right edge.
- [ ] One of them moved to another tab, by dropping its sub-tab on the tab.
- [ ] Brave quit and reopened: its windows come back to their panes.
- [ ] Brave, with windows in two panes of one tab, quit with Command+Q:
      their sub-tabs are hidden, and both panes stay. A pane left with
      nothing to show says "Brave isn't running", with **Open Brave**,
      **Remove from this pane**, and **Close this pane**; a pane that still
      shows another window lists Brave in its **⋯** menu, under **Closed
      windows**. Switching to another tab and back doesn't open Brave.
      Opening Brave from the Dock puts the windows it restores back in
      their panes, and a minute later, a new window from Command+N goes
      where `new_windows` says, not to a hidden place.
- [ ] **Open Brave** in an empty pane, with Brave quit: Brave opens, and a
      window comes to that pane. With Brave open but none of its windows
      in the pane, it opens a new Brave window there.
- [ ] **Remove from this pane** forgets Brave there, and a pane left with
      nothing closes. **Close this pane** closes an empty pane, and the
      panes beside it close up. In a tab that shows one pane with
      sub-tabs, there's no **Close this pane**, and **Remove from this
      pane** forgets Brave in the whole tab.
- [ ] With **Open Bench at login** on, and **Reopen windows when logging
      back in** off, log out with a tab's app open, and log in again:
      switching to the tab opens the app, because Bench didn't take the
      log-out's quit for yours.
- [ ] A link clicked in Slack that opens in a Brave window in another tab:
      Bench switches tabs (with `follow_focus = "switch-tab"`).
- [ ] In a Web item, a `zoommtg:` link that you click asks once, with
      **Cancel** as the default button, and one that the page's script
      clicks (`a.click()` from a timer) does nothing. A download link that
      you click saves the file, and one the script clicks, a minute after
      your last click in the page, doesn't.
- [ ] In a Web item, an embedded frame from another site that you've
      clicked in once, and that then sets `top.location` to another site,
      doesn't replace the page: the site opens in the browser if you've just
      clicked in the page, and otherwise nothing happens. So does a popup
      that sets `opener.location`. A clicked link whose server takes more
      than 2 seconds to redirect to another site, and one through Google's
      `/url`, open that site in the browser. A link you click to an Okta
      sign-in page of your own that then sends you to another site opens one
      browser tab, and the item goes back to its own address; swiping back
      to the sign-in page and letting it redirect again opens the next one in
      the browser too, not in the item. A page that clicks 100 `mailto:`
      links after one click opens one compose window. Signing in to Outlook
      through Microsoft and an AD FS server, starting from the item's own
      page, stays in the item.
- [ ] In a signed-out Jira item, a link you click to an Okta sign-in page
      of your own that then sends you to another site has the item go back
      to Jira, and on to Atlassian's sign-in page at `id.atlassian.com`,
      rather than stop on the Okta page. With `BENCH_DEBUG=1`, Bench's log
      shows that redirect's `web navigation` decision as `in-view`, with a
      `navigation` other than 0: WebKit's private `_mainFrameNavigation`
      still says which navigation a redirect is for.
- [ ] A Web item's pane header shows the host of the page in view, and
      changes only once another page has loaded: a link that's slow to
      load, or that fails or is stopped, leaves the host as it was. A page
      that changes its own address, as Slack does between channels, updates
      the item's address. An `http` page makes the header say **Not
      secure**, but only once it has loaded, and so does an `https` page
      once it loads something over `http`. Microsoft's sign-in page from
      Outlook makes it say **Other site**. A page that moves itself to a
      `blob:` page it made shows its own host, and one that moves to a
      `data:` page or `about:blank` says **page with no address** after a
      warning. **Open in browser** opens the page in the default browser,
      and after a restart, before the page has loaded, the item's own
      address. A popped-out web page's window, and a sign-in popup, show the
      host as their subtitle.
- [ ] Dialogs, settings windows, and picture-in-picture stay floating.
      With Bench filling the screen, start picture-in-picture in the
      browser that the default config's float rule names, move it over
      Bench, and click Bench's tab row: it stays. A Finder window over
      Bench hides instead.
- [ ] A docked window dragged out by the user: its pane shows **Dock again**.
- [ ] Bench on a second desktop (Space): items whose windows are on another
      desktop show "On another desktop".
- [ ] An app that moves or resizes its own docked window, as VS Code often
      does just after Bench sizes it, is put back in its pane.
- [ ] **Bench ▸ Hide Bench**, minimising Bench, and closing its window each
      hide the current tab's apps with Bench, and **Show Bench** in the menu
      bar item brings them back. **Hide** in Bench's Dock menu while a
      docked app is active hides them too, a second later. **Hide Others**
      in a docked app hides Bench but leaves that app's windows on screen.
- [ ] Tab shortcuts while Bench is hidden: hide Bench (Command+H),
      minimise it, and close its window, and each time press Control+Option+2.
      Bench comes back on tab 2, with its windows docked, and its focused
      pane has the keyboard: type, and the text goes there, not to Bench's
      window. Control+Option+] and Control+Option+[ do the same. With Bench
      hidden, Control+Option+arrow, Control+Option+Shift+arrow, and
      Control+Option+Tab leave it hidden. Control+Option+9 with fewer tabs
      brings Bench back saying "There's no tab 9.". After **Hide Others** in
      a docked app, Control+Option+2 brings Bench back too, on tab 2. With
      **Bring back a hidden Bench with tab shortcuts** unticked in
      Settings, Control+Option+2 leaves Bench hidden, and **Show Bench**
      then shows tab 2.
- [ ] The menu bar item: Bench's icon, tinted to suit the menu bar in light
      and dark mode, and over a light and a dark desktop picture, with the
      current tab's name beside it. Rename a tab to a name longer than 16
      characters: the item shows 15 of them and an ellipsis. Its menu ticks
      the current tab and shows each of the first nine tabs' shortcuts
      (⌃⌥1 and on), then **Jump to…** with ⌃⌥Space, **Settings…**, **Show
      Bench**, and **Quit Bench**. **Jump to…** and **Settings…** bring
      Bench forward with the palette or Settings open, from another app and
      with Bench hidden. With the menu open, Control+Option+2 switches to
      tab 2 once. Change `tab_modifiers` and reload `config.toml`: the menu
      shows the new shortcuts.
- [ ] **Settings ▸ Menu bar item**: **Icon only** shows the icon alone,
      with no gap beside it; **Hidden** removes the item as soon as Settings
      is saved; **Icon and tab name** brings it back, showing the current
      tab. Quit and open Bench with **Hidden** chosen: no item appears, even
      for a moment. Then choose **Icon only** and save: the item appears.
      Choose **Hidden** again, save, and quit; set `menu_bar = "name"` in
      `config.toml`, and open Bench: the item appears, with the tab's name.
      On a MacBook with a notch and a full menu bar, note
      whether **Icon only** keeps Bench's item in view.
- [ ] Bench's icon in the Dock brings Bench's window back: after Command+H,
      after minimising it, and after closing it, with and without a
      popped-out web page or terminal open. With Bench's window shown,
      clicking the Dock icon while a popped-out terminal has the keyboard
      leaves the terminal with it.
- [ ] In `Bench.app`, **Bench ▸ About Bench** shows the version, and under
      it the credits: a link to the repository, and a note that points to
      **Bench ▸ Acknowledgements**. The credits are readable in both light
      and dark mode (**System Settings ▸ Appearance**).
- [ ] **Bench ▸ Acknowledgements** opens one window. Choosing it again
      brings that window forward, minimised or not, rather than opening
      another. Command+W closes it. Its text can be selected, and copied
      with Command+C. With Bench's window closed, it opens without bringing
      Bench's window back, and clicking it from another app leaves that
      app's windows on screen. With Settings open in Bench's window, it
      opens in front of Bench's window, and clicking Bench's window brings
      Settings back in front of it.
- [ ] Focusing a pane with Control+Option+arrow gives its window the
      keyboard, whether it shows an App window, a web page, or a terminal.
      Bench doesn't yet take the keyboard back from a web page for a
      terminal: note whether typing reaches the terminal.
- [ ] Typing in a web page, opening the jump palette with its shortcut and
      closing it with Escape gives the page the keyboard back.
- [ ] Typing in a docked App window, such as Zed's, opening the jump
      palette with its shortcut and closing it with Escape, a click outside
      it, or the shortcut again gives Zed's window the keyboard back: typing
      carries on in Zed, without a click. Choosing another tab in the
      palette focuses that tab's pane instead, without Zed coming forward
      first. Choosing **Settings…** and then **Cancel** gives Zed the
      keyboard back once Settings closes. With another of Zed's windows
      typed in, one not in a tab or its Settings dialog, closing the palette
      with Escape leaves the docked window where it is.
- [ ] Right-clicking a tab, a sub-tab, and the empty part of a pane's
      header, and Control-clicking them, opens the menu at the pointer,
      both while Bench is active and while a docked app is. Near the
      window's right and bottom edges the menu stays inside the window. The
      empty part of a header opens the shown window's menu, and closing it
      without choosing focuses that pane. Right-clicking an empty pane, its
      header or its body, opens its menu at the pointer: **Add a window or
      app…** opens the add dialog for it, and **Close this pane** closes it.
- [ ] With Bench's window at its smallest, 900 by 600, Tab properties,
      Settings, the add dialog, the jump palette, and the welcome guide each
      fit inside the window, clear of the traffic lights, with their
      buttons in view. The middle of Tab properties scrolls with the
      trackpad, and Tab moves through it, scrolling to each control.
- [ ] A popped-out web page minimised to the Dock that calls `confirm()`
      shows the dialog when its window is restored.
- [ ] A web page that runs `alert('x'); location.reload()`: tick **Don't
      allow more dialogs from this page** on the second alert, and no more
      come, after reloads too; or don't, and no more come after the third,
      until you click a link in the page. A `data:` frame's `alert()` shows
      nothing, and an advert's names it as embedded in the page. An alert
      of `'x'.repeat(1999) + 'a' + '\u0301'.repeat(20e6)` shows 1,999 x's
      at once.
- [ ] A web page that calls `getUserMedia({ video: true })` asks with an
      alert above the docked windows that names the item and the page's
      host, only once Bench is active and the page can be seen, with **Don't
      Allow** as the default; **Allow** turns the camera on. A frame from
      another site that asks is refused without an alert.
- [ ] **Web page…** in the add dialog, with Safari, then Chrome, then
      Firefox as the default browser (**System Settings ▸ Desktop & Dock ▸
      Default web browser**). A site that has a web app adds it. One that
      hasn't opens in the browser with its steps, and once you've made the
      web app, it joins the pane in a second or two, with one window, and
      the dialog closes. So does one for `slack.com`, which sends you to
      `app.slack.com` once you're signed in. Meanwhile, clicking the
      browser's windows that belong to another tab doesn't switch tabs,
      clicking Bench doesn't hide the browser, and a browser that wasn't
      open opens with a window that stays out of the tab. With Firefox, the
      page shows in Bench, with a note that says why.
- [ ] `config.toml` edited outside Bench: with Bench running, change
      `gap` in the file in another editor, then click Bench. The status bar
      says "config.toml was changed outside Bench", with **Reload**; nothing
      is written. Save Settings without reloading: the toast says
      "Applied for now, not saved: config.toml was changed outside Bench",
      the file keeps the edit, and the notice offers **Reload** and
      **Overwrite**. Its buttons work with the mouse and by keyboard, both
      while Bench is active and while a docked app is, above the docked
      windows (raised mode). **Overwrite** saves Bench's config and keeps
      the edited file as `config.toml.bak`; **Reload** applies the edit.
      Edit the file again, then press Command-Tab straight to an app with a
      window docked in a tab, without clicking Bench: the notice appears.
      Edit it once more in a terminal in Bench, with `claude` or `vi`, and
      open Settings without leaving Bench: the notice appears too.
- [ ] **Bench ▸ Reload config.toml**, and the jump palette's **Reload
      config.toml**, after editing the file: change a tab's layout, add a
      tab with an app that's running, remove a tab whose App windows are
      docked, remove a Web item, and change `[shortcuts] jump`. The
      arrangement of the tabs that stay is kept;
      the new tab claims the app's windows that no tab had; the removed
      tab's windows go to "Not in a tab", shown and in their own frames,
      not parked or hidden, and the status bar names the tab whose
      arrangement Bench couldn't keep; the Web item's page closes; and the
      new jump shortcut works from another app at once, while the old one
      no longer does (another app can register it).
- [ ] Reload after renaming a split tab whose apps are running, and adding
      another tab, in `config.toml`: the renamed tab's windows are docked in
      it at once, in the file's layout, no app opens a new window, and its
      terminal keeps running, with its output still there. The status bar
      names the old tab. Rename it back in the file, reload, and choose
      **Restore previous arrangement**: the split comes back, with the same
      windows docked, and the terminal still running. Then move an `app`
      entry whose window is open to another tab and reload: the window is
      docked in the other tab, and the app doesn't open another.
- [ ] `config.toml` with a problem, such as `gapp = 4`, when Bench starts:
      Bench runs with the last config that loaded without problems (or the
      default config on a new Mac), and the status bar says which, with
      **Open config.toml**, **Show in Finder**, and **Reload**. **Open
      config.toml** opens it in the app that opens plain text (TextEdit
      unless you've chosen another), not an app that only shows `.toml`
      files, and brings it forward. **Show in Finder** opens a Finder
      window with the file selected. With a `config.toml.bak` that parses,
      **Use backup** swaps the two files and applies the backup. Saving
      Settings says "Applied for now, not saved: config.toml has problems",
      and `config.toml` doesn't change. Running with the last config that
      worked, and the arrangement saved for it, the first notice says Bench
      saves the arrangement only: moving a window to another pane changes
      `state.json` a second or so later, but adding a tab doesn't. Fixing
      the file and choosing **Reload** clears the notice.
- [ ] Tabs changed in `config.toml` while Bench is closed: split a tab
      whose apps are running into three panes, and quit. Move its
      `[[tab]]` block first, and add a tab: when Bench opens, the tab keeps
      its three panes, with its windows docked in them, and there's no
      notice. Quit, rename the tab in the file, and add another tab: the
      status bar names the tab whose arrangement Bench couldn't keep, and
      its windows are docked in the tab built from the file. Give that tab
      the old name in Tab properties, and choose **Restore previous
      arrangement**: the three panes come back, with the same windows docked
      in them, and none is left on screen outside Bench; the tab's terminal
      is the one that was running, not a new one.
- [ ] Hiding the status bar: untick **Show status bar** in Settings, with
      docked windows in two panes. The status bar goes, and the docked
      windows grow by its height at once, reaching the window's foot, with
      no gap or overlap below them; ticking it again shrinks them back.
      With a notice and a window dragged out of its pane, **1 needs
      attention** and **1 notice** are amber buttons left of the search
      button, and each opens its list below it, above the docked windows.
- [ ] Toasts in the title bar, with the status bar hidden: choose
      **View ▸ Rearrange**. "Rearranged" shows in the middle of
      the title bar's empty space, above the docked windows, for a few
      seconds. While it shows, dragging the title bar under it moves the
      window, and double-clicking there zooms it. In a window about 900
      points wide, a long message goes across the title bar, but never over
      the traffic lights, which still close, minimise, and zoom the window.
- [ ] The slim title bar: make Bench's window narrower than 1,200 points.
      The layout control becomes **Auto ▾**, which opens a menu of the
      three layouts above the docked windows, and **Jump to…** its
      magnifying glass, whose tooltip says "Jump to… (⌃⌥Space)". VoiceOver
      reads the button as "Jump to… (⌃⌥Space)" and the layout menu's items
      as radio items, with the one in use ticked. Widening the window brings
      the whole control back.
- [ ] **View ▸ Layout ▸ Auto**, **Wide**, and **Compact** have no
      shortcuts, and the one in use is ticked: choosing one
      changes the layout, and choosing a layout in the title bar or the jump
      palette moves the tick. The menu doesn't hang while Bench changes the
      layout.
- [ ] **Space between panes** at 2 points: the dividers between docked
      windows can still be dragged, and the pointer changes over them.
- [ ] `internal/mac` links the UniformTypeIdentifiers framework (for
      `UTTypePlainText`) on macOS 14 and 15, on Apple silicon and Intel.
- [ ] Strips that don't fit: with 15 tabs and a pane of 15 sub-tabs, and a
      docked app active, hold the pointer over the tab strip, then the
      sub-tabs, and turn a mouse wheel, then swipe up and down and sideways
      on a trackpad. Each strip scrolls sideways, smoothly, without moving
      the page or the docked windows, and an end with more to show fades
      out (`mask-image` in WKWebView). A live preview showing for a sub-tab
      hides as the wheel turns. Switch tabs with Control+Option+] past the
      end of the strip: the current tab scrolls into view, clear of the
      fade. Scroll the strip away from it by hand: it stays there as the
      docked app's title changes, and comes back when Bench's window is
      resized. With Keyboard navigation on in System Settings, press Tab
      to move to a tab out of view: it stays in view as the title changes.
- [ ] Dragging a sub-tab to a tab out of view, with the window raised and
      not: hold it near the right end of the tab strip, which scrolls until
      the last tab shows, tints the tab under the pointer as it passes, and
      drops there; near the left end it scrolls back.
- [ ] **Show scroll bars** on **Automatic**: with System Settings ▸
      Appearance ▸ Show scroll bars on **When scrolling**, the strips show
      no bar; set it to **Always** while Bench runs, and a thin, 4-point bar
      shows below each strip that doesn't fit, within a second, without
      restarting Bench (`benchCollectScrollBars` at start and on
      `NSPreferredScrollerStyleDidChangeNotification`); set it to
      **Automatically based on mouse or trackpad** with only a mouse, then
      plug in a trackpad, and the bars follow. With the bars shown, on
      macOS 14 (WebKit before Safari 18.2) and on macOS 15.2 or later: the
      bars are thin, not the system's full-height ones, the title bar and
      pane headers keep their heights, the docked windows don't move as the
      setting changes, and the fades don't cover the bars. Pressing and
      dragging the tab strip's bar scrolls it and doesn't move the window;
      the title bar's empty space beside it still does. **Never** hides
      them even with a mouse.
- [ ] `internal/mac` builds with the scroller observer
      (`screens_darwin.m`, `observe_darwin.m`, `exports_darwin.go`), and
      quitting Bench removes the observer without a crash.
- [ ] The compact density's title bar, on a 13- or 14-inch MacBook with
      **Density** on **Automatic** (and `compact_below` above the screen's
      width): open Bench. The native title bar is the unified compact
      toolbar's band (`MacToolbarStyleUnifiedCompact`), and the traffic
      lights sit centred in Bench's 39-point title bar (38 points and its
      line), level with the tabs, not above or below them. Note the band's
      real height: if it isn't about 38 points, change
      `--tab-height` and `--title-pad` for `data-title-bar="compact"` in
      `styles.css` to match. Check it on macOS 14, 15, and 26, whose
      toolbars are taller. On an external monitor, or with **Standard**, it
      opens with the 53-point title bar, and the traffic lights centred
      there.
- [ ] The title bar keeps its launch density: with Bench open in the
      compact density, choose **Standard** in Settings, which says "The
      title bar changes height when Bench next opens" (**Compact**, and
      **Automatic** on a laptop's screen, don't). The pane headers
      grow to 40 points at once, the docked windows move down to match, and
      the title bar and traffic lights stay as they were. Quit and open
      Bench: the title bar is the standard one. Unplugging a monitor that
      Bench opened on doesn't change it either.
- [ ] In the compact title bar, clicks on the tabs, the sub-tabs, and the
      buttons still act while another app is active (decision 12's
      pass-through), the free space still moves and zooms the window, and
      the traffic lights still close, minimise, and zoom it.
- [ ] Compact pane headers: with docked windows in two panes, each pane's
      window starts just below its 32-point header, with no gap and no
      overlap, and the 24-point **+**, split, and **⋯** buttons are easy to
      hit with the trackpad. With **Show scroll bars** on **Always**, a
      pane with more sub-tabs than fit shows a 3-point bar below them, and
      nothing moves.
- [ ] The "tabs" view on a laptop (Comms in the example config): the
      pane's sub-tabs, **+**, and **⋯** are in the title bar after the
      tabs, and the docked window reaches up to the title bar's line, with
      no header and no gap above it. Click a sub-tab to show its window,
      drag one onto another tab, and right-click one, and the empty space
      beside them, for the window's menu, which opens above the docked
      window. At 900 by 600 points, the tabs and the sub-tabs each scroll on
      their own, and nothing wraps onto a second row.
- [ ] **Hide the sub-tab row when a pane holds one app window**, with
      real apps: tick it in Settings, with one Obsidian (or Zed) window in
      a pane and three Brave windows in another. Obsidian's pane loses its
      header, and its window moves up to just inside the pane's border,
      with its own title bar where the header was; Brave's keeps its
      header, and so do a terminal's and a web page's. Drag another window
      into Obsidian's pane: the header comes back at once. Take it out:
      the header goes about 2 seconds later. Make Obsidian's pane too small
      for the window: the header comes back after about a second, says
      **Needs more room**, and stays, without the window jumping up and
      down. An app that resizes its own window just after docking doesn't
      make the header flicker. Jump to…'s **Add a window or app to this
      pane…**, its **This window** rows, such as **Show side by side
      (split right)**, and **Window options…** work on the headerless
      pane; the last opens the window's menu at the pane's top right, over
      Obsidian's title bar, without the window flickering.
- [ ] Undocking by accident, with the sub-tab row hidden: drag the app's
      own title bar a little, as when clicking it to focus the app. Note
      whether a small drag undocks the window (its pane says **Moved out of
      its pane**), and how easily: the app's title bar is now where the
      user reaches for Bench's header.
- [ ] Apps without a title bar, or with their toolbar hidden (**View ▸
      Hide Toolbar** in Finder, Mail, and Preview): they dock as before.
      A window with no title bar can't be dragged out of its pane, so it
      can't be undocked that way; **Pop out of Bench** in its menu still
      frees it.
- [ ] Moving and resizing dialogs (decision 34): open Settings, then drag
      its title. The dialog moves, not Bench's window, and the pointer
      shows a move cursor over the header and a resize cursor over the
      grip. Drag it into the top-left corner: it stops below or beside the
      traffic lights, which still close, minimise, and zoom the window.
      Double-click the header: the dialog goes back to the middle, and the
      window doesn't zoom. Drag the grip to make it larger and smaller. Do
      the same with Jump to… (by its search row, away from the field, and
      by its foot), the add dialog, and Tab properties, whose layout
      preview grows. Drag Bench's window smaller by its corner with a
      dialog moved to the bottom right: the dialog stays inside. Drag the
      window larger again: the dialog goes back where it was, at the size
      you gave it. A click on a dialog's header that wobbles a little
      doesn't move it. Start a drag while another app is active: the first
      press moves the dialog.
      Quit Bench and open it again: each dialog opens in the middle at the
      size you gave it. Escape still closes each dialog, and the keyboard
      goes back where it was.
- [ ] Asking before quitting while terminals are busy (decision 35). Run
      `claude`, or `sleep 600`, in a terminal in one tab, and give another
      tab a terminal with `command = "npm run dev"` (or `top`), so that two
      are busy, and a third terminal at its prompt. Quit from each of
      **Bench ▸ Quit Bench**, Command-Q, the menu bar item's **Quit Bench**,
      and the Dock icon's **Quit**, with Bench's window in front, behind
      another app, minimised, and closed: each time Bench comes forward,
      raised above its docked windows, with "2 terminals are still running:
      …", naming each by its sub-tab title and tab, and stays running.
      **Cancel**, Escape, and a click outside the question keep it running;
      **Quit** quits at once, without asking again, and puts every window
      back. With Settings open and changed, quit: **Cancel** brings Settings
      back with the change. With only the prompt left (exit the others, or
      wait for the command to finish), Command-Q quits without asking, at
      once. `osascript -e 'quit app "Bench"'` asks while one is busy, and
      quits at once otherwise.
- [ ] **Don't ask again**: tick it and **Quit**. Bench quits; open it
      again: `config.toml` has `confirm_quit_busy_terminals = false`,
      Settings shows **Ask before quitting while a terminal is running
      something** unticked, and quitting with a busy terminal doesn't ask.
      Tick it in Settings and save: Bench asks again.
- [ ] Log out, restart, and shut down with a terminal busy: none of them
      waits for Bench or is cancelled by it, and Bench puts every window
      back. Choose Log Out and cancel it from another app's prompt, then
      quit Bench within 5 minutes: it doesn't ask (`LogOutGrace`); after 5
      minutes it asks again. Run `BENCH_DEBUG=1` from a terminal and check
      the log's `LoggingOut` lines during a log out.
- [ ] Signals: start Bench from Terminal.app with a terminal busy inside
      it, and press Control-C in Terminal.app; then start it again, run
      `kill <pid>` from another terminal; then again, and close Terminal.app's
      window. Each time Bench quits without asking and puts every window
      back. Press Control-C twice quickly: Bench still puts every window
      back. Started with `nohup`, closing Terminal.app's window leaves Bench
      running.
- [ ] A popped-out terminal running `top`: close its window with its close
      button. The window stays open and asks "“top” is
      still running in this terminal. Close the window and end it?". Escape
      and **Cancel** keep it, with the keyboard back in the terminal;
      **Close** ends it and closes the window; with **Don't ask again**
      ticked, the main window's toast says that Bench won't ask again. A
      popped-out terminal at its prompt closes at once. Popping it back into
      a tab, and quitting Bench after answering **Quit**, close its window
      without asking. Check that the cancelled close leaves the window
      usable: it moves, resizes, and closes again.
- [ ] Closing an App window (decision 36). Dock TextEdit with unsaved
      changes, Zed, and Finder in the current tab. Click the × on Zed's
      sub-tab: the window closes and its sub-tab goes, with no toast. Click
      the × on TextEdit's: Bench lowers first, the save sheet shows on
      TextEdit's window in its pane, and about 1.5 seconds later the toast
      says "TextEdit may be asking about unsaved changes." with TextEdit
      focused. **Delete** closes it and the sub-tab goes; **Cancel** leaves
      it in its pane. Do the same from the ⋯ menu's **Close window**, the
      jump palette's **Close window**, and **Pane ▸ Close Window**: the
      press comes only after the menu or palette has closed.
- [ ] `PressClose` builds and links: `internal/mac` compiles
      `benchPressClose`, `benchFindCloseWindowItem`, and
      `benchPressCloseItem`, and `go test ./internal/mac/` passes on the
      Mac (`closeWindowRank` and the no-close-button status code are tested
      everywhere; the native calls only by these checks).
- [ ] A window without a close button: find one with Accessibility
      Inspector (some Electron apps' frameless windows, or a utility
      panel), dock it, and close it from Bench. Bench focuses it and
      chooses its app's **File ▸ Close Window** (or **Close**), never
      **Close Tab** or **Close All**. An app whose menus are in another
      language says "Bench couldn't close … Close it in …".
- [ ] Apps that hide their window on close: Slack's desktop app and Music.
      Close their windows from Bench: about 1.5 seconds later the sub-tab
      goes, the app keeps running, and no toast says it may be asking. Open
      the window again from the Dock: it comes back as a new window (under
      **Not in a tab**, or where a rule sends it), and Bench doesn't bring
      back the closed sub-tab. Then, with **Hide windows that belong to
      other tabs** on, give Slack a second window in the current tab, and
      close another tab that holds Slack's main window, which Bench has
      parked: open that window again from the Dock, and check that it's on
      screen, not in the screen's corner. If it is in the corner, quitting
      Bench, or `bench restore`, puts it back on screen.
- [ ] An app that quits with its last window, such as System Settings or
      Calculator: closing its window from Bench removes the sub-tab, the
      pane doesn't wait for the app ("isn't running"), and the next tab
      switch doesn't open it again for that sub-tab.
- [ ] **Close tab…** on a tab that isn't current, with **Hide windows that
      belong to other tabs** on, and again off: give the tab TextEdit with
      unsaved changes, Zed, a web page, and a terminal at its prompt. The
      question says "This closes 2 windows, 1 page, and 1 terminal."; after
      **Close tab**, the tab goes at once, TextEdit's app is shown (if
      hidden) and its window comes back on screen with its save sheet,
      under **Not in a tab**, with the toast naming TextEdit and Jump to….
      Windows of the same apps in other tabs stay parked or hidden. Zed's
      window closes. `config.toml` loses the tab and any rule naming it.
- [ ] **Close tab…** with a window that's in another tab too: it stays
      there, and the toast says so. **Close all windows…** on the only tab
      leaves it, with one empty pane; **Reset layout** brings back what
      `config.toml` lists.
- [ ] **Don't ask again** in Close tab's question: `config.toml` has
      `confirm_close = false`, Settings shows **Ask before closing a tab's
      windows** unticked, and closing a tab with two windows doesn't ask.
- [ ] An app with an app-modal alert, or one that's hung (`kill -STOP`
      it): closing its window returns within about a second, with the toast
      "Bench couldn't close … Close it in …", and Bench stays responsive.
      A window on another desktop: the toast says it's on another desktop.
- [ ] Without Accessibility permission: the ⋯ menu's **Close window** for
      an App item is greyed, with "Closing an app's window needs
      Accessibility permission."; web pages and terminals still close.
- [ ] The × in the app's WKWebView: the current tab shows its × in place of
      its ⌃⌥ hint, and other tabs and sub-tabs show theirs on hover,
      including while another app is active. On a 13 or 14 inch MacBook's
      screen, where tabs show no hint, the × is after the tab's name, in a
      slot of its own, and covers none of it. The title fades under the ×
      and no tab or sub-tab changes width. Clicking the × while another app
      is active closes at once (first mouse). Pressing the × and dragging
      doesn't drag the sub-tab, and hovering it doesn't start a live
      preview.
- [ ] Middle-click with a mouse that has a middle button: on a tab and on a
      sub-tab, it closes once (not twice, though WebKit may send both
      `mouseup` and `auxclick`), and does nothing while a menu or dialog is
      open. On a window in two tabs, the × and a middle-click take it out
      of this tab only.
- [ ] A busy terminal's sub-tab (`top`): its × asks "Close terminal?" in
      Bench's window; **Close** ends it.
- [ ] Double-click the × of TextEdit's sub-tab, with unsaved changes:
      TextEdit's save sheet shows once, Bench asks it only once (no "Bench
      couldn't close TextEdit" toast), and about 1.5 seconds later Bench
      shows the window. Choose **Don't Save**: the sub-tab goes, and the
      pane shows its next window, focused.
- [ ] The Tab menu: **Close Tab…** reads **Close All Windows…** with one
      tab, and **Close Tab…** again once there are two. Neither has a
      shortcut, and Command-W in a docked App window closes that window as
      the app does, and its sub-tab goes.
- [ ] **Undo** on a toast, over the status bar and, with the status bar
      hidden, in the title bar: after **Delete tab (keep windows)**, the
      toast shows **Undo** for 10 seconds; clicking it while another app is
      active works with the first click; pressing it and dragging doesn't
      move Bench's window, while the toast's text beside it still does. The
      tab comes back where it was, its windows dock in their panes (and are
      parked or hidden again if it isn't the current tab), and `config.toml`
      has its entry and rules again. Clicking **Undo** twice quickly undoes
      once.
- [ ] **Undo** after **Close tab…** with TextEdit asking about unsaved
      changes: the tab comes back with TextEdit's window in its pane,
      docked; Zed's window, the page, and the terminal that closed stay
      closed; TextEdit's save sheet can still be answered.
- [ ] **Undo** after dragging a sub-tab to another pane or tab, **Reset
      layout**, and **Close this pane**: the windows go back to their old
      panes and frames, and no window is left off screen.
- [ ] **Reopen closed tab** after quitting and reopening Bench: the tab's
      menu and the jump palette offer the deleted tab; reopening it docks
      its windows that are under **Not in a tab**, opens its pages and
      terminals again, and saves `config.toml`.
- [ ] Putting sub-tabs in order (decision 38): drag Zed's sub-tab along
      its pane's header, past VS Code's. A line shows between sub-tabs, no
      half of the pane lights up, and on release the two swap at once,
      with Zed's window still docked and shown; nothing splits however near
      the header's top you let go. Drag a sub-tab into the top quarter of
      another pane's body, just under its header: that still splits. Drag
      one onto another pane's header: it joins that pane at the line.
      Release quickly after a short drag, and again with a trackpad
      (tap to click on and off): the drop lands each time.
- [ ] The same in the "tabs" view, on the laptop's screen: drag a sub-tab
      along the title bar's strip, past the others, near the traffic
      lights, and near the strip's end so that it scrolls. The drag never
      moves Bench's window, and the order holds after **View ▸ Layout ▸
      Wide** and back. On Comms (Slack beside Outlook), drag Outlook
      before Slack: the toast says Outlook's old pane is empty in the wide
      layout, and in **View ▸ Layout ▸ Wide** Comms still has its two panes,
      at their sizes, with the right one empty.
- [ ] Dragging tabs: drag a tab along the title bar past two others. A
      thicker line with dots shows between tabs, Bench's window doesn't
      move, and on release the tab moves, keeps its windows docked, takes
      the shortcut of its new place (Control+Option+number), and
      `config.toml` lists the tabs in the new order, with the old file as
      `config.toml.bak`. Pressing a tab's × and dragging doesn't drag the
      tab or close it; pressing the title bar's empty space and dragging
      moves the window, as before. **Undo** in the toast puts the tab back,
      in `config.toml` too. Quit and reopen Bench: the order and each tab's
      arrangement are kept, with no notice about the arrangement.
- [ ] **Tab ▸ Move Tab Left** and **Move Tab Right**: disabled on the
      first and last tab, and both disabled with one tab; they follow as
      you switch tabs and as tabs are added, deleted, or moved; choosing
      one moves the current tab and Bench's menu bar item lists the tabs in
      the new order.
- [ ] Tab properties' **Position**: the list opens as a native pop-up menu
      over the sheet, with each place's shortcut, and choosing **1st** then
      **Save** moves the tab first. The pop-up works with the keyboard
      (Space opens it, arrows choose, Return closes it).
- [ ] Renaming a tab in place (decision 39): double-click the current tab.
      Its name becomes a field with the name selected, and Bench's window
      doesn't zoom or move; double-click the title bar's empty space and
      the window still zooms. Type a name and press Return: the tab keeps
      its windows docked and its layout, and `config.toml` has the new
      name, with the old file as `config.toml.bak`. Type the name of
      another tab: **→ Code (1)** shows after the field, and VoiceOver
      reads the sentence. Escape, a click in a docked app's window, and
      Command-Tab to another app each close the field without saving.
- [ ] The rename field keeps the keyboard after a double-click on a tab
      that isn't current and whose focused pane shows an app's window (Zed,
      say) or a web page, and after one on the current tab when that pane
      shows an app's window, a terminal, or a web page: typing goes into
      the field, not into the app, the terminal, or the page, and the field
      doesn't close by itself. With **Rename…** in a tab's menu too. A
      single click on the current tab still gives its window the keyboard,
      after a moment.
- [ ] Renaming a terminal (decision 39): **Rename…** in its sub-tab's
      menu, type a name, Return. The sub-tab keeps the name while the shell
      changes its title (`cd` somewhere, run `vim`); its tooltip has
      **Automatic title** with the shell's title. Quit and reopen Bench:
      the name is still there. **Use automatic title** brings back the
      shell's title. The same for a web page, as it navigates.
- [ ] Renaming a web page: with the page focused and its text field
      clicked, choose **Rename…** from its sub-tab's right-click menu, and
      again from the pane's **⋯** menu. Each time, typed keys go into
      Bench's field, not into the page, and Return saves the name.
- [ ] Dropping a sub-tab on the tabs' **+**: while you drag Zed's sub-tab,
      the **+** has a dashed outline; over it, and just to its right, it's
      tinted, and the drop makes a tab called **Zed** with Zed's window
      docked in its one pane, switches to it, and saves `config.toml`.
      **Undo** in the toast takes the tab away, from `config.toml` too,
      and docks Zed's window back in its pane. Again with Option held: the
      ghost shows a **+**, and the window is in both tabs, as **Zed**. Drop
      a sub-tab next to the controls at the title bar's right end: nothing
      is tinted, and nothing happens. The drag never moves Bench's window,
      and pressing the empty space after the **+** and dragging still
      moves it. A tab dragged over the **+** shows the line at the end of
      the tabs and goes last.
- [ ] **Move to new tab** and **Also show in new tab** in a window's menu
      and the jump palette: the same, with the window docked straight away
      in the new tab and nothing left behind in the old tab's pane.
- [ ] **Zoom pane.** On the default config's Code tab, focus Terminal's
      pane and press Control+Option+Return, from Bench and from another
      app: the pane fills the tab with its own sub-tabs and a **Zoomed**
      chip, and Zed's and VS Code's windows go out of sight, with no
      flicker of them over the zoomed pane. The status bar says **One pane
      zoomed**. **Pane ▸ Zoom Pane** reads **Show All Panes** while it
      lasts, and back again after, with the shortcut in its title, not as
      a key equivalent.
      Control+Option+Return again, the chip, the status bar's note, and
      **Pane ▸ Show All Panes** each bring Zed's and VS Code's windows
      back in their panes, at their sizes. Switching to another tab and
      back, Control+Option+Left, and **Move to pane** in the ⋯ menu each
      end the zoom too, with the windows where they belong. With
      Control+Option+Return used by another app, the status bar says so.
      Quit Bench while a pane is zoomed: it opens again with every pane.
- [ ] **Zoom pane** with **Hide the sub-tab row when a pane holds one app
      window** on: zoom a pane that holds one App window, Zed's. Its header
      appears, with the **Zoomed** chip, and Zed's window docks below it,
      with no flicker. After **Show all panes**, the header goes again and
      the window moves back up to the pane's top.
- [ ] **Equal sizes.** Double-clicking the gap between two panes gives them
      the same size, with **Undo** in the message, and docked windows
      follow. The double-click doesn't move the gap first, and doesn't zoom
      Bench's window. Pressing Return with the gap selected does the same.
- [ ] **Split right with an empty pane**, in a docked App window's ⋯ menu,
      adds an empty pane and opens the add dialog for it, over the docked
      windows. Escape takes the pane away, and the other windows go back
      to their sizes. Choosing an app there that isn't running opens it,
      and its new window docks in the new pane, which stays.
- [ ] `go vet ./...` and `go build ./cmd/bench` on macOS: the console
      emitter of `bench arrange` in `cmd/bench/app_darwin.go` builds with
      `ActionToast`, and `bench arrange` still prints its toasts.
- [ ] **Launchers** (decision 41). `go vet ./...` and `go build ./cmd/bench`
      on macOS: `folderChooser` in `internal/ui/native_darwin.go` builds
      against Wails' `OpenFileDialogStruct` (`CanChooseFiles`,
      `CanChooseDirectories`, `CanCreateDirectories`, `ResolvesAliases`,
      `SetMessage`, `SetButtonText`, `SetDirectory`, `AttachToWindow`, and
      `PromptForSingleSelection`), and `tty_darwin.go` builds with
      `unix.TIOCGETA`.
- [ ] With a `~/.zshrc` that puts nvm's `node` on the `PATH` (and with
      oh-my-zsh and Powerlevel10k's instant prompt, if you can), open a
      pane's **+**, type `node --version`, press Return twice: the
      terminal prints the version, and its prompt comes back in the folder.
      The command shows once, after the prompt, not before it, and isn't
      read by the start-up files. The same with `/bin/bash` and `fish` as
      the login shell. A config item with `command = "node --version"`
      still runs through `$SHELL -l -c`, as before.
- [ ] Add `sleep 4` to the end of `~/.zshrc` (and, with oh-my-zsh, set it
      to ask before updating): a launcher's command waits for the prompt,
      shows once after it, and isn't taken as the answer to the update
      question. Run Bench's binary from Terminal with `SHELL=/bin/sh`: a
      launcher's command is typed after a second.
- [ ] **New terminal…** with `claude --continue`, the name **Claude**, and
      **Choose…** for the folder: the folder picker opens as a sheet on
      Bench's window, over the docked windows, with Bench still raised;
      it starts in the folder typed, offers **New Folder**, and an alias
      to a folder resolves to the folder. **Choose** fills the field with
      the folder, `~` for the home folder; **Cancel** leaves it. Clicking
      **Choose…** twice shows one picker. Again in full screen.
- [ ] With the picker open, press Control+Option+2 to switch tabs: the
      add dialog closes, but Bench stays raised with the sheet over it
      until the picker closes, and then lowers; the folder chosen goes
      nowhere. Bench's window never hides behind docked windows with the
      sheet attached.
- [ ] **Save as launcher**: the launcher is in `config.toml` (keeping
      `config.toml.bak`), first in the add dialog, and after the tabs and
      windows in Jump to…: with a tab called Code and the launcher in
      `~/code/bench`, typing `code` in Jump to… and pressing Return
      switches to the tab, and typing `claude` finds the launcher. Each
      use adds a new terminal running `claude --continue` in the
      folder. Quitting Bench while it runs asks first, naming it. Reopen
      Bench: the terminals start again and run their command; one added
      with **Start again when Bench opens** unticked shows the command
      typed at its prompt, not run. Rename the folder and reopen: the
      terminal starts in `~`, says so, and doesn't run the command.
- [ ] An app launcher: add to `config.toml`
      `[[launcher]] name = "Bench in Zed", app = "dev.zed.Zed", folder = "~/code/bench"`
      (on separate lines) and reload. Choosing it with Zed not running
      opens Zed on the folder and docks the window titled after the
      folder; with that window already open, it's added at once. The same
      for a launcher with `url` and `title` in Brave. With an app that
      isn't installed, Bench says so and leaves nothing in the pane.
- [ ] **Settings ▸ Launchers**: edit a launcher's command and folder
      (**Choose…** opens the sheet over Settings), remove another, add
      **New terminal launcher**, and **Save**: `config.toml` has the
      changes. **Cancel** leaves the file alone.
- [ ] **Browse…** (decision 41). `go vet ./...` and `go build ./cmd/bench`
      on macOS: `benchChooseToAdd` in `internal/mac/apps_darwin.m` builds,
      with `LSRegisterURL` from CoreServices (a deprecation warning is
      fine, an error isn't), and `benchChoseToAdd` exports from
      `exports_darwin.go`.
- [ ] Open a pane's **+** with Zed and VS Code docked, and choose
      **Browse…**: the dialog closes, Bench lowers, and the open panel
      opens in front of the docked windows, as a window of its own, not a
      sheet, in `/Applications`, with **Add** as its button. Click a
      docked window, then **Browse…** again from another pane's **+**: the
      first panel comes back to the front, and Bench says that it's open
      already. Again in full screen.
- [ ] Choose an app in a folder inside `/Applications`, such as one in
      `/Applications/Setapp`: it's added to the pane. It was listed under
      **Apps** before, too. Choose an app outside the scanned folders, such
      as one in `~/Downloads` or on another disk, that has never been
      opened: it opens and docks, and stays under **Apps** until Bench
      quits. The next **Browse…** starts in its folder. An alias to an app
      adds the app.
- [ ] With the panel open, press Control+Option+2: choosing an app then
      adds nothing, and says why. Choosing a program reopens nothing, and
      says so.
- [ ] Choose `claude` (or `node`) in nvm's or Homebrew's `bin` folder
      (Command-Shift-G to go there): the add dialog opens again at **New
      terminal…** with the command `claude`, the name `claude`, and no
      folder. A program that isn't on your `PATH`, in a folder with a space
      in its name, gives its path, quoted, starting in its folder, and
      runs. A `.sh` script without the execute bit runs with `sh`, and a
      `.py` one with its `#!/usr/bin/env python3` line. A text file is
      refused. **Cancel** opens the add dialog again. The lookup doesn't
      leave a shell running (`ps`), even with a slow `~/.zshrc`. Start
      Bench from Terminal with `go run ./cmd/bench`, and **Browse…** for
      `claude` again: it's still found by its name, Terminal stays usable,
      and nothing from your start-up files is written to it.
- [ ] **Terminals** (decision 42). In a terminal in Bench's window and in a
      popped-out terminal, with Zed docked: ⌘F opens the find bar, and the
      menu bar's **Edit** menu doesn't flash; typing marks every match and
      counts them; Return, ⇧Return, ⌘G, and ⇧⌘G move between them; Escape
      closes the bar and the terminal has the keyboard again. ⌘+, ⌘−, and
      ⌘0 change only that terminal's text, and `tput cols` changes with it;
      ⌘K clears it; ⌘A selects everything, and the menu bar's **Select
      All** doesn't take it. With the add dialog's search field focused,
      ⌘A still selects its text.
- [ ] `echo secret-text`, then ⌘K: pop the terminal out and put it back,
      and `secret-text` doesn't come back in either view. Quit Bench and
      open it again: the restored output doesn't have it either. In zsh,
      bracketed paste still works after the pop-out (paste two lines: they
      don't run until Return).
- [ ] Right-click a terminal: **Copy** copies the selection to the Mac's
      clipboard (paste it into Zed), **Paste** pastes what Zed copied, with
      no Paste prompt from macOS, and **Find…**, **Clear**, and **Select
      all** work. The same in a popped-out terminal's window. Right-clicking
      the find bar's field shows the usual text menu.
- [ ] `echo https://example.com/a` in a terminal: holding the pointer over
      the address underlines it and shows the tooltip with the host in
      bold; a plain click doesn't open it, and a Command-click opens it in
      the default browser, as an OSC 8 link does.
- [ ] **Settings ▸ Terminals**: a text size of 16 applies to every terminal
      at once, popped-out ones too, and they fit their panes again. Tick
      **Use Option as Meta key**: Option-B and Option-F move by words in
      zsh; untick it, and Option-3 types # on a British keyboard.
- [ ] **A terminal that needs you** (decision 43), in `Bench.app`, as
      notifications need an app bundle. In the Code tab's terminal, run
      `sleep 5; printf '\a'` and switch to Comms: the terminal's sub-tab
      gets the blue bell after its title, the Code tab a bell after its
      name, and Bench's Dock icon the badge 1. The first time, macOS asks
      whether Bench may post notifications; allow it. A notification
      titled with the terminal's title, with **Code** under it, says "Rang
      the bell.". Ring it again: no second notification while it's marked.
      Show the terminal: the bells and the badge go.
- [ ] `sleep 5; printf '\e]9;Build done\a'` and then
      `sleep 5; printf '\e]777;notify;Claude Code;Needs your permission\a'`,
      each while another tab is shown: each posts a notification, with its
      text, and the sub-tab's tooltip says why. Two within 5 seconds post
      one. `printf '\e]9;4;1;50\a'` (a progress report) posts nothing.
      Claude Code, with
      `claude config set --global preferredNotifChannel terminal_bell`,
      and Codex, with `notifications = true` under `[tui]` in
      `~/.codex/config.toml`, each in a terminal out of sight, post one
      when they finish or need an answer. Check the user guide's steps for
      turning them on still match both tools.
- [ ] Clicking a terminal's notification shows the terminal: with Bench in
      the background, with Bench hidden (⌘H), with Bench's window
      minimised, and for a popped-out terminal, whose window comes
      forward. Clicking a Web item's notification does the same for it.
      A terminal shown in Bench's window while another app is in front
      gets a bell too, which goes when Bench comes forward. When Bench
      opens, it counts as active: a bell in the shown terminal straight
      after launch marks nothing.
- [ ] With Zed docked beside a terminal in the current tab, click into
      Zed and type, then ring the terminal's bell (`sleep 5; printf '\a'`):
      no bell, no badge, and no notification. Click an app that isn't in
      the tab, such as Finder, and ring it again: the bell and the
      notification come; click Zed's window: the bell goes.
- [ ] Untick **Notify me when a terminal in another tab needs attention**:
      the bells and the badge still come, but no notification.
- [ ] **A terminal's folder** (decision 44). `cd ~/code/bench` in a
      terminal in `~/code`: its **⋯** menu has **New terminal in
      ~/code/bench**, which opens one there, and so does the add dialog's
      first **Built in** row. **New terminal in…** opens the folder picker
      as a sheet, starting there, and opens a terminal in the folder
      chosen; **Cancel** leaves the dialog at its list.
- [ ] **Output between launches.** In that terminal, `ls --color` (or
      `ls -G`), then quit Bench and open it again: the terminal shows the
      listing faint, with its colours, then "Restored output from *time*.
      The program that wrote it has ended.", then a new prompt in
      `~/code/bench`. `~/Library/Application Support/Bench/terms/` holds
      one `.out` file per terminal, with `-rw-------`. Quit while `vim` is
      open: vim's screen isn't shown again, and the terminal doesn't
      switch to the alternate screen. Run `printf '\e[6n'` before quitting:
      nothing appears at the new prompt after the restart. Close the
      terminal: its file goes. Untick **Keep terminal output between
      launches**: the folder goes, and the next start shows no old output.
- [ ] Settings ▸ Keyboard builds and links: `make app` links Carbon
      (`-framework Carbon` in `internal/mac/platform_darwin.go`), for
      `TISCreateInputSourceList` and the `kVK_` codes in `keys_darwin.m`.
      With `BENCH_DEBUG` set, the log has a `SystemKeys` call at startup,
      and Settings ▸ Keyboard lists all nine shortcuts, each "Works.".
- [ ] Input sources: with two input sources and **Select next source in
      Input menu** on (System Settings ▸ Keyboard ▸ Keyboard Shortcuts ▸
      Input Sources), the status bar says "macOS uses ⌃⌥Space to switch
      input sources, so Bench won't see it. Try ⌃⌥J instead." (or another
      suggestion), with **Use this instead** and **Keyboard settings**, and
      Settings ▸ Keyboard's **Jump to…** row says the same, with **Use**.
      Note which gets ⌃⌥Space, Bench or the input sources. Change that
      shortcut in System Settings to ⌃⌥⌘Space and make Bench active again:
      the notice goes, since Bench reads `com.apple.symbolichotkeys` again
      as it becomes active. Turn it back on, then remove the second input
      source: the notice goes within a second, without making Bench
      active (the enabled input sources' notification). On a Mac where the
      user never changed these shortcuts (no entries 60 and 61 in
      `defaults read com.apple.symbolichotkeys AppleSymbolicHotKeys`),
      Bench takes them as ⌃Space and ⌃⌥Space, turned on.
- [ ] VoiceOver: with Bench open, turn VoiceOver on (⌘F5). Within a second
      the status bar says "VoiceOver is on, and it uses ⌃⌥ for its own
      commands, so Bench's shortcuts with ⌃⌥ may not work. Try ⌃⌘ in place
      of ⌃⌥ (and ⌃⌘J for Jump to…)." (the KVO observer of
      `voiceOverEnabled`), and every row of Settings ▸ Keyboard says so
      too. Note which gets ⌃⌥→, ⌃⌥Space, and ⌃⌥1, Bench or VoiceOver, and
      whether that changes with Caps Lock as the VO key. **Use this
      instead** changes every field to ⌃⌘, says "Changed the shortcuts",
      and ⌃⌘1 and ⌃⌘→ work at once. Turn VoiceOver off: the notice goes.
- [ ] Recording: in Settings ▸ Keyboard, click **Jump to…**'s keys and
      press ⌃⌥Space (Bench's own jump shortcut): the field shows ⌃⌥Space
      rather than the palette opening, since Bench releases its hot keys
      while it records. Record ⌃⌥J, click **Save**, then press ⌃⌥J from
      another app: the palette opens, and ⌃⌥Space no longer does, with no
      restart. Record **Focus a pane** by holding ⌃⌘ and letting go: it
      shows ⌃⌘ "with the arrow keys", and after **Save**, ⌃⌘→ focuses the
      pane to the right. Start recording, then press Escape, click
      elsewhere, switch to another app, or click **Cancel**: each stops
      recording, and Bench's shortcuts work again straight away. A
      shortcut that another app has registered shows "Bench couldn't
      register …" under the field as soon as you record it.
- [ ] A notice's **Keyboard settings** opens Settings scrolled to its
      **Keyboard** section, with the focus on the first shortcut with a
      problem, and **Use this instead** changes that shortcut, saves
      `config.toml`, and says what changed in a toast. Tab properties'
      **Shortcut** line names the tab's shortcut and says that Settings ▸
      Keyboard changes the keys.
- [ ] Settings' **Windows** and **Advanced** groups: **New windows go to**
      **The focused pane** puts a new Finder window (⌘N) in the focused
      pane, and **Leave floating** leaves it out of the tabs. **When another
      tab's window comes forward** **Bring it here** moves a window of
      another tab, clicked in the Dock, into the current tab's focused
      pane, and **Do nothing** stays on the tab. **Also hide apps that
      aren't in any tab** hides an app with no windows in a tab, such as
      Calculator, on a tab switch. **Wait for apps to open** at 30 seconds
      saves `launch_timeout = "30s"`. Settings still fits a 900 by 600
      window, scrolling in the middle.
- [ ] **The app menu** (decision 46). The menu bar reads Bench, Edit,
      View, Tab, Pane, Window, Help, each opening at once. **View ▸
      Layout** opens to **Auto**, **Wide**, and **Compact**, the mode in
      use ticked. The Tab menu ends with the tabs, the current one ticked,
      each with its shortcut in its title, such as "Code (⌃⌥1)"; **Next
      Tab**, **Previous Tab**, **Jump to…**, **Split Right**, **Zoom Pane**,
      and **Rearrange** show theirs the same way. None of Bench's own items
      has a key equivalent at the right: Command-W, Command-T, and
      Command-P in a docked VS Code for the Web reach the page, and ⌃⌥1
      switches tabs once, not twice. Change the jump shortcut in Settings ▸
      Keyboard: **Pane ▸ Jump to…**'s title follows on **Save**. While a
      field records keys, open the Tab menu and press ⌃⌥1: nothing
      switches.
- [ ] The Tab menu follows the tabs: add a tab, rename one in place,
      delete one, and move one, each with the Tab menu open in between and
      after: the list, the tick, and **Move Tab Left** and **Move Tab
      Right** (off at the ends) follow, and the menu bar doesn't flicker
      or hang. **Rename…** turns the current tab's name into a field with
      the keyboard, even from another app. **Delete Tab (Keep Windows)**
      is off with one tab. **Reopen Closed Tab** is off until a tab is
      deleted, then brings it back.
- [ ] The Pane menu follows the focused pane: focus a docked App window's
      pane, then an empty pane (**Split right with an empty pane**), then
      a pane in the `tabs` view (View ▸ Layout ▸ Compact). **Split Right**
      and **Split Down** put the window in a new pane beside its own, and
      are off for the empty pane and in the `tabs` view. **Zoom Pane**
      is off with one pane, and reads **Show All Panes** while one is
      zoomed. **Move to Tab ▸** lists every tab, the current one off, and
      **New Tab**; each moves the window. **Pop Out** takes the window out
      of its pane, and **Dock Again** puts it back; for a sub-tab with no
      window, **Pop Out** reads **Remove from Bench** and **Close Window**
      is off. **Close Pane** is on only for an empty pane of a tab with
      more panes, and closes it. **Add to Pane…** opens the add dialog for
      the focused pane.
- [ ] **Help**: the menu has macOS's search field at its top; typing
      "zoom" finds **Pane ▸ Zoom Pane**, and choosing the result zooms the
      pane. **Bench Help** and **Troubleshooting** open the user guide, at
      its Troubleshooting section for the second, in the default browser.
      **Keyboard Shortcuts** opens the Keyboard Shortcuts sheet, and
      **Show Welcome Guide** the welcome guide, each bringing Bench forward
      from another app. **Show Bench's Files in Finder** shows
      `config.toml`, selected. If there's no search field, note it:
      Wails doesn't set `NSApp.helpMenu`, and Bench relies on AppKit
      finding the menu called Help.
- [ ] **Report a Problem…** opens a new issue on GitHub in the default
      browser, with the body filled in: the headings, the Bench version (as
      `bin/bench version` prints it), "macOS 15.x (arm64)" or similar, each
      display with its size and which is main and which Bench is on, the
      layout, both permissions, the notices, and the log's path written from
      `~`. Nothing is sent until you submit it on GitHub. With 10 notices,
      it lists 8 and "And 2 more.", and GitHub still opens the page.
- [ ] **The log file** (decision 47). After `make app` and opening
      `Bench.app` from the Finder, `~/Library/Logs/Bench/bench.log` exists
      with `-rw-------`, in a folder with `drwx------`, and starts with a
      `msg="Bench started" version=…` line. Console ▸ **Log Reports** lists
      it. Make a notice (add a line that isn't TOML to `config.toml` and
      choose **Bench ▸ Reload config.toml**): the log gets
      `level=WARN msg=notice text=…` once, not again on each redraw.
      `chmod 644` the file and reopen Bench: it's `-rw-------` again. Quit,
      append 1 MB to the file
      (`head -c 1048576 /dev/zero | tr '\0' x >> …/bench.log`), and open
      Bench: the old file is now `bench.log.1`, and `bench.log` starts
      afresh. While Bench runs, open it again from the Finder: it comes
      forward, and the log gets no second `Bench started` line. Without
      `BENCH_DEBUG` the log has no `level=DEBUG` lines; with it
      (`open --env BENCH_DEBUG=1 bin/Bench.app`), it does.
- [ ] **Recent messages**: choose **View ▸ Rearrange** twice, then click
      the status bar's speech-bubble button: the list shows "Rearranged"
      with "2 times" and the time, latest first. **Copy** puts a message on
      the clipboard (paste it into TextEdit), and the button says
      **Copied**; the text can be selected with the mouse. Hide the status
      bar: **Help ▸ Recent Messages** opens the list below the title bar,
      above the docked windows, and Escape closes it.
- [ ] **Permissions and set-up** (decision 48). `make app` links the
      ServiceManagement framework (`-framework ServiceManagement` in
      `internal/mac/platform_darwin.go`), for `SMAppService`.
- [ ] Accessibility's deep link: reset Bench's permission
      (`tccutil reset Accessibility <bundle ID>`) and open `Bench.app`. The
      welcome guide's **Turn on Accessibility** shows macOS's prompt and
      opens System Settings ▸ Privacy & Security ▸ Accessibility, at its
      list. Click it again after dismissing the prompt: System Settings
      comes forward at the same list. Note if a version of macOS opens
      another pane. The status bar's **Turn on Accessibility** does the
      same.
- [ ] Screen Recording's deep link: with previews off and Bench not
      allowed, tick **Show live previews of windows** in Settings and save:
      macOS's prompt shows, and System Settings opens at Privacy & Security
      ▸ Screen Recording. Settings and the welcome guide now offer **Quit
      and Reopen Bench**. Turn Bench on in System Settings, then choose it:
      Bench quits and opens again within a few seconds, on the same tabs,
      and hovering a sub-tab shows a preview. With `sleep 60` running in a
      terminal, **Quit and Reopen Bench** first asks "Quit and reopen
      Bench, and end it?"; **Quit and Reopen** does. Run `bin/bench` (not
      in a bundle) and do it again: the binary opens again too.
- [ ] After an update: with Bench allowed, quit, replace `Bench.app` with
      a build of a newer version (`make app VERSION=…`, or one signed
      differently), and open it. If macOS stops applying the permission,
      the welcome guide and the status bar say "Bench was updated, so
      macOS needs you to allow it again: select Bench, click −, then turn
      it on again."; doing that in System Settings makes the notice go
      within a few seconds. `accessibility-version` in the support folder
      holds the new version once it has the permission. Open the older
      build without the permission: its notice is the usual one.
- [ ] Open at login that needs approval: with Bench's login item turned
      off in System Settings ▸ General ▸ Login Items (under **Allow in the
      Background**), tick **Open Bench when you log in** in the welcome
      guide. If macOS needs approval, Login Items opens, and the guide and
      Settings say "macOS needs you to allow Bench…", with **Open Login
      Items**. Turn Bench on there and switch back to Bench: the note goes.
      Log out and in: Bench opens. Note what `SMAppService` reports if the
      note never shows.
- [ ] **Bench ▸ Check for Updates…** opens
      `https://github.com/itbm/bench/releases/latest` in the default
      browser, with **Check for a new version once a day** off, and Bench
      makes no request of its own (Little Snitch or `nettop -p bench`
      shows none). Turn the setting on in a build older than the latest
      release (`make app VERSION=0.0.1`): within a few seconds the status
      bar says "Bench x.y.z is out. To update, download it and replace
      Bench in Applications.", with **Download**, which opens the
      release's page, and **Dismiss**, after which reopening Bench doesn't
      show it again. Quit and reopen: Bench
      doesn't ask GitHub again within the day (`state.json`'s `updates`
      has `checked`). Offline, the log has one warning and no notice.
- [ ] The welcome guide's set-up: move `~/Library/Application
      Support/Bench` aside and open `Bench.app`. Continue: **Choose a
      starting set-up** lists only tabs of apps this Mac has, such as
      Comms with Slack and Mail, with their apps' icons. Remove one,
      then **Add 1 tab**: the tab is added after the first one, with its
      apps docked once they open, and in `config.toml`. The tips read
      "Step 3 of 3", and Back from them goes to the permissions, skipping
      the set-up. **Done**, then **Help ▸ Show Welcome Guide**: it has two
      pages. With a `config.toml` of two tabs and no `first-run-done`, the
      set-up's other button reads **Don't add tabs**.
- [ ] The Keyboard Shortcuts sheet: click the status bar's shortcut hints
      (VoiceOver reads them as a button): the sheet lists every shortcut
      as Settings ▸ Keyboard does, then a terminal's keys. Change one in
      Settings and open the sheet again: it shows the new keys. **Change
      Shortcuts…** opens Settings ▸ Keyboard. An empty pane (**Split right
      with an empty pane**) says "Drag a sub-tab here, or press ⌃⌥⇧ and an
      arrow to move the shown window here.", and pressing that moves the
      window into it.
- [ ] The README's screenshots and tour (`docs/images/`) show the title bar
      and status bar as they are now: the tabs' **+** right after the last
      tab, no dot or tab name in the status bar, 4 points between panes,
      and the × on the current tab and each pane's shown sub-tab.
      If they don't, run the [macOS demo](#the-macos-demo) (`[macos-demo]`
      in the head commit), and [refresh them](#refresh-the-readmes-screenshots)
      with `scripts/readme-media.sh` and its artifact. Look at `tour.gif`
      before you commit it.
- [ ] **Last used tab** (decision 49): switch from Code to Research, then
      press ⌃⌥&#96; (the key left of 1) from another app: Bench goes back to
      Code, and again to Research. On a keyboard with an ISO layout, such
      as a British one, note which key macOS reports as &#96; (it may be the
      key left of Z) and whether ⌃⌥&#96; still works there. Settings ▸
      Keyboard lists **Last used tab** after **Previous tab**, saying
      "Works.", with no notice from another app or macOS. **Tab ▸ Last
      Used Tab (⌃⌥&#96;)** does the same, and is dimmed when Bench has just
      started. Hide Bench (⌘H) and press ⌃⌥&#96;: Bench comes back on the
      other tab. With VoiceOver on, its suggestion is ⌃⌘&#96;.
- [ ] **Jump to…** (decision 49): with a docked Zed window that has the
      keyboard, press ⌃⌥Space and type "zed spec" or "spec zed": Zed's
      spec.md window is first. Press ⌃⌥Space and Return straight away:
      Bench goes back to the tab you were on. The **This window** group
      lists what Zed's ⋯ menu has, with the shortcuts after **Show side by
      side (split right)** and **Split down**; **Pop out of Bench**,
      **Move to tab ▸** another tab, **Rename…** for a terminal, and
      **Close window** each work from it, and Zed gets the keyboard back
      or closes as it does from the menu. Typing "termnal" still finds
      the terminals. With Zed still typing, press ⌃⌥Space, type "new
      terminal", and press Return: the add dialog opens at its **New
      terminal…** form for Zed's pane; **Add a web page…** opens it at
      the web page's address. In the add dialog, typing `code .` offers
      **Run “code .” in a new terminal…**, even with a VS Code window in
      another tab.

### Bring windows into Bench

Decision 13. Start with **Drag windows onto Bench to add them** on, Bench
filling the screen, and a current tab of two panes. Each drag is by the
window's own title bar, and "hold" means keeping the pointer still for
about half a second.

- [ ] **Safari**: open a new Safari window that isn't in a tab (**New
      windows go to: Leave floating**, or **Pop out of Bench**). Drag it over
      the left pane and hold: a guide shows over the pane, an amber tint over
      its body with a target in the middle, one near each edge, and "Add to
      this pane" below the middle. Move onto the right-hand target: the
      right half tints and the label reads "New pane on the right". Release:
      the window goes into a new pane on the right, and the toast says
      "Added Safari — … to this pane." with **Pop out**, which pops it out
      again.
- [ ] **Finder**: drop a Finder window in a pane's middle: it joins the
      pane as a sub-tab, shown and focused.
- [ ] **A window in the background**: with Bench active, drag another
      app's window that's behind Bench, so that pressing its title bar
      makes its app active, onto a pane and hold: the guide shows and stays
      as the app comes forward, and the drop docks the window. While the
      guide shows, ⌘Tab to a third app takes it away.
- [ ] **An empty pane**: split a pane with **Split Right** so the new pane
      is empty, then drag a floating window over it and hold near its left
      edge: the guide has only the middle target, the whole body tints, and
      the label reads "Add to this pane"; the drop fills the pane. Drag
      that window out of its pane and back to its own pane's edge: again
      only the middle, and it docks again filling the pane.
- [ ] **Chrome tab tear-off**: drag a tab out of a Chrome window and, still
      dragging, hold over a pane: note whether the guide shows. The press
      was on the old window, which doesn't move, so it's expected not to;
      the torn-off window then floats, and dragging it by its title bar onto
      a pane adds it.
- [ ] **VS Code** and **iTerm2** (with its title bar shown, then with it
      hidden): drag onto a pane and drop. The window docks, and VS Code
      doesn't resize itself out of its pane afterwards.
- [ ] **A Catalyst app**, such as Stocks: the same, and note any app that
      moves its window without the guide showing.
- [ ] **Edge resizes**: drag a floating window's edge, then its corner,
      across a pane and hold: no guide, and the release only resizes it.
- [ ] **No guide** for: a window dragged across a pane without stopping (it
      stays where it's dropped); a window that a `float = true` rule
      matches, such as a browser's picture-in-picture; a sheet or panel; a
      window in full screen, such as one of a Split View dragged by its
      title bar to swap sides; any window while Bench is hidden (⌘H), or
      while Settings or the jump palette is open.
      With the guide showing, moving off the pane takes it away, and
      pressing ⌘Tab with the other hand takes it away too. Holding still
      for five seconds with the guide showing keeps it.
- [ ] **Undocked windows**: drag a docked window out of its pane (amber
      sign), then onto the other pane, and hold and drop: "Moved … to this
      pane." with **Undo**, which puts it back. Drag it out again and drop
      it back on its own pane: "Docked … again.". A window shown in two
      tabs keeps its place in the other tab.
- [ ] **Bench's own windows**: pop out a web page. Its title bar has **Back
      to Bench** at the right, small, centred in the title bar; clicking it
      puts the page back in the focused pane and brings Bench forward. Pop
      it out again and drag it onto a pane and hold: the guide shows, and
      the page docks once you let go. Pop out a terminal: its window has a
      bar with **Back to Bench**, and its right-click menu has it too; both
      work, and so does dragging its window onto a pane. With either window
      in front, **Window ▸ Back to Bench** is enabled and works; with Bench's
      main window in front, it's dimmed.
- [ ] **Add front window**: give **Add front window to this pane** a
      shortcut in Settings ▸ Keyboard, such as ⌃⌥A. Click a floating Finder
      window and press it: Finder goes into the focused pane. Hide Bench,
      click another floating window, and press it: Bench comes back with
      the window in its pane. Click a docked window and press it: the toast
      says "There's no window to add…", and **Pane ▸ Add Front Window** is
      dimmed.
- [ ] **The guide panel**: it's above the dragged window and other apps'
      floating panels, takes no clicks (the release always reaches the
      window), and shows on the display Bench is on, with two displays. With
      Bench on another desktop (Space), drag a window over where Bench's
      panes would be and hold: no guide, and the release leaves the window
      where it is. Do the same with a popped-out page on that desktop: no
      guide, and Bench doesn't pull you over to its desktop.
- [ ] **No permission**: watching drags asks for nothing: no Input
      Monitoring prompt, and nothing new under System Settings ▸ Privacy &
      Security. With `BENCH_DEBUG=1`, the log shows `window-dragged` events
      with `start`, `frame`, and `pointer`, at most about 30 a second, and
      none while the setting is off.
- [ ] **Cost of watching**: in Activity Monitor, Bench's CPU stays near
      what it is at rest while you select text by dragging in TextEdit for
      ten seconds, and while you drag a slider in System Settings: Bench
      asks the window server at most 30 times a second while the button is
      held, however fast the mouse reports.

### Page controls, calls, and sound

Decision 50. Use the example config's Slack and Outlook pages, a page you
can make a call on, such as Google Meet or a Teams meeting in the web
(sign in with an account that allows it), and a page with a video, such as
one on YouTube. Run with `BENCH_DEBUG=1` to see the `web-state-changed`
events in the log.

- [ ] **Back and Forward**: in a fresh Slack page, the ⋯ menu's **Page**
      row has **Back** and **Forward** dimmed, and **Pane ▸ Page ▸ Back**
      too. Click into another channel: **Back** turns on within a moment
      (the log shows `canGoBack=true`), and goes back. **Forward** then
      turns on and goes forward. A two-finger swipe does the same, and the
      buttons follow it. Going back to a page on another site that the
      page had moved to stays in the item, as before (decision 22's rules).
- [ ] **Reload**: **Reload** loads the page again, keeping its place in the
      history. On a page whose process was ended (Activity Monitor ▸ force
      quit its "Web Content" process), it loads the page again too.
- [ ] **Zoom**: **Zoom in** twice makes Slack's text bigger at once, with
      "Zoomed Slack to 125%." **Actual size** reads **125%**. Switch tabs
      and back, pop the page out and back in, and quit and reopen Bench: it
      stays at 125% each time, and `state.json` has `"zoom": 1.25` for it.
      Outlook stays at 100%. **Zoom in** stops at 300% and **Zoom out** at
      50%, each then dimmed. The page's own ⌘+ and ⌘− still work as before,
      as nothing in Bench's menus takes them.
- [ ] **Find in page**: **Find in page…** opens the bar at the top of the
      pane, the page moving down 36 points below it with nothing of the page
      under the bar, and the keyboard in its field (typing doesn't reach
      Slack's message box). Type a word on the page and press Return: the
      page scrolls to it and shows it selected, while the keyboard stays in
      the field; Return again moves to the next, Shift-Return back, and it
      goes round at the end. A word not on the page says **No matches**.
      Escape closes the bar, the page moves back up, and typing reaches the
      page again. Check it with the pane focused, in the "tabs" view, in a
      zoomed pane, and while a menu was open as it was chosen from the jump
      palette (the bar's field gets the keyboard once the palette closes).
      **Pane ▸ Page ▸ Find in Page…** opens it too. A popped-out page says
      it has no find bar.
- [ ] **Copy page link**: **Copy page link** puts the page's current address
      (the channel's, after moving to one) on the clipboard, with "Copied the
      page's link."; **Pane ▸ Page ▸ Copy Page Link** too. VS Code for the
      Web has it dimmed in the menu bar and absent from its menu.
- [ ] **The camera and microphone in a call**: join a call in a page and
      allow the camera and microphone. The page's sub-tab shows a green
      camera and microphone before its title, and its tab, once you switch
      to another, a green camera after its name, with "A page here is using
      the camera and the microphone" as its tooltip. VoiceOver reads the
      sub-tab as "… (using the camera, using the microphone)". Mute the
      microphone: the tab's mark stays a green camera, and its tooltip
      reads "A page here is using the camera, with the microphone turned
      off". Then turn the camera off and unmute the microphone: the tab's
      mark is a green microphone, saying "A page here is using the
      microphone, with the camera turned off".
- [ ] **Mute and turn off**: from the sub-tab's menu, **Mute microphone**:
      the call's own mute button shows muted, the others can't hear you,
      and the mark turns grey and struck through within a moment. **Unmute
      microphone** turns it back on, as the call shows. **Turn off camera**
      and **Turn on camera** do the same for the camera; macOS's camera
      light goes off and on. The page can't turn either back on itself.
      Check them from **Pane ▸ Page** and the jump palette too.
- [ ] **The call ends**: leave the call: the marks go, from the tab too.
      Close a page that's in a call: the call ends and its marks go.
- [ ] **A call in a popup**: start a call in a window that a page opens,
      such as a Slack huddle opened in its own window, or a meeting link
      that opens with `window.open`. The page's sub-tab and tab show the
      marks, the menu has **Mute microphone** and **Turn off camera**, and
      each mutes the popup's call, as the call's own buttons show. Close
      the popup: the marks go.
- [ ] **Stop sound and video**: play a video, switch to another tab, and
      choose **Page ▸ Stop sound and video** for it from the jump palette's
      **This window** after switching back, or from **Pane ▸ Page**: it
      pauses, with "Paused YouTube's sound and video.", and plays again
      from its own button. In a call it pauses the call's sound; note what
      the call does. A page's popup that plays sound pauses too.
- [ ] **No key equivalents**: with a web page focused, ⌘R, ⌘F, ⌘L, ⌘+, ⌘−,
      and ⌘0 reach the page, as before; none of **Pane ▸ Page**'s items
      shows a key equivalent.

### App and page icons

Decision 51. Use the Code tab of the example config, with Zed and VS Code,
three Brave windows, a terminal, and Web items for Slack and Outlook.

- [ ] **Apps**: each app's sub-tab shows its icon, sharp at 2x, in place of
      its monogram, and the pane header is as tall as before. The three
      Brave windows' sub-tabs show Brave's icon. A terminal's shows **>_**.
      An item for an app that isn't installed shows its monogram.
- [ ] **No wait for the main thread**: with many apps in the add dialog,
      open it for the first time after launch, and scroll its **Apps**:
      Bench stays responsive while the icons come in, and each row shows
      its monogram until its icon has loaded.
- [ ] **Pages**: Slack's and Outlook's sub-tabs show the sites' icons a
      moment after their pages load. Move about Slack: the icon doesn't
      flicker or go back to the monogram. Add a page whose site
      has no icon (such as `http://neverssl.com`): it keeps its monogram.
      Add a page of a site whose icon needs a sign-in: it keeps its
      monogram too, and nothing asks for a password.
- [ ] **Changes**: go from one site to another in a Web item, by a link:
      its sub-tab shows the new site's icon once the page loads, or its
      monogram if the new site has none. A page that changes its icon by
      script, such as Gmail's unread count, keeps the one it had as it
      loaded, as Bench reads a page's icons once it has loaded. Pop a page
      out and back: its sub-tab keeps its icon.
      Close a page's item and add the site again: the icon comes back once
      the page loads.
- [ ] **Lists**: Jump to…, the add dialog (**Windows not in a tab**,
      **New window**, **Apps**, **Web apps**), an empty pane's list of apps
      it waits for, **Settings ▸ Launchers**, and the welcome guide's
      starting set-up show the same icons. A sub-tab dragged shows its icon
      in the drag's ghost.
- [ ] **Markers**: an app that's opening has the grey ring on its icon's
      corner, readable over the icon, in dark and light colours.
- [ ] **Network**: Little Snitch or `nettop -p Bench` shows Bench's own
      requests, for the pages' icons, going only to the addresses that
      the pages' icons name, their sites or CDNs (Slack's icons are on
      a.slack-edge.com), and to `/favicon.ico` on the pages' own sites,
      never to anything else, and no cookies in them (a proxy such as
      Proxyman shows none).

### Light and dark

Decision 52. Use a Web item whose site follows macOS's appearance, such as
github.com or developer.apple.com, and a terminal, popped out too. Switch
macOS between light and dark in **System Settings ▸ Appearance**.

- [ ] **Web pages follow macOS**: with **Colours** on **Dark** (the
      default) and macOS light, the site is light in Bench, as in Safari.
      Switch macOS to dark: the page turns dark within a moment, without a
      reload, and back again. Its popups (a sign-in window) and a
      popped-out page do the same. Bench's own window stays dark.
- [ ] **Light**: choose **Settings ▸ Appearance ▸ Colours ▸ Light** and
      **Save**. The main window, a popped-out terminal's window, and
      **Bench ▸ Acknowledgements** turn light at once, title bars and
      traffic lights included, with no dark edge while resizing a window
      quickly. Terminals take light colours, and text in them stays
      readable (`ls -G`, `git diff`). Web pages still follow macOS.
- [ ] **Match macOS**: choose **Match macOS** and **Save** with macOS dark:
      Bench is dark. Switch macOS to light: every Bench window turns light
      within a moment, and back. Open Acknowledgements and a popped-out
      terminal while macOS is light: each opens light, with no dark flash.
- [ ] **At launch**: with **Light**, quit and reopen Bench: the window opens
      light, with no dark flash before the UI draws, filling the screen or
      not. With **Dark** and macOS light, it opens dark.
- [ ] **Placeholders and menus**: in light colours, check an empty pane's
      hint, a missing app's and a launching app's placeholder, a web page's
      find bar, the jump palette, Settings, Tab properties, the tab menu,
      a toast, the notices, the drop guide while dragging a window over a
      pane (its targets and label light, the target the pointer is on in
      the darker amber), and a live preview's card under a sub-tab: nothing
      is dark, and nothing is hard to read. Switch to **Dark**: the guide
      and the preview card are dark again the next time they show.
- [ ] **Match macOS at launch**: with **Match macOS** and macOS dark, quit
      and reopen Bench: the window opens dark, with no light flash before
      the UI draws, nor while resizing it at once. With macOS light, it
      opens light.

### Stage Manager and tiling

Decision 53. On macOS 14 or later.

- [ ] **The notice**: turn on Stage Manager in Control Centre, and click
      Bench's window: the status bar says "Stage Manager is on, so Bench
      can't keep windows in their panes. Turn it off in Control Centre."
      Turn it off in Control Centre, and click Bench again: the notice
      goes. Quit Bench with Stage Manager on and open it again: the notice
      is there from the start. `BENCH_DEBUG=1` logs `StageManager on=true`.
- [ ] **macOS 15 tiling**: on macOS 15, choose **Window ▸ Fill** and
      **Window ▸ Move & Resize ▸ Left** for Bench's own window: the docked
      windows follow the panes. Drag Bench's window to the screen's top edge
      to fill it: the same. No notice appears for tiling.

### Milestone checks

- [ ] **Arranging without Bench's window.** With the test config below,
      and Brave and Obsidian open, `bench arrange Code` docks Zed, VS Code,
      Terminal, and Brave's Documentation window correctly on a monitor and
      on a laptop. It keeps VS Code stacked behind Zed, and the
      Documentation window behind Terminal, in the same frames. It parks
      the other Brave window, which belongs to Research, at a bottom corner
      of the screen, and hides Obsidian, whose windows all belong to
      Research. When you press Control-C, it puts everything back.
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
      Terminal, and sessions survive tab switches. `yes` leaves Bench
      responsive, and Control-C stops it at once. A link that `gh pr view`
      prints opens in the default browser.
- [ ] **Shortcuts.** Every shortcut in the
      [user guide](user-guide.md#keyboard-shortcuts) works from another app.
      A shortcut that another app has already registered shows in the status
      bar, with another to try. Also set up two input sources, so that macOS
      uses Control+Option+Space: the status bar reports the clash, though
      macOS lets Bench register it. Change a shortcut in Settings ▸
      Keyboard and save, and another in `config.toml` and choose **Bench ▸
      Reload config.toml**: the new ones work without restarting Bench.
- [ ] **Palette, Tab properties, and Settings.** Every palette action works
      by keyboard alone. Right-clicking a tab, and Control-clicking it, opens
      its menu. Saving a tab's properties, and saving Settings, updates
      `config.toml` (keeping `config.toml.bak`) and the window.
      **Bench ▸ Settings…** opens Settings, and **Tab ▸ Tab Properties…**
      opens the current tab's properties.
- [ ] **Live previews.** With previews on and Screen Recording allowed,
      hovering an inactive sub-tab shows a preview within half a second.
- [ ] **Packaging.** `make app` builds a signed `Bench.app` that opens, with
      a Dock icon and a menu bar item showing the current tab. The welcome
      guide appears on first run, and **Open Bench at login** works.
- [ ] **Crash safety.** With windows parked (switch away from a tab), end
      Bench with `kill -9`: the watchdog puts the parked windows back. If it
      doesn't, `bench restore` does.

Test config for the arranging check, with Terminal as an App item in place
of the built-in terminal. Brave has an item in each tab: an app whose
windows all belong to other tabs is hidden, and only the other-tab windows
of an app that also has windows in this tab are parked.

```toml
[[tab]]
name = "Code"
layout = "main-side"

  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }, { app = "com.microsoft.VSCode" }]

  [[tab.pane]]
  items = [{ app = "com.apple.Terminal" }, { app = "com.brave.Browser", title = "Documentation" }]

[[tab]]
name = "Research"
layout = "side"

  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]

  [[tab.pane]]
  items = [{ app = "md.obsidian" }]
```

Save it as `/tmp/bench-arrange/config.toml`. Quit Bench, and open two
Brave windows: one showing <https://go.dev/doc/>, whose title starts with
"Documentation", and one showing any other page. Then run:

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
frames should match the spec's [layout maths](spec.md#layout-maths), with
the `tabs` view's pane reaching up to the top of the pane area, as in the
app.

### Arrange a tab without Bench's window

```sh
bin/bench arrange Code
```

It opens the tab's apps if they aren't running, and docks each pane's
first window across the screen's visible area, with the pane's other
windows behind it. With `hide_other_tabs` on, as it is by default, it
hides apps whose windows all belong to other tabs, and parks the other
tabs' windows of apps that also have windows in this tab. Press Control-C,
or close the terminal, to put everything back; a second Control-C while
it does has no effect. It skips Web and terminal items, and refuses while
Bench, or another `bench arrange`, is running, because both would arrange
the same windows. For the same reason, Bench can't open while it runs: it
holds Bench's single-instance lock. Your terminal app needs Accessibility
permission.

With a config not called `config.toml`, it runs from a temporary copy in
`$TMPDIR`, which holds its `parked.json`. If it can't put every window
back, it keeps that folder and prints the `bench restore --dir` command
that puts back the rest.

### Put windows back after a crash

```sh
bin/bench restore
```

It puts back the windows listed in `parked.json` and shows the apps that
Bench hid, then says how many it put back and how many had closed or quit
since. Bench does this itself when it next starts, and its watchdog does
it when Bench crashes, so you only need this if both failed. Use `--dir`
for a support folder other than the default; as with `BENCH_HOME`, it
must be a folder that only you can change. It refuses while Bench or
`bench arrange` runs from the folder, or a watchdog is putting its windows
back: each holds the folder's lock (`bench.lock`) meanwhile.

### Read the log

The app writes Wails' warnings and errors, its own, the engine's, and each
notice as it first appears to standard error and to
`~/Library/Logs/Bench/bench.log`, which only you can read. Each launch
starts with a `Bench started` line that has the version. Once the file
would pass 1 MiB, Bench moves it aside as `bench.log.1` and starts again.
Console shows it under **Log Reports**, or:

```sh
tail -f ~/Library/Logs/Bench/bench.log
```

**Help ▸ Report a Problem…** says where the file is in the issue it fills
in. The browser preview and `bench arrange` keep no log file: their log
goes to standard error alone.

For more, set `BENCH_DEBUG` to any value that isn't empty. Bench then also
logs, at debug level:

- the platform calls that open, move, raise, focus, or hide apps, windows,
  and web views, such as `SetFrame`, `Hide`, and `Raise`, with the error if
  one failed;
- each window it looks up (`Window`), with its frame, and whether it's
  minimised or in full screen;
- every platform event, such as `window-moved`, with its window and app,
  and whether the mouse button was held on the window (`mouseDown`);
- the engine's decisions, such as putting back a window that its app moved;
- what each Web item's web view decides about a navigation of its page, a
  new window, or a popup's navigation (`web navigation`, `web popup`, and
  `web popup navigation`), such as `decision=in-browser`, with the scheme
  and host of the address it's about, but none of the rest of the address.

It leaves out window titles, and the token in VS Code for the Web's
address (`tkn=redacted`), but it still names the pages your Web items show
and the apps you use, so keep it to yourself.

These go to `bench.log` too, which then grows faster, and moves aside
sooner. It also writes the latest view, as the frontend gets it in
`bench:state`, to `view.json` in the support folder. `open` doesn't pass
your shell's environment to the app, so give it the variable with `--env`:

```sh
open --env BENCH_DEBUG=1 bin/Bench.app
```

Quit Bench first: if it's running, `open` only brings it forward. Running
`BENCH_DEBUG=1 bin/Bench.app/Contents/MacOS/bench` from a terminal also
works, and shows the log there too, but then Bench has your terminal app's
permissions, not Bench.app's. The
browser preview takes `BENCH_DEBUG` too:
`(umask 077; BENCH_DEBUG=1 make preview 2>~/bench-preview.log)`.

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

To try changes without touching your own files, point Bench at another
support folder with `BENCH_HOME`. Make a new, private one with `mktemp -d`
rather than using a fixed name in `/tmp`: Bench runs the commands in the
folder's `config.toml`, so it refuses a folder that belongs to another user
or that others can change, or one inside such a folder, and creates a
missing one readable only by you.
`bench check-config` and `bench arrange` need a `config.toml` there, so
copy yours first:

```sh
export BENCH_HOME="$(mktemp -d)"
cp ~/Library/Application\ Support/Bench/config.toml "$BENCH_HOME/"
bin/bench check-config
```

The app and `bench layout` use the default config if the folder has none;
the app writes it there. `bench layout` shows Safari in place of your
default browser, which lays out the same. To run the app against the
folder, quit Bench, then pass the variable with `--env`, since `open`
doesn't pass your shell's environment:

```sh
open --env BENCH_HOME="$BENCH_HOME" bin/Bench.app
```
