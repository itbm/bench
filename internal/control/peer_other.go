//go:build unix && !darwin && !linux

package control

import "net"

// peerUID can't say who is at the other end of c on this system, where the
// socket's mode and its folder's alone keep other users out.
func peerUID(*net.UnixConn) (uid int, checked bool, err error) {
	return 0, false, nil
}
