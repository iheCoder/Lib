package redis_lock

import (
	"sync"
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

func TestGenerateUniqueKey(t *testing.T) {
	n := 1000_000
	m := make(map[string]struct{})
	lock := sync.Mutex{}
	wg := sync.WaitGroup{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock.Lock()
			m[GenerateUniqueKey()] = struct{}{}
			lock.Unlock()
		}()
	}
	wg.Wait()

	if len(m) != n {
		t.Errorf("Expected %d keys, but got %d\n", n, len(m))
	}
}
