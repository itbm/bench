// Package vscode runs `code serve-web`, so that VS Code for the Web can be a
// real embedded Web item.
//
// The server gives full access to the Mac's files and terminals, so it always
// needs a connection token: a random value in a file only the user can read,
// which Bench adds to the address it loads. The server listens on 127.0.0.1
// only, and Bench loads it there, never at localhost: macOS looks localhost
// up as ::1 first, where any other process could listen on the same port and
// so receive the token.
//
// If Bench crashes, the server would keep running in its own process group.
// So Start records it in a file (Options.Record), and StopRecorded, which the
// watchdog and the next Bench call, stops a server that's still running and
// removes its token.
package vscode

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AppCode is where the code command is in the VS Code app, used when it
// isn't in PATH.
const AppCode = "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"

// How long Stop waits, replaceable in tests.
var (
	// stopGrace is how long Stop waits after SIGTERM before SIGKILL.
	stopGrace = 3 * time.Second
	// killWait is how long Stop waits for the process after SIGKILL.
	killWait = 2 * time.Second
)

// groupSignal sends SIGTERM, or SIGKILL, to a process group, replaceable in
// tests.
var groupSignal = signalGroup

const (
	// startTimeout is the longest Start waits for the server's address.
	startTimeout = 30 * time.Second
	// drainWait is how long to wait for the last of the output from a
	// server that has ended.
	drainWait = 500 * time.Millisecond
	// tokenBytes is the size of the connection token, before hex encoding.
	tokenBytes = 32
	// keepLines is how many lines of the server's output are kept, to
	// explain a failure.
	keepLines = 20
	// maxLine is the longest line of output kept, in bytes.
	maxLine = 4096
	// listenHost is the address the server listens on. It's code
	// serve-web's default, but Bench says so, so that it's never every
	// interface.
	listenHost = "127.0.0.1"
	// Prefixes of the token file and of the folder Start makes for it,
	// which StopRecorded checks before removing what a record names.
	tokenPrefix   = "vscode-token-"
	tempDirPrefix = "bench-vscode-"
)

// ErrNoCode means the code command couldn't be found.
var ErrNoCode = errors.New("can't find VS Code's code command in PATH or in /Applications; " +
	"install VS Code 1.82 or later")

// Places to look for code, replaceable in tests.
var (
	lookPath = exec.LookPath
	appCode  = AppCode
)

// Options start a server.
type Options struct {
	// Code is the path to the `code` command. Empty means look in PATH, then
	// in /Applications/Visual Studio Code.app/Contents/Resources/app/bin.
	Code string
	// TokenDir is where to write the connection token file, readable only by
	// the user (0600 in a 0700 folder). Empty means a new temporary folder,
	// removed by Stop.
	TokenDir string
	// Port is the port to use. 0 means the server picks a free one as it
	// starts listening, and Start reads it from the address it prints: a
	// port Bench picked could be taken in the moment before the server
	// listened on it. A code from before VS Code 1.103 doesn't print the
	// port it picked, so for that Start asks for a port that's free now.
	Port int
	// Record is a file to record the running server in, such as
	// vscode.json in Bench's support folder, so that StopRecorded can stop
	// it if Bench crashes. Stop removes it. Empty means no record.
	Record string
}

// Server is a running `code serve-web`. Its methods are safe for concurrent
// use.
type Server struct {
	cmd       *exec.Cmd
	host      string // the host the server printed, or ""
	port      int
	token     string
	tokenFile string
	tempDir   string // a folder Start made for the token, or ""
	record    string // Options.Record, once written
	output    *os.File
	lines     *lineLog
	done      chan struct{} // closed when the process has ended
	readDone  chan struct{} // closed when the output has all been read
	// leftGroup is whether the server's process group still had members
	// when the process Start ran was reaped. If it hadn't, the group's ID
	// is free for another group to use, so Stop mustn't signal it. It's
	// set before done is closed.
	leftGroup bool

	stopOnce sync.Once
	stopErr  error
}

// Start writes a random token to a file only the user can read, runs `code
// serve-web --host 127.0.0.1 --port <port> --connection-token-file <file>
// --accept-server-license-terms`, records it in Options.Record, and waits
// (up to 30 s, or until ctx ends) for it to print the address it serves,
// which gives its port if Options.Port is 0. It never uses
// --without-connection-token: the server gives full access to files and
// terminals.
//
// The server runs in a process group of its own. ctx only limits the wait:
// once Start returns, the server runs until Stop.
func Start(ctx context.Context, opts Options) (*Server, error) {
	code, err := findCode(opts.Code)
	if err != nil {
		return nil, fmt.Errorf("start VS Code for the Web: %w", err)
	}
	s, err := start(ctx, code, opts)
	if err == nil && s.port == 0 {
		// Before VS Code 1.103, code printed the port it was asked for,
		// not the one it picked. So stop it, and ask for a port that's
		// free now instead. Another process could take that port first,
		// but then this start fails, and Bench tries again later.
		_ = s.Stop()
		if opts.Port, err = freePort(); err == nil {
			s, err = start(ctx, code, opts)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("start VS Code for the Web: %w", err)
	}
	return s, nil
}

// start starts the server, records it, and waits for it to be ready.
func start(ctx context.Context, code string, opts Options) (*Server, error) {
	if err := ctx.Err(); err != nil {
		// Don't start a server only to stop it straight away.
		return nil, err
	}
	s, err := launch(code, opts)
	if err != nil {
		return nil, err
	}
	if opts.Record != "" {
		if err := s.writeRecord(opts.Record); err != nil {
			_ = s.Stop()
			return nil, err
		}
	}
	if err := s.waitReady(ctx); err != nil {
		_ = s.Stop()
		return nil, err
	}
	return s, nil
}

// launch writes the token and starts the server, without waiting for it.
func launch(code string, opts Options) (s *Server, err error) {
	s = &Server{
		port:     opts.Port,
		lines:    newLineLog(keepLines),
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
	}
	defer func() {
		if err != nil {
			s.removeToken()
		}
	}()
	if err := s.writeToken(opts.TokenDir); err != nil {
		return nil, err
	}

	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("make a pipe for the server's output: %w", err)
	}
	s.output = r
	s.cmd = exec.Command(code, "serve-web",
		"--host", listenHost,
		"--port", strconv.Itoa(opts.Port),
		"--connection-token-file", s.tokenFile,
		"--accept-server-license-terms")
	s.cmd.Stdout = w
	s.cmd.Stderr = w
	s.cmd.SysProcAttr = ownProcessGroup()
	err = s.cmd.Start()
	_ = w.Close()
	if err != nil {
		_ = r.Close()
		return nil, fmt.Errorf("run %s serve-web: %w", code, err)
	}
	pid := s.cmd.Process.Pid
	go func() {
		_ = s.cmd.Wait()
		s.leftGroup = groupAlive(pid)
		close(s.done)
	}()
	return s, nil
}

// addrPattern finds addresses in the server's output.
var addrPattern = regexp.MustCompile(`https?://[^\s\x00-\x1f"'<>]+`)

// address is a host and port the server printed. Either can be empty or 0.
type address struct {
	host string
	port int
}

// waitReady reads the server's output until it prints a local address, then
// keeps reading in the background so that the server never blocks writing.
func (s *Server) waitReady(ctx context.Context) error {
	ready := make(chan address, 1)
	go s.readOutput(ready)

	timer := time.NewTimer(startTimeout)
	defer timer.Stop()
	select {
	case a := <-ready:
		if a.port != 0 {
			s.port = a.port
		}
		s.host = a.host
		return nil
	case <-s.done:
		// Let the reader catch up, so the error can quote the last
		// thing the server said.
		select {
		case <-s.readDone:
		case <-time.After(drainWait):
		}
		return fmt.Errorf("code serve-web ended before it was ready: %s: %s",
			s.cmd.ProcessState, s.lines.summary())
	case <-ctx.Done():
		return fmt.Errorf("code serve-web wasn't ready: %w", ctx.Err())
	case <-timer.C:
		return fmt.Errorf("code serve-web didn't print its address within %v: %s", startTimeout, s.lines.summary())
	}
}

// readOutput reads the server's output line by line, keeping the last few
// lines, and sends the first local address it sees.
func (s *Server) readOutput(ready chan<- address) {
	defer close(s.readDone)
	br := bufio.NewReaderSize(s.output, maxLine)
	found := false
	var line []byte
	for {
		frag, isPrefix, err := br.ReadLine()
		if len(line)+len(frag) <= maxLine {
			line = append(line, frag...)
		}
		if err != nil {
			return
		}
		if isPrefix {
			continue
		}
		text := string(line)
		line = line[:0]
		s.lines.add(text)
		if found {
			continue
		}
		if host, port, ok := localAddress(text, s.port); ok {
			found = true
			ready <- address{host: host, port: port}
		}
	}
}

// localAddress reports whether a line of output holds an address on this
// Mac, and returns its host and its port (0 if it has none). Other
// addresses, such as the licence's, don't count.
func localAddress(line string, port int) (string, int, bool) {
	for _, m := range addrPattern.FindAllString(line, -1) {
		m = strings.TrimRight(m, ".,;:)]")
		if strings.Contains(m, "[") && !strings.Contains(m, "]") {
			m += "]" // It closed an IPv6 host, as in http://[::1].
		}
		u, err := url.Parse(m)
		if err != nil {
			continue
		}
		host := u.Hostname()
		p, _ := strconv.Atoi(u.Port())
		switch host {
		case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::":
			return host, p, true
		}
		if p != 0 && p == port {
			return host, p, true
		}
	}
	return "", 0, false
}

// URL returns the address of VS Code for the Web with a folder open:
// http://127.0.0.1:<port>/?tkn=<token>&folder=<absolute path>, with "~"
// expanded and the folder query-escaped. A relative folder is taken to be in
// the home folder. An empty folder leaves out &folder=.
//
// The host is the loopback address the server listens on: [::1] if it
// printed an IPv6 address, and otherwise 127.0.0.1. It's never localhost,
// which macOS looks up as ::1 first: the server isn't listening there, so
// any other process could, and it would get the token.
func (s *Server) URL(folder string) string {
	u := "http://" + net.JoinHostPort(loopbackHost(s.host), strconv.Itoa(s.port)) +
		"/?tkn=" + url.QueryEscape(s.token)
	if folder == "" {
		return u
	}
	return u + "&folder=" + url.QueryEscape(absFolder(folder))
}

// loopbackHost returns the loopback address to load a server at, given the
// host it printed.
func loopbackHost(printed string) string {
	switch printed {
	case "::1", "::":
		return "::1"
	}
	return listenHost
}

// Port returns the port the server listens on.
func (s *Server) Port() int { return s.port }

// Done is closed when the server's process has ended, whether by Stop or
// not.
func (s *Server) Done() <-chan struct{} { return s.done }

// Stop ends the server and removes its token file and its record. It sends
// SIGTERM to the server's process group, then SIGKILL to whatever is left of
// the group after 3 seconds. It signals nothing if the group had already
// emptied when the server ended. It's safe to call more than once.
func (s *Server) Stop() error {
	s.stopOnce.Do(func() {
		s.stopErr = s.stop()
	})
	return s.stopErr
}

func (s *Server) stop() error {
	if s.cmd != nil && s.cmd.Process != nil && s.groupRunning() {
		pid := s.cmd.Process.Pid
		groupSignal(pid, false)
		// code is a wrapper that starts the real server, so wait for the
		// whole group, not only the process Start ran.
		s.waitEnded(pid, stopGrace)
		// Only if something is left: once the group has emptied, its ID is
		// free for another process to use.
		if s.groupRunning() {
			groupSignal(pid, true)
			s.waitEnded(pid, killWait)
		}
	}
	if s.output != nil {
		_ = s.output.Close()
	}
	return errors.Join(s.removeToken(), s.removeRecord())
}

// ended reports whether the process Start ran has ended.
func (s *Server) ended() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// groupRunning reports whether the server's process group may still have
// members: the process Start ran hasn't been reaped, so its ID, which is the
// group's, can't have been reused, or the group still had members when it
// was, and still has. A group that had emptied by then may since have been
// given to another process, which Stop mustn't signal.
func (s *Server) groupRunning() bool {
	if !s.ended() {
		return true
	}
	return s.leftGroup && groupAlive(s.cmd.Process.Pid)
}

// groupPoll is how often Stop checks whether the process group has ended.
const groupPoll = 20 * time.Millisecond

// waitEnded waits up to limit for the process Start ran to end and for its
// process group, led by pid, to empty.
func (s *Server) waitEnded(pid int, limit time.Duration) {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	select {
	case <-s.done:
	case <-deadline.C:
		return
	}
	tick := time.NewTicker(groupPoll)
	defer tick.Stop()
	for s.leftGroup && groupAlive(pid) {
		select {
		case <-tick.C:
		case <-deadline.C:
			return
		}
	}
}

// writeToken writes a new random token to a file only the user can read.
func (s *Server) writeToken(dir string) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	if dir == "" {
		if dir, err = os.MkdirTemp("", tempDirPrefix); err != nil {
			return fmt.Errorf("make a folder for the connection token: %w", err)
		}
		s.tempDir = dir
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("make a folder for the connection token: %w", err)
	}
	f, err := os.CreateTemp(dir, tokenPrefix)
	if err != nil {
		return fmt.Errorf("make the connection token file: %w", err)
	}
	s.tokenFile = f.Name()
	s.token = token
	if err := writeAll(f, token); err != nil {
		return fmt.Errorf("write the connection token file: %w", err)
	}
	return nil
}

// writeAll makes f readable only by its owner, writes text to it, and closes
// it.
func writeAll(f *os.File, text string) error {
	err := f.Chmod(0o600)
	if err == nil {
		_, err = io.WriteString(f, text)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// removeToken removes the token file, and the folder Start made for it.
func (s *Server) removeToken() error {
	var err error
	if s.tokenFile != "" {
		if rerr := os.Remove(s.tokenFile); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			err = fmt.Errorf("remove the connection token file: %w", rerr)
		}
	}
	if s.tempDir != "" {
		if rerr := os.RemoveAll(s.tempDir); rerr != nil && err == nil {
			err = fmt.Errorf("remove the connection token folder: %w", rerr)
		}
	}
	return err
}

// newToken returns 32 random bytes as hex.
func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("make a connection token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// findCode returns the code command to run: path if given, then code in
// PATH, then the one in the VS Code app.
func findCode(path string) (string, error) {
	if path != "" {
		if !isExecutable(path) {
			return "", fmt.Errorf("%s isn't a program that can run", path)
		}
		return path, nil
	}
	if p, err := lookPath("code"); err == nil {
		return p, nil
	}
	if isExecutable(appCode) {
		return appCode, nil
	}
	return "", ErrNoCode
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// freePort returns a TCP port on the loopback interface that's free now.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find a free port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// absFolder expands "~" and makes folder absolute. A relative folder is
// taken to be in the home folder, as it is for terminals: Bench's own working
// folder means nothing to the user (an app opened from the Finder runs in /).
func absFolder(folder string) string {
	if home, err := os.UserHomeDir(); err == nil {
		switch {
		case folder == "~" || strings.HasPrefix(folder, "~/"):
			folder = filepath.Join(home, folder[1:])
		case !filepath.IsAbs(folder):
			folder = filepath.Join(home, folder)
		}
	}
	if abs, err := filepath.Abs(folder); err == nil {
		return abs
	}
	return filepath.Clean(folder)
}

// lineLog keeps the last few lines of output. It's safe for concurrent use.
type lineLog struct {
	mu    sync.Mutex
	keep  int
	lines []string
}

func newLineLog(keep int) *lineLog { return &lineLog{keep: keep} }

func (l *lineLog) add(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, line)
	if len(l.lines) > l.keep {
		l.lines = l.lines[len(l.lines)-l.keep:]
	}
}

// summary returns the kept lines on one line, or a note that there were
// none.
func (l *lineLog) summary() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var parts []string
	for _, line := range l.lines {
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, line)
		}
	}
	if len(parts) == 0 {
		return "it printed nothing"
	}
	return strings.Join(parts, " / ")
}
