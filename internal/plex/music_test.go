package plex

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchReturnsMusicOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.URL.Query().Get("query") != "Kings of Leon" || r.URL.Query().Get("sectionId") != "7" {
			t.Errorf("query=%s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"MediaContainer":{"Hub":[{"Metadata":[{"type":"movie","ratingKey":"1","title":"Concert"},{"type":"artist","ratingKey":"12","title":"Kings of Leon"},{"type":"album","ratingKey":"13","title":"When You See Yourself"}]},{"Metadata":[{"type":"track","ratingKey":"14","title":"Echoing"},{"type":"artist","ratingKey":"12","title":"Duplicate"}]}]}}`)
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	c.State.LibraryID = "7"
	results, err := c.Search(context.Background(), "Kings of Leon")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || results[0].Type != "artist" || results[2].Type != "track" {
		t.Fatalf("%+v", results)
	}
}

func TestPlaylistAudioFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/playlists" || r.URL.Query().Get("playlistType") != "audio" {
			t.Errorf("path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"MediaContainer":{"Metadata":[{"type":"playlist","playlistType":"video","ratingKey":"1","title":"Film"},{"type":"playlist","playlistType":"audio","ratingKey":"2","title":"Rock"}]}}`)
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	items, more, err := c.Playlists(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if more || len(items) != 1 || items[0].Title != "Rock" {
		t.Fatalf("items=%+v more=%t", items, more)
	}
}
func TestFavoritesRating(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/metadata/15" {
			fmt.Fprint(w, `{"MediaContainer":{"Metadata":[{"type":"track","ratingKey":"15","userRating":10,"Media":[{"Part":[{"key":"/library/parts/15/file.flac"}]}]}]}}`)
			return
		}
		if r.URL.Path == "/:/rate" {
			called = true
			if r.Method != "PUT" || r.URL.Query().Get("key") != "15" || r.URL.Query().Get("rating") != "-1" {
				t.Errorf("invalid rating %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unknown %s", r.URL.Path)
		w.WriteHeader(404)
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	favorite, err := c.FavoriteState(context.Background(), "15")
	if err != nil || !favorite {
		t.Fatalf("favorite=%t err=%v", favorite, err)
	}
	if err = c.SetFavorite(context.Background(), "15", false); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("missing rate call")
	}
}
func TestThumbnailBindsPlexTokenAndChecksJPEG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "test-token" {
			t.Errorf("missing token")
		}
		if r.URL.Path != "/photo/:/transcode" || !strings.HasPrefix(r.URL.Query().Get("url"), "/library/") {
			t.Errorf("art URL=%s", r.URL.String())
		}
		im := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				im.Set(x, y, color.RGBA{R: 40, G: 70, B: 120, A: 255})
			}
		}
		w.Header().Set("Content-Type", "image/jpeg")
		if err := jpeg.Encode(w, im, nil); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	c.State.ServerToken = "test-token"
	if _, err := c.Thumbnail(context.Background(), "https://attacker.example/image", 64); err == nil {
		t.Fatal("accepted remote image")
	}
	if _, err := c.Thumbnail(context.Background(), "/library/metadata/42/thumb/111", 64); err != nil {
		t.Fatal(err)
	}
}
