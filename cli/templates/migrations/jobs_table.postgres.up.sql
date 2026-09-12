-- Background job queue tables for github.com/hbarral/regius/jobs (SQLStore,
-- postgres dialect). Matches jobs.Schema("postgres") — keep in sync.
CREATE TABLE regius_jobs (
  id VARCHAR(20) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  payload JSONB NOT NULL,
  queue VARCHAR(100) NOT NULL DEFAULT 'default',
  status VARCHAR(16) NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  max_attempts INT NOT NULL DEFAULT 3,
  run_at TIMESTAMPTZ NOT NULL,
  lease_until TIMESTAMPTZ NULL,
  last_error TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ NULL
);


CREATE INDEX regius_jobs_claim_idx ON regius_jobs (status, run_at);


CREATE INDEX regius_jobs_completed_idx ON regius_jobs (status, completed_at);


CREATE TABLE regius_locks (
  name VARCHAR(191) PRIMARY KEY,
  expires_at TIMESTAMPTZ NOT NULL
);
