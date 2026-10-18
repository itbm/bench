// bridge_darwin.h declares the native side of internal/mac for cgo, and the
// Go functions that the native side calls back.
//
// Only C types appear here, so that cgo can read it. Every BENCH_ constant
// has a Go constant with the same value in codes.go (or navigation.go);
// codes_test.go checks that they match.
//
// Strings passed in are copied before the function returns. Strings and
// arrays passed out are malloc'd, and the caller frees them with the
// matching benchFree function.

#ifndef BENCH_BRIDGE_DARWIN_H
#define BENCH_BRIDGE_DARWIN_H

#include <stdint.h>

// Statuses. Zero is success, positive values are Bench's own, and negative
// values are AXError codes.
#define BENCH_OK 0
#define BENCH_ERR_NO_WINDOW 1
#define BENCH_ERR_OTHER_DESKTOP 2
#define BENCH_ERR_NOT_INSTALLED 3
#define BENCH_ERR_BAD_URL 4
#define BENCH_ERR_NOT_RUNNING 5
#define BENCH_ERR_FAILED 6
#define BENCH_ERR_NO_MENU_ITEM 7

// Event codes.
#define BENCH_EV_WINDOW_CREATED 1
#define BENCH_EV_WINDOW_DESTROYED 2
#define BENCH_EV_WINDOW_MOVED 3
#define BENCH_EV_WINDOW_RESIZED 4
#define BENCH_EV_WINDOW_FOCUSED 5
#define BENCH_EV_WINDOW_RETITLED 6
#define BENCH_EV_WINDOW_MINIMISED 7
#define BENCH_EV_WINDOW_RESTORED 8
#define BENCH_EV_APP_LAUNCHED 9
#define BENCH_EV_APP_TERMINATED 10
#define BENCH_EV_APP_ACTIVATED 11
#define BENCH_EV_WEB_TITLE 12
#define BENCH_EV_WEB_URL 13
#define BENCH_EV_WEB_NOTIFICATION 14
#define BENCH_EV_NOTIFICATION_CLICKED 15
#define BENCH_EV_WEB_VIEW_CLOSED 16

// Navigation decisions (mac.Navigation) and popup decisions (mac.Popup).
#define BENCH_NAV_IN_VIEW 0
#define BENCH_NAV_IN_BROWSER 1
#define BENCH_NAV_CANCEL 2
#define BENCH_POPUP_WINDOW 0
#define BENCH_POPUP_BROWSER 1
#define BENCH_POPUP_CANCEL 2

// The first synthetic window ID, for windows whose CGWindowID can't be
// read. Real CGWindowIDs are far smaller.
#define BENCH_SYNTHETIC_BASE 0x80000000u

// A window, in global coordinates (top-left origin, y down, points).
typedef struct {
	uint32_t windowID;
	int32_t pid;
	char *bundleID;
	char *appName;
	char *title;
	char *subrole;
	double x;
	double y;
	double w;
	double h;
	int minimised;
	int fullScreen;
	int focused;
	int64_t created;
	// For moves and resizes: a mouse button was held with the pointer on the
	// window, which is a person dragging it.
	int mouseDown;
} BenchWindowInfo;

// A running app.
typedef struct {
	int32_t pid;
	char *bundleID;
	char *name;
	int hidden;
	int active;
	int regular;
} BenchAppInfo;

// A display, in AppKit coordinates (bottom-left origin, y up, points). The
// first one passed is the primary display.
typedef struct {
	uint32_t displayID;
	char *name;
	double frameX;
	double frameY;
	double frameW;
	double frameH;
	double visibleX;
	double visibleY;
	double visibleW;
	double visibleH;
} BenchScreenInfo;

// Lifecycle (observe_darwin.m). benchStart sets up observers; benchStop
// removes them and closes web views and the preview.
void benchStart(void);
void benchStop(void);

// Accessibility (ax_darwin.m). These run on the calling thread. pidHint is
// the window's app's process ID if the caller knows it, or 0.
int benchTrusted(int prompt);
int benchCopyWindows(int32_t pid, BenchWindowInfo **out, int *count);
int benchGetWindow(uint32_t windowID, int32_t pidHint, BenchWindowInfo *out);
int benchSetFrame(uint32_t windowID, int32_t pidHint, double x, double y, double w, double h);
int benchRaise(uint32_t windowID, int32_t pidHint);
int benchSetMinimised(uint32_t windowID, int32_t pidHint, int minimised);
int benchFocus(uint32_t windowID, int32_t pidHint);
int benchPressNewWindow(int32_t pid);
void benchFreeWindowInfo(BenchWindowInfo *info);
void benchFreeWindowInfos(BenchWindowInfo *infos, int count);

// Apps (apps_darwin.m).
int benchRunningApps(BenchAppInfo **out, int *count);
void benchFreeAppInfos(BenchAppInfo *infos, int count);
int32_t benchResolvePID(int32_t pid, const char *bundleID);
int benchOpenApp(const char *bundleID, const char *url);
int benchHideApp(int32_t pid, int hide);

// Screens and permissions (screens_darwin.m).
void benchRefreshScreens(void);
int benchScreenRecordingAllowed(int request);
int benchOSMajorVersion(void);
int benchHasBundleID(void);

// Bench's window (window_darwin.m).
void benchAttachWindow(void *nsWindow);
void benchSetRaised(int raised);

// Web views (web_darwin.m). Frames are in content coordinates (top-left
// origin, y down, points).
void benchWebShow(const char *itemID, const char *url, const char *profileUUID, const char *userAgentSuffix, double x, double y, double w, double h);
void benchWebHide(const char *itemID);
void benchWebClose(const char *itemID);
void benchWebPopOut(const char *itemID, const char *title);
void benchWebDock(const char *itemID);

// Live previews (preview_darwin.m). The panel frame is in AppKit
// coordinates; the capture size is in pixels.
void benchPreviewShowWindow(uint32_t windowID, const char *title, double x, double y, double w, double h, int captureW, int captureH);
void benchPreviewShowWeb(const char *itemID, const char *title, double x, double y, double w, double h);
void benchPreviewHide(void);

// Notifications (notify_darwin.m).
void benchNotify(const char *itemID, const char *title, const char *body);

// Go functions the native side calls (exports_darwin.go). They never block.
// The declarations match the ones cgo writes in _cgo_export.h.
extern void benchEmitWindowEvent(int kind, BenchWindowInfo *info);
extern void benchEmitAppEvent(int kind, BenchAppInfo *info);
extern void benchEmitItemEvent(int kind, char *itemID, char *title, char *body);
extern void benchScreensUpdated(BenchScreenInfo *screens, int count, int changed);
extern void benchContentFrameUpdated(int attached, double x, double y, double w, double h, double primaryHeight);
extern int benchDecideNavigation(char *itemURL, char *targetURL, int userClicked);
extern int benchDecidePopup(char *itemURL, char *targetURL);

#endif
