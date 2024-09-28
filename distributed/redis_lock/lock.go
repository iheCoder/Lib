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
	client     *redis.Client
	ttl        time.Duration
	key, value string
	ctx        context.Context
}

func (l *RedisLock) Lock() error {
	return l.client.SetNX(l.ctx, l.key, l.value, l.ttl).Err()
}

func (l *RedisLock) Unlock() error {
	return l.client.Eval(l.ctx, delLuaScript, []string{l.key}, l.value).Err()
}
