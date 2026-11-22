package main

import (
	"strings"
	"testing"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
)

func TestLocalQueuePathsNoPlexRequests(t *testing.T) {
	tracks := []plex.Item{{Type: "track", RatingKey: "55", Title: "Offline song", ParentTitle: "Album", GrandparentTitle: "Artist", Duration: 120000}}
	local := map[string]string{"55": "/mnt/mmc/PlexMusic/music/2d82d85a-55.flac"}
	data, err := buildURLsWithLocal(tracks, 0, local)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "/mnt/mmc/PlexMusic/music/2d82d85a-55.flac\tOffline song") {
		t.Fatalf("missing native card path %q", s)
	}
	if strings.Contains(s, "http://127.0.0.1:2328/song/55") || strings.Contains(s, "X-Plex-Token") {
		t.Fatal("offline queue requires Plex relay")
	}
}
func TestRejectUnsafeLocalQueuePath(t *testing.T) {
	tracks := []plex.Item{{Type: "track", RatingKey: "5", Title: "Song"}}
	if _, err := buildURLsWithLocal(tracks, 0, map[string]string{"5": "/tmp/abc.flac"}); err == nil {
		t.Fatal("accepted file outside saved music")
	}
}
