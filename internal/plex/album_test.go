package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAlbumTracksPaginatedAndBounded(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/library/metadata/77/children" {
			t.Errorf("bad path: %s", r.URL.Path)
		}
		off := r.URL.Query().Get("X-Plex-Container-Start")
		if off != "0" && off != "25" {
			t.Errorf("unexpected offset: %s", off)
		}
		items := make([]Item, 0, 26)
		start := 0
		if off == "25" {
			start = 25
		}
		for i := start; i < start+26; i++ {
			items = append(items, Item{Type: "track", RatingKey: fmt.Sprint(i + 1), Title: fmt.Sprint("Song ", i+1)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": items}})
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	songs, err := c.AlbumTracks(context.Background(), "77", 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 30 || songs[0].RatingKey != "1" || songs[29].RatingKey != "30" {
		t.Fatalf("wrong tracks %d %s", len(songs), songs[0].RatingKey)
	}
	if calls != 2 {
		t.Fatalf("wanted two pages, got %d", calls)
	}
}
func TestStreamDoesNotHaveNineSecondTotalDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/library/metadata/") {
			fmt.Fprint(w, `{"MediaContainer":{"Metadata":[{"type":"track","ratingKey":"2","Media":[{"Part":[{"key":"/library/parts/3/file.flac"}]}]}]}}`)
			return
		}
		if r.URL.Path != "/library/parts/3/file.flac" {
			t.Errorf("bad part: %s", r.URL.Path)
		}
		fmt.Fprint(w, "FLAC")
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	req := httptest.NewRequest("GET", "http://127.0.0.1/song/2", nil)
	recorder := httptest.NewRecorder()
	if err := c.Stream(context.Background(), "2", recorder, req); err != nil {
		t.Fatal(err)
	}
	if recorder.Body.String() != "FLAC" {
		t.Fatalf("unexpected body %q", recorder.Body.String())
	}
}
