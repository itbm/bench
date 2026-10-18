//go:build darwin

package vscode

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// From <sys/proc.h>.
const (
	// szomb is SZOMB: the process has ended, but its parent hasn't
	// collected it yet.
	szomb = 5
	// pWexit is P_WEXIT: the process is ending. It's set before the
	// process's files close, so before the watchdog's pipe does.
	pWexit = 0x00002000
)

// lookUpProcess returns what the kernel says about process pid: when it
// started (p_starttime, to the microsecond) and whether it has ended or is
// ending. It returns false if there's no such process.
func lookUpProcess(pid int) (process, bool) {
	if pid <= 0 {
		return process{}, false
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || info == nil || info.Proc.P_pid != int32(pid) {
		return process{}, false
	}
	t := info.Proc.P_starttime
	return process{
		started: fmt.Sprintf("%d.%06d", t.Sec, t.Usec),
		ended:   info.Proc.P_stat == szomb || info.Proc.P_flag&pWexit != 0,
	}, true
}
