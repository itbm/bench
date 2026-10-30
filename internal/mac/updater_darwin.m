//go:build darwin && cgo && !server

// updater_darwin.m: Sparkle, which installs new versions of Bench.
//
// make app puts Sparkle.framework in Bench.app only when it's given an
// update key, so Bench loads it at run time, with NSBundle, rather than
// linking it: a Bench.app without it, and a bare binary, run as before.
// Without Sparkle's headers, the few parts of its API that Bench uses are
// declared here as protocols, and its classes are found by name.
//
// Sparkle's standard user driver shows its own windows: the new version
// with its release notes, the download, and Install and Relaunch. Bench's
// delegate takes over only the news of a version that a daily look finds
// (Sparkle's gentle reminders), which goes to Go for the status bar's
// notice, so that Sparkle never interrupts with a window of its own. The
// notice's Install… shows Sparkle's window for that version.
//
// Sparkle runs on the main thread. Go calls these functions from any
// thread; each sends its work to the main thread and returns at once.

#import "internal_darwin.h"

NS_ASSUME_NONNULL_BEGIN

// SPUUpdater: the updater.
@protocol BenchSparkleUpdater <NSObject>
@property (nonatomic) BOOL automaticallyChecksForUpdates;
// Whether checkForUpdates: does anything: not while Sparkle looks without
// being asked, or for a few seconds after it starts.
@property (nonatomic, readonly) BOOL canCheckForUpdates;
- (BOOL)startUpdater:(NSError *__autoreleasing *)error;
@end

// SPUStandardUpdaterController: the updater, with Sparkle's standard user
// driver, which shows its windows.
@protocol BenchSparkleController <NSObject>
- (instancetype)initWithStartingUpdater:(BOOL)startUpdater updaterDelegate:(nullable id)updaterDelegate userDriverDelegate:(nullable id)userDriverDelegate;
@property (nonatomic, readonly) id<BenchSparkleUpdater> updater;
- (void)checkForUpdates:(nullable id)sender;
@end

// SUAppcastItem: a version in the appcast.
@protocol BenchSparkleItem <NSObject>
@property (nonatomic, readonly, copy) NSString *displayVersionString;
@end

// Sparkle's errors, and the codes of those that aren't failures, as Sparkle
// doesn't log them either: a look that found no newer version
// (SUNoUpdateError), and an installation that the user cancelled, or put
// off, when asked for an administrator's password
// (SUInstallationCanceledError and SUInstallationAuthorizeLaterError).
static NSString *const BenchSparkleErrorDomain = @"SUSparkleErrorDomain";
static const NSInteger BenchSparkleNoUpdate = 1001;
static const NSInteger BenchSparkleInstallationCanceled = 4007;
static const NSInteger BenchSparkleInstallationAuthorizeLater = 4008;

// How long a press of Check for Updates… or Install… waits for Sparkle to
// be free, and how often it looks whether it is.
static const NSTimeInterval BenchSparkleWait = 30;
static const NSTimeInterval BenchSparkleWaitStep = 0.25;

// BenchUpdaterDelegate is Sparkle's updater delegate and its standard user
// driver's delegate.
@interface BenchUpdaterDelegate : NSObject
@end

@implementation BenchUpdaterDelegate

// check_for_updates decides whether Sparkle looks each day, so Sparkle
// never asks. Info.plist's SUEnableAutomaticChecks says so too.
- (BOOL)updaterShouldPromptForPermissionToCheckForUpdates:(id)updater {
	(void)updater;
	return NO;
}

// A look that failed is only logged, as Go's own look is. One that found
// nothing newer, or an installation that the user cancelled, isn't a
// failure.
- (void)updater:(id)updater didFinishUpdateCycleForUpdateCheck:(NSInteger)updateCheck error:(nullable NSError *)error {
	(void)updater;
	(void)updateCheck;
	if (error == nil) {
		return;
	}
	if ([error.domain isEqualToString:BenchSparkleErrorDomain] &&
	    (error.code == BenchSparkleNoUpdate || error.code == BenchSparkleInstallationCanceled || error.code == BenchSparkleInstallationAuthorizeLater)) {
		return;
	}
	@autoreleasepool {
		NSString *text = [NSString stringWithFormat:@"%@ (%@ %ld)", error.localizedDescription, error.domain, (long)error.code];
		NSError *cause = error.userInfo[NSUnderlyingErrorKey];
		if ([cause isKindOfClass:[NSError class]]) {
			text = [text stringByAppendingFormat:@": %@", cause.localizedDescription];
		}
		benchUpdateFailed((char *)benchUTF8(text));
	}
}

- (BOOL)supportsGentleScheduledUpdateReminders {
	return YES;
}

// Bench, not Sparkle, shows the news of a version that a daily look found.
// Sparkle still shows what a look that the user asked for finds.
- (BOOL)standardUserDriverShouldHandleShowingScheduledUpdate:(id)update andInImmediateFocus:(BOOL)immediateFocus {
	(void)update;
	(void)immediateFocus;
	return NO;
}

- (void)standardUserDriverWillHandleShowingUpdate:(BOOL)handleShowingUpdate forUpdate:(id<BenchSparkleItem>)update state:(id)state {
	(void)state;
	if (handleShowingUpdate) {
		return;
	}
	@autoreleasepool {
		benchUpdateFound((char *)benchUTF8(update.displayVersionString));
	}
}

- (void)standardUserDriverWillFinishUpdateSession {
	benchUpdateFinished();
}

@end

// The controller and its delegate, which Sparkle holds weakly, and whether
// a press of Check for Updates… or Install… waits for Sparkle to be free.
// Main thread only.
static id<BenchSparkleController> benchSparkle = nil;
static BenchUpdaterDelegate *benchSparkleDelegate = nil;
static BOOL benchSparkleCheckWaits = NO;

// benchSparkleBundle returns Bench.app's Sparkle.framework, or nil if it
// has none, or no update key to check updates with.
static NSBundle *_Nullable benchSparkleBundle(void) {
	NSBundle *app = [NSBundle mainBundle];
	id key = [app objectForInfoDictionaryKey:@"SUPublicEDKey"];
	if (!benchHasBundleID() || ![key isKindOfClass:[NSString class]] || [key length] == 0) {
		return nil;
	}
	NSString *path = [app.privateFrameworksPath stringByAppendingPathComponent:@"Sparkle.framework"];
	if (path == nil || ![[NSFileManager defaultManager] fileExistsAtPath:path]) {
		return nil;
	}
	return [NSBundle bundleWithPath:path];
}

int benchUpdaterAvailable(void) {
	@autoreleasepool {
		return benchSparkleBundle() != nil ? 1 : 0;
	}
}

void benchUpdaterStart(int automatic) {
	benchOnMain(^{
		if (benchSparkle != nil) {
			return;
		}
		NSBundle *bundle = benchSparkleBundle();
		NSError *error = nil;
		if (bundle == nil || ![bundle loadAndReturnError:&error]) {
			benchUpdateFailed((char *)benchUTF8([NSString stringWithFormat:@"couldn't load Sparkle: %@", error.localizedDescription ?: @"Bench.app has no Sparkle.framework"]));
			return;
		}
		Class controllerClass = NSClassFromString(@"SPUStandardUpdaterController");
		if (controllerClass == Nil) {
			benchUpdateFailed("couldn't find Sparkle's SPUStandardUpdaterController");
			return;
		}
		benchSparkleDelegate = [[BenchUpdaterDelegate alloc] init];
		benchSparkle = [(id<BenchSparkleController>)[controllerClass alloc] initWithStartingUpdater:NO updaterDelegate:benchSparkleDelegate userDriverDelegate:benchSparkleDelegate];
		id<BenchSparkleUpdater> updater = benchSparkle.updater;
		// Before Sparkle starts, so that it doesn't schedule a look that the
		// setting doesn't allow.
		if (updater.automaticallyChecksForUpdates != (automatic != 0)) {
			updater.automaticallyChecksForUpdates = automatic != 0;
		}
		// The updater, not the controller, so that a failure is logged, not
		// shown in an alert at startup.
		if (![updater startUpdater:&error]) {
			benchUpdateFailed((char *)benchUTF8([NSString stringWithFormat:@"couldn't start Sparkle: %@", error.localizedDescription]));
		}
	});
}

void benchUpdaterSetAutomatic(int automatic) {
	benchOnMain(^{
		id<BenchSparkleUpdater> updater = benchSparkle.updater;
		// Setting it, even to what it is, makes Sparkle plan its next look
		// again.
		if (updater != nil && updater.automaticallyChecksForUpdates != (automatic != 0)) {
			updater.automaticallyChecksForUpdates = automatic != 0;
		}
	});
}

// benchSparkleCheckWhenFree has Sparkle look, or show the version it found,
// once it's free to, by deadline. Sparkle drops a look asked for while it's
// busy, and Bench's menu item and Install… are never disabled. Main thread
// only.
static void benchSparkleCheckWhenFree(NSDate *deadline) {
	if (benchSparkle.updater.canCheckForUpdates) {
		benchSparkleCheckWaits = NO;
		[benchSparkle checkForUpdates:nil];
		return;
	}
	if (deadline.timeIntervalSinceNow <= 0) {
		benchSparkleCheckWaits = NO;
		benchUpdateFailed("Sparkle was busy, so it didn't look for a new version");
		return;
	}
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(BenchSparkleWaitStep * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		benchSparkleCheckWhenFree(deadline);
	});
}

void benchUpdaterCheck(void) {
	benchOnMain(^{
		if (benchSparkle == nil || benchSparkleCheckWaits) {
			return;
		}
		benchSparkleCheckWaits = YES;
		benchSparkleCheckWhenFree([NSDate dateWithTimeIntervalSinceNow:BenchSparkleWait]);
	});
}

NS_ASSUME_NONNULL_END
