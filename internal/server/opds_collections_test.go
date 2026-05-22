package server

import (
	"strings"
	"testing"
	"time"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

func TestBuildOPDSCollectionsFeed(t *testing.T) {
	cols := []store.Collection{
		{ID: "c-1", Name: "Nancy Drew"},
		{ID: "c-2", Name: "Sci-Fi"},
	}
	feed := buildOPDSCollectionsFeed(cols, time.Unix(1700000000, 0))
	if len(feed.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(feed.Entries))
	}
	if !strings.Contains(feed.Entries[0].Links[0].Href, "c-1") {
		t.Errorf("entry 0 href = %q, want it to reference c-1", feed.Entries[0].Links[0].Href)
	}
}
