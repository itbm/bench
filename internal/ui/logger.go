//go:build darwin || server

package ui

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// logOutput is where Wails' messages and Bench's own warnings go. Tests
// replace it.
var logOutput io.Writer = os.Stderr

// refusalPrefix starts what Wails logs when a bound method returns an error.
// For Bench, that's a refused action, which the frontend shows as a toast,
// so it isn't worth logging. Panics in bound methods are logged in the same
// way but mention the panic, and still are.
const refusalPrefix = "Binding call failed: Bound method returned an error: "

// newLogger returns the logger for Wails' own messages: warnings and errors
// on w, without refused actions.
func newLogger(w io.Writer) *slog.Logger {
	inner := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelWarn})
	return slog.New(quietHandler{inner})
}

// quietHandler drops refused actions.
type quietHandler struct{ slog.Handler }

// Handle passes on a record unless it's a refused action.
func (h quietHandler) Handle(ctx context.Context, r slog.Record) error {
	if strings.HasPrefix(r.Message, refusalPrefix) && !strings.Contains(r.Message, ": panic: ") {
		return nil
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs keeps the handler quiet.
func (h quietHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return quietHandler{h.Handler.WithAttrs(attrs)}
}

// WithGroup keeps the handler quiet.
func (h quietHandler) WithGroup(name string) slog.Handler {
	return quietHandler{h.Handler.WithGroup(name)}
}
