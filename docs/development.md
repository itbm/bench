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
  binary on its own, with permissions granted in advance), the first-run
  guide and asking for permission, signing with a certificate, Developer
  ID signing and notarisation (which the [release workflow](#release-a-new-version)
  can do, but hasn't yet), Open at login, the About panel's credits, the
  Acknowledgements window, sign-ins and notifications in Web items, VS Code
  for the Web, live previews, `bench arrange`, crash recovery, multiple
  displays, desktops (Spaces), macOS 14, Intel Macs (the runner has macOS
  15 on Apple silicon), and any native code that the demo's tour doesn't
  reach. Expect bugs there until someone works through the
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
3. Starts Bench with `BENCH_DEBUG=1`, and takes a tour: it presses Bench's
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
commit the new `build/darwin/icon.png`.

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
      uses Control+Option+Space: macOS may let Bench register it but keep the
      keys for itself, so note whether the status bar reports the clash.
- [ ] **Palette, Tab properties, and Settings.** Every palette action works
      by keyboard alone. Right-clicking a tab, and Control-clicking it, opens
      its menu. Saving a tab's properties, and saving Settings, updates
      `config.toml` (keeping `config.toml.bak`) and the window.
      **Bench ▸ Settings…** opens Settings, and **View ▸ Tab Properties…**
      opens the current tab's properties.
- [ ] **Live previews.** With previews on and Screen Recording allowed,
      hovering an inactive sub-tab shows a preview within half a second.
- [ ] **Packaging.** `make app` builds a signed `Bench.app` that opens, with
      a Dock icon and a menu bar item showing the current tab. The first-run
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
frames should match the spec's [layout maths](spec.md#layout-maths).

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

Bench writes Wails' warnings and errors, and its own, to standard error. To
keep them when you open the app, send standard error to a file that only
you can read, in your own folder rather than `/tmp`, where other users of
the Mac could read it:

```sh
touch ~/Library/Logs/Bench.log
chmod 600 ~/Library/Logs/Bench.log
open --stderr ~/Library/Logs/Bench.log bin/Bench.app
tail -f ~/Library/Logs/Bench.log
```

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

It also writes the latest view, as the frontend gets it in `bench:state`,
to `view.json` in the support folder. `open` doesn't pass your shell's
environment to the app, so give it the variable with `--env`:

```sh
open --env BENCH_DEBUG=1 --stderr ~/Library/Logs/Bench.log bin/Bench.app
```

Quit Bench first: if it's running, `open` only brings it forward. Running
`bin/Bench.app/Contents/MacOS/bench` from a terminal also works, as
`BENCH_DEBUG=1 bin/Bench.app/Contents/MacOS/bench 2>>~/Library/Logs/Bench.log`,
but then Bench has your terminal app's permissions, not Bench.app's. The
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
