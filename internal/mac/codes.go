package mac

import (
	"errors"
	"fmt"

	"github.com/itbm/bench/internal/platform"
)

// This file holds the numbers that the native code and Go share. Each has a
// BENCH_ constant in bridge_darwin.h with the same value; codes_test.go
// checks that they match.

// Status codes returned by the native functions. Zero is success, positive
// codes are Bench's own, and negative codes are AXError values.
const (
	statusOK           = 0
	statusNoWindow     = 1
	statusOtherDesktop = 2
	statusNotInstalled = 3
	statusBadURL       = 4
	statusNotRunning   = 5
	statusFailed       = 6
	statusNoMenuItem   = 7
)

// AXError values, from HIServices' AXError.h.
const (
	axErrorFailure                           = -25200
	axErrorIllegalArgument                   = -25201
	axErrorInvalidUIElement                  = -25202
	axErrorInvalidUIElementObserver          = -25203
	axErrorCannotComplete                    = -25204
	axErrorAttributeUnsupported              = -25205
	axErrorActionUnsupported                 = -25206
	axErrorNotificationUnsupported           = -25207
	axErrorNotImplemented                    = -25208
	axErrorNotificationAlreadyRegistered     = -25209
	axErrorNotificationNotRegistered         = -25210
	axErrorAPIDisabled                       = -25211
	axErrorNoValue                           = -25212
	axErrorParameterizedAttributeUnsupported = -25213
	axErrorNotEnoughPrecision                = -25214
)

// Event codes, for the events the native code reports. ScreensChanged and
// BenchMoved are made in Go, when the native code reports new screens or a
// new content frame.
const (
	evWindowCreated       = 1
	evWindowDestroyed     = 2
	evWindowMoved         = 3
	evWindowResized       = 4
	evWindowFocused       = 5
	evWindowRetitled      = 6
	evWindowMinimised     = 7
	evWindowRestored      = 8
	evAppLaunched         = 9
	evAppTerminated       = 10
	evAppActivated        = 11
	evWebTitle            = 12
	evWebURL              = 13
	evWebNotification     = 14
	evNotificationClicked = 15
	evWebViewClosed       = 16
)

// syntheticBase is the first window ID the native registry gives windows
// whose CGWindowID it can't read. Real CGWindowIDs are far smaller.
const syntheticBase = 0x80000000

// eventKinds maps event codes to event kinds.
var eventKinds = map[int]platform.EventKind{
	evWindowCreated:       platform.WindowCreated,
	evWindowDestroyed:     platform.WindowDestroyed,
	evWindowMoved:         platform.WindowMoved,
	evWindowResized:       platform.WindowResized,
	evWindowFocused:       platform.WindowFocused,
	evWindowRetitled:      platform.WindowRetitled,
	evWindowMinimised:     platform.WindowMinimised,
	evWindowRestored:      platform.WindowRestored,
	evAppLaunched:         platform.AppLaunched,
	evAppTerminated:       platform.AppTerminated,
	evAppActivated:        platform.AppActivated,
	evWebTitle:            platform.WebTitle,
	evWebURL:              platform.WebURL,
	evWebNotification:     platform.WebNotification,
	evNotificationClicked: platform.NotificationClicked,
	evWebViewClosed:       platform.WebViewClosed,
}

// eventKindFor returns the event kind for an event code.
func eventKindFor(code int) (platform.EventKind, bool) {
	k, ok := eventKinds[code]
	return k, ok
}

// Errors for statuses that callers don't act on, but users see.
var (
	errNotInstalled = errors.New("no app with that bundle ID is installed")
	errBadURL       = errors.New("that isn't a valid address")
	errNotRunning   = errors.New("the app isn't running")
	errFailed       = errors.New("macOS didn't do it")
	errNoMenuItem   = errors.New("the app has no New Window menu item")
	errNoResponse   = errors.New("the app didn't respond in time")
	// errNoScreenRecording is returned, wrapped, by ShowPreview for a window
	// when Bench can't capture windows.
	errNoScreenRecording = errors.New("live previews need Screen Recording permission")
)

// statusError turns a native status into an error that says what failed
// (action, such as "moving window 12") and why, or nil for statusOK.
// Statuses that callers act on wrap the platform's errors, so errors.Is
// works: ErrNoWindow, ErrOtherDesktop, and ErrNotTrusted.
func statusError(code int, action string) error {
	var cause error
	switch code {
	case statusOK:
		return nil
	case statusNoWindow, axErrorInvalidUIElement:
		cause = platform.ErrNoWindow
	case statusOtherDesktop:
		cause = platform.ErrOtherDesktop
	case axErrorAPIDisabled:
		cause = platform.ErrNotTrusted
	case statusNotInstalled:
		cause = errNotInstalled
	case statusBadURL:
		cause = errBadURL
	case statusNotRunning:
		cause = errNotRunning
	case statusFailed:
		cause = errFailed
	case statusNoMenuItem:
		cause = errNoMenuItem
	case axErrorCannotComplete:
		cause = errNoResponse
	default:
		cause = fmt.Errorf("Accessibility error %d (%s)", code, axErrorName(code))
	}
	return fmt.Errorf("%s: %w", action, cause)
}

// axErrorName names an AXError value, in words.
func axErrorName(code int) string {
	switch code {
	case axErrorFailure:
		return "failure"
	case axErrorIllegalArgument:
		return "illegal argument"
	case axErrorInvalidUIElement:
		return "invalid element"
	case axErrorInvalidUIElementObserver:
		return "invalid observer"
	case axErrorCannotComplete:
		return "can't complete"
	case axErrorAttributeUnsupported:
		return "attribute unsupported"
	case axErrorActionUnsupported:
		return "action unsupported"
	case axErrorNotificationUnsupported:
		return "notification unsupported"
	case axErrorNotImplemented:
		return "not implemented"
	case axErrorNotificationAlreadyRegistered:
		return "notification already registered"
	case axErrorNotificationNotRegistered:
		return "notification not registered"
	case axErrorAPIDisabled:
		return "API disabled"
	case axErrorNoValue:
		return "no value"
	case axErrorParameterizedAttributeUnsupported:
		return "parameterised attribute unsupported"
	case axErrorNotEnoughPrecision:
		return "not enough precision"
	}
	return "unknown"
}

// clip shortens s to at most n runes, for text from web pages, which could
// be any length.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// Limits on text from web pages.
const (
	maxTitle = 500
	maxBody  = 2000
)
