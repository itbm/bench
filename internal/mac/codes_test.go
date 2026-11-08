package mac

import (
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/itbm/bench/internal/platform"
)

// headerConstants is every BENCH_ constant in bridge_darwin.h, with the Go
// value it must equal.
var headerConstants = map[string]int64{
	"BENCH_OK":                  int64(statusOK),
	"BENCH_ERR_NO_WINDOW":       statusNoWindow,
	"BENCH_ERR_OTHER_DESKTOP":   statusOtherDesktop,
	"BENCH_ERR_NOT_INSTALLED":   statusNotInstalled,
	"BENCH_ERR_BAD_URL":         statusBadURL,
	"BENCH_ERR_NOT_RUNNING":     statusNotRunning,
	"BENCH_ERR_FAILED":          statusFailed,
	"BENCH_ERR_NO_MENU_ITEM":    statusNoMenuItem,
	"BENCH_ERR_NO_CLOSE_BUTTON": statusNoCloseButton,

	"BENCH_EV_WINDOW_CREATED":       evWindowCreated,
	"BENCH_EV_WINDOW_DESTROYED":     evWindowDestroyed,
	"BENCH_EV_WINDOW_MOVED":         evWindowMoved,
	"BENCH_EV_WINDOW_RESIZED":       evWindowResized,
	"BENCH_EV_WINDOW_FOCUSED":       evWindowFocused,
	"BENCH_EV_WINDOW_RETITLED":      evWindowRetitled,
	"BENCH_EV_WINDOW_MINIMISED":     evWindowMinimised,
	"BENCH_EV_WINDOW_RESTORED":      evWindowRestored,
	"BENCH_EV_APP_LAUNCHED":         evAppLaunched,
	"BENCH_EV_APP_TERMINATED":       evAppTerminated,
	"BENCH_EV_APP_ACTIVATED":        evAppActivated,
	"BENCH_EV_WEB_TITLE":            evWebTitle,
	"BENCH_EV_WEB_URL":              evWebURL,
	"BENCH_EV_WEB_NOTIFICATION":     evWebNotification,
	"BENCH_EV_NOTIFICATION_CLICKED": evNotificationClicked,
	"BENCH_EV_WEB_VIEW_CLOSED":      evWebViewClosed,
	"BENCH_EV_WEB_FOCUSED":          evWebFocused,
	"BENCH_EV_BENCH_HIDDEN":         evBenchHidden,
	"BENCH_EV_BENCH_SHOWN":          evBenchShown,
	"BENCH_EV_LOGGING_OUT":          evLoggingOut,
	"BENCH_EV_WINDOW_DRAGGED":       evWindowDragged,
	"BENCH_EV_WINDOW_DROPPED":       evWindowDropped,
	"BENCH_EV_POP_OUT_FOCUSED":      evPopOutFocused,
	"BENCH_EV_POP_OUT_UNFOCUSED":    evPopOutUnfocused,
	"BENCH_EV_DOCK_REQUESTED":       evDockRequested,
	"BENCH_EV_WOKE":                 evWoke,
	"BENCH_EV_SCREENS_WOKE":         evScreensWoke,
	"BENCH_EV_SESSION_ACTIVE":       evSessionActive,
	"BENCH_EV_SCREEN_UNLOCKED":      evScreenUnlocked,
	"BENCH_EV_DESKTOP_CHANGED":      evDesktopChanged,

	"BENCH_PREF_NUMBER":      prefNumber,
	"BENCH_PREF_TEXT":        prefText,
	"BENCH_PREF_SHORTCUT":    prefShortcut,
	"BENCH_PREF_NO_SHORTCUT": prefNoShortcut,
	"BENCH_PREF_OTHER":       prefOther,

	"BENCH_MOD_CONTROL": heldControl,
	"BENCH_MOD_OPTION":  heldOption,
	"BENCH_MOD_SHIFT":   heldShift,
	"BENCH_MOD_COMMAND": heldCommand,

	"BENCH_GUIDE_AREA":      guideArea,
	"BENCH_GUIDE_MARK":      guideMark,
	"BENCH_GUIDE_MARK_ON":   guideMarkOn,
	"BENCH_GUIDE_LABEL":     guideLabel,
	"BENCH_GUIDE_MIDDLE":    guideMiddle,
	"BENCH_GUIDE_LEFT":      guideLeft,
	"BENCH_GUIDE_RIGHT":     guideRight,
	"BENCH_GUIDE_TOP":       guideTop,
	"BENCH_GUIDE_BOTTOM":    guideBottom,
	"BENCH_GUIDE_MARK_SIZE": guideMarkSize,

	"BENCH_NAV_IN_VIEW":    int64(NavigateInView),
	"BENCH_NAV_IN_BROWSER": int64(NavigateInBrowser),
	"BENCH_NAV_CANCEL":     int64(NavigateCancel),
	"BENCH_POPUP_WINDOW":   int64(PopupWindow),
	"BENCH_POPUP_BROWSER":  int64(PopupBrowser),
	"BENCH_POPUP_CANCEL":   int64(PopupCancel),
	"BENCH_NAV_ASK":        int64(NavigateAsk),
	"BENCH_POPUP_ASK":      int64(PopupAsk),

	"BENCH_RESPONSE_SHOW":     int64(ResponseShow),
	"BENCH_RESPONSE_DOWNLOAD": int64(ResponseDownload),

	"BENCH_NAV_KIND_OTHER":   int64(navOther),
	"BENCH_NAV_KIND_LINK":    int64(navLink),
	"BENCH_NAV_KIND_HISTORY": int64(navHistory),
	"BENCH_NAV_FROM_BENCH":   int64(fromBench),
	"BENCH_NAV_FROM_NONE":    int64(fromNone),
	"BENCH_NAV_FROM_PAGE":    int64(fromPage),
	"BENCH_NAV_FROM_FRAME":   int64(fromFrame),
	"BENCH_NAV_FROM_WINDOW":  int64(fromWindow),

	"BENCH_PROMPT_DIALOG":               int64(promptDialog),
	"BENCH_PROMPT_MEDIA":                int64(promptMedia),
	"BENCH_MEDIA_CAMERA":                int64(mediaCamera),
	"BENCH_MEDIA_MICROPHONE":            int64(mediaMicrophone),
	"BENCH_MEDIA_CAMERA_AND_MICROPHONE": int64(mediaCameraAndMicrophone),
	"BENCH_DIALOG_READ":                 maxDialogRead,
	"BENCH_CAPTURE_NONE":                captureNone,
	"BENCH_CAPTURE_ACTIVE":              captureActive,
	"BENCH_CAPTURE_MUTED":               captureMuted,
	"BENCH_APPEARANCE_SYSTEM":           appearanceSystem,
	"BENCH_APPEARANCE_LIGHT":            appearanceLight,
	"BENCH_APPEARANCE_DARK":             appearanceDark,

	"BENCH_NEW_WINDOW_OTHER":   newWindowOther,
	"BENCH_NEW_WINDOW_EXACT":   newWindowExact,
	"BENCH_CLOSE_WINDOW_PLAIN": closeWindowPlain,
	"BENCH_CLOSE_WINDOW_EXACT": closeWindowExact,

	"BENCH_SYNTHETIC_BASE": syntheticBase,
}

// TestHeaderConstants checks that bridge_darwin.h and the Go constants
// agree, since the compiler can't check it on any OS but macOS.
func TestHeaderConstants(t *testing.T) {
	src, err := os.ReadFile("bridge_darwin.h")
	if err != nil {
		t.Fatal(err)
	}
	define := regexp.MustCompile(`(?m)^#define\s+(BENCH_\w+)\s+(\S+)`)
	found := map[string]bool{}
	for _, m := range define.FindAllStringSubmatch(string(src), -1) {
		name, text := m[1], strings.TrimRight(m[2], "uUlL")
		if name == "BENCH_BRIDGE_DARWIN_H" {
			continue
		}
		found[name] = true
		got, err := strconv.ParseInt(text, 0, 64)
		if err != nil {
			t.Errorf("%s: can't read %q as a number", name, m[2])
			continue
		}
		want, ok := headerConstants[name]
		if !ok {
			t.Errorf("%s = %d is in the header but has no Go constant in this test", name, got)
			continue
		}
		if got != want {
			t.Errorf("%s = %d in the header, but %d in Go", name, got, want)
		}
	}
	for name := range headerConstants {
		if !found[name] {
			t.Errorf("%s is missing from the header", name)
		}
	}
}

func TestEventKindFor(t *testing.T) {
	seen := map[platform.EventKind]bool{}
	for code := evWindowCreated; code <= evDesktopChanged; code++ {
		k, ok := eventKindFor(code)
		if !ok {
			t.Errorf("event code %d has no kind", code)
			continue
		}
		if seen[k] {
			t.Errorf("event kind %s has two codes", k)
		}
		seen[k] = true
	}
	for _, code := range []int{0, -1, evDesktopChanged + 1} {
		if k, ok := eventKindFor(code); ok {
			t.Errorf("eventKindFor(%d) = %s, want none", code, k)
		}
	}
	for code, want := range map[int]platform.EventKind{
		evWindowMinimised: platform.WindowMinimised,
		evWebFocused:      platform.WebFocused,
		evBenchHidden:     platform.BenchHidden,
		evBenchShown:      platform.BenchShown,
		evLoggingOut:      platform.LoggingOut,
		evWindowDragged:   platform.WindowDragged,
		evDockRequested:   platform.DockRequested,
		evWoke:            platform.Woke,
		evScreensWoke:     platform.ScreensWoke,
		evSessionActive:   platform.SessionActive,
		evScreenUnlocked:  platform.ScreenUnlocked,
		evDesktopChanged:  platform.DesktopChanged,
	} {
		if k, _ := eventKindFor(code); k != want {
			t.Errorf("eventKindFor(%d) = %s, want %s", code, k, want)
		}
	}
}

func TestStatusError(t *testing.T) {
	tests := []struct {
		code    int
		is      error
		message string
	}{
		{statusNoWindow, platform.ErrNoWindow, "moving window 7: window not found"},
		{axErrorInvalidUIElement, platform.ErrNoWindow, "moving window 7: window not found"},
		{statusOtherDesktop, platform.ErrOtherDesktop, "moving window 7: window is on another desktop"},
		{axErrorAPIDisabled, platform.ErrNotTrusted, "moving window 7: Bench needs Accessibility permission"},
		{statusNotInstalled, platform.ErrNotInstalled, "moving window 7: no app with that bundle ID is installed"},
		{statusBadURL, errBadURL, "moving window 7: that isn't a valid address"},
		{statusNotRunning, errNotRunning, "moving window 7: the app isn't running"},
		{statusFailed, errFailed, "moving window 7: macOS didn't do it"},
		{statusNoMenuItem, errNoMenuItem, "moving window 7: the app has no New Window menu item"},
		{statusNoCloseButton, errNoCloseButton, "moving window 7: the window has no close button, and its app no Close Window menu item"},
		{axErrorCannotComplete, errNoResponse, "moving window 7: the app didn't respond in time"},
		{axErrorAttributeUnsupported, nil, "moving window 7: Accessibility error -25205 (attribute unsupported)"},
		{-1, nil, "moving window 7: Accessibility error -1 (unknown)"},
	}
	for _, tt := range tests {
		err := statusError(tt.code, "moving window 7")
		if err == nil {
			t.Errorf("statusError(%d) = nil", tt.code)
			continue
		}
		if err.Error() != tt.message {
			t.Errorf("statusError(%d) = %q, want %q", tt.code, err, tt.message)
		}
		if tt.is != nil && !errors.Is(err, tt.is) {
			t.Errorf("statusError(%d) doesn't wrap %v", tt.code, tt.is)
		}
	}
	if err := statusError(statusOK, "anything"); err != nil {
		t.Errorf("statusError(statusOK) = %v, want nil", err)
	}
}

func TestAXErrorNames(t *testing.T) {
	for code := axErrorNotEnoughPrecision; code <= axErrorFailure; code++ {
		if axErrorName(code) == "unknown" {
			t.Errorf("AXError %d has no name", code)
		}
	}
}

func TestClip(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 3, "hel"},
		{"héllo", 2, "hé"},
		{"日本語のテキスト", 3, "日本語"},
		{"", 3, ""},
		{"abc", 0, ""},
	}
	for _, tt := range tests {
		if got := clip(tt.in, tt.n); got != tt.want {
			t.Errorf("clip(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

func TestClipBytes(t *testing.T) {
	long := strings.Repeat("a", 3000)
	for _, s := range []string{
		long,
		"é" + long, // The cut falls inside a rune.
		strings.Repeat("日本", 400),
		strings.Repeat("😀a", 300),
		strings.Repeat("😀", 499) + "é" + long,
		"short",
	} {
		read := s[:min(len(s), clipBytes(maxTitle))]
		if got, want := clip(read, maxTitle), clip(s, maxTitle); got != want {
			t.Errorf("clipping the first %d bytes of a %d-byte string gave %d runes, want %d",
				len(read), len(s), utf8.RuneCountInString(got), utf8.RuneCountInString(want))
		}
	}
}
