package redis_lock

import (
	"Lib/utils/retry"
	"context"
	"github.com/RussellLuo/timingwheel"
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
	timer        *timingwheel.Timer
	retryOptions *retry.RetryOptions
}

func newRedisLock(client *redis.Client, key string, ttl time.Duration, options *retry.RetryOptions) *RedisLock {
	return &RedisLock{
		client:       client,
		ttl:          ttl,
		key:          key,
		value:        GenerateUniqueKey(),
		ctx:          context.Background(),
		retryOptions: options,
	}
}

func (l *RedisLock) addStopTimer(timer *timingwheel.Timer) {
	l.timer = timer
}

func (l *RedisLock) SetRetryOptions(options *retry.RetryOptions) {
	l.retryOptions = options
}

func (l *RedisLock) Lock() error {
	// retry to lock
	if err := retry.Retry(l.ctx, l.lock, *l.retryOptions); err != nil {
		return err
	}

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

func (l *RedisLock) doRenew() error {
	return l.client.Expire(l.ctx, l.key, l.ttl).Err()
}

func (l *RedisLock) cleanup() {
	l.timer.Stop()
}
