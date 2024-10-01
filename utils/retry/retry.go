package retry

import (
	"errors"
	"math/rand/v2"
	"time"
)

type RetryOptions struct {
	// MaxRetries the maximum number of retries
	MaxRetries int
	// InitialBackoff the initial backoff interval
	InitialBackoff time.Duration
	// MaxBackoff the maximum backoff interval
	MaxBackoff time.Duration
}

var (
	ErrExceededMaxRetryAttempts = errors.New("exceeded max retry attempts")
)

func Retry(operation func() error, options RetryOptions) error {
	var err error
	backoff := options.InitialBackoff
	for i := 0; i < options.MaxRetries; i++ {
		err = operation()
		// if no error, return nil
		if err == nil {
			return nil
		}

		// if last retry, return error
		if i == options.MaxRetries-1 {
			return err
		}

		// exponential backoff
		backoff = jitterBackoff(i, time.Duration(backoff))
		if backoff > options.MaxBackoff {
			backoff = options.MaxBackoff
		}

		// sleep
		time.Sleep(backoff)
	}

	return ErrExceededMaxRetryAttempts
}

func jitterBackoff(attempt int, base time.Duration) time.Duration {
	backoff := base * time.Duration(1<<uint(attempt))
	jitter := time.Duration(rand.Int64N(int64(backoff)))

	return backoff + jitter
}
