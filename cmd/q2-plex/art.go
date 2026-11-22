package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
	"github.com/lampsonlnguyen/q2-plex/internal/q2spot"
)

// Q2 Pod's art extension reads 64px JPEGs from /tmp/q2-plex.art/<FNV32a>.jpg.
// File names are based on the unescaped Q2SPOT URI + TAB + context.
type artworkJob struct {
	row    q2spot.Row
	source string
}
type artFetch struct {
	key, thumb string
	client     *plex.Client
}
type artworkCache struct {
	dir     string
	mu      sync.Mutex
	pending map[string]bool
	failed  map[string]time.Time
	jobs    chan artFetch
	enabled bool
}

const artEntries = 100
const artBudgetBytes = 3 << 20

func newArtworkCache(dir string) (*artworkCache, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	caps, _ := os.ReadFile("/tmp/q2-addon.caps")
	// Old firmware has no art extension. Avoid unnecessary network requests.
	enabled := strings.Contains(" "+string(caps)+" ", " art ")
	a := &artworkCache{dir: dir, enabled: enabled, pending: map[string]bool{}, failed: map[string]time.Time{}, jobs: make(chan artFetch, 16)}
	if enabled {
		// A single worker avoids concurrent image transcodes and radio wakeups.
		go a.worker()
	}
	return a, nil
}
func artKey(row q2spot.Row) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(row.URI + "\t" + row.Context))
	return fmt.Sprintf("%08x", h.Sum32())
}
func (a *artworkCache) path(key string) string { return filepath.Join(a.dir, key+".jpg") }
func (a *artworkCache) worker() {
	for job := range a.jobs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		raw, err := job.client.Thumbnail(ctx, job.thumb, 64)
		cancel()
		if err == nil {
			path := a.path(job.key)
			// Atomic rename is required: Q2 decodes art files while drawing the list.
			tmp := path + ".tmp"
			if err = os.WriteFile(tmp, raw, 0600); err == nil {
				err = os.Rename(tmp, path)
			}
			if err != nil {
				_ = os.Remove(tmp)
			}
		}
		a.mu.Lock()
		delete(a.pending, job.key)
		if err != nil {
			a.failed[job.key] = time.Now().Add(time.Minute)
		} else {
			delete(a.failed, job.key)
		}
		a.mu.Unlock()
		if err == nil {
			a.prune()
		}
	}
}
func (a *artworkCache) decorate(client *plex.Client, r q2spot.Reply, requests []artworkJob) q2spot.Reply {
	if a == nil || !a.enabled {
		return r
	}
	waiting := false
	for _, req := range requests {
		if req.source == "" {
			continue
		}
		key := artKey(req.row)
		if _, err := os.Stat(a.path(key)); err == nil {
			continue
		}
		a.mu.Lock()
		if until, ok := a.failed[key]; ok && time.Now().Before(until) {
			a.mu.Unlock()
			continue
		}
		if a.pending[key] {
			waiting = true
			a.mu.Unlock()
			continue
		}
		// Expired failed-art entries otherwise grow without bound after browsing
		// hundreds of albums on a long-running Q2.
		if len(a.failed)>128{
			now:=time.Now()
			for k,until:=range a.failed{
				if !now.Before(until){delete(a.failed,k)}
			}
			if len(a.failed)>128{a.failed=map[string]time.Time{}}
		}
		job := artFetch{key:key,thumb:req.source,client:client}
		select {
		case a.jobs <- job:
			a.pending[key] = true
			waiting = true
		default:
			// Workers are backlogged: a later navigation request may retry.
		}
		a.mu.Unlock()
	}
	if waiting && r.Status == "" && r.Message == "" {
		r.Message = "refresh"
	}
	return r
}
func (a *artworkCache) prune() {
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return
	}
	type asset struct {
		path string
		size int64
		at   time.Time
	}
	assets := make([]asset, 0, len(entries))
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jpg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		assets = append(assets, asset{path: filepath.Join(a.dir, e.Name()), size: info.Size(), at: info.ModTime()})
		total += info.Size()
	}
	if len(assets) <= artEntries && total <= artBudgetBytes {
		return
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].at.Before(assets[j].at) })
	remaining:=len(assets)
	for _,it:=range assets{
		if remaining<=artEntries && total<=artBudgetBytes{break}
		if os.Remove(it.path)==nil{
			total-=it.size
			remaining--
		}
	}
}
func itemArtwork(item plex.Item) string {
	switch item.Type {
	case "track":
		if item.Thumb != "" {
			return item.Thumb
		}
		return item.ParentThumb
	case "playlist":
		if item.Composite != "" {
			return item.Composite
		}
		return item.Thumb
	default:
		return item.Thumb
	}
}
