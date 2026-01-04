package retry

import (
	"context"
	"errors"
	"time"

	"github.com/sethvargo/go-retry"
)

// Retry attempts to execute the provided function with exponential backoff
// for up to 60 seconds. This is a useful generic retry mechanism for operations
// that may fail transiently.
//
// The returned error is the last error encountered if all retries fail and
// should be considered the root cause of the failure.
func Retry(fn func() error) error {

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var last error

	if err := retry.Exponential(ctx, 1*time.Second, func(ctx context.Context) error {
		if latestErr := fn(); latestErr != nil {
			if errors.Is(latestErr, context.DeadlineExceeded) {
				return last
			}
			last = latestErr
			return retry.RetryableError(latestErr)
		}
		return nil
	}); err != nil {
		return last
	}
	return nil
}
