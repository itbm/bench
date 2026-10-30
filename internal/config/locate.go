package config

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// Table kinds, for knowing which fields belong where.
const (
	kindNone      = ""
	kindTop       = "top"
	kindSettings  = "settings"
	kindShortcuts = "shortcuts"
	kindTab       = "tab"
	kindPane      = "pane"
	kindItem      = "item"
	kindRule      = "rule"
	kindLauncher  = "launcher"
)

// childKind returns the kind of the table under key in a table of kind
// parent, or kindNone if it isn't one Bench knows.
func childKind(parent, key string) string {
	switch {
	case parent == kindTop && key == "settings":
		return kindSettings
	case parent == kindTop && key == "shortcuts":
		return kindShortcuts
	case parent == kindTop && key == "tab":
		return kindTab
	case parent == kindTop && key == "rule":
		return kindRule
	case parent == kindTop && key == "launcher":
		return kindLauncher
	case parent == kindTab && key == "pane":
		return kindPane
	case parent == kindPane && key == "items":
		return kindItem
	}
	return kindNone
}

// knownFields lists the fields of each kind of table, from the struct tags.
var knownFields = map[string][]string{
	kindTop:       tomlKeys(reflect.TypeFor[Config]()),
	kindSettings:  tomlKeys(reflect.TypeFor[Settings]()),
	kindShortcuts: tomlKeys(reflect.TypeFor[Shortcuts]()),
	kindTab:       tomlKeys(reflect.TypeFor[Tab]()),
	kindPane:      tomlKeys(reflect.TypeFor[Pane]()),
	kindItem:      tomlKeys(reflect.TypeFor[Item]()),
	kindRule:      tomlKeys(reflect.TypeFor[Rule]()),
	kindLauncher:  tomlKeys(reflect.TypeFor[Launcher]()),
}

func tomlKeys(t reflect.Type) []string {
	var keys []string
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("toml"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// suggest returns the known field of a kind of table that's closest to
// name, if one is close enough to be a likely typo.
func suggest(kind, name string) string {
	best, bestDist := "", 3
	for _, k := range knownFields[kind] {
		if d := editDistance(strings.ToLower(name), k); d < bestDist && d < len(k) {
			best, bestDist = k, d
		}
	}
	return best
}

// editDistance is the Levenshtein distance between a and b, in bytes.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// A span is the part of a config file from start up to the next span's
// start, and what it is in the config.
type span struct {
	start int
	// tab is the [[tab]] it's in, counting from 0, or -1 outside tabs.
	tab int
	// path is the dotted field path: within the tab, or from the top of the
	// file outside tabs. Indexes count from 1.
	path string
	// kind is the kind of table the span is (or holds, for an array), and
	// parent the kind of table its field belongs to.
	kind, parent string
}

// locator finds where an offset in a config file is in the config, so that
// decoding problems can name the tab and field.
type locator struct {
	doc   []byte
	spans []span   // In document order.
	names []string // Each [[tab]]'s name, or "".
	// nested holds arrays inside arrays, such as the [1, 2] in
	// sizes = [[1, 2]]. go-toml gives them no position, so its errors about
	// them point at the start of the file; these let Parse find them by key
	// instead. Each starts at its first element, or at its key if it has
	// none.
	nested []span

	// walked is true once the whole file has been walked without a syntax
	// error. Until then, ctx is the table the walk is in, and next is
	// where the line after the last expression walked starts.
	walked bool
	ctx    span
	next   int

	tab, pane, item, rule, launcher int // Counters while walking.
}

// newLocator walks a config file's syntax tree. If the file has a syntax
// error, the spans stop there. Locating is best effort: anything unexpected
// leaves the spans found so far.
func newLocator(doc []byte) (l *locator) {
	l = &locator{doc: doc, tab: -1, ctx: span{tab: -1, kind: kindTop, parent: kindNone}}
	defer func() {
		// The unstable API panics if it's misused; that mustn't stop Parse
		// reporting the problem it's locating.
		_ = recover()
	}()
	var p unstable.Parser
	p.Reset(doc)
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.Table, unstable.ArrayTable:
			l.ctx = l.header(e)
			l.spans = append(l.spans, l.ctx)
			_, start := keyParts(e)
			l.next = l.lineEnd(start)
		case unstable.KeyValue:
			ctx := l.ctx
			s := l.keyValue(ctx, e)
			if ctx.kind == kindTab && ctx.path == "" && s.path == "name" && ctx.tab >= 0 && l.names[ctx.tab] == "" {
				if v := e.Value(); v.Kind == unstable.String {
					l.names[ctx.tab] = string(v.Data)
				}
			}
			l.next = l.lineEnd(int(e.Raw.Offset + e.Raw.Length))
		}
	}
	l.walked = p.Error() == nil
	return l
}

// lineEnd returns the offset just after the newline that ends the line
// holding offset, or the end of the file.
func (l *locator) lineEnd(offset int) int {
	if offset < 0 || offset >= len(l.doc) {
		return len(l.doc)
	}
	nl := bytes.IndexByte(l.doc[offset:], '\n')
	if nl < 0 {
		return len(l.doc)
	}
	return offset + nl + 1
}

// bareKeyRE matches a bare (possibly dotted) key and its equals sign at the
// start of a key-value expression.
var bareKeyRE = regexp.MustCompile(`^([A-Za-z0-9_-]+(?:[ \t]*\.[ \t]*[A-Za-z0-9_-]+)*)[ \t]*=`)

// failedKey returns the field of the expression the walk stopped at, when
// it's a key-value whose key and equals sign read correctly and the syntax
// error at offset comes after them, as in "gap = " with no value. It
// returns false when the whole file was walked or the expression isn't a
// key-value.
func (l *locator) failedKey(offset int) (string, bool) {
	if l.walked || offset < 0 {
		return "", false
	}
	start := skipBlank(l.doc, l.next)
	if start > offset {
		return "", false
	}
	m := bareKeyRE.FindSubmatch(l.doc[start:])
	if m == nil || start+len(m[0])-1 > offset {
		return "", false
	}
	key := strings.NewReplacer(" ", "", "\t", "").Replace(string(m[1]))
	return joinPath(l.ctx.path, key), true
}

// skipBlank returns the offset of the first byte from offset on that isn't
// white space, a line break, or in a comment.
func skipBlank(doc []byte, offset int) int {
	for offset < len(doc) {
		switch doc[offset] {
		case ' ', '\t', '\r', '\n':
			offset++
		case '#':
			nl := bytes.IndexByte(doc[offset:], '\n')
			if nl < 0 {
				return len(doc)
			}
			offset += nl + 1
		default:
			return offset
		}
	}
	return offset
}

func keyParts(n *unstable.Node) (parts []string, start int) {
	start = -1
	it := n.Key()
	for it.Next() {
		k := it.Node()
		if start < 0 {
			start = int(k.Raw.Offset)
		}
		parts = append(parts, string(k.Data))
	}
	return parts, start
}

// header returns the span for a [table] or [[array table]] header.
func (l *locator) header(e *unstable.Node) span {
	parts, start := keyParts(e)
	key := strings.Join(parts, ".")
	if e.Kind == unstable.ArrayTable {
		switch {
		case key == "tab":
			l.tab++
			l.pane, l.item = 0, 0
			l.names = append(l.names, "")
			return span{start: start, tab: l.tab, kind: kindTab, parent: kindTop}
		case key == "tab.pane" && l.tab >= 0:
			l.pane++
			l.item = 0
			return span{start: start, tab: l.tab, path: fmt.Sprintf("pane[%d]", l.pane), kind: kindPane, parent: kindTab}
		case key == "tab.pane.items" && l.tab >= 0 && l.pane > 0:
			l.item++
			return span{start: start, tab: l.tab, path: fmt.Sprintf("pane[%d].items[%d]", l.pane, l.item), kind: kindItem, parent: kindPane}
		case key == "rule":
			l.rule++
			return span{start: start, tab: -1, path: fmt.Sprintf("rule[%d]", l.rule), kind: kindRule, parent: kindTop}
		case key == "launcher":
			l.launcher++
			return span{start: start, tab: -1, path: fmt.Sprintf("launcher[%d]", l.launcher), kind: kindLauncher, parent: kindTop}
		}
	}
	// A plain table, or an array table Bench doesn't expect: follow the key
	// through the tables it knows.
	s := span{start: start, tab: -1, kind: kindTop}
	var path []string
	for _, part := range parts {
		s.parent = s.kind
		switch {
		case s.kind == kindTop && part == "tab" && l.tab >= 0:
			s.tab, s.kind = l.tab, kindTab
			continue // Paths within a tab start after it.
		case s.kind == kindTab && part == "pane" && l.pane > 0:
			part = fmt.Sprintf("pane[%d]", l.pane)
		case s.kind == kindPane && part == "items" && l.item > 0:
			part = fmt.Sprintf("items[%d]", l.item)
		case s.kind == kindTop && part == "rule" && l.rule > 0:
			part = fmt.Sprintf("rule[%d]", l.rule)
		case s.kind == kindTop && part == "launcher" && l.launcher > 0:
			part = fmt.Sprintf("launcher[%d]", l.launcher)
		}
		s.kind = childKind(s.kind, strings.SplitN(part, "[", 2)[0])
		path = append(path, part)
	}
	s.path = strings.Join(path, ".")
	return s
}

// keyValue records spans for a key-value expression in table ctx, and for
// everything in its value.
func (l *locator) keyValue(ctx span, e *unstable.Node) span {
	parts, _ := keyParts(e)
	s := ctx
	s.start = int(e.Raw.Offset)
	for _, part := range parts {
		s.parent = s.kind
		s.path = joinPath(s.path, part)
		s.kind = childKind(s.kind, part)
	}
	l.spans = append(l.spans, s)
	l.value(s, e.Value())
	return s
}

// value records spans for the elements of an array and the fields of an
// inline table.
func (l *locator) value(parent span, v *unstable.Node) {
	switch v.Kind {
	case unstable.Array:
		k := 0
		for it := v.Children(); it.Next(); {
			c := it.Node()
			if c.Kind == unstable.Comment {
				continue
			}
			k++
			s := parent
			s.path = fmt.Sprintf("%s[%d]", parent.path, k)
			s.parent = kindNone
			switch {
			case c.Raw.Length > 0:
				s.start = int(c.Raw.Offset)
				l.spans = append(l.spans, s)
			case c.Kind == unstable.Array:
				// Nested arrays have no range of their own; their elements
				// do.
				n := s
				n.start = firstOffset(c, parent.start)
				l.nested = append(l.nested, n)
			}
			l.value(s, c)
		}
	case unstable.InlineTable:
		for it := v.Children(); it.Next(); {
			if kv := it.Node(); kv.Kind == unstable.KeyValue {
				l.keyValue(parent, kv)
			}
		}
	}
}

// firstOffset returns the offset of the first thing in an array that has a
// range, or fallback if nothing does.
func firstOffset(n *unstable.Node, fallback int) int {
	for it := n.Children(); it.Next(); {
		c := it.Node()
		if c.Raw.Length > 0 {
			return int(c.Raw.Offset)
		}
		if off := firstOffset(c, -1); off >= 0 {
			return off
		}
	}
	return fallback
}

// indexRE matches the indexes in a field path.
var indexRE = regexp.MustCompile(`\[\d+\]`)

// key returns a span's TOML key without indexes, as go-toml's
// DecodeError.Key gives it, such as "tab.pane.items".
func (s span) key() string {
	k := indexRE.ReplaceAllString(s.path, "")
	if s.tab >= 0 {
		k = joinPath("tab", k)
	}
	return k
}

// nestedFor returns the first array inside an array whose key is key.
func (l *locator) nestedFor(key []string) (span, bool) {
	want := strings.Join(key, ".")
	for _, s := range l.nested {
		if s.key() == want {
			return s, true
		}
	}
	return span{tab: -1}, false
}

func joinPath(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

// offset converts a 1-based line and byte column to an offset, or -1.
func (l *locator) offset(line, column int) int {
	if line < 1 || column < 1 {
		return -1
	}
	off := 0
	for i := 1; i < line; i++ {
		nl := bytes.IndexByte(l.doc[off:], '\n')
		if nl < 0 {
			return -1
		}
		off += nl + 1
	}
	off += column - 1
	if off > len(l.doc) {
		return -1
	}
	return off
}

// at returns the span holding an offset.
func (l *locator) at(offset int) (span, bool) {
	i := sort.Search(len(l.spans), func(i int) bool { return l.spans[i].start > offset })
	if i == 0 || offset < 0 {
		return span{tab: -1}, false
	}
	return l.spans[i-1], true
}

// find returns the first span with a path outside tabs, such as
// "settings.launch_timeout".
func (l *locator) find(path string) (span, bool) {
	for _, s := range l.spans {
		if s.tab < 0 && s.path == path {
			return s, true
		}
	}
	return span{tab: -1}, false
}

// position returns the 1-based line and column of an offset.
func (l *locator) position(offset int) (line, column int) {
	line, lineStart := 1, 0
	for i := 0; i < offset && i < len(l.doc); i++ {
		if l.doc[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return line, offset - lineStart + 1
}

// tabLabel names the tab a span is in, or returns "" outside tabs.
func (l *locator) tabLabel(s span) string {
	if s.tab < 0 {
		return ""
	}
	name := ""
	if s.tab < len(l.names) {
		name = l.names[s.tab]
	}
	return tabLabel(name, s.tab)
}
