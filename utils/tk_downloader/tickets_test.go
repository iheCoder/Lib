package main

import (
	"testing"
	"time"
)

func TestDownloadTicketExpires(t *testing.T) {
	store := newDownloadTicketStore()
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	work := WorkInfo{ID: "work-id", Assets: []MediaAsset{{URL: "https://example.invalid/media", Kind: MediaKindVideo}}}
	ticket, err := store.Issue(work, 0)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if grant, exists := store.Get(ticket); !exists || grant.Work.ID != "work-id" || grant.AssetIndex != 0 {
		t.Fatalf("Get() = %#v, %v", grant, exists)
	}

	now = now.Add(downloadTicketLifetime)
	if _, exists := store.Get(ticket); exists {
		t.Fatal("expired ticket remains available")
	}
}
