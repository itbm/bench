package control

import (
	"net"

	"golang.org/x/sys/unix"
)

// peerUID returns the user ID of the process at the other end of c, as the
// system says (LOCAL_PEERCRED).
func peerUID(c *net.UnixConn) (uid int, checked bool, err error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, false, err
	}
	var cred *unix.Xucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil {
		return 0, false, err
	}
	if credErr != nil {
		return 0, false, credErr
	}
	return int(cred.Uid), true, nil
}
