package plex

import (
	"bytes"
	"errors"
	"log"
	"os"

	"github.com/lampsonlnguyen/q2-plex/internal/vault"
)

// Secrets keeps the login (State: the device key and the Plex tokens) sealed in the Q2's Vault card app
// instead of auth.json on the card, where anyone reading the card could copy it.
// https://github.com/js-surya/q2-vault
type Secrets interface {
	Get() ([]byte, error)
	Put([]byte) error
}

// Vault is Secrets in the Vault card app.
type Vault struct{}

func (Vault) Get() ([]byte, error) { return vault.Get() }
func (Vault) Put(b []byte) error   { return vault.Put(b) }

// readState returns the saved login, nil for none, and whether it came from auth.json. With the Vault
// answering, the login is sealed there (c.sealed); a plain auth.json beside it is newer (written while the
// Vault was away, see write) and wins, to be moved in. Without a Vault, auth.json as before.
func (c *Client) readState() ([]byte, bool, error) {
	file, err := os.ReadFile(c.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	if c.Secrets != nil {
		sealed, verr := c.Secrets.Get()
		switch {
		case verr == nil, errors.Is(verr, vault.ErrNone):
			c.sealed = true
			if file == nil {
				return sealed, false, nil
			}
		default:
			if !errors.Is(verr, vault.ErrNoVault) {
				log.Printf("vault: %v; using auth.json", verr)
			}
		}
	}
	return file, file != nil, nil
}

// moveIn seals b, a login read from auth.json, in the Vault, and deletes auth.json only once the Vault
// gives back exactly b. Anything else keeps the file, and the next start tries again.
func (c *Client) moveIn(b []byte) {
	if err := c.Secrets.Put(b); err != nil {
		log.Printf("vault: keeping auth.json: %v", err)
		c.sealed = false
		return
	}
	if got, err := c.Secrets.Get(); err != nil || !bytes.Equal(got, b) {
		log.Printf("vault: keeping auth.json: it didn't come back as saved")
		c.sealed = false
		return
	}
	if err := os.Remove(c.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("vault: auth.json: %v", err)
		return
	}
	log.Printf("vault: the login moved in from auth.json")
}

// write saves b in the Vault when the login lives there. If the Vault can't take it now (closed, the card
// away), it goes to auth.json as before, so a sign-in is never lost; the next start moves it in.
func (c *Client) write(b []byte) error {
	if c.sealed {
		err := c.Secrets.Put(b)
		if err == nil {
			return nil
		}
		log.Printf("vault: saving to auth.json instead: %v", err)
		c.sealed = false
	}
	path := c.Path + ".tmp"
	if err := os.WriteFile(path, b, 0600); err != nil {
		return err
	}
	return os.Rename(path, c.Path)
}
