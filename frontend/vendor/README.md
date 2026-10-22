# Vendored files

Bench's frontend has no build step and works offline, so its dependencies are
copied here unchanged from their npm packages.

| Folder | Package | Files | Licence |
| --- | --- | --- | --- |
| `xterm/` | `@xterm/xterm` 6.0.0 | `lib/xterm.mjs`, `css/xterm.css` | MIT (`xterm/LICENSE`) |
| `xterm-addon-fit/` | `@xterm/addon-fit` 0.11.0 | `lib/addon-fit.mjs` | MIT (`xterm-addon-fit/LICENSE`) |
| `fonts/` | `@fontsource/ibm-plex-sans` 5.3.0 | latin 400, 500, and 600 `.woff2` | SIL OFL 1.1 (`fonts/OFL-IBM-Plex-Sans.txt`) |
| `fonts/` | `@fontsource/ibm-plex-mono` 5.3.0 | latin 400 and 500 `.woff2` | SIL OFL 1.1 (`fonts/OFL-IBM-Plex-Mono.txt`) |

`fonts/fonts.css` declares the font faces; it's written for Bench, from the
packages' own CSS.

To update a package, run `npm pack <package>@<version>` in a temporary folder
outside the repository, extract it, and copy the same files over. Then
change its version in the table above and in
`build/acknowledgements/components.go`, and run `make acknowledgements` from
the repository's root, which writes the Acknowledgements window's page
again. The tests check this table against that list: see
[Add or update a dependency](../../docs/development.md#add-or-update-a-dependency).
