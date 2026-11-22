package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lampsonlnguyen/q2-plex/internal/offline"
)

func TestFirmwareFieldPreservesUnicodeBoundary(t *testing.T) {
	cases := []string{
		strings.Repeat("a", 249) + "ư",
		strings.Repeat("a", 248) + "界",
		strings.Repeat("a", 247) + "😀",
		strings.Repeat("ế", 150),
		"Tiếng Việt\n" + strings.Repeat("語", 120),
	}
	for _, input := range cases {
		got := field(input)
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 from field(%q)", input)
		}
		if len(got) > 250 {
			t.Fatalf("field grew to %d bytes", len(got))
		}
		if !strings.HasPrefix(input, got) && !strings.Contains(input, "\n") {
			t.Fatalf("field lost prefix: %q", got)
		}
	}
}

func TestOfflineBrowseRejectsBadCursor(t *testing.T) {
	store, err := offline.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &app{offline: store}
	if page := a.offlinePage(nil); len(page.Rows) == 0 {
		t.Fatal("empty offline library should show guidance")
	}
	if page := a.offlinePage([]string{"not-a-page"}); page.Status != "bad" {
		t.Fatalf("malformed cursor status: %q", page.Status)
	}
}
