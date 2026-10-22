package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// sources are what the page is made from.
type sources struct {
	// root is the repository's root, and goroot the Go toolchain's folder.
	root, goroot string
	// targets are the architectures and tags of Bench.app's build.
	targets targets
	// packages are the packages that the build links, for any of its
	// architectures, by import path.
	packages map[string]*goPackage
	// modules are the third-party modules that the build links, by path.
	modules map[string]*goModule
	// vendored are the packages that frontend/vendor/README.md lists, with
	// their versions.
	vendored map[string]string
	// suffixes is the Public Suffix List's revision and when it was
	// published, from golang.org/x/net.
	suffixes suffixList
}

// targets are the architectures and tags that the Makefile builds
// Bench.app's binary with.
type targets struct {
	archs []string
	tags  string
}

// A goPackage is the part of go list's output that the page needs.
type goPackage struct {
	ImportPath string
	Dir        string
	Standard   bool
	Module     *goModule
	GoFiles    []string
	CgoFiles   []string
	CFiles     []string
	CXXFiles   []string
	MFiles     []string
	HFiles     []string
	SFiles     []string
	SysoFiles  []string
	EmbedFiles []string
}

// files returns the package's files that the build compiles or embeds.
func (p *goPackage) files() []string {
	var all []string
	for _, names := range [][]string{p.GoFiles, p.CgoFiles, p.CFiles, p.CXXFiles, p.MFiles, p.HFiles, p.SFiles, p.SysoFiles, p.EmbedFiles} {
		all = append(all, names...)
	}
	return all
}

// A goModule is a module as go list reports it.
type goModule struct {
	Path    string
	Version string
	Dir     string
	Main    bool
}

// suffixList identifies the Public Suffix List that golang.org/x/net
// includes.
type suffixList struct {
	revision  string
	published time.Time
}

// load gathers the sources for the repository at root.
func load(root string) (*sources, error) {
	src := &sources{root: root, packages: map[string]*goPackage{}, modules: map[string]*goModule{}}
	var err error
	if src.targets, err = readTargets(filepath.Join(root, "Makefile")); err != nil {
		return nil, err
	}
	out, err := goCommand(root, nil, "env", "GOROOT")
	if err != nil {
		return nil, err
	}
	src.goroot = strings.TrimSpace(string(out))
	for _, arch := range src.targets.archs {
		if err := src.list(arch); err != nil {
			return nil, err
		}
	}
	if src.vendored, err = readVendored(filepath.Join(root, "frontend", "vendor", "README.md")); err != nil {
		return nil, err
	}
	if net := src.modules["golang.org/x/net"]; net != nil {
		if src.suffixes, err = readSuffixList(filepath.Join(net.Dir, "publicsuffix", "table.go")); err != nil {
			return nil, err
		}
	}
	return src, nil
}

// makeVariables are the Makefile's ARCHS and APP_TAGS.
var (
	makeArchs = regexp.MustCompile(`(?m)^ARCHS \?= *(.*)$`)
	makeTags  = regexp.MustCompile(`(?m)^APP_TAGS \?= *(.*)$`)
)

// readTargets reads the architectures and tags that the Makefile builds
// Bench.app's binary with.
func readTargets(makefile string) (targets, error) {
	data, err := os.ReadFile(makefile)
	if err != nil {
		return targets{}, err
	}
	archs, tags := makeArchs.FindSubmatch(data), makeTags.FindSubmatch(data)
	if archs == nil || tags == nil {
		return targets{}, fmt.Errorf("%s has no ARCHS ?= or APP_TAGS ?= line", makefile)
	}
	t := targets{archs: strings.Fields(string(archs[1])), tags: strings.TrimSpace(string(tags[1]))}
	if len(t.archs) == 0 {
		return targets{}, fmt.Errorf("%s's ARCHS is empty", makefile)
	}
	return t, nil
}

// list adds the packages that Bench.app's binary links for arch, as the
// Makefile's app-bin target builds it.
func (src *sources) list(arch string) error {
	env := []string{"CGO_ENABLED=1", "GOOS=darwin", "GOARCH=" + arch, "GOFLAGS=", "GOWORK=off"}
	out, err := goCommand(src.root, env, "list", "-deps", "-json", "-tags", src.targets.tags, "./cmd/bench")
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p goPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("reading go list's output: %w", err)
		}
		if old := src.packages[p.ImportPath]; old != nil {
			// The architectures can compile different files of a
			// package.
			old.merge(&p)
		} else {
			src.packages[p.ImportPath] = &p
		}
		if m := p.Module; m != nil && !m.Main && src.modules[m.Path] == nil {
			src.modules[m.Path] = m
		}
	}
}

// merge adds the files of another architecture's build of the package.
func (p *goPackage) merge(o *goPackage) {
	add := func(to *[]string, names []string) {
		for _, n := range names {
			if !slices.Contains(*to, n) {
				*to = append(*to, n)
			}
		}
	}
	add(&p.GoFiles, o.GoFiles)
	add(&p.CgoFiles, o.CgoFiles)
	add(&p.CFiles, o.CFiles)
	add(&p.CXXFiles, o.CXXFiles)
	add(&p.MFiles, o.MFiles)
	add(&p.HFiles, o.HFiles)
	add(&p.SFiles, o.SFiles)
	add(&p.SysoFiles, o.SysoFiles)
	add(&p.EmbedFiles, o.EmbedFiles)
}

// goCommand runs the go command in dir, with env added to the environment,
// and returns its standard output.
func goCommand(dir string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

// vendorRow matches a package and its version in frontend/vendor/README.md's
// table, such as "| `@xterm/xterm` 6.0.0 |".
var vendorRow = regexp.MustCompile("(?m)^\\|[^|]*\\| `([^`]+)` ([^ |]+) \\|")

// readVendored reads the packages in frontend/vendor/README.md's table,
// with their versions.
func readVendored(readme string) (map[string]string, error) {
	data, err := os.ReadFile(readme)
	if err != nil {
		return nil, err
	}
	pkgs := map[string]string{}
	for _, m := range vendorRow.FindAllSubmatch(data, -1) {
		pkgs[string(m[1])] = string(m[2])
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("%s's table lists no packages", readme)
	}
	return pkgs, nil
}

// suffixVersion matches the Public Suffix List's revision and date in
// golang.org/x/net/publicsuffix/table.go.
var suffixVersion = regexp.MustCompile(`public_suffix_list\.dat, git revision ([0-9a-f]{40}) \(([^)]+)\)`)

// readSuffixList reads which Public Suffix List golang.org/x/net includes.
func readSuffixList(table string) (suffixList, error) {
	data, err := os.ReadFile(table)
	if err != nil {
		return suffixList{}, err
	}
	m := suffixVersion.FindSubmatch(data)
	if m == nil {
		return suffixList{}, fmt.Errorf("%s doesn't name the Public Suffix List's revision", table)
	}
	published, err := time.Parse(time.RFC3339, string(m[2]))
	if err != nil {
		return suffixList{}, fmt.Errorf("%s: the Public Suffix List's date: %w", table, err)
	}
	return suffixList{revision: string(m[1]), published: published}, nil
}

// path returns the file that a text names, for a component of module m (or
// nil).
func (src *sources) path(t text, m *goModule) (string, error) {
	var dir string
	switch t.From {
	case module:
		if m == nil {
			return "", fmt.Errorf("%s: there's no module to take it from", t.File)
		}
		dir = m.Dir
	case goroot:
		dir = src.goroot
	case repo:
		dir = src.root
	case fixed:
		dir = filepath.Join(src.root, "build", "acknowledgements", "texts")
	}
	return filepath.Join(dir, filepath.FromSlash(t.File)), nil
}
