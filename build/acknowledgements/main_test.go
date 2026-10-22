package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode"
)

// root is the repository's root, from this package's folder.
var root = filepath.Join("..", "..")

var (
	loadOnce   sync.Once
	loaded     *sources
	loadFailed error
)

// testSources loads the sources once for all the tests, since go list takes
// a moment. It also opens go.mod and go.sum, and lists the folders and looks
// at the files of every package of Bench's that the app links, so that go
// test runs the tests again, rather than reusing a cached result, when a
// dependency or an import changes, a new file's among them: go list's own
// reads don't count towards the test cache.
func testSources(t *testing.T) *sources {
	t.Helper()
	loadOnce.Do(func() { loaded, loadFailed = load(root) })
	if loadFailed != nil {
		t.Fatalf("listing what Bench.app includes: %v", loadFailed)
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if _, err := os.ReadFile(filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range loaded.packages {
		if p.Module == nil || !p.Module.Main {
			continue
		}
		_, _ = os.ReadDir(p.Dir)
		for _, name := range p.files() {
			_, _ = os.Stat(filepath.Join(p.Dir, name))
		}
	}
	return loaded
}

func TestPageIsCurrent(t *testing.T) {
	src := testSources(t)
	entries, err := build(src)
	if err != nil {
		t.Fatal(err)
	}
	want, err := render(entries)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "frontend", "acknowledgements.html")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, want) {
		return
	}
	gotLines, wantLines := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := range max(len(gotLines), len(wantLines)) {
		g, w := line(gotLines, i), line(wantLines, i)
		if g != w {
			t.Fatalf("frontend/acknowledgements.html is out of date: run make acknowledgements.\nLine %d is\n\t%s\nbut should be\n\t%s", i+1, g, w)
		}
	}
}

// line returns line i, or "(none)" past the end.
func line(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "(none)"
}

func TestModulesAreListed(t *testing.T) {
	src := testSources(t)
	for _, err := range checkModules(src) {
		t.Error(err)
	}
	// Bench.app is built for both of these, so a module that only one of
	// them links must still be listed.
	if !slices.Equal(src.targets.archs, []string{"arm64", "amd64"}) || src.targets.tags != "production" {
		t.Errorf("Makefile builds Bench.app for %v with tags %q; want arm64 and amd64 with production, or change this test",
			src.targets.archs, src.targets.tags)
	}
}

// TestTargetsMatchTheBuild checks that the Makefile's app-bin target builds
// what load lists: ./cmd/bench for darwin with cgo, for each of ARCHS, with
// APP_TAGS.
func TestTargetsMatchTheBuild(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	recipe := regexp.MustCompile(`(?s)\napp-bin:.*?\n\n`).Find(data)
	for _, want := range []string{
		"for arch in $(ARCHS)",
		"CGO_ENABLED=1 GOOS=darwin GOARCH=$$arch",
		`-tags "$(APP_TAGS)"`,
		"./cmd/bench",
	} {
		if !bytes.Contains(recipe, []byte(want)) {
			t.Errorf("the Makefile's app-bin recipe has no %q, so go list may not list what Bench.app links:\n%s", want, recipe)
		}
	}
}

func TestVendoredPackagesAreListed(t *testing.T) {
	src := testSources(t)
	for _, err := range checkVendored(src) {
		t.Error(err)
	}
	// Every licence file in frontend/vendor must be on the page.
	used := map[string]bool{}
	walkTexts(components, func(c component, t text) {
		if t.From == repo {
			used[t.File] = true
		}
	})
	licence := regexp.MustCompile(`(?i)^(licen[cs]e|copying|ofl|notice)`)
	err := filepath.WalkDir(filepath.Join(root, "frontend", "vendor"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !licence.MatchString(d.Name()) {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !used[filepath.ToSlash(rel)] {
			t.Errorf("%s isn't on the page: add it to its component's Texts, then run make acknowledgements", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFixedTextsAreUsed(t *testing.T) {
	used := map[string]bool{}
	walkTexts(components, func(c component, t text) {
		if t.From == fixed {
			used[t.File] = true
		}
	})
	files, err := os.ReadDir("texts")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		// Finder leaves a .DS_Store in a folder it shows.
		if strings.HasPrefix(f.Name(), ".") {
			continue
		}
		if !used[f.Name()] {
			t.Errorf("texts/%s isn't used by any component", f.Name())
		}
	}
	mpl, err := os.ReadFile(filepath.Join("texts", "MPL-2.0.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(mpl, []byte("Mozilla Public License Version 2.0\n")) || !bytes.Contains(mpl, []byte("Exhibit A - Source Code Form License Notice")) {
		t.Error("texts/MPL-2.0.txt isn't the Mozilla Public License 2.0")
	}
}

// walkTexts calls fn for each text of each component and part.
func walkTexts(cs []component, fn func(component, text)) {
	for _, c := range cs {
		for _, t := range c.Texts {
			fn(c, t)
		}
		walkTexts(c.Parts, fn)
	}
}

// noticeStart matches where a copyright notice may start: the word
// copyright, as in "Copyright 2009 The Go Authors", "Copyright The Chromium
// Authors", or "SPDX-FileCopyrightText: 2023 Someone"; a copyright sign
// before a year or a name; or "(c)" before a year, or before a name and
// then a year, as in "(c) Lea Anthony 2019-present". A "(c)" with a name
// and no year is too like code and prose, such as "f(c) returns".
var noticeStart = regexp.MustCompile(`(?i)copyright|(©|&copy;)\s*[\pL\d]|\(c\)\s*\d|\(c\)\s+\pL.{0,80}?\b\d{4}\b`)

// notNotice matches, from where the word starts, the uses of copyright in
// licence texts that aren't notices, such as "copyright notice", "COPYRIGHT
// HOLDERS", and "copyrighted", or the word at the end of a line.
var notNotice = regexp.MustCompile(`(?i)^copyright(ed\b|s\b|\s+(notices?|holders?|owners?|statements?|laws?|and|or)\b|\s*\(s\)|,\s*patent|\s*$)`)

// mayHaveNotice reports whether a line may hold a notice: it's much quicker
// than noticeStart, which the test would otherwise run on every line of the
// standard library's files.
func mayHaveNotice(line string) bool {
	for _, s := range []string{"(c)", "(C)", "©", "&copy;"} {
		if strings.Contains(line, s) {
			return true
		}
	}
	return copyrightWord.MatchString(line)
}

var copyrightWord = regexp.MustCompile(`(?i)copyright`)

// notices returns the copyright notices in a line, each from where it
// starts to where the next one starts, or to 200 bytes at most.
func notices(line string) []string {
	if !mayHaveNotice(line) {
		return nil
	}
	// Markers with nothing but spaces and punctuation between them, as in
	// "Copyright (c) 2009" or "(C) Copyright IBM", start one notice.
	var starts []int
	at := noticeStart.FindAllStringIndex(line, -1)
	for i, m := range at {
		if i == 0 || strings.ContainsFunc(line[at[i-1][1]:m[0]], func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			starts = append(starts, m[0])
		}
	}
	var found []string
	for i, start := range starts {
		end := min(start+200, len(line))
		if i+1 < len(starts) {
			end = min(end, starts[i+1])
		}
		if notice := line[start:end]; !notNotice.MatchString(notice) {
			found = append(found, notice)
		}
	}
	return found
}

// A scanned is a file that TestNoticesAreListed looks for notices in, and
// the components whose holders its notices may name.
type scanned struct {
	path   string
	owners []string
}

// TestNoticesAreListed looks for copyright notices in the third-party files
// that Bench.app includes: the files of the standard library's packages and
// the modules' packages that the app links, the folders that components
// name in Scan, and frontend/vendor. Each notice's holder must be one of
// those of the component that the file belongs to, or of one of its parts,
// or the notice must be reviewed. A new notice usually means that a
// dependency has taken code from another project, which then needs a part
// of its own.
func TestNoticesAreListed(t *testing.T) {
	src := testSources(t)
	holders := map[string][]*regexp.Regexp{}
	var collect func(name string, cs []component)
	collect = func(name string, cs []component) {
		for _, c := range cs {
			for _, h := range c.Holders {
				holders[name] = append(holders[name], regexp.MustCompile("(?i)"+h))
			}
			collect(name, c.Parts)
		}
	}
	byModule := map[string]string{}
	for _, c := range components {
		collect(c.Name, []component{c})
		if c.Module != "" {
			byModule[c.Module] = c.Name
		}
	}

	var files []scanned
	for _, p := range src.packages {
		var owner string
		switch {
		case p.Standard:
			owner = "Go"
		case p.Module != nil && !p.Module.Main:
			// checkModules reports a module that no component lists.
			owner = byModule[p.Module.Path]
		default:
			continue
		}
		for _, name := range p.files() {
			files = append(files, scanned{filepath.Join(p.Dir, name), []string{owner}})
		}
	}
	for _, c := range components {
		m := src.modules[c.Module]
		for _, dir := range c.Scan {
			if m == nil {
				t.Errorf("%s has Scan but no module", c.Name)
				continue
			}
			for _, f := range walkFiles(t, filepath.Join(m.Dir, filepath.FromSlash(dir))) {
				files = append(files, scanned{f, []string{c.Name}})
			}
		}
	}
	// A file in frontend/vendor belongs to the components whose texts are
	// in the same folder.
	vendor := filepath.Join(root, "frontend", "vendor")
	for _, f := range walkFiles(t, vendor) {
		if strings.HasSuffix(f, ".woff2") {
			continue
		}
		rel, err := filepath.Rel(vendor, f)
		if err != nil {
			t.Fatal(err)
		}
		folder, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		var owners []string
		for _, c := range components {
			if slices.ContainsFunc(c.Texts, func(x text) bool {
				return x.From == repo && strings.HasPrefix(x.File, "frontend/vendor/"+folder+"/")
			}) {
				owners = append(owners, c.Name)
			}
		}
		files = append(files, scanned{f, owners})
	}
	slices.SortFunc(files, func(a, b scanned) int { return strings.Compare(a.path, b.path) })
	files = slices.CompactFunc(files, func(a, b scanned) bool { return a.path == b.path })

	used := make([]bool, len(reviewed))
	for _, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			t.Fatal(err)
		}
		var known []*regexp.Regexp
		for _, o := range f.owners {
			known = append(known, holders[o]...)
		}
		for _, l := range strings.Split(string(data), "\n") {
			for _, notice := range notices(l) {
				if slices.ContainsFunc(known, func(h *regexp.Regexp) bool { return h.MatchString(notice) }) {
					continue
				}
				i := slices.IndexFunc(reviewed, func(r review) bool {
					return strings.HasSuffix(filepath.ToSlash(f.path), "/"+r.File) && regexp.MustCompile(r.Holder).MatchString(notice)
				})
				if i >= 0 {
					used[i] = true
					continue
				}
				t.Errorf("%s has a notice that %s doesn't account for: %q. Check what code it covers, and add a part or a holder to the component in components.go, or add it to reviewed with its file (its path in its module, the Go toolchain's folder, or the repository) and the reason", f.path, owned(f.owners), notice)
			}
		}
	}
	for i, r := range reviewed {
		if !used[i] {
			t.Errorf("reviewed lists %s in %s, which no longer has that notice, or isn't that file's path: take it out or correct it", r.Holder, r.File)
		}
	}
}

// owned names the components that own a file, for a message.
func owned(owners []string) string {
	switch {
	case len(owners) == 0 || owners[0] == "":
		return "no component"
	case len(owners) == 1:
		return owners[0]
	}
	return strings.Join(owners, " or ")
}

func TestNotices(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{"// Copyright 2009 The Go Authors. All rights reserved.", []string{"Copyright 2009 The Go Authors. All rights reserved."}},
		{"// Copyright The Chromium Authors", []string{"Copyright The Chromium Authors"}},
		{" * (c) Lea Anthony 2019-present", []string{"(c) Lea Anthony 2019-present"}},
		{"(C) Copyright IBM Corp. 2016", []string{"(C) Copyright IBM Corp. 2016"}},
		{"// SPDX-FileCopyrightText: 2023 Jane Doe", []string{"CopyrightText: 2023 Jane Doe"}},
		{"Copyright: 2020 Foo Ltd", []string{"Copyright: 2020 Foo Ltd"}},
		{"© Bar GmbH", []string{"© Bar GmbH"}},
		{"Copyright &copy; 2021 Baz Inc", []string{"Copyright &copy; 2021 Baz Inc"}},
		{"Copyright (c) 2021 Foo; portions Copyright (c) Bar", []string{"Copyright (c) 2021 Foo; portions ", "Copyright (c) Bar"}},
		{"// (c) Failing all the above, memory for a new chunk is mapped.", nil},
		{"// f(c) returns the same value as c.Err().", nil},
		{"if r == '©' {", nil},
		{"The above copyright notice and this permission notice shall be included", nil},
		{"THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM", nil},
		{"Copyright Holder(s), copyrighted, and Copyright", nil},
		{"if(c)return a;", nil},
	}
	for _, tt := range tests {
		if got := notices(tt.line); !slices.Equal(got, tt.want) {
			t.Errorf("notices(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

// walkFiles returns the files in dir and its subfolders.
func walkFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCommentBlock(t *testing.T) {
	src := strings.Join([]string{
		"package x",
		"",
		"// Code from somewhere",
		"/*",
		"The MIT License (MIT)",
		"",
		"Copyright (c) 2015 Someone",
		"",
		"THE SOFTWARE.",
		"*/",
		"//     Copyright (C) 1993 by Sun",
		"//",
		"// is preserved.",
		" * BSD 2-Clause License",
		" *  1. Redistributions",
		" * OF THIS SOFTWARE.",
	}, "\n")
	tests := []struct {
		first, last, want string
	}{
		{"The MIT License", "SOFTWARE.", "The MIT License (MIT)\n\nCopyright (c) 2015 Someone\n\nTHE SOFTWARE."},
		{"Copyright (C) 1993", "is preserved.", "Copyright (C) 1993 by Sun\n\nis preserved."},
		{"BSD 2-Clause", "OF THIS SOFTWARE.", "BSD 2-Clause License\n1. Redistributions\nOF THIS SOFTWARE."},
	}
	for _, tt := range tests {
		got, err := commentBlock(src, tt.first, tt.last)
		if err != nil || got != tt.want {
			t.Errorf("commentBlock(%q, %q) = %q, %v; want %q", tt.first, tt.last, got, err, tt.want)
		}
	}
	if _, err := commentBlock(src, "Apache License", "END"); err == nil {
		t.Error("commentBlock found a block that isn't there")
	}
	if _, err := commentBlock(src, "BSD 2-Clause", "The MIT License"); err == nil {
		t.Error("commentBlock found a last line before the first")
	}
}

func TestLeadingComments(t *testing.T) {
	src := strings.Join([]string{
		"/**",
		" * Copyright (c) 2014 Them. All rights reserved.",
		" * @license MIT",
		" *",
		" * Forked from:",
		" *   Someone's code",
		" */",
		"/*-----------------------------",
		" *  Copyright (c) Microsoft Corporation.",
		" *----------------------------*/",
		"var a=1;/* not this */",
	}, "\n")
	want := "Copyright (c) 2014 Them. All rights reserved.\n@license MIT\n\nForked from:\n  Someone's code\n\nCopyright (c) Microsoft Corporation."
	if got := leadingComments(src); got != want {
		t.Errorf("leadingComments = %q, want %q", got, want)
	}
	if got := leadingComments("var a=1;\n/* Copyright */"); got != "" {
		t.Errorf("leadingComments of a file that starts with code = %q, want nothing", got)
	}
}

func TestIDs(t *testing.T) {
	tests := []struct{ name, parent, want string }{
		{"Go", "", "go"},
		{"golang.org/x/net", "", "golang-org-x-net"},
		{"xterm.js fit add-on", "", "xterm-js-fit-add-on"},
		{"Public Suffix List", "golang-org-x-net", "golang-org-x-net-public-suffix-list"},
	}
	for _, tt := range tests {
		if got := idOf(tt.name, tt.parent); got != tt.want {
			t.Errorf("idOf(%q, %q) = %q, want %q", tt.name, tt.parent, got, tt.want)
		}
	}
	if errs := checkNames(); len(errs) > 0 {
		t.Error(errs)
	}
}

// TestPageIsStatic checks that the page runs no scripts and links only
// within itself: its window has no address bar, so a link would load
// another page in its place.
func TestPageIsStatic(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "frontend", "acknowledgements.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	if strings.Contains(strings.ToLower(page), "<script") {
		t.Error("the page has a script")
	}
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(` id="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		if ids[m[1]] {
			t.Errorf("id %q is used twice", m[1])
		}
		ids[m[1]] = true
	}
	for _, m := range regexp.MustCompile(` href="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		href := m[1]
		switch {
		case strings.HasPrefix(href, "#"):
			if !ids[href[1:]] {
				t.Errorf("link %s goes nowhere", href)
			}
		case href == "styles.css" || href == "vendor/fonts/fonts.css":
		default:
			t.Errorf("the page links to %s", href)
		}
	}
}
