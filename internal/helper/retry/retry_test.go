package retry

import (
	"context"
	"errors"
	"testing"

	"github.com/shoenig/test/must"
)

func TestRetry_success(t *testing.T) {
	calls := 0
	err := Retry(func() error {
		calls++
		return nil
	})
	must.NoError(t, err)
	must.Eq(t, 1, calls)
}

func TestRetry_successAfterTransientFailures(t *testing.T) {
	sentinel := errors.New("transient")
	calls := 0

	err := Retry(func() error {
		calls++
		if calls < 2 {
			return sentinel
		}
		return nil
	})
	must.NoError(t, err)
	must.Eq(t, 2, calls)
}

func TestRetry_deadlineExceededOnFirstCall(t *testing.T) {
	err := Retry(func() error { return context.DeadlineExceeded })
	must.Error(t, err)
	must.True(t, errors.Is(err, context.DeadlineExceeded))
}

func TestRetry_deadlineExceededAfterPriorFailure(t *testing.T) {

	rootCause := errors.New("root cause")
	calls := 0

	err := Retry(func() error {
		calls++
		if calls == 1 {
			return rootCause
		}
		return context.DeadlineExceeded
	})
	must.Error(t, err)
	must.True(t, errors.Is(err, rootCause))
}
