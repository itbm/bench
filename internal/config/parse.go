package config

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Parse reads config.toml. Missing settings and shortcuts take their default
// values; missing tabs don't (a file with no [[tab]] is a problem). Syntax
// errors and unknown fields are returned as problems (with the tab and field
// where possible, and the line and column), as are validation problems; err
// is only for failures that aren't the file's fault. If there are problems,
// cfg is nil.
//
// A wrong type, such as gap = "8", stops decoding, so it's reported on its
// own. Unknown fields don't, so they're reported with any validation
// problems.
func Parse(data []byte) (cfg *Config, problems []Problem, err error) {
	defer func() {
		if r := recover(); r != nil {
			cfg, problems, err = nil, nil, fmt.Errorf("parsing the config: internal error: %v", r)
		}
	}()

	// Some editors start UTF-8 files with a byte order mark, which TOML
	// doesn't allow.
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	// Reading into a map first separates TOML syntax errors from problems
	// with Bench's fields, and records which item fields are present.
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, []Problem{newLocator(data).syntaxProblem(err)}, nil
	}

	def := Default()
	c := &Config{Settings: def.Settings, Shortcuts: def.Shortcuts}
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		loc := newLocator(data)
		var strict *toml.StrictMissingError
		if !errors.As(err, &strict) {
			return nil, []Problem{loc.decodeProblem(err)}, nil
		}
		for i := range strict.Errors {
			problems = append(problems, loc.unknownProblem(&strict.Errors[i]))
		}
	}
	tidy(c)

	problems = append(problems, validate(c, presenceOf(raw))...)
	if len(problems) > 0 {
		return nil, problems, nil
	}
	return c, nil, nil
}

// tidy makes empty lists nil, so that a parsed config compares equal to one
// built in Go.
func tidy(c *Config) {
	if len(c.Tabs) == 0 {
		c.Tabs = nil
	}
	if len(c.Rules) == 0 {
		c.Rules = nil
	}
	if len(c.Launchers) == 0 {
		c.Launchers = nil
	}
	for i := range c.Tabs {
		t := &c.Tabs[i]
		if len(t.Sizes) == 0 {
			t.Sizes = nil
		}
		if len(t.Panes) == 0 {
			t.Panes = nil
		}
		for j := range t.Panes {
			if len(t.Panes[j].Items) == 0 {
				t.Panes[j].Items = nil
			}
		}
	}
}

// presenceOf records which kind fields each item in a decoded file sets.
// go-toml matches keys to fields in lower case, so that Web sets the web
// field, and so does this: otherwise { Web = "…", app = "…" } would
// pass here as an app item, yet fail every later Validate.
func presenceOf(raw map[string]any) presence {
	out := presence{}
	tabs, _ := lookupFold(raw, "tab").([]any)
	for ti, t := range tabs {
		tm, _ := t.(map[string]any)
		panes, _ := lookupFold(tm, "pane").([]any)
		for pi, p := range panes {
			pm, _ := p.(map[string]any)
			items, _ := lookupFold(pm, "items").([]any)
			for ii, it := range items {
				im, _ := it.(map[string]any)
				var keys []string
				for _, k := range kindKeys {
					if lookupFold(im, k) != nil {
						keys = append(keys, k)
					}
				}
				out[itemRef{tab: ti, pane: pi, item: ii}] = keys
			}
		}
	}
	return out
}

// lookupFold returns the value of the lower-case key in m, or nil if there's
// none, matching keys as go-toml does: in lower case, so that WEB and Web
// match web. TOML values are never nil.
func lookupFold(m map[string]any, key string) any {
	if v, ok := m[key]; ok {
		return v
	}
	for k, v := range m {
		if strings.ToLower(k) == key {
			return v
		}
	}
	return nil
}

// syntaxProblem turns an error from reading a file as TOML into a problem.
func (l *locator) syntaxProblem(err error) Problem {
	var de *toml.DecodeError
	if !errors.As(err, &de) {
		return Problem{Message: plainMessage(err)}
	}
	line, col := de.Position()
	off := l.offset(line, col)
	s, ok := l.at(off)
	p := Problem{
		Tab:     l.tabLabel(s),
		Message: fmt.Sprintf("line %d, column %d: %s", line, col, plainMessage(de)),
	}
	switch {
	case l.walked && ok:
		// The syntax is fine, but go-toml found a key defined twice, a
		// number out of range, or similar, in a part Bench walked.
		p.Field = s.path
	case !l.walked:
		// The walk stopped at the same error, after the span. If the
		// expression it stopped at starts with a key, that's the field.
		if field, found := l.failedKey(off); found {
			p.Tab = l.tabLabel(l.ctx)
			p.Field = field
		}
	}
	return p
}

// decodeProblem turns an error from decoding a file into a Config, such as
// a wrong type, into a problem.
func (l *locator) decodeProblem(err error) Problem {
	var de *toml.DecodeError
	if errors.As(err, &de) {
		line, col := de.Position()
		off := l.offset(line, col)
		s, _ := l.at(off)
		if off == 0 {
			// go-toml points at the start of the file when the value is
			// an array inside an array, as in sizes = [[50, 50]].
			if n, ok := l.nestedFor(de.Key()); ok {
				s = n
				line, col = l.position(n.start)
			}
		}
		return Problem{
			Tab:     l.tabLabel(s),
			Field:   s.path,
			Message: fmt.Sprintf("%s (line %d, column %d)", plainMessage(de), line, col),
		}
	}
	var dur *durationError
	if errors.As(err, &dur) {
		// A launch_timeout that isn't a string reaches Duration without a
		// position, so find it.
		const field = "settings.launch_timeout"
		p := Problem{Field: field, Message: dur.Error()}
		if s, ok := l.find(field); ok {
			line, col := l.position(s.start)
			p.Message += fmt.Sprintf(" (line %d, column %d)", line, col)
		}
		return p
	}
	return Problem{Message: plainMessage(err)}
}

// unknownProblem turns a strict-mode error into a problem.
func (l *locator) unknownProblem(de *toml.DecodeError) Problem {
	line, col := de.Position()
	s, _ := l.at(l.offset(line, col))
	what := "field"
	if strings.Contains(de.Error(), "missing table") {
		what = "table"
	}
	msg := fmt.Sprintf("unknown %s (line %d, column %d)", what, line, col)
	key := de.Key()
	if len(key) > 0 {
		if hint := suggest(s.parent, key[len(key)-1]); hint != "" {
			msg += fmt.Sprintf("; did you mean %q?", hint)
		}
	}
	field := s.path
	if field == "" {
		field = strings.Join(key, ".")
	}
	return Problem{Tab: l.tabLabel(s), Field: field, Message: msg}
}

var mismatchRE = regexp.MustCompile(`^cannot decode TOML (.+?) into (?:struct field \S+ of type )?(\S+)$`)

// tomlTypeWords describes the TOML types go-toml names in its errors.
var tomlTypeWords = map[string]string{
	"string":         "text",
	"integer":        "a number",
	"float":          "a number",
	"boolean":        "true or false",
	"array":          "a list",
	"inline table":   "a table",
	"table":          "a table",
	"datetime":       "a date and time",
	"local datetime": "a date and time",
	"local date":     "a date",
	"local time":     "a time",
}

// goTypeWords describes the Go types of Config's fields.
var goTypeWords = map[string]string{
	"string":            "text in quotes",
	"float64":           "a number",
	"bool":              "true or false",
	"*bool":             "true or false",
	"config.Duration":   "a duration in quotes, such as \"15s\"",
	"[]float64":         "a list of numbers, such as [50, 50]",
	"config.Settings":   "the [settings] table",
	"shortcuts.Config":  "the [shortcuts] table",
	"[]config.Tab":      "[[tab]] tables",
	"config.Tab":        "a [[tab]] table",
	"[]config.Pane":     "[[tab.pane]] tables",
	"config.Pane":       "a [[tab.pane]] table",
	"[]config.Item":     "a list of items, such as [{ app = \"dev.zed.Zed\" }]",
	"config.Item":       "an item, such as { app = \"dev.zed.Zed\" }",
	"[]config.Rule":     "[[rule]] tables",
	"config.Rule":       "a [[rule]] table",
	"[]config.Launcher": "[[launcher]] tables",
	"config.Launcher":   "a [[launcher]] table",
}

// plainMessage returns a go-toml error's message without its "toml: "
// prefix, with type mismatches in plain words.
func plainMessage(err error) string {
	msg := strings.TrimPrefix(err.Error(), "toml: ")
	if m := mismatchRE.FindStringSubmatch(msg); m != nil {
		got, ok1 := tomlTypeWords[m[1]]
		want, ok2 := goTypeWords[m[2]]
		if ok1 && ok2 {
			return fmt.Sprintf("must be %s, not %s", want, got)
		}
	}
	return msg
}
