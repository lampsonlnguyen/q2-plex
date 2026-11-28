//go:build linux

package vault

import (
	"errors"
	"net"
	"os"
	"testing"
)

// The server here is this test process: its uid is the peer's uid. A normal
// user is not root, so the real SO_PEERCRED check must refuse it.
func TestLinuxRealPeerCheckRefusesNonRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root")
	}
	sock, _ := setup(t)
	peerUID = osPeerUID
	f := serve(t, sock, "Q2VAULT/1\nok\nx")
	if err := Put([]byte("password")); !errors.Is(err, ErrNotVault) {
		t.Fatalf("put: %v", err)
	}
	if _, err := Get(); !errors.Is(err, ErrNotVault) {
		t.Fatalf("get: %v", err)
	}
	for _, r := range f.requests() {
		if r != "" {
			t.Fatalf("server received %q", r)
		}
	}
}

func TestLinuxPeerUIDIsOurs(t *testing.T) {
	sock, _ := setup(t)
	ln, _ := net.Listen("unix", sock)
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			defer c.Close()
			select {}
		}
	}()
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	uid, err := osPeerUID(c.(*net.UnixConn))
	if err != nil || int(uid) != os.Getuid() {
		t.Fatalf("uid %d err %v", uid, err)
	}
}
