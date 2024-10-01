package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry(t *testing.T) {
	var counter int
	err := Retry(context.Background(), func() error {
		counter++
		if counter < 3 {
			return errors.New("try again")
		}
		return nil
	}, RetryOptions{
		MaxRetries:     3,
		InitialBackoff: time.Second,
		MaxBackoff:     time.Minute,
	})
	if err != nil {
		t.Errorf("Expected err to be nil, but got %v", err)
	}
	if counter != 3 {
		t.Errorf("Expected counter to be 3, but got %d", counter)
	}

}

// Test context cancellation
func TestRetryContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := RetryDefault(ctx, func() error {
		return errors.New("try again")
	})
	if err != context.Canceled {
		t.Errorf("Expected err to be context.Canceled, but got %v", err)
	}
}
