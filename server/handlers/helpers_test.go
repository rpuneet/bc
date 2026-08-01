package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClampInt(t *testing.T) {
	tests := []struct {
		name        string
		n, min, max int
		want        int
	}{
		{"within range", 5, 1, 10, 5},
		{"below min", -1, 0, 10, 0},
		{"above max", 20, 0, 10, 10},
		{"at min", 1, 1, 10, 1},
		{"at max", 10, 1, 10, 10},
		{"equal min max", 5, 5, 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampInt(tt.n, tt.min, tt.max)
			if got != tt.want {
				t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.n, tt.min, tt.max, got, tt.want)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		defaultLimit int
		wantLimit    int
		wantOffset   int
	}{
		{"defaults", "", 50, 50, 0},
		{"custom limit", "limit=10", 50, 10, 0},
		{"custom offset", "offset=20", 50, 50, 20},
		{"both", "limit=25&offset=5", 50, 25, 5},
		{"limit clamped to 1000", "limit=5000", 50, 1000, 0},
		{"limit clamped to 1", "limit=0", 50, 50, 0},
		{"negative limit ignored", "limit=-5", 50, 50, 0},
		{"negative offset ignored", "offset=-3", 50, 50, 0},
		{"non-numeric limit ignored", "limit=abc", 50, 50, 0},
		{"non-numeric offset ignored", "offset=xyz", 50, 50, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			limit, offset := parsePagination(req, tt.defaultLimit)
			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
			if offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", offset, tt.wantOffset)
			}
		})
	}
}

func TestRequireMethod(t *testing.T) {
	t.Run("allowed method", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		ok := requireMethod(w, r, http.MethodGet, http.MethodPost)
		if !ok {
			t.Error("expected true for allowed method")
		}
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("disallowed method", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/", nil)
		ok := requireMethod(w, r, http.MethodGet, http.MethodPost)
		if ok {
			t.Error("expected false for disallowed method")
		}
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", w.Code)
		}
	})
}

func TestHttpError(t *testing.T) {
	w := httptest.NewRecorder()
	httpError(w, "test error", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "test error") {
		t.Errorf("expected error message in body, got %q", body)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"key": "value"})

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"key":"value"`) {
		t.Errorf("expected JSON in body, got %q", body)
	}
}

func TestMaxBodySizeMiddleware(t *testing.T) {
	handler := MaxBodySize(100)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			httpError(w, "read failed", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"size": len(body)})
	}))

	t.Run("small body passes", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
		r.ContentLength = 5
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("large content-length rejected", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		r.ContentLength = 200
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413, got %d", w.Code)
		}
	})
}

func TestRecoveryMiddleware(t *testing.T) {
	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "internal server error") {
		t.Errorf("expected error in body, got %q", body)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("generates ID when absent", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)
		id := w.Header().Get("X-Request-ID")
		if id == "" {
			t.Error("expected X-Request-ID to be set")
		}
		if len(id) != 16 { // 8 bytes hex-encoded
			t.Errorf("expected 16-char hex ID, got %q (len %d)", id, len(id))
		}
	})

	t.Run("echoes provided ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("X-Request-ID", "my-custom-id")
		handler.ServeHTTP(w, r)
		if got := w.Header().Get("X-Request-ID"); got != "my-custom-id" {
			t.Errorf("expected my-custom-id, got %q", got)
		}
	})
}

func TestIsSSERequest(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		accept string
		want   bool
	}{
		{"events endpoint", "/api/events", "", true},
		{"mcp sse", "/_mcp/sse", "", true},
		{"mcp message", "/_mcp/message", "", true},
		{"agent output", "/api/agents/alice/output", "", true},
		{"accept event-stream", "/api/anything", "text/event-stream", true},
		{"deps install stream", "/api/deps/install", "", true},
		{"normal API", "/api/agents", "", false},
		{"health", "/health", "", false},
		{"agent non-output", "/api/agents/alice/stats", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.accept != "" {
				r.Header.Set("Accept", tt.accept)
			}
			got := isSSERequest(r)
			if got != tt.want {
				t.Errorf("isSSERequest(%s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCORSWithOrigin(t *testing.T) {
	handler := CORSWithOrigin("http://localhost:3000", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("sets origin header", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
			t.Errorf("expected origin http://localhost:3000, got %q", got)
		}
	})

	t.Run("preflight returns 204", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodOptions, "/", nil)
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}
	})
}
