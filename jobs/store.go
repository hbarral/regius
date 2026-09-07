package jobs

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors. Stores return them unwrapped or wrapped with %w so
// callers can branch on errors.Is.
var (
	// ErrNoJob is returned by Claim when no job is ready, and by Complete,
	// Fail, Get, Retry and Drop when the job is not in the state those
	// methods expect.
	ErrNoJob = errors.New("jobs: no ready job")

	// ErrUnknownJob is returned by Enqueue, once workers are running, for a
	// job name with no registered handler.
	ErrUnknownJob = errors.New("jobs: unknown job name")

	// ErrDuplicateHandler is returned by Register for an already-registered
	// job name.
	ErrDuplicateHandler = errors.New("jobs: duplicate handler registration")
)

// Stats is a snapshot of job counts by status.
type Stats struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Dead      int `json:"dead"`
}

// ListFilter narrows a List call. The zero value lists everything.
type ListFilter struct {
	// Status filters by lifecycle state; empty means all statuses.
	Status Status

	// Name filters by job name; empty means all names.
	Name string

	// Limit caps the number of jobs returned. Limit <= 0 means no limit at
	// the store level; Manager.List applies a default of 50 and a hard cap
	// of 200.
	Limit int
}

// Store is the persistence contract for the job queue. Every state
// transition must be atomic per method: Claim must behave as if the peek,
// the removal from pending, and the lease write happen indivisibly, so two
// concurrent workers can never claim the same job.
//
// MemoryStore ships with the package; Redis and SQL stores follow in later
// phases. Anything implementing this interface can back a Manager.
type Store interface {
	// Enqueue inserts a new pending job. Implementations must not alias j:
	// they store their own copy so later transitions never write through
	// the caller's pointer.
	Enqueue(ctx context.Context, j *Job) error

	// Claim atomically moves the oldest pending job whose RunAt is not after
	// now to running: it increments Attempts, sets LeaseUntil to leaseUntil,
	// and returns the claimed job. It returns ErrNoJob when nothing is
	// ready.
	Claim(ctx context.Context, now, leaseUntil time.Time) (*Job, error)

	// Complete marks the running job with the given ID completed.
	Complete(ctx context.Context, id string, now time.Time) error

	// Fail records a failed attempt on the running job j. With a non-zero
	// retryAt the job returns to pending with RunAt = retryAt; with a zero
	// retryAt the job is dead-lettered. lastErr is recorded as given (the
	// Manager truncates it).
	Fail(ctx context.Context, j *Job, now, retryAt time.Time, lastErr string) error

	// ReclaimExpired returns running jobs whose lease has expired to
	// pending, so a crashed worker's jobs are not lost. It returns how many
	// were reclaimed.
	ReclaimExpired(ctx context.Context, now time.Time) (int, error)

	// TryLock is a best-effort exclusive lock with a TTL, used to keep
	// schedulers on separate processes from double-firing. Single-process
	// stores implement it as a no-op returning true.
	TryLock(ctx context.Context, name string, ttl time.Duration) (bool, error)

	// Stats returns job counts by status.
	Stats(ctx context.Context) (Stats, error)

	// List returns jobs matching the filter, at most f.Limit when positive.
	List(ctx context.Context, f ListFilter) ([]*Job, error)

	// Get returns the job with the given ID regardless of status.
	Get(ctx context.Context, id string) (*Job, error)

	// Retry moves a dead job back to pending, resetting its attempts.
	Retry(ctx context.Context, id string) error

	// Drop permanently removes a dead job.
	Drop(ctx context.Context, id string) error

	// Prune deletes completed jobs older than retention, returning how many
	// were removed. Dead jobs are never pruned automatically; they are
	// removed only via Drop.
	Prune(ctx context.Context, now time.Time, retention time.Duration) (int, error)
}
