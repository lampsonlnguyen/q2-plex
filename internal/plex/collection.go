package plex

import (
	"context"
	"errors"
	"net/url"
)

// CollectionName returns a display name for a music album or audio playlist.
// The caller can continue with an ID fallback if this metadata lookup fails.
func (c *Client) CollectionName(ctx context.Context, kind, id string) (string, error) {
	if !ratingKey(id) {
		return "", errors.New("invalid collection ID")
	}
	var path string
	switch kind {
	case "album":
		path = "/library/metadata/" + url.PathEscape(id)
	case "playlist":
		path = "/playlists/" + url.PathEscape(id)
	default:
		return "", errors.New("unsupported collection kind")
	}
	var result container
	if err := c.Request(ctx, "GET", c.State.ServerURL, path, c.State.ServerToken, nil, &result); err != nil {
		return "", err
	}
	if len(result.MediaContainer.Metadata) == 0 {
		return "", errors.New("collection unavailable")
	}
	entry := result.MediaContainer.Metadata[0]
	if entry.Type != kind {
		return "", errors.New("Plex returned the wrong media type")
	}
	return entry.Title, nil
}
