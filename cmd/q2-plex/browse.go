package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
	"github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

func musicRow(it plex.Item, kind, parent string) q2spot.Row {
	switch it.Type {
	case "artist":
		return list("artist", it.Title, "Artist", it.RatingKey)
	case "album":
		return list("album", it.Title, it.ParentTitle, it.RatingKey)
	case "playlist":
		return list("playlist", it.Title, "Music playlist", it.RatingKey)
	case "track":
		ctx := it.RatingKey
		if kind == "playlist" {
			ctx = "pl:" + parent + ":" + it.RatingKey
		}
		if kind == "album" {
			ctx = parent + ":" + it.RatingKey
		}
		title := it.Title
		if it.UserRating != nil && *it.UserRating >= 9.5 {
			title = "* " + title
		}
		row := act("play", title, ctx)
		row.Subtitle = it.GrandparentTitle
		// Firmware sends Position as the op on a long press of Play/Pause,
		// and sends Context after that op. A normal press keeps playing.
		row.Position = "favtoggle"
		return row
	}
	return text(it.Title)
}
func (a *app) withArt(reply q2spot.Reply, rows []artworkJob) q2spot.Reply {
	if a.art == nil {
		return reply
	}
	return a.art.decorate(a.client, reply, rows)
}
func (a *app) searchReply(ctx context.Context, q string) q2spot.Reply {
	items, err := a.client.Search(ctx, q)
	if err != nil {
		return q2spot.Fail("error", "Plex search failed")
	}
	r := q2spot.Reply{Rows: []q2spot.Row{{Kind: "search", Title: "Search Plex Music"}}}
	art := make([]artworkJob, 0, len(items))
	for _, it := range items {
		row := musicRow(it, "", "")
		r.Rows = append(r.Rows, row)
		if thumb := itemArtwork(it); thumb != "" {
			art = append(art, artworkJob{row: row, source: thumb})
		}
	}
	if len(items) == 0 {
		r.Rows = append(r.Rows, text("No music found"))
	}
	return a.withArt(r, art)
}
func parsePage(s string) (int, bool) {
	if s == "" {
		return 0, true
	}
	n, err := strconv.Atoi(s)
	return n, err == nil && n >= 0 && n <= 100000
}
func (a *app) extra(ctx context.Context, req q2spot.Request) (q2spot.Reply, bool) {
	switch req.Op {
	case "searchpage":
		return q2spot.Reply{Rows: []q2spot.Row{
			{Kind: "search", Title: "Search Plex Music"},
			text("Search artists, albums and songs"),
		}}, true
	case "search":
		if len(req.Args) < 1 {
			return q2spot.Fail("bad", "Enter a search term"), true
		}
		return a.searchReply(ctx, req.Args[0]), true
	case "playlists", "favorites", "playlist":
		id, raw := "", ""
		if req.Op == "playlist" {
			if len(req.Args) < 1 || !numericID(req.Args[0]) {
				return q2spot.Fail("bad", "Invalid playlist"), true
			}
			id = req.Args[0]
			if len(req.Args) > 1 {
				raw = req.Args[1]
			}
		} else if len(req.Args) > 0 {
			raw = req.Args[0]
		}
		offset, ok := parsePage(raw)
		if !ok {
			return q2spot.Fail("bad", "Invalid page"), true
		}
		var items []plex.Item
		var more bool
		var err error
		if req.Op == "playlists" {
			items, more, err = a.client.Playlists(ctx, offset)
		} else {
			items, more, err = a.client.Items(ctx, req.Op, id, offset)
		}
		if err != nil {
			return q2spot.Fail("error", "Could not load Plex music"), true
		}
		reply := q2spot.Reply{}
		art := make([]artworkJob, 0, len(items))
		if offset == 0 {
			if req.Op == "playlist" {
				reply.Rows = append(reply.Rows, act("playplaylist", "Play Playlist", id))
				reply.Rows = append(reply.Rows,act("downloadplaylist","Download Playlist to SD",id))
			}
			if req.Op == "playlist" || req.Op == "favorites" {
				reply.Rows = append(reply.Rows, text("Hold Play/Pause on a song to toggle Favorite"))
			}
		}
		for _, it := range items {
			if req.Op == "playlists" && it.Type != "playlist" {
				continue
			}
			if req.Op == "favorites" && it.Type != "track" {
				continue
			}
			if req.Op == "playlist" && it.Type != "track" {
				continue
			}
			row := musicRow(it, req.Op, id)
			reply.Rows = append(reply.Rows, row)
			if thumb := itemArtwork(it); thumb != "" && req.Op != "playlist" {
				art = append(art, artworkJob{row: row, source: thumb})
			}
		}
		if len(reply.Rows) == 0 {
			reply.Rows = append(reply.Rows, text("No music found"))
		}
		if more {
			if req.Op == "playlists" {
				reply.Cursor = strconv.Itoa(offset + 25)
			} else {
				reply.Cursor = strconv.Itoa(offset + len(items))
			}
		}
		return a.withArt(reply, art), true
	case "playplaylist":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing playlist"), true
		}
		return a.playPlaylist(ctx, req.Args[0], ""), true
	case "favtoggle":
		if len(req.Args) == 0 {
			return q2spot.Fail("bad", "Missing track"), true
		}
		id := req.Args[0]
		if i := strings.LastIndexByte(id, ':'); i >= 0 {
			id = id[i+1:]
		}
		if !numericID(id) {
			return q2spot.Fail("bad", "Invalid track"), true
		}
		favorite, err := a.client.FavoriteState(ctx, id)
		if err != nil {
			return q2spot.Fail("error", "Could not read favorite"), true
		}
		if err = a.client.SetFavorite(ctx, id, !favorite); err != nil {
			return q2spot.Fail("error", "Could not update favorite"), true
		}
		if favorite {
			return q2spot.Reply{Message: "Removed Favorite"}, true
		}
		return q2spot.Reply{Message: "Added Favorite"}, true
	}
	return q2spot.Reply{}, false
}
func (a *app) playPlaylist(ctx context.Context, id, selected string) q2spot.Reply {
	if !numericID(id) {
		return q2spot.Fail("bad", "Invalid playlist")
	}
	tracks, err := a.client.PlaylistTracks(ctx, id, queueLimit)
	if err != nil {
		return q2spot.Fail("error", "Could not load playlist tracks")
	}
	if len(tracks) == 0 {
		return q2spot.Fail("unavailable", "Playlist is empty")
	}
	start := 0
	if selected != "" {
		start = -1
		for i, it := range tracks {
			if it.RatingKey == selected {
				start = i
				break
			}
		}
		if start < 0 {
			return q2spot.Fail("unavailable", "Track not in playlist")
		}
	}
	if err := a.startQueue(tracks, start); err != nil {
		return q2spot.Fail("error", "Could not start playlist")
	}
	return q2spot.Reply{Message: "Playing " + cleanField(tracks[start].Title)}
}
