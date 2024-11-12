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
	// create a new lock
	lock := newRedisLock(f.client, key, f.ttl)

	// add to renew wheel
	timer := f.addToRenewWheel(lock)
	lock.addStopTimer(timer)

	return lock
}

func (f *LockFac) renew() {
	// start timing wheel
	f.tw.Start()
	defer f.tw.Stop()

	// wait for cancel signal
	<-f.cancelSignal
}

func (f *LockFac) addToRenewWheel(l *RedisLock) *timingwheel.Timer {
	return f.tw.ScheduleFunc(&lockExpireScheduler{ttl: l.ttl}, func() {
		l.doRenew()
	})
}
