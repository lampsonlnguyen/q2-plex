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
	"unicode/utf8"

	"github.com/lampsonlnguyen/q2-plex/internal/offline"
	"github.com/lampsonlnguyen/q2-plex/internal/plex"
	"github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

const version = "1.0.2"
const relayAddr = "127.0.0.1:2328"

type app struct {
	mu            sync.Mutex
	client        *plex.Client
	pin           plex.Pin
	busy          bool
	lastError     string
	manualAddress string
	queue         playbackQueue
	art           *artworkCache
	offline       *offline.Store
	download      downloadJob
	ctx           context.Context
	syncer        *playbackSync
	connection    connectionStatus
	diag          *diagnostics
	diagDir       string
}

func main() {
	dir := flag.String("dir", "/mnt/mmc/.plex", "card app directory")
	tmp := flag.String("tmp", "/tmp", "firmware runtime directory")
	flag.Parse()
	debug.SetMemoryLimit(48 << 20)
	log.SetOutput(os.Stderr)
	cl, err := plex.LoadWith(*dir, plex.Vault{}) // the login sealed in the Vault card app when it's installed
	if err != nil {
		log.Fatal(err)
	}
	art, artErr := newArtworkCache(filepath.Join(*tmp, "q2-plex.art"))
	if artErr != nil {
		log.Printf("art disabled: %v", artErr)
	}
	stored, storeErr := offline.Load(filepath.Join(filepath.Dir(*dir), "PlexMusic"))
	if storeErr != nil {
		log.Printf("offline library unavailable: %v", storeErr)
	}
	_ = os.Remove("/tmp/q2-plex.state")
	pSync, syncErr := newPlaybackSync(*dir)
	if syncErr != nil {
		log.Printf("saved queue unavailable: %v", syncErr)
	}
	a := &app{client: cl, art: art, offline: stored, syncer: pSync, diag: &diagnostics{}, diagDir: *dir}
	a.loadSavedQueue()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	a.ctx = ctx
	defer cancel()
	if a.syncer != nil {
		go a.watchPlayback(ctx)
	}
	go a.initialize(ctx)
	go a.reconnectWorker(ctx)
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
	go func() {
		<-ctx.Done()
		// Do not terminate before the final playback bookmark reaches the SD.
		a.savePlaybackOnExit()
		_ = os.Remove(socket)
		_ = server.Close()
		os.Exit(0)
	}()
	if err := q2spot.Serve(socket, a.handle); err != nil {
		log.Fatal(err)
	}
}
func (a *app) initialize(ctx context.Context) {
	a.mu.Lock()
	hasToken := a.client.State.Token != ""
	a.mu.Unlock()
	if hasToken {
		a.launchConnection("")
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

// launchConnection marks the UI busy synchronously, before returning a response
// to Q2 Pod's action request. HTTP work takes place on a separate goroutine.
func (a *app) launchConnection(manual string) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return
	}
	a.busy = true
	a.lastError = ""
	a.connection.started = true
	a.mu.Unlock()
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go a.connectNow(ctx, manual)
}
func (a *app) connect(ctx context.Context) {
	a.launchConnection("")
}
func (a *app) connectNow(ctx context.Context, manual string) {
	var result error
	defer func() {
		a.mu.Lock()
		a.busy = false
		a.connection.record(time.Now(), result)
		if result != nil {
			a.lastError = result.Error()
			log.Printf("Plex connection: %v", result)
		} else {
			a.lastError = ""
		}
		a.mu.Unlock()
	}()
	// Prefer a verified, previously selected server. This saves a cloud request
	// on every reboot and also works on LAN when plex.tv is unreachable.
	if manual == "" {
		probeCtx, stop := context.WithTimeout(ctx, 7*time.Second)
		result = a.client.ProbeSavedServer(probeCtx)
		stop()
		if result == nil {
			return
		}
	}
	if time.Since(time.Unix(a.client.State.TokenTime, 0)) > 5*24*time.Hour {
		refreshCtx, stop := context.WithTimeout(ctx, 12*time.Second)
		result = a.client.Refresh(refreshCtx)
		stop()
		if result != nil {
			result = fmt.Errorf("Plex session refresh: %w", result)
			return
		}
	}
	connectCtx, stop := context.WithTimeout(ctx, 35*time.Second)
	if manual != "" {
		result = a.client.ConnectDirect(connectCtx, manual)
	} else {
		result = a.client.Discover(connectCtx)
	}
	stop()
	if result != nil {
		return
	}
	log.Printf("Plex music connected: %s", a.client.State.ServerName)
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
				a.launchConnection("")
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
	online := a.connection.online
	a.mu.Unlock()
	if pin.Code != "" {
		return q2spot.Reply{Message: "refresh", Rows: []q2spot.Row{
			text("Visit plex.tv/link on your phone"),
			text("Code: " + pin.Code),
			text("Waiting for approval..."),
			act("login", "New Login Code", ""),
		}}
	}
	if !signed {
		rows := []q2spot.Row{
			list("offline", "Offline Music", "Saved on microSD", ""),
			list("lastqueue", "Last Queue", "Previous music", ""),
			act("login", "Sign In to Plex", ""),
		}
		if busy {
			rows = append(rows, text("Connecting..."))
		}
		if msg != "" {
			rows = append(rows, text("Error: "+msg))
		}
		r := q2spot.Reply{Rows: rows}
		if busy {
			r.Message = "refresh"
		}
		return r
	}
	// Streaming stays the default: browse live music first, downloads last.
	rows := []q2spot.Row{}
	if server != "" && lib != "" {
		rows = append(rows,
			list("albums", "Albums", "", ""),
			list("artists", "Artists", "", ""),
			list("playlists", "Playlists", "", ""),
			list("searchpage", "Search Music", "", ""),
			list("favorites", "Favorites", "", ""),
			list("queue", "Up Next", "", ""),
		)
	}
	rows = append(rows,
		list("offline", "Offline Music", "On microSD", ""),
		list("downloadstatus", "Downloads", "", ""),
		list("lastqueue", "Last Queue", "Restore music", ""),
		list("connection", "Connection", connectionLabel(server, online, busy), ""),
	)
	if busy {
		rows = append(rows, text("Connecting to Plex..."))
	}
	if !online && !busy && msg != "" {
		rows = append(rows, text("Plex unavailable; retry is automatic"))
	}
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
		a.launchConnection("")
		return a.root()
	case "connection":
		return a.connectionPage()
	case "manual":
		a.mu.Lock()
		address := a.manualAddress
		a.mu.Unlock()
		return q2spot.Reply{Rows: []q2spot.Row{
			{Kind: "field", URI: "serveraddress", Title: "Server Address", Subtitle: address, Position: address},
			act("connectmanual", "Connect to Address", ""),
			text("Example: http://192.168.1.10:32400"),
		}}
	case "serveraddress":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing server address")
		}
		value := strings.TrimSpace(req.Args[len(req.Args)-1])
		if len(value) > 180 {
			return q2spot.Fail("bad", "Address too long")
		}
		a.mu.Lock()
		a.manualAddress = value
		a.mu.Unlock()
		return a.handle(q2spot.Request{Op: "manual"})
	case "connectmanual":
		a.mu.Lock()
		address := a.manualAddress
		a.mu.Unlock()
		if address == "" {
			return q2spot.Fail("bad", "Enter a server address")
		}
		a.launchConnection(address)
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
		if err == nil && a.syncer != nil {
			if clearErr := a.syncer.store.Clear(); clearErr != nil {
				log.Printf("clear saved queue: %v", clearErr)
			}
			_ = os.Remove(filepath.Join(a.syncer.root, "fav.log"))
			_ = os.Remove(filepath.Join(a.syncer.root, "fav.log.taking"))
		}
		if err != nil {
			return q2spot.Fail("error", "Could not save sign-out")
		}
		return a.root()
	}
	if reply, handled := a.handleDiagnostics(req); handled {
		return reply
	}
	if reply, handled := a.handleSync(req); handled {
		return reply
	}
	if reply, handled := a.handleOffline(req); handled {
		return reply
	}
	a.mu.Lock()
	ready := a.client.State.Token != "" && a.client.State.ServerURL != ""
	a.mu.Unlock()
	if !ready {
		return a.root()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if reply, handled := a.extra(ctx, req); handled {
		return reply
	}
	switch req.Op {
	case "libraries":
		libs, err := a.client.Libraries(ctx)
		if err != nil {
			a.notePlexFailure(err)
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
			a.notePlexFailure(err)
			return q2spot.Fail("error", "Could not browse music")
		}

		r := q2spot.Reply{}
		art := make([]artworkJob, 0, len(items))
		if req.Op == "album" && off == 0 {
			r.Rows = append(r.Rows, act("playalbum", "Play Album", id))
			r.Rows = append(r.Rows, act("downloadalbum", "Download Album to SD", id))
			r.Rows = append(r.Rows, text("Hold Play/Pause on a song to toggle Favorite"))
		}
		for _, it := range items {
			if it.Type != "album" && it.Type != "artist" && it.Type != "track" {
				continue
			}
			row := musicRow(it, req.Op, id)
			r.Rows = append(r.Rows, row)
			if thumb := itemArtwork(it); thumb != "" && it.Type != "track" {
				art = append(art, artworkJob{row: row, source: thumb})
			}
		}
		if len(r.Rows) == 0 {
			r.Rows = append(r.Rows, text("No music found"))
		}
		if more {
			r.Cursor = strconv.Itoa(off + len(items))
		}
		return a.withArt(r, art)
	case "playalbum":
		if len(req.Args) < 1 {
			return q2spot.Fail("bad", "Missing album")
		}
		return a.playAlbum(ctx, req.Args[0], "")
	case "queue":
		return a.queuePage(req.Args)
	case "playqueue":
		if len(req.Args) < 1 {
			return q2spot.Fail("bad", "Missing queued track")
		}
		return a.playQueueIndex(req.Args[0])
	case "play":
		if len(req.Args) < 1 {
			return q2spot.Fail("bad", "Missing track")
		}
		if strings.HasPrefix(req.Args[0], "pl:") {
			parts := strings.SplitN(req.Args[0], ":", 3)
			if len(parts) != 3 {
				return q2spot.Fail("bad", "Invalid playlist track")
			}
			return a.playPlaylist(ctx, parts[1], parts[2])
		}
		if album, song, ok := strings.Cut(req.Args[0], ":"); ok && album != "" && song != "" {
			return a.playAlbum(ctx, album, song)
		}
		it, err := a.client.Track(ctx, req.Args[0])
		if err != nil {
			a.notePlexFailure(err)
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
		end := 250
		// Never split a UTF-8 character when firmware fields are bounded.
		for end > 0 && !utf8.RuneStart(s[end]) {
			end--
		}
		s = s[:end]
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
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind, id, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if !ok || !numericID(id) || (kind != "song" && kind != "cover") {
		http.NotFound(w, r)
		return
	}
	if kind == "song" {
		// Direct streaming remains in Q2 Pod's native player and honors HTTP Range.
		if err := a.client.Stream(r.Context(), id, w, r); err != nil {
			a.notePlexFailure(err)
			log.Printf("stream %s: %v", id, err)
		}
		return
	}
	// Now Playing cover served only on loopback, without a Plex token in the URL.
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	track, err := a.client.Track(ctx, id)
	if err != nil {
		http.Error(w, "cover metadata unavailable", 502)
		return
	}
	src := itemArtwork(track)
	if src == "" {
		http.NotFound(w, r)
		return
	}
	data, err := a.client.Thumbnail(ctx, src, 256)
	if err != nil {
		a.notePlexFailure(err)
		log.Printf("cover %s: %v", id, err)
		http.Error(w, "cover unavailable", 502)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private,max-age=3600")
	if r.Method != "HEAD" {
		_, _ = w.Write(data)
	}
}
