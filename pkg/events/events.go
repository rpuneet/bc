// Package events provides an append-only event log for mycel.
//
// Events are stored in the events table of the single global database
// (see SQLiteLog); JSONLWriter additionally mirrors SSE events to a
// per-workspace events.jsonl. This provides an audit trail for agent
// spawns, stops, work assignments, status reports, and messages.
package events

import (
	"time"
)

// EventType identifies what happened.
type EventType string

const (
	AgentSpawned    EventType = "agent.spawned"
	AgentStopped    EventType = "agent.stopped"
	AgentReport     EventType = "agent.report"
	WorkAssigned    EventType = "work.assigned"
	WorkStarted     EventType = "work.started"
	WorkCompleted   EventType = "work.completed"
	WorkFailed      EventType = "work.failed"
	MessageSent     EventType = "message.sent"
	HealthCheck     EventType = "health.check"
	HealthFailed    EventType = "health.failed"
	HealthRecovered EventType = "health.recovered"
)

const (
	// DefaultMaxFileSize is the size threshold (in bytes) that triggers rotation.
	DefaultMaxFileSize int64 = 10 * 1024 * 1024 // 10 MB
	// DefaultMaxRotatedFiles is the number of rotated files to keep.
	DefaultMaxRotatedFiles = 5
	// DefaultReadLimit caps the number of events returned by Read and ReadByAgent
	// to prevent unbounded memory usage. Matches the SQLite store limit.
	DefaultReadLimit = 1000
	// MaxReadLastLimit caps the value of n in ReadLast to prevent abuse.
	MaxReadLastLimit = 10000
)

// Event is a single log entry.
type Event struct {
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"ts"`
	Type      EventType      `json:"type"`
	Agent     string         `json:"agent,omitempty"`
	Message   string         `json:"message,omitempty"`
	// Repo is the absolute path of the repo the event belongs to, used
	// for per-repo filtering in the single global database. Writers that
	// know the repo set it; otherwise the SQLite store best-effort
	// resolves it from the agents table (events are agent-keyed and both
	// tables live in the same mycel.db).
	Repo string `json:"repo,omitempty"`
	// ID is the store row id (SQLite/Postgres autoincrement). It is zero for
	// events that have not been persisted (e.g. freshly constructed events)
	// and is populated on read. It serves as the opaque cursor for the
	// before=<id> pagination on the activity/log endpoints.
	ID int64 `json:"id,omitempty"`
}

// EventStore is the interface for reading and writing events.
// Implemented by SQLiteLog (and its Postgres variant).
type EventStore interface {
	Append(event Event) error
	Read() ([]Event, error)
	ReadLast(n int) ([]Event, error)
	ReadByAgent(name string) ([]Event, error)
	// ReadByAgentPage returns the newest `limit` events for an agent, newest
	// first. When beforeID > 0, only events with id < beforeID are returned,
	// giving cursor pagination for older pages backed by the primary key.
	// Unlike ReadByAgent it pushes the limit into the query, so a small page
	// never materializes the full per-agent window.
	ReadByAgentPage(name string, limit int, beforeID int64) ([]Event, error)
	Close() error
}
