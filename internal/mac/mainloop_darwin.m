//go:build darwin && cgo && !server

// mainloop_darwin.m: running the main thread's run loop without
// NSApplication, for `bench arrange`, and the temporary folder.

#import <Foundation/Foundation.h>

#include <dispatch/dispatch.h>
#include <stdlib.h>
#include <string.h>

// Set by benchStopLoopOnMain. Main thread only.
static BOOL gLoopStopped = NO;

// benchStopLoopOnMain runs on the main queue, so inside benchRunMainLoop's
// run loop, which it stops.
static void benchStopLoopOnMain(void *context) {
	(void)context;
	gLoopStopped = YES;
	CFRunLoopStop(CFRunLoopGetMain());
}

int benchRunMainLoop(void) {
	if (![NSThread isMainThread]) {
		return 0;
	}
	gLoopStopped = NO;
	while (!gLoopStopped) {
		CFRunLoopRunResult result;
		@autoreleasepool {
			result = CFRunLoopRunInMode(kCFRunLoopDefaultMode, 1.0, false);
		}
		if (result == kCFRunLoopRunFinished) {
			// Nothing to run, not even the main queue, so nothing could
			// stop the loop.
			return 0;
		}
	}
	return 1;
}

void benchStopMainLoop(void) {
	// Through the main queue, which the run loop serves, so that a stop
	// that comes before the loop runs isn't lost, and so that work sent to
	// the main queue before it still runs.
	dispatch_async_f(dispatch_get_main_queue(), NULL, benchStopLoopOnMain);
}

char *benchTemporaryDirectory(void) {
	@autoreleasepool {
		const char *path = NSTemporaryDirectory().UTF8String;
		return path != NULL ? strdup(path) : NULL;
	}
}
