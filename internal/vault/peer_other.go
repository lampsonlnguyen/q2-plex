//go:build !linux

package vault

import (
	"errors"
	"net"
)

// osPeerUID fails closed: without SO_PEERCRED the client cannot tell who owns
// the socket, so every call that reaches the socket returns ErrNotVault (a
// missing install still gives ErrNoVault first). The Vault runs only on the Q2
// (Linux). Tests replace peerUID to simulate root and non-root servers.
func osPeerUID(*net.UnixConn) (uint32, error) {
	return 0, errors.New("vault: peer credentials need Linux")
}
