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
