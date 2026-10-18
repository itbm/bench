//go:build darwin

package term

import (
	"bytes"
	"unsafe"

	"golang.org/x/sys/unix"
)

// processName returns the name of process pid, as the kernel records it
// (p_comm, up to 16 characters).
func processName(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || info == nil || info.Proc.P_pid != int32(pid) {
		return "", false
	}
	comm := info.Proc.P_comm[:]
	if i := bytes.IndexByte(comm, 0); i >= 0 {
		comm = comm[:i]
	}
	return string(comm), len(comm) > 0
}

// Arguments and sizes from <sys/proc_info.h>, for asking the kernel for a
// process's current folder the way libproc's proc_pidinfo does, without cgo.
const (
	// procInfoCallPIDInfo is PROC_INFO_CALL_PIDINFO.
	procInfoCallPIDInfo = 2
	// procPIDVnodePathInfo is PROC_PIDVNODEPATHINFO: the current and root
	// folders, as a struct proc_vnodepathinfo.
	procPIDVnodePathInfo = 9
	// vnodeInfoSize is sizeof(struct vnode_info), which comes before the
	// path in a struct vnode_info_path.
	vnodeInfoSize = 152
	// vnodeInfoPathSize is sizeof(struct vnode_info_path): a vnode_info and
	// a MAXPATHLEN (1,024-byte) path.
	vnodeInfoPathSize = vnodeInfoSize + 1024
	// vnodePathInfoSize is sizeof(struct proc_vnodepathinfo): the current
	// folder's vnode_info_path, then the root folder's.
	vnodePathInfoSize = 2 * vnodeInfoPathSize
)

// processDir returns the current folder of process pid, from the proc_info
// system call that libproc's proc_pidinfo(PROC_PIDVNODEPATHINFO) wraps. tmux
// finds a pane's folder on macOS the same way.
func processDir(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	var info [vnodePathInfoSize]byte
	n, _, errno := unix.Syscall6(unix.SYS_PROC_INFO, procInfoCallPIDInfo, uintptr(pid),
		procPIDVnodePathInfo, 0, uintptr(unsafe.Pointer(&info[0])), vnodePathInfoSize)
	if errno != 0 || n != vnodePathInfoSize {
		return "", false
	}
	return absCString(info[vnodeInfoSize:vnodeInfoPathSize])
}
