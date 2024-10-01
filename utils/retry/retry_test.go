package retry

import (
	"errors"
	"testing"
	"time"
)

func TestRetry(t *testing.T) {
	var counter int
	err := Retry(func() error {
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
