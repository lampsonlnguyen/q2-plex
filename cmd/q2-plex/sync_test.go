package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lampsonlnguyen/q2-plex/internal/playback"
	"github.com/lampsonlnguyen/q2-plex/internal/plex"
)

func TestFavoriteEventParsing(t *testing.T) {
	kind, id, favorite, err := parseFavorite("item\t1\t121")
	if err != nil || kind != "item" || id != "121" || !favorite {
		t.Fatalf("bad favorite event %s %s %v %v", kind, id, favorite, err)
	}
	kind, id, favorite, err = parseFavorite("path\t0\t/mnt/mmc/PlexMusic/music/deadbeef-125.flac")
	if err != nil || kind != "path" || favorite || id == "" {
		t.Fatal(err)
	}
	for _, bad := range []string{"item\t2\t77", "item\t1\t../77", "path\t1\t/etc/passwd", "item\t0", "bogus\t0\t2"} {
		if _, err := func() (bool, error) { _, _, _, e := parseFavorite(bad); return true, e }(); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
func TestNowReportsOnlyQueueAndTruePlayerState(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Query().Get("state")+":"+r.URL.Query().Get("ratingKey"))
		w.WriteHeader(200)
	}))
	defer srv.Close()
	cl, err := plex.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cl.State.ServerURL = srv.URL
	cl.State.ServerToken = "token"
	cl.State.ServerName = "NAS"
	dir := t.TempDir()
	syncer, err := newPlaybackSync(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{client: cl, syncer: syncer}
	tracks := []plex.Item{{Type: "track", RatingKey: "1", Title: "One", Duration: 70000}, {Type: "track", RatingKey: "2", Title: "Two", Duration: 80000}}
	if err = a.rememberQueue(tracks, 0, nil, "10001"); err != nil {
		t.Fatal(err)
	}
	a.observeNow(context.Background(), playback.Now{Seq: "old", Index: 0, State: "playing", Seconds: 10, Length: 70})
	if len(calls) != 0 {
		t.Fatal("reported unrelated queue")
	}
	a.observeNow(context.Background(), playback.Now{Seq: "10001", Index: 0, State: "playing", Seconds: 10, Length: 70})
	a.observeNow(context.Background(), playback.Now{Seq: "10001", Index: 0, State: "playing", Seconds: 11, Length: 70})
	if len(calls) != 1 {
		t.Fatalf("duplicated timeline %v", calls)
	}
	a.observeNow(context.Background(), playback.Now{Seq: "10001", Index: 1, State: "paused", Seconds: 2, Length: 80})
	if len(calls) != 3 || calls[1] != "stopped:1" || calls[2] != "paused:2" {
		t.Fatalf("missing transition %v", calls)
	}
	snap := syncer.store.Snapshot()
	if snap.Index != 1 || snap.Seconds != 2 {
		t.Fatalf("position not saved %+v", snap)
	}
}
func TestRestoreQueuePersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := newPlaybackSync(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{client: &plex.Client{State: plex.State{ServerName: "NAS"}}, syncer: s}
	tracks := []plex.Item{{Type: "track", RatingKey: "10", Title: "Some Song", Duration: 40000}}
	if err = a.rememberQueue(tracks, 0, nil, "999"); err != nil {
		t.Fatal(err)
	}
	b := &app{client: a.client, syncer: s}
	b.loadSavedQueue()
	if len(b.queue.Tracks) != 1 || b.queue.Tracks[0].Title != "Some Song" {
		t.Fatal("lost playback queue")
	}
	if _, err = os.Stat(filepath.Join(dir, "last-queue.json")); err != nil {
		t.Fatal(err)
	}
}
