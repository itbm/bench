//go:build unix

package vscode

import "syscall"

// ownProcessGroup starts a process in a new process group, so that Stop can
// end it and everything it starts.
func ownProcessGroup() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// signalGroup sends SIGTERM, or SIGKILL if kill is true, to the process
// group led by pid. A group that has already gone is fine.
func signalGroup(pid int, kill bool) {
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-pid, sig)
}

// groupAlive reports whether the process group led by pid still has
// members. Once the leader has been reaped, its ID can't be reused while the
// group has members, so this can't mistake another group for it.
func groupAlive(pid int) bool {
	return syscall.Kill(-pid, 0) == nil
}
