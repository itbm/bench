//go:build darwin && cgo && !server

// observe_darwin.m: starting and stopping, the threads, strings, and the
// NSWorkspace notifications about apps launching, quitting, and coming
// forward.

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

// benchWatchWorkspace starts observing apps launching, quitting, and
// activating, the user logging out, the screens changing, and macOS
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
	// before the quits do.
	id poweringOff = [center addObserverForName:NSWorkspaceWillPowerOffNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
		dispatch_async(benchAXQueue(), ^{
			if (benchStopped()) {
				return;
			}
			benchEmitItem(BENCH_EV_LOGGING_OUT, @"", nil, nil);
		});
	}];
	[gWorkspaceTokens addObject:poweringOff];

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

	// Bench itself coming forward, which Go answers by hiding apps whose
	// windows cover Bench's (Bench sits below normal windows, so it can't
	// come in front of them), and by checking Screen Recording again. Only
	// the app event: Bench's own windows aren't items, and asking Bench's
	// own process through the Accessibility API would wait for the main
	// thread.
	gActiveToken = [[NSNotificationCenter defaultCenter] addObserverForName:NSApplicationDidBecomeActiveNotification object:nil queue:nil usingBlock:^(NSNotification *note) {
		(void)note;
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
		benchWebCloseAll();
		benchDetachWindow();
	});
	dispatch_async(benchAXQueue(), ^{
		benchForgetEverything();
	});
}
