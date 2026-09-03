// Package jobs provides a background job queue: handler registration,
// enqueueing, a worker pool, retries with backoff, dead-lettering, and
// monitoring, persisted through a pluggable Store.
//
// Usage:
//
//	store := jobs.NewMemoryStore()
//	m := jobs.New(store, jobs.ManagerOptions{})
//
//	m.MustRegister("send_welcome_email", func(ctx context.Context, j *jobs.Job) error {
//		var p welcomePayload
//		if err := j.Decode(&p); err != nil {
//			return err
//		}
//		return sendEmail(p)
//	}, jobs.Options{MaxAttempts: 5})
//
//	m.Start(ctx) // workers run until ctx is cancelled or Stop is called
//	defer m.Stop(context.Background())
//
//	_, err := m.Enqueue(ctx, "send_welcome_email", welcomePayload{UserID: 42})
//
// Delivery is at-least-once: a crash between claiming and completing a job
// means it runs again after its lease expires. Handlers must be idempotent.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rs/xid"
)

const (
	defaultWorkers      = 1
	defaultPollInterval = time.Second
	defaultLease        = 5 * time.Minute
	defaultMaxAttempts  = 3
	defaultListLimit    = 50
	maxListLimit        = 200
	defaultBackoffBase  = 30 * time.Second
	defaultBackoffMax   = 15 * time.Minute
	maxLastErrorLen     = 2048
)

// ManagerOptions configures a Manager. Zero values fall back to
// single-process defaults.
type ManagerOptions struct {
	// Workers is the number of worker goroutines. <= 0 means 1.
	Workers int

	// PollInterval is how often workers re-poll the store when idle. <= 0
	// means 1s.
	PollInterval time.Duration

	// Lease is how long a claim holds before crash recovery requeues the
	// job. <= 0 means 5m.
	Lease time.Duration

	// MaxAttempts is the default attempt budget for handlers that don't set
	// Options.MaxAttempts. <= 0 means 3.
	MaxAttempts int

	// InfoLog and ErrorLog receive lifecycle and failure logs. nil disables
	// them; the app's InfoLog/ErrorLog fit directly.
	InfoLog  *log.Logger
	ErrorLog *log.Logger
}

// Manager ties handlers, a Store, and a worker pool together: register
// handlers, Start the workers, Enqueue work, Stop to drain. A Manager cannot
// be restarted after Stop.
type Manager struct {
	store Store
	opts  ManagerOptions
	now   func() time.Time

	mu       sync.Mutex
	handlers map[string]handlerEntry
	started  bool
	stopped  bool
	cancel   context.CancelFunc

	wg sync.WaitGroup
}

type handlerEntry struct {
	fn          Handler
	maxAttempts int
	timeout     time.Duration
	backoff     BackoffFunc
}

// New creates a Manager over store, replacing zero-valued options with
// defaults.
func New(store Store, opts ManagerOptions) *Manager {
	if opts.Workers <= 0 {
		opts.Workers = defaultWorkers
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultPollInterval
	}
	if opts.Lease <= 0 {
		opts.Lease = defaultLease
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaultMaxAttempts
	}
	return &Manager{
		store:    store,
		opts:     opts,
		now:      time.Now,
		handlers: make(map[string]handlerEntry),
	}
}

// Register makes name executable by h. Registration is expected before
// Start; the map is mutex-guarded so late registration is safe, but jobs
// enqueued while workers are running must reference a registered name.
// Register returns ErrDuplicateHandler if name is already registered.
func (m *Manager) Register(name string, h Handler, opts Options) error {
	if name == "" {
		return errors.New("jobs: job name must not be empty")
	}
	if h == nil {
		return fmt.Errorf("jobs: handler for %q must not be nil", name)
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = m.opts.MaxAttempts
	}
	backoff := opts.Backoff
	if backoff == nil {
		backoff = ExponentialBackoff(defaultBackoffBase, defaultBackoffMax)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.handlers[name]; ok {
		return fmt.Errorf("%w: %q", ErrDuplicateHandler, name)
	}
	m.handlers[name] = handlerEntry{fn: h, maxAttempts: maxAttempts, timeout: opts.Timeout, backoff: backoff}
	return nil
}

// MustRegister is Register for app boot code; it panics on error.
func (m *Manager) MustRegister(name string, h Handler, opts Options) {
	if err := m.Register(name, h, opts); err != nil {
		panic(err)
	}
}

// Enqueue adds a job for the registered name, JSON-encoding payload (nil
// encodes as null). Success means the store accepted the job, not that it
// ran.
func (m *Manager) Enqueue(ctx context.Context, name string, payload any) (*Job, error) {
	return m.EnqueueWithOptions(ctx, name, payload, EnqueueOptions{})
}

// EnqueueWithOptions is Enqueue with per-job overrides.
func (m *Manager) EnqueueWithOptions(ctx context.Context, name string, payload any, o EnqueueOptions) (*Job, error) {
	h, registered := m.handler(name)
	if m.running() && !registered {
		return nil, fmt.Errorf("%w: %q", ErrUnknownJob, name)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("jobs: encode payload for %q: %w", name, err)
	}
	maxAttempts := o.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = h.maxAttempts
	}
	if maxAttempts <= 0 {
		maxAttempts = m.opts.MaxAttempts
	}
	now := m.now()
	j := &Job{
		ID:          xid.New().String(),
		Name:        name,
		Payload:     raw,
		Queue:       QueueDefault,
		Status:      StatusPending,
		MaxAttempts: maxAttempts,
		RunAt:       now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if !o.RunAt.IsZero() {
		j.RunAt = o.RunAt
	}
	if err := m.store.Enqueue(ctx, j); err != nil {
		return nil, fmt.Errorf("jobs: enqueue %q: %w", name, err)
	}
	return j, nil
}

// Start launches the worker pool under ctx. Workers stop when ctx is
// cancelled or Stop is called; in-flight attempts are cancelled and their
// jobs requeued. Start is a no-op when already started or stopped.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started || m.stopped {
		m.mu.Unlock()
		return
	}
	m.started = true
	ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	m.logInfo("jobs: started %d worker(s)", m.opts.Workers)
	for i := 0; i < m.opts.Workers; i++ {
		m.wg.Add(1)
		go m.work(ctx)
	}
}

// Stop cancels in-flight attempts and waits for all workers to exit,
// bounded by ctx. Jobs interrupted by the shutdown are requeued immediately
// (the attempt still counts, mirroring crash recovery). Stop returns nil
// once drained, or an error if ctx expires first. It is idempotent and safe
// to call without Start.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	m.stopped = true
	cancel := m.cancel
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	drained := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("jobs: stop timed out waiting for workers: %w", ctx.Err())
	}
}

// Stats returns job counts by status.
func (m *Manager) Stats(ctx context.Context) (Stats, error) {
	return m.store.Stats(ctx)
}

// List returns jobs matching the filter: pending and running ordered by
// RunAt, then completed and dead newest first. Limit defaults to 50 and is
// capped at 200.
func (m *Manager) List(ctx context.Context, f ListFilter) ([]*Job, error) {
	if f.Limit <= 0 {
		f.Limit = defaultListLimit
	}
	if f.Limit > maxListLimit {
		f.Limit = maxListLimit
	}
	return m.store.List(ctx, f)
}

// Get returns the job with the given ID regardless of status.
func (m *Manager) Get(ctx context.Context, id string) (*Job, error) {
	return m.store.Get(ctx, id)
}

// Retry moves a dead job back to pending with its attempts reset, so it
// gets a full attempt budget again.
func (m *Manager) Retry(ctx context.Context, id string) error {
	return m.store.Retry(ctx, id)
}

// Drop permanently removes a dead job.
func (m *Manager) Drop(ctx context.Context, id string) error {
	return m.store.Drop(ctx, id)
}

func (m *Manager) work(ctx context.Context) {
	defer m.wg.Done()
	for {
		if ctx.Err() != nil {
			return
		}
		j, err := m.store.Claim(ctx, m.now(), m.now().Add(m.opts.Lease))
		if err != nil {
			if !errors.Is(err, ErrNoJob) {
				m.logError("jobs: claim: %v", err)
			}
			if !m.idle(ctx) {
				return
			}
			continue
		}
		m.execute(ctx, j)
	}
}

// idle waits out the poll interval, reporting false once ctx is done.
func (m *Manager) idle(ctx context.Context) bool {
	t := time.NewTimer(m.opts.PollInterval)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (m *Manager) execute(ctx context.Context, j *Job) {
	// Final state writes must survive the worker context: a cancelled ctx
	// must not lose the transition the handler already earned. The Redis
	// and SQL stores depend on this.
	fctx := context.WithoutCancel(ctx)

	h, ok := m.handler(j.Name)
	if !ok {
		m.logError("jobs: claimed job %q (%s) with no registered handler; dead-lettering", j.Name, j.ID)
		if err := m.store.Fail(fctx, j, m.now(), time.Time{}, "no registered handler"); err != nil {
			m.logError("jobs: dead-letter %s: %v", j.ID, err)
		}
		return
	}

	err := m.runAttempt(ctx, j, h)
	now := m.now()
	switch {
	case err == nil:
		if cerr := m.store.Complete(fctx, j.ID, now); cerr != nil {
			m.logError("jobs: complete %s: %v", j.ID, cerr)
		}
	case ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)):
		// Shutdown interrupted the attempt: requeue immediately instead of
		// waiting out a lease, and skip the backoff. The attempt still
		// counts, mirroring crash recovery.
		m.logInfo("jobs: %s (%s) interrupted by shutdown; requeued", j.ID, j.Name)
		if ferr := m.store.Fail(fctx, j, now, now, "interrupted by shutdown"); ferr != nil {
			m.logError("jobs: requeue %s: %v", j.ID, ferr)
		}
	default:
		m.fail(fctx, j, h, err)
	}
}

func (m *Manager) runAttempt(ctx context.Context, j *Job, h handlerEntry) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("jobs: handler panic: %v", r)
		}
	}()
	actx := ctx
	cancel := func() {}
	if h.timeout > 0 {
		actx, cancel = context.WithTimeout(ctx, h.timeout)
	}
	defer cancel()
	return h.fn(actx, j)
}

func (m *Manager) fail(ctx context.Context, j *Job, h handlerEntry, cause error) {
	lastErr := truncateError(cause.Error())
	now := m.now()
	if j.Attempts >= h.maxAttempts {
		m.logError("jobs: %s (%s) dead after %d attempt(s): %s", j.ID, j.Name, j.Attempts, lastErr)
		if err := m.store.Fail(ctx, j, now, time.Time{}, lastErr); err != nil {
			m.logError("jobs: dead-letter %s: %v", j.ID, err)
		}
		return
	}
	retryAt := now.Add(h.backoff(j.Attempts))
	if err := m.store.Fail(ctx, j, now, retryAt, lastErr); err != nil {
		m.logError("jobs: requeue %s: %v", j.ID, err)
	}
}

func (m *Manager) handler(name string) (handlerEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.handlers[name]
	return h, ok
}

func (m *Manager) running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started && !m.stopped
}

func (m *Manager) logInfo(format string, args ...any) {
	if m.opts.InfoLog != nil {
		m.opts.InfoLog.Printf(format, args...)
	}
}

func (m *Manager) logError(format string, args ...any) {
	if m.opts.ErrorLog != nil {
		m.opts.ErrorLog.Printf(format, args...)
	}
}

func truncateError(s string) string {
	if len(s) <= maxLastErrorLen {
		return s
	}
	return s[:maxLastErrorLen] + "…(truncated)"
}
