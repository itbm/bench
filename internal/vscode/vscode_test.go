package vscode

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fake is a pretend code command, written as a shell script.
type fake struct {
	path     string
	argsFile string // its arguments, one per line
	pidFile  string // its process ID
	child    string // the process ID of a child it starts, if any
}

// Bodies for fake scripts. $asked is the value of --port, and $port is the
// same, or, for --port 0, the port the fake pretends to have picked.
const (
	// A server that prints the licence, then its address after a moment,
	// and runs a child process.
	bodyServer = `echo "* Visual Studio Code Server"
echo "* By using the software, you agree to the License Terms (https://aka.ms/vscode-server-license)"
sleep 0.3
echo "Web UI available at http://127.0.0.1:$port?tkn=ignored"
sleep 60 &
echo $! > "$child"
wait
`
	bodySilent  = "exec sleep 60\n"
	bodyFails   = "echo \"error: can't download the server\" >&2\nexit 1\n"
	bodyMyPort  = "echo 'Listening on http://localhost:4567'\nexec sleep 60\n"
	bodyLicence = "echo 'See https://aka.ms/vscode-server-license'\nexec sleep 60\n"
)

func newFake(t *testing.T, body string) fake {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake code command is a shell script")
	}
	dir := t.TempDir()
	f := fake{
		path:     filepath.Join(dir, "code"),
		argsFile: filepath.Join(dir, "args"),
		pidFile:  filepath.Join(dir, "pid"),
		child:    filepath.Join(dir, "child"),
	}
	script := "#!/bin/sh\n" +
		"args=" + shQuote(f.argsFile) + "\n" +
		"child=" + shQuote(f.child) + "\n" +
		"echo $$ > " + shQuote(f.pidFile) + "\n" +
		": > \"$args\"\n" +
		"for a in \"$@\"; do printf '%s\\n' \"$a\" >> \"$args\"; done\n" +
		"port=\nprev=\n" +
		"for a in \"$@\"; do [ \"$prev\" = --port ] && port=$a; prev=$a; done\n" +
		"asked=$port\n" +
		"[ \"$port\" = 0 ] && port=$((20000 + $$ % 40000))\n" +
		body
	if err := os.WriteFile(f.path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fake) args(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(f.argsFile)
	if err != nil {
		t.Fatalf("the fake code wasn't run: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func (f fake) pid(t *testing.T, file string) int {
	t.Helper()
	var pid int
	waitFor(t, "a process ID in "+filepath.Base(file), func() bool {
		b, err := os.ReadFile(file)
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
		return err == nil
	})
	return pid
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func startFake(t *testing.T, opts Options) *Server {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Start(ctx, opts)
	if err != nil {
		t.Fatalf("Start() = %v", err)
	}
	t.Cleanup(func() { _ = s.Stop() })
	return s
}

func TestStart(t *testing.T) {
	f := newFake(t, bodyServer)
	tokenDir := filepath.Join(t.TempDir(), "tokens")
	began := time.Now()
	s := startFake(t, Options{Code: f.path, TokenDir: tokenDir})
	if elapsed := time.Since(began); elapsed < 250*time.Millisecond {
		t.Errorf("Start() returned after %v: the licence's address counted as the server's", elapsed)
	}

	args := f.args(t)
	if len(args) != 8 {
		t.Fatalf("code ran with %q, want 8 arguments", args)
	}
	file := args[6]
	// The server picks the port, so that no other process can take it
	// first.
	want := []string{"serve-web", "--host", "127.0.0.1", "--port", "0",
		"--connection-token-file", file, "--accept-server-license-terms"}
	if !slices.Equal(args, want) {
		t.Errorf("code ran with %q, want %q", args, want)
	}
	if slices.Contains(args, "--without-connection-token") {
		t.Error("code ran with --without-connection-token")
	}
	if s.Port() <= 0 {
		t.Errorf("Port() = %d, want a port", s.Port())
	}

	if filepath.Dir(file) != tokenDir {
		t.Errorf("token file %s isn't in TokenDir %s", file, tokenDir)
	}
	checkMode(t, file, 0o600)
	checkMode(t, tokenDir, 0o700)
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	token := string(b)
	if raw, err := hex.DecodeString(token); err != nil || len(raw) != 32 {
		t.Errorf("token = %q, want 32 bytes as hex", token)
	}
	wantURL := "http://127.0.0.1:" + strconv.Itoa(s.Port()) + "/?tkn=" + token
	if got := s.URL(""); got != wantURL {
		t.Errorf("URL(\"\") = %q, want %q", got, wantURL)
	}

	child := f.pid(t, f.child)
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("token file still there after Stop: %v", err)
	}
	select {
	case <-s.Done():
	default:
		t.Error("Done() isn't closed after Stop")
	}
	if runtime.GOOS == "linux" {
		waitFor(t, "the server's child to end", func() bool { return !processAlive(child) })
	}
	if err := s.Stop(); err != nil {
		t.Errorf("second Stop() = %v, want nil", err)
	}
}

// bodyGraceful is a server whose child takes a moment to shut down after
// SIGTERM, as a real server might, then records that it finished. The
// script itself, which Start ran, ends straight away. It only prints its
// address once the child is ready for SIGTERM.
const bodyGraceful = `ready="$child.ready"
( trap 'sleep 0.3; echo finished > "$child"; exit 0' TERM
  : > "$ready"
  while :; do sleep 0.05; done ) &
while [ ! -e "$ready" ]; do sleep 0.01; done
echo "Web UI available at http://127.0.0.1:$port"
wait
`

func TestStopGivesTheWholeGroupTime(t *testing.T) {
	f := newFake(t, bodyGraceful)
	s := startFake(t, Options{Code: f.path, TokenDir: t.TempDir()})
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	b, err := os.ReadFile(f.child)
	if err != nil || strings.TrimSpace(string(b)) != "finished" {
		t.Errorf("the server's child didn't finish shutting down before Stop returned (%q, %v): SIGKILL came too soon", b, err)
	}
}

// bodyStubborn is a server that ignores SIGTERM, as does the child it
// starts (an ignored signal stays ignored across exec).
const bodyStubborn = `trap '' TERM
sleep 60 &
echo $! > "$child"
echo "Web UI available at http://127.0.0.1:$port"
wait
`

func TestStopKillsAServerThatIgnoresSIGTERM(t *testing.T) {
	oldGrace := stopGrace
	t.Cleanup(func() { stopGrace = oldGrace })
	stopGrace = 200 * time.Millisecond

	f := newFake(t, bodyStubborn)
	s := startFake(t, Options{Code: f.path, TokenDir: t.TempDir()})
	child := f.pid(t, f.child)
	began := time.Now()
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	select {
	case <-s.Done():
	default:
		t.Errorf("Done() isn't closed after Stop: SIGTERM was ignored, so Stop should have sent SIGKILL")
	}
	if elapsed := time.Since(began); elapsed > stopGrace+killWait+time.Second {
		t.Errorf("Stop() took %v", elapsed)
	}
	if runtime.GOOS == "linux" {
		waitFor(t, "the server's child to end", func() bool { return !processAlive(child) })
	}
}

// TestStopSparesAGroupThatHasEmptied stands in for a server that ended on
// its own long before Bench quit: its process group's ID may now be another
// group's, such as a build in Terminal, so Stop mustn't signal it.
func TestStopSparesAGroupThatHasEmptied(t *testing.T) {
	f := newFake(t, "echo \"Web UI available at http://127.0.0.1:$port\"\nsleep 0.2\n")
	s := startFake(t, Options{Code: f.path, TokenDir: t.TempDir()})
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the server didn't end on its own")
	}
	oldSignal := groupSignal
	t.Cleanup(func() { groupSignal = oldSignal })
	var signalled []int
	groupSignal = func(pid int, kill bool) { signalled = append(signalled, pid) }
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	if len(signalled) != 0 {
		t.Errorf("Stop() signalled process group %v, which had emptied when the server ended", signalled)
	}
}

// TestStopEndsWhatAnEndedServerLeft checks that Stop still ends the rest of
// the group when the process Start ran has ended but left a child running.
func TestStopEndsWhatAnEndedServerLeft(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc to see the child end")
	}
	f := newFake(t, "sleep 60 &\necho $! > \"$child\"\necho \"Web UI available at http://127.0.0.1:$port\"\nsleep 0.2\n")
	s := startFake(t, Options{Code: f.path, TokenDir: t.TempDir()})
	child := f.pid(t, f.child)
	select {
	case <-s.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the server didn't end on its own")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	waitFor(t, "the server's child to end", func() bool { return !processAlive(child) })
}

func TestStartWithEndedContext(t *testing.T) {
	f := newFake(t, bodyServer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tokenDir := t.TempDir()
	if _, err := Start(ctx, Options{Code: f.path, TokenDir: tokenDir}); !errors.Is(err, context.Canceled) {
		t.Errorf("Start() = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(f.argsFile); !errors.Is(err, os.ErrNotExist) {
		t.Error("Start() ran code although ctx had already ended")
	}
	checkEmpty(t, tokenDir)
}

func TestStartWithPort(t *testing.T) {
	f := newFake(t, bodyServer)
	s := startFake(t, Options{Code: f.path, TokenDir: t.TempDir(), Port: 45678})
	if s.Port() != 45678 {
		t.Errorf("Port() = %d, want 45678", s.Port())
	}
	if args := f.args(t); len(args) < 5 || args[3] != "--port" || args[4] != "45678" {
		t.Errorf("code ran with %q, want --port 45678", args)
	}
}

func TestStartWithoutTokenDir(t *testing.T) {
	f := newFake(t, bodyServer)
	s := startFake(t, Options{Code: f.path})
	file := f.args(t)[6]
	checkMode(t, file, 0o600)
	checkMode(t, filepath.Dir(file), 0o700)
	if err := s.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(file)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temporary token folder still there after Stop: %v", err)
	}
}

// TestStartWithACodeThatHidesItsPort stands in for a code from before VS
// Code 1.103, which prints the port it was asked for, 0, not the one it
// picked: Start stops it and asks for a port that's free.
func TestStartWithACodeThatHidesItsPort(t *testing.T) {
	f := newFake(t, "echo $$ >> \"$args.pids\"\n"+
		"echo \"Web UI available at http://127.0.0.1:$asked\"\nexec sleep 60\n")
	tokenDir := t.TempDir()
	s := startFake(t, Options{Code: f.path, TokenDir: tokenDir})
	args := f.args(t)
	if s.Port() == 0 || len(args) < 5 || args[4] != strconv.Itoa(s.Port()) {
		t.Errorf("Port() = %d, and code last ran with %q; want the port it was asked for", s.Port(), args)
	}
	b, err := os.ReadFile(f.argsFile + ".pids")
	if err != nil {
		t.Fatal(err)
	}
	pids := strings.Fields(string(b))
	if len(pids) != 2 {
		t.Fatalf("code ran %d times, want twice", len(pids))
	}
	if runtime.GOOS == "linux" {
		first, _ := strconv.Atoi(pids[0])
		waitFor(t, "the first server to end", func() bool { return !processAlive(first) })
	}
	if entries, _ := os.ReadDir(tokenDir); len(entries) != 1 {
		t.Errorf("%s holds %d files, want only the running server's token", tokenDir, len(entries))
	}
}

func TestStartUsesThePrintedPort(t *testing.T) {
	f := newFake(t, bodyMyPort)
	s := startFake(t, Options{Code: f.path, TokenDir: t.TempDir()})
	if s.Port() != 4567 {
		t.Errorf("Port() = %d, want the printed port, 4567", s.Port())
	}
}

// TestURLIsNeverLocalhost checks that VS Code for the Web is loaded at the
// loopback address the server listens on. macOS looks localhost up as ::1
// first, where code serve-web isn't listening, so another process could
// listen there on the same port and receive the token.
func TestURLIsNeverLocalhost(t *testing.T) {
	tests := []struct {
		name    string
		printed string // The address the server prints; $port is its port.
		host    string // The host URL should use.
	}{
		{"IPv4", "http://127.0.0.1:$port?tkn=x", "127.0.0.1"},
		{"localhost", "http://localhost:$port", "127.0.0.1"},
		{"every interface", "http://0.0.0.0:$port", "127.0.0.1"},
		{"IPv6", "http://[::1]:$port", "[::1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, "echo \"Web UI available at "+tt.printed+"\"\nexec sleep 60\n")
			s := startFake(t, Options{Code: f.path, TokenDir: t.TempDir()})
			want := "http://" + tt.host + ":" + strconv.Itoa(s.Port()) + "/?tkn="
			if got := s.URL(""); !strings.HasPrefix(got, want) {
				t.Errorf("URL(\"\") = %q, want it to start %q", got, want)
			}
		})
	}
}

func TestStartTimesOut(t *testing.T) {
	for _, body := range []string{bodySilent, bodyLicence} {
		f := newFake(t, body)
		tokenDir := t.TempDir()
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		began := time.Now()
		s, err := Start(ctx, Options{Code: f.path, TokenDir: tokenDir})
		cancel()
		if err == nil {
			_ = s.Stop()
			t.Fatal("Start() = nil error for a server that never prints its address")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Start() = %v, want context.DeadlineExceeded", err)
		}
		if elapsed := time.Since(began); elapsed > 5*time.Second {
			t.Errorf("Start() took %v, want it to stop when ctx ends", elapsed)
		}
		checkEmpty(t, tokenDir)
		if runtime.GOOS == "linux" {
			pid := f.pid(t, f.pidFile)
			waitFor(t, "the server to end", func() bool { return !processAlive(pid) })
		}
	}
}

func TestStartServerFails(t *testing.T) {
	f := newFake(t, bodyFails)
	tokenDir := t.TempDir()
	_, err := Start(context.Background(), Options{Code: f.path, TokenDir: tokenDir})
	if err == nil {
		t.Fatal("Start() = nil error for a server that fails")
	}
	if !strings.Contains(err.Error(), "can't download the server") {
		t.Errorf("Start() = %v, want it to quote the server's output", err)
	}
	checkEmpty(t, tokenDir)
}

func TestFindCode(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "code")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	inPath := func(string) (string, error) { return exe, nil }
	notInPath := func(string) (string, error) { return "", errors.New("not found") }

	tests := []struct {
		name    string
		code    string
		look    func(string) (string, error)
		app     string
		want    string
		wantErr error
	}{
		{name: "Options.Code", code: exe, look: notInPath, want: exe},
		{name: "Options.Code missing", code: filepath.Join(dir, "nope"), look: inPath, wantErr: errAny},
		{name: "Options.Code not executable", code: plain, look: inPath, wantErr: errAny},
		{name: "PATH", look: inPath, app: filepath.Join(dir, "nope"), want: exe},
		{name: "the app", look: notInPath, app: exe, want: exe},
		{name: "nowhere", look: notInPath, app: filepath.Join(dir, "nope"), wantErr: ErrNoCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldLook, oldApp := lookPath, appCode
			t.Cleanup(func() { lookPath, appCode = oldLook, oldApp })
			lookPath, appCode = tt.look, tt.app

			got, err := findCode(tt.code)
			switch {
			case tt.wantErr == errAny && err == nil:
				t.Errorf("findCode(%q) = %q, want an error", tt.code, got)
			case tt.wantErr != nil && tt.wantErr != errAny && !errors.Is(err, tt.wantErr):
				t.Errorf("findCode(%q) error = %v, want %v", tt.code, err, tt.wantErr)
			case tt.wantErr == nil && (err != nil || got != tt.want):
				t.Errorf("findCode(%q) = %q, %v; want %q", tt.code, got, err, tt.want)
			}
		})
	}
}

// errAny stands for any error in tables.
var errAny = errors.New("any error")

func TestStartWithoutCode(t *testing.T) {
	oldLook, oldApp := lookPath, appCode
	t.Cleanup(func() { lookPath, appCode = oldLook, oldApp })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	appCode = filepath.Join(t.TempDir(), "nope")
	if _, err := Start(context.Background(), Options{}); !errors.Is(err, ErrNoCode) {
		t.Errorf("Start() = %v, want ErrNoCode", err)
	}
}

func TestURL(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder")
	}
	s := &Server{port: 1234, token: "abc123"}
	const base = "http://127.0.0.1:1234/?tkn=abc123"
	tests := []struct {
		folder string
		want   string
	}{
		{"", base},
		{"/Users/me/code", base + "&folder=%2FUsers%2Fme%2Fcode"},
		{"/tmp/my project/ü&x=1#y", base + "&folder=%2Ftmp%2Fmy+project%2F%C3%BC%26x%3D1%23y"},
		{"/tmp/a/../b/", base + "&folder=%2Ftmp%2Fb"},
		{"~", base + "&folder=" + escapePath(home)},
		{"~/code", base + "&folder=" + escapePath(filepath.Join(home, "code"))},
		{"code", base + "&folder=" + escapePath(filepath.Join(home, "code"))},
		{"code/../bench/", base + "&folder=" + escapePath(filepath.Join(home, "bench"))},
	}
	for _, tt := range tests {
		if got := s.URL(tt.folder); got != tt.want {
			t.Errorf("URL(%q) = %q, want %q", tt.folder, got, tt.want)
		}
	}
}

// escapePath query-escapes a plain path, for expected URLs.
func escapePath(p string) string {
	return strings.NewReplacer("/", "%2F", " ", "+").Replace(p)
}

func TestLocalAddress(t *testing.T) {
	tests := []struct {
		line     string
		port     int
		wantHost string
		wantPort int
		wantOK   bool
	}{
		{"Web UI available at http://127.0.0.1:8000?tkn=abc", 9, "127.0.0.1", 8000, true},
		{"Web UI available at http://localhost:8000/", 9, "localhost", 8000, true},
		{"listening on http://[::1]:8001", 9, "::1", 8001, true},
		{"listening on [http://[::1]]", 9, "::1", 0, true},
		{"http://0.0.0.0:8002.", 9, "0.0.0.0", 8002, true},
		{"(http://localhost)", 9, "localhost", 0, true},
		{"[http://127.0.0.1:8003]", 9, "127.0.0.1", 8003, true},
		{"https://mac.local:4321/ is ours", 4321, "mac.local", 4321, true},
		{"the License Terms (https://aka.ms/vscode-server-license) and", 9, "", 0, false},
		{"https://mac.local:4321/ isn't ours", 9, "", 0, false},
		{"no address", 9, "", 0, false},
	}
	for _, tt := range tests {
		host, port, ok := localAddress(tt.line, tt.port)
		if host != tt.wantHost || port != tt.wantPort || ok != tt.wantOK {
			t.Errorf("localAddress(%q, %d) = %q, %d, %v; want %q, %d, %v",
				tt.line, tt.port, host, port, ok, tt.wantHost, tt.wantPort, tt.wantOK)
		}
	}
}

func TestLineLog(t *testing.T) {
	l := newLineLog(2)
	if got := l.summary(); got != "it printed nothing" {
		t.Errorf("summary() = %q for no lines", got)
	}
	for _, line := range []string{"one", "  ", "two", "three"} {
		l.add(line)
	}
	if got, want := l.summary(), "two / three"; got != want {
		t.Errorf("summary() = %q, want %q", got, want)
	}
}

func checkMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s has mode %v, want %v", path, got, want)
	}
}

func checkEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("%s still holds %d files, want none", dir, len(entries))
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// processAlive reports whether pid is a live process on Linux: not gone, and
// not a zombie waiting to be reaped.
func processAlive(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	return i < 0 || i+2 >= len(s) || s[i+2] != 'Z'
}
