//go:build unix

package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// SocketFile is the name of the socket in Bench's support folder that
// bench list asks. It's there only while Bench runs with the allow_scripts
// setting on.
const SocketFile = "bench.sock"

// nextFile is the name that Serve makes the socket under, before it takes
// SocketFile. It's no longer than SocketFile, so that the check of the
// socket's path against maxSocketPath covers it too.
const nextFile = "bench.new"

// socketPerm is the socket's mode: only the user can connect to it, which
// takes write permission.
const socketPerm = 0o600

// maxSocketPath is the longest path that a Unix socket can have on macOS,
// whose limit (104 bytes, with the zero at the end) is the smaller one.
// Bench refuses a longer one on Linux too, so that the preview fails as the
// app would.
const maxSocketPath = 103

// Request lines that the socket answers.
const listRequest = "list"

// ioWait is how long either end waits for the other to send its line.
const ioWait = 2 * time.Second

// ErrNotServing is Query's error when nothing answers at the socket: Bench
// isn't running, or the allow_scripts setting is off.
var ErrNotServing = errors.New("nothing answers at Bench's socket")

// Server answers bench list on the socket, until Close.
type Server struct {
	path   string
	info   fs.FileInfo // The socket file that Serve made, for Close.
	ln     *net.UnixListener
	answer func() (List, error)
	uid    int
	wg     sync.WaitGroup
	once   sync.Once
}

// Serve answers bench list on the socket in dir, Bench's support folder,
// with what answer returns, until Close. The caller has checked dir, which
// only the user can open (config.CheckDir). Only the user can connect to
// the socket: its mode is 0600, it's made under another name and given
// that mode before it takes its own name, and each connection's other end
// must be the user's, where the system says (macOS and Linux). A socket
// there already that nothing answers, as a Bench that crashed leaves, is
// replaced; one that something answers, or a file that isn't a socket, is
// left alone and refused.
func Serve(dir string, answer func() (List, error)) (*Server, error) {
	path := filepath.Join(dir, SocketFile)
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("the path of Bench's socket, %s, is longer than macOS allows (%d bytes): use a support folder with a shorter path", path, maxSocketPath)
	}
	if err := clearStale(path); err != nil {
		return nil, err
	}
	next := filepath.Join(dir, nextFile)
	if err := clearStale(next); err != nil {
		return nil, err
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: next, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("making Bench's socket: %w", err)
	}
	// Serve removes the socket itself, by its own name, in Close.
	ln.SetUnlinkOnClose(false)
	fail := func(err error) (*Server, error) {
		_ = ln.Close()
		_ = os.Remove(next)
		return nil, err
	}
	if err := os.Chmod(next, socketPerm); err != nil {
		return fail(fmt.Errorf("making Bench's socket private: %w", err))
	}
	if err := os.Rename(next, path); err != nil {
		return fail(fmt.Errorf("making Bench's socket: %w", err))
	}
	info, err := os.Lstat(path)
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("making Bench's socket: %w", err)
	}
	s := &Server{path: path, info: info, ln: ln, answer: answer, uid: os.Getuid()}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}

// clearStale removes the socket at path if nothing answers there, and
// refuses one that something answers, or a file that isn't a socket. It
// does nothing if there's nothing at path.
func clearStale(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking Bench's socket: %w", err)
	}
	if info.Mode()&fs.ModeSocket == 0 {
		return fmt.Errorf("there's a file at %s that isn't Bench's socket: move it, and Bench will try again", path)
	}
	if c, err := net.DialTimeout("unix", path, ioWait); err == nil {
		_ = c.Close()
		return fmt.Errorf("another Bench answers at %s", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing an old socket: %w", err)
	}
	return nil
}

// Path returns the socket's path.
func (s *Server) Path() string { return s.path }

// accept answers each connection, until the listener closes.
func (s *Server) accept() {
	defer s.wg.Done()
	for {
		c, err := s.ln.AcceptUnix()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			// Out of file descriptors, say: try again shortly.
			time.Sleep(50 * time.Millisecond)
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serve(c)
		}()
	}
}

// serve answers one connection: one request line, then one reply line.
func (s *Server) serve(c *net.UnixConn) {
	defer c.Close()
	if uid, checked, err := peerUID(c); err != nil || (checked && uid != s.uid) {
		return
	}
	_ = c.SetDeadline(time.Now().Add(ioWait))
	line, err := bufio.NewReader(&limited{c, 64}).ReadString('\n')
	if err != nil {
		return
	}
	var reply any
	switch line {
	case listRequest + "\n":
		l, err := s.answer()
		if err != nil {
			reply = errorReply{Error: err.Error()}
		} else {
			reply = l
		}
	default:
		reply = errorReply{Error: "Bench doesn't know that request"}
	}
	_ = json.NewEncoder(c).Encode(reply)
}

// errorReply is the socket's reply when it can't answer.
type errorReply struct {
	Error string `json:"error"`
}

// limited reads at most n bytes from r, then gives io.EOF.
type limited struct {
	r interface{ Read([]byte) (int, error) }
	n int
}

func (l *limited) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errors.New("the request is too long")
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= n
	return n, err
}

// Close stops answering, waits for the replies on their way, and removes
// the socket, if it's still the one that Serve made. It can be called more
// than once.
func (s *Server) Close() error {
	var err error
	s.once.Do(func() {
		err = s.ln.Close()
		s.wg.Wait()
		if info, lerr := os.Lstat(s.path); lerr == nil && os.SameFile(info, s.info) {
			if rerr := os.Remove(s.path); rerr != nil && err == nil {
				err = rerr
			}
		}
	})
	return err
}

// Query asks the Bench whose support folder is dir for its List, over its
// socket. It returns ErrNotServing if nothing answers there, and the
// socket's own error if Bench can't answer. Nothing answers if there's
// nothing at the path, a socket that nothing listens on, or a file that
// isn't a socket: Linux refuses the connection to that file, and macOS
// says that it isn't a socket (ENOTSOCK).
func Query(dir string) (List, error) {
	path := filepath.Join(dir, SocketFile)
	c, err := net.DialTimeout("unix", path, ioWait)
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOTSOCK) {
		return List{}, ErrNotServing
	}
	if err != nil {
		return List{}, fmt.Errorf("asking Bench: %w", err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(ioWait))
	if _, err := c.Write([]byte(listRequest + "\n")); err != nil {
		return List{}, fmt.Errorf("asking Bench: %w", err)
	}
	var reply struct {
		List
		Error string `json:"error"`
	}
	if err := json.NewDecoder(c).Decode(&reply); err != nil {
		return List{}, fmt.Errorf("reading Bench's answer: %w", err)
	}
	if reply.Error != "" {
		return List{}, errors.New(reply.Error)
	}
	return reply.List, nil
}
