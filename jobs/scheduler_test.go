package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestManager_EverySchedulesJob(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	var runs atomic.Int32
	m.MustRegister("tick", func(context.Context, *Job) error {
		runs.Add(1)
		return nil
	}, Options{})
	if err := m.Every(time.Second, "tick", nil); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop(context.Background())

	waitFor(t, 5*time.Second, func() bool { return runs.Load() >= 1 })
}

func TestManager_CronSchedulesJob(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	var runs atomic.Int32
	m.MustRegister("tick", func(context.Context, *Job) error {
		runs.Add(1)
		return nil
	}, Options{})
	if err := m.Cron("@every 1s", "tick", nil); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop(context.Background())

	waitFor(t, 5*time.Second, func() bool { return runs.Load() >= 1 })

	// invalid specs are rejected at registration
	if err := m.Cron("not a spec", "tick", nil); err == nil {
		t.Fatal("Cron with invalid spec: want error")
	}
	if err := m.Every(0, "tick", nil); err == nil {
		t.Fatal("Every with zero interval: want error")
	}
}

func TestManager_AtSchedulesOneShot(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	var runs atomic.Int32
	m.MustRegister("once", func(context.Context, *Job) error {
		runs.Add(1)
		return nil
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop(context.Background())

	j, err := m.At(ctx, time.Now().Add(200*time.Millisecond), "once", nil)
	if err != nil {
		t.Fatal(err)
	}
	if j.RunAt.Before(time.Now().Add(100 * time.Millisecond)) {
		t.Fatalf("RunAt = %v, want ~200ms in the future", j.RunAt)
	}

	// not before the deadline
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if runs.Load() != 0 {
			t.Fatal("job ran before its RunAt")
		}
		time.Sleep(5 * time.Millisecond)
	}
	waitFor(t, 3*time.Second, func() bool { return runs.Load() == 1 })
	time.Sleep(300 * time.Millisecond)
	if runs.Load() != 1 {
		t.Fatalf("one-shot job ran %d times, want 1", runs.Load())
	}
}

// lockedStore refuses every TryLock, simulating another process holding
// the schedule fire-lock.
type lockedStore struct {
	*MemoryStore
	locks atomic.Int32
}

func (s *lockedStore) TryLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	s.locks.Add(1)
	return false, nil
}

func TestManager_ScheduleLockPreventsFiring(t *testing.T) {
	store := &lockedStore{MemoryStore: NewMemoryStore()}
	m := New(store, ManagerOptions{Workers: 1, PollInterval: time.Millisecond})
	m.MustRegister("tick", func(context.Context, *Job) error { return nil }, Options{})
	if err := m.Every(time.Second, "tick", nil); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop(context.Background())

	// fires are attempted but the lock is refused, so nothing is enqueued
	waitFor(t, 3*time.Second, func() bool { return store.locks.Load() >= 1 })
	time.Sleep(1200 * time.Millisecond)
	st, err := m.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Pending+st.Running+st.Completed+st.Dead != 0 {
		t.Fatalf("stats = %+v, want all zero while the lock is held", st)
	}
}

func TestManager_ScheduleLockDisabledFiresAnyway(t *testing.T) {
	store := &lockedStore{MemoryStore: NewMemoryStore()}
	m := New(store, ManagerOptions{
		Workers:       1,
		PollInterval:  time.Millisecond,
		SchedulerLock: BoolPtr(false),
	})
	var runs atomic.Int32
	m.MustRegister("tick", func(context.Context, *Job) error {
		runs.Add(1)
		return nil
	}, Options{})
	if err := m.Every(time.Second, "tick", nil); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	defer m.Stop(context.Background())

	waitFor(t, 5*time.Second, func() bool { return runs.Load() >= 1 })
	if store.locks.Load() != 0 {
		t.Fatalf("TryLock called %d times despite the disabled scheduler lock", store.locks.Load())
	}
}

func TestManager_MaintenanceReclaimsAndPrunes(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// a completed job old enough to be pruned
	old := newTestJob("old", "tick", time.Now().Add(-2*time.Hour))
	if err := store.Enqueue(ctx, old); err != nil {
		t.Fatal(err)
	}
	c, err := store.Claim(ctx, old.RunAt, old.RunAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, c.ID, old.RunAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	// a running job whose lease expires almost immediately (crashed worker)
	stuck := newTestJob("stuck", "tick", time.Now().Add(-time.Second))
	if err := store.Enqueue(ctx, stuck); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, time.Now(), time.Now().Add(20*time.Millisecond)); err != nil {
		t.Fatal(err)
	}

	m := New(store, ManagerOptions{
		Workers:      1,
		PollInterval: time.Millisecond,
		Lease:        40 * time.Millisecond, // maintenance ticks every 10ms
		Retention:    time.Hour,
	})
	var calls atomic.Int32
	m.MustRegister("tick", func(context.Context, *Job) error {
		calls.Add(1)
		return nil
	}, Options{})

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(runCtx)
	defer m.Stop(context.Background())

	// the stuck job is reclaimed once its lease expires, then completed
	waitFor(t, 2*time.Second, func() bool { return calls.Load() == 1 })
	// the old completed job is pruned past the retention window
	waitFor(t, 2*time.Second, func() bool {
		_, err := store.Get(ctx, "old")
		return errors.Is(err, ErrNoJob)
	})
	st, err := m.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Completed != 1 || st.Pending != 0 || st.Running != 0 {
		t.Fatalf("stats = %+v, want 1 completed only", st)
	}
}
