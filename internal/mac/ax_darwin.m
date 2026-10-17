//go:build darwin && cgo && !server

// ax_darwin.m: other apps' windows, through the Accessibility API.
//
// It keeps a registry from window IDs to retained AXUIElementRefs, because
// cgo doesn't let Go hold these pointers: Go only ever passes integer IDs.
// It watches apps with AXObservers, and moves, raises, and focuses windows.
//
// Threads: the functions in bridge_darwin.h run on the calling goroutine's
// thread. Observer work (making observers, registering notifications, and
// handling them) runs on benchAXQueue(). The registry is guarded by gLock,
// which is never held during an Accessibility call or a call into Go.

#import "internal_darwin.h"

#include <dlfcn.h>
#include <errno.h>
#include <os/lock.h>
#include <signal.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

// How long an Accessibility call waits for an app that doesn't answer, in
// seconds.
static const float BenchMessagingTimeout = 1.0f;

// How many times benchObserveApp tries to watch an app that isn't ready.
static const int BenchObserveAttempts = 7;

// How long a closed window waits before it's reported, in seconds. Many
// apps (Chromium-based browsers among them) close their windows as they
// quit. Reported as closed, those windows' items would be removed, when the
// app quitting should leave them waiting for it to open again (Bench hears
// of that from AppTerminated). So WindowDestroyed waits this long, and isn't
// sent if the app has quit by then.
static const double BenchDestroyedGrace = 1.0;

// "AXFullScreen" has no constant in the SDK's headers.
#define BenchFullScreenAttribute CFSTR("AXFullScreen")

// Set to false while moving a window: when it's true (VoiceOver and some
// other tools turn it on), apps such as Chromium-based browsers animate
// every move, which makes docking slow.
#define BenchEnhancedUserInterfaceAttribute CFSTR("AXEnhancedUserInterface")

#pragma mark - Window identity

// _AXUIElementGetWindow is a private HIServices function that returns a
// window element's CGWindowID. There's no public way to get it; window
// managers such as AeroSpace, yabai, and Rectangle use this one. Bench looks
// it up at run time, so that if it ever goes away Bench falls back to
// synthetic IDs instead of failing to launch.
typedef AXError (*BenchGetWindowFunction)(AXUIElementRef element, CGWindowID *windowID);

static BenchGetWindowFunction benchGetWindowFunction(void) {
	static BenchGetWindowFunction function = NULL;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		function = (BenchGetWindowFunction)dlsym(RTLD_DEFAULT, "_AXUIElementGetWindow");
	});
	return function;
}

// benchWindowNumber returns a window element's CGWindowID, or 0 if it can't
// be read.
static CGWindowID benchWindowNumber(AXUIElementRef element) {
	BenchGetWindowFunction getWindow = benchGetWindowFunction();
	CGWindowID windowID = 0;
	if (getWindow != NULL && getWindow(element, &windowID) == kAXErrorSuccess) {
		return windowID;
	}
	return 0;
}

// benchWindowServerHas reports whether the window server has a window, on
// any desktop, and sets *owner to its app's process ID. Window numbers and
// owners are listed without Screen Recording permission; only titles need
// it.
static BOOL benchWindowServerHas(CGWindowID windowID, pid_t *owner) {
	if (windowID == 0 || windowID >= BENCH_SYNTHETIC_BASE) {
		return NO;
	}
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionAll, kCGNullWindowID);
	if (list == NULL) {
		return NO;
	}
	NSArray *windows = (__bridge_transfer NSArray *)list;
	for (NSDictionary *info in windows) {
		if (![info isKindOfClass:[NSDictionary class]]) {
			continue;
		}
		NSNumber *number = info[(__bridge NSString *)kCGWindowNumber];
		if (![number isKindOfClass:[NSNumber class]] || number.unsignedIntValue != windowID) {
			continue;
		}
		if (owner != NULL) {
			NSNumber *pid = info[(__bridge NSString *)kCGWindowOwnerPID];
			*owner = [pid isKindOfClass:[NSNumber class]] ? pid.intValue : 0;
		}
		return YES;
	}
	return NO;
}

// benchMissingStatus says why a window can't be found through the
// Accessibility API: it's on another desktop if the window server still
// has it, and gone otherwise.
static int benchMissingStatus(uint32_t windowID) {
	return benchWindowServerHas(windowID, NULL) ? BENCH_ERR_OTHER_DESKTOP : BENCH_ERR_NO_WINDOW;
}

#pragma mark - Registry

// BenchWindowEntry is a window Bench has seen.
@interface BenchWindowEntry : NSObject
@property (nonatomic, readonly) uint32_t windowID;
@property (nonatomic, readonly) pid_t pid;
@property (nonatomic, readonly) AXUIElementRef element;
@property (nonatomic, readonly) BOOL synthetic;
@property (nonatomic, readonly) int64_t created;
// What was last read, for events about windows that have gone. Guarded by
// gLock.
@property (nonatomic, copy) NSString *title;
@property (nonatomic, copy) NSString *subrole;
@property (nonatomic) BOOL minimised;
@property (nonatomic) BOOL fullScreen;
// Whether WindowCreated has been sent. Guarded by gLock.
@property (nonatomic) BOOL announced;
// Whether its per-window notifications are registered. Accessibility queue
// only.
@property (nonatomic) BOOL watched;
- (instancetype)initWithID:(uint32_t)windowID pid:(pid_t)pid element:(AXUIElementRef)element synthetic:(BOOL)synthetic created:(int64_t)created;
@end

@implementation BenchWindowEntry

- (instancetype)initWithID:(uint32_t)windowID pid:(pid_t)pid element:(AXUIElementRef)element synthetic:(BOOL)synthetic created:(int64_t)created {
	self = [super init];
	if (self) {
		_windowID = windowID;
		_pid = pid;
		_element = (AXUIElementRef)CFRetain(element);
		_synthetic = synthetic;
		_created = created;
		_title = @"";
		_subrole = @"";
		// Elements made from the app's element don't inherit its timeout.
		AXUIElementSetMessagingTimeout(_element, BenchMessagingTimeout);
	}
	return self;
}

- (void)dealloc {
	if (_element != NULL) {
		CFRelease(_element);
	}
}

@end

// BenchAppEntry is an app Bench has looked at, with its Accessibility
// element and, once it's watched, its observer.
@interface BenchAppEntry : NSObject
@property (nonatomic, readonly) pid_t pid;
@property (nonatomic, readonly, copy) NSString *bundleID;
@property (nonatomic, readonly, copy) NSString *name;
@property (nonatomic, readonly) AXUIElementRef element;
// Accessibility queue only. The entry owns the observer (+1).
@property (nonatomic) AXObserverRef observer;
@property (nonatomic) BOOL watching;
// Whether the windows found when watching starts are to be reported as
// created, because the app launched while Bench was running. It's kept until
// watching starts, however many attempts that takes, so that an activation
// that starts watching first doesn't lose it. Accessibility queue only.
@property (nonatomic) BOOL announcePending;
- (instancetype)initWithPID:(pid_t)pid;
@end

@implementation BenchAppEntry

- (instancetype)initWithPID:(pid_t)pid {
	self = [super init];
	if (self) {
		_pid = pid;
		NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
		_bundleID = [app.bundleIdentifier copy] ?: @"";
		_name = [app.localizedName copy] ?: @"";
		_element = AXUIElementCreateApplication(pid);
		AXUIElementSetMessagingTimeout(_element, BenchMessagingTimeout);
	}
	return self;
}

- (void)dealloc {
	if (_observer != NULL) {
		CFRunLoopRemoveSource(CFRunLoopGetMain(), AXObserverGetRunLoopSource(_observer), kCFRunLoopCommonModes);
		CFRelease(_observer);
	}
	if (_element != NULL) {
		CFRelease(_element);
	}
}

@end

static os_unfair_lock gLock = OS_UNFAIR_LOCK_INIT;
static NSMutableDictionary<NSNumber *, BenchWindowEntry *> *gWindows; // By window ID.
static NSMutableDictionary<NSNumber *, BenchAppEntry *> *gApps;       // By process ID.
static uint32_t gNextSynthetic = BENCH_SYNTHETIC_BASE;
static int64_t gCreated = 0;

// Accessibility queue only.
static BOOL gObservingAll = NO;
static BOOL gTrustCheckScheduled = NO;

// gSystemWide is kept for the life of the process: setting its messaging
// timeout sets the global one, for every element that doesn't set its own.
static AXUIElementRef gSystemWide = NULL;

static void benchRegistryInit(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		gWindows = [NSMutableDictionary dictionary];
		gApps = [NSMutableDictionary dictionary];
		// Start synthetic IDs at a random multiple of 65,536 above the base,
		// so that one remembered from an earlier run (in state.json) is
		// unlikely to name a different window now.
		gNextSynthetic = BENCH_SYNTHETIC_BASE + (arc4random_uniform(0x4000) << 16);
		gSystemWide = AXUIElementCreateSystemWide();
		AXUIElementSetMessagingTimeout(gSystemWide, BenchMessagingTimeout);
	});
}

static inline void benchLock(void) {
	os_unfair_lock_lock(&gLock);
}

static inline void benchUnlock(void) {
	os_unfair_lock_unlock(&gLock);
}

// benchAppEntry returns the entry for a process, making it if needed.
static BenchAppEntry *benchAppEntry(pid_t pid) {
	benchRegistryInit();
	benchLock();
	BenchAppEntry *app = gApps[@(pid)];
	benchUnlock();
	if (app != nil) {
		return app;
	}
	// Made outside the lock: it asks NSRunningApplication about the app.
	BenchAppEntry *made = [[BenchAppEntry alloc] initWithPID:pid];
	benchLock();
	app = gApps[@(pid)];
	if (app == nil) {
		gApps[@(pid)] = made;
		app = made;
	}
	benchUnlock();
	return app;
}

// benchLookupID returns the entry for a window ID, or nil.
static BenchWindowEntry *benchLookupID(uint32_t windowID) {
	benchRegistryInit();
	benchLock();
	BenchWindowEntry *entry = gWindows[@(windowID)];
	benchUnlock();
	return entry;
}

// benchFindEntry returns the entry for an element, or nil. CFEqual compares
// elements without asking the app.
static BenchWindowEntry *benchFindEntry(AXUIElementRef element, pid_t pid) {
	benchRegistryInit();
	BenchWindowEntry *found = nil;
	benchLock();
	for (BenchWindowEntry *entry in [gWindows objectEnumerator]) {
		if (entry.pid == pid && CFEqual(entry.element, element)) {
			found = entry;
			break;
		}
	}
	benchUnlock();
	return found;
}

// benchRegister returns the entry for element, registering it under
// windowID, or under the next synthetic ID if windowID is 0, if it's new.
//
// If windowID is registered with a different element of the same app, the
// app has made a new element for the same window: the entry is replaced with
// one holding the new element, keeping the window's creation order and what
// was last read, since the old element no longer answers. If it's registered
// for another app, the old window has gone and its ID is in use again, so
// the window is new.
static BenchWindowEntry *benchRegister(AXUIElementRef element, pid_t pid, CGWindowID windowID) {
	benchRegistryInit();
	BenchWindowEntry *entry = nil;
	BenchWindowEntry *known = nil; // Released after the lock, if replaced.
	benchLock();
	if (windowID != 0) {
		known = gWindows[@(windowID)];
		if (known != nil && known.pid == pid && CFEqual(known.element, element)) {
			entry = known;
		} else if (known != nil && known.pid == pid) {
			entry = [[BenchWindowEntry alloc] initWithID:windowID pid:pid element:element synthetic:NO created:known.created];
			entry.title = known.title;
			entry.subrole = known.subrole;
			entry.minimised = known.minimised;
			entry.fullScreen = known.fullScreen;
			entry.announced = known.announced;
			gWindows[@(windowID)] = entry;
		}
	} else {
		for (BenchWindowEntry *candidate in [gWindows objectEnumerator]) {
			if (candidate.synthetic && candidate.pid == pid && CFEqual(candidate.element, element)) {
				entry = candidate;
				break;
			}
		}
	}
	if (entry == nil) {
		BOOL synthetic = windowID == 0;
		uint32_t newID = synthetic ? gNextSynthetic++ : windowID;
		entry = [[BenchWindowEntry alloc] initWithID:newID pid:pid element:element synthetic:synthetic created:++gCreated];
		gWindows[@(newID)] = entry;
	}
	benchUnlock();
	return entry;
}

// benchEntryForElement returns the entry for a window element, registering
// it if it's new and create is set.
static BenchWindowEntry *benchEntryForElement(AXUIElementRef element, pid_t pid, BOOL create) {
	BenchWindowEntry *entry = benchFindEntry(element, pid);
	if (entry != nil || !create) {
		return entry;
	}
	return benchRegister(element, pid, benchWindowNumber(element));
}

// benchForgetWindow removes a window from the registry.
static void benchForgetWindow(uint32_t windowID) {
	benchRegistryInit();
	BenchWindowEntry *gone = nil;
	benchLock();
	gone = gWindows[@(windowID)];
	[gWindows removeObjectForKey:@(windowID)];
	benchUnlock();
	// gone is released here, outside the lock.
	(void)gone;
}

#pragma mark - Reading attributes

// benchCopyString reads a string attribute, or returns nil.
static NSString *benchCopyString(AXUIElementRef element, CFStringRef attribute, AXError *error) {
	CFTypeRef value = NULL;
	AXError err = AXUIElementCopyAttributeValue(element, attribute, &value);
	if (error != NULL) {
		*error = err;
	}
	if (err != kAXErrorSuccess || value == NULL) {
		if (value != NULL) {
			CFRelease(value);
		}
		return nil;
	}
	if (CFGetTypeID(value) != CFStringGetTypeID()) {
		CFRelease(value);
		return nil;
	}
	return (__bridge_transfer NSString *)value;
}

// benchCopyBool reads a Boolean attribute, or returns NO.
static BOOL benchCopyBool(AXUIElementRef element, CFStringRef attribute, AXError *error) {
	CFTypeRef value = NULL;
	AXError err = AXUIElementCopyAttributeValue(element, attribute, &value);
	if (error != NULL) {
		*error = err;
	}
	BOOL result = NO;
	if (err == kAXErrorSuccess && value != NULL) {
		if (CFGetTypeID(value) == CFBooleanGetTypeID()) {
			result = CFBooleanGetValue((CFBooleanRef)value) ? YES : NO;
		} else if (CFGetTypeID(value) == CFNumberGetTypeID()) {
			int number = 0;
			CFNumberGetValue((CFNumberRef)value, kCFNumberIntType, &number);
			result = number != 0;
		}
	}
	if (value != NULL) {
		CFRelease(value);
	}
	return result;
}

// benchCopyAXValue reads a point or size attribute into *out.
static BOOL benchCopyAXValue(AXUIElementRef element, CFStringRef attribute, AXValueType type, void *out, AXError *error) {
	CFTypeRef value = NULL;
	AXError err = AXUIElementCopyAttributeValue(element, attribute, &value);
	if (error != NULL) {
		*error = err;
	}
	BOOL ok = NO;
	if (err == kAXErrorSuccess && value != NULL && CFGetTypeID(value) == AXValueGetTypeID()) {
		ok = AXValueGetValue((AXValueRef)value, type, out) ? YES : NO;
	}
	if (value != NULL) {
		CFRelease(value);
	}
	return ok;
}

// benchCopyElements reads an attribute holding an array of elements, such as
// kAXWindowsAttribute or kAXChildrenAttribute. Elements of other types are
// left out. A missing value is an empty array.
static NSArray *benchCopyElements(AXUIElementRef element, CFStringRef attribute, AXError *error) {
	CFTypeRef value = NULL;
	AXError err = AXUIElementCopyAttributeValue(element, attribute, &value);
	if (err == kAXErrorNoValue) {
		err = kAXErrorSuccess;
	}
	if (error != NULL) {
		*error = err;
	}
	if (err != kAXErrorSuccess || value == NULL || CFGetTypeID(value) != CFArrayGetTypeID()) {
		if (value != NULL) {
			CFRelease(value);
		}
		return @[];
	}
	NSArray *all = (__bridge_transfer NSArray *)value;
	NSMutableArray *elements = [NSMutableArray arrayWithCapacity:all.count];
	for (id item in all) {
		if (CFGetTypeID((__bridge CFTypeRef)item) == AXUIElementGetTypeID()) {
			[elements addObject:item];
		}
	}
	return elements;
}

// benchCopyElement reads an attribute holding one element, such as
// kAXFocusedWindowAttribute, or returns nil.
static id benchCopyElement(AXUIElementRef element, CFStringRef attribute) {
	CFTypeRef value = NULL;
	if (AXUIElementCopyAttributeValue(element, attribute, &value) != kAXErrorSuccess || value == NULL) {
		if (value != NULL) {
			CFRelease(value);
		}
		return nil;
	}
	if (CFGetTypeID(value) != AXUIElementGetTypeID()) {
		CFRelease(value);
		return nil;
	}
	return (__bridge_transfer id)value;
}

// benchAppGone reports whether an app has quit. The Accessibility API
// answers calls to a quit app with kAXErrorCannotComplete, as it does for
// one that's busy, so this tells the two apart.
static BOOL benchAppGone(pid_t pid) {
	NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
	return app == nil || app.terminated;
}

// benchAppIsActive reports whether an app is frontmost.
static BOOL benchAppIsActive(pid_t pid) {
	NSRunningApplication *app = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
	return app != nil && app.active;
}

// benchIsFocused reports whether a window is the focused window of the
// frontmost app.
static BOOL benchIsFocused(BenchWindowEntry *entry) {
	if (!benchAppIsActive(entry.pid)) {
		return NO;
	}
	id focused = benchCopyElement(benchAppEntry(entry.pid).element, kAXFocusedWindowAttribute);
	return focused != nil && CFEqual((__bridge CFTypeRef)focused, entry.element);
}

#pragma mark - Describing windows

// What benchDescribe reads from the app. The rest comes from the registry.
typedef NS_OPTIONS(NSUInteger, BenchDescribe) {
	BenchDescribeFrame = 1 << 0,
	BenchDescribeTitle = 1 << 1,
	BenchDescribeState = 1 << 2, // Subrole, minimised, and full screen.
	BenchDescribeFocus = 1 << 3,
	BenchDescribeAll = BenchDescribeFrame | BenchDescribeTitle | BenchDescribeState | BenchDescribeFocus,
};

// benchDescribe fills *out, reading what's asked for and taking the rest
// from the registry. focused is 1 or 0 if the caller knows whether the
// window is focused, or -1 to read it (with BenchDescribeFocus). It returns
// kAXErrorInvalidUIElement if the window has gone, or another error if its
// frame couldn't be read. The caller frees *out with benchFreeWindowInfo,
// whatever it returns.
static AXError benchDescribe(BenchWindowEntry *entry, BenchDescribe what, int focused, BenchWindowInfo *out) {
	memset(out, 0, sizeof *out);
	AXUIElementRef element = entry.element;
	AXError result = kAXErrorSuccess;
	AXError err = kAXErrorSuccess;

	if (what & BenchDescribeFrame) {
		CGPoint origin = CGPointZero;
		CGSize size = CGSizeZero;
		if (!benchCopyAXValue(element, kAXPositionAttribute, kAXValueTypeCGPoint, &origin, &err)) {
			result = err != kAXErrorSuccess ? err : kAXErrorFailure;
		}
		if (result == kAXErrorInvalidUIElement) {
			return result;
		}
		if (!benchCopyAXValue(element, kAXSizeAttribute, kAXValueTypeCGSize, &size, &err) && result == kAXErrorSuccess) {
			result = err != kAXErrorSuccess ? err : kAXErrorFailure;
		}
		out->x = origin.x;
		out->y = origin.y;
		out->w = size.width;
		out->h = size.height;
	}
	if (what & BenchDescribeTitle) {
		NSString *title = benchCopyString(element, kAXTitleAttribute, &err);
		if (err == kAXErrorInvalidUIElement) {
			return err;
		}
		if (err == kAXErrorSuccess || err == kAXErrorNoValue) {
			benchLock();
			entry.title = title ?: @"";
			benchUnlock();
		}
	}
	if (what & BenchDescribeState) {
		NSString *subrole = benchCopyString(element, kAXSubroleAttribute, &err);
		if (err == kAXErrorInvalidUIElement) {
			return err;
		}
		BOOL subroleRead = err == kAXErrorSuccess || err == kAXErrorNoValue;
		AXError minimisedErr = kAXErrorSuccess;
		BOOL minimised = benchCopyBool(element, kAXMinimizedAttribute, &minimisedErr);
		AXError fullErr = kAXErrorSuccess;
		BOOL fullScreen = benchCopyBool(element, BenchFullScreenAttribute, &fullErr);
		benchLock();
		if (subroleRead) {
			entry.subrole = subrole ?: @"";
		}
		if (minimisedErr == kAXErrorSuccess) {
			entry.minimised = minimised;
		}
		if (fullErr == kAXErrorSuccess) {
			entry.fullScreen = fullScreen;
		}
		benchUnlock();
	}
	if (focused < 0) {
		focused = (what & BenchDescribeFocus) && benchIsFocused(entry) ? 1 : 0;
	}

	BenchAppEntry *app = benchAppEntry(entry.pid);
	out->windowID = entry.windowID;
	out->pid = entry.pid;
	out->created = entry.created;
	out->focused = focused > 0 ? 1 : 0;
	out->bundleID = benchCopyCString(app.bundleID);
	out->appName = benchCopyCString(app.name);
	benchLock();
	NSString *title = entry.title;
	NSString *subrole = entry.subrole;
	out->minimised = entry.minimised ? 1 : 0;
	out->fullScreen = entry.fullScreen ? 1 : 0;
	benchUnlock();
	out->title = benchCopyCString(title);
	out->subrole = benchCopyCString(subrole);
	return result;
}

void benchFreeWindowInfo(BenchWindowInfo *info) {
	if (info == NULL) {
		return;
	}
	free(info->bundleID);
	free(info->appName);
	free(info->title);
	free(info->subrole);
	info->bundleID = NULL;
	info->appName = NULL;
	info->title = NULL;
	info->subrole = NULL;
}

void benchFreeWindowInfos(BenchWindowInfo *infos, int count) {
	if (infos == NULL) {
		return;
	}
	for (int i = 0; i < count; i++) {
		benchFreeWindowInfo(&infos[i]);
	}
	free(infos);
}

// benchEmitWindow describes a window and reports an event about it to Go.
// It sends nothing for a window that has gone, except WindowDestroyed,
// which is described from the registry alone.
static void benchEmitWindow(int kind, BenchWindowEntry *entry, BenchDescribe what, int focused) {
	@autoreleasepool {
		BenchWindowInfo info;
		AXError err = benchDescribe(entry, what, focused, &info);
		if (err != kAXErrorInvalidUIElement || kind == BENCH_EV_WINDOW_DESTROYED) {
			benchEmitWindowEvent(kind, &info);
		}
		benchFreeWindowInfo(&info);
	}
}

#pragma mark - Listing windows

// benchRegisterWindows returns the entries for an app's window elements, in
// the same order, registering the new ones in the order the window server
// made them (their CGWindowIDs grow over time), so that Window.Created
// follows creation order. New windows without a CGWindowID go last.
static NSArray<BenchWindowEntry *> *benchRegisterWindows(NSArray *elements, pid_t pid) {
	NSMutableArray *entries = [NSMutableArray arrayWithCapacity:elements.count];
	NSMutableArray<NSArray<NSNumber *> *> *fresh = [NSMutableArray array]; // (index, window number)
	for (NSUInteger i = 0; i < elements.count; i++) {
		AXUIElementRef element = (__bridge AXUIElementRef)elements[i];
		BenchWindowEntry *entry = benchFindEntry(element, pid);
		[entries addObject:entry ?: (id)[NSNull null]];
		if (entry == nil) {
			[fresh addObject:@[ @(i), @(benchWindowNumber(element)) ]];
		}
	}
	[fresh sortWithOptions:NSSortStable usingComparator:^NSComparisonResult(NSArray<NSNumber *> *a, NSArray<NSNumber *> *b) {
		uint32_t x = a[1].unsignedIntValue;
		uint32_t y = b[1].unsignedIntValue;
		if (x == y) {
			return NSOrderedSame;
		}
		if (x == 0) {
			return NSOrderedDescending;
		}
		if (y == 0) {
			return NSOrderedAscending;
		}
		return x < y ? NSOrderedAscending : NSOrderedDescending;
	}];
	for (NSArray<NSNumber *> *pair in fresh) {
		NSUInteger i = pair[0].unsignedIntegerValue;
		AXUIElementRef element = (__bridge AXUIElementRef)elements[i];
		entries[i] = benchRegister(element, pid, pair[1].unsignedIntValue);
	}
	return entries;
}

// benchAppWindowElements returns an app's window elements on this desktop,
// front to back.
static NSArray *benchAppWindowElements(pid_t pid, AXError *error) {
	return benchCopyElements(benchAppEntry(pid).element, kAXWindowsAttribute, error);
}

static void benchWatchWindows(pid_t pid);

// benchRegisterAppWindows registers an app's windows on this desktop, and
// has them watched.
static void benchRegisterAppWindows(pid_t pid) {
	AXError err = kAXErrorSuccess;
	NSArray *elements = benchAppWindowElements(pid, &err);
	if (err != kAXErrorSuccess || elements.count == 0) {
		return;
	}
	benchRegisterWindows(elements, pid);
	dispatch_async(benchAXQueue(), ^{
		benchWatchWindows(pid);
	});
}

// BenchPresence says where a registered window is that the Accessibility
// API doesn't list for its app.
typedef NS_ENUM(NSInteger, BenchPresence) {
	// It's on this desktop: in native full screen (a desktop of its own),
	// minimised, or its app is hidden, any of which can leave it out of the
	// list.
	BenchPresenceHere,
	// Its element still answers, so it exists, but somewhere else: on
	// another desktop.
	BenchPresenceAway,
	// Its element has gone, so the window has closed.
	BenchPresenceGone,
};

// benchUnlistedPresence says where a registered window is that its app's
// window list leaves out.
static BenchPresence benchUnlistedPresence(BenchWindowEntry *entry) {
	AXError fullErr = kAXErrorSuccess;
	BOOL fullScreen = benchCopyBool(entry.element, BenchFullScreenAttribute, &fullErr);
	if (fullErr == kAXErrorInvalidUIElement) {
		return BenchPresenceGone;
	}
	AXError minimisedErr = kAXErrorSuccess;
	BOOL minimised = benchCopyBool(entry.element, kAXMinimizedAttribute, &minimisedErr);
	if (minimisedErr == kAXErrorInvalidUIElement) {
		return BenchPresenceGone;
	}
	NSRunningApplication *running = [NSRunningApplication runningApplicationWithProcessIdentifier:entry.pid];
	if ((fullErr == kAXErrorSuccess && fullScreen) || (minimisedErr == kAXErrorSuccess && minimised) || running.hidden) {
		return BenchPresenceHere;
	}
	return BenchPresenceAway;
}

// benchEntriesForPID returns the registered windows of an app.
static NSArray<BenchWindowEntry *> *benchEntriesForPID(pid_t pid) {
	benchRegistryInit();
	NSMutableArray<BenchWindowEntry *> *found = [NSMutableArray array];
	benchLock();
	for (BenchWindowEntry *entry in [gWindows objectEnumerator]) {
		if (entry.pid == pid) {
			[found addObject:entry];
		}
	}
	benchUnlock();
	return found;
}

// benchEntryForID returns the entry for a window ID. A real window ID
// that isn't registered yet (from an earlier run of Bench, say) is found by
// listing the windows of its app: pidHint's, or else the owner the window
// server names.
static BenchWindowEntry *benchEntryForID(uint32_t windowID, pid_t pidHint) {
	if (windowID == 0) {
		return nil;
	}
	BenchWindowEntry *entry = benchLookupID(windowID);
	if (entry != nil || windowID >= BENCH_SYNTHETIC_BASE) {
		return entry;
	}
	if (pidHint > 0 && pidHint != getpid()) {
		benchRegisterAppWindows(pidHint);
		entry = benchLookupID(windowID);
		if (entry != nil) {
			return entry;
		}
	}
	pid_t owner = 0;
	if (!benchWindowServerHas(windowID, &owner) || owner <= 0 || owner == pidHint || owner == getpid()) {
		return nil;
	}
	benchRegisterAppWindows(owner);
	return benchLookupID(windowID);
}

int benchCopyWindows(int32_t pid, BenchWindowInfo **out, int *count) {
	*out = NULL;
	*count = 0;
	if (pid <= 0) {
		return BENCH_ERR_NOT_RUNNING;
	}
	if (pid == getpid()) {
		// Bench's own windows aren't items, and asking its own process
		// would need the main thread to answer.
		return BENCH_OK;
	}
	@autoreleasepool {
		AXError err = kAXErrorSuccess;
		NSArray *elements = benchAppWindowElements(pid, &err);
		if (err != kAXErrorSuccess) {
			return err;
		}
		NSMutableArray<BenchWindowEntry *> *entries = [benchRegisterWindows(elements, pid) mutableCopy];
		// The list can leave out windows in native full screen, minimised
		// ones, and a hidden app's: add those that Bench has seen, as
		// benchGetWindow finds them too.
		for (BenchWindowEntry *entry in benchEntriesForPID(pid)) {
			if ([entries indexOfObjectIdenticalTo:entry] != NSNotFound) {
				continue;
			}
			switch (benchUnlistedPresence(entry)) {
			case BenchPresenceHere:
				[entries addObject:entry];
				break;
			case BenchPresenceGone:
				benchForgetWindow(entry.windowID);
				break;
			case BenchPresenceAway:
				break;
			}
		}
		if (entries.count == 0) {
			return BENCH_OK;
		}
		id focused = nil;
		if (benchAppIsActive(pid)) {
			focused = benchCopyElement(benchAppEntry(pid).element, kAXFocusedWindowAttribute);
		}
		BenchWindowInfo *infos = calloc(entries.count, sizeof(BenchWindowInfo));
		if (infos == NULL) {
			return BENCH_ERR_FAILED;
		}
		int n = 0;
		for (BenchWindowEntry *entry in entries) {
			int isFocused = focused != nil && CFEqual((__bridge CFTypeRef)focused, entry.element) ? 1 : 0;
			AXError describeErr = benchDescribe(entry, BenchDescribeFrame | BenchDescribeTitle | BenchDescribeState, isFocused, &infos[n]);
			if (describeErr == kAXErrorInvalidUIElement) {
				// It closed while being listed.
				benchFreeWindowInfo(&infos[n]);
				benchForgetWindow(entry.windowID);
				continue;
			}
			n++;
		}
		*out = infos;
		*count = n;
		dispatch_async(benchAXQueue(), ^{
			benchWatchWindows(pid);
		});
	}
	return BENCH_OK;
}

int benchGetWindow(uint32_t windowID, int32_t pidHint, BenchWindowInfo *out) {
	memset(out, 0, sizeof *out);
	@autoreleasepool {
		BenchWindowEntry *entry = benchEntryForID(windowID, pidHint);
		if (entry == nil) {
			return windowID == 0 ? BENCH_ERR_NO_WINDOW : benchMissingStatus(windowID);
		}
		if (benchAppGone(entry.pid)) {
			benchForgetWindow(entry.windowID);
			return BENCH_ERR_NO_WINDOW;
		}

		// The Accessibility API lists only windows on this desktop. A window
		// it doesn't list is usually on another desktop, or gone.
		AXError listErr = kAXErrorSuccess;
		NSArray *elements = benchAppWindowElements(entry.pid, &listErr);
		if (listErr == kAXErrorAPIDisabled) {
			return listErr;
		}
		BOOL listed = listErr != kAXErrorSuccess; // If the list can't be read, don't guess.
		for (id element in elements) {
			if (CFEqual((__bridge CFTypeRef)element, entry.element)) {
				listed = YES;
				break;
			}
		}
		if (!listed && !entry.synthetic) {
			// The app may have made a new element for the same window, which
			// only its window number matches.
			for (id element in elements) {
				AXUIElementRef candidate = (__bridge AXUIElementRef)element;
				if (benchWindowNumber(candidate) == entry.windowID) {
					entry = benchRegister(candidate, entry.pid, entry.windowID);
					listed = YES;
					pid_t pid = entry.pid;
					dispatch_async(benchAXQueue(), ^{
						benchWatchWindows(pid);
					});
					break;
				}
			}
		}
		if (!listed) {
			switch (benchUnlistedPresence(entry)) {
			case BenchPresenceHere:
				break;
			case BenchPresenceGone:
				benchForgetWindow(entry.windowID);
				return benchMissingStatus(entry.windowID);
			case BenchPresenceAway: {
				if (entry.synthetic) {
					// The window server can't be asked about a synthetic ID, but the
					// element still answers, so the window exists.
					return BENCH_ERR_OTHER_DESKTOP;
				}
				int status = benchMissingStatus(entry.windowID);
				if (status == BENCH_ERR_NO_WINDOW) {
					benchForgetWindow(entry.windowID);
				}
				return status;
			}
			}
		}

		AXError err = benchDescribe(entry, BenchDescribeAll, -1, out);
		if (err == kAXErrorInvalidUIElement) {
			benchFreeWindowInfo(out);
			benchForgetWindow(entry.windowID);
			return benchMissingStatus(entry.windowID);
		}
		if (err != kAXErrorSuccess) {
			benchFreeWindowInfo(out);
			return err;
		}
	}
	return BENCH_OK;
}

#pragma mark - Acting on windows

// benchEntryForOp finds the entry for an action on a window, or sets
// *status to why there isn't one.
static BenchWindowEntry *benchEntryForOp(uint32_t windowID, int32_t pidHint, int *status) {
	BenchWindowEntry *entry = benchEntryForID(windowID, pidHint);
	if (entry == nil) {
		*status = windowID == 0 ? BENCH_ERR_NO_WINDOW : benchMissingStatus(windowID);
		return nil;
	}
	if (benchAppGone(entry.pid)) {
		benchForgetWindow(entry.windowID);
		*status = BENCH_ERR_NO_WINDOW;
		return nil;
	}
	return entry;
}

// benchOpStatus turns an action's AXError into a status, forgetting a
// window that has gone.
static int benchOpStatus(AXError err, BenchWindowEntry *entry) {
	if (err == kAXErrorSuccess) {
		return BENCH_OK;
	}
	if (err == kAXErrorInvalidUIElement) {
		benchForgetWindow(entry.windowID);
		return benchMissingStatus(entry.windowID);
	}
	return err;
}

int benchSetFrame(uint32_t windowID, int32_t pidHint, double x, double y, double w, double h) {
	@autoreleasepool {
		int status = BENCH_OK;
		BenchWindowEntry *entry = benchEntryForOp(windowID, pidHint, &status);
		if (entry == nil) {
			return status;
		}
		AXUIElementRef element = entry.element;
		AXError err = kAXErrorSuccess;
		if (benchCopyBool(element, BenchFullScreenAttribute, &err)) {
			return BENCH_OK; // Leave windows in native full screen alone.
		}
		if (err == kAXErrorInvalidUIElement) {
			return benchOpStatus(err, entry);
		}

		BenchAppEntry *app = benchAppEntry(entry.pid);
		BOOL enhanced = benchCopyBool(app.element, BenchEnhancedUserInterfaceAttribute, NULL);
		if (enhanced) {
			AXUIElementSetAttributeValue(app.element, BenchEnhancedUserInterfaceAttribute, kCFBooleanFalse);
		}

		CGSize size = CGSizeMake(w, h);
		CGPoint origin = CGPointMake(x, y);
		AXValueRef sizeValue = AXValueCreate(kAXValueTypeCGSize, &size);
		AXValueRef originValue = AXValueCreate(kAXValueTypeCGPoint, &origin);
		AXError sizeErr = kAXErrorFailure;
		AXError originErr = kAXErrorFailure;
		if (sizeValue != NULL && originValue != NULL) {
			// Size, then position, then size again: some apps clamp the size
			// to fit the screen at the old position.
			sizeErr = AXUIElementSetAttributeValue(element, kAXSizeAttribute, sizeValue);
			originErr = AXUIElementSetAttributeValue(element, kAXPositionAttribute, originValue);
			if (sizeErr != kAXErrorInvalidUIElement && originErr != kAXErrorInvalidUIElement) {
				sizeErr = AXUIElementSetAttributeValue(element, kAXSizeAttribute, sizeValue);
			}
		}
		if (sizeValue != NULL) {
			CFRelease(sizeValue);
		}
		if (originValue != NULL) {
			CFRelease(originValue);
		}

		if (enhanced) {
			AXUIElementSetAttributeValue(app.element, BenchEnhancedUserInterfaceAttribute, kCFBooleanTrue);
		}
		if (originErr == kAXErrorInvalidUIElement || sizeErr == kAXErrorInvalidUIElement) {
			return benchOpStatus(kAXErrorInvalidUIElement, entry);
		}
		// A window that can't be resized still moves; the caller reads its
		// frame afterwards to see what it accepted.
		return benchOpStatus(originErr, entry);
	}
}

int benchRaise(uint32_t windowID, int32_t pidHint) {
	@autoreleasepool {
		int status = BENCH_OK;
		BenchWindowEntry *entry = benchEntryForOp(windowID, pidHint, &status);
		if (entry == nil) {
			return status;
		}
		return benchOpStatus(AXUIElementPerformAction(entry.element, kAXRaiseAction), entry);
	}
}

int benchSetMinimised(uint32_t windowID, int32_t pidHint, int minimised) {
	@autoreleasepool {
		int status = BENCH_OK;
		BenchWindowEntry *entry = benchEntryForOp(windowID, pidHint, &status);
		if (entry == nil) {
			return status;
		}
		CFBooleanRef value = minimised ? kCFBooleanTrue : kCFBooleanFalse;
		return benchOpStatus(AXUIElementSetAttributeValue(entry.element, kAXMinimizedAttribute, value), entry);
	}
}

int benchFocus(uint32_t windowID, int32_t pidHint) {
	@autoreleasepool {
		int status = BENCH_OK;
		BenchWindowEntry *entry = benchEntryForOp(windowID, pidHint, &status);
		if (entry == nil) {
			return status;
		}
		NSRunningApplication *running = [NSRunningApplication runningApplicationWithProcessIdentifier:entry.pid];
		if (running == nil || running.terminated) {
			return BENCH_ERR_NOT_RUNNING;
		}
		if (running.hidden) {
			[running unhide];
		}
		AXUIElementRef element = entry.element;
		AXError err = kAXErrorSuccess;
		if (benchCopyBool(element, kAXMinimizedAttribute, &err)) {
			AXUIElementSetAttributeValue(element, kAXMinimizedAttribute, kCFBooleanFalse);
		}
		if (err == kAXErrorInvalidUIElement) {
			return benchOpStatus(err, entry);
		}
		AXUIElementSetAttributeValue(element, kAXMainAttribute, kCFBooleanTrue);
		err = AXUIElementPerformAction(element, kAXRaiseAction);
		if (err == kAXErrorInvalidUIElement) {
			return benchOpStatus(err, entry);
		}
		// Setting AXFrontmost activates the app even when Bench isn't the
		// active app, which -[NSRunningApplication activateWithOptions:]
		// doesn't promise since macOS 14's cooperative activation.
		AXError frontErr = AXUIElementSetAttributeValue(benchAppEntry(entry.pid).element, kAXFrontmostAttribute, kCFBooleanTrue);
		if (frontErr != kAXErrorSuccess) {
			[running activateWithOptions:0];
		}
		return BENCH_OK;
	}
}

#pragma mark - New windows

// benchSubmenuItems returns the items of a menu bar item's or a menu item's
// menu, or an empty array if it has none.
static NSArray *benchSubmenuItems(AXUIElementRef element) {
	for (id child in benchCopyElements(element, kAXChildrenAttribute, NULL)) {
		NSString *role = benchCopyString((__bridge AXUIElementRef)child, kAXRoleAttribute, NULL);
		if ([role isEqualToString:(__bridge NSString *)kAXMenuRole]) {
			return benchCopyElements((__bridge AXUIElementRef)child, kAXChildrenAttribute, NULL);
		}
	}
	return @[];
}

// benchIsEnabled reports whether a menu item is enabled, assuming it is if
// the app doesn't say.
static BOOL benchIsEnabled(AXUIElementRef element) {
	AXError err = kAXErrorSuccess;
	BOOL enabled = benchCopyBool(element, kAXEnabledAttribute, &err);
	return err != kAXErrorSuccess || enabled;
}

// benchFindNewWindowItem finds the enabled menu item titled "New Window" in
// an app's menu bar, looking in the File menu first, where most apps keep
// it, and then the others (Terminal keeps it in Shell). If the item opens a
// submenu, as Terminal's does, it returns the submenu's first item.
static id benchFindNewWindowItem(AXUIElementRef menuBar) {
	NSArray *barItems = benchCopyElements(menuBar, kAXChildrenAttribute, NULL);
	NSMutableArray *ordered = [NSMutableArray arrayWithCapacity:barItems.count];
	for (NSUInteger i = 1; i < barItems.count; i++) { // Item 0 is the Apple menu.
		id barItem = barItems[i];
		NSString *title = benchCopyString((__bridge AXUIElementRef)barItem, kAXTitleAttribute, NULL);
		if ([title isEqualToString:@"File"]) {
			[ordered insertObject:barItem atIndex:0];
		} else {
			[ordered addObject:barItem];
		}
	}
	for (id barItem in ordered) {
		for (id menuItem in benchSubmenuItems((__bridge AXUIElementRef)barItem)) {
			AXUIElementRef element = (__bridge AXUIElementRef)menuItem;
			NSString *title = benchCopyString(element, kAXTitleAttribute, NULL);
			if (![title isEqualToString:@"New Window"] || !benchIsEnabled(element)) {
				continue;
			}
			NSArray *submenu = benchSubmenuItems(element);
			if (submenu.count == 0) {
				return menuItem;
			}
			for (id subItem in submenu) {
				AXUIElementRef sub = (__bridge AXUIElementRef)subItem;
				NSString *subTitle = benchCopyString(sub, kAXTitleAttribute, NULL);
				if (subTitle.length > 0 && benchIsEnabled(sub)) {
					return subItem;
				}
			}
		}
	}
	return nil;
}

int benchPressNewWindow(int32_t pid) {
	if (pid <= 0) {
		return BENCH_ERR_NOT_RUNNING;
	}
	@autoreleasepool {
		id menuBar = benchCopyElement(benchAppEntry(pid).element, kAXMenuBarAttribute);
		if (menuBar == nil) {
			return BENCH_ERR_NO_MENU_ITEM;
		}
		id item = benchFindNewWindowItem((__bridge AXUIElementRef)menuBar);
		if (item == nil) {
			return BENCH_ERR_NO_MENU_ITEM;
		}
		AXError err = AXUIElementPerformAction((__bridge AXUIElementRef)item, kAXPressAction);
		return err == kAXErrorSuccess ? BENCH_OK : err;
	}
}

#pragma mark - Trust

int benchTrusted(int prompt) {
	BOOL trusted = NO;
	@autoreleasepool {
		NSDictionary *options = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt : prompt ? @YES : @NO};
		trusted = AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options) ? YES : NO;
	}
	if (trusted) {
		dispatch_async(benchAXQueue(), ^{
			benchObserveAllApps();
		});
	}
	return trusted ? 1 : 0;
}

#pragma mark - Observing

// benchAXCallback receives Accessibility notifications on the main thread,
// where the observers' run loop sources are. It does no Accessibility work
// itself: it hands each notification to the Accessibility queue, so the
// main thread never waits for another app.
static void benchAXCallback(AXObserverRef observer, AXUIElementRef element, CFStringRef notification, void *refcon) {
	(void)observer;
	pid_t pid = 0;
	AXUIElementGetPid(element, &pid);
	uint32_t refID = (uint32_t)(uintptr_t)refcon;
	NSString *name = [(__bridge NSString *)notification copy];
	CFRetain(element);
	dispatch_async(benchAXQueue(), ^{
		@autoreleasepool {
			benchHandleAXNotification(element, name, pid, refID);
		}
		CFRelease(element);
	});
}

// benchWatchWindow registers a window's own notifications (moved, resized,
// destroyed) with its app's observer, once the app is being watched. The
// window ID goes in the refcon, since a destroyed element can't be asked
// for it. Accessibility queue only.
static void benchWatchWindow(BenchWindowEntry *entry) {
	if (entry.watched) {
		return;
	}
	BenchAppEntry *app = benchAppEntry(entry.pid);
	if (!app.watching || app.observer == NULL) {
		return; // benchObserveApp watches it once the app is ready.
	}
	void *refcon = (void *)(uintptr_t)entry.windowID;
	AXError err = AXObserverAddNotification(app.observer, entry.element, kAXUIElementDestroyedNotification, refcon);
	if (err != kAXErrorSuccess && err != kAXErrorNotificationAlreadyRegistered) {
		return;
	}
	AXObserverAddNotification(app.observer, entry.element, kAXWindowMovedNotification, refcon);
	AXObserverAddNotification(app.observer, entry.element, kAXWindowResizedNotification, refcon);
	entry.watched = YES;
}

// benchWatchWindows watches an app's registered windows that aren't watched
// yet. Accessibility queue only.
static void benchWatchWindows(pid_t pid) {
	benchRegistryInit();
	benchLock();
	NSArray<BenchWindowEntry *> *all = [gWindows allValues];
	benchUnlock();
	for (BenchWindowEntry *entry in all) {
		if (entry.pid == pid && !entry.watched) {
			benchWatchWindow(entry);
		}
	}
}

// benchAnnounce sends WindowCreated for a window, once.
static void benchAnnounce(BenchWindowEntry *entry) {
	benchLock();
	BOOL already = entry.announced;
	entry.announced = YES;
	benchUnlock();
	if (!already) {
		benchEmitWindow(BENCH_EV_WINDOW_CREATED, entry, BenchDescribeAll, -1);
	}
}

// benchSyncAppWindows registers and watches an app's windows, announcing
// them as created if announce is set. Accessibility queue only.
static void benchSyncAppWindows(pid_t pid, BOOL announce) {
	AXError err = kAXErrorSuccess;
	NSArray *elements = benchAppWindowElements(pid, &err);
	if (err != kAXErrorSuccess) {
		return;
	}
	for (BenchWindowEntry *entry in benchRegisterWindows(elements, pid)) {
		benchWatchWindow(entry);
		if (announce) {
			benchAnnounce(entry);
		}
	}
	benchWatchWindows(pid);
}

// benchStopObserving removes an app's observer from the main run loop and
// releases it. Accessibility queue only.
static void benchStopObserving(BenchAppEntry *app) {
	if (app == nil || app.observer == NULL) {
		return;
	}
	CFRunLoopRemoveSource(CFRunLoopGetMain(), AXObserverGetRunLoopSource(app.observer), kCFRunLoopCommonModes);
	CFRelease(app.observer);
	app.observer = NULL;
	app.watching = NO;
}

void benchObserveApp(pid_t pid, BOOL announce, int attempt) {
	if (benchStopped() || pid <= 0 || pid == getpid()) {
		return;
	}
	NSRunningApplication *running = [NSRunningApplication runningApplicationWithProcessIdentifier:pid];
	if (running == nil || running.terminated) {
		return;
	}
	BenchAppEntry *app = benchAppEntry(pid);
	if (announce) {
		app.announcePending = YES;
	}
	if (app.watching) {
		if (app.announcePending) {
			// Watching started without announcing (an activation got there
			// first), but the app has just launched: announce its windows.
			app.announcePending = NO;
			benchSyncAppWindows(pid, YES);
		}
		return;
	}
	AXError err = kAXErrorSuccess;
	if (app.observer == NULL) {
		AXObserverRef observer = NULL;
		err = AXObserverCreate(pid, benchAXCallback, &observer);
		if (err == kAXErrorSuccess && observer != NULL) {
			app.observer = observer;
			// The common modes, so that notifications still arrive while the
			// user drags a window or a menu is open (the event tracking mode).
			CFRunLoopAddSource(CFRunLoopGetMain(), AXObserverGetRunLoopSource(observer), kCFRunLoopCommonModes);
		}
	}
	if (app.observer != NULL) {
		err = AXObserverAddNotification(app.observer, app.element, kAXWindowCreatedNotification, NULL);
		if (err == kAXErrorNotificationAlreadyRegistered) {
			err = kAXErrorSuccess;
		}
	}
	if (err != kAXErrorSuccess) {
		// An app that has just launched isn't ready for a moment, so try
		// again after 0.25, 0.5, 1, 2, 4, and 8 seconds. Without permission,
		// benchObserveAllApps starts again once it's granted.
		if (err != kAXErrorAPIDisabled && attempt + 1 < BenchObserveAttempts) {
			double delay = 0.25 * (double)(1 << attempt);
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(delay * (double)NSEC_PER_SEC)), benchAXQueue(), ^{
				// announcePending carries announce over to the next attempt.
				benchObserveApp(pid, NO, attempt + 1);
			});
		}
		return;
	}
	CFStringRef appNotifications[] = {
		kAXFocusedWindowChangedNotification,
		kAXTitleChangedNotification,
		kAXWindowMiniaturizedNotification,
		kAXWindowDeminiaturizedNotification,
	};
	for (size_t i = 0; i < sizeof appNotifications / sizeof appNotifications[0]; i++) {
		AXObserverAddNotification(app.observer, app.element, appNotifications[i], NULL);
	}
	app.watching = YES;
	BOOL announceNow = app.announcePending;
	app.announcePending = NO;
	benchSyncAppWindows(pid, announceNow);
}

void benchObserveAllApps(void) {
	if (benchStopped() || gObservingAll) {
		return;
	}
	if (!AXIsProcessTrusted()) {
		if (!gTrustCheckScheduled) {
			gTrustCheckScheduled = YES;
			dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(2 * NSEC_PER_SEC)), benchAXQueue(), ^{
				gTrustCheckScheduled = NO;
				benchObserveAllApps();
			});
		}
		return;
	}
	gObservingAll = YES;
	for (NSRunningApplication *app in [[NSWorkspace sharedWorkspace] runningApplications]) {
		if (!app.terminated && benchIsRegular(app)) {
			benchObserveApp(app.processIdentifier, NO, 0);
		}
	}
}

void benchForgetApp(pid_t pid) {
	benchRegistryInit();
	NSMutableArray *gone = [NSMutableArray array];
	benchLock();
	BenchAppEntry *app = gApps[@(pid)];
	[gApps removeObjectForKey:@(pid)];
	for (NSNumber *key in [gWindows allKeys]) {
		BenchWindowEntry *entry = gWindows[key];
		if (entry.pid == pid) {
			[gone addObject:entry];
			[gWindows removeObjectForKey:key];
		}
	}
	benchUnlock();
	benchStopObserving(app);
}

void benchForgetEverything(void) {
	benchRegistryInit();
	benchLock();
	NSArray<BenchAppEntry *> *apps = [gApps allValues];
	NSArray<BenchWindowEntry *> *windows = [gWindows allValues];
	[gApps removeAllObjects];
	[gWindows removeAllObjects];
	benchUnlock();
	for (BenchAppEntry *app in apps) {
		benchStopObserving(app);
	}
	(void)windows; // Released here, outside the lock.
	gObservingAll = NO;
}

#pragma mark - Handling notifications

// benchWindowChanged reports a move or resize.
static void benchWindowChanged(int kind, AXUIElementRef element, pid_t pid, uint32_t refID, BenchDescribe what) {
	BenchWindowEntry *entry = refID != 0 ? benchLookupID(refID) : nil;
	if (entry == nil) {
		entry = benchFindEntry(element, pid);
	}
	if (entry != nil) {
		benchEmitWindow(kind, entry, what, -1);
	}
}

// benchProcessGone reports whether a process has exited. kill with signal 0
// sends nothing; it only checks. Unlike NSRunningApplication's terminated,
// which changes only when the main run loop runs, it's up to date at once.
static BOOL benchProcessGone(pid_t pid) {
	return pid > 0 && kill(pid, 0) != 0 && errno == ESRCH;
}

// benchReportClosedLater runs report, which sends WindowDestroyed, after the
// grace period, unless the window's app has quit by then: an app that quits
// is reported by AppTerminated alone (see BenchDestroyedGrace).
static void benchReportClosedLater(pid_t pid, dispatch_block_t report) {
	dispatch_time_t when = dispatch_time(DISPATCH_TIME_NOW, (int64_t)(BenchDestroyedGrace * (double)NSEC_PER_SEC));
	dispatch_after(when, benchAXQueue(), ^{
		if (benchStopped() || benchProcessGone(pid) || benchAppGone(pid)) {
			return;
		}
		@autoreleasepool {
			report();
		}
	});
}

// benchElementAnswers reports whether an element still exists.
static BOOL benchElementAnswers(AXUIElementRef element) {
	AXError err = kAXErrorSuccess;
	(void)benchCopyString(element, kAXRoleAttribute, &err);
	return err != kAXErrorInvalidUIElement;
}

// benchWindowDestroyed forgets a window and, after the grace period, reports
// it closed.
static void benchWindowDestroyed(AXUIElementRef element, pid_t pid, uint32_t refID) {
	BenchWindowEntry *entry = refID != 0 ? benchLookupID(refID) : nil;
	if (entry != nil && !CFEqual(entry.element, element) && benchElementAnswers(entry.element)) {
		// An old element of a window that has a new one (see benchRegister)
		// went away; the window is still open.
		return;
	}
	if (entry == nil) {
		entry = benchFindEntry(element, pid);
	}
	if (entry != nil) {
		benchForgetWindow(entry.windowID);
		benchReportClosedLater(entry.pid, ^{
			benchEmitWindow(BENCH_EV_WINDOW_DESTROYED, entry, 0, 0);
		});
		return;
	}
	if (refID != 0) {
		// Already forgotten (an action found it gone): report what's known.
		benchReportClosedLater(pid, ^{
			BenchWindowInfo info;
			memset(&info, 0, sizeof info);
			info.windowID = refID;
			info.pid = pid;
			benchEmitWindowEvent(BENCH_EV_WINDOW_DESTROYED, &info);
		});
	}
}

// benchWindowCreated registers, watches, and announces a new window.
static void benchWindowCreated(AXUIElementRef element, pid_t pid) {
	BenchWindowEntry *entry = benchFindEntry(element, pid);
	if (entry == nil) {
		// Some apps send this for things that aren't windows, such as sheets.
		NSString *role = benchCopyString(element, kAXRoleAttribute, NULL);
		if (![role isEqualToString:(__bridge NSString *)kAXWindowRole]) {
			return;
		}
		entry = benchEntryForElement(element, pid, YES);
	}
	benchWatchWindow(entry);
	benchAnnounce(entry);
}

// benchFocusChanged reports the frontmost app's newly focused window.
static void benchFocusChanged(AXUIElementRef element, pid_t pid) {
	if (!benchAppIsActive(pid)) {
		return; // Only the frontmost app's focus counts.
	}
	BenchWindowEntry *entry = benchFindEntry(element, pid);
	if (entry == nil) {
		NSString *role = benchCopyString(element, kAXRoleAttribute, NULL);
		if (![role isEqualToString:(__bridge NSString *)kAXWindowRole]) {
			benchEmitFocusedWindow(pid);
			return;
		}
		entry = benchEntryForElement(element, pid, YES);
	}
	benchWatchWindow(entry);
	benchEmitWindow(BENCH_EV_WINDOW_FOCUSED, entry, BenchDescribeAll, 1);
}

void benchEmitFocusedWindow(pid_t pid) {
	id focused = benchCopyElement(benchAppEntry(pid).element, kAXFocusedWindowAttribute);
	if (focused == nil) {
		return;
	}
	BenchWindowEntry *entry = benchEntryForElement((__bridge AXUIElementRef)focused, pid, YES);
	benchWatchWindow(entry);
	benchEmitWindow(BENCH_EV_WINDOW_FOCUSED, entry, BenchDescribeAll, 1);
}

// benchTitleChanged reports a known window's new title. Titles of other
// elements change too, and are ignored.
static void benchTitleChanged(AXUIElementRef element, pid_t pid) {
	BenchWindowEntry *entry = benchFindEntry(element, pid);
	if (entry == nil) {
		return;
	}
	AXError err = kAXErrorSuccess;
	NSString *title = benchCopyString(element, kAXTitleAttribute, &err) ?: @"";
	if (err != kAXErrorSuccess && err != kAXErrorNoValue) {
		return;
	}
	benchLock();
	BOOL changed = ![entry.title isEqualToString:title];
	entry.title = title;
	benchUnlock();
	if (changed) {
		benchEmitWindow(BENCH_EV_WINDOW_RETITLED, entry, BenchDescribeFrame, -1);
	}
}

// benchMinimiseChanged reports a window minimised or restored.
static void benchMinimiseChanged(int kind, AXUIElementRef element, pid_t pid) {
	BenchWindowEntry *entry = benchEntryForElement(element, pid, YES);
	benchWatchWindow(entry);
	benchEmitWindow(kind, entry, BenchDescribeAll, -1);
}

void benchHandleAXNotification(AXUIElementRef element, NSString *notification, pid_t pid, uint32_t refID) {
	if (benchStopped()) {
		return;
	}
	CFStringRef name = (__bridge CFStringRef)notification;
	if (CFEqual(name, kAXUIElementDestroyedNotification)) {
		benchWindowDestroyed(element, pid, refID);
	} else if (CFEqual(name, kAXWindowMovedNotification)) {
		benchWindowChanged(BENCH_EV_WINDOW_MOVED, element, pid, refID, BenchDescribeFrame);
	} else if (CFEqual(name, kAXWindowResizedNotification)) {
		// Entering or leaving full screen resizes, so read the state too.
		benchWindowChanged(BENCH_EV_WINDOW_RESIZED, element, pid, refID, BenchDescribeFrame | BenchDescribeState);
	} else if (CFEqual(name, kAXWindowCreatedNotification)) {
		benchWindowCreated(element, pid);
	} else if (CFEqual(name, kAXFocusedWindowChangedNotification)) {
		benchFocusChanged(element, pid);
	} else if (CFEqual(name, kAXTitleChangedNotification)) {
		benchTitleChanged(element, pid);
	} else if (CFEqual(name, kAXWindowMiniaturizedNotification)) {
		benchMinimiseChanged(BENCH_EV_WINDOW_MINIMISED, element, pid);
	} else if (CFEqual(name, kAXWindowDeminiaturizedNotification)) {
		benchMinimiseChanged(BENCH_EV_WINDOW_RESTORED, element, pid);
	}
}
