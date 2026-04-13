// Package handlers implements HTTP handlers for the bcd REST API.
// Each handler file covers one resource (agents, channels, workspace, etc.).
package handlers

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/rpuneet/bc/pkg/log"
)

// writeJSON encodes v as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Debug("json encode failed", "error", err)
	}
}

// httpError writes a JSON error response.
func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck // best-effort
}

// httpInternalError logs the detailed error server-side and returns a generic
// "internal server error" message to the client, preventing leakage of internal
// paths, database details, or stack traces.
func httpInternalError(w http.ResponseWriter, context string, err error) {
	log.Error(context, "error", err)
	httpError(w, "internal server error", http.StatusInternalServerError)
}

// methodNotAllowed writes a 405 response.
func methodNotAllowed(w http.ResponseWriter) {
	httpError(w, "method not allowed", http.StatusMethodNotAllowed)
}

// requireMethod returns false and writes 405 if the request method is not one of the allowed methods.
func requireMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if r.Method == m {
			return true
		}
	}
	methodNotAllowed(w)
	return false
}

// Recovery returns a middleware that recovers from panics, logs the error,
// and returns a 500 JSON response instead of crashing the server.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Error("panic recovered", "error", err, "method", r.Method, "path", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"}) //nolint:errcheck
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestID returns a middleware that generates a unique request ID for each request.
// If the incoming request has an X-Request-ID header, it is reused.
// The ID is set on the response header and available via the request context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = generateRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) //nolint:errcheck // crypto/rand never fails on supported platforms
	return hex.EncodeToString(b)
}

// parsePagination extracts limit and offset from query params with defaults and clamping.
func parsePagination(r *http.Request, defaultLimit int) (limit, offset int) {
	limit = defaultLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = clampInt(n, 1, 1000)
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}

// clampInt clamps n to the range [min, max].
func clampInt(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// MaxBodySize returns a middleware that limits request body size.
// Returns 413 Payload Too Large if the body exceeds maxBytes.
//
// MCP endpoints (/_mcp/*) are excluded because MCP tools like send_file
// legitimately handle large payloads (up to 50MB) and enforce their own
// per-tool size limits in server/mcp/tools.go.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/_mcp/") {
				next.ServeHTTP(w, r)
				return
			}
			if r.ContentLength > maxBytes {
				httpError(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// isSSERequest returns true if the request targets a Server-Sent Events or
// other streaming endpoint that must not be buffered/compressed. Checked by
// path (known SSE routes) and by Accept header (generic text/event-stream).
func isSSERequest(r *http.Request) bool {
	// Known SSE/streaming paths
	if r.URL.Path == "/api/events" || strings.HasPrefix(r.URL.Path, "/_mcp/") {
		return true
	}
	// /api/agents/{name}/output is an SSE stream
	if strings.HasPrefix(r.URL.Path, "/api/agents/") && strings.HasSuffix(r.URL.Path, "/output") {
		return true
	}
	// /api/cron/{name}/logs/live is an SSE stream
	if strings.HasPrefix(r.URL.Path, "/api/cron/") && strings.HasSuffix(r.URL.Path, "/logs/live") {
		return true
	}
	// Generic: any request explicitly asking for event-stream
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		return true
	}
	return false
}

// Gzip returns a middleware that compresses responses with gzip when the
// client sends Accept-Encoding: gzip. Skips SSE and MCP streaming endpoints
// because gzip buffering breaks chunked encoding required by EventSource.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		if isSSERequest(r) || isWebSocketRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		gz, err := gzip.NewWriterLevel(w, gzip.DefaultCompression)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		defer func() { _ = gz.Close() }()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, Writer: gz}, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	io.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// CORSWithOrigin returns a middleware that adds CORS headers with the specified
// allowed origin. Use "*" for permissive (safe on loopback) or a specific
// origin like "http://localhost:9374" when exposed beyond loopback.
func CORSWithOrigin(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CORS returns a middleware with permissive CORS headers (Allow-Origin: *).
// Safe because bcd only binds to loopback by default.
func CORS(next http.Handler) http.Handler {
	return CORSWithOrigin("*", next)
}
