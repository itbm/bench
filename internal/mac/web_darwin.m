//go:build darwin && cgo && !server

// web_darwin.m: Web items' web views. Each is a WKWebView in Bench's window,
// above the UI's own web view, with a persistent data store per profile, a
// navigation policy that keeps the item on its site, popups for sign-in,
// downloads, a bridge for the page's notifications, and the page in view,
// whose host Bench's UI shows.
//
// Main thread only, apart from the entry points, which hop there.

#import "internal_darwin.h"

// The name of the script message handler that the notification script
// posts to.
static NSString *const BenchNotifyHandler = @"benchNotify";

// The script added to every Web item's pages at document start. Web Push
// isn't available in WKWebView, so it replaces window.Notification, and
// service workers' showNotification, with versions that forward the title
// and body to Bench. Pages see permission as granted.
static NSString *const BenchNotificationScript =
	@"(() => {\n"
	@"  const handler = window.webkit?.messageHandlers?.benchNotify;\n"
	@"  if (!handler || window.Notification?.benchBridge) { return; }\n"
	@"  const post = (title, options) => {\n"
	@"    try {\n"
	@"      handler.postMessage({ title: String(title ?? ''), body: String(options?.body ?? '') });\n"
	@"    } catch (e) {}\n"
	@"  };\n"
	@"  class BenchNotification extends EventTarget {\n"
	@"    constructor(title, options = {}) {\n"
	@"      super();\n"
	@"      this.title = String(title);\n"
	@"      this.body = String(options.body ?? '');\n"
	@"      this.tag = String(options.tag ?? '');\n"
	@"      this.icon = String(options.icon ?? '');\n"
	@"      this.data = options.data ?? null;\n"
	@"      this.onclick = null;\n"
	@"      this.onshow = null;\n"
	@"      this.onclose = null;\n"
	@"      this.onerror = null;\n"
	@"      post(this.title, options);\n"
	@"      setTimeout(() => this.benchFire('show'), 0);\n"
	@"    }\n"
	@"    benchFire(type) {\n"
	@"      const event = new Event(type);\n"
	@"      const listener = this['on' + type];\n"
	@"      if (typeof listener === 'function') { listener.call(this, event); }\n"
	@"      this.dispatchEvent(event);\n"
	@"    }\n"
	@"    close() { this.benchFire('close'); }\n"
	@"    static get permission() { return 'granted'; }\n"
	@"    static requestPermission(callback) {\n"
	@"      if (typeof callback === 'function') { callback('granted'); }\n"
	@"      return Promise.resolve('granted');\n"
	@"    }\n"
	@"  }\n"
	@"  BenchNotification.benchBridge = true;\n"
	@"  BenchNotification.maxActions = 0;\n"
	@"  Object.defineProperty(window, 'Notification', { value: BenchNotification, writable: true, configurable: true });\n"
	@"  if (window.ServiceWorkerRegistration?.prototype) {\n"
	@"    ServiceWorkerRegistration.prototype.showNotification = function (title, options) {\n"
	@"      post(title, options);\n"
	@"      return Promise.resolve();\n"
	@"    };\n"
	@"    ServiceWorkerRegistration.prototype.getNotifications = function () { return Promise.resolve([]); };\n"
	@"  }\n"
	@"  const permissions = navigator.permissions;\n"
	@"  if (permissions?.query) {\n"
	@"    const query = permissions.query.bind(permissions);\n"
	@"    permissions.query = (descriptor) => descriptor?.name === 'notifications'\n"
	@"      ? Promise.resolve({ name: 'notifications', state: 'granted', onchange: null,\n"
	@"          addEventListener() {}, removeEventListener() {} })\n"
	@"      : query(descriptor);\n"
	@"  }\n"
	@"})();\n";

static void *BenchKVOContext = &BenchKVOContext;

// The longest message a page's alert or confirmation shows, in characters.
static const NSUInteger BenchDialogMaxLength = 2000;

// A page that asks for another dialog within this many seconds of the last
// one closing is offered a way to stop them, so that one asking in a loop
// can't keep Bench behind dialogs.
static const CFAbsoluteTime BenchDialogRepeat = 5;

// A page whose process crashes again within this many seconds of Bench
// reloading it after the last crash is reloaded only after a delay, which
// starts at BenchCrashDelay and doubles with each crash, up to
// BenchCrashMaxDelay, so that a page that crashes every time it loads
// doesn't reload without end. The window is measured from the reload, not
// the crash, so that waiting the longest delay doesn't start it all again.
static const CFAbsoluteTime BenchCrashWindow = 60;
static const NSTimeInterval BenchCrashDelay = 2;
static const NSTimeInterval BenchCrashMaxDelay = 60;

// For this many seconds after a prompt to open another app's link appears,
// its Open button can't be pressed, so that a key the user was already
// pressing for something else can't open the link.
static const NSTimeInterval BenchPromptDelay = 1;

@class BenchWebItem;

static void benchWatchUserEvents(void);

// BenchScriptHandler forwards the notification script's messages to its
// item. It holds the item weakly: the user content controller keeps its
// handlers, and the item owns the web view that owns the controller.
@interface BenchScriptHandler : NSObject <WKScriptMessageHandler>
@property (nonatomic, weak) BenchWebItem *item;
@end

// BenchPageState follows the page a web view shows: the one in view, as it
// was committed, not WebKit's URL, which also names pages that are still
// loading, some of which never come into view. The web view's navigation
// delegate passes on its navigation callbacks, the loads it asks for, and
// changes to its URL and hasOnlySecureContent. Each method that returns a
// BOOL reports whether the page's address, or whether it's secure, changed.
@interface BenchPageState : NSObject
// The page's address, or @"" until a page has come into view.
@property (nonatomic, readonly, copy) NSString *address;
// Whether everything the page has loaded came over secure connections:
// WKWebView's hasOnlySecureContent when the page came into view, until the
// page loads something that isn't secure.
@property (nonatomic, readonly) BOOL secureContent;
- (void)loadRequested:(WKNavigation *_Nullable)navigation webView:(WKWebView *)webView;
- (void)navigationStarted:(WKNavigation *_Nullable)navigation webView:(WKWebView *)webView;
- (void)responseReceived:(WKNavigationResponse *)response;
- (BOOL)navigationCommittedIn:(WKWebView *)webView;
- (BOOL)navigationFailed:(WKNavigation *_Nullable)navigation webView:(WKWebView *)webView;
- (void)processEnded;
- (BOOL)URLChangedIn:(WKWebView *)webView;
- (BOOL)secureContentChangedIn:(WKWebView *)webView;
- (NSString *)subtitle;
@end

// BenchPopup is a window that a Web item's page opened, for sign-in or a
// page on the item's site: a real window, so window.opener works.
@interface BenchPopup : NSObject <WKUIDelegate, WKNavigationDelegate, NSWindowDelegate>
@property (nonatomic, weak) BenchWebItem *owner;
@property (nonatomic, strong) NSWindow *window;
@property (nonatomic, strong) WKWebView *webView;
// The first page it showed, for its navigation policy (see
// DecidePopupNavigation): the address it was opened for, or the first one it
// went to after opening blank.
@property (nonatomic, copy) NSString *startURL;
// Whether it has shown a page other than a blank one.
@property (nonatomic) BOOL committed;
// The page in view, which its subtitle names.
@property (nonatomic, strong) BenchPageState *page;
@property (nonatomic) BOOL observing;
// When the user last released a mouse button or pressed a key in it, or 0.
@property (nonatomic) CFAbsoluteTime lastUserEvent;
- (instancetype)initWithConfiguration:(WKWebViewConfiguration *)configuration features:(WKWindowFeatures *)features owner:(BenchWebItem *)owner startURL:(NSString *)startURL;
- (void)show;
- (void)close;
@end

// BenchDialog is a page's alert or confirmation, waiting to be shown.
@interface BenchDialog : NSObject
@property (nonatomic, copy) NSString *message;
// The host of the frame that asked, so the dialog says who's asking.
@property (nonatomic, copy) NSString *host;
@property (nonatomic) BOOL confirm;
// Called once, with whether the user chose OK.
@property (nonatomic, copy) void (^done)(BOOL accepted);
@end

@implementation BenchDialog
@end

// BenchWebItem is a Web item's web view and everything around it.
@interface BenchWebItem : NSObject <WKNavigationDelegate, WKUIDelegate, NSWindowDelegate>
@property (nonatomic, copy) NSString *itemID;
@property (nonatomic, copy) NSString *profileUUID;
// The address Go last asked for: the item's own site, for the navigation
// policy, even after the page moves on.
@property (nonatomic, copy) NSString *requestedURL;
@property (nonatomic, strong) WKWebView *webView;
// The page in view, which Go hears about (see reportPage).
@property (nonatomic, strong) BenchPageState *page;
@property (nonatomic, strong) WKUserContentController *contentController;
@property (nonatomic, strong) BenchScriptHandler *scriptHandler;
@property (nonatomic, strong) NSMutableArray<BenchPopup *> *popups;
// The window it's popped out into, or nil.
@property (nonatomic, strong) NSWindow *popOutWindow;
@property (nonatomic) BOOL closingPopOut;
// Where Go last showed it, in content coordinates, and whether it's shown.
@property (nonatomic) NSRect frame;
@property (nonatomic) BOOL visible;
@property (nonatomic) BOOL observing;
// The page's dialog that waits for the page to be seen, the one showing,
// when the last one closed, and whether the user stopped the page's
// dialogs (until it goes to another page).
@property (nonatomic, strong, nullable) BenchDialog *waitingDialog;
@property (nonatomic) BOOL showingDialog;
@property (nonatomic) CFAbsoluteTime lastDialogEnded;
@property (nonatomic) BOOL dialogsStopped;
// Whether the page's process crashed and the page is still to be loaded
// again, whether that's waiting for a delay, how many times it has crashed
// in a row (each within BenchCrashWindow of being reloaded after the last),
// when it last did, and when Bench last reloaded it after a crash (0 for
// not since it was loaded).
@property (nonatomic) BOOL needsReload;
@property (nonatomic) BOOL reloadScheduled;
@property (nonatomic) NSUInteger crashCount;
@property (nonatomic) CFAbsoluteTime lastCrash;
@property (nonatomic) CFAbsoluteTime crashReloaded;
// When the user last released a mouse button or pressed a key in its web
// view, or 0: what tells a link the user clicked from one a script clicked.
@property (nonatomic) CFAbsoluteTime lastUserEvent;
// Whether the page in view is a sign-in page on another site that the user
// clicked through to (see SignInByClick), whether the item is going back to
// its own page after that page sent the user on to another site, and the
// navigation that loads the item's own address, which ends it. signInInView
// is what signInByClick was when the page in view came into view.
@property (nonatomic) BOOL signInByClick;
@property (nonatomic) BOOL signInInView;
@property (nonatomic) BOOL returningFromSignIn;
@property (nonatomic, strong, nullable) WKNavigation *homeNavigation;
// Whether a prompt to open another app's link is waiting or showing, and
// whether the user stopped the page's prompts (until it goes to another
// page).
@property (nonatomic) BOOL askingToOpen;
@property (nonatomic) BOOL opensStopped;
- (instancetype)initWithID:(NSString *)itemID profileUUID:(NSString *)profileUUID userAgentSuffix:(NSString *)suffix;
- (void)loadAddress:(NSString *)address;
- (void)returnFromPopOut;
- (void)tearDown;
- (void)showWaitingDialog;
- (void)reloadAfterCrash;
- (WKWebView *)popupWithConfiguration:(WKWebViewConfiguration *)configuration action:(WKNavigationAction *)action features:(WKWindowFeatures *)features webView:(WKWebView *)webView;
- (void)askToOpen:(NSURL *_Nullable)url source:(NSString *_Nullable)source webView:(WKWebView *)webView;
@end

#pragma mark - Helpers

// benchIsWebURL reports whether an address is an http or https one.
static BOOL benchIsWebURL(NSURL *_Nullable url) {
	NSString *scheme = url.scheme.lowercaseString;
	return [scheme isEqualToString:@"http"] || [scheme isEqualToString:@"https"];
}

// benchOriginString writes a security origin as an address, such as
// https://app.slack.com, or returns @"" for an origin without a host.
static NSString *benchOriginString(WKSecurityOrigin *_Nullable origin) {
	NSString *protocol = origin.protocol ?: @"";
	NSString *host = origin.host ?: @"";
	if (protocol.length == 0 || host.length == 0) {
		return @"";
	}
	if ([host containsString:@":"]) {
		host = [NSString stringWithFormat:@"[%@]", host]; // IPv6.
	}
	if (origin.port > 0) {
		return [NSString stringWithFormat:@"%@://%@:%ld", protocol, host, (long)origin.port];
	}
	return [NSString stringWithFormat:@"%@://%@", protocol, host];
}

// benchSameOrigin reports whether two addresses have the same scheme, host,
// and port.
static BOOL benchSameOrigin(NSURL *_Nullable a, NSURL *_Nullable b) {
	if (a.scheme == nil || b.scheme == nil || [a.scheme caseInsensitiveCompare:b.scheme] != NSOrderedSame) {
		return NO;
	}
	if ([(a.host ?: @"") caseInsensitiveCompare:(b.host ?: @"")] != NSOrderedSame) {
		return NO;
	}
	NSNumber *portA = a.port;
	NSNumber *portB = b.port;
	if (portA == nil || portB == nil) {
		return portA == portB;
	}
	return [portA isEqualToNumber:portB];
}

// benchSourceHost names the site that started a navigation, for asking the
// user about it. A navigation from the page itself, rather than a frame in
// it, is credited to the page's site. It returns nil for a frame inside the
// page with no host of its own (a data: frame, or a sandboxed one), which
// mustn't be taken for the page around it.
static NSString *_Nullable benchSourceHost(WKNavigationAction *action, WKWebView *webView) {
	WKFrameInfo *frame = action.sourceFrame;
	NSString *host = frame.securityOrigin.host;
	if (host.length > 0) {
		return host;
	}
	if (frame == nil || frame.isMainFrame) {
		return webView.URL.host ?: @"";
	}
	return nil;
}

// benchSinceUserEvent returns how many seconds ago a user event at a time
// from lastUserEvent was, or -1 for none.
static double benchSinceUserEvent(CFAbsoluteTime at) {
	return at > 0 ? CFAbsoluteTimeGetCurrent() - at : -1;
}

// benchCanBeSeen reports whether a web view can be seen, with Bench active,
// so that a prompt shown now is seen to be about it.
static BOOL benchCanBeSeen(WKWebView *webView) {
	return [NSApp isActive] && webView.window != nil && webView.window.visible && !webView.hiddenOrHasHiddenAncestor;
}

// benchRunModalSoon runs block, which runs an app-modal alert, from the main
// run loop once WebKit's call has finished. It doesn't go on the main
// dispatch queue: that queue is serial, so every later benchOnMain call from
// Go would wait until the alert closed. It runs only in the run loop's
// default mode, so never inside another alert's modal loop, where alerts
// would nest.
static void benchRunModalSoon(dispatch_block_t block) {
	CFRunLoopRef loop = CFRunLoopGetMain();
	CFRunLoopPerformBlock(loop, kCFRunLoopDefaultMode, block);
	CFRunLoopWakeUp(loop);
}

// benchApplyNavigation carries out a navigation decision that Go made, for
// item, in webView, its own or one of its popups.
static void benchApplyNavigation(int decision, NSURL *_Nullable url, NSString *_Nullable source, BenchWebItem *_Nullable item, WKWebView *webView, void (^decisionHandler)(WKNavigationActionPolicy)) {
	switch (decision) {
	case BENCH_NAV_IN_VIEW:
		decisionHandler(WKNavigationActionPolicyAllow);
		break;
	case BENCH_NAV_IN_BROWSER:
		decisionHandler(WKNavigationActionPolicyCancel);
		if (url != nil) {
			[[NSWorkspace sharedWorkspace] openURL:url];
		}
		break;
	case BENCH_NAV_ASK:
		decisionHandler(WKNavigationActionPolicyCancel);
		[item askToOpen:url source:source webView:webView];
		break;
	default:
		decisionHandler(WKNavigationActionPolicyCancel);
		break;
	}
}

#pragma mark - Pages

// benchEmitPage reports the page a Web item shows to Go: its address, and
// whether everything it has loaded came over secure connections. itemURL is
// the item's own address.
static void benchEmitPage(NSString *itemID, NSString *itemURL, NSString *address, BOOL secureContent) {
	@autoreleasepool {
		benchEmitPageEvent(BENCH_EV_WEB_URL, (char *)benchUTF8(itemID), (char *)benchUTF8(itemURL), (char *)benchUTF8(address), secureContent ? 1 : 0);
	}
}

@interface BenchPageState ()
@property (nonatomic, readwrite, copy) NSString *address;
@property (nonatomic, readwrite) BOOL secureContent;
// Whether a navigation of the main frame has started, and has neither come
// into view nor failed, which one it is (WebKit may give none), and the
// address it has reached: where it started, or after any redirects, its
// response's.
@property (nonatomic) BOOL provisional;
@property (nonatomic, strong, nullable) WKNavigation *provisionalNavigation;
@property (nonatomic, copy) NSString *provisionalAddress;
// A load the web view was asked for (loadRequest) that hasn't started yet,
// or nil, and WebKit's URL once it was asked: until a navigation starts,
// WebKit's URL is the address asked for, not the page in view's, even when
// another navigation comes into view.
@property (nonatomic, strong, nullable) WKNavigation *requestedNavigation;
@property (nonatomic, copy) NSString *requestedAddress;
@end

@implementation BenchPageState

- (instancetype)init {
	self = [super init];
	if (self) {
		_address = @"";
		_provisionalAddress = @"";
		_requestedAddress = @"";
	}
	return self;
}

// loadRequested notes a load that the web view has just been asked for.
- (void)loadRequested:(WKNavigation *)navigation webView:(WKWebView *)webView {
	self.requestedNavigation = navigation;
	self.requestedAddress = webView.URL.absoluteString ?: @"";
}

// URLRequestedIn reports whether WebKit's URL is still that of a load the
// web view was asked for, which hasn't started.
- (BOOL)URLRequestedIn:(WKWebView *)webView {
	return self.requestedNavigation != nil && [(webView.URL.absoluteString ?: @"") isEqualToString:self.requestedAddress];
}

// navigationStarted notes a navigation that has started. WebKit's URL is its
// address now: any load asked for has started, or another replaced it.
- (void)navigationStarted:(WKNavigation *)navigation webView:(WKWebView *)webView {
	self.provisional = YES;
	self.provisionalNavigation = navigation;
	self.provisionalAddress = webView.URL.absoluteString ?: @"";
	self.requestedNavigation = nil;
}

// responseReceived notes the address of the main frame's response, which
// is where the navigation under way has reached, after any redirects.
- (void)responseReceived:(WKNavigationResponse *)response {
	NSString *address = response.response.URL.absoluteString;
	if (self.provisional && response.isForMainFrame && address.length > 0) {
		self.provisionalAddress = address;
	}
}

// navigationCommittedIn takes the address of the page that has come into
// view, and whether it has only secure content. The address is WebKit's
// URL, unless that's still a load asked for since the page's navigation
// started: then it's the address the navigation reached.
- (BOOL)navigationCommittedIn:(WKWebView *)webView {
	NSString *address = webView.URL.absoluteString ?: @"";
	if ([self URLRequestedIn:webView] && self.provisionalAddress.length > 0) {
		address = self.provisionalAddress;
	}
	self.provisional = NO;
	self.provisionalNavigation = nil;
	self.provisionalAddress = @"";
	BOOL secure = webView.hasOnlySecureContent;
	BOOL changed = ![address isEqualToString:self.address] || secure != self.secureContent;
	self.address = address;
	self.secureContent = secure;
	return changed;
}

// navigationFailed ends a navigation that failed, or was cancelled, before
// its page came into view, which leaves the page that was in view. Its
// address, and whether it's secure, are read again (see URLChangedIn and
// secureContentChangedIn), in case they changed while the navigation was
// under way. A navigation that a newer one replaced changes nothing.
- (BOOL)navigationFailed:(WKNavigation *)navigation webView:(WKWebView *)webView {
	if (navigation != nil && navigation == self.requestedNavigation) {
		self.requestedNavigation = nil; // A load asked for that failed before it started.
	}
	if (!self.provisional || navigation != self.provisionalNavigation) {
		return NO; // An older navigation, which a newer one replaced.
	}
	self.provisional = NO;
	self.provisionalNavigation = nil;
	self.provisionalAddress = @"";
	BOOL changed = [self URLChangedIn:webView];
	return [self secureContentChangedIn:webView] || changed;
}

// processEnded forgets the navigations under way, or asked for, when the
// page's process ended. WebKit's URL is empty until a page loads again.
- (void)processEnded {
	self.provisional = NO;
	self.provisionalNavigation = nil;
	self.provisionalAddress = @"";
	self.requestedNavigation = nil;
}

// URLChangedIn takes the web view's URL as the page's address when the page
// changed it itself, with pushState or a link to a part of the page. WebKit
// reports a navigation's URL just before it says that the navigation has
// started, so the web view's observer calls this once WebKit is done. While
// a navigation is under way, or a load asked for hasn't started, the URL
// names a page that's still to come into view, so it's ignored. So is a URL
// with another origin than the page's, which a page can't move to by
// itself.
- (BOOL)URLChangedIn:(WKWebView *)webView {
	if (self.provisional || self.address.length == 0 || [self URLRequestedIn:webView]) {
		return NO;
	}
	NSURL *url = webView.URL;
	NSString *address = url.absoluteString ?: @"";
	if (address.length == 0 || [address isEqualToString:self.address] || !benchSameOrigin(url, [NSURL URLWithString:self.address])) {
		return NO;
	}
	self.address = address;
	// Content that isn't secure may have loaded meanwhile, which
	// secureContentChangedIn ignored while WebKit's URL wasn't the page's.
	[self secureContentChangedIn:webView];
	return YES;
}

// secureContentChangedIn notes a page in view loading content that isn't
// secure. Only a new page can be secure again: what a page has loaded stays
// loaded. hasOnlySecureContent is about the page in view only while no
// navigation is under way and WebKit's URL is on the page's origin: WebKit
// changes both for a navigation just before it says that the navigation has
// started, and clears both when the page's process ends. Otherwise it's
// ignored, and read again when the navigation fails.
- (BOOL)secureContentChangedIn:(WKWebView *)webView {
	if (self.provisional || self.address.length == 0 || !self.secureContent || webView.hasOnlySecureContent) {
		return NO;
	}
	if (!benchSameOrigin(webView.URL, [NSURL URLWithString:self.address])) {
		return NO;
	}
	self.secureContent = NO;
	return YES;
}

// subtitle is the subtitle of a window that shows the page: its host, or
// that it has no web address, after "Not secure — " if it isn't secure (see
// pageSubtitle), or @"" for no page in view yet.
- (NSString *)subtitle {
	if (self.address.length == 0) {
		return @"";
	}
	char *text = benchPageSubtitle((char *)benchUTF8(self.address), self.secureContent ? 1 : 0);
	NSString *subtitle = text != NULL ? benchString(text) : @"";
	free(text);
	return subtitle;
}

@end

#pragma mark - Downloads

// BenchDownloads saves Web items' downloads to the Downloads folder.
@interface BenchDownloads : NSObject <WKDownloadDelegate>
// Where each download in progress is going: the path Go chose for it.
@property (nonatomic, strong) NSMapTable<WKDownload *, NSString *> *destinations;
@end

// benchQuarantine marks a downloaded file as coming from the web, as
// browsers do, so that Gatekeeper checks it before it runs, unless it's
// marked already.
static void benchQuarantine(NSURL *file, NSURL *_Nullable data, NSURL *_Nullable origin) {
	id existing = nil;
	if ([file getResourceValue:&existing forKey:NSURLQuarantinePropertiesKey error:NULL] && existing != nil) {
		return;
	}
	NSMutableDictionary *properties = [NSMutableDictionary dictionary];
	properties[(__bridge NSString *)kLSQuarantineAgentNameKey] = @"Bench";
	properties[(__bridge NSString *)kLSQuarantineTypeKey] = (__bridge NSString *)kLSQuarantineTypeWebDownload;
	if (data != nil) {
		properties[(__bridge NSString *)kLSQuarantineDataURLKey] = data;
	}
	if (origin != nil) {
		properties[(__bridge NSString *)kLSQuarantineOriginURLKey] = origin;
	}
	[file setResourceValue:properties forKey:NSURLQuarantinePropertiesKey error:NULL];
}

@implementation BenchDownloads

- (instancetype)init {
	self = [super init];
	if (self) {
		_destinations = [NSMapTable strongToStrongObjectsMapTable];
	}
	return self;
}

- (void)download:(WKDownload *)download decideDestinationUsingResponse:(NSURLResponse *)response suggestedFilename:(NSString *)suggestedFilename completionHandler:(void (^)(NSURL *_Nullable destination))completionHandler {
	(void)response;
	NSURL *folder = [[NSFileManager defaultManager] URLsForDirectory:NSDownloadsDirectory inDomains:NSUserDomainMask].firstObject;
	// Go tells names apart as APFS does, ignoring case, but can't normalise
	// them, so every name it sees is in one form: "é" as one character, not
	// "e" and an accent.
	NSString *name = suggestedFilename.precomposedStringWithCanonicalMapping;
	char *path = folder != nil ? benchDownloadPath((char *)benchUTF8(folder.path), (char *)benchUTF8(name)) : NULL;
	NSString *chosen = path != NULL ? benchString(path) : @"";
	free(path);
	if (chosen.length == 0) {
		completionHandler(nil); // Cancels it.
		return;
	}
	[self.destinations setObject:chosen forKey:download];
	completionHandler([NSURL fileURLWithPath:chosen]);
}

- (void)downloadDidFinish:(WKDownload *)download {
	NSString *path = [self.destinations objectForKey:download];
	[self.destinations removeObjectForKey:download];
	if (path == nil) {
		return;
	}
	benchDownloadEnded((char *)benchUTF8(path));
	benchQuarantine([NSURL fileURLWithPath:path], download.originalRequest.URL, download.webView.URL);
	// Bounces the Downloads stack in the Dock, as browsers do.
	[[NSDistributedNotificationCenter defaultCenter] postNotificationName:@"com.apple.DownloadFileFinished" object:path];
}

- (void)download:(WKDownload *)download didFailWithError:(NSError *)error resumeData:(NSData *)resumeData {
	(void)error;
	(void)resumeData;
	NSString *path = [self.destinations objectForKey:download];
	[self.destinations removeObjectForKey:download];
	if (path != nil) {
		benchDownloadEnded((char *)benchUTF8(path));
	}
}

@end

// benchDownloads returns the downloads' delegate, which a download holds
// weakly.
static BenchDownloads *benchDownloads(void) {
	static BenchDownloads *downloads = nil;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		downloads = [[BenchDownloads alloc] init];
	});
	return downloads;
}

// benchDownloadAllowed reports whether a Web item's page may start a
// download now, after the user's last event in the web view asking,
// userEvent (see downloadLimiter), and if so counts it.
static BOOL benchDownloadAllowed(NSString *_Nullable itemID, CFAbsoluteTime userEvent) {
	return benchAllowDownload((char *)benchUTF8(itemID), userEvent, CFAbsoluteTimeGetCurrent()) != 0;
}

// benchResponsePolicy shows a response, or downloads it (see
// DecideResponse). itemURL is the Web item's own address, and userEvent the
// user's last event in the web view (see benchDownloadAllowed): a download
// the user didn't ask for is cancelled.
static WKNavigationResponsePolicy benchResponsePolicy(WKNavigationResponse *navigationResponse, NSString *itemURL, NSString *_Nullable itemID, CFAbsoluteTime userEvent) {
	NSString *disposition = @"";
	NSURLResponse *response = navigationResponse.response;
	if ([response isKindOfClass:[NSHTTPURLResponse class]]) {
		disposition = [(NSHTTPURLResponse *)response valueForHTTPHeaderField:@"Content-Disposition"] ?: @"";
	}
	NSString *address = response.URL.absoluteString ?: @"";
	int decision = benchDecideResponse((char *)benchUTF8(itemURL), (char *)benchUTF8(address), navigationResponse.forMainFrame ? 1 : 0,
	                                   navigationResponse.canShowMIMEType ? 1 : 0, (char *)benchUTF8(disposition));
	if (decision != BENCH_RESPONSE_DOWNLOAD) {
		return WKNavigationResponsePolicyAllow;
	}
	return benchDownloadAllowed(itemID, userEvent) ? WKNavigationResponsePolicyDownload : WKNavigationResponsePolicyCancel;
}

// benchDownloadLinkPolicy decides a link with a download attribute by the
// frame it's in, as benchResponsePolicy decides an attachment: the main
// frame, and frames on the item's site, may download, if the user asked.
// Embedded content from another site, such as an advert, can't save files by
// itself, so its download links do nothing.
static WKNavigationActionPolicy benchDownloadLinkPolicy(WKNavigationAction *navigationAction, NSString *itemURL, NSString *_Nullable itemID, CFAbsoluteTime userEvent) {
	WKFrameInfo *source = navigationAction.sourceFrame;
	NSString *origin = benchOriginString(source.securityOrigin);
	int decision = benchDecideResponse((char *)benchUTF8(itemURL), (char *)benchUTF8(origin), source.isMainFrame ? 1 : 0, 0, (char *)"attachment");
	if (decision != BENCH_RESPONSE_DOWNLOAD || !benchDownloadAllowed(itemID, userEvent)) {
		return WKNavigationActionPolicyCancel;
	}
	return WKNavigationActionPolicyDownload;
}

#pragma mark - Script messages

@implementation BenchScriptHandler

- (void)userContentController:(WKUserContentController *)userContentController didReceiveScriptMessage:(WKScriptMessage *)message {
	(void)userContentController;
	BenchWebItem *item = self.item;
	if (item == nil || ![message.name isEqualToString:BenchNotifyHandler]) {
		return;
	}
	// Only the item's own page may post, from its main frame on the item's
	// site: popups share the item's content controller, and every frame in
	// a page, embedded content from other sites among them, can reach the
	// handler.
	if (message.webView != item.webView) {
		return;
	}
	WKFrameInfo *frame = message.frameInfo;
	NSString *origin = benchOriginString(frame.securityOrigin);
	if (!benchNotificationAllowed((char *)benchUTF8(item.requestedURL), (char *)benchUTF8(origin), frame.isMainFrame ? 1 : 0)) {
		return;
	}
	NSDictionary *body = [message.body isKindOfClass:[NSDictionary class]] ? message.body : nil;
	id title = body[@"title"];
	id text = body[@"body"];
	NSString *titleText = [title isKindOfClass:[NSString class]] ? title : @"";
	NSString *bodyText = [text isKindOfClass:[NSString class]] ? text : @"";
	if (titleText.length == 0 && bodyText.length == 0) {
		return;
	}
	benchEmitItem(BENCH_EV_WEB_NOTIFICATION, item.itemID, titleText, bodyText);
}

@end

#pragma mark - Popups

@implementation BenchPopup

- (instancetype)initWithConfiguration:(WKWebViewConfiguration *)configuration features:(WKWindowFeatures *)features owner:(BenchWebItem *)owner startURL:(NSString *)startURL {
	self = [super init];
	if (self) {
		_owner = owner;
		_startURL = [startURL copy];
		CGFloat width = features.width != nil ? features.width.doubleValue : 520;
		CGFloat height = features.height != nil ? features.height.doubleValue : 640;
		width = MIN(MAX(width, 320), 1600);
		height = MIN(MAX(height, 320), 1200);
		NSRect contentRect = NSMakeRect(0, 0, width, height);
		NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
		_window = [[NSWindow alloc] initWithContentRect:contentRect styleMask:style backing:NSBackingStoreBuffered defer:NO];
		// Under ARC the popup owns the window; AppKit mustn't release it too.
		_window.releasedWhenClosed = NO;
		_window.delegate = self;
		// The configuration WebKit passes must be used as it is, so the new
		// page is connected to its opener.
		_webView = [[WKWebView alloc] initWithFrame:contentRect configuration:configuration];
		_webView.UIDelegate = self;
		_webView.navigationDelegate = self;
		_webView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		_window.contentView = _webView;
		_page = [[BenchPageState alloc] init];
		[_webView addObserver:self forKeyPath:@"title" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		[_webView addObserver:self forKeyPath:@"hasOnlySecureContent" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		_observing = YES;
	}
	return self;
}

- (void)dealloc {
	if (_observing) {
		[_webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[_webView removeObserver:self forKeyPath:@"hasOnlySecureContent" context:BenchKVOContext];
	}
}

- (void)show {
	[self.window center];
	[self.window makeKeyAndOrderFront:nil];
}

- (void)close {
	[self.window close];
}

// closeSoon closes the popup once WebKit has finished the call it's in.
- (void)closeSoon {
	BenchPopup *popup = self;
	dispatch_async(dispatch_get_main_queue(), ^{
		[popup close];
	});
}

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary<NSKeyValueChangeKey, id> *)change context:(void *)context {
	if (context != BenchKVOContext) {
		[super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
		return;
	}
	if ([keyPath isEqualToString:@"hasOnlySecureContent"]) {
		if ([self.page secureContentChangedIn:self.webView]) {
			[self showPage];
		}
		return;
	}
	NSString *title = self.webView.title;
	if (title.length > 0) {
		self.window.title = title;
	}
}

// showPage names the page in view in the popup's subtitle. A popup has no
// address bar: its subtitle says which site it shows, and whether the page
// isn't secure.
- (void)showPage {
	self.window.subtitle = [self.page subtitle];
}

- (void)windowWillClose:(NSNotification *)notification {
	(void)notification;
	if (self.observing) {
		[self.webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[self.webView removeObserver:self forKeyPath:@"hasOnlySecureContent" context:BenchKVOContext];
		self.observing = NO;
	}
	self.webView.UIDelegate = nil;
	self.webView.navigationDelegate = nil;
	[self.webView stopLoading];
	self.window.delegate = nil;
	// Let go of the popup (and its window) once AppKit has finished closing
	// the window, not in the middle of it.
	BenchPopup *popup = self;
	dispatch_async(dispatch_get_main_queue(), ^{
		[popup.owner.popups removeObject:popup];
	});
}

- (void)webViewDidClose:(WKWebView *)webView {
	(void)webView;
	[self.window close];
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
	return [self.owner popupWithConfiguration:configuration action:navigationAction features:windowFeatures webView:webView];
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	BenchWebItem *owner = self.owner;
	NSString *itemURL = owner.requestedURL ?: @"";
	if (navigationAction.shouldPerformDownload) {
		decisionHandler(benchDownloadLinkPolicy(navigationAction, itemURL, owner.itemID, self.lastUserEvent));
		return;
	}
	WKFrameInfo *target = navigationAction.targetFrame;
	if (target == nil || !target.isMainFrame) {
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	NSURL *url = navigationAction.request.URL;
	BOOL linkActivated = navigationAction.navigationType == WKNavigationTypeLinkActivated;
	BOOL clicked = benchUserClicked(linkActivated ? 1 : 0, benchSinceUserEvent(self.lastUserEvent)) != 0;
	int decision = benchDecidePopupNavigation((char *)benchUTF8(itemURL), (char *)benchUTF8(self.startURL), (char *)benchUTF8(url.absoluteString), clicked ? 1 : 0);
	if (decision == BENCH_NAV_IN_VIEW) {
		NSURL *start = [NSURL URLWithString:self.startURL];
		if (!benchIsWebURL(start) && benchIsWebURL(url)) {
			self.startURL = url.absoluteString ?: @"";
		}
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	benchApplyNavigation(decision, url, benchSourceHost(navigationAction, webView), owner, webView, decisionHandler);
	if ((decision == BENCH_NAV_IN_BROWSER || decision == BENCH_NAV_ASK) && (!self.committed || !clicked)) {
		// It was opened only to go there, as a redirector's popup is (some
		// show a page of their own first, which then sends itself on):
		// there's nothing to leave on screen. A link the user clicked in a
		// page that stays keeps its popup.
		[self closeSoon];
	}
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationResponse:(WKNavigationResponse *)navigationResponse decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
	(void)webView;
	BenchWebItem *owner = self.owner;
	WKNavigationResponsePolicy policy = benchResponsePolicy(navigationResponse, owner.requestedURL ?: @"", owner.itemID, self.lastUserEvent);
	decisionHandler(policy);
	if (policy == WKNavigationResponsePolicyCancel && navigationResponse.forMainFrame && !self.committed) {
		// A download the user didn't ask for, in a popup opened only for
		// it: there's nothing to leave on screen.
		[self closeSoon];
	}
}

- (void)webView:(WKWebView *)webView didStartProvisionalNavigation:(WKNavigation *)navigation {
	[self.page navigationStarted:navigation webView:webView];
}

- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
	(void)navigation;
	NSURL *url = webView.URL;
	if (url != nil && ![url.absoluteString isEqualToString:@"about:blank"]) {
		self.committed = YES;
	}
	if ([self.page navigationCommittedIn:webView]) {
		[self showPage];
	}
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
	(void)error;
	if ([self.page navigationFailed:navigation webView:webView]) {
		[self showPage];
	}
}

- (void)webView:(WKWebView *)webView navigationAction:(WKNavigationAction *)navigationAction didBecomeDownload:(WKDownload *)download {
	(void)webView;
	(void)navigationAction;
	download.delegate = benchDownloads();
	if (!self.committed) {
		[self closeSoon];
	}
}

- (void)webView:(WKWebView *)webView navigationResponse:(WKNavigationResponse *)navigationResponse didBecomeDownload:(WKDownload *)download {
	(void)webView;
	(void)navigationResponse;
	download.delegate = benchDownloads();
	if (!self.committed) {
		[self closeSoon];
	}
}

@end

#pragma mark - Web items

@implementation BenchWebItem

- (instancetype)initWithID:(NSString *)itemID profileUUID:(NSString *)profileUUID userAgentSuffix:(NSString *)suffix {
	self = [super init];
	if (self) {
		_itemID = [itemID copy];
		_profileUUID = [profileUUID copy];
		_requestedURL = @"";
		_page = [[BenchPageState alloc] init];
		_popups = [NSMutableArray array];
		benchWatchUserEvents();

		WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
		if (@available(macOS 14.0, *)) {
			// A persistent store per profile, found again by the same UUID
			// after Bench restarts, so logins last and stay separate.
			NSUUID *identifier = [[NSUUID alloc] initWithUUIDString:profileUUID];
			if (identifier != nil) {
				configuration.websiteDataStore = [WKWebsiteDataStore dataStoreForIdentifier:identifier];
			}
		}
		if (suffix.length > 0) {
			configuration.applicationNameForUserAgent = suffix;
		}
		// Pages open windows only when the user clicks, as in browsers, so
		// that a page or anything embedded in it can't open popups, or send
		// addresses to other apps, by itself. Popups inherit this.
		configuration.preferences.javaScriptCanOpenWindowsAutomatically = NO;

		_contentController = [[WKUserContentController alloc] init];
		WKUserScript *script = [[WKUserScript alloc] initWithSource:BenchNotificationScript injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES];
		[_contentController addUserScript:script];
		_scriptHandler = [[BenchScriptHandler alloc] init];
		_scriptHandler.item = self;
		[_contentController addScriptMessageHandler:_scriptHandler name:BenchNotifyHandler];
		configuration.userContentController = _contentController;

		_webView = [[WKWebView alloc] initWithFrame:NSMakeRect(0, 0, 800, 600) configuration:configuration];
		_webView.navigationDelegate = self;
		_webView.UIDelegate = self;
		_webView.allowsBackForwardNavigationGestures = YES;
		_webView.hidden = YES;
		[_webView addObserver:self forKeyPath:@"title" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		[_webView addObserver:self forKeyPath:@"URL" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		[_webView addObserver:self forKeyPath:@"hasOnlySecureContent" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		_observing = YES;
	}
	return self;
}

- (void)dealloc {
	// A web view mustn't outlive its observers' registrations.
	if (_observing) {
		[_webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[_webView removeObserver:self forKeyPath:@"URL" context:BenchKVOContext];
		[_webView removeObserver:self forKeyPath:@"hasOnlySecureContent" context:BenchKVOContext];
	}
}

- (void)loadAddress:(NSString *)address {
	self.requestedURL = address;
	// Loading starts a new process anyway, and a new page starts counting
	// crashes afresh.
	self.needsReload = NO;
	self.crashReloaded = 0;
	NSURL *url = [NSURL URLWithString:address];
	if (url == nil || url.scheme.length == 0) {
		url = [NSURL URLWithString:@"about:blank"];
	}
	self.homeNavigation = [self.webView loadRequest:[NSURLRequest requestWithURL:url]];
	[self.page loadRequested:self.homeNavigation webView:self.webView];
}

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary<NSKeyValueChangeKey, id> *)change context:(void *)context {
	if (context != BenchKVOContext) {
		[super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
		return;
	}
	if ([keyPath isEqualToString:@"title"]) {
		NSString *title = self.webView.title;
		if (title.length == 0) {
			return;
		}
		if (self.popOutWindow != nil) {
			self.popOutWindow.title = title;
		}
		benchEmitItem(BENCH_EV_WEB_TITLE, self.itemID, title, nil);
	} else if ([keyPath isEqualToString:@"URL"]) {
		// WebKit reports a navigation's URL just before it says that the
		// navigation has started, so the URL is looked at once it has.
		__weak BenchWebItem *weakItem = self;
		dispatch_async(dispatch_get_main_queue(), ^{
			BenchWebItem *item = weakItem;
			if (item != nil && item.observing && [item.page URLChangedIn:item.webView]) {
				[item reportPage];
			}
		});
	} else if ([keyPath isEqualToString:@"hasOnlySecureContent"]) {
		if ([self.page secureContentChangedIn:self.webView]) {
			[self reportPage];
		}
	}
}

// reportPage tells Go about the page in view (see BenchPageState), and names
// it in the subtitle of the item's own window, if it's popped out.
- (void)reportPage {
	NSString *address = self.page.address;
	if (address.length == 0) {
		return;
	}
	benchEmitPage(self.itemID, self.requestedURL, address, self.page.secureContent);
	if (self.popOutWindow != nil) {
		self.popOutWindow.subtitle = [self.page subtitle];
	}
}

- (void)returnFromPopOut {
	NSWindow *window = self.popOutWindow;
	if (window == nil) {
		return;
	}
	window.delegate = nil;
	self.popOutWindow = nil;
	WKWebView *web = self.webView;
	[web removeFromSuperview];
	web.autoresizingMask = NSViewNotSizable;
	web.hidden = YES;
	self.visible = NO;
	NSView *container = benchWebContainerView();
	if (container != nil) {
		[container addSubview:web];
		web.frame = self.frame;
	}
	// Keep the window until AppKit has finished closing it.
	dispatch_async(dispatch_get_main_queue(), ^{
		(void)window;
	});
}

- (void)tearDown {
	benchYieldFocusFrom(self.webView);
	if (self.observing) {
		[self.webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
		[self.webView removeObserver:self forKeyPath:@"URL" context:BenchKVOContext];
		[self.webView removeObserver:self forKeyPath:@"hasOnlySecureContent" context:BenchKVOContext];
		self.observing = NO;
	}
	[self.contentController removeScriptMessageHandlerForName:BenchNotifyHandler];
	[self.webView stopLoading];
	self.webView.navigationDelegate = nil;
	self.webView.UIDelegate = nil;
	self.needsReload = NO;
	// WebKit needs every dialog's answer, even from a page that's going.
	BenchDialog *dialog = self.waitingDialog;
	self.waitingDialog = nil;
	if (dialog != nil) {
		dialog.done(NO);
	}
	for (BenchPopup *popup in [self.popups copy]) {
		[popup close];
	}
	if (self.popOutWindow != nil) {
		NSWindow *window = self.popOutWindow;
		self.closingPopOut = YES;
		[self returnFromPopOut];
		[window close];
		self.closingPopOut = NO;
	}
	[self.webView removeFromSuperview];
}

#pragma mark Navigation

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	// A link with a download attribute.
	if (navigationAction.shouldPerformDownload) {
		decisionHandler(benchDownloadLinkPolicy(navigationAction, self.requestedURL, self.itemID, self.lastUserEvent));
		return;
	}
	// Frames inside the page (embedded content) always load. A navigation
	// without a target frame asks for a new window, which the UI delegate
	// decides.
	WKFrameInfo *target = navigationAction.targetFrame;
	if (target == nil || !target.isMainFrame) {
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	NSURL *url = navigationAction.request.URL;
	NSString *address = url.absoluteString ?: @"";
	BOOL linkActivated = navigationAction.navigationType == WKNavigationTypeLinkActivated;
	BOOL clicked = benchUserClicked(linkActivated ? 1 : 0, benchSinceUserEvent(self.lastUserEvent)) != 0;
	int decision = benchDecideNavigation((char *)benchUTF8(self.requestedURL), (char *)benchUTF8(address), clicked ? 1 : 0, self.signInByClick ? 1 : 0);
	if (decision == BENCH_NAV_IN_VIEW) {
		// A click to another site's sign-in page starts it here, but only the
		// page that then comes into view can end it (see
		// didCommitNavigation): a navigation that never does mustn't.
		self.signInByClick = benchSignInByClick((char *)benchUTF8(self.requestedURL), (char *)benchUTF8(address), clicked ? 1 : 0, 0, self.signInByClick ? 1 : 0) != 0;
	} else if (decision == BENCH_NAV_IN_BROWSER && !clicked && self.signInByClick && self.returningFromSignIn) {
		// The sign-in page is still in view, sending the user on again while
		// the item goes back to its own page: that would only open more of
		// the browser's tabs.
		decision = BENCH_NAV_CANCEL;
	} else if (decision == BENCH_NAV_IN_BROWSER && !clicked && self.signInByClick) {
		// A sign-in page the user clicked through to sent them on to another
		// site: that goes to the browser, and the item goes back to its own
		// page, so that the sign-in page can't send them anywhere else. It's
		// still a sign-in page the user clicked through to until the item's
		// page is in view, so that it can't send them on in the meantime.
		self.returningFromSignIn = YES;
		__weak BenchWebItem *weakItem = self;
		dispatch_async(dispatch_get_main_queue(), ^{
			BenchWebItem *item = weakItem;
			if (item != nil && item.webView.navigationDelegate == item) {
				[item loadAddress:item.requestedURL];
			}
		});
	}
	benchApplyNavigation(decision, url, benchSourceHost(navigationAction, webView), self, webView, decisionHandler);
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationResponse:(WKNavigationResponse *)navigationResponse decisionHandler:(void (^)(WKNavigationResponsePolicy))decisionHandler {
	(void)webView;
	[self.page responseReceived:navigationResponse];
	decisionHandler(benchResponsePolicy(navigationResponse, self.requestedURL, self.itemID, self.lastUserEvent));
}

- (void)webView:(WKWebView *)webView navigationAction:(WKNavigationAction *)navigationAction didBecomeDownload:(WKDownload *)download {
	(void)webView;
	(void)navigationAction;
	download.delegate = benchDownloads();
}

- (void)webView:(WKWebView *)webView navigationResponse:(WKNavigationResponse *)navigationResponse didBecomeDownload:(WKDownload *)download {
	(void)webView;
	(void)navigationResponse;
	download.delegate = benchDownloads();
}

- (void)webView:(WKWebView *)webView didStartProvisionalNavigation:(WKNavigation *)navigation {
	[self.page navigationStarted:navigation webView:webView];
}

- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
	// A new page may ask for dialogs again.
	self.dialogsStopped = NO;
	// And to open other apps' links.
	self.opensStopped = NO;
	// A page on the item's site ends a sign-in that the user clicked through
	// to (see SignInByClick), and so does the item's own address, wherever
	// it redirected to: the item's own sign-in starts without a click.
	if (navigation != nil && navigation == self.homeNavigation) {
		self.signInByClick = NO;
		self.homeNavigation = nil;
	} else {
		self.signInByClick = benchSignInByClick((char *)benchUTF8(self.requestedURL), (char *)benchUTF8(webView.URL.absoluteString), 0, 1, self.signInByClick ? 1 : 0) != 0;
	}
	self.signInInView = self.signInByClick;
	self.returningFromSignIn = NO;
	if ([self.page navigationCommittedIn:webView]) {
		[self reportPage];
	}
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
	if ([self.page navigationFailed:navigation webView:webView]) {
		[self reportPage];
	}
	// A navigation that fails before its page shows, such as a click to a
	// sign-in page that doesn't load, leaves the page in view, and so what
	// that page is: the item's own page mustn't be taken for a sign-in page
	// the user clicked through to. One cancelled because another navigation
	// replaced it changes nothing, since that one was decided by it.
	if ([error.domain isEqualToString:NSURLErrorDomain] && error.code == NSURLErrorCancelled) {
		return;
	}
	self.signInByClick = self.signInInView;
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
	(void)webView;
	[self.page processEnded];
	// The page's process crashed or was ended: load it again. With this
	// method, WebKit leaves reloading to Bench, which mustn't do it without
	// limit (see reloadAfterCrash).
	CFAbsoluteTime now = CFAbsoluteTimeGetCurrent();
	BOOL again = self.crashReloaded > 0 && now - self.crashReloaded < BenchCrashWindow;
	self.crashCount = again ? self.crashCount + 1 : 1;
	self.lastCrash = now;
	self.needsReload = YES;
	[self reloadAfterCrash];
}

// canBeSeen reports whether the page is on screen, in Bench's window or its
// own, as far as Bench can tell.
- (BOOL)canBeSeen {
	NSWindow *window = self.popOutWindow;
	if (window != nil) {
		return window.visible && !window.miniaturized;
	}
	return self.visible && benchWebContainerView() != nil;
}

// reloadAfterCrash loads the page again after its process crashed, once the
// page can be seen: a hidden page doesn't need a process, and one that
// crashes over and over in a tab the user isn't looking at shouldn't keep
// starting new ones. The first crash in a while reloads at once, and each
// one after that waits longer (see BenchCrashWindow), counted from the
// crash, so that a page shown after being hidden for that long reloads at
// once.
- (void)reloadAfterCrash {
	if (!self.needsReload || self.reloadScheduled || ![self canBeSeen]) {
		return;
	}
	NSTimeInterval delay = 0;
	if (self.crashCount > 1) {
		NSUInteger doublings = MIN(self.crashCount - 2, (NSUInteger)8);
		delay = MIN(BenchCrashMaxDelay, BenchCrashDelay * (double)((NSUInteger)1 << doublings));
		delay = MAX(0.0, delay - (CFAbsoluteTimeGetCurrent() - self.lastCrash));
	}
	self.reloadScheduled = YES;
	__weak BenchWebItem *weakSelf = self;
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(delay * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		BenchWebItem *item = weakSelf;
		if (item == nil) {
			return;
		}
		item.reloadScheduled = NO;
		// Hidden meanwhile, it reloads when it's shown again.
		if (!item.needsReload || ![item canBeSeen]) {
			return;
		}
		item.needsReload = NO;
		item.crashReloaded = CFAbsoluteTimeGetCurrent();
		[item.webView reload];
	});
}

#pragma mark UI

// lastUserEventIn returns when the user last released a mouse button or
// pressed a key in webView, the item's own or one of its popups', or 0.
- (CFAbsoluteTime)lastUserEventIn:(WKWebView *)webView {
	if (webView == self.webView) {
		return self.lastUserEvent;
	}
	for (BenchPopup *popup in self.popups) {
		if (popup.webView == webView) {
			return popup.lastUserEvent;
		}
	}
	return 0;
}

- (WKWebView *)popupWithConfiguration:(WKWebViewConfiguration *)configuration action:(WKNavigationAction *)action features:(WKWindowFeatures *)features webView:(WKWebView *)webView {
	NSURL *url = action.request.URL;
	NSString *address = url.absoluteString ?: @"";
	WKFrameInfo *source = action.sourceFrame;
	NSString *origin = benchOriginString(source.securityOrigin);
	int decision = benchDecidePopup((char *)benchUTF8(self.requestedURL), (char *)benchUTF8(address), (char *)benchUTF8(origin), source.isMainFrame ? 1 : 0);
	switch (decision) {
	case BENCH_POPUP_WINDOW:
		break;
	case BENCH_POPUP_BROWSER:
		if (url != nil) {
			[[NSWorkspace sharedWorkspace] openURL:url];
		}
		return nil;
	case BENCH_POPUP_ASK:
		[self askToOpen:url source:benchSourceHost(action, webView) webView:webView];
		return nil;
	default:
		return nil;
	}
	BenchPopup *popup = [[BenchPopup alloc] initWithConfiguration:configuration features:features owner:self startURL:address];
	// WebKit opens a window only for the user's click or key press in the
	// page that asked, so the popup starts with that event: a download it
	// was opened for counts as one the user asked for.
	popup.lastUserEvent = [self lastUserEventIn:webView];
	[self.popups addObject:popup];
	[popup show];
	return popup.webView;
}

- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
	return [self popupWithConfiguration:configuration action:navigationAction features:windowFeatures webView:webView];
}

// askToOpen asks the user whether to open an address for another app, naming
// the app and the site that asked (source), and opens it if they agree.
// webView, the item's own or a popup's, is where the page asked. It asks
// nothing for an address that no app on this Mac opens, for a frame with no
// site to name (a nil source), for a page whose prompts the user stopped,
// while another prompt from the item is waiting or showing, or unless the
// web view can be seen with Bench active: so a page can't stack prompts, or
// ask from a tab the user isn't looking at.
//
// The alert is app-modal, so it shows above docked windows, and it waits
// for the page's click to finish first. Cancel is its default button, and
// Open can't be pressed at first, so that a Return or a space the user was
// typing elsewhere can't open the link.
- (void)askToOpen:(NSURL *)url source:(NSString *)source webView:(WKWebView *)webView {
	if (url == nil || source == nil || self.opensStopped || self.askingToOpen || !benchCanBeSeen(webView)) {
		return;
	}
	NSURL *app = [[NSWorkspace sharedWorkspace] URLForApplicationToOpenURL:url];
	if (app == nil) {
		return;
	}
	NSString *appName = [[NSFileManager defaultManager] displayNameAtPath:app.path];
	NSString *from = source.length > 0 ? source : @"This page";
	NSString *scheme = url.scheme ?: @"";
	self.askingToOpen = YES;
	BenchWebItem *item = self;
	benchRunModalSoon(^{
		// The user may have gone to another tab, or app, or closed the item,
		// while an alert that was already showing kept this one waiting. A
		// popup that asked may have closed, which is why its window isn't
		// checked again.
		if (![NSApp isActive] || webView.hiddenOrHasHiddenAncestor || item.webView.navigationDelegate != item) {
			item.askingToOpen = NO;
			return;
		}
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = [NSString stringWithFormat:@"Open this link in %@?", appName];
		alert.informativeText = [NSString stringWithFormat:@"%@ wants to open a %@: link.", from, scheme];
		[alert addButtonWithTitle:@"Cancel"];
		NSButton *openButton = [alert addButtonWithTitle:@"Open"];
		openButton.enabled = NO;
		alert.showsSuppressionButton = YES;
		alert.suppressionButton.title = @"Don't allow more from this page";
		NSTimer *timer = [NSTimer timerWithTimeInterval:BenchPromptDelay repeats:NO block:^(NSTimer *t) {
			(void)t;
			openButton.enabled = YES;
		}];
		[[NSRunLoop mainRunLoop] addTimer:timer forMode:NSRunLoopCommonModes];
		NSModalResponse response = [alert runModal];
		[timer invalidate];
		item.askingToOpen = NO;
		if (alert.suppressionButton.state == NSControlStateValueOn) {
			item.opensStopped = YES;
		}
		if (response == NSAlertSecondButtonReturn) {
			[[NSWorkspace sharedWorkspace] openURL:url];
		}
	});
}

// canShowDialog reports whether the page can be seen, with Bench active, so
// that a dialog shown now is seen to be about it.
- (BOOL)canShowDialog {
	return benchCanBeSeen(self.webView);
}

// askWithMessage keeps a page's alert or confirmation until the page can be
// seen (see showWaitingDialog). A page whose dialogs the user stopped gets
// its answer at once: OK for an alert, Cancel for a confirmation.
- (void)askWithMessage:(NSString *)message frame:(WKFrameInfo *)frame confirm:(BOOL)confirm done:(void (^)(BOOL accepted))done {
	if (self.dialogsStopped || self.waitingDialog != nil || self.showingDialog) {
		done(NO);
		return;
	}
	NSString *text = message ?: @"";
	if (text.length > BenchDialogMaxLength) {
		text = [text substringWithRange:[text rangeOfComposedCharacterSequencesForRange:NSMakeRange(0, BenchDialogMaxLength)]];
	}
	BenchDialog *dialog = [[BenchDialog alloc] init];
	dialog.message = text;
	dialog.host = frame.securityOrigin.host ?: @"";
	dialog.confirm = confirm;
	dialog.done = done;
	self.waitingDialog = dialog;
	[self showWaitingDialog];
}

// showWaitingDialog shows the page's waiting dialog, if the page can be seen.
// It's app-modal rather than a sheet: Bench's window sits below the docked
// windows, which would cover a sheet's buttons. It names the site asking,
// and offers to stop a page that asks again and again.
- (void)showWaitingDialog {
	BenchDialog *dialog = self.waitingDialog;
	if (dialog == nil || self.showingDialog || ![self canShowDialog]) {
		return;
	}
	self.waitingDialog = nil;
	self.showingDialog = YES;
	BOOL offerStop = CFAbsoluteTimeGetCurrent() - self.lastDialogEnded < BenchDialogRepeat;
	BenchWebItem *item = self;
	benchRunModalSoon(^{
		// Another alert that was already showing may have kept this one
		// waiting, while the item was closed, or the user went to another
		// tab or app. A closed item's page gets its answer at once; one that
		// can't be seen keeps its dialog until it can.
		if (item.webView.navigationDelegate != item) {
			item.showingDialog = NO;
			dialog.done(NO);
			return;
		}
		if (![item canShowDialog]) {
			item.showingDialog = NO;
			item.waitingDialog = dialog;
			return;
		}
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = dialog.host.length > 0 ? [NSString stringWithFormat:@"%@ says", dialog.host] : @"This page says";
		alert.informativeText = dialog.message;
		[alert addButtonWithTitle:@"OK"];
		if (dialog.confirm) {
			[alert addButtonWithTitle:@"Cancel"];
		}
		if (offerStop) {
			alert.showsSuppressionButton = YES;
			alert.suppressionButton.title = @"Don't allow more dialogs from this page";
		}
		NSModalResponse response = [alert runModal];
		item.showingDialog = NO;
		item.lastDialogEnded = CFAbsoluteTimeGetCurrent();
		if (offerStop && alert.suppressionButton.state == NSControlStateValueOn) {
			item.dialogsStopped = YES;
		}
		dialog.done(response == NSAlertFirstButtonReturn);
	});
}

- (void)webView:(WKWebView *)webView runJavaScriptAlertPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(void))completionHandler {
	(void)webView;
	[self askWithMessage:message frame:frame confirm:NO done:^(BOOL accepted) {
		(void)accepted;
		completionHandler();
	}];
}

- (void)webView:(WKWebView *)webView runJavaScriptConfirmPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(BOOL))completionHandler {
	(void)webView;
	[self askWithMessage:message frame:frame confirm:YES done:^(BOOL accepted) {
		completionHandler(accepted);
	}];
}

- (void)webView:(WKWebView *)webView runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *))completionHandler {
	(void)webView;
	(void)frame;
	NSOpenPanel *panel = [NSOpenPanel openPanel];
	panel.canChooseFiles = YES;
	panel.canChooseDirectories = parameters.allowsDirectories;
	panel.allowsMultipleSelection = parameters.allowsMultipleSelection;
	[panel beginWithCompletionHandler:^(NSModalResponse result) {
		completionHandler(result == NSModalResponseOK ? panel.URLs : nil);
	}];
}

#pragma mark Popped out

// A page in its own window can't show a dialog while the window is
// minimised or behind another app's, and isn't reloaded after a crash while
// the window is minimised or Bench is hidden: it does both when the window
// is back.
- (void)windowDidDeminiaturize:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingDialog];
	[self reloadAfterCrash];
}

// Bench being unhidden puts the window back on screen without making it
// key.
- (void)windowDidChangeOcclusionState:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingDialog];
	[self reloadAfterCrash];
}

- (void)windowDidBecomeKey:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingDialog];
	[self reloadAfterCrash];
}

- (void)windowWillClose:(NSNotification *)notification {
	if (notification.object != self.popOutWindow || self.closingPopOut) {
		return;
	}
	// The user closed the window: keep the web view, hidden in Bench's
	// window, and tell Go.
	[self returnFromPopOut];
	benchEmitItem(BENCH_EV_WEB_VIEW_CLOSED, self.itemID, nil, nil);
}

@end

#pragma mark - Entry points

static NSMutableDictionary<NSString *, BenchWebItem *> *gWebItems = nil;

static NSMutableDictionary<NSString *, BenchWebItem *> *benchWebItems(void) {
	if (gWebItems == nil) {
		gWebItems = [NSMutableDictionary dictionary];
	}
	return gWebItems;
}

// benchEventIsIn reports whether a mouse-up or key-down went to a web view:
// a click inside it, or a key while it, or a view inside it, has the
// keyboard focus.
static BOOL benchEventIsIn(NSEvent *event, WKWebView *_Nullable webView) {
	if (webView == nil || webView.window != event.window || webView.hiddenOrHasHiddenAncestor) {
		return NO;
	}
	if (event.type == NSEventTypeKeyDown) {
		NSResponder *responder = event.window.firstResponder;
		return [responder isKindOfClass:[NSView class]] && [(NSView *)responder isDescendantOf:webView];
	}
	NSPoint point = [webView convertPoint:event.locationInWindow fromView:nil];
	return NSPointInRect(point, webView.bounds);
}

// benchNoteUserEvent records when the user last released a mouse button or
// pressed a key in each Web item's web view and popup, which is how Bench
// tells a link the user clicked from one a page's script clicked.
static void benchNoteUserEvent(NSEvent *event) {
	if (event.window == nil) {
		return;
	}
	CFAbsoluteTime now = CFAbsoluteTimeGetCurrent();
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		if (benchEventIsIn(event, item.webView)) {
			item.lastUserEvent = now;
			return;
		}
		for (BenchPopup *popup in item.popups) {
			if (benchEventIsIn(event, popup.webView)) {
				popup.lastUserEvent = now;
				return;
			}
		}
	}
}

// benchWatchUserEvents starts recording the user's events in web views (see
// benchNoteUserEvent), once. Main thread only.
static void benchWatchUserEvents(void) {
	static id monitor = nil;
	if (monitor != nil) {
		return;
	}
	NSEventMask mask = NSEventMaskLeftMouseUp | NSEventMaskRightMouseUp | NSEventMaskOtherMouseUp | NSEventMaskKeyDown;
	monitor = [NSEvent addLocalMonitorForEventsMatchingMask:mask handler:^NSEvent *(NSEvent *event) {
		benchNoteUserEvent(event);
		return event;
	}];
}

WKWebView *benchWebViewForItem(NSString *itemID) {
	return benchWebItems()[itemID].webView;
}

NSString *benchWebItemHost(NSString *itemID) {
	NSString *address = benchWebItems()[itemID].page.address;
	return address.length > 0 ? [NSURL URLWithString:address].host : nil;
}

NSString *benchWebItemAtPoint(NSWindow *window, NSPoint locationInWindow) {
	NSView *container = benchWebContainerView();
	if (container == nil || container.window != window || container.hiddenOrHasHiddenAncestor) {
		return nil;
	}
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		WKWebView *web = item.webView;
		if (item.popOutWindow != nil || web.superview != container || web.hidden) {
			continue;
		}
		NSPoint point = [web convertPoint:locationInWindow fromView:nil];
		if (NSPointInRect(point, web.bounds)) {
			return item.itemID;
		}
	}
	return nil;
}

void benchWebShowWaitingDialogs(void) {
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		[item showWaitingDialog];
	}
}

void benchWebAttachContainer(NSView *container) {
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		if (item.popOutWindow != nil) {
			continue;
		}
		[item.webView removeFromSuperview];
		item.webView.autoresizingMask = NSViewNotSizable;
		[container addSubview:item.webView];
		item.webView.frame = item.frame;
		item.webView.hidden = !item.visible;
		[item reloadAfterCrash];
	}
}

void benchWebCloseAll(void) {
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		[item tearDown];
	}
	[benchWebItems() removeAllObjects];
}

void benchWebShow(const char *itemID, const char *url, const char *profileUUID, const char *userAgentSuffix, double x, double y, double w, double h) {
	NSString *identifier = benchString(itemID);
	NSString *address = benchString(url);
	NSString *profile = benchString(profileUUID);
	NSString *suffix = benchString(userAgentSuffix);
	NSRect frame = NSMakeRect(x, y, w, h);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		NSMutableDictionary<NSString *, BenchWebItem *> *items = benchWebItems();
		BenchWebItem *item = items[identifier];
		if (item != nil && ![item.profileUUID isEqualToString:profile]) {
			// A new profile needs a new data store, so a new web view.
			[item tearDown];
			[items removeObjectForKey:identifier];
			item = nil;
		}
		if (item == nil) {
			item = [[BenchWebItem alloc] initWithID:identifier profileUUID:profile userAgentSuffix:suffix];
			items[identifier] = item;
			[item loadAddress:address];
		} else if (![item.requestedURL isEqualToString:address] && ![item.page.address isEqualToString:address]) {
			// A new address loads, but not the page's own address (as a WebURL
			// event reported it), which would reload the page every time it's
			// shown.
			[item loadAddress:address];
		}
		item.frame = frame;
		item.visible = YES;
		if (item.popOutWindow != nil) {
			return; // It stays in its own window until docked.
		}
		NSView *container = benchWebContainerView();
		if (container == nil) {
			return; // Shown once Bench's window is attached.
		}
		WKWebView *web = item.webView;
		if (web.superview != container) {
			[web removeFromSuperview];
			[container addSubview:web];
		}
		web.autoresizingMask = NSViewNotSizable;
		web.frame = frame;
		web.hidden = NO;
		benchFocusShownView(web);
		[item showWaitingDialog];
		[item reloadAfterCrash];
	});
}

void benchWebHide(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		item.visible = NO;
		if (item.popOutWindow == nil) {
			// Hiding a view with the keyboard focus passes it to an arbitrary
			// next view: give it to the UI instead.
			benchYieldFocusFrom(item.webView);
			item.webView.hidden = YES;
		}
	});
}

void benchWebClose(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		[item tearDown];
		[benchWebItems() removeObjectForKey:identifier];
	});
}

void benchWebFocus(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		NSWindow *window = item.popOutWindow;
		if (window != nil) {
			benchActivateBench();
			[window makeKeyAndOrderFront:nil];
			[window makeFirstResponder:item.webView];
			[item showWaitingDialog];
			return;
		}
		benchFocusView(item.webView);
	});
}

void benchWebPopOut(const char *itemID, const char *title) {
	NSString *identifier = benchString(itemID);
	NSString *name = benchString(title);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		if (item.popOutWindow != nil) {
			[item.popOutWindow makeKeyAndOrderFront:nil];
			[item showWaitingDialog];
			return;
		}
		NSSize size = item.frame.size;
		if (size.width < 400 || size.height < 300) {
			size = NSMakeSize(1000, 700);
		}
		NSWindowStyleMask style = NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable;
		NSWindow *window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, size.width, size.height) styleMask:style backing:NSBackingStoreBuffered defer:NO];
		window.releasedWhenClosed = NO;
		NSString *pageTitle = item.webView.title;
		window.title = name.length > 0 ? name : (pageTitle ?: @"");
		// The window has no address bar: its subtitle says which site it
		// shows, as a popup's does.
		window.subtitle = [item.page subtitle];
		window.delegate = item;

		WKWebView *web = item.webView;
		benchYieldFocusFrom(web);
		[web removeFromSuperview];
		web.frame = window.contentView.bounds;
		web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		web.hidden = NO;
		[window.contentView addSubview:web];
		item.popOutWindow = window;
		item.visible = NO;

		[window center];
		[window makeKeyAndOrderFront:nil];
		[window makeFirstResponder:web];
		[NSApp activate];
		[item showWaitingDialog];
		[item reloadAfterCrash];
	});
}

void benchWebDock(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		NSWindow *window = item.popOutWindow;
		if (window == nil) {
			return;
		}
		// returnFromPopOut clears the window's delegate first, so closing it
		// here doesn't report WebViewClosed.
		item.closingPopOut = YES;
		[item returnFromPopOut];
		[window close];
		item.closingPopOut = NO;
	});
}
