package regius

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hbarral/regius/jobs"
)

// jobsConfig holds the background-jobs configuration resolved from the
// JOBS_* environment variables. Jobs are opt-in: the manager is always
// constructed (so Enqueue works everywhere), but workers and the scheduler
// run only while enabled and started.
type jobsConfig struct {
	enabled          bool
	backend          string
	prefix           string
	workers          int
	pollInterval     time.Duration
	lease            time.Duration
	maxAttempts      int
	gracefulTimeout  time.Duration
	retention        time.Duration
	schedulerLock    bool
	dashboardEnabled bool
}

func (r *Regius) createJobsConfig() jobsConfig {
	enabled := false
	if v := os.Getenv("JOBS_ENABLED"); v != "" {
		enabled, _ = strconv.ParseBool(v)
	}

	backend := strings.ToLower(strings.TrimSpace(os.Getenv("JOBS_BACKEND")))
	if backend == "" {
		backend = "memory"
	}

	prefix := os.Getenv("JOBS_PREFIX")
	if prefix == "" {
		prefix = "regius:jobs"
	}

	workers, _ := strconv.Atoi(os.Getenv("JOBS_WORKERS"))
	if workers <= 0 {
		workers = 4
	}

	pollInterval := parseDurationEnv("JOBS_POLL_INTERVAL", time.Second)
	lease := parseDurationEnv("JOBS_LEASE", 5*time.Minute)

	maxAttempts, _ := strconv.Atoi(os.Getenv("JOBS_MAX_ATTEMPTS"))
	if maxAttempts <= 0 {
		maxAttempts = 3
	}

	gracefulTimeout := parseDurationEnv("JOBS_GRACEFUL_TIMEOUT", 30*time.Second)
	retention := parseDurationEnv("JOBS_RETENTION", 24*time.Hour)

	schedulerLock := true
	if v := os.Getenv("JOBS_SCHEDULER_LOCK"); v != "" {
		schedulerLock, _ = strconv.ParseBool(v)
	}

	dashboardEnabled := false
	if v := os.Getenv("JOBS_DASHBOARD_ENABLED"); v != "" {
		dashboardEnabled, _ = strconv.ParseBool(v)
	}

	return jobsConfig{
		enabled:          enabled,
		backend:          backend,
		prefix:           prefix,
		workers:          workers,
		pollInterval:     pollInterval,
		lease:            lease,
		maxAttempts:      maxAttempts,
		gracefulTimeout:  gracefulTimeout,
		retention:        retention,
		schedulerLock:    schedulerLock,
		dashboardEnabled: dashboardEnabled,
	}
}

// createJobsManager builds the jobs.Manager over the configured backend:
// memory (the default), redis (reusing the framework's REDIS_* pool wiring),
// or sql (the app's database pool, dialect from DATABASE_TYPE). The manager
// is constructed unconditionally so Enqueue always works — workers only run
// when JOBS_ENABLED is true and ListenAndServe (or an explicit Start)
// launches them.
func (r *Regius) createJobsManager() (*jobs.Manager, error) {
	var store jobs.Store
	switch r.config.jobs.backend {
	case "memory":
		store = jobs.NewMemoryStore()

	case "redis":
		pool := redisPool
		if pool == nil {
			pool = r.createRedisPool()
			redisPool = pool
		}
		store = jobs.NewRedisStore(pool, r.config.jobs.prefix)

	case "sql":
		if r.DB.Pool == nil {
			return nil, fmt.Errorf("jobs backend %q requires a database connection (set DATABASE_TYPE)", r.config.jobs.backend)
		}
		sqlStore, err := jobs.NewSQLStore(r.DB.Pool, r.DB.DataType)
		if err != nil {
			return nil, err
		}
		store = sqlStore

	default:
		return nil, fmt.Errorf("unsupported jobs backend %q (want memory, redis, or sql)", r.config.jobs.backend)
	}

	return jobs.New(store, jobs.ManagerOptions{
		Workers:       r.config.jobs.workers,
		PollInterval:  r.config.jobs.pollInterval,
		Lease:         r.config.jobs.lease,
		MaxAttempts:   r.config.jobs.maxAttempts,
		Retention:     r.config.jobs.retention,
		SchedulerLock: jobs.BoolPtr(r.config.jobs.schedulerLock),
		InfoLog:       r.InfoLog,
		ErrorLog:      r.ErrorLog,
	}), nil
}

// parseDurationEnv reads a Go duration (e.g. "500ms", "5m") from an
// environment variable, falling back to def when unset or invalid.
func parseDurationEnv(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
