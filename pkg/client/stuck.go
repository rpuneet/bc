package client

import (
	"strconv"
	"time"
)

// StuckReason describes why an agent is considered stuck.
type StuckReason string

const (
	// StuckNoActivity indicates no events in the timeout period.
	StuckNoActivity StuckReason = "no_activity"
	// StuckRepeatedFailures indicates the same task has failed multiple times.
	StuckRepeatedFailures StuckReason = "repeated_failures"
	// StuckWorkTimeout indicates work started but not completed within timeout.
	StuckWorkTimeout StuckReason = "work_timeout"
)

// Event type constants matching pkg/events.
const (
	eventWorkStarted   = "work.started"
	eventWorkCompleted = "work.completed"
	eventWorkFailed    = "work.failed"
)

// StuckDetection contains the result of analyzing an agent for stuck conditions.
//
//nolint:govet // JSON field order is more important than memory layout
type StuckDetection struct {
	LastEventTime time.Time     `json:"last_event_time,omitempty"`
	AgentName     string        `json:"agent_name"`
	Reason        StuckReason   `json:"reason,omitempty"`
	Details       string        `json:"details,omitempty"`
	IdleDuration  time.Duration `json:"idle_duration,omitempty"`
	FailureCount  int           `json:"failure_count,omitempty"`
	IsStuck       bool          `json:"is_stuck"`
}

// StuckConfig configures stuck detection thresholds.
type StuckConfig struct {
	// ActivityTimeout is how long without events before considering stuck.
	ActivityTimeout time.Duration
	// WorkTimeout is how long a task can run before considered stuck.
	WorkTimeout time.Duration
	// MaxFailures is the number of consecutive failures before considered stuck.
	MaxFailures int
}

// DetectStuck analyzes events to determine if an agent is stuck.
// This mirrors pkg/events.DetectStuck but operates on client EventInfo types,
// avoiding the need for CLI commands to import pkg/events directly.
func DetectStuck(events []EventInfo, config StuckConfig) StuckDetection {
	if len(events) == 0 {
		return StuckDetection{
			IsStuck: false,
		}
	}

	detection := StuckDetection{
		AgentName: events[0].Agent,
	}

	// Find the most recent event
	var lastEvent EventInfo
	for _, ev := range events {
		if ev.Timestamp.After(lastEvent.Timestamp) {
			lastEvent = ev
		}
	}
	detection.LastEventTime = lastEvent.Timestamp
	detection.IdleDuration = time.Since(lastEvent.Timestamp)

	// Check 1: No activity in timeout period
	if detection.IdleDuration > config.ActivityTimeout {
		detection.IsStuck = true
		detection.Reason = StuckNoActivity
		detection.Details = "no events in " + detection.IdleDuration.Round(time.Second).String()
		return detection
	}

	// Check 2: Repeated failures on same task
	failureCount := countRecentFailures(events, config.ActivityTimeout)
	detection.FailureCount = failureCount
	if failureCount >= config.MaxFailures {
		detection.IsStuck = true
		detection.Reason = StuckRepeatedFailures
		detection.Details = "task failed " + strconv.Itoa(failureCount) + " times"
		return detection
	}

	// Check 3: Work started but not completed within timeout
	if workTimedOut := checkWorkTimeout(events, config.WorkTimeout); workTimedOut != "" {
		detection.IsStuck = true
		detection.Reason = StuckWorkTimeout
		detection.Details = "work '" + workTimedOut + "' exceeds timeout"
		return detection
	}

	return detection
}

// countRecentFailures counts consecutive WorkFailed events in the recent window.
func countRecentFailures(events []EventInfo, window time.Duration) int {
	cutoff := time.Now().Add(-window)
	count := 0
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Timestamp.Before(cutoff) {
			break
		}
		if ev.Type == eventWorkFailed {
			count++
		} else if ev.Type == eventWorkCompleted {
			break
		}
	}
	return count
}

// checkWorkTimeout checks if any work has been running longer than the timeout.
func checkWorkTimeout(events []EventInfo, timeout time.Duration) string {
	startedWork := make(map[string]time.Time)

	for _, ev := range events {
		switch ev.Type {
		case eventWorkStarted:
			task := ""
			if ev.Message != "" {
				task = ev.Message
			} else if t, ok := ev.Data["task"].(string); ok {
				task = t
			}
			if task != "" {
				startedWork[task] = ev.Timestamp
			}
		case eventWorkCompleted, eventWorkFailed:
			task := ""
			if ev.Message != "" {
				task = ev.Message
			} else if t, ok := ev.Data["task"].(string); ok {
				task = t
			}
			delete(startedWork, task)
		}
	}

	now := time.Now()
	for task, startTime := range startedWork {
		if now.Sub(startTime) > timeout {
			return task
		}
	}

	return ""
}
