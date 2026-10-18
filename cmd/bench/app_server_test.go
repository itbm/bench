//go:build server

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/workspace"
)

// TestPreview runs the browser preview as `bench --port <port> --dir <dir>`
// does, with the real engine and the demo platform, and drives it the way
// the frontend does: it loads the page and the Wails runtime, calls bound
// methods, and listens for events. Wails' app is a process-wide singleton,
// so this is the only test here that runs it.
func TestPreview(t *testing.T) {
	if !wailsRuns.CompareAndSwap(false, true) {
		t.Skip("Wails' app can run only once in a process (go test -count above 1)")
	}
	dir := t.TempDir()
	port := freePort(t)
	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"--port", strconv.Itoa(port), "--dir", dir}, &stdout, &stderr)
	}()
	base := fmt.Sprintf("http://localhost:%d", port)
	waitForPreview(t, base, done)

	t.Run("page", func(t *testing.T) {
		for _, path := range []string{"/", "/wails/runtime.js", "/js/app.js"} {
			resp, err := http.Get(base + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s = %s", path, resp.Status)
			}
		}
	})

	var view workspace.View
	t.Run("ready", func(t *testing.T) {
		status, body := callService(t, base, "Ready")
		if status != http.StatusOK {
			t.Fatalf("Ready = %d %s", status, body)
		}
		if err := json.Unmarshal(body, &view); err != nil {
			t.Fatalf("Ready returned %s: %v", body, err)
		}
		if view.Demo == nil || len(view.Tabs) != 3 || view.Current != 0 {
			t.Errorf("Ready = %d tabs, current %d, demo %v; want the default config's 3 tabs in the demo",
				len(view.Tabs), view.Current, view.Demo)
		}
	})

	t.Run("switching sends the state", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://localhost:%d/wails/events", port), nil)
		if err != nil {
			t.Fatalf("connecting to the events: %v", err)
		}
		defer conn.CloseNow()
		states := make(chan workspace.View, 64)
		go func() {
			defer close(states)
			for {
				var ev struct {
					Name string          `json:"name"`
					Data json.RawMessage `json:"data"`
				}
				if err := wsjson.Read(ctx, conn, &ev); err != nil {
					return
				}
				var v workspace.View
				if ev.Name == "bench:state" && json.Unmarshal(ev.Data, &v) == nil {
					states <- v
				}
			}
		}()

		// The server registers the connection a moment after accepting it,
		// so change the mode until a bench:state arrives.
		modes := []string{"wide", "auto"}
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for registered := false; !registered; {
			select {
			case _, ok := <-states:
				if !ok {
					t.Fatal("the events connection closed")
				}
				registered = true
			case <-tick.C:
				callService(t, base, "SetMode", modes[0])
				modes[0], modes[1] = modes[1], modes[0]
			}
		}

		if status, body := callService(t, base, "SwitchTab", 1); status != http.StatusOK {
			t.Fatalf("SwitchTab(1) = %d %s", status, body)
		}
		for v := range states {
			if v.Current == 1 {
				return
			}
		}
		t.Fatal("no bench:state with the Code tab arrived")
	})

	t.Run("refused", func(t *testing.T) {
		status, body := callService(t, base, "SwitchTab", 7)
		var cerr struct{ Message string }
		_ = json.Unmarshal(body, &cerr)
		if status == http.StatusOK || cerr.Message == "" {
			t.Errorf("SwitchTab(7) = %d %s, want a refusal with a message", status, body)
		}
	})

	application.Get().Quit()
	select {
	case status := <-done:
		if status != exitOK {
			t.Errorf("bench = %d after quitting, want 0\nstderr: %s", status, stderr.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the preview didn't stop after quitting")
	}

	if !strings.Contains(stdout.String(), fmt.Sprintf("http://localhost:%d", port)) {
		t.Errorf("stdout = %q, want the preview's address", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, config.ParkedFile)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("parked.json is still there after quitting (%v); every window should be back", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, config.StateFile))
	if err != nil {
		t.Fatalf("state.json wasn't saved: %v", err)
	}
	var st struct{ Current int }
	if err := json.Unmarshal(data, &st); err != nil || st.Current != 1 {
		t.Errorf("state.json has current tab %d (%v), want 1, the tab switched to", st.Current, err)
	}
}

// wailsRuns records that a test has run Wails' app, which can run only once
// in a process.
var wailsRuns atomic.Bool

func TestPreviewPortErrors(t *testing.T) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	busy := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	tests := []struct {
		name      string
		args      []string
		status    int
		stderrHas string
	}{
		{"port in use", []string{"--port", busy, "--dir", t.TempDir()}, exitError, "try another with --port"},
		{"port out of range", []string{"--port", "70000"}, exitUsage, "--port must be from 1 to 65535"},
		{"argument", []string{"--port", "9000", "now"}, exitUsage, `unexpected argument "now"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, stdout, stderr := runBench(t, tt.args...)
			if status != tt.status || !strings.Contains(stderr, tt.stderrHas) {
				t.Errorf("bench %v = %d %q; want %d and %q", tt.args, status, stderr, tt.status, tt.stderrHas)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
		})
	}
}

// callService calls a bound method of ui.Service the way the frontend's
// runtime does, and returns the status and the body.
func callService(t *testing.T, base, method string, args ...any) (int, []byte) {
	t.Helper()
	if args == nil {
		args = []any{}
	}
	payload, err := json.Marshal(map[string]any{
		"object": 0, // Call
		"method": 0, // CallBinding
		"args": map[string]any{
			"call-id":    fmt.Sprintf("test-%s-%d", method, time.Now().UnixNano()),
			"methodName": "github.com/itbm/bench/internal/ui.Service." + method,
			"args":       args,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(base+"/wails/runtime", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("calling %s: %v", method, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("calling %s: %v", method, err)
	}
	return resp.StatusCode, body
}

// waitForPreview waits until the preview answers, or bench exits.
func waitForPreview(t *testing.T, base string, done <-chan int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case status := <-done:
			t.Fatalf("bench exited early with status %d", status)
		default:
		}
		if resp, err := http.Get(base + "/"); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s didn't answer", base)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
