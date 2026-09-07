package regius

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hbarral/regius/jobs"
)

func waitForJobsStat(t *testing.T, r *Regius, cond func(jobs.Stats) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st, err := r.Jobs.Stats(context.Background())
		if err == nil && cond(st) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("jobs stats condition not met within 2s")
}

func TestIntegration_JobsWiring(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"JOBS_ENABLED":       "true",
		"JOBS_POLL_INTERVAL": "1ms",
		"JOBS_MAX_ATTEMPTS":  "5",
	})

	require.NotNil(t, r.Jobs)
	assert.True(t, r.config.jobs.enabled)
	assert.Equal(t, "memory", r.config.jobs.backend)
	assert.Equal(t, 5, r.config.jobs.maxAttempts)

	// workers are started explicitly here (tests don't call
	// ListenAndServe); enqueue runs through the fully wired manager
	ran := make(chan struct{}, 1)
	r.Jobs.MustRegister("wired", func(_ context.Context, _ *jobs.Job) error {
		ran <- struct{}{}
		return nil
	}, jobs.Options{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Jobs.Start(ctx)

	j, err := r.Jobs.Enqueue(ctx, "wired", map[string]string{"hello": "world"})
	require.NoError(t, err)
	assert.Equal(t, jobs.StatusPending, j.Status)
	assert.Equal(t, 5, j.MaxAttempts)

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("job never ran")
	}
	waitForJobsStat(t, r, func(st jobs.Stats) bool { return st.Completed == 1 })

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, r.Jobs.Stop(stopCtx))
}

func TestIntegration_JobsDefaults(t *testing.T) {
	r := newTestApp(t, nil)

	require.NotNil(t, r.Jobs)
	assert.False(t, r.config.jobs.enabled)
	assert.Equal(t, "memory", r.config.jobs.backend)
	assert.Equal(t, "regius:jobs", r.config.jobs.prefix)
	assert.Equal(t, 4, r.config.jobs.workers)
	assert.Equal(t, time.Second, r.config.jobs.pollInterval)
	assert.Equal(t, 5*time.Minute, r.config.jobs.lease)
	assert.Equal(t, 3, r.config.jobs.maxAttempts)
	assert.Equal(t, 30*time.Second, r.config.jobs.gracefulTimeout)
	assert.Equal(t, 24*time.Hour, r.config.jobs.retention)
	assert.True(t, r.config.jobs.schedulerLock)
	assert.False(t, r.config.jobs.dashboardEnabled)

	// the manager exists and accepts work, but nothing runs until an
	// explicit Start / ListenAndServe
	r.Jobs.MustRegister("idle", func(context.Context, *jobs.Job) error { return nil }, jobs.Options{})
	ctx := context.Background()
	_, err := r.Jobs.Enqueue(ctx, "idle", nil)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	st, err := r.Jobs.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, st.Pending)
}

func TestIntegration_JobsBackendErrors(t *testing.T) {
	r := newTestApp(t, nil)

	r.config.jobs.backend = "bogus"
	_, err := r.createJobsManager()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported jobs backend")

	// sql backend without a database connection
	r.config.jobs.backend = "sql"
	_, err = r.createJobsManager()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a database connection")
}

func TestIntegration_ListenAndServeDrainsJobs(t *testing.T) {
	// Occupy a port so the HTTP server fails to bind right after the jobs
	// workers start; the deferred Stop must then drain: in-flight attempts
	// are cancelled and their jobs requeued.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	port := fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)

	r := newTestApp(t, map[string]string{
		"JOBS_ENABLED":          "true",
		"JOBS_POLL_INTERVAL":    "1ms",
		"JOBS_GRACEFUL_TIMEOUT": "2s",
		"PORT":                  port,
	})

	entered := make(chan struct{})
	r.Jobs.MustRegister("blocked", func(ctx context.Context, _ *jobs.Job) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}, jobs.Options{})

	_, err = r.Jobs.Enqueue(context.Background(), "blocked", nil)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- r.ListenAndServe() }()

	// Either the worker claims the job before the bind fails, or the bind
	// fails first and the job is never claimed — both are valid.
	select {
	case <-entered:
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case err := <-done:
		require.Error(t, err) // bind: address already in use
	case <-time.After(3 * time.Second):
		t.Fatal("ListenAndServe did not return")
	}

	// Whatever won the race, the job sits safely back in pending.
	waitForJobsStat(t, r, func(st jobs.Stats) bool { return st.Pending == 1 })

	got, err := r.Jobs.List(context.Background(), jobs.ListFilter{Status: jobs.StatusPending})
	require.NoError(t, err)
	require.Len(t, got, 1)
	select {
	case <-entered:
		assert.Equal(t, 1, got[0].Attempts)
		assert.Equal(t, "interrupted by shutdown", got[0].LastError)
	default:
		// never claimed: attempts 0, no error
		assert.Equal(t, 0, got[0].Attempts)
	}
}
