package jobs

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-process Store for development and tests. Jobs live
// only as long as the process: there is no persistence and no cross-process
// visibility. Pending jobs are kept ordered by RunAt.
type MemoryStore struct {
	// Now is the clock used by Retry. nil means time.Now. It exists for
	// deterministic tests.
	Now func() time.Time

	mu        sync.Mutex
	pending   []*Job // sorted by RunAt ascending
	running   map[string]*Job
	completed []*Job // append order, pruned by Prune
	dead      map[string]*Job
}

// NewMemoryStore creates an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		running: make(map[string]*Job),
		dead:    make(map[string]*Job),
	}
}

func (s *MemoryStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// insertPending keeps pending sorted by RunAt; the caller holds the lock.
func (s *MemoryStore) insertPending(j *Job) {
	i := sort.Search(len(s.pending), func(i int) bool {
		return s.pending[i].RunAt.After(j.RunAt)
	})
	s.pending = append(s.pending, nil)
	copy(s.pending[i+1:], s.pending[i:])
	s.pending[i] = j
}

func (s *MemoryStore) Enqueue(_ context.Context, j *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Store a copy: later transitions (Claim, Fail, Complete) must never
	// write through the caller's pointer.
	s.insertPending(j.copy())
	return nil
}

func (s *MemoryStore) Claim(_ context.Context, now, leaseUntil time.Time) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 || s.pending[0].RunAt.After(now) {
		return nil, ErrNoJob
	}
	j := s.pending[0]
	s.pending = s.pending[1:]
	j.Status = StatusRunning
	j.Attempts++
	j.LeaseUntil = leaseUntil
	j.UpdatedAt = now
	s.running[j.ID] = j
	return j.copy(), nil
}

func (s *MemoryStore) Complete(_ context.Context, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.running[id]
	if !ok {
		return ErrNoJob
	}
	delete(s.running, id)
	j.Status = StatusCompleted
	j.LeaseUntil = time.Time{}
	j.CompletedAt = now
	j.UpdatedAt = now
	s.completed = append(s.completed, j)
	return nil
}

func (s *MemoryStore) Fail(_ context.Context, j *Job, now, retryAt time.Time, lastErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.running[j.ID]
	if !ok {
		return ErrNoJob
	}
	delete(s.running, j.ID)
	cur.LastError = lastErr
	cur.UpdatedAt = now
	cur.LeaseUntil = time.Time{}
	if retryAt.IsZero() {
		cur.Status = StatusDead
		s.dead[cur.ID] = cur
		return nil
	}
	cur.Status = StatusPending
	cur.RunAt = retryAt
	s.insertPending(cur)
	return nil
}

func (s *MemoryStore) ReclaimExpired(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, j := range s.running {
		if j.LeaseUntil.Before(now) {
			delete(s.running, id)
			j.Status = StatusPending
			j.LeaseUntil = time.Time{}
			j.RunAt = now
			s.insertPending(j)
			n++
		}
	}
	return n, nil
}

func (s *MemoryStore) TryLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	// Single process: the in-process scheduler cannot double-fire, so no
	// lock is needed.
	return true, nil
}

func (s *MemoryStore) Stats(_ context.Context) (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{
		Pending:   len(s.pending),
		Running:   len(s.running),
		Completed: len(s.completed),
		Dead:      len(s.dead),
	}, nil
}

func (s *MemoryStore) List(_ context.Context, f ListFilter) ([]*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	match := func(j *Job) bool {
		return (f.Status == "" || j.Status == f.Status) && (f.Name == "" || j.Name == f.Name)
	}

	var out []*Job
	for _, j := range s.pending {
		if match(j) {
			out = append(out, j.copy())
		}
	}
	running := make([]*Job, 0, len(s.running))
	for _, j := range s.running {
		if match(j) {
			running = append(running, j)
		}
	}
	sort.Slice(running, func(i, k int) bool { return running[i].RunAt.Before(running[k].RunAt) })
	for _, j := range running {
		out = append(out, j.copy())
	}
	for i := len(s.completed) - 1; i >= 0; i-- {
		if match(s.completed[i]) {
			out = append(out, s.completed[i].copy())
		}
	}
	dead := make([]*Job, 0, len(s.dead))
	for _, j := range s.dead {
		if match(j) {
			dead = append(dead, j)
		}
	}
	sort.Slice(dead, func(i, k int) bool { return dead[i].UpdatedAt.After(dead[k].UpdatedAt) })
	for _, j := range dead {
		out = append(out, j.copy())
	}

	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.pending {
		if j.ID == id {
			return j.copy(), nil
		}
	}
	if j, ok := s.running[id]; ok {
		return j.copy(), nil
	}
	for _, j := range s.completed {
		if j.ID == id {
			return j.copy(), nil
		}
	}
	if j, ok := s.dead[id]; ok {
		return j.copy(), nil
	}
	return nil, ErrNoJob
}

func (s *MemoryStore) Retry(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.dead[id]
	if !ok {
		return ErrNoJob
	}
	delete(s.dead, id)
	now := s.now()
	j.Status = StatusPending
	j.Attempts = 0
	j.RunAt = now
	j.UpdatedAt = now
	s.insertPending(j)
	return nil
}

func (s *MemoryStore) Drop(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.dead[id]; !ok {
		return ErrNoJob
	}
	delete(s.dead, id)
	return nil
}

func (s *MemoryStore) Prune(_ context.Context, now time.Time, retention time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := now.Add(-retention)
	kept := s.completed[:0]
	n := 0
	for _, j := range s.completed {
		if j.CompletedAt.Before(cutoff) {
			n++
			continue
		}
		kept = append(kept, j)
	}
	s.completed = kept
	return n, nil
}
