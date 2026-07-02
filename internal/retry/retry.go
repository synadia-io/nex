package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

// Policy bundles retry tuning so call sites reference a named intent instead
// of repeating positional constants.
type Policy struct {
	Attempts int
	Base     time.Duration
	MaxDelay time.Duration
}

var (
	// Long suits startup paths that should ride out a real outage.
	Long = Policy{Attempts: 5, Base: 250 * time.Millisecond, MaxDelay: 5 * time.Second}
	// Short suits request handlers that must answer within the caller's
	// request timeout.
	Short = Policy{Attempts: 3, Base: 100 * time.Millisecond, MaxDelay: time.Second}
)

// Permanent marks err as non-retryable: Do stops immediately and returns the
// original err unwrapped. Use it for failures that resending cannot heal,
// e.g. a request that is not idempotent once the server has processed it.
func Permanent(err error) error {
	return &permanentError{err}
}

type permanentError struct{ err error }

func (p *permanentError) Error() string { return p.err.Error() }
func (p *permanentError) Unwrap() error { return p.err }

// Jitter returns a duration drawn uniformly from [d/2, d] so
// simultaneously-started retriers (e.g. many agents registering at once) do
// not stampede in lockstep.
func Jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return d/2 + rand.N(d/2+1)
}

// Do calls fn up to p.Attempts times, sleeping between failed attempts with
// jittered exponential backoff starting at p.Base and capped at p.MaxDelay.
// It aborts early when ctx is done (returning the last error from fn joined
// with the context error) or when fn returns an error marked Permanent.
func Do[T any](ctx context.Context, p Policy, fn func() (T, error)) (T, error) {
	var zero T
	attempts := max(p.Attempts, 1)
	delay := max(p.Base, time.Millisecond)
	maxDelay := max(p.MaxDelay, delay)

	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return zero, errors.Join(lastErr, err)
		}

		ret, err := fn()
		if err == nil {
			return ret, nil
		}
		var pe *permanentError
		if errors.As(err, &pe) {
			return zero, pe.err
		}
		lastErr = err

		if i == attempts-1 {
			break
		}

		select {
		case <-ctx.Done():
			return zero, errors.Join(lastErr, ctx.Err())
		case <-time.After(Jitter(delay)):
		}

		if delay > maxDelay/2 { // overflow-safe doubling
			delay = maxDelay
		} else {
			delay *= 2
		}
	}

	return zero, fmt.Errorf("failed after %d attempts: %w", attempts, lastErr)
}
