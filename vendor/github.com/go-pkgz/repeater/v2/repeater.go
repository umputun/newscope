// Package repeater implements retry functionality with different strategies.
// It provides fixed delays and various backoff strategies (constant, linear, exponential) with jitter support.
// The package allows custom retry strategies and error-specific handling. Context-aware implementation
// supports cancellation and timeouts.
package repeater

import (
	"context"
	"errors"
	"time"
)

// ErrAny is a special sentinel error that, when passed as a critical error to Do,
// makes it fail on any error from the function
var ErrAny = errors.New("any error")

// Repeater holds configuration for retry operations
type Repeater struct {
	strategy Strategy
	attempts int
}

// NewWithStrategy creates a repeater with a custom retry strategy
func NewWithStrategy(attempts int, strategy Strategy) *Repeater {
	if attempts <= 0 {
		attempts = 1
	}
	if strategy == nil {
		strategy = NewFixedDelay(time.Second)
	}
	return &Repeater{
		attempts: attempts,
		strategy: strategy,
	}
}

// NewBackoff creates a repeater with backoff strategy
// Default settings (can be overridden with options):
//   - 30s max delay
//   - exponential backoff
//   - 10% jitter
func NewBackoff(attempts int, initial time.Duration, opts ...backoffOption) *Repeater {
	return NewWithStrategy(attempts, newBackoff(initial, opts...))
}

// NewFixed creates a repeater with fixed delay strategy
func NewFixed(attempts int, delay time.Duration) *Repeater {
	return NewWithStrategy(attempts, NewFixedDelay(delay))
}

// Do repeats fun until it succeeds or max attempts reached
// terminates immediately on context cancellation or if err matches any in termErrs.
// if errs contains ErrAny, terminates on any error.
func (r *Repeater) Do(ctx context.Context, fun func() error, termErrs ...error) error {
	var lastErr error

	inErrors := func(err error) bool {
		for _, e := range termErrs {
			if errors.Is(e, ErrAny) {
				return true
			}
			if errors.Is(err, e) {
				return true
			}
		}
		return false
	}

	for attempt := 0; attempt < r.attempts; attempt++ {
		// check context before each attempt
		if err := ctx.Err(); err != nil {
			return err
		}

		var err error
		if err = fun(); err == nil {
			return nil
		}

		lastErr = err
		if inErrors(err) {
			return err
		}

		// don't sleep after the last attempt
		if attempt < r.attempts-1 {
			delay := r.strategy.NextDelay(attempt + 1)
			if delay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
		}
	}

	return lastErr
}
