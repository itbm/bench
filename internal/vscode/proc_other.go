//go:build !unix

package vscode

import (
	"os"
	"syscall"
)

// ownProcessGroup does nothing where there are no Unix process groups.
func ownProcessGroup() *syscall.SysProcAttr { return nil }

// groupAlive reports false: there are no process groups to wait for here,
// only the process itself.
func groupAlive(pid int) bool { return false }

// signalGroup ends the process pid; there are no Unix process groups here.
func signalGroup(pid int, kill bool) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
