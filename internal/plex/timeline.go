package plex

import (
	"context"
	"errors"
	"net/url"
	"strconv"
)

// ReportTimeline reports actual Q2 Pod audio-player state, never a guessed
// position. Plex uses timeline transitions to update play counts and history.
// Do not call /:/scrobble in addition: it can double-count listens.
func (c *Client) ReportTimeline(ctx context.Context, id, state string, positionSec, durationSec int64, continuing bool) error {
	if !ratingKey(id) {
		return errors.New("invalid music rating key")
	}
	if state != "playing" && state != "paused" && state != "stopped" {
		return errors.New("invalid playback state")
	}
	if positionSec < 0 || positionSec > 86400 || durationSec < 0 || durationSec > 86400 {
		return errors.New("invalid playback position")
	}
	if c.State.ServerURL == "" || c.State.ServerToken == "" {
		return errors.New("Plex server not connected")
	}
	q := url.Values{}
	q.Set("key", "/library/metadata/"+id)
	q.Set("ratingKey", id)
	q.Set("state", state)
	q.Set("time", strconv.FormatInt(positionSec*1000, 10))
	q.Set("duration", strconv.FormatInt(durationSec*1000, 10))
	if state == "stopped" && continuing {
		q.Set("continuing", "1")
	}
	// The authenticated Plex request includes X-Plex-Client-Identifier.
	return c.Request(ctx, "POST", c.State.ServerURL, "/:/timeline?"+q.Encode(), c.State.ServerToken, nil, nil)
}
