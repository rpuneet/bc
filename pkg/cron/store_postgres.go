package cron

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// PostgresStore provides Postgres-backed cron job storage.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a PostgresStore from an existing *sql.DB connection.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// InitSchema creates the cron tables in Postgres if they don't exist.
func (p *PostgresStore) InitSchema() error {
	ctx := context.Background()

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS cron_jobs (
			name        TEXT PRIMARY KEY,
			schedule    TEXT NOT NULL,
			agent_name  TEXT,
			prompt      TEXT,
			command     TEXT,
			enabled     BOOLEAN NOT NULL DEFAULT TRUE,
			last_run    TIMESTAMPTZ,
			next_run    TIMESTAMPTZ,
			run_count   INTEGER NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS cron_logs (
			id          BIGSERIAL PRIMARY KEY,
			job_name    TEXT NOT NULL REFERENCES cron_jobs(name) ON DELETE CASCADE,
			status      TEXT NOT NULL,
			duration_ms BIGINT NOT NULL DEFAULT 0,
			cost_usd    DOUBLE PRECISION NOT NULL DEFAULT 0,
			output      TEXT,
			run_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cron_logs_job ON cron_logs(job_name, run_at DESC)`,
	}

	for _, stmt := range stmts {
		if _, err := p.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("postgres cron schema: %w", err)
		}
	}
	return nil
}

// Close is a no-op — the shared DB is owned by the caller.
func (p *PostgresStore) Close() error {
	return nil
}

// AddJob inserts a new cron job.
func (p *PostgresStore) AddJob(ctx context.Context, job *Job) error {
	if strings.Contains(job.Command, "kill") && (strings.Contains(job.Command, "9374") || strings.Contains(job.Command, "bcd")) {
		log.Warn("cron job command may kill bcd (the cron host) — use an external supervisor for restarts", "job", job.Name)
	}
	nextRun, err := NextRun(job.Schedule, time.Now())
	if err != nil {
		return fmt.Errorf("compute next_run: %w", err)
	}

	enabled := 0
	if job.Enabled {
		enabled = 1
	}
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO cron_jobs (name, schedule, agent_name, prompt, command, enabled, next_run, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		job.Name, job.Schedule,
		nullStr(job.AgentName), nullStr(job.Prompt), nullStr(job.Command),
		enabled, nextRun, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("insert cron job: %w", err)
	}
	return nil
}

// GetJob returns a job by name. Returns nil, nil if not found.
func (p *PostgresStore) GetJob(ctx context.Context, name string) (*Job, error) {
	row := p.db.QueryRowContext(ctx,
		`SELECT name, schedule, agent_name, prompt, command, enabled, last_run, next_run, run_count, created_at
		 FROM cron_jobs WHERE name = $1`, name)
	job, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return job, err
}

// ListJobs returns all cron jobs ordered by name.
func (p *PostgresStore) ListJobs(ctx context.Context) ([]*Job, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT name, schedule, agent_name, prompt, command, enabled, last_run, next_run, run_count, created_at
		 FROM cron_jobs ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list cron jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []*Job
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// DeleteJob removes a cron job and its logs by name.
func (p *PostgresStore) DeleteJob(ctx context.Context, name string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM cron_jobs WHERE name = $1`, name)
	if err != nil {
		return fmt.Errorf("delete cron job: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cron job %q not found", name)
	}
	return nil
}

// SetEnabled enables or disables a job. Recomputes next_run when enabling.
func (p *PostgresStore) SetEnabled(ctx context.Context, name string, enabled bool) error {
	job, err := p.GetJob(ctx, name)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("cron job %q not found", name)
	}

	var nextRun *time.Time
	if enabled {
		t, calcErr := NextRun(job.Schedule, time.Now())
		if calcErr != nil {
			return calcErr
		}
		nextRun = &t
	}

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	_, err = p.db.ExecContext(ctx,
		`UPDATE cron_jobs SET enabled = $1, next_run = $2 WHERE name = $3`,
		enabledInt, nullTime(nextRun), name,
	)
	return err
}

// RecordRun records a job execution result and updates run stats.
func (p *PostgresStore) RecordRun(ctx context.Context, entry *LogEntry) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // rolled back on error

	_, err = tx.ExecContext(ctx,
		`INSERT INTO cron_logs (job_name, status, duration_ms, cost_usd, output, run_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		entry.JobName, entry.Status, entry.DurationMS, entry.CostUSD,
		nullStr(entry.Output), entry.RunAt,
	)
	if err != nil {
		return fmt.Errorf("insert cron log: %w", err)
	}

	now := time.Now()
	var nextRunPtr *time.Time
	var schedule string
	if scanErr := tx.QueryRowContext(ctx, `SELECT schedule FROM cron_jobs WHERE name = $1`, entry.JobName).Scan(&schedule); scanErr != nil {
		log.Warn("failed to query schedule for next_run", "job", entry.JobName, "error", scanErr)
	} else if t, calcErr := NextRun(schedule, now); calcErr != nil {
		log.Warn("failed to compute next_run", "job", entry.JobName, "schedule", schedule, "error", calcErr)
	} else {
		nextRunPtr = &t
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE cron_jobs
		 SET last_run = $1, next_run = $2, run_count = run_count + 1
		 WHERE name = $3`,
		now, nullTime(nextRunPtr), entry.JobName,
	)
	if err != nil {
		return fmt.Errorf("update cron job stats: %w", err)
	}

	return tx.Commit()
}

// RecordManualTrigger marks a job as manually triggered.
func (p *PostgresStore) RecordManualTrigger(ctx context.Context, name string) error {
	job, err := p.GetJob(ctx, name)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("cron job %q not found", name)
	}

	now := time.Now()
	var nextRunPtr *time.Time
	if t, calcErr := NextRun(job.Schedule, now); calcErr != nil {
		log.Warn("failed to compute next_run", "job", name, "schedule", job.Schedule, "error", calcErr)
	} else {
		nextRunPtr = &t
	}

	_, err = p.db.ExecContext(ctx,
		`UPDATE cron_jobs SET last_run = $1, next_run = $2, run_count = run_count + 1 WHERE name = $3`,
		now, nullTime(nextRunPtr), name,
	)
	return err
}

// GetLogs returns execution history for a job.
func (p *PostgresStore) GetLogs(ctx context.Context, jobName string, last int) ([]*LogEntry, error) {
	var rows *sql.Rows
	var err error

	if last > 0 {
		rows, err = p.db.QueryContext(ctx,
			`SELECT id, job_name, status, duration_ms, cost_usd, output, run_at
			 FROM cron_logs WHERE job_name = $1 ORDER BY run_at DESC LIMIT $2`,
			jobName, last)
	} else {
		rows, err = p.db.QueryContext(ctx,
			`SELECT id, job_name, status, duration_ms, cost_usd, output, run_at
			 FROM cron_logs WHERE job_name = $1 ORDER BY run_at DESC`,
			jobName)
	}
	if err != nil {
		return nil, fmt.Errorf("query cron logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []*LogEntry
	for rows.Next() {
		e := &LogEntry{}
		var output sql.NullString
		if scanErr := rows.Scan(&e.ID, &e.JobName, &e.Status, &e.DurationMS, &e.CostUSD, &output, &e.RunAt); scanErr != nil {
			return nil, fmt.Errorf("scan cron log: %w", scanErr)
		}
		if output.Valid {
			e.Output = output.String
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// scanJob, nullStr, nullTime reuse the shared helpers from store.go.
