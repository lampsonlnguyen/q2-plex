package main

import (
	"strconv"
	"strings"
	"testing"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
)

func TestQueueStartFromTrack(t *testing.T) {
	tracks := []plex.Item{
		{Type: "track", RatingKey: "123", Title: "One", ParentTitle: "Album", GrandparentTitle: "Artist", Duration: 75000},
		{Type: "track", RatingKey: "456", Title: "Two", ParentTitle: "Album", GrandparentTitle: "Artist", Duration: 92000},
	}
	data, err := buildURLs(tracks, 1)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.HasPrefix(s, "#Q2URLS/1\tstart=1\tnow=1\n") {
		t.Fatalf("start %s", s)
	}
	if !strings.Contains(s, "http://127.0.0.1:2328/song/456\tTwo\tArtist\tAlbum\thttp://127.0.0.1:2328/cover/456\t92\t456\t0\n") {
		t.Fatal(s)
	}
	if strings.Contains(s, "X-Plex-Token") {
		t.Fatal("token leaked")
	}
}
func TestQueueRejectsBadTrack(t *testing.T) {
	if _, err := buildURLs([]plex.Item{{Type: "movie", RatingKey: "1"}}, 0); err == nil {
		t.Fatal("accepted video")
	}
	if _, err := buildURLs([]plex.Item{{Type: "track", RatingKey: "../foo"}}, 0); err == nil {
		t.Fatal("accepted bad ID")
	}
	if _, err := buildURLs([]plex.Item{{Type: "track", RatingKey: "1"}}, 1); err == nil {
		t.Fatal("accepted bad index")
	}
}

func TestNativeQueueBrowseAllTracks(t *testing.T) {
	tracks := make([]plex.Item, queueLimit)
	for i := range tracks {
		tracks[i] = plex.Item{Type: "track", RatingKey: strconv.Itoa(i + 1), Title: "Track " + strconv.Itoa(i+1)}
	}
	a := &app{queue: playbackQueue{Tracks: tracks}}
	seen := 0
	cursor := ""
	for {
		var args []string
		if cursor != "" {
			args = []string{cursor}
		}
		page := a.queuePage(args)
		if page.Status != "" {
			t.Fatalf("unexpected status %q", page.Status)
		}
		if len(page.Rows) == 0 || len(page.Rows) > 40 {
			t.Fatalf("invalid page size %d", len(page.Rows))
		}
		for _, row := range page.Rows {
			if row.URI != "playqueue" || row.Context != strconv.Itoa(seen) {
				t.Fatalf("track %d: %+v", seen, row)
			}
			seen++
		}
		if page.Cursor == "" {
			break
		}
		if page.Cursor == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = page.Cursor
	}
	if seen != queueLimit {
		t.Fatalf("viewed %d of %d tracks", seen, queueLimit)
	}
	if page := a.queuePage([]string{"bad"}); page.Status != "bad" {
		t.Fatal("accepted malformed cursor")
	}
	if page := a.queuePage([]string{"250"}); page.Status != "bad" {
		t.Fatal("accepted out-of-range cursor")
	}
	if page := a.queuePage([]string{"1", "2"}); page.Status != "bad" {
		t.Fatal("accepted multiple cursors")
	}
}

func TestNativeQueueBrowseEmpty(t *testing.T) {
	page := (&app{}).queuePage(nil)
	if len(page.Rows) != 1 || page.Rows[0].Title != "No active queue" {
		t.Fatalf("unexpected empty queue: %+v", page)
	}
}
