package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const (
	downloadTicketBytes    = 24
	downloadTicketLifetime = 10 * time.Minute
)

type downloadTicket struct {
	grant     mediaGrant
	expiresAt time.Time
}

// mediaGrant narrows one ticket to one resolved asset. This lets the browser
// preview and download images independently without accepting arbitrary URLs.
type mediaGrant struct {
	Work       WorkInfo
	AssetIndex int
}

// Asset validates the stored index again at use time, keeping corrupt or stale
// in-memory state from becoming an out-of-bounds media request.
func (grant mediaGrant) Asset() (MediaAsset, bool) {
	if grant.AssetIndex < 0 || grant.AssetIndex >= len(grant.Work.Assets) {
		return MediaAsset{}, false
	}
	return grant.Work.Assets[grant.AssetIndex], true
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
func (store *downloadTicketStore) Issue(work WorkInfo, assetIndex int) (string, error) {
	if _, exists := (mediaGrant{Work: work, AssetIndex: assetIndex}).Asset(); !exists {
		return "", errors.New("下载任务引用了无效媒体")
	}
	random := make([]byte, downloadTicketBytes)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	ticketID := hex.EncodeToString(random)

	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked()
	store.entries[ticketID] = downloadTicket{
		grant:     mediaGrant{Work: work, AssetIndex: assetIndex},
		expiresAt: store.now().Add(downloadTicketLifetime),
	}
	return ticketID, nil
}

// Get returns only live entries. Tickets remain reusable until expiry so a
// browser retry after a transient connection failure does not force users to
// resolve the share page again.
func (store *downloadTicketStore) Get(ticketID string) (mediaGrant, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeExpiredLocked()
	ticket, exists := store.entries[ticketID]
	if !exists {
		return mediaGrant{}, false
	}
	return ticket.grant, true
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
