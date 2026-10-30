# macOS packaging

The files that `make app` uses to assemble and sign `bin/Bench.app`. See
[Signing and the Accessibility permission](../../docs/development.md#signing-and-the-accessibility-permission)
for how signing works.

| File | What it is |
| --- | --- |
| `Info.plist` | The bundle's `Info.plist`. The Makefile also reads Bench's version from `CFBundleShortVersionString`. |
| `entitlements.plist` | The entitlements that `make app` signs with, alongside the hardened runtime. `make app` copies them to `bin/entitlements.plist` first, and changes the copy for an ad hoc `Bench.app` with Sparkle. |
| `Credits.rtf` | The credits that the About panel shows under Bench's version. `make app` copies it into `Bench.app/Contents/Resources`. |
| `icon.png` | The app icon, 1,024 by 1,024 pixels. `make app` turns it into `Bench.icns` with `sips` and `iconutil`. |
| `icon/main.go` | The program that draws `icon.png`. Run `make icon` after changing it, and commit the new PNG. |
| `menubar/main.go` | The program that draws the menu bar item's icon, `internal/ui/menubar.png`, which `internal/ui` embeds: a template image, in black with transparency, that macOS tints to suit the menu bar. Run `make icon` after changing it, and commit the new PNG. |
| `bundle_test.go` | Checks these files against the build brief, against the Makefile and `internal/mac`, which build `Bench.app` with them, and against the app menu in `internal/ui`, which the credits name. `go test ./...` runs it on any OS. |
| `makefile_test.go` | Checks that the Makefile tries Playwright's install again after a failed download, with stand-ins for `npm` and `npx`. It needs `make`, and skips without it. |

## Info.plist

- Bench is a regular app, with a Dock icon as well as its menu bar item, so
  there's no `LSUIElement`.
- macOS has no usage-description key for the Accessibility or Screen
  Recording permissions. Bench's first-run guide explains them instead.
- `NSCameraUsageDescription`, `NSMicrophoneUsageDescription`, and
  `NSAppleEventsUsageDescription` are the reasons macOS shows when a web
  page in a Web item, or a program in a Bench terminal, asks for the camera,
  the microphone, or to control another app. Both run as Bench in macOS's
  eyes, and without these keys macOS refuses them without asking.
- `NSAllowsArbitraryLoadsInWebContent` lets Web items load any web page,
  `http://` ones on intranets among them, as a browser does. It applies only
  to web views, so the rest of Bench keeps App Transport Security's rules.
- `NSAllowsLocalNetworking` lets Web items load VS Code for the Web, which
  Bench serves from `http://127.0.0.1`.
- `LSMinimumSystemVersion` is 14.0, because Web items use
  `WKWebsiteDataStore`'s per-profile data stores, which are new in macOS 14.
  The Makefile builds with the same deployment target.
- The `SU` keys are the settings of Sparkle, the framework that installs
  new versions of Bench (see
  [Updates](../../docs/development.md#updates)). `SUFeedURL` is the
  appcast that the release workflow adds to each release, at GitHub's
  address for the files of the latest release. `SUEnableAutomaticChecks` is
  false, so that Sparkle doesn't ask the user whether to look for new
  versions: the `check_for_updates` setting decides, and Bench turns
  Sparkle's daily look on and off to match it. `SURequireSignedFeed` and
  `SUVerifyUpdateBeforeExtraction` make Sparkle refuse an appcast or an
  update that the update key didn't sign, before it unpacks anything.
  `make app` adds the update key itself, `SUPublicEDKey`, from
  `SPARKLE_PUBLIC_KEY`. Without a key, `Bench.app` has no Sparkle, and
  these keys do nothing.

## Entitlements

- Bench isn't sandboxed. The Accessibility API, which Bench uses to move and
  size other apps' windows, doesn't work in the App Sandbox.
- It needs no hardened-runtime exceptions. WKWebView compiles JavaScript in
  WebKit's own processes, so Bench needs no JIT or unsigned-memory
  exceptions. Wails' own production entitlements have none of these
  exceptions either.
- It keeps library validation, which lets Bench load only frameworks that
  its own team signed. `make app` signs Sparkle with Bench's certificate,
  so a `Bench.app` signed with a certificate loads it. An ad hoc signature
  has no team, so for an ad hoc `Bench.app` with Sparkle, `make app` signs
  with a copy of these entitlements that adds
  `com.apple.security.cs.disable-library-validation`. Releases signed with a
  Developer ID certificate never get that exception.
- It opts in to the hardened runtime's camera
  (`com.apple.security.device.camera`), microphone
  (`com.apple.security.device.audio-input`), and Apple Events
  (`com.apple.security.automation.apple-events`) entitlements, which the
  hardened runtime otherwise denies. Bench itself uses none of them, but
  macOS counts Web items' pages and the programs in Bench's terminals as
  Bench: without them, a video call in a Web item can't use the camera, and
  an `osascript` command in a Bench terminal can't control Finder. macOS
  still asks the user the first time each one is used.
- Accessibility, Screen Recording, and notifications are permissions that
  the user grants in System Settings, not entitlements.

## Credits.rtf

- **About Bench** opens macOS's standard About panel. It shows the icon, the
  name, and the version from `Info.plist`, then these credits, which AppKit
  finds by name in `Contents/Resources`. So the panel shows them only in
  `Bench.app`: a binary that `go build` makes on its own has no bundle, and
  its About panel has no credits.
- The credits have no text colour, so that they read in Dark Mode. AppKit
  draws text that has no colour in the panel's own text colour, which
  follows the appearance, but keeps a colour that the file sets, whatever
  the appearance. TextEdit writes a colour table (`\colortbl`) and colour
  commands (`\cf`) when it saves, so take them out again if you edit the
  file in TextEdit. `bundle_test.go` checks for them.
- RTF is ASCII, so the ▸ in **Bench ▸ Acknowledgements** is written
  `\u9656?`: its code point in decimal, then a `?` for readers that don't
  know Unicode. TextEdit writes `\uc0\u9656 ` instead, which works too. The
  test checks that the credits name the menu item by its label in
  `internal/ui`.
- There's no copyright line (`NSHumanReadableCopyright` in `Info.plist`)
  yet. Add one when Bench gets a licence.
