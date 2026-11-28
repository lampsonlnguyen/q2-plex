package vault

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// setup points the package at a fresh short socket path and pretends the
// server is root. It returns the socket path and the installed-marker path.
func setup(t *testing.T) (sock, installed string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "qv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	oS, oI, oP, oE, oF, oL, oR := Socket, Installed, peerUID, retryEvery, retryFor, PidFile, ProcRoot
	Socket = filepath.Join(dir, "s")
	Installed = filepath.Join(dir, "q2-vault")
	PidFile = filepath.Join(dir, "q2-vault.pid")
	ProcRoot = filepath.Join(dir, "proc")
	peerUID = func(*net.UnixConn) (uint32, error) { return 0, nil }
	retryEvery, retryFor = 20*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() {
		Socket, Installed, peerUID, retryEvery, retryFor, PidFile, ProcRoot = oS, oI, oP, oE, oF, oL, oR
	})
	install(t)
	launcher(t, "4242\n", true) // the Vault's launcher is running: it may be starting
	return Socket, Installed
}

// launcher writes the launcher's pid file and makes /proc/4242 exist or not.
func launcher(t *testing.T, pidFile string, alive bool) {
	t.Helper()
	if err := os.WriteFile(PidFile, []byte(pidFile), 0o644); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(ProcRoot, "4242")
	if alive {
		os.MkdirAll(p, 0o755)
	} else {
		os.RemoveAll(p)
	}
}

func install(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(Installed, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

type fake struct {
	mu  sync.Mutex
	got []string
	ln  net.Listener
}

func (f *fake) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

// serve answers every connection with reply (raw bytes); an empty reply closes
// without answering.
func serve(t *testing.T, sock, reply string) *fake {
	t.Helper()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				b, _ := io.ReadAll(c)
				f.mu.Lock()
				f.got = append(f.got, string(b))
				f.mu.Unlock()
				if reply != "" {
					c.Write([]byte(reply))
				}
			}()
		}
	}()
	return f
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		status string
		want   error
	}{
		{"none", ErrNone}, {"refused", ErrRefused}, {"too-large", ErrTooLarge},
		{"card-away", ErrCardAway}, {"bad-request", ErrBadRequest}, {"weird", ErrUnavailable},
	}
	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			sock, _ := setup(t)
			serve(t, sock, "Q2VAULT/1\n"+c.status+"\n")
			if _, err := Get(); !errors.Is(err, c.want) {
				t.Fatalf("Get: %v, want %v", err, c.want)
			}
			if err := Put([]byte("x")); !errors.Is(err, c.want) {
				t.Fatalf("Put: %v, want %v", err, c.want)
			}
			if err := Delete(); !errors.Is(err, c.want) {
				t.Fatalf("Delete: %v, want %v", err, c.want)
			}
		})
	}
}

func TestGetReturnsBody(t *testing.T) {
	sock, _ := setup(t)
	f := serve(t, sock, "Q2VAULT/1\nok\nsecret\nwith newline")
	b, err := Get()
	if err != nil || string(b) != "secret\nwith newline" {
		t.Fatalf("got %q, %v", b, err)
	}
	if r := f.requests(); len(r) != 1 || r[0] != "Q2VAULT/1\nget\n" {
		t.Fatalf("request %q", r)
	}
}

func TestPutAndDeleteRequests(t *testing.T) {
	sock, _ := setup(t)
	f := serve(t, sock, "Q2VAULT/1\nok\n")
	if err := Put([]byte("tok")); err != nil {
		t.Fatal(err)
	}
	if err := Delete(); err != nil {
		t.Fatal(err)
	}
	r := f.requests()
	if len(r) != 2 || r[0] != "Q2VAULT/1\nput\ntok" || r[1] != "Q2VAULT/1\ndelete\n" {
		t.Fatalf("requests %q", r)
	}
}

func TestPutLocalChecks(t *testing.T) {
	sock, _ := setup(t)
	f := serve(t, sock, "Q2VAULT/1\nok\n")
	if err := Put(nil); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("empty: %v", err)
	}
	if err := Put(make([]byte, 16385)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("big: %v", err)
	}
	if err := Put(make([]byte, 16384)); err != nil {
		t.Fatalf("max: %v", err)
	}
	if n := len(f.requests()); n != 1 {
		t.Fatalf("server saw %d requests, want 1", n)
	}
}

func TestNotInstalledIsImmediate(t *testing.T) {
	_, inst := setup(t)
	os.Remove(inst)
	start := time.Now()
	if _, err := Get(); !errors.Is(err, ErrNoVault) {
		t.Fatal(err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("not immediate")
	}
}

func TestStartsLate(t *testing.T) {
	sock, _ := setup(t)
	install(t)
	retryFor = 2 * time.Second
	go func() {
		time.Sleep(150 * time.Millisecond)
		ln, err := net.Listen("unix", sock)
		if err != nil {
			return
		}
		go func() {
			defer ln.Close()
			c, _ := ln.Accept()
			io.ReadAll(c)
			c.Write([]byte("Q2VAULT/1\nok\nhi"))
			c.Close()
			time.Sleep(50 * time.Millisecond)
		}()
	}()
	b, err := Get()
	if err != nil || string(b) != "hi" {
		t.Fatalf("%q %v", b, err)
	}
}

func TestNeverStarts(t *testing.T) {
	setup(t)
	install(t)
	start := time.Now()
	if err := Delete(); !errors.Is(err, ErrNoVault) {
		t.Fatal(err)
	}
	if d := time.Since(start); d < retryFor || d > retryFor+time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestInstalledButNotRunningIsImmediate(t *testing.T) {
	// Installed, but no live launcher: not now. Nothing is coming, so no wait.
	cases := []struct {
		name, pid string
		alive     bool
		noFile    bool
	}{
		{"no pid file", "", false, true},
		{"dead pid", "4242\n", false, false},
		{"empty", "", true, false},
		{"not decimal", "42a\n", true, false},
		{"signed", "+4242\n", true, false},
		{"two", "4242 4242\n", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setup(t)
			launcher(t, c.pid, c.alive)
			if c.noFile {
				os.Remove(PidFile)
			}
			retryFor = 2 * time.Second
			start := time.Now()
			if _, err := Get(); !errors.Is(err, ErrNoVault) {
				t.Fatal(err)
			}
			if time.Since(start) > 100*time.Millisecond {
				t.Fatalf("waited %v with no live launcher", time.Since(start))
			}
		})
	}
}

func TestRetryStopsWhenTheLauncherEnds(t *testing.T) {
	setup(t)
	retryFor = 3 * time.Second
	proc := filepath.Join(ProcRoot, "4242") // read here: setup's cleanup restores ProcRoot
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		time.Sleep(100 * time.Millisecond)
		os.RemoveAll(proc)
	}()
	defer func() { <-ended }()
	start := time.Now()
	if _, err := Get(); !errors.Is(err, ErrNoVault) {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("kept retrying %v after the launcher ended", d)
	}
}

func TestDefaults(t *testing.T) {
	if Socket != "/tmp/q2-vault.sock" || Installed != "/mnt/mmc/.vault/q2-vault" ||
		PidFile != "/tmp/q2-vault" || ProcRoot != "/proc" {
		t.Fatal("paths")
	}
	if retryEvery != 250*time.Millisecond || retryFor != 5*time.Second {
		t.Fatal("retry")
	}
}

func TestClosedWithoutAnswerIsTemporary(t *testing.T) {
	sock, _ := setup(t)
	install(t)
	serve(t, sock, "")
	start := time.Now()
	if _, err := Get(); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if time.Since(start) > retryFor {
		t.Fatal("should not retry")
	}
}

func TestNonRootServerSendsNothing(t *testing.T) {
	sock, _ := setup(t)
	peerUID = func(*net.UnixConn) (uint32, error) { return 1000, nil }
	f := serve(t, sock, "Q2VAULT/1\nok\nstolen")
	if err := Put([]byte("password")); !errors.Is(err, ErrNotVault) {
		t.Fatalf("put: %v", err)
	}
	if b, err := Get(); !errors.Is(err, ErrNotVault) || len(b) != 0 {
		t.Fatalf("get: %q %v", b, err)
	}
	time.Sleep(50 * time.Millisecond)
	for _, r := range f.requests() {
		if strings.Contains(r, "password") {
			t.Fatal("secret was sent")
		}
	}
}

func TestPeerCheckErrorRefuses(t *testing.T) {
	sock, _ := setup(t)
	peerUID = func(*net.UnixConn) (uint32, error) { return 0, errors.New("boom") }
	serve(t, sock, "Q2VAULT/1\nok\nx")
	if err := Put([]byte("x")); !errors.Is(err, ErrNotVault) {
		t.Fatal(err)
	}
}

func TestOversizeAnswerIsUnavailable(t *testing.T) {
	sock, _ := setup(t)
	serve(t, sock, "Q2VAULT/1\nok\n"+strings.Repeat("a", 16385))
	if _, err := Get(); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestMaxAnswerAccepted(t *testing.T) {
	sock, _ := setup(t)
	serve(t, sock, "Q2VAULT/1\nok\n"+strings.Repeat("a", 16384))
	if b, err := Get(); err != nil || len(b) != 16384 {
		t.Fatal(len(b), err)
	}
}

func TestEmptyOkGetIsUnavailable(t *testing.T) {
	sock, _ := setup(t)
	serve(t, sock, "Q2VAULT/1\nok\n")
	if _, err := Get(); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if err := Put([]byte("x")); err != nil {
		t.Fatalf("put ok with empty body must succeed: %v", err)
	}
}

func TestDefaultHookFailsClosedOffLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux reads the peer's credentials (peer_linux_test.go)")
	}
	if _, err := osPeerUID(nil); runtime.GOOS != "linux" && err == nil {
		t.Fatal("must fail closed")
	}
}
