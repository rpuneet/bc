// Package cron provides SQLite-backed scheduled task management for bc.
//
// Cron jobs trigger agent prompts or shell commands on a 5-field cron schedule.
// The scheduler itself runs inside bcd; this package provides the storage layer
// and cron expression utilities used by both the CLI and the daemon.
//
// # Usage
//
//	wsDB, driver, err := db.ForWorkspace("/path/to/workspace", nil)
//	if err != nil {
//	    return err
//	}
//	store, err := cron.Open(wsDB, driver)
//	if err != nil {
//	    return err
//	}
//	defer store.Close()
//
//	err = store.AddJob(ctx, &cron.Job{
//	    Name:      "daily-lint",
//	    Schedule:  "0 9 * * *",
//	    AgentName: "qa-01",
//	    Prompt:    "Run make lint and report results",
//	    Enabled:   true,
//	})
package cron

import (
	"time"
)

// Job represents a scheduled cron task.
type Job struct {
	LastRun   *time.Time `json:"last_run,omitempty"`
	NextRun   *time.Time `json:"next_run,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	Name      string     `json:"name"`
	Schedule  string     `json:"schedule"`
	AgentName string     `json:"agent_name,omitempty"`
	Prompt    string     `json:"prompt,omitempty"`
	Command   string     `json:"command,omitempty"`
	RunCount  int        `json:"run_count"`
	Enabled   bool       `json:"enabled"`
	Running   bool       `json:"running"`
}

// LogEntry records one execution of a cron job.
type LogEntry struct {
	RunAt      time.Time `json:"run_at"`
	JobName    string    `json:"job_name"`
	Status     string    `json:"status"` // success, failed, timeout
	Output     string    `json:"output,omitempty"`
	ID         int64     `json:"id"`
	DurationMS int64     `json:"duration_ms"`
	CostUSD    float64   `json:"cost_usd,omitempty"`
}
