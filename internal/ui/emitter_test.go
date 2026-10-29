//go:build darwin || server

package ui

import (
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/itbm/bench/internal/workspace"
)

// fakeSink records the events it's given.
type fakeSink struct {
	mu     sync.Mutex
	events []call
}

func (s *fakeSink) Emit(name string, data ...any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, call{Method: name, Args: data})
	return false
}

func (s *fakeSink) Events() []call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]call(nil), s.events...)
}

// fakeShell records what it's asked to do.
type fakeShell struct {
	mu    sync.Mutex
	calls []call
	err   error
}

func (s *fakeShell) record(method string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, call{Method: method, Args: args})
}

func (s *fakeShell) Calls() []call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]call(nil), s.calls...)
}

func (s *fakeShell) bringForward() { s.record("bringForward") }
func (s *fakeShell) popOutTerminal(itemID, title string) error {
	s.record("popOutTerminal", itemID, title)
	return s.err
}
func (s *fakeShell) closeTerminalWindow(itemID string) { s.record("closeTerminalWindow", itemID) }
func (s *fakeShell) setMenuBar(m workspace.MenuBar) {
	s.record("setMenuBar", m)
}
func (s *fakeShell) shortcutsChanged() { s.record("shortcutsChanged") }
func (s *fakeShell) fillScreen()       { s.record("fillScreen") }
func (s *fakeShell) quit()             { s.record("quit") }
func (s *fakeShell) relaunch()         { s.record("relaunch") }

func TestEmitterEvents(t *testing.T) {
	view := workspace.View{Current: 1, Mode: "auto", SizeClass: "wide"}
	confirmQuit := workspace.Command{Name: "confirm-quit", Terminals: []workspace.BusyTerminal{
		{ID: "w4", Title: "claude", Tab: "Code", Running: "claude"},
		{ID: "w7", Title: "Logs", Running: "tail -f app.log"}, // In a window of its own.
	}}
	undo := workspace.Toast{Text: "Deleted Code.", Action: workspace.ToastAction{Label: "Undo", Method: "Undo", Arg: 3}}
	tests := []struct {
		name string
		emit func(em *emitter)
		want call
		// json is the payload as the frontend sees it.
		json string
	}{
		{"state", func(em *emitter) { em.State(view) }, call{"bench:state", []any{view}}, ""},
		{"toast", func(em *emitter) { em.Toast("Moved to Code.") }, call{"bench:toast", []any{"Moved to Code."}},
			`"Moved to Code."`},
		{"toast with Undo", func(em *emitter) { em.ActionToast(undo) }, call{"bench:toast", []any{undo}},
			`{"text":"Deleted Code.","action":{"label":"Undo","method":"Undo","arg":3}}`},
		{"command", func(em *emitter) { em.Command(workspace.Command{Name: "new-tab"}) },
			call{"bench:command", []any{workspace.Command{Name: "new-tab"}}}, `{"name":"new-tab"}`},
		{"confirm quit", func(em *emitter) { em.Command(confirmQuit) }, call{"bench:command", []any{confirmQuit}},
			`{"name":"confirm-quit","terminals":[{"id":"w4","title":"claude","tab":"Code","running":"claude"},` +
				`{"id":"w7","title":"Logs","running":"tail -f app.log"}]}`},
		{"term data", func(em *emitter) { em.TermData(workspace.TermData{ID: "w4", Data: "aGk=", Offset: 7}) },
			call{"term:data", []any{workspace.TermData{ID: "w4", Data: "aGk=", Offset: 7}}}, `{"id":"w4","data":"aGk=","offset":7}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := &fakeSink{}
			em := newEmitter(sink, &fakeShell{})
			tt.emit(em)
			got := sink.Events()
			if len(got) != 1 || !reflect.DeepEqual(got[0], tt.want) {
				t.Fatalf("events = %v, want [%v]", got, tt.want)
			}
			if tt.json == "" {
				return
			}
			data, err := json.Marshal(got[0].Args[0])
			if err != nil || string(data) != tt.json {
				t.Errorf("payload JSON = %s, %v; want %s", data, err, tt.json)
			}
		})
	}
}

func TestEmitterStatePayload(t *testing.T) {
	sink := &fakeSink{}
	em := newEmitter(sink, &fakeShell{})
	em.State(workspace.View{Current: 1, Mode: "compact", Notices: []workspace.NoticeView{
		{Text: "config.toml was changed outside Bench.", Actions: []workspace.NoticeAction{{Label: "Reload", Method: "ReloadConfig"}}},
		{Text: "Couldn't register ⌃⌥Space."},
	}})
	data, err := json.Marshal(sink.Events()[0].Args[0])
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["current"] != 1.0 || got["mode"] != "compact" {
		t.Errorf("bench:state payload = %s; want current 1 and mode compact", data)
	}
	// A notice has its text and, if it has any, its buttons, each naming
	// the bound method it calls.
	notices, _ := json.Marshal(got["notices"])
	want := `[{"actions":[{"label":"Reload","method":"ReloadConfig"}],"text":"config.toml was changed outside Bench."},` +
		`{"text":"Couldn't register ⌃⌥Space."}]`
	if string(notices) != want {
		t.Errorf("notices = %s, want %s", notices, want)
	}
}

func TestEmitterCommandBringsBenchForward(t *testing.T) {
	tests := []struct {
		name    string
		forward bool
	}{
		{"toggle-palette", true},
		{"open-palette", true},
		{"new-tab", true},
		{"edit-tab", true},
		{"settings", true},
		{"first-run", true},
		// The question before quitting is in Bench's window. The one
		// before closing a terminal's window is in that window, which the
		// user is closing, so it's in front already.
		{"confirm-quit", true},
		{"confirm-close-terminal", false},
		// The engine has brought Bench forward already, in order with
		// other focus changes: doing it again here, later, could take the
		// keyboard from a pane focused since.
		{"focus-terminal", false},
		{"something-else", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink, sh := &fakeSink{}, &fakeShell{}
			cmd := workspace.Command{Name: tt.name, ItemID: "w5"}
			newEmitter(sink, sh).Command(cmd)
			forwarded := slicesContainsMethod(sh.Calls(), "bringForward")
			if forwarded != tt.forward {
				t.Errorf("brought Bench forward = %v, want %v", forwarded, tt.forward)
			}
			want := []call{{"bench:command", []any{cmd}}}
			if got := sink.Events(); !reflect.DeepEqual(got, want) {
				t.Errorf("events = %v, want %v", got, want)
			}
		})
	}
}

func TestEmitterShell(t *testing.T) {
	sh := &fakeShell{err: errors.New("no window")}
	em := newEmitter(&fakeSink{}, sh)
	if err := em.PopOutTerminal("w4", "zsh — ~/code"); err == nil {
		t.Error("PopOutTerminal didn't pass on the shell's error")
	}
	em.CloseTerminalWindow("w4")
	bar := workspace.MenuBar{Show: "icon", Current: 1, Tabs: []workspace.MenuBarTab{{Name: "Comms"}, {Name: "Code"}}}
	em.MenuBar(bar)
	em.ShortcutsChanged()
	em.FillScreen()
	em.BringForward()
	em.Quit()
	em.Relaunch()
	want := []call{
		{"popOutTerminal", []any{"w4", "zsh — ~/code"}},
		{"closeTerminalWindow", []any{"w4"}},
		{"setMenuBar", []any{bar}},
		{"shortcutsChanged", nil},
		{"fillScreen", nil},
		{"bringForward", nil},
		{"quit", nil},
		{"relaunch", nil},
	}
	if got := sh.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("shell calls = %v, want %v", got, want)
	}
}

func TestEmitterClosed(t *testing.T) {
	sink, sh := &fakeSink{}, &fakeShell{}
	em := newEmitter(sink, sh)
	em.close()
	em.State(workspace.View{})
	em.Toast("hello")
	em.Command(workspace.Command{Name: "toggle-palette"})
	em.TermData(workspace.TermData{ID: "w1"})
	if err := em.PopOutTerminal("w1", "zsh"); err != nil {
		t.Errorf("PopOutTerminal after close = %v, want nil", err)
	}
	em.CloseTerminalWindow("w1")
	em.MenuBar(workspace.MenuBar{})
	em.ShortcutsChanged()
	em.FillScreen()
	em.BringForward()
	em.Quit()
	em.Relaunch()
	if got := sink.Events(); len(got) != 0 {
		t.Errorf("events after close = %v, want none", got)
	}
	if got := sh.Calls(); len(got) != 0 {
		t.Errorf("shell calls after close = %v, want none", got)
	}
}

func slicesContainsMethod(calls []call, method string) bool {
	for _, c := range calls {
		if c.Method == method {
			return true
		}
	}
	return false
}
