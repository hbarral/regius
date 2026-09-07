package jobs

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// Handler processes one attempt of a job. Returning an error triggers the
// retry policy. A panic is recovered and treated as a failed attempt — it
// never kills the worker. Handlers should respect ctx cancellation so
// shutdown can interrupt long-running attempts.
type Handler func(ctx context.Context, j *Job) error

// Options configures one registered job name.
type Options struct {
	// MaxAttempts is how many total attempts a job gets before it is
	// dead-lettered. 0 falls back to ManagerOptions.MaxAttempts.
	MaxAttempts int

	// Timeout bounds each attempt; exceeding it counts as a failed attempt
	// with the deadline error recorded. 0 means no per-attempt deadline.
	Timeout time.Duration

	// Backoff computes how long to wait before the next attempt, given the
	// number of attempts that have already failed. nil falls back to
	// ExponentialBackoff(30s, 15m).
	Backoff BackoffFunc
}

// EnqueueOptions overrides per-job settings in Manager.EnqueueWithOptions.
type EnqueueOptions struct {
	// RunAt schedules the first attempt at that moment. Zero or a past time
	// means "as soon as possible".
	RunAt time.Time

	// MaxAttempts overrides the handler's Options.MaxAttempts when positive.
	MaxAttempts int
}

// BackoffFunc returns the delay before attempt n+1, where n is the number
// of attempts that have already failed (n >= 1).
type BackoffFunc func(attempt int) time.Duration

// FixedBackoff waits the same duration after every failed attempt.
func FixedBackoff(d time.Duration) BackoffFunc {
	return func(int) time.Duration { return d }
}

// LinearBackoff waits step*n after n failed attempts.
func LinearBackoff(step time.Duration) BackoffFunc {
	return func(attempt int) time.Duration { return step * time.Duration(attempt) }
}

// ExponentialBackoff waits base<<(n-1) after n failed attempts, capped at
// max, with full jitter: the actual delay is uniformly random in [0, delay).
// Jitter spreads retry storms. A max <= 0 means no cap.
func ExponentialBackoff(base, max time.Duration) BackoffFunc {
	return func(attempt int) time.Duration {
		d := base
		for i := 1; i < attempt; i++ {
			if max > 0 && d >= max {
				break
			}
			if d > math.MaxInt64/2 {
				d = math.MaxInt64
				break
			}
			d *= 2
		}
		if max > 0 && d > max {
			d = max
		}
		if d <= 0 {
			return 0
		}
		return time.Duration(rand.Int64N(int64(d)))
	}
}
