package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newSQLiteTestStore(t *testing.T) *SQLStore {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "jobs.db") + "?_busy_timeout=10000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ApplySchema(context.Background(), db, "sqlite"); err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLStore(db, "sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// resetSQLStore empties both tables and the injected clock so the shared
// test bodies can run sequentially against one store.
func resetSQLStore(t *testing.T, s *SQLStore) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM regius_jobs"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM regius_locks"); err != nil {
		t.Fatal(err)
	}
	s.Now = nil
}

// The test bodies below are dialect-agnostic: the sqlite matrix runs them
// directly, and the docker-backed integration tests (build tag
// "integration") run them against postgres and mysql.

func testSQLClaimOrderAndReadiness(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
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

func testSQLConcurrentClaims(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
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
	if st.Completed != total || st.Pending != 0 || st.Running != 0 {
		t.Fatalf("stats = %+v, want %d completed only", st, total)
	}
}

func testSQLPayloadRoundTrip(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
	ctx := context.Background()

	type order struct {
		UserID int    `json:"user_id"`
		Note   string `json:"note"`
	}
	j := newTestJob("p1", "email", testNow)
	j.Payload = []byte(`{"user_id":7,"note":"héllo <>&\"quoted\""}`)
	if err := s.Enqueue(ctx, j); err != nil {
		t.Fatal(err)
	}

	claimed, err := s.Claim(ctx, testNow, testNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var p order
	if err := claimed.Decode(&p); err != nil {
		t.Fatalf("decode claimed: %v", err)
	}
	if p.UserID != 7 || p.Note != `héllo <>&"quoted"` {
		t.Fatalf("payload after claim = %+v", p)
	}

	if err := s.Complete(ctx, claimed.ID, testNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	var p2 order
	if err := got.Decode(&p2); err != nil {
		t.Fatalf("decode stored: %v", err)
	}
	if p2 != p {
		t.Fatalf("payload after complete = %+v, want %+v", p2, p)
	}
}

func testSQLCompleteAndFail(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
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

	// dead path
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

func testSQLReclaimExpired(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
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

func testSQLRetryDrop(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
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

	// Retry only works on dead jobs (y never becomes claimable)
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

func testSQLStatsListGet(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
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

func testSQLListOrders(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
	ctx := context.Background()

	ids := []string{"a", "b", "c"}
	offsets := []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute}
	for i, id := range ids {
		j := newTestJob(id, "email", testNow)
		if err := s.Enqueue(ctx, j); err != nil {
			t.Fatal(err)
		}
		c, err := s.Claim(ctx, testNow, testNow.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Complete(ctx, c.ID, testNow.Add(offsets[i])); err != nil {
			t.Fatal(err)
		}
	}
	// completed at +1m, +2m, +3m: newest first is c, b, a
	got, err := s.List(ctx, ListFilter{Status: StatusCompleted})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "c" || got[1].ID != "b" || got[2].ID != "a" {
		var ids []string
		for _, j := range got {
			ids = append(ids, j.ID)
		}
		t.Fatalf("completed order = %v, want [c b a]", ids)
	}
}

func testSQLTryLock(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
	ctx := context.Background()
	current := testNow
	s.Now = func() time.Time { return current }

	ok, err := s.TryLock(ctx, "sched:email", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first TryLock = %v, %v; want true, nil", ok, err)
	}
	ok, err = s.TryLock(ctx, "sched:email", time.Minute)
	if err != nil || ok {
		t.Fatalf("second TryLock = %v, %v; want false, nil", ok, err)
	}
	ok, err = s.TryLock(ctx, "sched:report", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLock on another name = %v, %v; want true, nil", ok, err)
	}
	current = testNow.Add(90 * time.Second) // the email lock has expired
	ok, err = s.TryLock(ctx, "sched:email", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryLock after TTL expiry = %v, %v; want true, nil", ok, err)
	}
}

func testSQLPrune(t *testing.T, s *SQLStore) {
	resetSQLStore(t, s)
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
	if _, err := s.Get(ctx, "recent"); err != nil {
		t.Fatalf("recent completed job pruned: %v", err)
	}
}

// runSQLStoreMatrix runs every shared test body against one store, in
// subtests, resetting between them.
func runSQLStoreMatrix(t *testing.T, s *SQLStore) {
	t.Helper()
	t.Run("claim order and readiness", func(t *testing.T) { testSQLClaimOrderAndReadiness(t, s) })
	t.Run("concurrent claims", func(t *testing.T) { testSQLConcurrentClaims(t, s) })
	t.Run("payload round trip", func(t *testing.T) { testSQLPayloadRoundTrip(t, s) })
	t.Run("complete and fail", func(t *testing.T) { testSQLCompleteAndFail(t, s) })
	t.Run("reclaim expired", func(t *testing.T) { testSQLReclaimExpired(t, s) })
	t.Run("retry and drop", func(t *testing.T) { testSQLRetryDrop(t, s) })
	t.Run("stats list get", func(t *testing.T) { testSQLStatsListGet(t, s) })
	t.Run("list orders newest first", func(t *testing.T) { testSQLListOrders(t, s) })
	t.Run("try lock", func(t *testing.T) { testSQLTryLock(t, s) })
	t.Run("prune", func(t *testing.T) { testSQLPrune(t, s) })
}

func TestSQLiteStore_Matrix(t *testing.T) {
	runSQLStoreMatrix(t, newSQLiteTestStore(t))
}

func TestManager_OverSQLStore(t *testing.T) {
	s := newSQLiteTestStore(t)
	m := New(s, ManagerOptions{Workers: 1, PollInterval: time.Millisecond})

	type order struct{ ID int }
	var got order
	ran := make(chan struct{}, 1)
	m.MustRegister("charge", func(_ context.Context, j *Job) error {
		if err := j.Decode(&got); err != nil {
			return err
		}
		ran <- struct{}{}
		return nil
	}, Options{})
	var calls atomic.Int32
	m.MustRegister("doomed", func(context.Context, *Job) error {
		calls.Add(1)
		return errors.New("no")
	}, Options{MaxAttempts: 2, Backoff: FixedBackoff(time.Millisecond)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	if _, err := m.Enqueue(ctx, "charge", order{ID: 42}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never ran")
	}
	if got.ID != 42 {
		t.Fatalf("payload round-trip failed: %+v", got)
	}
	waitFor(t, 2*time.Second, func() bool { return currentStats(t, m).Completed == 1 })

	if _, err := m.Enqueue(ctx, "doomed", nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		return calls.Load() == 2 && currentStats(t, m).Dead == 1
	})

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
}
