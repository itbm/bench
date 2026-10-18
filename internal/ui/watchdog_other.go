//go:build server && !unix

package ui

import "syscall"

// ownProcessGroup does nothing where there are no Unix process groups.
func ownProcessGroup() *syscall.SysProcAttr { return nil }
