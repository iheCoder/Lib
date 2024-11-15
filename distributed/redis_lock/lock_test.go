package redis_lock

import (
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"testing"
)

var (
	redisClient *redis.Client
	ctx         = context.Background()
)

func setup() {
	redisClient = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
}

func teardown() {
	redisClient.FlushAll(ctx)
	redisClient.Close()
}

func TestRedisLock_LockUnlock(t *testing.T) {
	setup()
	defer teardown()

	lock := NewLockFac(redisClient)
	redisLock := lock.NewLock("test-key")

	// try to acquire the lock
	err := redisLock.Lock()
	assert.NoError(t, err, "failed to acquire the lock")

	// check if the lock is held
	isLocked, err := redisLock.IsHoldLock()
	assert.NoError(t, err, "failed to check if the lock is held")
	assert.True(t, isLocked, "the lock is not held")

	// try to release the lock
	err = redisLock.Unlock()
	assert.NoError(t, err, "failed to release the lock")

	// check if the lock is released
	isLocked, err = redisLock.IsHoldLock()
	assert.NoError(t, err, "failed to check if the lock is held")
	assert.False(t, isLocked, "the lock is held")
}
