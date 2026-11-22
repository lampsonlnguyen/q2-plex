package main

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/lampsonlnguyen/q2-plex/internal/plex"
)

func TestDownloadRetryRecoversInterruptedTransfer(t *testing.T) {
	attempts := 0
	retries := 0
	size, err := downloadWithRetry(context.Background(), func() (int64, error) {
		attempts++
		if attempts == 1 {
			return 0, io.ErrUnexpectedEOF
		}
		return 1234, nil
	}, func(n int) {
		retries++
		if n != 1 {
			t.Errorf("attempt %d", n)
		}
	})
	if err != nil || size != 1234 || attempts != 2 || retries != 1 {
		t.Fatalf("size=%d err=%v attempts=%d retries=%d", size, err, attempts, retries)
	}
}
func TestDownloadRetryRejectsPermanentError(t *testing.T) {
	attempts := 0
	_, err := downloadWithRetry(context.Background(), func() (int64, error) {
		attempts++
		return 0, &plex.HTTPStatusError{Code: 401}
	}, nil)
	if err == nil || attempts != 1 {
		t.Fatalf("unexpected attempts=%d error=%v", attempts, err)
	}
	_, err = downloadWithRetry(context.Background(), func() (int64, error) {
		return 0, errors.New("bad file")
	}, nil)
	if err == nil {
		t.Fatal("permanent file error ignored")
	}
}
