// Copied from https://github.com/js-surya/q2-vault/tree/main/vault (MIT): keep it in step with that.

// Package vault is a small client for the Q2 Vault, the root card app that
// keeps each card app's login sealed on the card and serves it over
// /tmp/q2-vault.sock. It uses only the standard library and imports nothing
// from the rest of q2-vault, so an app maker can copy this directory as is.
//
// The client works only on Linux (the Q2): elsewhere every call that reaches
// the socket returns ErrNotVault, because it cannot check who owns the socket
// (a missing install still gives ErrNoVault first).
//
// How an app uses it:
//
//	secret, err := vault.Get()
//	switch {
//	case err == nil:
//	    // sign in with secret
//	case errors.Is(err, vault.ErrNone):
//	    // nothing saved: sign in, then vault.Put(token)
//	case errors.Is(err, vault.ErrCardAway):
//	    // card not mounted: keep what you hold in memory, ask again later
//	case errors.Is(err, vault.ErrNoVault), errors.Is(err, vault.ErrUnavailable):
//	    // no Vault (or not answering): use your own file, as before
//	case errors.Is(err, vault.ErrNotVault):
//	    // something else owns the socket: use your own plain file, but warn,
//	    // and never migrate into it or delete the plain file
//	case errors.Is(err, vault.ErrRefused), errors.Is(err, vault.ErrTooLarge),
//	    errors.Is(err, vault.ErrBadRequest):
//	    // a bug (or an app the Vault does not know): don't retry
//	}
//
// Fallback: with no Vault installed (ErrNoVault) keep using your own plain
// file. Get, Put and Delete answer ErrNoVault at once when the Vault is not
// installed. When it is installed but the socket does not answer, they retry
// every 250 ms for up to 5 s while its launcher runs (/tmp/q2-vault names a
// live pid: it may be starting), then return ErrNoVault; with no launcher
// running they return ErrNoVault at once.
//
// ErrNoVault while the Vault is installed means "not now": the Vault is
// closed (on firmware without needs=, it starts with Wi-Fi or its row). Keep
// your plain file, don't move it in, and if you meant to Delete (Sign Out),
// try the Delete again later, so no login stays sealed on the card.
//
// Moving an old plain file in: at start, when a Vault is there and an old
// plain login file exists, Put its contents, Get the secret back and compare
// exact bytes with the file. Delete the plain file only after Put, Get and
// the compare all succeed. If anything fails, leave the plain file alone.
//
// Advice:
//   - Keep tokens, not passwords.
//   - Sign Out must call Delete.
//   - Call prctl(PR_SET_DUMPABLE, 0) at start, so other uid 1000 apps cannot
//     read your memory.
//
// Before it sends or trusts anything, the client checks that the socket's
// owner is root (uid 0), so an app that listens on the socket path while the
// Vault is closed collects nothing: that returns ErrNotVault.
package vault

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrNoVault: the Vault is not installed, or not running (no live launcher),
	// or never answered within the retry. With the Vault installed it means
	// "not now": keep your plain file, don't migrate, retry a Delete later.
	ErrNoVault = errors.New("vault: no Vault")
	// ErrNone: the app has no saved login.
	ErrNone = errors.New("vault: nothing saved")
	// ErrRefused: the Vault does not recognise the caller as a card app.
	ErrRefused = errors.New("vault: refused")
	// ErrCardAway: the card is not mounted, so the Vault cannot read or write.
	ErrCardAway = errors.New("vault: card away")
	// ErrTooLarge: the secret is over 16 KB.
	ErrTooLarge = errors.New("vault: too large")
	// ErrBadRequest: a malformed request, or an empty secret.
	ErrBadRequest = errors.New("vault: bad request")
	// ErrNotVault: the socket's owner is not root, so nothing was sent.
	ErrNotVault = errors.New("vault: the socket's owner is not root")
	// ErrUnavailable: the connection closed with no (or an unreadable) answer;
	// a temporary failure, try again later.
	ErrUnavailable = errors.New("vault: temporarily unavailable")
)

// Overridable in tests.
var (
	Socket    = "/tmp/q2-vault.sock"
	Installed = "/mnt/mmc/.vault/q2-vault"
	// PidFile is where the Vault's launcher keeps its pid while it runs; ProcRoot is /proc.
	PidFile  = "/tmp/q2-vault"
	ProcRoot = "/proc"

	retryEvery = 250 * time.Millisecond
	retryFor   = 5 * time.Second

	requestTimeout = 10 * time.Second

	// peerUID reports the uid of the process at the other end of c.
	peerUID = osPeerUID
)

const (
	magic   = "Q2VAULT/1\n"
	maxBody = 16384
)

// Get returns the app's saved secret.
func Get() ([]byte, error) { return do("get", nil) }

// Put seals b (1 byte to 16 KB) as the app's secret.
func Put(b []byte) error {
	if len(b) == 0 {
		return ErrBadRequest
	}
	if len(b) > maxBody {
		return ErrTooLarge
	}
	_, err := do("put", b)
	return err
}

// Delete removes the app's saved secret; it succeeds whether or not one existed.
func Delete() error {
	_, err := do("delete", nil)
	return err
}

func do(op string, body []byte) ([]byte, error) {
	if _, err := os.Stat(Installed); err != nil {
		return nil, ErrNoVault
	}
	deadline := time.Now().Add(retryFor)
	for {
		c, err := net.DialTimeout("unix", Socket, requestTimeout)
		if err == nil {
			return exchange(c.(*net.UnixConn), op, body)
		}
		if !launcherRunning() {
			return nil, ErrNoVault // nothing is starting it: not now
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil, ErrNoVault
		}
		time.Sleep(min(retryEvery, left))
	}
}

// launcherRunning reports whether PidFile holds a decimal pid whose ProcRoot/<pid> exists: the
// Vault's launcher is running, so the Vault may be starting.
func launcherRunning() bool {
	b, err := os.ReadFile(PidFile)
	if err != nil {
		return false
	}
	pid := strings.TrimSpace(string(b))
	if pid == "" || len(pid) > 10 {
		return false
	}
	for _, r := range pid {
		if r < '0' || r > '9' {
			return false
		}
	}
	_, err = os.Stat(filepath.Join(ProcRoot, pid))
	return err == nil
}

func exchange(c *net.UnixConn, op string, body []byte) ([]byte, error) {
	defer c.Close()
	uid, err := peerUID(c)
	if err != nil || uid != 0 {
		return nil, ErrNotVault
	}
	c.SetDeadline(time.Now().Add(requestTimeout))
	req := append([]byte(magic+op+"\n"), body...)
	if _, err := c.Write(req); err != nil {
		return nil, ErrUnavailable
	}
	if err := c.CloseWrite(); err != nil {
		return nil, ErrUnavailable
	}
	ans, err := io.ReadAll(io.LimitReader(c, maxBody+256+1))
	if err != nil || len(ans) == 0 || len(ans) > maxBody+256 {
		return nil, ErrUnavailable
	}
	out, err := parse(ans)
	if err == nil && op == "get" && len(out) == 0 {
		// the Vault never stores an empty secret
		return nil, ErrUnavailable
	}
	return out, err
}

func parse(ans []byte) ([]byte, error) {
	if len(ans) < len(magic) || string(ans[:len(magic)]) != magic {
		return nil, ErrUnavailable
	}
	rest := ans[len(magic):]
	i := 0
	for i < len(rest) && rest[i] != '\n' {
		i++
	}
	if i == len(rest) {
		return nil, ErrUnavailable
	}
	status, payload := string(rest[:i]), rest[i+1:]
	switch status {
	case "ok":
		if len(payload) > maxBody {
			return nil, ErrUnavailable
		}
		return payload, nil
	case "none":
		return nil, ErrNone
	case "refused":
		return nil, ErrRefused
	case "too-large":
		return nil, ErrTooLarge
	case "card-away":
		return nil, ErrCardAway
	case "bad-request":
		return nil, ErrBadRequest
	}
	return nil, ErrUnavailable
}
