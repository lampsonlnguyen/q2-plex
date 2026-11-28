package plex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lampsonlnguyen/q2-plex/internal/vault"
)

type fakeVault struct {
	data   []byte
	err    error // Get and Put fail with it while set
	putErr error
	echo   []byte // Get returns this instead of data, when set
	puts   int
}

func (f *fakeVault) Get() ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.echo != nil {
		return f.echo, nil
	}
	if f.data == nil {
		return nil, vault.ErrNone
	}
	return append([]byte(nil), f.data...), nil
}

func (f *fakeVault) Put(b []byte) error {
	if f.err != nil {
		return f.err
	}
	if f.putErr != nil {
		return f.putErr
	}
	f.puts++
	f.data = append([]byte(nil), b...)
	return nil
}

func signedIn(t *testing.T, dir string) State {
	t.Helper()
	c, err := Load(dir) // auth.json, as before the Vault
	if err != nil {
		t.Fatal(err)
	}
	c.State.Token, c.State.ServerURL = "account-token", "https://server.example:32400"
	if err = c.Save(); err != nil {
		t.Fatal(err)
	}
	return c.State
}

func sealedState(t *testing.T, v *fakeVault) State {
	t.Helper()
	var s State
	if err := json.Unmarshal(v.data, &s); err != nil {
		t.Fatalf("the Vault holds %q: %v", v.data, err)
	}
	return s
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestAnExistingLoginMovesIntoTheVault(t *testing.T) {
	dir := t.TempDir()
	want := signedIn(t, dir)
	v := &fakeVault{}
	c, err := LoadWith(dir, v)
	if err != nil {
		t.Fatal(err)
	}
	if c.State != want {
		t.Fatalf("state %+v, want %+v", c.State, want)
	}
	if exists(filepath.Join(dir, "auth.json")) {
		t.Fatal("auth.json still on the card")
	}
	if got := sealedState(t, v); got != want {
		t.Fatalf("the Vault holds %+v", got)
	}
	again, err := LoadWith(dir, v) // the next start
	if err != nil || again.State != want {
		t.Fatalf("next start: %+v, %v", again.State, err)
	}
}

func TestAFailedMoveKeepsAuthJSON(t *testing.T) {
	for name, v := range map[string]*fakeVault{
		"put fails":          {putErr: vault.ErrTooLarge},
		"comes back changed": {echo: []byte(`{"clientId":"other"}`)},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			want := signedIn(t, dir)
			c, err := LoadWith(dir, v)
			if err != nil || c.State != want {
				t.Fatalf("%+v, %v", c.State, err)
			}
			if !exists(filepath.Join(dir, "auth.json")) {
				t.Fatal("auth.json deleted though the move failed")
			}
		})
	}
}

func TestAFreshInstallSealsFromTheStart(t *testing.T) {
	dir := t.TempDir()
	v := &fakeVault{}
	c, err := LoadWith(dir, v)
	if err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, "auth.json")) {
		t.Fatal("auth.json written with a Vault")
	}
	if got := sealedState(t, v); got.ClientID != c.State.ClientID || got.PrivateKey == "" {
		t.Fatalf("the device key isn't in the Vault: %+v", got)
	}
}

func TestASignInIsSavedToTheVault(t *testing.T) {
	dir := t.TempDir()
	v := &fakeVault{}
	c, err := LoadWith(dir, v)
	if err != nil {
		t.Fatal(err)
	}
	c.State.Token = "new-token"
	if err = c.Save(); err != nil {
		t.Fatal(err)
	}
	if sealedState(t, v).Token != "new-token" || exists(filepath.Join(dir, "auth.json")) {
		t.Fatal("the sign-in didn't go to the Vault only")
	}
}

func TestTheVaultAwayAtASaveFallsBackToAuthJSON(t *testing.T) {
	dir := t.TempDir()
	v := &fakeVault{}
	c, err := LoadWith(dir, v)
	if err != nil {
		t.Fatal(err)
	}
	v.err = vault.ErrNoVault // closed or the card away
	c.State.Token = "signed-in-meanwhile"
	if err = c.Save(); err != nil {
		t.Fatalf("a save with the Vault away lost the sign-in: %v", err)
	}
	if !exists(filepath.Join(dir, "auth.json")) {
		t.Fatal("no auth.json to fall back to")
	}
	v.err = nil // the next start, the Vault back: the newer file wins and moves in
	again, err := LoadWith(dir, v)
	if err != nil || again.State.Token != "signed-in-meanwhile" {
		t.Fatalf("%+v, %v", again.State, err)
	}
	if exists(filepath.Join(dir, "auth.json")) || sealedState(t, v).Token != "signed-in-meanwhile" {
		t.Fatal("the newer login didn't move in")
	}
}

func TestNoVaultKeepsAuthJSONAsBefore(t *testing.T) {
	for _, e := range []error{vault.ErrNoVault, vault.ErrNotVault, vault.ErrCardAway, errors.New("other")} {
		dir := t.TempDir()
		want := signedIn(t, dir)
		v := &fakeVault{err: e}
		c, err := LoadWith(dir, v)
		if err != nil || c.State != want {
			t.Fatalf("%v: %+v, %v", e, c.State, err)
		}
		c.State.LibraryID = "7"
		if err = c.Save(); err != nil {
			t.Fatal(err)
		}
		if !exists(filepath.Join(dir, "auth.json")) || v.puts != 0 {
			t.Fatalf("%v: not auth.json as before", e)
		}
	}
}
