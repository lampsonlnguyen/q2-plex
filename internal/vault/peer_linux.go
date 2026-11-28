//go:build linux

package vault

import (
	"errors"
	"net"
	"syscall"
)

// osPeerUID reads the peer's uid with SO_PEERCRED.
func osPeerUID(c *net.UnixConn) (uint32, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var cred *syscall.Ucred
	var serr error
	if err := rc.Control(func(fd uintptr) {
		cred, serr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if serr != nil {
		return 0, serr
	}
	if cred == nil {
		return 0, errors.New("no credentials")
	}
	return cred.Uid, nil
}
