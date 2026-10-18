package workspace

import (
	"encoding/base64"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// This file has the terminal host's callbacks, which batch output for the
// frontend, and the VS Code for the Web server.

// termQueue collects what terminal sessions report between batches. The
// sessions' goroutines must not block, so it has its own lock, and the
// engine's lock is only taken to apply titles and exits.
type termQueue struct {
	mu        sync.Mutex
	data      map[string][]byte
	order     []string
	events    []termEvent
	scheduled bool
	sendMu    sync.Mutex // Keeps batches in order.
}

// termEvent is a session's title change or exit.
type termEvent struct {
	id    string
	title string
	exit  bool
}

// termCallbacks returns the callbacks for the terminal host.
func (e *Engine) termCallbacks() term.Callbacks {
	return term.Callbacks{
		Data: func(id string, data []byte) {
			e.queueTerm(func(q *termQueue) {
				if _, ok := q.data[id]; !ok {
					q.order = append(q.order, id)
				}
				q.data[id] = append(q.data[id], data...)
			})
		},
		Title: func(id, title string) {
			e.queueTerm(func(q *termQueue) { q.events = append(q.events, termEvent{id: id, title: title}) })
		},
		Exit: func(id string, _ error) {
			e.queueTerm(func(q *termQueue) { q.events = append(q.events, termEvent{id: id, exit: true}) })
		},
	}
}

// queueTerm records something a session reported, and makes sure a batch
// goes out within a frame.
func (e *Engine) queueTerm(add func(q *termQueue)) {
	q := &e.tq
	q.mu.Lock()
	add(q)
	schedule := !q.scheduled
	q.scheduled = true
	q.mu.Unlock()
	if schedule {
		e.clk.AfterFunc(frame, e.flushTerms)
	}
}

// flushTerms sends each session's output since the last batch as one
// term:data event, then applies titles and exits to the items.
func (e *Engine) flushTerms() {
	q := &e.tq
	q.sendMu.Lock()
	defer q.sendMu.Unlock()
	q.mu.Lock()
	data, order, events := q.data, q.order, q.events
	q.data, q.order, q.events = map[string][]byte{}, nil, nil
	q.scheduled = false
	q.mu.Unlock()

	e.lock()
	closed := e.closed
	e.unlock()
	if closed {
		return
	}
	for _, id := range order {
		e.em.TermData(TermData{ID: id, Data: base64.StdEncoding.EncodeToString(data[id])})
	}
	if len(events) == 0 {
		return
	}
	e.lock()
	defer e.unlock()
	if e.closed {
		return
	}
	for _, ev := range events {
		e.applyTermEventLocked(ev)
	}
}

// applyTermEventLocked updates a terminal's item: a new title, or, when the
// session has ended, the item is removed like a closed window.
func (e *Engine) applyTermEventLocked(ev termEvent) {
	it := e.model.Items[ev.id]
	if it == nil || it.Kind != KindBuiltin {
		return
	}
	if ev.exit {
		ti, _ := e.model.Where(it.ID)
		e.removeItemLocked(it)
		if ti == e.model.Current {
			e.arrangeLocked(arrangeOpts{})
		}
		return
	}
	if ev.title != "" && ev.title != it.Title {
		it.Title = ev.title
		e.changed()
	}
}

// vscodeRetry is the shortest time between starts of the VS Code for the
// Web server, so that a server that can't start, or ends as soon as it has,
// isn't started over and over while its items are shown.
const vscodeRetry = 10 * time.Second

// vscodeState is the VS Code for the Web server's state.
type vscodeState struct {
	starting bool // A start is under way.
	server   VSCodeServer
	failure  string    // Why it couldn't start, or that it stopped, for the items' status.
	tried    time.Time // When the last start began, or failed.
}

// vscodeStatus returns the status of VS Code for the Web items: launching
// while the server starts, missing when this build can't start it or it
// failed or stopped, and ready otherwise.
func (e *Engine) vscodeStatus() (string, string) {
	switch {
	case e.vs.server != nil:
		return StatusReady, ""
	case e.vs.starting:
		return StatusLaunching, vscodeStarting
	case e.vs.failure != "":
		return StatusMissing, e.vs.failure
	case e.opts.StartVSCode == nil:
		return StatusMissing, vscodeMissing
	}
	return StatusReady, ""
}

// ensureVSCodeLocked starts the VS Code for the Web server if it isn't
// running or starting. Starting can take a while, so it runs without the
// lock; the items show that it's starting, and are shown when it has. After
// a start that failed, or a server that stopped, an item that's shown starts
// it again, but not within vscodeRetry of the last start, or of the failure.
func (e *Engine) ensureVSCodeLocked() {
	if e.vs.server != nil || e.vs.starting {
		return
	}
	start := e.opts.StartVSCode
	if start == nil {
		if e.vs.failure != vscodeMissing {
			e.vs.failure = vscodeMissing
			e.touch()
		}
		return
	}
	now := e.clk.Now()
	if !e.vs.tried.IsZero() && now.Sub(e.vs.tried) < vscodeRetry {
		return
	}
	e.vs.starting, e.vs.tried, e.vs.failure = true, now, ""
	ctx := e.ctx
	e.touch()
	e.clk.AfterFunc(0, func() { e.vscodeStarted(start(ctx)) })
}

// vscodeStarted records the server once it has started, or why it couldn't.
func (e *Engine) vscodeStarted(srv VSCodeServer, err error) {
	e.lock()
	defer e.unlock()
	if e.closed {
		if srv != nil {
			e.later(func() { _ = srv.Stop() })
		}
		return
	}
	e.vs.starting = false
	switch {
	case err != nil:
		e.vs.failure = "VS Code for the Web couldn't start: " + err.Error()
	case srv == nil:
		e.vs.failure = "VS Code for the Web couldn't start"
	default:
		e.vs.server = srv
		go e.watchVSCode(srv)
		e.reloadPoppedOutVSCodeLocked()
	}
	if e.vs.server == nil {
		// A start can fail after longer than vscodeRetry, as one that
		// times out does. Count from the failure, so that the arrange
		// below doesn't start it again at once, and the items say why.
		e.vs.tried = e.clk.Now()
	}
	e.reopenVSCodeWindowsLocked()
	e.arrangeLocked(arrangeOpts{})
}

// poppedOutVSCodeLocked returns the VS Code for the Web items whose views
// are in windows of their own. Arranging doesn't show them, so it doesn't
// load them at a new server's address either.
func (e *Engine) poppedOutVSCodeLocked() []*Item {
	var items []*Item
	for _, id := range e.model.Floating {
		if it := e.model.Items[id]; it != nil && it.VSCode != "" && it.OwnWindow && e.webMade[id] {
			items = append(items, it)
		}
	}
	return items
}

// reloadPoppedOutVSCodeLocked loads the popped-out VS Code for the Web
// items' views at the new server's address. ShowWebView leaves a popped-out
// view in its own window, and docking it hides it until it's shown again.
func (e *Engine) reloadPoppedOutVSCodeLocked() {
	for _, it := range e.poppedOutVSCodeLocked() {
		view := platform.WebView{URL: e.vs.server.URL(it.VSCode), Profile: it.Profile}
		if err := e.p.ShowWebView(it.ID, view, layout.Rect{}); err != nil {
			e.log.Warn("couldn't load VS Code for the Web again", "item", it.ID, "error", err)
		}
	}
}

// watchVSCode waits for the server to end without Bench stopping it, as it
// does when VS Code updates itself, then forgets it, so that the next item
// shown starts another, and stops it, which removes its token and record.
// Close stops a server that's still running.
func (e *Engine) watchVSCode(srv VSCodeServer) {
	select {
	case <-srv.Done():
	case <-e.ctx.Done():
		return
	}
	if e.vscodeEnded(srv) {
		_ = srv.Stop()
	}
}

// vscodeEnded forgets a server that has ended, and reports whether it was
// still the engine's, for the caller to stop.
func (e *Engine) vscodeEnded(srv VSCodeServer) bool {
	e.lock()
	defer e.unlock()
	if e.closed || e.vs.server != srv {
		return false
	}
	e.log.Warn("VS Code for the Web stopped")
	e.vs.server = nil
	e.vs.failure = vscodeStopped
	// The items' views show a server that has gone. Hide them, so that each
	// shows its status, and is shown again at the new server's address once
	// it has started.
	for _, id := range slices.Sorted(maps.Keys(e.webShown)) {
		if it := e.model.Items[id]; it != nil && it.VSCode != "" {
			e.p.HideWebView(id)
			delete(e.webShown, id)
		}
	}
	// Arranging starts the server again for the docked items it shows, but
	// not for popped-out ones, which show a server that has gone too.
	if len(e.poppedOutVSCodeLocked()) > 0 {
		e.ensureVSCodeLocked()
	}
	e.arrangeLocked(arrangeOpts{})
	return true
}
