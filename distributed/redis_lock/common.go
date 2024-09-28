package redis_lock

import (
	"math/rand/v2"
	"time"
)

func GenerateRandomTTL(baseTTL, delta time.Duration) time.Duration {
	return baseTTL + time.Duration(rand.Int64N(int64(delta)))
}
