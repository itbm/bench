// internal_darwin.h shares declarations between the Objective-C files of
// internal/mac. Only .m files include it; cgo sees bridge_darwin.h alone.

#ifndef BENCH_INTERNAL_DARWIN_H
#define BENCH_INTERNAL_DARWIN_H

#import <Foundation/Foundation.h>
#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <WebKit/WebKit.h>

#include "bridge_darwin.h"

NS_ASSUME_NONNULL_BEGIN

// ---------- Threads (observe_darwin.m) ----------

// benchAXQueue is the serial queue for Accessibility observer work: making
// observers, registering notifications, and handling them. It keeps that
// work, which talks to other processes and can wait up to the messaging
// timeout, off the main thread, and in order.
dispatch_queue_t benchAXQueue(void);

// benchOnMain runs block on the main thread: at once if this is the main
// thread, or else later, with dispatch_async. It never waits.
void benchOnMain(dispatch_block_t block);

// benchOnMainIfRunning runs block on the main thread if the main thread is
// running its run loop (as in the app), and otherwise on this thread at
// once. It's for the few AppKit calls that are safe off the main thread
// but should run there when they can, so that `bench arrange`, which has
// no run loop, still works.
void benchOnMainIfRunning(dispatch_block_t block);

// benchStopped reports whether benchStop has run (and benchStart hasn't run
// again since).
BOOL benchStopped(void);

// ---------- Strings (observe_darwin.m) ----------

// benchString copies a C string into an NSString. It returns @"" for NULL
// or text that isn't valid UTF-8, never nil.
NSString *benchString(const char *_Nullable s);

// benchCopyCString returns a malloc'd copy of s in UTF-8, or NULL for nil.
char *_Nullable benchCopyCString(NSString *_Nullable s);

// benchUTF8 returns s in UTF-8, valid while s is, never NULL.
const char *benchUTF8(NSString *_Nullable s);

// ---------- Accessibility (ax_darwin.m) ----------

// benchHandleAXNotification handles an Accessibility notification on the
// Accessibility queue. refID is the window ID registered with per-window
// notifications, or 0.
void benchHandleAXNotification(AXUIElementRef element, NSString *notification, pid_t pid, uint32_t refID);

// benchObserveApp starts watching an app's windows, retrying with a
// growing delay while the app isn't ready, up to a few times. With
// announce, windows it finds once watching starts are reported as created,
// for apps that launched after Bench started; announce is remembered across
// retries, and if the app is already being watched its windows are announced
// at once. Accessibility queue only.
void benchObserveApp(pid_t pid, BOOL announce, int attempt);

// benchObserveAllApps starts watching every regular app that's running, if
// Bench has Accessibility permission, and otherwise checks again every few
// seconds until it does. Accessibility queue only.
void benchObserveAllApps(void);

// benchForgetApp stops watching an app that quit and forgets its windows.
// Accessibility queue only.
void benchForgetApp(pid_t pid);

// benchForgetEverything stops watching every app and empties the window
// registry. Accessibility queue only.
void benchForgetEverything(void);

// benchEmitFocusedWindow reports an app's focused window as focused, if the
// app is frontmost. Accessibility queue only.
void benchEmitFocusedWindow(pid_t pid);

// ---------- Apps (apps_darwin.m) ----------

// BenchAppSnapshot is what an app event reports, taken on the thread that
// saw the change. For BENCH_EV_APP_TERMINATED the app is neither active nor
// hidden; for BENCH_EV_APP_ACTIVATED it's active and not hidden, whatever
// NSRunningApplication says before its properties catch up.
@interface BenchAppSnapshot : NSObject
@property (nonatomic, readonly) pid_t pid;
@property (nonatomic, readonly, copy) NSString *bundleID;
@property (nonatomic, readonly, copy) NSString *name;
@property (nonatomic, readonly) BOOL hidden;
@property (nonatomic, readonly) BOOL active;
@property (nonatomic, readonly) BOOL regular;
- (instancetype)initWithApp:(NSRunningApplication *)app event:(int)kind NS_DESIGNATED_INITIALIZER;
- (instancetype)init NS_UNAVAILABLE;
@end

// benchEmitApp reports an app event to Go.
void benchEmitApp(int kind, BenchAppSnapshot *snapshot);

// benchIsRegular reports whether an app appears in the Dock.
BOOL benchIsRegular(NSRunningApplication *app);

// ---------- Screens (screens_darwin.m) ----------

// benchCollectScreens reports the screens to Go. With changed, Go also sends
// ScreensChanged. Main thread, except once at start.
void benchCollectScreens(BOOL changed);

// ---------- Bench's window (window_darwin.m) ----------

// benchWebContainerView returns the view that holds web views in Bench's
// window, or nil before AttachWindow. Main thread only.
NSView *_Nullable benchWebContainerView(void);

// benchBenchWindow returns Bench's window, or nil. Main thread only.
NSWindow *_Nullable benchBenchWindow(void);

// benchReportContentFrame tells Go where Bench's content area is. Main
// thread only.
void benchReportContentFrame(void);

// benchDetachWindow stops watching Bench's window. Main thread only.
void benchDetachWindow(void);

// ---------- Web views (web_darwin.m) ----------

// benchWebAttachContainer puts the web views that exist into a new
// container. Main thread only.
void benchWebAttachContainer(NSView *container);

// benchWebViewForItem returns an item's web view, or nil. Main thread only.
WKWebView *_Nullable benchWebViewForItem(NSString *itemID);

// benchWebCloseAll closes every web view. Main thread only.
void benchWebCloseAll(void);

// ---------- Previews (preview_darwin.m) ----------

// benchPreviewStopOnMain hides the preview and stops capturing. Main thread
// only.
void benchPreviewStopOnMain(void);

// ---------- Notifications (notify_darwin.m) ----------

// benchNotificationsSetUp makes Bench the notification centre's delegate,
// so clicks are reported. It does nothing without a bundle identifier.
void benchNotificationsSetUp(void);

// benchEmitItem reports an item event (a web view's title, address, or
// notification, or a click on a notification) to Go.
void benchEmitItem(int kind, NSString *itemID, NSString *_Nullable title, NSString *_Nullable body);

NS_ASSUME_NONNULL_END

#endif
