//go:build darwin && cgo && !server

// web_darwin.m: Web items' web views. Each is a WKWebView in Bench's window,
// above the UI's own web view, with a persistent data store per profile, a
// navigation policy that keeps the item on its site, popups for sign-in,
// downloads, a bridge for the page's notifications, the page in view,
// whose host Bench's UI shows, and the page's controls: reload, back and
// forward, zoom, find, and muting the camera and microphone, whose state
// Go hears about, and the page's icon, which Go fetches. Web views look as
// macOS does, whatever the appearance of Bench's windows, so that pages get
// the prefers-color-scheme they get in Safari.
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

// The script that reads a page's <link> elements, for its icon, once the
// page has loaded: at most 64 of them, as JSON, with their addresses
// resolved against the page's. It runs in a world of Bench's own, apart
// from the page's scripts (WKContentWorld.defaultClientWorld).
static NSString *const BenchIconLinksScript =
	@"JSON.stringify(Array.from(document.querySelectorAll('link[rel][href]'), (l) => ({\n"
	@"  href: String(l.href), rel: String(l.rel), sizes: l.getAttribute('sizes') || '', type: String(l.type || ''),\n"
	@"})).slice(0, 64))";

static void *BenchKVOContext = &BenchKVOContext;

// benchStateKeyPaths returns the web view's properties that make up its
// page's state, which Go hears about (see reportState): whether it can go
// back or forward, and how the page uses the camera and microphone. Each is
// key-value observable.
static NSArray<NSString *> *benchStateKeyPaths(void) {
	static NSArray<NSString *> *paths = nil;
	if (paths == nil) {
		paths = @[ @"canGoBack", @"canGoForward", @"cameraCaptureState", @"microphoneCaptureState" ];
	}
	return paths;
}

// benchCaptureKeyPaths returns the web view's properties that say how its
// page uses the camera and microphone: those of benchStateKeyPaths that a
// popup's web view adds to its item's.
static NSArray<NSString *> *benchCaptureKeyPaths(void) {
	static NSArray<NSString *> *paths = nil;
	if (paths == nil) {
		paths = @[ @"cameraCaptureState", @"microphoneCaptureState" ];
	}
	return paths;
}

// benchCombinedCapture returns how an item's pages, its own and its
// popups', use a device, from how two of them do: live if either is,
// else muted if either is.
static WKMediaCaptureState benchCombinedCapture(WKMediaCaptureState a, WKMediaCaptureState b) {
	if (a == WKMediaCaptureStateActive || b == WKMediaCaptureStateActive) {
		return WKMediaCaptureStateActive;
	}
	if (a == WKMediaCaptureStateMuted || b == WKMediaCaptureStateMuted) {
		return WKMediaCaptureStateMuted;
	}
	return WKMediaCaptureStateNone;
}

// A page whose process crashes again within this many seconds of Bench
// reloading it after the last crash is reloaded only after a delay, which
// starts at BenchCrashDelay and doubles with each crash, up to
// BenchCrashMaxDelay, so that a page that crashes every time it loads
// doesn't reload without end. The window is measured from the reload, not
// the crash, so that waiting the longest delay doesn't start it all again.
static const CFAbsoluteTime BenchCrashWindow = 60;
static const NSTimeInterval BenchCrashDelay = 2;
static const NSTimeInterval BenchCrashMaxDelay = 60;

// A page kept running in the background whose first load fails for want
// of the network, as when Bench opens at login before Wi-Fi is up, tries
// again after BenchLoadRetryDelay, and after twice as long with each
// failure, up to BenchLoadRetryMaxDelay (see retryLoadSoon).
static const NSTimeInterval BenchLoadRetryDelay = 15;
static const NSTimeInterval BenchLoadRetryMaxDelay = 300;

// For this many seconds after a prompt to open another app's link, or to
// let a page use the camera or microphone, appears, its Open or Allow
// button can't be pressed, so that a key the user was already pressing for
// something else can't press it.
static const NSTimeInterval BenchPromptDelay = 1;

@class BenchWebItem;

static void benchWatchUserEvents(void);
static void benchWatchAppearance(void);

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
// How many pages have come into view, which tells what a page asked for
// from what the page before it did.
@property (nonatomic, readonly) NSUInteger commits;
- (void)loadRequested:(WKNavigation *_Nullable)navigation webView:(WKWebView *)webView;
- (BOOL)isRequestedLoad:(NSString *)address;
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
// Who's asking (see dialogHeading in Go), such as "app.slack.com says".
@property (nonatomic, copy) NSString *heading;
@property (nonatomic) BOOL confirm;
// Called once, with whether the user chose OK.
@property (nonatomic, copy) void (^done)(BOOL accepted);
@end

@implementation BenchDialog
@end

// BenchMediaRequest is a page's request for the camera or microphone,
// waiting to be shown.
@interface BenchMediaRequest : NSObject
// The web view whose page asked, the item's own or a popup's, its page in
// view, and how many pages had come into view in it then (see isCurrent).
@property (nonatomic, weak) WKWebView *webView;
@property (nonatomic, strong) BenchPageState *page;
@property (nonatomic) NSUInteger commits;
// What the prompt says (see mediaPrompt in Go).
@property (nonatomic, copy) NSString *question;
@property (nonatomic, copy) NSString *detail;
// Called once, with whether the user allowed it.
@property (nonatomic, copy) void (^done)(BOOL allowed);
- (BOOL)isCurrent;
@end

@implementation BenchMediaRequest

// isCurrent reports whether the page that asked is still the one in view,
// in a web view that's still open: what the user allows is for that page,
// not one that has replaced it, nor a closed popup's or item's.
- (BOOL)isCurrent {
	WKWebView *webView = self.webView;
	return webView != nil && webView.UIDelegate != nil && self.page.commits == self.commits;
}

@end

// BenchWebItem is a Web item's web view and everything around it.
@interface BenchWebItem : NSObject <WKNavigationDelegate, WKUIDelegate, NSWindowDelegate>
@property (nonatomic, copy) NSString *itemID;
@property (nonatomic, copy) NSString *profileUUID;
// The item's name, which the prompts that its pages bring up use.
@property (nonatomic, copy) NSString *name;
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
// The page's dialog that waits for the page to be seen, and whether one is
// showing; and the same for a request for the camera or microphone from
// the item's page or a popup's. Go decides which may come (see promptGate).
@property (nonatomic, strong, nullable) BenchDialog *waitingDialog;
@property (nonatomic) BOOL showingDialog;
@property (nonatomic, strong, nullable) BenchMediaRequest *waitingMedia;
@property (nonatomic) BOOL showingMedia;
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
// Whether the page keeps running while it's hidden, as Go says
// (benchWebKeepRunning): a page kept running in the background (decision
// 76), which is loaded again after its process ends without waiting to be
// seen.
@property (nonatomic) BOOL keepRunning;
// Whether the item's own address is still to load, as its first load
// failed for want of the network before any page came into view; whether
// a try again is waiting; and how many have failed (see retryLoadSoon).
// benchWebShow loads it again.
@property (nonatomic) BOOL needsLoad;
@property (nonatomic) BOOL loadRetryScheduled;
@property (nonatomic) NSUInteger loadFailures;
// When the user last released a mouse button or pressed a key in its web
// view, or 0: what tells a link the user clicked from one a script clicked.
@property (nonatomic) CFAbsoluteTime lastUserEvent;
// The number Go knows each of its web view's navigations by (see navigator
// in Go), and the last one given out. WebKit keeps a navigation's
// WKNavigation until it ends, and numbers are never used again.
@property (nonatomic, strong) NSMapTable<WKNavigation *, NSNumber *> *navigationIDs;
@property (nonatomic) uint64_t lastNavigationID;
// Whether a prompt to open another app's link is waiting or showing, and
// whether the user stopped the page's prompts (until it goes to another
// page).
@property (nonatomic) BOOL askingToOpen;
@property (nonatomic) BOOL opensStopped;
- (instancetype)initWithID:(NSString *)itemID profileUUID:(NSString *)profileUUID userAgentSuffix:(NSString *)suffix;
- (void)loadAddress:(NSString *)address;
- (void)returnFromPopOut;
- (void)tearDown;
- (void)showWaitingPrompts;
- (void)reloadAfterCrash;
- (void)retryLoadSoon;
- (void)reloadPage;
- (void)pauseMedia;
- (void)reportState;
- (WKWebView *)popupWithConfiguration:(WKWebViewConfiguration *)configuration action:(WKNavigationAction *)action features:(WKWindowFeatures *)features webView:(WKWebView *)webView;
- (void)askToOpen:(NSURL *_Nullable)url source:(NSString *_Nullable)source webView:(WKWebView *)webView;
- (void)askForMedia:(WKMediaCaptureType)type frame:(WKFrameInfo *)frame webView:(WKWebView *)webView page:(BenchPageState *)page popup:(BOOL)popup decisionHandler:(void (^)(WKPermissionDecision))decisionHandler;
- (void)refuseStaleMedia;
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

// benchNavKind returns the kind of a navigation, for Go (see navKind).
static int benchNavKind(WKNavigationType type) {
	switch (type) {
	case WKNavigationTypeLinkActivated:
		return BENCH_NAV_KIND_LINK;
	case WKNavigationTypeBackForward:
	case WKNavigationTypeReload:
	case WKNavigationTypeFormResubmitted:
		return BENCH_NAV_KIND_HISTORY;
	default:
		return BENCH_NAV_KIND_OTHER;
	}
}

// benchActionNavigation returns the navigation that WebKit asks about a
// navigation action of the main frame for: a new one, or for a redirect,
// the one under way, which tells Bench's own load's redirects from the
// page's navigations (see navigator in Go). WebKit says only through a
// private property, _mainFrameNavigation (macOS 10.14.4 and later), so
// this returns nil if that's gone, and Go decides without it.
static WKNavigation *_Nullable benchActionNavigation(WKNavigationAction *action) {
	if (![action respondsToSelector:NSSelectorFromString(@"_mainFrameNavigation")]) {
		return nil;
	}
	id navigation = [action valueForKey:@"_mainFrameNavigation"];
	return [navigation isKindOfClass:[WKNavigation class]] ? navigation : nil;
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

// benchDialogMessage makes a page's alert or confirmation text fit to show
// (see dialogText in Go). It reads no more than BENCH_DIALOG_READ UTF-16
// code units of it, and no half of a character: the page chooses how long
// it is.
static NSString *benchDialogMessage(NSString *_Nullable message) {
	NSString *text = message ?: @"";
	if (text.length > BENCH_DIALOG_READ) {
		NSUInteger end = BENCH_DIALOG_READ;
		if (CFStringIsSurrogateHighCharacter([text characterAtIndex:end - 1])) {
			end -= 1;
		}
		text = [text substringToIndex:end];
	}
	// A page's text can hold half a character on its own, which UTF8String
	// can't convert: this conversion replaces it.
	NSData *utf8 = [text dataUsingEncoding:NSUTF8StringEncoding allowLossyConversion:YES];
	char *shown = benchDialogText((char *)utf8.bytes, (int)utf8.length);
	NSString *result = shown != NULL ? benchString(shown) : @"";
	free(shown);
	return result;
}

// benchDialogHeadingFor returns the heading of an alert or confirmation that
// frame brings up in the page at pageAddress, which says who's asking (see
// dialogHeading in Go), or nil for a frame whose dialogs aren't shown.
static NSString *_Nullable benchDialogHeadingFor(NSString *pageAddress, WKFrameInfo *frame) {
	NSString *origin = benchOriginString(frame.securityOrigin);
	char *text = benchDialogHeading((char *)benchUTF8(pageAddress), (char *)benchUTF8(origin), frame.isMainFrame ? 1 : 0);
	if (text == NULL) {
		return nil;
	}
	NSString *heading = benchString(text);
	free(text);
	return heading;
}

// benchMediaKind returns what a request for the camera or microphone asks
// to use, for Go (see mediaKind), or -1 for anything else, which Go refuses.
static int benchMediaKind(WKMediaCaptureType type) {
	switch (type) {
	case WKMediaCaptureTypeCamera:
		return BENCH_MEDIA_CAMERA;
	case WKMediaCaptureTypeMicrophone:
		return BENCH_MEDIA_MICROPHONE;
	case WKMediaCaptureTypeCameraAndMicrophone:
		return BENCH_MEDIA_CAMERA_AND_MICROPHONE;
	default:
		return -1;
	}
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
@property (nonatomic, readwrite) NSUInteger commits;
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

// isRequestedLoad reports whether a navigation to address that the web view
// asks about is a load it was asked for, which hasn't started: WebKit
// writes the address as WebKit's URL did when it was asked.
- (BOOL)isRequestedLoad:(NSString *)address {
	return self.requestedNavigation != nil && [address isEqualToString:self.requestedAddress];
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
	self.commits += 1;
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
		_webView.appearance = NSApp.effectiveAppearance;
		_window.contentView = _webView;
		_page = [[BenchPageState alloc] init];
		[_webView addObserver:self forKeyPath:@"title" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		[_webView addObserver:self forKeyPath:@"hasOnlySecureContent" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		// A call can run in a popup, as a meeting opened with window.open
		// does: its camera and microphone count as its item's.
		for (NSString *keyPath in benchCaptureKeyPaths()) {
			[_webView addObserver:self forKeyPath:keyPath options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		}
		_observing = YES;
	}
	return self;
}

// stopObserving stops watching the popup's web view, once.
- (void)stopObserving {
	if (!_observing) {
		return;
	}
	[_webView removeObserver:self forKeyPath:@"title" context:BenchKVOContext];
	[_webView removeObserver:self forKeyPath:@"hasOnlySecureContent" context:BenchKVOContext];
	for (NSString *keyPath in benchCaptureKeyPaths()) {
		[_webView removeObserver:self forKeyPath:keyPath context:BenchKVOContext];
	}
	_observing = NO;
}

- (void)dealloc {
	[self stopObserving];
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
	if ([benchCaptureKeyPaths() containsObject:keyPath]) {
		[self.owner reportState];
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
	[self stopObserving];
	self.webView.UIDelegate = nil;
	self.webView.navigationDelegate = nil;
	[self.webView stopLoading];
	self.window.delegate = nil;
	// A request for the camera or microphone that its page left waiting is
	// refused.
	[self.owner refuseStaleMedia];
	// Let go of the popup (and its window) once AppKit has finished closing
	// the window, not in the middle of it. A call in it has ended.
	BenchPopup *popup = self;
	dispatch_async(dispatch_get_main_queue(), ^{
		BenchWebItem *owner = popup.owner;
		[owner.popups removeObject:popup];
		[owner reportState];
	});
}

// A request for the camera or microphone that the popup's page left
// waiting while the popup couldn't be seen shows when it's back.
- (void)windowDidBecomeKey:(NSNotification *)notification {
	(void)notification;
	[self.owner showWaitingPrompts];
}

- (void)webViewDidClose:(WKWebView *)webView {
	(void)webView;
	[self.window close];
}

// The popup's page asks for the camera or microphone as the item's does
// (see askForMedia), one request at a time for the item and its popups.
- (void)webView:(WKWebView *)webView requestMediaCapturePermissionForOrigin:(WKSecurityOrigin *)origin initiatedByFrame:(WKFrameInfo *)frame type:(WKMediaCaptureType)type decisionHandler:(void (^)(WKPermissionDecision))decisionHandler {
	(void)origin;
	BenchWebItem *owner = self.owner;
	if (owner == nil) {
		decisionHandler(WKPermissionDecisionDeny);
		return;
	}
	[owner askForMedia:type frame:frame webView:webView page:self.page popup:YES decisionHandler:decisionHandler];
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
	// An address that opens the browser or another app needs the popup's
	// user event, or the click that opened it, and each allows one (see
	// webOpenLimit in Go).
	int decision = benchDecidePopupNavigation((char *)benchUTF8(owner.itemID), (char *)benchUTF8(itemURL), (char *)benchUTF8(self.startURL), (char *)benchUTF8(url.absoluteString),
	                                          clicked ? 1 : 0, self.lastUserEvent, CFAbsoluteTimeGetCurrent());
	if (decision == BENCH_NAV_IN_VIEW) {
		NSURL *start = [NSURL URLWithString:self.startURL];
		if (!benchIsWebURL(start) && benchIsWebURL(url)) {
			self.startURL = url.absoluteString ?: @"";
		}
		decisionHandler(WKNavigationActionPolicyAllow);
		return;
	}
	benchApplyNavigation(decision, url, benchSourceHost(navigationAction, webView), owner, webView, decisionHandler);
	if (!self.committed || ((decision == BENCH_NAV_IN_BROWSER || decision == BENCH_NAV_ASK) && !clicked)) {
		// It was opened only to go there, as a redirector's popup is (some
		// show a page of their own first, which then sends itself on), or
		// it has shown nothing and isn't going anywhere: there's nothing to
		// leave on screen. A link the user clicked in a page that stays keeps
		// its popup.
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
	// A request for the camera or microphone that the page before left
	// waiting is refused.
	[self.owner refuseStaleMedia];
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

// benchOffline reports whether a load failed for want of the network, or
// of a host it could reach, which a later try may not: not one that was
// cancelled, or that the navigation policy refused.
static BOOL benchOffline(NSError *error) {
	if (![error.domain isEqualToString:NSURLErrorDomain]) {
		return NO;
	}
	switch (error.code) {
	case NSURLErrorNotConnectedToInternet:
	case NSURLErrorNetworkConnectionLost:
	case NSURLErrorCannotFindHost:
	case NSURLErrorCannotConnectToHost:
	case NSURLErrorDNSLookupFailed:
	case NSURLErrorTimedOut:
	case NSURLErrorSecureConnectionFailed:
		return YES;
	default:
		return NO;
	}
}

@implementation BenchWebItem

- (instancetype)initWithID:(NSString *)itemID profileUUID:(NSString *)profileUUID userAgentSuffix:(NSString *)suffix {
	self = [super init];
	if (self) {
		_itemID = [itemID copy];
		_profileUUID = [profileUUID copy];
		_name = @"";
		_requestedURL = @"";
		_page = [[BenchPageState alloc] init];
		_popups = [NSMutableArray array];
		_navigationIDs = [NSMapTable weakToStrongObjectsMapTable];
		// A web view with this ID before, under another profile, was another
		// page.
		benchNavForget((char *)benchUTF8(_itemID));
		benchWatchUserEvents();
		benchWatchAppearance();

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
		// Not the appearance of Bench's window, which it would inherit (see
		// benchWatchAppearance).
		_webView.appearance = NSApp.effectiveAppearance;
		[_webView addObserver:self forKeyPath:@"title" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		[_webView addObserver:self forKeyPath:@"URL" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		[_webView addObserver:self forKeyPath:@"hasOnlySecureContent" options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		for (NSString *keyPath in benchStateKeyPaths()) {
			[_webView addObserver:self forKeyPath:keyPath options:NSKeyValueObservingOptionNew context:BenchKVOContext];
		}
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
		for (NSString *keyPath in benchStateKeyPaths()) {
			[_webView removeObserver:self forKeyPath:keyPath context:BenchKVOContext];
		}
	}
}

- (void)loadAddress:(NSString *)address {
	self.requestedURL = address;
	// Loading starts a new process anyway, and a new page starts counting
	// crashes afresh.
	self.needsReload = NO;
	self.needsLoad = NO;
	self.crashReloaded = 0;
	NSURL *url = [NSURL URLWithString:address];
	if (url == nil || url.scheme.length == 0) {
		url = [NSURL URLWithString:@"about:blank"];
	}
	WKNavigation *navigation = [self.webView loadRequest:[NSURLRequest requestWithURL:url]];
	benchNavLoadRequested((char *)benchUTF8(self.itemID), [self idOfNavigation:navigation]);
	[self.page loadRequested:navigation webView:self.webView];
}

// idOfNavigation returns the number Go knows a navigation by (see
// navigationIDs), or 0 for none.
- (uint64_t)idOfNavigation:(WKNavigation *_Nullable)navigation {
	if (navigation == nil) {
		return 0;
	}
	NSNumber *known = [self.navigationIDs objectForKey:navigation];
	if (known != nil) {
		return known.unsignedLongLongValue;
	}
	self.lastNavigationID += 1;
	[self.navigationIDs setObject:@(self.lastNavigationID) forKey:navigation];
	return self.lastNavigationID;
}

// returnHomeSoon loads the item's own address again, once WebKit has
// finished the call it's in: a sign-in page that the user clicked through to
// sent them on to another site (see navigator in Go).
- (void)returnHomeSoon {
	__weak BenchWebItem *weakItem = self;
	dispatch_async(dispatch_get_main_queue(), ^{
		BenchWebItem *item = weakItem;
		if (item != nil && item.webView.navigationDelegate == item) {
			[item loadAddress:item.requestedURL];
		}
	});
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
	} else if ([benchStateKeyPaths() containsObject:keyPath]) {
		[self reportState];
	}
}

// reportState tells Go what the page can do and is doing now: whether the
// web view can go back or forward, for the page's Back and Forward, and how
// the page and its popups use the camera and microphone, for the marks on
// its sub-tab and tab (see benchCombinedCapture). Each property is reported
// as it changes, with the others as they are, so Go may hear the same
// state twice. Nothing is reported once the item is torn down.
- (void)reportState {
	if (!self.observing) {
		return;
	}
	WKWebView *web = self.webView;
	WKMediaCaptureState camera = web.cameraCaptureState, microphone = web.microphoneCaptureState;
	for (BenchPopup *popup in self.popups) {
		camera = benchCombinedCapture(camera, popup.webView.cameraCaptureState);
		microphone = benchCombinedCapture(microphone, popup.webView.microphoneCaptureState);
	}
	benchEmitWebState((char *)benchUTF8(self.itemID), web.canGoBack ? 1 : 0, web.canGoForward ? 1 : 0, (int)camera, (int)microphone);
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
		for (NSString *keyPath in benchStateKeyPaths()) {
			[self.webView removeObserver:self forKeyPath:keyPath context:BenchKVOContext];
		}
		self.observing = NO;
	}
	[self.contentController removeScriptMessageHandlerForName:BenchNotifyHandler];
	[self.webView stopLoading];
	self.webView.navigationDelegate = nil;
	self.webView.UIDelegate = nil;
	benchNavForget((char *)benchUTF8(self.itemID));
	self.needsReload = NO;
	// WebKit needs every dialog's answer, even from a page that's going,
	// and every request's for the camera or microphone.
	BenchDialog *dialog = self.waitingDialog;
	self.waitingDialog = nil;
	if (dialog != nil) {
		dialog.done(NO);
	}
	BenchMediaRequest *request = self.waitingMedia;
	self.waitingMedia = nil;
	if (request != nil) {
		request.done(NO);
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
	// Go decides, by what came before too (see navigator): how the
	// navigation started, which navigation it is, whether a sign-in page
	// the user clicked through to is in view, and whether the item is going
	// back to its own address.
	NSURL *url = navigationAction.request.URL;
	NSString *address = url.absoluteString ?: @"";
	NSString *origin = benchOriginString(navigationAction.sourceFrame.securityOrigin);
	int returnHome = 0;
	uint64_t navigation = [self idOfNavigation:benchActionNavigation(navigationAction)];
	int decision = benchNavDecide((char *)benchUTF8(self.itemID), (char *)benchUTF8(self.requestedURL), navigation, (char *)benchUTF8(address),
	                              benchNavKind(navigationAction.navigationType), [self sourceOfNavigation:navigationAction address:address],
	                              (char *)benchUTF8(origin), self.lastUserEvent, CFAbsoluteTimeGetCurrent(), &returnHome);
	if (returnHome != 0) {
		[self returnHomeSoon];
	}
	benchApplyNavigation(decision, url, benchSourceHost(navigationAction, webView), self, webView, decisionHandler);
}

// sourceOfNavigation says what started a navigation of the item's main
// frame to address, for Go (see navSource): Bench's own load, the web view
// itself, the item's page, a frame inside it, or a frame in another web
// view, such as a popup's page navigating its opener.
- (int)sourceOfNavigation:(WKNavigationAction *)action address:(NSString *)address {
	if ([self.page isRequestedLoad:address]) {
		return BENCH_NAV_FROM_BENCH;
	}
	WKFrameInfo *source = action.sourceFrame;
	if (source == nil) {
		return BENCH_NAV_FROM_NONE;
	}
	if (source.webView != self.webView) {
		return BENCH_NAV_FROM_WINDOW;
	}
	return source.isMainFrame ? BENCH_NAV_FROM_PAGE : BENCH_NAV_FROM_FRAME;
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
	benchNavStarted((char *)benchUTF8(self.itemID), [self idOfNavigation:navigation]);
}

- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
	// A new page may ask to open other apps' links again. Whether it may
	// bring up dialogs, and ask for the camera or microphone, is up to Go
	// (see promptGate), which hears of the page below; a request that the
	// page before left waiting is refused.
	self.opensStopped = NO;
	self.loadFailures = 0;
	BOOL changed = [self.page navigationCommittedIn:webView];
	[self refuseStaleMedia];
	// Go follows what the page in view is (see navigator): the page's own
	// address, not WebKit's URL, which can still be a load Bench asked for.
	benchNavCommitted((char *)benchUTF8(self.itemID), (char *)benchUTF8(self.requestedURL), [self idOfNavigation:navigation],
	                  (char *)benchUTF8(self.page.address), CFAbsoluteTimeGetCurrent());
	if (changed) {
		[self reportPage];
	}
}

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
	(void)navigation;
	[self readIcon:webView];
}

// readIcon reads the <link> elements of the page that has just loaded, for
// Go to fetch the page's icon (benchPageIconLinks). Go fetches it only if
// the page names other icons than the last page did.
- (void)readIcon:(WKWebView *)webView {
	NSString *identifier = self.itemID;
	NSString *address = self.page.address ?: @"";
	[webView evaluateJavaScript:BenchIconLinksScript inFrame:nil inContentWorld:WKContentWorld.defaultClientWorld completionHandler:^(id result, NSError *error) {
		if (benchStopped()) {
			return;
		}
		NSString *links = error == nil && [result isKindOfClass:[NSString class]] ? (NSString *)result : @"[]";
		benchPageIconLinks((char *)benchUTF8(identifier), (char *)benchUTF8(address), (char *)benchUTF8(links));
	}];
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
	if ([self.page navigationFailed:navigation webView:webView]) {
		[self reportPage];
	}
	// A navigation that fails before its page shows, such as a click to a
	// sign-in page that doesn't load, leaves the page in view, and so what
	// that page is (see navigator in Go). One cancelled is most often one
	// that another navigation replaced, which was decided by it.
	BOOL replaced = [error.domain isEqualToString:NSURLErrorDomain] && error.code == NSURLErrorCancelled;
	if (benchNavFailed((char *)benchUTF8(self.itemID), [self idOfNavigation:navigation], replaced ? 1 : 0) != 0) {
		[self returnHomeSoon];
		return;
	}
	// The item's own address that fails to load for want of the network
	// before any page has come into view, as when Bench opens at login
	// before Wi-Fi is up, loads again when it's next shown, and a page kept
	// running in the background tries again by itself.
	if (self.page.address.length == 0 && benchOffline(error)) {
		self.needsLoad = YES;
		[self retryLoadSoon];
	}
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
	(void)webView;
	[self.page processEnded];
	benchNavProcessEnded((char *)benchUTF8(self.itemID));
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

// canRun reports whether the page needs a process now: it can be seen, or
// it keeps running in the background.
- (BOOL)canRun {
	return self.keepRunning || [self canBeSeen];
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
// page can be seen, or at once if it keeps running in the background, so
// that its notifications and its count go on: another hidden page doesn't
// need a process, and one that crashes over and over in a tab the user
// isn't looking at shouldn't keep starting new ones. The first crash in a
// while reloads at once, and each one after that waits longer (see
// BenchCrashWindow), counted from the crash, so that a page shown after
// being hidden for that long reloads at once.
- (void)reloadAfterCrash {
	if (!self.needsReload || self.reloadScheduled || ![self canRun]) {
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
		// Hidden meanwhile, and not kept running, it reloads when it's
		// shown again.
		if (!item.needsReload || ![item canRun]) {
			return;
		}
		item.needsReload = NO;
		item.crashReloaded = CFAbsoluteTimeGetCurrent();
		[item.webView reload];
	});
}

// retryLoadSoon loads the item's own address again, after a while, if its
// first load failed for want of the network (needsLoad) and the page keeps
// running in the background, so that a page loaded hidden before the
// network was up still runs. The wait starts at BenchLoadRetryDelay and
// doubles with each failure, up to BenchLoadRetryMaxDelay.
- (void)retryLoadSoon {
	if (!self.needsLoad || !self.keepRunning || self.loadRetryScheduled) {
		return;
	}
	NSUInteger doublings = MIN(self.loadFailures, (NSUInteger)8);
	NSTimeInterval delay = MIN(BenchLoadRetryMaxDelay, BenchLoadRetryDelay * (double)((NSUInteger)1 << doublings));
	self.loadFailures += 1;
	self.loadRetryScheduled = YES;
	__weak BenchWebItem *weakSelf = self;
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(delay * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
		BenchWebItem *item = weakSelf;
		if (item == nil) {
			return;
		}
		item.loadRetryScheduled = NO;
		// Loaded meanwhile, no longer kept running, or closed.
		if (benchStopped() || !item.needsLoad || !item.keepRunning || item.webView.navigationDelegate != item) {
			return;
		}
		[item loadAddress:item.requestedURL];
	});
}

// reloadPage loads the page in view again, as the user asked, or the item's
// own address if no page has come into view, as when its first load
// failed. A reload after a crash that's waiting isn't needed then.
- (void)reloadPage {
	self.needsReload = NO;
	if (self.page.address.length == 0 || self.webView.URL == nil) {
		[self loadAddress:self.requestedURL];
		return;
	}
	[self.webView reload];
}

// pauseMedia pauses every sound and video that the page plays, and its
// popups' pages: a call, or a video, that carries on out of sight.
- (void)pauseMedia {
	[self.webView pauseAllMediaPlaybackWithCompletionHandler:nil];
	for (BenchPopup *popup in self.popups) {
		[popup.webView pauseAllMediaPlaybackWithCompletionHandler:nil];
	}
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
	NSString *origin = benchOriginString(action.sourceFrame.securityOrigin);
	// WebKit opens a window only for the user's click or key press in the
	// page that asked, so that event allows it, and allows opening the
	// browser or another app once (see webOpenLimit in Go). A blank window
	// needs a page on the item's site in the item's own web view, not in a
	// popup.
	CFAbsoluteTime userEvent = [self lastUserEventIn:webView];
	int decision = benchDecidePopup((char *)benchUTF8(self.itemID), (char *)benchUTF8(self.requestedURL), (char *)benchUTF8(address), (char *)benchUTF8(origin),
	                                webView == self.webView ? 1 : 0, userEvent, CFAbsoluteTimeGetCurrent());
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
	// The popup starts with that event: a download it was opened for counts
	// as one the user asked for, and a redirector it was opened for may
	// send the user to the browser.
	popup.lastUserEvent = userEvent;
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
// seen (see showWaitingDialog), with its text made fit to show (see
// dialogText in Go). The page gets its answer at once, OK for an alert and
// Cancel for a confirmation, with nothing shown, while another dialog from
// it is waiting or showing, when the user stopped its dialogs or the item's
// dialogs are held back (see promptGate in Go), and for a frame inside the
// page with no host of its own (see dialogHeading in Go).
- (void)askWithMessage:(NSString *)message frame:(WKFrameInfo *)frame confirm:(BOOL)confirm done:(void (^)(BOOL accepted))done {
	if (self.waitingDialog != nil || self.showingDialog || !benchPromptAllowed((char *)benchUTF8(self.itemID), BENCH_PROMPT_DIALOG, CFAbsoluteTimeGetCurrent())) {
		done(NO);
		return;
	}
	NSString *heading = benchDialogHeadingFor(self.page.address, frame);
	if (heading == nil) {
		done(NO);
		return;
	}
	BenchDialog *dialog = [[BenchDialog alloc] init];
	dialog.message = benchDialogMessage(message);
	dialog.heading = heading;
	dialog.confirm = confirm;
	dialog.done = done;
	self.waitingDialog = dialog;
	[self showWaitingDialog];
}

// showWaitingDialog shows the page's waiting dialog, if the page can be seen.
// It's app-modal rather than a sheet: Bench's window sits below the docked
// windows, which would cover a sheet's buttons. It names the site asking,
// and offers to stop a page that asks again and again (see promptGate in
// Go).
- (void)showWaitingDialog {
	BenchDialog *dialog = self.waitingDialog;
	if (dialog == nil || self.showingDialog || ![self canShowDialog]) {
		return;
	}
	self.waitingDialog = nil;
	self.showingDialog = YES;
	BOOL offerStop = benchPromptOfferStop((char *)benchUTF8(self.itemID), CFAbsoluteTimeGetCurrent()) != 0;
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
		alert.messageText = dialog.heading;
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
		// An item closed meanwhile has no prompts to count.
		if (item.webView.navigationDelegate == item) {
			BOOL stop = offerStop && alert.suppressionButton.state == NSControlStateValueOn;
			benchPromptEnded((char *)benchUTF8(item.itemID), BENCH_PROMPT_DIALOG, stop ? 1 : 0, CFAbsoluteTimeGetCurrent());
		}
		dialog.done(response == NSAlertFirstButtonReturn);
	});
}

// askForMedia asks the user whether a page may use the camera or
// microphone, and calls decisionHandler once, with the answer. webView, the
// item's own or a popup's (popup), is where the page asked, and page its
// page in view. The request is refused at once while another from the item
// or its popups is waiting or showing, when the user stopped the item's
// pages' requests or they're held back (see promptGate in Go), for
// a frame inside the page that has no host or isn't on the item's site
// (see mediaPrompt in Go), and once the item is closed. Otherwise it
// waits, as a dialog does, until the web view can be seen with Bench
// active (see showWaitingMedia). Without this, WebKit would ask with a
// sheet on Bench's window, under the docked windows, at any time.
- (void)askForMedia:(WKMediaCaptureType)type frame:(WKFrameInfo *)frame webView:(WKWebView *)webView page:(BenchPageState *)page popup:(BOOL)popup decisionHandler:(void (^)(WKPermissionDecision))decisionHandler {
	if (self.webView.navigationDelegate != self || self.waitingMedia != nil || self.showingMedia || !benchPromptAllowed((char *)benchUTF8(self.itemID), BENCH_PROMPT_MEDIA, CFAbsoluteTimeGetCurrent())) {
		decisionHandler(WKPermissionDecisionDeny);
		return;
	}
	NSString *origin = benchOriginString(frame.securityOrigin);
	char *detail = NULL;
	char *question = benchMediaPrompt((char *)benchUTF8(self.requestedURL), (char *)benchUTF8(self.name), (char *)benchUTF8(page.address), (char *)benchUTF8(origin),
	                                  frame.isMainFrame ? 1 : 0, popup ? 1 : 0, benchMediaKind(type), &detail);
	NSString *questionText = question != NULL ? benchString(question) : nil;
	NSString *detailText = detail != NULL ? benchString(detail) : @"";
	free(question);
	free(detail);
	if (questionText == nil) {
		decisionHandler(WKPermissionDecisionDeny);
		return;
	}
	BenchMediaRequest *request = [[BenchMediaRequest alloc] init];
	request.webView = webView;
	request.page = page;
	request.commits = page.commits;
	request.question = questionText;
	request.detail = detailText;
	request.done = ^(BOOL allowed) {
		decisionHandler(allowed ? WKPermissionDecisionGrant : WKPermissionDecisionDeny);
	};
	self.waitingMedia = request;
	[self showWaitingMedia];
}

// showWaitingMedia shows the waiting request for the camera or microphone,
// if the web view that asked can be seen with Bench active, or refuses it
// if the page that asked has gone. It's app-modal, as a dialog is, and
// names the item and the site asking. Don't Allow is its default button,
// and Allow can't be pressed at first, so that a Return or a space the user
// was typing elsewhere can't allow it. It offers to stop the page's
// requests.
- (void)showWaitingMedia {
	BenchMediaRequest *request = self.waitingMedia;
	if (request == nil || self.showingMedia) {
		return;
	}
	if (![request isCurrent]) {
		self.waitingMedia = nil;
		request.done(NO);
		return;
	}
	if (!benchCanBeSeen(request.webView)) {
		return;
	}
	self.waitingMedia = nil;
	self.showingMedia = YES;
	BenchWebItem *item = self;
	benchRunModalSoon(^{
		// As for a dialog, another alert may have kept this one waiting, while
		// the page that asked went, or the user went to another tab or app.
		if (![request isCurrent]) {
			item.showingMedia = NO;
			request.done(NO);
			return;
		}
		if (!benchCanBeSeen(request.webView)) {
			item.showingMedia = NO;
			item.waitingMedia = request;
			return;
		}
		NSAlert *alert = [[NSAlert alloc] init];
		alert.messageText = request.question;
		alert.informativeText = request.detail;
		[alert addButtonWithTitle:@"Don't Allow"];
		NSButton *allowButton = [alert addButtonWithTitle:@"Allow"];
		allowButton.enabled = NO;
		alert.showsSuppressionButton = YES;
		alert.suppressionButton.title = @"Don't allow more requests from this page";
		NSTimer *timer = [NSTimer timerWithTimeInterval:BenchPromptDelay repeats:NO block:^(NSTimer *t) {
			(void)t;
			allowButton.enabled = YES;
		}];
		[[NSRunLoop mainRunLoop] addTimer:timer forMode:NSRunLoopCommonModes];
		NSModalResponse response = [alert runModal];
		[timer invalidate];
		item.showingMedia = NO;
		if (item.webView.navigationDelegate == item) {
			BOOL stop = alert.suppressionButton.state == NSControlStateValueOn;
			benchPromptEnded((char *)benchUTF8(item.itemID), BENCH_PROMPT_MEDIA, stop ? 1 : 0, CFAbsoluteTimeGetCurrent());
		}
		// What the user allowed is for the page that asked, if it's still
		// there.
		request.done(response == NSAlertSecondButtonReturn && [request isCurrent]);
	});
}

// refuseStaleMedia refuses the waiting request for the camera or
// microphone if the page that asked has gone (see BenchMediaRequest).
- (void)refuseStaleMedia {
	BenchMediaRequest *request = self.waitingMedia;
	if (request == nil || [request isCurrent]) {
		return;
	}
	self.waitingMedia = nil;
	request.done(NO);
}

// showWaitingPrompts shows what the item's pages left waiting for them to
// be seen: a dialog, and a request for the camera or microphone. Each waits
// for the other to close.
- (void)showWaitingPrompts {
	[self showWaitingDialog];
	[self showWaitingMedia];
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

// origin is the page's: the frame's own, which the request is for, is the
// one asked about.
- (void)webView:(WKWebView *)webView requestMediaCapturePermissionForOrigin:(WKSecurityOrigin *)origin initiatedByFrame:(WKFrameInfo *)frame type:(WKMediaCaptureType)type decisionHandler:(void (^)(WKPermissionDecision))decisionHandler {
	(void)origin;
	[self askForMedia:type frame:frame webView:webView page:self.page popup:NO decisionHandler:decisionHandler];
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

// A page in its own window can't show a dialog, or ask for the camera or
// microphone, while the window is minimised or behind another app's, and
// isn't reloaded after a crash while the window is minimised or Bench is
// hidden: it does both when the window is back.
- (void)windowDidDeminiaturize:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingPrompts];
	[self reloadAfterCrash];
}

// Bench being unhidden puts the window back on screen without making it
// key.
- (void)windowDidChangeOcclusionState:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingPrompts];
	[self reloadAfterCrash];
}

- (void)windowDidBecomeKey:(NSNotification *)notification {
	if (notification.object != self.popOutWindow) {
		return;
	}
	[self showWaitingPrompts];
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

// BenchAppearanceWatcher follows the app's effective appearance, which is
// macOS's own, as Bench never sets NSApp's appearance, only its windows'.
@interface BenchAppearanceWatcher : NSObject
@end

static void *BenchAppearanceContext = &BenchAppearanceContext;

// benchFollowAppearance gives every Web item's web view, and its popups',
// the app's effective appearance: a web view would otherwise inherit the
// appearance of the window it's in, which is Bench's appearance setting
// for Bench's own window. Main thread only.
static void benchFollowAppearance(void) {
	NSAppearance *appearance = NSApp.effectiveAppearance;
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		item.webView.appearance = appearance;
		for (BenchPopup *popup in item.popups) {
			popup.webView.appearance = appearance;
		}
	}
}

@implementation BenchAppearanceWatcher

- (void)observeValueForKeyPath:(NSString *)keyPath ofObject:(id)object change:(NSDictionary<NSKeyValueChangeKey, id> *)change context:(void *)context {
	if (context != BenchAppearanceContext) {
		[super observeValueForKeyPath:keyPath ofObject:object change:change context:context];
		return;
	}
	benchOnMain(^{
		benchFollowAppearance();
	});
}

@end

// benchWatchAppearance starts following the user switching macOS between
// light and dark, or macOS switching itself (see benchFollowAppearance),
// once. Main thread only.
static void benchWatchAppearance(void) {
	static BenchAppearanceWatcher *watcher = nil;
	if (watcher != nil) {
		return;
	}
	watcher = [[BenchAppearanceWatcher alloc] init];
	[NSApp addObserver:watcher forKeyPath:@"effectiveAppearance" options:NSKeyValueObservingOptionNew context:BenchAppearanceContext];
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

// benchPageZoom returns a page zoom that Go asked for as WKWebView's
// pageZoom: 0, for none, is actual size.
static CGFloat benchPageZoom(double zoom) {
	return zoom > 0 ? (CGFloat)zoom : 1;
}

// benchCaptureState returns the WKMediaCaptureState of a BENCH_CAPTURE_
// state.
static WKMediaCaptureState benchCaptureState(int state) {
	switch (state) {
	case BENCH_CAPTURE_ACTIVE:
		return WKMediaCaptureStateActive;
	case BENCH_CAPTURE_MUTED:
		return WKMediaCaptureStateMuted;
	default:
		return WKMediaCaptureStateNone;
	}
}

void benchWebShowWaitingDialogs(void) {
	for (BenchWebItem *item in [benchWebItems() allValues]) {
		[item showWaitingPrompts];
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

void benchWebShow(const char *itemID, const char *name, const char *url, const char *profileUUID, const char *userAgentSuffix, double zoom, double x, double y, double w, double h) {
	NSString *identifier = benchString(itemID);
	NSString *itemName = benchString(name);
	NSString *address = benchString(url);
	NSString *profile = benchString(profileUUID);
	NSString *suffix = benchString(userAgentSuffix);
	CGFloat scale = benchPageZoom(zoom);
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
		} else if (item.needsLoad || (![item.requestedURL isEqualToString:address] && ![item.page.address isEqualToString:address])) {
			// A new address loads, but not the page's own address (as a WebURL
			// event reported it), which would reload the page every time it's
			// shown; and so does one whose first load failed for want of the
			// network, as one loaded hidden before Wi-Fi was up.
			[item loadAddress:address];
		}
		item.name = itemName;
		item.frame = frame;
		item.visible = YES;
		if (item.webView.pageZoom != scale) {
			item.webView.pageZoom = scale;
		}
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
		[item showWaitingPrompts];
		[item reloadAfterCrash];
	});
}

void benchWebLoad(const char *itemID, const char *name, const char *url, const char *profileUUID, const char *userAgentSuffix, double zoom, double x, double y, double w, double h) {
	NSString *identifier = benchString(itemID);
	NSString *itemName = benchString(name);
	NSString *address = benchString(url);
	NSString *profile = benchString(profileUUID);
	NSString *suffix = benchString(userAgentSuffix);
	CGFloat scale = benchPageZoom(zoom);
	NSRect frame = NSMakeRect(x, y, w, h);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		NSMutableDictionary<NSString *, BenchWebItem *> *items = benchWebItems();
		if (items[identifier] != nil) {
			return; // Made already, by benchWebShow or an earlier load.
		}
		BenchWebItem *item = [[BenchWebItem alloc] initWithID:identifier profileUUID:profile userAgentSuffix:suffix];
		items[identifier] = item;
		item.name = itemName;
		item.frame = frame;
		item.visible = NO;
		item.keepRunning = YES;
		item.webView.pageZoom = scale;
		[item loadAddress:address];
		NSView *container = benchWebContainerView();
		if (container == nil) {
			return; // It loads outside a window; benchWebShow adds it.
		}
		// Hidden, in Bench's window, as a view that benchWebHide hid: its
		// page runs, and benchWebShow shows it where it belongs.
		WKWebView *web = item.webView;
		web.hidden = YES;
		web.autoresizingMask = NSViewNotSizable;
		web.frame = frame;
		[container addSubview:web];
	});
}

void benchWebKeepRunning(const char *itemID, int keep) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		BenchWebItem *item = benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		item.keepRunning = keep != 0;
		// Kept running now, a hidden page whose process ended, or whose
		// first load failed, loads again.
		[item reloadAfterCrash];
		[item retryLoadSoon];
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
			[item showWaitingPrompts];
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
			[item showWaitingPrompts];
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
		// Marked as the item's, for Go to hear when it has the keyboard and
		// when the user drags it onto a pane, with a button in its title bar
		// that puts the page back.
		benchMarkPopOut(window, identifier);
		benchAddBackToBench(window, identifier);

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
		[item showWaitingPrompts];
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

void benchWebReload(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		[benchWebItems()[identifier] reloadPage];
	});
}

void benchWebNavigate(const char *itemID, int step) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		BenchWebItem *item = benchStopped() ? nil : benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		// Each does nothing where there's no page to go to. Go decides
		// whether the page may go there, as it does for a swipe (see
		// navigator in Go).
		if (step < 0) {
			[item.webView goBack];
		} else if (step > 0) {
			[item.webView goForward];
		}
	});
}

void benchWebZoom(const char *itemID, double zoom) {
	NSString *identifier = benchString(itemID);
	CGFloat scale = benchPageZoom(zoom);
	benchOnMain(^{
		BenchWebItem *item = benchWebItems()[identifier];
		if (item != nil && item.webView.pageZoom != scale) {
			item.webView.pageZoom = scale;
		}
	});
}

void benchWebFind(const char *itemID, const char *text, int backwards, uint64_t request) {
	NSString *identifier = benchString(itemID);
	NSString *string = benchString(text);
	BOOL back = backwards != 0;
	benchOnMain(^{
		BenchWebItem *item = benchStopped() ? nil : benchWebItems()[identifier];
		if (item == nil || string.length == 0) {
			benchWebFound(request, 0);
			return;
		}
		WKFindConfiguration *configuration = [[WKFindConfiguration alloc] init];
		configuration.backwards = back;
		configuration.caseSensitive = NO;
		configuration.wraps = YES;
		// WebKit selects the match and scrolls to it, and calls back on the
		// main thread, once.
		[item.webView findString:string withConfiguration:configuration completionHandler:^(WKFindResult *result) {
			benchWebFound(request, result.matchFound ? 1 : 0);
		}];
	});
}

void benchWebSetCapture(const char *itemID, int device, int state) {
	NSString *identifier = benchString(itemID);
	WKMediaCaptureState capture = benchCaptureState(state);
	benchOnMain(^{
		BenchWebItem *item = benchStopped() ? nil : benchWebItems()[identifier];
		if (item == nil) {
			return;
		}
		// The item's page and its popups' pages, as Stop sound and video
		// does. A device a page isn't using stays unused: Bench never
		// starts one. KVO reports the change (see reportState).
		NSMutableArray<WKWebView *> *webViews = [NSMutableArray arrayWithObject:item.webView];
		for (BenchPopup *popup in item.popups) {
			[webViews addObject:popup.webView];
		}
		for (WKWebView *web in webViews) {
			if (device == BENCH_MEDIA_CAMERA && web.cameraCaptureState != WKMediaCaptureStateNone) {
				[web setCameraCaptureState:capture completionHandler:nil];
			} else if (device == BENCH_MEDIA_MICROPHONE && web.microphoneCaptureState != WKMediaCaptureStateNone) {
				[web setMicrophoneCaptureState:capture completionHandler:nil];
			}
		}
	});
}

void benchWebPauseMedia(const char *itemID) {
	NSString *identifier = benchString(itemID);
	benchOnMain(^{
		if (benchStopped()) {
			return;
		}
		[benchWebItems()[identifier] pauseMedia];
	});
}
