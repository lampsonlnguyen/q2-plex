package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// The artist browse screen must use the library-wide album index rather than
// /library/metadata/{id}/children, which can hide EPs and singles in Plex.
func TestArtistIncludesAlbumsEPsAndSingles(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/library/sections/7/all" {
			t.Errorf("path = %q (wanted library album index)", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("type") != "9" || query.Get("artist.id") != "42" {
			t.Errorf("missing album type or artist filter: %s", r.URL.RawQuery)
		}
		if query.Get("X-Plex-Container-Start") != "0" ||
			query.Get("X-Plex-Container-Size") != "26" {
			t.Errorf("wrong native list page: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"MediaContainer":{"Metadata":[{"type":"album","ratingKey":"101","title":"LP","parentRatingKey":"42"},{"type":"album","ratingKey":"102","title":"EP","parentRatingKey":"42"},{"type":"album","ratingKey":"103","title":"Single","parentRatingKey":"42"}]}}`)
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	c.State.LibraryID = "7"
	items, more, err := c.Items(context.Background(), "artist", "42", 0)
	if err != nil {
		t.Fatal(err)
	}
	if more || len(items) != 3 || calls != 1 {
		t.Fatalf("items=%+v more=%t calls=%d", items, more, calls)
	}
	for i, key := range []string{"101", "102", "103"} {
		if items[i].RatingKey != key || items[i].Type != "album" {
			t.Fatalf("item %d = %+v", i, items[i])
		}
	}
}

func TestArtistReleasesUsePlexPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/library/sections/3/all" ||
			q.Get("artist.id") != "77" || q.Get("type") != "9" {
			t.Errorf("unexpected artist request %s", r.URL.String())
		}
		offset, _ := strconv.Atoi(q.Get("X-Plex-Container-Start"))
		size, _ := strconv.Atoi(q.Get("X-Plex-Container-Size"))
		if size != pageSize+1 {
			t.Errorf("expected pageSize+1, got %d", size)
		}
		fmt.Fprint(w, `{"MediaContainer":{"Metadata":[`)
		for i := offset; i < 31 && i < offset+size; i++ {
			if i != offset {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"type":"album","ratingKey":"%d","title":"Release %d"}`, 100+i, i)
		}
		fmt.Fprint(w, `]}}`)
	}))
	defer srv.Close()
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.ServerURL = srv.URL
	c.State.LibraryID = "3"
	first, more, err := c.Items(context.Background(), "artist", "77", 0)
	if err != nil || !more || len(first) != pageSize {
		t.Fatalf("first page=%d more=%t err=%v", len(first), more, err)
	}
	last, more, err := c.Items(context.Background(), "artist", "77", len(first))
	if err != nil || more || len(last) != 6 {
		t.Fatalf("second page=%d more=%t err=%v", len(last), more, err)
	}
	if first[0].RatingKey != "100" || last[5].RatingKey != "130" {
		t.Fatalf("incorrect order: first=%+v last=%+v", first[0], last[5])
	}
}

func TestArtistRejectsNonNumericIDs(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.State.LibraryID = "3"
	for _, id := range []string{"", "../../secret", "77&artist.id=88"} {
		if _, _, err := c.Items(context.Background(), "artist", id, 0); err == nil {
			t.Fatalf("artist ID %q was accepted", id)
		}
	}
}
