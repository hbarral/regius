package regius

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hbarral/regius/api"
	"github.com/hbarral/regius/jobs"
)

func TestIntegration_JobsDashboard_DisabledByDefault(t *testing.T) {
	r := newTestApp(t, nil)
	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	resp := dashboardDo(t, ts, http.MethodGet, "/api/jobs/stats")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode) // no route registered
}

func TestIntegration_JobsDashboard(t *testing.T) {
	r := newTestApp(t, map[string]string{
		"JOBS_ENABLED":           "true",
		"JOBS_DASHBOARD_ENABLED": "true",
		"JOBS_POLL_INTERVAL":     "1ms",
		"JOBS_MAX_ATTEMPTS":      "1",
	})
	ts := httptest.NewServer(r.Handler())
	defer ts.Close()

	// Populate the store with one job per terminal state plus a delayed one.
	// "ok" completes, "bad1"/"bad2" exhaust attempts and go dead, "later"
	// stays pending (RunAt in the future).
	ctx := context.Background()
	var (
		okID    string
		bad1ID  string
		bad2ID  string
		laterID string
	)
	r.Jobs.MustRegister("ok", func(context.Context, *jobs.Job) error { return nil }, jobs.Options{})
	r.Jobs.MustRegister("bad1", func(context.Context, *jobs.Job) error { return errors.New("no") }, jobs.Options{MaxAttempts: 1})
	r.Jobs.MustRegister("bad2", func(context.Context, *jobs.Job) error { return errors.New("no") }, jobs.Options{MaxAttempts: 1})
	r.Jobs.MustRegister("later", func(context.Context, *jobs.Job) error { return nil }, jobs.Options{})

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Jobs.Start(runCtx)

	if j, err := r.Jobs.Enqueue(ctx, "ok", nil); err == nil {
		okID = j.ID
	}
	if j, err := r.Jobs.Enqueue(ctx, "bad1", nil); err == nil {
		bad1ID = j.ID
	}
	if j, err := r.Jobs.Enqueue(ctx, "bad2", nil); err == nil {
		bad2ID = j.ID
	}
	if j, err := r.Jobs.EnqueueWithOptions(ctx, "later", nil, jobs.EnqueueOptions{RunAt: time.Now().Add(time.Hour)}); err == nil {
		laterID = j.ID
	}

	waitForJobsStat(t, r, func(st jobs.Stats) bool { return st.Completed == 1 && st.Dead == 2 && st.Pending == 1 })
	require.NoError(t, r.Jobs.Stop(context.Background()))

	// stats
	resp := dashboardDo(t, ts, http.MethodGet, "/api/jobs/stats")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var st jobs.Stats
	dashboardData(t, resp, &st)
	assert.Equal(t, jobs.Stats{Pending: 1, Completed: 1, Dead: 2}, st)

	// list all (order: pending, completed, dead)
	resp = dashboardDo(t, ts, http.MethodGet, "/api/jobs")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var list []*jobs.Job
	dashboardData(t, resp, &list)
	require.Len(t, list, 4)
	assert.Equal(t, "later", list[0].Name) // pending first
	assert.Equal(t, jobs.StatusPending, list[0].Status)

	// filters
	assert.ElementsMatch(t, []string{bad1ID, bad2ID}, dashboardList(t, ts, "/api/jobs?status=dead"))
	assertJobIDs(t, dashboardList(t, ts, "/api/jobs?status=pending"), []string{laterID})
	assertJobIDs(t, dashboardList(t, ts, "/api/jobs?name=ok"), []string{okID})
	assertJobIDs(t, dashboardList(t, ts, "/api/jobs?status=dead&name=bad2"), []string{bad2ID})
	assertJobIDs(t, dashboardList(t, ts, "/api/jobs?limit=2"), []string{laterID, okID})

	// invalid params
	resp = dashboardDo(t, ts, http.MethodGet, "/api/jobs?status=bogus")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp = dashboardDo(t, ts, http.MethodGet, "/api/jobs?limit=-1")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp = dashboardDo(t, ts, http.MethodGet, "/api/jobs?limit=201")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// retry: bad1 -> pending, then retrying it again is a 404
	resp = dashboardDo(t, ts, http.MethodPost, "/api/jobs/"+bad1ID+"/retry")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp = dashboardDo(t, ts, http.MethodPost, "/api/jobs/"+bad1ID+"/retry")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp = dashboardDo(t, ts, http.MethodPost, "/api/jobs/no-such-id/retry")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// drop: bad2 -> gone, then dropping it again is a 404
	resp = dashboardDo(t, ts, http.MethodDelete, "/api/jobs/"+bad2ID)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp = dashboardDo(t, ts, http.MethodDelete, "/api/jobs/"+bad2ID)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// final stats reflect the mutations
	resp = dashboardDo(t, ts, http.MethodGet, "/api/jobs/stats")
	defer resp.Body.Close()
	var st2 jobs.Stats
	dashboardData(t, resp, &st2)
	assert.Equal(t, jobs.Stats{Pending: 2, Completed: 1, Dead: 0}, st2)
}

// dashboardDo issues a request against the dashboard and returns the response.
func dashboardDo(t *testing.T, ts *httptest.Server, method, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	require.NoError(t, err)
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// dashboardData decodes the response envelope's Data field into v.
func dashboardData(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	var body api.Response
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	raw, err := json.Marshal(body.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, v))
}

// dashboardList fetches /api/jobs[?...] and returns the job IDs in order.
func dashboardList(t *testing.T, ts *httptest.Server, path string) []string {
	t.Helper()
	resp := dashboardDo(t, ts, http.MethodGet, path)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, path)
	var list []*jobs.Job
	dashboardData(t, resp, &list)
	ids := make([]string, len(list))
	for i, j := range list {
		ids[i] = j.ID
	}
	return ids
}

func assertJobIDs(t *testing.T, got, want []string) {
	t.Helper()
	assert.Equal(t, want, got)
}
