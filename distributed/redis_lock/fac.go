package redis_lock

import (
	"time"

	"github.com/RussellLuo/timingwheel"
	"github.com/redis/go-redis/v9"
)

type LockFac struct {
	client       *redis.Client
	ttl          time.Duration
	tw           *timingwheel.TimingWheel
	cancelSignal chan struct{}
}

func (f *LockFac) NewLock(key string) *RedisLock {
	return NewRedisLock(f.client, key, f.ttl)
}

func (f *LockFac) renew() {
	// start timing wheel
	f.tw.Start()
	defer f.tw.Stop()

	// wait for cancel signal
	<-f.cancelSignal
}

func (f *LockFac) addToRenewWheel(l *RedisLock) {
	f.tw.ScheduleFunc(&lockExpireScheduler{ttl: l.ttl}, func() {
		l.doRenew()
	})
}
