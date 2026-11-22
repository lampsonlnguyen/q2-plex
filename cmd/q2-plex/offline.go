package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/offline"
	"github.com/lampsonlnguyen/q2-plex/internal/plex"
	"github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

// A single background download at a time prevents SD-card thrashing and keeps
// the Q2's Wi-Fi/radio and 128 MiB RAM demands predictable.
type downloadJob struct {
	Active   bool
	Kind     string
	ID       string
	Title    string
	Total    int
	Done     int
	Current  string
	Bytes    int64
	Expected int64
	Error    string
	Cancel   context.CancelFunc
}

func (a *app) downloadPage() q2spot.Reply {
	a.mu.Lock()
	j := a.download
	a.mu.Unlock()
	if !j.Active && j.Total == 0 && j.Error == "" {
		return q2spot.Reply{Rows: []q2spot.Row{
			text("No download in progress"),
			text("Open an album or playlist to Download"),
			text("Saved music plays without Wi-Fi"),
		}}
	}
	rows := []q2spot.Row{text(j.Title)}
	if j.Active {
		rows = append(rows, act("canceloffline", "Cancel Download", ""))
		subtitle := fmt.Sprintf("%d of %d tracks", j.Done, j.Total)
		if j.Total == 0 {
			subtitle = "Reading Plex music..."
		}
		percent := 0
		if j.Total > 0 {
			percent = (j.Done * 1000) / j.Total
			if j.Expected > 0 {
				percent += (int((j.Bytes * 1000) / j.Expected)) / j.Total
			}
			if percent > 1000 {
				percent = 1000
			}
		}
		row := q2spot.Row{Kind: "progress", Title: j.Current, Subtitle: subtitle, Position: strconv.Itoa(percent)}
		caps, _ := os.ReadFile("/tmp/q2-addon.caps")
		if !strings.Contains(" "+string(caps)+" ", " progress ") {
			row.Kind = "info"
			row.Position = ""
		}
		rows = append(rows, row)
	} else if j.Error != "" {
		rows = append(rows, text("Stopped: "+j.Error), text("Select Download again to resume"))
	} else {
		rows = append(rows, text(fmt.Sprintf("Saved %d tracks", j.Done)))
	}
	r := q2spot.Reply{Rows: rows}
	if j.Active {
		r.Message = "refresh"
	}
	return r
}
func (a *app) offlinePage(args []string) q2spot.Reply {
	if a.offline == nil {
		return q2spot.Reply{Rows: []q2spot.Row{text("Offline storage unavailable")}}
	}
	offset := 0
	if len(args) > 0 {
		n, err := strconv.Atoi(args[len(args)-1])
		if err != nil || n < 0 || n > 100000 {
			return q2spot.Fail("bad", "Invalid page")
		}
		offset = n
	}
	collections := a.offline.List()
	r := q2spot.Reply{Rows: []q2spot.Row{}}
	if len(collections) == 0 {
		return q2spot.Reply{Rows: []q2spot.Row{
			text("No music saved yet"), text("Connect to Plex and download an album or playlist"),
		}}
	}
	for i := offset; i < len(collections) && i < offset+40; i++ {
		c := collections[i]
		n := 0
		for _, tr := range c.Tracks {
			if a.offline.Exists(tr) {
				n++
			}
		}
		subtitle := fmt.Sprintf("%d tracks", n)
		if !c.Complete {
			subtitle += " (partial)"
		}
		if c.Kind == "playlist" {
			subtitle = "Playlist - " + subtitle
		}
		r.Rows = append(r.Rows, list("offlinegroup", c.Title, subtitle, c.Key))
	}
	if offset+40 < len(collections) {
		r.Cursor = strconv.Itoa(offset + 40)
	}
	return r
}
func (a *app) offlineTracks(key string) (offline.Collection, []offline.Track, bool) {
	if a.offline == nil {
		return offline.Collection{}, nil, false
	}
	col, ok := a.offline.Get(key)
	if !ok {
		return col, nil, false
	}
	tracks := make([]offline.Track, 0, len(col.Tracks))
	for _, tr := range col.Tracks {
		if a.offline.Exists(tr) {
			tracks = append(tracks, tr)
		}
	}
	return col, tracks, true
}
func (a *app) offlineGroup(args []string) q2spot.Reply {
	if len(args) == 0 {
		return q2spot.Fail("bad", "Missing offline collection")
	}
	col, tracks, ok := a.offlineTracks(args[0])
	if !ok {
		return q2spot.Fail("bad", "Offline collection not found")
	}
	offset := 0
	if len(args) > 1 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 0 || n > 250 {
			return q2spot.Fail("bad", "Invalid page")
		}
		offset = n
	}
	r := q2spot.Reply{}
	if offset == 0 {
		if len(tracks) > 0 {
			r.Rows = append(r.Rows, act("playofflineall", "Play Offline", col.Key))
		}
		r.Rows = append(r.Rows, list("offlineremoveconfirm", "Remove Download...", col.Title, col.Key))
		if !col.Complete {
			r.Rows = append(r.Rows, text("Partial download; retry online to finish"))
		}
	}
	for i := offset; i < len(tracks) && i < offset+40; i++ {
		t := tracks[i]
		r.Rows = append(r.Rows, q2spot.Row{Kind: "action", URI: "playoffline", Title: t.Title, Subtitle: t.Artist, Context: col.Key + "|" + strconv.Itoa(i)})
	}
	if offset+40 < len(tracks) {
		r.Cursor = strconv.Itoa(offset + 40)
	}
	if len(tracks) == 0 {
		r.Rows = append(r.Rows, text("No playable downloaded tracks"))
	}
	return r
}
func (a *app) startOfflineQueue(key string, from int) q2spot.Reply {
	_, tracks, ok := a.offlineTracks(key)
	if !ok || len(tracks) == 0 {
		return q2spot.Fail("unavailable", "No offline tracks found")
	}
	if from < 0 || from >= len(tracks) {
		return q2spot.Fail("bad", "Invalid track position")
	}
	songs := make([]plex.Item, 0, len(tracks))
	paths := map[string]string{}
	for _, tr := range tracks {
		songs = append(songs, plex.Item{Type: "track", RatingKey: tr.ID, Title: tr.Title, ParentTitle: tr.Album, GrandparentTitle: tr.Artist, Duration: tr.Duration})
		paths[tr.ID] = a.offline.MediaPath(tr.File)
	}
	if err := a.startQueueWithLocal(songs, from, paths); err != nil {
		return q2spot.Fail("error", "Could not start offline playback")
	}
	return q2spot.Reply{Message: "Playing " + cleanField(tracks[from].Title)}
}
func (a *app) handleOffline(req q2spot.Request) (q2spot.Reply, bool) {
	switch req.Op {
	case "offline":
		return a.offlinePage(req.Args), true
	case "offlinegroup":
		return a.offlineGroup(req.Args), true
	case "downloadstatus":
		return a.downloadPage(), true
	case "playofflineall":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing offline album"), true
		}
		return a.startOfflineQueue(req.Args[0], 0), true
	case "playoffline":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing offline song"), true
		}
		key, pos, ok := strings.Cut(req.Args[0], "|")
		if !ok {
			return q2spot.Fail("bad", "Invalid offline song"), true
		}
		i, err := strconv.Atoi(pos)
		if err != nil {
			return q2spot.Fail("bad", "Invalid track position"), true
		}
		return a.startOfflineQueue(key, i), true
	case "offlineremoveconfirm":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing offline collection"), true
		}
		_, _, ok := a.offlineTracks(req.Args[0])
		if !ok {
			return q2spot.Fail("bad", "Not found"), true
		}
		return q2spot.Reply{Rows: []q2spot.Row{
			text("Remove saved music from the SD card?"),
			act("offlineremove", "Confirm Remove Download", req.Args[0]),
		}}, true
	case "offlineremove":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing offline collection"), true
		}
		if a.offline == nil {
			return q2spot.Fail("error", "Offline storage unavailable"), true
		}
		a.mu.Lock()
		busy := a.download.Active
		a.mu.Unlock()
		if busy {
			return q2spot.Fail("error", "Cancel download before removing"), true
		}
		if err := a.offline.Remove(req.Args[0]); err != nil {
			return q2spot.Fail("error", "Could not remove download"), true
		}
		return q2spot.Reply{Message: "Download removed"}, true
	case "canceloffline":
		a.mu.Lock()
		cancel := a.download.Cancel
		if a.download.Active {
			a.download.Current = "Cancelling..."
		}
		a.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return q2spot.Reply{Message: "Stopping download"}, true
	case "downloadalbum", "downloadplaylist":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing collection"), true
		}
		kind := "album"
		if req.Op == "downloadplaylist" {
			kind = "playlist"
		}
		if err := a.beginDownload(kind, req.Args[0]); err != nil {
			return q2spot.Fail("error", err.Error()), true
		}
		return q2spot.Reply{Message: "Download started. Open Downloads to check progress"}, true
	case "queue":
		a.mu.Lock()
		offlineQueue := len(a.queue.Local) > 0
		a.mu.Unlock()
		if offlineQueue {
			return a.queuePage(), true
		}
	case "playqueue":
		a.mu.Lock()
		offlineQueue := len(a.queue.Local) > 0
		a.mu.Unlock()
		if offlineQueue {
			if len(req.Args) == 0 {
				return q2spot.Fail("bad", "Missing track"), true
			}
			return a.playQueueIndex(req.Args[0]), true
		}
	}
	return q2spot.Reply{}, false
}
func (a *app) beginDownload(kind, id string) error {
	if a.offline == nil {
		return errors.New("offline storage unavailable")
	}
	if !numericID(id) {
		return errors.New("invalid album or playlist")
	}
	a.mu.Lock()
	if a.download.Active {
		a.mu.Unlock()
		return errors.New("a download is already running")
	}
	if a.client.State.ServerToken == "" || a.client.State.ServerURL == "" {
		a.mu.Unlock()
		return errors.New("connect to Plex first")
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.download = downloadJob{Active: true, Kind: kind, ID: id, Title: "Getting " + kind + " tracks...", Cancel: cancel}
	a.mu.Unlock()
	if err := os.WriteFile("/tmp/q2-plex.state", []byte("hold=1\n"), 0600); err != nil {
		log.Printf("cannot set download power hold: %v", err)
	}
	go a.downloadCollection(ctx, kind, id)
	return nil
}
func (a *app) updateDownload(update func(*downloadJob)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	update(&a.download)
}
func (a *app) downloadCollection(ctx context.Context, kind, id string) {
	var err error
	defer func() {
		_ = os.Remove("/tmp/q2-plex.state")
		a.updateDownload(func(j *downloadJob) {
			j.Active = false
			j.Cancel = nil
			if err != nil {
				j.Error = err.Error()
			} else {
				j.Error = ""
			}
		})
		if err != nil {
			log.Printf("offline %s %s: %v", kind, id, err)
		}
	}()
	server := a.client.State.ServerName
	if server == "" {
		server = a.client.State.ServerURL
	}
	key := offline.CollectionKey(server, kind, id)
	if key == "" {
		err = errors.New("invalid collection")
		return
	}
	var items []plex.Item
	metaCtx, done := context.WithTimeout(ctx, 30*time.Second)
	if kind == "album" {
		items, err = a.client.AlbumTracks(metaCtx, id, queueLimit)
	} else {
		items, err = a.client.PlaylistTracks(metaCtx, id, queueLimit)
	}
	done()
	if err != nil {
		return
	}
	if len(items) == 0 {
		err = errors.New("collection contains no music")
		return
	}
	// Show a usable title even if the metadata name endpoint is unavailable.
	title := strings.ToUpper(kind[:1]) + kind[1:] + " " + id
	titleCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	if name, nameErr := a.client.CollectionName(titleCtx, kind, id); nameErr == nil && name != "" {
		title = name
	}
	cancel()
	if err = a.offline.Begin(server, kind, id, title); err != nil {
		return
	}
	a.updateDownload(func(j *downloadJob) { j.Title = title; j.Total = len(items); j.Current = "Starting download..." })
	ordered := make([]offline.Track, 0, len(items))
	for i, item := range items {
		if err = ctx.Err(); err != nil {
			return
		}
		a.updateDownload(func(j *downloadJob) { j.Done = i; j.Current = item.Title; j.Bytes = 0; j.Expected = 0 })
		var track plex.Item
		trackCtx, stop := context.WithTimeout(ctx, 12*time.Second)
		track, err = a.client.Track(trackCtx, item.RatingKey)
		stop()
		if err != nil {
			return
		}
		ext, extErr := plex.MediaExtension(track)
		if extErr != nil {
			err = extErr
			return
		}
		file := offline.MediaFile(server, track.RatingKey, ext)
		if file == "" {
			err = errors.New("unsupported media filename")
			return
		}
		dest := a.offline.MediaPath(file)
		size, downloadErr := a.client.DownloadTrack(ctx, track, dest, func(bytes, total int64) {
			a.updateDownload(func(j *downloadJob) { j.Bytes = bytes; j.Expected = total })
		})
		if downloadErr != nil {
			err = downloadErr
			return
		}
		saved := offline.Track{ID: track.RatingKey, Title: track.Title, Artist: track.GrandparentTitle, Album: track.ParentTitle, Duration: track.Duration, File: file, Size: size}
		if err = a.offline.Append(key, saved); err != nil {
			return
		}
		ordered = append(ordered, saved)
		a.updateDownload(func(j *downloadJob) { j.Done = i + 1; j.Current = track.Title; j.Bytes = 0; j.Expected = 0 })
	}
	if err = a.offline.Finish(key, ordered); err != nil {
		return
	}
	a.updateDownload(func(j *downloadJob) { j.Current = "Download complete" })
}
func (a *app) downloadSummary() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.download.Active {
		return "Ready"
	}
	return fmt.Sprintf("%d/%d tracks", a.download.Done, a.download.Total)
}
