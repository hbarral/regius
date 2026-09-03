-- Background job queue tables for github.com/hbarral/regius/jobs (SQLStore,
-- sqlite dialect). Matches jobs.Schema("sqlite") — keep in sync.
CREATE TABLE regius_jobs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  payload TEXT NOT NULL,
  queue TEXT NOT NULL DEFAULT 'default',
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 3,
  run_at timestamp NOT NULL,
  lease_until timestamp NULL,
  last_error TEXT NULL,
  created_at timestamp NOT NULL,
  updated_at timestamp NOT NULL,
  completed_at timestamp NULL
);


CREATE INDEX regius_jobs_claim_idx ON regius_jobs (status, run_at);


CREATE INDEX regius_jobs_completed_idx ON regius_jobs (status, completed_at);


CREATE TABLE regius_locks (
  name TEXT PRIMARY KEY,
  expires_at timestamp NOT NULL
);
