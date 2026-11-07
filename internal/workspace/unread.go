package workspace

import (
	"maps"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file has the unread counts on sub-tabs and tabs (the unread_counts
// setting, decision 61): the badges that apps show on their Dock icons,
// which the platform reads from the Dock, and the counts that pages put at
// the start of their titles, such as "(3) Inbox".

// maxUnread is the largest count Bench keeps. A longer number is taken as
// this many, and more.
const maxUnread = 999999

// Text returns the count as Bench shows it: "3", "99+", or "•" for a dot,
// or "" for none.
func (u UnreadView) Text() string {
	switch {
	case u.Count > 0 && u.More:
		return strconv.Itoa(u.Count) + "+"
	case u.Count > 0:
		return strconv.Itoa(u.Count)
	case u.Dot:
		return "•"
	}
	return ""
}

// none reports whether u has nothing unread.
func (u UnreadView) none() bool { return u.Count == 0 && !u.Dot }

// add returns the total of u and o: their counts added, or a dot if
// neither has a count and either has a dot.
func (u UnreadView) add(o UnreadView) UnreadView {
	t := UnreadView{Count: min(u.Count+o.Count, maxUnread), More: u.More || o.More || u.Count+o.Count > maxUnread}
	if t.Count == 0 {
		t.More = false
		t.Dot = u.Dot || o.Dot
	}
	return t
}

// parseBadge reads the text of an app's Dock badge: a count, such as "3",
// "1,234", or "99+", or something else, such as "•" or "!", which is a dot.
// An empty badge, or "0", is none.
func parseBadge(label string) UnreadView {
	label = strings.TrimSpace(label)
	if label == "" {
		return UnreadView{}
	}
	if u, rest, ok := leadingCount(label); ok && rest == "" {
		return u
	}
	return UnreadView{Dot: true}
}

// titleCount finds a count at the start of a page's title, in brackets, as
// web mail and chat sites put one: "(3) Inbox", "[12] Chat", or
// "(99+) Inbox". It returns the count and the title without it, and false
// if the title doesn't start with one. "(0)" is a count of none, which the
// title loses too.
func titleCount(title string) (UnreadView, string, bool) {
	t := strings.TrimLeft(title, " ")
	var closing byte
	switch {
	case strings.HasPrefix(t, "("):
		closing = ')'
	case strings.HasPrefix(t, "["):
		closing = ']'
	default:
		return UnreadView{}, title, false
	}
	end := strings.IndexByte(t, closing)
	if end < 2 {
		return UnreadView{}, title, false
	}
	u, rest, ok := leadingCount(t[1:end])
	if !ok || rest != "" {
		return UnreadView{}, title, false
	}
	return u, strings.TrimLeft(t[end+1:], " "), true
}

// leadingCount reads a count at the start of s: digits, which may be
// grouped in threes with commas, full stops, or spaces ("1,234"), and may
// end with "+" for more. It returns the rest of s after it, and false if s
// doesn't start with a digit. A count of 0 is none.
func leadingCount(s string) (UnreadView, string, bool) {
	var digits strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits.WriteByte(c)
			i++
			continue
		case (c == ',' || c == '.' || c == ' ') && digits.Len() > 0 && groupFollows(s[i+1:]):
			i++
			continue
		}
		break
	}
	if digits.Len() == 0 {
		return UnreadView{}, s, false
	}
	u := UnreadView{More: strings.HasPrefix(s[i:], "+")}
	if u.More {
		i++
	}
	if digits.Len() > len(strconv.Itoa(maxUnread)) {
		u.Count, u.More = maxUnread, true
	} else {
		n, _ := strconv.Atoi(digits.String())
		u.Count = min(n, maxUnread)
	}
	if u.Count == 0 {
		u = UnreadView{}
	}
	return u, s[i:], true
}

// groupFollows reports whether s starts with three digits that end a group
// of a number: none follows them but another separator or the number's end.
func groupFollows(s string) bool {
	if len(s) < 3 {
		return false
	}
	for i := range 3 {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	if len(s) == 3 {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[3:])
	return r < '0' || r > '9'
}

// unreadOnLocked reports whether Bench shows unread counts: the
// unread_counts setting is on.
func (e *Engine) unreadOnLocked() bool {
	return e.cfg.Settings.UnreadCounts
}

// dockBadgeWatchChangedLocked returns whether Bench should read the apps'
// Dock badges, and whether that has changed since the platform was last
// told: while it runs with the unread_counts setting on and Accessibility
// permission, which reading the Dock needs, and not in `bench arrange`.
func (e *Engine) dockBadgeWatchChangedLocked() (bool, bool) {
	want := e.started && e.unreadOnLocked() && e.trusted && !e.opts.Headless
	if e.badgeWatchSent && want == e.badgeWatch {
		return want, false
	}
	e.badgeWatch, e.badgeWatchSent = want, true
	if !want {
		e.dockBadges = nil
	}
	return want, true
}

// onDockBadgesChanged takes the badges that the platform read from the
// Dock, by lower-case bundle ID.
func (e *Engine) onDockBadgesChanged() {
	if !e.badgeWatch {
		return
	}
	found := e.p.DockBadges()
	badges := make(map[string]string, len(found))
	for bundleID, label := range found {
		badges[strings.ToLower(bundleID)] = label
	}
	if maps.Equal(badges, e.dockBadges) {
		return
	}
	e.dockBadges = badges
	e.touch()
}

// unreadLocked returns the items' unread counts, by item ID, for the View:
// for the first App item in tab order of each running app with a Dock
// badge, and for each Web item whose page has loaded with a count at the
// start of its title. A page has loaded once its web view has reported its
// title (Item.titled): until then, its title is from the last time Bench
// ran. Bench makes a web view the first time it shows the item, so a page
// in a tab that hasn't been shown since Bench started has no count. Items
// that aren't drawn as sub-tabs (hiddenLocked) get none. It returns nil
// while the unread_counts setting is off.
func (e *Engine) unreadLocked() map[string]UnreadView {
	if !e.unreadOnLocked() {
		return nil
	}
	out := map[string]UnreadView{}
	// first is the item showing each app's badge, by lower-case bundle ID,
	// and others counts the app's other items.
	first := map[string]string{}
	others := map[string]int{}
	for _, it := range e.model.orderedItems() {
		if _, inTab := e.model.Where(it.ID); inTab == nil || e.hiddenLocked(it) {
			continue
		}
		switch it.Kind {
		case KindApp:
			key := strings.ToLower(it.BundleID)
			if _, running := e.running[key]; !running || key == "" {
				continue
			}
			if _, ok := first[key]; ok {
				others[key]++
				continue
			}
			first[key] = it.ID
		case KindWeb:
			if !it.titled {
				continue
			}
			if u, _, ok := titleCount(it.Title); ok && !u.none() {
				out[it.ID] = u
			}
		}
	}
	for key, id := range first {
		u := parseBadge(e.dockBadges[key])
		if u.none() {
			continue
		}
		u.WholeApp = others[key] > 0
		out[id] = u
	}
	return out
}

// unreadTitleLocked returns the title that a Web item's page gives it
// (Item.Title) without the count at its start, which the item's Unread
// shows instead, while the unread_counts setting is on. A title that's the
// count alone gives the item's app's name. The count goes from the title
// even before the page has loaded (Item.titled), as a title from the last
// time Bench ran may have one that's out of date.
func (e *Engine) unreadTitleLocked(it *Item) string {
	if it.Kind != KindWeb || !e.unreadOnLocked() {
		return it.Title
	}
	_, rest, ok := titleCount(it.Title)
	switch {
	case !ok:
		return it.Title
	case rest == "":
		return it.App
	}
	return rest
}

// unreadTotal returns the total of the unread counts of the items with
// the given IDs.
func unreadTotal(unread map[string]UnreadView, ids []string) UnreadView {
	var t UnreadView
	for _, id := range ids {
		t = t.add(unread[id])
	}
	return t
}
