// bridge_darwin.h declares the native side of internal/mac for cgo, and the
// Go functions that the native side calls back.
//
// Only C types appear here, so that cgo can read it. Every BENCH_ constant
// has a Go constant with the same value in codes.go (or navigation.go,
// navstate.go, prompts.go, and pagecontrols.go); codes_test.go checks that
// they match.
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
#define BENCH_EV_WINDOW_DRAGGED 21
#define BENCH_EV_WINDOW_DROPPED 22
#define BENCH_EV_POP_OUT_FOCUSED 23
#define BENCH_EV_POP_OUT_UNFOCUSED 24
#define BENCH_EV_DOCK_REQUESTED 25
#define BENCH_EV_WOKE 26
#define BENCH_EV_SCREENS_WOKE 27
#define BENCH_EV_SESSION_ACTIVE 28
#define BENCH_EV_SCREEN_UNLOCKED 29
#define BENCH_EV_DESKTOP_CHANGED 30

// The modifier keys that benchHeldModifiers reports, as bits
// (mac.heldModifiers).
#define BENCH_MOD_CONTROL 1
#define BENCH_MOD_OPTION 2
#define BENCH_MOD_SHIFT 4
#define BENCH_MOD_COMMAND 8

// What the drop guide draws (mac.guideMarks): kinds of mark, the side each
// target stands for, and the targets' size, in points.
#define BENCH_GUIDE_AREA 0
#define BENCH_GUIDE_MARK 1
#define BENCH_GUIDE_MARK_ON 2
#define BENCH_GUIDE_LABEL 3
#define BENCH_GUIDE_MIDDLE 0
#define BENCH_GUIDE_LEFT 1
#define BENCH_GUIDE_RIGHT 2
#define BENCH_GUIDE_TOP 3
#define BENCH_GUIDE_BOTTOM 4
#define BENCH_GUIDE_MARK_SIZE 40

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

// How a page uses the camera or microphone, as WebKit's WKMediaCaptureState
// numbers it (mac.captureNone and so on).
#define BENCH_CAPTURE_NONE 0
#define BENCH_CAPTURE_ACTIVE 1
#define BENCH_CAPTURE_MUTED 2

// The appearance setting of Bench's windows (mac.appearanceCode).
#define BENCH_APPEARANCE_SYSTEM 0
#define BENCH_APPEARANCE_LIGHT 1
#define BENCH_APPEARANCE_DARK 2

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

// Something the drop guide draws, in the panel's coordinates (AppKit's:
// bottom-left origin, y up, points): its kind (BENCH_GUIDE_AREA and so on),
// and the side a target stands for.
typedef struct {
	int kind;
	int side;
	double x;
	double y;
	double w;
	double h;
} BenchGuideMark;

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

// A symbolic hot key, as com.apple.symbolichotkeys keeps it: whether it's
// turned on, its virtual key code, and its NSEventModifierFlags.
typedef struct {
	int enabled;
	int keyCode;
	int flags;
} BenchHotKeyInfo;

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
// benchChooseToAdd shows the open panel, as a window of its own, for an app,
// or a program or script, starting in the folder start, and returns at
// once: benchChoseToAdd gets the path chosen, or NULL, when the user is
// done. It returns BENCH_ERR_FAILED, and shows nothing, if the main thread
// isn't running its run loop. benchFrontChooser brings the panel, while
// it's open, to the front.
int benchChooseToAdd(const char *start);
void benchFrontChooser(void);

// Keys (keys_darwin.m). benchHeldModifiers returns the modifier keys held
// down now, wherever the keyboard is, as BENCH_MOD_ bits. It runs on the
// calling thread, and needs no permission.
int benchHeldModifiers(void);

// Screens and permissions (screens_darwin.m).
void benchRefreshScreens(void);
int benchScreenRecordingAllowed(int request);
// benchOpenSettingsURL opens a pane of System Settings by its address, such
// as x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility,
// and benchOpenLoginItems opens General ▸ Login Items. Both return once
// macOS has been asked. benchLoginItemNeedsApproval reports whether macOS
// needs the user to allow Bench's login item there (1) or not (0).
int benchOpenSettingsURL(const char *url);
void benchOpenLoginItems(void);
int benchLoginItemNeedsApproval(void);
int benchOSMajorVersion(void);
int benchHasBundleID(void);

// Apps' icons (apps_darwin.m). benchAppIcon draws an app's icon as a PNG
// pixels square into a malloc'd buffer, which the caller frees: BENCH_OK,
// BENCH_ERR_NOT_INSTALLED, or BENCH_ERR_FAILED. It draws on the calling
// thread, one icon at a time, never on the main thread.
int benchAppIcon(const char *bundleID, int pixels, void **png, int *length);

// Bench's window (window_darwin.m). benchSetWindowAppearance gives one of
// Bench's windows a BENCH_APPEARANCE_ (none, for system, follows macOS),
// and a background that matches the appearance it has.
// benchSetPanelsAppearance gives Bench's panels, the drop guide and the
// live preview, one, which they take as they next show. benchSystemDark
// reports whether macOS is in dark mode, from the user's defaults, on any
// thread, before the app runs too.
void benchAttachWindow(void *nsWindow);
void benchFillWindow(void *nsWindow);
void benchSetRaised(int raised);
void benchSetWindowAppearance(void *nsWindow, int appearance);
void benchSetPanelsAppearance(int appearance);
int benchSystemDark(void);

// Web views (web_darwin.m). Frames are in content coordinates (top-left
// origin, y down, points). name is the item's name, which Bench's prompts
// about its page use, and zoom its page's zoom (1 for actual size).
void benchWebShow(const char *itemID, const char *name, const char *url, const char *profileUUID, const char *userAgentSuffix, double zoom, double x, double y, double w, double h);
void benchWebHide(const char *itemID);
void benchWebClose(const char *itemID);
void benchWebPopOut(const char *itemID, const char *title);
void benchWebDock(const char *itemID);
// benchWebFocus activates Bench and gives a web view the keyboard focus;
// benchFocusUI gives it to the UI's own web view.
void benchWebFocus(const char *itemID);
void benchFocusUI(void);
// A page's controls (web_darwin.m). benchWebReload loads the page again,
// benchWebNavigate goes back (step -1) or forward (1), and benchWebZoom sets
// its zoom. benchWebFind finds text in it, and benchWebFound gets whether
// there's a match, with the request's number, once WebKit says.
// benchWebSetCapture sets how the page uses the camera or microphone
// (device, BENCH_MEDIA_CAMERA or BENCH_MEDIA_MICROPHONE) to a
// BENCH_CAPTURE_ state, if it's using it, and benchWebPauseMedia pauses its
// sound and video, and its popups'. Each returns at once.
void benchWebReload(const char *itemID);
void benchWebNavigate(const char *itemID, int step);
void benchWebZoom(const char *itemID, double zoom);
void benchWebFind(const char *itemID, const char *text, int backwards, uint64_t request);
void benchWebSetCapture(const char *itemID, int device, int state);
void benchWebPauseMedia(const char *itemID);

// Live previews (preview_darwin.m). The panel frame is in AppKit
// coordinates; the capture size is in pixels.
void benchPreviewShowWindow(uint32_t windowID, const char *title, double x, double y, double w, double h, int captureW, int captureH);
void benchPreviewShowWeb(const char *itemID, const char *title, double x, double y, double w, double h);
void benchPreviewHide(void);

// Drags (drag_darwin.m). benchTrackDrags turns on or off watching the user
// drag windows by their title bars: other apps' windows, through a global
// monitor of the left mouse button, and Bench's own popped-out windows.
// benchTagPopOut marks one of Bench's windows as an item's popped-out window
// (its identifier is "bench.item:" and the item's ID), for its drags, and
// when it gets and loses the keyboard, to be reported; popped-out web
// pages' windows are marked as they're made.
void benchTrackDrags(int on);
void benchTagPopOut(void *nsWindow, const char *itemID);

// The drop guide (guide_darwin.m): a panel at a frame in AppKit coordinates,
// above every window, letting the mouse through, with count marks, which
// are copied, and a label.
void benchShowDropGuide(double x, double y, double w, double h, const BenchGuideMark *marks, int count, const char *label);
void benchHideDropGuide(void);

// Notifications and the Dock icon's badge (notify_darwin.m).
void benchNotify(const char *itemID, const char *title, const char *subtitle, const char *body);
void benchSetBadge(int count);

// Updates, with Sparkle, if Bench.app has it (updater_darwin.m).
// benchUpdaterAvailable reports whether it does, with an update key.
// benchUpdaterStart loads and starts it, with its daily look turned on or
// off; the others do nothing until it has started.
int benchUpdaterAvailable(void);
void benchUpdaterStart(int automatic);
void benchUpdaterSetAutomatic(int automatic);
void benchUpdaterCheck(void);

// Go functions the native side calls (exports_darwin.go). They never block.
// The declarations match the ones cgo writes in _cgo_export.h.
extern void benchEmitWindowEvent(int kind, BenchWindowInfo *info);
extern void benchEmitAppEvent(int kind, BenchAppInfo *info);
extern void benchEmitItemEvent(int kind, char *itemID, char *title, char *body);
extern void benchEmitPageEvent(int kind, char *itemID, char *itemURL, char *pageURL, int onlySecureContent);
// benchEmitWebState reports what a Web item's page can do and is doing now
// (WebStateChanged): whether its view can go back or forward, and how it
// uses the camera and microphone, as BENCH_CAPTURE_ states.
extern void benchEmitWebState(char *itemID, int canGoBack, int canGoForward, int camera, int microphone);
// benchPageIconLinks passes on the <link> elements of the page that a Web
// item's view has loaded, at pageURL, as JSON, for Go to fetch its icon.
extern void benchPageIconLinks(char *itemID, char *pageURL, char *links);
// benchEmitDragEvent reports a drag (BENCH_EV_WINDOW_DRAGGED) or a drop
// (BENCH_EV_WINDOW_DROPPED) of another app's window (windowID and pid, with
// itemID empty) or of one of Bench's popped-out windows (itemID): the
// window's frame as the drag began (start), its frame now, and the pointer,
// all in global coordinates (top-left origin, y down, points).
extern void benchEmitDragEvent(int kind, uint32_t windowID, int32_t pid, char *itemID, double startX, double startY, double startW, double startH, double x, double y, double w, double h, double pointerX, double pointerY);
extern void benchScreensUpdated(BenchScreenInfo *screens, int count, int changed);
extern void benchContentFrameUpdated(int attached, double x, double y, double w, double h, double primaryHeight);
extern void benchScrollBarsUpdated(int shown);
extern void benchStageManagerUpdated(int on);
extern void benchSystemKeysUpdated(BenchHotKeyInfo *keys, int count, int sources, int voiceOver);
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
extern void benchChoseToAdd(char *path);
extern void benchWebFound(uint64_t request, int found);
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
// Sparkle's news (updater_darwin.go): a newer version that a daily look
// found, that it's done with that version, and a failure.
extern void benchUpdateFound(char *version);
extern void benchUpdateFinished(void);
extern void benchUpdateFailed(char *text);

#endif
