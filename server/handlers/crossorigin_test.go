package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRejectCrossOriginMutations covers the decision table directly, because
// every row is a policy choice rather than an implementation detail: the false
// rows are the vulnerability and the true rows are the clients that must keep
// working.
func TestRejectCrossOriginMutations(t *testing.T) {
	const daemonHost = "127.0.0.1:9374"

	tests := []struct {
		name          string
		method        string
		host          string
		origin        string
		secFetchSite  string
		allowedOrigin string
		wantAllowed   bool
	}{
		{
			name:        "attack: mutation from an unrelated website",
			method:      http.MethodPost,
			host:        daemonHost,
			origin:      "https://evil.example.com",
			wantAllowed: false,
		},
		{
			name:         "attack: sec-fetch-site betrays a browser that sent no origin",
			method:       http.MethodPost,
			host:         daemonHost,
			secFetchSite: "cross-site",
			wantAllowed:  false,
		},
		{
			name:        "attack: opaque origin from a sandboxed frame",
			method:      http.MethodPost,
			host:        daemonHost,
			origin:      "null",
			wantAllowed: false,
		},
		{
			name:        "attack: hostname merely containing localhost",
			method:      http.MethodPost,
			host:        daemonHost,
			origin:      "http://localhost.evil.example.com",
			wantAllowed: false,
		},
		{
			name:        "attack: delete from an unrelated website",
			method:      http.MethodDelete,
			host:        daemonHost,
			origin:      "https://evil.example.com",
			wantAllowed: false,
		},
		{
			name:        "web UI: same origin as the daemon",
			method:      http.MethodPost,
			host:        daemonHost,
			origin:      "http://127.0.0.1:9374",
			wantAllowed: true,
		},
		{
			name:        "CLI: no origin, no sec-fetch-site",
			method:      http.MethodPost,
			host:        daemonHost,
			wantAllowed: true,
		},
		{
			name:        "dev server: browser origin forwarded through the vite proxy",
			method:      http.MethodPost,
			host:        "localhost:9375",
			origin:      "http://localhost:9374",
			wantAllowed: true,
		},
		{
			name:        "desktop shell: another loopback origin on this machine",
			method:      http.MethodPost,
			host:        daemonHost,
			origin:      "http://localhost:9374",
			wantAllowed: true,
		},
		{
			name:          "hosted UI: the explicitly configured origin may write",
			method:        http.MethodPost,
			host:          "mycel.internal",
			origin:        "https://ui.example.com",
			allowedOrigin: "https://ui.example.com",
			wantAllowed:   true,
		},
		{
			name:          "wildcard does not confer write permission",
			method:        http.MethodPost,
			host:          "mycel.internal",
			origin:        "https://evil.example.com",
			allowedOrigin: "*",
			wantAllowed:   false,
		},
		{
			name:        "reads are left to CORS: GET from anywhere passes through",
			method:      http.MethodGet,
			host:        daemonHost,
			origin:      "https://evil.example.com",
			wantAllowed: true,
		},
		{
			name:        "preflight is not a mutation",
			method:      http.MethodOptions,
			host:        daemonHost,
			origin:      "https://evil.example.com",
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			h := RejectCrossOriginMutations(tt.allowedOrigin, http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					reached = true
					w.WriteHeader(http.StatusOK)
				}))

			req := httptest.NewRequest(tt.method, "http://"+tt.host+"/api/tools", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.secFetchSite)
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if reached != tt.wantAllowed {
				t.Errorf("handler reached = %v, want %v (status %d)", reached, tt.wantAllowed, rec.Code)
			}
			if !tt.wantAllowed && rec.Code != http.StatusForbidden {
				t.Errorf("rejected request answered %d, want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

// TestRejectCrossOriginMutationsCoversEveryWriteMethod guards against a method
// being overlooked: anything that is not a documented safe method has to be
// gated, so adding PATCH support to a route cannot silently reopen this.
func TestRejectCrossOriginMutationsCoversEveryWriteMethod(t *testing.T) {
	for _, m := range []string{
		http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodTrace,
	} {
		t.Run(m, func(t *testing.T) {
			reached := false
			h := RejectCrossOriginMutations("*", http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { reached = true }))

			req := httptest.NewRequest(m, "http://127.0.0.1:9374/api/tools", nil)
			req.Host = "127.0.0.1:9374"
			req.Header.Set("Origin", "https://evil.example.com")

			h.ServeHTTP(httptest.NewRecorder(), req)
			if reached {
				t.Errorf("%s from a foreign origin reached the handler", m)
			}
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	loopback := []string{
		"localhost", "localhost:9374", "LocalHost:9374",
		"127.0.0.1", "127.0.0.1:8080", "127.0.0.53",
		"[::1]", "[::1]:9374",
	}
	for _, h := range loopback {
		if !isLoopbackHost(h) {
			t.Errorf("isLoopbackHost(%q) = false, want true", h)
		}
	}

	remote := []string{
		"", "evil.example.com", "localhost.evil.example.com",
		"127.0.0.1.evil.example.com", "10.0.0.5", "8.8.8.8:80",
		"notlocalhost",
	}
	for _, h := range remote {
		if isLoopbackHost(h) {
			t.Errorf("isLoopbackHost(%q) = true, want false", h)
		}
	}
}
