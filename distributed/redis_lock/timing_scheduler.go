package redis_lock

import "time"

type lockExpireScheduler struct {
	ttl time.Duration
}

func (s *lockExpireScheduler) Next(prev time.Time) time.Time {
	return prev.Add(s.ttl)
}
