package redis_lock

import (
	"Lib/utils/retry"
	"time"

	"github.com/RussellLuo/timingwheel"
	"github.com/redis/go-redis/v9"
)

var (
	defaultRetryOption = retry.RetryOptions{
		MaxRetries:     5,
		InitialBackoff: time.Millisecond * 100,
		MaxBackoff:     time.Minute,
	}
	defaultTTL = time.Second * 10
	defaultTW  = timingwheel.NewTimingWheel(time.Millisecond, 60*1000)
)

type FacOption func(f *LockFac)

type LockFac struct {
	client       *redis.Client
	ttl          time.Duration
	tw           *timingwheel.TimingWheel
	cancelSignal chan struct{}
	retryOptions *retry.RetryOptions
}

func WithTTL(ttl time.Duration) FacOption {
	return func(f *LockFac) {
		f.ttl = ttl
	}
}

func WithRetryOptions(options *retry.RetryOptions) FacOption {
	return func(f *LockFac) {
		f.retryOptions = options
	}
}

func WithTimingWheel(tw *timingwheel.TimingWheel) FacOption {
	return func(f *LockFac) {
		f.tw = tw
	}
}

func NewLockFac(client *redis.Client, options ...FacOption) *LockFac {
	fac := &LockFac{
		client:       client,
		cancelSignal: make(chan struct{}),
		tw:           defaultTW,
		retryOptions: &defaultRetryOption,
		ttl:          defaultTTL,
	}

	// apply options
	for _, option := range options {
		option(fac)
	}

	return fac
}

func (f *LockFac) NewLock(key string) *RedisLock {
	// create a new lock
	lock := newRedisLock(f.client, key, f.ttl, f.retryOptions)

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
	return f.tw.ScheduleFunc(&lockExpireScheduler{ttl: genRenewScanInterval(l.ttl)}, func() {
		l.doRenew()
	})
}

// genRenewScanInterval generates the interval for scanning the renew wheel.
// It should be half of the lock's TTL.
func genRenewScanInterval(ttl time.Duration) time.Duration {
	return ttl / 2
}
