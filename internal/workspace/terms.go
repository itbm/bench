package workspace

import (
	"encoding/base64"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
	"github.com/itbm/bench/internal/term"
)

// This file has the terminal host's callbacks, which batch output for the
// frontend, opening links and pages in the default browser, and the VS Code
// for the Web server.

// termChunk is the most output one term:data event carries, in bytes. Wails
// fetches an event whose JSON is over 8 KB separately, one at a time, and
// drops it if it waits more than 30 seconds, so each event stays well under
// that once base64 has made it a third bigger.
const termChunk = 4 * 1024

// termQueue collects what terminal sessions report between batches. The
// sessions' goroutines must not block, so it has its own lock, and the
// engine's lock is only taken to apply titles and exits.
type termQueue struct {
	mu        sync.Mutex
	data      map[string][]byte
	from      map[string]int64 // where each session's queued output starts
	order     []string
	events    []termEvent
	scheduled bool
	sendMu    sync.Mutex // Keeps batches in order.
}

// termEvent is a session's title change or exit.
type termEvent struct {
	id      string
	title   string
	byShell bool // The shell set the title.
	exit    bool
}

// termCallbacks returns the callbacks for the terminal host.
func (e *Engine) termCallbacks() term.Callbacks {
	return term.Callbacks{
		Data: func(id string, offset int64, data []byte) {
			e.queueTerm(func(q *termQueue) {
				if _, ok := q.data[id]; !ok {
					// A session's output follows on from one chunk to
					// the next, so the batch needs only where it starts.
					q.order = append(q.order, id)
					if q.from == nil {
						q.from = map[string]int64{}
					}
					q.from[id] = offset
				}
				q.data[id] = append(q.data[id], data...)
			})
		},
		Title: func(id, title string, byShell bool) {
			e.queueTerm(func(q *termQueue) {
				q.events = append(q.events, termEvent{id: id, title: title, byShell: byShell})
			})
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

// flushTerms sends each session's output since the last batch, in term:data
// events of up to termChunk bytes, then applies titles and exits to the
// items. Output doesn't wait for the engine's lock, which an arrange can hold
// for a while.
func (e *Engine) flushTerms() {
	q := &e.tq
	q.sendMu.Lock()
	defer q.sendMu.Unlock()
	q.mu.Lock()
	data, from, order, events := q.data, q.from, q.order, q.events
	q.data, q.from, q.order, q.events = map[string][]byte{}, nil, nil, nil
	q.scheduled = false
	q.mu.Unlock()

	// Close cancels the context, as it sets closed.
	if e.ctx.Err() != nil {
		return
	}
	for _, id := range order {
		out, offset := data[id], from[id]
		for len(out) > 0 {
			n := min(len(out), termChunk)
			e.em.TermData(TermData{ID: id, Data: base64.StdEncoding.EncodeToString(out[:n]), Offset: offset})
			out, offset = out[n:], offset+int64(n)
		}
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
// session has ended, the item is removed like a closed window. A terminal
// that ran a command stays instead, with the command's output, marked as
// ended: the output is what the user wants to read, and a config entry
// would otherwise be lost for good (see TermInput).
func (e *Engine) applyTermEventLocked(ev termEvent) {
	it := e.model.Items[ev.id]
	if it == nil || it.Kind != KindBuiltin {
		return
	}
	if ev.exit {
		if it.Command != "" {
			it.ended = true
			e.touch()
			return
		}
		ti, _ := e.model.Where(it.ID)
		e.removeItemLocked(it)
		if ti == e.model.Current {
			e.arrangeLocked(arrangeOpts{})
		}
		return
	}
	title := ev.title
	if !ev.byShell && it.Name != "" {
		// The config's name stays until the shell sets a title.
		title = it.Name
	}
	if title != "" && title != it.Title {
		it.Title = title
		e.retitled()
	}
}

// vscodeRetry is the shortest time between starts of the VS Code for the
// Web server, so that a server that can't start, or ends as soon as it has,
// isn't started over and over while its items are shown.
const vscodeRetry = 10 * time.Second

// rerunLocked handles what's typed into a terminal whose command has ended,
// and reports whether it had: Return runs the command again, in the same
// view, and anything else is ignored. Once Bench is closing, nothing runs.
func (e *Engine) rerunLocked(itemID, data string) bool {
	it := e.model.Items[itemID]
	if e.closed || it == nil || it.Kind != KindBuiltin || !it.ended {
		return false
	}
	if strings.ContainsAny(data, "\r\n") {
		it.ended = false
		delete(e.termOn, it.ID)
		e.startTerminalLocked(it)
		e.changed()
	}
	return true
}

// TermAck tells a terminal's session that a view has drawn its output up to
// offset, so that it reads more of its program's output if it was waiting
// for its views. It doesn't take the engine's lock.
func (e *Engine) TermAck(itemID string, offset int64) error {
	e.terms.Ack(itemID, offset)
	return nil
}

// OpenLink opens a link that a terminal program printed, in the default
// browser. Only http and https links open.
func (e *Engine) OpenLink(address string) error {
	return e.openInBrowser(address, errNotWebLink)
}

// OpenInBrowser opens the page a Web item shows in the default browser: the
// one its web view last reported, or until it reports one, after a restart
// too, the address the view loads, the one the item was given. A page that
// isn't an http or https one opens as the site it belongs to, for a blob:
// page whose origin is (see browserAddress), or else as the item's own
// address. Only http and https pages open. VS Code for the Web doesn't: its
// address holds the server's connection token.
func (e *Engine) OpenInBrowser(itemID string) error {
	e.lock()
	it := e.model.Items[itemID]
	switch {
	case it == nil:
		e.unlock()
		return errGone
	case it.Kind != KindWeb || it.VSCode != "":
		e.unlock()
		return errNotPage
	}
	// Not it.URL, which can be the last page from before a restart.
	address := browserAddress(it.page)
	if address == "" {
		address, _ = e.webAddressLocked(it)
	}
	e.unlock()
	return e.openInBrowser(address, errNotHTTPPage)
}

// browserAddress returns the address of a page that a Web item shows for the
// default browser to open: the page's own, if it's an http or https one; the
// origin inside a blob: page's address, such as https://app.slack.com for
// blob:https://app.slack.com/1234, if that's an http or https one with a
// host, since the page itself is only in the item's view; or else "".
func browserAddress(page string) string {
	scheme, rest, _ := strings.Cut(page, ":")
	switch strings.ToLower(scheme) {
	case "http", "https":
		return page
	case "blob":
		u, err := url.Parse(rest)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return ""
		}
		return u.Scheme + "://" + u.Host
	}
	return ""
}

// openInBrowser opens an http or https address with Options.OpenLink, or
// returns refused for any other address. The engine's lock isn't held.
func (e *Engine) openInBrowser(address string, refused error) error {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return refused
	}
	if e.opts.OpenLink == nil {
		return errCantOpenLinks
	}
	if err := e.opts.OpenLink(u.String()); err != nil {
		return fmt.Errorf("Bench couldn't open %s.", u.Host)
	}
	return nil
}

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
		view := platform.WebView{URL: e.vs.server.URL(it.VSCode), Name: it.App, Profile: it.Profile}
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
