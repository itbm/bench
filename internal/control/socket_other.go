//go:build !unix

package control

import "errors"

// SocketFile is the name of the socket in Bench's support folder that
// bench list asks.
const SocketFile = "bench.sock"

// ErrNotServing is Query's error when nothing answers at the socket.
var ErrNotServing = errors.New("nothing answers at Bench's socket")

// errNoSockets is the error on a system without Unix sockets.
var errNoSockets = errors.New("bench list needs macOS")

// Server would answer bench list; this system has no Unix sockets.
type Server struct{}

// Serve fails on this system.
func Serve(string, func() (List, error)) (*Server, error) { return nil, errNoSockets }

// Path returns "".
func (*Server) Path() string { return "" }

// Close does nothing.
func (*Server) Close() error { return nil }

// Query fails on this system.
func Query(string) (List, error) { return List{}, errNoSockets }
