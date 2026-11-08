//go:build unix

package control

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// shortDir returns a new folder, private to the user, with a path short
// enough for a socket: t.TempDir's can be too long on macOS.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "bc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func sampleList() List {
	return List{
		Version: ListVersion, Current: 1, Attention: 1, Unread: Unread{Count: 3},
		Tabs: []Tab{
			{Number: 1, Name: "Code", Current: true, Attention: 1, Panes: []Pane{
				{ID: "p1", Focused: true, Items: []Item{
					{ID: "i1", Kind: KindApp, App: "Zed", Title: "main.go", Shown: true},
					{ID: "i2", Kind: KindTerminal, App: "Terminal", Title: "zsh", Attention: "Rang the bell"},
				}},
			}},
			{Number: 2, Name: "Mail", Unread: Unread{Count: 3}, Panes: []Pane{
				{ID: "p2", Focused: true, Items: []Item{
					{ID: "i3", Kind: KindWeb, App: "Gmail", Title: "Inbox", Shown: true, Unread: Unread{Count: 3}},
				}},
			}},
		},
	}
}

func TestServeAnswersListOnlyToTheUser(t *testing.T) {
	dir := shortDir(t)
	want := sampleList()
	s, err := Serve(dir, func() (List, error) { return want, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	path := filepath.Join(dir, SocketFile)
	if s.Path() != path {
		t.Errorf("Path() = %q, want %q", s.Path(), path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&fs.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		t.Errorf("the socket's mode is %v, want a socket with 0600", info.Mode())
	}
	if _, err := os.Lstat(path + ".new"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket's other name is still there: %v", err)
	}
	got, err := Query(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Query() = %+v, want %+v", got, want)
	}
}

func TestListJSONHasEveryField(t *testing.T) {
	// Scripts read these names, which the user guide documents: every
	// field is there, even when it's empty.
	data, err := json.Marshal(List{Version: 1, Current: 1, Tabs: []Tab{{Number: 1, Name: "Code", Panes: []Pane{{ID: "p1", Items: []Item{{ID: "i1", Kind: KindApp}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"current":1,"attention":0,"unread":{"count":0,"more":false,"dot":false},"tabs":[` +
		`{"number":1,"name":"Code","current":false,"colour":"","attention":0,"unread":{"count":0,"more":false,"dot":false},"panes":[` +
		`{"id":"p1","focused":false,"items":[{"id":"i1","kind":"app","app":"","title":"","shown":false,"attention":"","unread":{"count":0,"more":false,"dot":false},"keepRunning":false,"notLoaded":false}]}]}]` +
		`,"dropdown":null}`
	if string(data) != want {
		t.Errorf("JSON =\n%s\nwant\n%s", data, want)
	}

	// The drop-down, when there is one, has an item's fields.
	data, err = json.Marshal(List{Version: 1, Current: 1, Dropdown: &Item{ID: "w9", Kind: KindTerminal}})
	if err != nil {
		t.Fatal(err)
	}
	want = `{"version":1,"current":1,"attention":0,"unread":{"count":0,"more":false,"dot":false},"tabs":null,` +
		`"dropdown":{"id":"w9","kind":"terminal","app":"","title":"","shown":false,"attention":"","unread":{"count":0,"more":false,"dot":false},"keepRunning":false,"notLoaded":false}}`
	if string(data) != want {
		t.Errorf("JSON with a drop-down =\n%s\nwant\n%s", data, want)
	}
}

func TestUnreadText(t *testing.T) {
	for u, want := range map[Unread]string{{}: "", {Count: 3}: "3", {Count: 99, More: true}: "99+", {Dot: true}: "•"} {
		if got := u.Text(); got != want {
			t.Errorf("%+v.Text() = %q, want %q", u, got, want)
		}
	}
}

func TestQueryReportsBenchsError(t *testing.T) {
	dir := shortDir(t)
	s, err := Serve(dir, func() (List, error) { return List{}, errors.New("Allow scripts to control Bench is off") })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := Query(dir); err == nil || err.Error() != "Allow scripts to control Bench is off" {
		t.Errorf("Query() = %v", err)
	}
}

func TestQueryWithNothingThere(t *testing.T) {
	dir := shortDir(t)
	if _, err := Query(dir); !errors.Is(err, ErrNotServing) {
		t.Errorf("Query() with no socket = %v, want ErrNotServing", err)
	}
	// A socket that nothing answers, as a Bench that crashed leaves.
	stale(t, filepath.Join(dir, SocketFile))
	if _, err := Query(dir); !errors.Is(err, ErrNotServing) {
		t.Errorf("Query() with a stale socket = %v, want ErrNotServing", err)
	}
}

// stale leaves a socket at path that nothing answers.
func stale(t *testing.T, path string) {
	t.Helper()
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	ln.SetUnlinkOnClose(false)
	ln.Close()
	if _, err := os.Lstat(path); err != nil {
		t.Fatal(err)
	}
}

func TestServeReplacesAStaleSocket(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, SocketFile)
	stale(t, path)
	stale(t, path+".new")
	s, err := Serve(dir, func() (List, error) { return sampleList(), nil })
	if err != nil {
		t.Fatalf("Serve over a stale socket: %v", err)
	}
	defer s.Close()
	if _, err := Query(dir); err != nil {
		t.Error(err)
	}
}

func TestServeRefusesWhatItMustNotReplace(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, SocketFile)
	// A file that isn't a socket is the user's, and stays.
	if err := os.WriteFile(path, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Serve(dir, sampleAnswer); err == nil || !strings.Contains(err.Error(), "isn't Bench's socket") {
		t.Errorf("Serve over a file = %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "mine" {
		t.Errorf("the file has %q", data)
	}
	os.Remove(path)

	// A socket that something answers belongs to another Bench.
	first, err := Serve(dir, sampleAnswer)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if s, err := Serve(dir, sampleAnswer); err == nil || !strings.Contains(err.Error(), "another Bench answers") {
		t.Errorf("Serve over a live socket = %v", err)
		if s != nil {
			s.Close()
		}
	}
	if _, err := Query(dir); err != nil {
		t.Errorf("the first socket stopped answering: %v", err)
	}
}

func sampleAnswer() (List, error) { return sampleList(), nil }

func TestServeRefusesAPathTooLong(t *testing.T) {
	dir := filepath.Join(shortDir(t), strings.Repeat("d", 100))
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Serve(dir, sampleAnswer); err == nil || !strings.Contains(err.Error(), "longer than macOS allows") {
		t.Errorf("Serve = %v", err)
	}
}

func TestCloseRemovesOnlyItsOwnSocket(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, SocketFile)
	s, err := Serve(dir, sampleAnswer)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket is still there after Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("a second Close: %v", err)
	}
	if _, err := Query(dir); !errors.Is(err, ErrNotServing) {
		t.Errorf("Query() after Close = %v", err)
	}

	// Another socket at the path, made after Serve's, isn't Serve's to
	// remove.
	s, err = Serve(dir, sampleAnswer)
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	other, err := Serve(dir, sampleAnswer)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	s.Close()
	if _, err := Query(dir); err != nil {
		t.Errorf("Close removed another socket: %v", err)
	}
}

func TestServeIgnoresOtherRequests(t *testing.T) {
	dir := shortDir(t)
	s, err := Serve(dir, sampleAnswer)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ask := func(send string) string {
		t.Helper()
		c, err := net.Dial("unix", filepath.Join(dir, SocketFile))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		c.Write([]byte(send))
		buf := make([]byte, 4096)
		n, _ := c.Read(buf)
		return string(buf[:n])
	}
	if got := ask("quit\n"); got != "{\"error\":\"Bench doesn't know that request\"}\n" {
		t.Errorf("quit: %q", got)
	}
	// A request that never ends gets no answer.
	if got := ask(strings.Repeat("x", 100)); got != "" {
		t.Errorf("a long request: %q", got)
	}
	if got := ask("list\n"); !strings.HasPrefix(got, `{"version":1,`) {
		t.Errorf("list: %q", got)
	}
}
