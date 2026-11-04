package platform

import (
	"bytes"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itbm/bench/internal/clock"
	"github.com/itbm/bench/internal/layout"
)

// syncBuffer is a bytes.Buffer that the forwarding goroutine and the test
// can use at once.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestLoggedPassesThroughAndLogs(t *testing.T) {
	f := NewFake(clock.NewFake(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)))
	f.AddApp("dev.zed.Zed", "Zed")
	w := f.AddWindow("dev.zed.Zed", "main.go")
	var buf syncBuffer
	p := Logged(f, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	to := layout.Rect{X: 10, Y: 40, W: 800, H: 600}
	if err := p.SetFrame(w, to); err != nil {
		t.Fatalf("SetFrame: %v", err)
	}
	if got, _ := f.Frame(w.ID); got != to {
		t.Errorf("the call didn't reach the platform: frame %v, want %v", got, to)
	}
	if err := f.UserMoveWindow(w.ID, layout.Rect{X: 300, Y: 300, W: 800, H: 600}); err != nil {
		t.Fatal(err)
	}
	// The first move is Bench's own SetFrame; the user's comes next.
	var ev Event
	for ev.Kind != WindowMoved || ev.Window.Frame.X != 300 {
		select {
		case ev = <-p.Events():
		case <-time.After(5 * time.Second):
			t.Fatal("no WindowMoved event came through")
		}
	}
	if !ev.MouseDown {
		t.Errorf("a user's move should come through with MouseDown: %+v", ev)
	}
	f.SetScrollBarsShown(true)
	if !p.ScrollBarsShown() {
		t.Error("ScrollBarsShown didn't reach the platform")
	}
	f.SetSystemKeys(SystemKeys{InputSources: []string{"ctrl+alt+space"}, VoiceOver: true})
	if k := p.SystemKeys(); !k.VoiceOver || len(k.InputSources) != 1 {
		t.Errorf("SystemKeys didn't reach the platform: %+v", k)
	}
	f.SetStageManager(true)
	if !p.StageManager() {
		t.Error("StageManager didn't reach the platform")
	}
	f.SetLoggingOut(true)
	if !p.LoggingOut() {
		t.Error("LoggingOut didn't reach the platform")
	}
	f.SetAppIcon("dev.zed.Zed", []byte("zed"))
	if png, err := p.AppIcon("dev.zed.Zed"); err != nil || string(png) != "zed" {
		t.Errorf("AppIcon didn't reach the platform: %q, %v", png, err)
	}
	f.SetPageIcon("w1", []byte("page"))
	if png, err := p.PageIcon("w1"); err != nil || string(png) != "page" {
		t.Errorf("PageIcon didn't reach the platform: %q, %v", png, err)
	}
	if _, err := p.AppIcon("com.example.None"); !errors.Is(err, ErrNoIcon) {
		t.Errorf("AppIcon of an app with none = %v, want ErrNoIcon", err)
	}
	p.SetBadge(3)
	if f.Badge() != 3 {
		t.Errorf("SetBadge didn't reach the platform: badge %d, want 3", f.Badge())
	}
	f.SetLoginItemNeedsApproval(true)
	if !p.LoginItemNeedsApproval() {
		t.Error("LoginItemNeedsApproval didn't reach the platform")
	}
	if err := p.OpenSettings(SettingsLoginItems); err != nil {
		t.Fatalf("OpenSettings: %v", err)
	}
	if got := f.SettingsOpened(); len(got) != 1 || got[0] != SettingsLoginItems {
		t.Errorf("OpenSettings didn't reach the platform: %v", got)
	}
	p.TrackDrags(true)
	if !f.TrackingDrags() {
		t.Error("TrackDrags didn't reach the platform")
	}
	f.SetDockBadge("dev.zed.Zed", "2")
	p.WatchDockBadges(true)
	if !f.WatchingDockBadges() {
		t.Error("WatchDockBadges didn't reach the platform")
	}
	if got := p.DockBadges(); got["dev.zed.Zed"] != "2" {
		t.Errorf("DockBadges didn't reach the platform: %v", got)
	}
	p.ShowDropGuide(DropGuide{Pane: layout.Rect{W: 10, H: 10}, Zone: "center"})
	if _, ok := f.DropGuideShown(); !ok {
		t.Error("ShowDropGuide didn't reach the platform")
	}
	p.HideDropGuide()
	if _, ok := f.DropGuideShown(); ok {
		t.Error("HideDropGuide didn't reach the platform")
	}
	gone := f.AddWindow("dev.zed.Zed", "notes.md")
	if err := p.PressClose(gone); err != nil {
		t.Fatalf("PressClose: %v", err)
	}
	if _, ok := f.Frame(gone.ID); ok {
		t.Error("PressClose didn't reach the platform")
	}
	logs := buf.String()
	if strings.Contains(logs, "msg=AppIcon") || strings.Contains(logs, "msg=PageIcon") {
		t.Errorf("icons that were there, or weren't, were logged:\n%s", logs)
	}
	for _, want := range []string{"msg=SetFrame", "app=dev.zed.Zed", "msg=event", "kind=window-moved", "mouseDown=true", "msg=ScrollBarsShown shown=true",
		"msg=SystemKeys", "voiceOver=true", "msg=StageManager on=true", "msg=LoggingOut loggingOut=true", "msg=PressClose window=" + strconv.Itoa(int(gone.ID)), "msg=SetBadge count=3",
		"msg=LoginItemNeedsApproval needs=true", "msg=OpenSettings pane=login-items", "msg=TrackDrags on=true", "msg=WatchDockBadges on=true", "msg=DockBadges apps=1", "msg=ShowDropGuide",
		"zone=center", "msg=HideDropGuide"} {
		if !strings.Contains(logs, want) {
			t.Errorf("the log lacks %q:\n%s", want, logs)
		}
	}
	p.(interface{ SetDemoScreen(string, float64) }).SetDemoScreen("laptop", 0)
	if got := f.ScreenOfWindow().Frame.W; got != 1512 {
		t.Errorf("SetDemoScreen should pass on to the Fake: screen width %v, want 1512", got)
	}
}

func TestLoggedLeavesOutSecrets(t *testing.T) {
	f := NewFake(clock.NewFake(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)))
	f.AddApp("com.tinyspeck.slackmacgap", "Slack")
	var buf syncBuffer
	p := Logged(f, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	addr := "http://127.0.0.1:52011/?tkn=s3cret-token&folder=%2FUsers%2Fme%2Fbench"
	if err := p.ShowWebView("w9", WebView{URL: addr}, layout.Rect{W: 800, H: 600}); err != nil {
		t.Fatal(err)
	}
	if got := f.WebViews()["w9"].View.URL; got != addr {
		t.Errorf("the platform got %q, want the address as it was, %q", got, addr)
	}
	if _, err := f.OpenWindow("com.tinyspeck.slackmacgap", "Direct message from Sam"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.Events():
	case <-time.After(5 * time.Second):
		t.Fatal("no event came through")
	}
	logs := buf.String()
	for _, secret := range []string{"s3cret-token", "Direct message"} {
		if strings.Contains(logs, secret) {
			t.Errorf("the log has %q:\n%s", secret, logs)
		}
	}
	for _, want := range []string{"tkn=redacted", "folder=%2FUsers%2Fme%2Fbench", "kind=window-created"} {
		if !strings.Contains(logs, want) {
			t.Errorf("the log lacks %q:\n%s", want, logs)
		}
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"https://example.com/inbox?tab=2", "https://example.com/inbox?tab=2"},
		{"http://127.0.0.1:5000/?tkn=abc", "http://127.0.0.1:5000/?tkn=redacted"},
		{"http://[::1]:5000/?folder=%2Ftmp&tkn=abc", "http://[::1]:5000/?folder=%2Ftmp&tkn=redacted"},
		{"http://127.0.0.1:5000/?tkn=abc;x", "http://127.0.0.1:5000/"},
		{"http://bad host/?tkn=abc", "http://bad host/"},
	}
	for _, tt := range tests {
		if got := redactURL(tt.in); got != tt.want {
			t.Errorf("redactURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLoggedForwardsTheDemoScreen(t *testing.T) {
	p := Logged(NewDemo(clock.NewFake(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC))), slog.New(slog.DiscardHandler))
	ds, ok := p.(DemoScreens)
	if !ok {
		t.Fatal("the logged demo platform isn't DemoScreens")
	}
	ds.SetDemoScreen(DemoMonitor, 0)
	if got := ds.DemoScreen(); got != DemoMonitor {
		t.Errorf("DemoScreen() = %q, want %q", got, DemoMonitor)
	}
	if got := Logged(NewFake(nil), slog.New(slog.DiscardHandler)).(DemoScreens).DemoScreen(); got != "" {
		t.Errorf("DemoScreen() of a platform without one = %q, want \"\"", got)
	}
}

func TestLoggedMinimises(t *testing.T) {
	f := NewFake(nil)
	w := f.AddWindow("dev.zed.Zed", "main.go")
	var buf syncBuffer
	p := Logged(f, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err := p.Minimise(w); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.WindowState(w.ID); !got.Minimised {
		t.Error("the call didn't reach the platform")
	}
	if logs := buf.String(); !strings.Contains(logs, "msg=Minimise") {
		t.Errorf("the log lacks the call:\n%s", logs)
	}
}

func TestLoggedForwardsMinimiseWindow(t *testing.T) {
	// The browser preview's demo bar minimises a window as the user would,
	// with BENCH_DEBUG set too.
	f := NewFake(nil)
	w := f.AddWindow("dev.zed.Zed", "main.go")
	var buf syncBuffer
	p := Logged(f, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	dw, ok := p.(interface{ MinimiseWindow(WindowID) error })
	if !ok {
		t.Fatal("the logged demo platform has no MinimiseWindow")
	}
	if err := dw.MinimiseWindow(w.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.WindowState(w.ID); !got.Minimised {
		t.Error("the call didn't reach the platform")
	}
	if logs := buf.String(); !strings.Contains(logs, "msg=MinimiseWindow") {
		t.Errorf("the log lacks the call:\n%s", logs)
	}
	// A platform without it, such as the Mac's, refuses.
	bare := struct{ Platform }{NewFake(nil)}
	if err := Logged(bare, slog.New(slog.DiscardHandler)).(interface{ MinimiseWindow(WindowID) error }).MinimiseWindow(w.ID); !errors.Is(err, ErrUnsupported) {
		t.Errorf("MinimiseWindow on a platform without it = %v, want ErrUnsupported", err)
	}
}

func TestLoggedForwardsHeldModifiers(t *testing.T) {
	// The browser preview's page says which modifier keys it holds, with
	// BENCH_DEBUG set too.
	f := NewFake(nil)
	p := Logged(f, slog.New(slog.DiscardHandler))
	dk, ok := p.(interface{ SetHeldModifiers(Modifiers) })
	if !ok {
		t.Fatal("the logged demo platform has no SetHeldModifiers")
	}
	dk.SetHeldModifiers(ModControl | ModOption)
	if got := p.HeldModifiers(); got != ModControl|ModOption {
		t.Errorf("HeldModifiers() = %v, want Control and Option", got)
	}
}

func TestLoggedForwardsSystemKeys(t *testing.T) {
	// The browser preview's demo bar simulates VoiceOver, with BENCH_DEBUG
	// set too.
	f := NewFake(nil)
	p := Logged(f, slog.New(slog.DiscardHandler))
	dk, ok := p.(interface{ SetSystemKeys(SystemKeys) })
	if !ok {
		t.Fatal("the logged demo platform has no SetSystemKeys")
	}
	dk.SetSystemKeys(SystemKeys{VoiceOver: true})
	if got := p.SystemKeys(); !got.VoiceOver {
		t.Errorf("SystemKeys() = %+v, want VoiceOver on", got)
	}
}

func TestLoggedChoosesToAdd(t *testing.T) {
	f := NewFake(nil)
	f.SetChosen("/Applications/Tools/Zed.app")
	var buf syncBuffer
	p := Logged(f, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	path, err := p.ChooseToAdd("/Applications")
	if path != "/Applications/Tools/Zed.app" || err != nil {
		t.Fatalf("ChooseToAdd = %q, %v, want the Fake's choice", path, err)
	}
	if calls := f.CallsTo("ChooseToAdd"); len(calls) != 1 || calls[0].Arg != "/Applications" {
		t.Errorf("the call didn't reach the platform: %v", calls)
	}
	f.SetErr("ChooseToAdd", ErrBusy)
	if _, err := p.ChooseToAdd("/Applications"); !errors.Is(err, ErrBusy) {
		t.Errorf("ChooseToAdd = %v, want ErrBusy passed on", err)
	}
	logs := buf.String()
	for _, want := range []string{"msg=ChooseToAdd start=/Applications chosen=/Applications/Tools/Zed.app", `err="already in progress"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("the log lacks %q:\n%s", want, logs)
		}
	}
}

func TestLoggedPagesLeaveOutWhereTheyAre(t *testing.T) {
	f := NewFake(clock.NewFake(time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)))
	var buf syncBuffer
	p := Logged(f, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	page := Page{Host: "intranet.example", Site: "intranet.example", Secure: true, OffSite: true}
	f.EmitEvent(Event{Kind: WebURL, ItemID: "w1", Body: "https://intranet.example/payroll", Page: page})
	select {
	case ev := <-p.Events():
		if ev.Page != page {
			t.Errorf("the event's page is %+v, want %+v", ev.Page, page)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event came through")
	}
	logs := buf.String()
	for _, want := range []string{"kind=web-url", "item=w1", "secure=true", "offSite=true"} {
		if !strings.Contains(logs, want) {
			t.Errorf("the log lacks %q:\n%s", want, logs)
		}
	}
	if strings.Contains(logs, "intranet") || strings.Contains(logs, "payroll") {
		t.Errorf("the log says where the page is:\n%s", logs)
	}
}

// debugFake is a Fake that logs decisions of its own.
type debugFake struct {
	*Fake
	log *slog.Logger
}

func (d *debugFake) SetDebugLog(log *slog.Logger) { d.log = log }

func TestLoggedGivesTheLogToADebugLogger(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	d := &debugFake{Fake: NewFake(nil)}
	Logged(d, log)
	if d.log != log {
		t.Error("Logged didn't give the platform its log")
	}
}

func TestLoggedPageControls(t *testing.T) {
	f := NewFake(nil)
	var buf syncBuffer
	p := Logged(f, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err := p.ShowWebView("w1", WebView{URL: "https://app.slack.com/client"}, layout.Rect{W: 800, H: 600}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetPageText("w1", "Payroll for March"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetWebState("w1", WebState{CanGoBack: true, Microphone: CaptureActive}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-p.Events():
		if ev.Kind != WebStateChanged || !ev.Web.CanGoBack || ev.Web.Microphone != CaptureActive {
			t.Errorf("the event is %+v, want the page's new state", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event came through")
	}
	found, err := p.FindInWebView("w1", "payroll", true)
	if err != nil || !found {
		t.Fatalf("FindInWebView = %v, %v, want the page's text found", found, err)
	}
	for _, err := range []error{p.ReloadWebView("w1"), p.NavigateWebView("w1", -1), p.ZoomWebView("w1", 1.25),
		p.SetCaptureState("w1", Microphone, CaptureMuted), p.PauseWebViewMedia("w1")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	wv := f.WebViews()["w1"]
	if wv.Reloads != 1 || wv.Back != 1 || wv.Zoom != 1.25 || wv.State.Microphone != CaptureMuted || wv.Paused != 1 {
		t.Errorf("the calls didn't reach the platform: %+v", wv)
	}
	if err := p.ReloadWebView("w9"); err == nil {
		t.Error("ReloadWebView of a web view that doesn't exist should fail")
	}
	logs := buf.String()
	for _, want := range []string{"kind=web-state-changed", "canGoBack=true", "microphone=active", "msg=FindInWebView item=w1 length=7 backwards=true found=true",
		"msg=ReloadWebView item=w1", "msg=NavigateWebView item=w1 step=-1", "msg=ZoomWebView item=w1 zoom=1.25",
		"msg=SetCaptureState item=w1 device=microphone state=muted", "msg=PauseWebViewMedia item=w1", "msg=ReloadWebView item=w9 err="} {
		if !strings.Contains(logs, want) {
			t.Errorf("the log lacks %q:\n%s", want, logs)
		}
	}
	if strings.Contains(logs, "payroll") {
		t.Errorf("the log has the text found:\n%s", logs)
	}
}
