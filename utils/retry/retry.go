package retry

import (
	"context"
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

func Retry(ctx context.Context, operation func() error, options RetryOptions) error {
	var err error
	backoff := options.InitialBackoff
	for i := 0; i < options.MaxRetries; i++ {
		select {
		// if context is done, return context error
		case <-ctx.Done():
			return ctx.Err()
		default:
			err = operation()
			// if no error, return nil
			if err == nil {
				return nil
			}

			// if last retry, return error
			if i == options.MaxRetries {
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
	}

	return ErrExceededMaxRetryAttempts
}

func RetryDefault(ctx context.Context, operation func() error) error {
	return Retry(ctx, operation, RetryOptions{
		MaxRetries:     3,
		InitialBackoff: time.Second,
		MaxBackoff:     time.Minute,
	})
}

func jitterBackoff(attempt int, base time.Duration) time.Duration {
	backoff := float64(base * time.Duration(1<<uint(attempt)))
	jitter := time.Duration(backoff * (0.5 + rand.Float64()))
	return jitter
}
