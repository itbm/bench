// Package vscode runs `code serve-web`, so that VS Code for the Web can be a
// real embedded Web item.
package vscode

import "context"

// This file fixes the package's exported API so that other packages can build
// against it. Delete it once everything is implemented in its own file.

// Options start a server.
type Options struct {
	// Code is the path to the `code` command. Empty means look in PATH, then
	// in /Applications/Visual Studio Code.app/Contents/Resources/app/bin.
	Code string
	// TokenDir is where to write the connection token file, readable only by
	// the user (0600 in a 0700 folder).
	TokenDir string
	// Port is the port to use; 0 means pick a free one.
	Port int
}

// Server is a running `code serve-web`.
type Server struct{}

// Start picks a free port, writes a random token to a file only the user can
// read, runs `code serve-web --port <port> --connection-token-file <file>
// --accept-server-license-terms`, and waits (up to 30 s, or until ctx ends)
// for it to print the address it serves. It never uses
// --without-connection-token: the server gives full access to files and
// terminals.
func Start(ctx context.Context, opts Options) (*Server, error) { panic("not implemented") }

// URL returns the address of VS Code for the Web with a folder open:
// http://localhost:<port>/?tkn=<token>&folder=<absolute path>, with "~"
// expanded and the folder query-escaped.
func (s *Server) URL(folder string) string { panic("not implemented") }

// Stop ends the server and removes its token file.
func (s *Server) Stop() error { panic("not implemented") }
