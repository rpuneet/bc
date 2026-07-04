package cron

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rpuneet/mycel/pkg/db"
	"github.com/rpuneet/mycel/pkg/log"
)

// Store is a cron job store backed by SQLite or TimescaleDB (Postgres).
type Store struct {
	db *db.DB
	pg *PostgresStore // non-nil when using Postgres via OpenStore
}

// Open opens the cron store on the given workspace database. The handle
// is borrowed: callers (typically the per-workspace db registry) own its
// lifecycle.
func Open(d *db.DB, driver string) (*Store, error) {
	if d == nil {
		return nil, fmt.Errorf("cron store requires a workspace database (nil handle)")
	}

	s := &Store{db: d}
	if driver == "timescale" {
		// Use PostgresStore for proper $1 placeholder queries.
		pg := NewPostgresStore(d.DB)
		if schemaErr := pg.InitSchema(); schemaErr != nil {
			return nil, fmt.Errorf("init cron schema on timescale: %w", schemaErr)
		}
		s.pg = pg
	} else {
		if err := s.initSchema(); err != nil {
			return nil, fmt.Errorf("init cron schema: %w", err)
		}
	}
	return s, nil
}

// Close is a no-op — the workspace DB is owned by the caller.
func (s *Store) Close() error {
	if s.pg != nil {
		return s.pg.Close()
	}
	return nil
}

func (s *Store) initSchema() error {
	const schema = `
CREATE TABLE IF NOT EXISTS cron_jobs (
    name        TEXT PRIMARY KEY,
    schedule    TEXT NOT NULL,
    agent_name  TEXT,
    prompt      TEXT,
    command     TEXT,
    enabled     INTEGER NOT NULL DEFAULT 1,
    last_run    DATETIME,
    next_run    DATETIME,
    run_count   INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cron_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    job_name    TEXT NOT NULL REFERENCES cron_jobs(name) ON DELETE CASCADE,
    status      TEXT NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    cost_usd    REAL NOT NULL DEFAULT 0,
    output      TEXT,
    run_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cron_logs_job ON cron_logs(job_name, run_at DESC);
`
	// context.TODO() retained: initSchema runs synchronously during cron.Open at
	// startup before any request context exists; threading ctx through would
	// force a public API change on Open() and dozens of call sites across
	// tests/services for no operational benefit (schema DDL is fire-and-forget).
	_, err := s.db.ExecContext(context.TODO(), schema)
	return err
}

// AddJob inserts a new cron job. Returns an error if the name already exists.
// Note: commands that kill the bcd process will terminate the cron scheduler itself.
// Use an external supervisor (systemd, launchd) for bcd restarts.
func (s *Store) AddJob(ctx context.Context, job *Job) error {
	if s.pg != nil {
		return s.pg.AddJob(ctx, job)
	}
	if strings.Contains(job.Command, "kill") && (strings.Contains(job.Command, "9374") || strings.Contains(job.Command, "bcd")) {
		log.Warn("cron job command may kill bcd (the cron host) — use an external supervisor for restarts", "job", job.Name)
	}
	nextRun, err := NextRun(job.Schedule, time.Now())
	if err != nil {
		return fmt.Errorf("compute next_run: %w", err)
	}

	const q = `
INSERT INTO cron_jobs (name, schedule, agent_name, prompt, command, enabled, next_run, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	enabled := 1
	if !job.Enabled {
		enabled = 0
	}

	_, err = s.db.ExecContext(ctx, q,
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
func (s *Store) GetJob(ctx context.Context, name string) (*Job, error) {
	if s.pg != nil {
		return s.pg.GetJob(ctx, name)
	}
	const q = `
SELECT name, schedule, agent_name, prompt, command, enabled, last_run, next_run, run_count, created_at
FROM cron_jobs WHERE name = ?`

	row := s.db.QueryRowContext(ctx, q, name)
	job, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return job, err
}

// ListJobs returns all cron jobs ordered by name.
func (s *Store) ListJobs(ctx context.Context) ([]*Job, error) {
	if s.pg != nil {
		return s.pg.ListJobs(ctx)
	}
	const q = `
SELECT name, schedule, agent_name, prompt, command, enabled, last_run, next_run, run_count, created_at
FROM cron_jobs ORDER BY name`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list cron jobs: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort

	var jobs []*Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// DeleteJob removes a cron job and its logs by name.
func (s *Store) DeleteJob(ctx context.Context, name string) error {
	if s.pg != nil {
		return s.pg.DeleteJob(ctx, name)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM cron_jobs WHERE name = ?`, name)
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
func (s *Store) SetEnabled(ctx context.Context, name string, enabled bool) error {
	if s.pg != nil {
		return s.pg.SetEnabled(ctx, name, enabled)
	}
	job, err := s.GetJob(ctx, name)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("cron job %q not found", name)
	}

	enabledInt := 0
	var nextRun *time.Time
	if enabled {
		enabledInt = 1
		t, calcErr := NextRun(job.Schedule, time.Now())
		if calcErr != nil {
			return calcErr
		}
		nextRun = &t
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE cron_jobs SET enabled = ?, next_run = ? WHERE name = ?`,
		enabledInt, nullTime(nextRun), name,
	)
	return err
}

// RecordRun records a job execution result and updates run stats.
func (s *Store) RecordRun(ctx context.Context, entry *LogEntry) error {
	if s.pg != nil {
		return s.pg.RecordRun(ctx, entry)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rolled back on error

	_, err = tx.ExecContext(ctx,
		`INSERT INTO cron_logs (job_name, status, duration_ms, cost_usd, output, run_at)
         VALUES (?, ?, ?, ?, ?, ?)`,
		entry.JobName, entry.Status, entry.DurationMS, entry.CostUSD,
		nullStr(entry.Output), entry.RunAt,
	)
	if err != nil {
		return fmt.Errorf("insert cron log: %w", err)
	}

	// Update job stats: recompute next_run using the schedule queried via the
	// same transaction to avoid a deadlock (single SQLite connection pool).
	now := time.Now()
	var nextRunPtr *time.Time
	var schedule string
	if scanErr := tx.QueryRowContext(ctx, `SELECT schedule FROM cron_jobs WHERE name = ?`, entry.JobName).Scan(&schedule); scanErr != nil {
		log.Warn("failed to query schedule for next_run", "job", entry.JobName, "error", scanErr)
	} else if t, calcErr := NextRun(schedule, now); calcErr != nil {
		log.Warn("failed to compute next_run", "job", entry.JobName, "schedule", schedule, "error", calcErr)
	} else {
		nextRunPtr = &t
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE cron_jobs
         SET last_run = ?, next_run = ?, run_count = run_count + 1
         WHERE name = ?`,
		now, nullTime(nextRunPtr), entry.JobName,
	)
	if err != nil {
		return fmt.Errorf("update cron job stats: %w", err)
	}

	return tx.Commit()
}

// RecordManualTrigger marks a job as manually triggered (updates last_run + next_run).
func (s *Store) RecordManualTrigger(ctx context.Context, name string) error {
	if s.pg != nil {
		return s.pg.RecordManualTrigger(ctx, name)
	}
	job, err := s.GetJob(ctx, name)
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

	_, err = s.db.ExecContext(ctx,
		`UPDATE cron_jobs SET last_run = ?, next_run = ?, run_count = run_count + 1 WHERE name = ?`,
		now, nullTime(nextRunPtr), name,
	)
	return err
}

// GetLogs returns execution history for a job. If last > 0, limits to that many entries.
func (s *Store) GetLogs(ctx context.Context, jobName string, last int) ([]*LogEntry, error) {
	if s.pg != nil {
		return s.pg.GetLogs(ctx, jobName, last)
	}
	// Use a parameterized LIMIT to avoid string-building in SQL queries.
	// SQLite accepts -1 as "no limit" in the LIMIT clause.
	limit := -1
	if last > 0 {
		limit = last
	}

	const q = `SELECT id, job_name, status, duration_ms, cost_usd, output, run_at
          FROM cron_logs WHERE job_name = ? ORDER BY run_at DESC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, q, jobName, limit)
	if err != nil {
		return nil, fmt.Errorf("query cron logs: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort

	var entries []*LogEntry
	for rows.Next() {
		e := &LogEntry{}
		var output sql.NullString
		if err := rows.Scan(&e.ID, &e.JobName, &e.Status, &e.DurationMS, &e.CostUSD, &output, &e.RunAt); err != nil {
			return nil, fmt.Errorf("scan cron log: %w", err)
		}
		if output.Valid {
			e.Output = output.String
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// scanner abstracts *sql.Row and *sql.Rows for scanJob.
type scanner interface {
	Scan(dest ...any) error
}

func scanJob(s scanner) (*Job, error) {
	j := &Job{}
	var (
		agentName sql.NullString
		prompt    sql.NullString
		command   sql.NullString
		lastRun   sql.NullTime
		nextRun   sql.NullTime
		enabled   int
	)
	err := s.Scan(
		&j.Name, &j.Schedule, &agentName, &prompt, &command,
		&enabled, &lastRun, &nextRun, &j.RunCount, &j.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("scan cron job: %w", err)
	}
	j.Enabled = enabled != 0
	if agentName.Valid {
		j.AgentName = agentName.String
	}
	if prompt.Valid {
		j.Prompt = prompt.String
	}
	if command.Valid {
		j.Command = command.String
	}
	if lastRun.Valid {
		t := lastRun.Time
		j.LastRun = &t
	}
	if nextRun.Valid {
		t := nextRun.Time
		j.NextRun = &t
	}
	return j, nil
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}
