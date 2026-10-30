//go:build darwin && cgo && !server

// notify_darwin.m: notifications for Web items and terminals, with
// UserNotifications, and the badge on Bench's Dock icon.
//
// UNUserNotificationCenter needs an app bundle: in a bare binary it throws,
// so every entry point checks for a bundle identifier first, and Go returns
// ErrUnsupported. Its methods are safe on any thread.

#import "internal_darwin.h"

#import <UserNotifications/UserNotifications.h>

// The userInfo key that holds a notification's item ID.
static NSString *const BenchItemKey = @"benchItemID";

// BenchNotificationDelegate shows Bench's notifications even while Bench is
// frontmost, and reports clicks.
@interface BenchNotificationDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation BenchNotificationDelegate

- (void)userNotificationCenter:(UNUserNotificationCenter *)center willPresentNotification:(UNNotification *)notification withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
	(void)center;
	(void)notification;
	completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList | UNNotificationPresentationOptionSound);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)response withCompletionHandler:(void (^)(void))completionHandler {
	(void)center;
	id itemID = response.notification.request.content.userInfo[BenchItemKey];
	if ([itemID isKindOfClass:[NSString class]] && [response.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier]) {
		benchEmitItem(BENCH_EV_NOTIFICATION_CLICKED, itemID, nil, nil);
	}
	completionHandler();
}

@end

void benchEmitItem(int kind, NSString *itemID, NSString *title, NSString *body) {
	@autoreleasepool {
		benchEmitItemEvent(kind, (char *)benchUTF8(itemID), (char *)benchUTF8(title), (char *)benchUTF8(body));
	}
}

void benchNotificationsSetUp(void) {
	if (!benchHasBundleID()) {
		return;
	}
	static BenchNotificationDelegate *delegate = nil;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		// The centre holds its delegate weakly; this static keeps it.
		delegate = [[BenchNotificationDelegate alloc] init];
		[UNUserNotificationCenter currentNotificationCenter].delegate = delegate;
	});
}

void benchNotify(const char *itemID, const char *title, const char *subtitle, const char *body) {
	if (!benchHasBundleID()) {
		return;
	}
	NSString *identifier = benchString(itemID);
	NSString *heading = benchString(title);
	NSString *where = benchString(subtitle);
	NSString *text = benchString(body);
	benchNotificationsSetUp();
	benchOnMain(^{
		// For a Web item, the site of its page, which only pages on the
		// item's site can post from, as the subtitle: the title and body are
		// whatever the page says. For a terminal, which has no web view,
		// where Go says it is.
		NSString *site = benchWebItemHost(identifier) ?: where;
		UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
		// Asks the user the first time; after that it answers at once with
		// what they chose.
		[center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound) completionHandler:^(BOOL granted, NSError *error) {
			(void)error;
			if (!granted) {
				return;
			}
			UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
			content.title = heading;
			content.subtitle = site;
			content.body = text;
			content.sound = [UNNotificationSound defaultSound];
			content.threadIdentifier = identifier;
			content.userInfo = @{BenchItemKey : identifier};
			UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:[NSUUID UUID].UUIDString content:content trigger:nil];
			[center addNotificationRequest:request withCompletionHandler:^(NSError *addError) {
				(void)addError;
			}];
		}];
	});
}

void benchSetBadge(int count) {
	benchOnMain(^{
		// The Dock draws the label itself; nil takes it away.
		NSApp.dockTile.badgeLabel = count > 0 ? [NSString stringWithFormat:@"%d", count] : nil;
	});
}
