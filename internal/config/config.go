package config

import (
	"fmt"
	"strings"
)

// Clone returns a deep copy of the config. Empty lists stay empty and
// missing ones stay nil.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	out := &Config{
		Settings:  c.Settings,
		Shortcuts: c.Shortcuts,
		Launchers: cloneSlice(c.Launchers),
		Tabs:      cloneSlice(c.Tabs),
		Rules:     cloneSlice(c.Rules),
	}
	for i := range out.Launchers {
		out.Launchers[i].StartAgain = cloneBool(out.Launchers[i].StartAgain)
	}
	for i := range out.Tabs {
		out.Tabs[i] = out.Tabs[i].Clone()
	}
	return out
}

// Clone returns a deep copy of a [[tab]] entry.
func (t Tab) Clone() Tab {
	t.Sizes = cloneSlice(t.Sizes)
	t.Panes = cloneSlice(t.Panes)
	for j := range t.Panes {
		items := cloneSlice(t.Panes[j].Items)
		for k := range items {
			items[k].StartAgain = cloneBool(items[k].StartAgain)
			items[k].KeepRunning = cloneBool(items[k].KeepRunning)
		}
		t.Panes[j].Items = items
	}
	return t
}

// cloneBool returns a copy of an optional setting, such as start_again.
func cloneBool(b *bool) *bool {
	if b == nil {
		return nil
	}
	v := *b
	return &v
}

func cloneSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	return append(make([]T, 0, len(s)), s...)
}

// TabIndex returns the index of the tab with the given name, or -1. An exact
// match wins; otherwise it ignores case, as tab names are unique ignoring
// case. A nil config has no tabs.
func (c *Config) TabIndex(name string) int {
	if c == nil {
		return -1
	}
	for i, t := range c.Tabs {
		if t.Name == name {
			return i
		}
	}
	for i, t := range c.Tabs {
		if strings.EqualFold(t.Name, name) {
			return i
		}
	}
	return -1
}

// LauncherIndex returns the index of the launcher with the given name,
// ignoring case, as launcher names are unique ignoring case, or -1. An exact
// match wins. A nil config has no launchers.
func (c *Config) LauncherIndex(name string) int {
	if c == nil {
		return -1
	}
	for i, l := range c.Launchers {
		if l.Name == name {
			return i
		}
	}
	for i, l := range c.Launchers {
		if strings.EqualFold(l.Name, name) {
			return i
		}
	}
	return -1
}

// TabNames returns the names of the config's tabs, in order.
func (c *Config) TabNames() []string {
	out := make([]string, len(c.Tabs))
	for i, t := range c.Tabs {
		out[i] = t.Name
	}
	return out
}

// Problems is a list of problems as an error. Save returns one when it
// refuses a config; use errors.As to get the list.
type Problems []Problem

// Error lists the problems on one line, separated by semicolons, after a
// count when there's more than one.
func (ps Problems) Error() string {
	switch len(ps) {
	case 0:
		return "no problems"
	case 1:
		return ps[0].Error()
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.Error()
	}
	return fmt.Sprintf("%d problems: %s", len(ps), strings.Join(parts, "; "))
}
