package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReportTimelineUsesPlaybackState(t *testing.T) {
	var got bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = true
		if r.URL.Path != "/:/timeline" || r.Method != "POST" {
			t.Errorf("wrong request %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("ratingKey") != "77" || q.Get("key") != "/library/metadata/77" || q.Get("state") != "paused" || q.Get("time") != "52000" || q.Get("duration") != "200000" {
			t.Errorf("incorrect timeline %v", q)
		}
		if r.Header.Get("X-Plex-Token") != "token" {
			t.Error("no Plex authentication")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	c.State.ServerToken = "token"
	if err = c.ReportTimeline(context.Background(), "77", "paused", 52, 200, false); err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("timeline not sent")
	}
	for _, id := range []string{"../a", "", "abc"} {
		if err = c.ReportTimeline(context.Background(), id, "playing", 1, 100, false); err == nil {
			t.Errorf("accepted %q", id)
		}
	}
}
