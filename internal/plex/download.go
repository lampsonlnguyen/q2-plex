package plex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lampsonlnguyen/q2-plex/internal/offline"
)

// MediaExtension chooses a file suffix recognized by Q2's native audio player.
// Downloads stay original quality: no transcoding and no video media.
func MediaExtension(it Item) (string, error) {
	if it.Type != "track" || len(it.Media) == 0 || len(it.Media[0].Part) == 0 {
		return "", errors.New("Plex item has no audio media")
	}
	part := it.Media[0].Part[0]
	ext := strings.ToLower(strings.TrimPrefix(part.Container, "."))
	if ext == "alac" {
		ext = "m4a"
	}
	if ext == "" {
		u, err := url.Parse(part.Key)
		if err == nil {
			ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(u.Path)), ".")
		}
	}
	switch ext {
	case "flac", "mp3", "wav", "m4a", "aac", "ogg", "opus", "ape", "aiff":
		return "." + ext, nil
	}
	return "", fmt.Errorf("unsupported audio format %q", ext)
}

func validPartPath(part string) bool {
	if len(part) > 512 || strings.Contains(part, "..") || strings.ContainsAny(part, "\r\n\x00") {
		return false
	}
	u, err := url.Parse(part)
	return err == nil && u.Scheme == "" && u.Host == "" && strings.HasPrefix(u.Path, "/library/parts/")
}

// DownloadTrack streams a single original audio part to an SD-card .part file.
// A partial transfer resumes only if the Plex part ID and source size match.
// The final file is fsynced and renamed atomically before the caller records it.
func (c *Client) DownloadTrack(ctx context.Context, it Item, dest string, progress func(int64, int64)) (int64, error) {
	if it.Type != "track" || !ratingKey(it.RatingKey) || len(it.Media) == 0 || len(it.Media[0].Part) == 0 {
		return 0, errors.New("invalid Plex music track")
	}
	p := it.Media[0].Part[0]
	if !validPartPath(p.Key) || !allowedRoot(c.State.ServerURL) {
		return 0, errors.New("invalid Plex media source")
	}
	if _, err := MediaExtension(it); err != nil {
		return 0, err
	}
	if p.Size > offline.MaxTrackBytes {
		return 0, errors.New("track exceeds offline size limit")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return 0, err
	}
	if st, err := os.Lstat(dest); err == nil && st.Mode().IsRegular() && st.Size() > 0 && p.Size > 0 && st.Size() == p.Size {
		if progress != nil {
			progress(st.Size(), st.Size())
		}
		return st.Size(), nil
	}
	partial := dest + ".part"
	keyPath := partial + ".key"
	sourceSum := sha256.Sum256([]byte(p.Key + "|" + strconv.FormatInt(p.Size, 10)))
	sourceKey := hex.EncodeToString(sourceSum[:])
	from := int64(0)
	if p.Size > 0 {
		st, statErr := os.Lstat(partial)
		key, keyErr := os.ReadFile(keyPath)
		if statErr == nil && st.Mode().IsRegular() && st.Size() > 0 && st.Size() < p.Size && keyErr == nil && string(key) == sourceKey {
			from = st.Size()
		}
	}
	if from == 0 {
		if err := os.WriteFile(keyPath, []byte(sourceKey), 0600); err != nil {
			return 0, err
		}
	}
	root, err := url.Parse(c.State.ServerURL)
	if err != nil {
		return 0, err
	}
	rel, err := url.Parse(p.Key)
	if err != nil {
		return 0, err
	}
	target := root.ResolveReference(rel)
	var resp *http.Response
	// A 416 after a previously interrupted download retries once from zero.
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
		if err != nil {
			return 0, err
		}
		c.headers(req, c.State.ServerToken)
		req.Header.Set("Accept", "application/octet-stream")
		if from > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", from))
		}
		streamHTTP := &http.Client{Transport: c.HTTP.Transport, CheckRedirect: c.HTTP.CheckRedirect}
		resp, err = streamHTTP.Do(req)
		if err != nil {
			return 0, err
		}
		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && from > 0 {
			resp.Body.Close()
			from = 0
			continue
		}
		break
	}
	if resp == nil {
		return 0, errors.New("Plex returned no media")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("Plex download HTTP %d", resp.StatusCode)
	}
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "video/") {
		return 0, errors.New("Plex returned video instead of audio")
	}
	initial := int64(0)
	flags := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	if resp.StatusCode == http.StatusPartialContent {
		// Reject unsafely spliced bodies and incomplete or mismatched Content-Range.
		var first, last, total int64
		if n, _ := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &first, &last, &total); n != 3 || first != from || total <= 0 || last != total-1 || (p.Size > 0 && p.Size != total) || (resp.ContentLength >= 0 && resp.ContentLength != last-first+1) {
			return 0, errors.New("invalid Plex partial response")
		}
		if from > 0 {
			initial = from
			flags = os.O_CREATE | os.O_APPEND | os.O_WRONLY
		}
	} else if from > 0 {
		// Plex ignored Range; start over, never append a second whole file.
		from = 0
	}
	expected := p.Size
	if expected == 0 && resp.ContentLength > 0 {
		expected = initial + resp.ContentLength
	}
	if expected > offline.MaxTrackBytes {
		return 0, errors.New("track exceeds offline size limit")
	}
	if expected > 0 {
		if err := offline.CheckSpace(filepath.Dir(dest), expected-initial); err != nil {
			return 0, err
		}
	} else if err := offline.CheckSpace(filepath.Dir(dest), 16<<20); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(partial, flags, 0600)
	if err != nil {
		return 0, err
	}
	written := initial
	buf := make([]byte, 32<<10)
	nextSpaceCheck := written + (4 << 20)
	if progress != nil {
		progress(written, expected)
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			return 0, err
		}
		n, re := resp.Body.Read(buf)
		if n > 0 {
			if written+int64(n) > offline.MaxTrackBytes {
				_ = f.Close()
				return 0, errors.New("track exceeds offline size limit")
			}
			nw, we := f.Write(buf[:n])
			written += int64(n)
			if we != nil || nw != n {
				_ = f.Close()
				if we != nil {
					return 0, we
				}
				return 0, io.ErrShortWrite
			}
			if written >= nextSpaceCheck {
				if err := offline.CheckSpace(filepath.Dir(dest), 8<<20); err != nil {
					_ = f.Close()
					return 0, err
				}
				nextSpaceCheck = written + (4 << 20)
			}
			if progress != nil {
				progress(written, expected)
			}
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			_ = f.Close()
			return 0, re
		}
	}
	if expected > 0 && written != expected {
		_ = f.Close()
		return 0, fmt.Errorf("incomplete media %d/%d", written, expected)
	}
	if written <= 0 {
		_ = f.Close()
		return 0, errors.New("empty audio download")
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return 0, err
	}
	if err = f.Close(); err != nil {
		return 0, err
	}
	if err = os.Rename(partial, dest); err != nil {
		return 0, err
	}
	_ = os.Remove(keyPath)
	return written, nil
}
