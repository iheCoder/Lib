package redis_lock

import (
	"Lib/utils/retry"
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

const (
	delLuaScript = `
	if redis.call("get", KEYS[1]) == ARGV[1] then
		return redis.call("del", KEYS[1])
	else
		return 0
	end
	`
)

type RedisLock struct {
	client       *redis.Client
	ttl          time.Duration
	key, value   string
	ctx          context.Context
	stopRenewCh  chan struct{}
	retryOptions retry.RetryOptions
}

func NewRedisLock(client *redis.Client, key string, ttl time.Duration) *RedisLock {
	return &RedisLock{
		client:      client,
		ttl:         ttl,
		key:         key,
		value:       GenerateUniqueKey(),
		ctx:         context.Background(),
		stopRenewCh: make(chan struct{}),
	}
}

func (l *RedisLock) Lock() error {
	// retry to lock
	if err := retry.Retry(l.ctx, l.lock, l.retryOptions); err != nil {
		return err
	}

	// renew lock background
	go l.renew()
	return nil
}

func (l *RedisLock) lock() error {
	return l.client.SetNX(l.ctx, l.key, l.value, l.ttl).Err()
}

func (l *RedisLock) Unlock() error {
	// cleanup renew goroutine
	defer l.cleanup()

	// try to unlock
	return l.client.Eval(l.ctx, delLuaScript, []string{l.key}, l.value).Err()
}

func (l *RedisLock) renew() {
	ticker := time.NewTicker(l.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		// stop renew when unlock
		case <-l.stopRenewCh:
			return

		// renew lock
		case <-ticker.C:
			l.doRenew()
		}
	}
}

func (l *RedisLock) doRenew() error {
	return l.client.Expire(l.ctx, l.key, l.ttl).Err()
}

func (l *RedisLock) cleanup() {
	close(l.stopRenewCh)
}
