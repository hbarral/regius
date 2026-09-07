package regius

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/hbarral/regius/api"
	"github.com/hbarral/regius/jobs"
)

// registerJobsRoutes mounts the background-job monitoring endpoints on
// the outer mux when JOBS_DASHBOARD_ENABLED is true. They live under
// /api/jobs (alongside the NoSurf/sanitizer /api/.* exemptions) and are
// disabled by default: Retry/Drop mutate state, so layer APIKeyAuth or
// IPFilter on them in production.
func (r *Regius) registerJobsRoutes(mux *chi.Mux) {
	if !r.config.jobs.dashboardEnabled || r.Jobs == nil {
		return
	}

	mux.Route("/api/jobs", func(sub chi.Router) {
		sub.Get("/stats", r.jobsStats)
		sub.Get("/", r.jobsList)
		sub.Post("/{id}/retry", r.jobsRetry)
		sub.Delete("/{id}", r.jobsDrop)
	})
}

// jobsStats reports job counts by status.
func (r *Regius) jobsStats(w http.ResponseWriter, req *http.Request) {
	st, err := r.Jobs.Stats(req.Context())
	if err != nil {
		_ = r.WriteAPIError(w, http.StatusInternalServerError, "internal_error", "failed to read job stats")
		return
	}
	_ = r.WriteAPIResponse(w, http.StatusOK, st)
}

// jobsList returns jobs matching the status/name/limit query parameters.
func (r *Regius) jobsList(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()

	f := jobs.ListFilter{}
	if s := q.Get("status"); s != "" {
		switch jobs.Status(s) {
		case jobs.StatusPending, jobs.StatusRunning, jobs.StatusCompleted, jobs.StatusDead:
			f.Status = jobs.Status(s)
		default:
			_ = r.WriteAPIError(w, http.StatusBadRequest, "invalid_status",
				"unknown status: "+s)
			return
		}
	}
	f.Name = q.Get("name")

	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 0 {
			_ = r.WriteAPIError(w, http.StatusBadRequest, "invalid_limit",
				"limit must be a non-negative integer")
			return
		}
		if n > 200 {
			_ = r.WriteAPIError(w, http.StatusBadRequest, "invalid_limit",
				"limit must be at most 200")
			return
		}
		f.Limit = n
	}

	jobs, err := r.Jobs.List(req.Context(), f)
	if err != nil {
		_ = r.WriteAPIError(w, http.StatusInternalServerError, "internal_error",
			"failed to list jobs")
		return
	}
	_ = r.WriteAPIResponse(w, http.StatusOK, jobs, &api.Meta{Total: int64(len(jobs))})
}

// jobsRetry resurrects a dead job (back to pending with a fresh attempt
// budget).
func (r *Regius) jobsRetry(w http.ResponseWriter, req *http.Request) {
	id := chi.URLParam(req, "id")
	if err := r.Jobs.Retry(req.Context(), id); err != nil {
		if errors.Is(err, jobs.ErrNoJob) {
			_ = r.WriteAPIError(w, http.StatusNotFound, "not_found",
				"no dead job with that id")
			return
		}
		_ = r.WriteAPIError(w, http.StatusInternalServerError, "internal_error",
			"failed to retry job")
		return
	}
	_ = r.WriteAPIResponse(w, http.StatusOK, map[string]string{"status": "pending"})
}

// jobsDrop permanently removes a dead job.
func (r *Regius) jobsDrop(w http.ResponseWriter, req *http.Request) {
	id := chi.URLParam(req, "id")
	if err := r.Jobs.Drop(req.Context(), id); err != nil {
		if errors.Is(err, jobs.ErrNoJob) {
			_ = r.WriteAPIError(w, http.StatusNotFound, "not_found",
				"no dead job with that id")
			return
		}
		_ = r.WriteAPIError(w, http.StatusInternalServerError, "internal_error",
			"failed to drop job")
		return
	}
	_ = r.WriteAPIResponse(w, http.StatusOK, map[string]string{"status": "dropped"})
}
