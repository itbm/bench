# macOS packaging

The files that `make app` uses to assemble and sign `bin/Bench.app`. See
[Signing and the Accessibility permission](../../docs/development.md#signing-and-the-accessibility-permission)
for how signing works.

| File | What it is |
| --- | --- |
| `Info.plist` | The bundle's `Info.plist`. The Makefile also reads Bench's version from `CFBundleShortVersionString`. |
| `entitlements.plist` | The entitlements that `make app` signs with, alongside the hardened runtime. |
| `icon.png` | The app icon, 1,024 by 1,024 pixels. `make app` turns it into `Bench.icns` with `sips` and `iconutil`. |
| `icon/main.go` | The program that draws `icon.png`. Run `make icon` after changing it, and commit the new PNG. |
| `bundle_test.go` | Checks these files against the build brief, and against the Makefile and `internal/mac`, which build `Bench.app` with them. `go test ./...` runs it on any OS. |

## Info.plist

- Bench is a regular app, with a Dock icon as well as its menu bar item, so
  there's no `LSUIElement`.
- macOS has no usage-description key for the Accessibility or Screen
  Recording permissions. Bench's first-run guide explains them instead.
- `NSAllowsLocalNetworking` lets Web items load VS Code for the Web, which
  Bench serves from `http://localhost`.
- `LSMinimumSystemVersion` is 14.0, because Web items use
  `WKWebsiteDataStore`'s per-profile data stores, which are new in macOS 14.
  The Makefile builds with the same deployment target.

## Entitlements

- Bench isn't sandboxed. The Accessibility API, which Bench uses to move and
  size other apps' windows, doesn't work in the App Sandbox.
- It needs no hardened-runtime exceptions. WKWebView compiles JavaScript in
  WebKit's own processes, so Bench needs no JIT or unsigned-memory
  exceptions, and it loads no libraries that someone else signed, so it
  keeps library validation. Wails' own production entitlements have none of
  these exceptions either.
- Accessibility, Screen Recording, and notifications are permissions that
  the user grants in System Settings, not entitlements.
