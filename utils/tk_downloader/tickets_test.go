package main

import (
	"testing"
	"time"
)

func TestDownloadTicketExpires(t *testing.T) {
	store := newDownloadTicketStore()
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	ticket, err := store.Issue(VideoInfo{ID: "work-id"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if video, exists := store.Get(ticket); !exists || video.ID != "work-id" {
		t.Fatalf("Get() = %#v, %v", video, exists)
	}

	now = now.Add(downloadTicketLifetime)
	if _, exists := store.Get(ticket); exists {
		t.Fatal("expired ticket remains available")
	}
}
