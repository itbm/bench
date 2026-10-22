package main

// A component is software that Bench.app includes from someone else: a Go
// module that the app links, the Go toolchain's standard library, or a
// package in frontend/vendor. Parts are code that a component took from
// other projects, which carry notices of their own.
type component struct {
	// Name is the component's name on the page. Names are unique, and the
	// page sorts components by them.
	Name string
	// Module is the Go module that the app links, whose version comes from
	// go list. Empty for the standard library, vendored packages, and
	// parts.
	Module string
	// Package and Version are a vendored package and its version, as
	// frontend/vendor/README.md names them.
	Package, Version string
	// About says what a component that isn't a module or a vendored package
	// is, such as "The standard library and runtime".
	About string
	// Address is the project's web address, which the page shows as text.
	// A module's path is address enough.
	Address string
	// Licence is the licence's name.
	Licence string
	// Note is a sentence or two that the page shows before the texts.
	Note string
	// Texts are the notices and licence texts that the page reproduces, in
	// order.
	Texts []text
	// Evidence is a file and a string in it that show that the component
	// still includes the part, for a part whose texts don't come from the
	// component's own files.
	Evidence *text
	// Holders are regular expressions for the copyright holders that this
	// component accounts for, in the notices that TestNoticesAreListed
	// finds in the files that Bench.app includes. They match whatever the
	// case.
	Holders []string
	// Scan are more folders of the module to look for notices in, as
	// TestNoticesAreListed does in the files that the app links: those of
	// code that the module builds into a file it embeds.
	Scan  []string
	Parts []component
}

// A text is a notice or licence text, or with Contains, a string that a
// file must contain.
type text struct {
	// From is where File is.
	From place
	// File is the file's path, with forward slashes, from From's folder.
	File string
	// First and Last, if set, take only a block of comment lines, from the
	// one containing First to the next one containing Last, without the
	// comment markers and the indentation.
	First, Last string
	// Leading takes only the comments at the top of the file, without the
	// comment markers.
	Leading bool
	// Contains is the string that the file must contain, for Evidence.
	Contains string
}

// A place is where a text's file is.
type place int

const (
	// module is the folder of the component's module, or for a part, its
	// component's module, in the module cache.
	module place = iota
	// goroot is the Go toolchain's folder, as go env GOROOT names it.
	goroot
	// repo is the repository's root.
	repo
	// fixed is build/acknowledgements/texts, for texts that aren't in any
	// of the others. Each one says where it came from.
	fixed
)

// plexNotice is IBM's notice for IBM Plex, which names Plex as a Reserved
// Font Name. The licence files that come with Fontsource's packages, which
// frontend/vendor has, give the fonts' own copyright lines instead, without
// it. texts/ibm-plex.txt is IBM Plex's LICENSE.txt, from
// https://github.com/IBM/plex/blob/71f558992315a12ca1f06a2215d57962a1992162/LICENSE.txt.
var plexNotice = text{From: fixed, File: "ibm-plex.txt",
	First: `Copyright © 2017 IBM Corp. with Reserved Font Name "Plex"`, Last: `Reserved Font Name "Plex"`}

// plexNote introduces plexNotice and the licence after it.
const plexNote = "IBM's own notice for IBM Plex comes first, then the licence that comes with the font files that Bench includes."

// components are what Bench.app includes from others. Every third-party
// module that the app links must be here, and every vendored package that
// frontend/vendor/README.md lists.
var components = []component{
	{
		Name:    "Go",
		About:   "The standard library and runtime",
		Address: "go.dev",
		Licence: "BSD 3-Clause License",
		Texts:   []text{{From: goroot, File: "LICENSE"}},
		Holders: []string{`The Go Authors`},
		Parts: []component{
			{
				Name:    "Inferno",
				About:   "Go's memory copying for Intel Macs, derived from Inferno's",
				Address: "inferno-os.org",
				Licence: "MIT License",
				Texts: []text{{From: goroot, File: "src/runtime/memmove_amd64.s",
					First: "Copyright © 1994-1999 Lucent Technologies Inc.", Last: "THE SOFTWARE."}},
				Holders: []string{`Lucent Technologies`, `Vita Nuova`},
			},
			{
				Name:    "Cephes Math Library",
				About:   "Trigonometric, hyperbolic and gamma functions in Go's math package, from Cephes",
				Address: "netlib.org/cephes",
				Licence: "Stephen L. Moshier's notice",
				Texts: []text{{From: goroot, File: "src/math/sin.go",
					First: "Cephes Math Library Release 2.8", Last: "guarantee."}},
				Holders: []string{`Stephen L\. Moshier`},
			},
			{
				Name:    "fiat-crypto",
				About:   "Elliptic-curve arithmetic in Go's crypto packages, generated by fiat-crypto",
				Address: "github.com/mit-plv/fiat-crypto",
				Licence: "BSD 1-Clause License",
				Texts: []text{{From: goroot, File: "src/crypto/internal/fips140/edwards25519/scalar.go",
					First: "Copyright (c) 2015-2020 The fiat-crypto Authors", Last: "SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE."}},
				Holders: []string{`The fiat-crypto Authors`},
			},
			{
				Name:    "fdlibm",
				About:   "Functions in Go's math package, such as logarithms and exponentials, from Sun's fdlibm, most of them by way of FreeBSD's msun",
				Address: "netlib.org/fdlibm",
				Licence: "Sun Microsystems' notice",
				Texts: []text{
					{From: goroot, File: "src/math/log.go", First: "Copyright (C) 1993 by Sun Microsystems", Last: "is preserved."},
					{From: goroot, File: "src/math/exp.go", First: "Copyright (C) 2004 by Sun Microsystems", Last: "is preserved."},
					{From: goroot, File: "src/math/cbrt.go", First: "Copyright (C) 1993 by Sun Microsystems", Last: "is preserved."},
				},
				Holders: []string{`Sun Microsystems`},
			},
		},
	},
	{
		Name:    "go-toml",
		Module:  "github.com/pelletier/go-toml/v2",
		Licence: "MIT License",
		Texts:   []text{{From: module, File: "LICENSE"}},
		Holders: []string{`Thomas Pelletier`},
	},
	{
		Name:    "golang.org/x/net",
		Module:  "golang.org/x/net",
		Licence: "BSD 3-Clause License",
		Texts:   []text{{From: module, File: "LICENSE"}},
		Holders: []string{`The Go Authors`},
		Parts: []component{
			{
				// The note gives the list's revision and where its source
				// is, as the Mozilla Public License asks of software that
				// includes it in executable form (section 3.2).
				Name:    "Public Suffix List",
				About:   "Data in golang.org/x/net's publicsuffix package",
				Address: "publicsuffix.org",
				Licence: "Mozilla Public License 2.0",
				Note: "Bench includes the list as published on {suffix-published}, at revision {suffix-revision}. " +
					"Its source is at https://github.com/publicsuffix/list/blob/{suffix-revision}/public_suffix_list.dat.",
				// texts/MPL-2.0.txt is https://www.mozilla.org/media/MPL/2.0/index.txt.
				Texts:    []text{{From: fixed, File: "MPL-2.0.txt"}},
				Evidence: &text{From: module, File: "publicsuffix/table.go", Contains: "publicsuffix.org's public_suffix_list.dat"},
			},
		},
	},
	{
		Name:    "golang.org/x/sys",
		Module:  "golang.org/x/sys",
		Licence: "BSD 3-Clause License",
		Texts:   []text{{From: module, File: "LICENSE"}},
		Holders: []string{`The Go Authors`},
	},
	{
		Name:    "IBM Plex Mono",
		Package: "@fontsource/ibm-plex-mono",
		Version: "5.3.0",
		Address: "github.com/IBM/plex",
		Licence: "SIL Open Font License 1.1",
		Note:    plexNote,
		Texts: []text{
			plexNotice,
			{From: repo, File: "frontend/vendor/fonts/OFL-IBM-Plex-Mono.txt"},
		},
		Holders: []string{`IBM Corp`},
	},
	{
		Name:    "IBM Plex Sans",
		Package: "@fontsource/ibm-plex-sans",
		Version: "5.3.0",
		Address: "github.com/IBM/plex",
		Licence: "SIL Open Font License 1.1",
		Note:    plexNote,
		Texts: []text{
			plexNotice,
			{From: repo, File: "frontend/vendor/fonts/OFL-IBM-Plex-Sans.txt"},
		},
		Holders: []string{`IBM Corp`},
	},
	{
		Name:    "plist",
		Module:  "howett.net/plist",
		Licence: "BSD 2-Clause License and BSD 3-Clause License",
		Texts:   []text{{From: module, File: "LICENSE"}},
		Holders: []string{`Dustin L\. Howett`, `The Go Authors`},
	},
	{
		Name:    "pty",
		Module:  "github.com/creack/pty",
		Licence: "MIT License",
		Texts:   []text{{From: module, File: "LICENSE"}},
		Holders: []string{`Keith Rarick`},
	},
	{
		Name:    "Wails",
		Module:  "github.com/wailsapp/wails/v3",
		Licence: "MIT License",
		Texts:   []text{{From: module, File: "LICENSE"}},
		Holders: []string{`Lea Anthony`},
		// The JavaScript runtime that Wails serves to Bench's pages,
		// /wails/runtime.js, is built from these sources. The build drops
		// their comments, notices among them.
		Scan: []string{"internal/runtime/desktop/@wailsio/runtime/src"},
		Parts: []component{
			{
				Name:    "Chromium",
				About:   "Wails' screen layout code, adapted from Chromium's",
				Address: "chromium.org",
				Licence: "BSD 3-Clause License",
				// texts/chromium.txt is Chromium's LICENSE, from
				// https://chromium.googlesource.com/chromium/src/+/HEAD/LICENSE,
				// without its // comment markers.
				Texts:    []text{{From: fixed, File: "chromium.txt"}},
				Evidence: &text{From: module, File: "pkg/application/screenmanager.go", Contains: "Copyright 2015 The Chromium Authors"},
				Holders:  []string{`The Chromium Authors`},
			},
			{
				Name:    "htmx",
				About:   "A technique in Wails' JavaScript runtime, taken from htmx",
				Address: "htmx.org",
				Licence: "BSD 2-Clause License",
				Texts: []text{{From: module, File: "internal/runtime/desktop/@wailsio/runtime/src/utils.ts",
					First: "BSD 2-Clause License", Last: "OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE."}},
				Holders: []string{`Big Sky Software`},
			},
			{
				Name:    "is-callable",
				About:   "Code in Wails' JavaScript runtime",
				Address: "github.com/inspect-js/is-callable",
				Licence: "MIT License",
				Texts: []text{{From: module, File: "internal/runtime/desktop/@wailsio/runtime/src/callable.ts",
					First: "The MIT License (MIT)", Last: "SOFTWARE."}},
				Holders: []string{`Jordan Harband`},
			},
			{
				Name:    "nanoid",
				About:   "Code in Wails' JavaScript runtime",
				Address: "github.com/ai/nanoid",
				Licence: "MIT License",
				Texts: []text{{From: module, File: "internal/runtime/desktop/@wailsio/runtime/src/nanoid.ts",
					First: "The MIT License (MIT)", Last: "OTHER DEALINGS IN THE SOFTWARE."}},
				Holders: []string{`Andrey Sitnik`},
			},
			{
				Name:    "ringqueue",
				About:   "Code in Wails' asset server",
				Address: "github.com/erikdubbelboer/ringqueue",
				Licence: "MIT License",
				Texts: []text{{From: module, File: "internal/assetserver/ringqueue.go",
					First: "The MIT License (MIT)", Last: "SOFTWARE."}},
				Holders: []string{`Erik Dubbelboer`},
			},
		},
	},
	{
		Name:    "xdg",
		Module:  "github.com/adrg/xdg",
		Licence: "MIT License",
		Texts:   []text{{From: module, File: "LICENSE"}},
		Holders: []string{`Adrian-George Bostan`},
	},
	{
		Name:    "xterm.js",
		Package: "@xterm/xterm",
		Version: "6.0.0",
		Address: "xtermjs.org",
		Licence: "MIT License",
		Texts: []text{
			{From: repo, File: "frontend/vendor/xterm/xterm.mjs", Leading: true},
			{From: repo, File: "frontend/vendor/xterm/LICENSE"},
		},
		Holders: []string{`The xterm\.js authors`, `SourceLair`, `Christopher Jeffrey`, `Fabrice Bellard`},
		Parts: []component{
			{
				Name:    "Visual Studio Code",
				About:   "Code in xterm.js, from Visual Studio Code",
				Address: "github.com/microsoft/vscode",
				Licence: "MIT License",
				// texts/vscode.txt is Visual Studio Code's LICENSE.txt, from
				// https://github.com/microsoft/vscode/blob/main/LICENSE.txt,
				// which xterm.mjs's notice points to.
				Texts:    []text{{From: fixed, File: "vscode.txt"}},
				Evidence: &text{From: repo, File: "frontend/vendor/xterm/xterm.mjs", Contains: "Copyright (c) Microsoft Corporation"},
				Holders:  []string{`Microsoft Corporation`},
			},
		},
	},
	{
		Name:    "xterm.js fit add-on",
		Package: "@xterm/addon-fit",
		Version: "0.11.0",
		Address: "xtermjs.org",
		Licence: "MIT License",
		Texts: []text{
			{From: repo, File: "frontend/vendor/xterm-addon-fit/addon-fit.mjs", Leading: true},
			{From: repo, File: "frontend/vendor/xterm-addon-fit/LICENSE"},
		},
		Holders: []string{`The xterm\.js authors`, `Christopher Jeffrey`, `Fabrice Bellard`, `Microsoft Corporation`},
	},
}

// A review is a copyright notice in a file that Bench.app includes, which
// someone has checked and which needs no entry on the page.
type review struct {
	// File is the file's path in its module, or in the Go toolchain's or
	// the repository's folder, with forward slashes.
	File string
	// Holder is a regular expression for the notice's holder.
	Holder string
	// Reason says why the notice needs no entry.
	Reason string
}

// reviewed are the notices that need no entry. TestNoticesAreListed fails
// on one that its file no longer has.
var reviewed = []review{
	{
		File:   "pkg/application/roles.go",
		Holder: `Github Inc`,
		Reason: "Wails names Electron as the inspiration for its menu roles; it copies no code from Electron.",
	},
}
