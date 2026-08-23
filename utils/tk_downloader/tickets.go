package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	downloadTicketBytes    = 24
	downloadTicketLifetime = 10 * time.Minute
)

type downloadTicket struct {
	video     VideoInfo
	expiresAt time.Time
}

type downloadTicketStore struct {
	mu      sync.Mutex
	entries map[string]downloadTicket
	now     func() time.Time
}

// newDownloadTicketStore owns short-lived server-side download state. Keeping
// the media URL off the browser avoids exposing upstream gateway details and
// lets the download endpoint reapply the same outbound trust policy as the CLI.
func newDownloadTicketStore() *downloadTicketStore {
	return &downloadTicketStore{
		entries: make(map[string]downloadTicket),
		now:     time.Now,
	}
}

// Issue creates a cryptographically unguessable ticket and opportunistically
// removes expired entries. No background goroutine is needed for this small
// local tool, so server shutdown remains simple and deterministic.
func (store *downloadTicketStore) Issue(video VideoInfo) (string, error) {
	random := make([]byte, downloadTicketBytes)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	ticketID := hex.EncodeToString(random)

	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked()
	store.entries[ticketID] = downloadTicket{
		video:     video,
		expiresAt: store.now().Add(downloadTicketLifetime),
	}
	return ticketID, nil
}

// Get returns only live entries. Tickets remain reusable until expiry so a
// browser retry after a transient connection failure does not force users to
// resolve the share page again.
func (store *downloadTicketStore) Get(ticketID string) (VideoInfo, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked()
	ticket, exists := store.entries[ticketID]
	if !exists {
		return VideoInfo{}, false
	}
	return ticket.video, true
}

// purgeExpiredLocked assumes the caller holds store.mu and keeps expiration
// logic in one place so Issue and Get cannot disagree about ticket validity.
func (store *downloadTicketStore) purgeExpiredLocked() {
	now := store.now()
	for ticketID, ticket := range store.entries {
		if !ticket.expiresAt.After(now) {
			delete(store.entries, ticketID)
		}
	}
}
