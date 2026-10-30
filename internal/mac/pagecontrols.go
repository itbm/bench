package mac

import (
	"errors"
	"sync"
	"time"

	"github.com/itbm/bench/internal/platform"
)

// This file has the pure-Go side of a Web item's page controls: what its
// page's state and capture states are in Go, and the answers to finding
// text in a page, which WebKit gives on the main thread.

// Capture states, as WebKit's WKMediaCaptureState numbers them. The values
// match bridge_darwin.h's BENCH_CAPTURE_ constants.
const (
	captureNone   = 0
	captureActive = 1
	captureMuted  = 2
)

// captureStateFor returns the capture state for a WKMediaCaptureState
// value. A value that WebKit may add later counts as none.
func captureStateFor(code int) platform.CaptureState {
	switch code {
	case captureActive:
		return platform.CaptureActive
	case captureMuted:
		return platform.CaptureMuted
	}
	return platform.CaptureNone
}

// captureCode returns the WKMediaCaptureState value of a capture state, and
// reports false for one that isn't a capture state.
func captureCode(s platform.CaptureState) (int, bool) {
	switch s {
	case platform.CaptureNone:
		return captureNone, true
	case platform.CaptureActive:
		return captureActive, true
	case platform.CaptureMuted:
		return captureMuted, true
	}
	return 0, false
}

// deviceKind returns the media kind of a capture device, and reports false
// for one that isn't the camera or the microphone.
func deviceKind(d platform.CaptureDevice) (mediaKind, bool) {
	switch d {
	case platform.Camera:
		return mediaCamera, true
	case platform.Microphone:
		return mediaMicrophone, true
	}
	return 0, false
}

// webState returns a page's state from what WebKit says.
func webState(canGoBack, canGoForward bool, camera, microphone int) platform.WebState {
	return platform.WebState{
		CanGoBack: canGoBack, CanGoForward: canGoForward,
		Camera: captureStateFor(camera), Microphone: captureStateFor(microphone),
	}
}

// findTimeout is how long FindInWebView waits for WebKit's answer. A page
// answers at once, unless its process is stuck.
const findTimeout = 3 * time.Second

// maxFind is the most characters of text that FindInWebView looks for.
const maxFind = 500

// errFindTimeout is returned, wrapped, by FindInWebView when the page
// doesn't answer in time.
var errFindTimeout = errors.New("the page didn't answer in time")

// errFindClosed is returned, wrapped, by FindInWebView once the platform
// has closed.
var errFindClosed = errors.New("the platform is closed")

// finds hands FindInWebView the answers that WebKit gives, by request: each
// call waits for its own answer, on a channel with room for it, so the main
// thread, which answers, never waits. Its methods are safe for concurrent
// use.
type finds struct {
	mu      sync.Mutex
	last    uint64
	waiting map[uint64]chan bool
	closed  bool
}

// ask returns a new request's number and the channel its answer comes on.
// It reports false once the finds have closed.
func (f *finds) ask() (uint64, <-chan bool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return 0, nil, false
	}
	if f.waiting == nil {
		f.waiting = map[uint64]chan bool{}
	}
	f.last++
	answer := make(chan bool, 1)
	f.waiting[f.last] = answer
	return f.last, answer, true
}

// answer hands a request its answer, if it's still waiting. It never
// waits.
func (f *finds) answer(request uint64, found bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if answer, ok := f.waiting[request]; ok {
		delete(f.waiting, request)
		answer <- found
	}
}

// wait waits up to timeout for request's answer on answer, and stops
// waiting for it after that, returning errFindTimeout. An answer of the
// finds closing is errFindClosed.
func (f *finds) wait(request uint64, answer <-chan bool, timeout time.Duration) (bool, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case found, ok := <-answer:
		if !ok {
			return false, errFindClosed
		}
		return found, nil
	case <-timer.C:
		f.mu.Lock()
		delete(f.waiting, request)
		f.mu.Unlock()
		return false, errFindTimeout
	}
}

// close answers every request still waiting, which then fails with
// errFindClosed, and makes later ones fail at once. It's safe to call more
// than once.
func (f *finds) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	for request, answer := range f.waiting {
		delete(f.waiting, request)
		close(answer)
	}
}
