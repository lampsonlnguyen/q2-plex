package main

import (
	"context"
	"errors"
	"time"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
)

// Retry two interrupted transfers, each through the existing on-card .part
// resume path. Never retry bad media, authentication or low-space errors.
func downloadWithRetry(ctx context.Context, fetch func() (int64, error), onRetry func(int)) (int64, error) {
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		size, err := fetch()
		if err == nil {
			return size, nil
		}
		if !plex.Retryable(err) || attempt == 2 {
			return 0, err
		}
		if onRetry != nil {
			onRetry(attempt + 1)
		}
		delay := time.Duration(2<<attempt) * time.Second
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
	return 0, errors.New("download retries exhausted")
}
