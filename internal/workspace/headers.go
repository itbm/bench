package workspace

import (
	"time"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// This file has how much room Bench's own chrome takes: the density, which
// sizes the title bar, pane headers, tabs, and sub-tabs, and which panes
// have a header at all. The "tabs" view's one pane never has one: its
// sub-tabs are in the title bar. With pane_headers = "auto", a pane that
// shows one App window, with nothing to report, has none either, so that
// the app's own title bar stands in for it, unless the pane is zoomed: its
// header says so, and shows every pane again when clicked.

const (
	// headerShowDelay is how long a pane without its header must have
	// something to report before the header comes back, so that a status
	// that comes and goes at once, as when an app resizes its own window,
	// doesn't move the window down and up again. A second window brings
	// it back at once, since its sub-tabs are needed.
	headerShowDelay = time.Second
	// headerHideDelay is how long a pane must show its one App window, with
	// nothing to report, before its header goes: the least time a header
	// stays once it has come back.
	headerHideDelay = 2 * time.Second
)

// paneHeader is whether a pane goes without its header, under
// pane_headers = "auto", as last arranged.
type paneHeader struct {
	hidden bool
	// since is when the pane began to want the other way, or zero while it
	// wants what it has.
	since time.Time
}

// densityFor returns the density in use, standard or compact, for the
// density setting: auto is compact in the compact size class.
func densityFor(setting string, class layout.SizeClass) string {
	switch setting {
	case config.DensityStandard, config.DensityCompact:
		return setting
	}
	if class == layout.Compact {
		return config.DensityCompact
	}
	return config.DensityStandard
}

// densityLocked returns the density in use now.
func (e *Engine) densityLocked() string {
	return densityFor(e.cfg.Settings.Density, e.sizeClassLocked())
}

// titleBarLocked returns the density of Bench's title bar. In the app, the
// window's native title bar, where the traffic lights sit, is made at
// launch (WindowStart), and the title bar keeps the density it had then
// until Bench opens again. Without one, as in the browser preview, it
// follows the density in use.
func (e *Engine) titleBarLocked() string {
	if e.titleBar != "" {
		return e.titleBar
	}
	return e.densityLocked()
}

// headerHeight returns how tall a pane header is in a density, in points.
func headerHeight(density string) float64 {
	if density == config.DensityCompact {
		return layout.CompactHeaderHeight
	}
	return layout.HeaderHeight
}

// soleAppLocked returns the App item a pane shows if it's the pane's only
// item and has a window, while pane_headers is auto and Bench can dock
// windows; otherwise nil. Only such a pane can go without its header: a web
// page or a terminal has no title bar of its own, and the sub-tabs of a
// pane with more items are needed.
func (e *Engine) soleAppLocked(n *layout.Node) *Item {
	if e.cfg.Settings.PaneHeaders != config.PaneHeadersAuto || !e.trusted || n == nil || len(n.Items) != 1 {
		return nil
	}
	it := e.model.Items[n.Items[0]]
	if it == nil || it.Kind != KindApp || it.WindowID == 0 {
		return nil
	}
	return it
}

// nothingToReportLocked reports whether an App item in a pane with frame
// has nothing to report, which its pane's header would say. A window that
// needs more room is judged against the body without a header, which is
// taller: one that didn't fit the body below the header may fit that,
// which docking it there tells. Only one that didn't fit a body as big
// says so.
func (e *Engine) nothingToReportLocked(it *Item, frame layout.Rect) bool {
	switch status, _ := e.statusOf(it); status {
	case StatusReady:
		return true
	case StatusNeedsRoom:
		return !e.win.NeedsRoom(it.WindowID, layout.Body(frame, 0))
	}
	return false
}

// headerHiddenLocked reports whether a displayed pane goes without its
// header, as last arranged. A pane that doesn't show one App window any more
// has its header straight away, before it's arranged again.
func (e *Engine) headerHiddenLocked(n *layout.Node) bool {
	st, ok := e.headers[n.ID]
	return ok && st.hidden && e.soleAppLocked(n) != nil
}

// updateHeadersLocked decides which of the displayed panes go without their
// header, under pane_headers = "auto", before the current tab is arranged,
// and reports whether any header has come or gone. A pane drawn for the
// first time since its tab was last shown gets what it wants at once, and so
// does a pane that doesn't show one App window. Otherwise a header comes
// back only once its pane has had something to report for headerShowDelay,
// and goes only once it has had nothing for headerHideDelay; a timer
// arranges the tab again then.
func (e *Engine) updateHeadersLocked(d display) bool {
	changed := false
	if e.cfg.Settings.PaneHeaders != config.PaneHeadersAuto || d.compact || d.zoomed {
		for _, st := range e.headers {
			changed = changed || st.hidden
		}
		clear(e.headers)
		return changed
	}
	now := e.clk.Now()
	var wait time.Duration
	drawn := map[string]bool{}
	for _, g := range d.panes {
		id := g.node.ID
		drawn[id] = true
		st, seen := e.headers[id]
		was := st.hidden
		it := e.soleAppLocked(g.node)
		want := it != nil && e.nothingToReportLocked(it, g.frame)
		switch {
		case !seen || it == nil:
			st = paneHeader{hidden: want}
		case want == st.hidden:
			st.since = time.Time{}
		default:
			if st.since.IsZero() {
				st.since = now
			}
			delay := headerShowDelay
			if want {
				delay = headerHideDelay
			}
			if left := st.since.Add(delay).Sub(now); left > 0 {
				if wait == 0 || left < wait {
					wait = left
				}
			} else {
				st = paneHeader{hidden: want}
			}
		}
		changed = changed || st.hidden != was
		e.headers[id] = st
	}
	// A tab that's shown again starts afresh.
	for id := range e.headers {
		if !drawn[id] {
			delete(e.headers, id)
		}
	}
	if wait > 0 {
		e.checkHeadersLocked(now.Add(wait))
	}
	return changed
}

// checkHeadersLocked arranges the current tab again at due, for its panes'
// headers to come or go then, unless it's due to sooner.
func (e *Engine) checkHeadersLocked(due time.Time) {
	if e.headerTimer != nil {
		if !e.headerDue.After(due) {
			return
		}
		e.headerTimer.Stop()
	}
	e.headerDue = due
	e.headerTimer = e.after(due.Sub(e.clk.Now()), func() {
		e.headerTimer = nil
		e.arrangeLocked(arrangeOpts{})
	})
}
