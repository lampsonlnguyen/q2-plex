// q2-plex is a music-only Plex client for Q2 Pod's card-app interface.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
	"github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

const version = "0.2.0"
const relayAddr = "127.0.0.1:2328"

type app struct {
	mu        sync.Mutex
	client    *plex.Client
	pin       plex.Pin
	busy      bool
	lastError string
	queue playbackQueue
}

func main() {
	dir := flag.String("dir", "/mnt/mmc/.plex", "card app directory")
	tmp := flag.String("tmp", "/tmp", "firmware runtime directory")
	flag.Parse()
	debug.SetMemoryLimit(48 << 20)
	log.SetOutput(os.Stderr)
	cl, err := plex.Load(*dir)
	if err != nil {
		log.Fatal(err)
	}
	a := &app{client: cl}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go a.initialize(ctx)
	server := &http.Server{Addr: relayAddr, Handler: http.HandlerFunc(a.relay), ReadHeaderTimeout: 5 * time.Second}
	ln, err := net.Listen("tcp4", relayAddr)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Print(err)
		}
	}()
	defer server.Close()
	socket := filepath.Join(*tmp, "q2-plex.browse.sock")
	log.Printf("q2-plex %s serving %s", version, socket)
	go func() { <-ctx.Done(); _ = os.Remove(socket); _ = server.Close(); os.Exit(0) }()
	if err := q2spot.Serve(socket, a.handle); err != nil {
		log.Fatal(err)
	}
}
func (a *app) initialize(ctx context.Context) {
	a.mu.Lock()
	hasToken := a.client.State.Token != ""
	a.mu.Unlock()
	if hasToken {
		a.connect(ctx)
	}
}
func (a *app) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.busy = false
	a.pin = plex.Pin{}
	if err != nil {
		a.lastError = err.Error()
	} else {
		a.lastError = ""
	}
}
func (a *app) connect(ctx context.Context) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return
	}
	a.busy = true
	a.lastError = ""
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.busy = false; a.mu.Unlock() }()
	// Only refresh a token when it is old; on a failed refresh the old token remains.
	if time.Since(time.Unix(a.client.State.TokenTime, 0)) > 5*24*time.Hour {
		refreshCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		err := a.client.Refresh(refreshCtx)
		cancel()
		if err != nil {
			a.setError(fmt.Errorf("Plex login expired: %w", err))
			return
		}
	}
	discoverCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := a.client.Discover(discoverCtx)
	cancel()
	if err != nil {
		a.setError(err)
		return
	}
	libsCtx, done := context.WithTimeout(ctx, 10*time.Second)
	libs, err := a.client.Libraries(libsCtx)
	done()
	if err == nil && len(libs) > 0 && a.client.State.LibraryID == "" {
		a.client.State.LibraryID = libs[0].Key
		err = a.client.Save()
	}
	a.setError(err)
}
func (a *app) beginLogin() {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return
	}
	a.busy = true
	a.pin = plex.Pin{}
	a.lastError = ""
	a.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		pin, err := a.client.StartPIN(ctx)
		cancel()
		if err != nil {
			a.setError(err)
			return
		}
		a.mu.Lock()
		a.pin = pin
		a.busy = false
		a.mu.Unlock()
		// Short PINs expire quickly; only poll while active.
		for i := 0; i < 90; i++ {
			time.Sleep(3 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
			token, err := a.client.PollPIN(ctx, pin.ID)
			cancel()
			if err != nil {
				a.setError(err)
				return
			}
			if token != "" {
				a.mu.Lock()
				a.pin = plex.Pin{}
				a.client.State.Token = token
				a.client.State.TokenTime = time.Now().Unix()
				saveErr := a.client.Save()
				a.mu.Unlock()
				if saveErr != nil {
					a.setError(saveErr)
					return
				}
				a.connect(context.Background())
				return
			}
		}
		a.setError(errors.New("login expired; select Sign In again"))
	}()
}
func text(title string) q2spot.Row { return q2spot.Row{Kind: "info", Title: title} }
func act(op, title, ctx string) q2spot.Row {
	return q2spot.Row{Kind: "action", URI: op, Title: title, Context: ctx}
}
func list(op, title, sub, id string) q2spot.Row {
	return q2spot.Row{Kind: "list", URI: op, Title: title, Subtitle: sub, Context: id}
}
func (a *app) root() q2spot.Reply {
	a.mu.Lock()
	pin, busy, msg := a.pin, a.busy, a.lastError
	signed := a.client.State.Token != ""
	server, lib := a.client.State.ServerName, a.client.State.LibraryID
	a.mu.Unlock()
	if pin.Code != "" {
		return q2spot.Reply{Message: "refresh", Rows: []q2spot.Row{
			text("Visit plex.tv/link on your phone"),
			text("Enter code: " + pin.Code),
			text("Waiting for Plex authorization..."),
			act("login", "Get a new code", ""),
		}}
	}
	if !signed {
		rows := []q2spot.Row{text("Plex Music for Q2 Pod"), act("login", "Sign In with Plex", "")}
		if busy {
			rows = append(rows, text("Getting login code..."))
		}
		if msg != "" {
			rows = append(rows, text("Error: "+msg))
		}
		return q2spot.Reply{Message: func() string {
			if busy {
				return "refresh"
			}
			return ""
		}(), Rows: rows}
	}
	rows := []q2spot.Row{}
	if server != "" && lib != "" {
		rows = append(rows, list("albums", "Albums", server, ""), list("artists", "Artists", server, ""), list("libraries", "Music Libraries", "Choose library", ""), list("queue", "Play Queue", "Current album", ""))
	} else if !busy {
		rows = append(rows, act("reconnect", "Connect to Plex", ""))
	}
	if busy {
		rows = append(rows, text("Connecting to Plex..."))
	}
	if msg != "" {
		rows = append(rows, text("Error: "+msg))
	}
	rows = append(rows, act("logout", "Sign Out", ""))
	r := q2spot.Reply{Rows: rows}
	if busy {
		r.Message = "refresh"
	}
	return r
}
func (a *app) handle(req q2spot.Request) q2spot.Reply {
	if req.Op == "" || req.Op == "root" || req.Op == "menu" {
		return a.root()
	}
	switch req.Op {
	case "login":
		a.beginLogin()
		return a.root()
	case "reconnect":
		go a.connect(context.Background())
		return a.root()
	case "logout":
		a.mu.Lock()
		a.client.State.Token = ""
		a.client.State.ServerToken = ""
		a.client.State.ServerName = ""
		a.client.State.ServerURL = ""
		a.client.State.LibraryID = ""
		err := a.client.Save()
		a.pin = plex.Pin{}
		a.lastError = ""
		a.mu.Unlock()
		if err != nil {
			return q2spot.Fail("error", "Could not save sign-out")
		}
		return a.root()
	}
	a.mu.Lock()
	ready := a.client.State.Token != "" && a.client.State.ServerURL != ""
	a.mu.Unlock()
	if !ready {
		return a.root()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	switch req.Op {
	case "libraries":
		libs, err := a.client.Libraries(ctx)
		if err != nil {
			return q2spot.Fail("error", "Could not load libraries")
		}
		var rows []q2spot.Row
		for _, lib := range libs {
			rows = append(rows, act("selectlibrary", lib.Title, lib.Key))
		}
		if len(rows) == 0 {
			rows = append(rows, text("No music libraries on this server"))
		}
		return q2spot.Reply{Rows: rows}
	case "selectlibrary":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing library ID")
		}
		a.mu.Lock()
		a.client.State.LibraryID = req.Args[0]
		err := a.client.Save()
		a.mu.Unlock()
		if err != nil {
			return q2spot.Fail("error", "Could not save library")
		}
		return q2spot.Reply{Message: "Music library selected"}
	case "albums", "artists", "artist", "album":
		id, cursor := "", "0"
		if req.Op == "artist" || req.Op == "album" {
			if len(req.Args) < 1 {
				return q2spot.Fail("bad", "Missing item ID")
			}
			id = req.Args[0]
			if len(req.Args) > 1 {
				cursor = req.Args[1]
			}
		} else if len(req.Args) > 0 {
			cursor = req.Args[len(req.Args)-1]
		}
		off, err := strconv.Atoi(cursor)
		if err != nil || off < 0 || off > 100000 {
			return q2spot.Fail("bad", "Invalid page")
		}
		items, more, err := a.client.Items(ctx, req.Op, id, off)
		if err != nil {
			return q2spot.Fail("error", "Could not browse music")
		}
		r := q2spot.Reply{}
		if req.Op == "album" && off == 0 {
			r.Rows = append(r.Rows, act("playalbum", "Play Album", id))
		}
		for _, it := range items {
			switch it.Type {
			case "artist":
				r.Rows = append(r.Rows, list("artist", it.Title, fmt.Sprintf("%d tracks", it.LeafCount), it.RatingKey))
			case "album":
				r.Rows = append(r.Rows, list("album", it.Title, it.ParentTitle, it.RatingKey))
			case "track":
				ctx := it.RatingKey
				if req.Op == "album" {
					ctx = id + ":" + it.RatingKey
				}
				r.Rows = append(r.Rows, act("play", it.Title, ctx))
			}
		}
		if len(r.Rows) == 0 {
			r.Rows = append(r.Rows, text("No music found"))
		}
		if more {
			r.Cursor = strconv.Itoa(off + len(items))
		}
		return r
	case "playalbum":
		if len(req.Args) < 1 { return q2spot.Fail("bad", "Missing album") }
		return a.playAlbum(ctx, req.Args[0], "")
	case "queue":
		return a.queuePage()
	case "playqueue":
		if len(req.Args) < 1 { return q2spot.Fail("bad", "Missing queued track") }
		return a.playQueueIndex(req.Args[0])
	case "play":
		if len(req.Args) < 1 {
			return q2spot.Fail("bad", "Missing track")
		}
		if album, song, ok := strings.Cut(req.Args[0], ":"); ok && album != "" && song != "" {
			return a.playAlbum(ctx, album, song)
		}
		it, err := a.client.Track(ctx, req.Args[0])
		if err != nil {
			return q2spot.Fail("error", "Track unavailable")
		}
		if err = a.startQueue([]plex.Item{it}, 0); err != nil {
			return q2spot.Fail("error", "Could not start music")
		}
		return q2spot.Reply{Message: "Playing " + it.Title}
	}
	return q2spot.Fail("bad", "Unsupported action")
}
func field(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == 0 {
			return ' '
		}
		return r
	}, s)
	if len(s) > 250 {
		s = s[:250]
	}
	return s
}
func atomic(path string, b []byte) error {
	temp := path + ".tmp"
	if err := os.WriteFile(temp, b, 0600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
func (a *app) relay(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/song/") {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/song/")
	if id == "" || len(id) > 40 || strings.Trim(id, "0123456789") != "" {
		http.NotFound(w, r)
		return
	}
	// Do not truncate long FLAC playback using an arbitrary elapsed-time limit.
	if err := a.client.Stream(r.Context(), id, w, r); err != nil {
		log.Printf("stream %s: %v", id, err)
	}
}
