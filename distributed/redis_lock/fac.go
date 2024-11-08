package redis_lock

import (
	"time"

	"github.com/redis/go-redis/v9"
)

type LockFac struct {
	client *redis.Client
	ttl    time.Duration
}

func (l *LockFac) NewLock(key string) *RedisLock {
	return NewRedisLock(l.client, key, l.ttl)
}
