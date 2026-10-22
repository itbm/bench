package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"os"
	"regexp"
	"slices"
	"strings"
)

// An entry is a component as the page shows it, with its texts read.
type entry struct {
	// ID is the section's id, which the contents link to.
	ID   string
	Name string
	// About says what it is, under its licence and address: "The standard
	// library and runtime · BSD 3-Clause License · go.dev".
	About string
	Note  string
	Texts []string
	Parts []entry
}

// build checks the components against the sources, and reads their texts.
func build(src *sources) ([]entry, error) {
	errs := slices.Concat(checkModules(src), checkVendored(src), checkNames())
	var entries []entry
	for _, c := range components {
		e, err := src.entry(c, src.modules[c.Module], "")
		if err != nil {
			errs = append(errs, err)
			continue
		}
		entries = append(entries, e)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	sortEntries(entries)
	return entries, nil
}

// checkModules reports a third-party module that Bench.app links but that
// isn't a component, and a component's module that it doesn't link.
func checkModules(src *sources) []error {
	var errs []error
	listed := map[string]bool{}
	for _, c := range components {
		if c.Module == "" {
			continue
		}
		listed[c.Module] = true
		if src.modules[c.Module] == nil {
			errs = append(errs, fmt.Errorf("components lists %s, which Bench.app no longer links: remove it, then run make acknowledgements", c.Module))
		}
	}
	for _, path := range sortedKeys(src.modules) {
		if !listed[path] {
			errs = append(errs, fmt.Errorf("Bench.app links %s %s, which components doesn't list: check its licence, and any code it includes from other projects, add it to components in build/acknowledgements/components.go, then run make acknowledgements",
				path, src.modules[path].Version))
		}
	}
	return errs
}

// checkVendored reports a vendored component whose package or version
// isn't in frontend/vendor/README.md's table, and a package in the table
// that isn't a component.
func checkVendored(src *sources) []error {
	var errs []error
	listed := map[string]bool{}
	for _, c := range components {
		if c.Package == "" {
			continue
		}
		listed[c.Package] = true
		if v, ok := src.vendored[c.Package]; !ok {
			errs = append(errs, fmt.Errorf("components lists %s, which frontend/vendor/README.md doesn't: remove it, then run make acknowledgements", c.Package))
		} else if v != c.Version {
			errs = append(errs, fmt.Errorf("components has %s %s, but frontend/vendor/README.md has %s: change its Version, check what it includes from other projects, then run make acknowledgements",
				c.Package, c.Version, v))
		}
	}
	for _, pkg := range sortedKeys(src.vendored) {
		if !listed[pkg] {
			errs = append(errs, fmt.Errorf("frontend/vendor/README.md lists %s, which components doesn't: check its licence, add it to components in build/acknowledgements/components.go, then run make acknowledgements", pkg))
		}
	}
	return errs
}

// checkNames reports components or parts with the same section id.
func checkNames() []error {
	var errs []error
	seen := map[string]bool{}
	var walk func(cs []component, parent string)
	walk = func(cs []component, parent string) {
		for _, c := range cs {
			id := idOf(c.Name, parent)
			if seen[id] {
				errs = append(errs, fmt.Errorf("more than one component has the id %q", id))
			}
			seen[id] = true
			walk(c.Parts, id)
		}
	}
	walk(components, "")
	return errs
}

// entry reads a component's texts, and its parts', which take their texts
// from the component's module, m.
func (src *sources) entry(c component, m *goModule, parent string) (entry, error) {
	e := entry{ID: idOf(c.Name, parent), Name: c.Name, Note: src.expand(c.Note)}
	var about []string
	switch {
	case c.Module != "":
		if m == nil {
			return entry{}, fmt.Errorf("%s: Bench.app doesn't link %s", c.Name, c.Module)
		}
		about = append(about, m.Path+" "+m.Version)
	case c.Package != "":
		about = append(about, c.Package+" "+c.Version)
	default:
		about = append(about, c.About)
	}
	about = append(about, c.Licence)
	if c.Address != "" {
		about = append(about, c.Address)
	}
	e.About = strings.Join(about, " · ")

	if c.Evidence != nil {
		if err := src.checkEvidence(*c.Evidence, m, c.Name); err != nil {
			return entry{}, err
		}
	}
	for _, t := range c.Texts {
		s, err := src.read(t, m)
		if err != nil {
			return entry{}, fmt.Errorf("%s: %w", c.Name, err)
		}
		e.Texts = append(e.Texts, s)
	}
	if len(e.Texts) == 0 {
		return entry{}, fmt.Errorf("%s has no texts", c.Name)
	}
	for _, p := range c.Parts {
		pe, err := src.entry(p, m, e.ID)
		if err != nil {
			return entry{}, err
		}
		e.Parts = append(e.Parts, pe)
	}
	return e, nil
}

// expand fills in a note's {suffix-revision} and {suffix-published}.
func (src *sources) expand(note string) string {
	return strings.NewReplacer(
		"{suffix-revision}", src.suffixes.revision,
		"{suffix-published}", src.suffixes.published.Format("2 January 2006"),
	).Replace(note)
}

// checkEvidence checks that a part's evidence is still there.
func (src *sources) checkEvidence(t text, m *goModule, name string) error {
	path, err := src.path(t, m)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !bytes.Contains(data, []byte(t.Contains)) {
		return fmt.Errorf("%s: %s no longer contains %q: check whether Bench.app still includes %s, and update components", name, t.File, t.Contains, name)
	}
	return nil
}

// read returns a text, tidied.
func (src *sources) read(t text, m *goModule) (string, error) {
	path, err := src.path(t, m)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	switch {
	case t.Leading:
		s = leadingComments(s)
	case t.First != "":
		if s, err = commentBlock(s, t.First, t.Last); err != nil {
			return "", fmt.Errorf("%s: %w", t.File, err)
		}
	}
	s = tidy(s)
	if s == "" {
		return "", fmt.Errorf("%s: the text is empty", t.File)
	}
	return s, nil
}

// commentBlock returns the comment lines from the one containing first to
// the next one containing last, without their comment markers or
// indentation.
func commentBlock(s, first, last string) (string, error) {
	lines := strings.Split(s, "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, first) })
	if start < 0 {
		return "", fmt.Errorf("no line contains %q", first)
	}
	end := slices.IndexFunc(lines[start:], func(l string) bool { return strings.Contains(l, last) })
	if end < 0 {
		return "", fmt.Errorf("no line after %q contains %q", first, last)
	}
	block := lines[start : start+end+1]
	out := make([]string, len(block))
	for i, l := range block {
		out[i] = strings.TrimSpace(stripMarker(strings.TrimSpace(l)))
	}
	return strings.Join(out, "\n"), nil
}

// stripMarker removes a comment marker from the start of a line that has
// no indentation: //, /*, or the * that starts a block comment's lines.
func stripMarker(l string) string {
	for _, marker := range []string{"//", "/*", "*"} {
		if strings.HasPrefix(l, marker) && !strings.HasPrefix(l, "*/") {
			return l[len(marker):]
		}
	}
	return l
}

// decoration matches a comment line that holds only decoration, such as
// /** or *---*/.
var decoration = regexp.MustCompile(`^[\s/*=-]*$`)

// leadingComments returns the block comments at the top of a file, such as
// a bundle's notices, without their markers, with a blank line between
// them.
func leadingComments(s string) string {
	var blocks []string
	rest := strings.TrimLeft(s, " \t\n")
	for strings.HasPrefix(rest, "/*") {
		end := strings.Index(rest, "*/")
		if end < 0 {
			break
		}
		var lines []string
		for _, l := range strings.Split(rest[:end+2], "\n") {
			if decoration.MatchString(l) {
				lines = append(lines, "")
				continue
			}
			l = strings.TrimSuffix(strings.TrimRight(l, " \t"), "*/")
			lines = append(lines, stripMarker(strings.TrimLeft(l, " \t")))
		}
		blocks = append(blocks, tidy(dedent(lines)))
		rest = strings.TrimLeft(rest[end+2:], " \t\n")
	}
	return strings.Join(blocks, "\n\n")
}

// dedent removes the indentation that all of the lines that aren't blank
// share.
func dedent(lines []string) string {
	indent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if n := len(l) - len(strings.TrimLeft(l, " ")); indent < 0 || n < indent {
			indent = n
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= indent && indent > 0 {
			l = l[indent:]
		}
		out[i] = l
	}
	return strings.Join(out, "\n")
}

// tidy removes the spaces at the ends of lines, and the blank lines at the
// start and end.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// nonWord matches what a section id leaves out of a name.
var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// idOf returns the section id for a component's name, after its parent's
// id for a part: "golang.org/x/net" is golang-org-x-net.
func idOf(name, parent string) string {
	id := strings.Trim(nonWord.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if parent != "" {
		id = parent + "-" + id
	}
	return id
}

// sortEntries sorts entries and their parts by name, ignoring case.
func sortEntries(entries []entry) {
	slices.SortFunc(entries, func(a, b entry) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	for i := range entries {
		sortEntries(entries[i].Parts)
	}
}

// sortedKeys returns a map's keys in order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// pageComment heads the page, in place of a comment in the template, which
// html/template would drop.
const pageComment = `<!--
  Bench's Acknowledgements window: the open source software that Bench.app
  includes, each with its notices and licence. build/acknowledgements writes
  this file, so don't edit it: run make acknowledgements instead.
-->`

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en-GB">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Acknowledgements</title>
{{.Comment}}
<link rel="stylesheet" href="vendor/fonts/fonts.css">
<link rel="stylesheet" href="styles.css">
</head>
<body class="is-acknowledgements">
<main class="ack">
<h1>Acknowledgements</h1>
<p class="ack-intro">Bench includes the open source software listed here. Each one's copyright notices and licence follow its name, along with those of any code it took from other projects.</p>
<nav class="ack-contents" aria-label="Contents">
<ul>
{{- range .Entries}}
<li><a href="#{{.ID}}">{{.Name}}</a></li>
{{- end}}
</ul>
</nav>
{{- range .Entries}}
<section class="ack-entry" id="{{.ID}}" aria-labelledby="{{.ID}}-name">
<h2 id="{{.ID}}-name">{{.Name}}</h2>
{{- template "body" .}}
{{- range .Parts}}
<section class="ack-part" id="{{.ID}}" aria-labelledby="{{.ID}}-name">
<h3 id="{{.ID}}-name">{{.Name}}</h3>
{{- template "body" .}}
</section>
{{- end}}
</section>
{{- end}}
</main>
</body>
</html>
{{define "body"}}
<p class="ack-about">{{.About}}</p>
{{- if .Note}}
<p class="ack-note">{{.Note}}</p>
{{- end}}
{{- range .Texts}}
<pre class="ack-text">{{.}}</pre>
{{- end}}
{{- end}}`))

// render returns the page.
func render(entries []entry) ([]byte, error) {
	var buf bytes.Buffer
	data := struct {
		Comment template.HTML
		Entries []entry
	}{template.HTML(pageComment), entries}
	if err := page.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
