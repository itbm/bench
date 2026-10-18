//go:build (darwin || server) && unix

package ui

import "syscall"

// ownProcessGroup starts a process in a new process group.
func ownProcessGroup() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
