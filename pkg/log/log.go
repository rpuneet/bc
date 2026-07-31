// Package log provides structured logging for mycel using log/slog.
package log

import (
	"io"
	"log/slog"
	"os"
	"sync"
)

var (
	// mu protects logger, verbose, and format
	mu sync.RWMutex

	// logger is the global logger instance
	logger *slog.Logger

	// verbose controls whether debug-level logs are shown
	verbose bool

	// format is the current log format ("text" or "json")
	format string = "text"
)

func init() {
	// Default to info level, text output to stderr
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// SetVerbose enables or disables verbose (debug) logging.
func SetVerbose(v bool) {
	mu.Lock()
	defer mu.Unlock()
	verbose = v
	rebuildLogger(os.Stderr)
}

// SetFormat sets the log output format ("text" or "json").
// Default is "text". JSON format is useful for log aggregation tools.
func SetFormat(f string) {
	mu.Lock()
	defer mu.Unlock()
	if f == "json" || f == "text" {
		format = f
	}
	rebuildLogger(os.Stderr)
}

// SetOutput sets the output writer for the logger.
func SetOutput(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	rebuildLogger(w)
}

// rebuildLogger creates a new logger with current settings.
// Must be called with mu held.
func rebuildLogger(w io.Writer) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}
	logger = slog.New(handler)
}

// Debug logs a debug message. Only shown when verbose is enabled.
func Debug(msg string, args ...any) {
	mu.RLock()
	l := logger
	mu.RUnlock()
	l.Debug(msg, args...)
}

// Info logs an info message.
func Info(msg string, args ...any) {
	mu.RLock()
	l := logger
	mu.RUnlock()
	l.Info(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	mu.RLock()
	l := logger
	mu.RUnlock()
	l.Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...any) {
	mu.RLock()
	l := logger
	mu.RUnlock()
	l.Error(msg, args...)
}

// With returns a logger with the given attributes.
func With(args ...any) *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger.With(args...)
}

// Logger returns the underlying slog.Logger.
func Logger() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger
}
