package offline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const MaxManifestBytes = 4 << 20
const KeepFreeBytes int64 = 256 << 20
const MaxTrackBytes int64 = 512 << 20

// A collection owns an ordered list of tracks already fully committed to the card.
// A partial transfer is never in Tracks. Playlist duplicates are preserved by Finish.
type Track struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int64  `json:"duration"` // milliseconds
	File     string `json:"file"`     // file name only, under offline/music
	Size     int64  `json:"size"`
}
type Collection struct {
	Key      string  `json:"key"`
	Kind     string  `json:"kind"`
	Server   string  `json:"server"`
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Tracks   []Track `json:"tracks"`
	Complete bool    `json:"complete"`
	Updated  int64   `json:"updated"`
}
type manifest struct {
	Version     int                   `json:"version"`
	Collections map[string]Collection `json:"collections"`
}
type Store struct {
	mu   sync.Mutex
	Root string
	data manifest
}

func validID(s string) bool {
	if len(s) == 0 || len(s) > 40 {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

var validExtensions = map[string]bool{
	".flac": true, ".mp3": true, ".wav": true, ".m4a": true, ".aac": true,
	".ogg": true, ".opus": true, ".ape": true, ".aiff": true,
}

func ValidFile(name string) bool {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, "/\x00\\") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !validExtensions[ext] {
		return false
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if len(base) < 10 || len(base) > 49 || base[8] != '-' {
		return false
	} // 8 hex server prefix - Plex numeric ID
	if _, err := hex.DecodeString(base[:8]); err != nil {
		return false
	}
	return validID(base[9:])
}
func ServerTag(server string) string {
	h := sha256.Sum256([]byte(server))
	return hex.EncodeToString(h[:4])
}
func CollectionKey(server, kind, id string) string {
	if (kind != "album" && kind != "playlist") || !validID(id) {
		return ""
	}
	return ServerTag(server) + ":" + kind + ":" + id
}
func MediaFile(server, id, extension string) string {
	extension = strings.ToLower(extension)
	if !validID(id) || !validExtensions[extension] {
		return ""
	}
	return ServerTag(server) + "-" + id + extension
}
func Load(root string) (*Store, error) {
	s := &Store{Root: root, data: manifest{Version: 1, Collections: map[string]Collection{}}}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "music"), 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(root, "index.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > MaxManifestBytes {
		return nil, errors.New("offline manifest exceeds limit")
	}
	if err = json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("offline manifest: %w", err)
	}
	if s.data.Version != 1 {
		return nil, errors.New("unknown offline manifest version")
	}
	if s.data.Collections == nil {
		s.data.Collections = map[string]Collection{}
	}
	if len(s.data.Collections) > 512 {
		return nil, errors.New("too many offline collections")
	}
	for key, col := range s.data.Collections {
		if key != col.Key || CollectionKey(col.Server, col.Kind, col.ID) != key || len(col.Tracks) > 250 {
			return nil, errors.New("invalid offline collection metadata")
		}
		for _, tr := range col.Tracks {
			if !validID(tr.ID) || !ValidFile(tr.File) || tr.Size < 0 || tr.Size > MaxTrackBytes {
				return nil, errors.New("invalid offline track metadata")
			}
		}
	}
	return s, nil
}
func (s *Store) mediaPath(file string) string {
	if !ValidFile(file) {
		return ""
	}
	return filepath.Join(s.Root, "music", file)
}
func (s *Store) MediaPath(file string) string { return s.mediaPath(file) }
func (s *Store) Exists(t Track) bool {
	p := s.mediaPath(t.File)
	if p == "" {
		return false
	}
	st, err := os.Lstat(p)
	return err == nil && st.Mode().IsRegular() && st.Size() > 0 && (t.Size <= 0 || st.Size() == t.Size)
}
func (s *Store) List() []Collection {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := make([]Collection, 0, len(s.data.Collections))
	for _, v := range s.data.Collections {
		v.Tracks = append([]Track(nil), v.Tracks...)
		r = append(r, v)
	}
	sort.Slice(r, func(i, j int) bool {
		if r[i].Updated == r[j].Updated {
			return r[i].Title < r[j].Title
		}
		return r[i].Updated > r[j].Updated
	})
	return r
}
func (s *Store) Get(key string) (Collection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data.Collections[key]
	v.Tracks = append([]Track(nil), v.Tracks...)
	return v, ok
}
func (s *Store) saveLocked() error {
	b, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	if len(b) > MaxManifestBytes {
		return errors.New("offline manifest too large")
	}
	p := filepath.Join(s.Root, "index.json")
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err = f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err = os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Begin keeps already downloaded tracks playable while a collection is resynced.
func (s *Store) Begin(server, kind, id, title string) error {
	key := CollectionKey(server, kind, id)
	if key == "" {
		return errors.New("invalid offline collection")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data.Collections) >= 512 {
		if _, exists := s.data.Collections[key]; !exists {
			return errors.New("offline collection limit reached")
		}
	}
	before, exists := s.data.Collections[key]
	next := before
	next.Key = key
	next.Server = server
	next.Kind = kind
	next.ID = id
	next.Title = title
	next.Complete = false
	next.Updated = time.Now().Unix()
	s.data.Collections[key] = next
	if err := s.saveLocked(); err != nil {
		if exists {
			s.data.Collections[key] = before
		} else {
			delete(s.data.Collections, key)
		}
		return err
	}
	return nil
}
func (s *Store) Append(key string, track Track) error {
	if !validID(track.ID) || !ValidFile(track.File) || !s.Exists(track) {
		return errors.New("track is not safely stored on card")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	col, ok := s.data.Collections[key]
	if !ok {
		return errors.New("offline collection does not exist")
	}
	before := col
	col.Tracks = append([]Track(nil), before.Tracks...)
	found := false
	for i, it := range col.Tracks {
		if it.ID == track.ID {
			col.Tracks[i] = track
			found = true
			break
		}
	}
	if !found {
		if len(col.Tracks) >= 250 {
			return errors.New("offline track limit reached")
		}
		col.Tracks = append(col.Tracks, track)
	}
	col.Updated = time.Now().Unix()
	s.data.Collections[key] = col
	if err := s.saveLocked(); err != nil {
		s.data.Collections[key] = before
		return err
	}
	return nil
}
func (s *Store) Finish(key string, ordered []Track) error {
	if len(ordered) > 250 {
		return errors.New("offline track limit reached")
	}
	for _, tr := range ordered {
		if !s.Exists(tr) {
			return errors.New("offline track missing")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before, ok := s.data.Collections[key]
	if !ok {
		return errors.New("offline collection missing")
	}
	col := before
	col.Tracks = append([]Track(nil), ordered...)
	col.Complete = true
	col.Updated = time.Now().Unix()
	s.data.Collections[key] = col
	if err := s.saveLocked(); err != nil {
		s.data.Collections[key] = before
		return err
	}
	return nil
}
func (s *Store) Remove(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, ok := s.data.Collections[key]
	if !ok {
		return errors.New("offline collection missing")
	}
	delete(s.data.Collections, key)
	if err := s.saveLocked(); err != nil {
		s.data.Collections[key] = before
		return err
	}
	// Delete only files no remaining collection references. A crash here leaves an
	// orphan file, never a manifest referencing a file the app has removed.
	used := map[string]bool{}
	for _, other := range s.data.Collections {
		for _, tr := range other.Tracks {
			used[tr.File] = true
		}
	}
	for _, tr := range before.Tracks {
		if used[tr.File] {
			continue
		}
		if p := s.mediaPath(tr.File); p != "" {
			_ = os.Remove(p)
		}
	}
	return nil
}
func FreeBytes(dir string) (int64, error) {
	var info syscall.Statfs_t
	if err := syscall.Statfs(dir, &info); err != nil {
		return 0, err
	}
	return int64(info.Bavail) * int64(info.Bsize), nil
}
func CheckSpace(dir string, required int64) error {
	available, err := FreeBytes(dir)
	if err != nil {
		return err
	}
	if required < 0 || available-required < KeepFreeBytes {
		return fmt.Errorf("SD card low on space; keep at least %d MB free", KeepFreeBytes>>20)
	}
	return nil
}
