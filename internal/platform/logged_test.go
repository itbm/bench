package platform

import (
	"bytes"
	"log/slog"
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
	logs := buf.String()
	for _, want := range []string{"msg=SetFrame", "app=dev.zed.Zed", "msg=event", "kind=window-moved", "mouseDown=true"} {
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
