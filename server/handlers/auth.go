package handlers

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// APIKeyAuth returns middleware that validates Bearer token authentication.
// If apiKey is empty, the middleware is a no-op (auth disabled).
// Exempt paths (health, SSE) skip authentication.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		// No API key configured — auth disabled (zero-config localhost)
		if apiKey == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Exempt paths: health check, SSE streams, MCP protocol
			path := r.URL.Path
			if path == "/health" || path == "/api/health" || path == "/healthz" || strings.HasPrefix(path, "/_mcp/") {
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header
			auth := r.Header.Get("Authorization")
			if auth == "" {
				// Also check X-API-Key header (alternative)
				auth = "Bearer " + r.Header.Get("X-API-Key")
			}

			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":"unauthorized: missing Bearer token"}`, http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(auth, "Bearer ")
			// Constant-time comparison to prevent timing attacks
			if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
				http.Error(w, `{"error":"unauthorized: invalid API key"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
