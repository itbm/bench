// bridge_darwin.h declares the native side of internal/mac for cgo, and the
// Go functions that the native side calls back.
//
// Only C types appear here, so that cgo can read it. Every BENCH_ constant
// has a Go constant with the same value in codes.go (or navigation.go,
// navstate.go, and prompts.go); codes_test.go checks that they match.
//
// Strings passed in are copied before the function returns. Strings and
// arrays passed out are malloc'd, and the caller frees them with the
// matching benchFree function, or a string returned on its own with free.

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
#define BENCH_ERR_NO_CLOSE_BUTTON 8

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
#define BENCH_EV_WEB_FOCUSED 17
#define BENCH_EV_BENCH_HIDDEN 18
#define BENCH_EV_BENCH_SHOWN 19
#define BENCH_EV_LOGGING_OUT 20

// Navigation decisions (mac.Navigation), popup decisions (mac.Popup), and
// response decisions (mac.Response).
#define BENCH_NAV_IN_VIEW 0
#define BENCH_NAV_IN_BROWSER 1
#define BENCH_NAV_CANCEL 2
#define BENCH_NAV_ASK 3
#define BENCH_POPUP_WINDOW 0
#define BENCH_POPUP_BROWSER 1
#define BENCH_POPUP_CANCEL 2
#define BENCH_POPUP_ASK 3
#define BENCH_RESPONSE_SHOW 0
#define BENCH_RESPONSE_DOWNLOAD 1

// Kinds of main-frame navigation (mac.navKind), and what started one
// (mac.navSource).
#define BENCH_NAV_KIND_OTHER 0
#define BENCH_NAV_KIND_LINK 1
#define BENCH_NAV_KIND_HISTORY 2
#define BENCH_NAV_FROM_BENCH 0
#define BENCH_NAV_FROM_NONE 1
#define BENCH_NAV_FROM_PAGE 2
#define BENCH_NAV_FROM_FRAME 3
#define BENCH_NAV_FROM_WINDOW 4

// Prompts that a Web item's page brings up (mac.promptKind), and what a
// page's request for the camera or microphone asks to use (mac.mediaKind).
#define BENCH_PROMPT_DIALOG 0
#define BENCH_PROMPT_MEDIA 1
#define BENCH_MEDIA_CAMERA 0
#define BENCH_MEDIA_MICROPHONE 1
#define BENCH_MEDIA_CAMERA_AND_MICROPHONE 2

// The most UTF-16 code units of a page's alert or confirmation text that
// the native side reads (mac.maxDialogRead).
#define BENCH_DIALOG_READ 32000

// How well a menu item's title names the New Window item (mac.newWindowRank).
#define BENCH_NEW_WINDOW_OTHER 1
#define BENCH_NEW_WINDOW_EXACT 2

// How well a menu item's title names the item that closes the app's main
// window (mac.closeWindowRank).
#define BENCH_CLOSE_WINDOW_PLAIN 1
#define BENCH_CLOSE_WINDOW_EXACT 2

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
// benchQuitForLogOut reports whether the quit that macOS is asking Bench
// for is because the user is logging out, restarting, or shutting down:
// the Apple Event being handled is a quit with a quit reason that says
// so. It's 0 off the main thread, where no Apple Event is handled.
int benchQuitForLogOut(void);

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
// benchPressClose presses a window's close button, returning
// BENCH_ERR_NO_CLOSE_BUTTON for a window without one, or whose button is
// disabled. benchPressCloseItem presses the Close Window, or Close, menu
// item of an app, which closes its main window, returning
// BENCH_ERR_NO_MENU_ITEM if it has none that's enabled.
int benchPressClose(uint32_t windowID, int32_t pidHint);
int benchPressCloseItem(int32_t pid);
void benchFreeWindowInfo(BenchWindowInfo *info);
void benchFreeWindowInfos(BenchWindowInfo *infos, int count);

// Apps (apps_darwin.m).
int benchRunningApps(BenchAppInfo **out, int *count);
void benchFreeAppInfos(BenchAppInfo *infos, int count);
// benchOwnAppHidden reports whether Bench itself is hidden. It runs on the
// calling thread.
int benchOwnAppHidden(void);
int32_t benchResolvePID(int32_t pid, const char *bundleID);
int benchOpenApp(const char *bundleID, const char *url);
int benchHideApp(int32_t pid, int hide);
// benchDefaultBrowser returns the bundle ID of the app that opens https
// links, malloc'd for the caller to free with free, or NULL if there's none,
// or it's an agent (LSUIElement or LSBackgroundOnly), such as a browser
// picker.
char *benchDefaultBrowser(void);
// benchOpenTextFile opens the file at path in the default text editor: the
// app that opens plain text, or TextEdit. benchShowInFinder opens a Finder
// window with the file selected. Both return once macOS has been asked.
int benchOpenTextFile(const char *path);
int benchShowInFinder(const char *path);

// Screens and permissions (screens_darwin.m).
void benchRefreshScreens(void);
int benchScreenRecordingAllowed(int request);
int benchOSMajorVersion(void);
int benchHasBundleID(void);

// Bench's window (window_darwin.m).
void benchAttachWindow(void *nsWindow);
void benchFillWindow(void *nsWindow);
void benchSetRaised(int raised);

// Web views (web_darwin.m). Frames are in content coordinates (top-left
// origin, y down, points). name is the item's name, which Bench's prompts
// about its page use.
void benchWebShow(const char *itemID, const char *name, const char *url, const char *profileUUID, const char *userAgentSuffix, double x, double y, double w, double h);
void benchWebHide(const char *itemID);
void benchWebClose(const char *itemID);
void benchWebPopOut(const char *itemID, const char *title);
void benchWebDock(const char *itemID);
// benchWebFocus activates Bench and gives a web view the keyboard focus;
// benchFocusUI gives it to the UI's own web view.
void benchWebFocus(const char *itemID);
void benchFocusUI(void);

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
extern void benchEmitPageEvent(int kind, char *itemID, char *itemURL, char *pageURL, int onlySecureContent);
extern void benchScreensUpdated(BenchScreenInfo *screens, int count, int changed);
extern void benchContentFrameUpdated(int attached, double x, double y, double w, double h, double primaryHeight);
extern void benchScrollBarsUpdated(int shown);
extern void benchWillPowerOff(void);
extern int benchNavDecide(char *itemID, char *itemURL, uint64_t navigation, char *targetURL, int kind, int source, char *sourceOrigin, double userEvent, double now, int *returnHome);
extern void benchNavLoadRequested(char *itemID, uint64_t navigation);
extern void benchNavStarted(char *itemID, uint64_t navigation);
extern void benchNavCommitted(char *itemID, char *itemURL, uint64_t navigation, char *pageURL, double now);
extern int benchNavFailed(char *itemID, uint64_t navigation, int replaced);
extern void benchNavProcessEnded(char *itemID);
extern void benchNavForget(char *itemID);
extern int benchDecidePopup(char *itemID, char *itemURL, char *targetURL, char *sourceOrigin, int sourceInItem, double userEvent, double now);
extern int benchDecidePopupNavigation(char *itemID, char *itemURL, char *startURL, char *targetURL, int userClicked, double userEvent, double now);
extern int benchDecideResponse(char *itemURL, char *responseURL, int mainFrame, int canShow, char *contentDisposition);
extern char *benchDownloadPath(char *dir, char *suggested);
extern void benchDownloadEnded(char *path);
extern int benchNewWindowRank(char *title);
extern int benchCloseWindowRank(char *title);
extern char *benchPageSubtitle(char *pageURL, int onlySecureContent);
extern int benchNotificationAllowed(char *itemURL, char *origin, int mainFrame);
extern int benchUserClicked(int linkActivated, double sinceUserEvent);
extern int benchAllowDownload(char *itemID, double userEvent, double now);
extern char *benchDialogText(char *text, int length);
extern char *benchDialogHeading(char *pageURL, char *frameOrigin, int mainFrame);
extern char *benchMediaPrompt(char *itemURL, char *itemName, char *pageURL, char *frameOrigin, int mainFrame, int popup, int media, char **detail);
extern int benchPromptAllowed(char *itemID, int kind, double now);
extern int benchPromptOfferStop(char *itemID, double now);
extern void benchPromptEnded(char *itemID, int kind, int stop, double now);

#endif
