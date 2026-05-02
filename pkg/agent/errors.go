package agent

import "errors"

var (
	ErrNotFound       = errors.New("agent not found")
	ErrAlreadyRunning = errors.New("agent already running")
	ErrNotRunning     = errors.New("agent not running")
	ErrInvalidState   = errors.New("invalid agent state for this operation")
)
