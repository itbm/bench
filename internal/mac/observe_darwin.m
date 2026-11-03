//go:build darwin && cgo && !server

// observe_darwin.m: starting and stopping, the threads, strings, the
// NSWorkspace notifications about apps launching, quitting, and coming
// forward, the Mac waking and the user coming back to it, and why macOS
// asks Bench to quit.

#import "internal_darwin.h"

#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static atomic_bool gStopped = false;
// Set once a block sent to the main queue has run, which happens only when
// the main thread runs its run loop: in the app, and in `bench arrange` once
// it calls RunMainLoop.
static atomic_bool gMainRunning = false;

// Main thread only.
static NSMutableArray *gWorkspaceTokens = nil;
static id gScreenToken = nil;
static id gActiveToken = nil;
static id gScrollerToken = nil;
static id gUnlockObserver = nil; // A BenchUnlockObserver.

#pragma mark - Threads

dispatch_queue_t benchAXQueue(void) {
	static dispatch_queue_t queue;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		queue = dispatch_queue_create("com.github.itbm.bench.accessibility", DISPATCH_QUEUE_SERIAL);
	});
	return queue;
}

void benchOnMain(dispatch_block_t block) {
	if ([NSThread isMainThread]) {
		block();
	} else {
		dispatch_async(dispatch_get_main_queue(), block);
	}
}

void benchOnMainIfRunning(dispatch_block_t block) {
	if ([NSThread isMainThread] || !atomic_load(&gMainRunning)) {
		block();
	} else {
		dispatch_async(dispatch_get_main_queue(), block);
	}
}

BOOL benchStopped(void) {
	return atomic_load(&gStopped) ? YES : NO;
}

BOOL benchMainRunning(void) {
	return atomic_load(&gMainRunning) ? YES : NO;
}

#pragma mark - Strings

NSString *benchString(const char *s) {
	if (s == NULL) {
		return @"";
	}
	NSString *string = [NSString stringWithUTF8String:s];
	return string ?: @"";
}

char *benchCopyCString(NSString *s) {
	if (s == nil) {
		return NULL;
	}
	const char *utf8 = s.UTF8String;
	if (utf8 == NULL) {
		return strdup("");
	}
	return strdup(utf8);
}

const char *benchUTF8(NSString *s) {
	const char *utf8 = s.UTF8String;
	return utf8 != NULL ? utf8 : "";
}

#pragma mark - Apps coming and going

// benchNotedApp returns the app a workspace notification is about, or nil
// for Bench itself, whose activation benchWatchWorkspace reports from its
// own notification.
static NSRunningApplication *benchNotedApp(NSNotification *note) {
	NSRunningApplication *app = note.userInfo[NSWorkspaceApplicationKey];
	if (![app isKindOfClass:[NSRunningApplication class]] || app.processIdentifier == getpid()) {
		return nil;
	}
	return app;
}

// benchEmitQueued reports an event with no item through the Accessibility
// queue, so that it reaches Go after the window events before it.
static void benchEmitQueued(int kind) {
	dispatch_async(benchAXQueue(), ^{
		if (benchStopped()) {
			return;
		}
		benchEmitItem(kind, @"", nil, nil);
	});
}

// BenchUnlockObserver hears the com.apple.screenIsUnlocked distributed
// notification. AppKit holds distributed notifications back while Bench
// isn't the active app, and an observer with a block can't ask for them
// at once, so this one has a selector, and asks for
// NSNotificationSuspensionBehaviorDeliverImmediately: the unlock reaches Go
// as it happens, not the next time Bench becomes active.
@interface BenchUnlockObserver : NSObject
@end

@implementation BenchUnlockObserver

- (void)screenUnlocked:(NSNotification *)note {
	(void)note;
	benchEmitQueued(BENCH_EV_SCREEN_UNLOCKED);
}

@end

// benchWatchWorkspace starts observing apps launching, quitting, and
// activating, the user logging out, the Mac waking, the user coming back
// to their session or to Bench's desktop, the screens changing, and macOS
// changing how it shows scroll bars. The handlers run on the posting
// thread (queue:nil), which is the main thread for these notifications.
// Each hands its work to the Accessibility queue, so that app events and
// window events reach Go in the order they happened. Main thread only.
static void benchWatchWorkspace(void) {
	if (gWorkspaceTokens != nil) {
		return;
	}
	gWorkspaceTokens = [NSMutableArray array];
	NSNotificationCenter *center = [[NSWorkspace sharedWorkspace] notificationCenter];

	id launched = [center addObserverForName:NSWorkspaceDidLaunchApplicationNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		NSRunningApplication *app = benchNotedApp(note);
		if (app == nil) {
			return;
		}
		BenchAppSnapshot *snapshot = [[BenchAppSnapshot alloc] initWithApp:app event:BENCH_EV_APP_LAUNCHED];
		pid_t pid = app.processIdentifier;
		BOOL regular = benchIsRegular(app);
		dispatch_async(benchAXQueue(), ^{
			if (benchStopped()) {
				return;
			}
			benchEmitApp(BENCH_EV_APP_LAUNCHED, snapshot);
			if (regular) {
				benchObserveApp(pid, YES, 0);
			}
		});
	}];
	[gWorkspaceTokens addObject:launched];

	id terminated = [center addObserverForName:NSWorkspaceDidTerminateApplicationNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		NSRunningApplication *app = benchNotedApp(note);
		if (app == nil) {
			return;
		}
		BenchAppSnapshot *snapshot = [[BenchAppSnapshot alloc] initWithApp:app event:BENCH_EV_APP_TERMINATED];
		pid_t pid = app.processIdentifier;
		dispatch_async(benchAXQueue(), ^{
			if (benchStopped()) {
				return;
			}
			benchForgetApp(pid);
			benchEmitApp(BENCH_EV_APP_TERMINATED, snapshot);
		});
	}];
	[gWorkspaceTokens addObject:terminated];

	id activated = [center addObserverForName:NSWorkspaceDidActivateApplicationNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		NSRunningApplication *app = benchNotedApp(note);
		if (app == nil) {
			return;
		}
		BenchAppSnapshot *snapshot = [[BenchAppSnapshot alloc] initWithApp:app event:BENCH_EV_APP_ACTIVATED];
		pid_t pid = app.processIdentifier;
		BOOL regular = benchIsRegular(app);
		dispatch_async(benchAXQueue(), ^{
			if (benchStopped()) {
				return;
			}
			benchEmitApp(BENCH_EV_APP_ACTIVATED, snapshot);
			if (regular) {
				// Activating an app brings its focused window forward without
				// changing which window is focused, so no Accessibility
				// notification says so: report it here, for focus following.
				benchObserveApp(pid, NO, 0);
				benchEmitFocusedWindow(pid);
			}
		});
	}];
	[gWorkspaceTokens addObject:activated];

	// The user chose Log Out, Restart, or Shut Down: the apps that quit
	// next quit because of it, so Go doesn't take those quits for the
	// user's own. Through the Accessibility queue, so that it reaches Go
	// before the quits do. Go also notes it at once, so that Bench's own
	// quit, which macOS asks for next, doesn't wait to ask about its
	// terminals (Platform.LoggingOut).
	id poweringOff = [center addObserverForName:NSWorkspaceWillPowerOffNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
		benchWillPowerOff();
		dispatch_async(benchAXQueue(), ^{
			if (benchStopped()) {
				return;
			}
			benchEmitItem(BENCH_EV_LOGGING_OUT, @"", nil, nil);
		});
	}];
	[gWorkspaceTokens addObject:poweringOff];

	// The Mac woke from sleep, its displays woke, or the user's session
	// became active again, as after fast user switching. macOS can move
	// windows as each happens, as displays come back, so Go waits for the
	// screens to settle and arranges the tab again.
	NSDictionary<NSString *, NSNumber *> *wakes = @{
		NSWorkspaceDidWakeNotification : @(BENCH_EV_WOKE),
		NSWorkspaceScreensDidWakeNotification : @(BENCH_EV_SCREENS_WOKE),
		NSWorkspaceSessionDidBecomeActiveNotification : @(BENCH_EV_SESSION_ACTIVE),
	};
	for (NSString *name in wakes) {
		int kind = wakes[name].intValue;
		id woke = [center addObserverForName:name object:nil queue:nil usingBlock:^(NSNotification *note) {
			(void)note;
			benchEmitQueued(kind);
		}];
		[gWorkspaceTokens addObject:woke];
	}

	// The user went to another desktop (Space). It's reported only while
	// Bench's window is on a desktop that's shown, which, with "Displays
	// have separate Spaces" on, includes a change on another display: Go
	// looks again then for the windows that it couldn't see from another
	// desktop. Arranging from another desktop would only find them away.
	id space = [center addObserverForName:NSWorkspaceActiveSpaceDidChangeNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
		NSWindow *window = benchBenchWindow();
		if (window != nil && window.isVisible && window.isOnActiveSpace) {
			benchEmitQueued(BENCH_EV_DESKTOP_CHANGED);
		}
	}];
	[gWorkspaceTokens addObject:space];

	// The user unlocked the screen. Apple doesn't document this
	// notification, so it may not come; the screens waking and the session
	// becoming active come at the same times.
	gUnlockObserver = [[BenchUnlockObserver alloc] init];
	[[NSDistributedNotificationCenter defaultCenter] addObserver:gUnlockObserver selector:@selector(screenUnlocked:) name:@"com.apple.screenIsUnlocked" object:nil suspensionBehavior:NSNotificationSuspensionBehaviorDeliverImmediately];

	gScreenToken = [[NSNotificationCenter defaultCenter] addObserverForName:NSApplicationDidChangeScreenParametersNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
		benchCollectScreens(YES);
		benchReportContentFrame();
	}];

	// The user changed System Settings' Show scroll bars, or connected or
	// disconnected a mouse, which can change it. The tab and sub-tab
	// strips follow it while the scroll_bars setting is auto.
	gScrollerToken = [[NSNotificationCenter defaultCenter] addObserverForName:NSPreferredScrollerStyleDidChangeNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
		benchOnMain(^{
			benchCollectScrollBars();
		});
	}];

	// VoiceOver, and the input sources, for the keys that macOS keeps for
	// itself.
	benchWatchSystemKeys();

	// Bench's own popped-out windows getting and losing the keyboard, for
	// the Window menu's Back to Bench.
	benchWatchPopOuts();

	// Bench itself coming forward, which Go answers by hiding apps whose
	// windows cover Bench's (Bench sits below normal windows, so it can't
	// come in front of them), and by checking Screen Recording again. Only
	// the app event: Bench's own windows aren't items, and asking Bench's
	// own process through the Accessibility API would wait for the main
	// thread. The input sources' shortcuts, and whether Stage Manager is
	// on, are read again too, as the user may have changed them in System
	// Settings or Control Centre, which say nothing.
	gActiveToken = [[NSNotificationCenter defaultCenter] addObserverForName:NSApplicationDidBecomeActiveNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
		benchCollectSystemKeys();
		benchCollectStageManager();
		BenchAppSnapshot *snapshot = [[BenchAppSnapshot alloc] initWithApp:[NSRunningApplication currentApplication] event:BENCH_EV_APP_ACTIVATED];
		dispatch_async(benchAXQueue(), ^{
			if (benchStopped()) {
				return;
			}
			benchEmitApp(BENCH_EV_APP_ACTIVATED, snapshot);
		});
	}];
}

// benchUnwatchWorkspace stops what benchWatchWorkspace started. Main thread
// only.
static void benchUnwatchWorkspace(void) {
	NSNotificationCenter *center = [[NSWorkspace sharedWorkspace] notificationCenter];
	for (id token in gWorkspaceTokens) {
		[center removeObserver:token];
	}
	gWorkspaceTokens = nil;
	if (gScreenToken != nil) {
		[[NSNotificationCenter defaultCenter] removeObserver:gScreenToken];
		gScreenToken = nil;
	}
	if (gActiveToken != nil) {
		[[NSNotificationCenter defaultCenter] removeObserver:gActiveToken];
		gActiveToken = nil;
	}
	if (gScrollerToken != nil) {
		[[NSNotificationCenter defaultCenter] removeObserver:gScrollerToken];
		gScrollerToken = nil;
	}
	if (gUnlockObserver != nil) {
		[[NSDistributedNotificationCenter defaultCenter] removeObserver:gUnlockObserver];
		gUnlockObserver = nil;
	}
	benchUnwatchSystemKeys();
	benchUnwatchPopOuts();
}

#pragma mark - Starting and stopping

void benchStart(void) {
	atomic_store(&gStopped, false);
	benchNotificationsSetUp();
	dispatch_async(dispatch_get_main_queue(), ^{
		atomic_store(&gMainRunning, true);
		if (benchStopped()) {
			return;
		}
		benchWatchWorkspace();
		benchCollectScreens(NO);
		benchCollectScrollBars();
		benchCollectSystemKeys();
		benchCollectStageManager();
	});
	dispatch_async(benchAXQueue(), ^{
		benchObserveAllApps();
	});
}

void benchStop(void) {
	atomic_store(&gStopped, true);
	benchOnMain(^{
		benchUnwatchWorkspace();
		benchPreviewStopOnMain();
		benchDropGuideStopOnMain();
		benchWebCloseAll();
		benchDetachWindow();
	});
	dispatch_async(benchAXQueue(), ^{
		benchForgetEverything();
	});
}

#pragma mark - Quitting

int benchQuitForLogOut(void) {
	if (![NSThread isMainThread]) {
		return 0;
	}
	@autoreleasepool {
		// ⌘Q and the menus call terminate: with no Apple Event. Logging
		// out, restarting, and shutting down send a quit event whose quit
		// reason says which; the Dock's Quit sends one with none.
		NSAppleEventDescriptor *event = [[NSAppleEventManager sharedAppleEventManager] currentAppleEvent];
		if (event == nil || event.eventClass != kCoreEventClass || event.eventID != kAEQuitApplication) {
			return 0;
		}
		NSAppleEventDescriptor *reason = [event attributeDescriptorForKeyword:kAEQuitReason];
		if (reason == nil) {
			return 0;
		}
		OSType why = reason.enumCodeValue;
		if (why == 0) {
			why = reason.typeCodeValue;
		}
		switch (why) {
		case kAELogOut:
		case kAEReallyLogOut:
		case kAEShowRestartDialog:
		case kAERestart:
		case kAEShowShutdownDialog:
		case kAEShutDown:
			return 1;
		default:
			return 0;
		}
	}
}
