package workspace

import (
	"encoding/base64"
	"sync"

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

// vscodeState is the VS Code for the Web server's state.
type vscodeState struct {
	started bool // Starting has been tried.
	server  VSCodeServer
	failure string // Why it couldn't start, for the items' status.
}

// vscodeStatus returns the status of VS Code for the Web items: missing
// when this build can't start the server or it failed, launching while it
// starts, and ready otherwise.
func (e *Engine) vscodeStatus() (string, string) {
	switch {
	case e.vs.server != nil:
		return StatusReady, ""
	case e.vs.failure != "":
		return StatusMissing, e.vs.failure
	case e.opts.StartVSCode == nil:
		return StatusMissing, vscodeMissing
	case e.vs.started:
		return StatusLaunching, vscodeStarting
	}
	return StatusReady, ""
}

// ensureVSCodeLocked starts the VS Code for the Web server, once. Starting
// can take a while, so it runs without the lock; the items show that it's
// starting, and are shown when it has.
func (e *Engine) ensureVSCodeLocked() {
	if e.vs.started {
		return
	}
	e.vs.started = true
	start := e.opts.StartVSCode
	if start == nil {
		e.vs.failure = vscodeMissing
		e.touch()
		return
	}
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
	switch {
	case err != nil:
		e.vs.failure = "VS Code for the Web couldn't start: " + err.Error()
	case srv == nil:
		e.vs.failure = "VS Code for the Web couldn't start"
	default:
		e.vs.server = srv
	}
	e.arrangeLocked(arrangeOpts{})
}
