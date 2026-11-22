package plex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func trackForDownload(key string, size int64) Item {
	it := Item{Type: "track", RatingKey: "101", Title: "Rock Song"}
	it.Media = make([]struct {
		Part []struct {
			Key       string `json:"key"`
			Container string `json:"container"`
			Size      int64  `json:"size"`
		} `json:"Part"`
	}, 1)
	it.Media[0].Part = append(it.Media[0].Part, struct {
		Key       string `json:"key"`
		Container string `json:"container"`
		Size      int64  `json:"size"`
	}{Key: key, Container: "flac", Size: size})
	return it
}
func TestOriginalQualityDownloadAndResume(t *testing.T) {
	body := []byte("fLaC0123456789ABCDEFGHIJ")
	calledRange := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "abc" {
			t.Error("missing header authentication")
		}
		if r.URL.Path != "/library/parts/88/file.flac" {
			t.Errorf("bad part path %s", r.URL.Path)
		}
		if r.Header.Get("Range") != "" {
			calledRange = true
			if r.Header.Get("Range") != "bytes=4-" {
				t.Errorf("bad Range: %s", r.Header.Get("Range"))
			}
			w.Header().Set("Content-Range", "bytes 4-23/24")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(body[4:])
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	c.State.ServerToken = "abc"
	key := "/library/parts/88/file.flac"
	item := trackForDownload(key, int64(len(body)))
	root := filepath.Join(t.TempDir(), "music")
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "song.flac")
	if err = os.WriteFile(dest+".part", body[:4], 0600); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(key + "|24"))
	if err = os.WriteFile(dest+".part.key", []byte(hex.EncodeToString(h[:])), 0600); err != nil {
		t.Fatal(err)
	}
	size, err := c.DownloadTrack(context.Background(), item, dest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !calledRange {
		t.Fatal("did not resume original audio")
	}
	if size != int64(len(body)) {
		t.Fatalf("size %d", size)
	}
	saved, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != string(body) {
		t.Fatalf("bad download %q", saved)
	}
	if _, err = os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal("partial file not cleared")
	}
}
func TestRejectVideoAndUnsafeParts(t *testing.T) {
	if _, err := MediaExtension(Item{Type: "movie", RatingKey: "8"}); err == nil {
		t.Fatal("allowed movie")
	}
	for _, part := range []string{"http://evil.example/video", "/library/parts/../secret", "/metadata/not-a-part", "bad\r\n", "/library/parts/9/video.mp4"} {
		if strings.Contains(part, "mp4") {
			continue
		}
		if validPartPath(part) {
			t.Errorf("accepted unsafe part %q", part)
		}
	}
}
