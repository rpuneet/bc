package handlers

import (
	"net/http"
	"time"

	"github.com/rpuneet/mycel/pkg/log"
)

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// RequestLogger returns middleware that logs every HTTP request.
// SSE and MCP long-lived connections are excluded to avoid log spam.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip SSE, WebSocket, and other streaming endpoints.
		// statusRecorder wraps ResponseWriter, breaking http.Flusher
		// (needed by SSE) and http.Hijacker (needed by WebSocket).
		// Reuse isSSERequest() to stay in sync with Gzip bypass.
		if isSSERequest(r) || isWebSocketRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)

		if rec.status >= 400 {
			log.Warn("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", duration.Milliseconds(),
			)
		} else {
			log.Debug("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", duration.Milliseconds(),
			)
		}
	})
}
