//go:build darwin || server

package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoggerDropsRefusals(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		logged bool
	}{
		{"refusal", refusalPrefix + "A pane can't be split by its only window.", false},
		{"panic", refusalPrefix + "github.com/itbm/bench/internal/ui.Service.Ready: panic: oops", true},
		{"other error", "failed to call binding: could not parse argument #0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			newLogger(&buf).With("window", 1).Error(tt.msg)
			if got := strings.Contains(buf.String(), "msg="); got != tt.logged {
				t.Errorf("logged %q = %v, want %v", buf.String(), got, tt.logged)
			}
		})
	}
	var buf bytes.Buffer
	newLogger(&buf).Info("Server mode starting")
	if buf.Len() != 0 {
		t.Errorf("logged %q at info level, want only warnings and errors", buf.String())
	}
}
