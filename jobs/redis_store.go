package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/gomodule/redigo/redis"
)

const (
	// zeroTimeRFC3339 is the JSON encoding of time.Time{}; scripts use it to
	// clear lease fields.
	zeroTimeRFC3339 = "0001-01-01T00:00:00Z"

	// reclaimBatch is how many expired leases one ReclaimExpired call
	// processes; the maintenance loop calls it repeatedly.
	reclaimBatch = 100

	// listScanLimit caps how many jobs per status bucket List inspects
	// before applying the name filter and limit.
	listScanLimit = 500
)

// claimScript atomically moves the oldest ready pending job to running and
// returns its updated JSON: peek, removal from the pending index, the lease
// write, and the index insert happen indivisibly, so concurrent workers can
// never claim the same job. A pending member without data (a crash between
// the two Enqueue commands cannot leave this, but manual tampering can) is
// self-healed by removing it and reporting no job.
//
// KEYS: 1 pending, 2 data, 3 running
// ARGV: 1 now unix micros, 2 lease-until RFC3339, 3 lease-until unix micros, 4 now RFC3339
const claimScript = `
local id = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, 1)[1]
if not id then return false end
local raw = redis.call('HGET', KEYS[2], id)
if not raw then
	redis.call('ZREM', KEYS[1], id)
	return false
end
local job = cjson.decode(raw)
job['status'] = 'running'
job['attempts'] = (job['attempts'] or 0) + 1
job['lease_until'] = ARGV[2]
job['updated_at'] = ARGV[4]
raw = cjson.encode(job)
redis.call('HSET', KEYS[2], id, raw)
redis.call('ZREM', KEYS[1], id)
redis.call('ZADD', KEYS[3], ARGV[3], id)
return raw
`

// completeScript marks a running job completed, guarded by a status check so
// the transition happens exactly once.
//
// KEYS: 1 data, 2 running, 3 completed
// ARGV: 1 id, 2 now RFC3339, 3 now unix micros, 4 zero time
const completeScript = `
local raw = redis.call('HGET', KEYS[1], ARGV[1])
if not raw then return 0 end
local job = cjson.decode(raw)
if job['status'] ~= 'running' then return 0 end
job['status'] = 'completed'
job['completed_at'] = ARGV[2]
job['updated_at'] = ARGV[2]
job['lease_until'] = ARGV[4]
redis.call('HSET', KEYS[1], ARGV[1], cjson.encode(job))
redis.call('ZREM', KEYS[2], ARGV[1])
redis.call('ZADD', KEYS[3], ARGV[3], ARGV[1])
return 1
`

// failScript records a failed attempt on a running job: a non-empty retryAt
// (ARGV[3]) requeues it as pending with RunAt = retryAt; an empty one
// dead-letters it.
//
// KEYS: 1 data, 2 running, 3 pending, 4 dead
// ARGV: 1 id, 2 now RFC3339, 3 retryAt RFC3339 ("" = dead), 4 retryAt unix micros, 5 now unix micros, 6 last error, 7 zero time
const failScript = `
local raw = redis.call('HGET', KEYS[1], ARGV[1])
if not raw then return 0 end
local job = cjson.decode(raw)
if job['status'] ~= 'running' then return 0 end
job['last_error'] = ARGV[6]
job['updated_at'] = ARGV[2]
job['lease_until'] = ARGV[7]
if ARGV[3] == '' then
	job['status'] = 'dead'
	redis.call('ZADD', KEYS[4], ARGV[5], ARGV[1])
else
	job['status'] = 'pending'
	job['run_at'] = ARGV[3]
	redis.call('ZADD', KEYS[3], ARGV[4], ARGV[1])
end
redis.call('HSET', KEYS[1], ARGV[1], cjson.encode(job))
redis.call('ZREM', KEYS[2], ARGV[1])
return 1
`

// reclaimScript returns running jobs whose lease expired to pending (crash
// recovery), up to reclaimBatch per call. Stale index members without data
// or with a non-running status are cleaned up.
//
// KEYS: 1 running, 2 pending, 3 data
// ARGV: 1 now unix micros, 2 now RFC3339, 3 zero time
var reclaimScript = `
local ids = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0,` + fmt.Sprint(reclaimBatch) + `)
local n = 0
for _, id in ipairs(ids) do
	local raw = redis.call('HGET', KEYS[3], id)
	if raw then
		local job = cjson.decode(raw)
		if job['status'] == 'running' then
			job['status'] = 'pending'
			job['run_at'] = ARGV[2]
			job['lease_until'] = ARGV[3]
			job['updated_at'] = ARGV[2]
			redis.call('HSET', KEYS[3], id, cjson.encode(job))
			redis.call('ZREM', KEYS[1], id)
			redis.call('ZADD', KEYS[2], ARGV[1], id)
			n = n + 1
		else
			redis.call('ZREM', KEYS[1], id)
		end
	else
		redis.call('ZREM', KEYS[1], id)
	end
end
return n
`

// retryScript resurrects a dead job with a fresh attempt budget.
//
// KEYS: 1 data, 2 dead, 3 pending
// ARGV: 1 id, 2 now RFC3339, 3 now unix micros, 4 zero time
const retryScript = `
local raw = redis.call('HGET', KEYS[1], ARGV[1])
if not raw then return 0 end
local job = cjson.decode(raw)
if job['status'] ~= 'dead' then return 0 end
job['status'] = 'pending'
job['attempts'] = 0
job['run_at'] = ARGV[2]
job['updated_at'] = ARGV[2]
job['lease_until'] = ARGV[4]
redis.call('HSET', KEYS[1], ARGV[1], cjson.encode(job))
redis.call('ZREM', KEYS[2], ARGV[1])
redis.call('ZADD', KEYS[3], ARGV[3], ARGV[1])
return 1
`

// dropScript permanently removes a dead job.
//
// KEYS: 1 data, 2 dead
// ARGV: 1 id
const dropScript = `
local raw = redis.call('HGET', KEYS[1], ARGV[1])
if not raw then return 0 end
local job = cjson.decode(raw)
if job['status'] ~= 'dead' then return 0 end
redis.call('HDEL', KEYS[1], ARGV[1])
redis.call('ZREM', KEYS[2], ARGV[1])
return 1
`

// RedisStore is a Store backed by Redis over a redigo pool (the framework's
// existing REDIS_* wiring). Job state lives in:
//
//	<prefix>:pending    zset  member = job ID, score = RunAt (unix micros)
//	<prefix>:running    zset  member = job ID, score = lease expiry (unix micros)
//	<prefix>:completed  zset  member = job ID, score = completed-at (unix micros)
//	<prefix>:dead       zset  member = job ID, score = final-failure time (unix micros)
//	<prefix>:data       hash  job ID → full Job JSON
//	<prefix>:lock:<n>   key   scheduler fire-locks (SET NX EX)
//
// All conditional transitions (claim, complete, fail, reclaim, retry, drop)
// run as Lua scripts, so they are atomic and guarded by status checks.
// Scores are unix microseconds, which fit exactly in Redis's double
// precision zset scores; nanoseconds would silently lose precision.
//
// Commands are not context-aware: they are bounded by the pool's timeouts,
// matching how the cache package uses redigo. For Redis Cluster, put a hash
// tag in the prefix (e.g. "{regius}:jobs") so all keys hash to one slot.
type RedisStore struct {
	// Pool is the redigo connection pool.
	Pool *redis.Pool

	// Prefix namespaces all keys. Empty defaults to "regius:jobs".
	Prefix string

	// Now is the clock used by Retry. nil means time.Now. It exists for
	// deterministic tests.
	Now func() time.Time
}

// NewRedisStore creates a store over pool with the given key prefix.
func NewRedisStore(pool *redis.Pool, prefix string) *RedisStore {
	if prefix == "" {
		prefix = "regius:jobs"
	}
	return &RedisStore{Pool: pool, Prefix: prefix}
}

func (s *RedisStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *RedisStore) key(bucket string) string { return s.Prefix + ":" + bucket }

func (s *RedisStore) lockKey(name string) string { return s.Prefix + ":lock:" + name }

func (s *RedisStore) conn() redis.Conn {
	return s.Pool.Get()
}

// Enqueue stores the job's JSON snapshot and indexes it as pending in one
// transaction. The caller's Job is never retained (no aliasing).
func (s *RedisStore) Enqueue(_ context.Context, j *Job) error {
	raw, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("jobs: encode job %s: %w", j.ID, err)
	}
	conn := s.conn()
	defer conn.Close()
	if err := conn.Send("MULTI"); err != nil {
		return err
	}
	if err := conn.Send("ZADD", s.key("pending"), j.RunAt.UnixMicro(), j.ID); err != nil {
		return err
	}
	if err := conn.Send("HSET", s.key("data"), j.ID, raw); err != nil {
		return err
	}
	_, err = conn.Do("EXEC")
	return err
}

// Claim atomically claims the oldest ready pending job (see claimScript).
func (s *RedisStore) Claim(_ context.Context, now, leaseUntil time.Time) (*Job, error) {
	conn := s.conn()
	defer conn.Close()
	reply, err := redis.Bytes(conn.Do("EVAL", claimScript, 3,
		s.key("pending"), s.key("data"), s.key("running"),
		now.UnixMicro(), leaseUntil.Format(time.RFC3339Nano), leaseUntil.UnixMicro(), now.Format(time.RFC3339Nano),
	))
	if err == redis.ErrNil {
		return nil, ErrNoJob
	}
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(reply, &j); err != nil {
		return nil, fmt.Errorf("jobs: decode claimed job: %w", err)
	}
	return &j, nil
}

// Complete marks a running job completed (see completeScript).
func (s *RedisStore) Complete(_ context.Context, id string, now time.Time) error {
	conn := s.conn()
	defer conn.Close()
	n, err := redis.Int(conn.Do("EVAL", completeScript, 3,
		s.key("data"), s.key("running"), s.key("completed"),
		id, now.Format(time.RFC3339Nano), now.UnixMicro(), zeroTimeRFC3339,
	))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoJob
	}
	return nil
}

// Fail records a failed attempt on a running job: requeue or dead-letter
// (see failScript).
func (s *RedisStore) Fail(_ context.Context, j *Job, now, retryAt time.Time, lastErr string) error {
	conn := s.conn()
	defer conn.Close()
	retryAtStr := ""
	var retryScore int64
	if !retryAt.IsZero() {
		retryAtStr = retryAt.Format(time.RFC3339Nano)
		retryScore = retryAt.UnixMicro()
	}
	n, err := redis.Int(conn.Do("EVAL", failScript, 4,
		s.key("data"), s.key("running"), s.key("pending"), s.key("dead"),
		j.ID, now.Format(time.RFC3339Nano), retryAtStr, retryScore, now.UnixMicro(), lastErr, zeroTimeRFC3339,
	))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoJob
	}
	return nil
}

// ReclaimExpired returns expired running jobs to pending, at most
// reclaimBatch per call (see reclaimScript).
func (s *RedisStore) ReclaimExpired(_ context.Context, now time.Time) (int, error) {
	conn := s.conn()
	defer conn.Close()
	return redis.Int(conn.Do("EVAL", reclaimScript, 3,
		s.key("running"), s.key("pending"), s.key("data"),
		now.UnixMicro(), now.Format(time.RFC3339Nano), zeroTimeRFC3339,
	))
}

// TryLock acquires a best-effort exclusive lock with a TTL via SET NX EX,
// keeping schedulers on separate processes from double-firing.
func (s *RedisStore) TryLock(_ context.Context, name string, ttl time.Duration) (bool, error) {
	secs := int(ttl / time.Second)
	if secs < 1 {
		secs = 1
	}
	conn := s.conn()
	defer conn.Close()
	reply, err := redis.String(conn.Do("SET", s.lockKey(name), "1", "NX", "EX", secs))
	if err == redis.ErrNil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return reply == "OK", nil
}

// Stats returns job counts by status.
func (s *RedisStore) Stats(_ context.Context) (Stats, error) {
	conn := s.conn()
	defer conn.Close()
	for _, bucket := range []string{"pending", "running", "completed", "dead"} {
		if err := conn.Send("ZCARD", s.key(bucket)); err != nil {
			return Stats{}, err
		}
	}
	if err := conn.Flush(); err != nil {
		return Stats{}, err
	}
	var st Stats
	for _, count := range []*int{&st.Pending, &st.Running, &st.Completed, &st.Dead} {
		n, err := redis.Int(conn.Receive())
		if err != nil {
			return Stats{}, err
		}
		*count = n
	}
	return st, nil
}

// List returns jobs matching the filter: pending and running ordered by
// RunAt, then completed and dead newest first. Each status bucket is
// scanned up to listScanLimit members before the name filter and limit are
// applied.
func (s *RedisStore) List(_ context.Context, f ListFilter) ([]*Job, error) {
	conn := s.conn()
	defer conn.Close()

	buckets := []struct {
		name string
		desc bool
	}{
		{"pending", false},
		{"running", false},
		{"completed", true},
		{"dead", true},
	}

	var out []*Job
	for _, b := range buckets {
		if f.Status != "" && string(f.Status) != b.name {
			continue
		}
		ids, err := s.rangeIDs(conn, b.name, b.desc)
		if err != nil {
			return nil, err
		}
		jobs, err := s.jobsByIDs(conn, ids)
		if err != nil {
			return nil, err
		}
		if b.name == "running" {
			// the running zset is scored by lease expiry; sort by RunAt
			// for display, matching the other stores
			sort.Slice(jobs, func(i, k int) bool { return jobs[i].RunAt.Before(jobs[k].RunAt) })
		}
		for _, j := range jobs {
			if f.Name != "" && j.Name != f.Name {
				continue
			}
			out = append(out, j)
		}
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *RedisStore) rangeIDs(conn redis.Conn, bucket string, desc bool) ([]string, error) {
	cmd := "ZRANGE"
	if desc {
		cmd = "ZREVRANGE"
	}
	return redis.Strings(conn.Do(cmd, s.key(bucket), 0, listScanLimit-1))
}

func (s *RedisStore) jobsByIDs(conn redis.Conn, ids []string) ([]*Job, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, s.key("data"))
	for _, id := range ids {
		args = append(args, id)
	}
	raws, err := redis.ByteSlices(conn.Do("HMGET", args...))
	if err != nil {
		return nil, err
	}
	var jobs []*Job
	for _, raw := range raws {
		if raw == nil {
			continue // stale index member; the claim script self-heals these
		}
		var j Job
		if err := json.Unmarshal(raw, &j); err != nil {
			return nil, fmt.Errorf("jobs: decode job: %w", err)
		}
		jobs = append(jobs, &j)
	}
	return jobs, nil
}

// Get returns the job with the given ID regardless of status.
func (s *RedisStore) Get(_ context.Context, id string) (*Job, error) {
	conn := s.conn()
	defer conn.Close()
	raw, err := redis.Bytes(conn.Do("HGET", s.key("data"), id))
	if err == redis.ErrNil {
		return nil, ErrNoJob
	}
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, fmt.Errorf("jobs: decode job %s: %w", id, err)
	}
	return &j, nil
}

// Retry moves a dead job back to pending with its attempts reset (see
// retryScript).
func (s *RedisStore) Retry(_ context.Context, id string) error {
	conn := s.conn()
	defer conn.Close()
	now := s.now()
	n, err := redis.Int(conn.Do("EVAL", retryScript, 3,
		s.key("data"), s.key("dead"), s.key("pending"),
		id, now.Format(time.RFC3339Nano), now.UnixMicro(), zeroTimeRFC3339,
	))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoJob
	}
	return nil
}

// Drop permanently removes a dead job (see dropScript).
func (s *RedisStore) Drop(_ context.Context, id string) error {
	conn := s.conn()
	defer conn.Close()
	n, err := redis.Int(conn.Do("EVAL", dropScript, 2,
		s.key("data"), s.key("dead"),
		id,
	))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoJob
	}
	return nil
}

// Prune deletes completed jobs older than retention from both the completed
// index and the data hash. Dead jobs are never pruned automatically.
func (s *RedisStore) Prune(_ context.Context, now time.Time, retention time.Duration) (int, error) {
	cutoff := now.Add(-retention).UnixMicro()
	conn := s.conn()
	defer conn.Close()
	ids, err := redis.Strings(conn.Do("ZRANGEBYSCORE", s.key("completed"), "-inf", cutoff))
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := conn.Send("MULTI"); err != nil {
		return 0, err
	}
	hdelArgs := make([]interface{}, 0, len(ids)+1)
	hdelArgs = append(hdelArgs, s.key("data"))
	for _, id := range ids {
		hdelArgs = append(hdelArgs, id)
	}
	if err := conn.Send("HDEL", hdelArgs...); err != nil {
		return 0, err
	}
	if err := conn.Send("ZREMRANGEBYSCORE", s.key("completed"), "-inf", cutoff); err != nil {
		return 0, err
	}
	if _, err := conn.Do("EXEC"); err != nil {
		return 0, err
	}
	return len(ids), nil
}
