package redis_lock

import (
	"Lib/utils/retry"
	"context"
	"github.com/RussellLuo/timingwheel"
	"github.com/redis/go-redis/v9"
	"sync"
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
	fac          *LockFac

	maxRenewCount int
	renewCount    int

	enableLocalLock bool
	localLock       sync.Mutex
	once            sync.Once
}

func newRedisLock(client *redis.Client, key string, ttl time.Duration, options *retry.RetryOptions, maxRenewCount int, enableLocalLock bool, fac *LockFac) *RedisLock {
	lock := &RedisLock{
		client:        client,
		ttl:           ttl,
		key:           key,
		value:         GenerateUniqueKey(),
		ctx:           context.Background(),
		retryOptions:  options,
		maxRenewCount: maxRenewCount,
		fac:           fac,
	}

	if enableLocalLock {
		lock.localLock = sync.Mutex{}
		lock.once = sync.Once{}
	}

	return lock
}

func (l *RedisLock) SetRetryOptions(options *retry.RetryOptions) {
	l.retryOptions = options
}

func (l *RedisLock) SetMaxRenewCount(count int) {
	l.maxRenewCount = count
}

func (l *RedisLock) Lock() error {
	// lock locally to avoid unnecessary redis lock request
	if l.enableLocalLock {
		l.localLock.Lock()
	}

	// retry to lock
	if err := retry.Retry(l.ctx, l.lock, *l.retryOptions); err != nil {
		return err
	}

	// add to renew wheel
	timer := l.fac.addToRenewWheel(l)
	l.timer = timer

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
	// cleanup renew goroutine if renew count exceeds max renew
	defer func() {
		l.renewCount++
		if l.renewCount > l.maxRenewCount {
			l.cleanup()
		}
	}()

	// try to renew
	return l.client.Expire(l.ctx, l.key, l.ttl).Err()
}

func (l *RedisLock) cleanup() {
	l.once.Do(
		func() {
			// unlock local lock
			if l.enableLocalLock {
				l.localLock.Unlock()
			}

			// stop renew timer
			if l.timer != nil {
				l.timer.Stop()
			}
		},
	)
}
