package jobs

import (
	"testing"
	"time"
)

func TestBackoffPresets(t *testing.T) {
	table := []struct {
		name    string
		fn      BackoffFunc
		attempt int
		min     time.Duration
		max     time.Duration
	}{
		{"fixed attempt 1", FixedBackoff(time.Second), 1, time.Second, time.Second},
		{"fixed attempt 5", FixedBackoff(time.Second), 5, time.Second, time.Second},
		{"linear attempt 1", LinearBackoff(time.Second), 1, time.Second, time.Second},
		{"linear attempt 3", LinearBackoff(time.Second), 3, 3 * time.Second, 3 * time.Second},
		{"exponential attempt 1", ExponentialBackoff(time.Second, 8*time.Second), 1, 0, time.Second},
		{"exponential attempt 2", ExponentialBackoff(time.Second, 8*time.Second), 2, 0, 2 * time.Second},
		{"exponential attempt 3", ExponentialBackoff(time.Second, 8*time.Second), 3, 0, 4 * time.Second},
		{"exponential capped", ExponentialBackoff(time.Second, 3*time.Second), 10, 0, 3 * time.Second},
		{"exponential zero base", ExponentialBackoff(0, time.Second), 5, 0, 0},
	}

	for _, e := range table {
		t.Run(e.name, func(t *testing.T) {
			// Jittered presets return a different duration per call, so
			// every sample must stay within the bounds.
			for i := 0; i < 250; i++ {
				got := e.fn(e.attempt)
				if got < e.min || got > e.max {
					t.Fatalf("backoff(%d) = %v, want within [%v, %v]", e.attempt, got, e.min, e.max)
				}
			}
		})
	}
}

func TestExponentialBackoffNoCap(t *testing.T) {
	fn := ExponentialBackoff(time.Second, 0)
	for attempt := 1; attempt <= 20; attempt++ {
		want := time.Second << 20 // far above 2^19s, the largest computed delay
		if got := fn(attempt); got < 0 || got > want {
			t.Fatalf("backoff(%d) = %v, want within [0, %v]", attempt, got, want)
		}
	}
}
