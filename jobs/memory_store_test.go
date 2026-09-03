package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func newTestJob(id, name string, runAt time.Time) *Job {
	return &Job{
		ID:          id,
		Name:        name,
		Queue:       QueueDefault,
		Status:      StatusPending,
		MaxAttempts: 3,
		RunAt:       runAt,
		CreatedAt:   runAt,
		UpdatedAt:   runAt,
	}
}

func TestMemoryStore_Claim(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	lease := testNow.Add(5 * time.Minute)

	if _, err := s.Claim(ctx, testNow, lease); !errors.Is(err, ErrNoJob) {
		t.Fatalf("claim on empty store: err = %v, want ErrNoJob", err)
	}

	older := newTestJob("older", "email", testNow.Add(-2*time.Minute))
	ready := newTestJob("ready", "email", testNow.Add(-time.Minute))
	future := newTestJob("future", "email", testNow.Add(time.Hour))
	for _, j := range []*Job{ready, future, older} {
		if err := s.Enqueue(ctx, j); err != nil {
			t.Fatal(err)
		}
	}

	j, err := s.Claim(ctx, testNow, lease)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if j.ID != "older" {
		t.Fatalf("claimed %q, want oldest ready job %q", j.ID, "older")
	}
	if j.Status != StatusRunning || j.Attempts != 1 {
		t.Fatalf("status = %q, attempts = %d, want running/1", j.Status, j.Attempts)
	}
	if !j.LeaseUntil.Equal(lease) {
		t.Fatalf("LeaseUntil = %v, want %v", j.LeaseUntil, lease)
	}

	j, err = s.Claim(ctx, testNow, lease)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if j.ID != "ready" {
		t.Fatalf("claimed %q, want %q", j.ID, "ready")
	}
	if _, err := s.Claim(ctx, testNow, lease); !errors.Is(err, ErrNoJob) {
		t.Fatalf("claim before RunAt: err = %v, want ErrNoJob", err)
	}
	if _, err := s.Claim(ctx, testNow.Add(2*time.Hour), testNow.Add(3*time.Hour)); err != nil {
		t.Fatalf("claim after RunAt: %v", err)
	}
}

func TestMemoryStore_CompleteAndFail(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	// complete path
	j1 := newTestJob("c1", "email", testNow)
	if err := s.Enqueue(ctx, j1); err != nil {
		t.Fatal(err)
	}
	c1, err := s.Claim(ctx, testNow, testNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	doneAt := testNow.Add(2 * time.Second)
	if err := s.Complete(ctx, c1.ID, doneAt); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := s.Get(ctx, j1.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusCompleted || !got.CompletedAt.Equal(doneAt) || !got.LeaseUntil.IsZero() {
		t.Fatalf("completed job = %+v", got)
	}
	if err := s.Complete(ctx, c1.ID, doneAt); !errors.Is(err, ErrNoJob) {
		t.Fatalf("complete twice: err = %v, want ErrNoJob", err)
	}

	// retry path
	j2 := newTestJob("c2", "email", testNow)
	if err := s.Enqueue(ctx, j2); err != nil {
		t.Fatal(err)
	}
	c2, err := s.Claim(ctx, testNow, testNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	retryAt := testNow.Add(10 * time.Second)
	if err := s.Fail(ctx, c2, testNow, retryAt, "boom"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	got, err = s.Get(ctx, j2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusPending || got.Attempts != 1 || got.LastError != "boom" ||
		!got.RunAt.Equal(retryAt) || !got.LeaseUntil.IsZero() {
		t.Fatalf("retried job = %+v", got)
	}

	// dead path: claim the retry, fail terminally
	c3, err := s.Claim(ctx, retryAt, retryAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(ctx, c3, retryAt, time.Time{}, "fatal"); err != nil {
		t.Fatalf("fail dead: %v", err)
	}
	got, err = s.Get(ctx, j2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusDead || got.LastError != "fatal" || got.Attempts != 2 {
		t.Fatalf("dead job = %+v", got)
	}

	// fail on a job that is not running
	if err := s.Fail(ctx, got, testNow, testNow, "x"); !errors.Is(err, ErrNoJob) {
		t.Fatalf("fail non-running: err = %v, want ErrNoJob", err)
	}
}

func TestMemoryStore_ReclaimExpired(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	j1 := newTestJob("r1", "email", testNow)
	if err := s.Enqueue(ctx, j1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, testNow, testNow.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	j2 := newTestJob("r2", "email", testNow)
	if err := s.Enqueue(ctx, j2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, testNow, testNow.Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	reclaimedAt := testNow.Add(time.Minute)
	n, err := s.ReclaimExpired(ctx, reclaimedAt)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reclaimed %d, want 1", n)
	}
	got, err := s.Get(ctx, j1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusPending || got.Attempts != 1 ||
		!got.RunAt.Equal(reclaimedAt) || !got.LeaseUntil.IsZero() {
		t.Fatalf("reclaimed job = %+v", got)
	}
	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Running != 1 || st.Pending != 1 {
		t.Fatalf("stats = %+v, want 1 running / 1 pending", st)
	}
}

func TestMemoryStore_StatsListGet(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	p2 := newTestJob("p2", "report", testNow.Add(time.Hour)) // pending, delayed
	if err := s.Enqueue(ctx, p2); err != nil {
		t.Fatal(err)
	}
	r1 := newTestJob("r1", "email", testNow.Add(-time.Minute))
	if err := s.Enqueue(ctx, r1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, testNow, testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	} // r1 -> running
	p1 := newTestJob("p1", "email", testNow) // pending, due
	if err := s.Enqueue(ctx, p1); err != nil {
		t.Fatal(err)
	}
	c1 := newTestJob("c1", "email", testNow.Add(-2*time.Minute))
	if err := s.Enqueue(ctx, c1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, testNow, testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	} // c1 (oldest ready) -> running
	if err := s.Complete(ctx, c1.ID, testNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	d1 := newTestJob("d1", "report", testNow.Add(-3*time.Minute))
	if err := s.Enqueue(ctx, d1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(ctx, testNow, testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	} // d1 (oldest ready) -> running
	if err := s.Fail(ctx, d1, testNow, time.Time{}, "dead"); err != nil {
		t.Fatal(err)
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st != (Stats{Pending: 2, Running: 1, Completed: 1, Dead: 1}) {
		t.Fatalf("stats = %+v", st)
	}

	table := []struct {
		name   string
		filter ListFilter
		want   []string
	}{
		{"all", ListFilter{}, []string{"p1", "p2", "r1", "c1", "d1"}},
		{"by status pending", ListFilter{Status: StatusPending}, []string{"p1", "p2"}},
		{"by status running", ListFilter{Status: StatusRunning}, []string{"r1"}},
		{"by status dead", ListFilter{Status: StatusDead}, []string{"d1"}},
		{"by name email", ListFilter{Name: "email"}, []string{"p1", "r1", "c1"}},
		{"by status and name", ListFilter{Status: StatusPending, Name: "report"}, []string{"p2"}},
		{"limit", ListFilter{Limit: 2}, []string{"p1", "p2"}},
	}
	for _, e := range table {
		t.Run(e.name, func(t *testing.T) {
			got, err := s.List(ctx, e.filter)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(got))
			for _, j := range got {
				ids = append(ids, j.ID)
			}
			if len(ids) != len(e.want) {
				t.Fatalf("list ids = %v, want %v", ids, e.want)
			}
			for i := range ids {
				if ids[i] != e.want[i] {
					t.Fatalf("list ids = %v, want %v", ids, e.want)
				}
			}
		})
	}

	if _, err := s.Get(ctx, "nope"); !errors.Is(err, ErrNoJob) {
		t.Fatalf("get missing: err = %v, want ErrNoJob", err)
	}
}

func TestMemoryStore_RetryDrop(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	j := newTestJob("x", "email", testNow)
	if err := s.Enqueue(ctx, j); err != nil {
		t.Fatal(err)
	}
	c, err := s.Claim(ctx, testNow, testNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(ctx, c, testNow, time.Time{}, "dead"); err != nil {
		t.Fatal(err)
	}

	// Retry only works on dead jobs (y stays not-ready for the whole test,
	// so it can never be claimed by mistake)
	p := newTestJob("y", "email", testNow.Add(2*time.Hour))
	if err := s.Enqueue(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.Retry(ctx, p.ID); !errors.Is(err, ErrNoJob) {
		t.Fatalf("retry pending: err = %v, want ErrNoJob", err)
	}

	// Retry resets attempts and requeues
	retryAt := testNow.Add(time.Hour)
	s.Now = func() time.Time { return retryAt }
	if err := s.Retry(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusPending || got.Attempts != 0 || !got.RunAt.Equal(retryAt) {
		t.Fatalf("retried job = %+v", got)
	}

	// Drop only works on dead jobs
	if err := s.Drop(ctx, p.ID); !errors.Is(err, ErrNoJob) {
		t.Fatalf("drop pending: err = %v, want ErrNoJob", err)
	}
	c2, err := s.Claim(ctx, retryAt, retryAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(ctx, c2, retryAt, time.Time{}, "dead again"); err != nil {
		t.Fatal(err)
	}
	if err := s.Drop(ctx, j.ID); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := s.Get(ctx, j.ID); !errors.Is(err, ErrNoJob) {
		t.Fatalf("get dropped: err = %v, want ErrNoJob", err)
	}
}

func TestMemoryStore_Prune(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	old := newTestJob("old", "email", testNow.Add(-3*time.Hour))
	recent := newTestJob("recent", "email", testNow.Add(-30*time.Minute))
	for _, j := range []*Job{old, recent} {
		if err := s.Enqueue(ctx, j); err != nil {
			t.Fatal(err)
		}
		c, err := s.Claim(ctx, j.RunAt, j.RunAt.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Complete(ctx, c.ID, j.RunAt.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	// dead jobs are never pruned automatically
	d := newTestJob("dead", "email", testNow.Add(-3*time.Hour))
	if err := s.Enqueue(ctx, d); err != nil {
		t.Fatal(err)
	}
	dc, err := s.Claim(ctx, d.RunAt, d.RunAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(ctx, dc, d.RunAt, time.Time{}, "dead"); err != nil {
		t.Fatal(err)
	}

	n, err := s.Prune(ctx, testNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Completed != 1 || st.Dead != 1 {
		t.Fatalf("stats after prune = %+v, want 1 completed / 1 dead", st)
	}
	if _, err := s.Get(ctx, "old"); !errors.Is(err, ErrNoJob) {
		t.Fatal("old completed job still present after prune")
	}
}

func TestMemoryStore_TryLockIsNoOp(t *testing.T) {
	s := NewMemoryStore()
	ok, err := s.TryLock(context.Background(), "sched:email", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLock = %v, %v; want true, nil", ok, err)
	}
}

func TestMemoryStore_ConcurrentClaimsAreExclusive(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	const total = 50
	for i := 0; i < total; i++ {
		if err := s.Enqueue(ctx, newTestJob(fmt.Sprintf("j%d", i), "email", testNow)); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	claimed := make(map[string]int)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				j, err := s.Claim(ctx, testNow, testNow.Add(time.Minute))
				if errors.Is(err, ErrNoJob) {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				claimed[j.ID]++
				mu.Unlock()
				if err := s.Complete(ctx, j.ID, testNow); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if len(claimed) != total {
		t.Fatalf("claimed %d distinct jobs, want %d", len(claimed), total)
	}
	for id, n := range claimed {
		if n != 1 {
			t.Fatalf("job %s claimed %d times", id, n)
		}
	}
	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Completed != total || st.Pending != 0 {
		t.Fatalf("stats = %+v, want %d completed / 0 pending", st, total)
	}
}
