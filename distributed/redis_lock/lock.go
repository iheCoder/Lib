package redis_lock

import (
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
	client      *redis.Client
	ttl         time.Duration
	key, value  string
	ctx         context.Context
	stopRenewCh chan struct{}
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
	err := l.client.SetNX(l.ctx, l.key, l.value, l.ttl).Err()
	if err != nil {
		return err
	}

	go l.renew()
	return nil
}

func (l *RedisLock) Unlock() error {
	defer l.cleanup()

	return l.client.Eval(l.ctx, delLuaScript, []string{l.key}, l.value).Err()
}

func (l *RedisLock) renew() {
	ticker := time.NewTicker(l.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopRenewCh:
			return
		case <-ticker.C:
			l.client.Expire(l.ctx, l.key, l.ttl)
		}
	}
}

func (l *RedisLock) cleanup() {
	close(l.stopRenewCh)
}
