package redis_lock

import (
	"testing"
	"time"
)

func TestGenerateRandomTTL(t *testing.T) {
	baseTTL := 10 * time.Second
	delta := 2 * time.Second

	n := 100_000
	for i := 0; i < n; i++ {
		ttl := GenerateRandomTTL(baseTTL, delta)
		if ttl < baseTTL || ttl > baseTTL+delta {
			t.Errorf("Expected TTL to be between 10 and 20, but got %d\n", ttl)
		}
	}
}
