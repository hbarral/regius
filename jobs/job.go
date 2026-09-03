package jobs

import (
	"encoding/json"
	"time"
)

// QueueDefault is the only queue served in v1. The Job.Queue field is
// reserved for future named/priority queues and is always QueueDefault.
const QueueDefault = "default"

// Status is the lifecycle state of a job.
type Status string

const (
	// StatusPending means the job is waiting for a worker. Delayed and
	// retrying jobs are pending with a future RunAt.
	StatusPending Status = "pending"

	// StatusRunning means a worker claimed the job and holds its lease.
	StatusRunning Status = "running"

	// StatusCompleted means the job finished successfully.
	StatusCompleted Status = "completed"

	// StatusDead means the job exhausted its attempts and awaits Retry or
	// Drop.
	StatusDead Status = "dead"
)

// Job is a unit of background work, tracked from enqueue through execution.
// Jobs are created by Manager.Enqueue.
type Job struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Payload     []byte    `json:"payload"`
	Queue       string    `json:"queue"`
	Status      Status    `json:"status"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"max_attempts"`
	RunAt       time.Time `json:"run_at"`
	LeaseUntil  time.Time `json:"lease_until"`
	LastError   string    `json:"last_error"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// Decode unmarshals the JSON payload into v.
func (j *Job) Decode(v any) error {
	return json.Unmarshal(j.Payload, v)
}

// copy returns a shallow copy so callers cannot mutate stored state.
func (j *Job) copy() *Job {
	c := *j
	return &c
}
