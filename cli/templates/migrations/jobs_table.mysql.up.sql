-- Background job queue tables for github.com/hbarral/regius/jobs (SQLStore,
-- mysql dialect). Matches jobs.Schema("mysql") — keep in sync.
CREATE TABLE regius_jobs (
  id VARCHAR(20) NOT NULL,
  name VARCHAR(255) NOT NULL,
  payload JSON NOT NULL,
  queue VARCHAR(100) NOT NULL DEFAULT 'default',
  status VARCHAR(16) NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  max_attempts INT NOT NULL DEFAULT 3,
  run_at DATETIME (6) NOT NULL,
  lease_until DATETIME (6) NULL,
  last_error TEXT NULL,
  created_at DATETIME (6) NOT NULL,
  updated_at DATETIME (6) NOT NULL,
  completed_at DATETIME (6) NULL,
  PRIMARY KEY (id),
  INDEX regius_jobs_claim_idx (status, run_at),
  INDEX regius_jobs_completed_idx (status, completed_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;


CREATE TABLE regius_locks (
  name VARCHAR(191) NOT NULL,
  expires_at DATETIME (6) NOT NULL,
  PRIMARY KEY (name)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;
