package jobs

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestManager(t *testing.T, opts ManagerOptions) *Manager {
	t.Helper()
	if opts.Workers <= 0 {
		opts.Workers = 1
	}
	opts.PollInterval = time.Millisecond
	return New(NewMemoryStore(), opts)
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %v", d)
}

func currentStats(t *testing.T, m *Manager) Stats {
	t.Helper()
	st, err := m.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestManager_EnqueueRunComplete(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	type welcome struct{ UserID int }

	var got welcome
	ran := make(chan *Job, 1)
	m.MustRegister("welcome", func(_ context.Context, j *Job) error {
		if err := j.Decode(&got); err != nil {
			return err
		}
		ran <- j
		return nil
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	j, err := m.Enqueue(ctx, "welcome", welcome{UserID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if j.ID == "" || j.Status != StatusPending || j.Queue != QueueDefault || j.MaxAttempts != defaultMaxAttempts {
		t.Fatalf("enqueued job = %+v", j)
	}

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never ran")
	}
	if got.UserID != 7 {
		t.Fatalf("decoded payload = %+v, want UserID 7", got)
	}
	waitFor(t, 2*time.Second, func() bool { return currentStats(t, m).Completed == 1 })
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
}

func TestManager_RetriesUntilSuccess(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	var calls atomic.Int32
	m.MustRegister("flaky", func(context.Context, *Job) error {
		if calls.Add(1) < 3 {
			return errors.New("transient")
		}
		return nil
	}, Options{MaxAttempts: 5, Backoff: FixedBackoff(time.Millisecond)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	j, err := m.Enqueue(ctx, "flaky", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return currentStats(t, m).Completed == 1 })
	if calls.Load() != 3 {
		t.Fatalf("handler calls = %d, want 3", calls.Load())
	}
	got, err := m.Get(context.Background(), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempts != 3 || got.Status != StatusCompleted {
		t.Fatalf("completed job = %+v", got)
	}
	m.Stop(context.Background())
}

func TestManager_DeadLetterRetryDrop(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	var calls atomic.Int32
	m.MustRegister("doomed", func(context.Context, *Job) error {
		calls.Add(1)
		return errors.New("always fails")
	}, Options{MaxAttempts: 2, Backoff: FixedBackoff(time.Millisecond)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	j, err := m.Enqueue(ctx, "doomed", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return currentStats(t, m).Dead == 1 })
	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2", calls.Load())
	}
	got, err := m.Get(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempts != 2 || got.LastError != "always fails" {
		t.Fatalf("dead job = %+v", got)
	}

	// Retry resurrects the job with a fresh attempt budget
	if err := m.Retry(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		return calls.Load() == 4 && currentStats(t, m).Dead == 1
	})

	// Drop removes it for good
	if err := m.Drop(ctx, j.ID); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := m.Get(ctx, j.ID); !errors.Is(err, ErrNoJob) {
		t.Fatalf("get dropped: err = %v, want ErrNoJob", err)
	}
	m.Stop(context.Background())
}

func TestManager_AttemptTimeout(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	m.MustRegister("slow", func(ctx context.Context, _ *Job) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}, Options{Timeout: 5 * time.Millisecond, MaxAttempts: 1, Backoff: FixedBackoff(time.Millisecond)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	j, err := m.Enqueue(ctx, "slow", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return currentStats(t, m).Dead == 1 })
	got, err := m.Get(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.LastError, "context deadline exceeded") {
		t.Fatalf("LastError = %q, want deadline error", got.LastError)
	}
	m.Stop(context.Background())
}

func TestManager_HandlerPanicIsFailedAttempt(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	m.MustRegister("panicky", func(context.Context, *Job) error {
		panic("boom")
	}, Options{MaxAttempts: 1, Backoff: FixedBackoff(time.Millisecond)})
	ran := make(chan struct{}, 1)
	m.MustRegister("healthy", func(context.Context, *Job) error {
		ran <- struct{}{}
		return nil
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	pj, err := m.Enqueue(ctx, "panicky", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return currentStats(t, m).Dead == 1 })
	got, err := m.Get(ctx, pj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.LastError, "panic") {
		t.Fatalf("LastError = %q, want panic recorded", got.LastError)
	}

	// the worker survived the panic and processes further work
	if _, err := m.Enqueue(ctx, "healthy", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not survive the panic")
	}
	m.Stop(context.Background())
}

func TestManager_StopRequeuesInFlightJob(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	entered := make(chan struct{})
	m.MustRegister("blocked", func(ctx context.Context, _ *Job) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}, Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	j, err := m.Enqueue(ctx, "blocked", nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never started")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := m.Stop(stopCtx); err != nil {
		t.Fatalf("stop: %v", err)
	}

	got, err := m.Get(context.Background(), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusPending || got.Attempts != 1 {
		t.Fatalf("interrupted job = %+v, want pending with 1 attempt", got)
	}
	if st := currentStats(t, m); st.Pending != 1 || st.Running != 0 || st.Completed != 0 {
		t.Fatalf("stats = %+v, want 1 pending only", st)
	}
}

func TestManager_StopVariants(t *testing.T) {
	// Stop without Start is a no-op
	m := New(NewMemoryStore(), ManagerOptions{})
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("stop without start: %v", err)
	}

	// Stop while idle drains immediately
	m2 := newTestManager(t, ManagerOptions{})
	m2.Start(context.Background())
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m2.Stop(stopCtx); err != nil {
		t.Fatalf("stop idle: %v", err)
	}

	// Start after Stop is a no-op: no workers come back
	m3 := newTestManager(t, ManagerOptions{})
	m3.Start(context.Background())
	m3.Stop(context.Background())
	m3.Start(context.Background())
	ran := make(chan struct{}, 1)
	m3.MustRegister("x", func(context.Context, *Job) error {
		ran <- struct{}{}
		return nil
	}, Options{})
	if _, err := m3.Enqueue(context.Background(), "x", nil); err != nil {
		t.Fatalf("enqueue after restart attempt: %v", err)
	}
	select {
	case <-ran:
		t.Fatal("worker ran after Stop")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestManager_RegistrationAndUnknownJobs(t *testing.T) {
	m := newTestManager(t, ManagerOptions{})
	h := func(context.Context, *Job) error { return nil }

	// boot-time enqueue of an unregistered name is allowed (the handler may
	// be registered before workers start)
	if _, err := m.Enqueue(context.Background(), "later", nil); err != nil {
		t.Fatalf("boot enqueue: %v", err)
	}

	if err := m.Register("", h, Options{}); err == nil {
		t.Fatal("register empty name: want error")
	}
	if err := m.Register("nil", nil, Options{}); err == nil {
		t.Fatal("register nil handler: want error")
	}
	m.MustRegister("dup", h, Options{})
	if err := m.Register("dup", h, Options{}); !errors.Is(err, ErrDuplicateHandler) {
		t.Fatalf("duplicate register: err = %v, want ErrDuplicateHandler", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	// a running manager rejects unregistered names
	if _, err := m.Enqueue(ctx, "ghost", nil); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("enqueue unregistered: err = %v, want ErrUnknownJob", err)
	}

	// the boot-enqueued unregistered job is dead-lettered by the worker
	waitFor(t, 2*time.Second, func() bool { return currentStats(t, m).Dead == 1 })
	got, err := m.List(context.Background(), ListFilter{Status: StatusDead})
	if err != nil || len(got) != 1 {
		t.Fatalf("dead list = %v, err = %v", got, err)
	}
	if got[0].Name != "later" || got[0].LastError != "no registered handler" {
		t.Fatalf("dead job = %+v", got[0])
	}
	m.Stop(context.Background())
}

func TestManager_EnqueueOptionsResolution(t *testing.T) {
	m := newTestManager(t, ManagerOptions{MaxAttempts: 4})
	h := func(context.Context, *Job) error { return nil }
	m.MustRegister("defaults", h, Options{})
	m.MustRegister("own", h, Options{MaxAttempts: 2})

	ctx := context.Background()
	a, err := m.Enqueue(ctx, "defaults", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.MaxAttempts != 4 {
		t.Fatalf("defaults MaxAttempts = %d, want 4", a.MaxAttempts)
	}
	b, err := m.Enqueue(ctx, "own", nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.MaxAttempts != 2 {
		t.Fatalf("own MaxAttempts = %d, want 2", b.MaxAttempts)
	}
	c, err := m.EnqueueWithOptions(ctx, "defaults", nil, EnqueueOptions{MaxAttempts: 9, RunAt: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxAttempts != 9 || !c.RunAt.Equal(testNow) {
		t.Fatalf("overridden job = %+v", c)
	}
}
